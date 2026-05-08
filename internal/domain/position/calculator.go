package position

import (
	"fmt"
	"sort"

	"github.com/govalues/decimal"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
)

// GroupTransactionsIntoLots separates buy and sell transactions, groups them
// by lot_id, and computes aggregated lot quantities and cash flows.
//
// Returns (buyLots, sellLots) sorted chronologically by open_date.
// Only buy/sell transactions with a non-nil lot_id are included.
// Deposit, withdrawal, dividend, interest, fee, and tax transactions are
// excluded from lot grouping.
func GroupTransactionsIntoLots(transactions []transaction.Transaction) ([]LotGroup, []LotGroup) {
	buyMap := make(map[string][]transaction.Transaction)
	sellMap := make(map[string][]transaction.Transaction)

	for _, tx := range transactions {
		if tx.LotID == nil || *tx.LotID == "" {
			continue
		}

		switch tx.Type {
		case "buy":
			buyMap[*tx.LotID] = append(buyMap[*tx.LotID], tx)
		case "sell":
			sellMap[*tx.LotID] = append(sellMap[*tx.LotID], tx)
		}
	}

	buyLots := buildLots(buyMap)
	sellLots := buildLots(sellMap)

	return buyLots, sellLots
}

// buildLots constructs LotGroups from a map of lot_id → transactions.
func buildLots(txnMap map[string][]transaction.Transaction) []LotGroup {
	lots := make([]LotGroup, 0, len(txnMap))

	for lotID, txns := range txnMap {
		// Sort transactions by date within the lot for deterministic ordering.
		sort.Slice(txns, func(i, j int) bool {
			return txns[i].Date.Before(txns[j].Date)
		})

		var (
			quantity  decimal.Decimal = decimal.Zero
			netCash   decimal.Decimal = decimal.Zero
			openDate                  = txns[0].Date
		)

		refs := make([]TransactionRef, 0, len(txns))
		for _, tx := range txns {
			var err error
			quantity, err = quantity.Add(tx.Quantity)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow summing lot quantities: %v", err))
			}
			netCash, err = netCash.Add(tx.NetCash)
			if err != nil {
				panic(fmt.Sprintf("decimal overflow summing lot net_cash: %v", err))
			}
			if tx.Date.Before(openDate) {
				openDate = tx.Date
			}
			refs = append(refs, toTransactionRef(tx))
		}

		lotType := txns[0].Type
		lot := LotGroup{
			LotID:        lotID,
			AccountID:    txns[0].AccountID,
			Symbol:       txns[0].Symbol,
			LotType:      lotType,
			Transactions: refs,
			OpenDate:     openDate,
			Quantity:     quantity,
			CostBasis:    netCash, // for buys: negative (cash outflow)
		}

		if lotType == "sell" {
			lot.SellProceeds = netCash // for sells: positive (cash inflow)
		}

		lots = append(lots, lot)
	}

	// Sort lots chronologically by open_date.
	sort.Slice(lots, func(i, j int) bool {
		return lots[i].OpenDate.Before(lots[j].OpenDate)
	})

	return lots
}

// toTransactionRef converts a domain Transaction to a lightweight TransactionRef.
func toTransactionRef(tx transaction.Transaction) TransactionRef {
	return TransactionRef{
		ID:       tx.ID,
		Date:     tx.Date,
		Type:     tx.Type,
		Symbol:   tx.Symbol,
		Quantity: tx.Quantity,
		Price:    tx.Price,
		Currency: tx.Currency,
		NetCash:  tx.NetCash,
	}
}

// SortLotsByDate sorts a slice of LotGroups chronologically by open_date.
// Provided as a helper for consumers that need custom ordering.
func SortLotsByDate(lots []LotGroup) {
	sort.Slice(lots, func(i, j int) bool {
		return lots[i].OpenDate.Before(lots[j].OpenDate)
	})
}
