package handlers

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
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
		in   []position.EquityCurvePoint
		want string
	}{
		{
			name: "empty nil",
			in:   nil,
			want: "[]",
		},
		{
			name: "empty slice",
			in:   []position.EquityCurvePoint{},
			want: "[]",
		},
		{
			name: "single point",
			in: []position.EquityCurvePoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(100000, 2), NetDeposit: decimal.MustNew(100000, 2)},
			},
			want: `[{"date":"2024-01-15","portfolio_value":"1000.00","net_deposit":"1000.00"}]`,
		},
		{
			name: "multiple points",
			in: []position.EquityCurvePoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(100000, 2), NetDeposit: decimal.MustNew(100000, 2)},
				{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(110000, 2), NetDeposit: decimal.MustNew(100000, 2)},
			},
			want: `[{"date":"2024-01-01","portfolio_value":"1000.00","net_deposit":"1000.00"},{"date":"2024-06-01","portfolio_value":"1100.00","net_deposit":"1000.00"}]`,
		},
		{
			name: "negative values",
			in: []position.EquityCurvePoint{
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
		want        string
	}{
		{"no filter", "", "/performance/refresh"},
		{"portfolio only", "3", "/performance/refresh?portfolio_id=3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRefreshURL(tt.portfolioID)
			if got != tt.want {
				t.Errorf("buildRefreshURL(%q) = %q, want %q", tt.portfolioID, got, tt.want)
			}
		})
	}
}

func TestBuildPeriodURLs(t *testing.T) {
	urls := buildPeriodURLs("", "")
	if urls["All"] != "/performance" {
		t.Errorf("All = %q, want /performance", urls["All"])
	}
	if urls["1W"] != "/performance?period=1W" {
		t.Errorf("1W = %q, want /performance?period=1W", urls["1W"])
	}

	urls2 := buildPeriodURLs("5", "1Y")
	if urls2["All"] != "/performance?portfolio_id=5" {
		t.Errorf("All = %q, want /performance?portfolio_id=5", urls2["All"])
	}
	if urls2["1M"] != "/performance?portfolio_id=5&period=1M" {
		t.Errorf("1M = %q, want /performance?portfolio_id=5&period=1M", urls2["1M"])
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

	curve := []position.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
		{Date: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(12000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	twr := decimal.MustNew(2000, 2)
	annTWR := decimal.MustNew(1900, 2)
	mwr := decimal.MustNew(1800, 2)
	hpMwr := decimal.MustNew(1750, 2)
	result := &position.PerformanceResult{
		EquityCurve: curve,
		ReturnMetrics: position.ReturnMetrics{
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
			"1W": "/performance?portfolio_id=1&period=1W",
			"1M": "/performance?portfolio_id=1&period=1M",
			"3M": "/performance?portfolio_id=1&period=3M",
			"1Y": "/performance?portfolio_id=1&period=1Y",
			"3Y": "/performance?portfolio_id=1&period=3Y",
			"5Y": "/performance?portfolio_id=1&period=5Y",
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

	curve := []position.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &position.PerformanceResult{
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
		LastRefreshText: "Just now",
		StaleSymbols:   []string{},
		RefreshURL:     "/performance/refresh",
		PeriodURLs:     map[string]string{"All": "/performance"},
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

	curve := []position.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &position.PerformanceResult{
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

	curve := []position.EquityCurvePoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), PortfolioValue: decimal.MustNew(10000000, 2), NetDeposit: decimal.MustNew(10000000, 2)},
	}
	result := &position.PerformanceResult{
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
