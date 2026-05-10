package position

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- Mocks for equity curve tests ---

type mockHistoricalFetcher struct {
	prices   map[string][]market.HistoricalPrice
	failed   []string
	upserted map[string][]market.HistoricalPrice
}

func (m *mockHistoricalFetcher) FetchQuote(_ context.Context, _ string) (*market.MarketData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHistoricalFetcher) FetchFxRate(_ context.Context, _, _ string) (*market.MarketData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHistoricalFetcher) FetchQuotesBatch(_ context.Context, _ []string) map[string]*market.MarketData {
	return nil
}

func (m *mockHistoricalFetcher) FetchHistoricalPricesBatch(_ context.Context, symbols []string, _, _ time.Time) (map[string][]market.HistoricalPrice, []string) {
	result := make(map[string][]market.HistoricalPrice)
	for _, sym := range symbols {
		if prices, ok := m.prices[sym]; ok {
			result[sym] = prices
		}
	}
	return result, m.failed
}

type mockHistoricalRepo struct {
	upserted      map[string][]market.HistoricalPrice
	cachedPrices  map[string][]market.HistoricalPrice
	latestDates   map[string]*time.Time
}

func newMockHistoricalRepo() *mockHistoricalRepo {
	return &mockHistoricalRepo{
		upserted:     make(map[string][]market.HistoricalPrice),
		cachedPrices: make(map[string][]market.HistoricalPrice),
		latestDates:  make(map[string]*time.Time),
	}
}

func (m *mockHistoricalRepo) SetCachedPrices(symbol string, prices []market.HistoricalPrice) {
	m.cachedPrices[symbol] = prices
	if len(prices) > 0 {
		latest := prices[len(prices)-1].Date
		m.latestDates[symbol] = &latest
	}
}

func (m *mockHistoricalRepo) SetLatestDate(symbol string, date time.Time) {
	m.latestDates[symbol] = &date
}

func (m *mockHistoricalRepo) GetLatest(_ context.Context, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockHistoricalRepo) GetBySourceAndDate(_ context.Context, _, _, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockHistoricalRepo) Upsert(_ context.Context, _ *market.MarketData) error {
	return nil
}

func (m *mockHistoricalRepo) GetCurrentFxRate(_ context.Context, _, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (m *mockHistoricalRepo) UpsertHistoricalPrices(_ context.Context, symbol string, prices []market.HistoricalPrice, _dataType string) error {
	m.upserted[symbol] = prices
	return nil
}

func (m *mockHistoricalRepo) GetHistoricalPricesBySymbol(_ context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error) {
	prices, ok := m.cachedPrices[symbol]
	if !ok {
		return nil, nil
	}
	// Filter by date range.
	var filtered []market.HistoricalPrice
	for _, p := range prices {
		if !start.IsZero() && p.Date.Before(start) {
			continue
		}
		if !end.IsZero() && p.Date.After(end) {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered, nil
}

func (m *mockHistoricalRepo) GetLatestQuotesBatch(_ context.Context, _ []string) map[string]*market.MarketData {
	return nil
}

func (m *mockHistoricalRepo) GetLatestPriceDatePerSymbol(_ context.Context, symbols []string) map[string]*time.Time {
	result := make(map[string]*time.Time)
	for _, sym := range symbols {
		if date, ok := m.latestDates[sym]; ok {
			result[sym] = date
		}
	}
	return result
}

// mockEqMarketService wraps mockHistoricalRepo to implement MarketDataService.
type mockEqMarketService struct {
	repo *mockHistoricalRepo
}

func (m *mockEqMarketService) GetQuotes(_ context.Context, _ []string) map[string]*market.MarketData {
	return nil
}

func (m *mockEqMarketService) GetHistoricalPrices(_ context.Context, symbol string, _, _ time.Time) ([]market.HistoricalPrice, error) {
	return m.repo.GetHistoricalPricesBySymbol(context.Background(), symbol, time.Time{}, time.Time{})
}

func (m *mockEqMarketService) GetLatestPriceDatePerSymbol(_ context.Context, symbols []string) map[string]*time.Time {
	return m.repo.GetLatestPriceDatePerSymbol(context.Background(), symbols)
}

func (m *mockEqMarketService) RefreshQuotes(_ context.Context, _ []string) marketservice.RefreshResult {
	return marketservice.RefreshResult{}
}

func (m *mockEqMarketService) GetCurrentFxRate(_ context.Context, _, _ string) (*market.FxRate, error) {
	return nil, nil
}

func (m *mockEqMarketService) GetHistoricalFxRate(_ context.Context, _, _ string, _ time.Time) (*market.FxRate, error) {
	return nil, nil
}

func (m *mockEqMarketService) RefreshFxRates(_ context.Context, _ []marketservice.FxPair) marketservice.FxRefreshResult {
	return marketservice.FxRefreshResult{}
}

// --- Test helpers ---

func testTime(year, month int, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func histPrice(date time.Time, closeVal int64, currency string) market.HistoricalPrice {
	return market.HistoricalPrice{
		Date:     date,
		Close:    decimal.MustNew(closeVal, 2),
		Currency: currency,
	}
}

func eqTxn(accountID int64, date time.Time, typ, symbol, currency string, qty, price, netCash int64) transaction.Transaction {
	return transaction.Transaction{
		AccountID: accountID,
		Date:      date,
		Type:      typ,
		Symbol:    symbol,
		Quantity:  decimal.MustNew(qty, 2),
		Price:     decimal.MustNew(price, 2),
		Currency:  currency,
		NetCash:   decimal.MustNew(netCash, 2),
		CreatedAt: date,
		UpdatedAt: date,
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}

// --- Tests for helper functions ---

func TestDetermineDateRange_AllPeriod(t *testing.T) {
	filters := PerformanceFilters{Period: "All"}
	dateFrom, dateTo := determineDateRange(filters)

	if !dateFrom.IsZero() {
		t.Errorf("expected zero dateFrom for All period, got %v", dateFrom)
	}
	if dateTo.IsZero() {
		t.Error("expected non-zero dateTo")
	}
}

func TestDetermineDateRange_Periods(t *testing.T) {
	now := time.Now().UTC()
	toDay := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	tests := []struct {
		name     string
		period   string
		wantFrom time.Time
	}{
		{"1W", "1W", toDay(now.AddDate(0, 0, -7))},
		{"1M", "1M", toDay(now.AddDate(0, -1, 0))},
		{"3M", "3M", toDay(now.AddDate(0, -3, 0))},
		{"1Y", "1Y", toDay(now.AddDate(-1, 0, 0))},
		{"3Y", "3Y", toDay(now.AddDate(-3, 0, 0))},
		{"5Y", "5Y", toDay(now.AddDate(-5, 0, 0))},
		{"YTD", "YTD", time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)},
		{"empty", "", time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := PerformanceFilters{Period: tt.period}
			dateFrom, _ := determineDateRange(filters)
			gotDay := toDay(dateFrom)
			if !gotDay.Equal(tt.wantFrom) {
				t.Errorf("dateFrom: got %v, want %v", gotDay, tt.wantFrom)
			}
		})
	}
}

func TestDetermineDateRange_ExplicitDates(t *testing.T) {
	from := testTime(2024, 3, 1)
	to := testTime(2024, 6, 30)
	filters := PerformanceFilters{
		DateFrom: &from,
		DateTo:   &to,
	}
	dateFrom, dateTo := determineDateRange(filters)
	if !dateFrom.Equal(from) {
		t.Errorf("dateFrom: got %v, want %v", dateFrom, from)
	}
	if !dateTo.Equal(to) {
		t.Errorf("dateTo: got %v, want %v", dateTo, to)
	}
}

func TestFilterByDateRange(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 3, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 6, 15), "buy", "MSFT", "USD", 500, 40000, -2000000),
	}

	from := testTime(2024, 2, 1)
	to := testTime(2024, 5, 31)

	result := filterByDateRange(txns, from, to)
	if len(result) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(result))
	}
	if result[0].Symbol != "AAPL" {
		t.Errorf("expected AAPL, got %s", result[0].Symbol)
	}
}

func TestFilterByDateRange_ZeroFrom(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 6, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
	}

	// Zero dateFrom = no lower bound.
	result := filterByDateRange(txns, time.Time{}, testTime(2024, 12, 31))
	if len(result) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(result))
	}
}

func TestWalkTransactions_HappyPath(t *testing.T) {
	txns := []transaction.Transaction{
		// Jan 15: deposit $10,000
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		// Feb 15: buy 10 AAPL at $150
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		// Mar 15: deposit $5,000
		eqTxn(1, testTime(2024, 3, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
		// Apr 15: sell 5 AAPL at $170 (negative quantity, matching real DB convention)
		eqTxn(1, testTime(2024, 4, 15), "sell", "AAPL", "USD", -500, 17000, 850000),
	}

	snapshots := walkTransactions(txns)
	if len(snapshots) != 4 {
		t.Fatalf("expected 4 snapshots, got %d", len(snapshots))
	}

	// Snapshot 1 (Jan 15): no positions, cash = $10,000, net deposit = $10,000
	s1 := snapshots[0]
	if len(s1.positions) != 0 {
		t.Errorf("snapshot 1: expected 0 positions, got %d", len(s1.positions))
	}
	wantCash := decimal.MustNew(1000000, 2)
	if !s1.cashBalance["USD"].Equal(wantCash) {
		t.Errorf("snapshot 1 cash: got %s, want %s", s1.cashBalance["USD"].String(), wantCash.String())
	}
	if !s1.netDeposit["USD"].Equal(wantCash) {
		t.Errorf("snapshot 1 net deposit: got %s, want %s", s1.netDeposit["USD"].String(), wantCash.String())
	}

	// Snapshot 2 (Feb 15): 10 AAPL, cash = -$5,000, net deposit = $10,000
	s2 := snapshots[1]
	wantQty := decimal.MustNew(1000, 2)
	if !s2.positions["AAPL"].Equal(wantQty) {
		t.Errorf("snapshot 2 AAPL qty: got %s, want %s", s2.positions["AAPL"].String(), wantQty.String())
	}
	wantCash2 := decimal.MustNew(-500000, 2)
	if !s2.cashBalance["USD"].Equal(wantCash2) {
		t.Errorf("snapshot 2 cash: got %s, want %s", s2.cashBalance["USD"].String(), wantCash2.String())
	}
	if !s2.netDeposit["USD"].Equal(wantCash) {
		t.Errorf("snapshot 2 net deposit: got %s, want %s", s2.netDeposit["USD"].String(), wantCash.String())
	}

	// Snapshot 3 (Mar 15): 10 AAPL, cash = $0, net deposit = $15,000
	s3 := snapshots[2]
	if !s3.cashBalance["USD"].IsZero() {
		t.Errorf("snapshot 3 cash: expected 0, got %s", s3.cashBalance["USD"].String())
	}
	wantND3 := decimal.MustNew(1500000, 2)
	if !s3.netDeposit["USD"].Equal(wantND3) {
		t.Errorf("snapshot 3 net deposit: got %s, want %s", s3.netDeposit["USD"].String(), wantND3.String())
	}

	// Snapshot 4 (Apr 15): 5 AAPL, cash = $8,500, net deposit = $15,000
	s4 := snapshots[3]
	wantQty4 := decimal.MustNew(500, 2)
	if !s4.positions["AAPL"].Equal(wantQty4) {
		t.Errorf("snapshot 4 AAPL qty: got %s, want %s", s4.positions["AAPL"].String(), wantQty4.String())
	}
	wantCash4 := decimal.MustNew(850000, 2)
	if !s4.cashBalance["USD"].Equal(wantCash4) {
		t.Errorf("snapshot 4 cash: got %s, want %s", s4.cashBalance["USD"].String(), wantCash4.String())
	}
}

func TestWalkTransactions_NegativeSellQuantity(t *testing.T) {
	// Real transaction data stores sell quantities as negative.
	// walkTransactions must handle this correctly — not double-negate.
	txns := []transaction.Transaction{
		// Jan 15: buy 10 AAPL at $150
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		// Feb 15: sell 5 AAPL at $170 (quantity is negative, matching real DB convention)
		eqTxn(1, testTime(2024, 2, 15), "sell", "AAPL", "USD", -500, 17000, 850000),
	}

	snapshots := walkTransactions(txns)
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	// Snapshot 1: 10 AAPL
	wantQty1 := decimal.MustNew(1000, 2)
	if !snapshots[0].positions["AAPL"].Equal(wantQty1) {
		t.Errorf("snapshot 1 AAPL qty: got %s, want %s", snapshots[0].positions["AAPL"].String(), wantQty1.String())
	}

	// Snapshot 2: 10 + (-5) = 5 AAPL remaining
	wantQty2 := decimal.MustNew(500, 2)
	if !snapshots[1].positions["AAPL"].Equal(wantQty2) {
		t.Errorf("snapshot 2 AAPL qty: got %s, want %s", snapshots[1].positions["AAPL"].String(), wantQty2.String())
	}
}

func TestWalkTransactions_MultipleTxnsSameDate(t *testing.T) {
	txns := []transaction.Transaction{
		// Jan 15: deposit $10,000
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		// Jan 15: buy 10 AAPL at $150 (same day)
		eqTxn(1, testTime(2024, 1, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
	}

	snapshots := walkTransactions(txns)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot (both txns same date), got %d", len(snapshots))
	}

	s := snapshots[0]
	// After both txns: positions = 10 AAPL, cash = -$5,000, net deposit = $10,000
	wantQty := decimal.MustNew(1000, 2)
	if !s.positions["AAPL"].Equal(wantQty) {
		t.Errorf("AAPL qty: got %s, want %s", s.positions["AAPL"].String(), wantQty.String())
	}
	wantCash := decimal.MustNew(-500000, 2)
	if !s.cashBalance["USD"].Equal(wantCash) {
		t.Errorf("cash: got %s, want %s", s.cashBalance["USD"].String(), wantCash.String())
	}
}

func TestWalkTransactions_NegativeNetDeposit(t *testing.T) {
	txns := []transaction.Transaction{
		// Jan 15: deposit $5,000
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
		// Feb 15: withdrawal $8,000
		eqTxn(1, testTime(2024, 2, 15), "withdrawal", "$CASH-USD", "USD", 0, 0, -800000),
	}

	snapshots := walkTransactions(txns)
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	// After withdrawal: net deposit = $5,000 - $8,000 = -$3,000
	wantND := decimal.MustNew(-300000, 2)
	if !snapshots[1].netDeposit["USD"].Equal(wantND) {
		t.Errorf("net deposit: got %s, want %s", snapshots[1].netDeposit["USD"].String(), wantND.String())
	}
}

func TestCollectUniqueSymbols(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 3, 15), "buy", "MSFT", "USD", 500, 40000, -2000000),
		eqTxn(1, testTime(2024, 4, 15), "sell", "AAPL", "USD", -500, 17000, 850000),
	}

	symbols := collectUniqueSymbols(txns)
	sort.Strings(symbols)

	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
	if symbols[0] != "AAPL" || symbols[1] != "MSFT" {
		t.Errorf("expected [AAPL, MSFT], got %v", symbols)
	}
}

func TestCollectUniqueSymbols_ExcludesCash(t *testing.T) {
	txns := []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
	}

	symbols := collectUniqueSymbols(txns)
	if len(symbols) != 1 || symbols[0] != "AAPL" {
		t.Errorf("expected [AAPL], got %v", symbols)
	}
}

func TestInterpolateDaily_NoPoints(t *testing.T) {
	result := interpolateDaily([]EquityCurvePoint{})
	if len(result) != 0 {
		t.Errorf("expected 0 points, got %d", len(result))
	}
}

func TestInterpolateDaily_SinglePoint(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: testTime(2024, 1, 15), PortfolioValue: decimal.MustNew(1000000, 2), NetDeposit: decimal.MustNew(1000000, 2)},
	}
	result := interpolateDaily(points)
	if len(result) != 1 {
		t.Errorf("expected 1 point, got %d", len(result))
	}
}

func TestInterpolateDaily_FillsGaps(t *testing.T) {
	points := []EquityCurvePoint{
		{Date: testTime(2024, 1, 1), PortfolioValue: decimal.MustNew(1000000, 2), NetDeposit: decimal.MustNew(1000000, 2)},
		{Date: testTime(2024, 1, 4), PortfolioValue: decimal.MustNew(1100000, 2), NetDeposit: decimal.MustNew(1000000, 2)},
	}
	result := interpolateDaily(points)
	// Jan 1 (original), Jan 2 (carried), Jan 3 (carried), Jan 4 (original) = 4 points
	if len(result) != 4 {
		t.Fatalf("expected 4 points, got %d", len(result))
	}

	// Jan 2 should carry forward Jan 1 values.
	if !result[1].Date.Equal(testTime(2024, 1, 2)) {
		t.Errorf("expected Jan 2, got %v", result[1].Date)
	}
	if !result[1].PortfolioValue.Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("Jan 2 portfolio value: got %s, want 1000000", result[1].PortfolioValue.String())
	}

	// Jan 4 should be the original value.
	if !result[3].PortfolioValue.Equal(decimal.MustNew(1100000, 2)) {
		t.Errorf("Jan 4 portfolio value: got %s, want 1100000", result[3].PortfolioValue.String())
	}
}

func TestConvertToBase_SameCurrency(t *testing.T) {
	value := decimal.MustNew(1000000, 2)
	result, found := convertToBase(context.Background(), nil, "USD", "USD", value, testTime(2024, 1, 1))
	if !found {
		t.Error("expected found=true for same currency")
	}
	if !result.Equal(value) {
		t.Errorf("expected unchanged value, got %s", result.String())
	}
}

func TestConvertToBase_WithRate(t *testing.T) {
	rate := decimal.MustNew(12700, 2)
	fx := &mockMarketDataService{
		historicalFx: map[string]*market.FxRate{
			"GBP/USD": {BaseCurrency: "GBP", QuoteCurrency: "USD", Rate: rate},
		},
	}

	value := decimal.MustNew(1000000, 2) // £10,000
	result, found := convertToBase(context.Background(), fx, "GBP", "USD", value, testTime(2024, 1, 1))
	if !found {
		t.Error("expected found=true")
	}
	want := decimal.MustNew(12700000000, 4) // $127,000
	if !result.Equal(want) {
		t.Errorf("expected %s, got %s", want.String(), result.String())
	}
}

func TestConvertToBase_NoRate(t *testing.T) {
	fx := &mockMarketDataService{}
	value := decimal.MustNew(1000000, 2)
	result, found := convertToBase(context.Background(), fx, "GBP", "USD", value, testTime(2024, 1, 1))
	if found {
		t.Error("expected found=false when no rate available")
	}
	// Returns original value unchanged when no rate.
	if !result.Equal(value) {
		t.Errorf("expected original value, got %s", result.String())
	}
}

func TestConvertToBase_NilProvider(t *testing.T) {
	value := decimal.MustNew(1000000, 2)
	_, found := convertToBase(context.Background(), nil, "GBP", "USD", value, testTime(2024, 1, 1))
	if found {
		t.Error("expected found=false with nil provider")
	}
}

func TestDetermineBaseCurrency_SingleCurrency(t *testing.T) {
	accounts := []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
		{ID: 2, PortfolioCurrency: "USD"},
	}
	currency, err := determineBaseCurrency(accounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if currency != "USD" {
		t.Errorf("expected USD, got %s", currency)
	}
}

func TestDetermineBaseCurrency_MismatchedCurrencies(t *testing.T) {
	accounts := []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
		{ID: 2, PortfolioCurrency: "GBP"},
	}
	_, err := determineBaseCurrency(accounts)
	if err == nil {
		t.Fatal("expected error for mismatched currencies")
	}
	posErr, ok := err.(*PositionError)
	if !ok {
		t.Fatalf("expected *PositionError, got %T", err)
	}
	if posErr.Code != "mismatched_currencies" {
		t.Errorf("expected code mismatched_currencies, got %s", posErr.Code)
	}
}

func TestDetermineBaseCurrency_Empty(t *testing.T) {
	currency, err := determineBaseCurrency([]AccountRef{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if currency != "" {
		t.Errorf("expected empty currency, got %s", currency)
	}
}

// --- Integration-style tests for ComputeEquityCurve ---

func newTestServiceForEquity() (*Service, *mockTransactionRepository, *mockAccountLister) {
	txnRepo := newMockTransactionRepository()
	accountLister := newMockAccountLister()
	return NewService(
		newMockPositionRepository(),
		txnRepo,
		newMockAccountChecker(1, 2),
		newMockPortfolioChecker(1),
		accountLister,
		nil, // no portfolio currency checker
	), txnRepo, accountLister
}

func TestComputeEquityCurve_EmptyState(t *testing.T) {
	svc, _, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// No transactions.
	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.EquityCurve) != 0 {
		t.Errorf("expected empty equity curve, got %d points", len(result.EquityCurve))
	}
	if !result.ReturnMetrics.HasInsufficientData {
		t.Error("expected HasInsufficientData=true")
	}
	if result.BaseCurrency != "USD" {
		t.Errorf("expected base currency USD, got %s", result.BaseCurrency)
	}
}

func TestComputeEquityCurve_OnlyDeposits(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// Only deposits, no buys/sells.
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 3, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
	})

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have interpolated points from Jan 15 to Mar 15.
	if len(result.EquityCurve) < 2 {
		t.Fatalf("expected at least 2 points, got %d", len(result.EquityCurve))
	}

	// First point: portfolio value = net deposit = $10,000.
	first := result.EquityCurve[0]
	if !first.PortfolioValue.Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("first portfolio value: got %s, want 1000000", first.PortfolioValue.String())
	}
	if !first.NetDeposit.Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("first net deposit: got %s, want 1000000", first.NetDeposit.String())
	}

	// Last point: portfolio value = net deposit = $15,000.
	last := result.EquityCurve[len(result.EquityCurve)-1]
	if !last.PortfolioValue.Equal(decimal.MustNew(1500000, 2)) {
		t.Errorf("last portfolio value: got %s, want 1500000", last.PortfolioValue.String())
	}
	if !last.NetDeposit.Equal(decimal.MustNew(1500000, 2)) {
		t.Errorf("last net deposit: got %s, want 1500000", last.NetDeposit.String())
	}
}

func TestComputeEquityCurve_HappyPath(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// Deposit, buy AAPL, sell some AAPL.
	// net_cash: deposit $10,000, buy 10×$150 = -$1,500, sell 5×$170 = +$850
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -150000),
		eqTxn(1, testTime(2024, 3, 15), "sell", "AAPL", "USD", -500, 17000, 85000),
	})

	// Set up cached historical prices.
	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 1, 15), 15000, "USD"),
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
		histPrice(testTime(2024, 3, 15), 17000, "USD"),
	})
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.EquityCurve) < 3 {
		t.Fatalf("expected at least 3 points, got %d", len(result.EquityCurve))
	}

	// Find the Jan 15 point (first snapshot).
	var jan15, feb15, mar15 *EquityCurvePoint
	for i := range result.EquityCurve {
		switch {
		case result.EquityCurve[i].Date.Equal(testTime(2024, 1, 15)):
			jan15 = &result.EquityCurve[i]
		case result.EquityCurve[i].Date.Equal(testTime(2024, 2, 15)):
			feb15 = &result.EquityCurve[i]
		case result.EquityCurve[i].Date.Equal(testTime(2024, 3, 15)):
			mar15 = &result.EquityCurve[i]
		}
	}

	// Jan 15: only cash deposit = $10,000. No positions yet.
	if jan15 == nil {
		t.Fatal("expected Jan 15 point")
	}
	if !jan15.PortfolioValue.Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("Jan 15 portfolio value: got %s, want 1000000", jan15.PortfolioValue.String())
	}
	if !jan15.NetDeposit.Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("Jan 15 net deposit: got %s, want 1000000", jan15.NetDeposit.String())
	}

	// Feb 15: 10 AAPL at $150 = $1,500 + cash $8,500 = $10,000. Net deposit = $10,000.
	if feb15 == nil {
		t.Fatal("expected Feb 15 point")
	}
	wantFebPV := decimal.MustNew(1000000, 2)
	if !feb15.PortfolioValue.Equal(wantFebPV) {
		t.Errorf("Feb 15 portfolio value: got %s, want %s", feb15.PortfolioValue.String(), wantFebPV.String())
	}

	// Mar 15: 5 AAPL at $170 = $850 + cash $9,350 = $10,200. Net deposit = $10,000.
	if mar15 == nil {
		t.Fatal("expected Mar 15 point")
	}
	wantMarPV := decimal.MustNew(1020000, 2)
	if !mar15.PortfolioValue.Equal(wantMarPV) {
		t.Errorf("Mar 15 portfolio value: got %s, want %s", mar15.PortfolioValue.String(), wantMarPV.String())
	}
}

func TestComputeEquityCurve_MissingMarketData(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
	})

	// No cached data for AAPL.
	repo := newMockHistoricalRepo()
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have a warning about missing market data.
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if result.Warnings[0] != "missing market data for AAPL" {
		t.Errorf("unexpected warning: %s", result.Warnings[0])
	}

	// Feb 15 point: only cash (-$500), AAPL position excluded (no price).
	var feb15 *EquityCurvePoint
	for i := range result.EquityCurve {
		if result.EquityCurve[i].Date.Equal(testTime(2024, 2, 15)) {
			feb15 = &result.EquityCurve[i]
			break
		}
	}
	if feb15 == nil {
		t.Fatal("expected Feb 15 point")
	}
	wantPV := decimal.MustNew(-500000, 2) // only cash, no AAPL value
	if !feb15.PortfolioValue.Equal(wantPV) {
		t.Errorf("Feb 15 portfolio value: got %s, want %s", feb15.PortfolioValue.String(), wantPV.String())
	}
}

func TestComputeEquityCurve_MismatchedCurrencies(t *testing.T) {
	svc, _, accountLister := newTestServiceForEquity()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, PortfolioID: 1, PortfolioCurrency: "USD"},
		{ID: 2, PortfolioID: 2, PortfolioCurrency: "GBP"},
	})

	_, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		Period: "All", // no portfolio filter → all portfolios
	})
	if err == nil {
		t.Fatal("expected error for mismatched currencies")
	}
	posErr, ok := err.(*PositionError)
	if !ok {
		t.Fatalf("expected *PositionError, got %T", err)
	}
	if posErr.Code != "mismatched_currencies" {
		t.Errorf("expected code mismatched_currencies, got %s", posErr.Code)
	}
}

func TestComputeEquityCurve_MultiCurrencyWithFX(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
		{ID: 2, PortfolioCurrency: "USD"},
	})

	// Account 1 (USD): deposit $10,000
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
	})
	// Account 2 (GBP): deposit £5,000 (in GBP account)
	txnRepo.SetTransactions(2, []transaction.Transaction{
		eqTxn(2, testTime(2024, 1, 15), "deposit", "$CASH-GBP", "GBP", 0, 0, 500000),
	})

	// FX provider: GBP → USD at 1.27
	svc.WithMarketDataService(&mockMarketDataService{
		historicalFx: map[string]*market.FxRate{
			"GBP/USD": {BaseCurrency: "GBP", QuoteCurrency: "USD", Rate: decimal.MustNew(127, 2)},
		},
	}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.EquityCurve) < 1 {
		t.Fatalf("expected at least 1 point, got %d", len(result.EquityCurve))
	}

	// First point: $10,000 (USD cash) + £5,000 × 1.27 = $6,350 (GBP cash → USD) = $16,350
	first := result.EquityCurve[0]
	// 5000.00 × 1.27 = 6350.0000 (scale 4), + 10000.00 = 16350.0000 (scale 4)
	wantPV := decimal.MustNew(163500000, 4) // $16,350.0000 at scale 4
	if !first.PortfolioValue.Equal(wantPV) {
		t.Errorf("portfolio value: got %s, want %s", first.PortfolioValue.String(), wantPV.String())
	}

	// Net deposit: $10,000 + £5,000 × 1.27 = $16,350
	wantND := decimal.MustNew(163500000, 4)
	if !first.NetDeposit.Equal(wantND) {
		t.Errorf("net deposit: got %s, want %s", first.NetDeposit.String(), wantND.String())
	}
}

func TestComputeEquityCurve_PeriodFiltering(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// Transactions across multiple months.
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 6, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
	})

	// Filter with explicit dates: only include June deposit.
	from := testTime(2024, 5, 1)
	to := testTime(2024, 12, 31)
	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		DateFrom:    &from,
		DateTo:      &to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only have the June deposit (Jan deposit is before dateFrom).
	if len(result.EquityCurve) < 1 {
		t.Fatalf("expected at least 1 point, got %d", len(result.EquityCurve))
	}

	// The first point should be June 15 with net deposit = $5,000.
	first := result.EquityCurve[0]
	if !first.NetDeposit.Equal(decimal.MustNew(500000, 2)) {
		t.Errorf("first net deposit: got %s, want 500000", first.NetDeposit.String())
	}
}

func TestComputeEquityCurve_AllPortfoliosMatchingCurrencies(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAllAccounts([]AccountRef{
		{ID: 1, PortfolioID: 1, PortfolioCurrency: "USD"},
		{ID: 2, PortfolioID: 2, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
	})
	txnRepo.SetTransactions(2, []transaction.Transaction{
		eqTxn(2, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
	})

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		// No PortfolioID → all portfolios.
		Period: "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Combined: $10,000 + $5,000 = $15,000.
	first := result.EquityCurve[0]
	if !first.PortfolioValue.Equal(decimal.MustNew(1500000, 2)) {
		t.Errorf("portfolio value: got %s, want 1500000", first.PortfolioValue.String())
	}
	if !first.NetDeposit.Equal(decimal.MustNew(1500000, 2)) {
		t.Errorf("net deposit: got %s, want 1500000", first.NetDeposit.String())
	}
}

func TestComputeEquityCurve_NoMarketFetcher(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
	})

	// No market fetcher configured — positions should be excluded from value.
	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Feb 15: only cash (-$500), AAPL excluded (no fetcher).
	var feb15 *EquityCurvePoint
	for i := range result.EquityCurve {
		if result.EquityCurve[i].Date.Equal(testTime(2024, 2, 15)) {
			feb15 = &result.EquityCurve[i]
			break
		}
	}
	if feb15 == nil {
		t.Fatal("expected Feb 15 point")
	}
	wantPV := decimal.MustNew(-500000, 2)
	if !feb15.PortfolioValue.Equal(wantPV) {
		t.Errorf("Feb 15 portfolio value: got %s, want %s", feb15.PortfolioValue.String(), wantPV.String())
	}
}

func TestComputeEquityCurve_NegativeNetDeposit(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
		eqTxn(1, testTime(2024, 2, 15), "withdrawal", "$CASH-USD", "USD", 0, 0, -800000),
	})

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Last point: net deposit = $5,000 - $8,000 = -$3,000.
	last := result.EquityCurve[len(result.EquityCurve)-1]
	wantND := decimal.MustNew(-300000, 2)
	if !last.NetDeposit.Equal(wantND) {
		t.Errorf("net deposit: got %s, want %s", last.NetDeposit.String(), wantND.String())
	}
	// Portfolio value should also be -$3,000 (only cash, no positions).
	if !last.PortfolioValue.Equal(wantND) {
		t.Errorf("portfolio value: got %s, want %s", last.PortfolioValue.String(), wantND.String())
	}
}

func TestComputeEquityCurve_StaleDataWarning(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
	})

	// Set cached prices with an old latest date (far in the past).
	oldDate := testTime(2024, 2, 15)
	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
	})
	// Override latest date to be old.
	repo.SetLatestDate("AAPL", oldDate)
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have a stale data warning for AAPL.
	var foundStale bool
	for _, w := range result.Warnings {
		if strings.HasPrefix(w, "stale market") && strings.Contains(w, "AAPL") {
			foundStale = true
			break
		}
	}
	if !foundStale {
		t.Errorf("expected stale market data warning for AAPL, got warnings: %v", result.Warnings)
	}
}

func TestComputeEquityCurve_PartialCache(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -1500000),
		eqTxn(1, testTime(2024, 3, 15), "buy", "MSFT", "USD", 500, 40000, -2000000),
	})

	// Only AAPL is cached, MSFT is missing.
	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
	})
	// Set AAPL's latest date to today so it doesn't trigger a stale warning.
	now := time.Now().UTC()
	repo.SetLatestDate("AAPL", now)
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have a missing data warning for MSFT.
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if result.Warnings[0] != "missing market data for MSFT" {
		t.Errorf("unexpected warning: %s", result.Warnings[0])
	}
}
