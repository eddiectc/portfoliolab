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

	"github.com/eddiectc/portfoliolab/internal/domain/comparison"
)

// --- Mock comparison service ---

type mockComparisonService struct {
	result *comparison.ComparisonResult
	err    error
	called bool
	req    comparison.ComparisonRequest
}

func (m *mockComparisonService) ComputeComparison(_ context.Context, req comparison.ComparisonRequest) (*comparison.ComparisonResult, error) {
	m.called = true
	m.req = req
	return m.result, m.err
}

// --- Test helpers ---

func makeTestComparisonResult() *comparison.ComparisonResult {
	twrPct := decimal.MustParse("15.50")
	cagrPct := decimal.MustParse("7.50")
	volPct := decimal.MustParse("12.30")
	sharpe := decimal.MustParse("0.65")
	sortino := decimal.MustParse("0.92")
	maxDD := decimal.MustParse("-8.50")
	curDD := decimal.MustParse("-2.30")
	overlapPct := decimal.MustParse("45.00")
	beta := decimal.MustParse("1.05")
	alpha := decimal.MustParse("2.50")
	correlation := decimal.MustParse("0.87")

	return &comparison.ComparisonResult{
		ComputedAt: time.Now().UTC(),
		PortfolioA: &comparison.PortfolioComparison{
			ID:   1,
			Name: "Growth Portfolio",
			Type: comparison.PortTypeModel,
			ReturnMetrics: &comparison.ReturnMetrics{
				TWRPct:      &twrPct,
				CAGRPct:     &cagrPct,
				DaysElapsed: 365,
			},
			RiskMetrics: &comparison.RiskMetrics{
				AnnualizedVolatilityPct: &volPct,
				SharpeRatio:             &sharpe,
				SortinoRatio:            &sortino,
			},
			Drawdown: &comparison.DrawdownResult{
				MaxDrawdownPct:       &maxDD,
				CurrentDrawdownPct:   &curDD,
				DrawdownDurationDays: intPtr(45),
			},
			YearlyReturns: []comparison.YearlyReturn{
				{Year: 2024, ReturnPct: &twrPct},
			},
		},
		PortfolioB: &comparison.PortfolioComparison{
			ID:   2,
			Name: "Value Portfolio",
			Type: comparison.PortTypeModel,
			ReturnMetrics: &comparison.ReturnMetrics{
				TWRPct:      &twrPct,
				CAGRPct:     &cagrPct,
				DaysElapsed: 365,
			},
			RiskMetrics: &comparison.RiskMetrics{
				AnnualizedVolatilityPct: &volPct,
				SharpeRatio:             &sharpe,
				SortinoRatio:            &sortino,
			},
			Drawdown: &comparison.DrawdownResult{
				MaxDrawdownPct:       &maxDD,
				CurrentDrawdownPct:   &curDD,
				DrawdownDurationDays: intPtr(30),
			},
		},
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			BetaAlpha: &comparison.BetaAlphaResult{
				Beta:  &beta,
				Alpha: &alpha,
			},
			Correlation: &comparison.PortfolioCorrelationResult{
				Correlation: &correlation,
			},
			Overlap: &comparison.OverlapResult{
				OverlapPct: &overlapPct,
			},
		},
		Warnings: []string{"symbol data clipped for GOOG"},
	}
}

func intPtr(n int) *int { return &n }

// --- HandleComparison Tests ---

func TestComparisonHandleComparison_Success(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if !mockSvc.called {
		t.Fatal("service.ComputeComparison was not called")
	}

	var result comparison.ComparisonResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.PortfolioA == nil {
		t.Fatal("expected portfolio_a")
	}
	if result.PortfolioA.Name != "Growth Portfolio" {
		t.Errorf("expected 'Growth Portfolio', got %q", result.PortfolioA.Name)
	}
	if result.PortfolioB == nil {
		t.Fatal("expected portfolio_b")
	}
	if result.CrossMetrics == nil {
		t.Error("expected cross_metrics")
	}
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestComparisonHandleComparison_ModelVsReal(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=5&portfolio_b_type=real", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if !mockSvc.called {
		t.Fatal("service was not called")
	}

	if mockSvc.req.PortfolioAType != comparison.PortTypeModel {
		t.Errorf("expected portfolio_a_type=model, got %q", mockSvc.req.PortfolioAType)
	}
	if mockSvc.req.PortfolioBType != comparison.PortTypeReal {
		t.Errorf("expected portfolio_b_type=real, got %q", mockSvc.req.PortfolioBType)
	}
}

func TestComparisonHandleComparison_WithPeriod(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&period=3Y", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if mockSvc.req.Period != "3Y" {
		t.Errorf("expected period 3Y, got %q", mockSvc.req.Period)
	}
}

func TestComparisonHandleComparison_WithCustomDates(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&date_from=2024-01-01&date_to=2024-12-31", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if mockSvc.req.DateFrom == nil {
		t.Fatal("expected date_from to be set")
	}
	if !mockSvc.req.DateFrom.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected date_from 2024-01-01, got %v", mockSvc.req.DateFrom)
	}
	if mockSvc.req.DateTo == nil {
		t.Fatal("expected date_to to be set")
	}
}

func TestComparisonHandleComparison_WithBaseCurrency(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&base_currency=EUR", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if mockSvc.req.BaseCurrency != "EUR" {
		t.Errorf("expected base_currency EUR, got %q", mockSvc.req.BaseCurrency)
	}
}

func TestComparisonHandleComparison_WithStartingValue(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&starting_value=50000", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	expected := decimal.MustParse("50000")
	if !mockSvc.req.StartingValue.Equal(expected) {
		t.Errorf("expected starting_value 50000, got %s", mockSvc.req.StartingValue.String())
	}
}

func TestComparisonHandleComparison_DefaultStartingValue(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	expected := decimal.MustParse("10000")
	if !mockSvc.req.StartingValue.Equal(expected) {
		t.Errorf("expected default starting_value 10000, got %s", mockSvc.req.StartingValue.String())
	}
}

func TestComparisonHandleComparison_InternalError(t *testing.T) {
	mockSvc := &mockComparisonService{err: fmt.Errorf("database connection failed")}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INTERNAL_ERROR" {
		t.Errorf("expected INTERNAL_ERROR, got %q", errResp.Code)
	}
}

// --- Validation Tests ---

func TestComparisonHandleComparison_MissingPortfolioAID(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if mockSvc.called {
		t.Error("service should not be called")
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISSING_PORTFOLIO_A_ID" {
		t.Errorf("expected MISSING_PORTFOLIO_A_ID, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_MissingPortfolioAType(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISSING_PORTFOLIO_A_TYPE" {
		t.Errorf("expected MISSING_PORTFOLIO_A_TYPE, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_MissingPortfolioBID(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISSING_PORTFOLIO_B_ID" {
		t.Errorf("expected MISSING_PORTFOLIO_B_ID, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_MissingPortfolioBType(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISSING_PORTFOLIO_B_TYPE" {
		t.Errorf("expected MISSING_PORTFOLIO_B_TYPE, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_InvalidPortfolioAType(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=custom&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if mockSvc.called {
		t.Error("service should not be called")
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PORTFOLIO_A_TYPE" {
		t.Errorf("expected INVALID_PORTFOLIO_A_TYPE, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_InvalidPortfolioBType(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=custom", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PORTFOLIO_B_TYPE" {
		t.Errorf("expected INVALID_PORTFOLIO_B_TYPE, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_InvalidPeriod(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&period=2Y", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if mockSvc.called {
		t.Error("service should not be called")
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PERIOD" {
		t.Errorf("expected INVALID_PERIOD, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_ValidPeriods(t *testing.T) {
	periods := []string{"1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"}

	for _, period := range periods {
		mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
		handler := NewComparisonHandler(mockSvc)

		req := httptest.NewRequest(http.MethodGet,
			fmt.Sprintf("/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&period=%s", period), nil)
		w := httptest.NewRecorder()
		handler.HandleComparison(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for period %q, got %d", period, w.Code)
		}
		if !mockSvc.called {
			t.Errorf("service should be called for valid period %q", period)
		}
	}
}

func TestComparisonHandleComparison_InvalidDateFrom(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&date_from=not-a-date", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_DATE_FROM" {
		t.Errorf("expected INVALID_DATE_FROM, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_InvalidDateTo(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&date_to=not-a-date", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_DATE_TO" {
		t.Errorf("expected INVALID_DATE_TO, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_InvalidStartingValue(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&starting_value=abc", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_STARTING_VALUE" {
		t.Errorf("expected INVALID_STARTING_VALUE, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_ZeroStartingValue(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&starting_value=0", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_STARTING_VALUE" {
		t.Errorf("expected INVALID_STARTING_VALUE, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_NegativeStartingValue(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&starting_value=-1000", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_STARTING_VALUE" {
		t.Errorf("expected INVALID_STARTING_VALUE, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_InvalidPortfolioAID(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=abc&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PORTFOLIO_A_ID" {
		t.Errorf("expected INVALID_PORTFOLIO_A_ID, got %q", errResp.Code)
	}
}

func TestComparisonHandleComparison_InvalidPortfolioBID(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=abc&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PORTFOLIO_B_ID" {
		t.Errorf("expected INVALID_PORTFOLIO_B_ID, got %q", errResp.Code)
	}
}

// --- Empty State Test ---

func TestComparisonHandleComparison_EmptyState(t *testing.T) {
	mockSvc := &mockComparisonService{
		result: &comparison.ComparisonResult{
			ComputedAt: time.Now().UTC(),
			PortfolioA: &comparison.PortfolioComparison{
				ID:   1,
				Name: "Test",
				Type: comparison.PortTypeModel,
				ReturnMetrics: &comparison.ReturnMetrics{
					HasInsufficientData: true,
				},
				Message: "Insufficient data for comparison",
			},
			PortfolioB: &comparison.PortfolioComparison{
				ID:   2,
				Name: "Test2",
				Type: comparison.PortTypeModel,
				ReturnMetrics: &comparison.ReturnMetrics{
					HasInsufficientData: true,
				},
				Message: "Insufficient data for comparison",
			},
			Message: "Insufficient data for both portfolios",
		},
	}
	handler := NewComparisonHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()

	handler.HandleComparison(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result comparison.ComparisonResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result.Message == "" {
		t.Error("expected non-empty message for empty state")
	}
}

// --- parseComparisonRequest Tests ---

func TestParseComparisonRequest_AllParams(t *testing.T) {
	query := map[string][]string{
		"portfolio_a_id":   []string{"1"},
		"portfolio_a_type": []string{"model"},
		"portfolio_b_id":   []string{"2"},
		"portfolio_b_type": []string{"model"},
		"period":           []string{"3Y"},
		"date_from":        []string{"2024-01-01"},
		"date_to":          []string{"2024-12-31"},
		"base_currency":    []string{"EUR"},
		"starting_value":   []string{"50000"},
	}
	req, err := parseComparisonRequest(url.Values(query))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.PortfolioAID != 1 {
		t.Errorf("expected portfolio_a_id 1, got %d", req.PortfolioAID)
	}
	if req.PortfolioAType != comparison.PortTypeModel {
		t.Errorf("expected portfolio_a_type model, got %q", req.PortfolioAType)
	}
	if req.PortfolioBID != 2 {
		t.Errorf("expected portfolio_b_id 2, got %d", req.PortfolioBID)
	}
	if req.PortfolioBType != comparison.PortTypeModel {
		t.Errorf("expected portfolio_b_type model, got %q", req.PortfolioBType)
	}
	if req.Period != "3Y" {
		t.Errorf("expected period 3Y, got %q", req.Period)
	}
	if req.DateFrom == nil || !req.DateFrom.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected date_from: %v", req.DateFrom)
	}
	if req.DateTo == nil || !req.DateTo.Equal(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected date_to: %v", req.DateTo)
	}
	if req.BaseCurrency != "EUR" {
		t.Errorf("expected base_currency EUR, got %q", req.BaseCurrency)
	}
	expected := decimal.MustParse("50000")
	if !req.StartingValue.Equal(expected) {
		t.Errorf("expected starting_value 50000, got %s", req.StartingValue.String())
	}
}

func TestParseComparisonRequest_MinimalParams(t *testing.T) {
	query := map[string][]string{
		"portfolio_a_id":   []string{"1"},
		"portfolio_a_type": []string{"model"},
		"portfolio_b_id":   []string{"2"},
		"portfolio_b_type": []string{"real"},
	}
	req, err := parseComparisonRequest(url.Values(query))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.PortfolioAID != 1 {
		t.Errorf("expected portfolio_a_id 1, got %d", req.PortfolioAID)
	}
	if req.PortfolioAType != comparison.PortTypeModel {
		t.Errorf("expected portfolio_a_type model, got %q", req.PortfolioAType)
	}
	if req.PortfolioBID != 2 {
		t.Errorf("expected portfolio_b_id 2, got %d", req.PortfolioBID)
	}
	if req.PortfolioBType != comparison.PortTypeReal {
		t.Errorf("expected portfolio_b_type real, got %q", req.PortfolioBType)
	}
	if req.Period != "" {
		t.Errorf("expected empty period, got %q", req.Period)
	}
	if req.DateFrom != nil {
		t.Error("expected nil date_from")
	}
	if req.DateTo != nil {
		t.Error("expected nil date_to")
	}
	if req.BaseCurrency != "" {
		t.Errorf("expected empty base_currency, got %q", req.BaseCurrency)
	}
	expected := decimal.MustParse("10000")
	if !req.StartingValue.Equal(expected) {
		t.Errorf("expected default starting_value 10000, got %s", req.StartingValue.String())
	}
}

func TestParseComparisonRequest_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name     string
		query    url.Values
		wantCode string
	}{
		{
			name:     "missing portfolio_a_id",
			query:    url.Values{"portfolio_a_type": []string{"model"}, "portfolio_b_id": []string{"2"}, "portfolio_b_type": []string{"model"}},
			wantCode: "MISSING_PORTFOLIO_A_ID",
		},
		{
			name:     "missing portfolio_a_type",
			query:    url.Values{"portfolio_a_id": []string{"1"}, "portfolio_b_id": []string{"2"}, "portfolio_b_type": []string{"model"}},
			wantCode: "MISSING_PORTFOLIO_A_TYPE",
		},
		{
			name:     "missing portfolio_b_id",
			query:    url.Values{"portfolio_a_id": []string{"1"}, "portfolio_a_type": []string{"model"}, "portfolio_b_type": []string{"model"}},
			wantCode: "MISSING_PORTFOLIO_B_ID",
		},
		{
			name:     "missing portfolio_b_type",
			query:    url.Values{"portfolio_a_id": []string{"1"}, "portfolio_a_type": []string{"model"}, "portfolio_b_id": []string{"2"}},
			wantCode: "MISSING_PORTFOLIO_B_TYPE",
		},
		{
			name:     "all missing",
			query:    url.Values{},
			wantCode: "MISSING_PORTFOLIO_A_ID",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseComparisonRequest(tc.query)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Code != tc.wantCode {
				t.Errorf("expected code %q, got %q", tc.wantCode, err.Code)
			}
		})
	}
}

// --- Route Registration Tests ---

func TestComparisonRoutesRegistered(t *testing.T) {
	mockSvc := &mockComparisonService{result: makeTestComparisonResult()}
	r := chi.NewRouter()
	handler := NewComparisonHandler(mockSvc)
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/comparison, got %d", w.Code)
	}
}

// --- JSON round-trip Test ---

func TestComparisonResultJSONRoundTrip(t *testing.T) {
	result := makeTestComparisonResult()

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded comparison.ComparisonResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.PortfolioA.Name != result.PortfolioA.Name {
		t.Errorf("portfolio_a name mismatch: %q vs %q", decoded.PortfolioA.Name, result.PortfolioA.Name)
	}
	if decoded.PortfolioB.Name != result.PortfolioB.Name {
		t.Errorf("portfolio_b name mismatch: %q vs %q", decoded.PortfolioB.Name, result.PortfolioB.Name)
	}
	if decoded.CrossMetrics == nil {
		t.Error("cross_metrics should not be nil after round-trip")
	}
	if decoded.CrossMetrics.BetaAlpha == nil {
		t.Error("beta_alpha should not be nil after round-trip")
	}
	if decoded.CrossMetrics.Correlation == nil {
		t.Error("correlation should not be nil after round-trip")
	}
}
