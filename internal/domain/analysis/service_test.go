package analysis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

var ctx = context.Background()

// --- Mocks ---

type mockPositionSource struct {
	positions       []position.Position
	enriched        []position.PositionWithMarket
	getOpenErr      error
	enrichFn        func(ctx context.Context, positions []position.Position, baseCurrency string) []position.PositionWithMarket
}

func (m *mockPositionSource) GetOpenPositions(_ context.Context, accountIDs []int64, limit, offset int) ([]position.Position, error) {
	if m.getOpenErr != nil {
		return nil, m.getOpenErr
	}
	// Filter positions by account IDs.
	var result []position.Position
	accountSet := make(map[int64]struct{})
	for _, id := range accountIDs {
		accountSet[id] = struct{}{}
	}
	for _, p := range m.positions {
		if _, ok := accountSet[p.AccountID]; ok {
			result = append(result, p)
		}
	}
	if result == nil {
		result = []position.Position{}
	}
	// Apply pagination.
	if len(result) <= offset {
		return []position.Position{}, nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockPositionSource) EnrichWithMarketData(ctx context.Context, positions []position.Position, baseCurrency string) []position.PositionWithMarket {
	if m.enrichFn != nil {
		return m.enrichFn(ctx, positions, baseCurrency)
	}
	// Default: return pre-set enriched positions.
	return m.enriched
}

type mockSymbolDetailsSource struct {
	details map[string]*symbol.SymbolDetails
	getErr  error
}

func (m *mockSymbolDetailsSource) GetByInternalSymbol(_ context.Context, internalSymbol string) (*symbol.SymbolDetails, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	sd, ok := m.details[internalSymbol]
	if !ok {
		return nil, fmt.Errorf("symbol details not found for %s", internalSymbol)
	}
	return sd, nil
}

type mockMarketHistorySource struct {
	prices map[string][]market.HistoricalPrice
	err    error
}

func (m *mockMarketHistorySource) GetHistoricalPrices(_ context.Context, marketSymbol string, start, end time.Time) ([]market.HistoricalPrice, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return prices for this symbol, filtered to date range.
	prices, ok := m.prices[marketSymbol]
	if !ok {
		return []market.HistoricalPrice{}, nil
	}
	var filtered []market.HistoricalPrice
	for _, p := range prices {
		if (!p.Date.Before(start) || start.IsZero()) && (!p.Date.After(end) || end.IsZero()) {
			filtered = append(filtered, p)
		}
	}
	if filtered == nil {
		filtered = []market.HistoricalPrice{}
	}
	return filtered, nil
}

type mockAccountResolver struct {
	accounts []position.AccountRef
	err      error
}

func (m *mockAccountResolver) GetAccountsByPortfolio(_ context.Context, portfolioID int64) ([]position.AccountRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []position.AccountRef
	for _, a := range m.accounts {
		if a.PortfolioID == portfolioID {
			result = append(result, a)
		}
	}
	if result == nil {
		result = []position.AccountRef{}
	}
	return result, nil
}

func (m *mockAccountResolver) GetAllAccounts(_ context.Context) ([]position.AccountRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.accounts == nil {
		return []position.AccountRef{}, nil
	}
	return m.accounts, nil
}

type mockPortfolioCurrencySource struct {
	currencies map[int64]string
	err        error
}

func (m *mockPortfolioCurrencySource) GetPortfolioCurrency(_ context.Context, portfolioID int64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	cur, ok := m.currencies[portfolioID]
	if !ok {
		return "", fmt.Errorf("portfolio %d not found", portfolioID)
	}
	return cur, nil
}

type mockMarketDataSymbolResolver struct {
	mappings map[string]string
	err      error
}

func (m *mockMarketDataSymbolResolver) GetMarketDataSymbol(_ context.Context, internalSymbol string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	sym, ok := m.mappings[internalSymbol]
	if !ok {
		return "", fmt.Errorf("market data symbol not found for %s", internalSymbol)
	}
	return sym, nil
}

type mockSymbolRefresher struct {
	refreshed []string
}

func (m *mockSymbolRefresher) RefreshSymbol(_ context.Context, internalSymbol, marketDataSymbol string) error {
	m.refreshed = append(m.refreshed, internalSymbol)
	return nil
}

// --- Test helpers ---

func makePosition(accountID int64, sym, currency string, quantity, costBasis decimal.Decimal) position.Position {
	return position.Position{
		ID:         int64(accountID),
		AccountID:  accountID,
		Symbol:     sym,
		Currency:   currency,
		Quantity:   quantity,
		CostBasis:  costBasis,
		OpenDate:   time.Now().AddDate(0, 0, -30),
		IsClosed:   false,
	}
}

func makeEnrichedPosition(p position.Position, marketValue decimal.Decimal, available bool, mvBase *decimal.Decimal) position.PositionWithMarket {
	return position.PositionWithMarket{
		Position:            p,
		MarketValue:         marketValue,
		MarketDataAvailable: available,
		MarketValueBase:     mvBase,
	}
}

func makeSymbolDetails(sym, quoteType string, sector string, holdings []symbol.TopHolding, val *symbol.EquityValuation, fund *symbol.FundProfile, geo []symbol.GeographicAllocation, fetchedAt time.Time) *symbol.SymbolDetails {
	return &symbol.SymbolDetails{
		InternalSymbol:        sym,
		ShortName:             sym,
		QuoteType:             quoteType,
		Sector:                sector,
		TopHoldings:           holdings,
		EquityValuation:       val,
		FundProfile:           fund,
		GeographicAllocations: geo,
		FetchedAt:             fetchedAt,
	}
}

func makePrice(date time.Time, close decimal.Decimal) market.HistoricalPrice {
	return market.HistoricalPrice{
		Date:     date,
		Close:    close,
		Currency: "USD",
	}
}

// --- Tests ---

func TestComputeAnalysis_HappyPath(t *testing.T) {
	// Portfolio with 2 ETFs + 1 stock, all data available.
	etf1Details := makeSymbolDetails("VOO", "ETF", "",
		[]symbol.TopHolding{{Symbol: "AAPL", Name: "Apple", Percent: 5.0}},
		&symbol.EquityValuation{PriceToEarnings: 25, PriceToBook: 4},
		&symbol.FundProfile{TotalNetAssets: 1e12},
		[]symbol.GeographicAllocation{{Country: "United States", Percent: 100}},
		time.Now().AddDate(0, 0, -3),
	)
	etf2Details := makeSymbolDetails("VEA", "ETF", "",
		[]symbol.TopHolding{{Symbol: "AAPL", Name: "Apple", Percent: 2.0}},
		&symbol.EquityValuation{PriceToEarnings: 15, PriceToBook: 2},
		&symbol.FundProfile{TotalNetAssets: 5e11},
		[]symbol.GeographicAllocation{{Country: "Germany", Percent: 50}, {Country: "France", Percent: 50}},
		time.Now().AddDate(0, 0, -3),
	)
	stockDetails := makeSymbolDetails("AAPL", "EQUITY", "Technology",
		nil,
		&symbol.EquityValuation{PriceToEarnings: 30, PriceToBook: 8},
		nil,
		nil,
		time.Now().AddDate(0, 0, -3),
	)

	pos1 := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2)) // -100.00
	pos2 := makePosition(1, "VEA", "USD", decimal.MustNew(50, 0), decimal.MustNew(-25000000, 2))  // -250.00
	pos3 := makePosition(1, "AAPL", "USD", decimal.MustNew(5, 0), decimal.MustNew(-5000000, 2))  // -50.00

	mv1 := decimal.MustNew(10500000, 2)  // 105.00
	mv2 := decimal.MustNew(26000000, 2)  // 260.00
	mv3 := decimal.MustNew(5500000, 2)   // 55.00

	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos1, mv1, true, &mv1),
		makeEnrichedPosition(pos2, mv2, true, &mv2),
		makeEnrichedPosition(pos3, mv3, true, &mv3),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos1, pos2, pos3},
		enriched:  enriched,
	}
	symSource := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{
			"VOO":  etf1Details,
			"VEA":  etf2Details,
			"AAPL": stockDetails,
		},
	}
	marketSource := &mockMarketHistorySource{
		prices: map[string][]market.HistoricalPrice{
			"VOO":  {makePrice(time.Now().AddDate(0, 0, -1), mv1)},
			"VEA":  {makePrice(time.Now().AddDate(0, 0, -1), mv2)},
			"AAPL": {makePrice(time.Now().AddDate(0, 0, -1), mv3)},
		},
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1, PortfolioCurrency: "USD"}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}
	marketSymResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA", "AAPL": "AAPL"},
	}

	svc := NewService(posSource, symSource, marketSource, accResolver, curSource, marketSymResolver)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
		Period:      "1Y",
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PortfolioID != 1 {
		t.Errorf("expected portfolio_id 1, got %d", result.PortfolioID)
	}
	if result.ComputedAt.IsZero() {
		t.Error("computed_at is zero")
	}

	// Overlap should have data (2 ETFs).
	if result.Overlap == nil {
		t.Fatal("overlap should not be nil")
	}
	if len(result.Overlap.PairwiseMatrix) == 0 {
		t.Error("expected pairwise matrix with 2 ETFs")
	}

	// Sector allocation should have data.
	if result.SectorAllocation == nil {
		t.Fatal("sector allocation should not be nil")
	}

	// Stress test should have data.
	if result.StressTest == nil {
		t.Fatal("stress test should not be nil")
	}
	if len(result.StressTest.Scenarios) == 0 {
		t.Error("expected stress test scenarios")
	}

	// Factor exposure should have data.
	if result.FactorExposure == nil {
		t.Fatal("factor exposure should not be nil")
	}
}

func TestComputeAnalysis_NoPositions(t *testing.T) {
	posSource := &mockPositionSource{
		positions: []position.Position{},
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, nil, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message == "" {
		t.Error("expected empty-state message")
	}
	if result.Overlap != nil {
		t.Error("overlap should be nil when no positions")
	}
	if result.SectorAllocation != nil {
		t.Error("sector allocation should be nil when no positions")
	}
}

func TestComputeAnalysis_MissingSymbolDetails(t *testing.T) {
	pos := makePosition(1, "UNKNOWN", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(11000000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	symSource := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{}, // No details for UNKNOWN
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, symSource, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still compute but with warnings.
	if result.SectorAllocation == nil {
		t.Fatal("sector allocation should still be computed")
	}
	// With no symbol details, allocation should fall to Unknown.
	if result.SectorAllocation.UnknownWeightPct <= 0 {
		t.Errorf("expected unknown weight > 0, got %f", result.SectorAllocation.UnknownWeightPct)
	}
}

func TestComputeAnalysis_SectionFilter(t *testing.T) {
	etfDetails := makeSymbolDetails("VOO", "ETF", "",
		[]symbol.TopHolding{{Symbol: "AAPL", Name: "Apple", Percent: 5.0}},
		nil, nil, nil,
		time.Now().AddDate(0, 0, -3),
	)
	pos := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(10500000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	symSource := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{"VOO": etfDetails},
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, symSource, nil, accResolver, curSource, nil)

	// Request only sector_allocation.
	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
		Section:     string(SectionSectorAllocation),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SectorAllocation == nil {
		t.Fatal("sector allocation should be computed")
	}
	// Other sections should be nil (not computed).
	if result.Overlap != nil {
		t.Error("overlap should be nil when filtered to sector_allocation")
	}
	if result.Correlation != nil {
		t.Error("correlation should be nil when filtered to sector_allocation")
	}
}

func TestComputeAnalysis_AllAccounts(t *testing.T) {
	// No portfolio filter → all accounts.
	pos1 := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	pos2 := makePosition(2, "AAPL", "USD", decimal.MustNew(5, 0), decimal.MustNew(-5000000, 2))
	mv1 := decimal.MustNew(10500000, 2)
	mv2 := decimal.MustNew(5500000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos1, mv1, true, &mv1),
		makeEnrichedPosition(pos2, mv2, true, &mv2),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos1, pos2},
		enriched:  enriched,
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{
			{ID: 1, PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, PortfolioID: 1, PortfolioCurrency: "USD"},
		},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, nil, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{} // No portfolio filter.

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PortfolioID != 0 {
		t.Errorf("expected portfolio_id 0 (all accounts), got %d", result.PortfolioID)
	}
	// Should have computed from both positions.
	if result.SectorAllocation == nil {
		t.Fatal("sector allocation should be computed")
	}
}

func TestComputeAnalysis_StaleSymbolRefresh(t *testing.T) {
	// Symbol details fetched >7 days ago → should trigger refresh.
	staleDetails := makeSymbolDetails("VOO", "ETF", "", nil, nil, nil, nil,
		time.Now().AddDate(0, 0, -10), // 10 days ago
	)
	pos := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(10500000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	symSource := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{"VOO": staleDetails},
	}
	marketSymResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO"},
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}
	refresher := &mockSymbolRefresher{}

	svc := NewService(posSource, symSource, nil, accResolver, curSource, marketSymResolver)
	svc.WithSymbolRefresher(refresher)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	_, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Note: refresh is done in a background goroutine, so we need a small delay.
	time.Sleep(100 * time.Millisecond)
	if len(refresher.refreshed) == 0 {
		t.Error("expected stale symbol to be refreshed")
	}
}

func TestComputeAnalysis_SingleStockPortfolio(t *testing.T) {
	// Single stock (no ETFs) → overlap shows message, other sections work.
	stockDetails := makeSymbolDetails("AAPL", "EQUITY", "Technology", nil, nil, nil, nil,
		time.Now().AddDate(0, 0, -3),
	)
	pos := makePosition(1, "AAPL", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(11000000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	symSource := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{"AAPL": stockDetails},
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, symSource, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Overlap should have a message (no ETFs).
	if result.Overlap == nil {
		t.Fatal("overlap should not be nil")
	}
	if result.Overlap.Message == "" {
		t.Error("expected overlap message for single-stock portfolio")
	}

	// Sector allocation should still work.
	if result.SectorAllocation == nil {
		t.Fatal("sector allocation should be computed")
	}
}

func TestComputeAnalysis_StressTestWithoutSectorFilter(t *testing.T) {
	// When stress_test is requested alone (not sector_allocation),
	// sector allocation should still be computed internally.
	etfDetails := &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "VOO",
		QuoteType:      "ETF",
		TopHoldings:    []symbol.TopHolding{{Symbol: "AAPL", Name: "Apple", Percent: 5.0}},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 30},
			{Sector: "Financials", Percent: 20},
			{Sector: "Healthcare", Percent: 15},
		},
		GeographicAllocations: []symbol.GeographicAllocation{{Country: "United States", Percent: 100}},
		FetchedAt:             time.Now().AddDate(0, 0, -3),
	}
	pos := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(10500000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	symSource := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{"VOO": etfDetails},
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, symSource, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
		Section:     string(SectionStressTest),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StressTest == nil {
		t.Fatal("stress test should be computed")
	}
	if len(result.StressTest.Scenarios) == 0 {
		t.Error("expected stress test scenarios")
	}
	// Sector allocation should NOT be in the result (only computed internally).
	if result.SectorAllocation != nil {
		t.Error("sector allocation should not be in result when only stress_test requested")
	}
}

func TestComputeAnalysis_WarningsCollected(t *testing.T) {
	// Portfolio with ETF missing holdings data → should generate warnings.
	etfNoHoldings := makeSymbolDetails("NOHO", "ETF", "", nil, nil, nil, nil,
		time.Now().AddDate(0, 0, -3),
	)
	pos := makePosition(1, "NOHO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(10500000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	symSource := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{"NOHO": etfNoHoldings},
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, symSource, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have warnings from sections that detected missing data.
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for missing data")
	}
}

func TestComputeAnalysis_NoMarketDataAvailable(t *testing.T) {
	// Positions with no market data → empty result.
	pos := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	enriched := []position.PositionWithMarket{
		{
			Position:            pos,
			MarketDataAvailable: false,
		},
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, nil, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With no market data, positions can't be weighted → sections get empty input.
	// Overlap should still exist but with empty/message state.
	if result.Overlap == nil {
		t.Fatal("overlap should not be nil")
	}
}

func TestComputeAnalysis_WarningsForMissingSymbolDetails(t *testing.T) {
	// When symbol details source is nil, a warning should be emitted.
	pos := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(10500000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	// No symbol details source.
	svc := NewService(posSource, nil, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for missing symbol details source")
	}
	found := false
	for _, w := range result.Warnings {
		if w == "symbol details source not configured" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'symbol details source not configured' warning, got: %v", result.Warnings)
	}
}

func TestComputeAnalysis_WarningsForMissingPriceData(t *testing.T) {
	// When market history source is nil, a warning should be emitted.
	pos := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	mv := decimal.MustNew(10500000, 2)
	enriched := []position.PositionWithMarket{
		makeEnrichedPosition(pos, mv, true, &mv),
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	// No market history source.
	svc := NewService(posSource, nil, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range result.Warnings {
		if w == "historical price data source not configured — correlation and time-series factors unavailable" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected price data warning, got: %v", result.Warnings)
	}
}

func TestComputeAnalysis_WarningsForNoMarketData(t *testing.T) {
	// When all positions lack market data, a warning should be emitted.
	pos := makePosition(1, "VOO", "USD", decimal.MustNew(10, 0), decimal.MustNew(-10000000, 2))
	enriched := []position.PositionWithMarket{
		{
			Position:            pos,
			MarketDataAvailable: false,
		},
	}

	posSource := &mockPositionSource{
		positions: []position.Position{pos},
		enriched:  enriched,
	}
	accResolver := &mockAccountResolver{
		accounts: []position.AccountRef{{ID: 1, PortfolioID: 1}},
	}
	curSource := &mockPortfolioCurrencySource{
		currencies: map[int64]string{1: "USD"},
	}

	svc := NewService(posSource, nil, nil, accResolver, curSource, nil)

	filters := AnalysisFilters{
		PortfolioID: int64Ptr(1),
	}

	result, err := svc.ComputeAnalysis(ctx, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundMarket := false
	foundEmpty := false
	for _, w := range result.Warnings {
		if w == "market data unavailable for 1 of 1 positions" {
			foundMarket = true
		}
		if w == "no market data available for any position — analysis results will be empty" {
			foundEmpty = true
		}
	}
	if !foundMarket {
		t.Errorf("expected market data unavailable warning, got: %v", result.Warnings)
	}
	if !foundEmpty {
		t.Errorf("expected empty analysis warning, got: %v", result.Warnings)
	}
}

func int64Ptr(i int64) *int64 {
	return &i
}
