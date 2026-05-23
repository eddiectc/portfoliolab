package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/comparison"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
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
				TWRPct:              &twr,
				AnnualizedTWRPct:    &annTwr,
				SimpleReturnPct:     &simpleRet,
				AnnualizedSimplePct: &annTwr,
				CAGRPct:             &cagr,
				DaysElapsed:         365,
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
				TWRPct:          &twr,
				SimpleReturnPct: &simpleRet,
				CAGRPct:         &cagr,
				DaysElapsed:     365,
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
	portfolios := handler.fetchPortfolios(nil)
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
	summaries := handler.fetchModelPortfolios(nil)
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

	if len(data.Years) != 2 {
		t.Errorf("expected 2 years, got %d", len(data.Years))
	}
	if data.NameA != "A" {
		t.Errorf("expected name_a=A, got %q", data.NameA)
	}
	if len(data.ReturnA) != 2 || len(data.ReturnB) != 2 {
		t.Errorf("expected 2 return values each, got A=%d B=%d", len(data.ReturnA), len(data.ReturnB))
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

func TestSerializeMonthlyHistogram(t *testing.T) {
	result := &comparison.ComparisonResult{
		PortfolioA: &comparison.PortfolioComparison{
			ReturnDistribution: &comparison.ReturnDistribution{
				Monthly: []comparison.ReturnBucket{
					{Label: "-5% to 0%", Count: 10},
					{Label: "0% to 5%", Count: 15},
				},
			},
		},
	}

	jsonStr := serializeMonthlyHistogram(result, "A")
	var data monthlyHistogramData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(data.Bins) != 2 {
		t.Errorf("expected 2 bins, got %d", len(data.Bins))
	}
}

func TestSerializeMonthlyHistogram_Nil(t *testing.T) {
	jsonStr := serializeMonthlyHistogram(nil, "A")
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
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Year != 2023 {
		t.Errorf("first row year: got %d, want 2023", rows[0].Year)
	}
	if rows[1].Year != 2024 {
		t.Errorf("second row year: got %d, want 2024", rows[1].Year)
	}
	if rows[1].ReturnA == nil || rows[1].ReturnB == nil {
		t.Error("2024 should have both A and B returns")
	}
}

func TestMergeYearlyReturns_Nil(t *testing.T) {
	rows := mergeYearlyReturns(nil)
	if rows != nil {
		t.Error("expected nil for nil result")
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

// --- Helpers ---

func checkComparisonContains(t *testing.T, body, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("expected body to contain %q", substr)
	}
}

// Ensure types compile.
var _ portfolio.Portfolio
var _ modelportfolio.ModelPortfolioSummary
var _ web.Renderer
