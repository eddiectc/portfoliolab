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
