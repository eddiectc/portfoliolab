package ibkrimport

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/brokerimport"
	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

// Type aliases for domain interfaces (defined in brokerimport).
type (
	SymbolResolver     = brokerimport.SymbolResolver
	DuplicateChecker   = brokerimport.DuplicateChecker
	TransactionCreator = brokerimport.TransactionCreator
	AccountChecker     = brokerimport.AccountChecker
	SymbolCreator      = brokerimport.SymbolCreator
	BrokerSymbolAdder  = brokerimport.BrokerSymbolAdder
)

// ---- Service errors ----

var (
	// ErrAccountNotFound indicates the target account does not exist.
	ErrAccountNotFound = fmt.Errorf("account not found")

	// ErrInvalidXML indicates the XML data could not be parsed.
	ErrInvalidXML = fmt.Errorf("invalid XML data")

	// ErrNoImportableTransactions indicates the preview produced zero importable transactions.
	ErrNoImportableTransactions = fmt.Errorf("no importable transactions")
)

const (
	// externalSystem is the broker identifier used for duplicate detection.
	externalSystem = "IBKR"

	// cashSymbolPrefix is the prefix for auto-created cash symbols.
	cashSymbolPrefix = "$CASH-"
)

// PositionRecalculator defines the interface for triggering position
// recalculation after transaction mutations.
type PositionRecalculator interface {
	RecalculateAccount(ctx context.Context, accountID int64) error
}

// ---- Service ----

// Service orchestrates IBKR Flex XML import: preview generation and
// transactional import. It depends on injected interfaces for symbol
// resolution, duplicate detection, transaction creation, and account
// verification.
type Service struct {
	resolver       SymbolResolver
	dupCheck       DuplicateChecker
	creator        TransactionCreator
	accounts       AccountChecker
	symCreate      SymbolCreator
	brokerAdd      BrokerSymbolAdder
	positionRecalc PositionRecalculator
	logger         *slog.Logger
}

// ServiceOption configures the import service.
type ServiceOption func(*Service)

// WithPositionRecalculator sets the position recalculator dependency.
func WithPositionRecalculator(recalc PositionRecalculator) ServiceOption {
	return func(s *Service) { s.positionRecalc = recalc }
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) ServiceOption {
	return func(s *Service) { s.logger = logger }
}

// NewService creates a new import service.
func NewService(resolver SymbolResolver, dupCheck DuplicateChecker, creator TransactionCreator,
	accounts AccountChecker, symCreate SymbolCreator, brokerAdd BrokerSymbolAdder, opts ...ServiceOption,
) *Service {
	s := &Service{
		resolver:  resolver,
		dupCheck:  dupCheck,
		creator:   creator,
		accounts:  accounts,
		symCreate: symCreate,
		brokerAdd: brokerAdd,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Preview parses the XML and returns a categorized preview of transactions
// that would be imported. It checks symbol resolution and duplicate status
// for each record.
func (s *Service) Preview(ctx context.Context, xmlData []byte, accountID int64) (*PreviewResponse, error) {
	if !s.accounts.AccountExists(ctx, accountID) {
		return nil, ErrAccountNotFound
	}

	report, err := ParseXML(xmlData)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidXML, err)
	}

	var importable []PreviewTransaction
	var skipped []SkippedTransaction
	var errored []ErroredTransaction

	for _, trade := range report.Trades {
		typ, entry, skip := s.processTrade(ctx, trade)
		if skip != nil {
			skipped = append(skipped, *skip)
			continue
		}
		// Check duplicate
		// For FX trades, check composite references (fx_withdrawal / fx_deposit)
		// For other trades, check the raw transaction ID
		isDuplicate := false
		if typ == "fx" {
			withdrawalRef := trade.TransactionID + "_fx_withdrawal"
			depositRef := trade.TransactionID + "_fx_deposit"
			isDuplicate = s.dupCheck.ExternalReferenceExists(ctx, externalSystem, withdrawalRef) ||
				s.dupCheck.ExternalReferenceExists(ctx, externalSystem, depositRef)
		} else {
			isDuplicate = s.dupCheck.ExternalReferenceExists(ctx, externalSystem, trade.TransactionID)
		}
		if isDuplicate {
			skipped = append(skipped, SkippedTransaction{
				ExternalReference: trade.TransactionID,
				Reason:            "duplicate — already imported",
				Date:              trade.TradeDate,
				Type:              typ,
				Symbol:            trade.Symbol,
				Quantity:          trade.Quantity,
				Price:             trade.TradePrice,
				Currency:          trade.Currency,
				NetCash:           trade.NetCash,
				Description:       trade.Description,
			})
			continue
		}
		// For FX trades, entry contains two entries
		if typ == "fx" {
			importable = append(importable, entry.fxEntries...)
		} else {
			importable = append(importable, entry.single)
		}
	}

	for _, ct := range report.CashTransactions {
		entry, skip := s.processCashTransaction(ctx, ct)
		if skip != nil {
			skipped = append(skipped, *skip)
			continue
		}
		if s.dupCheck.ExternalReferenceExists(ctx, externalSystem, ct.TransactionID) {
			skipped = append(skipped, SkippedTransaction{
				ExternalReference: ct.TransactionID,
				Reason:            "duplicate — already imported",
				Date:              ct.ReportDate,
				Type:              entry.Type,
				Symbol:            entry.Symbol,
				Quantity:          entry.Quantity,
				Price:             entry.Price,
				Currency:          entry.Currency,
				NetCash:           entry.NetCash,
				Description:       entry.Description,
			})
			continue
		}
		importable = append(importable, *entry)
	}

	for _, tr := range report.Transfers {
		entry := s.processTransfer(tr)
		if s.dupCheck.ExternalReferenceExists(ctx, externalSystem, tr.TransactionID) {
			trTyp := classifyTransfer(tr)
			skipped = append(skipped, SkippedTransaction{
				ExternalReference: tr.TransactionID,
				Reason:            "duplicate — already imported",
				Date:              tr.Date,
				Type:              trTyp,
				Symbol:            cashSymbol(tr.Currency),
				Quantity:          tr.CashTransfer,
				Price:             "1",
				Currency:          tr.Currency,
				NetCash:           tr.CashTransfer,
				Description:       tr.Description,
			})
			continue
		}
		importable = append(importable, *entry)
	}

	if importable == nil {
		importable = []PreviewTransaction{}
	}
	if skipped == nil {
		skipped = []SkippedTransaction{}
	}
	if errored == nil {
		errored = []ErroredTransaction{}
	}

	return &PreviewResponse{
		Importable:      importable,
		Skipped:         skipped,
		Errored:         errored,
		ImportableCount: len(importable),
		SkippedCount:    len(skipped),
		ErroredCount:    len(errored),
	}, nil
}

// ConfirmImport re-parses the XML and creates all importable transactions
// within a single database transaction. Symbols are re-resolved and
// duplicates are re-checked (data may have changed since preview).
func (s *Service) ConfirmImport(ctx context.Context, xmlData []byte, accountID int64) (*ImportResult, error) {
	if !s.accounts.AccountExists(ctx, accountID) {
		return nil, ErrAccountNotFound
	}

	report, err := ParseXML(xmlData)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidXML, err)
	}

	var txns []*transaction.Transaction
	skippedCount := 0
	now := time.Now()
	extSys := externalSystem

	for _, trade := range report.Trades {
		typ, entries, skip := s.buildTradeTxns(ctx, trade, accountID, now, &extSys)
		if skip {
			skippedCount++
			continue
		}
		if typ == "fx" {
			txns = append(txns, entries.fxTxns...)
		} else {
			txns = append(txns, entries.singleTxn)
		}
	}

	for _, ct := range report.CashTransactions {
		txn, skip := s.buildCashTxn(ctx, ct, accountID, now, &extSys)
		if skip {
			skippedCount++
			continue
		}
		txns = append(txns, txn)
	}

	for _, tr := range report.Transfers {
		txn, skip := s.buildTransferTxn(ctx, tr, accountID, now, &extSys)
		if skip {
			skippedCount++
			continue
		}
		txns = append(txns, txn)
	}

	if len(txns) == 0 {
		return &ImportResult{
			CreatedCount: 0,
			SkippedCount: skippedCount,
		}, nil
	}

	if err := s.creator.BatchCreate(ctx, txns); err != nil {
		return nil, fmt.Errorf("batch create transactions: %w", err)
	}

	// Trigger position recalculation for the affected account.
	if s.positionRecalc != nil {
		if err := s.positionRecalc.RecalculateAccount(ctx, accountID); err != nil {
			if s.logger != nil {
				s.logger.Warn("position recalculation after import failed", "account_id", accountID, "error", err)
			}
		}
	}

	return &ImportResult{
		CreatedCount: len(txns),
		SkippedCount: skippedCount,
	}, nil
}

// CreateSymbol creates a new symbol mapping.
func (s *Service) CreateSymbol(ctx context.Context, internalSymbol, marketDataSymbol string) error {
	return s.symCreate.CreateSymbol(ctx, internalSymbol, marketDataSymbol)
}

// AddBrokerSymbolMapping adds a broker symbol to an existing internal symbol.
func (s *Service) AddBrokerSymbolMapping(ctx context.Context, brokerName, brokerSymbol, internalSymbol string) error {
	return s.brokerAdd.AddBrokerSymbolMapping(ctx, brokerName, brokerSymbol, internalSymbol)
}

// ---- Trade processing ----

type tradeResult struct {
	single    PreviewTransaction
	fxEntries []PreviewTransaction
	singleTxn *transaction.Transaction
	fxTxns    []*transaction.Transaction
}

func (s *Service) processTrade(ctx context.Context, trade Trade) (string, tradeResult, *SkippedTransaction) {
	// FX trade (assetCategory=CASH) — handled before instrument check
	if trade.AssetCategory == "CASH" {
		return "fx", s.buildFXPreview(trade, ""), nil
	}

	// Classify instrument type
	if !isSupportedTrade(trade) {
		return "", tradeResult{}, &SkippedTransaction{
			ExternalReference: trade.TransactionID,
			Reason:            "unsupported instrument type",
			Date:              trade.TradeDate,
			Type:              classifyTrade(trade),
			Symbol:            trade.Symbol,
			Quantity:          trade.Quantity,
			Price:             trade.TradePrice,
			Currency:          trade.Currency,
			NetCash:           trade.NetCash,
			Description:       trade.Description,
		}
	}

	// Resolve symbol
	internalSymbol := s.resolveSymbol(ctx, trade.Symbol)
	if internalSymbol == "" {
		return "", tradeResult{}, &SkippedTransaction{
			ExternalReference: trade.TransactionID,
			Reason:            "unmapped symbol: " + trade.Symbol,
			BrokerSymbol:      trade.Symbol,
			Date:              trade.TradeDate,
			Type:              classifyTrade(trade),
			Symbol:            trade.Symbol,
			Quantity:          trade.Quantity,
			Price:             trade.TradePrice,
			Currency:          trade.Currency,
			NetCash:           trade.NetCash,
			Description:       trade.Description,
		}
	}

	typ := classifyTrade(trade)
	return typ, tradeResult{
		single: PreviewTransaction{
			Date:              trade.TradeDate,
			Type:              typ,
			Symbol:            internalSymbol,
			Quantity:          trade.Quantity,
			Price:             trade.TradePrice,
			Currency:          trade.Currency,
			NetCash:           trade.NetCash,
			ExternalReference: trade.TransactionID,
			Description:       trade.Description,
		},
	}, nil
}

func (s *Service) buildTradeTxns(ctx context.Context, trade Trade, accountID int64, now time.Time, extSys *string) (string, tradeResult, bool) {
	// FX trade (assetCategory=CASH) — handled before instrument check
	if trade.AssetCategory == "CASH" {
		// Check composite references for FX trades
		withdrawalRef := trade.TransactionID + "_fx_withdrawal"
		depositRef := trade.TransactionID + "_fx_deposit"
		if s.dupCheck.ExternalReferenceExists(ctx, *extSys, withdrawalRef) ||
			s.dupCheck.ExternalReferenceExists(ctx, *extSys, depositRef) {
			return "", tradeResult{}, true
		}
		txns := s.buildFXTxns(trade, accountID, now, extSys)
		return "fx", tradeResult{fxTxns: txns}, false
	}

	if !isSupportedTrade(trade) {
		return "", tradeResult{}, true
	}

	internalSymbol := s.resolveSymbol(ctx, trade.Symbol)
	if internalSymbol == "" {
		return "", tradeResult{}, true
	}

	if s.dupCheck.ExternalReferenceExists(ctx, *extSys, trade.TransactionID) {
		return "", tradeResult{}, true
	}

	typ := classifyTrade(trade)
	qty, _ := decimal.Parse(trade.Quantity)
	price, _ := decimal.Parse(trade.TradePrice)
	netCash, _ := decimal.Parse(trade.NetCash)
	date := parseDate(trade.TradeDate)

	// Generate lot_id from IBKR order ID so partial fills of the same
	// order share one lot. Format: LOT-IBKR-<ibOrderID>.
	lotID := "LOT-IBKR-" + trade.IbOrderID

	return typ, tradeResult{
		singleTxn: &transaction.Transaction{
			AccountID:         accountID,
			Date:              date,
			Type:              typ,
			Symbol:            internalSymbol,
			Quantity:          qty,
			Price:             price,
			Currency:          trade.Currency,
			NetCash:           netCash,
			Description:       toStringPtr(trade.Description),
			LotID:             &lotID,
			ExternalSystem:    extSys,
			ExternalReference: &trade.TransactionID,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}, false
}

func (s *Service) buildFXPreview(trade Trade, _ string) tradeResult {
	// FX symbol format: {currencyA}.{currencyB} (e.g. GBP.USD)
	// The target currency is whichever currency in the pair is NOT the trade currency.
	// The trade currency (trade.Currency) is the one you pay/receive in proceeds.
	parts := strings.SplitN(trade.Symbol, ".", 2)
	targetCurrency := trade.Currency // fallback
	if len(parts) == 2 {
		if parts[0] != trade.Currency {
			targetCurrency = parts[0]
		} else {
			targetCurrency = parts[1]
		}
	}
	// Use raw Proceeds (already negative for FX withdrawal leg)
	// so preview matches the stored transaction values.

	return tradeResult{
		fxEntries: []PreviewTransaction{
			{
				Date:              trade.TradeDate,
				Type:              "withdrawal",
				Symbol:            cashSymbol(trade.Currency),
				Quantity:          trade.Proceeds,
				Price:             "1",
				Currency:          trade.Currency,
				NetCash:           trade.Proceeds,
				ExternalReference: trade.TransactionID + "_fx_withdrawal",
				Description:       trade.Description + " " + trade.TradePrice,
			},
			{
				Date:              trade.TradeDate,
				Type:              "deposit",
				Symbol:            cashSymbol(targetCurrency),
				Quantity:          trade.Quantity,
				Price:             "1",
				Currency:          targetCurrency,
				NetCash:           trade.Quantity,
				ExternalReference: trade.TransactionID + "_fx_deposit",
				Description:       trade.Description + " " + trade.TradePrice,
			},
		},
	}
}

func (s *Service) buildFXTxns(trade Trade, accountID int64, now time.Time, extSys *string) []*transaction.Transaction {
	// FX symbol format: {currencyA}.{currencyB} (e.g. GBP.USD)
	// The target currency is whichever currency in the pair is NOT the trade currency.
	parts := strings.SplitN(trade.Symbol, ".", 2)
	targetCurrency := trade.Currency // fallback
	if len(parts) == 2 {
		if parts[0] != trade.Currency {
			targetCurrency = parts[0]
		} else {
			targetCurrency = parts[1]
		}
	}
	proceeds, _ := decimal.Parse(absStr(trade.Proceeds))
	qty, _ := decimal.Parse(trade.Quantity)
	date := parseDate(trade.TradeDate)

	withdrawalRef := trade.TransactionID + "_fx_withdrawal"
	depositRef := trade.TransactionID + "_fx_deposit"
	fxDesc := trade.Description + " " + trade.TradePrice

	return []*transaction.Transaction{
		{
			AccountID:         accountID,
			Date:              date,
			Type:              "withdrawal",
			Symbol:            cashSymbol(trade.Currency),
			Quantity:          proceeds.Neg(),
			Price:             decimal.One,
			Currency:          trade.Currency,
			NetCash:           proceeds.Neg(),
			Description:       toStringPtr(fxDesc),
			ExternalSystem:    extSys,
			ExternalReference: &withdrawalRef,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			AccountID:         accountID,
			Date:              date,
			Type:              "deposit",
			Symbol:            cashSymbol(targetCurrency),
			Quantity:          qty,
			Price:             decimal.One,
			Currency:          targetCurrency,
			NetCash:           qty,
			Description:       toStringPtr(fxDesc),
			ExternalSystem:    extSys,
			ExternalReference: &depositRef,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}
}

// ---- Cash transaction processing ----

func (s *Service) processCashTransaction(ctx context.Context, ct CashTransaction) (*PreviewTransaction, *SkippedTransaction) {
	typ, needsSymbol := classifyCashTransaction(ct)

	var symbol string
	if needsSymbol {
		symbol = s.resolveSymbol(ctx, ct.Symbol)
		if symbol == "" {
			typ, _ := classifyCashTransaction(ct)
			return nil, &SkippedTransaction{
				ExternalReference: ct.TransactionID,
				Reason:            "unmapped symbol: " + ct.Symbol,
				BrokerSymbol:      ct.Symbol,
				Date:              ct.ReportDate,
				Type:              typ,
				Symbol:            ct.Symbol,
				Quantity:          ct.Amount,
				Price:             "1",
				Currency:          ct.Currency,
				NetCash:           ct.Amount,
				Description:       ct.Description,
			}
		}
	} else {
		symbol = cashSymbol(ct.Currency)
	}

	// Use raw amount (signed) so preview matches the stored transaction.
	// Deposits are positive, withdrawals are negative.
	amount := ct.Amount

	return &PreviewTransaction{
		Date:              ct.ReportDate,
		Type:              typ,
		Symbol:            symbol,
		Quantity:          amount,
		Price:             "1",
		Currency:          ct.Currency,
		NetCash:           amount,
		ExternalReference: ct.TransactionID,
		Description:       ct.Description,
	}, nil
}

func (s *Service) buildCashTxn(ctx context.Context, ct CashTransaction, accountID int64, now time.Time, extSys *string) (*transaction.Transaction, bool) {
	typ, needsSymbol := classifyCashTransaction(ct)

	var symbol string
	if needsSymbol {
		symbol = s.resolveSymbol(ctx, ct.Symbol)
		if symbol == "" {
			return nil, true
		}
	} else {
		symbol = cashSymbol(ct.Currency)
	}

	if s.dupCheck.ExternalReferenceExists(ctx, *extSys, ct.TransactionID) {
		return nil, true
	}

	amount, _ := decimal.Parse(ct.Amount)
	date := parseDate(ct.ReportDate)

	return &transaction.Transaction{
		AccountID:         accountID,
		Date:              date,
		Type:              typ,
		Symbol:            symbol,
		Quantity:          amount,
		Price:             decimal.One,
		Currency:          ct.Currency,
		NetCash:           amount,
		Description:       toStringPtr(ct.Description),
		ExternalSystem:    extSys,
		ExternalReference: &ct.TransactionID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, false
}

// ---- Transfer processing ----

func (s *Service) processTransfer(tr Transfer) *PreviewTransaction {
	typ := classifyTransfer(tr)

	// Use raw cashTransfer (signed) so preview matches the stored transaction.
	// IN (deposit) is positive, OUT (withdrawal) is negative.
	amount := tr.CashTransfer

	return &PreviewTransaction{
		Date:              tr.Date,
		Type:              typ,
		Symbol:            cashSymbol(tr.Currency),
		Quantity:          amount,
		Price:             "1",
		Currency:          tr.Currency,
		NetCash:           amount,
		ExternalReference: tr.TransactionID,
		Description:       tr.Description,
	}
}

func (s *Service) buildTransferTxn(ctx context.Context, tr Transfer, accountID int64, now time.Time, extSys *string) (*transaction.Transaction, bool) {
	typ := classifyTransfer(tr)

	if s.dupCheck.ExternalReferenceExists(ctx, *extSys, tr.TransactionID) {
		return nil, true
	}

	amount, _ := decimal.Parse(tr.CashTransfer)
	date := parseDate(tr.Date)

	netCash := amount

	return &transaction.Transaction{
		AccountID:         accountID,
		Date:              date,
		Type:              typ,
		Symbol:            cashSymbol(tr.Currency),
		Quantity:          amount,
		Price:             decimal.One,
		Currency:          tr.Currency,
		NetCash:           netCash,
		Description:       toStringPtr(tr.Description),
		ExternalSystem:    extSys,
		ExternalReference: &tr.TransactionID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, false
}

// ---- Classification helpers ----

// classifyTrade maps BuySell to transaction type.
func classifyTrade(trade Trade) string {
	switch trade.BuySell {
	case "BUY":
		return "buy"
	case "SELL":
		return "sell"
	default:
		return "buy" // fallback
	}
}

// classifyCashTransaction maps CashTransaction.Type to transaction type.
// Returns (type, needsSymbol) where needsSymbol is true for dividends.
func classifyCashTransaction(ct CashTransaction) (string, bool) {
	switch ct.Type {
	case "Dividends":
		return "dividend", true
	case "Withholding Tax":
		return "tax", false
	case "Broker Interest Received":
		return "interest", false
	case "Other Fees":
		return "fee", false
	default:
		// Deposits/Withdrawals or unknown — determine by amount sign
		if ct.Amount == "" || ct.Amount == "0" {
			return "deposit", false
		}
		// Check if amount is negative
		if strings.HasPrefix(ct.Amount, "-") {
			return "withdrawal", false
		}
		return "deposit", false
	}
}

// classifyTransfer maps Transfer direction to transaction type.
func classifyTransfer(tr Transfer) string {
	switch tr.Direction {
	case "IN":
		return "deposit"
	case "OUT":
		return "withdrawal"
	default:
		// Fallback: check cashTransfer sign
		if strings.HasPrefix(tr.CashTransfer, "-") {
			return "withdrawal"
		}
		return "deposit"
	}
}

// isSupportedTrade checks if the trade has a supported instrument type.
func isSupportedTrade(trade Trade) bool {
	if trade.AssetCategory != "STK" {
		return false
	}
	return trade.SubCategory == "COMMON" || trade.SubCategory == "ETF"
}

// resolveSymbol tries broker symbol map first, then direct internal symbol match.
func (s *Service) resolveSymbol(ctx context.Context, brokerSymbol string) string {
	// Check broker symbol map first
	internal := s.resolver.ResolveBrokerSymbol(ctx, externalSystem, brokerSymbol)
	if internal != "" {
		return internal
	}
	// Fall back to direct match
	if s.resolver.SymbolExists(ctx, brokerSymbol) {
		return brokerSymbol
	}
	return ""
}

// cashSymbol returns the $CASH-{currency} symbol for a given currency.
func cashSymbol(currency string) string {
	return cashSymbolPrefix + currency
}

// absStr removes a leading '-' from a string (for display purposes).
func absStr(s string) string {
	return strings.TrimPrefix(s, "-")
}

// parseDate parses a date string in YYYYMMDD or YYYY-MM-DD format.
// IBKR Flex XML uses YYYYMMDD without dashes.
func parseDate(s string) time.Time {
	if t, err := time.Parse("20060102", s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	return time.Time{}
}

// toStringPtr returns a pointer to s if non-empty, nil otherwise.
func toStringPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
