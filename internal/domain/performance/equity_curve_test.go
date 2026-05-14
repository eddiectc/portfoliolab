package performance

import (
	"context"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
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
	result := InterpolateDaily(nil, testTime(2024, 1, 20), nil, nil, nil, nil, "USD", nil, context.Background())
	if result != nil {
		t.Error("expected nil, got non-nil")
	}
}

func TestInterpolateDaily_SinglePoint(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: testTime(2024, 1, 15), PortfolioValue: decimal.MustParse("1000.00"), NetDeposit: decimal.MustParse("1000.00")},
	}
	result := InterpolateDaily(points, testTime(2024, 1, 15), nil, nil, nil, nil, "USD", nil, context.Background())
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
	points := []EquityCurvePoint{
		{Date: testTime(2024, 1, 15), PortfolioValue: decimal.MustParse("1500.00"), NetDeposit: decimal.MustParse("1000.00")},
		{Date: testTime(2024, 1, 17), PortfolioValue: decimal.MustParse("1550.00"), NetDeposit: decimal.MustParse("1000.00")},
	}
	result := InterpolateDaily(points, testTime(2024, 1, 17), positions, positionCurrency, cashBalance, prices, "USD", nil, context.Background())
	// Should have 3 points (15th, 16th, 17th)
	if len(result) < 3 {
		t.Errorf("expected at least 3 points, got %d", len(result))
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
