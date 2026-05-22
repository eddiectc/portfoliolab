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

func portfolioHoldingETF(t *testing.T, sym string, weight float64, holdings []symbol.TopHolding) PortfolioHolding {
	t.Helper()
	return PortfolioHolding{
		Symbol:      sym,
		Weight:      decF(weight),
		Name:        sym,
		QuoteType:   "ETF",
		TopHoldings: holdings,
	}
}

func portfolioHoldingStock(t *testing.T, sym string, weight float64, name string) PortfolioHolding {
	t.Helper()
	return PortfolioHolding{
		Symbol:    sym,
		Weight:    decF(weight),
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
	// Expanded A: AAPL = 0.5*5/100 + 0.5*6/100 = 0.025+0.030 = 0.055
	//             MSFT = 0.5*4/100 = 0.020
	//             GOOGL = 0.5*4/100 = 0.020
	//
	// Expanded B: AAPL = 0.6*4.5/100 + 0.4*5/100 = 0.027+0.020 = 0.047
	//             MSFT = 0.6*3.5/100 + 0.4*4/100 = 0.021+0.016 = 0.037

	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingETF(t, "VOO", 0.5, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{5, 4},
				[]string{"Apple", "Microsoft"},
			)),
			portfolioHoldingETF(t, "QQQ", 0.5, topHoldings(
				[]string{"AAPL", "GOOGL"},
				[]float64{6, 4},
				[]string{"Apple", "Alphabet"},
			)),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingETF(t, "IVV", 0.6, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{4.5, 3.5},
				[]string{"Apple", "Microsoft"},
			)),
			portfolioHoldingETF(t, "VOO", 0.4, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{5, 4},
				[]string{"Apple", "Microsoft"},
			)),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Check top holdings A: AAPL (0.055), MSFT (0.020), GOOGL (0.020)
	if len(result.TopHoldingsA) != 3 {
		t.Fatalf("TopHoldingsA len = %d, want 3", len(result.TopHoldingsA))
	}
	aaplW, _ := result.TopHoldingsA[0].Weight.Float64()
	if !floatEq(aaplW, 0.055, 0.001) {
		t.Errorf("TopHoldingsA[0] (AAPL) weight = %.4f, want 0.055", aaplW)
	}

	// Check top holdings B: AAPL (0.047), MSFT (0.037)
	if len(result.TopHoldingsB) != 2 {
		t.Fatalf("TopHoldingsB len = %d, want 2", len(result.TopHoldingsB))
	}
	bAaplW, _ := result.TopHoldingsB[0].Weight.Float64()
	if !floatEq(bAaplW, 0.047, 0.001) {
		t.Errorf("TopHoldingsB[0] (AAPL) weight = %.4f, want 0.047", bAaplW)
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
	// Expanded A: AAPL = 0.4 + 0.6*5/100 = 0.4+0.03 = 0.43
	//             MSFT = 0.6*4/100 = 0.024
	//
	// Expanded B: MSFT = 0.3 + 0.7*3.5/100 = 0.3+0.0245 = 0.3245
	//             AAPL = 0.7*4.5/100 = 0.0315

	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 0.4, "Apple"),
			portfolioHoldingETF(t, "VOO", 0.6, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{5, 4},
				[]string{"Apple", "Microsoft"},
			)),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "MSFT", 0.3, "Microsoft"),
			portfolioHoldingETF(t, "IVV", 0.7, topHoldings(
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
			portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
			portfolioHoldingStock(t, "MSFT", 0.5, "Microsoft"),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "JNJ", 0.6, "J&J"),
			portfolioHoldingStock(t, "PFE", 0.4, "Pfizer"),
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
			portfolioHoldingStock(t, "AAPL", 1.0, "Apple"),
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
			portfolioHoldingETF(t, "VOO", 0.5, topHoldings(
				[]string{"AAPL"},
				[]float64{5},
				[]string{"Apple"},
			)),
			portfolioHoldingETF(t, "UNKNOWN_ETF", 0.5, []symbol.TopHolding{}),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 1.0, "Apple"),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Should have 1 warning for UNKNOWN_ETF
	if len(result.Warnings) != 1 {
		t.Errorf("Warnings len = %d, want 1; warnings = %v", len(result.Warnings), result.Warnings)
	}

	// UNKNOWN_ETF with no holdings is treated as atomic, so A expands to:
	// AAPL (from VOO) = 0.5*5/100 = 0.025, UNKNOWN_ETF (atomic) = 0.5
	// B expands to: AAPL = 1.0
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
	holdings = append(holdings, portfolioHoldingETF(t, "MEGA_ETF", 1.0, tfHoldings))

	input := CrossPortfolioOverlapInput{
		PortfolioA: holdings,
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 1.0, "Apple"),
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
		portfolioHoldingStock(t, "AAPL", 0.4, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.35, "Microsoft"),
		portfolioHoldingStock(t, "GOOGL", 0.25, "Alphabet"),
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
		portfolioHoldingStock(t, "AAPL", 0.4, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.6, "Microsoft"),
	}

	result := expandETFHoldings(holdings)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	aaplW, _ := result["AAPL"].weight.Float64()
	if !floatEq(aaplW, 0.4, 0.001) {
		t.Errorf("AAPL weight = %.4f, want 0.4", aaplW)
	}

	msftW, _ := result["MSFT"].weight.Float64()
	if !floatEq(msftW, 0.6, 0.001) {
		t.Errorf("MSFT weight = %.4f, want 0.6", msftW)
	}
}

func TestExpandETFHoldings_MixedETFAndStock(t *testing.T) {
	holdings := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.3, "Apple"),
		portfolioHoldingETF(t, "VOO", 0.7, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{5, 4},
			[]string{"Apple", "Microsoft"},
		)),
	}

	result := expandETFHoldings(holdings)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// AAPL = 0.3 + 0.7*5/100 = 0.3 + 0.035 = 0.335
	aaplW, _ := result["AAPL"].weight.Float64()
	if !floatEq(aaplW, 0.335, 0.001) {
		t.Errorf("AAPL weight = %.4f, want 0.335", aaplW)
	}

	// MSFT = 0.7*4/100 = 0.028
	msftW, _ := result["MSFT"].weight.Float64()
	if !floatEq(msftW, 0.028, 0.001) {
		t.Errorf("MSFT weight = %.4f, want 0.028", msftW)
	}
}

// --- computeOverlapPercentage ---

func TestComputeOverlapPercentage(t *testing.T) {
	tests := []struct {
		name    string
		setA    map[string]*holdingInfo
		setB    map[string]*holdingInfo
		wantPct float64
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
