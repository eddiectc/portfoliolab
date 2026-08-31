package position

import (
	"fmt"

	"github.com/govalues/decimal"
)

// absDecimal returns the absolute value of a decimal.
func absDecimal(d decimal.Decimal) decimal.Decimal {
	return d.Abs()
}

// MatchSellLotsAgainstBuys matches sell lots against buy lots using FIFO
// (first-in, first-out) ordering. Sell lots are processed chronologically,
// consuming from the oldest buy lot first.
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
// Returns consumptions and a map of buy lot_id → remaining quantity.
func MatchSellLotsAgainstBuys(buyLots, sellLots []LotGroup) ([]LotConsumption, map[string]decimal.Decimal) {
	// Track remaining quantity per buy lot.
	remaining := make(map[string]decimal.Decimal)
	for _, lot := range buyLots {
		remaining[lot.LotID] = lot.Quantity
	}

	var consumptions []LotConsumption

	for _, sellLot := range sellLots {
		sellQty := absDecimal(sellLot.Quantity)
		sellRemaining := sellQty

		for _, buyLot := range buyLots {
			if sellRemaining.Equal(decimal.Zero) {
				break
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
