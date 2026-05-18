package analysis

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func TestAnalysisResultJSONRoundTrip(t *testing.T) {
	computedAt := time.Date(2025, 5, 15, 10, 30, 0, 0, time.UTC)
	overlap := &OverlapResult{
		PairwiseMatrix: []OverlapPair{
			{ETFA: "VTI", ETFB: "VXUS", OverlappingCount: 3, CombinedWeightPct: 5.2},
		},
		TopConcentratedStocks: []ConcentratedStock{
			{Symbol: "AAPL", Name: "Apple Inc.", TotalWeightPct: 4.1, HeldByETFs: []string{"VTI", "VXUS"}},
		},
	}
	correlation := &CorrelationResult{
		Matrix:  ptrMatrix([][]float64{{1.0, 0.85}, {0.85, 1.0}}),
		Symbols: []string{"VTI", "VXUS"},
		Period:  "1Y",
	}
	sectorAlloc := &AllocationResult{
		Breakdown:       map[string]float64{"technology": 30.5, "financials": 15.2},
		UnknownWeightPct: 2.1,
	}
	stress := &StressTestResult{
		Scenarios: []StressScenarioResult{
			{
				Name:               "2008 GFC",
				DateRange:          "2007-10-09 to 2009-03-09",
				EstimatedReturnPct: -35.2,
			},
		},
	}
	factor := &FactorExposureResult{
		ValueGrowthTilt: FactorValueGrowth{
			WeightedPE: 18.5,
			WeightedPB: 3.2,
			Tilt:       "growth",
		},
		Concentration: FactorConcentration{
			HHI:          0.025,
			Interpretation: "moderately-concentrated",
		},
		TopHoldingWeightPct: 5.8,
	}

	result := AnalysisResult{
		PortfolioID:        1,
		ComputedAt:         computedAt,
		Overlap:            overlap,
		Correlation:        correlation,
		SectorAllocation:   sectorAlloc,
		GeographicAllocation: nil, // explicitly nil — should serialize as omitted
		StressTest:         stress,
		FactorExposure:     factor,
		Warnings:           []string{"some data may be stale"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AnalysisResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.PortfolioID != result.PortfolioID {
		t.Errorf("portfolio_id: got %d, want %d", got.PortfolioID, result.PortfolioID)
	}
	if !got.ComputedAt.Equal(result.ComputedAt) {
		t.Errorf("computed_at: got %v, want %v", got.ComputedAt, result.ComputedAt)
	}
	if got.Overlap == nil {
		t.Error("overlap should not be nil")
	} else if len(got.Overlap.PairwiseMatrix) != 1 {
		t.Errorf("pairwise_matrix len: got %d, want 1", len(got.Overlap.PairwiseMatrix))
	}
	if got.GeographicAllocation != nil {
		t.Error("geographic_allocation should be nil (omitted)")
	}
	if got.StressTest == nil || len(got.StressTest.Scenarios) != 1 {
		t.Error("stress_test scenarios missing")
	}
	if got.Warnings[0] != "some data may be stale" {
		t.Errorf("warnings: got %v", got.Warnings)
	}
}

func TestAnalysisResultEmptyState(t *testing.T) {
	result := AnalysisResult{
		PortfolioID: 1,
		ComputedAt:  time.Now().UTC(),
		Message:     "no open positions to analyze",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AnalysisResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Message != "no open positions to analyze" {
		t.Errorf("message: got %q, want %q", got.Message, "no open positions to analyze")
	}
	if got.Overlap != nil {
		t.Error("overlap should be nil in empty state")
	}
	if got.Correlation != nil {
		t.Error("correlation should be nil in empty state")
	}
	if got.SectorAllocation != nil {
		t.Error("sector_allocation should be nil in empty state")
	}
}

func TestStressScenarioResultDollarImpactRoundTrip(t *testing.T) {
	impact := decimal.MustParse("-35200.50")
	scenario := StressScenarioResult{
		Name:                "2008 GFC",
		DateRange:           "2007-10-09 to 2009-03-09",
		EstimatedReturnPct:  -35.2,
		EstimatedDollarImpact: impact,
		SectorContributions: map[string]float64{
			"financials": -15.3,
			"technology": -8.7,
		},
	}

	data, err := json.Marshal(scenario)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got StressScenarioResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !got.EstimatedDollarImpact.Equal(impact) {
		t.Errorf("dollar impact: got %v, want %v", got.EstimatedDollarImpact, impact)
	}
	if got.EstimatedReturnPct != -35.2 {
		t.Errorf("return pct: got %v, want -35.2", got.EstimatedReturnPct)
	}
	if len(got.SectorContributions) != 2 {
		t.Errorf("sector contributions len: got %d, want 2", len(got.SectorContributions))
	}
}

func TestCorrelationResultNilMatrix(t *testing.T) {
	result := CorrelationResult{
		Symbols: []string{"VTI"},
		Period:  "1Y",
		Message: "insufficient data for correlation",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got CorrelationResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Matrix != nil {
		t.Error("matrix should be nil (omitted) when insufficient data")
	}
	if got.Message != "insufficient data for correlation" {
		t.Errorf("message: got %q", got.Message)
	}
}

func TestAllocationResultEmptyBreakdown(t *testing.T) {
	result := AllocationResult{
		Breakdown:       map[string]float64{},
		UnknownWeightPct: 100.0,
		Message:         "no allocation data available",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AllocationResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.UnknownWeightPct != 100.0 {
		t.Errorf("unknown_weight_pct: got %v, want 100", got.UnknownWeightPct)
	}
}

func TestFactorExposureResultJSON(t *testing.T) {
	result := FactorExposureResult{
		ValueGrowthTilt: FactorValueGrowth{
			WeightedPE: 22.3,
			WeightedPB: 4.1,
			Tilt:       "growth",
		},
		SizeTilt: FactorSizeTilt{
			LargeCapPct: 75.0,
			MidCapPct:   20.0,
			SmallCapPct: 5.0,
			Tilt:        "large",
		},
		Concentration: FactorConcentration{
			HHI:          0.015,
			Interpretation: "well-diversified",
		},
		TopHoldingWeightPct: 3.5,
		Warnings:            []string{"valuation data missing for 2 positions"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got FactorExposureResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ValueGrowthTilt.Tilt != "growth" {
		t.Errorf("vg tilt: got %q, want %q", got.ValueGrowthTilt.Tilt, "growth")
	}
	if got.SizeTilt.Tilt != "large" {
		t.Errorf("size tilt: got %q, want %q", got.SizeTilt.Tilt, "large")
	}
	if got.Concentration.Interpretation != "well-diversified" {
		t.Errorf("concentration: got %q", got.Concentration.Interpretation)
	}
	if len(got.Warnings) != 1 {
		t.Errorf("warnings: got %d, want 1", len(got.Warnings))
	}
}

func TestOverlapResultWithWarnings(t *testing.T) {
	result := OverlapResult{
		PairwiseMatrix: []OverlapPair{
			{ETFA: "VTI", ETFB: "VXUS", OverlappingCount: 5, CombinedWeightPct: 7.3},
		},
		Warnings: []string{
			"holdings data missing for 1 ETF",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got OverlapResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Warnings) != 1 {
		t.Errorf("warnings: got %d, want 1", len(got.Warnings))
	}
	if got.PairwiseMatrix[0].OverlappingCount != 5 {
		t.Errorf("overlapping_count: got %d, want 5", got.PairwiseMatrix[0].OverlappingCount)
	}
}

func TestAnalysisFilters(t *testing.T) {
	portfolioID := int64(42)
	filters := AnalysisFilters{
		PortfolioID: &portfolioID,
		Section:     "overlap",
		Period:      "3Y",
	}

	if *filters.PortfolioID != 42 {
		t.Errorf("portfolio_id: got %d, want 42", *filters.PortfolioID)
	}
	if filters.Section != "overlap" {
		t.Errorf("section: got %q, want %q", filters.Section, "overlap")
	}
	if filters.Period != "3Y" {
		t.Errorf("period: got %q, want %q", filters.Period, "3Y")
	}

	// Nil portfolio ID (all portfolios)
	filters2 := AnalysisFilters{}
	if filters2.PortfolioID != nil {
		t.Error("portfolio_id should be nil when not set")
	}
	if filters2.Section != "" {
		t.Error("section should be empty when not set")
	}
}

func TestAnalysisSectionConstants(t *testing.T) {
	sections := []AnalysisSection{
		SectionOverlap,
		SectionCorrelation,
		SectionSectorAllocation,
		SectionGeographicAllocation,
		SectionStressTest,
		SectionFactorExposure,
	}

	expected := []string{
		"overlap",
		"correlation",
		"sector_allocation",
		"geographic_allocation",
		"stress_test",
		"factor_exposure",
	}

	if len(sections) != len(expected) {
		t.Fatalf("sections count: got %d, want %d", len(sections), len(expected))
	}

	for i, want := range expected {
		if string(sections[i]) != want {
			t.Errorf("section[%d]: got %q, want %q", i, sections[i], want)
		}
	}
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
