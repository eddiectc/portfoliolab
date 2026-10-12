package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/comparison"
	"github.com/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
	"github.com/eddiectc/portfoliolab/internal/web"
)

func TestComparisonWebHandler_RenderPageWithResult(t *testing.T) {
	twr := decimal.MustParse("25.50")
	annTwr := decimal.MustParse("8.17")
	simpleRet := decimal.MustParse("25.50")
	cagr := decimal.MustParse("8.17")
	sharpe := decimal.MustParse("1.2500")
	vol := decimal.MustParse("12.50")
	maxDD := decimal.MustParse("-15.00")
	curDD := decimal.MustParse("-3.20")

	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{
			ID:   1,
			Name: "Model A",
			Type: comparison.PortTypeModel,
			ReturnMetrics: &comparison.ReturnMetrics{
				TWRPct:           &twr,
				AnnualizedTWRPct: &annTwr,
				CAGRPct:          &cagr,
				DaysElapsed:      365,
			},
			RiskMetrics: &comparison.RiskMetrics{
				AnnualizedVolatilityPct: &vol,
				SharpeRatio:             &sharpe,
			},
			Drawdown: &comparison.DrawdownResult{
				MaxDrawdownPct:     &maxDD,
				CurrentDrawdownPct: &curDD,
			},
			YearlyReturns: []comparison.YearlyReturn{
				{Year: 2024, ReturnPct: &simpleRet},
				{Year: 2023, ReturnPct: &cagr},
			},
		},
		PortfolioB: &comparison.PortfolioComparison{
			ID:   2,
			Name: "Model B",
			Type: comparison.PortTypeModel,
			ReturnMetrics: &comparison.ReturnMetrics{
				TWRPct:      &twr,
				CAGRPct:     &cagr,
				DaysElapsed: 365,
			},
			RiskMetrics: &comparison.RiskMetrics{
				AnnualizedVolatilityPct: &vol,
				SharpeRatio:             &sharpe,
			},
			Drawdown: &comparison.DrawdownResult{
				MaxDrawdownPct: &maxDD,
			},
			YearlyReturns: []comparison.YearlyReturn{
				{Year: 2024, ReturnPct: &cagr},
			},
		},
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			BetaAlpha: &comparison.BetaAlphaResult{
				Beta:  ptrDecimal(decimal.MustParse("1.0500")),
				Alpha: ptrDecimal(decimal.MustParse("2.50")),
			},
			Correlation: &comparison.PortfolioCorrelationResult{
				Correlation: ptrDecimal(decimal.MustParse("0.8500")),
			},
			Overlap: &comparison.OverlapResult{
				TopHoldingsA: []comparison.HoldingWeight{
					{Symbol: "AAPL", Weight: decimal.MustParse("0.50"), Name: "Apple Inc"},
				},
				TopHoldingsB: []comparison.HoldingWeight{
					{Symbol: "GOOG", Weight: decimal.MustParse("0.60"), Name: "Alphabet Inc"},
				},
				OverlapPct: ptrDecimal(decimal.MustParse("25.00")),
			},
		},
	}

	mockSvc := &mockComparisonService{result: result}
	apiHandler := NewComparisonHandler(mockSvc)

	renderer := newTestRenderer(t)
	webHandler := NewComparisonWebHandler(
		apiHandler,
		nil, // portfolioSvc
		nil, // modelPortfolioSvc
		renderer,
	)

	router := chi.NewRouter()
	webHandler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/comparison?portfolio_a_id=m1&portfolio_b_id=m2&period=1Y", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	checkComparisonContains(t, body, "Portfolio Comparison")
	checkComparisonContains(t, body, "Model A")
	checkComparisonContains(t, body, "Model B")
	checkComparisonContains(t, body, "Performance Metrics")
	checkComparisonContains(t, body, "Risk Metrics")
	checkComparisonContains(t, body, "Drawdown")
	checkComparisonContains(t, body, "Annual Returns")
	checkComparisonContains(t, body, "Cross-Portfolio Metrics")
	checkComparisonContains(t, body, "Holdings Overlap")
}

func TestComparisonWebHandler_RenderPageRebalancedStrategy(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{ID: 1, Name: "Model A", Type: comparison.PortTypeModel},
		PortfolioB: &comparison.PortfolioComparison{ID: 2, Name: "Model B", Type: comparison.PortTypeModel},
	}
	mockSvc := &mockComparisonService{result: result}
	apiHandler := NewComparisonHandler(mockSvc)
	renderer := newTestRenderer(t)
	webHandler := NewComparisonWebHandler(apiHandler, nil, nil, renderer)

	router := chi.NewRouter()
	webHandler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/comparison?portfolio_a_id=m1&portfolio_b_id=m2&period=1Y&strategy=rebalanced&rebalance_interval=monthly&rebalance_threshold_pct=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Strategy select shows rebalanced as selected.
	checkComparisonContains(t, body, `<option value="rebalanced" selected`)
	// Interval select is visible with monthly selected; drift input carries the value.
	checkComparisonContains(t, body, `<option value="monthly" selected`)
	checkComparisonContains(t, body, `value="5"`)
	// Period button URLs carry the strategy params.
	checkComparisonContains(t, body, "strategy=rebalanced")
	checkComparisonContains(t, body, "rebalance_interval=monthly")
	checkComparisonContains(t, body, "rebalance_threshold_pct=5")
	// Service received the strategy params.
	if mockSvc.req.Strategy != "rebalanced" || mockSvc.req.RebalanceInterval != "monthly" {
		t.Errorf("service request strategy params: got %+v", mockSvc.req)
	}
	if mockSvc.req.RebalanceThresholdPct == nil || !mockSvc.req.RebalanceThresholdPct.Equal(decimal.MustParse("5")) {
		t.Errorf("service request threshold: got %v", mockSvc.req.RebalanceThresholdPct)
	}
}

func TestComparisonWebHandler_RenderPageAsymmetricStrategy(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{ID: 1, Name: "Model A", Type: comparison.PortTypeModel},
		PortfolioB: &comparison.PortfolioComparison{ID: 2, Name: "Model B", Type: comparison.PortTypeModel},
	}
	mockSvc := &mockComparisonService{result: result}
	apiHandler := NewComparisonHandler(mockSvc)
	renderer := newTestRenderer(t)
	selector := &mockComparisonModelPortfolioSelector{
		summaries: []modelportfolio.ModelPortfolioSummary{
			{ID: 5, Name: "Stressed A MP"},
			{ID: 6, Name: "Stressed B MP"},
		},
		portfolio: modelportfolio.ModelPortfolio{
			ID:   5,
			Name: "Stressed",
			Entries: []modelportfolio.ModelPortfolioEntry{
				{Symbol: "MSFT", WeightPct: decimal.MustParse("30")},
				{Symbol: "AGG", WeightPct: decimal.MustParse("70")},
			},
		},
	}
	webHandler := NewComparisonWebHandler(apiHandler, nil, selector, renderer)

	router := chi.NewRouter()
	webHandler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/comparison?portfolio_a_id=m1&portfolio_b_id=m2&period=1Y&strategy=rebalanced_asymmetric&stressed_a_id=5&stressed_b_id=6&drawdown_trigger_pct=15&rebalance_interval=monthly&rebalance_threshold_pct=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Strategy select shows asymmetric as selected.
	checkComparisonContains(t, body, `<option value="rebalanced_asymmetric" selected`)
	// Stressed target selects exist with the selected option.
	checkComparisonContains(t, body, `Stressed A`)
	checkComparisonContains(t, body, `Stressed B`)
	checkComparisonContains(t, body, `<option value="5" selected>Stressed A MP</option>`)
	checkComparisonContains(t, body, `<option value="6" selected>Stressed B MP</option>`)
	// Trigger input carries the value.
	checkComparisonContains(t, body, `value="15"`)
	// Stressed target weight tables render with the portfolio name and weights.
	checkComparisonContains(t, body, "Stressed Target Weights")
	checkComparisonContains(t, body, "Side A — Stressed")
	checkComparisonContains(t, body, "MSFT")
	checkComparisonContains(t, body, "30.00")
	checkComparisonContains(t, body, "AGG")
	checkComparisonContains(t, body, "70.00")
	// Period button URLs carry the asymmetric params.
	checkComparisonContains(t, body, "strategy=rebalanced_asymmetric")
	checkComparisonContains(t, body, "stressed_a_id=5")
	checkComparisonContains(t, body, "stressed_b_id=6")
	checkComparisonContains(t, body, "drawdown_trigger_pct=15")
	// Service received the asymmetric params.
	if mockSvc.req.Strategy != comparison.StrategyRebalancedAsymmetric {
		t.Errorf("service request strategy: got %q", mockSvc.req.Strategy)
	}
	if mockSvc.req.StressedAID == nil || *mockSvc.req.StressedAID != 5 {
		t.Errorf("service request StressedAID: got %v", mockSvc.req.StressedAID)
	}
	if mockSvc.req.StressedBID == nil || *mockSvc.req.StressedBID != 6 {
		t.Errorf("service request StressedBID: got %v", mockSvc.req.StressedBID)
	}
	if mockSvc.req.DrawdownTriggerPct == nil || !mockSvc.req.DrawdownTriggerPct.Equal(decimal.MustParse("15")) {
		t.Errorf("service request trigger: got %v", mockSvc.req.DrawdownTriggerPct)
	}
}

func TestComparisonWebHandler_RenderPageStrategyControlsHiddenByDefault(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{ID: 1, Name: "Model A", Type: comparison.PortTypeModel},
		PortfolioB: &comparison.PortfolioComparison{ID: 2, Name: "Model B", Type: comparison.PortTypeModel},
	}
	mockSvc := &mockComparisonService{result: result}
	apiHandler := NewComparisonHandler(mockSvc)
	renderer := newTestRenderer(t)
	webHandler := NewComparisonWebHandler(apiHandler, nil, nil, renderer)

	router := chi.NewRouter()
	webHandler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/comparison?portfolio_a_id=m1&portfolio_b_id=m2&period=1Y", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Buy-and-hold is selected by default.
	checkComparisonContains(t, body, `<option value="buy_and_hold" selected`)
	// Interval and drift controls are hidden when strategy is buy-and-hold.
	checkComparisonContains(t, body, `<label class="rebalance-opt" style="display:none;">Interval`)
	checkComparisonContains(t, body, `<label class="rebalance-opt" style="display:none;">Drift%`)
}

func TestComparisonWebHandler_RenderPageWithError(t *testing.T) {
	mockSvc := &mockComparisonService{err: errors.New("service error")}
	apiHandler := NewComparisonHandler(mockSvc)

	renderer := newTestRenderer(t)
	webHandler := NewComparisonWebHandler(
		apiHandler,
		nil,
		nil,
		renderer,
	)

	router := chi.NewRouter()
	webHandler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/comparison?portfolio_a_id=m1&portfolio_b_id=m2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "An error occurred") {
		t.Error("expected error message in page")
	}
}

func TestComparisonWebHandler_RenderPageEmptyState(t *testing.T) {
	renderer := newTestRenderer(t)
	webHandler := NewComparisonWebHandler(
		nil, // no API handler — returns nil result
		nil,
		nil,
		renderer,
	)

	router := chi.NewRouter()
	webHandler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/comparison", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Select two portfolios") {
		t.Error("expected empty state message")
	}
}

func TestComparisonWebHandler_FetchPortfolios_NilService(t *testing.T) {
	handler := &ComparisonWebHandler{
		portfolioSvc: nil,
	}
	portfolios := handler.fetchPortfolios(context.TODO())
	if portfolios == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(portfolios) != 0 {
		t.Errorf("expected empty slice, got %d items", len(portfolios))
	}
}

func TestComparisonWebHandler_FetchModelPortfolios_NilService(t *testing.T) {
	handler := &ComparisonWebHandler{
		modelPortfolioSvc: nil,
	}
	summaries := handler.fetchModelPortfolios(context.TODO())
	if summaries == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(summaries) != 0 {
		t.Errorf("expected empty slice, got %d items", len(summaries))
	}
}

func TestParseComparisonFilter(t *testing.T) {
	tests := []struct {
		name   string
		query  map[string][]string
		expect comparisonFilter
	}{
		{
			name:   "all params combined format",
			query:  map[string][]string{"portfolio_a_id": {"m1"}, "portfolio_b_id": {"r2"}, "period": {"3Y"}, "date_from": {"2023-01-01"}, "date_to": {"2023-12-31"}, "base_currency": {"EUR"}, "starting_value": {"50000"}},
			expect: comparisonFilter{PortfolioAID: 1, PortfolioAType: "model", PortfolioBID: 2, PortfolioBType: "real", Period: "3Y", DateFrom: "2023-01-01", DateTo: "2023-12-31", BaseCurrency: "EUR", StartingValue: "50000"},
		},
		{
			name:   "minimal",
			query:  map[string][]string{},
			expect: comparisonFilter{PortfolioAType: "model", PortfolioBType: "model"},
		},
		{
			name:   "invalid id ignored",
			query:  map[string][]string{"portfolio_a_id": {"abc"}},
			expect: comparisonFilter{PortfolioAID: 0, PortfolioAType: "model", PortfolioBType: "model"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseComparisonFilter(tc.query)
			if got.PortfolioAID != tc.expect.PortfolioAID {
				t.Errorf("PortfolioAID: got %d, want %d", got.PortfolioAID, tc.expect.PortfolioAID)
			}
			if got.PortfolioAType != tc.expect.PortfolioAType {
				t.Errorf("PortfolioAType: got %q, want %q", got.PortfolioAType, tc.expect.PortfolioAType)
			}
			if got.Period != tc.expect.Period {
				t.Errorf("Period: got %q, want %q", got.Period, tc.expect.Period)
			}
		})
	}
}

func TestParseComparisonFilter_StrategyParams(t *testing.T) {
	tests := []struct {
		name   string
		query  map[string][]string
		expect comparisonFilter
	}{
		{
			name:   "rebalanced with interval and threshold",
			query:  map[string][]string{"portfolio_a_id": {"m1"}, "portfolio_b_id": {"m2"}, "strategy": {"rebalanced"}, "rebalance_interval": {"monthly"}, "rebalance_threshold_pct": {"2.5"}},
			expect: comparisonFilter{PortfolioAID: 1, PortfolioAType: "model", PortfolioBID: 2, PortfolioBType: "model", Strategy: "rebalanced", RebalanceInterval: "monthly", RebalanceThresholdPct: "2.5"},
		},
		{
			name:   "defaults when absent",
			query:  map[string][]string{"portfolio_a_id": {"m1"}, "portfolio_b_id": {"m2"}},
			expect: comparisonFilter{PortfolioAID: 1, PortfolioAType: "model", PortfolioBID: 2, PortfolioBType: "model"},
		},
		{
			name:   "buy_and_hold",
			query:  map[string][]string{"portfolio_a_id": {"m1"}, "portfolio_b_id": {"m2"}, "strategy": {"buy_and_hold"}},
			expect: comparisonFilter{PortfolioAID: 1, PortfolioAType: "model", PortfolioBID: 2, PortfolioBType: "model", Strategy: "buy_and_hold"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseComparisonFilter(tc.query)
			if got.Strategy != tc.expect.Strategy {
				t.Errorf("Strategy: got %q, want %q", got.Strategy, tc.expect.Strategy)
			}
			if got.RebalanceInterval != tc.expect.RebalanceInterval {
				t.Errorf("RebalanceInterval: got %q, want %q", got.RebalanceInterval, tc.expect.RebalanceInterval)
			}
			if got.RebalanceThresholdPct != tc.expect.RebalanceThresholdPct {
				t.Errorf("RebalanceThresholdPct: got %q, want %q", got.RebalanceThresholdPct, tc.expect.RebalanceThresholdPct)
			}
		})
	}
}

func TestBuildComparisonRequest_StrategyParams(t *testing.T) {
	t.Run("maps strategy, interval and threshold", func(t *testing.T) {
		req := buildComparisonRequest(comparisonFilter{Strategy: "rebalanced", RebalanceInterval: "annual", RebalanceThresholdPct: "3.5"})
		if req.Strategy != "rebalanced" {
			t.Errorf("Strategy: got %q", req.Strategy)
		}
		if req.RebalanceInterval != "annual" {
			t.Errorf("RebalanceInterval: got %q", req.RebalanceInterval)
		}
		if req.RebalanceThresholdPct == nil || !req.RebalanceThresholdPct.Equal(decimal.MustParse("3.5")) {
			t.Errorf("RebalanceThresholdPct: got %v", req.RebalanceThresholdPct)
		}
	})
	t.Run("invalid threshold silently ignored", func(t *testing.T) {
		req := buildComparisonRequest(comparisonFilter{Strategy: "rebalanced", RebalanceThresholdPct: "abc"})
		if req.RebalanceThresholdPct != nil {
			t.Errorf("expected nil threshold, got %v", req.RebalanceThresholdPct)
		}
	})
	t.Run("empty filter omits strategy params", func(t *testing.T) {
		req := buildComparisonRequest(comparisonFilter{})
		if req.Strategy != "" || req.RebalanceInterval != "" || req.RebalanceThresholdPct != nil {
			t.Errorf("expected empty strategy params, got %+v", req)
		}
	})
}

func TestParseComparisonFilter_AsymmetricParams(t *testing.T) {
	t.Run("asymmetric with stressed ids and trigger", func(t *testing.T) {
		query := map[string][]string{
			"portfolio_a_id":       {"m1"},
			"portfolio_b_id":       {"m2"},
			"strategy":             {"rebalanced_asymmetric"},
			"stressed_a_id":        {"7"},
			"stressed_b_id":        {"9"},
			"drawdown_trigger_pct": {"15"},
		}
		got := parseComparisonFilter(query)
		if got.Strategy != "rebalanced_asymmetric" {
			t.Errorf("Strategy: got %q", got.Strategy)
		}
		if got.StressedAID != 7 {
			t.Errorf("StressedAID: got %d, want 7", got.StressedAID)
		}
		if got.StressedBID != 9 {
			t.Errorf("StressedBID: got %d, want 9", got.StressedBID)
		}
		if got.DrawdownTriggerPct != "15" {
			t.Errorf("DrawdownTriggerPct: got %q", got.DrawdownTriggerPct)
		}
	})
	t.Run("invalid stressed ids silently ignored", func(t *testing.T) {
		query := map[string][]string{
			"portfolio_a_id":       {"m1"},
			"portfolio_b_id":       {"m2"},
			"strategy":             {"rebalanced_asymmetric"},
			"stressed_a_id":        {"abc"},
			"stressed_b_id":        {"12x"},
			"drawdown_trigger_pct": {"20"},
		}
		got := parseComparisonFilter(query)
		if got.StressedAID != 0 {
			t.Errorf("StressedAID: got %d, want 0 (ignored)", got.StressedAID)
		}
		if got.StressedBID != 0 {
			t.Errorf("StressedBID: got %d, want 0 (ignored)", got.StressedBID)
		}
		if got.DrawdownTriggerPct != "20" {
			t.Errorf("DrawdownTriggerPct: got %q", got.DrawdownTriggerPct)
		}
	})
	t.Run("absent asymmetric params", func(t *testing.T) {
		got := parseComparisonFilter(map[string][]string{"strategy": {"rebalanced_asymmetric"}})
		if got.StressedAID != 0 || got.StressedBID != 0 || got.DrawdownTriggerPct != "" {
			t.Errorf("expected zero asymmetric params, got %+v", got)
		}
	})
}

func TestBuildComparisonRequest_AsymmetricParams(t *testing.T) {
	t.Run("maps stressed ids and trigger", func(t *testing.T) {
		req := buildComparisonRequest(comparisonFilter{
			Strategy: "rebalanced_asymmetric", StressedAID: 3, StressedBID: 4, DrawdownTriggerPct: "12.5",
		})
		if req.StressedAID == nil || *req.StressedAID != 3 {
			t.Errorf("StressedAID: got %v, want ptr to 3", req.StressedAID)
		}
		if req.StressedBID == nil || *req.StressedBID != 4 {
			t.Errorf("StressedBID: got %v, want ptr to 4", req.StressedBID)
		}
		if req.DrawdownTriggerPct == nil || !req.DrawdownTriggerPct.Equal(decimal.MustParse("12.5")) {
			t.Errorf("DrawdownTriggerPct: got %v", req.DrawdownTriggerPct)
		}
	})
	t.Run("invalid trigger silently ignored", func(t *testing.T) {
		req := buildComparisonRequest(comparisonFilter{Strategy: "rebalanced_asymmetric", DrawdownTriggerPct: "abc"})
		if req.DrawdownTriggerPct != nil {
			t.Errorf("expected nil trigger, got %v", req.DrawdownTriggerPct)
		}
	})
	t.Run("unset ids omitted", func(t *testing.T) {
		req := buildComparisonRequest(comparisonFilter{Strategy: "rebalanced_asymmetric"})
		if req.StressedAID != nil || req.StressedBID != nil {
			t.Errorf("expected nil stressed ids, got %+v", req)
		}
	})
}

func TestComparisonWebHandler_FetchStressedWeights(t *testing.T) {
	t.Run("nil service returns empty", func(t *testing.T) {
		h := &ComparisonWebHandler{modelPortfolioSvc: nil}
		rows, name := h.fetchStressedWeights(context.TODO(), 1)
		if rows != nil || name != "" {
			t.Errorf("expected empty, got %v %q", rows, name)
		}
	})
	t.Run("zero id returns empty", func(t *testing.T) {
		h := &ComparisonWebHandler{modelPortfolioSvc: &mockComparisonModelPortfolioSelector{}}
		rows, name := h.fetchStressedWeights(context.TODO(), 0)
		if rows != nil || name != "" {
			t.Errorf("expected empty, got %v %q", rows, name)
		}
	})
	t.Run("get error returns empty", func(t *testing.T) {
		h := &ComparisonWebHandler{modelPortfolioSvc: &mockComparisonModelPortfolioSelector{err: errors.New("not found")}}
		rows, name := h.fetchStressedWeights(context.TODO(), 5)
		if rows != nil || name != "" {
			t.Errorf("expected empty, got %v %q", rows, name)
		}
	})
	t.Run("maps entries to rows", func(t *testing.T) {
		h := &ComparisonWebHandler{modelPortfolioSvc: &mockComparisonModelPortfolioSelector{
			portfolio: modelportfolio.ModelPortfolio{
				ID:   5,
				Name: "Stressed",
				Entries: []modelportfolio.ModelPortfolioEntry{
					{Symbol: "MSFT", WeightPct: decimal.MustParse("30")},
					{Symbol: "AGG", WeightPct: decimal.MustParse("70")},
				},
			},
		}}
		rows, name := h.fetchStressedWeights(context.TODO(), 5)
		if name != "Stressed" {
			t.Errorf("name: got %q, want %q", name, "Stressed")
		}
		if len(rows) != 2 {
			t.Fatalf("rows: got %d, want 2", len(rows))
		}
		if rows[0].Symbol != "MSFT" || rows[0].WeightPct != 30 {
			t.Errorf("row 0: got %+v", rows[0])
		}
		if rows[1].Symbol != "AGG" || rows[1].WeightPct != 70 {
			t.Errorf("row 1: got %+v", rows[1])
		}
	})
}

func TestSerializeAnnualReturnsChartData(t *testing.T) {
	ret1 := decimal.MustParse("15.50")
	ret2 := decimal.MustParse("-3.20")
	ret3 := decimal.MustParse("8.00")

	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{
			Name: "A",
			YearlyReturns: []comparison.YearlyReturn{
				{Year: 2024, ReturnPct: &ret1},
				{Year: 2023, ReturnPct: &ret2},
			},
		},
		PortfolioB: &comparison.PortfolioComparison{
			Name: "B",
			YearlyReturns: []comparison.YearlyReturn{
				{Year: 2024, ReturnPct: &ret3},
			},
		},
	}

	jsonStr := serializeAnnualReturnsChartData(result)
	var data annualReturnsChartData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal chart data: %v", err)
	}

	// Intersection: only 2024 is in both portfolios.
	if len(data.Years) != 1 {
		t.Errorf("expected 1 year (intersection), got %d", len(data.Years))
	}
	if data.NameA != "A" {
		t.Errorf("expected name_a=A, got %q", data.NameA)
	}
	if len(data.ReturnA) != 1 || len(data.ReturnB) != 1 {
		t.Errorf("expected 1 return value each (intersection), got A=%d B=%d", len(data.ReturnA), len(data.ReturnB))
	}
}

func TestSerializeAnnualReturnsChartData_NilResult(t *testing.T) {
	jsonStr := serializeAnnualReturnsChartData(nil)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestSerializeDrawdownChartData_NegatedValues(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	dd5 := decimal.MustParse("5.00")
	dd10 := decimal.MustParse("10.00")
	dd0 := decimal.MustParse("0.00")

	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{
			Name: "Portfolio A",
			DrawdownSeries: []comparison.DrawdownSeriesPoint{
				{Date: base, Pct: dd0},
				{Date: base.AddDate(0, 0, 1), Pct: dd5},
				{Date: base.AddDate(0, 0, 2), Pct: dd10},
			},
		},
		PortfolioB: &comparison.PortfolioComparison{
			Name: "Portfolio B",
			DrawdownSeries: []comparison.DrawdownSeriesPoint{
				{Date: base, Pct: dd0},
				{Date: base.AddDate(0, 0, 1), Pct: dd5},
				{Date: base.AddDate(0, 0, 2), Pct: dd0},
			},
		},
	}

	jsonStr := serializeDrawdownChartData(result)
	var data drawdownChartData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal chart data: %v", err)
	}

	// Values should be negated so drawdown goes downward from 0%.
	if len(data.PortfolioA) != 3 {
		t.Fatalf("expected 3 data points for A, got %d", len(data.PortfolioA))
	}
	if data.PortfolioA[0] != 0.0 {
		t.Errorf("PortfolioA[0] = %.2f, want 0.0", data.PortfolioA[0])
	}
	if data.PortfolioA[1] != -5.0 {
		t.Errorf("PortfolioA[1] = %.2f, want -5.0 (negated)", data.PortfolioA[1])
	}
	if data.PortfolioA[2] != -10.0 {
		t.Errorf("PortfolioA[2] = %.2f, want -10.0 (negated)", data.PortfolioA[2])
	}
}

func TestSerializeMonthlyHistogramCombined(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{
			Name: "Portfolio A",
			ReturnDistribution: &comparison.ReturnDistribution{
				Monthly: []comparison.ReturnBucket{
					{Label: "-5% to 0%", Count: 10},
					{Label: "0% to 5%", Count: 15},
				},
			},
		},
		PortfolioB: &comparison.PortfolioComparison{
			Name: "Portfolio B",
			ReturnDistribution: &comparison.ReturnDistribution{
				Monthly: []comparison.ReturnBucket{
					{Label: "0% to 5%", Count: 8},
					{Label: "5% to 10%", Count: 5},
				},
			},
		},
	}

	jsonStr := serializeMonthlyHistogramCombined(result)
	var data combinedHistogramData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	// Union of bins: -5% to 0%, 0% to 5%, 5% to 10%
	if len(data.Bins) != 3 {
		t.Errorf("expected 3 bins, got %d", len(data.Bins))
	}
	// countsA: 10, 15, 0
	if data.CountsA[0] != 10 {
		t.Errorf("CountsA[0] = %.0f, want 10", data.CountsA[0])
	}
	if data.CountsA[1] != 15 {
		t.Errorf("CountsA[1] = %.0f, want 15", data.CountsA[1])
	}
	if data.CountsA[2] != 0 {
		t.Errorf("CountsA[2] = %.0f, want 0", data.CountsA[2])
	}
	// countsB: 0, 8, 5
	if data.CountsB[0] != 0 {
		t.Errorf("CountsB[0] = %.0f, want 0", data.CountsB[0])
	}
	if data.CountsB[1] != 8 {
		t.Errorf("CountsB[1] = %.0f, want 8", data.CountsB[1])
	}
	if data.CountsB[2] != 5 {
		t.Errorf("CountsB[2] = %.0f, want 5", data.CountsB[2])
	}
}

func TestSerializeMonthlyHistogramCombined_Nil(t *testing.T) {
	jsonStr := serializeMonthlyHistogramCombined(nil)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestSerializeOverlapChartData(t *testing.T) {
	overlap := decimal.MustParse("25.00")
	result := &comparison.ComparisonResult{
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			Overlap: &comparison.OverlapResult{
				TopHoldingsA: []comparison.HoldingWeight{
					{Symbol: "AAPL", Weight: decimal.MustParse("0.50")},
				},
				TopHoldingsB: []comparison.HoldingWeight{
					{Symbol: "GOOG", Weight: decimal.MustParse("0.60")},
				},
				OverlapPct: &overlap,
			},
		},
	}

	jsonStr := serializeOverlapChartData(result)
	var data overlapChartData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(data.HoldingsA) != 1 {
		t.Errorf("expected 1 holding A, got %d", len(data.HoldingsA))
	}
	if data.OverlapPct == nil || *data.OverlapPct != 25.0 {
		t.Errorf("expected overlap_pct=25.0, got %v", data.OverlapPct)
	}
}

func TestMergeYearlyReturns(t *testing.T) {
	ret1 := decimal.MustParse("15.50")
	ret2 := decimal.MustParse("-3.20")
	ret3 := decimal.MustParse("8.00")

	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{
			YearlyReturns: []comparison.YearlyReturn{
				{Year: 2024, ReturnPct: &ret1},
				{Year: 2023, ReturnPct: &ret2},
			},
		},
		PortfolioB: &comparison.PortfolioComparison{
			YearlyReturns: []comparison.YearlyReturn{
				{Year: 2024, ReturnPct: &ret3},
			},
		},
	}

	rows := mergeYearlyReturns(result)
	// Intersection: only 2024 is in both portfolios.
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (intersection), got %d", len(rows))
	}
	if rows[0].Year != 2024 {
		t.Errorf("row year: got %d, want 2024", rows[0].Year)
	}
	if rows[0].ReturnA == nil || rows[0].ReturnB == nil {
		t.Error("2024 should have both A and B returns")
	}
}

func TestMergeYearlyReturns_Nil(t *testing.T) {
	rows := mergeYearlyReturns(nil)
	if rows != nil {
		t.Error("expected nil for nil result")
	}
}

func TestSerializeSectorDriftChart(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{Name: "Portfolio A"},
		PortfolioB: &comparison.PortfolioComparison{Name: "Portfolio B"},
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			Overlap: &comparison.OverlapResult{
				SectorAllocationA: &comparison.SectorAllocationResult{
					Breakdown: map[string]float64{
						"Technology": 0.35,
						"Healthcare": 0.20,
						"Finance":    0.15,
						"Energy":     0.10,
						"Unknown":    0.20,
					},
				},
				SectorAllocationB: &comparison.SectorAllocationResult{
					Breakdown: map[string]float64{
						"Technology": 0.25,
						"Healthcare": 0.25,
						"Finance":    0.20,
						"Energy":     0.10,
						"Consumer":   0.20,
					},
				},
			},
		},
	}

	jsonStr := serializeSectorDriftChart(result)
	var data driftChartData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Union: Technology, Healthcare, Finance, Energy, Unknown (from A), Consumer (from B) = 6 categories
	// Sorted by abs diff desc: Unknown(20pp), Consumer(20pp), Technology(10pp), Finance(5pp), Healthcare(5pp), Energy(0pp)
	if len(data.Categories) != 6 {
		t.Fatalf("expected 6 categories, got %d", len(data.Categories))
	}
	if data.NameA != "Portfolio A" {
		t.Errorf("name_a: got %q, want Portfolio A", data.NameA)
	}
	if data.NameB != "Portfolio B" {
		t.Errorf("name_b: got %q, want Portfolio B", data.NameB)
	}

	// Technology: A=35%, B=25% → drift = +10
	found := false
	for i, cat := range data.Categories {
		if cat == "Technology" {
			if data.Values[i] != 10.0 {
				t.Errorf("Technology drift: got %.2f, want 10.0", data.Values[i])
			}
			found = true
		}
	}
	if !found {
		t.Error("Technology not found in categories")
	}

	// Consumer: A=0%, B=20% → drift = -20
	found = false
	for i, cat := range data.Categories {
		if cat == "Consumer" {
			if data.Values[i] != -20.0 {
				t.Errorf("Consumer drift: got %.2f, want -20.0", data.Values[i])
			}
			found = true
		}
	}
	if !found {
		t.Error("Consumer not found in categories")
	}
}

func TestSerializeSectorDriftChart_NilResult(t *testing.T) {
	jsonStr := serializeSectorDriftChart(nil)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestSerializeSectorDriftChart_NoOverlap(t *testing.T) {
	result := &comparison.ComparisonResult{
		CrossMetrics: &comparison.CrossPortfolioMetrics{},
	}
	jsonStr := serializeSectorDriftChart(result)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestSerializeSectorDriftChart_MissingSectorData(t *testing.T) {
	result := &comparison.ComparisonResult{
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			Overlap: &comparison.OverlapResult{
				SectorAllocationA: &comparison.SectorAllocationResult{},
				// SectorAllocationB is nil
			},
		},
	}
	jsonStr := serializeSectorDriftChart(result)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestSerializeCountryDriftChart(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{Name: "A"},
		PortfolioB: &comparison.PortfolioComparison{Name: "B"},
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			Overlap: &comparison.OverlapResult{
				CountryAllocationA: &comparison.CountryAllocationResult{
					Breakdown: map[string]float64{
						"United States": 0.50,
						"Germany":       0.20,
						"UK":            0.10,
						"Japan":         0.20,
					},
				},
				CountryAllocationB: &comparison.CountryAllocationResult{
					Breakdown: map[string]float64{
						"United States": 0.40,
						"Germany":       0.30,
						"France":        0.20,
						"Japan":         0.10,
					},
				},
			},
		},
	}

	jsonStr := serializeCountryDriftChart(result)
	var data driftChartData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(data.Categories) != 5 {
		t.Fatalf("expected 5 categories, got %d", len(data.Categories))
	}

	// United States: A=50%, B=40% → +10
	// Germany: A=20%, B=30% → -10
	// UK: A=10%, B=0% → +10
	// Japan: A=20%, B=10% → +10
	// France: A=0%, B=20% → -20
	for i, cat := range data.Categories {
		switch cat {
		case "United States":
			if data.Values[i] != 10.0 {
				t.Errorf("US drift: got %.2f, want 10.0", data.Values[i])
			}
		case "Germany":
			if data.Values[i] != -10.0 {
				t.Errorf("Germany drift: got %.2f, want -10.0", data.Values[i])
			}
		case "UK":
			if data.Values[i] != 10.0 {
				t.Errorf("UK drift: got %.2f, want 10.0", data.Values[i])
			}
		case "Japan":
			if data.Values[i] != 10.0 {
				t.Errorf("Japan drift: got %.2f, want 10.0", data.Values[i])
			}
		case "France":
			if data.Values[i] != -20.0 {
				t.Errorf("France drift: got %.2f, want -20.0", data.Values[i])
			}
		}
	}
}

func TestSerializeCountryDriftChart_NilResult(t *testing.T) {
	jsonStr := serializeCountryDriftChart(nil)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestSerializeCountryDriftChart_TruncatesToTop15(t *testing.T) {
	// Build 20 countries — more than the countryDriftLimit of 15.
	breakdownA := make(map[string]float64)
	breakdownB := make(map[string]float64)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("Country_%02d", i)
		// Each country has a unique difference: Country_00 has diff 19pp, Country_19 has diff 0pp.
		breakdownA[name] = float64(19-i) / 100.0
		breakdownB[name] = 0.0
	}
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{Name: "A"},
		PortfolioB: &comparison.PortfolioComparison{Name: "B"},
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			Overlap: &comparison.OverlapResult{
				CountryAllocationA: &comparison.CountryAllocationResult{Breakdown: breakdownA},
				CountryAllocationB: &comparison.CountryAllocationResult{Breakdown: breakdownB},
			},
		},
	}

	jsonStr := serializeCountryDriftChart(result)
	var data driftChartData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(data.Categories) != 15 {
		t.Errorf("expected 15 categories (truncated), got %d", len(data.Categories))
	}
	if data.Note == "" {
		t.Error("expected truncation note, got empty string")
	}
	// Top country should be Country_00 (diff 19pp).
	if data.Categories[0] != "Country_00" {
		t.Errorf("first category: got %q, want Country_00", data.Categories[0])
	}
}

func TestSerializeMergedHoldings(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{Name: "Portfolio A"},
		PortfolioB: &comparison.PortfolioComparison{Name: "Portfolio B"},
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			Overlap: &comparison.OverlapResult{
				MergedHoldings: []comparison.MergedHolding{
					{Symbol: "AAPL", Name: "Apple Inc", WeightA: decimal.MustParse("0.05"), WeightB: decimal.MustParse("0.04"), OverlapPct: 4.0},
					{Symbol: "MSFT", Name: "Microsoft", WeightA: decimal.MustParse("0.03"), WeightB: decimal.MustParse("0.03"), OverlapPct: 3.0},
					{Symbol: "GOOG", Name: "Alphabet", WeightA: decimal.MustParse("0.02"), WeightB: decimal.MustParse("0.00"), OverlapPct: 0},
				},
			},
		},
	}

	jsonStr := serializeMergedHoldings(result)
	var data mergedHoldingsData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(data.Holdings) != 3 {
		t.Fatalf("expected 3 holdings, got %d", len(data.Holdings))
	}

	// AAPL: weightA=5%, weightB=4%, overlap=4%
	if data.Holdings[0].Symbol != "AAPL" {
		t.Errorf("first holding symbol: got %q, want AAPL", data.Holdings[0].Symbol)
	}
	if data.Holdings[0].WeightAPct != 5.0 {
		t.Errorf("AAPL weight_a_pct: got %.2f, want 5.0", data.Holdings[0].WeightAPct)
	}
	if data.Holdings[0].WeightBPct != 4.0 {
		t.Errorf("AAPL weight_b_pct: got %.2f, want 4.0", data.Holdings[0].WeightBPct)
	}
	if data.Holdings[0].OverlapPct != 4.0 {
		t.Errorf("AAPL overlap_pct: got %.2f, want 4.0", data.Holdings[0].OverlapPct)
	}

	// GOOG: weightA=2%, weightB=0%, overlap=0
	if data.Holdings[2].Symbol != "GOOG" {
		t.Errorf("third holding symbol: got %q, want GOOG", data.Holdings[2].Symbol)
	}
	if data.Holdings[2].WeightBPct != 0.0 {
		t.Errorf("GOOG weight_b_pct: got %.2f, want 0.0", data.Holdings[2].WeightBPct)
	}
}

func TestSerializeMergedHoldings_NilResult(t *testing.T) {
	jsonStr := serializeMergedHoldings(nil)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestSerializeMergedHoldings_EmptyHoldings(t *testing.T) {
	result := &comparison.ComparisonResult{
		CrossMetrics: &comparison.CrossPortfolioMetrics{
			Overlap: &comparison.OverlapResult{
				MergedHoldings: []comparison.MergedHolding{},
			},
		},
	}
	jsonStr := serializeMergedHoldings(result)
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}

func TestComputeAllocationDrift_SortedByAbsDiff(t *testing.T) {
	breakdownA := map[string]float64{
		"X": 0.40,
		"Y": 0.30,
		"Z": 0.30,
	}
	breakdownB := map[string]float64{
		"X": 0.20,
		"Y": 0.25,
		"W": 0.55,
	}

	drift := computeAllocationDrift(breakdownA, breakdownB)

	// X: 40-20=+20, Y: 30-25=+5, Z: 30-0=+30, W: 0-55=-55
	// Sorted by abs desc: W(55), Z(30), X(20), Y(5)
	if len(drift.categories) != 4 {
		t.Fatalf("expected 4 categories, got %d", len(drift.categories))
	}
	if drift.categories[0] != "W" {
		t.Errorf("first category: got %q, want W (abs diff 55)", drift.categories[0])
	}
	if drift.values[0] != -55.0 {
		t.Errorf("W value: got %.2f, want -55.0", drift.values[0])
	}
	if drift.categories[1] != "Z" {
		t.Errorf("second category: got %q, want Z (abs diff 30)", drift.categories[1])
	}
}

func TestComputeAllocationDrift_EmptyBreakdowns(t *testing.T) {
	drift := computeAllocationDrift(map[string]float64{}, map[string]float64{})
	if len(drift.categories) != 0 {
		t.Errorf("expected 0 categories, got %d", len(drift.categories))
	}
}

func TestRoundTo2_PositiveValues(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0.125, 0.13},
		{0.124, 0.12},
		{5.995, 6.0},
		{100.0, 100.0},
	}
	for _, tc := range tests {
		got := roundTo2(tc.input)
		if got != tc.want {
			t.Errorf("roundTo2(%.4f) = %.2f, want %.2f", tc.input, got, tc.want)
		}
	}
}

func TestRoundTo2_NegativeValues(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{-0.125, -0.13},
		{-0.124, -0.12},
		{-5.995, -6.0},
		{-100.0, -100.0},
		{-54.995, -55.0}, // Go's int() truncates toward zero; roundTo2 must handle this
	}
	for _, tc := range tests {
		got := roundTo2(tc.input)
		if got != tc.want {
			t.Errorf("roundTo2(%.4f) = %.2f, want %.2f", tc.input, got, tc.want)
		}
	}
}

func TestBuildComparisonPeriodURLs(t *testing.T) {
	filter := comparisonFilter{
		PortfolioAID:   1,
		PortfolioAType: "model",
		PortfolioBID:   2,
		PortfolioBType: "real",
		BaseCurrency:   "EUR",
	}
	urls := buildComparisonPeriodURLs(filter, "1Y")
	if len(urls) != 8 {
		t.Errorf("expected 8 period URLs, got %d", len(urls))
	}
	if !strings.Contains(urls["1Y"], "portfolio_a_id=m1") {
		t.Errorf("expected portfolio_a_id=m1 in URL, got: %s", urls["1Y"])
	}
	if !strings.Contains(urls["1Y"], "portfolio_b_id=r2") {
		t.Errorf("expected portfolio_b_id=r2 in URL, got: %s", urls["1Y"])
	}
	if !strings.Contains(urls["3Y"], "period=3Y") {
		t.Error("expected period=3Y in 3Y URL")
	}
	if !strings.Contains(urls["1Y"], "period=1Y") {
		t.Error("expected period=1Y in 1Y URL (all period URLs include the param)")
	}
}

func TestBuildComparisonPeriodURLs_Strategy(t *testing.T) {
	t.Run("rebalanced adds strategy params to every period URL", func(t *testing.T) {
		filter := comparisonFilter{
			PortfolioAID: 1, PortfolioAType: "model", PortfolioBID: 2, PortfolioBType: "model",
			Strategy: "rebalanced", RebalanceInterval: "annual", RebalanceThresholdPct: "5",
		}
		urls := buildComparisonPeriodURLs(filter, "0")
		for p, u := range urls {
			if !strings.Contains(u, "strategy=rebalanced") {
				t.Errorf("period %s: missing strategy=rebalanced in %s", p, u)
			}
			if !strings.Contains(u, "rebalance_interval=annual") {
				t.Errorf("period %s: missing rebalance_interval=annual in %s", p, u)
			}
			if !strings.Contains(u, "rebalance_threshold_pct=5") {
				t.Errorf("period %s: missing rebalance_threshold_pct=5 in %s", p, u)
			}
		}
	})
	t.Run("buy_and_hold omits strategy params", func(t *testing.T) {
		filter := comparisonFilter{PortfolioAID: 1, PortfolioAType: "model", PortfolioBID: 2, PortfolioBType: "model", Strategy: "buy_and_hold"}
		urls := buildComparisonPeriodURLs(filter, "0")
		for p, u := range urls {
			if strings.Contains(u, "strategy=") || strings.Contains(u, "rebalance_interval=") || strings.Contains(u, "rebalance_threshold_pct=") {
				t.Errorf("period %s: unexpected strategy params in %s", p, u)
			}
		}
	})
	t.Run("asymmetric adds stressed params to every period URL", func(t *testing.T) {
		filter := comparisonFilter{
			PortfolioAID: 1, PortfolioAType: "model", PortfolioBID: 2, PortfolioBType: "model",
			Strategy: "rebalanced_asymmetric", StressedAID: 7, StressedBID: 9,
			DrawdownTriggerPct: "15", RebalanceInterval: "monthly", RebalanceThresholdPct: "2",
		}
		urls := buildComparisonPeriodURLs(filter, "0")
		for p, u := range urls {
			if !strings.Contains(u, "strategy=rebalanced_asymmetric") {
				t.Errorf("period %s: missing strategy in %s", p, u)
			}
			if !strings.Contains(u, "stressed_a_id=7") {
				t.Errorf("period %s: missing stressed_a_id=7 in %s", p, u)
			}
			if !strings.Contains(u, "stressed_b_id=9") {
				t.Errorf("period %s: missing stressed_b_id=9 in %s", p, u)
			}
			if !strings.Contains(u, "drawdown_trigger_pct=15") {
				t.Errorf("period %s: missing drawdown_trigger_pct=15 in %s", p, u)
			}
			if !strings.Contains(u, "rebalance_interval=monthly") {
				t.Errorf("period %s: missing rebalance_interval=monthly in %s", p, u)
			}
			if !strings.Contains(u, "rebalance_threshold_pct=2") {
				t.Errorf("period %s: missing rebalance_threshold_pct=2 in %s", p, u)
			}
		}
	})
}

// --- Helpers ---

// mockComparisonModelPortfolioSelector implements comparisonModelPortfolioSelector.
type mockComparisonModelPortfolioSelector struct {
	summaries []modelportfolio.ModelPortfolioSummary
	portfolio modelportfolio.ModelPortfolio
	err       error
}

func (m *mockComparisonModelPortfolioSelector) GetAllForSelector(_ context.Context) ([]modelportfolio.ModelPortfolioSummary, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.summaries, nil
}

func (m *mockComparisonModelPortfolioSelector) Get(_ context.Context, _ int64) (modelportfolio.ModelPortfolio, error) {
	if m.err != nil {
		return modelportfolio.ModelPortfolio{}, m.err
	}
	return m.portfolio, nil
}

func checkComparisonContains(t *testing.T, body, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("expected body to contain %q", substr)
	}
}

// Ensure types compile.
var (
	_ portfolio.Portfolio
	_ modelportfolio.ModelPortfolioSummary
	_ web.Renderer
)
