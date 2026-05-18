package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/analysis"
)

// --- Mock analysis service ---

type mockAnalysisService struct {
	result  *analysis.AnalysisResult
	err     error
	called  bool
	filters analysis.AnalysisFilters
}

func (m *mockAnalysisService) ComputeAnalysis(_ context.Context, filters analysis.AnalysisFilters) (*analysis.AnalysisResult, error) {
	m.called = true
	m.filters = filters
	return m.result, m.err
}

// --- Test helpers ---

func makeTestResult() *analysis.AnalysisResult {
	return &analysis.AnalysisResult{
		PortfolioID: 1,
		ComputedAt:  time.Now().UTC(),
		Overlap: &analysis.OverlapResult{
			PairwiseMatrix: []analysis.OverlapPair{
				{ETFA: "VTI", ETFB: "VXUS", OverlappingCount: 5, CombinedWeightPct: 3.2},
			},
			TopConcentratedStocks: []analysis.ConcentratedStock{
				{Symbol: "AAPL", Name: "Apple Inc.", TotalWeightPct: 4.5, HeldByETFs: []string{"VTI", "VXUS"}},
			},
		},
		Correlation: &analysis.CorrelationResult{
			Matrix:  ptrMatrix([][]float64{{1.0, 0.85}, {0.85, 1.0}}),
			Symbols: []string{"VTI", "VXUS"},
			Period:  "1Y",
		},
		SectorAllocation: &analysis.AllocationResult{
			Breakdown: map[string]float64{
				"Technology":  30.5,
				"Healthcare":  15.2,
				"Unknown":     5.0,
			},
			UnknownWeightPct: 5.0,
		},
		GeographicAllocation: &analysis.AllocationResult{
			Breakdown: map[string]float64{
				"United States": 60.0,
				"Japan":         10.0,
			},
			UnknownWeightPct: 2.0,
		},
		StressTest: &analysis.StressTestResult{
			Scenarios: []analysis.StressScenarioResult{
				{
					Name:               "2008 Global Financial Crisis",
					DateRange:          "2007-10-09 to 2009-03-09",
					EstimatedReturnPct: -42.5,
					EstimatedDollarImpact: decimal.MustParse("-42500.00"),
					SectorContributions: map[string]float64{
						"Financials": -18.0,
					},
				},
			},
		},
		FactorExposure: &analysis.FactorExposureResult{
			ValueGrowthTilt: analysis.FactorValueGrowth{
				WeightedPE: 22.5,
				WeightedPB: 4.2,
				Tilt:       "growth",
			},
			SizeTilt: analysis.FactorSizeTilt{
				LargeCapPct: 75.0,
				MidCapPct:   20.0,
				SmallCapPct: 5.0,
				Tilt:        "large",
			},
			Concentration: analysis.FactorConcentration{
				HHI:            0.015,
				Interpretation: "well-diversified",
			},
			TopHoldingWeightPct: 4.5,
		},
		Warnings: []string{"symbol details not cached for 1 symbol(s): MSFT"},
	}
}

// --- HandleAnalysis Tests ---

func TestAnalysisHandleAnalysis_Success(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if !mockSvc.called {
		t.Fatal("service.ComputeAnalysis was not called")
	}

	var result analysis.AnalysisResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.PortfolioID != 1 {
		t.Errorf("expected portfolio_id 1, got %d", result.PortfolioID)
	}
	if result.Overlap == nil {
		t.Error("expected overlap section")
	} else if len(result.Overlap.PairwiseMatrix) != 1 {
		t.Errorf("expected 1 pairwise pair, got %d", len(result.Overlap.PairwiseMatrix))
	}
	if result.Correlation == nil {
		t.Error("expected correlation section")
	}
	if result.SectorAllocation == nil {
		t.Error("expected sector allocation section")
	}
	if result.GeographicAllocation == nil {
		t.Error("expected geographic allocation section")
	}
	if result.StressTest == nil {
		t.Error("expected stress test section")
	}
	if result.FactorExposure == nil {
		t.Error("expected factor exposure section")
	}
}

func TestAnalysisHandleAnalysis_NoParams(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if mockSvc.filters.PortfolioID != nil {
		t.Errorf("expected nil portfolio_id, got %d", *mockSvc.filters.PortfolioID)
	}
	if mockSvc.filters.Section != "" {
		t.Errorf("expected empty section, got %q", mockSvc.filters.Section)
	}
	if mockSvc.filters.Period != "" {
		t.Errorf("expected empty period, got %q", mockSvc.filters.Period)
	}
}

func TestAnalysisHandleAnalysis_SectionFilter(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?section=overlap", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if mockSvc.filters.Section != "overlap" {
		t.Errorf("expected section 'overlap', got %q", mockSvc.filters.Section)
	}
}

func TestAnalysisHandleAnalysis_PeriodFilter(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?period=3Y", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if mockSvc.filters.Period != "3Y" {
		t.Errorf("expected period '3Y', got %q", mockSvc.filters.Period)
	}
}

func TestAnalysisHandleAnalysis_AllFilters(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?portfolio_id=42&section=correlation&period=5Y", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if mockSvc.filters.PortfolioID == nil || *mockSvc.filters.PortfolioID != 42 {
		t.Errorf("expected portfolio_id 42, got %v", mockSvc.filters.PortfolioID)
	}
	if mockSvc.filters.Section != "correlation" {
		t.Errorf("expected section 'correlation', got %q", mockSvc.filters.Section)
	}
	if mockSvc.filters.Period != "5Y" {
		t.Errorf("expected period '5Y', got %q", mockSvc.filters.Period)
	}
}

func TestAnalysisHandleAnalysis_InternalError(t *testing.T) {
	mockSvc := &mockAnalysisService{err: fmt.Errorf("database connection failed")}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INTERNAL_ERROR" {
		t.Errorf("expected INTERNAL_ERROR, got %q", errResp.Code)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestAnalysisHandleAnalysis_EmptyState(t *testing.T) {
	// Service returns a result with a message (no positions) but no error.
	mockSvc := &mockAnalysisService{
		result: &analysis.AnalysisResult{
			PortfolioID: 1,
			ComputedAt:  time.Now().UTC(),
			Message:     "No open positions found. Analysis requires at least one open position.",
		},
	}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result analysis.AnalysisResult
	json.NewDecoder(w.Body).Decode(&result)
	if result.Message == "" {
		t.Error("expected non-empty message for empty state")
	}
	if result.Overlap != nil {
		t.Error("expected nil overlap for empty state")
	}
}

func TestAnalysisHandleAnalysis_Warnings(t *testing.T) {
	mockSvc := &mockAnalysisService{
		result: &analysis.AnalysisResult{
			PortfolioID: 1,
			ComputedAt:  time.Now().UTC(),
			Warnings:    []string{"market data unavailable for 2 of 5 positions"},
			Overlap: &analysis.OverlapResult{
				PairwiseMatrix: []analysis.OverlapPair{},
			},
		},
	}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?portfolio_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result analysis.AnalysisResult
	json.NewDecoder(w.Body).Decode(&result)
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestAnalysisHandleAnalysis_InvalidSection(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?section=foobar", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if mockSvc.called {
		t.Error("service should not be called with invalid section")
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_SECTION" {
		t.Errorf("expected INVALID_SECTION, got %q", errResp.Code)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestAnalysisHandleAnalysis_ValidSections(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	sections := []string{
		"overlap", "correlation", "sector_allocation",
		"geographic_allocation", "stress_test", "factor_exposure",
	}

	for _, section := range sections {
		mockSvc.called = false
		req := httptest.NewRequest(http.MethodGet, "/api/analysis?section="+section, nil)
		w := httptest.NewRecorder()
		handler.HandleAnalysis(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for section %q, got %d", section, w.Code)
		}
		if !mockSvc.called {
			t.Errorf("service should be called for valid section %q", section)
		}
	}
}

func TestAnalysisHandleAnalysis_InvalidPeriod(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis?period=2Y", nil)
	w := httptest.NewRecorder()

	handler.HandleAnalysis(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if mockSvc.called {
		t.Error("service should not be called with invalid period")
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PERIOD" {
		t.Errorf("expected INVALID_PERIOD, got %q", errResp.Code)
	}
}

func TestAnalysisHandleAnalysis_ValidPeriods(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	handler := NewAnalysisHandler(mockSvc)

	periods := []string{"1Y", "3Y", "5Y", "10Y"}

	for _, period := range periods {
		mockSvc.called = false
		req := httptest.NewRequest(http.MethodGet, "/api/analysis?period="+period, nil)
		w := httptest.NewRecorder()
		handler.HandleAnalysis(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for period %q, got %d", period, w.Code)
		}
		if !mockSvc.called {
			t.Errorf("service should be called for valid period %q", period)
		}
	}
}

// --- parseAnalysisFilters Tests ---

func TestParseAnalysisFilters_PortfolioID(t *testing.T) {
	query := map[string][]string{"portfolio_id": []string{"7"}}
	filters := parseAnalysisFilters(url.Values(query))
	if filters.PortfolioID == nil || *filters.PortfolioID != 7 {
		t.Errorf("expected portfolio_id 7, got %v", filters.PortfolioID)
	}
}

func TestParseAnalysisFilters_InvalidPortfolioID(t *testing.T) {
	query := map[string][]string{"portfolio_id": []string{"not_a_number"}}
	filters := parseAnalysisFilters(url.Values(query))
	if filters.PortfolioID != nil {
		t.Errorf("expected nil portfolio_id for invalid input, got %v", filters.PortfolioID)
	}
}

func TestParseAnalysisFilters_Section(t *testing.T) {
	sections := []string{"overlap", "correlation", "sector_allocation", "geographic_allocation", "stress_test", "factor_exposure"}
	for _, section := range sections {
		query := map[string][]string{"section": []string{section}}
		filters := parseAnalysisFilters(url.Values(query))
		if filters.Section != section {
			t.Errorf("expected section %q, got %q", section, filters.Section)
		}
	}
}

func TestParseAnalysisFilters_Period(t *testing.T) {
	periods := []string{"1Y", "3Y", "5Y", "10Y"}
	for _, period := range periods {
		query := map[string][]string{"period": []string{period}}
		filters := parseAnalysisFilters(url.Values(query))
		if filters.Period != period {
			t.Errorf("expected period %q, got %q", period, filters.Period)
		}
	}
}

func TestParseAnalysisFilters_AllParams(t *testing.T) {
	query := map[string][]string{
		"portfolio_id": []string{"99"},
		"section":      []string{"stress_test"},
		"period":       []string{"10Y"},
	}
	filters := parseAnalysisFilters(url.Values(query))
	if filters.PortfolioID == nil || *filters.PortfolioID != 99 {
		t.Errorf("expected portfolio_id 99, got %v", filters.PortfolioID)
	}
	if filters.Section != "stress_test" {
		t.Errorf("expected section 'stress_test', got %q", filters.Section)
	}
	if filters.Period != "10Y" {
		t.Errorf("expected period '10Y', got %q", filters.Period)
	}
}

func TestParseAnalysisFilters_Empty(t *testing.T) {
	filters := parseAnalysisFilters(url.Values{})
	if filters.PortfolioID != nil {
		t.Error("expected nil portfolio_id")
	}
	if filters.Section != "" {
		t.Error("expected empty section")
	}
	if filters.Period != "" {
		t.Error("expected empty period")
	}
}

// --- Route Registration Tests ---

func TestAnalysisRoutesRegistered(t *testing.T) {
	mockSvc := &mockAnalysisService{result: makeTestResult()}
	r := chi.NewRouter()
	handler := NewAnalysisHandler(mockSvc)
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/analysis, got %d", w.Code)
	}
}

// --- JSON round-trip Tests ---

func TestAnalysisResultJSONRoundTrip(t *testing.T) {
	result := makeTestResult()

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded analysis.AnalysisResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.PortfolioID != result.PortfolioID {
		t.Errorf("portfolio_id mismatch: %d vs %d", decoded.PortfolioID, result.PortfolioID)
	}
	if decoded.Overlap == nil {
		t.Error("overlap should not be nil after round-trip")
	}
	if decoded.FactorExposure == nil {
		t.Error("factor_exposure should not be nil after round-trip")
	}
	if decoded.FactorExposure.ValueGrowthTilt.Tilt != "growth" {
		t.Errorf("expected tilt 'growth', got %q", decoded.FactorExposure.ValueGrowthTilt.Tilt)
	}
}

func TestAnalysisResult_OmitEmpty(t *testing.T) {
	// When all sections are nil, only portfolio_id, computed_at, and message should appear
	result := &analysis.AnalysisResult{
		PortfolioID: 1,
		ComputedAt:  time.Date(2025, 5, 18, 0, 0, 0, 0, time.UTC),
		Message:     "no positions",
	}

	data, _ := json.Marshal(result)
	// Verify the JSON doesn't contain section keys
	jsonStr := string(data)
	if containsStr(jsonStr, "overlap") {
		t.Error("should not contain 'overlap' when nil")
	}
	if containsStr(jsonStr, "correlation") {
		t.Error("should not contain 'correlation' when nil")
	}
	if containsStr(jsonStr, "warnings") {
		t.Error("should not contain 'warnings' when empty")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ptrMatrix converts a [][]float64 to [][]*float64 for test convenience.
func ptrMatrix(m [][]float64) [][]*float64 {
	result := make([][]*float64, len(m))
	for i, row := range m {
		result[i] = make([]*float64, len(row))
		for j, v := range row {
			result[i][j] = &v
		}
	}
	return result
}
