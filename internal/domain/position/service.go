package position

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
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
	GetSymbolsWithEarliestDate(ctx context.Context) (map[string]time.Time, error)
	GetSymbolsByOpenPositions(ctx context.Context) (map[string]time.Time, error)
	GetFxPairsByOpenPositions(ctx context.Context) (map[string]time.Time, error)
	GetEarliestDateBySymbol(ctx context.Context, symbol string) (*time.Time, error)
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

// MarketDataService abstracts market data retrieval for stock quotes,
// historical prices, and FX rates. Consumers don't know whether data comes
// from cache or a live fetch.
type MarketDataService interface {
	GetQuotes(ctx context.Context, symbols []string) map[string]*market.MarketData
	GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
	GetLatestPriceDatePerSymbol(ctx context.Context, symbols []string) map[string]*time.Time
	RefreshQuotes(ctx context.Context, symbols []string) marketservice.RefreshResult
	GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*market.FxRate, error)
	GetHistoricalFxRate(ctx context.Context, baseCurrency, quoteCurrency string, date time.Time) (*market.FxRate, error)
	RefreshFxRates(ctx context.Context, pairs []marketservice.FxPair) marketservice.FxRefreshResult
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
	positions                PositionRepository
	transactions             TransactionRepository
	accounts                 AccountChecker
	portfolios               PortfolioChecker
	accountLister            AccountLister
	portfolioCurrencyChecker PortfolioCurrencyChecker
	marketService            MarketDataService
	cacheScheduler           marketcache.MarketCacheScheduler
	logger                   *slog.Logger
}

// NewService creates a new position service.
func NewService(
	positions PositionRepository,
	transactions TransactionRepository,
	accounts AccountChecker,
	portfolios PortfolioChecker,
	accountLister AccountLister,
	portfolioCurrencyChecker PortfolioCurrencyChecker,
) *Service {
	return &Service{
		positions:                positions,
		transactions:             transactions,
		accounts:                 accounts,
		portfolios:               portfolios,
		accountLister:            accountLister,
		portfolioCurrencyChecker: portfolioCurrencyChecker,
	}
}

// WithMarketDataService sets the market data service for enriching open
// positions with market data and refreshing quotes. If nil, market data
// enrichment and refresh are disabled.
func (s *Service) WithMarketDataService(marketService MarketDataService, logger *slog.Logger) {
	s.marketService = marketService
	s.logger = logger
}

// WithMarketCache sets the market cache scheduler for triggering background
// fetches of historical prices and FX rates. If nil, cache scheduling is
// disabled (market data is only refreshed on-demand).
func (s *Service) WithMarketCache(scheduler marketcache.MarketCacheScheduler) {
	s.cacheScheduler = scheduler
}

// --- SymbolDiscoverer implementation ---

// ActiveSymbols returns symbols with open positions, keyed by symbol with the
// earliest transaction date as value. Implements marketcache.SymbolDiscoverer.
func (s *Service) ActiveSymbols(ctx context.Context) (map[string]time.Time, error) {
	return s.transactions.GetSymbolsByOpenPositions(ctx)
}

// AllSymbols returns all symbols with any transactions (open + closed), keyed
// by symbol with the earliest transaction date as value. Implements
// marketcache.SymbolDiscoverer.
func (s *Service) AllSymbols(ctx context.Context) (map[string]time.Time, error) {
	return s.transactions.GetSymbolsWithEarliestDate(ctx)
}

// ActiveFxPairs returns FX pairs needed for open positions, keyed by
// "BASE/QUOTE" with the earliest transaction date as value. Implements
// marketcache.SymbolDiscoverer.
func (s *Service) ActiveFxPairs(ctx context.Context) (map[string]time.Time, error) {
	return s.transactions.GetFxPairsByOpenPositions(ctx)
}

// --- MarketCacheScheduler passthrough ---

// ScheduleSymbolFetch delegates to the market cache scheduler to queue a
// historical price fetch for a stock symbol.
func (s *Service) ScheduleSymbolFetch(symbol string, fromDate time.Time) {
	if s.cacheScheduler != nil {
		s.cacheScheduler.ScheduleSymbolFetch(symbol, fromDate)
	}
}

// ScheduleFxPairFetch delegates to the market cache scheduler to queue a
// historical FX rate fetch for a currency pair.
func (s *Service) ScheduleFxPairFetch(baseCurrency, quoteCurrency string, fromDate time.Time) {
	if s.cacheScheduler != nil {
		s.cacheScheduler.ScheduleFxPairFetch(baseCurrency, quoteCurrency, fromDate)
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

	// Determine base currency for P&L conversion and cache scheduling.
	var baseCurrency string
	if s.portfolioCurrencyChecker != nil {
		accounts, listErr := s.accountLister.GetAllAccounts(ctx)
		if listErr != nil {
			return fmt.Errorf("list accounts for FX conversion: %w", listErr)
		}
		baseCurrency = s.getBaseCurrencyForAccount(accounts, accountID)
	}

	// Convert P&L to portfolio base currency.
	if s.marketService != nil && baseCurrency != "" {
		s.convertPnlToBase(ctx, result, baseCurrency)
	}

	if err := s.positions.Recalculate(ctx, accountID, result); err != nil {
		return err
	}

	// Schedule background market data fetches for open positions.
	s.scheduleCacheFetches(ctx, result, accountID, baseCurrency)

	return nil
}

// scheduleCacheFetches schedules background fetches for symbols and FX pairs
// found in the open positions of the calculate result. It skips cash symbols
// and uses the position's open date as the fetch start date.
func (s *Service) scheduleCacheFetches(ctx context.Context, result *CalculateResult, accountID int64, baseCurrency string) {
	if s.cacheScheduler == nil {
		return
	}

	// Collect unique non-cash symbols, keeping the earliest open date per symbol.
	symbolDates := make(map[string]time.Time)
	// Collect FX pairs (currency/baseCurrency) with earliest date.
	pairDates := make(map[string]time.Time)

	for _, p := range result.OpenPositions {
		if isCashPosition(p.Symbol) {
			continue
		}

		// Track earliest open date per symbol.
		if existing, ok := symbolDates[p.Symbol]; !ok || p.OpenDate.Before(existing) {
			symbolDates[p.Symbol] = p.OpenDate
		}

		// Track FX pair if position currency differs from base.
		if baseCurrency != "" && p.Currency != baseCurrency {
			pairKey := p.Currency + "/" + baseCurrency
			if existing, ok := pairDates[pairKey]; !ok || p.OpenDate.Before(existing) {
				pairDates[pairKey] = p.OpenDate
			}
		}
	}

	// Schedule symbol fetches.
	for sym, fromDate := range symbolDates {
		s.ScheduleSymbolFetch(sym, fromDate)
	}

	// Schedule FX pair fetches.
	for pair, fromDate := range pairDates {
		base, quote := splitFxPair(pair)
		s.ScheduleFxPairFetch(base, quote, fromDate)
	}
}

// splitFxPair splits "BASE/QUOTE" into its components.
func splitFxPair(pair string) (base, quote string) {
	for i, c := range pair {
		if c == '/' {
			return pair[:i], pair[i+1:]
		}
	}
	return pair, ""
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
	// Process each slice separately to avoid append copy issues.
	for i := range result.OpenPositions {
		s.convertPositionPnl(&result.OpenPositions[i], baseCurrency, ctx, false)
	}
	for i := range result.ClosedPositions {
		s.convertPositionPnl(&result.ClosedPositions[i], baseCurrency, ctx, true)
	}
	// Cash positions are skipped — already in cash currency.
}

// convertPositionPnl converts a single position's realized P&L to base currency
// and sets FxRateUsed / FxRateFallback.
func (s *Service) convertPositionPnl(p *Position, baseCurrency string, ctx context.Context, isClosed bool) {
	// Skip cash positions.
	if isCashPosition(p.Symbol) {
		return
	}

	// Same currency as base — no conversion needed, rate is 1.
	if p.Currency == baseCurrency {
		p.RealizedPnlBase = &p.RealizedPnL
		rate := decimal.One
		p.FxRateUsed = &rate
		p.FxRateFallback = false
		return
	}

	// Determine the date to use for the FX rate.
	var date time.Time
	if p.CloseDate != nil {
		date = *p.CloseDate
	} else {
		date = p.OpenDate
	}

	// Get the FX rate from cache (no live fetch fallback).
	var rate *market.FxRate
	var isFallback bool
	if s.marketService != nil {
		if isClosed {
			// Closed position: try historical rate for the open date.
			rate, _ = s.marketService.GetHistoricalFxRate(ctx, p.Currency, baseCurrency, date)
		} else {
			// Open position: try current spot rate.
			rate, _ = s.marketService.GetCurrentFxRate(ctx, p.Currency, baseCurrency)
		}
		if rate == nil {
			isFallback = true
		}
	}

	converted, rateUsed, fallback := ConvertPnlToBase(
		p.RealizedPnL, p.Currency, baseCurrency, rate, isFallback,
	)
	p.RealizedPnlBase = &converted
	p.FxRateUsed = rateUsed
	p.FxRateFallback = fallback
}

// ConvertPnlToBase converts realized P&L from the position currency to the
// base currency using the given FX rate. Returns the converted value, the
// rate used (nil if same currency), and whether a fallback was applied.
func ConvertPnlToBase(pnl decimal.Decimal, positionCurrency, baseCurrency string,
	rate *market.FxRate, isFallback bool) (decimal.Decimal, *decimal.Decimal, bool) {

	// Same currency — no conversion needed.
	if positionCurrency == baseCurrency {
		return pnl, nil, false
	}

	// No rate available — return original P&L as fallback.
	if rate == nil {
		return pnl, nil, true
	}

	// Convert: pnl is in positionCurrency, rate is positionCurrency/baseCurrency.
	// pnl_in_base = pnl * rate.
	converted, err := pnl.Mul(rate.Rate)
	if err != nil {
		// On decimal error, return original as fallback.
		return pnl, &rate.Rate, true
	}
	return converted, &rate.Rate, isFallback
}

// FxRateDisplay holds an FX rate formatted in standard market convention.
type FxRateDisplay struct {
	Pair string          // e.g. "GBP/USD"
	Rate decimal.Decimal // rate in convention order
}

// ConventionFxRate returns the FX rate displayed in standard market convention.
// Returns nil if currencies match, rate is nil, or rate is zero.
func ConventionFxRate(positionCurrency, baseCurrency string, storedRate *decimal.Decimal) *FxRateDisplay {
	if positionCurrency == baseCurrency || storedRate == nil || storedRate.Equal(decimal.Zero) {
		return nil
	}

	conventionPair, inverted := conventionPairOrder(positionCurrency, baseCurrency)

	var rate decimal.Decimal
	if inverted {
		// storedRate is position/base, convention is base/position → invert
		rate, _ = decimal.One.Quo(*storedRate)
	} else {
		// storedRate is already in convention order
		rate = *storedRate
	}

	return &FxRateDisplay{
		Pair: conventionPair,
		Rate: rate,
	}
}

// conventionPairOrder returns the standard market-convention pair string and
// whether the position/base order is inverted relative to convention.
//
// Convention rules:
//   - USD vs GBP/EUR/AUD/NZD/CAD → USD is quote (e.g. GBP/USD, EUR/USD)
//   - EUR vs GBP → EUR is base (e.g. EUR/GBP)
//   - otherwise → first currency is base (position/base)
func conventionPairOrder(currencyA, currencyB string) (string, bool) {
	// Currencies where USD is conventionally the quote currency.
	usdMajors := map[string]bool{
		"GBP": true, "EUR": true, "AUD": true, "NZD": true, "CAD": true,
	}

	if currencyA == "USD" && usdMajors[currencyB] {
		// Convention: GBP/USD (USD is quote). Position/base was USD/GBP → inverted.
		return fmt.Sprintf("%s/%s", currencyB, currencyA), true
	}
	if currencyB == "USD" && usdMajors[currencyA] {
		// Convention: GBP/USD (USD is quote). Position/base was GBP/USD → not inverted.
		return fmt.Sprintf("%s/%s", currencyA, currencyB), false
	}

	// EUR/GBP convention: EUR is base.
	if currencyA == "EUR" && currencyB == "GBP" {
		return "EUR/GBP", false
	}
	if currencyA == "GBP" && currencyB == "EUR" {
		return "EUR/GBP", true
	}

	// Default: position/base order.
	return fmt.Sprintf("%s/%s", currencyA, currencyB), false
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
// Fetches from each account, merges, sorts, applies pagination, and populates
// AccountName from the account lister.
func (s *Service) getPositions(ctx context.Context, accountIDs []int64, limit, offset int, closed bool) ([]Position, error) {
	if len(accountIDs) == 0 {
		return []Position{}, nil
	}

	// Build account name lookup.
	nameMap := make(map[int64]string)
	if s.accountLister != nil {
		accounts, err := s.accountLister.GetAllAccounts(ctx)
		if err == nil {
			for _, a := range accounts {
				nameMap[a.ID] = a.Name
			}
		}
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

	// Populate account names.
	for i := range all {
		if name, ok := nameMap[all[i].AccountID]; ok {
			all[i].AccountName = name
		}
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

// ClosedPositionSummary aggregates realized P&L across all closed positions.
type ClosedPositionSummary struct {
	TotalRealizedPnLB decimal.Decimal
}

// OpenPositionSummary aggregates totals across all open positions.
type OpenPositionSummary struct {
	TotalCostBasisBase  decimal.Decimal
	TotalMktValueBase   decimal.Decimal
	TotalUnrealizedPnLB decimal.Decimal
}

// GetClosedPositionsSummary computes the summary from ALL closed positions
// (not limited by pagination), respecting the given filters.
func (s *Service) GetClosedPositionsSummary(ctx context.Context, filters ListFilters, baseCurrency string) (ClosedPositionSummary, error) {
	accountIDs, err := s.resolveAccountIDs(ctx, filters)
	if err != nil {
		return ClosedPositionSummary{}, err
	}
	var summary ClosedPositionSummary
	const fetchLimit = 10000
	for _, id := range accountIDs {
		items, err := s.positions.GetClosedPositions(ctx, id, fetchLimit, 0)
		if err != nil {
			return ClosedPositionSummary{}, fmt.Errorf("get closed positions for account %d: %w", id, err)
		}
		for _, p := range items {
			if p.RealizedPnlBase != nil {
				summary.TotalRealizedPnLB, _ = summary.TotalRealizedPnLB.Add(*p.RealizedPnlBase)
			}
		}
	}
	return summary, nil
}

// GetOpenPositionsSummary computes the summary from ALL open positions
// (not limited by pagination), respecting the given filters.
// It enriches positions with market data using the provided baseCurrency.
func (s *Service) GetOpenPositionsSummary(ctx context.Context, filters ListFilters, baseCurrency string) (OpenPositionSummary, error) {
	accountIDs, err := s.resolveAccountIDs(ctx, filters)
	if err != nil {
		return OpenPositionSummary{}, err
	}
	// Fetch all open positions (unbounded).
	const fetchLimit = 10000
	var all []Position
	for _, id := range accountIDs {
		items, err := s.positions.GetOpenPositions(ctx, id, fetchLimit, 0)
		if err != nil {
			return OpenPositionSummary{}, fmt.Errorf("get open positions for account %d: %w", id, err)
		}
		all = append(all, items...)
	}
	// Enrich with market data.
	enriched := s.EnrichWithMarketData(ctx, all, baseCurrency)
	var summary OpenPositionSummary
	for _, p := range enriched {
		if !p.MarketDataAvailable {
			continue
		}
		if p.CostBasisBase != nil {
			summary.TotalCostBasisBase, _ = summary.TotalCostBasisBase.Add(*p.CostBasisBase)
		}
		if p.MarketValueBase != nil {
			summary.TotalMktValueBase, _ = summary.TotalMktValueBase.Add(*p.MarketValueBase)
		}
		if p.UnrealizedPnLBase != nil {
			summary.TotalUnrealizedPnLB, _ = summary.TotalUnrealizedPnLB.Add(*p.UnrealizedPnLBase)
		}
	}
	return summary, nil
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

// EnrichWithMarketData reads current market prices from the cache for open
// positions and computes MarketValue, UnrealizedPnL, and UnrealizedPnlPct.
// Cash positions get MarketValue = balance with no P&L. If the market data
// repository is not configured, returns positions with MarketDataAvailable=false
// and zero market values.
// baseCurrency, if non-empty, is used to convert MarketValue and UnrealizedPnL
// to the portfolio base currency.
//
// Market data is read from the cache (populated by the MarketCache background
// service) rather than fetched live.
func (s *Service) EnrichWithMarketData(ctx context.Context, positions []Position, baseCurrency string) []PositionWithMarket {
	if s.marketService == nil {
		// No market data service configured — return positions with no market data.
		result := make([]PositionWithMarket, len(positions))
		for i, p := range positions {
			result[i] = PositionWithMarket{
				Position:            p,
				MarketDataAvailable: false,
				BaseCurrency:        baseCurrency,
			}
		}
		return result
	}

	// Collect unique non-cash symbols for batch reading.
	symbolSet := make(map[string]struct{})
	for _, p := range positions {
		if !isCashPosition(p.Symbol) {
			symbolSet[p.Symbol] = struct{}{}
		}
	}

	symbols := make([]string, 0, len(symbolSet))
	for sym := range symbolSet {
		symbols = append(symbols, sym)
	}

	// Read cached quotes for all unique symbols.
	quotes := make(map[string]*market.MarketData)
	if len(symbols) > 0 {
		quotes = s.marketService.GetQuotes(ctx, symbols)
	}

	result := make([]PositionWithMarket, len(positions))
	for i, p := range positions {
		if isCashPosition(p.Symbol) {
			// Cash positions: MarketValue = balance (quantity), no P&L, no market fetch.
			entry := PositionWithMarket{
				Position:            p,
				MarketValue:         p.Quantity,
				MarketDataAvailable: true,
				BaseCurrency:        baseCurrency,
			}
			// Convert cash position to base currency if needed.
			if baseCurrency != "" && p.Currency != baseCurrency {
				entry.MarketValueBase, entry.UnrealizedPnLBase = convertValuesToBase(ctx, s.marketService, p.Currency, baseCurrency, p.Quantity, decimal.Zero)
				// CostBasis = Quantity for cash, so CostBasisBase = MarketValueBase.
				entry.CostBasisBase = entry.MarketValueBase
			} else if baseCurrency != "" && p.Currency == baseCurrency {
				mv := p.Quantity
				entry.MarketValueBase = &mv
				entry.UnrealizedPnLBase = &decimal.Zero
				entry.CostBasisBase = &mv
			}
			// Compute P&L% using standard formula (same as non-cash positions).
			entry.UnrealizedPnL = decimal.Zero
			totalCost := p.CostBasis.Abs()
			if !totalCost.IsZero() {
				pct, _ := entry.UnrealizedPnL.Quo(totalCost)
				pct, _ = pct.Mul(decimal.MustNew(10000, 2))
				entry.UnrealizedPnlPct = &pct
			}
			result[i] = entry
			continue
		}

		// Look up the cached quote.
		quote, found := quotes[p.Symbol]
		if !found {
			if s.logger != nil {
				s.logger.Debug("no cached market quote found", "symbol", p.Symbol)
			}
			result[i] = PositionWithMarket{
				Position:            p,
				MarketDataAvailable: false,
				BaseCurrency:        baseCurrency,
			}
			continue
		}

		// Compute market value and unrealized P&L.
		// CostBasis is negative (cash outflow), so total cost = Abs(CostBasis).
		// MarketValue = quantity * price (positive for long, negative for short).
		//
		// GBp handling: Yahoo Finance returns some UK stock prices in GBp (pence)
		// instead of GBP. Detect this from the quote's Currency field — if the
		// quote currency is "GBp" but the position currency is "GBP", divide by 100.
		price := quote.Price
		if quote.Currency == "GBp" && p.Currency == "GBP" {
			price, _ = price.Quo(decimal.MustNew(100, 0))
		}

		marketValue, _ := p.Quantity.Mul(price)
		// UnrealizedPnL = market_value - total_cost = market_value + cost_basis
		// (since cost_basis is negative, adding it is equivalent to subtracting abs).
		unrealizedPnL, _ := marketValue.Add(p.CostBasis)

		entry := PositionWithMarket{
			Position:            p,
			MarketPrice:         &price,
			MarketValue:         marketValue,
			UnrealizedPnL:       unrealizedPnL,
			MarketDataAvailable: true,
			BaseCurrency:        baseCurrency,
		}

		// Compute P&L percentage relative to total cost.
		totalCost := p.CostBasis.Abs()
		if !totalCost.IsZero() {
			pct, _ := unrealizedPnL.Quo(totalCost)
			pct, _ = pct.Mul(decimal.MustNew(10000, 2)) // × 100 for percentage
			entry.UnrealizedPnlPct = &pct
		}

		// Convert to base currency if needed.
		if baseCurrency != "" && p.Currency != baseCurrency {
			entry.MarketValueBase, entry.UnrealizedPnLBase = convertValuesToBase(ctx, s.marketService, p.Currency, baseCurrency, marketValue, unrealizedPnL)
			// Cost basis in base currency: CostBasis.Abs() × FX rate.
			if entry.MarketValueBase != nil {
				// Derive rate from MarketValueBase / MarketValue, then apply to cost basis.
				rate, _ := entry.MarketValueBase.Quo(marketValue)
				cbBase, _ := p.CostBasis.Abs().Mul(rate)
				entry.CostBasisBase = &cbBase
			}
		} else if baseCurrency != "" {
			entry.MarketValueBase = &marketValue
			entry.UnrealizedPnLBase = &unrealizedPnL
			cbBase := p.CostBasis.Abs()
			entry.CostBasisBase = &cbBase
		}

		result[i] = entry
	}

	return result
}

// convertValuesToBase converts market value and unrealized P&L from one currency to another
// using the current FX rate. Returns nil pointers if conversion is not possible.
func convertValuesToBase(ctx context.Context, marketService MarketDataService, fromCurrency, toCurrency string, marketValue, unrealizedPnL decimal.Decimal) (*decimal.Decimal, *decimal.Decimal) {
	if fromCurrency == toCurrency {
		return nil, nil
	}

	rate, _ := marketService.GetCurrentFxRate(ctx, fromCurrency, toCurrency)
	if rate == nil {
		return nil, nil
	}

	mvBase, _ := marketValue.Mul(rate.Rate)
	pnlBase, _ := unrealizedPnL.Mul(rate.Rate)
	return &mvBase, &pnlBase
}

// isCashPosition returns true if the symbol represents a cash position.
func isCashPosition(symbol string) bool {
	return strings.HasPrefix(symbol, "$CASH-")
}
