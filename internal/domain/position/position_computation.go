package position

import (
	"fmt"
	"sort"
	"time"

	"github.com/govalues/decimal"
)

// ComputePositions computes open and closed positions from lots and their
// date-aware FIFO matching.
//
// Closed positions — one row per sell lot that consumed at least one share
// of a buy lot. Each row aggregates that sell lot's consumptions: the
// matched share count, their cost basis, and the realized P&L, with the
// sell lot's own date and price. A sell lot with no consumption (a short
// sell) produces no closed row.
//
// Open positions — one row per symbol with a non-zero net quantity (the
// running total of buys and sells). Cost basis is attributed from the
// remaining (unconsumed) buy lots in FIFO order. A negative net quantity
// is a short position: signed negative quantity, zero cost basis.
//
// consumptions and remaining must come from MatchSellLotsAgainstBuys over
// the same buyLots/sellLots.
func ComputePositions(buyLots, sellLots []LotGroup, consumptions []LotConsumption, remaining map[string]decimal.Decimal) ([]Position, []Position) {
	open := []Position{}
	closed := []Position{}

	buyByID := make(map[string]LotGroup, len(buyLots))
	for _, lot := range buyLots {
		buyByID[lot.LotID] = lot
	}

	// --- Closed rows: one per sell lot with at least one consumption.
	bySell := make(map[string][]LotConsumption)
	for _, c := range consumptions {
		bySell[c.SellLotID] = append(bySell[c.SellLotID], c)
	}
	// Iterate sell lots chronologically for a deterministic row order.
	for _, sellLot := range sortedLotGroups(sellLots) {
		cs := bySell[sellLot.LotID]
		if len(cs) == 0 {
			// Short sell: matched no buy lot → no closed row.
			continue
		}
		closed = append(closed, buildClosedPosition(sellLot, cs, buyByID))
	}

	// --- Open rows: one per symbol with non-zero net quantity.
	bySymbol := make(map[string][]LotGroup)
	for _, lot := range buyLots {
		bySymbol[lot.Symbol] = append(bySymbol[lot.Symbol], lot)
	}
	for _, lot := range sellLots {
		bySymbol[lot.Symbol] = append(bySymbol[lot.Symbol], lot)
	}
	symbols := make([]string, 0, len(bySymbol))
	for sym := range bySymbol {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols)

	for _, sym := range symbols {
		if pos, ok := buildOpenPosition(bySymbol[sym], remaining); ok {
			open = append(open, pos)
		}
	}

	return open, closed
}

// buildClosedPosition builds a closed position row from a sell lot and its
// FIFO consumptions.
func buildClosedPosition(sellLot LotGroup, cs []LotConsumption, buyByID map[string]LotGroup) Position {
	var (
		qty       = decimal.Zero
		costBasis = decimal.Zero
		realizedP = decimal.Zero
	)
	var oldestOpen time.Time
	for i, c := range cs {
		q, err := qty.Add(c.QuantityConsumed)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing closed quantity: %v", err))
		}
		qty = q
		cb, err := costBasis.Add(c.CostBasisConsumed)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing closed cost basis: %v", err))
		}
		costBasis = cb
		p, err := realizedP.Add(c.RealizedPnL)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing closed P&L: %v", err))
		}
		realizedP = p

		buyLot, ok := buyByID[c.BuyLotID]
		if !ok {
			panic(fmt.Sprintf("consumption references unknown buy lot %q", c.BuyLotID))
		}
		if i == 0 || buyLot.OpenDate.Before(oldestOpen) {
			oldestOpen = buyLot.OpenDate
		}
	}

	// Average prices: cost per matched share; net sale price per share.
	var avgOpen decimal.Decimal
	if !qty.Equal(decimal.Zero) {
		p, err := costBasis.Abs().Quo(qty)
		if err != nil {
			panic(fmt.Sprintf("decimal division error computing closed avg open price: %v", err))
		}
		avgOpen = p
	}
	sellQty := sellLot.Quantity.Abs()
	var avgClose *decimal.Decimal
	if !sellQty.Equal(decimal.Zero) {
		p, err := sellLot.SellProceeds.Quo(sellQty)
		if err != nil {
			panic(fmt.Sprintf("decimal division error computing closed avg close price: %v", err))
		}
		avgClose = &p
	}

	// P&L as % of cost basis.
	var realizedPct *decimal.Decimal
	absCost := costBasis.Abs()
	if !absCost.Equal(decimal.Zero) {
		pct, err := realizedP.Quo(absCost)
		if err == nil {
			pct, _ = pct.Mul(decimal.MustNew(10000, 2)) // × 100 for percentage
			realizedPct = &pct
		}
	}

	closeDate := sellLot.OpenDate
	return Position{
		AccountID:      sellLot.AccountID,
		Symbol:         sellLot.Symbol,
		Currency:       getCurrencyFromLots([]LotGroup{sellLot}),
		Quantity:       qty,
		CostBasis:      costBasis,
		AvgOpenPrice:   avgOpen,
		AvgClosePrice:  avgClose,
		RealizedPnL:    realizedP,
		RealizedPnlPct: realizedPct,
		OpenDate:       oldestOpen,
		CloseDate:      ptrTime(closeDate),
		IsClosed:       true,
	}
}

// buildOpenPosition builds the open position row for one symbol's lots,
// using the per-buy-lot remaining quantity from FIFO matching. ok is false
// when the net quantity is zero (fully closed — no open row).
func buildOpenPosition(lots []LotGroup, remaining map[string]decimal.Decimal) (Position, bool) {
	sorted := sortedLotGroups(lots)

	// Running quantity walk: final net quantity and the last point where the
	// quantity was zero (for the short open-date rule). Deltas are normalized
	// by lot type (buy +, sell −) so the walk is correct regardless of the
	// sign convention used upstream when storing sell quantities.
	running := decimal.Zero
	lastZeroIdx := -1
	for i, lot := range sorted {
		delta := absDecimal(lot.Quantity)
		if lot.LotType == "sell" {
			delta = delta.Neg()
		}
		q, err := running.Add(delta)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow computing running quantity: %v", err))
		}
		running = q
		if running.Equal(decimal.Zero) {
			lastZeroIdx = i
		}
	}
	if running.Equal(decimal.Zero) {
		return Position{}, false
	}

	var (
		costBasis = decimal.Zero
		avgOpen   decimal.Decimal
		openDate  time.Time
	)

	if running.IsPos() {
		// Long: attribute cost from remaining buy lots in FIFO order.
		needed := running
		for _, lot := range sorted {
			if lot.LotType != "buy" || !needed.IsPos() {
				continue
			}
			rem := remaining[lot.LotID]
			if !rem.IsPos() {
				continue
			}
			take := rem
			if needed.Less(rem) {
				take = needed
			}
			ratio, err := take.Quo(lot.Quantity)
			if err != nil {
				panic(fmt.Sprintf("decimal division error computing open cost ratio: %v", err))
			}
			part, err := lot.CostBasis.Mul(ratio)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow computing open cost portion: %v", err))
			}
			part = roundMoney(part)
			costBasis, err = costBasis.Add(part)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow summing open cost basis: %v", err))
			}
			needed, err = needed.Sub(take)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow reducing open needed quantity: %v", err))
			}
		}
		if !needed.Equal(decimal.Zero) {
			panic(fmt.Sprintf("FIFO remaining insufficient for open position %s: %q shares unattributed", sorted[0].Symbol, needed.String()))
		}

		p, err := costBasis.Abs().Quo(running)
		if err != nil {
			panic(fmt.Sprintf("decimal division error computing open avg price: %v", err))
		}
		avgOpen = p

		// Open date: oldest buy lot with remaining shares.
		for _, lot := range sorted {
			if lot.LotType == "buy" && remaining[lot.LotID].IsPos() {
				openDate = lot.OpenDate
				break
			}
		}
	} else {
		// Short: signed negative quantity, zero cost basis. Open date is the
		// first lot after the last point where the running quantity was zero
		// (the short leg that is currently outstanding).
		start := lastZeroIdx + 1
		if start < 0 {
			start = 0
		}
		openDate = sorted[start].OpenDate
	}

	return Position{
		AccountID:    sorted[0].AccountID,
		Symbol:       sorted[0].Symbol,
		Currency:     getCurrencyFromLots(sorted),
		Quantity:     running,
		CostBasis:    costBasis,
		AvgOpenPrice: avgOpen,
		RealizedPnL:  decimal.Zero,
		OpenDate:     openDate,
		IsClosed:     false,
	}, true
}

// ptrTime returns a pointer to a time.Time.
func ptrTime(t time.Time) *time.Time {
	return &t
}

// getCurrencyFromLots extracts the currency from the first transaction in the
// first lot that has a non-empty currency. This handles both trade and cash
// lots uniformly.
func getCurrencyFromLots(lots []LotGroup) string {
	for _, lot := range lots {
		for _, txn := range lot.Transactions {
			if txn.Currency != "" {
				return txn.Currency
			}
		}
	}
	return ""
}
