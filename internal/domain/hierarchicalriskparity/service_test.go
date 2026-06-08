package hierarchicalriskparity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/util"
	"github.com/govalues/decimal"
)

var ctx = context.Background()

// --- Mocks ---

type mockMarketDataHistorySource struct {
	prices map[string][]market.HistoricalPrice
	err    error
}

func (m *mockMarketDataHistorySource) GetHistoricalPrices(_ context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error) {
	if m.err != nil {
		return nil, m.err
	}
	prices, ok := m.prices[symbol]
	if !ok {
		return []market.HistoricalPrice{}, nil
	}
	// Filter by date range.
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

type mockSymbolLister struct {
	symbols []string
	err     error
}

func (m *mockSymbolLister) ListAllSymbols(_ context.Context) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.symbols == nil {
		return []string{}, nil
	}
	return m.symbols, nil
}

type mockPortfolioSymbolSource struct {
	symbolsByPortfolio map[int64][]string
	err                error
}

func (m *mockPortfolioSymbolSource) GetSymbolsByPortfolio(_ context.Context, portfolioID int64) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	symbols, ok := m.symbolsByPortfolio[portfolioID]
	if !ok {
		return []string{}, nil
	}
	return symbols, nil
}

type mockModelPortfolioSource struct {
	portfolios map[int64]ModelPortfolioRef
	err        error
}

func (m *mockModelPortfolioSource) Get(_ context.Context, id int64) (ModelPortfolioRef, error) {
	if m.err != nil {
		return ModelPortfolioRef{}, m.err
	}
	ref, ok := m.portfolios[id]
	if !ok {
		return ModelPortfolioRef{}, fmt.Errorf("model portfolio %d not found", id)
	}
	return ref, nil
}

type mockFxRateSource struct {
	rates map[string]map[string]*FxRate // base -> quote -> rate
	err   error
}

func (m *mockFxRateSource) GetCurrentFxRate(_ context.Context, baseCurrency, quoteCurrency string) (*FxRate, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.rates == nil {
		return nil, nil
	}
	quoteRates, ok := m.rates[baseCurrency]
	if !ok {
		return nil, nil
	}
	rate, ok := quoteRates[quoteCurrency]
	if !ok {
		return nil, nil
	}
	return rate, nil
}

// --- Test helpers ---

func makePrice(daysAgo int, close float64, currency string) market.HistoricalPrice {
	return market.HistoricalPrice{
		Date:     time.Now().AddDate(0, 0, -daysAgo),
		Close:    testDec(close),
		Currency: currency,
	}
}

func makeSimplePriceSeries(startPrice float64, days int, currency string) []market.HistoricalPrice {
	prices := make([]market.HistoricalPrice, days)
	price := startPrice
	for i := 0; i < days; i++ {
		price *= (1 + 0.002*float64(i%7-3)) // small oscillation
		prices[i] = makePrice(days-i, price, currency)
	}
	return prices
}

func testDec(v float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat64(v)
	return d
}

// --- Tests ---

func TestComputeHrp_HappyPath(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO":  makeSimplePriceSeries(100, 260, "USD"),
		"VEA":  makeSimplePriceSeries(25, 260, "USD"),
		"AAPL": makeSimplePriceSeries(150, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA", "AAPL": "AAPL"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA", "AAPL"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Result.Allocations) != 4 {
		t.Errorf("expected 4 allocations, got %d", len(result.Result.Allocations))
	}
	// Verify each allocation has weights summing to ~1.0.
	for _, alloc := range result.Result.Allocations {
		sum := 0.0
		for _, w := range alloc.Weights {
			sum += w
		}
		if sum < 0.99 || sum > 1.01 {
			t.Errorf("allocation %s weights sum to %f, expected ~1.0", alloc.Method, sum)
		}
	}
}

func TestComputeHrp_DefaultPeriod(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 700, "USD"),
		"VEA": makeSimplePriceSeries(25, 700, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	// Empty period should default to 3Y.
	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "", // empty
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Result.Message != "" {
		t.Errorf("expected no empty-state message, got: %s", result.Result.Message)
	}
}

func TestComputeHrp_MissingMarketDataSymbol(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	// VEA has no market data symbol mapping.
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// VEA should be excluded, VOO alone is insufficient — engine error converted to empty-state.
	if result.Result.Message == "" {
		t.Error("expected empty-state message when all but one symbol excluded")
	}
	found := false
	for _, s := range result.ExcludedSymbols {
		if s == "VEA" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected VEA in excluded symbols")
	}
}

func TestComputeHrp_NoPriceData(t *testing.T) {
	historySource := &mockMarketDataHistorySource{prices: map[string][]market.HistoricalPrice{}}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result.Message == "" {
		t.Error("expected empty-state message when no price data available")
	}
	if len(result.ExcludedSymbols) != 2 {
		t.Errorf("expected 2 excluded symbols, got %d", len(result.ExcludedSymbols))
	}
}

func TestComputeHrp_PartialData(t *testing.T) {
	// VOO has data, VEA has data, AAPL has no data.
	prices := map[string][]market.HistoricalPrice{
		"VOO":  makeSimplePriceSeries(100, 260, "USD"),
		"VEA":  makeSimplePriceSeries(25, 260, "USD"),
		"AAPL": []market.HistoricalPrice{}, // no data
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA", "AAPL": "AAPL"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA", "AAPL"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should compute HRP from VOO + VEA, with AAPL excluded.
	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Result.Allocations) != 4 {
		t.Errorf("expected 4 allocations from partial data, got %d", len(result.Result.Allocations))
	}
	found := false
	for _, s := range result.ExcludedSymbols {
		if s == "AAPL" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected AAPL in excluded symbols")
	}
	// Should have warning about AAPL.
	warningFound := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "AAPL:") {
			warningFound = true
			break
		}
	}
	if !warningFound {
		t.Errorf("expected warning about AAPL, got: %v", result.Warnings)
	}
}

func TestComputeHrp_FetchError(t *testing.T) {
	historySource := &mockMarketDataHistorySource{
		prices: map[string][]market.HistoricalPrice{},
		err:    fmt.Errorf("database unavailable"),
	}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result.Message == "" {
		t.Error("expected empty-state message when fetch fails")
	}
	if len(result.ExcludedSymbols) != 2 {
		t.Errorf("expected 2 excluded symbols, got %d", len(result.ExcludedSymbols))
	}
}

func TestComputeHrp_UnrecognizedPeriod(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "7Y", // unrecognized
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have warning about unrecognized period.
	warningFound := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "unrecognized period") {
			warningFound = true
			break
		}
	}
	if !warningFound {
		t.Errorf("expected unrecognized period warning, got: %v", result.Warnings)
	}
}

func TestComputeHrp_NoMarketHistorySource(t *testing.T) {
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(nil, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result.Message == "" {
		t.Error("expected empty-state message when no history source")
	}
}

func TestComputeHrp_FxCurrencyConversion(t *testing.T) {
	// VOO in USD, VEA in GBP. FX rate GBP/USD = 1.27.
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 260, "GBP"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}
	fxSource := &mockFxRateSource{
		rates: map[string]map[string]*FxRate{
			"GBP": {"USD": {BaseCurrency: "GBP", QuoteCurrency: "USD", Rate: 1.27}},
		},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, fxSource)

	req := ComputeHrpRequest{
		Symbols:      []string{"VOO", "VEA"},
		Period:       "1Y",
		BaseCurrency: "USD",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	// Should have conversion warning for VEA.
	conversionFound := false
	for _, w := range result.Warnings {
		if len(w) > 30 && w[:30] == "VEA: prices converted from GBP" {
			conversionFound = true
			break
		}
	}
	if !conversionFound {
		t.Errorf("expected FX conversion warning for VEA, got: %v", result.Warnings)
	}
}

func TestComputeHrp_FXRateMissing(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 260, "EUR"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}
	fxSource := &mockFxRateSource{
		rates: map[string]map[string]*FxRate{}, // no EUR/USD rate
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, fxSource)

	req := ComputeHrpRequest{
		Symbols:      []string{"VOO", "VEA"},
		Period:       "1Y",
		BaseCurrency: "USD",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	// Should have warning about missing FX rate.
	warningFound := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "FX rate") {
			warningFound = true
			break
		}
	}
	if !warningFound {
		t.Errorf("expected FX rate missing warning, got: %v", result.Warnings)
	}
}

func TestComputeHrp_SingleSymbol(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single symbol triggers engine error, converted to empty-state.
	if result.Result.Message == "" {
		t.Error("expected empty-state message for single symbol")
	}
}

func TestComputeHrp_TooManySymbols(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{}
	for i := 0; i < 22; i++ {
		sym := fmt.Sprintf("SYM%d", i)
		prices[sym] = makeSimplePriceSeries(100, 260, "USD")
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	mappings := make(map[string]string)
	for i := 0; i < 22; i++ {
		sym := fmt.Sprintf("SYM%d", i)
		mappings[sym] = sym
	}
	symResolver := &mockMarketDataSymbolResolver{mappings: mappings}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	symbols := make([]string, 22)
	for i := 0; i < 22; i++ {
		symbols[i] = fmt.Sprintf("SYM%d", i)
	}
	req := ComputeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Too many symbols triggers engine error, converted to empty-state.
	if result.Result.Message == "" {
		t.Error("expected empty-state message for too many symbols")
	}
}

func TestComputeHrp_InsufficientData(t *testing.T) {
	// Only 2 price points — 1 return, below the minimum of 2 aligned observations.
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 2, "USD"),
		"VEA": makeSimplePriceSeries(25, 2, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Too few data points triggers engine error, converted to empty-state.
	if result.Result.Message == "" {
		t.Error("expected empty-state message for insufficient data")
	}
}

func TestComputeHrp_IdenticalSymbols(t *testing.T) {
	// Two symbols with identical price series produce a singular covariance matrix.
	// The HRP engine should handle this gracefully via equal-weight fallback.
	identicalPrices := makeSimplePriceSeries(100, 260, "USD")
	prices := map[string][]market.HistoricalPrice{
		"SYM_A": identicalPrices,
		"SYM_B": identicalPrices,
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"SYM_A": "SYM_A", "SYM_B": "SYM_B"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"SYM_A", "SYM_B"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The engine should still produce results.
	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Result.Allocations) != 4 {
		t.Errorf("expected 4 allocations, got %d", len(result.Result.Allocations))
	}
}

func TestComputeHrp_ConstantPrices(t *testing.T) {
	// Constant prices produce zero returns → zero variance → correlation 0.
	// The engine handles this gracefully: equal distances (sqrt(2)) for all pairs,
	// clustering proceeds, and weights are assigned.
	constPrices := func() []market.HistoricalPrice {
		prices := make([]market.HistoricalPrice, 100)
		for i := range prices {
			prices[i] = makePrice(i, 100.0, "USD") // all same price
		}
		return prices
	}()

	prices := map[string][]market.HistoricalPrice{
		"SYM_A": constPrices,
		"SYM_B": constPrices,
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"SYM_A": "SYM_A", "SYM_B": "SYM_B"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"SYM_A", "SYM_B"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	// Engine succeeds: zero-variance series get correlation 0, distance sqrt(2).
	if len(result.Result.Allocations) != 4 {
		t.Errorf("expected 4 allocations, got %d", len(result.Result.Allocations))
	}
}

func TestComputeHrp_CustomPeriods(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 700, "USD"),
		"VEA": makeSimplePriceSeries(25, 700, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	for _, period := range []string{"1Y", "3Y", "5Y"} {
		t.Run(period, func(t *testing.T) {
			req := ComputeHrpRequest{
				Symbols: []string{"VOO", "VEA"},
				Period:  period,
			}

			result, err := svc.ComputeHrp(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Result == nil || len(result.Result.Allocations) != 4 {
				t.Error("expected 4 allocations")
			}
		})
	}
}

func TestComputeHrp_BaseCurrencyFromFirstSymbol(t *testing.T) {
	// When BaseCurrency is empty, should use first symbol's currency.
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "EUR"),
		"VEA": makeSimplePriceSeries(25, 260, "EUR"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols:      []string{"VOO", "VEA"},
		Period:       "1Y",
		BaseCurrency: "", // empty — should auto-detect EUR
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	// No FX conversion warnings since both symbols are in same currency.
	for _, w := range result.Warnings {
		if len(w) > 4 && w[:4] == "EUR/" {
			t.Errorf("unexpected FX warning: %s", w)
		}
	}
}

func TestGetCandidateSymbols_HappyPath(t *testing.T) {
	lister := &mockSymbolLister{symbols: []string{"VOO", "VEA", "AAPL", "MSFT"}}
	svc := NewService(nil, nil, lister, nil, nil, nil)

	symbols, err := svc.GetCandidateSymbols(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 4 {
		t.Errorf("expected 4 symbols, got %d", len(symbols))
	}
}

func TestGetCandidateSymbols_EmptyList(t *testing.T) {
	lister := &mockSymbolLister{symbols: []string{}}
	svc := NewService(nil, nil, lister, nil, nil, nil)

	symbols, err := svc.GetCandidateSymbols(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected empty slice, got %d symbols", len(symbols))
	}
	if symbols == nil {
		t.Error("expected non-nil empty slice")
	}
}

func TestGetCandidateSymbols_NoLister(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)

	symbols, err := svc.GetCandidateSymbols(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected empty slice, got %d symbols", len(symbols))
	}
}

func TestGetCandidateSymbols_ListError(t *testing.T) {
	lister := &mockSymbolLister{err: fmt.Errorf("database error")}
	svc := NewService(nil, nil, lister, nil, nil, nil)

	_, err := svc.GetCandidateSymbols(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetSymbolsFromPortfolio_HappyPath(t *testing.T) {
	source := &mockPortfolioSymbolSource{
		symbolsByPortfolio: map[int64][]string{
			1: {"VOO", "VEA", "AAPL"},
			2: {"MSFT", "GOOGL"},
		},
	}
	svc := NewService(nil, nil, nil, source, nil, nil)

	symbols, err := svc.GetSymbolsFromPortfolio(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(symbols))
	}
}

func TestGetSymbolsFromPortfolio_EmptyPortfolio(t *testing.T) {
	source := &mockPortfolioSymbolSource{
		symbolsByPortfolio: map[int64][]string{},
	}
	svc := NewService(nil, nil, nil, source, nil, nil)

	symbols, err := svc.GetSymbolsFromPortfolio(ctx, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected empty slice, got %d symbols", len(symbols))
	}
	if symbols == nil {
		t.Error("expected non-nil empty slice")
	}
}

func TestGetSymbolsFromPortfolio_NoSource(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)

	symbols, err := svc.GetSymbolsFromPortfolio(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected empty slice, got %d symbols", len(symbols))
	}
}

func TestGetSymbolsFromPortfolio_Error(t *testing.T) {
	source := &mockPortfolioSymbolSource{err: fmt.Errorf("database error")}
	svc := NewService(nil, nil, nil, source, nil, nil)

	_, err := svc.GetSymbolsFromPortfolio(ctx, 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetSymbolsFromModelPortfolio_HappyPath(t *testing.T) {
	source := &mockModelPortfolioSource{
		portfolios: map[int64]ModelPortfolioRef{
			1: {Symbols: []string{"VOO", "VEA", "BND"}},
			2: {Symbols: []string{"MSFT", "GOOGL", "AAPL"}},
		},
	}
	svc := NewService(nil, nil, nil, nil, source, nil)

	symbols, err := svc.GetSymbolsFromModelPortfolio(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(symbols))
	}
}

func TestGetSymbolsFromModelPortfolio_NotFound(t *testing.T) {
	source := &mockModelPortfolioSource{
		portfolios: map[int64]ModelPortfolioRef{},
	}
	svc := NewService(nil, nil, nil, nil, source, nil)

	_, err := svc.GetSymbolsFromModelPortfolio(ctx, 999)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestGetSymbolsFromModelPortfolio_NoSource(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)

	symbols, err := svc.GetSymbolsFromModelPortfolio(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected empty slice, got %d symbols", len(symbols))
	}
}

func TestGetSymbolsFromModelPortfolio_Error(t *testing.T) {
	source := &mockModelPortfolioSource{err: fmt.Errorf("database error")}
	svc := NewService(nil, nil, nil, nil, source, nil)

	_, err := svc.GetSymbolsFromModelPortfolio(ctx, 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPeriodCutoff(t *testing.T) {
	now := time.Now()
	tests := []struct {
		period   string
		wantDays int
		wantWarn bool
	}{
		{"3M", 90, false},
		{"6M", 180, false},
		{"1Y", 365, false},
		{"3Y", 1095, false},
		{"5Y", 1825, false},
		{"10Y", 3650, false},
		{"7Y", 365, true}, // unrecognized, defaults to 1Y
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			cutoff, warn := util.PeriodCutoff(tt.period)
			days := int(now.Sub(cutoff).Hours() / 24)

			// Allow ±3 day tolerance.
			if days < tt.wantDays-3 || days > tt.wantDays+3 {
				t.Errorf("expected ~%d days, got %d", tt.wantDays, days)
			}

			if tt.wantWarn && warn == "" {
				t.Error("expected warning for unrecognized period")
			}
			if !tt.wantWarn && warn != "" {
				t.Errorf("unexpected warning: %s", warn)
			}
		})
	}
}

func TestServiceResult_NilSafety(t *testing.T) {
	// Ensure ServiceResult fields are safe when nil.
	result := &ServiceResult{
		Result:          nil,
		Warnings:        nil,
		ExcludedSymbols: nil,
	}

	// These should not panic.
	_ = result.Warnings
	_ = result.ExcludedSymbols
}

func TestApproximatePeriodLabel(t *testing.T) {
	now := time.Now()
	tests := []struct {
		startDaysAgo int
		wantContains string
	}{
		{10, "D"},
		{45, "M"},
		{180, "M"},
		{365, "Y"},
		{730, "Y"},
	}

	for _, tt := range tests {
		start := now.AddDate(0, 0, -tt.startDaysAgo)
		label := approximatePeriodLabel(start, now)
		if !strings.Contains(label, tt.wantContains) {
			t.Errorf("start %d days ago: label %q should contain %q", tt.startDaysAgo, label, tt.wantContains)
		}
	}
}

func TestComputeHrp_InsufficientDataWarning(t *testing.T) {
	// VOO has full 1Y data (260 days), VEA has only 100 days — below 80% threshold.
	// Should compute HRP from both but warn about VEA's short data.
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 100, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeHrpRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still compute (both symbols have data, just VEA is short).
	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Result.Allocations) != 4 {
		t.Errorf("expected 4 allocations, got %d", len(result.Result.Allocations))
	}

	// Should have warning about VEA's insufficient data.
	veaWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "VEA:") && strings.Contains(w, "expected") && strings.Contains(w, "trading days") {
			veaWarning = true
			break
		}
	}
	if !veaWarning {
		t.Errorf("expected insufficient data warning for VEA, got: %v", result.Warnings)
	}

	// Should NOT warn about VOO (has sufficient data).
	for _, w := range result.Warnings {
		if strings.Contains(w, "VOO:") && strings.Contains(w, "expected") && strings.Contains(w, "trading days") {
			t.Errorf("unexpected insufficient data warning for VOO: %s", w)
		}
	}
}

func TestComputeHrp_PreservesSymbolOrder(t *testing.T) {
	// Verify that the result symbols preserve the order from the request.
	prices := map[string][]market.HistoricalPrice{
		"AAPL": makeSimplePriceSeries(150, 260, "USD"),
		"VOO":  makeSimplePriceSeries(100, 260, "USD"),
		"VEA":  makeSimplePriceSeries(25, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"AAPL": "AAPL", "VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	// Request in a specific order.
	req := ComputeHrpRequest{
		Symbols: []string{"VEA", "AAPL", "VOO"},
		Period:  "1Y",
	}

	result, err := svc.ComputeHrp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	// Result symbols should preserve request order.
	if len(result.Result.Symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d", len(result.Result.Symbols))
	}
	if result.Result.Symbols[0] != "VEA" {
		t.Errorf("expected first symbol VEA, got %s", result.Result.Symbols[0])
	}
	if result.Result.Symbols[1] != "AAPL" {
		t.Errorf("expected second symbol AAPL, got %s", result.Result.Symbols[1])
	}
	if result.Result.Symbols[2] != "VOO" {
		t.Errorf("expected third symbol VOO, got %s", result.Result.Symbols[2])
	}
}

func TestExpectedTradingDays(t *testing.T) {
	tests := []struct {
		period string
		want   int
	}{
		{"1Y", 252},
		{"3Y", 756},
		{"5Y", 1260},
		{"7Y", 0},   // unrecognized
		{"10Y", 0},  // unrecognized
		{"", 0},     // empty
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			got := expectedTradingDays(tt.period)
			if got != tt.want {
				t.Errorf("expectedTradingDays(%q) = %d, want %d", tt.period, got, tt.want)
			}
		})
	}
}

// Ensure decimal import is used.
var _ = decimal.Decimal{}
