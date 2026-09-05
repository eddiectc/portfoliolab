package position

import (
	"testing"
	"time"

	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
)

// dec is shorthand for decimal.MustNew.
func dec(value int64, scale int) decimal.Decimal {
	return decimal.MustNew(value, scale)
}

// ptrStr returns a pointer to a string.
func ptrStr(s string) *string {
	return &s
}

// mustTime parses a YYYY-MM-DD string into time.Time.
func mustTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// makeTxn builds a transaction.Transaction for test fixtures.
func makeTxn(lotID string, txType, symbol, currency string, date string, qty, price, netCash decimal.Decimal) transaction.Transaction {
	return transaction.Transaction{
		AccountID: 1,
		Date:      mustTime(date),
		Type:      txType,
		Symbol:    symbol,
		Quantity:  qty,
		Price:     price,
		Currency:  currency,
		NetCash:   netCash,
		LotID:     ptrStr(lotID),
	}
}

func TestGroupTransactionsIntoLots_SingleBuy(t *testing.T) {
	txn := makeTxn("LOT-1", "buy", "AAPL", "USD", "2025-01-15",
		dec(10, 0), dec(15000, 2), dec(-150000, 2))

	buyLots, sellLots := GroupTransactionsIntoLots([]transaction.Transaction{txn})

	if len(buyLots) != 1 {
		t.Fatalf("expected 1 buy lot, got %d", len(buyLots))
	}
	if len(sellLots) != 0 {
		t.Fatalf("expected 0 sell lots, got %d", len(sellLots))
	}

	lot := buyLots[0]
	if lot.LotID != "LOT-1" {
		t.Errorf("expected LotID LOT-1, got %q", lot.LotID)
	}
	if lot.LotType != "buy" {
		t.Errorf("expected LotType buy, got %q", lot.LotType)
	}
	if !lot.Quantity.Equal(dec(10, 0)) {
		t.Errorf("expected Quantity 10, got %q", lot.Quantity.String())
	}
	if !lot.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected CostBasis -1500.00, got %q", lot.CostBasis.String())
	}
	if !lot.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected OpenDate 2025-01-15, got %q", lot.OpenDate.Format("2006-01-02"))
	}
	if len(lot.Transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(lot.Transactions))
	}
}

func TestGroupTransactionsIntoLots_MultipleBuysSameLot(t *testing.T) {
	// Two buys with same lot_id (e.g., partial fills of the same order).
	txn1 := makeTxn("LOT-1", "buy", "AAPL", "USD", "2025-01-15",
		dec(5, 0), dec(15000, 2), dec(-75000, 2))
	txn2 := makeTxn("LOT-1", "buy", "AAPL", "USD", "2025-01-16",
		dec(5, 0), dec(15000, 2), dec(-75000, 2))

	buyLots, sellLots := GroupTransactionsIntoLots([]transaction.Transaction{txn1, txn2})

	if len(buyLots) != 1 {
		t.Fatalf("expected 1 buy lot, got %d", len(buyLots))
	}
	if len(sellLots) != 0 {
		t.Fatalf("expected 0 sell lots, got %d", len(sellLots))
	}

	lot := buyLots[0]
	if !lot.Quantity.Equal(dec(10, 0)) {
		t.Errorf("expected Quantity 10 (summed), got %q", lot.Quantity.String())
	}
	if !lot.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("expected CostBasis -1500.00 (summed), got %q", lot.CostBasis.String())
	}
	if !lot.OpenDate.Equal(mustTime("2025-01-15")) {
		t.Errorf("expected OpenDate 2025-01-15 (earliest), got %q", lot.OpenDate.Format("2006-01-02"))
	}
	if len(lot.Transactions) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(lot.Transactions))
	}
}

func TestGroupTransactionsIntoLots_MultipleBuysDifferentLots(t *testing.T) {
	txn1 := makeTxn("LOT-1", "buy", "AAPL", "USD", "2025-01-15",
		dec(10, 0), dec(15000, 2), dec(-150000, 2))
	txn2 := makeTxn("LOT-2", "buy", "AAPL", "USD", "2025-02-20",
		dec(5, 0), dec(16000, 2), dec(-80000, 2))

	buyLots, sellLots := GroupTransactionsIntoLots([]transaction.Transaction{txn1, txn2})

	if len(buyLots) != 2 {
		t.Fatalf("expected 2 buy lots, got %d", len(buyLots))
	}
	if len(sellLots) != 0 {
		t.Fatalf("expected 0 sell lots, got %d", len(sellLots))
	}

	if buyLots[0].LotID != "LOT-1" {
		t.Errorf("expected first lot LOT-1, got %q", buyLots[0].LotID)
	}
	if buyLots[1].LotID != "LOT-2" {
		t.Errorf("expected second lot LOT-2, got %q", buyLots[1].LotID)
	}
}

func TestGroupTransactionsIntoLots_MixedBuyAndSell(t *testing.T) {
	txnBuy := makeTxn("LOT-B1", "buy", "AAPL", "USD", "2025-01-15",
		dec(10, 0), dec(15000, 2), dec(-150000, 2))
	txnSell := makeTxn("LOT-S1", "sell", "AAPL", "USD", "2025-03-20",
		dec(-5, 0), dec(17500, 2), dec(87000, 2))

	buyLots, sellLots := GroupTransactionsIntoLots([]transaction.Transaction{txnBuy, txnSell})

	if len(buyLots) != 1 {
		t.Fatalf("expected 1 buy lot, got %d", len(buyLots))
	}
	if len(sellLots) != 1 {
		t.Fatalf("expected 1 sell lot, got %d", len(sellLots))
	}

	// Verify buy lot.
	if buyLots[0].LotID != "LOT-B1" {
		t.Errorf("expected buy lot LOT-B1, got %q", buyLots[0].LotID)
	}
	if !buyLots[0].Quantity.Equal(dec(10, 0)) {
		t.Errorf("expected buy quantity 10, got %q", buyLots[0].Quantity.String())
	}

	// Verify sell lot.
	if sellLots[0].LotID != "LOT-S1" {
		t.Errorf("expected sell lot LOT-S1, got %q", sellLots[0].LotID)
	}
	if !sellLots[0].Quantity.Equal(dec(-5, 0)) {
		t.Errorf("expected sell quantity -5, got %q", sellLots[0].Quantity.String())
	}
	if !sellLots[0].SellProceeds.Equal(dec(87000, 2)) {
		t.Errorf("expected SellProceeds 870.00, got %q", sellLots[0].SellProceeds.String())
	}
}

func TestGroupTransactionsIntoLots_EmptyTransactions(t *testing.T) {
	buyLots, sellLots := GroupTransactionsIntoLots(nil)

	if len(buyLots) != 0 {
		t.Errorf("expected 0 buy lots, got %d", len(buyLots))
	}
	if len(sellLots) != 0 {
		t.Errorf("expected 0 sell lots, got %d", len(sellLots))
	}
}

func TestGroupTransactionsIntoLots_ChronologicalSorting(t *testing.T) {
	// Three buy lots in reverse chronological order of creation.
	txn1 := makeTxn("LOT-3", "buy", "AAPL", "USD", "2025-03-01",
		dec(5, 0), dec(17000, 2), dec(-85000, 2))
	txn2 := makeTxn("LOT-1", "buy", "AAPL", "USD", "2025-01-15",
		dec(10, 0), dec(15000, 2), dec(-150000, 2))
	txn3 := makeTxn("LOT-2", "buy", "AAPL", "USD", "2025-02-20",
		dec(5, 0), dec(16000, 2), dec(-80000, 2))

	buyLots, _ := GroupTransactionsIntoLots([]transaction.Transaction{txn1, txn2, txn3})

	if len(buyLots) != 3 {
		t.Fatalf("expected 3 buy lots, got %d", len(buyLots))
	}

	// Should be sorted by open_date: LOT-1 (Jan), LOT-2 (Feb), LOT-3 (Mar).
	wantOrder := []string{"LOT-1", "LOT-2", "LOT-3"}
	for i, want := range wantOrder {
		if buyLots[i].LotID != want {
			t.Errorf("position %d: expected %q, got %q", i, want, buyLots[i].LotID)
		}
	}
}

func TestGroupTransactionsIntoLots_NonTradeTransactionsExcluded(t *testing.T) {
	// Deposit, withdrawal, dividend, etc. should not appear in lots.
	deposit := makeTxn("", "deposit", "$CASH-USD", "USD", "2025-01-01",
		dec(1000000, 2), dec(1, 0), dec(1000000, 2))
	deposit.LotID = nil // deposits don't get lot_ids

	buy := makeTxn("LOT-1", "buy", "AAPL", "USD", "2025-01-15",
		dec(10, 0), dec(15000, 2), dec(-150000, 2))

	buyLots, sellLots := GroupTransactionsIntoLots([]transaction.Transaction{deposit, buy})

	if len(buyLots) != 1 {
		t.Fatalf("expected 1 buy lot (deposit excluded), got %d", len(buyLots))
	}
	if len(sellLots) != 0 {
		t.Fatalf("expected 0 sell lots, got %d", len(sellLots))
	}
}

func TestGroupTransactionsIntoLots_EmptyLotIDExcluded(t *testing.T) {
	// Transaction with nil or empty lot_id should be excluded.
	txn := makeTxn("", "buy", "AAPL", "USD", "2025-01-15",
		dec(10, 0), dec(15000, 2), dec(-150000, 2))
	// ptrStr("") → empty string lot_id
	buyLots, sellLots := GroupTransactionsIntoLots([]transaction.Transaction{txn})

	if len(buyLots) != 0 {
		t.Errorf("expected 0 buy lots (empty lot_id excluded), got %d", len(buyLots))
	}
	if len(sellLots) != 0 {
		t.Errorf("expected 0 sell lots, got %d", len(sellLots))
	}
}

func TestGroupTransactionsIntoLots_MultipleSellsDifferentLots(t *testing.T) {
	sell1 := makeTxn("LOT-S1", "sell", "AAPL", "USD", "2025-03-20",
		dec(-3, 0), dec(17500, 2), dec(52000, 2))
	sell2 := makeTxn("LOT-S2", "sell", "AAPL", "USD", "2025-04-10",
		dec(-7, 0), dec(18000, 2), dec(125000, 2))

	_, sellLots := GroupTransactionsIntoLots([]transaction.Transaction{sell1, sell2})

	if len(sellLots) != 2 {
		t.Fatalf("expected 2 sell lots, got %d", len(sellLots))
	}

	// Sorted chronologically: S1 (Mar) then S2 (Apr).
	if sellLots[0].LotID != "LOT-S1" {
		t.Errorf("expected first sell lot LOT-S1, got %q", sellLots[0].LotID)
	}
	if sellLots[1].LotID != "LOT-S2" {
		t.Errorf("expected second sell lot LOT-S2, got %q", sellLots[1].LotID)
	}

	// Verify aggregated sell proceeds.
	if !sellLots[0].SellProceeds.Equal(dec(52000, 2)) {
		t.Errorf("expected S1 SellProceeds 520.00, got %q", sellLots[0].SellProceeds.String())
	}
	if !sellLots[1].SellProceeds.Equal(dec(125000, 2)) {
		t.Errorf("expected S2 SellProceeds 1250.00, got %q", sellLots[1].SellProceeds.String())
	}
}

func TestGroupTransactionsIntoLots_LotGroupHasCorrectAccountAndSymbol(t *testing.T) {
	txn := makeTxn("LOT-1", "buy", "MSFT", "EUR", "2025-06-01",
		dec(20, 0), dec(40000, 2), dec(-800000, 2))
	txn.AccountID = 42

	buyLots, _ := GroupTransactionsIntoLots([]transaction.Transaction{txn})

	if len(buyLots) != 1 {
		t.Fatalf("expected 1 buy lot, got %d", len(buyLots))
	}

	lot := buyLots[0]
	if lot.AccountID != 42 {
		t.Errorf("expected AccountID 42, got %d", lot.AccountID)
	}
	if lot.Symbol != "MSFT" {
		t.Errorf("expected Symbol MSFT, got %q", lot.Symbol)
	}
}

func TestGroupTransactionsIntoLots_LotIDCaseSensitive(t *testing.T) {
	// LOT-ABC and lot-abc are different lot IDs (case-sensitive).
	txn1 := makeTxn("LOT-ABC", "buy", "AAPL", "USD", "2025-01-15",
		dec(10, 0), dec(15000, 2), dec(-150000, 2))
	txn2 := makeTxn("lot-abc", "buy", "AAPL", "USD", "2025-02-15",
		dec(5, 0), dec(16000, 2), dec(-80000, 2))

	buyLots, _ := GroupTransactionsIntoLots([]transaction.Transaction{txn1, txn2})

	if len(buyLots) != 2 {
		t.Fatalf("expected 2 buy lots (case-sensitive), got %d", len(buyLots))
	}

	// Verify both lot IDs are preserved as-is.
	lotIDs := make(map[string]bool)
	for _, lot := range buyLots {
		lotIDs[lot.LotID] = true
	}
	if !lotIDs["LOT-ABC"] {
		t.Error("expected LOT-ABC lot")
	}
	if !lotIDs["lot-abc"] {
		t.Error("expected lot-abc lot")
	}
}
