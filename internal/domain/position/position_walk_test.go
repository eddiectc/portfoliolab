package position

import (
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

func TestWalkPositionQuantities_Empty(t *testing.T) {
	result := WalkPositionQuantities(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 snapshots, got %d", len(result))
	}
}

func TestWalkPositionQuantities_BuyThenSell(t *testing.T) {
	txns := []transaction.Transaction{
		// Jan 15: buy 10 AAPL
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		// Feb 15: sell 5 AAPL (negative quantity)
		eqTxn(1, testTime(2024, 2, 15), "sell", "AAPL", "USD", -500, 17000, 850000),
	}

	snapshots := WalkPositionQuantities(txns)
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	// Snapshot 1: 10 AAPL
	wantQty1 := decimal.MustNew(1000, 2)
	if !snapshots[0].Quantities["AAPL"].Equal(wantQty1) {
		t.Errorf("snapshot 1 AAPL qty: got %s, want %s", snapshots[0].Quantities["AAPL"].String(), wantQty1.String())
	}

	// Snapshot 2: 10 + (-5) = 5 AAPL
	wantQty2 := decimal.MustNew(500, 2)
	if !snapshots[1].Quantities["AAPL"].Equal(wantQty2) {
		t.Errorf("snapshot 2 AAPL qty: got %s, want %s", snapshots[1].Quantities["AAPL"].String(), wantQty2.String())
	}
}

func TestWalkPositionQuantities_FullyClosed(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 2, 15), "sell", "AAPL", "USD", -1000, 17000, 1700000),
	}

	snapshots := WalkPositionQuantities(txns)
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	// Snapshot 2: position closed to zero
	if !snapshots[1].Quantities["AAPL"].IsZero() {
		t.Errorf("snapshot 2 AAPL qty: expected 0, got %s", snapshots[1].Quantities["AAPL"].String())
	}
}

func TestWalkPositionQuantities_MultipleSymbols(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "MSFT", "USD", 500, 30000, -1500000),
		eqTxn(1, testTime(2024, 3, 15), "sell", "AAPL", "USD", -1000, 17000, 1700000),
	}

	snapshots := WalkPositionQuantities(txns)
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
	}

	// Snapshot 1: 10 AAPL, 0 MSFT
	if !snapshots[0].Quantities["AAPL"].Equal(decimal.MustNew(1000, 2)) {
		t.Errorf("snapshot 1 AAPL: got %s", snapshots[0].Quantities["AAPL"].String())
	}
	if _, ok := snapshots[0].Quantities["MSFT"]; ok {
		t.Errorf("snapshot 1: MSFT should not be in quantities yet")
	}

	// Snapshot 2: 10 AAPL, 5 MSFT
	if !snapshots[1].Quantities["MSFT"].Equal(decimal.MustNew(500, 2)) {
		t.Errorf("snapshot 2 MSFT: got %s", snapshots[1].Quantities["MSFT"].String())
	}

	// Snapshot 3: 0 AAPL, 5 MSFT
	if !snapshots[2].Quantities["AAPL"].IsZero() {
		t.Errorf("snapshot 3 AAPL: expected 0, got %s", snapshots[2].Quantities["AAPL"].String())
	}
	if !snapshots[2].Quantities["MSFT"].Equal(decimal.MustNew(500, 2)) {
		t.Errorf("snapshot 3 MSFT: got %s", snapshots[2].Quantities["MSFT"].String())
	}
}

func TestWalkPositionQuantities_SameDateMultipleTxns(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 500, 15000, -750000), // same day
	}

	snapshots := WalkPositionQuantities(txns)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot (same date), got %d", len(snapshots))
	}

	// Combined: 15 AAPL
	wantQty := decimal.MustNew(1500, 2)
	if !snapshots[0].Quantities["AAPL"].Equal(wantQty) {
		t.Errorf("AAPL qty: got %s, want %s", snapshots[0].Quantities["AAPL"].String(), wantQty.String())
	}
}

func TestFinalPositionQuantities_Empty(t *testing.T) {
	result := FinalPositionQuantities(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

func TestFinalPositionQuantities_BuyThenSell(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 2, 15), "sell", "AAPL", "USD", -500, 17000, 850000),
	}

	result := FinalPositionQuantities(txns)
	wantQty := decimal.MustNew(500, 2)
	if !result["AAPL"].Equal(wantQty) {
		t.Errorf("AAPL qty: got %s, want %s", result["AAPL"].String(), wantQty.String())
	}
}

func TestWalkPortfolioState_FullFlow(t *testing.T) {
	txns := []transaction.Transaction{
		// Jan 15: deposit $10,000
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		// Feb 15: buy 10 AAPL at $150 (net_cash = -$1,500)
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		// Mar 15: sell 5 AAPL at $170 (net_cash = +$850)
		eqTxn(1, testTime(2024, 3, 15), "sell", "AAPL", "USD", -500, 17000, 850000),
	}

	snapshots := WalkPortfolioState(txns)
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
	}

	// Snapshot 1 (Jan 15): no positions, cash = $10,000, net deposit = $10,000
	s1 := snapshots[0]
	if len(s1.Quantities) != 0 {
		t.Errorf("snapshot 1: expected 0 quantities, got %d", len(s1.Quantities))
	}
	if !s1.CashBalance["USD"].Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("snapshot 1 cash: got %s", s1.CashBalance["USD"].String())
	}
	if !s1.NetDeposit["USD"].Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("snapshot 1 net deposit: got %s", s1.NetDeposit["USD"].String())
	}

	// Snapshot 2 (Feb 15): 10 AAPL, cash = -$5,000, net deposit = $10,000
	s2 := snapshots[1]
	if !s2.Quantities["AAPL"].Equal(decimal.MustNew(1000, 2)) {
		t.Errorf("snapshot 2 AAPL qty: got %s", s2.Quantities["AAPL"].String())
	}
	if !s2.CashBalance["USD"].Equal(decimal.MustNew(-500000, 2)) {
		t.Errorf("snapshot 2 cash: got %s", s2.CashBalance["USD"].String())
	}
	if !s2.NetDeposit["USD"].Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("snapshot 2 net deposit: got %s", s2.NetDeposit["USD"].String())
	}

	// Snapshot 3 (Mar 15): 5 AAPL, cash = $350, net deposit = $10,000
	s3 := snapshots[2]
	if !s3.Quantities["AAPL"].Equal(decimal.MustNew(500, 2)) {
		t.Errorf("snapshot 3 AAPL qty: got %s", s3.Quantities["AAPL"].String())
	}
	if !s3.CashBalance["USD"].Equal(decimal.MustNew(350000, 2)) {
		t.Errorf("snapshot 3 cash: got %s", s3.CashBalance["USD"].String())
	}
	if !s3.NetDeposit["USD"].Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("snapshot 3 net deposit: got %s", s3.NetDeposit["USD"].String())
	}
}

func TestFinalPortfolioState(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 3, 15), "sell", "AAPL", "USD", -500, 17000, 850000),
	}

	result := FinalPortfolioState(txns)
	if !result.Quantities["AAPL"].Equal(decimal.MustNew(500, 2)) {
		t.Errorf("AAPL qty: got %s", result.Quantities["AAPL"].String())
	}
	if !result.CashBalance["USD"].Equal(decimal.MustNew(350000, 2)) {
		t.Errorf("cash: got %s", result.CashBalance["USD"].String())
	}
	if !result.NetDeposit["USD"].Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("net deposit: got %s", result.NetDeposit["USD"].String())
	}
}
