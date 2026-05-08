package position

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
	"github.com/arch-portfolio-lab/portfoliolab/internal/market"
)

// PositionRepository defines the data access interface for positions.
type PositionRepository interface {
	CreatePosition(ctx context.Context, p *Position) error
	CreateLot(ctx context.Context, l *Lot) error
	CreateConsumption(ctx context.Context, c *LotConsumption) error
	GetOpenPositions(ctx context.Context, accountID int64, limit, offset int) ([]Position, error)
	GetClosedPositions(ctx context.Context, accountID int64, limit, offset int) ([]Position, error)
	GetLotByLotID(ctx context.Context, lotID string) (*Lot, error)
	GetConsumptionsBySellLot(ctx context.Context, sellLotID string) ([]LotConsumption, error)
	DeleteAllForAccount(ctx context.Context, accountID int64) error
	Recalculate(ctx context.Context, accountID int64, result *CalculateResult) error
}

// TransactionRepository defines the data access interface for transactions
// needed by the position service.
type TransactionRepository interface {
	ListAllTransactionsByAccount(ctx context.Context, accountID int64) ([]transaction.Transaction, error)
}

// AccountChecker defines the interface for checking account existence.
type AccountChecker interface {
	AccountExists(ctx context.Context, id int64) bool
}

// PortfolioChecker defines the interface for checking portfolio existence.
type PortfolioChecker interface {
	PortfolioExists(ctx context.Context, id int64) bool
}

// AccountLister defines the interface for listing accounts.
type AccountLister interface {
	GetAllAccounts(ctx context.Context) ([]AccountRef, error)
	GetAccountsByPortfolio(ctx context.Context, portfolioID int64) ([]AccountRef, error)
}

// AccountRef holds minimal account info for position queries.
type AccountRef struct {
	ID               int64
	Name             string
	PortfolioID      int64
	PortfolioCurrency string
}

// PortfolioCurrencyChecker returns the base currency of a portfolio.
type PortfolioCurrencyChecker interface {
	GetPortfolioCurrency(ctx context.Context, portfolioID int64) (string, error)
}

// --- Service errors ---

var (
	// ErrAccountNotFound indicates the referenced account does not exist.
	ErrAccountNotFound = &PositionError{Code: "account_not_found", Message: "account not found"}

	// ErrPortfolioNotFound indicates the referenced portfolio does not exist.
	ErrPortfolioNotFound = &PositionError{Code: "portfolio_not_found", Message: "portfolio not found"}
)

// Service handles position business logic: recalculation and queries.
type Service struct {
	positions               PositionRepository
	transactions            TransactionRepository
	accounts                AccountChecker
	portfolios              PortfolioChecker
	accountLister           AccountLister
	portfolioCurrencyChecker PortfolioCurrencyChecker
	fxProvider              FxRateProvider
}

// NewService creates a new position service.
func NewService(
	positions PositionRepository,
	transactions TransactionRepository,
	accounts AccountChecker,
	portfolios PortfolioChecker,
	accountLister AccountLister,
	portfolioCurrencyChecker PortfolioCurrencyChecker,
	fxProvider FxRateProvider,
) *Service {
	return &Service{
		positions:                positions,
		transactions:             transactions,
		accounts:                 accounts,
		portfolios:               portfolios,
		accountLister:            accountLister,
		portfolioCurrencyChecker: portfolioCurrencyChecker,
		fxProvider:               fxProvider,
	}
}

// RecalculateAccount fetches all transactions for the account, runs the
// position calculator, converts P&L to portfolio base currency, and persists
// the results (delete old, insert new) within a single database transaction.
func (s *Service) RecalculateAccount(ctx context.Context, accountID int64) error {
	if !s.accounts.AccountExists(ctx, accountID) {
		return ErrAccountNotFound
	}

	txns, err := s.transactions.ListAllTransactionsByAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("list transactions for account %d: %w", accountID, err)
	}

	result, err := CalculatePositions(ctx, accountID, txns)
	if err != nil {
		return fmt.Errorf("calculate positions for account %d: %w", accountID, err)
	}

	// Convert P&L to portfolio base currency.
	if s.portfolioCurrencyChecker != nil && s.fxProvider != nil {
		accounts, listErr := s.accountLister.GetAllAccounts(ctx)
		if listErr != nil {
			return fmt.Errorf("list accounts for FX conversion: %w", listErr)
		}
		baseCurrency := s.getBaseCurrencyForAccount(accounts, accountID)
		if baseCurrency != "" {
			s.convertPnlToBase(ctx, result, baseCurrency)
		}
	}

	return s.positions.Recalculate(ctx, accountID, result)
}

// getBaseCurrencyForAccount looks up the portfolio base currency for an account.
func (s *Service) getBaseCurrencyForAccount(accounts []AccountRef, accountID int64) string {
	for _, a := range accounts {
		if a.ID == accountID {
			return a.PortfolioCurrency
		}
	}
	return ""
}

// convertPnlToBase converts realized P&L for all positions in the result
// from their transaction currency to the portfolio base currency.
func (s *Service) convertPnlToBase(ctx context.Context, result *CalculateResult, baseCurrency string) {
	allPositions := append(append(result.OpenPositions, result.ClosedPositions...), result.CashPositions...)
	for i := range allPositions {
		p := &allPositions[i]
		// Skip cash positions — they are already in the cash currency.
		if isCashPosition(p.Symbol) {
			continue
		}

		pair := BuildFxPair(p.Currency, baseCurrency)
		if pair == "" {
			// Same currency as base — no conversion needed.
			continue
		}

		// Determine the date to use for the FX rate.
		var date time.Time
		if p.CloseDate != nil {
			date = *p.CloseDate
		} else {
			date = p.OpenDate
		}

		// Get the FX rate.
		var rate *market.FxRate
		var isFallback bool
		if p.IsClosed {
			rate, isFallback = s.fxProvider.GetRateForDate(ctx, pair, date)
		} else {
			// For open positions, use current spot rate.
			var found bool
			rate, found = s.fxProvider.GetCurrentRate(ctx, pair)
			if !found {
				// Try GetRateForDate as a last resort.
				rate, isFallback = s.fxProvider.GetRateForDate(ctx, pair, date)
			}
		}

		converted, rateUsed, fallback := ConvertPnlToBase(
			p.RealizedPnL, p.Currency, baseCurrency, rate, isFallback,
		)
		p.RealizedPnlBase = &converted
		p.FxRateUsed = rateUsed
		p.FxRateFallback = fallback
	}
}

// RecalculatePortfolio recalculates positions for all accounts in a portfolio.
func (s *Service) RecalculatePortfolio(ctx context.Context, portfolioID int64) error {
	if !s.portfolios.PortfolioExists(ctx, portfolioID) {
		return ErrPortfolioNotFound
	}

	accounts, err := s.accountLister.GetAccountsByPortfolio(ctx, portfolioID)
	if err != nil {
		return fmt.Errorf("list accounts for portfolio %d: %w", portfolioID, err)
	}

	for _, acc := range accounts {
		if err := s.RecalculateAccount(ctx, acc.ID); err != nil {
			return fmt.Errorf("recalculate account %d: %w", acc.ID, err)
		}
	}

	return nil
}

// RecalculateAll recalculates positions for all accounts across all portfolios.
func (s *Service) RecalculateAll(ctx context.Context) error {
	accounts, err := s.accountLister.GetAllAccounts(ctx)
	if err != nil {
		return fmt.Errorf("list all accounts: %w", err)
	}

	for _, acc := range accounts {
		if err := s.RecalculateAccount(ctx, acc.ID); err != nil {
			return fmt.Errorf("recalculate account %d: %w", acc.ID, err)
		}
	}

	return nil
}

// GetOpenPositions retrieves open positions for the given account IDs with pagination.
// Results from multiple accounts are merged and sorted by symbol, then open_date.
func (s *Service) GetOpenPositions(ctx context.Context, accountIDs []int64, limit, offset int) ([]Position, error) {
	return s.getPositions(ctx, accountIDs, limit, offset, false)
}

// GetClosedPositions retrieves closed positions for the given account IDs with pagination.
func (s *Service) GetClosedPositions(ctx context.Context, accountIDs []int64, limit, offset int) ([]Position, error) {
	return s.getPositions(ctx, accountIDs, limit, offset, true)
}

// getPositions retrieves positions (open or closed) for the given account IDs.
// Fetches from each account, merges, sorts, and applies pagination.
func (s *Service) getPositions(ctx context.Context, accountIDs []int64, limit, offset int, closed bool) ([]Position, error) {
	if len(accountIDs) == 0 {
		return []Position{}, nil
	}

	// Fetch from each account (unbounded — pagination applied after merge).
	const fetchLimit = 10000
	var all []Position
	for _, id := range accountIDs {
		var items []Position
		var err error
		if closed {
			items, err = s.positions.GetClosedPositions(ctx, id, fetchLimit, 0)
		} else {
			items, err = s.positions.GetOpenPositions(ctx, id, fetchLimit, 0)
		}
		if err != nil {
			return nil, fmt.Errorf("get positions for account %d: %w", id, err)
		}
		all = append(all, items...)
	}

	// Sort: symbol ASC, open_date ASC.
	sortPositions(all)

	// Apply pagination.
	if len(all) <= offset {
		return []Position{}, nil
	}
	all = all[offset:]
	if len(all) > limit {
		all = all[:limit]
	}

	return all, nil
}

// sortPositions sorts positions by symbol ASC, then open_date ASC.
func sortPositions(positions []Position) {
	for i := 0; i < len(positions); i++ {
		for j := i + 1; j < len(positions); j++ {
			if positions[j].Symbol < positions[i].Symbol ||
				(positions[j].Symbol == positions[i].Symbol && positions[j].OpenDate.Before(positions[i].OpenDate)) {
				positions[i], positions[j] = positions[j], positions[i]
			}
		}
	}
}

// resolveAccountIDs resolves ListFilters to a list of account IDs.
// account_id → single ID, portfolio_id → accounts in portfolio,
// account_ids → explicit list, none → all accounts.
func (s *Service) resolveAccountIDs(ctx context.Context, filters ListFilters) ([]int64, error) {
	if filters.AccountID != nil {
		return []int64{*filters.AccountID}, nil
	}
	if filters.AccountIDs != nil && len(*filters.AccountIDs) > 0 {
		return *filters.AccountIDs, nil
	}
	if filters.PortfolioID != nil {
		accounts, err := s.accountLister.GetAccountsByPortfolio(ctx, *filters.PortfolioID)
		if err != nil {
			return nil, fmt.Errorf("resolve accounts for portfolio %d: %w", *filters.PortfolioID, err)
		}
		ids := make([]int64, len(accounts))
		for i, a := range accounts {
			ids[i] = a.ID
		}
		return ids, nil
	}
	// No filter → all accounts.
	accounts, err := s.accountLister.GetAllAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all accounts: %w", err)
	}
	ids := make([]int64, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	return ids, nil
}

// GetOpenPositionsFiltered retrieves open positions with filter resolution.
func (s *Service) GetOpenPositionsFiltered(ctx context.Context, filters ListFilters, limit, offset int) ([]Position, error) {
	accountIDs, err := s.resolveAccountIDs(ctx, filters)
	if err != nil {
		return nil, err
	}
	return s.GetOpenPositions(ctx, accountIDs, limit, offset)
}

// GetClosedPositionsFiltered retrieves closed positions with filter resolution.
func (s *Service) GetClosedPositionsFiltered(ctx context.Context, filters ListFilters, limit, offset int) ([]Position, error) {
	accountIDs, err := s.resolveAccountIDs(ctx, filters)
	if err != nil {
		return nil, err
	}
	return s.GetClosedPositions(ctx, accountIDs, limit, offset)
}

// GetLotDetails retrieves a lot with its consumptions.
func (s *Service) GetLotDetails(ctx context.Context, lotID string) (*LotWithDetails, error) {
	lot, err := s.positions.GetLotByLotID(ctx, lotID)
	if err != nil {
		return nil, err
	}

	consumptions, err := s.positions.GetConsumptionsBySellLot(ctx, lotID)
	if err != nil {
		return nil, fmt.Errorf("get consumptions for lot %s: %w", lotID, err)
	}

	return &LotWithDetails{
		Lot:          *lot,
		Consumptions: consumptions,
	}, nil
}

// GetLotInfo returns the minimal lot metadata for the LotChecker interface.
func (s *Service) GetLotInfo(ctx context.Context, lotID string) (*transaction.LotInfo, error) {
	lot, err := s.positions.GetLotByLotID(ctx, lotID)
	if err != nil {
		return nil, err
	}

	return &transaction.LotInfo{
		AccountID: lot.AccountID,
		Symbol:    lot.Symbol,
		LotType:   lot.LotType,
	}, nil
}

// isCashPosition returns true if the symbol represents a cash position.
func isCashPosition(symbol string) bool {
	return strings.HasPrefix(symbol, "$CASH-")
}
