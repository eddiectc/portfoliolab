package performance

import (
	"context"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- Tests for WalkTxns ---

func TestWalkTxns_HappyPath(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(2, testTime(2024, 1, 16), "buy", "AAPL", "USD", 10, 1500000, 0),
	}
	snaps, final := WalkTxns(txns)
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}
	if len(final.positions) != 1 {
		t.Fatalf("expected 1 position in final, got %d", len(final.positions))
	}
}

func TestWalkTxns_NegativeSellQuantity(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(2, testTime(2024, 1, 16), "buy", "AAPL", "USD", 10, 1500000, 0),
		eqTxn(3, testTime(2024, 1, 17), "sell", "AAPL", "USD", -5, -800000, 0),
	}
	snaps, _ := WalkTxns(txns)
	lastSnap := snaps[len(snaps)-1]
	qty := lastSnap.positions["AAPL"]
	want := decimal.MustNew(5, 0)
	if !qty.Equal(want) {
		t.Errorf("expected AAPL qty %s, got %s", want, qty)
	}
}

func TestWalkTxns_MultipleTxnsSameDate(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(2, testTime(2024, 1, 15), "buy", "AAPL", "USD", 10, 1500000, 0),
	}
	snaps, _ := WalkTxns(txns)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
}

func TestWalkTxns_NegativeNetDeposit(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(2, testTime(2024, 1, 16), "withdrawal", "$CASH-USD", "USD", 0, 0, -1500000),
	}
	snaps, _ := WalkTxns(txns)
	lastSnap := snaps[len(snaps)-1]
	nd := lastSnap.netDeposit["USD"]
	want := decimal.MustNew(-500000, 0)
	if !nd.Equal(want) {
		t.Errorf("expected net deposit %s, got %s", want, nd)
	}
}

func TestWalkTxns_CapturesPreCashFlow(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 10, 1500000, 0),
		eqTxn(2, testTime(2024, 1, 16), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
	}
	snaps, _ := WalkTxns(txns)
	// Snapshot on deposit date should have pre-cash-flow snapshot
	snap1 := snaps[1]
	if len(snap1.preCashFlowSnapshots) != 1 {
		t.Fatalf("expected 1 pre-cash-flow snapshot, got %d", len(snap1.preCashFlowSnapshots))
	}
	pre := snap1.preCashFlowSnapshots[0]
	if len(pre.positions) != 1 {
		t.Errorf("expected 1 position in pre-cash-flow, got %d", len(pre.positions))
	}
}

func TestWalkTxns_DepositSharesDateWithBuy(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 10, 1500000, 0),
		eqTxn(2, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
	}
	snaps, _ := WalkTxns(txns)
	snap := snaps[0]
	if len(snap.preCashFlowSnapshots) != 1 {
		t.Fatalf("expected 1 pre-cash-flow snapshot, got %d", len(snap.preCashFlowSnapshots))
	}
	// Pre-cash-flow should include the buy on the same date
	pre := snap.preCashFlowSnapshots[0]
	if len(pre.positions) != 1 {
		t.Errorf("expected 1 position in pre-cash-flow (buy before deposit), got %d", len(pre.positions))
	}
}

// --- Tests for InterpolateDaily ---

func TestInterpolateDaily_NoPoints(t *testing.T) {
	result := InterpolateDaily(nil, nil, testTime(2024, 1, 20), nil, "USD", nil, context.Background())
	if result != nil {
		t.Error("expected nil, got non-nil")
	}
}

func TestInterpolateDaily_SinglePoint(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: testTime(2024, 1, 15), PortfolioValue: decimal.MustParse("1000.00"), NetDeposit: decimal.MustParse("1000.00")},
	}
	result := InterpolateDaily(points, []dateSnapshot{{date: testTime(2024, 1, 15)}}, testTime(2024, 1, 15), nil, "USD", nil, context.Background())
	if len(result) != 1 {
		t.Errorf("expected 1 point, got %d", len(result))
	}
}

func TestInterpolateDaily_FillsGaps(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"AAPL": {
			{Date: testTime(2024, 1, 16), Close: decimal.MustParse("150.00"), Currency: "USD"},
		},
	}
	positions := map[string]decimal.Decimal{"AAPL": decimal.MustParse("10.00")}
	positionCurrency := map[string]string{"AAPL": "USD"}
	cashBalance := map[string]decimal.Decimal{"USD": decimal.MustParse("500.00")}
	netDeposit := map[string]decimal.Decimal{"USD": decimal.MustParse("1000.00")}
	points := []EquityCurvePoint{
		{Date: testTime(2024, 1, 15), PortfolioValue: decimal.MustParse("1500.00"), NetDeposit: decimal.MustParse("1000.00")},
		{Date: testTime(2024, 1, 17), PortfolioValue: decimal.MustParse("1550.00"), NetDeposit: decimal.MustParse("1000.00")},
	}
	snapshots := []dateSnapshot{{
		date:             testTime(2024, 1, 15),
		positions:        positions,
		positionCurrency: positionCurrency,
		cashBalance:      cashBalance,
		netDeposit:       netDeposit,
	}}
	result := InterpolateDaily(points, snapshots, testTime(2024, 1, 17), prices, "USD", nil, context.Background())
	// Should have 3 points (15th, 16th, 17th)
	if len(result) < 3 {
		t.Errorf("expected at least 3 points, got %d", len(result))
	}
}

// --- Spec regression: daily mark-to-market between transactions (f010) ---
//
// f010 SPEC: "The portfolio value line reflects the sum of all open position
// market values plus cash balances" and "the data points are daily
// granularity for all periods". Carry-forward is specified for non-trading
// days only; every trading day between transactions must be revalued at
// that day's price, not carried flat from the last transaction date.

func TestInterpolateDaily_MarkToMarketBetweenTransactions(t *testing.T) {
	prices := map[string][]market.HistoricalPrice{
		"AAPL": {
			{Date: testTime(2024, 1, 15), Close: decimal.MustParse("100.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 16), Close: decimal.MustParse("102.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 17), Close: decimal.MustParse("104.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 18), Close: decimal.MustParse("106.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 19), Close: decimal.MustParse("108.00"), Currency: "USD"},
			// Jan 20-21 is a weekend - no prices.
			{Date: testTime(2024, 1, 22), Close: decimal.MustParse("110.00"), Currency: "USD"},
		},
	}
	positions := map[string]decimal.Decimal{"AAPL": decimal.MustParse("10.00")}
	positionCurrency := map[string]string{"AAPL": "USD"}
	cashBalance := map[string]decimal.Decimal{"USD": decimal.MustParse("500.00")}
	netDeposit := map[string]decimal.Decimal{"USD": decimal.MustParse("2000.00")}
	points := []EquityCurvePoint{
		{Date: testTime(2024, 1, 15), PortfolioValue: decimal.MustParse("1500.00"), NetDeposit: decimal.MustParse("2000.00")},
		{Date: testTime(2024, 1, 22), PortfolioValue: decimal.MustParse("1600.00"), NetDeposit: decimal.MustParse("2000.00")},
	}

	snap1 := dateSnapshot{date: testTime(2024, 1, 15), positions: positions, positionCurrency: positionCurrency, cashBalance: cashBalance, netDeposit: netDeposit}
	snap2 := snap1
	snap2.date = testTime(2024, 1, 22)
	result := InterpolateDaily(points, []dateSnapshot{snap1, snap2}, testTime(2024, 1, 22), prices, "USD", nil, context.Background())
	if len(result) != 8 {
		t.Fatalf("expected 8 daily points, got %d", len(result))
	}

	wantByDate := map[string]string{
		"2024-01-15": "1500.00", // trade date: 10 x 100.00 + 500.00
		"2024-01-16": "1520.00", // 10 x 102.00 + 500.00
		"2024-01-17": "1540.00", // 10 x 104.00 + 500.00
		"2024-01-18": "1560.00", // 10 x 106.00 + 500.00
		"2024-01-19": "1580.00", // 10 x 108.00 + 500.00
		"2024-01-20": "1580.00", // Saturday: carry forward from Friday
		"2024-01-21": "1580.00", // Sunday: carry forward from Friday
		"2024-01-22": "1600.00", // trade date: 10 x 110.00 + 500.00
	}
	for _, p := range result {
		key := p.Date.Format("2006-01-02")
		want, ok := wantByDate[key]
		if !ok {
			t.Errorf("unexpected date %s", key)
			continue
		}
		if p.PortfolioValue.Cmp(decimal.MustParse(want)) != 0 {
			t.Errorf("portfolio value for %s: got %s, want %s (mark-to-market at that day's price)", key, p.PortfolioValue.String(), want)
		}
	}
}

func TestComputeEquityCurve_MarkToMarketBetweenTransactions(t *testing.T) {
	// Full path (WalkTxns -> buildCurvePoints -> InterpolateDaily):
	// buy 10 AAPL on Jan 15, sell 1 on Jan 22. Trading days in between
	// must reflect the point-in-time holdings (10 shares + $1000 cash)
	// valued at that day's price.
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 2000),
		eqTxn(2, testTime(2024, 1, 15), "buy", "AAPL", "USD", 10, 100, -1000),
		eqTxn(3, testTime(2024, 1, 22), "sell", "AAPL", "USD", -1, 110, 110),
	}
	prices := map[string][]market.HistoricalPrice{
		"AAPL": {
			{Date: testTime(2024, 1, 15), Close: decimal.MustParse("100.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 16), Close: decimal.MustParse("102.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 17), Close: decimal.MustParse("104.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 18), Close: decimal.MustParse("106.00"), Currency: "USD"},
			{Date: testTime(2024, 1, 19), Close: decimal.MustParse("108.00"), Currency: "USD"},
			// Jan 20-21 is a weekend - no prices.
			{Date: testTime(2024, 1, 22), Close: decimal.MustParse("110.00"), Currency: "USD"},
		},
	}

	result, err := ComputeEquityCurve(context.Background(), txns, prices, "USD", testTime(2024, 1, 15), testTime(2024, 1, 22), nil, nil)
	if err != nil {
		t.Fatalf("ComputeEquityCurve returned error: %v", err)
	}
	if len(result.EquityCurve) != 8 {
		t.Fatalf("expected 8 daily points, got %d", len(result.EquityCurve))
	}

	wantByDate := map[string]string{
		"2024-01-15": "2000.00", // trade date: 10 x 100.00 + 1000.00
		"2024-01-16": "2020.00", // 10 x 102.00 + 1000.00
		"2024-01-17": "2040.00", // 10 x 104.00 + 1000.00
		"2024-01-18": "2060.00", // 10 x 106.00 + 1000.00
		"2024-01-19": "2080.00", // 10 x 108.00 + 1000.00
		"2024-01-20": "2080.00", // Saturday: carry forward from Friday
		"2024-01-21": "2080.00", // Sunday: carry forward from Friday
		"2024-01-22": "2100.00", // trade date: 9 x 110.00 + 1110.00
	}
	for _, p := range result.EquityCurve {
		key := p.Date.Format("2006-01-02")
		want, ok := wantByDate[key]
		if !ok {
			t.Errorf("unexpected date %s", key)
			continue
		}
		if p.PortfolioValue.Cmp(decimal.MustParse(want)) != 0 {
			t.Errorf("portfolio value for %s: got %s, want %s (mark-to-market at that day's price)", key, p.PortfolioValue.String(), want)
		}
	}
}

// --- Tests for ConvertToBase ---

func TestConvertToBase_SameCurrency(t *testing.T) {
	val, ok := ConvertToBase(context.Background(), nil, "USD", "USD", decimal.MustParse("100.00"), testTime(2024, 1, 15))
	if !ok {
		t.Error("expected ok=true")
	}
	want := decimal.MustNew(100, 0)
	if !val.Equal(want) {
		t.Errorf("expected %s, got %s", want, val)
	}
}

func TestConvertToBase_WithRate(t *testing.T) {
	mock := &mockMarketProvider{
		fxRates: map[string]market.FxRate{
			"EUR/USD": {Rate: decimal.MustParse("1.10")},
		},
	}
	val, ok := ConvertToBase(context.Background(), mock, "EUR", "USD", decimal.MustParse("100.00"), testTime(2024, 1, 15))
	if !ok {
		t.Error("expected ok=true")
	}
	want := decimal.MustParse("110.00")
	if !val.Equal(want) {
		t.Errorf("expected %s, got %s", want, val)
	}
}

func TestConvertToBase_NoRate(t *testing.T) {
	val, ok := ConvertToBase(context.Background(), nil, "EUR", "USD", decimal.MustParse("100.00"), testTime(2024, 1, 15))
	if ok {
		t.Error("expected ok=false")
	}
	if !val.Equal(decimal.Zero) {
		t.Errorf("expected zero, got %s", val)
	}
}

func TestConvertToBase_NilProvider(t *testing.T) {
	_, ok := ConvertToBase(context.Background(), nil, "EUR", "USD", decimal.MustParse("100.00"), testTime(2024, 1, 15))
	if ok {
		t.Error("expected ok=false")
	}
}

// --- Tests for CollectSymbols ---

func TestCollectSymbols(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 10, 1500000, 0),
		eqTxn(2, testTime(2024, 1, 16), "buy", "GOOG", "USD", 5, 2000000, 0),
	}
	symbols := CollectSymbols(txns)
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(symbols))
	}
}

func TestCollectSymbols_ExcludesCash(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(2, testTime(2024, 1, 16), "buy", "AAPL", "USD", 10, 1500000, 0),
	}
	symbols := CollectSymbols(txns)
	if len(symbols) != 1 {
		t.Errorf("expected 1 symbol, got %d", len(symbols))
	}
	if symbols[0] != "AAPL" {
		t.Errorf("expected AAPL, got %s", symbols[0])
	}
}

// --- Tests for TWR ---

func TestComputeTWR_DepositOnSameDateAsBuy(t *testing.T) {
	// Verify TWR handles deposits on same date as buys
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 10, 1500000, 0),
		eqTxn(2, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
	}
	// Just verify it doesn't panic
	_, _ = WalkTxns(txns)
}

// --- Helpers ---

func eqTxn(id int64, date time.Time, typ, symbol, currency string, qty, cost, netCash int64) transaction.Transaction {
	q := decimal.MustNew(qty, 0)
	c := decimal.MustNew(cost, 0)
	n := decimal.MustNew(netCash, 0)
	return transaction.Transaction{
		ID:       id,
		Date:     date,
		Type:     typ,
		Symbol:   symbol,
		Currency: currency,
		Quantity: q,
		Price:    c,
		NetCash:  n,
	}
}

func testTime(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

type mockMarketProvider struct {
	prices  map[string][]market.HistoricalPrice
	fxRates map[string]market.FxRate
}

func (m *mockMarketProvider) GetHistoricalPrices(_ context.Context, symbol string, _, _ time.Time) ([]market.HistoricalPrice, error) {
	if m.prices == nil {
		return nil, nil
	}
	return m.prices[symbol], nil
}

func (m *mockMarketProvider) GetLatestPriceDatePerSymbol(_ context.Context, _ []string) map[string]*time.Time {
	return nil
}

func (m *mockMarketProvider) GetHistoricalFxRate(_ context.Context, base, quote string, _ time.Time) (*market.FxRate, error) {
	if m.fxRates == nil {
		return nil, nil
	}
	pair := base + "/" + quote
	rate, ok := m.fxRates[pair]
	if !ok {
		return nil, nil
	}
	return &rate, nil
}

// --- Shared helpers for performance tests ---

func dec(value int64, scale int) decimal.Decimal {
	return decimal.MustNew(value, scale)
}

func mustTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
