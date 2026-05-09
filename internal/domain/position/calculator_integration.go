package position

import (
	"context"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// CalculatePositions runs the full position calculation pipeline:
//
//  1. Group transactions into buy/sell lots (GroupTransactionsIntoLots)
//  2. Match sell lots against buy lots via FIFO (MatchSellLotsAgainstBuys)
//  3. Compute open and closed positions from matched lots (ComputePositions)
//  4. Compute cash positions from cash-affecting transactions (ComputeCashPositions)
//
// Returns a CalculateResult with all computed positions, lots, and consumptions.
// The lots and consumptions in the result have DB fields (ID, CreatedAt, UpdatedAt)
// set to zero — they are populated by the service layer when persisted.
func CalculatePositions(_ context.Context, accountID int64, transactions []transaction.Transaction) (*CalculateResult, error) {
	// Step 1: Group transactions into buy and sell lots.
	buyLots, sellLots := GroupTransactionsIntoLots(transactions)

	// Step 2: Match sell lots against buy lots using FIFO.
	consumptions, _ := MatchSellLotsAgainstBuys(buyLots, sellLots)

	// Step 3: Compute open and closed positions from the matched lots.
	openPositions, closedPositions := ComputePositions(buyLots, sellLots)

	// Step 4: Compute cash positions from cash-affecting transactions.
	cashPositions := ComputeCashPositions(transactions)

	// Convert LotGroups to Lot domain models (DB fields left zero — set by service layer).
	lots := lotsToDomain(buyLots, sellLots)

	// Inject account_id into positions (calculator doesn't know the account).
	for i := range openPositions {
		openPositions[i].AccountID = accountID
	}
	for i := range closedPositions {
		closedPositions[i].AccountID = accountID
	}
	for i := range cashPositions {
		cashPositions[i].AccountID = accountID
	}

	return &CalculateResult{
		OpenPositions:   openPositions,
		ClosedPositions: closedPositions,
		CashPositions:   cashPositions,
		Lots:            lots,
		Consumptions:    consumptions,
	}, nil
}

// lotsToDomain converts LotGroups (calculator intermediate) to Lot domain models.
// DB fields (ID, CreatedAt, UpdatedAt) are left at zero values — they are
// populated when the service layer persists the results.
func lotsToDomain(buyLots, sellLots []LotGroup) []Lot {
	lots := make([]Lot, 0, len(buyLots)+len(sellLots))

	for _, lot := range buyLots {
		lots = append(lots, Lot{
			LotID:     lot.LotID,
			AccountID: lot.AccountID,
			Symbol:    lot.Symbol,
			LotType:   "buy",
			Quantity:  lot.Quantity,
			CostBasis: lot.CostBasis,
			OpenDate:  lot.OpenDate,
		})
	}

	for _, lot := range sellLots {
		absQty := lot.Quantity.Abs()
		lots = append(lots, Lot{
			LotID:     lot.LotID,
			AccountID: lot.AccountID,
			Symbol:    lot.Symbol,
			LotType:   "sell",
			Quantity:  absQty,
			SellPrice: ptrDecimal(lot.SellProceeds),
			OpenDate:  lot.OpenDate,
		})
	}

	return lots
}

// ptrDecimal returns a pointer to a decimal.Decimal.
func ptrDecimal(d decimal.Decimal) *decimal.Decimal {
	return &d
}
