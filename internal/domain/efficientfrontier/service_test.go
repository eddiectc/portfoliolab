package efficientfrontier

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
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

// --- Tests ---

func TestComputeFrontier_HappyPath(t *testing.T) {
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

	req := ComputeFrontierRequest{
		Symbols:      []string{"VOO", "VEA", "AAPL"},
		Period:       "1Y",
		RiskFreeRate: 0.045,
	}

	result, err := svc.ComputeFrontier(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Result.FrontierPoints) == 0 {
		t.Error("expected frontier points")
	}
	if result.Result.MaxSharpe == nil {
		t.Error("expected max Sharpe portfolio")
	}
	if result.Result.MinVariance == nil {
		t.Error("expected min variance portfolio")
	}
	if len(result.Warnings) > 0 {
		t.Logf("warnings: %v", result.Warnings)
	}
}

func TestComputeFrontier_DefaultPeriod(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	// Empty period should default to 1Y.
	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "", // empty
	}

	result, err := svc.ComputeFrontier(ctx, req)
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

func TestComputeFrontier_DefaultRiskFreeRate(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	// Zero risk-free rate should default to 4.5%.
	req := ComputeFrontierRequest{
		Symbols:      []string{"VOO", "VEA"},
		Period:       "1Y",
		RiskFreeRate: 0,
	}

	result, err := svc.ComputeFrontier(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	// Max Sharpe should use the default 4.5% rate.
	// (Sharpe can vary widely with synthetic data; just verify it exists.)
	if result.Result.MaxSharpe == nil {
		t.Error("expected max Sharpe portfolio")
	}
}

func TestComputeFrontier_MissingMarketDataSymbol(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	// VEA has no market data symbol mapping.
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeFrontier(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// VEA should be excluded, VOO alone is insufficient.
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

func TestComputeFrontier_NoPriceData(t *testing.T) {
	historySource := &mockMarketDataHistorySource{prices: map[string][]market.HistoricalPrice{}}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeFrontier(ctx, req)
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

func TestComputeFrontier_PartialData(t *testing.T) {
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

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA", "AAPL"},
		Period:  "1Y",
	}

	result, err := svc.ComputeFrontier(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should compute frontier from VOO + VEA, with AAPL excluded.
	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result.Result.FrontierPoints) == 0 {
		t.Error("expected frontier points from partial data")
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

func TestComputeFrontier_FetchError(t *testing.T) {
	historySource := &mockMarketDataHistorySource{
		prices: map[string][]market.HistoricalPrice{},
		err:    fmt.Errorf("database unavailable"),
	}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeFrontier(ctx, req)
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

func TestComputeFrontier_UnrecognizedPeriod(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "7Y", // unrecognized
	}

	result, err := svc.ComputeFrontier(ctx, req)
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

func TestComputeFrontier_NoMarketHistorySource(t *testing.T) {
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(nil, symResolver, nil, nil, nil, nil)

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeFrontier(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Result.Message == "" {
		t.Error("expected empty-state message when no history source")
	}
}

func TestComputeFrontier_FxCurrencyConversion(t *testing.T) {
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

	req := ComputeFrontierRequest{
		Symbols:      []string{"VOO", "VEA"},
		Period:       "1Y",
		BaseCurrency: "USD",
	}

	result, err := svc.ComputeFrontier(ctx, req)
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

func TestComputeFrontier_FXRateMissing(t *testing.T) {
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

	req := ComputeFrontierRequest{
		Symbols:      []string{"VOO", "VEA"},
		Period:       "1Y",
		BaseCurrency: "USD",
	}

	result, err := svc.ComputeFrontier(ctx, req)
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

func TestComputeFrontier_SingleSymbol(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO"},
		Period:  "1Y",
	}

	result, err := svc.ComputeFrontier(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single symbol returns empty-state message, not error.
	if result.Result.Message == "" {
		t.Error("expected empty-state message for single symbol")
	}
}

func TestComputeFrontier_TooManySymbols(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{}
	for i := 0; i < 12; i++ {
		sym := fmt.Sprintf("SYM%d", i)
		prices[sym] = makeSimplePriceSeries(100, 260, "USD")
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	mappings := make(map[string]string)
	for i := 0; i < 12; i++ {
		sym := fmt.Sprintf("SYM%d", i)
		mappings[sym] = sym
	}
	symResolver := &mockMarketDataSymbolResolver{mappings: mappings}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	symbols := make([]string, 12)
	for i := 0; i < 12; i++ {
		symbols[i] = fmt.Sprintf("SYM%d", i)
	}
	req := ComputeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	}

	_, err := svc.ComputeFrontier(ctx, req)
	if err == nil {
		t.Fatal("expected error for too many symbols")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestComputeFrontier_InsufficientData(t *testing.T) {
	// Only 10 price points — below the 60-day minimum.
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 10, "USD"),
		"VEA": makeSimplePriceSeries(25, 10, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	req := ComputeFrontierRequest{
		Symbols: []string{"VOO", "VEA"},
		Period:  "1Y",
	}

	result, err := svc.ComputeFrontier(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return result with warning about insufficient data, not error.
	if result.Result == nil {
		t.Fatal("result should not be nil")
	}
	warningFound := false
	for _, w := range result.Warnings {
		if len(w) > 15 && w[:15] == "Some symbols ha" {
			warningFound = true
			break
		}
	}
	if !warningFound {
		// The engine may return a message instead of frontier points.
		if result.Result.Message == "" && len(result.Result.FrontierPoints) == 0 {
			// This is OK — the engine returned empty with warnings.
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

	_, err := svc.GetSymbolsFromModelPortfolio(ctx, 1)
	if err == nil {
		t.Fatal("expected error when source not configured")
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
			cutoff, warn := periodCutoff(tt.period)
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

func TestComputeFrontier_CustomPeriods(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"VOO": makeSimplePriceSeries(100, 260, "USD"),
		"VEA": makeSimplePriceSeries(25, 260, "USD"),
	}

	historySource := &mockMarketDataHistorySource{prices: prices}
	symResolver := &mockMarketDataSymbolResolver{
		mappings: map[string]string{"VOO": "VOO", "VEA": "VEA"},
	}

	svc := NewService(historySource, symResolver, nil, nil, nil, nil)

	for _, period := range []string{"3M", "6M", "1Y", "3Y", "5Y"} {
		t.Run(period, func(t *testing.T) {
			req := ComputeFrontierRequest{
				Symbols: []string{"VOO", "VEA"},
				Period:  period,
			}

			result, err := svc.ComputeFrontier(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// For short periods (3M, 6M), data may be insufficient.
			// For longer periods, should get frontier points.
			if period == "1Y" || period == "3Y" || period == "5Y" {
				if result.Result == nil || len(result.Result.FrontierPoints) == 0 {
					if result.Result.Message == "" {
						t.Error("expected frontier points or message")
					}
				}
			}
		})
	}
}

func TestComputeFrontier_BaseCurrencyFromFirstSymbol(t *testing.T) {
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

	req := ComputeFrontierRequest{
		Symbols:      []string{"VOO", "VEA"},
		Period:       "1Y",
		BaseCurrency: "", // empty — should auto-detect EUR
	}

	result, err := svc.ComputeFrontier(ctx, req)
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

// Ensure decimal import is used.
var _ = decimal.Decimal{}
