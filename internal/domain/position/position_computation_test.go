package position

import (
	"context"
	"testing"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// decPtr returns a pointer to a decimal.Decimal.
func decPtr(value int64, scale int) *decimal.Decimal {
	d := decimal.MustNew(value, scale)
	return &d
}

func TestComputePositions_SimpleBuy(t *testing.T) {
	// Single buy → one open position.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 0 {
		t.Fatalf("expected 0 closed positions, got %d", len(closed))
	}

	p := open[0]
	if p.Symbol != "AAPL" {
		t.Errorf("expected Symbol AAPL, got %q", p.Symbol)
	}
	if !p.Quantity.Equal(dec(10, 0)) {
		t.Errorf("expected Quantity 10, got %q", p.Quantity.String())
	}
	if !p.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected CostBasis -1500.00, got %q", p.CostBasis.String())
	}
	if !p.AvgOpenPrice.Equal(dec(15000, 2)) {
		t.Errorf("expected AvgOpenPrice 150.00, got %q", p.AvgOpenPrice.String())
	}
	if p.AvgClosePrice != nil {
		t.Errorf("expected AvgClosePrice nil, got %q", p.AvgClosePrice.String())
	}
	if !p.RealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected RealizedPnL 0 (no sells), got %q", p.RealizedPnL.String())
	}
	if !p.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected OpenDate 2025-01-15, got %q", p.OpenDate.Format("2006-01-02"))
	}
	if p.IsClosed {
		t.Error("expected IsClosed false")
	}
	if p.CloseDate != nil {
		t.Error("expected CloseDate nil")
	}
}

func TestComputePositions_BuyPartialSell(t *testing.T) {
	// Buy 10, sell 5 → one open position with reduced quantity.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-5, 0), decimal.Zero, dec(87500, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 0 {
		t.Fatalf("expected 0 closed positions, got %d", len(closed))
	}

	p := open[0]
	if !p.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected Quantity 5 (remaining), got %q", p.Quantity.String())
	}
	if !p.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected CostBasis -1500.00, got %q", p.CostBasis.String())
	}
	if !p.AvgOpenPrice.Equal(dec(15000, 2)) {
		t.Errorf("expected AvgOpenPrice 150.00, got %q", p.AvgOpenPrice.String())
	}
	if p.AvgClosePrice == nil || !p.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected AvgClosePrice 175.00, got %v", p.AvgClosePrice)
	}
	// P&L = -1500.00 + 875.00 = -625.00
	if !p.RealizedPnL.Equal(dec(-62500, 2)) {
		t.Errorf("expected RealizedPnL -625.00, got %q", p.RealizedPnL.String())
	}
	if p.IsClosed {
		t.Error("expected IsClosed false")
	}
}

func TestComputePositions_BuyFullSell(t *testing.T) {
	// Buy 10, sell 10 → one closed position.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-10, 0), decimal.Zero, dec(175000, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 0 {
		t.Fatalf("expected 0 open positions, got %d", len(open))
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}

	p := closed[0]
	if !p.Quantity.Equal(dec(10, 0)) {
		t.Errorf("expected Quantity 10, got %q", p.Quantity.String())
	}
	if !p.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected CostBasis -1500.00, got %q", p.CostBasis.String())
	}
	if !p.AvgOpenPrice.Equal(dec(15000, 2)) {
		t.Errorf("expected AvgOpenPrice 150.00, got %q", p.AvgOpenPrice.String())
	}
	if p.AvgClosePrice == nil || !p.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected AvgClosePrice 175.00, got %v", p.AvgClosePrice)
	}
	// P&L = -1500.00 + 1750.00 = 250.00
	if !p.RealizedPnL.Equal(dec(25000, 2)) {
		t.Errorf("expected RealizedPnL 250.00, got %q", p.RealizedPnL.String())
	}
	if !p.IsClosed {
		t.Error("expected IsClosed true")
	}
	if p.CloseDate == nil || !p.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected CloseDate 2025-03-20, got %v", p.CloseDate)
	}
}

func TestComputePositions_OneClosedOneOpen(t *testing.T) {
	// Buy 10, sell 10 (close), buy 5 → one closed + one open.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
		makeLotGroup("LOT-B2", "buy", "AAPL", "2025-04-01",
			dec(5, 0), dec(-80000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-10, 0), decimal.Zero, dec(175000, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}

	// Closed position: B1 → S1.
	cp := closed[0]
	if cp.Symbol != "AAPL" {
		t.Errorf("expected closed Symbol AAPL, got %q", cp.Symbol)
	}
	if !cp.Quantity.Equal(dec(10, 0)) {
		t.Errorf("expected closed Quantity 10, got %q", cp.Quantity.String())
	}
	if !cp.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected closed CostBasis -1500.00, got %q", cp.CostBasis.String())
	}
	if !cp.AvgOpenPrice.Equal(dec(15000, 2)) {
		t.Errorf("expected closed AvgOpenPrice 150.00, got %q", cp.AvgOpenPrice.String())
	}
	if cp.AvgClosePrice == nil || !cp.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected closed AvgClosePrice 175.00, got %v", cp.AvgClosePrice)
	}
	if !cp.RealizedPnL.Equal(dec(25000, 2)) {
		t.Errorf("expected closed RealizedPnL 250.00, got %q", cp.RealizedPnL.String())
	}
	if !cp.IsClosed {
		t.Error("expected closed IsClosed true")
	}
	if cp.CloseDate == nil || !cp.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected closed CloseDate 2025-03-20, got %v", cp.CloseDate)
	}

	// Open position: B2.
	op := open[0]
	if !op.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected open Quantity 5, got %q", op.Quantity.String())
	}
	if !op.CostBasis.Equal(dec(-80000, 2)) {
		t.Errorf("expected open CostBasis -800.00, got %q", op.CostBasis.String())
	}
	if !op.AvgOpenPrice.Equal(dec(16000, 2)) {
		t.Errorf("expected open AvgOpenPrice 160.00, got %q", op.AvgOpenPrice.String())
	}
	if op.AvgClosePrice != nil {
		t.Errorf("expected open AvgClosePrice nil, got %q", op.AvgClosePrice.String())
	}
	if !op.RealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected open RealizedPnL 0 (no sells), got %q", op.RealizedPnL.String())
	}
	if op.IsClosed {
		t.Error("expected open IsClosed false")
	}
}

func TestComputePositions_TwoFullCycles(t *testing.T) {
	// Buy 10, sell 10, buy 5, sell 5 → two closed positions.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
		makeLotGroup("LOT-B2", "buy", "AAPL", "2025-04-01",
			dec(5, 0), dec(-80000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-10, 0), decimal.Zero, dec(175000, 2)),
		makeLotGroup("LOT-S2", "sell", "AAPL", "2025-05-15",
			dec(-5, 0), decimal.Zero, dec(90000, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 0 {
		t.Fatalf("expected 0 open positions, got %d", len(open))
	}
	if len(closed) != 2 {
		t.Fatalf("expected 2 closed positions, got %d", len(closed))
	}

	// First cycle: B1 → S1.
	p1 := closed[0]
	if !p1.Quantity.Equal(dec(10, 0)) {
		t.Errorf("expected p1 Quantity 10, got %q", p1.Quantity.String())
	}
	if !p1.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected p1 CostBasis -1500.00, got %q", p1.CostBasis.String())
	}
	if p1.AvgClosePrice == nil || !p1.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected p1 AvgClosePrice 175.00, got %v", p1.AvgClosePrice)
	}
	if !p1.RealizedPnL.Equal(dec(25000, 2)) {
		t.Errorf("expected p1 RealizedPnL 250.00, got %q", p1.RealizedPnL.String())
	}
	if p1.CloseDate == nil || !p1.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected p1 CloseDate 2025-03-20, got %v", p1.CloseDate)
	}

	// Second cycle: B2 → S2.
	p2 := closed[1]
	if !p2.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected p2 Quantity 5, got %q", p2.Quantity.String())
	}
	if !p2.CostBasis.Equal(dec(-80000, 2)) {
		t.Errorf("expected p2 CostBasis -800.00, got %q", p2.CostBasis.String())
	}
	if p2.AvgClosePrice == nil || !p2.AvgClosePrice.Equal(dec(18000, 2)) {
		t.Errorf("expected p2 AvgClosePrice 180.00, got %v", p2.AvgClosePrice)
	}
	if !p2.RealizedPnL.Equal(dec(10000, 2)) {
		t.Errorf("expected p2 RealizedPnL 100.00, got %q", p2.RealizedPnL.String())
	}
	if p2.CloseDate == nil || !p2.CloseDate.Equal(mustTime("2025-05-15")) {
		t.Errorf("expected p2 CloseDate 2025-05-15, got %v", p2.CloseDate)
	}
}

func TestComputePositions_ShortPosition(t *testing.T) {
	// Sell 10 (no buy), then buy 5 → short position, open with 5.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-04-01",
			dec(5, 0), dec(-90000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-01-15",
			dec(-10, 0), decimal.Zero, dec(175000, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 0 {
		t.Fatalf("expected 0 closed positions, got %d", len(closed))
	}

	p := open[0]
	// Quantity = abs(runningQty) = abs(-10 + 5) = 5
	if !p.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected Quantity 5, got %q", p.Quantity.String())
	}
	// CostBasis = sum of buy net_cash = -900.00
	if !p.CostBasis.Equal(dec(-90000, 2)) {
		t.Errorf("expected CostBasis -900.00, got %q", p.CostBasis.String())
	}
	// AvgOpenPrice = Abs(-900.00) / 5 = 180.00
	if !p.AvgOpenPrice.Equal(dec(18000, 2)) {
		t.Errorf("expected AvgOpenPrice 180.00, got %q", p.AvgOpenPrice.String())
	}
	// AvgClosePrice = 1750.00 / 10 = 175.00
	if p.AvgClosePrice == nil || !p.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected AvgClosePrice 175.00, got %v", p.AvgClosePrice)
	}
	// RealizedPnL = -900.00 + 1750.00 = 850.00
	if !p.RealizedPnL.Equal(dec(85000, 2)) {
		t.Errorf("expected RealizedPnL 850.00, got %q", p.RealizedPnL.String())
	}
	if p.IsClosed {
		t.Error("expected IsClosed false")
	}
}

func TestComputePositions_MixedSymbols(t *testing.T) {
	// Different symbols should produce separate positions.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
		makeLotGroup("LOT-B2", "buy", "MSFT", "2025-02-01",
			dec(5, 0), dec(-100000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-5, 0), decimal.Zero, dec(87500, 2)),
		makeLotGroup("LOT-S2", "sell", "MSFT", "2025-04-10",
			dec(-3, 0), decimal.Zero, dec(67500, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 2 {
		t.Fatalf("expected 2 open positions, got %d", len(open))
	}
	if len(closed) != 0 {
		t.Fatalf("expected 0 closed positions, got %d", len(closed))
	}

	// Find positions by symbol.
	var aaplPos, msftPos Position
	foundAAPL, foundMSFT := false, false
	for _, p := range open {
		if p.Symbol == "AAPL" {
			aaplPos = p
			foundAAPL = true
		} else if p.Symbol == "MSFT" {
			msftPos = p
			foundMSFT = true
		}
	}

	if !foundAAPL {
		t.Fatal("missing AAPL position")
	}
	if !foundMSFT {
		t.Fatal("missing MSFT position")
	}

	// AAPL: bought 10, sold 5 → 5 remaining.
	if !aaplPos.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected AAPL Quantity 5, got %q", aaplPos.Quantity.String())
	}
	if !aaplPos.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected AAPL CostBasis -1500.00, got %q", aaplPos.CostBasis.String())
	}

	// MSFT: bought 5, sold 3 → 2 remaining.
	if !msftPos.Quantity.Equal(dec(2, 0)) {
		t.Errorf("expected MSFT Quantity 2, got %q", msftPos.Quantity.String())
	}
	if !msftPos.CostBasis.Equal(dec(-100000, 2)) {
		t.Errorf("expected MSFT CostBasis -1000.00, got %q", msftPos.CostBasis.String())
	}
	if msftPos.AvgClosePrice == nil || !msftPos.AvgClosePrice.Equal(dec(22500, 2)) {
		t.Errorf("expected MSFT AvgClosePrice 225.00, got %v", msftPos.AvgClosePrice)
	}
	// P&L = -1000.00 + 675.00 = -325.00
	if !msftPos.RealizedPnL.Equal(dec(-32500, 2)) {
		t.Errorf("expected MSFT RealizedPnL -325.00, got %q", msftPos.RealizedPnL.String())
	}
}

func TestComputePositions_EmptyLots(t *testing.T) {
	open, closed := ComputePositions(nil, nil)

	if len(open) != 0 {
		t.Errorf("expected 0 open positions, got %d", len(open))
	}
	if len(closed) != 0 {
		t.Errorf("expected 0 closed positions, got %d", len(closed))
	}
	// Should return non-nil slices.
	if open == nil {
		t.Error("expected open to be non-nil empty slice")
	}
	if closed == nil {
		t.Error("expected closed to be non-nil empty slice")
	}
}

func TestComputePositions_MultipleBuysOneSell(t *testing.T) {
	// Two buy lots, one sell lot that partially consumes both.
	// Buy 5 @ $150, buy 5 @ $160, sell 8 @ $175 → open with 2 remaining.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(5, 0), dec(-75000, 2), decimal.Zero),
		makeLotGroup("LOT-B2", "buy", "AAPL", "2025-02-20",
			dec(5, 0), dec(-80000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-8, 0), decimal.Zero, dec(140000, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 0 {
		t.Fatalf("expected 0 closed positions, got %d", len(closed))
	}

	p := open[0]
	// Quantity = abs(5 + 5 - 8) = 2
	if !p.Quantity.Equal(dec(2, 0)) {
		t.Errorf("expected Quantity 2, got %q", p.Quantity.String())
	}
	// CostBasis = -750.00 + -800.00 = -1550.00
	if !p.CostBasis.Equal(dec(-155000, 2)) {
		t.Errorf("expected CostBasis -1550.00, got %q", p.CostBasis.String())
	}
	// AvgOpenPrice = Abs(-1550.00) / 10 = 155.00
	if !p.AvgOpenPrice.Equal(dec(15500, 2)) {
		t.Errorf("expected AvgOpenPrice 155.00, got %q", p.AvgOpenPrice.String())
	}
	// AvgClosePrice = 1400.00 / 8 = 175.00
	if p.AvgClosePrice == nil || !p.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected AvgClosePrice 175.00, got %v", p.AvgClosePrice)
	}
	// RealizedPnL = -1550.00 + 1400.00 = -150.00
	if !p.RealizedPnL.Equal(dec(-15000, 2)) {
		t.Errorf("expected RealizedPnL -150.00, got %q", p.RealizedPnL.String())
	}
}

func TestComputePositions_SellThenBuySameCycle(t *testing.T) {
	// Sell 5, buy 5, sell 3 → one open position (short → less short).
	// Chronologically: S1 (Jan), B1 (Feb), S2 (Mar).
	// Running qty: -5 → 0 (close!) → -3 (new cycle).
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-02-01",
			dec(5, 0), dec(-75000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-01-15",
			dec(-5, 0), decimal.Zero, dec(80000, 2)),
		makeLotGroup("LOT-S2", "sell", "AAPL", "2025-03-20",
			dec(-3, 0), decimal.Zero, dec(52500, 2)),
	}

	open, closed := ComputePositions(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}

	// Closed: S1 → B1 (sell 5, buy 5, quantity goes -5 → 0).
	cp := closed[0]
	if !cp.IsClosed {
		t.Error("expected first position to be closed")
	}
	if !cp.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected closed Quantity 5, got %q", cp.Quantity.String())
	}
	// CostBasis = -750.00 (buy), SellProceeds = 800.00 (sell)
	// P&L = -750.00 + 800.00 = 50.00
	if !cp.RealizedPnL.Equal(dec(5000, 2)) {
		t.Errorf("expected closed RealizedPnL 50.00, got %q", cp.RealizedPnL.String())
	}

	// Open: S2 (sell 3, no matching buy → short 3).
	op := open[0]
	if op.IsClosed {
		t.Error("expected second position to be open")
	}
	if !op.Quantity.Equal(dec(3, 0)) {
		t.Errorf("expected open Quantity 3, got %q", op.Quantity.String())
	}
	// No buys in this cycle, only sells.
	if !op.CostBasis.Equal(decimal.Zero) {
		t.Errorf("expected open CostBasis 0, got %q", op.CostBasis.String())
	}
	if !op.AvgOpenPrice.Equal(decimal.Zero) {
		t.Errorf("expected open AvgOpenPrice 0, got %q", op.AvgOpenPrice.String())
	}
	// SellProceeds = 525.00, P&L = 0 + 525.00 = 525.00
	if !op.RealizedPnL.Equal(dec(52500, 2)) {
		t.Errorf("expected open RealizedPnL 525.00, got %q", op.RealizedPnL.String())
	}
}

func TestCalculatePositions_IdempotentRecalc(t *testing.T) {
	// Running CalculatePositions twice on the same transactions produces identical results.
	txns := []transaction.Transaction{
		makeTxn("LOT-1", "buy", "AAPL", "USD", "2025-01-15",
			dec(100, 0), dec(15000, 2), dec(-1500000, 2)),
		makeTxn("LOT-2", "buy", "AAPL", "USD", "2025-02-20",
			dec(50, 0), dec(16000, 2), dec(-800000, 2)),
		makeTxn("LOT-3", "sell", "AAPL", "USD", "2025-03-20",
			dec(-120, 0), dec(17000, 2), dec(2040000, 2)),
	}

	result1, err := CalculatePositions(context.Background(), 1, txns)
	if err != nil {
		t.Fatalf("first calc: %v", err)
	}

	result2, err := CalculatePositions(context.Background(), 1, txns)
	if err != nil {
		t.Fatalf("second calc: %v", err)
	}

	// Compare open positions.
	if len(result1.OpenPositions) != len(result2.OpenPositions) {
		t.Fatalf("open position count: %d vs %d", len(result1.OpenPositions), len(result2.OpenPositions))
	}
	for i := range result1.OpenPositions {
		if result1.OpenPositions[i].Symbol != result2.OpenPositions[i].Symbol {
			t.Errorf("open[%d] symbol: %q vs %q", i, result1.OpenPositions[i].Symbol, result2.OpenPositions[i].Symbol)
		}
		if !result1.OpenPositions[i].Quantity.Equal(result2.OpenPositions[i].Quantity) {
			t.Errorf("open[%d] quantity: %q vs %q", i, result1.OpenPositions[i].Quantity.String(), result2.OpenPositions[i].Quantity.String())
		}
		if !result1.OpenPositions[i].CostBasis.Equal(result2.OpenPositions[i].CostBasis) {
			t.Errorf("open[%d] cost_basis: %q vs %q", i, result1.OpenPositions[i].CostBasis.String(), result2.OpenPositions[i].CostBasis.String())
		}
	}

	// Compare closed positions.
	if len(result1.ClosedPositions) != len(result2.ClosedPositions) {
		t.Fatalf("closed position count: %d vs %d", len(result1.ClosedPositions), len(result2.ClosedPositions))
	}
	for i := range result1.ClosedPositions {
		if !result1.ClosedPositions[i].Quantity.Equal(result2.ClosedPositions[i].Quantity) {
			t.Errorf("closed[%d] quantity: %q vs %q", i, result1.ClosedPositions[i].Quantity.String(), result2.ClosedPositions[i].Quantity.String())
		}
		if !result1.ClosedPositions[i].RealizedPnL.Equal(result2.ClosedPositions[i].RealizedPnL) {
			t.Errorf("closed[%d] realized_pnl: %q vs %q", i, result1.ClosedPositions[i].RealizedPnL.String(), result2.ClosedPositions[i].RealizedPnL.String())
		}
	}
}

func TestGetCurrencyFromLots(t *testing.T) {
	tests := []struct {
		name     string
		lots     []LotGroup
		wantCur  string
	}{
		{
			name: "single_buy_lot_with_currency",
			lots: []LotGroup{
				makeLotGroup("LOT-1", "buy", "AAPL", "2025-01-15",
					dec(10, 0), dec(-150000, 2), decimal.Zero),
			},
			wantCur: "",
		},
		{
			name: "lot_with_transaction_currency",
			lots: []LotGroup{
				{
					LotID: "LOT-1", LotType: "buy", Symbol: "AAPL", OpenDate: mustTime("2025-01-15"),
					Quantity: dec(10, 0), CostBasis: dec(-150000, 2),
					Transactions: []TransactionRef{
						{ID: 1, Date: mustTime("2025-01-15"), Symbol: "AAPL", Currency: "USD"},
					},
				},
			},
			wantCur: "USD",
		},
		{
			name: "multiple_lots_first_has_currency",
			lots: []LotGroup{
				{
					LotID: "LOT-1", LotType: "buy", Symbol: "AAPL", OpenDate: mustTime("2025-01-15"),
					Quantity: dec(10, 0), CostBasis: dec(-150000, 2),
					Transactions: []TransactionRef{
						{ID: 1, Date: mustTime("2025-01-15"), Symbol: "AAPL", Currency: "GBP"},
					},
				},
				{
					LotID: "LOT-2", LotType: "sell", Symbol: "AAPL", OpenDate: mustTime("2025-02-01"),
					Quantity: dec(-5, 0), CostBasis: dec(0, 2), SellProceeds: dec(87500, 2),
					Transactions: []TransactionRef{
						{ID: 2, Date: mustTime("2025-02-01"), Symbol: "AAPL", Currency: "EUR"},
					},
				},
			},
			wantCur: "GBP",
		},
		{
			name: "first_lot_empty_currency_second_has_currency",
			lots: []LotGroup{
				{
					LotID: "LOT-1", LotType: "buy", Symbol: "AAPL", OpenDate: mustTime("2025-01-15"),
					Quantity: dec(10, 0), CostBasis: dec(-150000, 2),
					Transactions: []TransactionRef{
						{ID: 1, Date: mustTime("2025-01-15"), Symbol: "AAPL", Currency: ""},
					},
				},
				{
					LotID: "LOT-2", LotType: "sell", Symbol: "AAPL", OpenDate: mustTime("2025-02-01"),
					Quantity: dec(-5, 0), CostBasis: dec(0, 2), SellProceeds: dec(87500, 2),
					Transactions: []TransactionRef{
						{ID: 2, Date: mustTime("2025-02-01"), Symbol: "AAPL", Currency: "EUR"},
					},
				},
			},
			wantCur: "EUR",
		},
		{
			name:    "empty_lots",
			lots:    []LotGroup{},
			wantCur: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCurrencyFromLots(tt.lots)
			if got != tt.wantCur {
				t.Errorf("getCurrencyFromLots() = %q, want %q", got, tt.wantCur)
			}
		})
	}
}
