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
	urls := buildComparisonPeriodURLs(filter, "1Y", "0")
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
