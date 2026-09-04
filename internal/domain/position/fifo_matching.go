package position

import (
	"fmt"
	"sort"

	"github.com/govalues/decimal"
)

// absDecimal returns the absolute value of a decimal.
func absDecimal(d decimal.Decimal) decimal.Decimal {
	return d.Abs()
}

// sortedLotGroups returns a copy of lots sorted chronologically by open date,
// with lot ID as a deterministic tie-breaker for same-date lots.
func sortedLotGroups(lots []LotGroup) []LotGroup {
	sorted := make([]LotGroup, len(lots))
	copy(sorted, lots)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].OpenDate.Equal(sorted[j].OpenDate) {
			return sorted[i].OpenDate.Before(sorted[j].OpenDate)
		}
		return sorted[i].LotID < sorted[j].LotID
	})
	return sorted
}

// MatchSellLotsAgainstBuys matches sell lots against buy lots using FIFO
// (first-in, first-out) ordering. Sell lots are processed chronologically,
// consuming from the oldest buy lot first.
//
// Matching is date-aware: a sell lot can only consume buy lots that were
// opened on or before the sell lot's date (a share not yet bought cannot be
// sold). Same-day buys count as available — within the same day, buys are
// treated as preceding sells. A sell lot with no available buy lots (a
// short sell) produces no consumptions.
//
// For each consumption, realized P&L is computed as:
//
//	sell_proceeds_portion + cost_basis_consumed
//
// where sell_proceeds_portion is positive (cash inflow) and
// cost_basis_consumed is negative (cash outflow). The result is
// positive for a gain, negative for a loss.
//
// If a sell lot exceeds all available buy lots, the excess is treated
// as a short position (no consumption entry for the unmatched portion).
//
// Input slices are not mutated; matching order is always chronological
// (ties broken by lot ID), regardless of input order.
//
// Returns consumptions and a map of buy lot_id → remaining quantity.
func MatchSellLotsAgainstBuys(buyLots, sellLots []LotGroup) ([]LotConsumption, map[string]decimal.Decimal) {
	// Sort copies so the result is deterministic regardless of input order.
	// Stable sort keeps same-date lots in input order; tie-break by LotID for
	// full determinism.
	sortedBuys := sortedLotGroups(buyLots)
	sortedSells := sortedLotGroups(sellLots)

	// Track remaining quantity per buy lot.
	remaining := make(map[string]decimal.Decimal)
	for _, lot := range sortedBuys {
		remaining[lot.LotID] = lot.Quantity
	}

	var consumptions []LotConsumption

	for _, sellLot := range sortedSells {
		sellQty := absDecimal(sellLot.Quantity)
		sellRemaining := sellQty

		for _, buyLot := range sortedBuys {
			if sellRemaining.Equal(decimal.Zero) {
				break
			}

			// Date-aware FIFO: only shares bought on or before the sell date
			// can be consumed. Later buy lots are not yet available.
			if buyLot.OpenDate.After(sellLot.OpenDate) {
				continue
			}

			buyRemaining := remaining[buyLot.LotID]
			if buyRemaining.Equal(decimal.Zero) {
				continue
			}

			// Determine how much to consume (min of sell remaining, buy remaining).
			consumeQty := sellRemaining
			if buyRemaining.Less(sellRemaining) {
				consumeQty = buyRemaining
			}

			// Proportional cost basis consumed from this buy lot.
			ratio, err := consumeQty.Quo(buyLot.Quantity)
			if err != nil {
				panic(fmt.Sprintf("decimal division error computing consumption ratio: %v", err))
			}
			costBasisConsumed, err := buyLot.CostBasis.Mul(ratio)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow computing cost basis consumed: %v", err))
			}

			// Proportional sell proceeds for this consumption chunk.
			sellRatio, err := consumeQty.Quo(sellQty)
			if err != nil {
				panic(fmt.Sprintf("decimal division error computing sell ratio: %v", err))
			}
			sellProceedsPortion, err := sellLot.SellProceeds.Mul(sellRatio)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow computing sell proceeds portion: %v", err))
			}

			// Realized P&L = sell inflow (positive) + buy outflow (negative).
			realizedPnL, err := sellProceedsPortion.Add(costBasisConsumed)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow computing realized P&L: %v", err))
			}

			consumptions = append(consumptions, LotConsumption{
				SellLotID:         sellLot.LotID,
				BuyLotID:          buyLot.LotID,
				QuantityConsumed:  consumeQty,
				CostBasisConsumed: costBasisConsumed,
				RealizedPnL:       realizedPnL,
			})

			// Update remaining quantities.
			newBuyRemaining, err := buyRemaining.Sub(consumeQty)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow updating buy remaining: %v", err))
			}
			remaining[buyLot.LotID] = newBuyRemaining

			sellRemaining, err = sellRemaining.Sub(consumeQty)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow updating sell remaining: %v", err))
			}
		}
		// If sellRemaining > 0 after all buy lots exhausted → short position.
		// No consumption entry for the unmatched portion; tracked implicitly
		// by the caller (position computation step).
	}

	return consumptions, remaining
}
