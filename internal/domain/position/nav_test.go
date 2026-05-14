package position

import (
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- Tests for NAV integration in ComputeEquityCurve ---

func TestComputeEquityCurve_NAVFieldsPopulated(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -150000),
		eqTxn(1, testTime(2024, 3, 15), "sell", "AAPL", "USD", -500, 17000, 85000),
	})

	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 1, 15), 15000, "USD"),
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
		histPrice(testTime(2024, 3, 15), 17000, "USD"),
	})
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All points should have NAV fields populated.
	for i, pt := range result.EquityCurve {
		if pt.NavPerUnit == nil {
			t.Errorf("point %d (%s): NavPerUnit is nil", i, pt.Date.Format("2006-01-02"))
		}
		if pt.Units == nil {
			t.Errorf("point %d (%s): Units is nil", i, pt.Date.Format("2006-01-02"))
		}
	}

	// First point: NAV = $10,000 / 10000 = $1.00, Units = 10000
	first := result.EquityCurve[0]
	wantNAV := decimal.MustParse("1.00")
	if !first.NavPerUnit.Equal(wantNAV) {
		t.Errorf("first NavPerUnit: got %s, want %s", first.NavPerUnit.String(), wantNAV.String())
	}
	wantUnits := decimal.MustNew(10000, 0)
	if !first.Units.Equal(wantUnits) {
		t.Errorf("first Units: got %s, want %s", first.Units.String(), wantUnits.String())
	}
}

func TestComputeEquityCurve_NAVSummary(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -150000),
		eqTxn(1, testTime(2024, 3, 15), "sell", "AAPL", "USD", -500, 17000, 85000),
	})

	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 1, 15), 15000, "USD"),
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
		histPrice(testTime(2024, 3, 15), 17000, "USD"),
	})
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// NavSummary should be populated.
	if result.NavSummary == nil {
		t.Fatal("NavSummary is nil")
	}

	// Inception date should be the first transaction date.
	wantInception := testTime(2024, 1, 15)
	if !result.NavSummary.InceptionDate.Equal(wantInception) {
		t.Errorf("InceptionDate: got %v, want %v", result.NavSummary.InceptionDate, wantInception)
	}

	// Total units should be 10000 (no subsequent deposits).
	wantUnits := decimal.MustNew(10000, 0)
	if !result.NavSummary.TotalUnits.Equal(wantUnits) {
		t.Errorf("TotalUnits: got %s, want %s", result.NavSummary.TotalUnits.String(), wantUnits.String())
	}

	// Total value should match the last equity curve point.
	lastPt := result.EquityCurve[len(result.EquityCurve)-1]
	if !result.NavSummary.TotalValue.Equal(lastPt.PortfolioValue) {
		t.Errorf("TotalValue: got %s, want %s",
			result.NavSummary.TotalValue.String(), lastPt.PortfolioValue.String())
	}
}

func TestComputeEquityCurve_NAVWithSubsequentDeposit(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// Deposit, buy, deposit again.
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),  // $10,000
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -150000),   // buy 10 AAPL @ $150
		eqTxn(1, testTime(2024, 3, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),  // deposit $5,000
	})

	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 1, 15), 15000, "USD"),
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
		histPrice(testTime(2024, 3, 15), 16000, "USD"), // AAPL grew to $160
	})
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the Mar 15 point (after deposit).
	var mar15 *EquityCurvePoint
	for i := range result.EquityCurve {
		if result.EquityCurve[i].Date.Equal(testTime(2024, 3, 15)) {
			mar15 = &result.EquityCurve[i]
			break
		}
	}
	if mar15 == nil {
		t.Fatal("expected Mar 15 point")
	}

	// After deposit: units should be > 10000 (new units bought at Mar 15 NAV).
	// Pre-deposit: 10 AAPL @ $160 = $1,600 + cash $8,500 = $10,100
	// NAV before deposit = $10,100 / 10000 = $1.01
	// New units = $5,000 / $1.01 ≈ 4950
	// Total units ≈ 14950
	wantMinUnits := decimal.MustNew(14000, 0)
	if mar15.Units.Less(wantMinUnits) {
		t.Errorf("Mar 15 Units: got %s, want at least %s", mar15.Units.String(), wantMinUnits.String())
	}

	// NAV should be positive and reasonable.
	if !mar15.NavPerUnit.IsPos() {
		t.Errorf("Mar 15 NavPerUnit: got %s, expected positive", mar15.NavPerUnit.String())
	}
}

func TestComputeEquityCurve_NAVEmptyState(t *testing.T) {
	svc, _, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// No transactions.
	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// NavSummary should be nil for empty state.
	if result.NavSummary != nil {
		t.Error("NavSummary should be nil for empty state")
	}

	// Equity curve should be empty.
	if len(result.EquityCurve) != 0 {
		t.Errorf("expected empty equity curve, got %d points", len(result.EquityCurve))
	}
}

func TestComputeEquityCurve_NAVOnlyDeposits(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// Only deposits, no buys/sells.
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000), // $10,000
		eqTxn(1, testTime(2024, 3, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),  // $5,000
	})

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First point: NAV = $10,000 / 10000 = $1.00, Units = 10000
	first := result.EquityCurve[0]
	if first.NavPerUnit == nil {
		t.Fatal("first NavPerUnit is nil")
	}
	wantNAV := decimal.MustParse("1.00")
	if !first.NavPerUnit.Equal(wantNAV) {
		t.Errorf("first NavPerUnit: got %s, want %s", first.NavPerUnit.String(), wantNAV.String())
	}

	// Last point: after second deposit, units = 10000 + 5000/1.00 = 15000
	// NAV = $15,000 / 15000 = $1.00
	last := result.EquityCurve[len(result.EquityCurve)-1]
	if last.NavPerUnit == nil {
		t.Fatal("last NavPerUnit is nil")
	}
	if !last.NavPerUnit.Equal(wantNAV) {
		t.Errorf("last NavPerUnit: got %s, want %s", last.NavPerUnit.String(), wantNAV.String())
	}
	wantUnits := decimal.MustNew(15000, 0)
	if !last.Units.Equal(wantUnits) {
		t.Errorf("last Units: got %s, want %s", last.Units.String(), wantUnits.String())
	}

	// NavSummary should reflect final state.
	if result.NavSummary == nil {
		t.Fatal("NavSummary is nil")
	}
	if !result.NavSummary.TotalUnits.Equal(wantUnits) {
		t.Errorf("NavSummary TotalUnits: got %s, want %s",
			result.NavSummary.TotalUnits.String(), wantUnits.String())
	}
}

func TestComputeEquityCurve_NAVCarriedForwardInInterpolation(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 1, 20), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
	})

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Interpolated points between Jan 15 and Jan 20 should have NAV fields.
	for i, pt := range result.EquityCurve {
		if pt.NavPerUnit == nil {
			t.Errorf("point %d (%s): NavPerUnit is nil (should be carried forward)",
				i, pt.Date.Format("2006-01-02"))
		}
		if pt.Units == nil {
			t.Errorf("point %d (%s): Units is nil (should be carried forward)",
				i, pt.Date.Format("2006-01-02"))
		}
	}
}

func TestComputeEquityCurve_NAVWithWithdrawal(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000), // $10,000
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -150000),  // buy 10 @ $150
		eqTxn(1, testTime(2024, 3, 15), "withdrawal", "$CASH-USD", "USD", 0, 0, -300000), // withdraw $3,000
	})

	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 1, 15), 15000, "USD"),
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
		histPrice(testTime(2024, 3, 15), 17000, "USD"), // AAPL grew to $170
	})
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the Mar 15 point (after withdrawal).
	var mar15 *EquityCurvePoint
	for i := range result.EquityCurve {
		if result.EquityCurve[i].Date.Equal(testTime(2024, 3, 15)) {
			mar15 = &result.EquityCurve[i]
			break
		}
	}
	if mar15 == nil {
		t.Fatal("expected Mar 15 point")
	}

	// After withdrawal: units should be < 10000 (units redeemed).
	// Pre-withdrawal: 10 AAPL @ $170 = $1,700 + cash $8,500 = $10,200
	// NAV before withdrawal = $10,200 / 10000 = $1.02
	// Redeemed units = $3,000 / $1.02 ≈ 2941
	// Total units ≈ 7059
	wantMaxUnits := decimal.MustNew(8000, 0)
	if mar15.Units.Cmp(wantMaxUnits) > 0 {
		t.Errorf("Mar 15 Units: got %s, want at most %s", mar15.Units.String(), wantMaxUnits.String())
	}

	// NAV should still be positive.
	if !mar15.NavPerUnit.IsPos() {
		t.Errorf("Mar 15 NavPerUnit: got %s, expected positive", mar15.NavPerUnit.String())
	}
}

func TestComputeEquityCurve_NAVPeriodSlicing(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 6, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
	})

	// Filter with explicit dates (Mar-Dec).
	from := testTime(2024, 3, 1)
	to := testTime(2024, 12, 31)
	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		DateFrom:    &from,
		DateTo:      &to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sliced curve should still have NAV fields populated.
	for i, pt := range result.EquityCurve {
		if pt.NavPerUnit == nil {
			t.Errorf("point %d (%s): NavPerUnit is nil after period slicing",
				i, pt.Date.Format("2006-01-02"))
		}
		if pt.Units == nil {
			t.Errorf("point %d (%s): Units is nil after period slicing",
				i, pt.Date.Format("2006-01-02"))
		}
	}

	// First point in sliced period should have correct units (from full history).
	first := result.EquityCurve[0]
	// Units should be 10000 (from Jan deposit, no subsequent deposits in visible period).
	wantUnits := decimal.MustNew(10000, 0)
	if !first.Units.Equal(wantUnits) {
		t.Errorf("first Units: got %s, want %s", first.Units.String(), wantUnits.String())
	}
}

// TestComputeNavHistory_DepositsOnly verifies the fallback for zero-value
// pre-cash-flow snapshots in deposits-only portfolios.
func TestComputeNavHistory_DepositsOnly(t *testing.T) {
	// Two deposits with no positions. Pre-cash-flow snapshots yield zero value
	// because there are no positions to value and cash hasn't been deposited yet.
	// The fallback uses the previous equity curve point's portfolio value.

	equityCurve := []EquityCurvePoint{
		{Date: mustTime("2024-01-15"), PortfolioValue: dec(1000000, 2)}, // $10,000
		{Date: mustTime("2024-01-16"), PortfolioValue: dec(1000000, 2)}, // $10,000 (flat)
		{Date: mustTime("2024-03-15"), PortfolioValue: dec(1500000, 2)}, // $15,000 (after $5k deposit)
	}

	// Breakpoints: zero values (from pre-cash-flow snapshots with no positions).
	breakpoints := []navBreakpoint{
		{date: mustTime("2024-01-15"), value: decimal.Zero}, // initial deposit
		{date: mustTime("2024-03-15"), value: decimal.Zero}, // subsequent deposit
	}

	result := ComputeNavHistory(equityCurve, breakpoints)

	if len(result) != 3 {
		t.Fatalf("got %d points, want 3", len(result))
	}

	// Day 1: NAV = $10,000 / 10000 = $1.00
	wantNAV := decimal.MustParse("1.00")
	if !result[0].NavPerUnit.Equal(wantNAV) {
		t.Errorf("day 1 NAV: got %s, want %s", result[0].NavPerUnit.String(), wantNAV.String())
	}

	// Day 3: after deposit, fallback uses previous point's value ($10,000).
	// preCashFlowNAV = $10,000 / 10000 = $1.00
	// newUnits = $5,000 / $1.00 = 5000, totalUnits = 15000
	// NAV = $15,000 / 15000 = $1.00
	if !result[2].NavPerUnit.Equal(wantNAV) {
		t.Errorf("day 3 NAV: got %s, want %s", result[2].NavPerUnit.String(), wantNAV.String())
	}
	wantUnits := decimal.MustNew(15000, 0)
	if !result[2].Units.Equal(wantUnits) {
		t.Errorf("day 3 Units: got %s, want %s", result[2].Units.String(), wantUnits.String())
	}
}
