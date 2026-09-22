package comparison

import (
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- helpers ---

func day(t *testing.T, y int, m time.Month, d int) time.Time {
	t.Helper()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func prices(t *testing.T, base time.Time, values []float64, currency string) []market.HistoricalPrice {
	t.Helper()
	prices := make([]market.HistoricalPrice, len(values))
	for i, v := range values {
		c, _ := decimal.NewFromFloat64(v)
		prices[i] = market.HistoricalPrice{
			Date:     base.AddDate(0, 0, i),
			Close:    c,
			Currency: currency,
		}
	}
	return prices
}

func weight(t *testing.T, symbol, marketSym, currency string, weightFrac float64) ModelPortfolioWeight {
	t.Helper()
	w, _ := decimal.NewFromFloat64(weightFrac)
	return ModelPortfolioWeight{
		Symbol:    symbol,
		Weight:    w,
		Currency:  currency,
		MarketSym: marketSym,
	}
}

func assertCurveLen(t *testing.T, name string, got []EquityCurvePoint, want int) {
	t.Helper()
	if len(got) != want {
		t.Errorf("%s: curve length = %d, want %d", name, len(got), want)
	}
}

// assertCurveValueApprox checks that curve[idx].PortfolioValue is within 0.01 of wantF.
func assertCurveValueApprox(t *testing.T, name string, curve []EquityCurvePoint, idx int, wantF float64) {
	t.Helper()
	if idx >= len(curve) {
		t.Errorf("%s: index %d out of range (len=%d)", name, idx, len(curve))
		return
	}
	want, _ := decimal.NewFromFloat64(wantF)
	gotF, _ := curve[idx].PortfolioValue.Float64()
	wantF64, _ := want.Float64()
	diff := gotF - wantF64
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.01 {
		t.Errorf("%s[%d]: PortfolioValue = %v (%v), want ~%v", name, idx, curve[idx].PortfolioValue, gotF, wantF)
	}
}

// pricesAt builds a price series for explicit dates (used when the trading
// days are not consecutive calendar days, e.g. a gap across a month boundary).
func pricesAt(t *testing.T, dates []time.Time, values []float64, currency string) []market.HistoricalPrice {
	t.Helper()
	if len(dates) != len(values) {
		t.Fatalf("pricesAt: dates(%d) and values(%d) length mismatch", len(dates), len(values))
	}
	out := make([]market.HistoricalPrice, len(values))
	for i, v := range values {
		c, _ := decimal.NewFromFloat64(v)
		out[i] = market.HistoricalPrice{Date: dates[i], Close: c, Currency: currency}
	}
	return out
}

// fp returns a pointer to a float64 (for optional threshold test fields).
func fp(v float64) *float64 { return &v }

// --- SimulateEquityCurve ---

func TestSimulateEquityCurve(t *testing.T) {
	base := day(t, 2024, 1, 2) // Monday Jan 2, 2024

	tests := []struct {
		name               string
		startingValue      float64
		weights            []ModelPortfolioWeight
		pricesBySym        map[string][]market.HistoricalPrice
		fxRates            map[string][]market.HistoricalPrice
		baseCurrency       string
		dateFrom           time.Time
		dateTo             time.Time
		wantCurveLen       int
		wantFirstValue     float64
		wantLastValue      float64
		wantMinWarnings    int
		wantLimitedSymbols int
		wantPeriodClipped  bool
	}{
		{
			name:          "normal two-symbol portfolio",
			startingValue: 10000,
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 0.5),
				weight(t, "GOOG", "GOOG", "USD", 0.5),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, base, []float64{100, 102, 105, 103, 108}, "USD"),
				"GOOG": prices(t, base, []float64{150, 151, 153, 152, 155}, "USD"),
			},
			baseCurrency: "USD",
			wantCurveLen: 5,
			// Day 0: 5000*(100/100) + 5000*(150/150) = 5000 + 5000 = 10000
			wantFirstValue: 10000,
			// Day 4: 5000*(108/100) + 5000*(155/150) = 5400 + 5166.67 = 10566.67
			wantLastValue:      10566.67,
			wantMinWarnings:    0,
			wantLimitedSymbols: 0,
		},
		{
			name:          "single symbol",
			startingValue: 5000,
			weights: []ModelPortfolioWeight{
				weight(t, "MSFT", "MSFT", "USD", 1.0),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"MSFT": prices(t, base, []float64{300, 310, 305, 320}, "USD"),
			},
			baseCurrency:       "USD",
			wantCurveLen:       4,
			wantFirstValue:     5000,    // 5000 * 300/300
			wantLastValue:      5333.33, // 5000 * 320/300
			wantMinWarnings:    0,
			wantLimitedSymbols: 0,
		},
		{
			name:          "fx conversion",
			startingValue: 10000,
			weights: []ModelPortfolioWeight{
				weight(t, "BP.L", "BP.L", "GBP", 1.0),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"BP.L": prices(t, base, []float64{500, 510, 505, 520}, "GBP"),
			},
			fxRates: map[string][]market.HistoricalPrice{
				"GBP/USD": prices(t, base, []float64{1.27, 1.28, 1.275, 1.29}, "USD"),
			},
			baseCurrency:   "USD",
			wantCurveLen:   4,
			wantFirstValue: 12700, // 10000*(500/500)*1.27 = 10000*1.27
			// Day 3: 10000*(520/500)*1.29 = 10400*1.29 = 13416
			wantLastValue:      13416,
			wantMinWarnings:    0,
			wantLimitedSymbols: 0,
		},
		{
			name:          "missing data for some symbols — partial curve",
			startingValue: 10000,
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 0.5),
				weight(t, "META", "META", "USD", 0.5),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, base, []float64{100, 102, 105, 103, 108}, "USD"),
				// META has no data — curve includes AAPL only
			},
			baseCurrency:       "USD",
			wantCurveLen:       5,    // AAPL data is still available
			wantFirstValue:     5000, // 5000*(100/100) = 5000 (AAPL only, META skipped)
			wantLastValue:      5400, // 5000*(108/100) = 5400
			wantMinWarnings:    1,    // "no price data for META"
			wantLimitedSymbols: 0,
		},
		{
			name:            "empty weights",
			startingValue:   10000,
			weights:         []ModelPortfolioWeight{},
			pricesBySym:     map[string][]market.HistoricalPrice{},
			baseCurrency:    "USD",
			wantCurveLen:    0,
			wantMinWarnings: 1, // "no weights or zero starting value"
		},
		{
			name:          "zero starting value",
			startingValue: 0,
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 1.0),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, base, []float64{100, 102, 105}, "USD"),
			},
			baseCurrency:    "USD",
			wantCurveLen:    0,
			wantMinWarnings: 1, // "no weights or zero starting value"
		},
		{
			name:          "period clipping — one symbol has shorter history",
			startingValue: 10000,
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 0.5),
				weight(t, "TSLA", "TSLA", "USD", 0.5),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				// AAPL: Jan 2 - Jan 6 (5 days)
				"AAPL": prices(t, base, []float64{100, 102, 105, 103, 108}, "USD"),
				// TSLA: Jan 3 - Jan 6 (4 days, starts one day later)
				"TSLA": prices(t, base.AddDate(0, 0, 1), []float64{200, 205, 202, 210}, "USD"),
			},
			baseCurrency:       "USD",
			wantCurveLen:       4,     // clipped to Jan 3 - Jan 6
			wantFirstValue:     10100, // 5000*(102/100) + 5000*(200/200) = 5100 + 5000
			wantLastValue:      10650, // 5000*(108/100) + 5000*(210/200) = 5400 + 5250
			wantMinWarnings:    0,     // TSLA covers the full clipped period (Jan 3-6)
			wantLimitedSymbols: 0,
		},
		{
			name:          "symbol with shorter history — period clipped warning",
			startingValue: 10000,
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 0.5),
				weight(t, "META", "META", "USD", 0.5),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				// AAPL: Jan 2 - Jan 20 (19 days of data)
				"AAPL": prices(t, base, []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118}, "USD"),
				// META: Jan 10 - Jan 20 (11 days of data, only)
				"META": prices(t, base.AddDate(0, 0, 8), []float64{300, 305, 310, 308, 312, 315, 318, 320, 322, 325, 328}, "USD"),
			},
			baseCurrency: "USD",
			dateFrom:     base,                   // Jan 2 (requested full range)
			dateTo:       base.AddDate(0, 0, 18), // Jan 20
			wantCurveLen: 11,                     // clipped to Jan 10 - Jan 20 (META range)
			// AAPL base price = Jan 2 = 100, META base price = Jan 10 = 300
			// Jan 10: AAPL price = 108 (index 8), META price = 300
			// Jan 10: 5000*(108/100) + 5000*(300/300) = 5400 + 5000 = 10400
			wantFirstValue: 10400,
			// Jan 20: AAPL price = 118 (index 18), META price = 328 (index 10)
			// Jan 20: 5000*(118/100) + 5000*(328/300) = 5900 + 5466.67 = 11366.67
			wantLastValue:      11366.67,
			wantMinWarnings:    2, // period clipped warning + META limited
			wantLimitedSymbols: 1, // META
			wantPeriodClipped:  true,
		},
		{
			name:          "date range filtering",
			startingValue: 10000,
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 1.0),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, base, []float64{100, 102, 105, 103, 108}, "USD"),
			},
			baseCurrency:       "USD",
			dateFrom:           base.AddDate(0, 0, 1), // Jan 3
			dateTo:             base.AddDate(0, 0, 3), // Jan 5
			wantCurveLen:       3,
			wantFirstValue:     10200, // 10000 * 102/100 (base price from Jan 2)
			wantLastValue:      10300, // 10000 * 103/100 (Jan 5 price, base from Jan 2)
			wantMinWarnings:    0,
			wantLimitedSymbols: 0,
		},
		{
			name:          "fx fallback — no FX data available",
			startingValue: 10000,
			weights: []ModelPortfolioWeight{
				weight(t, "BP.L", "BP.L", "GBP", 1.0),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"BP.L": prices(t, base, []float64{500, 510, 505}, "GBP"),
			},
			fxRates:            map[string][]market.HistoricalPrice{}, // no FX data
			baseCurrency:       "USD",
			wantCurveLen:       3,
			wantFirstValue:     0, // no FX data means value skipped for all days
			wantLastValue:      0,
			wantMinWarnings:    1, // "no FX data for GBP/USD"
			wantLimitedSymbols: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startingValue, _ := decimal.NewFromFloat64(tt.startingValue)
			input := SimulateEquityCurveInput{
				StartingValue: startingValue,
				Weights:       tt.weights,
				PricesBySym:   tt.pricesBySym,
				BaseCurrency:  tt.baseCurrency,
				DateFrom:      tt.dateFrom,
				DateTo:        tt.dateTo,
				FxRates:       tt.fxRates,
			}

			output := SimulateEquityCurve(input)

			assertCurveLen(t, tt.name, output.EquityCurve, tt.wantCurveLen)

			if tt.wantCurveLen > 0 {
				assertCurveValueApprox(t, tt.name, output.EquityCurve, 0, tt.wantFirstValue)
				assertCurveValueApprox(t, tt.name, output.EquityCurve, tt.wantCurveLen-1, tt.wantLastValue)
				// NavPerUnit must be set for every point (model portfolio = single deposit).
				for i, p := range output.EquityCurve {
					if p.NavPerUnit == nil {
						t.Errorf("%s: point %d NavPerUnit is nil, want non-nil", tt.name, i)
					} else if !p.NavPerUnit.Equal(p.PortfolioValue) {
						t.Errorf("%s: point %d NavPerUnit = %v, want == PortfolioValue %v", tt.name, i, p.NavPerUnit, p.PortfolioValue)
					}
				}
			}

			if len(output.Warnings) < tt.wantMinWarnings {
				t.Errorf("%s: warnings = %d (got %v), want >= %d", tt.name, len(output.Warnings), output.Warnings, tt.wantMinWarnings)
			}

			if len(output.LimitedHistorySymbols) != tt.wantLimitedSymbols {
				t.Errorf("%s: limitedSymbols = %d (%v), want %d", tt.name, len(output.LimitedHistorySymbols), output.LimitedHistorySymbols, tt.wantLimitedSymbols)
			}

			if output.PeriodClipped != tt.wantPeriodClipped {
				t.Errorf("%s: PeriodClipped = %v, want %v", tt.name, output.PeriodClipped, tt.wantPeriodClipped)
			}
		})
	}
}

// --- SimulateEquityCurve (rebalanced) ---

func TestSimulateEquityCurveRebalanced(t *testing.T) {
	weights := []ModelPortfolioWeight{
		weight(t, "AAPL", "AAPL", "USD", 0.5),
		weight(t, "GOOG", "GOOG", "USD", 0.5),
	}
	startingValue, _ := decimal.NewFromFloat64(10000)

	// janDiverge: prices cross the Feb 1, 2024 monthly boundary (Jan 29 - Feb 3).
	// At the boundary AAPL has risen to 110 and GOOG fallen to 90 (5pp drift
	// each from the 50/50 target), then both keep moving so the strategies
	// diverge after the re-anchor.
	janDiverge := map[string][]market.HistoricalPrice{
		"AAPL": prices(t, day(t, 2024, 1, 29), []float64{100, 100, 100, 110, 121, 133.1}, "USD"),
		"GOOG": prices(t, day(t, 2024, 1, 29), []float64{100, 100, 100, 90, 81, 72.9}, "USD"),
	}

	tests := []struct {
		name              string
		pricesBySym       map[string][]market.HistoricalPrice
		interval          string
		thresholdPct      *float64 // drift threshold in percentage points; nil = unconditional
		wantCurveLen      int
		wantRebalancedEnd float64
		wantBuyAndHoldEnd float64
	}{
		{
			name:              "rebalanced diverges from buy-and-hold (monthly, no threshold)",
			pricesBySym:       janDiverge,
			interval:          IntervalMonthly,
			thresholdPct:      nil,
			wantCurveLen:      6,
			wantRebalancedEnd: 10100, // re-anchored to 50/50 at the Feb 1 boundary
			wantBuyAndHoldEnd: 10300, // held from the period start
		},
		{
			name:              "threshold suppresses rebalance when drift is below it",
			pricesBySym:       janDiverge,
			interval:          IntervalMonthly,
			thresholdPct:      fp(6), // 5pp drift < 6pp threshold -> boundary skipped
			wantCurveLen:      6,
			wantRebalancedEnd: 10300, // matches buy-and-hold
			wantBuyAndHoldEnd: 10300,
		},
		{
			name:              "threshold triggers rebalance when drift meets it",
			pricesBySym:       janDiverge,
			interval:          IntervalMonthly,
			thresholdPct:      fp(5), // 5pp drift >= 5pp threshold -> boundary fires
			wantCurveLen:      6,
			wantRebalancedEnd: 10100,
			wantBuyAndHoldEnd: 10300,
		},
		{
			name: "boundary on a non-trading day rolls forward to the next trading day",
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": pricesAt(t, []time.Time{
					day(t, 2024, 1, 30), day(t, 2024, 1, 31), day(t, 2024, 2, 2), day(t, 2024, 2, 3),
				}, []float64{100, 100, 120, 144}, "USD"),
				"GOOG": pricesAt(t, []time.Time{
					day(t, 2024, 1, 30), day(t, 2024, 1, 31), day(t, 2024, 2, 2), day(t, 2024, 2, 3),
				}, []float64{100, 100, 80, 64}, "USD"),
			},
			interval:          IntervalMonthly,
			thresholdPct:      nil,
			wantCurveLen:      4,
			wantRebalancedEnd: 10000, // rebalanced on Feb 2 (Feb 1 has no data)
			wantBuyAndHoldEnd: 10400,
		},
		{
			name: "no interval boundary in period matches buy-and-hold",
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, day(t, 2024, 1, 2), []float64{100, 101, 102, 103, 104, 105, 106}, "USD"),
				"GOOG": prices(t, day(t, 2024, 1, 2), []float64{100, 99, 98, 97, 96, 95, 94}, "USD"),
			},
			interval:          IntervalMonthly,
			thresholdPct:      nil,
			wantCurveLen:      7,
			wantRebalancedEnd: 10000, // no boundary before Feb 1
			wantBuyAndHoldEnd: 10000,
		},
		{
			name: "quarterly boundary re-anchors",
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, day(t, 2024, 3, 30), []float64{100, 100, 110, 121}, "USD"),
				"GOOG": prices(t, day(t, 2024, 3, 30), []float64{100, 100, 90, 81}, "USD"),
			},
			interval:          IntervalQuarterly,
			thresholdPct:      nil,
			wantCurveLen:      4,
			wantRebalancedEnd: 10000, // rebalanced on Apr 1
			wantBuyAndHoldEnd: 10100,
		},
		{
			name: "annual boundary re-anchors",
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, day(t, 2023, 12, 30), []float64{100, 100, 110, 121}, "USD"),
				"GOOG": prices(t, day(t, 2023, 12, 30), []float64{100, 100, 90, 81}, "USD"),
			},
			interval:          IntervalAnnual,
			thresholdPct:      nil,
			wantCurveLen:      4,
			wantRebalancedEnd: 10000, // rebalanced on Jan 1
			wantBuyAndHoldEnd: 10100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var thresholdPtr *decimal.Decimal
			if tt.thresholdPct != nil {
				d, _ := decimal.NewFromFloat64(*tt.thresholdPct)
				thresholdPtr = &d
			}

			run := func(strategy string) []EquityCurvePoint {
				out := SimulateEquityCurve(SimulateEquityCurveInput{
					StartingValue:         startingValue,
					Weights:               weights,
					PricesBySym:           tt.pricesBySym,
					BaseCurrency:          "USD",
					Strategy:              strategy,
					RebalanceInterval:     tt.interval,
					RebalanceThresholdPct: thresholdPtr,
				})
				return out.EquityCurve
			}

			rebalanced := run(StrategyRebalanced)
			assertCurveLen(t, tt.name+" (rebalanced)", rebalanced, tt.wantCurveLen)
			if len(rebalanced) == 0 {
				t.Fatalf("%s: rebalanced curve is empty", tt.name)
			}
			assertCurveValueApprox(t, tt.name+" (rebalanced)", rebalanced, len(rebalanced)-1, tt.wantRebalancedEnd)

			// NavPerUnit must equal PortfolioValue for every point.
			for i, p := range rebalanced {
				if p.NavPerUnit == nil {
					t.Errorf("%s: rebalanced point %d NavPerUnit is nil, want non-nil", tt.name, i)
				} else if !p.NavPerUnit.Equal(p.PortfolioValue) {
					t.Errorf("%s: rebalanced point %d NavPerUnit = %v, want == PortfolioValue %v", tt.name, i, p.NavPerUnit, p.PortfolioValue)
				}
			}

			buyAndHold := run(StrategyBuyAndHold)
			assertCurveLen(t, tt.name+" (buy-and-hold)", buyAndHold, tt.wantCurveLen)
			if len(buyAndHold) == 0 {
				t.Fatalf("%s: buy-and-hold curve is empty", tt.name)
			}
			assertCurveValueApprox(t, tt.name+" (buy-and-hold)", buyAndHold, len(buyAndHold)-1, tt.wantBuyAndHoldEnd)
		})
	}
}

// --- clipPeriod ---

func TestClipPeriod(t *testing.T) {
	base := day(t, 2024, 1, 2)

	tests := []struct {
		name             string
		weights          []ModelPortfolioWeight
		pricesBySym      map[string][]market.HistoricalPrice
		dateFrom         time.Time
		dateTo           time.Time
		wantClipFrom     time.Time
		wantClipTo       time.Time
		wantLimitedCount int
		wantMissingCount int
	}{
		{
			name: "full overlap",
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 0.5),
				weight(t, "GOOG", "GOOG", "USD", 0.5),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, base, []float64{100, 102, 105}, "USD"),
				"GOOG": prices(t, base, []float64{150, 151, 153}, "USD"),
			},
			wantClipFrom:     base,
			wantClipTo:       base.AddDate(0, 0, 2),
			wantLimitedCount: 0,
			wantMissingCount: 0,
		},
		{
			name: "partial overlap — one symbol starts later",
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 0.5),
				weight(t, "TSLA", "TSLA", "USD", 0.5),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, base, []float64{100, 102, 105, 103}, "USD"),
				"TSLA": prices(t, base.AddDate(0, 0, 1), []float64{200, 205, 202}, "USD"),
			},
			wantClipFrom:     base.AddDate(0, 0, 1), // Jan 3 (TSLA start)
			wantClipTo:       base.AddDate(0, 0, 3), // Jan 5 (both end)
			wantLimitedCount: 0,                     // TSLA covers the full clipped period
			wantMissingCount: 0,
		},
		{
			name: "one symbol missing entirely",
			weights: []ModelPortfolioWeight{
				weight(t, "AAPL", "AAPL", "USD", 0.5),
				weight(t, "META", "META", "USD", 0.5),
			},
			pricesBySym: map[string][]market.HistoricalPrice{
				"AAPL": prices(t, base, []float64{100, 102, 105}, "USD"),
			},
			wantClipFrom:     base,
			wantClipTo:       base.AddDate(0, 0, 2),
			wantLimitedCount: 0,
			wantMissingCount: 1, // META
		},
		{
			name: "all symbols missing",
			weights: []ModelPortfolioWeight{
				weight(t, "XYZ", "XYZ", "USD", 1.0),
			},
			pricesBySym:      map[string][]market.HistoricalPrice{},
			wantClipFrom:     time.Time{},
			wantClipTo:       time.Time{},
			wantLimitedCount: 0,
			wantMissingCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clipFrom, clipTo, limited, missing := clipPeriod(tt.weights, tt.pricesBySym, tt.dateFrom, tt.dateTo)

			if !clipFrom.Equal(tt.wantClipFrom) {
				t.Errorf("clipFrom = %v, want %v", clipFrom, tt.wantClipFrom)
			}
			if !clipTo.Equal(tt.wantClipTo) {
				t.Errorf("clipTo = %v, want %v", clipTo, tt.wantClipTo)
			}
			if len(limited) != tt.wantLimitedCount {
				t.Errorf("limited = %d (%v), want %d", len(limited), limited, tt.wantLimitedCount)
			}
			if len(missing) != tt.wantMissingCount {
				t.Errorf("missing = %d (%v), want %d", len(missing), missing, tt.wantMissingCount)
			}
		})
	}
}

// --- buildFxLookup ---

func TestBuildFxLookup(t *testing.T) {
	base := day(t, 2024, 1, 2)

	rates := prices(t, base, []float64{1.27, 1.28, 1.275, 1.29}, "USD")
	lookup := buildFxLookup(map[string][]market.HistoricalPrice{
		"GBP/USD": rates,
	})

	if len(lookup) != 1 {
		t.Fatalf("lookup length = %d, want 1", len(lookup))
	}

	gbpusd := lookup["GBP/USD"]
	if len(gbpusd.dates) != 4 {
		t.Errorf("dates length = %d, want 4", len(gbpusd.dates))
	}

	rate, ok := gbpusd.values["2024-01-02"]
	if !ok {
		t.Error("missing rate for 2024-01-02")
	}
	want, _ := decimal.NewFromFloat64(1.27)
	if !rate.Equal(want) {
		t.Errorf("rate = %v, want %v", rate, want)
	}
}

// --- convertWithFxLookup ---

func TestConvertWithFxLookup(t *testing.T) {
	base := day(t, 2024, 1, 2)

	rates := prices(t, base, []float64{1.27, 1.28, 1.275, 1.29}, "USD")
	lookup := buildFxLookup(map[string][]market.HistoricalPrice{
		"GBP/USD": rates,
	})

	value, _ := decimal.NewFromFloat64(1000)

	tests := []struct {
		name   string
		date   time.Time
		wantF  float64
		wantOk bool
	}{
		{
			name:   "exact match",
			date:   base, // Jan 2
			wantF:  1270, // 1000 * 1.27
			wantOk: true,
		},
		{
			name:   "forward fill",
			date:   base.AddDate(0, 0, 5), // Jan 7, after last rate
			wantF:  1290,                  // 1000 * 1.29 (last known rate)
			wantOk: true,
		},
		{
			name:   "backward fill",
			date:   base.AddDate(0, 0, -1), // Jan 1, before first rate
			wantF:  1270,                   // 1000 * 1.27 (first known rate)
			wantOk: true,
		},
		{
			name:   "no FX pair — pair not in lookup",
			date:   base,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pair := "GBP/USD"
			if tt.name == "no FX pair — pair not in lookup" {
				pair = "EUR/USD" // not in lookup
			}
			got, ok := convertWithFxLookup(lookup, pair, value, tt.date)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if tt.wantOk {
				want, _ := decimal.NewFromFloat64(tt.wantF)
				if !got.Equal(want) {
					t.Errorf("got = %v, want %v", got, want)
				}
			}
		})
	}
}

// --- collectDatesInRange ---

func TestCollectDatesInRange(t *testing.T) {
	base := day(t, 2024, 1, 2)

	pricesBySym := map[string][]market.HistoricalPrice{
		"AAPL": prices(t, base, []float64{100, 102, 105}, "USD"),
		"GOOG": prices(t, base.AddDate(0, 0, 1), []float64{150, 151}, "USD"),
	}
	priceLookup := buildPriceLookup(pricesBySym)

	weights := []ModelPortfolioWeight{
		weight(t, "AAPL", "AAPL", "USD", 0.5),
		weight(t, "GOOG", "GOOG", "USD", 0.5),
	}

	dates := collectDatesInRange(weights, priceLookup, base, base.AddDate(0, 0, 3))
	// Jan 2 (AAPL), Jan 3 (AAPL+GOOG), Jan 4 (AAPL+GOOG) = 3 dates
	// Jan 5 is in range but no symbol has data for it
	if len(dates) != 3 {
		t.Errorf("dates = %d, want 3", len(dates))
		for i, d := range dates {
			t.Logf("  [%d] %s", i, d.Format("2006-01-02"))
		}
	}
}
