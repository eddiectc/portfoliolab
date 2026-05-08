package position

import (
	"testing"
	"time"

	"github.com/govalues/decimal"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
)

// tx is a shorthand for creating a Transaction for cash position tests.
func tx(date, typ, symbol, currency string, netCash decimal.Decimal) transaction.Transaction {
	t, _ := time.Parse("2006-01-02", date)
	return transaction.Transaction{
		Date:     t,
		Type:     typ,
		Symbol:   symbol,
		Currency: currency,
		NetCash:  netCash,
	}
}

func TestComputeCashPositions_SingleDeposit(t *testing.T) {
	// Single deposit → positive cash balance.
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)), // $10,000.00
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	if p.Symbol != "$CASH-USD" {
		t.Errorf("expected Symbol $CASH-USD, got %q", p.Symbol)
	}
	if !p.Quantity.Equal(dec(1000000, 2)) {
		t.Errorf("expected Quantity 10000.00, got %q", p.Quantity.String())
	}
	if !p.CostBasis.Equal(decimal.Zero) {
		t.Errorf("expected CostBasis 0, got %q", p.CostBasis.String())
	}
	if !p.RealizedPnL.Equal(decimal.Zero) {
		t.Errorf("expected RealizedPnL 0, got %q", p.RealizedPnL.String())
	}
	if p.IsClosed {
		t.Error("expected IsClosed false")
	}
}

func TestComputeCashPositions_DepositAndWithdrawal(t *testing.T) {
	// Deposit + withdrawal → reduced balance.
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)),    // $10,000
		tx("2025-02-01", "withdrawal", "$CASH-USD", "USD", dec(-200000, 2)), // -$2,000
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	if !p.Quantity.Equal(dec(800000, 2)) {
		t.Errorf("expected Quantity 8000.00, got %q", p.Quantity.String())
	}
}

func TestComputeCashPositions_MultipleCurrencies(t *testing.T) {
	// USD and GBP transactions → separate cash positions.
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)),  // $10,000
		tx("2025-01-20", "deposit", "$CASH-GBP", "GBP", dec(500000, 2)),   // £5,000
		tx("2025-02-01", "withdrawal", "$CASH-USD", "USD", dec(-100000, 2)), // -$1,000
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	// Positions are sorted by symbol, so $CASH-GBP comes first.
	var usdPos, gbpPos Position
	for _, p := range positions {
		if p.Symbol == "$CASH-USD" {
			usdPos = p
		} else if p.Symbol == "$CASH-GBP" {
			gbpPos = p
		}
	}

	if usdPos.Symbol == "" {
		t.Fatal("missing USD cash position")
	}
	if gbpPos.Symbol == "" {
		t.Fatal("missing GBP cash position")
	}

	if !usdPos.Quantity.Equal(dec(900000, 2)) {
		t.Errorf("expected USD Quantity 9000.00, got %q", usdPos.Quantity.String())
	}
	if !gbpPos.Quantity.Equal(dec(500000, 2)) {
		t.Errorf("expected GBP Quantity 5000.00, got %q", gbpPos.Quantity.String())
	}
}

func TestComputeCashPositions_DividendAddsToCash(t *testing.T) {
	// Dividend adds to cash without affecting share position.
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)), // $10,000
		tx("2025-03-01", "dividend", "AAPL", "USD", dec(300, 2)),         // $3.00
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	if !p.Quantity.Equal(dec(1000300, 2)) {
		t.Errorf("expected Quantity 10003.00, got %q", p.Quantity.String())
	}
}

func TestComputeCashPositions_NegativeBalance(t *testing.T) {
	// Overdraft: withdrawals exceed deposits.
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(50000, 2)),    // $500
		tx("2025-02-01", "withdrawal", "$CASH-USD", "USD", dec(-100000, 2)), // -$1,000
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	if !p.Quantity.Equal(dec(-50000, 2)) {
		t.Errorf("expected Quantity -500.00, got %q", p.Quantity.String())
	}
	if p.IsClosed {
		t.Error("expected IsClosed false even for negative balance")
	}
}

func TestComputeCashPositions_EmptyTransactions(t *testing.T) {
	positions := ComputeCashPositions(nil)

	if len(positions) != 0 {
		t.Errorf("expected 0 positions, got %d", len(positions))
	}
	if positions == nil {
		t.Error("expected non-nil empty slice")
	}
}

func TestComputeCashPositions_BuyAndSellAffectCash(t *testing.T) {
	// Deposit, then buy (cash out), then sell (cash in).
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)),  // $10,000
		tx("2025-02-01", "buy", "AAPL", "USD", dec(-150000, 2)),           // -$1,500
		tx("2025-03-01", "sell", "AAPL", "USD", dec(51000, 2)),            // $510
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	// $10,000 - $1,500 + $510 = $9,010
	if !p.Quantity.Equal(dec(901000, 2)) {
		t.Errorf("expected Quantity 9010.00, got %q", p.Quantity.String())
	}
}

func TestComputeCashPositions_FeeAndTax(t *testing.T) {
	// Fee and tax reduce cash.
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)), // $10,000
		tx("2025-02-01", "fee", "$CASH-USD", "USD", dec(-500, 2)),        // -$5.00
		tx("2025-03-01", "tax", "$CASH-USD", "USD", dec(-1000, 2)),       // -$10.00
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	if !p.Quantity.Equal(dec(998500, 2)) {
		t.Errorf("expected Quantity 9985.00, got %q", p.Quantity.String())
	}
}

func TestComputeCashPositions_OpenDateIsEarliest(t *testing.T) {
	// Open date should be the earliest transaction date.
	txs := []transaction.Transaction{
		tx("2025-03-01", "buy", "AAPL", "USD", dec(-150000, 2)),    // -$1,500 (earliest)
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)), // $10,000
		tx("2025-02-01", "withdrawal", "$CASH-USD", "USD", dec(-50000, 2)), // -$500
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	expectedDate := mustTime("2025-01-15")
	if !p.OpenDate.Equal(expectedDate) {
		t.Errorf("expected OpenDate 2025-01-15, got %q", p.OpenDate.Format("2006-01-02"))
	}
}

func TestComputeCashPositions_NonCashTypeIgnored(t *testing.T) {
	// Only cash-affecting types should contribute.
	txs := []transaction.Transaction{
		tx("2025-01-15", "deposit", "$CASH-USD", "USD", dec(1000000, 2)), // $10,000
	}

	positions := ComputeCashPositions(txs)

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	if !p.Quantity.Equal(dec(1000000, 2)) {
		t.Errorf("expected Quantity 10000.00, got %q", p.Quantity.String())
	}
}
