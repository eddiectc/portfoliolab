package position

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// cashAffectingTypes are transaction types whose net_cash flows into or out
// of the account's cash balance. Only these types contribute to cash positions.
var cashAffectingTypes = map[string]bool{
	"deposit":   true,
	"withdrawal": true,
	"dividend":  true,
	"interest":  true,
	"fee":       true,
	"tax":       true,
	"buy":       true,
	"sell":      true,
}

// ComputeCashPositions computes cash positions from all transactions that
// affect the account's cash balance (deposits, withdrawals, dividends,
// interest, fees, taxes, buys, sells).
//
// Each currency in the account gets one cash position with symbol
// `$CASH-{currency}`. The quantity field holds the running net_cash sum
// (positive = surplus, negative = overdraft). Cost basis and realized P&L
// are always zero — cash positions have no P&L.
//
// Cash positions are always open (never closed).
func ComputeCashPositions(transactions []transaction.Transaction) []Position {
	if len(transactions) == 0 {
		return []Position{}
	}

	// Sum net_cash per currency.
	balances := make(map[string]decimal.Decimal)
	// Track earliest date per currency for the open_date.
	earliestDate := make(map[string]string)

	for _, tx := range transactions {
		if !cashAffectingTypes[tx.Type] {
			continue
		}

		if tx.NetCash.Equal(decimal.Zero) {
			continue
		}

		currency := tx.Currency
		sym := cashSymbol(currency)

		prev, ok := balances[sym]
		if !ok {
			balances[sym] = tx.NetCash
			earliestDate[sym] = tx.Date.Format("2006-01-02")
			continue
		}

		sum, err := prev.Add(tx.NetCash)
		if err != nil {
			panic(fmt.Sprintf("decimal overflow summing cash balance for %s: %v", currency, err))
		}
		balances[sym] = sum

		if tx.Date.Format("2006-01-02") < earliestDate[sym] {
			earliestDate[sym] = tx.Date.Format("2006-01-02")
		}
	}

	if len(balances) == 0 {
		return []Position{}
	}

	// Sort symbols for deterministic output.
	symbols := make([]string, 0, len(balances))
	for sym := range balances {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols)

	positions := make([]Position, 0, len(balances))
	for _, sym := range symbols {
		date, _ := time.Parse("2006-01-02", earliestDate[sym])
		positions = append(positions, Position{
			Symbol:   sym,
			Currency: strings.TrimPrefix(sym, "$CASH-"),
			Quantity: balances[sym],
			OpenDate: date,
			IsClosed: false,
		})
	}

	return positions
}

// cashSymbol returns the cash position symbol for a currency,
// e.g. "$CASH-USD" for "USD".
func cashSymbol(currency string) string {
	return "$CASH-" + currency
}
