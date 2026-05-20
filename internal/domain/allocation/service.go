package allocation

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"github.com/govalues/decimal"
)

// PositionSource fetches and enriches positions for allocation computation.
type PositionSource interface {
	GetOpenPositions(ctx context.Context, accountIDs []int64, limit, offset int) ([]position.Position, error)
	EnrichWithMarketData(ctx context.Context, positions []position.Position, baseCurrency string) []position.PositionWithMarket
	GetMarketPrice(ctx context.Context, symbol string) (*decimal.Decimal, error)
}

// AccountLister resolves account IDs from portfolios or returns all accounts.
type AccountLister interface {
	GetAccountsByPortfolio(ctx context.Context, portfolioID int64) ([]AccountRef, error)
	GetAllAccounts(ctx context.Context) ([]AccountRef, error)
}

// AccountRef is an alias for position.AccountRef.
type AccountRef = position.AccountRef

// Service computes portfolio allocation from open positions.
type Service struct {
	positions PositionSource
	accounts  AccountLister
	targets   TargetRepository
	logger    *slog.Logger
}

// NewService creates a new allocation service.
func NewService(positions PositionSource, accounts AccountLister, targets TargetRepository) *Service {
	return &Service{
		positions: positions,
		accounts:  accounts,
		targets:   targets,
	}
}

// WithLogger sets the logger for the service.
func (s *Service) WithLogger(logger *slog.Logger) {
	s.logger = logger
}

// ComputeAllocation computes the current allocation breakdown for the given
// filter. It resolves account IDs from portfolio IDs (or all accounts if
// empty), fetches open positions, enriches them with market data, groups by
// symbol, and computes allocation percentages.
//
// Cash positions ($CASH-*) are aggregated into a single "$CASH" row converted
// to the portfolio base currency. Percentages are computed as:
//
//	allocation_pct = symbol_market_value_base / total_market_value_base * 100
func (s *Service) ComputeAllocation(ctx context.Context, filter AllocationFilter) (*AllocationResult, error) {
	// 1. Resolve accounts and base currency.
	accounts, baseCurrency, err := s.resolveAccounts(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("resolve accounts: %w", err)
	}

	// 2. Handle empty accounts.
	if len(accounts) == 0 {
		return &AllocationResult{
			BaseCurrency: baseCurrency,
			Message:      "No accounts found for the selected portfolios",
		}, nil
	}

	accountIDs := extractAccountIDs(accounts)

	// 3. Fetch open positions (unbounded — allocation needs all positions).
	const fetchLimit = 10000
	positions, err := s.positions.GetOpenPositions(ctx, accountIDs, fetchLimit, 0)
	if err != nil {
		return nil, fmt.Errorf("fetch open positions: %w", err)
	}

	// 4. Handle empty positions.
	if len(positions) == 0 {
		return &AllocationResult{
			BaseCurrency: baseCurrency,
			Message:      "No open positions found",
		}, nil
	}

	// 5. Enrich with market data.
	enriched := s.positions.EnrichWithMarketData(ctx, positions, baseCurrency)

	// 6. Compute total portfolio value (sum of MarketValueBase).
	var totalValueBase decimal.Decimal
	var hasMarketData bool
	var warnings []string

	for _, p := range enriched {
		if !p.MarketDataAvailable {
			continue
		}
		if p.MarketValueBase != nil {
			totalValueBase, _ = totalValueBase.Add(*p.MarketValueBase)
			hasMarketData = true
		} else if !p.MarketValue.IsZero() {
			// Same currency as base — MarketValueBase is nil but MarketValue is valid.
			totalValueBase, _ = totalValueBase.Add(p.MarketValue)
			hasMarketData = true
		}
	}

	// 7. Check for zero/negative total value.
	if !totalValueBase.IsPos() {
		return nil, ErrZeroTotalValue
	}

	// 8. Group enriched positions by symbol (cash symbols grouped separately).
	groups := groupBySymbol(enriched)

	// 9. Build allocation rows.
	var rows []AllocationRow
	var allCashEntries []position.PositionWithMarket

	for sym, entries := range groups {
		if isCashSymbol(sym) {
			// Collect all cash entries for aggregation.
			allCashEntries = append(allCashEntries, entries...)
			continue
		}

		row := buildAllocationRow(sym, entries, totalValueBase, accounts)
		// Skip symbols where no position has market data.
		if !row.HasMarketData {
			continue
		}
		rows = append(rows, row)
	}

	// Build single aggregated cash row from all cash entries.
	var cashRow *AllocationRow
	if len(allCashEntries) > 0 {
		cashRow = buildCashRow(allCashEntries, totalValueBase, baseCurrency, accounts)
	}

	// Collect warnings for positions without market data.
	missingSymbols := collectMissingMarketData(enriched)
	if len(missingSymbols) > 0 {
		warnings = append(warnings, fmt.Sprintf("market data unavailable for %d position(s): %s",
			len(missingSymbols), strings.Join(missingSymbols, ", ")))
	}

	// 10. Sort rows by allocation percentage descending.
	sort.Slice(rows, func(i, j int) bool {
		diff, _ := rows[i].AllocationPct.Sub(rows[j].AllocationPct)
		return diff.IsPos()
	})

	return &AllocationResult{
		Rows:                rows,
		TotalValueBase:      totalValueBase,
		BaseCurrency:        baseCurrency,
		CashRow:             cashRow,
		LastUpdated:         time.Now().UTC(),
		MarketDataAvailable: hasMarketData,
		Warnings:            warnings,
	}, nil
}

// resolveAccounts resolves account IDs and base currency from the filter.
// If PortfolioIDs is empty, all accounts are returned.
// All accounts must share the same base currency; ErrMixedCurrencies is
// returned if conflicting currencies are detected.
func (s *Service) resolveAccounts(ctx context.Context, filter AllocationFilter) ([]AccountRef, string, error) {
	var allAccounts []AccountRef

	if len(filter.PortfolioIDs) > 0 {
		for _, pid := range filter.PortfolioIDs {
			accounts, err := s.accounts.GetAccountsByPortfolio(ctx, pid)
			if err != nil {
				return nil, "", fmt.Errorf("get accounts for portfolio %d: %w", pid, err)
			}
			allAccounts = append(allAccounts, accounts...)
		}
	} else {
		// No filter → all accounts.
		var err error
		allAccounts, err = s.accounts.GetAllAccounts(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("list all accounts: %w", err)
		}
	}

	if len(allAccounts) == 0 {
		return []AccountRef{}, "", nil
	}

	// Verify all accounts share the same base currency.
	baseCurrency := allAccounts[0].PortfolioCurrency
	for _, a := range allAccounts[1:] {
		if a.PortfolioCurrency != baseCurrency {
			return nil, "", ErrMixedCurrencies
		}
	}

	return allAccounts, baseCurrency, nil
}

// extractAccountIDs extracts account IDs from account refs.
func extractAccountIDs(accounts []AccountRef) []int64 {
	ids := make([]int64, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	return ids
}

// groupBySymbol groups enriched positions by symbol.
// Cash symbols ($CASH-*) are grouped under their individual symbol keys
// (e.g., "$CASH-USD", "$CASH-GBP") — the caller aggregates them into one row.
func groupBySymbol(enriched []position.PositionWithMarket) map[string][]position.PositionWithMarket {
	groups := make(map[string][]position.PositionWithMarket)
	for _, p := range enriched {
		groups[p.Symbol] = append(groups[p.Symbol], p)
	}
	return groups
}

// isCashSymbol returns true if the symbol is a cash position.
func isCashSymbol(symbol string) bool {
	return strings.HasPrefix(symbol, "$CASH-")
}

// buildAllocationRow builds an AllocationRow for a non-cash symbol.
func buildAllocationRow(symbol string, entries []position.PositionWithMarket, totalValueBase decimal.Decimal, accounts []AccountRef) AllocationRow {
	// Sum market values.
	var mvBase decimal.Decimal
	var mvNative decimal.Decimal
	var hasMarketData bool
	var currency string

	for _, p := range entries {
		if !p.MarketDataAvailable {
			continue
		}
		if p.MarketValueBase != nil {
			mvBase, _ = mvBase.Add(*p.MarketValueBase)
		} else if !p.MarketValue.IsZero() {
			mvBase, _ = mvBase.Add(p.MarketValue)
		}
		mvNative, _ = mvNative.Add(p.MarketValue)
		hasMarketData = true
		if currency == "" {
			currency = p.Currency
		}
	}

	// Compute allocation percentage.
	allocPct := computePercentage(mvBase, totalValueBase)

	// Build account breakdown.
	breakdown := buildAccountBreakdown(entries, mvBase, accounts)

	return AllocationRow{
		Symbol:           symbol,
		MarketValue:      mvNative,
		MarketValueBase:  &mvBase,
		AllocationPct:    allocPct,
		Currency:         currency,
		HasMarketData:    hasMarketData,
		AccountBreakdown: breakdown,
	}
}

// buildCashRow aggregates all cash positions into a single "$CASH" row.
func buildCashRow(entries []position.PositionWithMarket, totalValueBase decimal.Decimal, baseCurrency string, accounts []AccountRef) *AllocationRow {
	var mvBase decimal.Decimal
	var hasMarketData bool
	breakdownMap := make(map[int64]position.PositionWithMarket)

	for _, p := range entries {
		if !p.MarketDataAvailable {
			continue
		}
		if p.MarketValueBase != nil {
			mvBase, _ = mvBase.Add(*p.MarketValueBase)
		} else if !p.MarketValue.IsZero() {
			mvBase, _ = mvBase.Add(p.MarketValue)
		}
		hasMarketData = true
		// Keep one entry per account for breakdown.
		if _, exists := breakdownMap[p.AccountID]; !exists {
			breakdownMap[p.AccountID] = p
		}
	}

	// Build account breakdown from the map.
	var breakdown []AccountBreakdown
	for _, p := range breakdownMap {
		var mvB *decimal.Decimal
		if p.MarketValueBase != nil {
			v := *p.MarketValueBase
			mvB = &v
		} else if !p.MarketValue.IsZero() {
			v := p.MarketValue
			mvB = &v
		}
		pct := decimal.Zero
		if !mvBase.IsZero() && mvB != nil {
			pct, _ = mvB.Quo(mvBase)
			pct, _ = pct.Mul(decimal.MustNew(10000, 2))
		}
		breakdown = append(breakdown, AccountBreakdown{
			AccountID:       p.AccountID,
			AccountName:     p.AccountName,
			Quantity:        p.Quantity,
			MarketValue:     p.MarketValue,
			MarketValueBase: mvB,
			PctOfSymbol:     pct,
		})
	}

	allocPct := computePercentage(mvBase, totalValueBase)

	row := &AllocationRow{
		Symbol:           "$CASH",
		MarketValue:      mvBase,
		MarketValueBase:  &mvBase,
		AllocationPct:    allocPct,
		Currency:         baseCurrency,
		HasMarketData:    hasMarketData,
		AccountBreakdown: breakdown,
	}

	return row
}

// buildAccountBreakdown builds per-account breakdown for a symbol.
func buildAccountBreakdown(entries []position.PositionWithMarket, symbolMVBase decimal.Decimal, accounts []AccountRef) []AccountBreakdown {
	// Build account name lookup.
	nameMap := make(map[int64]string)
	for _, a := range accounts {
		nameMap[a.ID] = a.Name
	}

	var breakdown []AccountBreakdown
	for _, p := range entries {
		if !p.MarketDataAvailable {
			continue
		}
		var mvB *decimal.Decimal
		if p.MarketValueBase != nil {
			mvB = new(decimal.Decimal)
			*mvB = *p.MarketValueBase
		} else if !p.MarketValue.IsZero() {
			mvB = new(decimal.Decimal)
			*mvB = p.MarketValue
		}

		pct := decimal.Zero
		if !symbolMVBase.IsZero() && mvB != nil {
			pct, _ = mvB.Quo(symbolMVBase)
			pct, _ = pct.Mul(decimal.MustNew(10000, 2))
		}

		accountName := p.AccountName
		if accountName == "" {
			accountName = nameMap[p.AccountID]
		}

		breakdown = append(breakdown, AccountBreakdown{
			AccountID:       p.AccountID,
			AccountName:     accountName,
			Quantity:        p.Quantity,
			MarketValue:     p.MarketValue,
			MarketValueBase: mvB,
			PctOfSymbol:     pct,
		})
	}

	return breakdown
}

// computePercentage computes (value / total * 100) rounded to 1 decimal place.
func computePercentage(value, total decimal.Decimal) decimal.Decimal {
	if total.IsZero() {
		return decimal.Zero
	}
	pct, _ := value.Quo(total)
	pct, _ = pct.Mul(decimal.MustNew(10000, 2)) // × 100
	// Round to 1 decimal place.
	pct = pct.Round(1)
	return pct
}

// collectMissingMarketData returns a deduplicated list of symbols
// that lack market data in the enriched positions.
func collectMissingMarketData(enriched []position.PositionWithMarket) []string {
	seen := make(map[string]struct{})
	var missing []string
	for _, p := range enriched {
		if !p.MarketDataAvailable && !isCashSymbol(p.Symbol) {
			if _, exists := seen[p.Symbol]; !exists {
				seen[p.Symbol] = struct{}{}
				missing = append(missing, p.Symbol)
			}
		}
	}
	return missing
}

// --- Target Allocation CRUD ---

// GetTargetAllocation retrieves all target allocations for a portfolio.
func (s *Service) GetTargetAllocation(ctx context.Context, portfolioID int64) ([]TargetAllocation, error) {
	targets, err := s.targets.GetByPortfolio(ctx, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("get target allocations: %w", err)
	}
	if targets == nil {
		targets = []TargetAllocation{}
	}
	return targets, nil
}

// SaveTargetAllocation validates and persists target allocations for a portfolio.
// Each entry's TargetPct must be in [0, 100] and all entries must sum to exactly 100.
// The error message includes the current total and delta from 100 when the sum check fails.
func (s *Service) SaveTargetAllocation(ctx context.Context, portfolioID int64, entries []TargetEntry) error {
	if len(entries) == 0 {
		return ErrTargetSumNot100
	}

	// Check for duplicate symbols.
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if _, exists := seen[e.Symbol]; exists {
			return ErrDuplicateSymbol
		}
		seen[e.Symbol] = struct{}{}
	}

	// Validate each percentage is in [0, 100].
	for _, e := range entries {
		if e.Symbol == "" {
			return ErrInvalidTargetPct
		}
		if e.TargetPct.IsNeg() {
			return ErrInvalidTargetPct
		}
		hundred := decimal.MustNew(10000, 2)
		cmp, _ := e.TargetPct.Sub(hundred)
		if cmp.IsPos() {
			return ErrInvalidTargetPct
		}
	}

	// Validate sum == 100.
	var sum decimal.Decimal
	for _, e := range entries {
		sum, _ = sum.Add(e.TargetPct)
	}
	hundred := decimal.MustNew(10000, 2)
	if !sum.Equal(hundred) {
		delta, _ := sum.Sub(hundred)
		return &AllocationError{
			Code:    "target_sum_not_100",
			Message: fmt.Sprintf("target percentages must sum to exactly 100%% (current total: %s%%, delta: %s%%)", sum.String(), delta.String()),
		}
	}

	// Upsert each entry.
	for _, e := range entries {
		ta := TargetAllocation{
			PortfolioID: portfolioID,
			Symbol:      e.Symbol,
			TargetPct:   e.TargetPct,
		}
		if err := s.targets.Upsert(ctx, ta); err != nil {
			return fmt.Errorf("upsert target allocation %q: %w", e.Symbol, err)
		}
	}

	return nil
}

// DeleteTargetAllocation removes a target allocation for a portfolio+symbol.
func (s *Service) DeleteTargetAllocation(ctx context.Context, portfolioID int64, symbol string) error {
	if err := s.targets.DeleteBySymbol(ctx, portfolioID, symbol); err != nil {
		return fmt.Errorf("delete target allocation %q: %w", symbol, err)
	}
	return nil
}

// DeleteAllTargetAllocations removes all target allocations for a portfolio.
func (s *Service) DeleteAllTargetAllocations(ctx context.Context, portfolioID int64) error {
	if err := s.targets.DeleteByPortfolio(ctx, portfolioID); err != nil {
		return fmt.Errorf("delete all target allocations: %w", err)
	}
	return nil
}

// driftTolerance is the threshold (in percentage points) below which a symbol
// is considered balanced. A drift of |actual - target| <= 5% is balanced.
var driftTolerance = decimal.MustParse("5.0")

// ComputeDrift compares actual allocation against saved target allocation and
// computes drift per symbol. It builds a unified symbol list from the union of
// actual and target symbols.
//
// For each symbol:
//   - actual_pct = allocation percentage from positions (0 if not held)
//   - target_pct = saved target percentage (0 if not in target)
//   - drift_pct = actual_pct - target_pct
//   - is_balanced = |drift_pct| <= 5%
//
// If no target is saved, returns actual percentages only with has_target=false.
// Rows are sorted by |drift| descending.
func (s *Service) ComputeDrift(ctx context.Context, filter AllocationFilter, portfolioID int64) (*DriftResult, error) {
	// 1. Compute actual allocation for the portfolio.
	allocFilter := AllocationFilter{PortfolioIDs: []int64{portfolioID}}
	actual, err := s.ComputeAllocation(ctx, allocFilter)
	if err != nil {
		return nil, fmt.Errorf("compute actual allocation: %w", err)
	}

	// 2. Fetch target allocation for the portfolio.
	targets, err := s.GetTargetAllocation(ctx, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("get target allocation: %w", err)
	}

	hasTarget := len(targets) > 0

	// 3. Build lookup maps.
	actualMap := make(map[string]decimal.Decimal)
	for _, row := range actual.Rows {
		actualMap[row.Symbol] = row.AllocationPct
	}
	if actual.CashRow != nil {
		actualMap["$CASH"] = actual.CashRow.AllocationPct
	}

	targetMap := make(map[string]decimal.Decimal)
	for _, t := range targets {
		targetMap[t.Symbol] = t.TargetPct
	}

	// 4. Build unified symbol list (union of actual + target symbols).
	symbolSet := make(map[string]struct{})
	for sym := range actualMap {
		symbolSet[sym] = struct{}{}
	}
	for sym := range targetMap {
		symbolSet[sym] = struct{}{}
	}

	// 5. Build drift rows.
	var rows []DriftRow
	for sym := range symbolSet {
		actualPct := actualMap[sym] // zero if not held
		targetPct := targetMap[sym] // zero if not in target
		driftPct, _ := actualPct.Sub(targetPct)
		absDrift := driftPct.Abs()
		diff, _ := absDrift.Sub(driftTolerance)
		isBalanced := !diff.IsPos() // absDrift <= tolerance

		rows = append(rows, DriftRow{
			Symbol:     sym,
			ActualPct:  actualPct,
			TargetPct:  targetPct,
			DriftPct:   driftPct,
			IsBalanced: isBalanced,
		})
	}

	// 6. Sort by |drift| descending.
	sort.Slice(rows, func(i, j int) bool {
		absI := rows[i].DriftPct.Abs()
		absJ := rows[j].DriftPct.Abs()
		cmp, _ := absI.Sub(absJ)
		return cmp.IsPos()
	})

	baseCurrency := actual.BaseCurrency
	if baseCurrency == "" {
		baseCurrency = "USD"
	}

	return &DriftResult{
		Rows:         rows,
		BaseCurrency: baseCurrency,
		HasTarget:    hasTarget,
	}, nil
}

// ComputeRebalancingSuggestions generates trade suggestions to close the gap
// between actual and target allocation. It computes drift, identifies symbols
// with |drift| > 5%, and calculates share quantities and dollar values.
//
// For each symbol with significant drift:
//   - drift > 0 (overweight): suggest SELL shares_to_sell = drift_value / current_price
//   - drift < 0 (underweight): suggest BUY shares_to_buy = abs(drift_value) / current_price
//   - drift_value = drift_pct / 100 * total_portfolio_value
//
// Suggestions are sorted by |drift| descending.
// Symbols without market data generate a warning and are excluded.
// Cash symbols are excluded from suggestions (no meaningful "buy cash" action).
// If all symbols are within tolerance, returns is_balanced=true.
func (s *Service) ComputeRebalancingSuggestions(ctx context.Context, filter AllocationFilter, portfolioID int64) (*RebalanceResult, error) {
	// 1. Compute allocation for total portfolio value.
	allocFilter := AllocationFilter{PortfolioIDs: []int64{portfolioID}}
	actual, err := s.ComputeAllocation(ctx, allocFilter)
	if err != nil {
		return nil, fmt.Errorf("compute actual allocation: %w", err)
	}

	// 2. Compute drift.
	drift, err := s.ComputeDrift(ctx, filter, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("compute drift: %w", err)
	}

	totalValue := actual.TotalValueBase

	// 3. Build price lookup from allocation rows (for symbols currently held).
	// Price = MarketValue / total_quantity from account breakdown.
	priceMap := make(map[string]decimal.Decimal)
	for _, row := range actual.Rows {
		var totalQty, totalMV decimal.Decimal
		for _, ab := range row.AccountBreakdown {
			totalQty, _ = totalQty.Add(ab.Quantity)
			if ab.MarketValueBase != nil {
				totalMV, _ = totalMV.Add(*ab.MarketValueBase)
			} else {
				totalMV, _ = totalMV.Add(ab.MarketValue)
			}
		}
		if !totalQty.IsZero() {
			price, _ := totalMV.Quo(totalQty)
			priceMap[row.Symbol] = price
		}
	}

	// 4. Generate suggestions for symbols with |drift| > tolerance.
	var suggestions []RebalanceSuggestion
	var warnings []string

	for _, row := range drift.Rows {
		absDrift := row.DriftPct.Abs()
		diff, _ := absDrift.Sub(driftTolerance)
		if !diff.IsPos() {
			continue // Within tolerance — skip.
		}

		// Skip cash — no meaningful "buy cash" or "sell cash" action.
		if row.Symbol == "$CASH" {
			warnings = append(warnings, fmt.Sprintf("cash allocation drift of %s%% exceeds tolerance (no rebalancing action for cash)", row.DriftPct.String()))
			continue
		}

		// Compute drift value in dollars.
		driftValue, _ := absDrift.Quo(decimal.MustNew(10000, 2)) // drift_pct / 100
		driftValue, _ = driftValue.Mul(totalValue)

		// Resolve price.
		var price decimal.Decimal
		var hasPrice bool

		if p, ok := priceMap[row.Symbol]; ok {
			price = p
			hasPrice = true
		} else {
			// Symbol not currently held — look up market price.
			marketPrice, err := s.positions.GetMarketPrice(ctx, row.Symbol)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to get market price for %s: %v", row.Symbol, err))
				continue
			}
			if marketPrice == nil || marketPrice.IsZero() {
				warnings = append(warnings, fmt.Sprintf("market data unavailable for %s — excluded from rebalancing suggestions", row.Symbol))
				continue
			}
			price = *marketPrice
			hasPrice = true
		}

		if !hasPrice {
			warnings = append(warnings, fmt.Sprintf("market data unavailable for %s — excluded from rebalancing suggestions", row.Symbol))
			continue
		}

		// Compute shares.
		shares, _ := driftValue.Quo(price)
		// Round shares to 2 decimal places (standard for fractional shares).
		shares = shares.Round(2)

		direction := "sell"
		if row.DriftPct.IsNeg() {
			direction = "buy"
		}

		suggestions = append(suggestions, RebalanceSuggestion{
			Symbol:         row.Symbol,
			Direction:      direction,
			Shares:         shares,
			DollarValue:    driftValue,
			DriftReduction: absDrift,
		})
	}

	// 5. Sort by |drift| (drift_reduction) descending.
	sort.Slice(suggestions, func(i, j int) bool {
		cmp, _ := suggestions[i].DriftReduction.Sub(suggestions[j].DriftReduction)
		return cmp.IsPos()
	})

	// 6. Compute total dollar value.
	var totalDollarValue decimal.Decimal
	for _, s := range suggestions {
		totalDollarValue, _ = totalDollarValue.Add(s.DollarValue)
	}

	isBalanced := len(suggestions) == 0

	baseCurrency := actual.BaseCurrency
	if baseCurrency == "" {
		baseCurrency = "USD"
	}

	return &RebalanceResult{
		Suggestions:      suggestions,
		BaseCurrency:     baseCurrency,
		TotalDollarValue: totalDollarValue,
		Warnings:         warnings,
		IsBalanced:       isBalanced,
	}, nil
}
