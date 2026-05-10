package position

import (
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

// PositionSnapshot is the quantity of each symbol at a point in time,
// computed by walking transactions once and emitting a snapshot at
// each unique date boundary.
//
// This is the single source of truth for position quantity tracking,
// shared by the equity curve and any other consumer that needs to
// know what positions existed at each point in time.
type PositionSnapshot struct {
	Date      time.Time
	Quantities map[string]decimal.Decimal // symbol → qty (signed: positive=long, negative=short)
}

// WalkPositionQuantities walks transactions chronologically and emits
// a PositionSnapshot at each unique date. Transaction quantities are
// signed: positive for buys, negative for sells — added directly.
//
// Transactions must be sorted by date ASC, then ID ASC.
func WalkPositionQuantities(txns []transaction.Transaction) []PositionSnapshot {
	var snapshots []PositionSnapshot
	quantities := make(map[string]decimal.Decimal)

	for i, txn := range txns {
		if txn.Type == "buy" || txn.Type == "sell" {
			qty, _ := quantities[txn.Symbol].Add(txn.Quantity)
			quantities[txn.Symbol] = qty
		}

		// Emit snapshot at end of each date.
		isLastForDate := i == len(txns)-1 || txns[i+1].Date.After(txn.Date)
		if isLastForDate {
			snapshots = append(snapshots, PositionSnapshot{
				Date:       txn.Date,
				Quantities: copyDecimalMap(quantities),
			})
		}
	}

	return snapshots
}

// FinalPositionQuantities returns the position quantities after all
// transactions have been processed. Returns an empty map if there are
// no transactions.
func FinalPositionQuantities(txns []transaction.Transaction) map[string]decimal.Decimal {
	quantities := make(map[string]decimal.Decimal)

	for _, txn := range txns {
		if txn.Type == "buy" || txn.Type == "sell" {
			qty, _ := quantities[txn.Symbol].Add(txn.Quantity)
			quantities[txn.Symbol] = qty
		}
	}

	return quantities
}
