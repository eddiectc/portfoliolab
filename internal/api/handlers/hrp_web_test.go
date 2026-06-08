package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// --- Route Registration ---

func TestRegisterRoutes_HrpWeb(t *testing.T) {
	r := chi.NewRouter()
	handler := &HrpWebHandler{}
	handler.RegisterRoutes(r)
	// Verify it doesn't panic
}

// --- Template Tests ---

// Test that the HRP page template renders without panic (empty state).
func TestHrpTemplate_EmptyState(t *testing.T) {
	renderer := newTestRenderer(t)

	data := hrpPageData{
		PageData:            web.PageData{Title: "Hierarchical Risk Parity"},
		CandidateSymbols:    []string{},
		Portfolios:          []portfolio.Portfolio{},
		ModelPortfolios:     []modelportfolio.ModelPortfolioSummary{},
		PortfoliosJSON:      "[]",
		ModelPortfoliosJSON: "[]",
		PeriodURLs:          map[string]string{"3Y": "/hrp"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "hierarchical_risk_parity/index", data); err != nil {
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
	if !strings.Contains(body, "Hierarchical Risk Parity") {
		t.Error("missing title")
	}
}

// Test that the HRP page renders with results.
func TestHrpTemplate_WithResults(t *testing.T) {
	renderer := newTestRenderer(t)

	result := &hierarchicalriskparity.HrpResult{
		Symbols:     []string{"SPY", "EFA", "BND"},
		TradingDays: 756,
		Allocations: []hierarchicalriskparity.HrpAllocation{
			{
				Method: "single",
				Weights: map[string]float64{
					"SPY": 0.4, "EFA": 0.3, "BND": 0.3,
				},
				Dendrogram: &hierarchicalriskparity.DendrogramNode{
					Distance: 1.2,
					Children: []*hierarchicalriskparity.DendrogramNode{
						{Name: "SPY"},
						{Name: "EFA"},
					},
				},
			},
			{
				Method: "complete",
				Weights: map[string]float64{
					"SPY": 0.35, "EFA": 0.35, "BND": 0.3,
				},
			},
			{
				Method: "average",
				Weights: map[string]float64{
					"SPY": 0.38, "EFA": 0.32, "BND": 0.3,
				},
			},
			{
				Method: "ward",
				Weights: map[string]float64{
					"SPY": 0.42, "EFA": 0.28, "BND": 0.3,
				},
			},
		},
	}

	data := hrpPageData{
		PageData:          web.PageData{Title: "Hierarchical Risk Parity"},
		Result:            result,
		HrpChartData:      serializeHrpChartData(result),
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
		PeriodURLs: map[string]string{
			"1Y": "/hrp?period=1Y",
			"3Y": "/hrp",
			"5Y": "/hrp?period=5Y",
		},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "hierarchical_risk_parity/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(body, "single") {
		t.Error("missing single linkage method")
	}
	if !strings.Contains(body, "complete") {
		t.Error("missing complete linkage method")
	}
	if !strings.Contains(body, "average") {
		t.Error("missing average linkage method")
	}
	if !strings.Contains(body, "ward") {
		t.Error("missing ward linkage method")
	}
}

// Test that the template renders warnings correctly.
func TestHrpTemplate_Warnings(t *testing.T) {
	renderer := newTestRenderer(t)

	data := hrpPageData{
		PageData:            web.PageData{Title: "Hierarchical Risk Parity"},
		Warnings:            []string{"Symbol X: limited data available"},
		ExcludedSymbols:     []string{"DELETED"},
		CandidateSymbols:    []string{},
		Portfolios:          []portfolio.Portfolio{},
		ModelPortfolios:     []modelportfolio.ModelPortfolioSummary{},
		PortfoliosJSON:      "[]",
		ModelPortfoliosJSON: "[]",
		PeriodURLs:          map[string]string{"3Y": "/hrp"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "hierarchical_risk_parity/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "limited data available") {
		t.Error("missing warning message")
	}
	if !strings.Contains(body, "Excluded") && !strings.Contains(body, "excluded") {
		t.Error("missing excluded symbols section")
	}
	if !strings.Contains(body, "DELETED") {
		t.Error("missing excluded symbol name")
	}
}

// --- Serialization Tests ---

func TestSerializeHrpChartData_Nil(t *testing.T) {
	got := serializeHrpChartData(nil)
	if got != "{}" {
		t.Errorf("serializeHrpChartData(nil) = %q, want {}", got)
	}
}

func TestSerializeHrpChartData_EmptyAllocations(t *testing.T) {
	got := serializeHrpChartData(&hierarchicalriskparity.HrpResult{})
	if got != "{}" {
		t.Errorf("serializeHrpChartData(empty) = %q, want {}", got)
	}
}

func TestSerializeHrpChartData_Full(t *testing.T) {
	result := &hierarchicalriskparity.HrpResult{
		Symbols: []string{"SPY", "EFA"},
		Allocations: []hierarchicalriskparity.HrpAllocation{
			{
				Method:  "single",
				Weights: map[string]float64{"SPY": 0.6, "EFA": 0.4},
				Dendrogram: &hierarchicalriskparity.DendrogramNode{
					Distance: 1.0,
					Children: []*hierarchicalriskparity.DendrogramNode{
						{Name: "SPY"},
						{Name: "EFA"},
					},
				},
			},
			{
				Method:  "ward",
				Weights: map[string]float64{"SPY": 0.55, "EFA": 0.45},
			},
		},
		TradingDays: 252,
	}

	got := serializeHrpChartData(result)
	var data hrpChartData
	if err := json.Unmarshal([]byte(got), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(data.Allocations) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(data.Allocations))
	}
	// Verify weights are converted to percentages.
	if data.Allocations[0].Weights["SPY"] != 60.0 {
		t.Errorf("expected SPY weight 60.0 (pct), got %f", data.Allocations[0].Weights["SPY"])
	}
	if data.Allocations[0].Method != "single" {
		t.Errorf("expected method single, got %s", data.Allocations[0].Method)
	}
	if data.Allocations[0].Dendrogram == nil {
		t.Error("missing dendrogram")
	}
	if len(data.Symbols) != 2 || data.Symbols[0] != "SPY" {
		t.Errorf("symbols = %v, want [SPY, EFA]", data.Symbols)
	}
	if data.TradingDays != 252 {
		t.Errorf("trading_days = %d, want 252", data.TradingDays)
	}
}

// --- Parsing Tests ---

func TestParseHrpSymbols(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
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
			got := parseHrpSymbols(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseHrpSymbols(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseHrpSymbols(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- URL Building Tests ---

func TestBuildHrpPeriodURLs(t *testing.T) {
	// No symbols, default period, no currency.
	urls := buildHrpPeriodURLs(nil, "3Y", "")
	if urls["3Y"] != "/hrp" {
		t.Errorf("3Y = %q, want /hrp", urls["3Y"])
	}
	if urls["1Y"] != "/hrp?period=1Y" {
		t.Errorf("1Y = %q, want /hrp?period=1Y", urls["1Y"])
	}

	// With symbols.
	urls2 := buildHrpPeriodURLs([]string{"SPY", "EFA"}, "3Y", "")
	if urls2["3Y"] != "/hrp?symbols=SPY,EFA" {
		t.Errorf("3Y = %q, want /hrp?symbols=SPY,EFA", urls2["3Y"])
	}
	if urls2["5Y"] != "/hrp?symbols=SPY,EFA&period=5Y" {
		t.Errorf("5Y = %q, want /hrp?symbols=SPY,EFA&period=5Y", urls2["5Y"])
	}

	// With base currency.
	urls3 := buildHrpPeriodURLs([]string{"SPY"}, "3Y", "EUR")
	if urls3["3Y"] != "/hrp?symbols=SPY&base_currency=EUR" {
		t.Errorf("3Y = %q, want /hrp?symbols=SPY&base_currency=EUR", urls3["3Y"])
	}
	if urls3["1Y"] != "/hrp?symbols=SPY&period=1Y&base_currency=EUR" {
		t.Errorf("1Y = %q, want /hrp?symbols=SPY&period=1Y&base_currency=EUR", urls3["1Y"])
	}
}

// --- Data Span Tests ---

func TestFindHrpLeastDataSymbol(t *testing.T) {
	// Empty map.
	sym, days := findHrpLeastDataSymbol(nil)
	if sym != "" || days != 0 {
		t.Errorf("findHrpLeastDataSymbol(nil) = %q, %d, want \"\", 0", sym, days)
	}

	// Single symbol.
	span := map[string]hierarchicalriskparity.DataSpan{
		"SPY": {TradingDays: 250},
	}
	sym, days = findHrpLeastDataSymbol(span)
	if sym != "SPY" || days != 250 {
		t.Errorf("findHrpLeastDataSymbol(single) = %q, %d, want SPY, 250", sym, days)
	}

	// Multiple symbols — find the one with fewest days.
	span2 := map[string]hierarchicalriskparity.DataSpan{
		"SPY": {TradingDays: 756},
		"EFA": {TradingDays: 500},
		"BND": {TradingDays: 700},
	}
	sym, days = findHrpLeastDataSymbol(span2)
	if sym != "EFA" || days != 500 {
		t.Errorf("findHrpLeastDataSymbol(multi) = %q, %d, want EFA, 500", sym, days)
	}
}

// --- Error Message Tests ---

func TestHrpErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantText string
	}{
		{
			name:     "insufficient symbols",
			err:      hierarchicalriskparity.ErrInsufficientSymbols,
			wantText: "At least 2 symbols",
		},
		{
			name:     "too many symbols",
			err:      hierarchicalriskparity.ErrTooManySymbols,
			wantText: "Maximum 20 symbols",
		},
		{
			name:     "insufficient data",
			err:      hierarchicalriskparity.ErrInsufficientData,
			wantText: "Insufficient price data",
		},
		{
			name:     "numerical failure",
			err:      hierarchicalriskparity.ErrNumericalFailure,
			wantText: "numerical error",
		},
		{
			name:     "unknown error",
			err:      nil,
			wantText: "unexpected error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hrpErrorMessage(tc.err)
			if !strings.Contains(got, tc.wantText) {
				t.Errorf("hrpErrorMessage(%v) = %q, want containing %q", tc.err, got, tc.wantText)
			}
		})
	}
}

// --- Handler Tests ---

// mockHrpService implements hrpService for web handler tests.
type mockHrpService struct {
	result  *hierarchicalriskparity.ServiceResult
	err     error
	symbols []string
}

func (m *mockHrpService) ComputeHrp(_ context.Context, _ hierarchicalriskparity.ComputeHrpRequest) (*hierarchicalriskparity.ServiceResult, error) {
	return m.result, m.err
}

func (m *mockHrpService) GetCandidateSymbols(_ context.Context) ([]string, error) {
	return m.symbols, nil
}

func (m *mockHrpService) GetSymbolsFromPortfolio(_ context.Context, _ int64) ([]string, error) {
	return []string{}, nil
}

func (m *mockHrpService) GetSymbolsFromModelPortfolio(_ context.Context, _ int64) ([]string, error) {
	return []string{}, nil
}

// Test that the handler renders the page without symbols (empty state).
func TestHandleHrp_NoSymbols(t *testing.T) {
	mockSvc := &mockHrpService{
		symbols: []string{"SPY", "EFA", "BND"},
	}
	handler := NewHrpHandler(mockSvc)
	webHandler := NewHrpWebHandler(handler, nil, nil, newTestRenderer(t))

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/hrp", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Hierarchical Risk Parity") {
		t.Error("missing title")
	}
}

// Test that the handler renders with computed results.
func TestHandleHrp_WithSymbols(t *testing.T) {
	mockSvc := &mockHrpService{
		result: &hierarchicalriskparity.ServiceResult{
			Result: &hierarchicalriskparity.HrpResult{
				Symbols: []string{"SPY", "EFA"},
				Allocations: []hierarchicalriskparity.HrpAllocation{
					{
						Method:  "single",
						Weights: map[string]float64{"SPY": 0.6, "EFA": 0.4},
					},
					{
						Method:  "ward",
						Weights: map[string]float64{"SPY": 0.55, "EFA": 0.45},
					},
				},
			},
		},
		symbols: []string{"SPY", "EFA", "BND"},
	}
	handler := NewHrpHandler(mockSvc)
	webHandler := NewHrpWebHandler(handler, nil, nil, newTestRenderer(t))

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/hrp?symbols=SPY,EFA&period=3Y", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "single") {
		t.Error("missing single linkage method in output")
	}
}

// Test that the handler shows error state when computation fails.
func TestHandleHrp_ComputeError(t *testing.T) {
	mockSvc := &mockHrpService{
		err:     hierarchicalriskparity.ErrInsufficientData,
		symbols: []string{"SPY"},
	}
	handler := NewHrpHandler(mockSvc)
	webHandler := NewHrpWebHandler(handler, nil, nil, newTestRenderer(t))

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/hrp?symbols=SPY,EFA", nil)
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

// Test that the handler shows specific error messages for different error types.
func TestHandleHrp_ComputeErrorSpecific(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantText string
	}{
		{
			name:     "insufficient data",
			err:      hierarchicalriskparity.ErrInsufficientData,
			wantText: "Insufficient price data",
		},
		{
			name:     "numerical failure",
			err:      hierarchicalriskparity.ErrNumericalFailure,
			wantText: "numerical error",
		},
		{
			name:     "too many symbols",
			err:      hierarchicalriskparity.ErrTooManySymbols,
			wantText: "Maximum 20 symbols",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockHrpService{
				err:     tc.err,
				symbols: []string{"SPY", "EFA"},
			}
			handler := NewHrpHandler(mockSvc)
			webHandler := NewHrpWebHandler(handler, nil, nil, newTestRenderer(t))

			r := chi.NewRouter()
			webHandler.RegisterRoutes(r)

			req := httptest.NewRequest("GET", "/hrp?symbols=SPY,EFA", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", w.Code)
			}

			body := w.Body.String()
			if !strings.Contains(body, tc.wantText) {
				t.Errorf("expected error message containing %q, got: %s", tc.wantText, body[:min(500, len(body))])
			}
		})
	}
}

// --- Save Handler Tests ---

// Test that the save handler creates a model portfolio and redirects.
func TestHandleSaveAsModelPortfolio_Success(t *testing.T) {
	mockSvc := &mockHrpService{symbols: []string{"SPY", "EFA"}}
	handler := NewHrpHandler(mockSvc)
	webHandler := NewHrpWebHandler(handler, nil, nil, newTestRenderer(t))
	webHandler.WithModelPortfolioCreator(&mockModelPortfolioCreator{
		createFn: func(_ context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
			return modelportfolio.ModelPortfolio{
				ID:    1,
				Name:  req.Name,
				Entries: req.Entries,
			}, nil
		},
	})

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	body := strings.NewReader("name=HRP Test&symbol=SPY&weight=0.6&symbol=EFA&weight=0.4")
	req := httptest.NewRequest("POST", "/hrp/save", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/model-portfolios" {
		t.Errorf("Location = %q, want /model-portfolios", location)
	}
}

// Test that the save handler rejects empty allocation data.
func TestHandleSaveAsModelPortfolio_NoData(t *testing.T) {
	mockSvc := &mockHrpService{symbols: []string{"SPY"}}
	handler := NewHrpHandler(mockSvc)
	webHandler := NewHrpWebHandler(handler, nil, nil, newTestRenderer(t))

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	body := strings.NewReader("name=HRP Test")
	req := httptest.NewRequest("POST", "/hrp/save", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/hrp" {
		t.Errorf("Location = %q, want /hrp", location)
	}
}

// Test that the save handler uses default name when name is empty.
func TestHandleSaveAsModelPortfolio_DefaultName(t *testing.T) {
	var receivedName string
	mockSvc := &mockHrpService{symbols: []string{"SPY"}}
	handler := NewHrpHandler(mockSvc)
	webHandler := NewHrpWebHandler(handler, nil, nil, newTestRenderer(t))
	webHandler.WithModelPortfolioCreator(&mockModelPortfolioCreator{
		createFn: func(_ context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
			receivedName = req.Name
			return modelportfolio.ModelPortfolio{ID: 1, Name: req.Name}, nil
		},
	})

	r := chi.NewRouter()
	webHandler.RegisterRoutes(r)

	body := strings.NewReader("name=&symbol=SPY&weight=0.5")
	req := httptest.NewRequest("POST", "/hrp/save", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if receivedName != "HRP Portfolio" {
		t.Errorf("received name = %q, want HRP Portfolio", receivedName)
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", w.Code)
	}
}

// mockModelPortfolioCreator implements modelPortfolioCreator for tests.
type mockModelPortfolioCreator struct {
	createFn func(context.Context, modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error)
}

func (m *mockModelPortfolioCreator) Create(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
	return m.createFn(ctx, req)
}
