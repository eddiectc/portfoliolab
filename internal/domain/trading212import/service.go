package trading212import

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/govalues/decimal"
	"github.com/oklog/ulid/v2"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
)

// ---- Service errors ----

var (
	// ErrAccountNotFound indicates the target account does not exist.
	ErrAccountNotFound = fmt.Errorf("account not found")

	// ErrInvalidCSV indicates the CSV data could not be parsed.
	ErrInvalidCSV = fmt.Errorf("invalid CSV data")
)

const (
	// externalSystem is the broker identifier used for duplicate detection.
	externalSystem = "Trading212"

	// brokerName is the broker name used for symbol resolution.
	brokerName = "Trading212"

	// cashSymbolPrefix is the prefix for auto-created cash symbols.
	cashSymbolPrefix = "$CASH-"
)

// PositionRecalculator defines the interface for triggering position
// recalculation after transaction mutations.
type PositionRecalculator interface {
	RecalculateAccount(ctx context.Context, accountID int64) error
}

// ---- Service ----

// Service orchestrates Trading 212 CSV import: preview generation and
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

// Preview parses the CSV and returns a categorized preview of transactions
// that would be imported. It checks symbol resolution and duplicate status
// for each record.
func (s *Service) Preview(ctx context.Context, csvData []byte, accountID int64) (*PreviewResponse, error) {
	if !s.accounts.AccountExists(ctx, accountID) {
		return nil, ErrAccountNotFound
	}

	report, err := ParseCSV(csvData)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCSV, err)
	}

	var importable []PreviewTransaction
	var skipped []SkippedTransaction
	var errored []ErroredTransaction

	for _, row := range report.Rows {
		entry, skip := s.processRow(ctx, row)
		if skip != nil {
			skipped = append(skipped, *skip)
			continue
		}

		// Check duplicate
		if s.dupCheck.ExternalReferenceExists(ctx, externalSystem, row.ID) {
			skipped = append(skipped, SkippedTransaction{
				ExternalReference: row.ID,
				Reason:            "duplicate — already imported",
				Date:              entry.Date,
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

	// Ensure non-nil slices for JSON serialization consistency.
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

// ConfirmImport re-parses the CSV and creates all importable transactions
// within a single database transaction. Symbols are re-resolved and
// duplicates are re-checked (data may have changed since preview).
func (s *Service) ConfirmImport(ctx context.Context, csvData []byte, accountID int64) (*ImportResult, error) {
	if !s.accounts.AccountExists(ctx, accountID) {
		return nil, ErrAccountNotFound
	}

	report, err := ParseCSV(csvData)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCSV, err)
	}

	var txns []*transaction.Transaction
	skippedCount := 0
	now := time.Now()

	for _, row := range report.Rows {
		txn, skip := s.buildTxn(ctx, row, accountID, now)
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
func (s *Service) AddBrokerSymbolMapping(ctx context.Context, brokerNameArg, brokerSymbol, internalSymbol string) error {
	return s.brokerAdd.AddBrokerSymbolMapping(ctx, brokerNameArg, brokerSymbol, internalSymbol)
}

// ---- Row processing ----

// processRow classifies a parsed CSV row and returns a preview entry,
// a skip reason, or an error.
func (s *Service) processRow(ctx context.Context, row ParsedRow) (*PreviewTransaction, *SkippedTransaction) {
	// Unknown actions are skipped.
	if row.Action == ActionUnknown {
		return nil, &SkippedTransaction{
			ExternalReference: row.ID,
			Reason:            "unsupported action type",
			Date:              row.Date,
			Type:              "unknown",
			Description:       row.Name,
		}
	}

	// Trades: resolve symbol
	if isTradeAction(row.Action) {
		txnType := actionToTxnType(row.Action)
		internalSymbol := s.resolveSymbol(ctx, row.Ticker)
		if internalSymbol == "" {
			return nil, &SkippedTransaction{
				ExternalReference: row.ID,
				Reason:            "unmapped symbol: " + row.Ticker,
				BrokerSymbol:      row.Ticker,
				Date:              row.Date,
				Type:              txnType,
				Symbol:            row.Ticker,
				Quantity:          row.Quantity,
				Price:             row.Price,
				Currency:          row.Currency,
				NetCash:           computeNetCashDisplay(row),
				Description:       row.Name,
			}
		}

		return &PreviewTransaction{
			Date:              row.Date,
			Type:              txnType,
			Symbol:            internalSymbol,
			Quantity:          row.Quantity,
			Price:             row.Price,
			Currency:          row.Currency,
			NetCash:           computeNetCashDisplay(row),
			ExternalReference: row.ID,
			Description:       row.Name,
		}, nil
	}

	// Cash transactions: deposit, withdrawal, interest
	txnType := actionToTxnType(row.Action)
	currency := row.TotalCurrency
	if currency == "" {
		currency = row.Currency
	}
	symbol := cashSymbol(currency)

	// Net cash: deposits and interest are inflows (positive), withdrawals are outflows (negative).
	netCash := row.Total
	if txnType == "withdrawal" {
		netCash = "-" + row.Total
	}

	return &PreviewTransaction{
		Date:              row.Date,
		Type:              txnType,
		Symbol:            symbol,
		Quantity:          row.Total,
		Price:             "1",
		Currency:          currency,
		NetCash:           netCash,
		ExternalReference: row.ID,
		Description:       row.Name,
	}, nil
}

// buildTxn builds a transaction for a parsed CSV row, checking symbol
// resolution and duplicates. Returns (transaction, skip).
func (s *Service) buildTxn(ctx context.Context, row ParsedRow, accountID int64, now time.Time) (*transaction.Transaction, bool) {
	// Unknown actions are skipped.
	if row.Action == ActionUnknown {
		return nil, true
	}

	txnType := actionToTxnType(row.Action)

	var symbol string
	if isTradeAction(row.Action) {
		symbol = s.resolveSymbol(ctx, row.Ticker)
		if symbol == "" {
			return nil, true
		}
	} else {
		currency := row.TotalCurrency
		if currency == "" {
			currency = row.Currency
		}
		symbol = cashSymbol(currency)
	}

	// Check duplicate
	if s.dupCheck.ExternalReferenceExists(ctx, externalSystem, row.ID) {
		return nil, true
	}

	// Parse date
	date := parseDate(row.Date)

	// Parse numeric fields
	quantity, err := decimal.Parse(row.Quantity)
	if err != nil {
		quantity = decimal.Zero
	}
	price, err := decimal.Parse(row.Price)
	if err != nil {
		price = decimal.Zero
	}
	total, err := decimal.Parse(row.Total)
	if err != nil {
		total = decimal.Zero
	}

	ref := row.ID

	if isTradeAction(row.Action) {
		netCash := computeNetCash(total, row.Action)
		// Auto-generate lot_id for each trade (no order ID in T212 data).
		lotID := generateLotID()
		return &transaction.Transaction{
			AccountID:         accountID,
			Date:              date,
			Type:              txnType,
			Symbol:            symbol,
			Quantity:          quantity,
			Price:             price,
			Currency:          row.Currency,
			NetCash:           netCash,
			Description:       toStringPtr(row.Name),
			LotID:             &lotID,
			ExternalSystem:    strPtr(externalSystem),
			ExternalReference: &ref,
			CreatedAt:         now,
			UpdatedAt:         now,
		}, false
	}

	// Cash transactions: quantity = total amount, price = 1
	netCash := total
	if txnType == "withdrawal" {
		netCash = total.Neg()
	}

	return &transaction.Transaction{
		AccountID:         accountID,
		Date:              date,
		Type:              txnType,
		Symbol:            symbol,
		Quantity:          total,
		Price:             decimal.One,
		Currency:          row.TotalCurrency,
		NetCash:           netCash,
		Description:       toStringPtr(row.Name),
		ExternalSystem:    strPtr(externalSystem),
		ExternalReference: &ref,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, false
}

// ---- Helpers ----

// isTradeAction returns true for buy/sell actions.
func isTradeAction(action string) bool {
	switch action {
	case ActionLimitBuy, ActionMarketBuy, ActionLimitSell, ActionMarketSell:
		return true
	default:
		return false
	}
}

// actionToTxnType maps a Trading 212 action constant to a transaction type.
func actionToTxnType(action string) string {
	switch action {
	case ActionLimitBuy, ActionMarketBuy:
		return "buy"
	case ActionLimitSell, ActionMarketSell:
		return "sell"
	case ActionDeposit:
		return "deposit"
	case ActionWithdrawal:
		return "withdrawal"
	case ActionInterestOnCash:
		return "interest"
	default:
		return "unknown"
	}
}

// computeNetCash computes the net cash decimal for a trade row.
// Buys: negative total (cash outflow). Sells: positive total (cash inflow).
func computeNetCash(total decimal.Decimal, action string) decimal.Decimal {
	switch action {
	case ActionLimitBuy, ActionMarketBuy:
		return total.Neg()
	default:
		// Sell actions: positive total
		return total
	}
}

// computeNetCashDisplay computes the net cash display string for a row.
// For trades: buys show negative total, sells show positive total.
// For cash: returns the total as-is (sign handled by the caller for display).
func computeNetCashDisplay(row ParsedRow) string {
	if !isTradeAction(row.Action) {
		return row.Total
	}
	if row.Action == ActionLimitBuy || row.Action == ActionMarketBuy {
		return "-" + row.Total
	}
	return row.Total
}

// resolveSymbol tries broker symbol map first, then direct internal symbol match.
func (s *Service) resolveSymbol(ctx context.Context, brokerSymbol string) string {
	// Check broker symbol map first
	internal := s.resolver.ResolveBrokerSymbol(ctx, brokerName, brokerSymbol)
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

// parseDate parses a date string in YYYY-MM-DD format.
func parseDate(s string) time.Time {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	return time.Time{}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// toStringPtr returns a pointer to s if non-empty, nil otherwise.
func toStringPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// generateLotID creates a new unique lot ID in the format LOT-<ulid>.
func generateLotID() string {
	return "LOT-" + ulid.Make().String()
}
