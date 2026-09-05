package position

import (
	"context"
	"testing"

	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
)

func TestCalculatePositions_FullFlowWithOpenAndCashPosition(t *testing.T) {
	// Deposits + buys + sells → open position + cash position.
	ctx := context.Background()
	txns := []transaction.Transaction{
		// Deposit $10,000 (no lot_id — affects cash only).
		{
			AccountID: 3, Date: mustTime("2025-01-01"), Type: "deposit",
			Symbol: "$CASH-USD", Currency: "USD",
			Quantity: dec(1000000, 2), Price: dec(1, 0), NetCash: dec(1000000, 2),
		},
		// Buy 10 AAPL @ $150 (net_cash = -$1,500).
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(10, 0), Price: dec(15000, 2), NetCash: dec(-150000, 2),
			LotID: ptrStr("LOT-B1"),
		},
		// Sell 3 AAPL @ $170 (net_cash = $510).
		{
			AccountID: 3, Date: mustTime("2025-03-20"), Type: "sell",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(-3, 0), Price: dec(17000, 2), NetCash: dec(51000, 2),
			LotID: ptrStr("LOT-S1"),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 open AAPL position.
	if len(result.OpenPositions) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(result.OpenPositions))
	}

	aaplPos := result.OpenPositions[0]
	if aaplPos.Symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got %q", aaplPos.Symbol)
	}
	if aaplPos.AccountID != 3 {
		t.Errorf("expected AccountID 3, got %d", aaplPos.AccountID)
	}
	if !aaplPos.Quantity.Equal(dec(7, 0)) {
		t.Errorf("expected quantity 7, got %q", aaplPos.Quantity.String())
	}
	// Cost basis is the remaining buy cost: -1500 + proportional sell (3/10 of 1500 = 450) = -1050
	// Actually cost_basis from position computation = totalCostBasis of remaining lots
	// The open position has cost_basis = totalCostBasis (negative) from unmatched buys
	// After selling 3 from 10, remaining buy cost basis = -1500 * (7/10) = -1050
	if !aaplPos.IsClosed {
		// Open position: cost_basis should reflect remaining shares
		if aaplPos.CostBasis.Sign() >= 0 {
			t.Errorf("expected negative cost basis, got %q", aaplPos.CostBasis.String())
		}
	}

	// Open position: RealizedPnL = 0 (sell proceeds are in cash balance)
	if !aaplPos.RealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected realized P&L 0 (open position), got %q", aaplPos.RealizedPnL.String())
	}

	// One closed position: the 3 shares sold by S1 (per-sell-lot rows).
	if len(result.ClosedPositions) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(result.ClosedPositions))
	}
	closedPos := result.ClosedPositions[0]
	if !closedPos.Quantity.Equal(dec(3, 0)) {
		t.Errorf("expected closed quantity 3, got %q", closedPos.Quantity.String())
	}
	// Cost basis = 3/10 of -1500 = -450; proceeds 510 → P&L +60
	if !closedPos.CostBasis.Equal(dec(-45000, 2)) {
		t.Errorf("expected closed cost basis -450.00, got %q", closedPos.CostBasis.String())
	}
	if !closedPos.RealizedPnL.Equal(dec(6000, 2)) {
		t.Errorf("expected closed realized P&L 60.00, got %q", closedPos.RealizedPnL.String())
	}
	if closedPos.AvgClosePrice == nil || !closedPos.AvgClosePrice.Equal(dec(17000, 2)) {
		t.Errorf("expected closed avg close price 170.00, got %v", closedPos.AvgClosePrice)
	}
	if closedPos.CloseDate == nil || !closedPos.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected close date 2025-03-20, got %v", closedPos.CloseDate)
	}

	// Open position cost basis: the remaining 7 shares, FIFO from B1: -1050.
	if !aaplPos.CostBasis.Equal(dec(-105000, 2)) {
		t.Errorf("expected open cost basis -1050.00, got %q", aaplPos.CostBasis.String())
	}
	if !aaplPos.AvgOpenPrice.Equal(dec(15000, 2)) {
		t.Errorf("expected open avg open price 150.00, got %q", aaplPos.AvgOpenPrice.String())
	}

	// Cash position: deposit 10000 - buy 1500 + sell 510 = 9010
	// (no double counting — proceeds flow into cash exactly once).
	if len(result.CashPositions) != 1 {
		t.Fatalf("expected 1 cash position, got %d", len(result.CashPositions))
	}

	cashPos := result.CashPositions[0]
	if cashPos.Symbol != "$CASH-USD" {
		t.Errorf("expected symbol $CASH-USD, got %q", cashPos.Symbol)
	}
	if !cashPos.Quantity.Equal(dec(901000, 2)) {
		t.Errorf("expected cash balance 9010.00, got %q", cashPos.Quantity.String())
	}

	// Lots: 1 buy + 1 sell = 2
	if len(result.Lots) != 2 {
		t.Errorf("expected 2 lots, got %d", len(result.Lots))
	}

	// Consumptions: 1 (sell lot consumed from buy lot)
	if len(result.Consumptions) != 1 {
		t.Errorf("expected 1 consumption, got %d", len(result.Consumptions))
	}
}

func TestCalculatePositions_FullFlowClosedPosition(t *testing.T) {
	// Buy + sell all → closed position + cash position.
	ctx := context.Background()
	txns := []transaction.Transaction{
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "TSLA", Currency: "USD",
			Quantity: dec(100, 0), Price: dec(20000, 2), NetCash: dec(-2000000, 2),
			LotID: ptrStr("LOT-B1"),
		},
		{
			AccountID: 3, Date: mustTime("2025-03-20"), Type: "sell",
			Symbol: "TSLA", Currency: "USD",
			Quantity: dec(-40, 0), Price: dec(25000, 2), NetCash: dec(1000000, 2),
			LotID: ptrStr("LOT-S1"),
		},
		{
			AccountID: 3, Date: mustTime("2025-04-10"), Type: "sell",
			Symbol: "TSLA", Currency: "USD",
			Quantity: dec(-60, 0), Price: dec(22000, 2), NetCash: dec(1320000, 2),
			LotID: ptrStr("LOT-S2"),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No open positions (all sold).
	if len(result.OpenPositions) != 0 {
		t.Errorf("expected 0 open positions, got %d", len(result.OpenPositions))
	}

	// 2 closed positions — one per sell lot (R1: sell-based rows).
	if len(result.ClosedPositions) != 2 {
		t.Fatalf("expected 2 closed positions, got %d", len(result.ClosedPositions))
	}

	// S1: 40 shares sold @ $250.
	s1 := result.ClosedPositions[0]
	if s1.Symbol != "TSLA" {
		t.Errorf("expected symbol TSLA, got %q", s1.Symbol)
	}
	if s1.AccountID != 3 {
		t.Errorf("expected AccountID 3, got %d", s1.AccountID)
	}
	if !s1.Quantity.Equal(dec(40, 0)) {
		t.Errorf("expected S1 quantity 40, got %q", s1.Quantity.String())
	}
	if !s1.IsClosed {
		t.Error("expected S1 to be closed")
	}
	// S1 cost basis = 40/100 of -20,000 = -8,000; proceeds 10,000 → P&L +2,000
	if !s1.CostBasis.Equal(dec(-800000, 2)) {
		t.Errorf("expected S1 cost basis -8000.00, got %q", s1.CostBasis.String())
	}
	if !s1.RealizedPnL.Equal(dec(200000, 2)) {
		t.Errorf("expected S1 realized P&L 2000.00, got %q", s1.RealizedPnL.String())
	}
	if s1.AvgClosePrice == nil || !s1.AvgClosePrice.Equal(dec(25000, 2)) {
		t.Errorf("expected S1 avg close price 250.00, got %v", s1.AvgClosePrice)
	}
	if s1.CloseDate == nil || !s1.CloseDate.Equal(mustTime("2025-03-20")) {
		t.Errorf("expected S1 close date 2025-03-20, got %v", s1.CloseDate)
	}

	// S2: 60 shares sold @ $220.
	s2 := result.ClosedPositions[1]
	if !s2.Quantity.Equal(dec(60, 0)) {
		t.Errorf("expected S2 quantity 60, got %q", s2.Quantity.String())
	}
	// S2 cost basis = 60/100 of -20,000 = -12,000; proceeds 13,200 → P&L +1,200
	if !s2.CostBasis.Equal(dec(-1200000, 2)) {
		t.Errorf("expected S2 cost basis -12000.00, got %q", s2.CostBasis.String())
	}
	if !s2.RealizedPnL.Equal(dec(120000, 2)) {
		t.Errorf("expected S2 realized P&L 1200.00, got %q", s2.RealizedPnL.String())
	}
	if s2.AvgClosePrice == nil || !s2.AvgClosePrice.Equal(dec(22000, 2)) {
		t.Errorf("expected S2 avg close price 220.00, got %v", s2.AvgClosePrice)
	}

	// Total realized P&L: sell inflow $23,200 + buy outflow -$20,000 = $3,200
	totalPnL, err := s1.RealizedPnL.Add(s2.RealizedPnL)
	if err != nil {
		t.Fatalf("decimal add: %v", err)
	}
	if !totalPnL.Equal(dec(320000, 2)) {
		t.Errorf("expected total realized P&L 3200.00, got %q", totalPnL.String())
	}
}

func TestCalculatePositions_MultipleSymbols(t *testing.T) {
	// Multiple symbols in same account → separate positions.
	ctx := context.Background()
	txns := []transaction.Transaction{
		// AAPL buy
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(10, 0), Price: dec(15000, 2), NetCash: dec(-150000, 2),
			LotID: ptrStr("LOT-A1"),
		},
		// MSFT buy
		{
			AccountID: 3, Date: mustTime("2025-01-20"), Type: "buy",
			Symbol: "MSFT", Currency: "USD",
			Quantity: dec(5, 0), Price: dec(40000, 2), NetCash: dec(-200000, 2),
			LotID: ptrStr("LOT-M1"),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.OpenPositions) != 2 {
		t.Fatalf("expected 2 open positions, got %d", len(result.OpenPositions))
	}

	// Verify both symbols present.
	symbols := make(map[string]bool)
	for _, pos := range result.OpenPositions {
		symbols[pos.Symbol] = true
	}
	if !symbols["AAPL"] || !symbols["MSFT"] {
		t.Errorf("expected both AAPL and MSFT, got %v", symbols)
	}
}

func TestCalculatePositions_EndToEndFIFO(t *testing.T) {
	// Full FIFO matching: two buy lots, one sell lot consuming from both.
	ctx := context.Background()
	txns := []transaction.Transaction{
		// Buy lot L1: 100 shares @ $150 (Jan)
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(100, 0), Price: dec(15000, 2), NetCash: dec(-1500000, 2),
			LotID: ptrStr("LOT-L1"),
		},
		// Buy lot L2: 50 shares @ $160 (Feb)
		{
			AccountID: 3, Date: mustTime("2025-02-20"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(50, 0), Price: dec(16000, 2), NetCash: dec(-800000, 2),
			LotID: ptrStr("LOT-L2"),
		},
		// Sell 120 shares @ $170 (Mar) — consumes all 100 from L1 + 20 from L2
		{
			AccountID: 3, Date: mustTime("2025-03-20"), Type: "sell",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(-120, 0), Price: dec(17000, 2), NetCash: dec(2040000, 2),
			LotID: ptrStr("LOT-S1"),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 open position (30 shares remaining from L2).
	if len(result.OpenPositions) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(result.OpenPositions))
	}

	pos := result.OpenPositions[0]
	if !pos.Quantity.Equal(dec(30, 0)) {
		t.Errorf("expected quantity 30, got %q", pos.Quantity.String())
	}

	// 2 consumptions: sell lot consumed from L1 (100) and L2 (20).
	if len(result.Consumptions) != 2 {
		t.Fatalf("expected 2 consumptions, got %d", len(result.Consumptions))
	}

	// First consumption: from L1 (100 shares).
	if result.Consumptions[0].BuyLotID != "LOT-L1" {
		t.Errorf("expected first consumption from LOT-L1, got %q", result.Consumptions[0].BuyLotID)
	}
	if !result.Consumptions[0].QuantityConsumed.Equal(dec(100, 0)) {
		t.Errorf("expected consume 100 from L1, got %q", result.Consumptions[0].QuantityConsumed.String())
	}

	// Second consumption: from L2 (20 shares).
	if result.Consumptions[1].BuyLotID != "LOT-L2" {
		t.Errorf("expected second consumption from LOT-L2, got %q", result.Consumptions[1].BuyLotID)
	}
	if !result.Consumptions[1].QuantityConsumed.Equal(dec(20, 0)) {
		t.Errorf("expected consume 20 from L2, got %q", result.Consumptions[1].QuantityConsumed.String())
	}

	// Total realized P&L from consumptions:
	// L1: sell 100 @ $170 ($17,000) + cost -$15,000 = $2,000
	// L2: sell 20 @ $170 ($3,400) + cost -$320 (20/50 * $1,600) = $200
	// Total: $2,200
	totalPnL := decimal.Zero
	for _, c := range result.Consumptions {
		p, err := totalPnL.Add(c.RealizedPnL)
		if err != nil {
			t.Fatalf("decimal error: %v", err)
		}
		totalPnL = p
	}
	// Use approximate comparison due to Quo(20, 120) = 1/6 (repeating decimal).
	wantPnL := dec(220000, 2)
	diff, _ := totalPnL.Sub(wantPnL)
	tolerance := decimal.MustNew(1, 6) // 0.000001
	if !diff.Abs().Less(tolerance) {
		t.Errorf("expected total realized P&L ~2200.00, got %q", totalPnL.String())
	}
}

func TestCalculatePositions_EmptyTransactions(t *testing.T) {
	ctx := context.Background()
	result, err := CalculatePositions(ctx, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.OpenPositions) != 0 {
		t.Errorf("expected 0 open positions, got %d", len(result.OpenPositions))
	}
	if len(result.ClosedPositions) != 0 {
		t.Errorf("expected 0 closed positions, got %d", len(result.ClosedPositions))
	}
	if len(result.CashPositions) != 0 {
		t.Errorf("expected 0 cash positions, got %d", len(result.CashPositions))
	}
	if len(result.Lots) != 0 {
		t.Errorf("expected 0 lots, got %d", len(result.Lots))
	}
	if len(result.Consumptions) != 0 {
		t.Errorf("expected 0 consumptions, got %d", len(result.Consumptions))
	}
}

func TestCalculatePositions_OnlyNonTradeTransactions(t *testing.T) {
	// Only deposits/withdrawals → cash position only, no share positions.
	ctx := context.Background()
	txns := []transaction.Transaction{
		{
			AccountID: 3, Date: mustTime("2025-01-01"), Type: "deposit",
			Symbol: "$CASH-USD", Currency: "USD",
			Quantity: dec(500000, 2), Price: dec(1, 0), NetCash: dec(500000, 2),
		},
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "withdrawal",
			Symbol: "$CASH-USD", Currency: "USD",
			Quantity: dec(200000, 2), Price: dec(1, 0), NetCash: dec(-200000, 2),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.OpenPositions) != 0 {
		t.Errorf("expected 0 open positions (no buys/sells), got %d", len(result.OpenPositions))
	}

	// Cash position: 5000 - 2000 = 3000
	if len(result.CashPositions) != 1 {
		t.Fatalf("expected 1 cash position, got %d", len(result.CashPositions))
	}

	if !result.CashPositions[0].Quantity.Equal(dec(300000, 2)) {
		t.Errorf("expected cash balance 3000.00, got %q", result.CashPositions[0].Quantity.String())
	}
}

func TestCalculatePositions_MultipleCycles(t *testing.T) {
	// Buy 100, sell 100, buy 50, sell 50 → two closed positions.
	ctx := context.Background()
	txns := []transaction.Transaction{
		{
			AccountID: 3, Date: mustTime("2025-01-01"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(100, 0), Price: dec(15000, 2), NetCash: dec(-1500000, 2),
			LotID: ptrStr("LOT-B1"),
		},
		{
			AccountID: 3, Date: mustTime("2025-02-01"), Type: "sell",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(-100, 0), Price: dec(17000, 2), NetCash: dec(1700000, 2),
			LotID: ptrStr("LOT-S1"),
		},
		{
			AccountID: 3, Date: mustTime("2025-03-01"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(50, 0), Price: dec(18000, 2), NetCash: dec(-900000, 2),
			LotID: ptrStr("LOT-B2"),
		},
		{
			AccountID: 3, Date: mustTime("2025-04-01"), Type: "sell",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(-50, 0), Price: dec(19000, 2), NetCash: dec(950000, 2),
			LotID: ptrStr("LOT-S2"),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No open positions (both cycles closed).
	if len(result.OpenPositions) != 0 {
		t.Errorf("expected 0 open positions, got %d", len(result.OpenPositions))
	}

	// Two closed positions.
	if len(result.ClosedPositions) != 2 {
		t.Fatalf("expected 2 closed positions, got %d", len(result.ClosedPositions))
	}

	// First cycle: 100 shares
	if !result.ClosedPositions[0].Quantity.Equal(dec(100, 0)) {
		t.Errorf("expected first cycle quantity 100, got %q", result.ClosedPositions[0].Quantity.String())
	}

	// Second cycle: 50 shares
	if !result.ClosedPositions[1].Quantity.Equal(dec(50, 0)) {
		t.Errorf("expected second cycle quantity 50, got %q", result.ClosedPositions[1].Quantity.String())
	}
}

func TestCalculatePositions_AccountIDInjected(t *testing.T) {
	ctx := context.Background()
	txns := []transaction.Transaction{
		{
			AccountID: 7, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(10, 0), Price: dec(15000, 2), NetCash: dec(-150000, 2),
			LotID: ptrStr("LOT-B1"),
		},
	}

	result, err := CalculatePositions(ctx, 7, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify account_id is injected into all positions.
	for _, pos := range result.OpenPositions {
		if pos.AccountID != 7 {
			t.Errorf("open position AccountID: expected 7, got %d", pos.AccountID)
		}
	}
	for _, pos := range result.ClosedPositions {
		if pos.AccountID != 7 {
			t.Errorf("closed position AccountID: expected 7, got %d", pos.AccountID)
		}
	}
	for _, pos := range result.CashPositions {
		if pos.AccountID != 7 {
			t.Errorf("cash position AccountID: expected 7, got %d", pos.AccountID)
		}
	}
}

func TestCalculatePositions_LotsHaveCorrectFields(t *testing.T) {
	ctx := context.Background()
	txns := []transaction.Transaction{
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(10, 0), Price: dec(15000, 2), NetCash: dec(-150000, 2),
			LotID: ptrStr("LOT-B1"),
		},
		{
			AccountID: 3, Date: mustTime("2025-03-20"), Type: "sell",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(-5, 0), Price: dec(17000, 2), NetCash: dec(85000, 2),
			LotID: ptrStr("LOT-S1"),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Lots) != 2 {
		t.Fatalf("expected 2 lots, got %d", len(result.Lots))
	}

	// Find buy lot.
	var buyLot, sellLot *Lot
	for i := range result.Lots {
		if result.Lots[i].LotType == "buy" {
			buyLot = &result.Lots[i]
		} else {
			sellLot = &result.Lots[i]
		}
	}

	if buyLot == nil || sellLot == nil {
		t.Fatal("expected both buy and sell lots")
	}

	// Buy lot: positive quantity, cost basis set, no sell price.
	if !buyLot.Quantity.Equal(dec(10, 0)) {
		t.Errorf("buy lot quantity: expected 10, got %q", buyLot.Quantity.String())
	}
	if !buyLot.CostBasis.Equal(dec(-150000, 2)) {
		t.Errorf("buy lot cost basis: expected -1500.00, got %q", buyLot.CostBasis.String())
	}
	if buyLot.SellPrice != nil {
		t.Errorf("buy lot should not have sell price")
	}

	// Sell lot: positive quantity (abs), sell price set.
	if !sellLot.Quantity.Equal(dec(5, 0)) {
		t.Errorf("sell lot quantity: expected 5 (abs), got %q", sellLot.Quantity.String())
	}
	if sellLot.SellPrice == nil {
		t.Error("sell lot should have sell price")
	} else if !sellLot.SellPrice.Equal(dec(85000, 2)) {
		t.Errorf("sell lot sell price: expected 850.00, got %q", sellLot.SellPrice.String())
	}

	// DB fields should be zero (set by service layer on persist).
	if buyLot.ID != 0 {
		t.Errorf("buy lot ID should be 0, got %d", buyLot.ID)
	}
	if !buyLot.CreatedAt.IsZero() {
		t.Errorf("buy lot CreatedAt should be zero, got %v", buyLot.CreatedAt)
	}
}

func TestCalculatePositions_ShortPosition(t *testing.T) {
	// Sell without matching buy → short position.
	ctx := context.Background()
	txns := []transaction.Transaction{
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "sell",
			Symbol: "TSLA", Currency: "USD",
			Quantity: dec(-100, 0), Price: dec(20000, 2), NetCash: dec(2000000, 2),
			LotID: ptrStr("LOT-S1"),
		},
		{
			AccountID: 3, Date: mustTime("2025-02-01"), Type: "buy",
			Symbol: "TSLA", Currency: "USD",
			Quantity: dec(30, 0), Price: dec(18000, 2), NetCash: dec(-540000, 2),
			LotID: ptrStr("LOT-B1"),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 open position with 70 shares short.
	if len(result.OpenPositions) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(result.OpenPositions))
	}

	pos := result.OpenPositions[0]
	// Short position: signed negative quantity (70 shares owed).
	if !pos.Quantity.Equal(dec(-70, 0)) {
		t.Errorf("expected quantity -70 (short), got %q", pos.Quantity.String())
	}
	// A short sell (no buy before the sell date) matches no buy lot →
	// no closed position row.
	if len(result.ClosedPositions) != 0 {
		t.Errorf("expected 0 closed positions, got %d", len(result.ClosedPositions))
	}
}

func TestCalculatePositions_DividendAffectsCashOnly(t *testing.T) {
	ctx := context.Background()
	txns := []transaction.Transaction{
		{
			AccountID: 3, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(100, 0), Price: dec(15000, 2), NetCash: dec(-1500000, 2),
			LotID: ptrStr("LOT-B1"),
		},
		{
			AccountID: 3, Date: mustTime("2025-03-15"), Type: "dividend",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(1, 0), Price: dec(300, 2), NetCash: dec(30000, 2),
		},
	}

	result, err := CalculatePositions(ctx, 3, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AAPL position unchanged (still 100 shares).
	if len(result.OpenPositions) != 1 {
		t.Fatalf("expected 1 open position, got %d", len(result.OpenPositions))
	}
	if !result.OpenPositions[0].Quantity.Equal(dec(100, 0)) {
		t.Errorf("expected quantity 100, got %q", result.OpenPositions[0].Quantity.String())
	}

	// Cash position includes dividend.
	if len(result.CashPositions) != 1 {
		t.Fatalf("expected 1 cash position, got %d", len(result.CashPositions))
	}
	// Cash: -$15,000 (buy) + $300 (dividend) = -$14,700
	if !result.CashPositions[0].Quantity.Equal(dec(-1470000, 2)) {
		t.Errorf("expected cash -14700.00, got %q", result.CashPositions[0].Quantity.String())
	}
}

func TestCalculatePositions_MultipleCurrencies(t *testing.T) {
	ctx := context.Background()
	txns := []transaction.Transaction{
		// USD buy
		{
			AccountID: 5, Date: mustTime("2025-01-15"), Type: "buy",
			Symbol: "AAPL", Currency: "USD",
			Quantity: dec(10, 0), Price: dec(15000, 2), NetCash: dec(-150000, 2),
			LotID: ptrStr("LOT-A1"),
		},
		// GBP buy
		{
			AccountID: 5, Date: mustTime("2025-01-20"), Type: "buy",
			Symbol: "VOD.L", Currency: "GBP",
			Quantity: dec(100, 0), Price: dec(750, 2), NetCash: dec(-75000, 2),
			LotID: ptrStr("LOT-V1"),
		},
	}

	result, err := CalculatePositions(ctx, 5, txns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two open positions (AAPL USD, VOD.L GBP).
	if len(result.OpenPositions) != 2 {
		t.Fatalf("expected 2 open positions, got %d", len(result.OpenPositions))
	}

	// Two cash positions (USD and GBP).
	if len(result.CashPositions) != 2 {
		t.Fatalf("expected 2 cash positions, got %d", len(result.CashPositions))
	}
}
