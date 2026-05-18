package analysis

import (
	"strings"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// --- ComputeFactorExposure tests ---

func TestComputeFactorExposure_HappyPath(t *testing.T) {
	// Two ETFs with known P/E, P/B, and market caps.
	// ETF1: 60% weight, P/E=18 (value), P/B=3 (value), large cap ($50B), 3 holdings
	// ETF2: 40% weight, P/E=25 (growth), P/B=5 (growth), large cap ($30B), 2 holdings
	positions := []PositionWithDetails{
		getETFWithValuation("ETF1", 60, 18, 3, 50_000_000_000, []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple", Percent: 5},
			{Symbol: "MSFT", Name: "Microsoft", Percent: 4},
			{Symbol: "GOOG", Name: "Alphabet", Percent: 3},
		}),
		getETFWithValuation("ETF2", 40, 25, 5, 30_000_000_000, []symbol.TopHolding{
			{Symbol: "TSLA", Name: "Tesla", Percent: 6},
			{Symbol: "NVDA", Name: "NVIDIA", Percent: 4},
		}),
	}

	result := ComputeFactorExposure(positions)

	// Weighted P/E: (60*18 + 40*25) / 100 = (1080 + 1000) / 100 = 20.8
	// Weighted P/B: (60*3 + 40*5) / 100 = (180 + 200) / 100 = 3.8
	if result.ValueGrowthTilt.WeightedPE != 20.8 {
		t.Errorf("weighted P/E = %.2f, want 20.80", result.ValueGrowthTilt.WeightedPE)
	}
	if result.ValueGrowthTilt.WeightedPB != 3.8 {
		t.Errorf("weighted P/B = %.2f, want 3.80", result.ValueGrowthTilt.WeightedPB)
	}

	// P/E 20.8 vs benchmark 20: within 15% (17-23) → neutral
	// P/B 3.8 vs benchmark 4: within 15% (3.4-4.6) → neutral
	// Both neutral → "neutral"
	if result.ValueGrowthTilt.Tilt != "neutral" {
		t.Errorf("tilt = %q, want neutral", result.ValueGrowthTilt.Tilt)
	}

	// Size: both large cap → 100% large
	if result.SizeTilt.LargeCapPct != 100.0 {
		t.Errorf("large cap pct = %.2f, want 100", result.SizeTilt.LargeCapPct)
	}
	if result.SizeTilt.Tilt != "large" {
		t.Errorf("size tilt = %q, want large", result.SizeTilt.Tilt)
	}

	// HHI: ETF1 holdings: 0.6*0.05=0.03, 0.6*0.04=0.024, 0.6*0.03=0.018
	// ETF2 holdings: 0.4*0.06=0.024, 0.4*0.04=0.016
	// HHI = 0.03² + 0.024² + 0.018² + 0.024² + 0.016²
	//     = 0.0009 + 0.000576 + 0.000324 + 0.000576 + 0.000256
	//     = 0.002632 → roundTo4 = 0.0026
	expectedHHI := roundTo4(0.03*0.03 + 0.024*0.024 + 0.018*0.018 + 0.024*0.024 + 0.016*0.016)
	if result.Concentration.HHI != expectedHHI {
		t.Errorf("HHI = %.4f, want %.4f", result.Concentration.HHI, expectedHHI)
	}
	if result.Concentration.Interpretation != "well-diversified" {
		t.Errorf("interpretation = %q, want well-diversified", result.Concentration.Interpretation)
	}

	// Top holding: ETF1 AAPL at 0.6*0.05 = 0.03 (3%)
	if result.TopHoldingWeightPct != 3.0 {
		t.Errorf("top holding pct = %.2f, want 3.00", result.TopHoldingWeightPct)
	}

	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if result.Message != "" {
		t.Errorf("unexpected message: %q", result.Message)
	}
}

func TestComputeFactorExposure_SingleETF(t *testing.T) {
	// Single ETF: 100% weight, P/E=15 (value), P/B=2.5 (value), large cap
	positions := []PositionWithDetails{
		getETFWithValuation("VTV", 100, 15, 2.5, 80_000_000_000, []symbol.TopHolding{
			{Symbol: "JPM", Name: "JPMorgan", Percent: 4},
			{Symbol: "BAC", Name: "Bank of America", Percent: 3},
		}),
	}

	result := ComputeFactorExposure(positions)

	// Weighted P/E: 15 (only one position)
	if result.ValueGrowthTilt.WeightedPE != 15.0 {
		t.Errorf("weighted P/E = %.2f, want 15", result.ValueGrowthTilt.WeightedPE)
	}
	// Weighted P/B: 2.5
	if result.ValueGrowthTilt.WeightedPB != 2.5 {
		t.Errorf("weighted P/B = %.2f, want 2.5", result.ValueGrowthTilt.WeightedPB)
	}

	// P/E 15 < 17 (20-3) → value
	// P/B 2.5 < 3.4 (4-0.6) → value
	// Both agree → "value"
	if result.ValueGrowthTilt.Tilt != "value" {
		t.Errorf("tilt = %q, want value", result.ValueGrowthTilt.Tilt)
	}

	// Size: 100% large cap
	if result.SizeTilt.LargeCapPct != 100.0 {
		t.Errorf("large cap pct = %.2f, want 100", result.SizeTilt.LargeCapPct)
	}
	if result.SizeTilt.Tilt != "large" {
		t.Errorf("size tilt = %q, want large", result.SizeTilt.Tilt)
	}

	// HHI: 0.04² + 0.03² = 0.0016 + 0.0009 = 0.0025
	expectedHHI := roundTo4(0.04*0.04 + 0.03*0.03)
	if result.Concentration.HHI != expectedHHI {
		t.Errorf("HHI = %.4f, want %.4f", result.Concentration.HHI, expectedHHI)
	}

	// Top holding: JPM at 0.04 (4%)
	if result.TopHoldingWeightPct != 4.0 {
		t.Errorf("top holding pct = %.2f, want 4.00", result.TopHoldingWeightPct)
	}
}

func TestComputeFactorExposure_MixedETFAndStock(t *testing.T) {
	// ETF with valuation data + stock without valuation data.
	// Stock contributes to HHI but not to P/E or P/B.
	positions := []PositionWithDetails{
		getETFWithValuation("VTI", 60, 22, 4.5, 300_000_000_000, []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple", Percent: 7},
			{Symbol: "MSFT", Name: "Microsoft", Percent: 6},
		}),
		getStockWithNoValuation("AAPL", 25),
		getStockWithNoValuation("JNJ", 15),
	}

	result := ComputeFactorExposure(positions)

	// P/E: only VTI contributes → 22
	if result.ValueGrowthTilt.WeightedPE != 22.0 {
		t.Errorf("weighted P/E = %.2f, want 22", result.ValueGrowthTilt.WeightedPE)
	}
	// P/B: only VTI contributes → 4.5
	if result.ValueGrowthTilt.WeightedPB != 4.5 {
		t.Errorf("weighted P/B = %.2f, want 4.5", result.ValueGrowthTilt.WeightedPB)
	}

	// P/E 22 vs benchmark 20: within 15% (17-23) → neutral
	// P/B 4.5 vs benchmark 4: within 15% (3.4-4.6) → neutral
	// Both neutral → "neutral"
	if result.ValueGrowthTilt.Tilt != "neutral" {
		t.Errorf("tilt = %q, want neutral", result.ValueGrowthTilt.Tilt)
	}

	// Size: only VTI has FundProfile → 60% large (of total portfolio, not tracked)
	if result.SizeTilt.LargeCapPct != 60.0 {
		t.Errorf("large cap pct = %.2f, want 60", result.SizeTilt.LargeCapPct)
	}

	// HHI: VTI holdings: 0.6*0.07=0.042, 0.6*0.06=0.036
	// Stocks: 0.25, 0.15
	// HHI = 0.042² + 0.036² + 0.25² + 0.15²
	//     = 0.001764 + 0.001296 + 0.0625 + 0.0225
	//     = 0.08806 → roundTo4 = 0.0881
	expectedHHI := roundTo4(0.042*0.042 + 0.036*0.036 + 0.25*0.25 + 0.15*0.15)
	if result.Concentration.HHI != expectedHHI {
		t.Errorf("HHI = %.4f, want %.4f", result.Concentration.HHI, expectedHHI)
	}
	if result.Concentration.Interpretation != "highly-concentrated" {
		t.Errorf("interpretation = %q, want highly-concentrated", result.Concentration.Interpretation)
	}

	// Top holding: AAPL stock at 0.25 (25%)
	if result.TopHoldingWeightPct != 25.0 {
		t.Errorf("top holding pct = %.2f, want 25.00", result.TopHoldingWeightPct)
	}
}

func TestComputeFactorExposure_MissingValuationData(t *testing.T) {
	// Some positions with valuation, some without.
	positions := []PositionWithDetails{
		getETFWithValuation("ETF1", 50, 18, 3, 20_000_000_000, []symbol.TopHolding{
			{Symbol: "X", Name: "X", Percent: 5},
		}),
		// ETF with no EquityValuation
		PositionWithDetails{
			Symbol:          "ETF2",
			PortfolioWeight: 30,
			SymbolDetails: &symbol.SymbolDetails{
				QuoteType: "ETF",
				TopHoldings: []symbol.TopHolding{
					{Symbol: "Y", Name: "Y", Percent: 8},
				},
			},
		},
		getStockWithNoValuation("STOCK1", 20),
	}

	result := ComputeFactorExposure(positions)

	// P/E: only ETF1 contributes → 18
	if result.ValueGrowthTilt.WeightedPE != 18.0 {
		t.Errorf("weighted P/E = %.2f, want 18", result.ValueGrowthTilt.WeightedPE)
	}
	// P/B: only ETF1 contributes → 3
	if result.ValueGrowthTilt.WeightedPB != 3.0 {
		t.Errorf("weighted P/B = %.2f, want 3", result.ValueGrowthTilt.WeightedPB)
	}

	// P/E 18 vs benchmark 20: within 15% (17-23) → neutral
	// P/B 3 vs benchmark 4: below 3.4 → value
	// Neutral + value → use "value" (one empty/neutral, one has data)
	// Actually: peTilt="" (neutral → empty), pbTilt="value"
	// combineTilts("", "value") → "value"
	if result.ValueGrowthTilt.Tilt != "value" {
		t.Errorf("tilt = %q, want value", result.ValueGrowthTilt.Tilt)
	}

	// Size: only ETF1 has FundProfile ($20B → large) → 50% large (of total portfolio)
	if result.SizeTilt.LargeCapPct != 50.0 {
		t.Errorf("large cap pct = %.2f, want 50", result.SizeTilt.LargeCapPct)
	}
	// 50% exactly is not >50, so tilt is "mixed" (not "large")
	if result.SizeTilt.Tilt != "mixed" {
		t.Errorf("size tilt = %q, want mixed", result.SizeTilt.Tilt)
	}

	// Should have a warning about partial size coverage (only ETF1 has FundProfile, 50% of portfolio)
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !containsWarning(result.Warnings, "size data available for only") {
		t.Errorf("expected partial size coverage warning: %v", result.Warnings)
	}
}

func TestComputeFactorExposure_AllMissingData(t *testing.T) {
	// No positions have valuation or size data.
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 50),
		getStockWithNoValuation("MSFT", 50),
	}

	result := ComputeFactorExposure(positions)

	if result.ValueGrowthTilt.WeightedPE != 0 {
		t.Errorf("weighted P/E = %.2f, want 0", result.ValueGrowthTilt.WeightedPE)
	}
	if result.ValueGrowthTilt.WeightedPB != 0 {
		t.Errorf("weighted P/B = %.2f, want 0", result.ValueGrowthTilt.WeightedPB)
	}
	if result.ValueGrowthTilt.Tilt != "unavailable" {
		t.Errorf("tilt = %q, want unavailable", result.ValueGrowthTilt.Tilt)
	}

	if result.SizeTilt.LargeCapPct != 0 {
		t.Errorf("large cap pct = %.2f, want 0", result.SizeTilt.LargeCapPct)
	}
	if result.SizeTilt.Tilt != "" {
		t.Errorf("size tilt = %q, want empty", result.SizeTilt.Tilt)
	}

	// HHI: 0.5² + 0.5² = 0.25 + 0.25 = 0.5
	if result.Concentration.HHI != 0.5 {
		t.Errorf("HHI = %.4f, want 0.50", result.Concentration.HHI)
	}
	if result.Concentration.Interpretation != "highly-concentrated" {
		t.Errorf("interpretation = %q, want highly-concentrated", result.Concentration.Interpretation)
	}

	// Both stocks equal weight → top is 50%
	if result.TopHoldingWeightPct != 50.0 {
		t.Errorf("top holding pct = %.2f, want 50.00", result.TopHoldingWeightPct)
	}

	// Should have warnings about missing valuation and size data.
	if len(result.Warnings) < 2 {
		t.Errorf("expected at least 2 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !containsWarning(result.Warnings, "no valuation data") {
		t.Error("expected warning about missing valuation data")
	}
	if !containsWarning(result.Warnings, "no size data") {
		t.Error("expected warning about missing size data")
	}
}

func TestComputeFactorExposure_HHICalculation(t *testing.T) {
	// Verify HHI calculation with known weights.
	// 3 equal stocks at 33.33% each.
	positions := []PositionWithDetails{
		getStockWithNoValuation("A", 33.33),
		getStockWithNoValuation("B", 33.33),
		getStockWithNoValuation("C", 33.34),
	}

	result := ComputeFactorExposure(positions)

	// HHI = 0.3333² + 0.3333² + 0.3334²
	//     = 0.111089 + 0.111089 + 0.111156
	//     = 0.333334 → roundTo4 = 0.3333
	expectedHHI := roundTo4(0.3333*0.3333 + 0.3333*0.3333 + 0.3334*0.3334)
	if result.Concentration.HHI != expectedHHI {
		t.Errorf("HHI = %.4f, want %.4f", result.Concentration.HHI, expectedHHI)
	}

	// Single stock at 100%.
	positions2 := []PositionWithDetails{
		getStockWithNoValuation("ONLY", 100),
	}
	result2 := ComputeFactorExposure(positions2)

	// HHI = 1.0² = 1.0
	if result2.Concentration.HHI != 1.0 {
		t.Errorf("HHI = %.4f, want 1.0000", result2.Concentration.HHI)
	}
	if result2.Concentration.Interpretation != "highly-concentrated" {
		t.Errorf("interpretation = %q, want highly-concentrated", result2.Concentration.Interpretation)
	}
}

func TestComputeFactorExposure_ValueVsGrowthAxis(t *testing.T) {
	tests := []struct {
		name       string
		pe, pb     float64
		wantTilt   string
	}{
		{
			name: "clear value — low P/E and P/B",
			pe:   12, pb: 2,
			wantTilt: "value",
		},
		{
			name: "clear growth — high P/E and P/B",
			pe:   30, pb: 6,
			wantTilt: "growth",
		},
		{
			name: "neutral — at benchmark",
			pe:   20, pb: 4,
			wantTilt: "neutral",
		},
		{
			name: "mixed — value P/E, growth P/B",
			pe:   12, pb: 6,
			wantTilt: "neutral", // disagree → neutral
		},
		{
			name: "growth by P/E only (P/B at benchmark)",
			pe:   28, pb: 4,
			wantTilt: "growth", // peTilt="growth", pbTilt="" → "growth"
		},
		{
			name: "value by P/B only (P/E at benchmark)",
			pe:   20, pb: 2,
			wantTilt: "value", // peTilt="", pbTilt="value" → "value"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			positions := []PositionWithDetails{
				getETFWithValuation("ETF", 100, tt.pe, tt.pb, 50_000_000_000, []symbol.TopHolding{
					{Symbol: "X", Name: "X", Percent: 10},
				}),
			}

			result := ComputeFactorExposure(positions)
			if result.ValueGrowthTilt.Tilt != tt.wantTilt {
				t.Errorf("tilt = %q, want %q (P/E=%.1f, P/B=%.1f)",
					result.ValueGrowthTilt.Tilt, tt.wantTilt, tt.pe, tt.pb)
			}
		})
	}
}

func TestComputeFactorExposure_Empty(t *testing.T) {
	result := ComputeFactorExposure([]PositionWithDetails{})

	if result.Message == "" {
		t.Error("expected empty-state message")
	}
	if result.ValueGrowthTilt.WeightedPE != 0 {
		t.Errorf("weighted P/E = %.2f, want 0", result.ValueGrowthTilt.WeightedPE)
	}
	if result.Concentration.HHI != 0 {
		t.Errorf("HHI = %.4f, want 0", result.Concentration.HHI)
	}
}

func TestComputeFactorExposure_SizeTiltMixed(t *testing.T) {
	// Mix of large, mid, and small cap ETFs.
	positions := []PositionWithDetails{
		getETFWithValuation("LARGE", 40, 20, 4, 50_000_000_000, nil),
		getETFWithValuation("MID", 35, 22, 4.5, 5_000_000_000, nil),
		getETFWithValuation("SMALL", 25, 25, 5, 500_000_000, nil),
	}

	result := ComputeFactorExposure(positions)

	if result.SizeTilt.LargeCapPct != 40.0 {
		t.Errorf("large cap pct = %.2f, want 40", result.SizeTilt.LargeCapPct)
	}
	if result.SizeTilt.MidCapPct != 35.0 {
		t.Errorf("mid cap pct = %.2f, want 35", result.SizeTilt.MidCapPct)
	}
	if result.SizeTilt.SmallCapPct != 25.0 {
		t.Errorf("small cap pct = %.2f, want 25", result.SizeTilt.SmallCapPct)
	}
	if result.SizeTilt.Tilt != "mixed" {
		t.Errorf("size tilt = %q, want mixed", result.SizeTilt.Tilt)
	}
}

func TestComputeFactorExposure_SizeTiltDominant(t *testing.T) {
	// One size category dominates.
	positions := []PositionWithDetails{
		getETFWithValuation("LARGE1", 35, 20, 4, 100_000_000_000, nil),
		getETFWithValuation("LARGE2", 30, 22, 4.5, 50_000_000_000, nil),
		getETFWithValuation("MID", 20, 18, 3, 5_000_000_000, nil),
		getETFWithValuation("SMALL", 15, 25, 5, 1_000_000_000, nil),
	}

	result := ComputeFactorExposure(positions)

	// Large: 35+30=65%, Mid: 20%, Small: 15%
	if result.SizeTilt.LargeCapPct != 65.0 {
		t.Errorf("large cap pct = %.2f, want 65", result.SizeTilt.LargeCapPct)
	}
	if result.SizeTilt.Tilt != "large" {
		t.Errorf("size tilt = %q, want large", result.SizeTilt.Tilt)
	}
}

func TestComputeFactorExposure_NoHoldingsData(t *testing.T) {
	// ETF with no top holdings — falls back to position weight for HHI.
	positions := []PositionWithDetails{
		PositionWithDetails{
			Symbol:          "NOHOLDINGS",
			PortfolioWeight: 100,
			SymbolDetails: &symbol.SymbolDetails{
				QuoteType: "ETF",
				EquityValuation: &symbol.EquityValuation{
					PriceToEarnings: 20,
					PriceToBook:     4,
				},
				FundProfile: &symbol.FundProfile{
					TotalNetAssets: 30_000_000_000,
				},
				// No TopHoldings
			},
		},
	}

	result := ComputeFactorExposure(positions)

	// HHI should use position weight: 1.0² = 1.0
	if result.Concentration.HHI != 1.0 {
		t.Errorf("HHI = %.4f, want 1.0000", result.Concentration.HHI)
	}

	// Should have warning about missing holdings data.
	if !containsWarning(result.Warnings, "no holdings data") {
		t.Errorf("expected warning about missing holdings data: %v", result.Warnings)
	}
}

func TestComputeFactorExposure_NoSymbolDetails(t *testing.T) {
	// Position with nil SymbolDetails.
	positions := []PositionWithDetails{
		{
			Symbol:          "GHOST",
			PortfolioWeight: 100,
			SymbolDetails:   nil,
		},
	}

	result := ComputeFactorExposure(positions)

	// Should generate warnings about missing data.
	if len(result.Warnings) < 2 {
		t.Errorf("expected at least 2 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !containsWarning(result.Warnings, "no symbol details") {
		t.Error("expected warning about missing symbol details")
	}
}

func TestComputeFactorExposure_ZeroPEOrPB(t *testing.T) {
	// P/E or P/B of 0 should be skipped (not included in weighted average).
	positions := []PositionWithDetails{
		getETFWithValuation("ETF1", 50, 0, 3, 20_000_000_000, nil),
		getETFWithValuation("ETF2", 50, 25, 0, 15_000_000_000, nil),
	}

	result := ComputeFactorExposure(positions)

	// P/E: only ETF2 (25) → weighted P/E = 25
	if result.ValueGrowthTilt.WeightedPE != 25.0 {
		t.Errorf("weighted P/E = %.2f, want 25", result.ValueGrowthTilt.WeightedPE)
	}
	// P/B: only ETF1 (3) → weighted P/B = 3
	if result.ValueGrowthTilt.WeightedPB != 3.0 {
		t.Errorf("weighted P/B = %.2f, want 3", result.ValueGrowthTilt.WeightedPB)
	}
}

func TestComputeFactorExposure_NegativePEOrPB(t *testing.T) {
	// Negative P/E (unprofitable company) should be skipped.
	positions := []PositionWithDetails{
		getETFWithValuation("ETF1", 50, -5, 3, 20_000_000_000, nil),
		getETFWithValuation("ETF2", 50, 22, 4.5, 15_000_000_000, nil),
	}

	result := ComputeFactorExposure(positions)

	// P/E: only ETF2 (22) → weighted P/E = 22
	if result.ValueGrowthTilt.WeightedPE != 22.0 {
		t.Errorf("weighted P/E = %.2f, want 22", result.ValueGrowthTilt.WeightedPE)
	}
	// P/B: both contribute → (50*3 + 50*4.5) / 100 = 3.75
	if result.ValueGrowthTilt.WeightedPB != 3.75 {
		t.Errorf("weighted P/B = %.2f, want 3.75", result.ValueGrowthTilt.WeightedPB)
	}
}

func TestComputeFactorExposure_PartialSizeCoverage(t *testing.T) {
	// Mix of ETFs (with FundProfile) and stocks (without) —
	// only ETF weight has size data, should warn.
	positions := []PositionWithDetails{
		getETFWithValuation("VTI", 60, 22, 4.5, 300_000_000_000, nil),
		getStockWithNoValuation("AAPL", 25),
		getStockWithNoValuation("JNJ", 15),
	}

	result := ComputeFactorExposure(positions)

	// Size percentages are relative to total portfolio, so large = 60% (gap = missing data).
	if result.SizeTilt.LargeCapPct != 60.0 {
		t.Errorf("large cap pct = %.2f, want 60", result.SizeTilt.LargeCapPct)
	}

	// Should have a warning about partial size coverage.
	if !containsWarning(result.Warnings, "size data available for only") {
		t.Errorf("expected partial size coverage warning: %v", result.Warnings)
	}
	if !containsWarning(result.Warnings, "60.00%") {
		t.Errorf("warning should mention 60%%: %v", result.Warnings)
	}
}

func TestComputeFactorExposure_TiltUnavailable(t *testing.T) {
	// All stocks (no EquityValuation) — tilt should be "unavailable", not "neutral".
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 50),
		getStockWithNoValuation("MSFT", 50),
	}

	result := ComputeFactorExposure(positions)

	if result.ValueGrowthTilt.Tilt != "unavailable" {
		t.Errorf("tilt = %q, want unavailable", result.ValueGrowthTilt.Tilt)
	}
	if result.ValueGrowthTilt.WeightedPE != 0 {
		t.Errorf("weighted P/E = %.2f, want 0", result.ValueGrowthTilt.WeightedPE)
	}
	if result.ValueGrowthTilt.WeightedPB != 0 {
		t.Errorf("weighted P/B = %.2f, want 0", result.ValueGrowthTilt.WeightedPB)
	}
	if !containsWarning(result.Warnings, "no valuation data") {
		t.Errorf("expected valuation data warning: %v", result.Warnings)
	}
}

// --- Helper function tests ---

func TestClassifyTilt(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		benchmark float64
		want      string
	}{
		{"below threshold", 15, 20, "value"},
		{"above threshold", 25, 20, "growth"},
		{"at benchmark", 20, 20, ""},
		{"within band", 18, 20, ""},
		{"within band high", 23, 20, ""},
		{"zero value", 0, 20, ""},
		{"just below", 16.9, 20, "value"},
		{"just above", 23.1, 20, "growth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTilt(tt.value, tt.benchmark)
			if got != tt.want {
				t.Errorf("classifyTilt(%f, %f) = %q, want %q", tt.value, tt.benchmark, got, tt.want)
			}
		})
	}
}

func TestCombineTilts(t *testing.T) {
	tests := []struct {
		name   string
		peTilt string
		pbTilt string
		want   string
	}{
		{"both value", "value", "value", "value"},
		{"both growth", "growth", "growth", "growth"},
		{"both neutral", "", "", "neutral"},
		{"pe value, pb neutral", "value", "", "value"},
		{"pe neutral, pb growth", "", "growth", "growth"},
		{"disagree", "value", "growth", "neutral"},
		{"disagree reverse", "growth", "value", "neutral"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := combineTilts(tt.peTilt, tt.pbTilt)
			if got != tt.want {
				t.Errorf("combineTilts(%q, %q) = %q, want %q", tt.peTilt, tt.pbTilt, got, tt.want)
			}
		})
	}
}

func TestClassifySizeTilt(t *testing.T) {
	tests := []struct {
		name  string
		large float64
		mid   float64
		small float64
		want  string
	}{
		{"large dominant", 60, 25, 15, "large"},
		{"mid dominant", 30, 55, 15, "mid"},
		{"small dominant", 20, 29, 51, "small"},
		{"mixed — no majority", 40, 35, 25, "mixed"},
		{"equal thirds", 33.33, 33.33, 33.34, "mixed"},
		{"just over 50 large", 50.1, 30, 19.9, "large"},
		{"exactly 50 large", 50, 30, 20, "mixed"}, // must exceed 50
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySizeTilt(tt.large, tt.mid, tt.small)
			if got != tt.want {
				t.Errorf("classifySizeTilt(%.2f, %.2f, %.2f) = %q, want %q",
					tt.large, tt.mid, tt.small, got, tt.want)
			}
		})
	}
}

func TestClassifyHHI(t *testing.T) {
	tests := []struct {
		name  string
		hhi   float64
		want  string
	}{
		{"well diversified", 0.01, "well-diversified"},
		{"boundary well diversified", 0.0199, "well-diversified"},
		{"moderately concentrated", 0.03, "moderately-concentrated"},
		{"boundary moderate", 0.05, "moderately-concentrated"},
		{"highly concentrated", 0.06, "highly-concentrated"},
		{"max concentration", 1.0, "highly-concentrated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyHHI(tt.hhi)
			if got != tt.want {
				t.Errorf("classifyHHI(%.4f) = %q, want %q", tt.hhi, got, tt.want)
			}
		})
	}
}

// containsWarning checks if any warning in the slice contains the given substring.
func containsWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
