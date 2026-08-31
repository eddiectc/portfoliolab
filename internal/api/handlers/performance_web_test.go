package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/performance"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

func TestRegisterRoutes_PerformanceWeb(t *testing.T) {
	r := chi.NewRouter()
	handler := &PerformanceWebHandler{}
	handler.RegisterRoutes(r)
	// Verify it doesn't panic
}

func TestSerializeChartData(t *testing.T) {
	tests := []struct {
		name string
		in   []performance.EquityCurvePoint
		want string
	}{
		{
			name: "empty nil",
			in:   nil,
			want: "[]",
		},
		{
			name: "empty slice",
			in:   []performance.EquityCurvePoint{},
			want: "[]",
		},
		{
			name: "single point",
			in: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(100000, 2), NetDeposit: decimal.MustNew(100000, 2)},
			},
			want: `[{"date":"2024-01-15","portfolio_value":"1000.00","net_deposit":"1000.00"}]`,
		},
		{
			name: "multiple points",
			in: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(100000, 2), NetDeposit: decimal.MustNew(100000, 2)},
				{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(110000, 2), NetDeposit: decimal.MustNew(100000, 2)},
			},
			want: `[{"date":"2024-01-01","portfolio_value":"1000.00","net_deposit":"1000.00"},{"date":"2024-06-01","portfolio_value":"1100.00","net_deposit":"1000.00"}]`,
		},
		{
			name: "negative values",
			in: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(-5000, 2), NetDeposit: decimal.MustNew(-5000, 2)},
			},
			want: `[{"date":"2024-03-01","portfolio_value":"-50.00","net_deposit":"-50.00"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serializeChartData(tt.in)
			if got != tt.want {
				t.Errorf("serializeChartData() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildRefreshURL(t *testing.T) {
	tests := []struct {
		name        string
		portfolioID string
		benchmark   string
		mode        string
		want        string
	}{
		{"no filter", "", "", "", "/performance/refresh"},
		{"portfolio only", "3", "", "", "/performance/refresh?portfolio_id=3"},
		{"benchmark only", "", "^GSPC", "", "/performance/refresh?benchmark=^GSPC"},
		{"portfolio + benchmark", "3", "^GSPC", "", "/performance/refresh?portfolio_id=3&benchmark=^GSPC"},
		{"mode nav", "", "", "nav", "/performance/refresh?mode=nav"},
		{"portfolio + mode nav", "3", "", "nav", "/performance/refresh?portfolio_id=3&mode=nav"},
		{"all params", "3", "^GSPC", "nav", "/performance/refresh?portfolio_id=3&benchmark=^GSPC&mode=nav"},
		{"mode equity omitted", "3", "", "equity", "/performance/refresh?portfolio_id=3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRefreshURL(tt.portfolioID, tt.benchmark, tt.mode)
			if got != tt.want {
				t.Errorf("buildRefreshURL(%q, %q, %q) = %q, want %q", tt.portfolioID, tt.benchmark, tt.mode, got, tt.want)
			}
		})
	}
}

func TestBuildPeriodURLs(t *testing.T) {
	urls := buildPeriodURLs("", "", "", "")
	if urls["All"] != "/performance" {
		t.Errorf("All = %q, want /performance", urls["All"])
	}
	if urls["1W"] != "/performance?period=1W" {
		t.Errorf("1W = %q, want /performance?period=1W", urls["1W"])
	}

	urls2 := buildPeriodURLs("5", "1Y", "", "")
	if urls2["All"] != "/performance?portfolio_id=5" {
		t.Errorf("All = %q, want /performance?portfolio_id=5", urls2["All"])
	}
	if urls2["1M"] != "/performance?portfolio_id=5&period=1M" {
		t.Errorf("1M = %q, want /performance?portfolio_id=5&period=1M", urls2["1M"])
	}

	// With benchmark — should be preserved in all period URLs.
	urls3 := buildPeriodURLs("5", "1Y", "^GSPC", "")
	if urls3["All"] != "/performance?portfolio_id=5&benchmark=^GSPC" {
		t.Errorf("All = %q, want /performance?portfolio_id=5&benchmark=^GSPC", urls3["All"])
	}
	if urls3["1M"] != "/performance?portfolio_id=5&period=1M&benchmark=^GSPC" {
		t.Errorf("1M = %q, want /performance?portfolio_id=5&period=1M&benchmark=^GSPC", urls3["1M"])
	}
	if urls3["3Y"] != "/performance?portfolio_id=5&period=3Y&benchmark=^GSPC" {
		t.Errorf("3Y = %q, want /performance?portfolio_id=5&period=3Y&benchmark=^GSPC", urls3["3Y"])
	}

	// Benchmark only, no portfolio.
	urls4 := buildPeriodURLs("", "", "^IXIC", "")
	if urls4["All"] != "/performance?benchmark=^IXIC" {
		t.Errorf("All = %q, want /performance?benchmark=^IXIC", urls4["All"])
	}
	if urls4["1W"] != "/performance?period=1W&benchmark=^IXIC" {
		t.Errorf("1W = %q, want /performance?period=1W&benchmark=^IXIC", urls4["1W"])
	}

	// With mode=nav — should be preserved in all period URLs.
	urls5 := buildPeriodURLs("5", "1Y", "", "nav")
	if urls5["All"] != "/performance?portfolio_id=5&mode=nav" {
		t.Errorf("All = %q, want /performance?portfolio_id=5&mode=nav", urls5["All"])
	}
	if urls5["1M"] != "/performance?portfolio_id=5&period=1M&mode=nav" {
		t.Errorf("1M = %q, want /performance?portfolio_id=5&period=1M&mode=nav", urls5["1M"])
	}

	// Mode=equity omitted from URLs.
	urls6 := buildPeriodURLs("5", "1Y", "", "equity")
	if urls6["All"] != "/performance?portfolio_id=5" {
		t.Errorf("All = %q, want /performance?portfolio_id=5", urls6["All"])
	}

	// All params: portfolio, period, benchmark, mode.
	urls7 := buildPeriodURLs("5", "1Y", "^GSPC", "nav")
	if urls7["3M"] != "/performance?portfolio_id=5&period=3M&benchmark=^GSPC&mode=nav" {
		t.Errorf("3M = %q, want /performance?portfolio_id=5&period=3M&benchmark=^GSPC&mode=nav", urls7["3M"])
	}
}

func TestUserFriendlyPerformanceError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "mismatched currencies",
			err:  &position.PositionError{Code: "mismatched_currencies", Message: "currencies differ"},
			want: "different base currencies",
		},
		{
			name: "other position error",
			err:  &position.PositionError{Code: "some_error", Message: "something went wrong"},
			want: "something went wrong",
		},
		{
			name: "unknown error",
			err:  errors.New("generic error"),
			want: "An error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userFriendlyPerformanceError(tt.err)
			if !strings.Contains(got, tt.want) {
				t.Errorf("userFriendlyPerformanceError() = %q, want contains %q", got, tt.want)
			}
		})
	}
}

// Test that the performance page template parses and renders without panic.
func TestPerformanceTemplate_Renders(t *testing.T) {
	renderer := newTestRenderer(t)

	// Minimal valid data — no result, no error, empty state
	data := performancePageData{
		PageData: web.PageData{
			Title: "Performance",
		},
		Portfolios: []portfolio.Portfolio{},
		PeriodURLs: map[string]string{"All": "/performance"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(body, "Performance") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "No performance data available") {
		t.Error("expected empty state message")
	}
}

// Test that the performance page template renders with data.
func TestPerformanceTemplate_WithData(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
		{Date: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(12000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	twr := decimal.MustNew(2000, 2)
	annTWR := decimal.MustNew(1900, 2)
	mwr := decimal.MustNew(1800, 2)
	hpMwr := decimal.MustNew(1750, 2)
	result := &performance.PerformanceResult{
		EquityCurve: curve,
		ReturnMetrics: performance.ReturnMetrics{
			TWRPct:              &twr,
			AnnualizedTWRPct:    &annTWR,
			MWRPct:              &mwr,
			HoldingPeriodMWRPct: &hpMwr,
		},
		BaseCurrency: "USD",
	}

	data := performancePageData{
		PageData: web.PageData{
			Title: "Performance",
		},
		Result:            result,
		ChartData:         serializeChartData(curve),
		CurrentValue:      "120000.00",
		CurrentNetDeposit: "100000.00",
		Portfolios: []portfolio.Portfolio{
			{ID: 1, Name: "Main", Currency: "USD"},
		},
		SelectedPeriod:      "1Y",
		SelectedPortfolioID: "1",
		RefreshURL:          "/performance/refresh?portfolio_id=1&period=1Y",
		PeriodURLs: map[string]string{
			"1W":  "/performance?portfolio_id=1&period=1W",
			"1M":  "/performance?portfolio_id=1&period=1M",
			"3M":  "/performance?portfolio_id=1&period=3M",
			"1Y":  "/performance?portfolio_id=1&period=1Y",
			"3Y":  "/performance?portfolio_id=1&period=3Y",
			"5Y":  "/performance?portfolio_id=1&period=5Y",
			"YTD": "/performance?portfolio_id=1&period=YTD",
			"All": "/performance?portfolio_id=1",
		},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	checkContains("chart div", `id="equity-chart"`)
	checkContains("echarts script", "echarts.min.js")
	checkContains("portfolio value", "120,000.00")
	checkContains("net deposit", "100,000.00")
	checkContains("total return", "20.00%")
	checkContains("CAGR", "19.00%")
	checkContains("base currency", "USD")
}

// Test that the performance page template renders error state.
func TestPerformanceTemplate_ErrorState(t *testing.T) {
	renderer := newTestRenderer(t)

	data := performancePageData{
		PageData: web.PageData{
			Title: "Performance",
		},
		Portfolios: []portfolio.Portfolio{},
		Error:      "Cannot combine portfolios with different base currencies.",
		PeriodURLs: map[string]string{"All": "/performance"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Cannot combine portfolios") {
		t.Error("expected error message in page")
	}
}

// --- buildBenchmarkURLs tests ---

func TestBuildBenchmarkURLs(t *testing.T) {
	benchNames := map[string]string{"^GSPC": "S&P 500", "^IXIC": "NASDAQ Composite"}

	// No portfolio, no period, no mode.
	urls := buildBenchmarkURLs(benchNames, "", "", "", "")
	if urls["None"] != "/performance" {
		t.Errorf("None = %q, want /performance", urls["None"])
	}
	if urls["S&P 500 (^GSPC)"] != "/performance?benchmark=^GSPC" {
		t.Errorf("S&P 500 = %q, want /performance?benchmark=^GSPC", urls["S&P 500 (^GSPC)"])
	}

	// With portfolio and period — preserved.
	urls2 := buildBenchmarkURLs(benchNames, "^GSPC", "5", "1Y", "")
	if urls2["None"] != "/performance?portfolio_id=5&period=1Y" {
		t.Errorf("None = %q, want /performance?portfolio_id=5&period=1Y", urls2["None"])
	}
	if urls2["S&P 500 (^GSPC)"] != "/performance?portfolio_id=5&period=1Y&benchmark=^GSPC" {
		t.Errorf("S&P 500 = %q, want /performance?portfolio_id=5&period=1Y&benchmark=^GSPC", urls2["S&P 500 (^GSPC)"])
	}
	if urls2["NASDAQ Composite (^IXIC)"] != "/performance?portfolio_id=5&period=1Y&benchmark=^IXIC" {
		t.Errorf("NASDAQ = %q, want /performance?portfolio_id=5&period=1Y&benchmark=^IXIC", urls2["NASDAQ Composite (^IXIC)"])
	}

	// With "All" period (no period param).
	urls3 := buildBenchmarkURLs(benchNames, "", "5", "All", "")
	if urls3["None"] != "/performance?portfolio_id=5" {
		t.Errorf("None = %q, want /performance?portfolio_id=5", urls3["None"])
	}
	if urls3["S&P 500 (^GSPC)"] != "/performance?portfolio_id=5&benchmark=^GSPC" {
		t.Errorf("S&P 500 = %q, want /performance?portfolio_id=5&benchmark=^GSPC", urls3["S&P 500 (^GSPC)"])
	}

	// With mode=nav — preserved in all benchmark URLs.
	urls5 := buildBenchmarkURLs(benchNames, "", "5", "1Y", "nav")
	if urls5["None"] != "/performance?portfolio_id=5&period=1Y&mode=nav" {
		t.Errorf("None = %q, want /performance?portfolio_id=5&period=1Y&mode=nav", urls5["None"])
	}
	if urls5["S&P 500 (^GSPC)"] != "/performance?portfolio_id=5&period=1Y&benchmark=^GSPC&mode=nav" {
		t.Errorf("S&P 500 = %q, want /performance?portfolio_id=5&period=1Y&benchmark=^GSPC&mode=nav", urls5["S&P 500 (^GSPC)"])
	}

	// Mode=equity omitted from URLs.
	urls6 := buildBenchmarkURLs(benchNames, "", "5", "1Y", "equity")
	if urls6["None"] != "/performance?portfolio_id=5&period=1Y" {
		t.Errorf("None = %q, want /performance?portfolio_id=5&period=1Y", urls6["None"])
	}

	// Empty benchmark names — only None option.
	urls7 := buildBenchmarkURLs(map[string]string{}, "", "", "", "")
	if len(urls7) != 1 {
		t.Errorf("expected 1 URL (None), got %d", len(urls7))
	}
	if urls7["None"] != "/performance" {
		t.Errorf("None = %q, want /performance", urls7["None"])
	}
}

// --- buildModeURLs tests ---

func TestBuildModeURLs(t *testing.T) {
	// No portfolio, no period, no benchmark.
	urls := buildModeURLs("", "", "", "equity")
	if urls["equity"] != "/performance" {
		t.Errorf("equity = %q, want /performance", urls["equity"])
	}
	if urls["nav"] != "/performance?mode=nav" {
		t.Errorf("nav = %q, want /performance?mode=nav", urls["nav"])
	}

	// With portfolio, period, benchmark — preserved.
	urls2 := buildModeURLs("5", "1Y", "^GSPC", "nav")
	if urls2["equity"] != "/performance?portfolio_id=5&period=1Y&benchmark=^GSPC" {
		t.Errorf("equity = %q, want /performance?portfolio_id=5&period=1Y&benchmark=^GSPC", urls2["equity"])
	}
	if urls2["nav"] != "/performance?portfolio_id=5&period=1Y&benchmark=^GSPC&mode=nav" {
		t.Errorf("nav = %q, want /performance?portfolio_id=5&period=1Y&benchmark=^GSPC&mode=nav", urls2["nav"])
	}

	// With "All" period (no period param).
	urls3 := buildModeURLs("5", "All", "", "equity")
	if urls3["equity"] != "/performance?portfolio_id=5" {
		t.Errorf("equity = %q, want /performance?portfolio_id=5", urls3["equity"])
	}
	if urls3["nav"] != "/performance?portfolio_id=5&mode=nav" {
		t.Errorf("nav = %q, want /performance?portfolio_id=5&mode=nav", urls3["nav"])
	}
}

// --- computeNavChartData tests ---

func TestComputeNavChartData(t *testing.T) {
	tests := []struct {
		name string
		in   []performance.EquityCurvePoint
		want string
	}{
		{
			name: "empty nil",
			in:   nil,
			want: "[]",
		},
		{
			name: "empty slice",
			in:   []performance.EquityCurvePoint{},
			want: "[]",
		},
		{
			name: "single point — actual NAV",
			in: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(920000, 6))},
			},
			want: `[{"date":"2024-01-15","value":0.92}]`,
		},
		{
			name: "NAV growth over time",
			in: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(900000, 6))},
				{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(11000000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(910000, 6))},
				{Date: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(12000000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(920000, 6))},
			},
			want: `[{"date":"2024-01-01","value":0.9},{"date":"2024-06-01","value":0.91},{"date":"2024-12-01","value":0.92}]`,
		},
		{
			name: "NAV decline",
			in: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(920000, 6))},
				{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(8000000, 2), NavPerUnit: ptrDecimal(decimal.MustNew(880000, 6))},
			},
			want: `[{"date":"2024-01-01","value":0.92},{"date":"2024-06-01","value":0.88}]`,
		},
		{
			name: "nil NavPerUnit produces zero",
			in: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2)},
			},
			want: `[{"date":"2024-01-01","value":0}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeNavChartData(tt.in)
			if got != tt.want {
				t.Errorf("computeNavChartData() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- serializeBenchmarkChartData tests ---

func TestSerializeBenchmarkChartData(t *testing.T) {
	tests := []struct {
		name string
		in   []market.HistoricalPrice
		want string
	}{
		{
			name: "empty nil",
			in:   nil,
			want: "[]",
		},
		{
			name: "empty slice",
			in:   []market.HistoricalPrice{},
			want: "[]",
		},
		{
			name: "single price",
			in: []market.HistoricalPrice{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(470000, 2), Currency: "USD"},
			},
			want: `[{"date":"2024-01-15","price":"4700.00"}]`,
		},
		{
			name: "multiple prices",
			in: []market.HistoricalPrice{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(470000, 2), Currency: "USD"},
				{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(520000, 2), Currency: "USD"},
			},
			want: `[{"date":"2024-01-01","price":"4700.00"},{"date":"2024-06-01","price":"5200.00"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serializeBenchmarkChartData(tt.in)
			if got != tt.want {
				t.Errorf("serializeBenchmarkChartData() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Handler integration tests for benchmark ---

// mockBenchmarkLister implements benchmarkSymbolLister for tests.
type mockBenchmarkLister struct {
	mappings []symbolmapping.SymbolMapping
}

func (m *mockBenchmarkLister) ListBenchmarks(_ context.Context) ([]symbolmapping.SymbolMapping, error) {
	result := make([]symbolmapping.SymbolMapping, len(m.mappings))
	copy(result, m.mappings)
	return result, nil
}

// --- extractStaleSymbols tests ---

func TestExtractStaleSymbols(t *testing.T) {
	tests := []struct {
		name     string
		warnings []string
		want     []string
	}{
		{
			name:     "empty warnings",
			warnings: []string{},
			want:     nil,
		},
		{
			name:     "nil warnings",
			warnings: nil,
			want:     nil,
		},
		{
			name: "stale warning",
			warnings: []string{
				"stale market data for AAPL (last updated 5 days ago)",
			},
			want: []string{"AAPL"},
		},
		{
			name: "missing warning",
			warnings: []string{
				"missing market data for GOOGL",
			},
			want: []string{"GOOGL"},
		},
		{
			name: "mixed warnings",
			warnings: []string{
				"stale market data for AAPL (last updated 5 days ago)",
				"missing market data for GOOGL",
				"stale market data for MSFT (last updated 3 days ago)",
			},
			want: []string{"AAPL", "GOOGL", "MSFT"},
		},
		{
			name: "unrelated warnings ignored",
			warnings: []string{
				"some other warning",
				"stale market data for AAPL (last updated 5 days ago)",
			},
			want: []string{"AAPL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStaleSymbols(tt.warnings)
			if len(got) != len(tt.want) {
				t.Errorf("extractStaleSymbols() got %d symbols, want %d: %v", len(got), len(tt.want), got)
				return
			}
			for i, sym := range got {
				if sym != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, sym, tt.want[i])
				}
			}
		})
	}
}

// --- formatLastRefresh tests ---

func TestFormatLastRefresh(t *testing.T) {
	tests := []struct {
		name  string
		delta string // e.g. "5s", "2m", "3h", "2d"
		want  string
	}{
		{"just now", "10s", "Just now"},
		{"seconds", "45s", "Updated 45s ago"},
		{"minutes", "5m", "Updated 5m ago"},
		{"hours", "3h", "Updated 3h ago"},
		{"one day", "24h", "Updated 1d ago"},
		{"multiple days", "72h", "Updated 3d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := time.ParseDuration(tt.delta)
			if err != nil {
				t.Fatalf("invalid duration %q: %v", tt.delta, err)
			}
			got := formatLastRefresh(time.Now().Add(-d))
			if got != tt.want {
				t.Errorf("formatLastRefresh() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Template tests with cache status ---

func TestPerformanceTemplate_CacheStatusCurrent(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	data := performancePageData{
		PageData:        web.PageData{Title: "Performance"},
		Result:          result,
		ChartData:       serializeChartData(curve),
		CurrentValue:    "100000.00",
		Portfolios:      []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod:  "1Y",
		HasCacheStatus:  true,
		LastRefreshText: "Just now",
		StaleSymbols:    []string{},
		RefreshURL:      "/performance/refresh",
		PeriodURLs:      map[string]string{"All": "/performance"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "cache-status-current") {
		t.Error("expected cache-status-current class")
	}
	if !strings.Contains(body, "Just now") {
		t.Error("expected 'Just now' in page")
	}
}

func TestPerformanceTemplate_CacheStatusStale(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
		Warnings:     []string{"stale market data for AAPL (last updated 5 days ago)"},
	}

	data := performancePageData{
		PageData:        web.PageData{Title: "Performance"},
		Result:          result,
		ChartData:       serializeChartData(curve),
		CurrentValue:    "100000.00",
		Portfolios:      []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod:  "1Y",
		HasCacheStatus:  true,
		LastRefreshText: "Updated 5d ago",
		StaleSymbols:    []string{"AAPL"},
		RefreshURL:      "/performance/refresh",
		PeriodURLs:      map[string]string{"All": "/performance"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "cache-status-stale") {
		t.Error("expected cache-status-stale class")
	}
	if !strings.Contains(body, "1 symbol(s) stale") {
		t.Error("expected stale symbol count in page")
	}
}

func TestPerformanceTemplate_CacheStatusRefreshing(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	data := performancePageData{
		PageData:       web.PageData{Title: "Performance"},
		Result:         result,
		ChartData:      serializeChartData(curve),
		CurrentValue:   "100000.00",
		Portfolios:     []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod: "1Y",
		HasCacheStatus: true,
		CacheStatus:    marketcache.CacheStatus{Refreshing: true},
		RefreshURL:     "/performance/refresh",
		PeriodURLs:     map[string]string{"All": "/performance"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "cache-status-refreshing") {
		t.Error("expected cache-status-refreshing class")
	}
	if !strings.Contains(body, "Refreshing...") {
		t.Error("expected 'Refreshing...' in page")
	}
}

func TestPerformanceTemplate_NoCacheStatus(t *testing.T) {
	renderer := newTestRenderer(t)

	data := performancePageData{
		PageData:       web.PageData{Title: "Performance"},
		Portfolios:     []portfolio.Portfolio{},
		HasCacheStatus: false,
		PeriodURLs:     map[string]string{"All": "/performance"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if strings.Contains(body, "cache-status") {
		t.Error("expected no cache status indicator when HasCacheStatus is false")
	}
}

// --- Benchmark template tests ---

func TestPerformanceTemplate_WithBenchmark(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
		{Date: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(12000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}
	benchMWR := decimal.MustNew(1500, 2)

	benchPrices := []market.HistoricalPrice{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(470000, 2), Currency: "USD"},
		{Date: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(540000, 2), Currency: "USD"},
	}

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance"},
		Result:              result,
		ChartData:           serializeChartData(curve),
		CurrentValue:        "120000.00",
		CurrentNetDeposit:   "100000.00",
		Portfolios:          []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod:      "1Y",
		SelectedPortfolioID: "1",
		RefreshURL:          "/performance/refresh?portfolio_id=1&benchmark=^GSPC",
		PeriodURLs: map[string]string{
			"1Y":  "/performance?portfolio_id=1&period=1Y&benchmark=^GSPC",
			"All": "/performance?portfolio_id=1&benchmark=^GSPC",
		},
		// Benchmark fields.
		SelectedBenchmark:  "^GSPC",
		BenchmarkNames:     map[string]string{"^GSPC": "S&P 500", "^IXIC": "NASDAQ Composite"},
		BenchmarkTicker:    "^GSPC",
		BenchmarkChartData: serializeBenchmarkChartData(benchPrices),
		BenchmarkMWRPct:    &benchMWR,
		BenchmarkCurrency:  "USD",
		BenchmarkWarning:   "",
		BenchmarkURLs: map[string]string{
			"None":                     "/performance?portfolio_id=1&period=1Y",
			"S&P 500 (^GSPC)":          "/performance?portfolio_id=1&period=1Y&benchmark=^GSPC",
			"NASDAQ Composite (^IXIC)": "/performance?portfolio_id=1&period=1Y&benchmark=^IXIC",
		},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}
	checkNotContains := func(label, text string) {
		t.Helper()
		if strings.Contains(body, text) {
			t.Errorf("page should not contain %s: %q", label, text)
		}
	}

	// Benchmark selector present.
	checkContains("benchmark selector", `id="benchmark"`)

	// Benchmark MWR card visible.
	checkContains("benchmark MWR label", "Benchmark MWR")
	checkContains("benchmark MWR value", "15.00%")
	checkContains("benchmark MWR currency", "USD")

	// Benchmark chart data present in script.
	checkContains("benchmark chart data", `benchRaw`)

	// Benchmark chart indicators.
	checkContains("hasBenchmark check", "hasBenchmark")
	checkContains("benchmark line style", `type: 'dashed'`)
	checkContains("percentage tooltip", "pctStr")
	checkContains("return label", "'Return'")

	// No warning shown.
	checkNotContains("benchmark warning", "benchmark-warning")
}

func TestPerformanceTemplate_WithoutBenchmark(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance"},
		Result:              result,
		ChartData:           serializeChartData(curve),
		CurrentValue:        "100000.00",
		Portfolios:          []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod:      "All",
		SelectedPortfolioID: "1",
		SelectedBenchmark:   "", // No benchmark selected.
		BenchmarkNames:      map[string]string{"^GSPC": "S&P 500"},
		BenchmarkURLs: map[string]string{
			"None":            "/performance?portfolio_id=1",
			"S&P 500 (^GSPC)": "/performance?portfolio_id=1&benchmark=^GSPC",
		},
		PeriodURLs: map[string]string{"All": "/performance?portfolio_id=1"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Benchmark selector present but "None" selected (empty SelectedBenchmark).
	if !strings.Contains(body, `id="benchmark"`) {
		t.Error("expected benchmark selector")
	}

	// Benchmark MWR card hidden (no BenchmarkTicker).
	if strings.Contains(body, "Benchmark MWR") {
		t.Error("benchmark MWR card should be hidden when no benchmark")
	}

	// Benchmark chart data block should be empty (no BenchmarkChartData).
	if strings.Contains(body, `benchRaw = JSON.parse`) {
		t.Error("benchmark chart data parse should not appear when no benchmark")
	}
}

func TestPerformanceTemplate_WithBenchmarkWarning(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	data := performancePageData{
		PageData:          web.PageData{Title: "Performance"},
		Result:            result,
		ChartData:         serializeChartData(curve),
		CurrentValue:      "100000.00",
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedBenchmark: "^GSPC",
		BenchmarkTicker:   "^GSPC",
		BenchmarkNames:    map[string]string{"^GSPC": "S&P 500"},
		BenchmarkWarning:  "no cached data available for benchmark",
		BenchmarkURLs: map[string]string{
			"None":            "/performance",
			"S&P 500 (^GSPC)": "/performance?benchmark=^GSPC",
		},
		PeriodURLs: map[string]string{"All": "/performance?benchmark=^GSPC"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	if !strings.Contains(body, "benchmark-warning") {
		t.Error("expected benchmark-warning class")
	}
	if !strings.Contains(body, "no cached data available for benchmark") {
		t.Error("expected benchmark warning text")
	}
}

func TestPerformanceTemplate_BenchmarkSelectorURLs(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	data := performancePageData{
		PageData:          web.PageData{Title: "Performance"},
		Result:            result,
		ChartData:         serializeChartData(curve),
		CurrentValue:      "100000.00",
		Portfolios:        []portfolio.Portfolio{},
		SelectedBenchmark: "^IXIC",
		BenchmarkNames:    map[string]string{"^GSPC": "S&P 500", "^IXIC": "NASDAQ Composite"},
		BenchmarkURLs: map[string]string{
			"None":                     "/performance?period=1Y",
			"S&P 500 (^GSPC)":          "/performance?period=1Y&benchmark=^GSPC",
			"NASDAQ Composite (^IXIC)": "/performance?period=1Y&benchmark=^IXIC",
		},
		PeriodURLs: map[string]string{"1Y": "/performance?period=1Y&benchmark=^IXIC"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Text input with selected benchmark as value.
	if !strings.Contains(body, `value="^IXIC"`) {
		t.Error("expected selected benchmark value ^IXIC on input")
	}

	// Datalist with benchmark options.
	if !strings.Contains(body, `id="benchmark-list"`) {
		t.Error("expected datalist with id=benchmark-list")
	}

	// Datalist options contain ticker values.
	if !strings.Contains(body, `<option value="^GSPC">`) {
		t.Error("expected S&P 500 option in datalist")
	}
	if !strings.Contains(body, `<option value="^IXIC">`) {
		t.Error("expected NASDAQ option in datalist")
	}

	// JS handler for benchmark change.
	if !strings.Contains(body, `handleBenchmarkChange`) {
		t.Error("expected handleBenchmarkChange JS function")
	}
}

// --- computeMonthlyReturnsFromCurve tests ---

func TestComputeMonthlyReturnsFromCurve(t *testing.T) {
	tests := []struct {
		name          string
		curve         []performance.EquityCurvePoint
		benchPrices   []market.HistoricalPrice
		benchmark     string
		wantLen       int
		wantFirstYear int
		wantFirstMon  int
		wantFirstRet  string // portfolio return of first row
		wantFirstDiff string // diff of first row (empty if no benchmark)
	}{
		{
			name:          "empty curve",
			curve:         nil,
			benchPrices:   nil,
			benchmark:     "",
			wantLen:       0,
			wantFirstRet:  "",
			wantFirstDiff: "",
		},
		{
			name: "single point — no monthly return",
			curve: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
			},
			wantLen:       1,
			wantFirstYear: 2024,
			wantFirstMon:  1,
			wantFirstRet:  "", // single point → no return computable (need ≥ 2 points for TWR)
			wantFirstDiff: "",
		},
		{
			name: "two months positive returns",
			curve: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10200000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
			},
			wantLen:       1, // 1 year row (2024), months are in the Months map
			wantFirstYear: 2024,
			wantFirstMon:  1,
			wantFirstRet:  "5.00", // (10500000/10000000 - 1) * 100 = 5%
			wantFirstDiff: "",
		},
		{
			name: "with benchmark — diff computed",
			curve: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
			},
			benchPrices: []market.HistoricalPrice{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(470000, 2), Currency: "USD"},
				{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(480000, 2), Currency: "USD"},
			},
			benchmark:     "^GSPC",
			wantLen:       1,
			wantFirstYear: 2024,
			wantFirstMon:  1,
			wantFirstRet:  "5.00",
			wantFirstDiff: "2.87", // bench: (480000/470000-1)*100 = 2.13%, diff = 5.00 - 2.13 = 2.87
		},
		{
			name: "negative return",
			curve: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(9500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
			},
			wantLen:       1,
			wantFirstYear: 2024,
			wantFirstMon:  3,
			wantFirstRet:  "-5.00",
			wantFirstDiff: "",
		},
		{
			name: "benchmark only month (no portfolio data)",
			curve: []performance.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
			},
			benchPrices: []market.HistoricalPrice{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(470000, 2), Currency: "USD"},
				{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(480000, 2), Currency: "USD"},
				{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(480000, 2), Currency: "USD"},
				{Date: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(490000, 2), Currency: "USD"},
			},
			benchmark:     "^GSPC",
			wantLen:       1, // 1 year row (2024), Jan + Feb in Months map
			wantFirstYear: 2024,
			wantFirstMon:  1,
			wantFirstRet:  "0",     // no change, no cash flow → 0%
			wantFirstDiff: "-2.13", // bench 2.13%, portfolio 0, diff = -2.13
		},
		{
			name: "multi-year — groups by year not month",
			curve: []performance.EquityCurvePoint{
				// 2024: Jan, Feb, Mar
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10200000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10200000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(11000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				// 2025: Jan, Feb
				{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(11000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(11500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(11500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				{Date: time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(11200000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
			},
			wantLen:       2, // 2 year rows (2024, 2025)
			wantFirstYear: 2024,
			wantFirstMon:  1,
			wantFirstRet:  "5.00",
			wantFirstDiff: "",
		},
		{
			name: "mid-month deposit — TWR isolates cash flow",
			curve: []performance.EquityCurvePoint{
				// Jan 1: start with 100k, ND=100k
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
				// Jan 15: deposited 100k more, PV jumped to 200k (no market gain yet), ND=200k
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(20000000, 2), NetDeposit: decimal.MustNew(20000000, 2)},
				// Jan 31: both 100k positions gained 5% each → PV=210k, ND=200k
				{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(21000000, 2), NetDeposit: decimal.MustNew(20000000, 2)},
			},
			wantLen:       1,
			wantFirstYear: 2024,
			wantFirstMon:  1,
			wantFirstRet:  "5.00", // TWR: sub-period1 (100→100)=0%, sub-period2 (100→105)=5% → linked=5%
			// Simple return would be (210-100)/100 = 110% — wrong!
			wantFirstDiff: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeMonthlyReturnsFromCurve(tt.curve, tt.benchPrices, tt.benchmark)
			if len(got) != tt.wantLen {
				t.Errorf("got %d rows, want %d", len(got), tt.wantLen)
				return
			}
			if tt.wantLen == 0 {
				return
			}
			first := got[0]
			if first.Year != tt.wantFirstYear {
				t.Errorf("first row year = %d, want %d", first.Year, tt.wantFirstYear)
			}
			cell, ok := first.Months[tt.wantFirstMon]
			if !ok {
				t.Errorf("month %d not found in first row", tt.wantFirstMon)
				return
			}
			if tt.wantFirstRet == "" {
				if cell.ReturnPct != nil {
					t.Errorf("first row return_pct = %q, want nil", cell.ReturnPct.String())
				}
			} else {
				if cell.ReturnPct == nil {
					t.Errorf("first row return_pct is nil, want %q", tt.wantFirstRet)
				} else if cell.ReturnPct.String() != tt.wantFirstRet {
					t.Errorf("first row return_pct = %q, want %q", cell.ReturnPct.String(), tt.wantFirstRet)
				}
			}
			if tt.wantFirstDiff == "" {
				if cell.DiffPct != nil {
					t.Errorf("first row diff_pct = %q, want nil", cell.DiffPct.String())
				}
			} else {
				if cell.DiffPct == nil {
					t.Errorf("first row diff_pct is nil, want %q", tt.wantFirstDiff)
				} else if cell.DiffPct.String() != tt.wantFirstDiff {
					t.Errorf("first row diff_pct = %q, want %q", cell.DiffPct.String(), tt.wantFirstDiff)
				}
			}
		})
	}
}

// --- Heatmap template tests ---

func TestPerformanceTemplate_HeatmapWithoutBenchmark(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
		{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
		{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
		{Date: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10200000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	monthlyReturns := computeMonthlyReturnsFromCurve(curve, nil, "")

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance"},
		Result:              result,
		ChartData:           serializeChartData(curve),
		CurrentValue:        "102000.00",
		Portfolios:          []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod:      "All",
		SelectedPortfolioID: "1",
		SelectedBenchmark:   "",
		BenchmarkNames:      map[string]string{"^GSPC": "S&P 500"},
		BenchmarkURLs:       map[string]string{"None": "/performance?portfolio_id=1"},
		PeriodURLs:          map[string]string{"All": "/performance?portfolio_id=1"},
		MonthlyReturns:      monthlyReturns,
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Heatmap container present.
	if !strings.Contains(body, "heatmap-container") {
		t.Error("expected heatmap-container")
	}
	// Heatmap table present.
	if !strings.Contains(body, "heatmap-table") {
		t.Error("expected heatmap-table")
	}
	// Month headers present.
	if !strings.Contains(body, "<th>Jan</th>") {
		t.Error("expected Jan header")
	}
	if !strings.Contains(body, "<th>Dec</th>") {
		t.Error("expected Dec header")
	}
	// Year row header.
	if !strings.Contains(body, "<td>2024</td>") {
		t.Error("expected 2024 year cell")
	}
	// Positive return cell with absolute coloring.
	if !strings.Contains(body, "heat-positive") {
		t.Error("expected heat-positive class for positive return")
	}
	// Absolute legend.
	if !strings.Contains(body, "Positive") {
		t.Error("expected 'Positive' in legend")
	}
	if !strings.Contains(body, "Negative") {
		t.Error("expected 'Negative' in legend")
	}
	// Should NOT show relative legend items.
	if strings.Contains(body, "Outperformed") {
		t.Error("should not show 'Outperformed' in legend without benchmark")
	}
}

func TestPerformanceTemplate_HeatmapWithBenchmark(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
		{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10500000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	benchPrices := []market.HistoricalPrice{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(470000, 2), Currency: "USD"},
		{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(480000, 2), Currency: "USD"},
	}

	monthlyReturns := computeMonthlyReturnsFromCurve(curve, benchPrices, "^GSPC")

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance"},
		Result:              result,
		ChartData:           serializeChartData(curve),
		CurrentValue:        "105000.00",
		Portfolios:          []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod:      "All",
		SelectedPortfolioID: "1",
		SelectedBenchmark:   "^GSPC",
		BenchmarkNames:      map[string]string{"^GSPC": "S&P 500"},
		BenchmarkURLs:       map[string]string{"None": "/performance?portfolio_id=1"},
		PeriodURLs:          map[string]string{"All": "/performance?portfolio_id=1"},
		MonthlyReturns:      monthlyReturns,
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Heatmap title includes benchmark name (& is HTML-escaped to &amp;).
	if !strings.Contains(body, "vs S&amp;P 500") {
		t.Error("expected 'vs S&amp;P 500' in heatmap title")
	}
	// Relative legend.
	if !strings.Contains(body, "Outperformed") {
		t.Error("expected 'Outperformed' in legend")
	}
	if !strings.Contains(body, "Underperformed") {
		t.Error("expected 'Underperformed' in legend")
	}
	// Diff value shown in cells with alpha indicator.
	if !strings.Contains(body, "heat-alpha") {
		t.Error("expected heat-alpha class in benchmark comparison cells")
	}
	if !strings.Contains(body, "heat-main") {
		t.Error("expected heat-main class in benchmark comparison cells")
	}
	// Relative coloring class.
	if !strings.Contains(body, "heat-outperform") && !strings.Contains(body, "heat-underperform") && !strings.Contains(body, "heat-even") {
		t.Error("expected relative coloring class in heatmap cells")
	}
}

func TestPerformanceTemplate_HeatmapEmptyData(t *testing.T) {
	renderer := newTestRenderer(t)

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance"},
		Portfolios:          []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPeriod:      "All",
		SelectedPortfolioID: "1",
		BenchmarkNames:      map[string]string{"^GSPC": "S&P 500"},
		BenchmarkURLs:       map[string]string{"None": "/performance?portfolio_id=1"},
		PeriodURLs:          map[string]string{"All": "/performance?portfolio_id=1"},
		MonthlyReturns:      nil, // No data.
		Result:              nil, // No result.
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Empty state — no heatmap table, no heatmap container with data.
	if strings.Contains(body, "heatmap-table") {
		t.Error("should not render heatmap table with no data")
	}
	// Since Result is nil and Error is empty, shows "No performance data available".
	if !strings.Contains(body, "No performance data available") {
		t.Error("expected empty state message")
	}
}

// --- clipToPortfolioRange tests ---

func TestClipToPortfolioRange_ClipsToPortfolioRange(t *testing.T) {
	// Verify that benchmark prices are clipped to the portfolio date range
	// so the chart aligns with the portfolio for easy comparison.

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
		{Date: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)},
	}

	var prices []market.HistoricalPrice
	for y := 2000; y <= 2026; y++ {
		for m := 1; m <= 12; m++ {
			d := time.Date(y, time.Month(m), 15, 0, 0, 0, 0, time.UTC)
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			prices = append(prices, market.HistoricalPrice{
				Date: d, Close: decimal.MustParse("1000.00"), Currency: "USD",
			})
		}
	}

	clipped := clipToPortfolioRange(prices, curve)

	if len(clipped) == 0 {
		t.Fatal("clipped should not be empty")
	}

	if clipped[0].Date.Before(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("first point %s is before portfolio start", clipped[0].Date.Format("2006-01-02"))
	}
	if clipped[len(clipped)-1].Date.After(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("last point %s is after portfolio end", clipped[len(clipped)-1].Date.Format("2006-01-02"))
	}
}

func TestClipToPortfolioRange_EmptyCurve(t *testing.T) {
	var prices []market.HistoricalPrice
	for d := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC); d.Before(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		prices = append(prices, market.HistoricalPrice{
			Date: d, Close: decimal.MustParse("1000.00"), Currency: "USD",
		})
	}

	clipped := clipToPortfolioRange(prices, nil)

	// With empty curve, all prices are returned unchanged
	if len(clipped) != len(prices) {
		t.Errorf("expected %d prices, got %d", len(prices), len(clipped))
	}
}

func TestClipToPortfolioRange_1YPeriod(t *testing.T) {
	// Verify clipping works when filter period is wider than portfolio dates.

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
		{Date: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)},
	}

	var prices []market.HistoricalPrice
	for d := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC); !d.After(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		prices = append(prices, market.HistoricalPrice{
			Date: d, Close: decimal.MustParse("5000.00"), Currency: "USD",
		})
	}

	clipped := clipToPortfolioRange(prices, curve)

	if len(clipped) == 0 {
		t.Fatal("clipped should not be empty")
	}

	if clipped[0].Date.Before(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("first point %s is before portfolio start", clipped[0].Date.Format("2006-01-02"))
	}
	if clipped[len(clipped)-1].Date.After(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("last point %s is after portfolio end", clipped[len(clipped)-1].Date.Format("2006-01-02"))
	}
}

func TestClipToPortfolioRange_5YPeriod(t *testing.T) {
	// Verify that chart data is clipped to portfolio range, not filter period.

	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Date: now},
	}

	var prices []market.HistoricalPrice
	for d := time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC); !d.After(now); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		years := d.Sub(time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC)).Hours() / (365.25 * 24)
		closeVal := 1000 * (1 + 0.10*years)
		prices = append(prices, market.HistoricalPrice{
			Date: d, Close: decimal.MustParse(fmt.Sprintf("%.2f", closeVal)), Currency: "USD",
		})
	}

	clipped := clipToPortfolioRange(prices, curve)

	if len(clipped) == 0 {
		t.Fatal("clipped should not be empty")
	}

	if clipped[0].Date.Before(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("first point %s is before portfolio start",
			clipped[0].Date.Format("2006-01-02"))
	}
	if clipped[len(clipped)-1].Date.After(now) {
		t.Errorf("last point %s is after portfolio end",
			clipped[len(clipped)-1].Date.Format("2006-01-02"))
	}
}

// --- Task 7: User-defined benchmark tests ---

func TestLoadBenchmarkNames(t *testing.T) {
	tests := []struct {
		name     string
		lister   *mockBenchmarkLister
		wantLen  int
		wantKey  string
		wantName string
	}{
		{
			name: "multiple benchmarks",
			lister: &mockBenchmarkLister{
				mappings: []symbolmapping.SymbolMapping{
					{InternalSymbol: "S&P 500", MarketDataSymbol: "^GSPC"},
					{InternalSymbol: "NASDAQ", MarketDataSymbol: "^IXIC"},
				},
			},
			wantLen:  2,
			wantKey:  "^GSPC",
			wantName: "S&P 500",
		},
		{
			name:     "no benchmarks",
			lister:   &mockBenchmarkLister{mappings: []symbolmapping.SymbolMapping{}},
			wantLen:  0,
			wantKey:  "",
			wantName: "",
		},
		{
			name:     "nil lister returns empty",
			lister:   nil, // typed nil *mockBenchmarkLister
			wantLen:  0,
			wantKey:  "",
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handler *PerformanceWebHandler
			if tt.lister == nil {
				// Use empty struct — benchmarkLister is truly nil (no type)
				handler = &PerformanceWebHandler{}
			} else {
				handler = &PerformanceWebHandler{
					benchmarkLister: tt.lister,
				}
			}
			names := handler.loadBenchmarkNames(context.Background())
			if len(names) != tt.wantLen {
				t.Errorf("got %d names, want %d", len(names), tt.wantLen)
			}
			if tt.wantKey != "" {
				if name, ok := names[tt.wantKey]; !ok {
					t.Errorf("missing key %q", tt.wantKey)
				} else if name != tt.wantName {
					t.Errorf("name for %q = %q, want %q", tt.wantKey, name, tt.wantName)
				}
			}
		})
	}
}

func TestPerformanceTemplate_NoBenchmarks(t *testing.T) {
	renderer := newTestRenderer(t)

	data := performancePageData{
		PageData:       web.PageData{Title: "Performance"},
		Portfolios:     []portfolio.Portfolio{},
		BenchmarkNames: nil, // No benchmarks configured.
		BenchmarkURLs:  map[string]string{"None": "/performance"},
		PeriodURLs:     map[string]string{"All": "/performance"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Shows "no benchmarks" message with link to create one.
	if !strings.Contains(body, "No benchmarks configured") {
		t.Error("expected 'No benchmarks configured' message")
	}
	if !strings.Contains(body, `/symbol-mappings"`) {
		t.Error("expected link to /symbol-mappings")
	}
	// Should NOT show the datalist or input.
	if strings.Contains(body, `id="benchmark-list"`) {
		t.Error("should not show datalist when no benchmarks")
	}
}

func TestPerformanceTemplate_WithBenchmarkInput(t *testing.T) {
	renderer := newTestRenderer(t)

	curve := []performance.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &performance.PerformanceResult{
		EquityCurve:  curve,
		BaseCurrency: "USD",
	}

	data := performancePageData{
		PageData:          web.PageData{Title: "Performance"},
		Result:            result,
		ChartData:         serializeChartData(curve),
		CurrentValue:      "100000.00",
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedBenchmark: "^GSPC",
		BenchmarkNames:    map[string]string{"^GSPC": "S&P 500", "^IXIC": "NASDAQ Composite"},
		BenchmarkURLs:     map[string]string{"None": "/performance", "S&P 500 (^GSPC)": "/performance?benchmark=^GSPC"},
		PeriodURLs:        map[string]string{"All": "/performance?benchmark=^GSPC"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "performance/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Text input with selected value.
	if !strings.Contains(body, `id="benchmark"`) {
		t.Error("expected benchmark input")
	}
	if !strings.Contains(body, `value="^GSPC"`) {
		t.Error("expected selected benchmark value")
	}
	// Datalist present.
	if !strings.Contains(body, `id="benchmark-list"`) {
		t.Error("expected datalist")
	}
	// JS handler present.
	if !strings.Contains(body, `handleBenchmarkChange`) {
		t.Error("expected handleBenchmarkChange JS function")
	}
}

// ptrDecimal returns a pointer to the given decimal, useful for setting
// optional pointer fields in test fixtures.
func ptrDecimal(d decimal.Decimal) *decimal.Decimal {
	return &d
}
