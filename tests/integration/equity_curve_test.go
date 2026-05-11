package integration

import (
	"testing"
	"time"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// ---------------------------------------------------------------------------
// End-to-end equity curve integration test.
//
// Validates the full path: transactions → position recalc → equity curve
// computation → FX conversion → price interpolation → final curve values.
//
// Catches cross-layer bugs that unit tests with mocks cannot detect:
//   - Signed quantity convention (sells store negative quantities)
//   - FX forward-fill through cached historical rates
//   - Equity curve extending beyond last transaction date
//   - Cash balance tracking through deposits, buys, and sells
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Test: Buys, sells, deposits — single currency
//
//   Day 1 (Mon 2025-01-06): deposit £1000 + buy 10 STK @£100
//                            → 10 shares, cash=£0, equity=£1000
//   Day 3 (Wed 2025-01-08): deposit £500
//                            → 10 shares, cash=£500, equity=£1550
//                            (STK @£105 on this day)
//   Day 5 (Fri 2025-01-10): sell 3 STK
//                            → 7 shares, cash=£500+£315=£815
//                            equity = 7×£108 + £815 = £1571
//   Day 6 (Mon 2025-01-13): no transactions
//                            → 7 shares @£110 + £815 cash = £1585
//
// STK prices: day1=£100, day3=£105, day5=£108, day6=£110
//
// This test verifies:
//   - Sell transactions (negative quantity) correctly reduce position
//   - Cash from sell proceeds is tracked in the equity curve
//   - Equity curve extends beyond last transaction with real prices
//   - Multiple transactions on the same day are handled correctly
// ---------------------------------------------------------------------------

func TestEquityCurve_BuysSellsDeposits(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	// Day 1: deposit + buy
	createTx(t, router, accountID, "2025-01-06", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-06", "buy", "GBPSTK", "GBP", 10, 10000, -1000)

	// Day 3: additional deposit
	createTx(t, router, accountID, "2025-01-08", "deposit", "$CASH-GBP", "GBP", 500, 1, 500)

	// Day 5: partial sell (negative quantity)
	createTx(t, router, accountID, "2025-01-10", "sell", "GBPSTK", "GBP", -3, 10800, 315)

	insertMarketData(t, db, map[string][]market.HistoricalPrice{
		"GBPSTK": {
			histPrice("2025-01-06", 10000, "GBP"),
			histPrice("2025-01-07", 10200, "GBP"),
			histPrice("2025-01-08", 10500, "GBP"),
			histPrice("2025-01-09", 10700, "GBP"),
			histPrice("2025-01-10", 10800, "GBP"),
			histPrice("2025-01-13", 11000, "GBP"),
		},
	}, nil)

	result := getPerformance(t, router, portfolioID)

	if len(result.EquityCurve) == 0 {
		t.Fatal("equity curve is empty")
	}

	// Helper: find curve point for a given date, return PortfolioValue.
	findDate := func(dateStr string) *decimal.Decimal {
		target, _ := time.Parse("2006-01-02", dateStr)
		for _, pt := range result.EquityCurve {
			if pt.Date.Equal(target) {
				v := pt.PortfolioValue
				return &v
			}
		}
		return nil
	}

	// Day 1: 10 × £100 = £1000 (100000 pence)
	v := findDate("2025-01-06")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-06")
	}
	assertClose(t, "2025-01-06", v, 100000, 500)

	// Day 3: 10 × £105 + £500 cash = £1550 (155000 pence)
	v = findDate("2025-01-08")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-08")
	}
	assertClose(t, "2025-01-08", v, 155000, 1000)

	// Day 5: 7 × £108 + £815 cash = £1571 (157100 pence)
	v = findDate("2025-01-10")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-10")
	}
	assertClose(t, "2025-01-10", v, 157100, 1000)

	// Day 6: 7 × £110 + £815 cash = £1585 (158500 pence)
	// This verifies the curve extends beyond the last transaction date.
	v = findDate("2025-01-13")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-13")
	}
	assertClose(t, "2025-01-13", v, 158500, 1000)

	// Verify curve has points for all trading days (not just transaction dates).
	if len(result.EquityCurve) < 5 {
		t.Errorf("expected at least 5 curve points, got %d", len(result.EquityCurve))
	}

	t.Logf("equity curve: %d points, last=%s", len(result.EquityCurve),
		result.EquityCurve[len(result.EquityCurve)-1].PortfolioValue.String())
}

// ---------------------------------------------------------------------------
// Test: Multi-currency with FX forward-fill
//
//   GBP portfolio with GBP stock and USD cash.
//   FX rate (USD/GBP) is only cached for day 1 and day 4.
//   Days 2-3 must forward-fill from day 1.
//   Day 5+ must forward-fill from day 4.
//
//   Day 1 (Mon): deposit £1000 + buy 10 STK @£100 → 10 shares, cash=£0
//   Day 3 (Wed): deposit $1000 → cash=$1000=£800 (FX=0.80 from day 1)
//   Day 4 (Thu): no transactions, FX rate updated to 0.79
//   Day 5 (Fri): no transactions, FX forward-filled from day 4
//
// STK: day1=£100, day5=£105
// FX:  day1=0.80, day4=0.79
//
// This test verifies:
//   - FX rates are forward-filled when no rate exists for the exact date
//   - FX data (data_type='fx') is included in historical price queries
//   - Missing FX doesn't cause silent currency mixing
// ---------------------------------------------------------------------------

func TestEquityCurve_FXForwardFill(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	// Day 1: deposit + buy in GBP
	createTx(t, router, accountID, "2025-01-06", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-06", "buy", "GBPSTK", "GBP", 10, 10000, -1000)

	// Day 3: deposit in USD (FX needed — forward-filled from day 1)
	createTx(t, router, accountID, "2025-01-08", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)

	insertMarketData(t, db, map[string][]market.HistoricalPrice{
		"GBPSTK": {
			histPrice("2025-01-06", 10000, "GBP"),
			histPrice("2025-01-07", 10200, "GBP"),
			histPrice("2025-01-08", 10500, "GBP"),
			histPrice("2025-01-09", 10300, "GBP"),
			histPrice("2025-01-10", 10500, "GBP"),
		},
	}, map[string][]market.HistoricalPrice{
		// FX rate only on day 1 and day 4 — gaps must be forward-filled.
		"USD/GBP": {
			histPrice("2025-01-06", 80, "GBP"), // 1 USD = £0.80
			// day 7-8: forward-fill from day 6 (0.80)
			histPrice("2025-01-09", 79, "GBP"), // 1 USD = £0.79
			// day 10: forward-fill from day 9 (0.79)
		},
	})

	result := getPerformance(t, router, portfolioID)

	if len(result.EquityCurve) == 0 {
		t.Fatal("equity curve is empty")
	}

	findDate := func(dateStr string) *decimal.Decimal {
		target, _ := time.Parse("2006-01-02", dateStr)
		for _, pt := range result.EquityCurve {
			if pt.Date.Equal(target) {
				v := pt.PortfolioValue
				return &v
			}
		}
		return nil
	}

	// Day 3 (Wed): 10 × £105 + $1000 × 0.80 = £1050 + £800 = £1850
	// FX forward-filled from day 1 (0.80).
	v := findDate("2025-01-08")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-08")
	}
	// Allow wider tolerance for FX forward-fill rounding.
	assertClose(t, "2025-01-08", v, 185000, 2000)

	// Day 5 (Fri): 10 × £105 + $1000 × 0.79 = £1050 + £790 = £1840
	// FX forward-filled from day 4 (0.79).
	v = findDate("2025-01-10")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-10")
	}
	assertClose(t, "2025-01-10", v, 184000, 2000)

	t.Logf("equity curve: %d points, FX forward-fill verified", len(result.EquityCurve))
}

// ---------------------------------------------------------------------------
// Test: Full sell — position closes, equity reflects cash only
//
//   Day 1: deposit £1000 + buy 10 STK @£100 → 10 shares, cash=£0
//   Day 3: sell all 10 STK @£105 → 0 shares, cash=£1050
//   Day 4: no transactions → 0 shares, cash=£1050
//
// After the full sell, the equity curve should be flat at £1050
// (no price sensitivity since there are no open positions).
// ---------------------------------------------------------------------------

func TestEquityCurve_FullSell(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	createTx(t, router, accountID, "2025-01-06", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-06", "buy", "GBPSTK", "GBP", 10, 10000, -1000)
	createTx(t, router, accountID, "2025-01-08", "sell", "GBPSTK", "GBP", -10, 10500, 1050)

	insertMarketData(t, db, map[string][]market.HistoricalPrice{
		"GBPSTK": {
			histPrice("2025-01-06", 10000, "GBP"),
			histPrice("2025-01-07", 10200, "GBP"),
			histPrice("2025-01-08", 10500, "GBP"),
			histPrice("2025-01-09", 10300, "GBP"),
			histPrice("2025-01-10", 11000, "GBP"), // price spikes but no position
		},
	}, nil)

	result := getPerformance(t, router, portfolioID)

	findDate := func(dateStr string) *decimal.Decimal {
		target, _ := time.Parse("2006-01-02", dateStr)
		for _, pt := range result.EquityCurve {
			if pt.Date.Equal(target) {
				v := pt.PortfolioValue
				return &v
			}
		}
		return nil
	}

	// Day 3: after selling all, equity = £1050 (all cash)
	v := findDate("2025-01-08")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-08")
	}
	assertClose(t, "2025-01-08", v, 105000, 500)

	// Day 5: still £1050 (flat — no open positions, price spike irrelevant)
	v = findDate("2025-01-10")
	if v == nil {
		t.Fatal("missing curve point for 2025-01-10")
	}
	assertClose(t, "2025-01-10", v, 105000, 500)

	t.Logf("full sell: equity flat at %s after position closed", v.String())
}

// assertClose checks that the actual value (in base currency units) is within
// tolerance of the expected value (in hundredths, i.e., pence for GBP).
func assertClose(t *testing.T, date string, actual *decimal.Decimal, expected, tolerance int64) {
	t.Helper()
	if actual == nil {
		t.Fatalf("%s: value is nil", date)
	}
	// Equity curve values are in base currency units (e.g., 1000.00 for £1000).
	// expected is in hundredths (pence): 100000 = £1000.00.
	// Multiply actual by 100 to compare in the same unit.
	a, _ := actual.Mul(decimal.MustNew(100, 0))
	aInt, _, _ := a.Int64(0)
	diff := aInt - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Errorf("%s: equity = %s (%d), expected %d ± %d (diff = %d)",
			date, actual.String(), aInt, expected, tolerance, diff)
	}
}
