package comparison

import (
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// --- helpers ---

func decF(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat64(f)
	return d
}

func portfolioHoldingETF(t *testing.T, sym string, weightPct float64, holdings []symbol.TopHolding) PortfolioHolding {
	t.Helper()
	return PortfolioHolding{
		Symbol:      sym,
		WeightPct:   decF(weightPct),
		Name:        sym,
		QuoteType:   "ETF",
		TopHoldings: holdings,
	}
}

func portfolioHoldingStock(t *testing.T, sym string, weightPct float64, name string) PortfolioHolding {
	t.Helper()
	return PortfolioHolding{
		Symbol:    sym,
		WeightPct: decF(weightPct),
		Name:      name,
		QuoteType: "EQUITY",
	}
}

func topHoldings(symbols []string, percents []float64, names []string) []symbol.TopHolding {
	h := make([]symbol.TopHolding, len(symbols))
	for i := range symbols {
		h[i] = symbol.TopHolding{
			Symbol:  symbols[i],
			Percent: percents[i],
			Name:    names[i],
		}
	}
	return h
}

func floatEq(got, want, eps float64) bool {
	return (got-want) < eps && (want-got) < eps
}

func decEq(t *testing.T, got, want decimal.Decimal, eps float64) bool {
	t.Helper()
	gotF, _ := got.Float64()
	wantF, _ := want.Float64()
	if !floatEq(gotF, wantF, eps) {
		t.Errorf("decimal %.4f != %.4f (eps %.4f)", gotF, wantF, eps)
		return false
	}
	return true
}

// --- ComputeCrossPortfolioOverlap ---

func TestComputeCrossPortfolioOverlap_TwoETFPortfolios(t *testing.T) {
	// Portfolio A: 50% VOO (holds AAPL 5%, MSFT 4%), 50% QQQ (holds AAPL 6%, GOOGL 4%)
	// Portfolio B: 60% IVV (holds AAPL 4.5%, MSFT 3.5%), 40% VOO (holds AAPL 5%, MSFT 4%)
	//
	// Expanded A: AAPL = 50*5/100 + 50*6/100 = 2.5+3.0 = 5.5
	//             MSFT = 50*4/100 = 2.0
	//             GOOGL = 50*4/100 = 2.0
	//
	// Expanded B: AAPL = 60*4.5/100 + 40*5/100 = 2.7+2.0 = 4.7
	//             MSFT = 60*3.5/100 + 40*4/100 = 2.1+1.6 = 3.7

	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingETF(t, "VOO", 50, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{5, 4},
				[]string{"Apple", "Microsoft"},
			)),
			portfolioHoldingETF(t, "QQQ", 50, topHoldings(
				[]string{"AAPL", "GOOGL"},
				[]float64{6, 4},
				[]string{"Apple", "Alphabet"},
			)),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingETF(t, "IVV", 60, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{4.5, 3.5},
				[]string{"Apple", "Microsoft"},
			)),
			portfolioHoldingETF(t, "VOO", 40, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{5, 4},
				[]string{"Apple", "Microsoft"},
			)),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Check top holdings A: AAPL (5.5), MSFT (2.0), GOOGL (2.0)
	if len(result.TopHoldingsA) != 3 {
		t.Fatalf("TopHoldingsA len = %d, want 3", len(result.TopHoldingsA))
	}
	aaplW, _ := result.TopHoldingsA[0].Weight.Float64()
	if !floatEq(aaplW, 5.5, 0.01) {
		t.Errorf("TopHoldingsA[0] (AAPL) weight = %.2f, want 5.5", aaplW)
	}

	// Check top holdings B: AAPL (4.7), MSFT (3.7)
	if len(result.TopHoldingsB) != 2 {
		t.Fatalf("TopHoldingsB len = %d, want 2", len(result.TopHoldingsB))
	}
	bAaplW, _ := result.TopHoldingsB[0].Weight.Float64()
	if !floatEq(bAaplW, 4.7, 0.01) {
		t.Errorf("TopHoldingsB[0] (AAPL) weight = %.2f, want 4.7", bAaplW)
	}

	// Overlap: A has {AAPL, MSFT, GOOGL}, B has {AAPL, MSFT}
	// Intersection = {AAPL, MSFT} = 2, Union = {AAPL, MSFT, GOOGL} = 3
	// Overlap = 2/3 * 100 = 66.67
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 66.67, 0.1) {
		t.Errorf("OverlapPct = %.2f, want 66.67", overlapF)
	}
}

func TestComputeCrossPortfolioOverlap_MixedETFAndStocks(t *testing.T) {
	// Portfolio A: 40% AAPL (stock), 60% VOO (ETF: AAPL 5%, MSFT 4%)
	// Portfolio B: 30% MSFT (stock), 70% IVV (ETF: AAPL 4.5%, MSFT 3.5%)
	//
	// Expanded A: AAPL = 40 + 60*5/100 = 40+3 = 43
	//             MSFT = 60*4/100 = 2.4
	//
	// Expanded B: MSFT = 30 + 70*3.5/100 = 30+2.45 = 32.45
	//             AAPL = 70*4.5/100 = 3.15

	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 40, "Apple"),
			portfolioHoldingETF(t, "VOO", 60, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{5, 4},
				[]string{"Apple", "Microsoft"},
			)),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "MSFT", 30, "Microsoft"),
			portfolioHoldingETF(t, "IVV", 70, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{4.5, 3.5},
				[]string{"Apple", "Microsoft"},
			)),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Both portfolios expand to {AAPL, MSFT} → 100% overlap
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 100.0, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 100.0", overlapF)
	}
}

func TestComputeCrossPortfolioOverlap_NoOverlap(t *testing.T) {
	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 50, "Apple"),
			portfolioHoldingStock(t, "MSFT", 50, "Microsoft"),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "JNJ", 60, "J&J"),
			portfolioHoldingStock(t, "PFE", 40, "Pfizer"),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 0.0, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 0.0", overlapF)
	}
}

func TestComputeCrossPortfolioOverlap_EmptyPortfolio(t *testing.T) {
	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 100, "Apple"),
		},
		PortfolioB: []PortfolioHolding{},
	}

	result := ComputeCrossPortfolioOverlap(input)

	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 0.0, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 0.0", overlapF)
	}

	// TopHoldingsB should be empty
	if len(result.TopHoldingsB) != 0 {
		t.Errorf("TopHoldingsB len = %d, want 0", len(result.TopHoldingsB))
	}
}

func TestComputeCrossPortfolioOverlap_ETFWithNoHoldings(t *testing.T) {
	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingETF(t, "VOO", 50, topHoldings(
				[]string{"AAPL"},
				[]float64{5},
				[]string{"Apple"},
			)),
			portfolioHoldingETF(t, "UNKNOWN_ETF", 50, []symbol.TopHolding{}),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 100, "Apple"),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Should have 1 warning for UNKNOWN_ETF
	if len(result.Warnings) != 1 {
		t.Errorf("Warnings len = %d, want 1; warnings = %v", len(result.Warnings), result.Warnings)
	}

	// UNKNOWN_ETF with no holdings is treated as atomic, so A expands to:
	// AAPL (from VOO) = 50*5/100 = 2.5, UNKNOWN_ETF (atomic) = 50
	// B expands to: AAPL = 100
	// Overlap: A has {AAPL, UNKNOWN_ETF}, B has {AAPL}
	// Intersection = {AAPL} = 1, Union = {AAPL, UNKNOWN_ETF} = 2
	// Overlap = 50%
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 50.0, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 50.0", overlapF)
	}
}

func TestComputeCrossPortfolioOverlap_CappedAtTop10(t *testing.T) {
	// Create a portfolio with 15 unique underlying holdings
	var holdings []PortfolioHolding
	// One ETF holding 15 unique stocks, each at ~6.67%
	var tfHoldings []symbol.TopHolding
	for i := 0; i < 15; i++ {
		tfHoldings = append(tfHoldings, symbol.TopHolding{
			Symbol:  "SYM" + string(rune('A'+i)),
			Percent: 6.67,
			Name:    "Stock " + string(rune('A'+i)),
		})
	}
	holdings = append(holdings, portfolioHoldingETF(t, "MEGA_ETF", 100, tfHoldings))

	input := CrossPortfolioOverlapInput{
		PortfolioA: holdings,
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 100, "Apple"),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Should be capped at 10
	if len(result.TopHoldingsA) != 10 {
		t.Errorf("TopHoldingsA len = %d, want 10", len(result.TopHoldingsA))
	}
}

func TestComputeCrossPortfolioOverlap_IdenticalPortfolios(t *testing.T) {
	shared := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 40, "Apple"),
		portfolioHoldingStock(t, "MSFT", 35, "Microsoft"),
		portfolioHoldingStock(t, "GOOGL", 25, "Alphabet"),
	}

	input := CrossPortfolioOverlapInput{
		PortfolioA: shared,
		PortfolioB: shared,
	}

	result := ComputeCrossPortfolioOverlap(input)

	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 100.0, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 100.0", overlapF)
	}
}

// --- expandETFHoldings ---

func TestExpandETFHoldings_DirectHoldingsOnly(t *testing.T) {
	holdings := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 40, "Apple"),
		portfolioHoldingStock(t, "MSFT", 60, "Microsoft"),
	}

	result := expandETFHoldings(holdings)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	aaplW, _ := result["AAPL"].weight.Float64()
	if !floatEq(aaplW, 40.0, 0.01) {
		t.Errorf("AAPL weight = %.2f, want 40.0", aaplW)
	}

	msftW, _ := result["MSFT"].weight.Float64()
	if !floatEq(msftW, 60.0, 0.01) {
		t.Errorf("MSFT weight = %.2f, want 60.0", msftW)
	}
}

func TestExpandETFHoldings_MixedETFAndStock(t *testing.T) {
	holdings := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 30, "Apple"),
		portfolioHoldingETF(t, "VOO", 70, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{5, 4},
			[]string{"Apple", "Microsoft"},
		)),
	}

	result := expandETFHoldings(holdings)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// AAPL = 30 + 70*5/100 = 30 + 3.5 = 33.5
	aaplW, _ := result["AAPL"].weight.Float64()
	if !floatEq(aaplW, 33.5, 0.01) {
		t.Errorf("AAPL weight = %.2f, want 33.5", aaplW)
	}

	// MSFT = 70*4/100 = 2.8
	msftW, _ := result["MSFT"].weight.Float64()
	if !floatEq(msftW, 2.8, 0.01) {
		t.Errorf("MSFT weight = %.2f, want 2.8", msftW)
	}
}

// --- computeOverlapPercentage ---

func TestComputeOverlapPercentage(t *testing.T) {
	tests := []struct {
		name     string
		setA     map[string]*holdingInfo
		setB     map[string]*holdingInfo
		wantPct  float64
	}{
		{
			name:    "identical sets",
			setA:    map[string]*holdingInfo{"AAPL": {}, "MSFT": {}},
			setB:    map[string]*holdingInfo{"AAPL": {}, "MSFT": {}},
			wantPct: 100.0,
		},
		{
			name:    "no overlap",
			setA:    map[string]*holdingInfo{"AAPL": {}, "MSFT": {}},
			setB:    map[string]*holdingInfo{"JNJ": {}, "PFE": {}},
			wantPct: 0.0,
		},
		{
			name:    "partial overlap (1 of 3 unique)",
			setA:    map[string]*holdingInfo{"AAPL": {}, "MSFT": {}, "GOOGL": {}},
			setB:    map[string]*holdingInfo{"AAPL": {}, "JNJ": {}},
			wantPct: 1.0 / 4.0 * 100.0, // {AAPL} / {AAPL,MSFT,GOOGL,JNJ} = 25%
		},
		{
			name:    "one empty set",
			setA:    map[string]*holdingInfo{},
			setB:    map[string]*holdingInfo{"AAPL": {}},
			wantPct: 0.0,
		},
		{
			name:    "both empty",
			setA:    map[string]*holdingInfo{},
			setB:    map[string]*holdingInfo{},
			wantPct: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeOverlapPercentage(tt.setA, tt.setB)
			gotF, _ := got.Float64()
			if !floatEq(gotF, tt.wantPct, 0.1) {
				t.Errorf("OverlapPct = %.2f, want %.2f", gotF, tt.wantPct)
			}
		})
	}
}

// --- decEq helper test ---

func TestDecEq(t *testing.T) {
	a := decF(1.234)
	b := decF(1.235)
	decEq(t, a, b, 0.01) // should pass
}
