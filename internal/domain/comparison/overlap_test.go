package comparison

import (
	"strings"
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

func portfolioHoldingETFWithSector(t *testing.T, sym string, weight float64, holdings []symbol.TopHolding, sectors []symbol.SectorWeighting, geo []symbol.GeographicAllocation) PortfolioHolding {
	t.Helper()
	return PortfolioHolding{
		Symbol:                sym,
		Weight:                decF(weight),
		Name:                  sym,
		QuoteType:             "ETF",
		TopHoldings:           holdings,
		SectorWeightings:      sectors,
		GeographicAllocations: geo,
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

func portfolioHoldingStockWithSector(t *testing.T, sym string, weight float64, name string, sector string) PortfolioHolding {
	t.Helper()
	return PortfolioHolding{
		Symbol:    sym,
		Weight:    decF(weight),
		Name:      name,
		QuoteType: "EQUITY",
		Sector:    sector,
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

	// Weighted overlap = min(AAPL_A, AAPL_B) + min(MSFT_A, MSFT_B)
	//                   = min(0.055, 0.047) + min(0.020, 0.037)
	//                   = 0.047 + 0.020 = 0.067 = 6.7%
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 6.7, 0.1) {
		t.Errorf("OverlapPct = %.2f, want 6.7", overlapF)
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

	// Weighted overlap = min(AAPL_A, AAPL_B) + min(MSFT_A, MSFT_B)
	//                   = min(0.43, 0.0315) + min(0.024, 0.3245)
	//                   = 0.0315 + 0.024 = 0.0555 = 5.55%
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 5.55, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 5.55", overlapF)
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

	// Should have at least 1 warning for UNKNOWN_ETF (plus sector/country warnings from enhanced overlap)
	if len(result.Warnings) < 1 {
		t.Fatalf("Warnings len = %d, want >= 1; warnings = %v", len(result.Warnings), result.Warnings)
	}
	// Verify the UNKNOWN_ETF warning is present.
	foundUnknownETF := false
	for _, w := range result.Warnings {
		if strings.HasPrefix(w, "ETF UNKNOWN_ETF") {
			foundUnknownETF = true
			break
		}
	}
	if !foundUnknownETF {
		t.Errorf("Expected UNKNOWN_ETF warning not found in: %v", result.Warnings)
	}

	// UNKNOWN_ETF with no holdings is treated as atomic, so A expands to:
	// AAPL (from VOO) = 0.5*5/100 = 0.025, UNKNOWN_ETF (atomic) = 0.5
	// B expands to: AAPL = 1.0
	// Weighted overlap = min(0.025, 1.0) = 0.025 = 2.5%
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 2.5, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 2.5", overlapF)
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

	result := expandETFHoldings(holdings, keyISIN)

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

	result := expandETFHoldings(holdings, keySymbol)

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

// --- computeWeightedOverlap ---

func TestComputeWeightedOverlap(t *testing.T) {
	d := func(f float64) decimal.Decimal { d, _ := decimal.NewFromFloat64(f); return d }

	tests := []struct {
		name    string
		setA    map[string]*holdingInfo
		setB    map[string]*holdingInfo
		wantPct float64
	}{
		{
			name:    "identical weights",
			setA:    map[string]*holdingInfo{"AAPL": {weight: d(0.05)}, "MSFT": {weight: d(0.04)}},
			setB:    map[string]*holdingInfo{"AAPL": {weight: d(0.05)}, "MSFT": {weight: d(0.04)}},
			wantPct: 9.0, // min(0.05,0.05) + min(0.04,0.04) = 0.09 = 9%
		},
		{
			name:    "different weights takes minimum",
			setA:    map[string]*holdingInfo{"AAPL": {weight: d(0.08)}, "MSFT": {weight: d(0.04)}},
			setB:    map[string]*holdingInfo{"AAPL": {weight: d(0.03)}, "MSFT": {weight: d(0.06)}},
			wantPct: 7.0, // min(0.08,0.03) + min(0.04,0.06) = 0.03+0.04 = 0.07 = 7%
		},
		{
			name:    "no overlap",
			setA:    map[string]*holdingInfo{"AAPL": {weight: d(0.05)}, "MSFT": {weight: d(0.04)}},
			setB:    map[string]*holdingInfo{"JNJ": {weight: d(0.05)}, "PFE": {weight: d(0.03)}},
			wantPct: 0.0,
		},
		{
			name:    "partial overlap",
			setA:    map[string]*holdingInfo{"AAPL": {weight: d(0.05)}, "MSFT": {weight: d(0.04)}},
			setB:    map[string]*holdingInfo{"AAPL": {weight: d(0.03)}, "JNJ": {weight: d(0.06)}},
			wantPct: 3.0, // min(0.05,0.03) = 0.03 = 3%
		},
		{
			name:    "one empty set",
			setA:    map[string]*holdingInfo{},
			setB:    map[string]*holdingInfo{"AAPL": {weight: d(0.05)}},
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
			got, _ := computeWeightedOverlap(tt.setA, tt.setB)
			gotF, _ := got.Float64()
			if !floatEq(gotF, tt.wantPct, 0.1) {
				t.Errorf("OverlapPct = %.2f, want %.2f", gotF, tt.wantPct)
			}
		})
	}
}

// --- isETF ---

func TestIsETF_WithTopHoldings(t *testing.T) {
	// Holding with TopHoldings populated should be treated as ETF,
	// regardless of QuoteType value.
	tests := []struct {
		name    string
		holding PortfolioHolding
		wantETF bool
	}{
		{
			name: "quote_type_etf_with_holdings",
			holding: PortfolioHolding{
				Symbol:      "VWRP",
				QuoteType:   "ETF",
				TopHoldings: []symbol.TopHolding{{Symbol: "AAPL", Percent: 5}},
			},
			wantETF: true,
		},
		{
			name: "empty_quote_type_with_holdings",
			holding: PortfolioHolding{
				Symbol:      "VWRP",
				QuoteType:   "", // older records may have empty QuoteType
				TopHoldings: []symbol.TopHolding{{Symbol: "AAPL", Percent: 5}},
			},
			wantETF: true,
		},
		{
			name: "equity_with_no_holdings",
			holding: PortfolioHolding{
				Symbol:    "AAPL",
				QuoteType: "EQUITY",
			},
			wantETF: false,
		},
		{
			name: "empty_quote_type_no_holdings",
			holding: PortfolioHolding{
				Symbol:    "AAPL",
				QuoteType: "",
			},
			wantETF: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isETF(tt.holding)
			if got != tt.wantETF {
				t.Errorf("isETF() = %v, want %v", got, tt.wantETF)
			}
		})
	}
}

// --- holdingKey ---

func TestHoldingKey(t *testing.T) {
	tests := []struct {
		name  string
		uh    symbol.TopHolding
		level keyLevel
		want  string
	}{
		{
			name:  "isin_level_returns_isin",
			uh:    symbol.TopHolding{Symbol: "AAPL", Name: "Apple Inc", ISIN: "US0378331005"},
			level: keyISIN,
			want:  "US0378331005",
		},
		{
			name:  "symbol_level_returns_uppercase_symbol",
			uh:    symbol.TopHolding{Symbol: "AAPL", Name: "Apple Inc", ISIN: "US0378331005"},
			level: keySymbol,
			want:  "AAPL",
		},
		{
			name:  "name_level_returns_normalized_name",
			uh:    symbol.TopHolding{Symbol: "AAPL", Name: "Apple Inc", ISIN: "US0378331005"},
			level: keyName,
			want:  "APPLE INC",
		},
		{
			name:  "name_level_strips_trailing_period",
			uh:    symbol.TopHolding{Symbol: "", Name: "Apple Inc."},
			level: keyName,
			want:  "APPLE INC",
		},
		{
			name:  "dash_isin_treated_as_empty_isin_level",
			uh:    symbol.TopHolding{Symbol: "", Name: "Cash Position", ISIN: "-"},
			level: keyISIN,
			want:  "",
		},
		{
			name:  "both_empty",
			uh:    symbol.TopHolding{Symbol: "", Name: ""},
			level: keyISIN,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := holdingKey(tt.uh, tt.level)
			if got != tt.want {
				t.Errorf("holdingKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"uppercase", "Apple Inc", "APPLE INC"},
		{"strip trailing period", "Apple Inc.", "APPLE INC"},
		{"trim whitespace", "  Apple Inc  ", "APPLE INC"},
		{"mixed case with period", "  Apple Inc.  ", "APPLE INC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeName(tt.in)
			if got != tt.want {
				t.Errorf("normalizeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMinKeyLevel(t *testing.T) {
	tests := []struct {
		name string
		a    []PortfolioHolding
		b    []PortfolioHolding
		want keyLevel
	}{
		{
			name: "both have isins",
			a: []PortfolioHolding{
				portfolioHoldingETF(t, "VOO", 1.0, topHoldingsWithISIN(
					[]string{"AAPL", "MSFT"},
					[]string{"US0378331005", "US5949181045"},
					[]float64{5, 4},
					[]string{"Apple", "Microsoft"},
				)),
			},
			b: []PortfolioHolding{
				portfolioHoldingETF(t, "IVV", 1.0, topHoldingsWithISIN(
					[]string{"AAPL", "MSFT"},
					[]string{"US0378331005", "US5949181045"},
					[]float64{5, 4},
					[]string{"Apple", "Microsoft"},
				)),
			},
			want: keyISIN,
		},
		{
			name: "one has isin other has only names",
			a: []PortfolioHolding{
				portfolioHoldingETF(t, "VOO", 1.0, topHoldingsWithISIN(
					[]string{"AAPL", "MSFT"},
					[]string{"US0378331005", "US5949181045"},
					[]float64{5, 4},
					[]string{"Apple", "Microsoft"},
				)),
			},
			b: []PortfolioHolding{
				portfolioHoldingETF(t, "IVV", 1.0, topHoldings(
					[]string{"", ""},
					[]float64{5, 4},
					[]string{"Apple Inc.", "Microsoft Corp"},
				)),
			},
			want: keyName,
		},
		{
			name: "both have only symbols",
			a: []PortfolioHolding{
				portfolioHoldingETF(t, "VOO", 1.0, topHoldings(
					[]string{"AAPL", "MSFT"},
					[]float64{5, 4},
					[]string{"Apple", "Microsoft"},
				)),
			},
			b: []PortfolioHolding{
				portfolioHoldingETF(t, "IVV", 1.0, topHoldings(
					[]string{"AAPL", "MSFT"},
					[]float64{5, 4},
					[]string{"Apple", "Microsoft"},
				)),
			},
			want: keySymbol,
		},
		{
			name: "direct holdings default to symbol",
			a: []PortfolioHolding{
				portfolioHoldingStock(t, "AAPL", 0.5, "Apple"),
			},
			b: []PortfolioHolding{
				portfolioHoldingStock(t, "MSFT", 0.5, "Microsoft"),
			},
			want: keySymbol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minKeyLevel(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("minKeyLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOverlap_ISINvsName_DegradesToName(t *testing.T) {
	// One fund has ISINs, the other has only names.
	// Should degrade both to name level and match via normalized names.
	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingETF(t, "VOO", 1.0, topHoldingsWithISIN(
				[]string{"AAPL", "MSFT", "GOOGL"},
				[]string{"US0378331005", "US5949181045", "US02079K3059"},
				[]float64{5, 4, 3},
				[]string{"Apple Inc", "Microsoft Corp", "Alphabet Inc"},
			)),
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingETF(t, "IVV", 1.0, topHoldings(
				[]string{"", "", ""},
				[]float64{5, 4, 3},
				[]string{"Apple Inc.", "Microsoft Corp", "Amazon.com Inc"},
			)),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// At name level: APPLE INC, MICROSOFT CORP match
	// Weighted overlap = min(0.05,0.05) + min(0.04,0.04) = 0.09 = 9%
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 9.0, 0.1) {
		t.Errorf("OverlapPct = %.2f, want 9.0", overlapF)
	}
}

// topHoldingsWithISIN creates TopHoldings with ISIN populated.
func topHoldingsWithISIN(symbols, isins []string, weights []float64, names []string) []symbol.TopHolding {
	var holdings []symbol.TopHolding
	for i := range symbols {
		holdings = append(holdings, symbol.TopHolding{
			Symbol:  symbols[i],
			ISIN:    isins[i],
			Percent: weights[i],
			Name:    names[i],
		})
	}
	return holdings
}

// --- ETF expansion with empty Symbol in TopHoldings (the real-world bug) ---

func TestComputeCrossPortfolioOverlap_EmptySymbolInTopHoldings(t *testing.T) {
	// Simulates the real-world case: TopHoldings are populated but Symbol
	// field is empty (extractor bug). Name field is populated.
	//
	// Portfolio: 100% VWRP
	// VWRP holds: NVIDIA Corp 4.58%, Apple Inc 3.83%, Alphabet 3.20%
	//
	// Expected expanded: NVIDIA Corp = 0.0458, Apple Inc = 0.0383, Alphabet = 0.032

	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			{
				Symbol:    "VWRP",
				Weight:    decF(1.0),
				Name:      "Vanguard FTSE All-World UCITS",
				QuoteType: "ETF",
				TopHoldings: []symbol.TopHolding{
					{Symbol: "", Name: "NVIDIA Corp", Percent: 4.58302},
					{Symbol: "", Name: "Apple Inc", Percent: 3.83449},
					{Symbol: "", Name: "Alphabet Inc", Percent: 3.2},
				},
			},
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 1.0, "Apple"),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Should be expanded to 3 separate holdings, not collapsed into one.
	if len(result.TopHoldingsA) != 3 {
		t.Fatalf("TopHoldingsA len = %d, want 3 (holdings should not collapse when Symbol is empty)", len(result.TopHoldingsA))
	}

	// NVIDIA Corp = 1.0 * 4.58302 / 100 = 0.04583
	nvidiaW, _ := result.TopHoldingsA[0].Weight.Float64()
	if !floatEq(nvidiaW, 0.04583, 0.001) {
		t.Errorf("TopHoldingsA[0] (NVIDIA) weight = %.4f, want 0.04583", nvidiaW)
	}

	// Apple Inc = 1.0 * 3.83449 / 100 = 0.03834
	appleW, _ := result.TopHoldingsA[1].Weight.Float64()
	if !floatEq(appleW, 0.03834, 0.001) {
		t.Errorf("TopHoldingsA[1] (Apple) weight = %.4f, want 0.03834", appleW)
	}

	// Alphabet = 1.0 * 3.2 / 100 = 0.032
	alphabetW, _ := result.TopHoldingsA[2].Weight.Float64()
	if !floatEq(alphabetW, 0.032, 0.001) {
		t.Errorf("TopHoldingsA[2] (Alphabet) weight = %.4f, want 0.032", alphabetW)
	}

	// Overlap: A has {NVIDIA Corp, Apple Inc, Alphabet Inc}, B has {AAPL}
	// No common symbols → 0% overlap
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 0.0, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 0.0", overlapF)
	}
}

// --- ETF expansion with empty QuoteType (the real-world bug) ---

func TestComputeCrossPortfolioOverlap_EmptyQuoteTypeExpandedByTopHoldings(t *testing.T) {
	// Simulates the real-world case: symbol details stored before QuoteType
	// was added to extractResultToSymbolDetails. QuoteType is empty but
	// TopHoldings is populated. The holding should still be expanded.
	//
	// Portfolio: 100% VWRP (QuoteType empty, TopHoldings populated)
	// VWRP holds: AAPL 4.5%, MSFT 3.8%, GOOGL 3.2%
	//
	// Expected expanded: AAPL = 0.045, MSFT = 0.038, GOOGL = 0.032

	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			{
				Symbol:    "VWRP",
				Weight:    decF(1.0),
				Name:      "Vanguard FTSE All-World UCITS",
				QuoteType: "", // empty — stored before QuoteType was added
				TopHoldings: topHoldings(
					[]string{"AAPL", "MSFT", "GOOGL"},
					[]float64{4.5, 3.8, 3.2},
					[]string{"Apple", "Microsoft", "Alphabet"},
				),
			},
		},
		PortfolioB: []PortfolioHolding{
			portfolioHoldingStock(t, "AAPL", 1.0, "Apple"),
		},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Should be expanded to 3 underlying holdings, not treated as atomic.
	if len(result.TopHoldingsA) != 3 {
		t.Fatalf("TopHoldingsA len = %d, want 3 (ETF should be expanded despite empty QuoteType)", len(result.TopHoldingsA))
	}

	// AAPL = 1.0 * 4.5 / 100 = 0.045
	aaplW, _ := result.TopHoldingsA[0].Weight.Float64()
	if !floatEq(aaplW, 0.045, 0.001) {
		t.Errorf("TopHoldingsA[0] (AAPL) weight = %.4f, want 0.045", aaplW)
	}

	// MSFT = 1.0 * 3.8 / 100 = 0.038
	msftW, _ := result.TopHoldingsA[1].Weight.Float64()
	if !floatEq(msftW, 0.038, 0.001) {
		t.Errorf("TopHoldingsA[1] (MSFT) weight = %.4f, want 0.038", msftW)
	}

	// GOOGL = 1.0 * 3.2 / 100 = 0.032
	googlW, _ := result.TopHoldingsA[2].Weight.Float64()
	if !floatEq(googlW, 0.032, 0.001) {
		t.Errorf("TopHoldingsA[2] (GOOGL) weight = %.4f, want 0.032", googlW)
	}

	// Weighted overlap = min(AAPL_A, AAPL_B) = min(0.045, 1.0) = 0.045 = 4.5%
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 4.5, 0.1) {
		t.Errorf("OverlapPct = %.2f, want 4.5", overlapF)
	}
}

// --- decEq helper test ---

func TestDecEq(t *testing.T) {
	a := decF(1.234)
	b := decF(1.235)
	decEq(t, a, b, 0.01) // should pass
}

// --- Task 6 integration: all new OverlapResult fields populated ---

func TestComputeCrossPortfolioOverlap_EnhancedFieldsPopulated(t *testing.T) {
	// Portfolio A: ETF with sector + geographic data + stock with sector
	// Portfolio B: ETF with sector + geographic data + stock with sector

	sectorA := []symbol.SectorWeighting{
		{Sector: "Technology", Percent: 50},
		{Sector: "Finance", Percent: 30},
		{Sector: "Healthcare", Percent: 20},
	}
	geoA := []symbol.GeographicAllocation{
		{Country: "United States", Percent: 60},
		{Country: "Germany", Percent: 25},
		{Country: "Japan", Percent: 15},
	}

	sectorB := []symbol.SectorWeighting{
		{Sector: "Technology", Percent: 40},
		{Sector: "Finance", Percent: 40},
		{Sector: "Energy", Percent: 20},
	}
	geoB := []symbol.GeographicAllocation{
		{Country: "United States", Percent: 70},
		{Country: "United Kingdom", Percent: 20},
		{Country: "France", Percent: 10},
	}

	input := CrossPortfolioOverlapInput{
		PortfolioA:   []PortfolioHolding{
			portfolioHoldingETFWithSector(t, "VOO", 0.6, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{5, 4},
				[]string{"Apple", "Microsoft"},
			), sectorA, geoA),
			portfolioHoldingStockWithSector(t, "AAPL", 0.4, "Apple", "Technology"),
		},
		PortfolioAName: "My Model",
		PortfolioB:   []PortfolioHolding{
			portfolioHoldingETFWithSector(t, "IVV", 0.5, topHoldings(
				[]string{"AAPL", "MSFT"},
				[]float64{4.5, 3.5},
				[]string{"Apple", "Microsoft"},
			), sectorB, geoB),
			portfolioHoldingStockWithSector(t, "MSFT", 0.5, "Microsoft", "Technology"),
		},
		PortfolioBName: "My Real",
	}

	result := ComputeCrossPortfolioOverlap(input)

	// --- Existing fields still present ---
	if len(result.TopHoldingsA) == 0 {
		t.Error("TopHoldingsA is empty")
	}
	if len(result.TopHoldingsB) == 0 {
		t.Error("TopHoldingsB is empty")
	}
	if result.OverlapPct == nil {
		t.Error("OverlapPct is nil")
	}

	// --- Sector allocation ---
	if result.SectorAllocationA == nil {
		t.Fatal("SectorAllocationA is nil")
	}
	if len(result.SectorAllocationA.Breakdown) == 0 {
		t.Error("SectorAllocationA.Breakdown is empty")
	}
	// VOO (0.6) contributes: Tech 0.6*50/100=0.3, Finance 0.6*30/100=0.18, Healthcare 0.6*20/100=0.12
	// AAPL (0.4) contributes: Tech 0.4
	// Total Tech = 0.3+0.4 = 0.7
	if tech, ok := result.SectorAllocationA.Breakdown["Technology"]; !ok || !floatEq(tech, 0.7, 0.01) {
		t.Errorf("SectorAllocationA Technology = %v, want ~0.7", tech)
	}

	if result.SectorAllocationB == nil {
		t.Fatal("SectorAllocationB is nil")
	}
	if len(result.SectorAllocationB.Breakdown) == 0 {
		t.Error("SectorAllocationB.Breakdown is empty")
	}

	// --- Country allocation ---
	if result.CountryAllocationA == nil {
		t.Fatal("CountryAllocationA is nil")
	}
	if len(result.CountryAllocationA.Breakdown) == 0 {
		t.Error("CountryAllocationA.Breakdown is empty")
	}
	// VOO (0.6) contributes: US 0.6*60/100=0.36, Germany 0.6*25/100=0.15, Japan 0.6*15/100=0.09
	// AAPL has no geographic data → unknown
	if us, ok := result.CountryAllocationA.Breakdown["United States"]; !ok || !floatEq(us, 0.36, 0.01) {
		t.Errorf("CountryAllocationA United States = %v, want ~0.36", us)
	}
	// AAPL has no geo data so unknown should be > 0
	if result.CountryAllocationA.UnknownWeightPct == 0 {
		t.Error("CountryAllocationA.UnknownWeightPct should be > 0 (AAPL has no geo data)")
	}

	// Verify portfolio names appear in warnings (not generic "A"/"B")
	foundNameWarning := false
	for _, w := range result.Warnings {
		if strings.HasPrefix(w, "[country My Model]") {
			foundNameWarning = true
			break
		}
	}
	if !foundNameWarning {
		t.Errorf("Expected warning with portfolio name 'My Model', got warnings: %v", result.Warnings)
	}

	if result.CountryAllocationB == nil {
		t.Fatal("CountryAllocationB is nil")
	}
	if len(result.CountryAllocationB.Breakdown) == 0 {
		t.Error("CountryAllocationB.Breakdown is empty")
	}

	// --- Merged holdings ---
	if len(result.MergedHoldings) == 0 {
		t.Error("MergedHoldings is empty")
	}
	// Shared holdings (AAPL, MSFT) should come first
	if len(result.MergedHoldings) > 0 && result.MergedHoldings[0].OverlapPct == 0 {
		t.Error("First merged holding should be shared (overlap > 0)")
	}

	// --- Overweight/underweight/neutral ---
	// At least one of these should be non-empty
	hasDiff := len(result.OverweightHoldings) > 0 ||
		len(result.UnderweightHoldings) > 0 ||
		len(result.NeutralHoldings) > 0
	if !hasDiff {
		t.Error("All of Overweight/Underweight/Neutral holdings are empty")
	}

	// Verify overweight/underweight signs
	for _, h := range result.OverweightHoldings {
		if h.Difference <= 0 {
			t.Errorf("Overweight holding %s has non-positive difference %f", h.Symbol, h.Difference)
		}
	}
	for _, h := range result.UnderweightHoldings {
		if h.Difference >= 0 {
			t.Errorf("Underweight holding %s has non-negative difference %f", h.Symbol, h.Difference)
		}
	}
}

func TestComputeCrossPortfolioOverlap_EnhancedFields_EmptyPortfolio(t *testing.T) {
	// Edge case: empty portfolio — enhanced fields should handle gracefully
	input := CrossPortfolioOverlapInput{
		PortfolioA: []PortfolioHolding{
			portfolioHoldingStockWithSector(t, "AAPL", 1.0, "Apple", "Technology"),
		},
		PortfolioB: []PortfolioHolding{},
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Sector/country for A should still be populated
	if result.SectorAllocationA == nil || len(result.SectorAllocationA.Breakdown) == 0 {
		t.Error("SectorAllocationA should be populated even when B is empty")
	}

	// Sector/country for B should have empty-state message
	if result.SectorAllocationB == nil || result.SectorAllocationB.Message == "" {
		t.Error("SectorAllocationB should have empty-state message")
	}
	if result.CountryAllocationB == nil || result.CountryAllocationB.Message == "" {
		t.Error("CountryAllocationB should have empty-state message")
	}

	// Merged/weight diff should still work with one empty side
	if len(result.MergedHoldings) == 0 {
		t.Error("MergedHoldings should have entries from portfolio A")
	}
}

func TestComputeCrossPortfolioOverlap_EnhancedFields_IdenticalPortfolios(t *testing.T) {
	// Edge case: identical portfolios — zero drift, all neutral
	// Use direct stock holdings so overlap sums to 100%
	shared := []PortfolioHolding{
		portfolioHoldingStockWithSector(t, "AAPL", 0.4, "Apple", "Technology"),
		portfolioHoldingStockWithSector(t, "MSFT", 0.35, "Microsoft", "Technology"),
		portfolioHoldingStockWithSector(t, "JNJ", 0.25, "J&J", "Healthcare"),
	}

	input := CrossPortfolioOverlapInput{
		PortfolioA: shared,
		PortfolioB: shared,
	}

	result := ComputeCrossPortfolioOverlap(input)

	// Overlap should be 100%
	if result.OverlapPct == nil {
		t.Fatal("OverlapPct is nil")
	}
	overlapF, _ := result.OverlapPct.Float64()
	if !floatEq(overlapF, 100.0, 0.01) {
		t.Errorf("OverlapPct = %.2f, want 100.0", overlapF)
	}

	// All holdings should be neutral (zero difference)
	if len(result.OverweightHoldings) > 0 {
		t.Errorf("OverweightHoldings should be empty for identical portfolios, got %d", len(result.OverweightHoldings))
	}
	if len(result.UnderweightHoldings) > 0 {
		t.Errorf("UnderweightHoldings should be empty for identical portfolios, got %d", len(result.UnderweightHoldings))
	}
	if len(result.NeutralHoldings) == 0 {
		t.Error("NeutralHoldings should be non-empty for identical portfolios")
	}

	// Sector allocations should be identical
	if len(result.SectorAllocationA.Breakdown) != len(result.SectorAllocationB.Breakdown) {
		t.Error("Sector allocations should have same number of entries")
	}
	for sector, pctA := range result.SectorAllocationA.Breakdown {
		pctB, ok := result.SectorAllocationB.Breakdown[sector]
		if !ok || !floatEq(pctA, pctB, 0.01) {
			t.Errorf("Sector %s: A=%.4f, B=%.4f", sector, pctA, pctB)
		}
	}
}
