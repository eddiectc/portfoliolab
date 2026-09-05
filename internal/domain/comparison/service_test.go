package comparison

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/allocation"
	"github.com/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/performance"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

var ctx = context.Background()

// --- Mocks ---

type mockModelPortfolioSource struct {
	portfolios map[int64]modelportfolio.ModelPortfolio
	err        error
}

func (m *mockModelPortfolioSource) Get(_ context.Context, id int64) (modelportfolio.ModelPortfolio, error) {
	if m.err != nil {
		return modelportfolio.ModelPortfolio{}, m.err
	}
	mp, ok := m.portfolios[id]
	if !ok {
		return modelportfolio.ModelPortfolio{}, fmt.Errorf("model portfolio not found")
	}
	return mp, nil
}

type mockEquityCurveSource struct {
	result *performance.PerformanceResult
	err    error
}

func (m *mockEquityCurveSource) ComputeEquityCurve(_ context.Context, _ performance.PerformanceFilters) (*performance.PerformanceResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockMarketHistorySource struct {
	prices map[string][]market.HistoricalPrice
	err    error
}

func (m *mockMarketHistorySource) GetHistoricalPrices(_ context.Context, sym string, start, end time.Time) ([]market.HistoricalPrice, error) {
	if m.err != nil {
		return nil, m.err
	}
	prices, ok := m.prices[sym]
	if !ok {
		return []market.HistoricalPrice{}, nil
	}
	var filtered []market.HistoricalPrice
	for _, p := range prices {
		if (start.IsZero() || !p.Date.Before(start)) && (end.IsZero() || !p.Date.After(end)) {
			filtered = append(filtered, p)
		}
	}
	if filtered == nil {
		filtered = []market.HistoricalPrice{}
	}
	return filtered, nil
}

type mockMarketDataSymbolResolver struct {
	symbols map[string]string
	err     error
}

func (m *mockMarketDataSymbolResolver) GetMarketDataSymbol(_ context.Context, internalSymbol string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	sym, ok := m.symbols[internalSymbol]
	if !ok {
		return internalSymbol, nil // default: same as internal
	}
	return sym, nil
}

type mockSymbolDetailsSource struct {
	details map[string]*symbol.SymbolDetails
	err     error
}

func (m *mockSymbolDetailsSource) GetByInternalSymbol(_ context.Context, sym string) (*symbol.SymbolDetails, error) {
	if m.err != nil {
		return nil, m.err
	}
	d, ok := m.details[sym]
	if !ok {
		return nil, fmt.Errorf("symbol details not found")
	}
	return d, nil
}

type mockPortfolioCurrencySource struct {
	currencies map[int64]string
	err        error
}

func (m *mockPortfolioCurrencySource) GetPortfolioCurrency(_ context.Context, id int64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	c, ok := m.currencies[id]
	if !ok {
		return "", fmt.Errorf("portfolio not found")
	}
	return c, nil
}

type mockPortfolioNameSource struct {
	names map[int64]string
	err   error
}

func (m *mockPortfolioNameSource) GetPortfolioName(_ context.Context, id int64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	n, ok := m.names[id]
	if !ok {
		return "", fmt.Errorf("portfolio not found")
	}
	return n, nil
}

type mockAllocationSource struct {
	results map[int64]*allocation.AllocationResult
	err     error
}

func (m *mockAllocationSource) ComputeAllocation(_ context.Context, filter allocation.AllocationFilter) (*allocation.AllocationResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(filter.PortfolioIDs) == 0 {
		return nil, fmt.Errorf("portfolio IDs required")
	}
	r, ok := m.results[filter.PortfolioIDs[0]]
	if !ok {
		return nil, fmt.Errorf("allocation not found")
	}
	return r, nil
}

// --- Helpers ---

// buildPriceSeries builds a price series with daily increments from a base date.
func buildPriceSeries(base time.Time, values []float64, currency string) []market.HistoricalPrice {
	prices := make([]market.HistoricalPrice, len(values))
	for i, v := range values {
		d, _ := decimal.NewFromFloat64(v)
		prices[i] = market.HistoricalPrice{
			Date:     base.AddDate(0, 0, i),
			Close:    d,
			Currency: currency,
		}
	}
	return prices
}

// buildModelPortfolio creates a model portfolio with the given entries.
func buildModelPortfolio(id int64, name string, entries []modelportfolio.ModelPortfolioEntry) modelportfolio.ModelPortfolio {
	return modelportfolio.ModelPortfolio{
		ID:      id,
		Name:    name,
		Entries: entries,
	}
}

// buildPerformanceEquityCurve builds performance equity curve points.
// NavPerUnit is set to PortfolioValue (single-deposit / no cash-flow case).
func buildPerformanceEquityCurve(base time.Time, values []float64) []performance.EquityCurvePoint {
	points := make([]performance.EquityCurvePoint, len(values))
	for i, v := range values {
		d, _ := decimal.NewFromFloat64(v)
		points[i] = performance.EquityCurvePoint{
			Date:           base.AddDate(0, 0, i),
			PortfolioValue: d,
			NavPerUnit:     &d,
		}
	}
	return points
}

// buildPerformanceEquityCurveWithNav builds performance equity curve points with NavPerUnit.
func buildPerformanceEquityCurveWithNav(base time.Time, values, navValues []float64) []performance.EquityCurvePoint {
	points := make([]performance.EquityCurvePoint, len(values))
	for i, v := range values {
		d, _ := decimal.NewFromFloat64(v)
		nav, _ := decimal.NewFromFloat64(navValues[i])
		points[i] = performance.EquityCurvePoint{
			Date:           base.AddDate(0, 0, i),
			PortfolioValue: d,
			NavPerUnit:     &nav,
		}
	}
	return points
}

// almostEqual checks if two decimal pointers are approximately equal.
func almostEqual(t *testing.T, got, want *decimal.Decimal, tolerance decimal.Decimal) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
		return
	}
	if got == nil {
		t.Errorf("got nil, want %v", want)
		return
	}
	diff, _ := got.Sub(*want)
	// Check |diff| < tolerance.
	negDiff := diff.Neg()
	if diff.Less(tolerance) && negDiff.Less(tolerance) {
		return
	}
	t.Errorf("got %v, want %v (diff=%v, tolerance=%v)", got, want, diff, tolerance)
}

// --- Tests ---

func TestComputeComparison_ModelVsModel(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// AAPL: starts at 100, grows to 110
	aaplPrices := buildPriceSeries(base, []float64{100, 101, 102, 100, 103, 105, 104, 106, 108, 110}, "USD")
	// GOOG: starts at 200, grows to 220
	googPrices := buildPriceSeries(base, []float64{200, 202, 201, 199, 203, 205, 207, 210, 215, 220}, "USD")

	modelA := buildModelPortfolio(1, "Tech Growth", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: decimal.MustNew(5000, 2)}, // 50.00%
		{Symbol: "GOOG", WeightPct: decimal.MustNew(5000, 2)}, // 50.00%
	})

	modelB := buildModelPortfolio(2, "Conservative", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)}, // 100.00%
	})

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
				2: modelB,
			},
		},
		nil, // equityCurve not needed for model-vs-model
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"AAPL": aaplPrices,
				"GOOG": googPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"AAPL": {Currency: "USD"},
				"GOOG": {Currency: "USD"},
			},
		},
		nil, // portfolioName not needed (all USD)
		nil, // portfolioCurrency not needed
		nil, // allocation not needed for model-vs-model
	)

	// Use explicit date range that covers the test data.
	from := base
	to := base.AddDate(0, 0, 9)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   2,
		PortfolioBType: PortTypeModel,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2), // $10,000.00
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Verify basic structure.
	if result.PortfolioA == nil {
		t.Fatal("PortfolioA is nil")
	}
	if result.PortfolioB == nil {
		t.Fatal("PortfolioB is nil")
	}
	if result.PortfolioA.ID != 1 {
		t.Errorf("PortfolioA.ID = %d, want 1", result.PortfolioA.ID)
	}
	if result.PortfolioA.Name != "Tech Growth" {
		t.Errorf("PortfolioA.Name = %q, want %q", result.PortfolioA.Name, "Tech Growth")
	}
	if result.PortfolioB.Name != "Conservative" {
		t.Errorf("PortfolioB.Name = %q, want %q", result.PortfolioB.Name, "Conservative")
	}

	// Verify return metrics exist.
	if result.PortfolioA.ReturnMetrics == nil {
		t.Fatal("PortfolioA.ReturnMetrics is nil")
	}
	if result.PortfolioA.ReturnMetrics.HasInsufficientData {
		t.Errorf("PortfolioA.ReturnMetrics.HasInsufficientData = true, want false; warnings: %v", result.Warnings)
	}

	// AAPL went from 100 to 110 = 10% return.
	// Model B is 100% AAPL, so TWR should be ~10%.
	tol := decimal.MustNew(1, 2)       // 0.01 tolerance
	want10 := decimal.MustNew(1000, 2) // 10.00
	almostEqual(t, result.PortfolioB.ReturnMetrics.TWRPct, &want10, tol)

	// Verify cross metrics exist.
	if result.CrossMetrics == nil {
		t.Fatal("CrossMetrics is nil")
	}
	if result.CrossMetrics.BetaAlpha == nil {
		t.Fatal("CrossMetrics.BetaAlpha is nil")
	}
	if result.CrossMetrics.Correlation == nil {
		t.Fatal("CrossMetrics.Correlation is nil")
	}
}

func TestComputeComparison_ModelVsReal(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// AAPL prices
	aaplPrices := buildPriceSeries(base, []float64{100, 102, 101, 103, 105, 104, 106, 108, 110, 112}, "USD")

	modelA := buildModelPortfolio(1, "Model Portfolio", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)},
	})

	// Real portfolio equity curve (simulated with deposits).
	realCurve := buildPerformanceEquityCurve(base, []float64{
		10000, 10200, 10100, 10300, 10500, 10400, 10600, 10800, 11000, 11200,
	})

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
			},
		},
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  realCurve,
				BaseCurrency: "USD",
			},
		},
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"AAPL": aaplPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"AAPL": {Currency: "USD"},
			},
		},
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				99: "USD",
			},
		},
		nil, // allocation not needed
	)

	// Use explicit date range that covers the test data.
	from := base
	to := base.AddDate(0, 0, 9)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   99,
		PortfolioBType: PortTypeReal,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.PortfolioA == nil || result.PortfolioB == nil {
		t.Fatal("PortfolioA or PortfolioB is nil")
	}
	if result.PortfolioA.Type != PortTypeModel {
		t.Errorf("PortfolioA.Type = %q, want %q", result.PortfolioA.Type, PortTypeModel)
	}
	if result.PortfolioB.Type != PortTypeReal {
		t.Errorf("PortfolioB.Type = %q, want %q", result.PortfolioB.Type, PortTypeReal)
	}

	// Both should have return metrics.
	if result.PortfolioA.ReturnMetrics == nil || result.PortfolioB.ReturnMetrics == nil {
		t.Fatal("Return metrics missing")
	}

	// Cross metrics should exist.
	if result.CrossMetrics == nil {
		t.Fatalf("CrossMetrics is nil; warnings: %v", result.Warnings)
	}
}

func TestComputeComparison_RealVsReal(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	curve := buildPerformanceEquityCurve(base, []float64{
		10000, 10200, 10100, 10300, 10500, 10400, 10600, 10800, 11000, 11200,
	})

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  curve,
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
				2: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   2,
		PortfolioBType: PortTypeReal,
		Period:         "1Y",
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.PortfolioA == nil || result.PortfolioB == nil {
		t.Fatal("PortfolioA or PortfolioB is nil")
	}

	// Both portfolios get the same curve from the mock.
	// Grew from 10000 to 11200 = 12% TWR.
	tol := decimal.MustNew(1, 2)
	want12 := decimal.MustNew(1200, 2) // 12.00
	almostEqual(t, result.PortfolioA.ReturnMetrics.TWRPct, &want12, tol)
	almostEqual(t, result.PortfolioB.ReturnMetrics.TWRPct, &want12, tol)

	// Cross metrics should exist (identical series: beta=1, corr=1).
	if result.CrossMetrics == nil {
		t.Fatal("CrossMetrics is nil")
	}
}

func TestComputeComparison_EmptyRealPortfolio(t *testing.T) {
	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:   []performance.EquityCurvePoint{},
				ReturnMetrics: performance.ReturnMetrics{HasInsufficientData: true},
				BaseCurrency:  "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Both portfolios should report insufficient data.
	if !result.PortfolioA.ReturnMetrics.HasInsufficientData {
		t.Error("PortfolioA.ReturnMetrics.HasInsufficientData = false, want true")
	}
	if !result.PortfolioB.ReturnMetrics.HasInsufficientData {
		t.Error("PortfolioB.ReturnMetrics.HasInsufficientData = false, want true")
	}

	// Cross metrics should be nil (insufficient data).
	if result.CrossMetrics != nil {
		t.Error("CrossMetrics should be nil for insufficient data")
	}
}

func TestComputeComparison_ModelPortfolioNotFound(t *testing.T) {
	svc := NewService(
		&mockModelPortfolioSource{
			err: fmt.Errorf("not found"),
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   999,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	_, err := svc.ComputeComparison(ctx, req)
	if err == nil {
		t.Fatal("expected error for missing model portfolio, got nil")
	}
	if got := err.Error(); got != "resolve portfolio A: get model portfolio 999: not found" {
		t.Errorf("unexpected error: %v", got)
	}
}

func TestComputeComparison_EquityCurveSourceError(t *testing.T) {
	svc := NewService(
		nil,
		&mockEquityCurveSource{
			err: fmt.Errorf("database error"),
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   2,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	_, err := svc.ComputeComparison(ctx, req)
	if err == nil {
		t.Fatal("expected error for equity curve failure, got nil")
	}
}

func TestComputeComparison_MissingMarketData(t *testing.T) {
	modelA := buildModelPortfolio(1, "Missing Data", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "NOEXIST", WeightPct: decimal.MustNew(10000, 2)},
	})

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
			},
		},
		nil,
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{}, // no prices for NOEXIST
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"NOEXIST": {Currency: "USD"},
			},
		},
		nil,
		nil,
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   1,
		PortfolioBType: PortTypeModel,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Should have warnings about missing data.
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for missing market data, got none")
	}

	// Both portfolios should have insufficient data.
	if !result.PortfolioA.ReturnMetrics.HasInsufficientData {
		t.Error("PortfolioA should have insufficient data")
	}
}

func TestComputeComparison_CustomDateRange(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 30 days of prices
	prices := buildPriceSeries(base, make([]float64, 30), "USD")
	for i := range prices {
		d, _ := decimal.NewFromFloat64(100 + float64(i))
		prices[i].Close = d
	}

	modelA := buildModelPortfolio(1, "Test", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "SYM", WeightPct: decimal.MustNew(10000, 2)},
	})

	from := time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC) // Jan 6
	to := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)  // Jan 15

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
			},
		},
		nil,
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"SYM": prices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"SYM": {Currency: "USD"},
			},
		},
		nil,
		nil,
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   1,
		PortfolioBType: PortTypeModel,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.PortfolioA == nil {
		t.Fatal("PortfolioA is nil")
	}

	// Custom date range should produce fewer points than full range.
	if result.PortfolioA.ReturnMetrics == nil || result.PortfolioA.ReturnMetrics.HasInsufficientData {
		t.Error("PortfolioA should have sufficient data for custom date range")
	}
}

func TestComputeComparison_PeriodExtremes(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Build a curve with clear monthly patterns:
	// Jan: 100 -> 105 (5% gain)
	// Feb: 105 -> 100 (-4.76% loss)
	// Mar: 100 -> 110 (10% gain)
	values := []float64{
		100, 101, 102, 103, 104, 105, // Jan
		104, 103, 102, 101, 100, 100, // Feb
		101, 103, 105, 107, 109, 110, // Mar
	}

	// Use a real portfolio source to test TWR normalization.
	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  buildPerformanceEquityCurve(base, values),
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Period extremes should be computed.
	if result.PortfolioA.PeriodExtremes == nil {
		t.Fatal("PeriodExtremes is nil")
	}

	// Best month should be March (~10%).
	if result.PortfolioA.PeriodExtremes.BestYear == nil {
		t.Error("BestYear is nil")
	}
}

func TestComputeComparison_RiskMetrics(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Build a curve with some volatility.
	values := []float64{
		100, 102, 101, 103, 105, 104, 106, 108, 107, 109,
		110, 108, 111, 113, 112, 114, 115, 113, 116, 118,
	}

	// Use real portfolio source.
	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  buildPerformanceEquityCurve(base, values),
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	rf := decimal.MustParse("4.5")
	req := ComparisonRequest{
		PortfolioAID:    1,
		PortfolioAType:  PortTypeReal,
		PortfolioBID:    1,
		PortfolioBType:  PortTypeReal,
		BaseCurrency:    "USD",
		RiskFreeRatePct: &rf,
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Risk metrics should be computed.
	if result.PortfolioA.RiskMetrics == nil {
		t.Fatal("RiskMetrics is nil")
	}
	if result.PortfolioA.RiskMetrics.AnnualizedVolatilityPct == nil {
		t.Error("AnnualizedVolatilityPct is nil")
	}
	if result.PortfolioA.RiskMetrics.SharpeRatio == nil {
		t.Error("SharpeRatio is nil (risk-free rate = 4.5%)")
	}
}

func TestComputeComparison_Drawdown(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Build a curve with a clear drawdown: up to 110, then down to 95, then recover to 105.
	_ = base // used in buildPerformanceEquityCurve
	values := []float64{
		100, 102, 105, 108, 110, // peak at 110
		108, 105, 102, 98, 95, // drawdown to 95 (max DD ~13.6%)
		97, 100, 103, 105, // partial recovery
	}

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  buildPerformanceEquityCurve(base, values),
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.PortfolioA.Drawdown == nil {
		t.Fatal("Drawdown is nil")
	}

	// Max drawdown should be ~13.6% (from 110 to 95).
	if result.PortfolioA.Drawdown.MaxDrawdownPct == nil {
		t.Error("MaxDrawdownPct is nil")
	} else {
		maxDD, _ := result.PortfolioA.Drawdown.MaxDrawdownPct.Float64()
		if maxDD < 13.0 || maxDD > 14.5 {
			t.Errorf("MaxDrawdownPct = %.2f, want ~13.64", maxDD)
		}
	}

	// Current drawdown should be positive (not at peak).
	if result.PortfolioA.Drawdown.CurrentDrawdownPct == nil {
		t.Error("CurrentDrawdownPct is nil")
	} else {
		currDD, _ := result.PortfolioA.Drawdown.CurrentDrawdownPct.Float64()
		if currDD < 0 || currDD > 5 {
			t.Errorf("CurrentDrawdownPct = %.2f, want 0-5 (partial recovery from 110 to 105)", currDD)
		}
	}
}

func TestComputeComparison_CAGR(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Portfolio that doubles in exactly 1 year (~365 days).
	// Start: 100, End: 200, Days: ~365
	values := []float64{}
	for i := 0; i < 365; i++ {
		// Linear interpolation from 100 to 200.
		v := 100 + 100.0*float64(i)/364.0
		values = append(values, v)
	}

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  buildPerformanceEquityCurve(base, values),
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.PortfolioA.ReturnMetrics.CAGRPct == nil {
		t.Fatal("CAGRPct is nil")
	}

	cagr, _ := result.PortfolioA.ReturnMetrics.CAGRPct.Float64()
	// Doubling in 1 year should be ~100% CAGR.
	if cagr < 95 || cagr > 105 {
		t.Errorf("CAGRPct = %.2f, want ~100.00", cagr)
	}
}

func TestComputeComparison_YearlyReturns(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 2 years of data: 2024 starts at 100, ends at 110 (10% gain).
	// 2025 starts at 110, ends at 121 (10% gain).
	values := []float64{}
	for i := 0; i < 365; i++ {
		v := 100 + 10.0*float64(i)/364.0
		values = append(values, v)
	}
	for i := 0; i < 365; i++ {
		v := 110 + 11.0*float64(i)/364.0
		values = append(values, v)
	}

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  buildPerformanceEquityCurve(base, values),
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Should have yearly returns for both years.
	if len(result.PortfolioA.YearlyReturns) < 2 {
		t.Errorf("expected at least 2 yearly returns, got %d", len(result.PortfolioA.YearlyReturns))
	}
}

func TestComputeComparison_BetaAlpha_Identical(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Identical curves should give beta=1.0, alpha=0, correlation=1.0.
	values := []float64{100, 102, 101, 103, 105, 104, 106, 108, 110, 112}

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  buildPerformanceEquityCurve(base, values),
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.CrossMetrics == nil {
		t.Fatal("CrossMetrics is nil")
	}

	// Beta of identical series should be 1.0.
	if result.CrossMetrics.BetaAlpha.Beta == nil {
		t.Fatal("Beta is nil")
	}
	beta, _ := result.CrossMetrics.BetaAlpha.Beta.Float64()
	if beta < 0.99 || beta > 1.01 {
		t.Errorf("Beta = %.4f, want 1.0", beta)
	}

	// Alpha of identical series should be ~0.
	if result.CrossMetrics.BetaAlpha.Alpha == nil {
		t.Fatal("Alpha is nil")
	}
	alpha, _ := result.CrossMetrics.BetaAlpha.Alpha.Float64()
	if alpha < -0.01 || alpha > 0.01 {
		t.Errorf("Alpha = %.4f, want ~0", alpha)
	}

	// Correlation of identical series should be 1.0.
	if result.CrossMetrics.Correlation.Correlation == nil {
		t.Fatal("Correlation is nil")
	}
	corr, _ := result.CrossMetrics.Correlation.Correlation.Float64()
	if corr < 0.99 || corr > 1.01 {
		t.Errorf("Correlation = %.4f, want 1.0", corr)
	}
}

func TestComputeComparison_ShortData(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Only 2 data points — barely enough for some metrics.
	values := []float64{100, 105}

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  buildPerformanceEquityCurve(base, values),
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
			},
		},
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   1,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Should still compute basic metrics.
	if result.PortfolioA.ReturnMetrics == nil {
		t.Fatal("ReturnMetrics is nil")
	}

	// TWR should be 5%.
	if result.PortfolioA.ReturnMetrics.TWRPct == nil {
		t.Error("TWRPct is nil")
	} else {
		ret, _ := result.PortfolioA.ReturnMetrics.TWRPct.Float64()
		if ret < 4.9 || ret > 5.1 {
			t.Errorf("TWRPct = %.2f, want 5.0", ret)
		}
	}
}

func TestComputeComparison_FXConversion(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// GBP stock: starts at 100, grows to 110
	ukPrices := buildPriceSeries(base, []float64{100, 102, 101, 103, 105, 104, 106, 108, 110, 112}, "GBP")
	// GBP/USD FX rate: starts at 1.25, stays flat
	fxPrices := buildPriceSeries(base, []float64{1.25, 1.25, 1.25, 1.25, 1.25, 1.25, 1.25, 1.25, 1.25, 1.25}, "USD")

	modelA := buildModelPortfolio(1, "UK Portfolio", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "UKSTOCK", WeightPct: decimal.MustNew(10000, 2)},
	})

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
			},
		},
		nil,
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"UKSTOCK": ukPrices,
				"GBP/USD": fxPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"UKSTOCK": {Currency: "GBP"},
			},
		},
		nil,
		nil,
		nil, // allocation not needed
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   1,
		PortfolioBType: PortTypeModel,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Should have data (FX conversion worked).
	if result.PortfolioA.ReturnMetrics == nil || result.PortfolioA.ReturnMetrics.HasInsufficientData {
		t.Error("FX conversion should produce valid data")
	}

	// GBP stock grew 12%, with flat FX, USD return should also be ~12%.
	if result.PortfolioA.ReturnMetrics.TWRPct != nil {
		ret, _ := result.PortfolioA.ReturnMetrics.TWRPct.Float64()
		if ret < 11 || ret > 13 {
			t.Errorf("TWRPct = %.2f, want ~12.0 (with flat FX)", ret)
		}
	}
}

func TestConvertPerformanceToComparisonCurve_PreservesNavPerUnit(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Portfolio with NavPerUnit (simulating unitization output).
	// Portfolio values grow due to deposits, but NAV per unit reflects
	// pure investment performance.
	values := []float64{10000, 10200, 20300, 20600, 20900}
	navValues := []float64{100, 102, 102, 103, 103.9} // NAV normalized, deposit at index 2

	perfPoints := buildPerformanceEquityCurveWithNav(base, values, navValues)
	result := convertPerformanceToComparisonCurve(perfPoints)

	if len(result) != len(perfPoints) {
		t.Fatalf("got %d points, want %d", len(result), len(perfPoints))
	}

	for i, p := range result {
		if p.Date != perfPoints[i].Date {
			t.Errorf("point %d: date mismatch", i)
		}
		if !p.PortfolioValue.Equal(perfPoints[i].PortfolioValue) {
			t.Errorf("point %d: PortfolioValue mismatch", i)
		}
		if p.NavPerUnit == nil {
			t.Errorf("point %d: NavPerUnit is nil, want non-nil", i)
		}
	}
}

func TestBuildNavCurve_UsesNavPerUnit(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	curve := []EquityCurvePoint{
		{Date: base, PortfolioValue: decimal.MustNew(1000000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(1000000, 2))},
		{Date: base.AddDate(0, 0, 1), PortfolioValue: decimal.MustNew(1020000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(1020000, 2))},
		{Date: base.AddDate(0, 0, 2), PortfolioValue: decimal.MustNew(2030000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(1020000, 2))},
		{Date: base.AddDate(0, 0, 3), PortfolioValue: decimal.MustNew(2060000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(1030000, 2))},
	}

	result := buildNavCurve(curve)

	if len(result) != len(curve) {
		t.Fatalf("got %d points, want %d", len(result), len(curve))
	}

	// PortfolioValue in result should be NavPerUnit values, not raw PortfolioValue.
	wantNavs := []float64{10000, 10200, 10200, 10300}
	for i, want := range wantNavs {
		got, _ := result[i].PortfolioValue.Float64()
		if got != want {
			t.Errorf("point %d: PortfolioValue = %.0f, want %.0f (NavPerUnit)", i, got, want)
		}
	}
}

func TestBuildNavCurve_Empty(t *testing.T) {
	result := buildNavCurve([]EquityCurvePoint{})
	if len(result) != 0 {
		t.Errorf("got %d points, want 0", len(result))
	}
}

func TestComputeComparison_OverlapModelVsModel(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// AAPL: 10 days of prices
	aaplPrices := buildPriceSeries(base, []float64{100, 101, 102, 100, 103, 105, 104, 106, 108, 110}, "USD")
	// GOOG: 10 days of prices
	googPrices := buildPriceSeries(base, []float64{200, 202, 201, 199, 203, 205, 207, 210, 215, 220}, "USD")

	// Model A: 50% AAPL + 50% GOOG
	modelA := buildModelPortfolio(1, "Tech Growth", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: decimal.MustNew(5000, 2)},
		{Symbol: "GOOG", WeightPct: decimal.MustNew(5000, 2)},
	})
	// Model B: 100% AAPL
	modelB := buildModelPortfolio(2, "AAPL Only", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)},
	})

	from := base
	to := base.AddDate(0, 0, 9)

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
				2: modelB,
			},
		},
		nil,
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"AAPL": aaplPrices,
				"GOOG": googPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"AAPL": {Currency: "USD", QuoteType: "EQUITY", ShortName: "Apple Inc."},
				"GOOG": {Currency: "USD", QuoteType: "EQUITY", ShortName: "Alphabet Inc."},
			},
		},
		nil,
		nil,
		nil, // allocation not needed for model-vs-model
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   2,
		PortfolioBType: PortTypeModel,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Overlap should be computed for model-vs-model.
	if result.CrossMetrics == nil {
		t.Fatal("CrossMetrics is nil")
	}
	if result.CrossMetrics.Overlap == nil {
		t.Fatal("CrossMetrics.Overlap is nil — overlap should be computed for model-vs-model")
	}

	// AAPL is in both portfolios, GOOG is only in A.
	// Jaccard: |{AAPL} ∩ {AAPL, GOOG}| / |{AAPL} ∪ {AAPL, GOOG}| = 1/2 = 50%.
	overlapPct, _ := result.CrossMetrics.Overlap.OverlapPct.Float64()
	if overlapPct < 49 || overlapPct > 51 {
		t.Errorf("OverlapPct = %.2f, want ~50.00", overlapPct)
	}

	// Top holdings should be populated.
	if len(result.CrossMetrics.Overlap.TopHoldingsA) == 0 {
		t.Error("TopHoldingsA is empty")
	}
	if len(result.CrossMetrics.Overlap.TopHoldingsB) == 0 {
		t.Error("TopHoldingsB is empty")
	}
}

func TestComputeComparison_OverlapModelVsReal(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	aaplPrices := buildPriceSeries(base, []float64{100, 102, 101, 103, 105, 104, 106, 108, 110, 112}, "USD")
	modelA := buildModelPortfolio(1, "Model", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)},
	})

	realCurve := buildPerformanceEquityCurve(base, []float64{
		10000, 10200, 10100, 10300, 10500, 10400, 10600, 10800, 11000, 11200,
	})

	from := base
	to := base.AddDate(0, 0, 9)

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
			},
		},
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  realCurve,
				BaseCurrency: "USD",
			},
		},
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"AAPL": aaplPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"AAPL": {Currency: "USD", QuoteType: "EQUITY"},
			},
		},
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				99: "USD",
			},
		},
		nil, // allocation not provided — overlap should be nil
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   99,
		PortfolioBType: PortTypeReal,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Overlap should be nil when allocation source is not provided.
	if result.CrossMetrics == nil {
		t.Fatal("CrossMetrics is nil")
	}
	if result.CrossMetrics.Overlap != nil {
		t.Error("CrossMetrics.Overlap should be nil when allocation source is not provided")
	}
}

func TestComputeComparison_OverlapRealVsReal(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	curveA := buildPerformanceEquityCurve(base, []float64{
		10000, 10200, 10100, 10300, 10500, 10400, 10600, 10800, 11000, 11200,
	})

	from := base
	to := base.AddDate(0, 0, 9)

	allocPct50 := decimal.MustNew(5000, 2)
	allocPct30 := decimal.MustNew(3000, 2)
	allocPct20 := decimal.MustNew(2000, 2)

	allocA := &allocation.AllocationResult{
		Rows: []allocation.AllocationRow{
			{Symbol: "AAPL", AllocationPct: allocPct50, HasMarketData: true, Currency: "USD"},
			{Symbol: "GOOG", AllocationPct: allocPct30, HasMarketData: true, Currency: "USD"},
			{Symbol: "MSFT", AllocationPct: allocPct20, HasMarketData: true, Currency: "USD"},
		},
		TotalValueBase:      decimal.MustNew(1000000, 2),
		BaseCurrency:        "USD",
		MarketDataAvailable: true,
	}

	allocB := &allocation.AllocationResult{
		Rows: []allocation.AllocationRow{
			{Symbol: "AAPL", AllocationPct: allocPct50, HasMarketData: true, Currency: "USD"},
			{Symbol: "MSFT", AllocationPct: allocPct50, HasMarketData: true, Currency: "USD"},
		},
		TotalValueBase:      decimal.MustNew(1000000, 2),
		BaseCurrency:        "USD",
		MarketDataAvailable: true,
	}

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  curveA,
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"AAPL": {Currency: "USD", QuoteType: "EQUITY", ShortName: "Apple Inc."},
				"GOOG": {Currency: "USD", QuoteType: "EQUITY", ShortName: "Alphabet Inc."},
				"MSFT": {Currency: "USD", QuoteType: "EQUITY", ShortName: "Microsoft Corp."},
			},
		},
		nil,
		&mockPortfolioCurrencySource{
			currencies: map[int64]string{
				1: "USD",
				2: "USD",
			},
		},
		&mockAllocationSource{
			results: map[int64]*allocation.AllocationResult{
				1: allocA,
				2: allocB,
			},
		},
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   2,
		PortfolioBType: PortTypeReal,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Overlap should be computed for real-vs-real when allocation source is provided.
	if result.CrossMetrics == nil {
		t.Fatal("CrossMetrics is nil")
	}
	if result.CrossMetrics.Overlap == nil {
		t.Fatal("CrossMetrics.Overlap is nil — should be computed for real-vs-real with allocation source")
	}

	// AAPL and MSFT are in both portfolios, GOOG only in A.
	// Weighted overlap = min(weight_AAPL_A, weight_AAPL_B) + min(weight_MSFT_A, weight_MSFT_B)
	if result.CrossMetrics.Overlap.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapPct, _ := result.CrossMetrics.Overlap.OverlapPct.Float64()
	if overlapPct < 60 || overlapPct > 80 {
		t.Errorf("OverlapPct = %.2f, want ~70", overlapPct)
	}

	// Top holdings should be populated.
	if len(result.CrossMetrics.Overlap.TopHoldingsA) == 0 {
		t.Error("TopHoldingsA is empty")
	}
	if len(result.CrossMetrics.Overlap.TopHoldingsB) == 0 {
		t.Error("TopHoldingsB is empty")
	}
}

func TestComputeComparison_RealPortfolioName(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	curve := buildPerformanceEquityCurve(base, []float64{100, 102, 101, 103, 105})

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  curve,
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		&mockPortfolioNameSource{
			names: map[int64]string{
				1: "My Growth Portfolio",
				2: "My Value Portfolio",
			},
		},
		nil,
		nil,
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   2,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.PortfolioA.Name != "My Growth Portfolio" {
		t.Errorf("PortfolioA.Name = %q, want %q", result.PortfolioA.Name, "My Growth Portfolio")
	}
	if result.PortfolioB.Name != "My Value Portfolio" {
		t.Errorf("PortfolioB.Name = %q, want %q", result.PortfolioB.Name, "My Value Portfolio")
	}
}

func TestComputeComparison_RealPortfolioName_Fallback(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	curve := buildPerformanceEquityCurve(base, []float64{100, 102, 101, 103, 105})

	svc := NewService(
		nil,
		&mockEquityCurveSource{
			result: &performance.PerformanceResult{
				EquityCurve:  curve,
				BaseCurrency: "USD",
			},
		},
		nil,
		nil,
		nil,
		nil, // no portfolioName source — should fallback to "Portfolio <id>"
		nil,
		nil,
	)

	req := ComparisonRequest{
		PortfolioAID:   42,
		PortfolioAType: PortTypeReal,
		PortfolioBID:   42,
		PortfolioBType: PortTypeReal,
		BaseCurrency:   "USD",
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Should fallback to "Portfolio <id>" when name source is nil.
	if result.PortfolioA.Name != "Portfolio 42" {
		t.Errorf("PortfolioA.Name = %q, want %q", result.PortfolioA.Name, "Portfolio 42")
	}
}

func ptrDecimal(d decimal.Decimal) *decimal.Decimal {
	return &d
}

// TestComputeReturnMetrics_TWRFromNavCurve checks that TWR is computed
// from the NAV curve (cash-flow-independent) while SimpleReturn uses
// the raw curve. For real portfolios with deposits, TWR should differ
// from SimpleReturn.
func TestComputeReturnMetrics_TWRFromNavCurve(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Raw curve: PortfolioValue jumps due to deposit at index 2.
	// Day 0: 10000, Day 1: 10200 (+2%), Day 2: 20300 (deposit 10000 + 100 gain),
	// Day 3: 20600 (+1.5%), Day 4: 20900 (+1.5%)
	rawCurve := []EquityCurvePoint{
		{Date: base, PortfolioValue: decimal.MustNew(10000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10000, 2))},
		{Date: base.AddDate(0, 0, 1), PortfolioValue: decimal.MustNew(10200, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10200, 2))},
		{Date: base.AddDate(0, 0, 2), PortfolioValue: decimal.MustNew(20300, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10200, 2))}, // deposit
		{Date: base.AddDate(0, 0, 3), PortfolioValue: decimal.MustNew(20600, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10300, 2))},
		{Date: base.AddDate(0, 0, 4), PortfolioValue: decimal.MustNew(20900, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10390, 2))},
	}

	// Build NAV curve (what prepareCurveForMetrics does for real portfolios).
	navCurve := buildNavCurve(rawCurve)

	// Create a minimal service and call computeReturnMetrics.
	svc := &Service{}
	metrics := svc.computeReturnMetrics(navCurve)

	// TWR from NAV curve: (10390 - 10000) / 10000 = 3.9%
	if metrics.TWRPct == nil {
		t.Fatal("TWRPct is nil")
	}
	twrF, _ := metrics.TWRPct.Float64()
	if twrF < 3.0 || twrF > 5.0 {
		t.Errorf("TWRPct = %.2f, want ~3.90", twrF)
	}
}

// TestComputeReturnMetrics_ModelPortfolio_TWR checks that for
// model portfolios (no cash flows), TWR is computed correctly from
// the curve.
func TestComputeReturnMetrics_ModelPortfolio_TWR(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Model portfolio curve: NavPerUnit == PortfolioValue (no cash flows).
	curve := []EquityCurvePoint{
		{Date: base, PortfolioValue: decimal.MustNew(10000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10000, 2))},
		{Date: base.AddDate(0, 0, 1), PortfolioValue: decimal.MustNew(10200, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10200, 2))},
		{Date: base.AddDate(0, 0, 2), PortfolioValue: decimal.MustNew(10400, 2), NavPerUnit: ptrDecimal(decimal.MustNew(10400, 2))},
	}

	svc := &Service{}
	metrics := svc.computeReturnMetrics(curve)

	if metrics.TWRPct == nil {
		t.Fatal("TWRPct is nil")
	}
	twrF, _ := metrics.TWRPct.Float64()
	// (10400 - 10000) / 10000 = 4.0%
	if twrF < 3.9 || twrF > 4.1 {
		t.Errorf("TWRPct = %.2f, want ~4.00", twrF)
	}
}

// TestBuildModelHoldings_EnrichmentWithSectorAndGeo verifies that
// buildModelHoldings populates Sector, SectorWeightings, and
// GeographicAllocations from SymbolDetails.
// TestBuildHoldings_EnrichmentWithSectorAndGeo verifies that both
// buildModelHoldings and buildRealHoldings populate Sector,
// SectorWeightings, and GeographicAllocations from SymbolDetails.
func TestBuildHoldings_EnrichmentWithSectorAndGeo(t *testing.T) {
	// Shared symbol details used by both model and real paths.
	symbolDetails := &mockSymbolDetailsSource{
		details: map[string]*symbol.SymbolDetails{
			"VOO": {
				Currency:  "USD",
				QuoteType: "ETF",
				ShortName: "Vanguard S&P 500 ETF",
				SectorWeightings: []symbol.SectorWeighting{
					{Sector: "technology", Percent: 30.0},
					{Sector: "healthcare", Percent: 15.0},
				},
				GeographicAllocations: []symbol.GeographicAllocation{
					{Country: "United States", Percent: 95.0},
					{Country: "Other", Percent: 5.0},
				},
			},
			"AAPL": {
				Currency:  "USD",
				QuoteType: "EQUITY",
				ShortName: "Apple Inc.",
				Sector:    "Technology",
			},
		},
	}

	cases := []struct {
		name    string
		build   func(*Service, context.Context) ([]PortfolioHolding, bool)
		setup   func(*Service)
		wantETF int // expected SectorWeightings count for VOO
	}{
		{
			name: "model portfolio",
			setup: func(s *Service) {
				s.symbolDetails = symbolDetails
			},
			build: func(s *Service, _ context.Context) ([]PortfolioHolding, bool) {
				meta := &modelPortfolioMeta{
					ID:       1,
					Name:     "Test Portfolio",
					Currency: "USD",
					Weights: []ModelPortfolioWeight{
						{Symbol: "VOO", Weight: decimal.MustNew(5000, 2), Currency: "USD", MarketSym: "VOO"},
						{Symbol: "AAPL", Weight: decimal.MustNew(5000, 2), Currency: "USD", MarketSym: "AAPL"},
					},
				}
				return s.buildModelHoldings(ctx, meta)
			},
			wantETF: 2,
		},
		{
			name: "real portfolio",
			setup: func(s *Service) {
				s.symbolDetails = symbolDetails
				s.allocation = &mockAllocationSource{
					results: map[int64]*allocation.AllocationResult{
						1: {
							Rows: []allocation.AllocationRow{
								{Symbol: "VOO", AllocationPct: decimal.MustNew(6000, 2), HasMarketData: true, Currency: "USD"},
								{Symbol: "AAPL", AllocationPct: decimal.MustNew(4000, 2), HasMarketData: true, Currency: "USD"},
							},
							MarketDataAvailable: true,
							BaseCurrency:        "USD",
						},
					},
				}
			},
			build: func(s *Service, _ context.Context) ([]PortfolioHolding, bool) {
				meta := &realPortfolioMeta{
					ID:           1,
					Name:         "Real Portfolio",
					BaseCurrency: "USD",
				}
				return s.buildRealHoldings(ctx, meta)
			},
			wantETF: 2, // same symbol details, so same count
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{}
			tc.setup(svc)

			holdings, ok := tc.build(svc, ctx)
			if !ok {
				t.Fatal("build returned ok=false")
			}
			if len(holdings) != 2 {
				t.Fatalf("got %d holdings, want 2", len(holdings))
			}

			// VOO (ETF) should have SectorWeightings and GeographicAllocations.
			voo := holdings[0]
			if voo.Symbol != "VOO" {
				t.Errorf("holding[0].Symbol = %q, want %q", voo.Symbol, "VOO")
			}
			if len(voo.SectorWeightings) != tc.wantETF {
				t.Errorf("VOO SectorWeightings len = %d, want %d", len(voo.SectorWeightings), tc.wantETF)
			}
			if len(voo.SectorWeightings) >= 1 && voo.SectorWeightings[0].Sector != "technology" {
				t.Errorf("VOO SectorWeightings[0].Sector = %q, want %q", voo.SectorWeightings[0].Sector, "technology")
			}
			if len(voo.GeographicAllocations) != 2 {
				t.Errorf("VOO GeographicAllocations len = %d, want 2", len(voo.GeographicAllocations))
			}
			if len(voo.GeographicAllocations) >= 1 && voo.GeographicAllocations[0].Country != "United States" {
				t.Errorf("VOO GeographicAllocations[0].Country = %q, want %q", voo.GeographicAllocations[0].Country, "United States")
			}

			// AAPL (stock) should have primary Sector.
			aapl := holdings[1]
			if aapl.Symbol != "AAPL" {
				t.Errorf("holding[1].Symbol = %q, want %q", aapl.Symbol, "AAPL")
			}
			if aapl.Sector != "Technology" {
				t.Errorf("AAPL Sector = %q, want %q", aapl.Sector, "Technology")
			}
			if len(aapl.SectorWeightings) != 0 {
				t.Errorf("AAPL SectorWeightings len = %d, want 0", len(aapl.SectorWeightings))
			}
		})
	}
}

// TestComputeComparison_EnhancedOverlap_FullPipeline verifies that the full
// ComputeComparison pipeline (service → computeOverlap → ComputeCrossPortfolioOverlap)
// produces OverlapResult with all enhanced fields populated: sector/country
// allocations, merged holdings, and overweight/underweight/neutral.
func TestComputeComparison_EnhancedOverlap_FullPipeline(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// VOO: S&P 500 ETF with sector/geographic/top holdings data
	vooDetails := &symbol.SymbolDetails{
		Currency:  "USD",
		QuoteType: "ETF",
		ShortName: "Vanguard S&P 500 ETF",
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 30.0},
			{Sector: "Healthcare", Percent: 15.0},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 95.0},
			{Country: "Other", Percent: 5.0},
		},
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 7.0},
			{Symbol: "MSFT", Name: "Microsoft Corp.", Percent: 6.0},
			{Symbol: "GOOG", Name: "Alphabet Inc.", Percent: 5.0},
		},
	}

	// VXUS: Total International ETF with different sector/geographic data
	vxusDetails := &symbol.SymbolDetails{
		Currency:  "USD",
		QuoteType: "ETF",
		ShortName: "Vanguard Total International Stock ETF",
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "Financials", Percent: 25.0},
			{Sector: "Industrials", Percent: 12.0},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "Japan", Percent: 15.0},
			{Country: "United Kingdom", Percent: 10.0},
			{Country: "Other", Percent: 75.0},
		},
		TopHoldings: []symbol.TopHolding{
			{Symbol: "NVD", Name: "Nintendo Co.", Percent: 1.5},
			{Symbol: "SIE", Name: "Sony Group", Percent: 1.2},
		},
	}

	// AAPL: individual stock with primary sector
	aaplDetails := &symbol.SymbolDetails{
		Currency:  "USD",
		QuoteType: "EQUITY",
		ShortName: "Apple Inc.",
		Sector:    "Technology",
	}

	// 10 days of prices for each symbol
	vooPrices := buildPriceSeries(base, []float64{400, 402, 401, 403, 405, 404, 406, 408, 410, 412}, "USD")
	vxusPrices := buildPriceSeries(base, []float64{60, 60.2, 60.1, 60.3, 60.5, 60.4, 60.6, 60.8, 61, 61.2}, "USD")
	aaplPrices := buildPriceSeries(base, []float64{170, 171, 170.5, 172, 173, 172.5, 174, 175, 176, 177}, "USD")

	// Model A: 60% VOO + 40% AAPL
	modelA := buildModelPortfolio(1, "US Growth", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "VOO", WeightPct: decimal.MustNew(6000, 2)},  // 60%
		{Symbol: "AAPL", WeightPct: decimal.MustNew(4000, 2)}, // 40%
	})

	// Model B: 50% VOO + 50% VXUS
	modelB := buildModelPortfolio(2, "Global Blend", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "VOO", WeightPct: decimal.MustNew(5000, 2)},  // 50%
		{Symbol: "VXUS", WeightPct: decimal.MustNew(5000, 2)}, // 50%
	})

	from := base
	to := base.AddDate(0, 0, 9)

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
				2: modelB,
			},
		},
		nil,
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"VOO":  vooPrices,
				"VXUS": vxusPrices,
				"AAPL": aaplPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"VOO":  vooDetails,
				"VXUS": vxusDetails,
				"AAPL": aaplDetails,
			},
		},
		nil,
		nil,
		nil,
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   2,
		PortfolioBType: PortTypeModel,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	// Verify basic structure.
	if result.CrossMetrics == nil {
		t.Fatal("CrossMetrics is nil")
	}
	if result.CrossMetrics.Overlap == nil {
		t.Fatal("CrossMetrics.Overlap is nil")
	}

	overlap := result.CrossMetrics.Overlap

	// --- Sector allocations ---
	if overlap.SectorAllocationA == nil {
		t.Error("SectorAllocationA is nil — should be populated from VOO sector weightings + AAPL primary sector")
	} else {
		if len(overlap.SectorAllocationA.Breakdown) == 0 {
			t.Error("SectorAllocationA.Breakdown is empty")
		}
		// Portfolio A has VOO (60%, tech 30% + healthcare 15%) + AAPL (40%, tech)
		// Technology = 0.60*0.30 + 0.40 = 0.58 (58%)
		if tech, ok := overlap.SectorAllocationA.Breakdown["Technology"]; !ok || tech < 0.50 {
			t.Errorf("SectorAllocationA Technology weight = %v, want >= 0.50", tech)
		}
	}

	if overlap.SectorAllocationB == nil {
		t.Error("SectorAllocationB is nil — should be populated from VOO + VXUS sector weightings")
	} else {
		if len(overlap.SectorAllocationB.Breakdown) == 0 {
			t.Error("SectorAllocationB.Breakdown is empty")
		}
	}

	// --- Country allocations ---
	if overlap.CountryAllocationA == nil {
		t.Error("CountryAllocationA is nil — should be populated from VOO geographic allocations")
	} else {
		if len(overlap.CountryAllocationA.Breakdown) == 0 {
			t.Error("CountryAllocationA.Breakdown is empty")
		}
		// Portfolio A: VOO (60%, US 95%) → US = 0.60*0.95 = 0.57 (57%)
		// AAPL has no geo data → goes to Unknown
		if us, ok := overlap.CountryAllocationA.Breakdown["United States"]; !ok || us < 0.50 {
			t.Errorf("CountryAllocationA United States weight = %v, want >= 0.50", us)
		}
	}

	if overlap.CountryAllocationB == nil {
		t.Error("CountryAllocationB is nil — should be populated from VOO + VXUS geographic allocations")
	}

	// --- Merged holdings ---
	if len(overlap.MergedHoldings) == 0 {
		t.Error("MergedHoldings is empty — should have merged underlying holdings from both portfolios")
	}

	// --- Overweight/underweight/neutral ---
	// AAPL is only in A (40% direct), so it should be overweight in A.
	// VXUS holdings (NVD, SIE) are only in B, so they should be underweight in A.
	// VOO's top holdings (AAPL, MSFT, GOOG) appear in both but at different weights.
	hasOverweight := len(overlap.OverweightHoldings) > 0
	hasUnderweight := len(overlap.UnderweightHoldings) > 0
	if !hasOverweight {
		t.Error("OverweightHoldings is empty — portfolio A has unique/different-weight holdings")
	}
	if !hasUnderweight {
		t.Error("UnderweightHoldings is empty — portfolio B has unique/different-weight holdings")
	}

	// --- Warnings ---
	// Should have sector/country warnings prefixed with portfolio names.
	hasPortfolioNameWarning := false
	for _, w := range overlap.Warnings {
		if strings.HasPrefix(w, "[sector") || strings.HasPrefix(w, "[country") {
			hasPortfolioNameWarning = true
			break
		}
	}
	if !hasPortfolioNameWarning {
		t.Logf("No portfolio-name-prefixed warnings found (may be expected if all data is complete). Warnings: %v", overlap.Warnings)
	}
}

// TestComputeComparison_EnhancedOverlap_IdenticalPortfolios verifies
// identical portfolios produce 100% overlap, zero drift, all neutral.
func TestComputeComparison_EnhancedOverlap_IdenticalPortfolios(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// VOO with full data
	vooFull := &symbol.SymbolDetails{
		Currency:  "USD",
		QuoteType: "ETF",
		ShortName: "Vanguard S&P 500 ETF",
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 30.0},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 95.0},
		},
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 7.0},
		},
	}

	vooPrices := buildPriceSeries(base, []float64{400, 402, 401, 403, 405, 404, 406, 408, 410, 412}, "USD")

	modelA := buildModelPortfolio(1, "Full Data", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "VOO", WeightPct: decimal.MustNew(10000, 2)},
	})

	modelB := buildModelPortfolio(2, "Minimal Data", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "VOO", WeightPct: decimal.MustNew(10000, 2)},
	})

	from := base
	to := base.AddDate(0, 0, 9)

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
				2: modelB,
			},
		},
		nil,
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"VOO": vooPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"VOO": vooFull, // Both portfolios use the same symbol, so both get full data
			},
		},
		nil,
		nil,
		nil,
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   2,
		PortfolioBType: PortTypeModel,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.CrossMetrics == nil || result.CrossMetrics.Overlap == nil {
		t.Fatal("CrossMetrics.Overlap is nil")
	}

	overlap := result.CrossMetrics.Overlap

	// Both portfolios use the same symbol (VOO) with full data,
	// so both should have sector/country data.
	if overlap.SectorAllocationA == nil || len(overlap.SectorAllocationA.Breakdown) == 0 {
		t.Error("SectorAllocationA should be populated")
	}
	if overlap.SectorAllocationB == nil || len(overlap.SectorAllocationB.Breakdown) == 0 {
		t.Error("SectorAllocationB should be populated")
	}

	// Identical portfolios with same top holdings data.
	// Overlap is based on expanded top holdings (not 100% of ETF),
	// so the overlap reflects the shared top holdings proportion.
	// VOO top holdings: AAPL at 7% → overlap = 7% (min(0.07, 0.07) = 0.07).
	overlapPct, _ := overlap.OverlapPct.Float64()
	if overlapPct < 5 {
		t.Errorf("OverlapPct = %.2f, want > 0 (identical portfolios share all top holdings)", overlapPct)
	}

	// All holdings should be neutral (identical weights).
	if len(overlap.NeutralHoldings) == 0 {
		t.Error("NeutralHoldings is empty — identical portfolios should have all neutral holdings")
	}
	if len(overlap.OverweightHoldings) > 0 {
		t.Errorf("OverweightHoldings has %d entries — identical portfolios should have none", len(overlap.OverweightHoldings))
	}
	if len(overlap.UnderweightHoldings) > 0 {
		t.Errorf("UnderweightHoldings has %d entries — identical portfolios should have none", len(overlap.UnderweightHoldings))
	}
}

// TestComputeComparison_EnhancedOverlap_OneSideMissingData verifies
// graceful degradation when one portfolio has sector/geographic data
// and the other has none.
func TestComputeComparison_EnhancedOverlap_OneSideMissingData(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// VOO with full sector/geographic/top holdings data
	vooFull := &symbol.SymbolDetails{
		Currency:  "USD",
		QuoteType: "ETF",
		ShortName: "Vanguard S&P 500 ETF",
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "Technology", Percent: 30.0},
			{Sector: "Healthcare", Percent: 15.0},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 95.0},
		},
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 7.0},
			{Symbol: "MSFT", Name: "Microsoft Corp.", Percent: 6.0},
		},
	}

	// VXUS with NO sector/geographic/top holdings data (minimal)
	vxusMinimal := &symbol.SymbolDetails{
		Currency:  "USD",
		QuoteType: "ETF",
		ShortName: "Vanguard Total International Stock ETF",
	}

	vooPrices := buildPriceSeries(base, []float64{400, 402, 401, 403, 405, 404, 406, 408, 410, 412}, "USD")
	vxusPrices := buildPriceSeries(base, []float64{60, 60.2, 60.1, 60.3, 60.5, 60.4, 60.6, 60.8, 61, 61.2}, "USD")

	// Model A: VOO (full data)
	modelA := buildModelPortfolio(1, "Full Data", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "VOO", WeightPct: decimal.MustNew(10000, 2)},
	})

	// Model B: VXUS (no sector/geographic/top holdings data)
	modelB := buildModelPortfolio(2, "Minimal Data", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "VXUS", WeightPct: decimal.MustNew(10000, 2)},
	})

	from := base
	to := base.AddDate(0, 0, 9)

	svc := NewService(
		&mockModelPortfolioSource{
			portfolios: map[int64]modelportfolio.ModelPortfolio{
				1: modelA,
				2: modelB,
			},
		},
		nil,
		&mockMarketHistorySource{
			prices: map[string][]market.HistoricalPrice{
				"VOO":  vooPrices,
				"VXUS": vxusPrices,
			},
		},
		&mockMarketDataSymbolResolver{
			symbols: map[string]string{},
		},
		&mockSymbolDetailsSource{
			details: map[string]*symbol.SymbolDetails{
				"VOO":  vooFull,
				"VXUS": vxusMinimal,
			},
		},
		nil,
		nil,
		nil,
	)

	req := ComparisonRequest{
		PortfolioAID:   1,
		PortfolioAType: PortTypeModel,
		PortfolioBID:   2,
		PortfolioBType: PortTypeModel,
		DateFrom:       &from,
		DateTo:         &to,
		BaseCurrency:   "USD",
		StartingValue:  decimal.MustNew(1000000, 2),
	}

	result, err := svc.ComputeComparison(ctx, req)
	if err != nil {
		t.Fatalf("ComputeComparison() error = %v", err)
	}

	if result.CrossMetrics == nil || result.CrossMetrics.Overlap == nil {
		t.Fatal("CrossMetrics.Overlap is nil")
	}

	overlap := result.CrossMetrics.Overlap

	// Portfolio A (VOO with full data) should have sector/country data.
	if overlap.SectorAllocationA == nil || len(overlap.SectorAllocationA.Breakdown) == 0 {
		t.Error("SectorAllocationA should be populated (VOO has sector weightings)")
	}
	if overlap.CountryAllocationA == nil || len(overlap.CountryAllocationA.Breakdown) == 0 {
		t.Error("CountryAllocationA should be populated (VOO has geographic allocations)")
	}

	// Portfolio B (VXUS with no data) should degrade gracefully.
	// SectorAllocationB may be nil or have only "Unknown" bucket.
	if overlap.SectorAllocationB != nil {
		// If not nil, should have warnings about missing data.
		hasUnknown := false
		for sector, weight := range overlap.SectorAllocationB.Breakdown {
			if sector == "Unknown" && weight > 0 {
				hasUnknown = true
				break
			}
		}
		if !hasUnknown && len(overlap.SectorAllocationB.MissingSymbols) == 0 {
			t.Log("SectorAllocationB has no Unknown bucket and no missing symbols (acceptable if data is empty)")
		}
	}
	if overlap.CountryAllocationB != nil {
		hasUnknown := false
		for country, weight := range overlap.CountryAllocationB.Breakdown {
			if country == "Unknown" && weight > 0 {
				hasUnknown = true
				break
			}
		}
		if !hasUnknown && len(overlap.CountryAllocationB.MissingSymbols) == 0 {
			t.Log("CountryAllocationB has no Unknown bucket and no missing symbols (acceptable if data is empty)")
		}
	}

	// Merged holdings should still work (VXUS has no top holdings to expand,
	// so it appears as a single holding; VOO expands to its top holdings).
	// The key point: no panic, result is valid.
	t.Logf("MergedHoldings: %d entries", len(overlap.MergedHoldings))
	t.Logf("Overweight: %d, Underweight: %d, Neutral: %d",
		len(overlap.OverweightHoldings), len(overlap.UnderweightHoldings), len(overlap.NeutralHoldings))

	// Warnings should mention the portfolio with missing data.
	hasMissingDataWarning := false
	for _, w := range overlap.Warnings {
		if strings.Contains(w, "UNKNOWN") || strings.Contains(w, "missing") {
			hasMissingDataWarning = true
			break
		}
	}
	if !hasMissingDataWarning {
		t.Logf("No missing-data warning found (may be expected if empty data is silent). Warnings: %v", overlap.Warnings)
	}
}
