package position

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
)

func makeLotGroup(lotID, lotType, symbol string, date string, qty, basis, proceeds decimal.Decimal) LotGroup {
	t, _ := time.Parse("2006-01-02", date)
	return LotGroup{
		LotID:        lotID,
		Symbol:       symbol,
		LotType:      lotType,
		OpenDate:     t,
		Quantity:     qty,
		CostBasis:    basis,
		SellProceeds: proceeds,
	}
}

func TestMatchSellLotsAgainstBuys_SimpleFIFO(t *testing.T) {
	// One buy lot (10 shares), one sell lot (5 shares) — partial consume.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero), // 10 @ $150, cost basis -1500.00
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-5, 0), decimal.Zero, dec(87500, 2)), // 5 @ $175, proceeds 875.00
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 1 {
		t.Fatalf("expected 1 consumption, got %d", len(consumptions))
	}

	c := consumptions[0]
	if c.SellLotID != "LOT-S1" {
		t.Errorf("expected SellLotID LOT-S1, got %q", c.SellLotID)
	}
	if c.BuyLotID != "LOT-B1" {
		t.Errorf("expected BuyLotID LOT-B1, got %q", c.BuyLotID)
	}
	if !c.QuantityConsumed.Equal(dec(5, 0)) {
		t.Errorf("expected QuantityConsumed 5, got %q", c.QuantityConsumed.String())
	}
	// cost_basis_consumed = -1500.00 * (5/10) = -750.00
	if !c.CostBasisConsumed.Equal(dec(-75000, 2)) {
		t.Errorf("expected CostBasisConsumed -750.00, got %q", c.CostBasisConsumed.String())
	}
	// sell_proceeds_portion = 875.00 * (5/5) = 875.00
	// pnl = 875.00 + (-750.00) = 125.00
	if !c.RealizedPnL.Equal(dec(12500, 2)) {
		t.Errorf("expected RealizedPnL 125.00, got %q", c.RealizedPnL.String())
	}

	// Remaining: 10 - 5 = 5
	if rem, ok := remaining["LOT-B1"]; !ok {
		t.Errorf("missing remaining for LOT-B1")
	} else if !rem.Equal(dec(5, 0)) {
		t.Errorf("expected remaining 5 for LOT-B1, got %q", rem.String())
	}
}

func TestMatchSellLotsAgainstBuys_FullConsume(t *testing.T) {
	// Sell equals buy quantity — full consume.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-10, 0), decimal.Zero, dec(175000, 2)), // 10 @ $175, proceeds 1750.00
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 1 {
		t.Fatalf("expected 1 consumption, got %d", len(consumptions))
	}

	c := consumptions[0]
	if !c.QuantityConsumed.Equal(dec(10, 0)) {
		t.Errorf("expected QuantityConsumed 10, got %q", c.QuantityConsumed.String())
	}
	// cost_basis_consumed = -1500.00 * (10/10) = -1500.00
	if !c.CostBasisConsumed.Equal(dec(-150000, 2)) {
		t.Errorf("expected CostBasisConsumed -1500.00, got %q", c.CostBasisConsumed.String())
	}
	// pnl = 1750.00 + (-1500.00) = 250.00
	if !c.RealizedPnL.Equal(dec(25000, 2)) {
		t.Errorf("expected RealizedPnL 250.00, got %q", c.RealizedPnL.String())
	}

	// Remaining: 10 - 10 = 0
	if rem, ok := remaining["LOT-B1"]; !ok {
		t.Errorf("missing remaining for LOT-B1")
	} else if !rem.Equal(decimal.Zero) {
		t.Errorf("expected remaining 0 for LOT-B1, got %q", rem.String())
	}
}

func TestMatchSellLotsAgainstBuys_MultiBuyConsume(t *testing.T) {
	// One sell lot consumes from two buy lots.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(5, 0), dec(-75000, 2), decimal.Zero), // 5 @ $150
		makeLotGroup("LOT-B2", "buy", "AAPL", "2025-02-20",
			dec(5, 0), dec(-80000, 2), decimal.Zero), // 5 @ $160
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-8, 0), decimal.Zero, dec(140000, 2)), // 8 @ $175, proceeds 1400.00
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 2 {
		t.Fatalf("expected 2 consumptions, got %d", len(consumptions))
	}

	// First consumption: 5 from LOT-B1 (full).
	c1 := consumptions[0]
	if c1.BuyLotID != "LOT-B1" {
		t.Errorf("expected first consumption from LOT-B1, got %q", c1.BuyLotID)
	}
	if !c1.QuantityConsumed.Equal(dec(5, 0)) {
		t.Errorf("expected consumed 5 from B1, got %q", c1.QuantityConsumed.String())
	}
	// cost = -750.00 * (5/5) = -750.00
	if !c1.CostBasisConsumed.Equal(dec(-75000, 2)) {
		t.Errorf("expected cost basis -750.00, got %q", c1.CostBasisConsumed.String())
	}
	// sell_portion = 1400.00 * (5/8) = 875.00
	// pnl = 875.00 + (-750.00) = 125.00
	if !c1.RealizedPnL.Equal(dec(12500, 2)) {
		t.Errorf("expected P&L 125.00, got %q", c1.RealizedPnL.String())
	}

	// Second consumption: 3 from LOT-B2 (partial).
	c2 := consumptions[1]
	if c2.BuyLotID != "LOT-B2" {
		t.Errorf("expected second consumption from LOT-B2, got %q", c2.BuyLotID)
	}
	if !c2.QuantityConsumed.Equal(dec(3, 0)) {
		t.Errorf("expected consumed 3 from B2, got %q", c2.QuantityConsumed.String())
	}
	// cost = -800.00 * (3/5) = -480.00
	if !c2.CostBasisConsumed.Equal(dec(-48000, 2)) {
		t.Errorf("expected cost basis -480.00, got %q", c2.CostBasisConsumed.String())
	}
	// sell_portion = 1400.00 * (3/8) = 525.00
	// pnl = 525.00 + (-480.00) = 45.00
	if !c2.RealizedPnL.Equal(dec(4500, 2)) {
		t.Errorf("expected P&L 45.00, got %q", c2.RealizedPnL.String())
	}

	// Remaining: B1=0, B2=2
	if rem, ok := remaining["LOT-B1"]; !ok || !rem.Equal(decimal.Zero) {
		t.Errorf("expected remaining 0 for LOT-B1, got %v", rem)
	}
	if rem, ok := remaining["LOT-B2"]; !ok || !rem.Equal(dec(2, 0)) {
		t.Errorf("expected remaining 2 for LOT-B2, got %v", rem)
	}
}

func TestMatchSellLotsAgainstBuys_MultipleSellsOneBuy(t *testing.T) {
	// Two sell lots against one buy lot.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(20, 0), dec(-300000, 2), decimal.Zero), // 20 @ $150
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-5, 0), decimal.Zero, dec(80000, 2)), // 5 @ $160, proceeds 800.00
		makeLotGroup("LOT-S2", "sell", "AAPL", "2025-04-10",
			dec(-8, 0), decimal.Zero, dec(136000, 2)), // 8 @ $170, proceeds 1360.00
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 2 {
		t.Fatalf("expected 2 consumptions, got %d", len(consumptions))
	}

	// S1 consumes 5 from B1.
	c1 := consumptions[0]
	if c1.SellLotID != "LOT-S1" {
		t.Errorf("expected first consumption for LOT-S1, got %q", c1.SellLotID)
	}
	if !c1.QuantityConsumed.Equal(dec(5, 0)) {
		t.Errorf("expected consumed 5, got %q", c1.QuantityConsumed.String())
	}
	// cost = -3000.00 * (5/20) = -750.00
	if !c1.CostBasisConsumed.Equal(dec(-75000, 2)) {
		t.Errorf("expected cost basis -750.00, got %q", c1.CostBasisConsumed.String())
	}
	// pnl = 800.00 + (-750.00) = 50.00
	if !c1.RealizedPnL.Equal(dec(5000, 2)) {
		t.Errorf("expected P&L 50.00, got %q", c1.RealizedPnL.String())
	}

	// S2 consumes 8 from B1.
	c2 := consumptions[1]
	if c2.SellLotID != "LOT-S2" {
		t.Errorf("expected second consumption for LOT-S2, got %q", c2.SellLotID)
	}
	if !c2.QuantityConsumed.Equal(dec(8, 0)) {
		t.Errorf("expected consumed 8, got %q", c2.QuantityConsumed.String())
	}
	// cost = -3000.00 * (8/20) = -1200.00
	if !c2.CostBasisConsumed.Equal(dec(-120000, 2)) {
		t.Errorf("expected cost basis -1200.00, got %q", c2.CostBasisConsumed.String())
	}
	// pnl = 1360.00 + (-1200.00) = 160.00
	if !c2.RealizedPnL.Equal(dec(16000, 2)) {
		t.Errorf("expected P&L 160.00, got %q", c2.RealizedPnL.String())
	}

	// Remaining: 20 - 5 - 8 = 7
	if rem, ok := remaining["LOT-B1"]; !ok || !rem.Equal(dec(7, 0)) {
		t.Errorf("expected remaining 7 for LOT-B1, got %v", rem)
	}
}

func TestMatchSellLotsAgainstBuys_ShortPosition(t *testing.T) {
	// Sell exceeds total buy quantity — creates short.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(5, 0), dec(-75000, 2), decimal.Zero), // 5 @ $150
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-10, 0), decimal.Zero, dec(175000, 2)), // 10 @ $175, proceeds 1750.00
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	// Only 5 shares matched (from B1), 5 unmatched (short).
	if len(consumptions) != 1 {
		t.Fatalf("expected 1 consumption (5 matched), got %d", len(consumptions))
	}

	c := consumptions[0]
	if !c.QuantityConsumed.Equal(dec(5, 0)) {
		t.Errorf("expected consumed 5, got %q", c.QuantityConsumed.String())
	}
	// cost = -750.00 * (5/5) = -750.00
	if !c.CostBasisConsumed.Equal(dec(-75000, 2)) {
		t.Errorf("expected cost basis -750.00, got %q", c.CostBasisConsumed.String())
	}
	// sell_portion = 1750.00 * (5/10) = 875.00
	// pnl = 875.00 + (-750.00) = 125.00
	if !c.RealizedPnL.Equal(dec(12500, 2)) {
		t.Errorf("expected P&L 125.00, got %q", c.RealizedPnL.String())
	}

	// Buy lot fully consumed.
	if rem, ok := remaining["LOT-B1"]; !ok || !rem.Equal(decimal.Zero) {
		t.Errorf("expected remaining 0 for LOT-B1, got %v", rem)
	}
	// 5 shares unmatched — no additional consumption entry.
	// The caller (position computation) handles the short.
}

func TestMatchSellLotsAgainstBuys_MultipleCycles(t *testing.T) {
	// Buy 10, sell 10 (close), buy 5, sell 5 (close).
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

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 2 {
		t.Fatalf("expected 2 consumptions, got %d", len(consumptions))
	}

	// S1 (10 shares) consumes all of B1 (10 shares).
	c1 := consumptions[0]
	if c1.BuyLotID != "LOT-B1" || c1.SellLotID != "LOT-S1" {
		t.Errorf("expected S1→B1, got %s→%s", c1.SellLotID, c1.BuyLotID)
	}
	if !c1.QuantityConsumed.Equal(dec(10, 0)) {
		t.Errorf("expected consumed 10, got %q", c1.QuantityConsumed.String())
	}

	// S2 (5 shares) consumes all of B2 (5 shares).
	c2 := consumptions[1]
	if c2.BuyLotID != "LOT-B2" || c2.SellLotID != "LOT-S2" {
		t.Errorf("expected S2→B2, got %s→%s", c2.SellLotID, c2.BuyLotID)
	}
	if !c2.QuantityConsumed.Equal(dec(5, 0)) {
		t.Errorf("expected consumed 5, got %q", c2.QuantityConsumed.String())
	}

	// Both buy lots fully consumed.
	if rem, ok := remaining["LOT-B1"]; !ok || !rem.Equal(decimal.Zero) {
		t.Errorf("expected remaining 0 for LOT-B1, got %v", rem)
	}
	if rem, ok := remaining["LOT-B2"]; !ok || !rem.Equal(decimal.Zero) {
		t.Errorf("expected remaining 0 for LOT-B2, got %v", rem)
	}
}

func TestMatchSellLotsAgainstBuys_EmptyBuyLots(t *testing.T) {
	// No buy lots — all sells unmatched.
	buyLots := []LotGroup{}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-03-20",
			dec(-5, 0), decimal.Zero, dec(87500, 2)),
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 0 {
		t.Errorf("expected 0 consumptions, got %d", len(consumptions))
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty remaining map, got %d entries", len(remaining))
	}
}

func TestMatchSellLotsAgainstBuys_EmptySellLots(t *testing.T) {
	// No sell lots — no consumptions, buy lots untouched.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-15",
			dec(10, 0), dec(-150000, 2), decimal.Zero),
	}
	sellLots := []LotGroup{}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 0 {
		t.Errorf("expected 0 consumptions, got %d", len(consumptions))
	}
	// Buy lot fully remaining.
	if rem, ok := remaining["LOT-B1"]; !ok || !rem.Equal(dec(10, 0)) {
		t.Errorf("expected remaining 10 for LOT-B1, got %v", rem)
	}
}

func TestMatchSellLotsAgainstBuys_SellBeforeBuy_NoConsumption(t *testing.T) {
	// Sell happens before any buy — no shares were held, so nothing matches.
	// The later buy cannot retroactively consume the earlier sell.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-02-10",
			dec(10, 0), dec(-80000, 2), decimal.Zero), // 10 @ $80
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-01-10",
			dec(-5, 0), decimal.Zero, dec(50000, 2)), // 5 @ $100 (short sell)
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 0 {
		t.Fatalf("expected 0 consumptions (sell before buy), got %d", len(consumptions))
	}
	// Buy lot is untouched — it is a real open position.
	if rem, ok := remaining["LOT-B1"]; !ok || !rem.Equal(dec(10, 0)) {
		t.Errorf("expected remaining 10 for LOT-B1, got %v", rem)
	}
}

func TestMatchSellLotsAgainstBuys_FIFOOnlyAmongAvailableLots(t *testing.T) {
	// Two buy lots (Jan and Mar); a Feb sell must consume only the Jan lot.
	// The Mar lot is not yet available at the sell date and must be skipped.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-10",
			dec(10, 0), dec(-100000, 2), decimal.Zero), // 10 @ $100
		makeLotGroup("LOT-B2", "buy", "AAPL", "2025-03-01",
			dec(10, 0), dec(-120000, 2), decimal.Zero), // 10 @ $120
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-02-01",
			dec(-6, 0), decimal.Zero, dec(78000, 2)), // 6 @ $130
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 1 {
		t.Fatalf("expected 1 consumption, got %d", len(consumptions))
	}
	// Must be from the Jan lot, not the Mar lot.
	if consumptions[0].BuyLotID != "LOT-B1" {
		t.Errorf("expected consumption from LOT-B1 (Jan lot), got %q", consumptions[0].BuyLotID)
	}
	if !consumptions[0].QuantityConsumed.Equal(dec(6, 0)) {
		t.Errorf("expected consumed 6, got %q", consumptions[0].QuantityConsumed.String())
	}
	// Jan lot: 10 - 6 = 4 remaining; Mar lot untouched.
	if rem, ok := remaining["LOT-B1"]; !ok || !rem.Equal(dec(4, 0)) {
		t.Errorf("expected remaining 4 for LOT-B1, got %v", rem)
	}
	if rem, ok := remaining["LOT-B2"]; !ok || !rem.Equal(dec(10, 0)) {
		t.Errorf("expected remaining 10 for LOT-B2 (not yet available), got %v", rem)
	}
}

func TestMatchSellLotsAgainstBuys_SameDayBuyAvailable(t *testing.T) {
	// Same-day buys count as available: a sell on the buy date consumes it.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-05-05",
			dec(10, 0), dec(-100000, 2), decimal.Zero), // 10 @ $100
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-05-05",
			dec(-4, 0), decimal.Zero, dec(44000, 2)), // 4 @ $110
	}

	consumptions, remaining := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 1 {
		t.Fatalf("expected 1 consumption (same-day), got %d", len(consumptions))
	}
	if consumptions[0].BuyLotID != "LOT-B1" {
		t.Errorf("expected consumption from LOT-B1, got %q", consumptions[0].BuyLotID)
	}
	if !remaining["LOT-B1"].Equal(dec(6, 0)) {
		t.Errorf("expected remaining 6 for LOT-B1, got %v", remaining["LOT-B1"])
	}
}

func TestMatchSellLotsAgainstBuys_UnsortedInput(t *testing.T) {
	// Matching must be chronological regardless of input slice order.
	// The Feb sell must still match the Jan buy (not the Mar buy) even when
	// the March lot appears first in the input.
	buyLots := []LotGroup{
		makeLotGroup("LOT-B2", "buy", "AAPL", "2025-03-01",
			dec(10, 0), dec(-120000, 2), decimal.Zero), // 10 @ $120
		makeLotGroup("LOT-B1", "buy", "AAPL", "2025-01-10",
			dec(10, 0), dec(-100000, 2), decimal.Zero), // 10 @ $100
	}
	sellLots := []LotGroup{
		makeLotGroup("LOT-S1", "sell", "AAPL", "2025-02-01",
			dec(-6, 0), decimal.Zero, dec(78000, 2)), // 6 @ $130
	}

	consumptions, _ := MatchSellLotsAgainstBuys(buyLots, sellLots)

	if len(consumptions) != 1 {
		t.Fatalf("expected 1 consumption, got %d", len(consumptions))
	}
	if consumptions[0].BuyLotID != "LOT-B1" {
		t.Errorf("expected consumption from LOT-B1 (Jan lot), got %q", consumptions[0].BuyLotID)
	}
}
