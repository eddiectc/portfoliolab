package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/analysis"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

func TestRegisterRoutes_AnalysisWeb(t *testing.T) {
	r := chi.NewRouter()
	handler := &AnalysisWebHandler{}
	handler.RegisterRoutes(r)
	// Verify it doesn't panic
}

// mockAnalysisServiceForWeb implements analysisService for web handler tests.
type mockAnalysisServiceForWeb struct {
	result *analysis.AnalysisResult
	err    error
}

func (m *mockAnalysisServiceForWeb) ComputeAnalysis(_ context.Context, _ analysis.AnalysisFilters) (*analysis.AnalysisResult, error) {
	return m.result, m.err
}

// Test that the analysis page template renders without panic (empty state).
func TestAnalysisTemplate_EmptyState(t *testing.T) {
	renderer := newTestRenderer(t)

	data := analysisPageData{
		PageData:    web.PageData{Title: "Portfolio Analysis"},
		Portfolios:  []portfolio.Portfolio{},
		PeriodURLs:  map[string]string{"1Y": "/analysis"},
		SectionURLs: map[string]string{"all": "/analysis"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "analysis/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(body, "Portfolio Analysis") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "No analysis data available") {
		t.Error("expected empty state message")
	}
}

// Test that the analysis page renders with all sections populated.
func TestAnalysisTemplate_FullResult(t *testing.T) {
	renderer := newTestRenderer(t)

	result := &analysis.AnalysisResult{
		Overlap: &analysis.OverlapResult{
			PairwiseMatrix: []analysis.OverlapPair{
				{ETFA: "ETF1", ETFB: "ETF2", OverlappingCount: 5, CombinedWeightPct: 12.5},
			},
			TopConcentratedStocks: []analysis.ConcentratedStock{
				{Symbol: "AAPL", Name: "Apple Inc.", TotalWeightPct: 8.3, HeldByETFs: []string{"ETF1", "ETF2"}},
			},
		},
		Correlation: &analysis.CorrelationResult{
			Matrix: [][]float64{
				{1.0, 0.8},
				{0.8, 1.0},
			},
			Symbols: []string{"AAPL", "MSFT"},
			Period:  "1Y",
		},
		SectorAllocation: &analysis.AllocationResult{
			Breakdown: map[string]float64{
				"Technology": 45.0,
				"Finance":    25.0,
				"Healthcare": 20.0,
			},
		},
		GeographicAllocation: &analysis.AllocationResult{
			Breakdown: map[string]float64{
				"United States": 60.0,
				"Japan":         15.0,
				"Germany":       10.0,
			},
		},
		StressTest: &analysis.StressTestResult{
			Scenarios: []analysis.StressScenarioResult{
				{
					Name:                "2008 GFC",
					DateRange:           "2007-10 to 2009-03",
					EstimatedReturnPct:  -35.2,
					EstimatedDollarImpact: decimal.MustParse("-35200.00"),
				},
			},
		},
		FactorExposure: &analysis.FactorExposureResult{
			ValueGrowthTilt: analysis.FactorValueGrowth{WeightedPE: 18.5, WeightedPB: 3.2, Tilt: "value"},
			SizeTilt:        analysis.FactorSizeTilt{LargeCapPct: 70.0, MidCapPct: 25.0, SmallCapPct: 5.0, Tilt: "large"},
			Concentration:   analysis.FactorConcentration{HHI: 0.015, Interpretation: "well-diversified"},
			TopHoldingWeightPct: 8.5,
			Quality:           analysis.FactorQuality{WeightedPCF: 8.5, WeightedPS: 2.1, Tilt: "high-quality"},
			Cost:              analysis.FactorCost{WeightedExpenseRatio: 0.45, WeightedTurnover: 22.0},
			Momentum:          analysis.FactorMomentum{Return3M: 5.2, Return6M: 8.1, Return12M: 12.3, Tilt: "positive"},
			Volatility:        analysis.FactorVolatility{AnnualizedVol: 14.5, Tilt: "medium"},
		},
	}

	correlationData := serializeCorrelationData(result)
	sectorData := serializeAllocationChartData(result.SectorAllocation)
	geoData := serializeAllocationChartData(result.GeographicAllocation)

	data := analysisPageData{
		PageData:            web.PageData{Title: "Portfolio Analysis"},
		Result:              result,
		CorrelationChartData: correlationData,
		SectorChartData:      sectorData,
		GeographicChartData:  geoData,
		Portfolios:           []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolioID:  "1",
		SelectedPeriod:       "1Y",
		PeriodURLs:           map[string]string{"1Y": "/analysis?portfolio_id=1"},
		SectionURLs:          map[string]string{"all": "/analysis?portfolio_id=1"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "analysis/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	// Overlap section
	checkContains("overlap header", "ETF Overlap")
	checkContains("pairwise table", "Pairwise Overlap")
	checkContains("overlap ETF A", "ETF1")
	checkContains("overlap ETF B", "ETF2")
	checkContains("overlap count", "5")
	checkContains("concentrated stocks", "Top Concentrated Stocks")
	checkContains("concentrated symbol", "AAPL")
	checkContains("held by", "ETF1, ETF2")

	// Correlation section
	checkContains("correlation header", "Correlation Matrix")
	checkContains("correlation period", "1Y lookback")
	checkContains("correlation chart div", `id="correlation-chart"`)

	// Sector section
	checkContains("sector header", "Sector Allocation")
	checkContains("sector chart div", `id="sector-chart"`)
	checkContains("sector Technology", "Technology")
	checkContains("sector Finance", "Finance")

	// Geographic section
	checkContains("geo header", "Geographic Allocation")
	checkContains("geo chart div", `id="geographic-chart"`)
	checkContains("geo United States", "United States")

	// Stress test section
	checkContains("stress header", "Stress Testing")
	checkContains("stress scenario", "2008 GFC")
	checkContains("stress return", "-35.2%")

	// Factor exposure section
	checkContains("factor header", "Factor Exposure")
	checkContains("value/growth tilt", "value")
	checkContains("size tilt", "large")
	checkContains("concentration", "well-diversified")
	checkContains("quality tilt", "high-quality")
	checkContains("momentum tilt", "positive")
	checkContains("volatility tilt", "medium")

	// ECharts script
	checkContains("echarts script", "echarts.min.js")
}

// Test that the analysis page renders with error state.
func TestAnalysisTemplate_ErrorState(t *testing.T) {
	renderer := newTestRenderer(t)

	data := analysisPageData{
		PageData:    web.PageData{Title: "Portfolio Analysis"},
		Portfolios:  []portfolio.Portfolio{},
		Error:       "An error occurred while computing analysis data.",
		PeriodURLs:  map[string]string{"1Y": "/analysis"},
		SectionURLs: map[string]string{"all": "/analysis"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "analysis/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "An error occurred") {
		t.Error("expected error message in page")
	}
}

// Test that section filtering shows only the selected section.
func TestAnalysisTemplate_SectionFilter(t *testing.T) {
	renderer := newTestRenderer(t)

	result := &analysis.AnalysisResult{
		Overlap: &analysis.OverlapResult{
			Message: "Need 2+ ETFs for pairwise overlap analysis.",
		},
		Correlation: &analysis.CorrelationResult{
			Matrix:  [][]float64{{1.0}},
			Symbols: []string{"AAPL"},
			Period:  "1Y",
		},
	}

	correlationData := serializeCorrelationData(result)

	data := analysisPageData{
		PageData:           web.PageData{Title: "Portfolio Analysis"},
		Result:             result,
		CorrelationChartData: correlationData,
		SelectedSection:    "correlation",
		PeriodURLs:         map[string]string{"1Y": "/analysis"},
		SectionURLs:        map[string]string{"all": "/analysis", "correlation": "/analysis?section=correlation"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "analysis/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	// Correlation section should be shown
	if !strings.Contains(body, "Correlation Matrix") {
		t.Error("expected correlation section when selected")
	}
	// Overlap section should NOT be shown when correlation is selected
	if strings.Contains(body, "ETF Overlap") {
		t.Error("should not show overlap section when correlation is selected")
	}
}

// Test that warnings are displayed.
func TestAnalysisTemplate_WithWarnings(t *testing.T) {
	renderer := newTestRenderer(t)

	result := &analysis.AnalysisResult{
		Warnings: []string{
			"symbol details source not configured",
			"historical price data source not configured",
		},
		Overlap: &analysis.OverlapResult{
			Message: "No ETF positions found.",
		},
	}

	data := analysisPageData{
		PageData:    web.PageData{Title: "Portfolio Analysis"},
		Result:      result,
		PeriodURLs:  map[string]string{"1Y": "/analysis"},
		SectionURLs: map[string]string{"all": "/analysis"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "analysis/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "symbol details source not configured") {
		t.Error("expected warning in page")
	}
}

// Test that empty state message from result is displayed.
func TestAnalysisTemplate_ResultMessage(t *testing.T) {
	renderer := newTestRenderer(t)

	result := &analysis.AnalysisResult{
		Message: "No positions to analyze. Add transactions to your portfolio.",
	}

	data := analysisPageData{
		PageData:    web.PageData{Title: "Portfolio Analysis"},
		Result:      result,
		PeriodURLs:  map[string]string{"1Y": "/analysis"},
		SectionURLs: map[string]string{"all": "/analysis"},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "analysis/index", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "No positions to analyze") {
		t.Error("expected result message in page")
	}
}

// Test serializeCorrelationData with various inputs.
func TestSerializeCorrelationData(t *testing.T) {
	tests := []struct {
		name string
		result *analysis.AnalysisResult
		wantEmpty bool
	}{
		{
			name:      "nil result",
			result:    nil,
			wantEmpty: true,
		},
		{
			name:      "nil correlation",
			result:    &analysis.AnalysisResult{},
			wantEmpty: true,
		},
		{
			name: "empty matrix",
			result: &analysis.AnalysisResult{
				Correlation: &analysis.CorrelationResult{
					Matrix:  [][]float64{},
					Symbols: []string{},
				},
			},
			wantEmpty: true,
		},
		{
			name: "single symbol",
			result: &analysis.AnalysisResult{
				Correlation: &analysis.CorrelationResult{
					Matrix:  [][]float64{{1.0}},
					Symbols: []string{"AAPL"},
					Period:  "1Y",
				},
			},
			wantEmpty: false,
		},
		{
			name: "two symbols",
			result: &analysis.AnalysisResult{
				Correlation: &analysis.CorrelationResult{
					Matrix: [][]float64{
						{1.0, 0.85},
						{0.85, 1.0},
					},
					Symbols: []string{"AAPL", "MSFT"},
					Period:  "3Y",
				},
			},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serializeCorrelationData(tt.result)
			if tt.wantEmpty {
				if got != "{}" {
					t.Errorf("serializeCorrelationData() = %q, want {}", got)
				}
			} else {
				if got == "{}" {
					t.Error("serializeCorrelationData() returned empty, expected data")
				} else {
					// Verify it's valid JSON
					var m map[string]interface{}
					if err := json.Unmarshal([]byte(got), &m); err != nil {
						t.Errorf("serializeCorrelationData() produced invalid JSON: %v", err)
					}
				}
			}
		})
	}
}

// Test serializeAllocationChartData with various inputs.
func TestSerializeAllocationChartData(t *testing.T) {
	tests := []struct {
		name string
		result *analysis.AllocationResult
		wantEmpty bool
	}{
		{
			name:      "nil result",
			result:    nil,
			wantEmpty: true,
		},
		{
			name:      "empty breakdown",
			result:    &analysis.AllocationResult{},
			wantEmpty: true,
		},
		{
			name: "single sector",
			result: &analysis.AllocationResult{
				Breakdown: map[string]float64{"Technology": 100.0},
			},
			wantEmpty: false,
		},
		{
			name: "multiple sectors",
			result: &analysis.AllocationResult{
				Breakdown: map[string]float64{
					"Technology": 45.0,
					"Finance":    25.0,
					"Healthcare": 20.0,
				},
			},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serializeAllocationChartData(tt.result)
			if tt.wantEmpty {
				if got != "{}" {
					t.Errorf("serializeAllocationChartData() = %q, want {}", got)
				}
			} else {
				if got == "{}" {
					t.Error("serializeAllocationChartData() returned empty, expected data")
				} else {
					var m map[string]interface{}
					if err := json.Unmarshal([]byte(got), &m); err != nil {
						t.Errorf("serializeAllocationChartData() produced invalid JSON: %v", err)
					}
				}
			}
		})
	}
}

// Test buildAnalysisPeriodURLs.
func TestBuildAnalysisPeriodURLs(t *testing.T) {
	// No portfolio, no section.
	urls := buildAnalysisPeriodURLs("", "1Y", "")
	if urls["1Y"] != "/analysis" {
		t.Errorf("1Y = %q, want /analysis", urls["1Y"])
	}
	if urls["3Y"] != "/analysis?period=3Y" {
		t.Errorf("3Y = %q, want /analysis?period=3Y", urls["3Y"])
	}

	// With portfolio.
	urls2 := buildAnalysisPeriodURLs("5", "1Y", "")
	if urls2["1Y"] != "/analysis?portfolio_id=5" {
		t.Errorf("1Y = %q, want /analysis?portfolio_id=5", urls2["1Y"])
	}
	if urls2["5Y"] != "/analysis?portfolio_id=5&period=5Y" {
		t.Errorf("5Y = %q, want /analysis?portfolio_id=5&period=5Y", urls2["5Y"])
	}

	// With section.
	urls3 := buildAnalysisPeriodURLs("", "3Y", "correlation")
	if urls3["1Y"] != "/analysis?section=correlation" {
		t.Errorf("1Y = %q, want /analysis?section=correlation", urls3["1Y"])
	}
	if urls3["5Y"] != "/analysis?period=5Y&section=correlation" {
		t.Errorf("5Y = %q, want /analysis?period=5Y&section=correlation", urls3["5Y"])
	}
}

// Test buildAnalysisSectionURLs.
func TestBuildAnalysisSectionURLs(t *testing.T) {
	// No portfolio, default period.
	urls := buildAnalysisSectionURLs("", "1Y", "")
	if urls["all"] != "/analysis" {
		t.Errorf("all = %q, want /analysis", urls["all"])
	}
	if urls["overlap"] != "/analysis?section=overlap" {
		t.Errorf("overlap = %q, want /analysis?section=overlap", urls["overlap"])
	}
	if urls["correlation"] != "/analysis?section=correlation" {
		t.Errorf("correlation = %q, want /analysis?section=correlation", urls["correlation"])
	}

	// With portfolio and period.
	urls2 := buildAnalysisSectionURLs("5", "3Y", "sector_allocation")
	if urls2["all"] != "/analysis?portfolio_id=5&period=3Y" {
		t.Errorf("all = %q, want /analysis?portfolio_id=5&period=3Y", urls2["all"])
	}
	if urls2["stress"] != "/analysis?portfolio_id=5&period=3Y&section=stress_test" {
		t.Errorf("stress = %q, want /analysis?portfolio_id=5&period=3Y&section=stress_test", urls2["stress"])
	}
}
