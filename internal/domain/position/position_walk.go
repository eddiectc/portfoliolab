package position

import (
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

// PortfolioSnapshot is the complete portfolio state at a point in time:
// position quantities, cash balances, and cumulative net deposits.
// Computed by walking transactions once and emitting a snapshot at
// each unique date boundary.
//
// This is the single source of truth for portfolio state tracking,
// shared by the equity curve and any other consumer that needs to
// know what the portfolio looked like at each point in time.
type PortfolioSnapshot struct {
	Date        time.Time
	Quantities  map[string]decimal.Decimal // symbol → qty (signed: positive=long, negative=short)
	CashBalance map[string]decimal.Decimal // currency → balance
	NetDeposit  map[string]decimal.Decimal // currency → cumulative deposit/withdrawal
}

// WalkPortfolioState walks transactions chronologically and emits a
// PortfolioSnapshot at each unique date. Transaction quantities are
// signed: positive for buys, negative for sells — added directly.
// Cash balance is updated from every transaction's net_cash.
// Net deposit is updated only from deposit/withdrawal transactions.
//
// Transactions must be sorted by date ASC, then ID ASC.
func WalkPortfolioState(txns []transaction.Transaction) []PortfolioSnapshot {
	var snapshots []PortfolioSnapshot
	quantities := make(map[string]decimal.Decimal)
	cashBalance := make(map[string]decimal.Decimal)
	netDeposit := make(map[string]decimal.Decimal)

	for i, txn := range txns {
		// Update position quantities for buy/sell.
		if txn.Type == "buy" || txn.Type == "sell" {
			qty, _ := quantities[txn.Symbol].Add(txn.Quantity)
			quantities[txn.Symbol] = qty
		}

		// Update cash balance for all transaction types.
		bal, _ := cashBalance[txn.Currency].Add(txn.NetCash)
		cashBalance[txn.Currency] = bal

		// Update cumulative net deposit for deposit/withdrawal only.
		if txn.Type == "deposit" || txn.Type == "withdrawal" {
			dep, _ := netDeposit[txn.Currency].Add(txn.NetCash)
			netDeposit[txn.Currency] = dep
		}

		// Emit snapshot at end of each date.
		isLastForDate := i == len(txns)-1 || txns[i+1].Date.After(txn.Date)
		if isLastForDate {
			snapshots = append(snapshots, PortfolioSnapshot{
				Date:        txn.Date,
				Quantities:  copyDecimalMap(quantities),
				CashBalance: copyDecimalMap(cashBalance),
				NetDeposit:  copyDecimalMap(netDeposit),
			})
		}
	}

	return snapshots
}

// FinalPortfolioState returns the portfolio state after all transactions
// have been processed.
func FinalPortfolioState(txns []transaction.Transaction) PortfolioSnapshot {
	quantities := make(map[string]decimal.Decimal)
	cashBalance := make(map[string]decimal.Decimal)
	netDeposit := make(map[string]decimal.Decimal)

	for _, txn := range txns {
		if txn.Type == "buy" || txn.Type == "sell" {
			qty, _ := quantities[txn.Symbol].Add(txn.Quantity)
			quantities[txn.Symbol] = qty
		}
		bal, _ := cashBalance[txn.Currency].Add(txn.NetCash)
		cashBalance[txn.Currency] = bal
		if txn.Type == "deposit" || txn.Type == "withdrawal" {
			dep, _ := netDeposit[txn.Currency].Add(txn.NetCash)
			netDeposit[txn.Currency] = dep
		}
	}

	return PortfolioSnapshot{
		Quantities:  quantities,
		CashBalance: cashBalance,
		NetDeposit:  netDeposit,
	}
}

// copyDecimalMap creates a deep copy of a decimal map.
func copyDecimalMap(src map[string]decimal.Decimal) map[string]decimal.Decimal {
	dst := make(map[string]decimal.Decimal, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// WalkPositionQuantities walks transactions and returns position quantity
// snapshots. Deprecated: use WalkPortfolioState instead for the full
// portfolio state (includes cash and net deposit).
// Kept for backward compatibility with existing tests.
func WalkPositionQuantities(txns []transaction.Transaction) []PositionSnapshot {
	snapshots := WalkPortfolioState(txns)
	result := make([]PositionSnapshot, 0, len(snapshots))
	for _, s := range snapshots {
		result = append(result, PositionSnapshot{
			Date:       s.Date,
			Quantities: s.Quantities,
		})
	}
	return result
}

// PositionSnapshot is the quantity of each symbol at a point in time.
//
// Deprecated: use PortfolioSnapshot instead. Kept for backward compatibility
// with existing tests.
type PositionSnapshot struct {
	Date       time.Time
	Quantities map[string]decimal.Decimal
}

// FinalPositionQuantities returns the position quantities after all
// transactions have been processed. Deprecated: use FinalPortfolioState.
func FinalPositionQuantities(txns []transaction.Transaction) map[string]decimal.Decimal {
	return FinalPortfolioState(txns).Quantities
}
