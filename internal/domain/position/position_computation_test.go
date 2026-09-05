package position

import (
	"context"
	"testing"

	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
)

// compute is a test helper wiring the real matcher into ComputePositions so
// the tests exercise the full calculator pipeline, not hand-crafted
// consumptions.
func compute(buyLots, sellLots []LotGroup) ([]Position, []Position) {
	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)
	return ComputePositions(buyLots, sellLots, consumptions, remaining)
}

func TestComputePositions_SimpleBuy(t *testing.T) {
	// Single buy → one open position.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{}

	open, closed := compute(buyLots, sellLots)

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
	// Buy 10, sell 5 → one closed row (5 sold) + one open row (5 remaining).
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-5, 0), decimal.Zero, dec(87500, 2)),
	}

	open, closed := compute(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}

	// Open: 5 remaining shares, FIFO cost from B1.
	p := open[0]
	if !p.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected open Quantity 5 (remaining), got %q", p.Quantity.String())
	}
	if !p.CostBasis.Equal(dec(-75000, 2)) {
		t.Errorf("expected open CostBasis -750.00, got %q", p.CostBasis.String())
	}
	if !p.AvgOpenPrice.Equal(dec(15000, 2)) {
		t.Errorf("expected open AvgOpenPrice 150.00, got %q", p.AvgOpenPrice.String())
	}
	if p.AvgClosePrice != nil {
		t.Errorf("expected open AvgClosePrice nil, got %q", p.AvgClosePrice.String())
	}
	if !p.RealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected open RealizedPnL 0, got %q", p.RealizedPnL.String())
	}
	if !p.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected open OpenDate 2025-01-15, got %q", p.OpenDate.Format("2006-01-02"))
	}
	if p.IsClosed {
		t.Error("expected IsClosed false")
	}

	// Closed: the 5 shares sold by S1.
	c := closed[0]
	if !c.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected closed Quantity 5, got %q", c.Quantity.String())
	}
	if !c.CostBasis.Equal(dec(-75000, 2)) {
		t.Errorf("expected closed CostBasis -750.00, got %q", c.CostBasis.String())
	}
	if !c.RealizedPnL.Equal(dec(12500, 2)) {
		t.Errorf("expected closed RealizedPnL 125.00, got %q", c.RealizedPnL.String())
	}
	if c.AvgClosePrice == nil || !c.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected closed AvgClosePrice 175.00, got %v", c.AvgClosePrice)
	}
	if !c.IsClosed {
		t.Error("expected IsClosed true")
	}
	if c.CloseDate == nil || !c.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected CloseDate 2025-03-20, got %v", c.CloseDate)
	}
}

func TestComputePositions_BuyFullSell(t *testing.T) {
	// Buy 10, sell 10 → one closed position, no open position.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-10, 0), decimal.Zero, dec(175000, 2)),
	}

	open, closed := compute(buyLots, sellLots)

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

	open, closed := compute(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}

	// Closed position: S1 consumed all of B1.
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
	if !cp.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected closed OpenDate 2025-01-15, got %q", cp.OpenDate.Format("2006-01-02"))
	}
	if !cp.IsClosed {
		t.Error("expected closed IsClosed true")
	}
	if cp.CloseDate == nil || !cp.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected CloseDate 2025-03-20, got %v", cp.CloseDate)
	}

	// Open position: B2 (the re-open after the full close).
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
	if !op.OpenDate.Equal(mustTime("2025-04-01")) {
		t.Errorf("expected open OpenDate 2025-04-01 (re-open buy), got %q", op.OpenDate.Format("2006-01-02"))
	}
	if !op.RealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected open RealizedPnL 0 (no sells), got %q", op.RealizedPnL.String())
	}
	if op.IsClosed {
		t.Error("expected open IsClosed false")
	}
}

func TestComputePositions_TwoFullCycles(t *testing.T) {
	// Buy 10, sell 10, buy 5, sell 5 → two closed positions (one per sell lot).
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

	open, closed := compute(buyLots, sellLots)

	if len(open) != 0 {
		t.Fatalf("expected 0 open positions, got %d", len(open))
	}
	if len(closed) != 2 {
		t.Fatalf("expected 2 closed positions, got %d", len(closed))
	}

	// First sell lot: S1 consumed B1.
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
	if !p1.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected p1 OpenDate 2025-01-15, got %q", p1.OpenDate.Format("2006-01-02"))
	}
	if p1.CloseDate == nil || !p1.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected p1 CloseDate 2025-03-20, got %v", p1.CloseDate)
	}

	// Second sell lot: S2 consumed B2.
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
	if !p2.OpenDate.Equal(mustTime("2025-04-01")) {
		t.Errorf("expected p2 OpenDate 2025-04-01, got %q", p2.OpenDate.Format("2006-01-02"))
	}
	if p2.CloseDate == nil || !p2.CloseDate.Equal(mustTime("2025-05-15")) {
		t.Errorf("expected p2 CloseDate 2025-05-15, got %v", p2.CloseDate)
	}
}

func TestComputePositions_ShortPosition(t *testing.T) {
	// Sell 10 (no buy before the sell date), then buy 5 → net short 5.
	// The short sell matches no buy lot → no closed row.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-04-01",
			dec(5, 0), dec(-90000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-01-15",
			dec(-10, 0), decimal.Zero, dec(175000, 2)),
	}

	open, closed := compute(buyLots, sellLots)

	if len(closed) != 0 {
		t.Fatalf("expected 0 closed positions (short sell matches no buy), got %d", len(closed))
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}

	p := open[0]
	// Signed negative quantity: short 5 shares.
	if !p.Quantity.Equal(dec(-5, 0)) {
		t.Errorf("expected Quantity -5 (short), got %q", p.Quantity.String())
	}
	// No cost basis attributed to a short position.
	if !p.CostBasis.Equal(decimal.Zero) {
		t.Errorf("expected CostBasis 0, got %q", p.CostBasis.String())
	}
	if !p.AvgOpenPrice.Equal(decimal.Zero) {
		t.Errorf("expected AvgOpenPrice 0, got %q", p.AvgOpenPrice.String())
	}
	if p.AvgClosePrice != nil {
		t.Errorf("expected AvgClosePrice nil, got %q", p.AvgClosePrice.String())
	}
	if !p.RealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected RealizedPnL 0 (open position), got %q", p.RealizedPnL.String())
	}
	// Open date: the short leg itself (first lot after the last zero point).
	if !p.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected OpenDate 2025-01-15 (short sell), got %q", p.OpenDate.Format("2006-01-02"))
	}
	if p.IsClosed {
		t.Error("expected IsClosed false")
	}
}

func TestComputePositions_PureShort(t *testing.T) {
	// Sell only, never bought → open short position, no closed rows.
	buyLots := []LotGroup{}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-01-15",
			dec(-5, 0), decimal.Zero, dec(50000, 2)),
	}

	open, closed := compute(buyLots, sellLots)

	if len(closed) != 0 {
		t.Fatalf("expected 0 closed positions, got %d", len(closed))
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}

	p := open[0]
	if !p.Quantity.Equal(dec(-5, 0)) {
		t.Errorf("expected Quantity -5, got %q", p.Quantity.String())
	}
	if !p.CostBasis.Equal(decimal.Zero) {
		t.Errorf("expected CostBasis 0, got %q", p.CostBasis.String())
	}
	if !p.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected OpenDate 2025-01-15, got %q", p.OpenDate.Format("2006-01-02"))
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

	open, closed := compute(buyLots, sellLots)

	if len(open) != 2 {
		t.Fatalf("expected 2 open positions, got %d", len(open))
	}
	if len(closed) != 2 {
		t.Fatalf("expected 2 closed positions, got %d", len(closed))
	}

	// Find positions by symbol.
	var aaplOpen, msftOpen, aaplClosed, msftClosed Position
	found := map[string]bool{}
	for _, p := range open {
		switch p.Symbol {
		case "AAPL":
			aaplOpen = p
			found["o-aapl"] = true
		case "MSFT":
			msftOpen = p
			found["o-msft"] = true
		}
	}
	for _, p := range closed {
		switch p.Symbol {
		case "AAPL":
			aaplClosed = p
			found["c-aapl"] = true
		case "MSFT":
			msftClosed = p
			found["c-msft"] = true
		}
	}
	for k, v := range found {
		if !v {
			t.Fatalf("missing position %s", k)
		}
	}

	// AAPL open: bought 10, sold 5 → 5 remaining.
	if !aaplOpen.Quantity.Equal(dec(5, 0)) {
		t.Errorf("expected AAPL open Quantity 5, got %q", aaplOpen.Quantity.String())
	}
	if !aaplOpen.CostBasis.Equal(dec(-75000, 2)) {
		t.Errorf("expected AAPL open CostBasis -750.00, got %q", aaplOpen.CostBasis.String())
	}

	// MSFT open: bought 5, sold 3 → 2 remaining.
	if !msftOpen.Quantity.Equal(dec(2, 0)) {
		t.Errorf("expected MSFT open Quantity 2, got %q", msftOpen.Quantity.String())
	}
	if !msftOpen.CostBasis.Equal(dec(-40000, 2)) {
		t.Errorf("expected MSFT open CostBasis -400.00, got %q", msftOpen.CostBasis.String())
	}
	if !msftOpen.AvgOpenPrice.Equal(dec(20000, 2)) {
		t.Errorf("expected MSFT open AvgOpenPrice 200.00, got %q", msftOpen.AvgOpenPrice.String())
	}

	// AAPL closed: 5 shares sold by S1.
	if !aaplClosed.RealizedPnL.Equal(dec(12500, 2)) {
		t.Errorf("expected AAPL closed RealizedPnL 125.00, got %q", aaplClosed.RealizedPnL.String())
	}
	if aaplClosed.AvgClosePrice == nil || !aaplClosed.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected AAPL closed AvgClosePrice 175.00, got %v", aaplClosed.AvgClosePrice)
	}

	// MSFT closed: 3 shares sold by S2.
	if !msftClosed.RealizedPnL.Equal(dec(7500, 2)) {
		t.Errorf("expected MSFT closed RealizedPnL 75.00, got %q", msftClosed.RealizedPnL.String())
	}
	if msftClosed.AvgClosePrice == nil || !msftClosed.AvgClosePrice.Equal(dec(22500, 2)) {
		t.Errorf("expected MSFT closed AvgClosePrice 225.00, got %v", msftClosed.AvgClosePrice)
	}
}

func TestComputePositions_EmptyLots(t *testing.T) {
	open, closed := ComputePositions(nil, nil, nil, nil)

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
	// Buy 5 @ $150 (Jan), buy 5 @ $160 (Feb), sell 8 @ $175 (Mar).
	// → one closed row (8 shares spanning both buys) + open row (2 from B2).
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

	open, closed := compute(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}

	// Closed: 8 shares = 5 from B1 + 3 from B2.
	c := closed[0]
	if !c.Quantity.Equal(dec(8, 0)) {
		t.Errorf("expected closed Quantity 8, got %q", c.Quantity.String())
	}
	// CostBasis = -750.00 + -480.00 = -1230.00
	if !c.CostBasis.Equal(dec(-123000, 2)) {
		t.Errorf("expected closed CostBasis -1230.00, got %q", c.CostBasis.String())
	}
	// P&L = 1400.00 - 1230.00 = 170.00
	if !c.RealizedPnL.Equal(dec(17000, 2)) {
		t.Errorf("expected closed RealizedPnL 170.00, got %q", c.RealizedPnL.String())
	}
	// AvgOpenPrice = 1230.00 / 8 = 153.75
	if !c.AvgOpenPrice.Equal(dec(15375, 2)) {
		t.Errorf("expected closed AvgOpenPrice 153.75, got %q", c.AvgOpenPrice.String())
	}
	if c.AvgClosePrice == nil || !c.AvgClosePrice.Equal(dec(17500, 2)) {
		t.Errorf("expected closed AvgClosePrice 175.00, got %v", c.AvgClosePrice)
	}
	// OpenDate = oldest buy lot consumed (B1).
	if !c.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected closed OpenDate 2025-01-15, got %q", c.OpenDate.Format("2006-01-02"))
	}

	// Open: 2 remaining shares, both from B2.
	p := open[0]
	if !p.Quantity.Equal(dec(2, 0)) {
		t.Errorf("expected open Quantity 2, got %q", p.Quantity.String())
	}
	if !p.CostBasis.Equal(dec(-32000, 2)) {
		t.Errorf("expected open CostBasis -320.00, got %q", p.CostBasis.String())
	}
	if !p.AvgOpenPrice.Equal(dec(16000, 2)) {
		t.Errorf("expected open AvgOpenPrice 160.00, got %q", p.AvgOpenPrice.String())
	}
	if !p.OpenDate.Equal(mustTime("2025-02-20")) {
		t.Errorf("expected open OpenDate 2025-02-20 (B2), got %q", p.OpenDate.Format("2006-01-02"))
	}
}

func TestComputePositions_SellThenBuySameCycle(t *testing.T) {
	// Chronologically: S1 (Jan, sell 5), B1 (Feb, buy 5), S2 (Mar, sell 3).
	// Running qty: -5 → 0 → -3.
	// Date-aware FIFO: S1 has no prior buy → no closed row. B1 covers the
	// short; S2 (Mar) matches 3 shares of B1 → one closed row. The net -3
	// from S2 is a short open position dated at S2.
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

	open, closed := compute(buyLots, sellLots)

	if len(open) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(open))
	}
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}

	// Closed: S2's 3 shares matched against B1.
	cp := closed[0]
	if !cp.IsClosed {
		t.Error("expected closed position")
	}
	if !cp.Quantity.Equal(dec(3, 0)) {
		t.Errorf("expected closed Quantity 3, got %q", cp.Quantity.String())
	}
	// CostBasis = 3/5 × -750.00 = -450.00; proceeds = 525.00 → P&L = 75.00
	if !cp.CostBasis.Equal(dec(-45000, 2)) {
		t.Errorf("expected closed CostBasis -450.00, got %q", cp.CostBasis.String())
	}
	if !cp.RealizedPnL.Equal(dec(7500, 2)) {
		t.Errorf("expected closed RealizedPnL 75.00, got %q", cp.RealizedPnL.String())
	}
	if !cp.OpenDate.Equal(mustTime("2025-02-01")) {
		t.Errorf("expected closed OpenDate 2025-02-01 (B1), got %q", cp.OpenDate.Format("2006-01-02"))
	}
	if cp.CloseDate == nil || !cp.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected CloseDate 2025-03-20, got %v", cp.CloseDate)
	}

	// Open: net short 3 (S2's unmatched shares), dated at the short leg.
	op := open[0]
	if op.IsClosed {
		t.Error("expected open position")
	}
	if !op.Quantity.Equal(dec(-3, 0)) {
		t.Errorf("expected open Quantity -3 (short), got %q", op.Quantity.String())
	}
	if !op.CostBasis.Equal(decimal.Zero) {
		t.Errorf("expected open CostBasis 0, got %q", op.CostBasis.String())
	}
	if !op.AvgOpenPrice.Equal(decimal.Zero) {
		t.Errorf("expected open AvgOpenPrice 0, got %q", op.AvgOpenPrice.String())
	}
	if !op.OpenDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected open OpenDate 2025-03-20 (S2 short leg), got %q", op.OpenDate.Format("2006-01-02"))
	}
}

func TestComputePositions_FullCloseTwoSells(t *testing.T) {
	// Buy 100 @ $100 (Jan), sell 40 @ $110 (Feb), sell 60 @ $120 (Mar).
	// → two closed rows (one per sell lot); combined P&L = the full close P&L.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(100, 0), dec(-1000000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-02-15",
			dec(-40, 0), decimal.Zero, dec(440000, 2)),
		makeLotGroup("LOT-S2", "sell", "AAPL", "2025-03-15",
			dec(-60, 0), decimal.Zero, dec(720000, 2)),
	}

	open, closed := compute(buyLots, sellLots)

	if len(open) != 0 {
		t.Fatalf("expected 0 open positions, got %d", len(open))
	}
	if len(closed) != 2 {
		t.Fatalf("expected 2 closed positions, got %d", len(closed))
	}

	// S1: 40 shares.
	p1 := closed[0]
	if !p1.Quantity.Equal(dec(40, 0)) {
		t.Errorf("expected p1 Quantity 40, got %q", p1.Quantity.String())
	}
	if !p1.CostBasis.Equal(dec(-400000, 2)) {
		t.Errorf("expected p1 CostBasis -4000.00, got %q", p1.CostBasis.String())
	}
	if !p1.RealizedPnL.Equal(dec(40000, 2)) {
		t.Errorf("expected p1 RealizedPnL 400.00, got %q", p1.RealizedPnL.String())
	}
	if p1.AvgClosePrice == nil || !p1.AvgClosePrice.Equal(dec(11000, 2)) {
		t.Errorf("expected p1 AvgClosePrice 110.00, got %v", p1.AvgClosePrice)
	}
	if p1.CloseDate == nil || !p1.CloseDate.Equal(mustTime("2025-02-15")) {
		t.Errorf("expected p1 CloseDate 2025-02-15, got %v", p1.CloseDate)
	}

	// S2: 60 shares.
	p2 := closed[1]
	if !p2.Quantity.Equal(dec(60, 0)) {
		t.Errorf("expected p2 Quantity 60, got %q", p2.Quantity.String())
	}
	if !p2.CostBasis.Equal(dec(-600000, 2)) {
		t.Errorf("expected p2 CostBasis -6000.00, got %q", p2.CostBasis.String())
	}
	if !p2.RealizedPnL.Equal(dec(120000, 2)) {
		t.Errorf("expected p2 RealizedPnL 1200.00, got %q", p2.RealizedPnL.String())
	}
	if p2.AvgClosePrice == nil || !p2.AvgClosePrice.Equal(dec(12000, 2)) {
		t.Errorf("expected p2 AvgClosePrice 120.00, got %v", p2.AvgClosePrice)
	}

	// Combined P&L must equal the full close P&L: 4400 + 7200 - 10000 = 1600.
	combined, err := p1.RealizedPnL.Add(p2.RealizedPnL)
	if err != nil {
		t.Fatalf("decimal add: %v", err)
	}
	if !combined.Equal(dec(160000, 2)) {
		t.Errorf("expected combined RealizedPnL 1600.00, got %q", combined.String())
	}
}

func TestComputePositions_SellSpanningTwoBuyLots(t *testing.T) {
	// Buy 5 @ $100 (Jan), buy 5 @ $120 (Feb).
	// Sell 8 @ $110 (Mar) → spans both buy lots (one row, OpenDate = Jan).
	// Sell 2 @ $130 (Apr) → consumes the last of B2 (row OpenDate = Feb).
	// Fully closed → no open row.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-01",
			dec(5, 0), dec(-50000, 2), decimal.Zero),
		makeLotGroup("LOT-B2", "buy", "AAPL", "2025-02-01",
			dec(5, 0), dec(-60000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-01",
			dec(-8, 0), decimal.Zero, dec(88000, 2)),
		makeLotGroup("LOT-S2", "sell", "AAPL", "2025-04-01",
			dec(-2, 0), decimal.Zero, dec(26000, 2)),
	}

	open, closed := compute(buyLots, sellLots)

	if len(open) != 0 {
		t.Fatalf("expected 0 open positions, got %d", len(open))
	}
	if len(closed) != 2 {
		t.Fatalf("expected 2 closed positions, got %d", len(closed))
	}

	// S1: 8 shares = 5 from B1 + 3 from B2.
	p1 := closed[0]
	if !p1.Quantity.Equal(dec(8, 0)) {
		t.Errorf("expected p1 Quantity 8, got %q", p1.Quantity.String())
	}
	// CostBasis = -500.00 + -360.00 = -860.00; P&L = 880.00 - 860.00 = 20.00
	if !p1.CostBasis.Equal(dec(-86000, 2)) {
		t.Errorf("expected p1 CostBasis -860.00, got %q", p1.CostBasis.String())
	}
	if !p1.RealizedPnL.Equal(dec(2000, 2)) {
		t.Errorf("expected p1 RealizedPnL 20.00, got %q", p1.RealizedPnL.String())
	}
	// OpenDate = oldest buy lot consumed (B1).
	if !p1.OpenDate.Equal(mustTime("2025-01-01")) {
		t.Errorf("expected p1 OpenDate 2025-01-01, got %q", p1.OpenDate.Format("2006-01-02"))
	}
	if p1.CloseDate == nil || !p1.CloseDate.Equal(mustTime("2025-03-01")) {
		t.Errorf("expected p1 CloseDate 2025-03-01, got %v", p1.CloseDate)
	}

	// S2: 2 shares, all from B2.
	p2 := closed[1]
	if !p2.Quantity.Equal(dec(2, 0)) {
		t.Errorf("expected p2 Quantity 2, got %q", p2.Quantity.String())
	}
	// CostBasis = 2/5 × -600.00 = -240.00; P&L = 260.00 - 240.00 = 20.00
	if !p2.CostBasis.Equal(dec(-24000, 2)) {
		t.Errorf("expected p2 CostBasis -240.00, got %q", p2.CostBasis.String())
	}
	if !p2.RealizedPnL.Equal(dec(2000, 2)) {
		t.Errorf("expected p2 RealizedPnL 20.00, got %q", p2.RealizedPnL.String())
	}
	// OpenDate = B2 (the only buy lot consumed).
	if !p2.OpenDate.Equal(mustTime("2025-02-01")) {
		t.Errorf("expected p2 OpenDate 2025-02-01, got %q", p2.OpenDate.Format("2006-01-02"))
	}
	if p2.CloseDate == nil || !p2.CloseDate.Equal(mustTime("2025-04-01")) {
		t.Errorf("expected p2 CloseDate 2025-04-01, got %v", p2.CloseDate)
	}
}

func TestCalculatePositions_VWRPPartialSale(t *testing.T) {
	// The original bug report scenario, end-to-end at the calculator level:
	// VWRP.L bought 666 on 2024-07-01 @ 105.00 GBP, sold 18 on 2025-07-15
	// @ 144.60 GBP.
	// Expected: open 648 shares (cost -68,040.00) + closed 18 shares
	// (cost -1,890.00, P&L +712.80) — and the sale proceeds still flow into
	// the cash position (no double counting).
	txns := []transaction.Transaction{
		makeTxn("LOT-B1", "buy", "VWRP.L", "GBP", "2024-07-01",
			dec(666, 0), dec(10500, 2), dec(-6993000, 2)),
		makeTxn("LOT-S1", "sell", "VWRP.L", "GBP", "2025-07-15",
			dec(-18, 0), dec(14460, 2), dec(260280, 2)),
	}

	result, err := CalculatePositions(context.Background(), 1, txns)
	if err != nil {
		t.Fatalf("CalculatePositions: %v", err)
	}

	// Open: 666 - 18 = 648 shares remaining, all from B1.
	if len(result.OpenPositions) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(result.OpenPositions))
	}
	op := result.OpenPositions[0]
	if op.AccountID != 1 {
		t.Errorf("expected AccountID 1, got %d", op.AccountID)
	}
	if op.Symbol != "VWRP.L" {
		t.Errorf("expected Symbol VWRP.L, got %q", op.Symbol)
	}
	if !op.Quantity.Equal(dec(648, 0)) {
		t.Errorf("expected Quantity 648, got %q", op.Quantity.String())
	}
	if !op.CostBasis.Equal(dec(-6804000, 2)) {
		t.Errorf("expected CostBasis -68040.00, got %q", op.CostBasis.String())
	}
	if !op.AvgOpenPrice.Equal(dec(10500, 2)) {
		t.Errorf("expected AvgOpenPrice 105.00, got %q", op.AvgOpenPrice.String())
	}
	if op.AvgClosePrice != nil {
		t.Errorf("expected open AvgClosePrice nil, got %q", op.AvgClosePrice.String())
	}
	if !op.OpenDate.Equal(mustTime("2024-07-01")) {
		t.Errorf("expected OpenDate 2024-07-01, got %q", op.OpenDate.Format("2006-01-02"))
	}

	// Closed: the 18 shares sold by S1.
	if len(result.ClosedPositions) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(result.ClosedPositions))
	}
	cp := result.ClosedPositions[0]
	if cp.AccountID != 1 {
		t.Errorf("expected AccountID 1, got %d", cp.AccountID)
	}
	if !cp.Quantity.Equal(dec(18, 0)) {
		t.Errorf("expected Quantity 18, got %q", cp.Quantity.String())
	}
	if !cp.CostBasis.Equal(dec(-189000, 2)) {
		t.Errorf("expected CostBasis -1890.00, got %q", cp.CostBasis.String())
	}
	if !cp.RealizedPnL.Equal(dec(71280, 2)) {
		t.Errorf("expected RealizedPnL 712.80, got %q", cp.RealizedPnL.String())
	}
	// 712.80 / 1890.00 × 100
	if cp.RealizedPnlPct == nil || !cp.RealizedPnlPct.Equal(dec(3771428571428571429, 17)) {
		t.Errorf("expected RealizedPnlPct 37.71428571428571429, got %v", cp.RealizedPnlPct)
	}
	if !cp.AvgOpenPrice.Equal(dec(10500, 2)) {
		t.Errorf("expected AvgOpenPrice 105.00, got %q", cp.AvgOpenPrice.String())
	}
	if cp.AvgClosePrice == nil || !cp.AvgClosePrice.Equal(dec(14460, 2)) {
		t.Errorf("expected AvgClosePrice 144.60, got %v", cp.AvgClosePrice)
	}
	if !cp.OpenDate.Equal(mustTime("2024-07-01")) {
		t.Errorf("expected OpenDate 2024-07-01, got %q", cp.OpenDate.Format("2006-01-02"))
	}
	if cp.CloseDate == nil || !cp.CloseDate.Equal(mustTime("2025-07-15")) {
		t.Errorf("expected CloseDate 2025-07-15, got %v", cp.CloseDate)
	}

	// No double counting: the sale proceeds still flow into the cash
	// position (net cash = -69,930.00 + 2,602.80 = -67,327.20 GBP).
	if len(result.CashPositions) != 1 {
		t.Fatalf("expected 1 cash position, got %d", len(result.CashPositions))
	}
	if result.CashPositions[0].Currency != "GBP" {
		t.Errorf("expected cash Currency GBP, got %q", result.CashPositions[0].Currency)
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
		name    string
		lots    []LotGroup
		wantCur string
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
