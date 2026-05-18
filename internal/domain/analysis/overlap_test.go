package analysis

import (
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// --- helpers ---

func etfPosition(t *testing.T, sym string, weight float64, holdings []symbol.TopHolding) PositionWithDetails {
	t.Helper()
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: weight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType:     "ETF",
			TopHoldings:   holdings,
		},
	}
}

func stockPosition(t *testing.T, sym string, weight float64) PositionWithDetails {
	t.Helper()
	return PositionWithDetails{
		Symbol:          sym,
		PortfolioWeight: weight,
		SymbolDetails: &symbol.SymbolDetails{
			QuoteType: "EQUITY",
		},
	}
}

func holdings(symbols []string, percents []float64, names []string) []symbol.TopHolding {
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

// --- ComputeOverlap ---

func TestComputeOverlap(t *testing.T) {
	tests := []struct {
		name           string
		positions      []PositionWithDetails
		wantPairs      []OverlapPair
		wantStocks     []ConcentratedStock
		wantMessage    string
		wantWarningLen int
	}{
		{
			name: "3 ETFs with known overlapping holdings",
			positions: []PositionWithDetails{
				// VOO: 30% of portfolio, holds AAPL 5%, MSFT 4%, GOOGL 3%
				etfPosition(t, "VOO", 30, holdings(
					[]string{"AAPL", "MSFT", "GOOGL"},
					[]float64{5, 4, 3},
					[]string{"Apple", "Microsoft", "Alphabet"},
				)),
				// IVV: 20% of portfolio, holds AAPL 4.5%, MSFT 3.5%, AMZN 3%
				etfPosition(t, "IVV", 20, holdings(
					[]string{"AAPL", "MSFT", "AMZN"},
					[]float64{4.5, 3.5, 3},
					[]string{"Apple", "Microsoft", "Amazon"},
				)),
				// QQQ: 15% of portfolio, holds AAPL 6%, MSFT 5%, GOOGL 4%, AMZN 3.5%
				etfPosition(t, "QQQ", 15, holdings(
					[]string{"AAPL", "MSFT", "GOOGL", "AMZN"},
					[]float64{6, 5, 4, 3.5},
					[]string{"Apple", "Microsoft", "Alphabet", "Amazon"},
				)),
			},
			wantPairs: []OverlapPair{
				// VOO-IVV: AAPL, MSFT = 2 overlapping → 2.4 + 1.9 = 4.3
				{ETFA: "VOO", ETFB: "IVV", OverlappingCount: 2, CombinedWeightPct: 4.3},
				// VOO-QQQ: AAPL, MSFT, GOOGL = 3 overlapping → 2.4 + 1.95 + 1.5 = 5.85
				{ETFA: "VOO", ETFB: "QQQ", OverlappingCount: 3, CombinedWeightPct: 5.85},
				// IVV-QQQ: AAPL, MSFT, AMZN = 3 overlapping → 1.8 + 1.45 + 1.125 = 4.375 → 4.38
				{ETFA: "IVV", ETFB: "QQQ", OverlappingCount: 3, CombinedWeightPct: 4.38},
			},
			wantStocks: nil, // checked separately below
			wantMessage:    "",
			wantWarningLen: 0,
		},

		{
			name: "zero overlap between some pairs",
			positions: []PositionWithDetails{
				etfPosition(t, "ETF_A", 50, holdings(
					[]string{"AAPL", "MSFT"},
					[]float64{5, 5},
					[]string{"Apple", "Microsoft"},
				)),
				etfPosition(t, "ETF_B", 50, holdings(
					[]string{"JNJ", "PFE"},
					[]float64{4, 4},
					[]string{"Johnson & Johnson", "Pfizer"},
				)),
			},
			wantPairs: []OverlapPair{
				{ETFA: "ETF_A", ETFB: "ETF_B", OverlappingCount: 0, CombinedWeightPct: 0},
			},
			wantStocks:     nil,
			wantMessage:    "",
			wantWarningLen: 0,
		},

		{
			name: "single ETF returns message for pairwise but still computes concentrated stocks",
			positions: []PositionWithDetails{
				etfPosition(t, "VOO", 50, holdings(
					[]string{"AAPL", "MSFT"},
					[]float64{5, 4},
					[]string{"Apple", "Microsoft"},
				)),
			},
			wantPairs:      []OverlapPair{},
			wantStocks:     nil, // checked separately
			wantMessage:    "Only 1 unique ETF found. ETF overlap requires at least 2 ETFs for pairwise comparison.",
			wantWarningLen: 0,
		},

		{
			name:           "no ETFs returns message",
			positions: []PositionWithDetails{
				stockPosition(t, "AAPL", 40),
				stockPosition(t, "MSFT", 30),
			},
			wantPairs:      []OverlapPair{},
			wantStocks:     []ConcentratedStock{},
			wantMessage:    "No ETF positions to analyze. ETF overlap requires at least one ETF.",
			wantWarningLen: 0,
		},

		{
			name: "ETFs with no cached holdings generates warning",
			positions: []PositionWithDetails{
				etfPosition(t, "VOO", 30, holdings(
					[]string{"AAPL"},
					[]float64{5},
					[]string{"Apple"},
				)),
				etfPosition(t, "OBLIVION", 20, holdings([]string{}, []float64{}, []string{})),
			},
			wantPairs: []OverlapPair{
				{ETFA: "VOO", ETFB: "OBLIVION", OverlappingCount: 0, CombinedWeightPct: 0},
			},
			wantStocks:     nil,
			wantMessage:    "",
			wantWarningLen: 1,
		},

		{
			name: "stocks are filtered out, only ETFs used",
			positions: []PositionWithDetails{
				stockPosition(t, "AAPL", 40),
				etfPosition(t, "VOO", 30, holdings(
					[]string{"AAPL", "MSFT"},
					[]float64{5, 4},
					[]string{"Apple", "Microsoft"},
				)),
				stockPosition(t, "MSFT", 10),
				etfPosition(t, "IVV", 20, holdings(
					[]string{"AAPL", "GOOGL"},
					[]float64{4.5, 3},
					[]string{"Apple", "Alphabet"},
				)),
			},
			wantPairs: []OverlapPair{
				// VOO-IVV: AAPL only
				// AAPL: 30*5/100 + 20*4.5/100 = 1.5+0.9 = 2.4
				{ETFA: "VOO", ETFB: "IVV", OverlappingCount: 1, CombinedWeightPct: 2.4},
			},
			wantStocks:     nil,
			wantMessage:    "",
			wantWarningLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeOverlap(tt.positions)

			// Check message.
			if got.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMessage)
			}

			// Check warnings count.
			if len(got.Warnings) != tt.wantWarningLen {
				t.Errorf("Warnings len = %d, want %d; warnings = %v", len(got.Warnings), tt.wantWarningLen, got.Warnings)
			}

			// Check pairwise matrix.
			if len(got.PairwiseMatrix) != len(tt.wantPairs) {
				t.Errorf("PairwiseMatrix len = %d, want %d", len(got.PairwiseMatrix), len(tt.wantPairs))
				return
			}
			for i, want := range tt.wantPairs {
				got := got.PairwiseMatrix[i]
				if got.ETFA != want.ETFA || got.ETFB != want.ETFB {
					t.Errorf("Pair %d: (%s, %s), want (%s, %s)", i, got.ETFA, got.ETFB, want.ETFA, want.ETFB)
				}
				if got.OverlappingCount != want.OverlappingCount {
					t.Errorf("Pair %d: OverlappingCount = %d, want %d", i, got.OverlappingCount, want.OverlappingCount)
				}
				if !floatEq(got.CombinedWeightPct, want.CombinedWeightPct, 0.01) {
					t.Errorf("Pair %d: CombinedWeightPct = %.4f, want %.4f", i, got.CombinedWeightPct, want.CombinedWeightPct)
				}
			}
		})
	}
}

func TestComputeOverlap_ConcentratedStocks(t *testing.T) {
	tests := []struct {
		name       string
		positions  []PositionWithDetails
		wantCount  int
		wantTopSym string // top stock symbol
		wantTopWt  float64
	}{
		{
			name: "top concentrated stocks correctly aggregated",
			positions: []PositionWithDetails{
				etfPosition(t, "VOO", 30, holdings(
					[]string{"AAPL", "MSFT", "GOOGL"},
					[]float64{5, 4, 3},
					[]string{"Apple", "Microsoft", "Alphabet"},
				)),
				etfPosition(t, "IVV", 20, holdings(
					[]string{"AAPL", "MSFT", "AMZN"},
					[]float64{4.5, 3.5, 3},
					[]string{"Apple", "Microsoft", "Amazon"},
				)),
				etfPosition(t, "QQQ", 15, holdings(
					[]string{"AAPL", "MSFT", "GOOGL", "AMZN"},
					[]float64{6, 5, 4, 3.5},
					[]string{"Apple", "Microsoft", "Alphabet", "Amazon"},
				)),
			},
			// Unique: AAPL, MSFT, GOOGL, AMZN = 4
			wantCount:  4,
			wantTopSym: "AAPL",
			// AAPL total weight: 30*5/100 + 20*4.5/100 + 15*6/100 = 1.5+0.9+0.9 = 3.3
			wantTopWt: 3.3,
		},

		{
			name: "single position — 100% in one stock",
			positions: []PositionWithDetails{
				etfPosition(t, "VOO", 100, holdings(
					[]string{"AAPL"},
					[]float64{10},
					[]string{"Apple"},
				)),
			},
			wantCount:  1,
			wantTopSym: "AAPL",
			wantTopWt:  10, // 100*10/100 = 10
		},

		{
			name: "more than 10 stocks — capped at 10",
			positions: func() []PositionWithDetails {
				var positions []PositionWithDetails
				// Create 3 ETFs each holding 5 unique stocks = 15 unique
				for i, etf := range []string{"ETF1", "ETF2", "ETF3"} {
					syms := []string{}
					pcts := []float64{}
					names := []string{}
					for j := 0; j < 5; j++ {
						idx := i*5 + j
						syms = append(syms, "SYM"+string(rune('0'+idx)))
						pcts = append(pcts, float64(10-j*2)) // 10, 8, 6, 4, 2
						names = append(names, "Stock "+string(rune('0'+idx)))
					}
					positions = append(positions, etfPosition(t, etf, 30, holdings(syms, pcts, names)))
				}
				return positions
			}(),
			wantCount:  10, // capped
			wantTopSym: "", // just check count
			wantTopWt:  0,
		},

		{
			name: "no ETFs — empty concentrated stocks",
			positions: []PositionWithDetails{
				stockPosition(t, "AAPL", 50),
			},
			wantCount: 0,
		},

		{
			name: "ETFs with no holdings — empty",
			positions: []PositionWithDetails{
				etfPosition(t, "ETF1", 50, holdings([]string{}, []float64{}, []string{})),
				etfPosition(t, "ETF2", 50, holdings([]string{}, []float64{}, []string{})),
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeOverlap(tt.positions)

			if len(got.TopConcentratedStocks) != tt.wantCount {
				t.Errorf("TopConcentratedStocks len = %d, want %d", len(got.TopConcentratedStocks), tt.wantCount)
			}

			if tt.wantCount > 0 && tt.wantTopSym != "" {
				top := got.TopConcentratedStocks[0]
				if top.Symbol != tt.wantTopSym {
					t.Errorf("Top stock symbol = %q, want %q", top.Symbol, tt.wantTopSym)
				}
				if !floatEq(top.TotalWeightPct, tt.wantTopWt, 0.01) {
					t.Errorf("Top stock weight = %.4f, want %.4f", top.TotalWeightPct, tt.wantTopWt)
				}
			}

			// Verify sorted descending.
			for i := 1; i < len(got.TopConcentratedStocks); i++ {
				if got.TopConcentratedStocks[i].TotalWeightPct > got.TopConcentratedStocks[i-1].TotalWeightPct {
					t.Errorf("Stocks not sorted descending: index %d (%.2f) > index %d (%.2f)",
						i, got.TopConcentratedStocks[i].TotalWeightPct,
						i-1, got.TopConcentratedStocks[i-1].TotalWeightPct)
				}
			}
		})
	}
}

// floatEq checks two float64 values are within epsilon.
func floatEq(got, want, eps float64) bool {
	return (got - want) < eps && (want - got) < eps
}

func TestComputeOverlap_DuplicateETFAggregated(t *testing.T) {
	// Same ETF held in 3 accounts should produce unique pairs only.
	holdingsA := []symbol.TopHolding{
		{Symbol: "AAPL", Percent: 10, Name: "Apple"},
		{Symbol: "MSFT", Percent: 8, Name: "Microsoft"},
	}
	holdingsB := []symbol.TopHolding{
		{Symbol: "GOOGL", Percent: 12, Name: "Alphabet"},
		{Symbol: "AAPL", Percent: 6, Name: "Apple"},
	}

	positions := []PositionWithDetails{
		etfPosition(t, "ETF_A", 10, holdingsA), // account 1
		etfPosition(t, "ETF_A", 8, holdingsA),  // account 2
		etfPosition(t, "ETF_A", 6, holdingsA),  // account 3
		etfPosition(t, "ETF_B", 12, holdingsB), // account 1
	}

	result := ComputeOverlap(positions)

	// Should produce exactly 1 pair (ETF_A vs ETF_B), not 6.
	if len(result.PairwiseMatrix) != 1 {
		t.Fatalf("pairwise matrix len = %d, want 1", len(result.PairwiseMatrix))
	}

	pair := result.PairwiseMatrix[0]
	if pair.ETFA != "ETF_A" || pair.ETFB != "ETF_B" {
		t.Errorf("pair = %s vs %s, want ETF_A vs ETF_B", pair.ETFA, pair.ETFB)
	}

	// AAPL is the only overlapping holding.
	// Combined weight = 24*10/100 + 12*6/100 = 2.4 + 0.72 = 3.12
	if !floatEq(pair.CombinedWeightPct, 3.12, 0.01) {
		t.Errorf("combined weight = %.2f, want 3.12", pair.CombinedWeightPct)
	}
	if pair.OverlappingCount != 1 {
		t.Errorf("overlapping count = %d, want 1", pair.OverlappingCount)
	}
}

func TestAggregateBySymbol(t *testing.T) {
	holdings := []symbol.TopHolding{
		{Symbol: "AAPL", Percent: 10, Name: "Apple"},
	}

	positions := []PositionWithDetails{
		etfPosition(t, "ETF_A", 10, holdings),
		etfPosition(t, "ETF_B", 20, holdings),
		etfPosition(t, "ETF_A", 5, holdings),
		etfPosition(t, "ETF_B", 3, holdings),
		etfPosition(t, "ETF_C", 7, holdings),
	}

	result := aggregateBySymbol(positions)

	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}

	// Check combined weights.
	want := map[string]float64{
		"ETF_A": 15, // 10 + 5
		"ETF_B": 23, // 20 + 3
		"ETF_C": 7,
	}
	for _, p := range result {
		w, ok := want[p.Symbol]
		if !ok {
			t.Errorf("unexpected symbol %q", p.Symbol)
			continue
		}
		if !floatEq(p.PortfolioWeight, w, 0.01) {
			t.Errorf("%s weight = %.2f, want %.2f", p.Symbol, p.PortfolioWeight, w)
		}
	}
}
