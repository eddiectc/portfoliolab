package position

import (
	"fmt"
	"time"

	"github.com/govalues/decimal"
)

// ComputePositions computes open and closed positions from FIFO-matched lots.
//
// It groups lots by symbol, then walks through each symbol's lots
// chronologically, tracking the running quantity. When the quantity
// transitions from non-zero to zero, a closed position is created.
// If quantity is non-zero at the end, an open position is created.
//
// Multiple open-to-close cycles within the same symbol produce separate
// closed positions. Direction changes (long → short or short → long) also
// terminate the current cycle and start a new one.
//
// consumptions is accepted for API completeness but P&L is derived from
// the lots directly (sell proceeds + cost basis), which is mathematically
// equivalent to summing consumption P&Ls.
func ComputePositions(buyLots, sellLots []LotGroup, consumptions []LotConsumption) ([]Position, []Position) {
	allLots := make([]LotGroup, 0, len(buyLots)+len(sellLots))
	allLots = append(allLots, buyLots...)
	allLots = append(allLots, sellLots...)

	if len(allLots) == 0 {
		return []Position{}, []Position{}
	}

	// Group lots by symbol.
	bySymbol := make(map[string][]LotGroup)
	for _, lot := range allLots {
		bySymbol[lot.Symbol] = append(bySymbol[lot.Symbol], lot)
	}

	var open, closed []Position

	for _, lots := range bySymbol {
		// Sort this symbol's lots chronologically.
		SortLotsByDate(lots)

		o, c := computePositionsForLots(lots)
		open = append(open, o...)
		closed = append(closed, c...)
	}

	return open, closed
}

// computePositionsForLots processes a chronologically-sorted slice of lots
// for a single symbol and returns open and closed positions.
func computePositionsForLots(lots []LotGroup) ([]Position, []Position) {
	// Walk through lots, grouping them into cycles based on quantity
	// direction changes and zero-crossings.
	var cycles []cycleState
	var current cycleState
	var runningQty decimal.Decimal = decimal.Zero

	for _, lot := range lots {
		// Buy lots have positive quantity, sell lots have negative quantity.
		// Use directly as the delta.
		delta := lot.Quantity

		newQty, err := runningQty.Add(delta)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow computing running quantity: %v", err))
		}

		newSign := newQty.Sign()
		oldSign := runningQty.Sign()

		// Check for direction change (e.g., long → short or short → long).
		if newSign != 0 && newSign != oldSign && len(current.lots) > 0 {
			current.finalQty = runningQty
			cycles = append(cycles, current)
			current = cycleState{lots: []LotGroup{lot}, quantitySign: newSign}
			runningQty = newQty
			continue
		}

		// Check for zero-crossing (position closes).
		if newSign == 0 && len(current.lots) > 0 {
			current.lots = append(current.lots, lot)
			current.finalQty = decimal.Zero
			cycles = append(cycles, current)
			current = cycleState{}
			runningQty = decimal.Zero
			continue
		}

		// Normal accumulation within the current cycle.
		if len(current.lots) == 0 {
			current = cycleState{lots: []LotGroup{lot}, quantitySign: newSign}
		} else {
			current.lots = append(current.lots, lot)
		}
		runningQty = newQty
	}

	// Remaining lots form the final (open) cycle.
	if len(current.lots) > 0 {
		current.finalQty = runningQty
		cycles = append(cycles, current)
	}

	var open, closed []Position

	for _, c := range cycles {
		pos := buildPosition(c)

		if pos.IsClosed {
			closed = append(closed, pos)
		} else {
			open = append(open, pos)
		}
	}

	return open, closed
}

// buildPosition constructs a Position from a cycle of lots.
func buildPosition(c cycleState) Position {
	var (
			buyLotsInCycle  []LotGroup
			sellLotsInCycle []LotGroup
	)

	for _, lot := range c.lots {
		if lot.LotType == "buy" {
			buyLotsInCycle = append(buyLotsInCycle, lot)
		} else {
			sellLotsInCycle = append(sellLotsInCycle, lot)
		}
	}

	// Aggregate quantities, cost basis, and sell proceeds.
	var (
		totalBuyQty    decimal.Decimal = decimal.Zero
		totalSellQty   decimal.Decimal = decimal.Zero
		totalCostBasis decimal.Decimal = decimal.Zero
		totalSellProc  decimal.Decimal = decimal.Zero
	)

	for _, lot := range buyLotsInCycle {
		q, err := totalBuyQty.Add(lot.Quantity)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing buy quantity: %v", err))
		}
		totalBuyQty = q
		cb, err := totalCostBasis.Add(lot.CostBasis)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing cost basis: %v", err))
		}
		totalCostBasis = cb
	}

	for _, lot := range sellLotsInCycle {
		q, err := totalSellQty.Add(lot.Quantity.Abs())
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing sell quantity: %v", err))
		}
		totalSellQty = q
		sp, err := totalSellProc.Add(lot.SellProceeds)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing sell proceeds: %v", err))
		}
		totalSellProc = sp
	}

	// Realized P&L = sell proceeds (positive) + cost basis (negative).
	realizedPnL, err := totalCostBasis.Add(totalSellProc)
	if err != nil {
		panic(fmt.Sprintf("decimal overflow computing realized P&L: %v", err))
	}

	// Average prices per share.
	var avgOpenPrice decimal.Decimal
	var avgClosePrice *decimal.Decimal

	if !totalBuyQty.Equal(decimal.Zero) {
		p, err := totalCostBasis.Quo(totalBuyQty)
		if err != nil {
			panic(fmt.Sprintf("decimal division error computing avg open price: %v", err))
		}
		avgOpenPrice = p
	}

	if !totalSellQty.Equal(decimal.Zero) {
		p, err := totalSellProc.Quo(totalSellQty)
		if err != nil {
			panic(fmt.Sprintf("decimal division error computing avg close price: %v", err))
		}
		avgClosePrice = &p
	}

	isClosed := c.finalQty.Equal(decimal.Zero)

	pos := Position{
		AccountID:     c.lots[0].AccountID,
		Symbol:        c.lots[0].Symbol,
		Currency:      getCurrencyFromLots(c.lots),
		CostBasis:     totalCostBasis,
		AvgOpenPrice:  avgOpenPrice.Abs(),
		AvgClosePrice: avgClosePrice,
		RealizedPnL:   realizedPnL,
		OpenDate:      c.lots[0].OpenDate,
		IsClosed:      isClosed,
	}

	if isClosed {
		pos.Quantity = totalBuyQty
		pos.CloseDate = ptrTime(c.lots[len(c.lots)-1].OpenDate)
	} else {
		pos.Quantity = c.finalQty.Abs()
	}

	return pos
}

// cycleState groups lots that belong to the same open-to-close cycle.
type cycleState struct {
	lots         []LotGroup
	quantitySign int            // -1, 0, or +1
	finalQty     decimal.Decimal // runningQty at the time the cycle ended
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
