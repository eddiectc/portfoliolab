package allocation

import (
	"context"
	"errors"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"github.com/govalues/decimal"
)

var ctx = context.Background()

// --- Mocks ---

type mockPositionSource struct {
	positions []position.Position
	enriched  []position.PositionWithMarket
	getErr    error
	prices    map[string]*decimal.Decimal // explicit prices for GetMarketPrice
	priceErr  map[string]error            // explicit errors for GetMarketPrice
}

func (m *mockPositionSource) GetOpenPositions(_ context.Context, accountIDs []int64, limit, offset int) ([]position.Position, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	// If positions not explicitly set, derive stub positions from enriched data.
	positions := m.positions
	if len(positions) == 0 && len(m.enriched) > 0 {
		positions = make([]position.Position, len(m.enriched))
		for i, e := range m.enriched {
			positions[i] = e.Position
		}
	}
	// Filter positions by account IDs.
	var result []position.Position
	if len(accountIDs) > 0 {
		accountSet := make(map[int64]struct{})
		for _, id := range accountIDs {
			accountSet[id] = struct{}{}
		}
		for _, p := range positions {
			if _, ok := accountSet[p.AccountID]; ok {
				result = append(result, p)
			}
		}
	} else {
		result = positions
	}
	if result == nil {
		result = []position.Position{}
	}
	// Apply pagination.
	if len(result) <= offset {
		return []position.Position{}, nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockPositionSource) EnrichWithMarketData(_ context.Context, positions []position.Position, _ string) []position.PositionWithMarket {
	if len(positions) == 0 {
		return []position.PositionWithMarket{}
	}
	// If enriched is set, return it (simulating enrichment).
	if m.enriched != nil && len(m.enriched) > 0 {
		return m.enriched
	}
	// Default: return positions with no market data.
	result := make([]position.PositionWithMarket, len(positions))
	for i, p := range positions {
		result[i] = position.PositionWithMarket{
			Position:            p,
			MarketDataAvailable: false,
		}
	}
	return result
}

func (m *mockPositionSource) GetMarketPrice(_ context.Context, symbol string) (*decimal.Decimal, error) {
	// Check explicit error map first.
	if m.priceErr != nil {
		if err, ok := m.priceErr[symbol]; ok {
			return nil, err
		}
	}
	// Check explicit price map first.
	if m.prices != nil {
		if price, ok := m.prices[symbol]; ok {
			return price, nil
		}
	}
	// Derive from enriched data: MarketValue / Quantity.
	if m.enriched != nil {
		for _, e := range m.enriched {
			if e.Symbol == symbol && e.MarketDataAvailable && !e.Quantity.IsZero() {
				mv := e.MarketValue
				price, _ := mv.Quo(e.Quantity)
				return &price, nil
			}
		}
	}
	return nil, nil
}

type mockAccountLister struct {
	accounts []position.AccountRef
	err      error
}

func (m *mockAccountLister) GetAccountsByPortfolio(_ context.Context, portfolioID int64) ([]position.AccountRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []position.AccountRef
	for _, a := range m.accounts {
		if a.PortfolioID == portfolioID {
			result = append(result, a)
		}
	}
	if result == nil {
		result = []position.AccountRef{}
	}
	return result, nil
}

func (m *mockAccountLister) GetAllAccounts(_ context.Context) ([]position.AccountRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.accounts == nil {
		return []position.AccountRef{}, nil
	}
	return m.accounts, nil
}

// noopTargetRepo is a stub TargetRepository for tests that don't exercise target CRUD.
type noopTargetRepo struct{}

func (m *noopTargetRepo) GetByPortfolio(_ context.Context, _ int64) ([]TargetAllocation, error) {
	return []TargetAllocation{}, nil
}
func (m *noopTargetRepo) Upsert(_ context.Context, _ TargetAllocation) error {
	return nil
}
func (m *noopTargetRepo) DeleteBySymbol(_ context.Context, _ int64, _ string) error {
	return nil
}
func (m *noopTargetRepo) DeleteByPortfolio(_ context.Context, _ int64) error {
	return nil
}

// --- Test helpers ---

func mkEnriched(accountID int64, accountName, symbol, currency string, quantity, marketValue, mvBase decimal.Decimal, available bool) position.PositionWithMarket {
	var mvBasePtr *decimal.Decimal
	if !mvBase.IsZero() {
		mvBasePtr = &mvBase
	}
	return position.PositionWithMarket{
		Position: position.Position{
			AccountID:   accountID,
			AccountName: accountName,
			Symbol:      symbol,
			Currency:    currency,
			Quantity:    quantity,
		},
		MarketValue:         marketValue,
		MarketValueBase:     mvBasePtr,
		MarketDataAvailable: available,
	}
}

func mkCashEnriched(accountID int64, accountName, symbol, currency string, quantity, mvBase decimal.Decimal) position.PositionWithMarket {
	return position.PositionWithMarket{
		Position: position.Position{
			AccountID:   accountID,
			AccountName: accountName,
			Symbol:      symbol,
			Currency:    currency,
			Quantity:    quantity,
		},
		MarketValue:         quantity, // Cash: MarketValue = balance
		MarketValueBase:     &mvBase,
		MarketDataAvailable: true,
	}
}

// --- Tests ---

func TestComputeAllocation_SingleSymbol(t *testing.T) {
	// Single symbol, 100% allocation.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	row := result.Rows[0]
	if row.Symbol != "AAPL" {
		t.Errorf("symbol = %q, want %q", row.Symbol, "AAPL")
	}
	if !row.AllocationPct.Equal(decimal.MustParse("100.0")) {
		t.Errorf("allocation_pct = %v, want 100.0", row.AllocationPct)
	}
	if !result.TotalValueBase.Equal(decimal.MustParse("1500.00")) {
		t.Errorf("total_value_base = %v, want 1500.00", result.TotalValueBase)
	}
	if result.BaseCurrency != "USD" {
		t.Errorf("base_currency = %q, want %q", result.BaseCurrency, "USD")
	}
}

func TestComputeAllocation_MultipleSymbols(t *testing.T) {
	// Two symbols: AAPL 60%, MSFT 40%.
	// AAPL: $9000, MSFT: $6000, Total: $15000
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("6000.00"),
			decimal.MustParse("6000.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}

	// Should be sorted by allocation % descending.
	if result.Rows[0].Symbol != "AAPL" {
		t.Errorf("first row symbol = %q, want %q", result.Rows[0].Symbol, "AAPL")
	}
	if !result.Rows[0].AllocationPct.Equal(decimal.MustParse("60.0")) {
		t.Errorf("AAPL allocation_pct = %v, want 60.0", result.Rows[0].AllocationPct)
	}
	if result.Rows[1].Symbol != "MSFT" {
		t.Errorf("second row symbol = %q, want %q", result.Rows[1].Symbol, "MSFT")
	}
	if !result.Rows[1].AllocationPct.Equal(decimal.MustParse("40.0")) {
		t.Errorf("MSFT allocation_pct = %v, want 40.0", result.Rows[1].AllocationPct)
	}
}

func TestComputeAllocation_CashOnly(t *testing.T) {
	// Cash-only portfolio: $5000 USD cash → single Cash row at 100%.
	enriched := []position.PositionWithMarket{
		mkCashEnriched(1, "Broker A", "$CASH-USD", "USD",
			decimal.MustParse("5000.00"),
			decimal.MustParse("5000.00")),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CashRow == nil {
		t.Fatal("expected cash row, got nil")
	}
	if result.CashRow.Symbol != "$CASH" {
		t.Errorf("cash symbol = %q, want %q", result.CashRow.Symbol, "$CASH")
	}
	if !result.CashRow.AllocationPct.Equal(decimal.MustParse("100.0")) {
		t.Errorf("cash allocation_pct = %v, want 100.0", result.CashRow.AllocationPct)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 non-cash rows, got %d", len(result.Rows))
	}
}

func TestComputeAllocation_MultiCurrencyCash(t *testing.T) {
	// USD base, $5000 USD cash + £3000 GBP cash (FX 1.27) = $8810 total cash.
	// Plus AAPL $6190 → Total $15000.
	// Cash: $8810 / $15000 = 58.7%, AAPL: $6190 / $15000 = 41.3%
	enriched := []position.PositionWithMarket{
		mkCashEnriched(1, "Broker A", "$CASH-USD", "USD",
			decimal.MustParse("5000.00"),
			decimal.MustParse("5000.00")),
		mkCashEnriched(2, "Broker B", "$CASH-GBP", "GBP",
			decimal.MustParse("3000.00"),
			decimal.MustParse("3810.00")), // 3000 * 1.27
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("40"),
			decimal.MustParse("6190.00"),
			decimal.MustParse("6190.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Broker B", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CashRow == nil {
		t.Fatal("expected cash row, got nil")
	}

	// Cash total should be $5000 + $3810 = $8810
	expectedCashMV := decimal.MustParse("8810.00")
	if !result.CashRow.MarketValueBase.Equal(expectedCashMV) {
		t.Errorf("cash market_value_base = %v, want %v", result.CashRow.MarketValueBase, expectedCashMV)
	}

	// Cash allocation: 8810 / 15000 * 100 = 58.73... → 58.7
	expectedCashPct := decimal.MustParse("58.7")
	if !result.CashRow.AllocationPct.Equal(expectedCashPct) {
		t.Errorf("cash allocation_pct = %v, want %v", result.CashRow.AllocationPct, expectedCashPct)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 non-cash row, got %d", len(result.Rows))
	}

	// AAPL allocation: 6190 / 15000 * 100 = 41.266... → 41.3
	expectedAAPLPct := decimal.MustParse("41.3")
	if !result.Rows[0].AllocationPct.Equal(expectedAAPLPct) {
		t.Errorf("AAPL allocation_pct = %v, want %v", result.Rows[0].AllocationPct, expectedAAPLPct)
	}
}

func TestComputeAllocation_EmptyPortfolio(t *testing.T) {
	svc := NewService(
		&mockPositionSource{positions: []position.Position{}},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message == "" {
		t.Error("expected empty-state message, got empty string")
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestComputeAllocation_NoAccounts(t *testing.T) {
	svc := NewService(
		&mockPositionSource{},
		&mockAccountLister{accounts: []AccountRef{}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message == "" {
		t.Error("expected message for no accounts, got empty string")
	}
}

func TestComputeAllocation_FilterByPortfolio(t *testing.T) {
	// Two portfolios: portfolio 1 has AAPL, portfolio 2 has MSFT.
	// Filter for portfolio 1 should only show AAPL.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{
			positions: []position.Position{
				{AccountID: 1, Symbol: "AAPL"},
				{AccountID: 2, Symbol: "MSFT"},
			},
			enriched: enriched,
		},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Broker B", PortfolioID: 2, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{PortfolioIDs: []int64{1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0].Symbol != "AAPL" {
		t.Errorf("symbol = %q, want %q", result.Rows[0].Symbol, "AAPL")
	}
}

func TestComputeAllocation_AccountBreakdown(t *testing.T) {
	// Same symbol held in two accounts.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("6"),
			decimal.MustParse("900.00"),
			decimal.MustParse("900.00"),
			true),
		mkEnriched(2, "Broker B", "AAPL", "USD",
			decimal.MustParse("4"),
			decimal.MustParse("600.00"),
			decimal.MustParse("600.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Broker B", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	row := result.Rows[0]
	if len(row.AccountBreakdown) != 2 {
		t.Fatalf("expected 2 account breakdown entries, got %d", len(row.AccountBreakdown))
	}

	// Broker A: 900/1500 = 60% of symbol
	if !row.AccountBreakdown[0].PctOfSymbol.Equal(decimal.MustParse("60.0")) {
		t.Errorf("Broker A pct_of_symbol = %v, want 60.0", row.AccountBreakdown[0].PctOfSymbol)
	}
	// Broker B: 600/1500 = 40% of symbol
	if !row.AccountBreakdown[1].PctOfSymbol.Equal(decimal.MustParse("40.0")) {
		t.Errorf("Broker B pct_of_symbol = %v, want 40.0", row.AccountBreakdown[1].PctOfSymbol)
	}
}

func TestComputeAllocation_MissingMarketData(t *testing.T) {
	// One position with market data, one without.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
		mkEnriched(1, "Broker A", "GOOG", "USD",
			decimal.MustParse("5"),
			decimal.Zero,
			decimal.Zero,
			false), // No market data
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only AAPL should appear in rows (GOOG has no market data).
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	// Should have a warning about missing market data.
	if len(result.Warnings) == 0 {
		t.Error("expected warning about missing market data")
	}
}

func TestComputeAllocation_ZeroTotalValue(t *testing.T) {
	// All positions have zero market value.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.Zero,
			decimal.Zero,
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	_, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err == nil {
		t.Fatal("expected error for zero total value")
	}
	if err != ErrZeroTotalValue {
		t.Errorf("expected ErrZeroTotalValue, got %v", err)
	}
}

func TestComputeAllocation_PositionFetchError(t *testing.T) {
	svc := NewService(
		&mockPositionSource{getErr: context.DeadlineExceeded},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	_, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err == nil {
		t.Fatal("expected error for position fetch failure")
	}
}

func TestComputeAllocation_Sorting(t *testing.T) {
	// Three symbols with different allocations.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("5000.00"),
			decimal.MustParse("5000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("3000.00"),
			decimal.MustParse("3000.00"),
			true),
		mkEnriched(1, "Broker A", "GOOG", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("2000.00"),
			decimal.MustParse("2000.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be sorted: AAPL (50%), MSFT (30%), GOOG (20%)
	if result.Rows[0].Symbol != "AAPL" || result.Rows[1].Symbol != "MSFT" || result.Rows[2].Symbol != "GOOG" {
		t.Errorf("expected [AAPL, MSFT, GOOG], got [%s, %s, %s]",
			result.Rows[0].Symbol, result.Rows[1].Symbol, result.Rows[2].Symbol)
	}
}

func TestComputeAllocation_CashMultipleCurrencies(t *testing.T) {
	// Three cash currencies: USD, GBP, EUR.
	// USD base: $5000 USD + £3000 GBP (1.27) + €2000 EUR (1.08)
	// = $5000 + $3810 + $2160 = $10970
	enriched := []position.PositionWithMarket{
		mkCashEnriched(1, "Broker A", "$CASH-USD", "USD",
			decimal.MustParse("5000.00"),
			decimal.MustParse("5000.00")),
		mkCashEnriched(2, "Broker B", "$CASH-GBP", "GBP",
			decimal.MustParse("3000.00"),
			decimal.MustParse("3810.00")),
		mkCashEnriched(3, "Broker C", "$CASH-EUR", "EUR",
			decimal.MustParse("2000.00"),
			decimal.MustParse("2160.00")),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Broker B", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 3, Name: "Broker C", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CashRow == nil {
		t.Fatal("expected cash row, got nil")
	}

	expectedTotal := decimal.MustParse("10970.00")
	if !result.CashRow.MarketValueBase.Equal(expectedTotal) {
		t.Errorf("cash total = %v, want %v", result.CashRow.MarketValueBase, expectedTotal)
	}

	// Cash should be 100% since it's the only holding.
	if !result.CashRow.AllocationPct.Equal(decimal.MustParse("100.0")) {
		t.Errorf("cash allocation_pct = %v, want 100.0", result.CashRow.AllocationPct)
	}

	// Should have 3 account breakdown entries.
	if len(result.CashRow.AccountBreakdown) != 3 {
		t.Errorf("expected 3 cash account breakdown entries, got %d", len(result.CashRow.AccountBreakdown))
	}
}

func TestComputeAllocation_MixedCashAndSymbols(t *testing.T) {
	// AAPL $9000 + Cash $6000 = $15000 total.
	// AAPL: 60%, Cash: 40%
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkCashEnriched(1, "Broker A", "$CASH-USD", "USD",
			decimal.MustParse("6000.00"),
			decimal.MustParse("6000.00")),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 1 || result.Rows[0].Symbol != "AAPL" {
		t.Errorf("expected 1 AAPL row, got %d rows", len(result.Rows))
	}
	if !result.Rows[0].AllocationPct.Equal(decimal.MustParse("60.0")) {
		t.Errorf("AAPL allocation_pct = %v, want 60.0", result.Rows[0].AllocationPct)
	}

	if result.CashRow == nil {
		t.Fatal("expected cash row, got nil")
	}
	if !result.CashRow.AllocationPct.Equal(decimal.MustParse("40.0")) {
		t.Errorf("cash allocation_pct = %v, want 40.0", result.CashRow.AllocationPct)
	}
}

func TestComputeAllocation_MarketDataAvailableFlag(t *testing.T) {
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.MarketDataAvailable {
		t.Error("expected MarketDataAvailable = true")
	}
	if !result.Rows[0].HasMarketData {
		t.Error("expected row HasMarketData = true")
	}
}

func TestComputeAllocation_LastUpdated(t *testing.T) {
	before := ctx // Just ensure the call works
	svc := NewService(
		&mockPositionSource{enriched: []position.PositionWithMarket{
			mkEnriched(1, "Broker A", "AAPL", "USD",
				decimal.MustParse("10"),
				decimal.MustParse("1500.00"),
				decimal.MustParse("1500.00"),
				true),
		}},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(before, AllocationFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.LastUpdated.IsZero() {
		t.Error("expected LastUpdated to be set")
	}
}

func TestComputeAllocation_MultiplePortfolios(t *testing.T) {
	// Two portfolios, same currency: portfolio 1 has AAPL, portfolio 2 has MSFT.
	// Filter for both should show both symbols aggregated.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("3000.00"),
			decimal.MustParse("3000.00"),
			true),
		mkEnriched(2, "Broker B", "MSFT", "USD",
			decimal.MustParse("20"),
			decimal.MustParse("7000.00"),
			decimal.MustParse("7000.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Broker B", PortfolioID: 2, PortfolioCurrency: "USD"},
		}},
		&noopTargetRepo{},
	)

	result, err := svc.ComputeAllocation(ctx, AllocationFilter{PortfolioIDs: []int64{1, 2}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	// Total = $10000, AAPL = 30%, MSFT = 70%
	if !result.Rows[0].AllocationPct.Equal(decimal.MustParse("70.0")) {
		t.Errorf("MSFT allocation_pct = %v, want 70.0", result.Rows[0].AllocationPct)
	}
	if !result.Rows[1].AllocationPct.Equal(decimal.MustParse("30.0")) {
		t.Errorf("AAPL allocation_pct = %v, want 30.0", result.Rows[1].AllocationPct)
	}
	if !result.TotalValueBase.Equal(decimal.MustParse("10000.00")) {
		t.Errorf("total_value_base = %v, want 10000.00", result.TotalValueBase)
	}
}

func TestComputeAllocation_MixedCurrencies(t *testing.T) {
	// Two portfolios with different base currencies should return ErrMixedCurrencies.
	svc := NewService(
		&mockPositionSource{},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Broker B", PortfolioID: 2, PortfolioCurrency: "GBP"},
		}},
		&noopTargetRepo{},
	)

	_, err := svc.ComputeAllocation(ctx, AllocationFilter{PortfolioIDs: []int64{1, 2}})
	if err == nil {
		t.Fatal("expected error for mixed currencies")
	}
	if !errors.Is(err, ErrMixedCurrencies) {
		t.Errorf("expected ErrMixedCurrencies, got %v", err)
	}
}

func TestComputeAllocation_MixedCurrenciesAllAccounts(t *testing.T) {
	// No filter (all accounts) with mixed currencies should also error.
	svc := NewService(
		&mockPositionSource{},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
			{ID: 2, Name: "Broker B", PortfolioID: 2, PortfolioCurrency: "EUR"},
		}},
		&noopTargetRepo{},
	)

	_, err := svc.ComputeAllocation(ctx, AllocationFilter{})
	if err == nil {
		t.Fatal("expected error for mixed currencies")
	}
	if !errors.Is(err, ErrMixedCurrencies) {
		t.Errorf("expected ErrMixedCurrencies, got %v", err)
	}
}
