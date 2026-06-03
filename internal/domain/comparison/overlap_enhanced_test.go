package comparison

import (
	"testing"
)

// --- ComputeMergedHoldings ---

func TestComputeMergedHoldings_SharedAndUniqueHoldings(t *testing.T) {
	// Portfolio A: 40% AAPL, 30% MSFT, 30% GOOGL
	// Portfolio B: 50% AAPL, 20% MSFT, 30% JNJ
	//
	// Shared: AAPL (A=0.4, B=0.5, overlap=40%), MSFT (A=0.3, B=0.2, overlap=20%)
	// Unique A: GOOGL (A=0.3, B=0)
	// Unique B: JNJ (A=0, B=0.3)

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.4, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.3, "Microsoft"),
		portfolioHoldingStock(t, "GOOGL", 0.3, "Alphabet"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.2, "Microsoft"),
		portfolioHoldingStock(t, "JNJ", 0.3, "J&J"),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 10)

	if len(result) != 4 {
		t.Fatalf("len = %d, want 4", len(result))
	}

	// First two should be shared, sorted by overlap % desc.
	// AAPL: overlap = min(0.4, 0.5)*100 = 40%
	if result[0].Symbol != "AAPL" {
		t.Errorf("[0] Symbol = %s, want AAPL", result[0].Symbol)
	}
	if !floatEq(result[0].OverlapPct, 40.0, 0.1) {
		t.Errorf("[0] OverlapPct = %.1f, want 40.0", result[0].OverlapPct)
	}
	aaplWA, _ := result[0].WeightA.Float64()
	aaplWB, _ := result[0].WeightB.Float64()
	if !floatEq(aaplWA, 0.4, 0.001) {
		t.Errorf("[0] WeightA = %.3f, want 0.4", aaplWA)
	}
	if !floatEq(aaplWB, 0.5, 0.001) {
		t.Errorf("[0] WeightB = %.3f, want 0.5", aaplWB)
	}

	// MSFT: overlap = min(0.3, 0.2)*100 = 20%
	if result[1].Symbol != "MSFT" {
		t.Errorf("[1] Symbol = %s, want MSFT", result[1].Symbol)
	}
	if !floatEq(result[1].OverlapPct, 20.0, 0.1) {
		t.Errorf("[1] OverlapPct = %.1f, want 20.0", result[1].OverlapPct)
	}

	// Unique holdings follow, sorted by weight desc.
	// GOOGL (A=0.3) and JNJ (B=0.3) — same weight, so alphabetical.
	if result[2].Symbol != "GOOGL" {
		t.Errorf("[2] Symbol = %s, want GOOGL", result[2].Symbol)
	}
	if !floatEq(result[2].OverlapPct, 0.0, 0.01) {
		t.Errorf("[2] OverlapPct = %.1f, want 0.0", result[2].OverlapPct)
	}
	googlWA, _ := result[2].WeightA.Float64()
	googlWB, _ := result[2].WeightB.Float64()
	if !floatEq(googlWA, 0.3, 0.001) {
		t.Errorf("[2] WeightA = %.3f, want 0.3", googlWA)
	}
	if !floatEq(googlWB, 0.0, 0.001) {
		t.Errorf("[2] WeightB = %.3f, want 0.0", googlWB)
	}

	if result[3].Symbol != "JNJ" {
		t.Errorf("[3] Symbol = %s, want JNJ", result[3].Symbol)
	}
	jnjWA, _ := result[3].WeightA.Float64()
	jnjWB, _ := result[3].WeightB.Float64()
	if !floatEq(jnjWA, 0.0, 0.001) {
		t.Errorf("[3] WeightA = %.3f, want 0.0", jnjWA)
	}
	if !floatEq(jnjWB, 0.3, 0.001) {
		t.Errorf("[3] WeightB = %.3f, want 0.3", jnjWB)
	}
}

func TestComputeMergedHoldings_IdenticalPortfolios(t *testing.T) {
	// Both portfolios: 50% AAPL, 30% MSFT, 20% GOOGL
	// All holdings shared with 100% overlap relative to their weight.

	holdings := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.3, "Microsoft"),
		portfolioHoldingStock(t, "GOOGL", 0.2, "Alphabet"),
	}

	result := ComputeMergedHoldings(holdings, holdings, 10)

	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}

	// All shared, sorted by overlap % desc (which is weight*100).
	// AAPL: min(0.5,0.5)*100 = 50%, MSFT: 30%, GOOGL: 20%
	if result[0].Symbol != "AAPL" {
		t.Errorf("[0] Symbol = %s, want AAPL", result[0].Symbol)
	}
	if !floatEq(result[0].OverlapPct, 50.0, 0.1) {
		t.Errorf("[0] OverlapPct = %.1f, want 50.0", result[0].OverlapPct)
	}

	if result[1].Symbol != "MSFT" {
		t.Errorf("[1] Symbol = %s, want MSFT", result[1].Symbol)
	}
	if !floatEq(result[1].OverlapPct, 30.0, 0.1) {
		t.Errorf("[1] OverlapPct = %.1f, want 30.0", result[1].OverlapPct)
	}

	if result[2].Symbol != "GOOGL" {
		t.Errorf("[2] Symbol = %s, want GOOGL", result[2].Symbol)
	}
	if !floatEq(result[2].OverlapPct, 20.0, 0.1) {
		t.Errorf("[2] OverlapPct = %.1f, want 20.0", result[2].OverlapPct)
	}
}

func TestComputeMergedHoldings_NoSharedHoldings(t *testing.T) {
	// Portfolio A: 60% AAPL, 40% MSFT
	// Portfolio B: 50% JNJ, 50% PFE
	// All unique, sorted by weight desc.

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.6, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.4, "Microsoft"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "JNJ", 0.5, "J&J"),
		portfolioHoldingStock(t, "PFE", 0.5, "Pfizer"),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 10)

	if len(result) != 4 {
		t.Fatalf("len = %d, want 4", len(result))
	}

	// All overlap should be 0.
	for i, h := range result {
		if !floatEq(h.OverlapPct, 0.0, 0.01) {
			t.Errorf("[%d] %s OverlapPct = %.1f, want 0.0", i, h.Symbol, h.OverlapPct)
		}
	}

	// AAPL (0.6) should be first (highest weight).
	if result[0].Symbol != "AAPL" {
		t.Errorf("[0] Symbol = %s, want AAPL", result[0].Symbol)
	}

	// Next three: JNJ (0.5), PFE (0.5), MSFT (0.4) — JNJ and PFE same weight, alphabetical.
	if result[1].Symbol != "JNJ" {
		t.Errorf("[1] Symbol = %s, want JNJ", result[1].Symbol)
	}
	if result[2].Symbol != "PFE" {
		t.Errorf("[2] Symbol = %s, want PFE", result[2].Symbol)
	}
	if result[3].Symbol != "MSFT" {
		t.Errorf("[3] Symbol = %s, want MSFT", result[3].Symbol)
	}
}

func TestComputeMergedHoldings_LimitRespected(t *testing.T) {
	// Portfolio A: 15 holdings, each ~6.67%
	// Portfolio B: 5 holdings, each 20%
	// Limit = 3 → top 3 from A + top 3 from B (deduplicated)

	var holdingsA []PortfolioHolding
	for i := 0; i < 15; i++ {
		sym := string(rune('A'+i)) + "SYM"
		holdingsA = append(holdingsA, portfolioHoldingStock(t, sym, 1.0/15, "Stock "+sym))
	}

	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "BSYM1", 0.25, "B Stock 1"),
		portfolioHoldingStock(t, "BSYM2", 0.25, "B Stock 2"),
		portfolioHoldingStock(t, "BSYM3", 0.25, "B Stock 3"),
		portfolioHoldingStock(t, "BSYM4", 0.15, "B Stock 4"),
		portfolioHoldingStock(t, "BSYM5", 0.10, "B Stock 5"),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 3)

	// Top 3 from A + top 3 from B (no overlap) = 6
	if len(result) != 6 {
		t.Fatalf("len = %d, want 6 (3 from A + 3 from B)", len(result))
	}

	// All should be unique (overlap = 0).
	for i, h := range result {
		if !floatEq(h.OverlapPct, 0.0, 0.01) {
			t.Errorf("[%d] %s OverlapPct = %.1f, want 0.0", i, h.Symbol, h.OverlapPct)
		}
	}
}

func TestComputeMergedHoldings_LimitWithSharedHoldings(t *testing.T) {
	// Portfolio A: 5% AAPL, 5% MSFT, 90% GOOGL
	// Portfolio B: 5% AAPL, 5% MSFT, 5% AMZN, 85% JNJ
	// Limit = 2 → top 2 from A (GOOGL + one of AAPL/MSFT) + top 2 from B (JNJ + one of AAPL/MSFT/AMZN)
	// Since AAPL, MSFT, AMZN all have 5%, any can be selected as the second slot.
	// Deduplicated: GOOGL, JNJ, plus up to 2 from {AAPL, MSFT, AMZN}

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.05, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.05, "Microsoft"),
		portfolioHoldingStock(t, "GOOGL", 0.90, "Alphabet"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.05, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.05, "Microsoft"),
		portfolioHoldingStock(t, "AMZN", 0.05, "Amazon"),
		portfolioHoldingStock(t, "JNJ", 0.85, "J&J"),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 2)

	found := make(map[string]bool)
	for _, h := range result {
		found[h.Symbol] = true
	}

	// GOOGL and JNJ must be present (they're the clear top from each).
	if !found["GOOGL"] {
		t.Error("GOOGL not found in result")
	}
	if !found["JNJ"] {
		t.Error("JNJ not found in result")
	}

	// At least one of the shared stocks (AAPL/MSFT) should be present.
	sharedCount := 0
	if found["AAPL"] {
		sharedCount++
	}
	if found["MSFT"] {
		sharedCount++
	}
	if sharedCount == 0 {
		t.Error("neither AAPL nor MSFT found in result")
	}

	// Result should be at most 4 (GOOGL + JNJ + up to 2 from {AAPL, MSFT, AMZN}).
	if len(result) > 4 {
		t.Errorf("len = %d, want at most 4", len(result))
	}
}

func TestComputeMergedHoldings_ETFExpansion(t *testing.T) {
	// Portfolio A: 100% VOO (holds AAPL 5%, MSFT 4%)
	// Portfolio B: 100% IVV (holds AAPL 4.5%, MSFT 3.5%)
	//
	// Expanded A: AAPL = 0.05, MSFT = 0.04
	// Expanded B: AAPL = 0.045, MSFT = 0.035
	// Shared: AAPL (overlap = min(0.05,0.045)*100 = 4.5%), MSFT (overlap = 3.5%)

	holdingsA := []PortfolioHolding{
		portfolioHoldingETF(t, "VOO", 1.0, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{5, 4},
			[]string{"Apple", "Microsoft"},
		)),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingETF(t, "IVV", 1.0, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{4.5, 3.5},
			[]string{"Apple", "Microsoft"},
		)),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 10)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// AAPL first (higher overlap).
	if result[0].Symbol != "AAPL" {
		t.Errorf("[0] Symbol = %s, want AAPL", result[0].Symbol)
	}
	aaplWA, _ := result[0].WeightA.Float64()
	aaplWB, _ := result[0].WeightB.Float64()
	if !floatEq(aaplWA, 0.05, 0.001) {
		t.Errorf("[0] WeightA = %.4f, want 0.05", aaplWA)
	}
	if !floatEq(aaplWB, 0.045, 0.001) {
		t.Errorf("[0] WeightB = %.4f, want 0.045", aaplWB)
	}
	if !floatEq(result[0].OverlapPct, 4.5, 0.1) {
		t.Errorf("[0] OverlapPct = %.1f, want 4.5", result[0].OverlapPct)
	}

	// MSFT second.
	if result[1].Symbol != "MSFT" {
		t.Errorf("[1] Symbol = %s, want MSFT", result[1].Symbol)
	}
	msftWA, _ := result[1].WeightA.Float64()
	msftWB, _ := result[1].WeightB.Float64()
	if !floatEq(msftWA, 0.04, 0.001) {
		t.Errorf("[1] WeightA = %.4f, want 0.04", msftWA)
	}
	if !floatEq(msftWB, 0.035, 0.001) {
		t.Errorf("[1] WeightB = %.4f, want 0.035", msftWB)
	}
	if !floatEq(result[1].OverlapPct, 3.5, 0.1) {
		t.Errorf("[1] OverlapPct = %.1f, want 3.5", result[1].OverlapPct)
	}
}

func TestComputeMergedHoldings_MixedETFAndStocks(t *testing.T) {
	// Portfolio A: 50% AAPL (stock), 50% VOO (ETF: AAPL 5%, MSFT 4%)
	// Portfolio B: 30% MSFT (stock), 70% IVV (ETF: AAPL 4.5%, MSFT 3.5%)
	//
	// Expanded A: AAPL = 0.5 + 0.5*5/100 = 0.525, MSFT = 0.5*4/100 = 0.02
	// Expanded B: MSFT = 0.3 + 0.7*3.5/100 = 0.3245, AAPL = 0.7*4.5/100 = 0.0315
	//
	// Shared: AAPL (A=0.525, B=0.0315, overlap=3.15%), MSFT (A=0.02, B=0.3245, overlap=2.0%)

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
		portfolioHoldingETF(t, "VOO", 0.5, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{5, 4},
			[]string{"Apple", "Microsoft"},
		)),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "MSFT", 0.3, "Microsoft"),
		portfolioHoldingETF(t, "IVV", 0.7, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{4.5, 3.5},
			[]string{"Apple", "Microsoft"},
		)),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 10)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// AAPL first (higher overlap: 3.15% > 2.0%).
	if result[0].Symbol != "AAPL" {
		t.Errorf("[0] Symbol = %s, want AAPL", result[0].Symbol)
	}
	aaplWA, _ := result[0].WeightA.Float64()
	aaplWB, _ := result[0].WeightB.Float64()
	if !floatEq(aaplWA, 0.525, 0.001) {
		t.Errorf("[0] WeightA = %.4f, want 0.525", aaplWA)
	}
	if !floatEq(aaplWB, 0.0315, 0.001) {
		t.Errorf("[0] WeightB = %.4f, want 0.0315", aaplWB)
	}
	if !floatEq(result[0].OverlapPct, 3.15, 0.1) {
		t.Errorf("[0] OverlapPct = %.2f, want 3.15", result[0].OverlapPct)
	}

	// MSFT second.
	if result[1].Symbol != "MSFT" {
		t.Errorf("[1] Symbol = %s, want MSFT", result[1].Symbol)
	}
	msftWA, _ := result[1].WeightA.Float64()
	msftWB, _ := result[1].WeightB.Float64()
	if !floatEq(msftWA, 0.02, 0.001) {
		t.Errorf("[1] WeightA = %.4f, want 0.02", msftWA)
	}
	if !floatEq(msftWB, 0.3245, 0.001) {
		t.Errorf("[1] WeightB = %.4f, want 0.3245", msftWB)
	}
	if !floatEq(result[1].OverlapPct, 2.0, 0.1) {
		t.Errorf("[1] OverlapPct = %.2f, want 2.0", result[1].OverlapPct)
	}
}

func TestComputeMergedHoldings_EmptyPortfolios(t *testing.T) {
	result := ComputeMergedHoldings(nil, nil, 10)
	if result != nil {
		t.Errorf("nil result expected, got %d items", len(result))
	}

	// One empty, one with holdings.
	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 1.0, "Apple"),
	}
	result = ComputeMergedHoldings(holdingsA, nil, 10)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].Symbol != "AAPL" {
		t.Errorf("Symbol = %s, want AAPL", result[0].Symbol)
	}
	if !floatEq(result[0].OverlapPct, 0.0, 0.01) {
		t.Errorf("OverlapPct = %.1f, want 0.0 (unique holding)", result[0].OverlapPct)
	}
	aaplWA, _ := result[0].WeightA.Float64()
	aaplWB, _ := result[0].WeightB.Float64()
	if !floatEq(aaplWA, 1.0, 0.001) {
		t.Errorf("WeightA = %.3f, want 1.0", aaplWA)
	}
	if !floatEq(aaplWB, 0.0, 0.001) {
		t.Errorf("WeightB = %.3f, want 0.0", aaplWB)
	}
}

func TestComputeMergedHoldings_ZeroLimit(t *testing.T) {
	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 0)
	if len(result) != 0 {
		t.Errorf("len = %d, want 0 (limit=0)", len(result))
	}
}

func TestComputeMergedHoldings_ETFWithUniqueUnderlying(t *testing.T) {
	// Portfolio A: 100% VOO (holds AAPL 5%, MSFT 4%, GOOGL 3%)
	// Portfolio B: 100% QQQ (holds AAPL 6%, AMZN 4%, GOOGL 3%)
	//
	// Shared: AAPL (A=0.05, B=0.06, overlap=5%), GOOGL (A=0.03, B=0.03, overlap=3%)
	// Unique A: MSFT (A=0.04, B=0)
	// Unique B: AMZN (A=0, B=0.04)

	holdingsA := []PortfolioHolding{
		portfolioHoldingETF(t, "VOO", 1.0, topHoldings(
			[]string{"AAPL", "MSFT", "GOOGL"},
			[]float64{5, 4, 3},
			[]string{"Apple", "Microsoft", "Alphabet"},
		)),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingETF(t, "QQQ", 1.0, topHoldings(
			[]string{"AAPL", "AMZN", "GOOGL"},
			[]float64{6, 4, 3},
			[]string{"Apple", "Amazon", "Alphabet"},
		)),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 10)

	if len(result) != 4 {
		t.Fatalf("len = %d, want 4", len(result))
	}

	// Shared first: AAPL (overlap 5%), GOOGL (overlap 3%)
	if result[0].Symbol != "AAPL" || !floatEq(result[0].OverlapPct, 5.0, 0.1) {
		t.Errorf("[0] = %s overlap=%.1f, want AAPL overlap=5.0", result[0].Symbol, result[0].OverlapPct)
	}
	if result[1].Symbol != "GOOGL" || !floatEq(result[1].OverlapPct, 3.0, 0.1) {
		t.Errorf("[1] = %s overlap=%.1f, want GOOGL overlap=3.0", result[1].Symbol, result[1].OverlapPct)
	}

	// Unique: MSFT (A=0.04) and AMZN (B=0.04), same weight → alphabetical.
	if result[2].Symbol != "AMZN" {
		t.Errorf("[2] Symbol = %s, want AMZN", result[2].Symbol)
	}
	if result[3].Symbol != "MSFT" {
		t.Errorf("[3] Symbol = %s, want MSFT", result[3].Symbol)
	}
}

// --- selectTopN ---

func TestSelectTopN_LessThanLimit(t *testing.T) {
	expanded := map[string]*holdingInfoDisplay{
		"AAPL": {weight: decF(0.5), symbol: "AAPL", name: "Apple"},
		"MSFT": {weight: decF(0.3), symbol: "MSFT", name: "Microsoft"},
	}

	result := selectTopN(expanded, 10)
	if len(result) != 2 {
		t.Errorf("len = %d, want 2", len(result))
	}
}

func TestSelectTopN_MoreThanLimit(t *testing.T) {
	expanded := map[string]*holdingInfoDisplay{
		"AAPL":  {weight: decF(0.5), symbol: "AAPL", name: "Apple"},
		"MSFT":  {weight: decF(0.3), symbol: "MSFT", name: "Microsoft"},
		"GOOGL": {weight: decF(0.15), symbol: "GOOGL", name: "Alphabet"},
		"AMZN":  {weight: decF(0.05), symbol: "AMZN", name: "Amazon"},
	}

	result := selectTopN(expanded, 2)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if _, ok := result["AAPL"]; !ok {
		t.Error("AAPL not in result")
	}
	if _, ok := result["MSFT"]; !ok {
		t.Error("MSFT not in result")
	}
	if _, ok := result["GOOGL"]; ok {
		t.Error("GOOGL should not be in result")
	}
}

func TestSelectTopN_Empty(t *testing.T) {
	result := selectTopN(nil, 10)
	if result != nil {
		t.Errorf("nil expected, got %d items", len(result))
	}

	expanded := map[string]*holdingInfoDisplay{}
	result = selectTopN(expanded, 10)
	if result != nil {
		t.Errorf("nil expected, got %d items", len(result))
	}
}

func TestSelectTopN_ZeroLimit(t *testing.T) {
	expanded := map[string]*holdingInfoDisplay{
		"AAPL": {weight: decF(0.5), symbol: "AAPL"},
	}
	result := selectTopN(expanded, 0)
	if result != nil {
		t.Errorf("nil expected, got %d items", len(result))
	}
}

func TestComputeMergedHoldings_ETFMatchingByISIN(t *testing.T) {
	// Two ETFs with same underlying stocks identified by ISIN.
	holdingsA := []PortfolioHolding{
		portfolioHoldingETF(t, "VOO", 1.0, topHoldingsWithISIN(
			[]string{"AAPL", "MSFT"},
			[]string{"US0378331005", "US5949181045"},
			[]float64{5, 4},
			[]string{"Apple Inc", "Microsoft Corp"},
		)),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingETF(t, "IVV", 1.0, topHoldingsWithISIN(
			[]string{"AAPL", "MSFT"},
			[]string{"US0378331005", "US5949181045"},
			[]float64{4.5, 3.5},
			[]string{"Apple Inc.", "Microsoft Corp"},
		)),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 10)

	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}

	// Both should be shared (matched by ISIN).
	for _, h := range result {
		if floatEq(h.OverlapPct, 0.0, 0.01) {
			t.Errorf("%s has 0%% overlap — should be shared", h.Symbol)
		}
	}
}

func TestComputeMergedHoldings_NameFieldPreserved(t *testing.T) {
	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple Inc"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.3, "Apple"),
	}

	result := ComputeMergedHoldings(holdingsA, holdingsB, 10)

	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}

	// Name should be populated (from whichever source has it).
	if result[0].Name == "" {
		t.Error("Name should not be empty")
	}
}

// --- ComputeWeightDifferences ---

func TestComputeWeightDifferences_ClearOverweightUnderweight(t *testing.T) {
	// Portfolio A: 50% AAPL, 30% MSFT, 20% GOOGL
	// Portfolio B: 20% AAPL, 40% MSFT, 40% JNJ
	//
	// AAPL: A=0.5, B=0.2, diff = +30pp → overweight
	// MSFT: A=0.3, B=0.4, diff = -10pp → underweight
	// GOOGL: A=0.2, B=0, diff = +20pp → overweight
	// JNJ: A=0, B=0.4, diff = -40pp → underweight

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.3, "Microsoft"),
		portfolioHoldingStock(t, "GOOGL", 0.2, "Alphabet"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.2, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.4, "Microsoft"),
		portfolioHoldingStock(t, "JNJ", 0.4, "J&J"),
	}

	overweight, underweight, neutral := ComputeWeightDifferences(holdingsA, holdingsB, 10)

	// Overweight: AAPL (+30pp), GOOGL (+20pp)
	if len(overweight) != 2 {
		t.Fatalf("overweight len = %d, want 2", len(overweight))
	}
	if overweight[0].Symbol != "AAPL" {
		t.Errorf("overweight[0] = %s, want AAPL", overweight[0].Symbol)
	}
	if !floatEq(overweight[0].Difference, 30.0, 0.1) {
		t.Errorf("overweight[0] diff = %.1f, want 30.0", overweight[0].Difference)
	}
	if overweight[1].Symbol != "GOOGL" {
		t.Errorf("overweight[1] = %s, want GOOGL", overweight[1].Symbol)
	}
	if !floatEq(overweight[1].Difference, 20.0, 0.1) {
		t.Errorf("overweight[1] diff = %.1f, want 20.0", overweight[1].Difference)
	}

	// Underweight: JNJ (-40pp), MSFT (-10pp)
	if len(underweight) != 2 {
		t.Fatalf("underweight len = %d, want 2", len(underweight))
	}
	if underweight[0].Symbol != "JNJ" {
		t.Errorf("underweight[0] = %s, want JNJ", underweight[0].Symbol)
	}
	if !floatEq(underweight[0].Difference, -40.0, 0.1) {
		t.Errorf("underweight[0] diff = %.1f, want -40.0", underweight[0].Difference)
	}
	if underweight[1].Symbol != "MSFT" {
		t.Errorf("underweight[1] = %s, want MSFT", underweight[1].Symbol)
	}
	if !floatEq(underweight[1].Difference, -10.0, 0.1) {
		t.Errorf("underweight[1] diff = %.1f, want -10.0", underweight[1].Difference)
	}

	// No neutral holdings (no holding has identical weight in both portfolios).
	if len(neutral) != 0 {
		t.Errorf("neutral len = %d, want 0", len(neutral))
	}
}

func TestComputeWeightDifferences_IdenticalPortfolios(t *testing.T) {
	// Both portfolios: 50% AAPL, 30% MSFT, 20% GOOGL
	// All differences = 0, so all holdings are neutral.

	holdings := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.3, "Microsoft"),
		portfolioHoldingStock(t, "GOOGL", 0.2, "Alphabet"),
	}

	overweight, underweight, neutral := ComputeWeightDifferences(holdings, holdings, 10)

	if len(overweight) != 0 {
		t.Errorf("overweight len = %d, want 0", len(overweight))
	}
	if len(underweight) != 0 {
		t.Errorf("underweight len = %d, want 0", len(underweight))
	}

	// All 3 holdings are neutral (identical weights), sorted by weight desc.
	if len(neutral) != 3 {
		t.Fatalf("neutral len = %d, want 3", len(neutral))
	}
	if neutral[0].Symbol != "AAPL" {
		t.Errorf("neutral[0] = %s, want AAPL", neutral[0].Symbol)
	}
	if !floatEq(neutral[0].Difference, 0.0, 0.01) {
		t.Errorf("neutral[0] diff = %.1f, want 0.0", neutral[0].Difference)
	}
	if neutral[1].Symbol != "MSFT" {
		t.Errorf("neutral[1] = %s, want MSFT", neutral[1].Symbol)
	}
	if neutral[2].Symbol != "GOOGL" {
		t.Errorf("neutral[2] = %s, want GOOGL", neutral[2].Symbol)
	}
}

func TestComputeWeightDifferences_OnePortfolioEmpty(t *testing.T) {
	// Portfolio A: 60% AAPL, 40% MSFT
	// Portfolio B: empty
	// All holdings are overweight for A.

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.6, "Apple"),
		portfolioHoldingStock(t, "MSFT", 0.4, "Microsoft"),
	}

	overweight, underweight, neutral := ComputeWeightDifferences(holdingsA, nil, 10)

	if len(underweight) != 0 {
		t.Errorf("underweight len = %d, want 0", len(underweight))
	}
	if len(overweight) != 2 {
		t.Fatalf("overweight len = %d, want 2", len(overweight))
	}
	// AAPL (+60pp) first, then MSFT (+40pp).
	if overweight[0].Symbol != "AAPL" {
		t.Errorf("overweight[0] = %s, want AAPL", overweight[0].Symbol)
	}
	if !floatEq(overweight[0].Difference, 60.0, 0.1) {
		t.Errorf("overweight[0] diff = %.1f, want 60.0", overweight[0].Difference)
	}
	if overweight[1].Symbol != "MSFT" {
		t.Errorf("overweight[1] = %s, want MSFT", overweight[1].Symbol)
	}
	if !floatEq(overweight[1].Difference, 40.0, 0.1) {
		t.Errorf("overweight[1] diff = %.1f, want 40.0", overweight[1].Difference)
	}
	if len(neutral) != 0 {
		t.Errorf("neutral len = %d, want 0", len(neutral))
	}

	// Reverse: B empty, A has holdings → all underweight.
	overweight2, underweight2, neutral2 := ComputeWeightDifferences(nil, holdingsA, 10)
	if len(overweight2) != 0 {
		t.Errorf("overweight len = %d, want 0", len(overweight2))
	}
	if len(underweight2) != 2 {
		t.Fatalf("underweight len = %d, want 2", len(underweight2))
	}
	if len(neutral2) != 0 {
		t.Errorf("neutral len = %d, want 0", len(neutral2))
	}
	// AAPL (-60pp) first (highest abs), then MSFT (-40pp).
	if underweight2[0].Symbol != "AAPL" {
		t.Errorf("underweight[0] = %s, want AAPL", underweight2[0].Symbol)
	}
	if !floatEq(underweight2[0].Difference, -60.0, 0.1) {
		t.Errorf("underweight[0] diff = %.1f, want -60.0", underweight2[0].Difference)
	}
}

func TestComputeWeightDifferences_LimitRespected(t *testing.T) {
	// Portfolio A: 5 holdings, each 20%
	// Portfolio B: 5 different holdings, each 20%
	// limit = 2 → 2 overweight (A's holdings), 2 underweight (B's holdings)

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "A1", 0.2, "A1"),
		portfolioHoldingStock(t, "A2", 0.2, "A2"),
		portfolioHoldingStock(t, "A3", 0.2, "A3"),
		portfolioHoldingStock(t, "A4", 0.2, "A4"),
		portfolioHoldingStock(t, "A5", 0.2, "A5"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "B1", 0.2, "B1"),
		portfolioHoldingStock(t, "B2", 0.2, "B2"),
		portfolioHoldingStock(t, "B3", 0.2, "B3"),
		portfolioHoldingStock(t, "B4", 0.2, "B4"),
		portfolioHoldingStock(t, "B5", 0.2, "B5"),
	}

	overweight, underweight, neutral := ComputeWeightDifferences(holdingsA, holdingsB, 2)

	if len(overweight) != 2 {
		t.Errorf("overweight len = %d, want 2", len(overweight))
	}
	if len(underweight) != 2 {
		t.Errorf("underweight len = %d, want 2", len(underweight))
	}
	if len(neutral) != 0 {
		t.Errorf("neutral len = %d, want 0", len(neutral))
	}
	// With same diff (20pp each), sorted alphabetically.
	if overweight[0].Symbol != "A1" {
		t.Errorf("overweight[0] = %s, want A1", overweight[0].Symbol)
	}
	if overweight[1].Symbol != "A2" {
		t.Errorf("overweight[1] = %s, want A2", overweight[1].Symbol)
	}
	if underweight[0].Symbol != "B1" {
		t.Errorf("underweight[0] = %s, want B1", underweight[0].Symbol)
	}
	if underweight[1].Symbol != "B2" {
		t.Errorf("underweight[1] = %s, want B2", underweight[1].Symbol)
	}
}

func TestComputeWeightDifferences_ETFExpansion(t *testing.T) {
	// Portfolio A: 100% VOO (holds AAPL 5%, MSFT 4%)
	// Portfolio B: 100% IVV (holds AAPL 4.5%, MSFT 3.5%)
	//
	// AAPL: A=0.05, B=0.045, diff = +0.5pp → overweight
	// MSFT: A=0.04, B=0.035, diff = +0.5pp → overweight

	holdingsA := []PortfolioHolding{
		portfolioHoldingETF(t, "VOO", 1.0, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{5, 4},
			[]string{"Apple", "Microsoft"},
		)),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingETF(t, "IVV", 1.0, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{4.5, 3.5},
			[]string{"Apple", "Microsoft"},
		)),
	}

	overweight, underweight, neutral := ComputeWeightDifferences(holdingsA, holdingsB, 10)

	if len(underweight) != 0 {
		t.Errorf("underweight len = %d, want 0", len(underweight))
	}
	if len(neutral) != 0 {
		t.Errorf("neutral len = %d, want 0", len(neutral))
	}
	if len(overweight) != 2 {
		t.Fatalf("overweight len = %d, want 2", len(overweight))
	}

	// Both have +0.5pp diff, so sorted alphabetically.
	if overweight[0].Symbol != "AAPL" {
		t.Errorf("overweight[0] = %s, want AAPL", overweight[0].Symbol)
	}
	aaplWA, _ := overweight[0].WeightA.Float64()
	aaplWB, _ := overweight[0].WeightB.Float64()
	if !floatEq(aaplWA, 0.05, 0.001) {
		t.Errorf("AAPL WeightA = %.4f, want 0.05", aaplWA)
	}
	if !floatEq(aaplWB, 0.045, 0.001) {
		t.Errorf("AAPL WeightB = %.4f, want 0.045", aaplWB)
	}
	if !floatEq(overweight[0].Difference, 0.5, 0.1) {
		t.Errorf("AAPL diff = %.2f, want 0.5", overweight[0].Difference)
	}

	if overweight[1].Symbol != "MSFT" {
		t.Errorf("overweight[1] = %s, want MSFT", overweight[1].Symbol)
	}
	msftWA, _ := overweight[1].WeightA.Float64()
	msftWB, _ := overweight[1].WeightB.Float64()
	if !floatEq(msftWA, 0.04, 0.001) {
		t.Errorf("MSFT WeightA = %.4f, want 0.04", msftWA)
	}
	if !floatEq(msftWB, 0.035, 0.001) {
		t.Errorf("MSFT WeightB = %.4f, want 0.035", msftWB)
	}
}

func TestComputeWeightDifferences_MixedETFAndStocks(t *testing.T) {
	// Portfolio A: 50% AAPL (stock), 50% VOO (ETF: AAPL 5%, MSFT 4%)
	// Portfolio B: 30% MSFT (stock), 70% IVV (ETF: AAPL 4.5%, MSFT 3.5%)
	//
	// Expanded A: AAPL = 0.5 + 0.025 = 0.525, MSFT = 0.02
	// Expanded B: MSFT = 0.3 + 0.0245 = 0.3245, AAPL = 0.0315
	//
	// AAPL: A=0.525, B=0.0315, diff = +49.35pp → overweight
	// MSFT: A=0.02, B=0.3245, diff = -30.45pp → underweight

	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
		portfolioHoldingETF(t, "VOO", 0.5, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{5, 4},
			[]string{"Apple", "Microsoft"},
		)),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "MSFT", 0.3, "Microsoft"),
		portfolioHoldingETF(t, "IVV", 0.7, topHoldings(
			[]string{"AAPL", "MSFT"},
			[]float64{4.5, 3.5},
			[]string{"Apple", "Microsoft"},
		)),
	}

	overweight, underweight, neutral := ComputeWeightDifferences(holdingsA, holdingsB, 10)

	if len(overweight) != 1 || len(underweight) != 1 {
		t.Fatalf("overweight=%d underweight=%d, want 1 each", len(overweight), len(underweight))
	}
	if len(neutral) != 0 {
		t.Errorf("neutral len = %d, want 0", len(neutral))
	}

	// AAPL overweight.
	if overweight[0].Symbol != "AAPL" {
		t.Errorf("overweight[0] = %s, want AAPL", overweight[0].Symbol)
	}
	aaplWA, _ := overweight[0].WeightA.Float64()
	aaplWB, _ := overweight[0].WeightB.Float64()
	if !floatEq(aaplWA, 0.525, 0.001) {
		t.Errorf("AAPL WeightA = %.4f, want 0.525", aaplWA)
	}
	if !floatEq(aaplWB, 0.0315, 0.001) {
		t.Errorf("AAPL WeightB = %.4f, want 0.0315", aaplWB)
	}

	// MSFT underweight.
	if underweight[0].Symbol != "MSFT" {
		t.Errorf("underweight[0] = %s, want MSFT", underweight[0].Symbol)
	}
	msftWA, _ := underweight[0].WeightA.Float64()
	msftWB, _ := underweight[0].WeightB.Float64()
	if !floatEq(msftWA, 0.02, 0.001) {
		t.Errorf("MSFT WeightA = %.4f, want 0.02", msftWA)
	}
	if !floatEq(msftWB, 0.3245, 0.001) {
		t.Errorf("MSFT WeightB = %.4f, want 0.3245", msftWB)
	}
}

func TestComputeWeightDifferences_BothEmpty(t *testing.T) {
	overweight, underweight, neutral := ComputeWeightDifferences(nil, nil, 10)
	if overweight != nil {
		t.Errorf("overweight = %d, want nil", len(overweight))
	}
	if underweight != nil {
		t.Errorf("underweight = %d, want nil", len(underweight))
	}
	if neutral != nil {
		t.Errorf("neutral = %d, want nil", len(neutral))
	}
}

func TestComputeWeightDifferences_ZeroLimit(t *testing.T) {
	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.3, "Apple"),
	}

	overweight, underweight, neutral := ComputeWeightDifferences(holdingsA, holdingsB, 0)
	if len(overweight) != 0 {
		t.Errorf("overweight len = %d, want 0 (limit=0)", len(overweight))
	}
	if len(underweight) != 0 {
		t.Errorf("underweight len = %d, want 0 (limit=0)", len(underweight))
	}
	if len(neutral) != 0 {
		t.Errorf("neutral len = %d, want 0 (limit=0)", len(neutral))
	}
}

func TestComputeWeightDifferences_NameFieldPreserved(t *testing.T) {
	holdingsA := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.5, "Apple Inc"),
	}
	holdingsB := []PortfolioHolding{
		portfolioHoldingStock(t, "AAPL", 0.3, "Apple"),
	}

	overweight, _, _ := ComputeWeightDifferences(holdingsA, holdingsB, 10)

	if len(overweight) != 1 {
		t.Fatalf("overweight len = %d, want 1", len(overweight))
	}
	if overweight[0].Name == "" {
		t.Error("Name should not be empty")
	}
}
