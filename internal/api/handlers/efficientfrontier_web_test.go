package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/efficientfrontier"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// --- Route Registration ---

func TestRegisterRoutes_EfficientFrontierWeb(t *testing.T) {
	r := chi.NewRouter()
	handler := &EfficientFrontierWebHandler{}
	handler.RegisterRoutes(r)
	// Verify it doesn't panic
}

// --- Template Tests ---

// Test that the efficient frontier page template renders without panic (empty state).
func TestFrontierTemplate_EmptyState(t *testing.T) {
	renderer := newTestRenderer(t)

	data := frontierPageData{
		PageData:          web.PageData{Title: "Efficient Frontier"},
		CandidateSymbols:  []string{},
		Portfolios:        []portfolio.Portfolio{},
		ModelPortfolios:   []modelportfolio.ModelPortfolioSummary{},
		PortfoliosJSON:    "[]",
		ModelPortfoliosJSON: "[]",
		PeriodURLs:        map[string]string{"1Y": "/efficient-frontier"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "efficient_frontier/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	if w.Code != 200 {
		t.Fatalf("status code = %d", w.Code)
	}
	body := w.Body.String()
	if len(body) == 0 {
		t.Fatal("empty response body")
	}
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("missing DOCTYPE, body starts with: %q (len=%d)", body[:min(200, len(body))], len(body))
	}
	if !strings.Contains(body, "Efficient Frontier") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "Enter candidate symbols") {
		t.Error("expected empty state message")
	}
}

// Test that the efficient frontier page renders with results.
func TestFrontierTemplate_WithResults(t *testing.T) {
	renderer := newTestRenderer(t)

	result := &efficientfrontier.FrontierResult{
		Symbols:     []string{"SPY", "EFA", "BND"},
		TradingDays: 252,
		FrontierPoints: []efficientfrontier.FrontierPoint{
			{ReturnPct: 5.0, VolatilityPct: 3.0, SharpeRatio: 0.5, Weights: []float64{0.2, 0.3, 0.5}},
			{ReturnPct: 8.0, VolatilityPct: 8.0, SharpeRatio: 0.7, Weights: []float64{0.5, 0.3, 0.2}},
			{ReturnPct: 12.0, VolatilityPct: 15.0, SharpeRatio: 0.6, Weights: []float64{0.8, 0.1, 0.1}},
		},
		MaxSharpe: &efficientfrontier.OptimizedPortfolio{
			Name:        "Max Sharpe",
			ReturnPct:   8.0,
			VolatilityPct: 8.0,
			SharpeRatio: 0.7,
			Weights:     []float64{0.5, 0.3, 0.2},
		},
		MinVariance: &efficientfrontier.OptimizedPortfolio{
			Name:        "Min Variance",
			ReturnPct:   5.0,
			VolatilityPct: 3.0,
			SharpeRatio: 0.5,
			Weights:     []float64{0.2, 0.3, 0.5},
		},
		HighestReturn: &efficientfrontier.OptimizedPortfolio{
			Name:        "Highest Return",
			ReturnPct:   12.0,
			VolatilityPct: 15.0,
			SharpeRatio: 0.6,
			Weights:     []float64{0.8, 0.1, 0.1},
		},
	}

	data := frontierPageData{
		PageData:          web.PageData{Title: "Efficient Frontier"},
		Result:            result,
		FrontierChartData: serializeFrontierChartData(result),
		CandidateSymbols:  []string{"SPY", "EFA", "BND", "VNQ"},
		Portfolios: []portfolio.Portfolio{
			{ID: 1, Name: "Test Portfolio", Currency: "USD"},
		},
		ModelPortfolios: []modelportfolio.ModelPortfolioSummary{
			{ID: 1, Name: "60/40", EntryCount: 2},
		},
		PortfoliosJSON:      "[{\"id\":1,\"name\":\"Test Portfolio\",\"currency\":\"USD\"}]",
		ModelPortfoliosJSON: "[{\"id\":1,\"name\":\"60/40\",\"entry_count\":2}]",
		SelectedSymbols:     []string{"SPY", "EFA", "BND"},
		SelectedPeriod:      "3Y",
		SelectedRiskFreeRate: "4.5",
		PeriodURLs: map[string]string{
			"1Y": "/efficient-frontier?period=1Y",
			"3Y": "/efficient-frontier?period=3Y",
			"5Y": "/efficient-frontier?period=5Y",
		},
		SelectedPortfolio: result.MaxSharpe,
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "efficient_frontier/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(body, "Max Sharpe Ratio") {
		t.Error("missing max Sharpe card")
	}
	if !strings.Contains(body, "Min Variance") {
		t.Error("missing min variance card")
	}
	if !strings.Contains(body, "Highest Return") {
		t.Error("missing highest return card")
	}
	if !strings.Contains(body, "frontier-chart") {
		t.Error("missing chart container")
	}
	if !strings.Contains(body, "Allocation Weights") {
		t.Error("missing allocation weights section")
	}
	if !strings.Contains(body, "Save as Model Portfolio") {
		t.Error("missing save as model portfolio section")
	}
}

// Test that the template renders warnings correctly.
func TestFrontierTemplate_Warnings(t *testing.T) {
	renderer := newTestRenderer(t)

	data := frontierPageData{
		PageData:          web.PageData{Title: "Efficient Frontier"},
		Warnings:          []string{"Symbol X: limited data available"},
		ExcludedSymbols:   []string{"DELETED"},
		CandidateSymbols:  []string{},
		Portfolios:        []portfolio.Portfolio{},
		ModelPortfolios:   []modelportfolio.ModelPortfolioSummary{},
		PortfoliosJSON:    "[]",
		ModelPortfoliosJSON: "[]",
		PeriodURLs:        map[string]string{"1Y": "/efficient-frontier"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "efficient_frontier/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "limited data available") {
		t.Error("missing warning message")
	}
	if !strings.Contains(body, "Excluded symbols") {
		t.Error("missing excluded symbols section")
	}
	if !strings.Contains(body, "DELETED") {
		t.Error("missing excluded symbol name")
	}
}

// --- Serialization Tests ---

func TestSerializeFrontierChartData_Nil(t *testing.T) {
	got := serializeFrontierChartData(nil)
	if got != "{}" {
		t.Errorf("serializeFrontierChartData(nil) = %q, want {}", got)
	}
}

func TestSerializeFrontierChartData_EmptyPoints(t *testing.T) {
	got := serializeFrontierChartData(&efficientfrontier.FrontierResult{})
	if got != "{}" {
		t.Errorf("serializeFrontierChartData(empty) = %q, want {}", got)
	}
}

func TestSerializeFrontierChartData_Full(t *testing.T) {
	result := &efficientfrontier.FrontierResult{
		Symbols: []string{"SPY", "EFA"},
		FrontierPoints: []efficientfrontier.FrontierPoint{
			{ReturnPct: 8.0, VolatilityPct: 10.0, SharpeRatio: 0.5, Weights: []float64{0.6, 0.4}},
		},
		MaxSharpe: &efficientfrontier.OptimizedPortfolio{
			Name: "Max Sharpe", ReturnPct: 8.0, VolatilityPct: 10.0, SharpeRatio: 0.5,
			Weights: []float64{0.6, 0.4},
		},
		MinVariance: &efficientfrontier.OptimizedPortfolio{
			Name: "Min Variance", ReturnPct: 5.0, VolatilityPct: 5.0, SharpeRatio: 0.3,
			Weights: []float64{0.3, 0.7},
		},
	}

	got := serializeFrontierChartData(result)
	var data frontierChartData
	if err := json.Unmarshal([]byte(got), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(data.FrontierPoints) != 1 {
		t.Errorf("expected 1 frontier point, got %d", len(data.FrontierPoints))
	}
	if data.MaxSharpe == nil {
		t.Error("missing max_sharpe")
	}
	if data.MinVariance == nil {
		t.Error("missing min_variance")
	}
	if len(data.Symbols) != 2 || data.Symbols[0] != "SPY" {
		t.Errorf("symbols = %v, want [SPY, EFA]", data.Symbols)
	}
}

// --- Parsing Tests ---

func TestParseFrontierSymbols(t *testing.T) {
	tests := []struct {
		name string
		input string
		want []string
	}{
		{"empty", "", nil},
		{"single", "SPY", []string{"SPY"}},
		{"multiple", "SPY, EFA, BND", []string{"SPY", "EFA", "BND"}},
		{"no spaces", "SPY,EFA,BND", []string{"SPY", "EFA", "BND"}},
		{"extra spaces", "  SPY ,  EFA  ", []string{"SPY", "EFA"}},
		{"trailing comma", "SPY,EFA,", []string{"SPY", "EFA"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFrontierSymbols(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseFrontierSymbols(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseFrontierSymbols(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseRiskFreeRate(t *testing.T) {
	tests := []struct {
		name string
		input string
		want float64
	}{
		{"empty", "", 4.5},
		{"default", "4.5", 4.5},
		{"custom", "3.0", 3.0},
		{"zero", "0", 0},
		{"invalid", "abc", 4.5},
		{"negative", "-1", 4.5},
		{"too high", "60", 4.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRiskFreeRate(tt.input)
			if got != tt.want {
				t.Errorf("parseRiskFreeRate(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

// --- URL Building Tests ---

func TestBuildFrontierPeriodURLs(t *testing.T) {
	// No symbols, default rate.
	urls := buildFrontierPeriodURLs(nil, "1Y", 4.5)
	if urls["1Y"] != "/efficient-frontier" {
		t.Errorf("1Y = %q, want /efficient-frontier", urls["1Y"])
	}
	if urls["3Y"] != "/efficient-frontier?period=3Y" {
		t.Errorf("3Y = %q, want /efficient-frontier?period=3Y", urls["3Y"])
	}

	// With symbols.
	urls2 := buildFrontierPeriodURLs([]string{"SPY", "EFA"}, "3Y", 4.5)
	if urls2["1Y"] != "/efficient-frontier?symbols=SPY,EFA" {
		t.Errorf("1Y = %q, want /efficient-frontier?symbols=SPY,EFA", urls2["1Y"])
	}
	if urls2["5Y"] != "/efficient-frontier?symbols=SPY,EFA&period=5Y" {
		t.Errorf("5Y = %q, want /efficient-frontier?symbols=SPY,EFA&period=5Y", urls2["5Y"])
	}

	// With custom risk-free rate.
	urls3 := buildFrontierPeriodURLs(nil, "1Y", 3.0)
	if urls3["1Y"] != "/efficient-frontier?risk_free_rate=3.0" {
		t.Errorf("1Y = %q, want /efficient-frontier?risk_free_rate=3.0", urls3["1Y"])
	}
}

// --- Handler Tests ---

// mockFrontierService implements efficientFrontierService for web handler tests.
type mockFrontierService struct {
	result *efficientfrontier.ServiceResult
	err    error
	symbols []string
}

func (m *mockFrontierService) ComputeFrontier(_ context.Context, _ efficientfrontier.ComputeFrontierRequest) (*efficientfrontier.ServiceResult, error) {
	return m.result, m.err
}

func (m *mockFrontierService) GetCandidateSymbols(_ context.Context) ([]string, error) {
	return m.symbols, nil
}

func (m *mockFrontierService) GetSymbolsFromPortfolio(_ context.Context, _ int64) ([]string, error) {
	return []string{}, nil
}

func (m *mockFrontierService) GetSymbolsFromModelPortfolio(_ context.Context, _ int64) ([]string, error) {
	return []string{}, nil
}

// Test that the handler renders the page without symbols (empty state).
func TestHandleEfficientFrontier_NoSymbols(t *testing.T) {
	mockSvc := &mockFrontierService{
		symbols: []string{"SPY", "EFA", "BND"},
	}
	handler := NewEfficientFrontierHandler(mockSvc)
	webHandler := NewEfficientFrontierWebHandler(handler, nil, nil, newTestRenderer(t))

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/efficient-frontier", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Efficient Frontier") {
		t.Error("missing title")
	}
}

// Test that the handler renders with computed results.
func TestHandleEfficientFrontier_WithSymbols(t *testing.T) {
	mockSvc := &mockFrontierService{
		result: &efficientfrontier.ServiceResult{
			Result: &efficientfrontier.FrontierResult{
				Symbols: []string{"SPY", "EFA"},
				FrontierPoints: []efficientfrontier.FrontierPoint{
					{ReturnPct: 8.0, VolatilityPct: 10.0, SharpeRatio: 0.5, Weights: []float64{0.6, 0.4}},
				},
				MaxSharpe: &efficientfrontier.OptimizedPortfolio{
					Name: "Max Sharpe", ReturnPct: 8.0, VolatilityPct: 10.0, SharpeRatio: 0.5,
					Weights: []float64{0.6, 0.4},
				},
			},
		},
		symbols: []string{"SPY", "EFA", "BND"},
	}
	handler := NewEfficientFrontierHandler(mockSvc)
	webHandler := NewEfficientFrontierWebHandler(handler, nil, nil, newTestRenderer(t))

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/efficient-frontier?symbols=SPY,EFA&period=3Y", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Max Sharpe") {
		t.Error("missing max Sharpe in output")
	}
}

// Test that the handler shows error state when computation fails.
func TestHandleEfficientFrontier_ComputeError(t *testing.T) {
	mockSvc := &mockFrontierService{
		err: efficientfrontier.ErrInsufficientData,
		symbols: []string{"SPY"},
	}
	handler := NewEfficientFrontierHandler(mockSvc)
	webHandler := NewEfficientFrontierWebHandler(handler, nil, nil, newTestRenderer(t))

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/efficient-frontier?symbols=SPY,EFA", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "error") && !strings.Contains(body, "Error") {
		t.Error("expected error message in output")
	}
}
