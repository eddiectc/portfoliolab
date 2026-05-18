package analysis

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
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

	result := ComputeFactorExposure(positions, nil)

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

	// Quality unavailable because getETFWithValuation doesn't set P/CF or P/Sales.
	// Cost unavailable because getETFWithValuation doesn't set expense ratio or turnover.
	if len(result.Warnings) != 2 {
		t.Errorf("expected 2 warnings (quality + cost unavailable), got %d: %v", len(result.Warnings), result.Warnings)
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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	// Should have warnings about partial size coverage and quality data.
	if len(result.Warnings) < 2 {
		t.Errorf("expected at least 2 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !containsWarning(result.Warnings, "size data available for only") {
		t.Errorf("expected partial size coverage warning: %v", result.Warnings)
	}
	if !containsWarning(result.Warnings, "no quality data") {
		t.Errorf("expected quality data warning: %v", result.Warnings)
	}
}

func TestComputeFactorExposure_AllMissingData(t *testing.T) {
	// No positions have valuation or size data.
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 50),
		getStockWithNoValuation("MSFT", 50),
	}

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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
	result2 := ComputeFactorExposure(positions2, nil)

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

			result := ComputeFactorExposure(positions, nil)
			if result.ValueGrowthTilt.Tilt != tt.wantTilt {
				t.Errorf("tilt = %q, want %q (P/E=%.1f, P/B=%.1f)",
					result.ValueGrowthTilt.Tilt, tt.wantTilt, tt.pe, tt.pb)
			}
		})
	}
}

func TestComputeFactorExposure_Empty(t *testing.T) {
	result := ComputeFactorExposure([]PositionWithDetails{}, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

	result := ComputeFactorExposure(positions, nil)

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

// --- Quality tests ---

func TestComputeFactorExposure_QualityHappyPath(t *testing.T) {
	// Two ETFs with P/CF and P/Sales data.
	// ETF1: 60% weight, P/CF=8 (high quality), P/Sales=2.0 (high quality)
	// ETF2: 40% weight, P/CF=12 (low quality), P/Sales=3.5 (low quality)
	positions := []PositionWithDetails{
		getETFWithFullData("ETF1", 60, 18, 3, 8, 2.0, 50_000_000_000, 0.03, 25, nil),
		getETFWithFullData("ETF2", 40, 25, 5, 12, 3.5, 30_000_000_000, 0.05, 40, nil),
	}

	result := ComputeFactorExposure(positions, nil)

	// Weighted P/CF: (60*8 + 40*12) / 100 = (480+480)/100 = 9.6
	if result.Quality.WeightedPCF != 9.6 {
		t.Errorf("weighted P/CF = %.2f, want 9.60", result.Quality.WeightedPCF)
	}
	// Weighted P/Sales: (60*2.0 + 40*3.5) / 100 = (120+140)/100 = 2.6
	if result.Quality.WeightedPS != 2.6 {
		t.Errorf("weighted P/Sales = %.2f, want 2.60", result.Quality.WeightedPS)
	}

	// P/CF 9.6 vs benchmark 10: within 20% (8-12) → neutral
	// P/Sales 2.6 vs benchmark 2.5: within 20% (2.0-3.0) → neutral
	// Both neutral → "neutral"
	if result.Quality.Tilt != "neutral" {
		t.Errorf("quality tilt = %q, want neutral", result.Quality.Tilt)
	}
}

func TestComputeFactorExposure_QualityHigh(t *testing.T) {
	// Low P/CF and P/Sales → high quality.
	positions := []PositionWithDetails{
		getETFWithFullData("ETF1", 100, 15, 2.5, 6, 1.5, 80_000_000_000, 0.03, 20, nil),
	}

	result := ComputeFactorExposure(positions, nil)

	// P/CF 6 < 8 (10-2) → high-quality
	// P/Sales 1.5 < 2.0 (2.5-0.5) → high-quality
	if result.Quality.Tilt != "high-quality" {
		t.Errorf("quality tilt = %q, want high-quality", result.Quality.Tilt)
	}
}

func TestComputeFactorExposure_QualityLow(t *testing.T) {
	// High P/CF and P/Sales → low quality.
	positions := []PositionWithDetails{
		getETFWithFullData("ETF1", 100, 30, 6, 14, 4.0, 50_000_000_000, 0.05, 50, nil),
	}

	result := ComputeFactorExposure(positions, nil)

	// P/CF 14 > 12 (10+2) → low-quality
	// P/Sales 4.0 > 3.0 (2.5+0.5) → low-quality
	if result.Quality.Tilt != "low-quality" {
		t.Errorf("quality tilt = %q, want low-quality", result.Quality.Tilt)
	}
}

func TestComputeFactorExposure_QualityUnavailable(t *testing.T) {
	// No P/CF or P/Sales data.
	positions := []PositionWithDetails{
		getETFWithValuation("ETF1", 100, 20, 4, 50_000_000_000, nil),
	}

	result := ComputeFactorExposure(positions, nil)

	if result.Quality.Tilt != "unavailable" {
		t.Errorf("quality tilt = %q, want unavailable", result.Quality.Tilt)
	}
	if !containsWarning(result.Warnings, "no quality data") {
		t.Errorf("expected quality data warning: %v", result.Warnings)
	}
}

// --- Cost tests ---

func TestComputeFactorExposure_CostHappyPath(t *testing.T) {
	// Two ETFs with expense ratio and turnover.
	positions := []PositionWithDetails{
		getETFWithFullData("ETF1", 60, 18, 3, 8, 2.0, 50_000_000_000, 0.03, 25, nil),
		getETFWithFullData("ETF2", 40, 25, 5, 12, 3.5, 30_000_000_000, 0.05, 40, nil),
	}

	result := ComputeFactorExposure(positions, nil)

	// Weighted expense: (60*0.03 + 40*0.05) / 100 = (1.8+2.0)/100 = 0.038
	if result.Cost.WeightedExpenseRatio != 0.04 {
		t.Errorf("weighted expense = %.4f, want 0.04", result.Cost.WeightedExpenseRatio)
	}
	// Weighted turnover: (60*25 + 40*40) / 100 = (1500+1600)/100 = 31.0
	if result.Cost.WeightedTurnover != 31.0 {
		t.Errorf("weighted turnover = %.2f, want 31.0", result.Cost.WeightedTurnover)
	}
}

func TestComputeFactorExposure_CostZeroValues(t *testing.T) {
	// Expense ratio or turnover of 0 should be skipped.
	positions := []PositionWithDetails{
		getETFWithFullData("ETF1", 50, 18, 3, 8, 2.0, 50_000_000_000, 0, 25, nil),
		getETFWithFullData("ETF2", 50, 25, 5, 12, 3.5, 30_000_000_000, 0.05, 0, nil),
	}

	result := ComputeFactorExposure(positions, nil)

	// Expense: only ETF2 (0.05) → 0.05
	if result.Cost.WeightedExpenseRatio != 0.05 {
		t.Errorf("weighted expense = %.4f, want 0.05", result.Cost.WeightedExpenseRatio)
	}
	// Turnover: only ETF1 (25) → 25
	if result.Cost.WeightedTurnover != 25.0 {
		t.Errorf("weighted turnover = %.2f, want 25", result.Cost.WeightedTurnover)
	}
}

func TestComputeFactorExposure_PartialCoverageWarnings(t *testing.T) {
	// One ETF with full data, one with no FundProfile — should warn on partial cost.
	positions := []PositionWithDetails{
		getETFWithFullData("ETF1", 60, 18, 3, 8, 2.0, 50_000_000_000, 0.03, 25, nil),
		getStockWithNoValuation("STOCK1", 40),
	}

	result := ComputeFactorExposure(positions, nil)

	// Should have a warning about partial cost coverage.
	if !containsWarning(result.Warnings, "cost data available for only") {
		t.Errorf("expected partial cost warning, got: %v", result.Warnings)
	}
}

func TestComputeFactorExposure_MomentumPartialCoverageWarning(t *testing.T) {
	// Only one of two positions has price data.
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 60),
		getStockWithNoValuation("MSFT", 40),
	}

	// Only provide prices for AAPL (60% of portfolio).
	prices := makePriceMap([]priceEntry{
		{"AAPL", -92, 100}, {"AAPL", 0, 106},
	})

	result := ComputeFactorExposure(positions, prices)

	if !containsWarning(result.Warnings, "momentum data available for only") {
		t.Errorf("expected partial momentum warning, got: %v", result.Warnings)
	}
}

func TestComputeFactorExposure_VolatilityPartialCoverageWarning(t *testing.T) {
	// Only one of two positions has price data.
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 60),
		getStockWithNoValuation("MSFT", 40),
	}

	// Only provide prices for AAPL (60% of portfolio).
	prices := makePriceMap([]priceEntry{
		{"AAPL", -92, 100}, {"AAPL", -90, 101}, {"AAPL", -60, 99}, {"AAPL", -30, 102}, {"AAPL", 0, 100},
	})

	result := ComputeFactorExposure(positions, prices)

	if !containsWarning(result.Warnings, "volatility data available for only") {
		t.Errorf("expected partial volatility warning, got: %v", result.Warnings)
	}
}

// --- Momentum tests ---

func TestComputeFactorExposure_MomentumHappyPath(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 60),
		getStockWithNoValuation("MSFT", 40),
	}

	// AAPL: prices from 12 months ago to now, with ~10% gain over 12M.
	// MSFT: similar but ~5% gain.
	prices := makePriceMap([]priceEntry{
		// AAPL: 12 months of daily prices, rising from 100 to 110.
		{"AAPL", -365, 100}, {"AAPL", -183, 103}, {"AAPL", -92, 106}, {"AAPL", 0, 110},
		// MSFT: rising from 100 to 105.
		{"MSFT", -365, 100}, {"MSFT", -183, 101.5}, {"MSFT", -92, 103}, {"MSFT", 0, 105},
	})

	result := ComputeFactorExposure(positions, prices)

	// Both have positive returns → positive momentum
	if result.Momentum.Tilt != "positive" {
		t.Errorf("momentum tilt = %q, want positive", result.Momentum.Tilt)
	}
	if result.Momentum.Return12M <= 0 {
		t.Errorf("12M return = %.2f, want >0", result.Momentum.Return12M)
	}
}

func TestComputeFactorExposure_MomentumNegative(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithNoValuation("FALLING", 100),
	}

	// Prices declining from 100 to 80.
	prices := makePriceMap([]priceEntry{
		{"FALLING", -365, 100}, {"FALLING", -183, 92}, {"FALLING", -92, 86}, {"FALLING", 0, 80},
	})

	result := ComputeFactorExposure(positions, prices)

	if result.Momentum.Tilt != "negative" {
		t.Errorf("momentum tilt = %q, want negative", result.Momentum.Tilt)
	}
}

func TestComputeFactorExposure_MomentumUnavailable(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 100),
	}

	result := ComputeFactorExposure(positions, nil)

	if result.Momentum.Tilt != "unavailable" {
		t.Errorf("momentum tilt = %q, want unavailable", result.Momentum.Tilt)
	}
}

// --- Volatility tests ---

func TestComputeFactorExposure_VolatilityHappyPath(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithNoValuation("STABLE", 100),
	}

	// Prices with low daily variance (~0.3% daily → ~5% annualized).
	prices := makePriceMap([]priceEntry{
		{"STABLE", -60, 100}, {"STABLE", -59, 100.3}, {"STABLE", -58, 100.1},
		{"STABLE", -57, 100.4}, {"STABLE", -56, 100.2}, {"STABLE", -55, 100.5},
		{"STABLE", -54, 100.3}, {"STABLE", -53, 100.1}, {"STABLE", -52, 100.4},
		{"STABLE", -51, 100.2}, {"STABLE", -50, 100.3},
	})

	result := ComputeFactorExposure(positions, prices)

	if result.Volatility.Tilt != "low" {
		t.Errorf("volatility tilt = %q, want low (vol=%.2f)", result.Volatility.Tilt, result.Volatility.AnnualizedVol)
	}
}

func TestComputeFactorExposure_VolatilityHigh(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithNoValuation("VOLATILE", 100),
	}

	// Prices oscillating wildly (~2% daily → ~32% annualized).
	prices := makePriceMap([]priceEntry{
		{"VOLATILE", -60, 100}, {"VOLATILE", -59, 102}, {"VOLATILE", -58, 98},
		{"VOLATILE", -57, 101}, {"VOLATILE", -56, 97}, {"VOLATILE", -55, 103},
		{"VOLATILE", -54, 96}, {"VOLATILE", -53, 104}, {"VOLATILE", -52, 95},
		{"VOLATILE", -51, 105}, {"VOLATILE", -50, 94},
	})

	result := ComputeFactorExposure(positions, prices)

	if result.Volatility.Tilt != "high" {
		t.Errorf("volatility tilt = %q, want high (vol=%.2f)", result.Volatility.Tilt, result.Volatility.AnnualizedVol)
	}
}

func TestComputeFactorExposure_VolatilityUnavailable(t *testing.T) {
	positions := []PositionWithDetails{
		getStockWithNoValuation("AAPL", 100),
	}

	result := ComputeFactorExposure(positions, nil)

	if result.Volatility.Tilt != "unavailable" {
		t.Errorf("volatility tilt = %q, want unavailable", result.Volatility.Tilt)
	}
}

// --- Helper for price-based tests ---

type priceEntry struct {
	sym  string
	daysAgo int
	price  float64
}

func makePriceMap(entries []priceEntry) map[string][]market.HistoricalPrice {
	result := make(map[string][]market.HistoricalPrice)
	now := time.Now()

	for _, e := range entries {
		date := now.AddDate(0, 0, e.daysAgo)
		price := decimal.MustParse(fmt.Sprintf("%.2f", e.price))
		result[e.sym] = append(result[e.sym], market.HistoricalPrice{
			Date:     date,
			Close:    price,
			Currency: "USD",
		})
	}

	return result
}

// --- Helper function tests for new factors ---

func TestClassifyMomentum(t *testing.T) {
	tests := []struct {
		name string
		r3   float64
		r6   float64
		r12  float64
		want string
	}{
		{"all positive", 5, 8, 10, "positive"},
		{"all negative", -5, -8, -10, "negative"},
		{"mixed — more positive", 5, -1, 3, "positive"},
		{"mixed — more negative", -5, 1, -3, "negative"},
		{"all zero", 0, 0, 0, "unavailable"},
		{"within threshold", 1, 1, 1, "neutral"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyMomentum(tt.r3, tt.r6, tt.r12)
			if got != tt.want {
				t.Errorf("classifyMomentum(%.1f, %.1f, %.1f) = %q, want %q",
					tt.r3, tt.r6, tt.r12, got, tt.want)
			}
		})
	}
}

func TestClassifyVolatility(t *testing.T) {
	tests := []struct {
		name string
		vol  float64
		want string
	}{
		{"low", 8, "low"},
		{"boundary low", 10, "low"},
		{"medium", 15, "medium"},
		{"boundary high", 20, "medium"},
		{"high", 25, "high"},
		{"zero", 0, "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyVolatility(tt.vol)
			if got != tt.want {
				t.Errorf("classifyVolatility(%.1f) = %q, want %q", tt.vol, got, tt.want)
			}
		})
	}
}

func TestRemapQualityTilt(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"value", "high-quality"},
		{"growth", "low-quality"},
		{"neutral", "neutral"},
		{"unavailable", "unavailable"},
	}

	for _, tt := range tests {
		got := remapQualityTilt(tt.input)
		if got != tt.want {
			t.Errorf("remapQualityTilt(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
