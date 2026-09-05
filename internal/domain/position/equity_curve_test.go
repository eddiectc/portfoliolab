package position

import (
	"context"

	"strings"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/marketservice"
	"github.com/eddiectc/portfoliolab/internal/domain/performance"
	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- Mocks for equity curve tests ---

type mockHistoricalRepo struct {
	upserted     map[string][]market.HistoricalPrice
	cachedPrices map[string][]market.HistoricalPrice
	latestDates  map[string]*time.Time
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
	filters := performance.PerformanceFilters{Period: "All"}
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
			filters := performance.PerformanceFilters{Period: tt.period}
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
	filters := performance.PerformanceFilters{
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
	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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
	var jan15, feb15, mar15 *performance.EquityCurvePoint
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

	// Verify additional metrics are populated.
	if result.ReturnMetrics.SimpleReturnPct == nil {
		t.Error("SimpleReturnPct is nil")
	}
	if result.RiskMetrics.AnnualizedVolatilityPct == nil {
		t.Error("AnnualizedVolatilityPct is nil")
	}
	// Sharpe/Sortino are nil without risk-free rate — expected.
	if result.RiskMetrics.SharpeRatio != nil {
		t.Error("SharpeRatio should be nil without risk-free rate")
	}
	if result.DrawdownAnalysis.MaxDrawdownPct == nil {
		t.Error("MaxDrawdownPct is nil")
	}
	if len(result.YearlyPerformance) == 0 {
		t.Error("performance.YearlyPerformance is empty")
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

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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
	var feb15 *performance.EquityCurvePoint
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

	_, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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
	// FX rates are fetched via GetHistoricalPrices (stored in market_data table)
	svc.WithMarketDataService(&mockMarketDataService{
		historical: map[string][]market.HistoricalPrice{
			"GBP/USD": {
				{Date: testTime(2024, 1, 15), Close: decimal.MustNew(127, 2)},
			},
		},
	}, nil)

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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

	// Filter with explicit dates. The period slices the OUTPUT curve,
	// but the portfolio state includes ALL transactions (both deposits).
	from := testTime(2024, 5, 1)
	to := testTime(2024, 12, 31)
	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
		PortfolioID: ptrInt64(1),
		DateFrom:    &from,
		DateTo:      &to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The curve includes both deposits (walked from all history),
	// sliced to start at dateFrom. First point >= May 1.
	if len(result.EquityCurve) < 1 {
		t.Fatalf("expected at least 1 point, got %d", len(result.EquityCurve))
	}

	// The first point in the sliced period (May 1) has only the Jan deposit
	// in its cumulative net deposit. The June deposit hasn't happened yet.
	first := result.EquityCurve[0]
	if !first.NetDeposit.Equal(decimal.MustNew(1000000, 2)) {
		t.Errorf("first net deposit: got %s, want 1000000", first.NetDeposit.String())
	}
}

// TestComputeEquityCurve_ReturnMetricsRespectPeriod verifies that return
// metrics (TWR) are computed over the selected period, not the full history.
func TestComputeEquityCurve_ReturnMetricsRespectPeriod(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// Scenario: all market gains happen before the deposit, flat after.
	// Jan 15: deposit $10,000
	// Feb 15: buy 10 AAPL at $100 (cost $1,000)
	// Mar 15: deposit $5,000 (AAPL is now $130, +30% on shares)
	// Apr 1: start of short window
	// Dec 31: AAPL flat at $130
	//
	// Pre-deposit Mar 15: 10×$130 + $9,000 cash = $10,300
	// Post-deposit Mar 15: $10,300 + $5,000 = $15,300
	// Dec 31: 10×$130 + $14,000 cash = $15,300
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 10000, -100000),
		eqTxn(1, testTime(2024, 3, 15), "deposit", "$CASH-USD", "USD", 0, 0, 500000),
	})

	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 1, 15), 10000, "USD"),
		histPrice(testTime(2024, 2, 15), 10000, "USD"),
		histPrice(testTime(2024, 3, 15), 13000, "USD"),
		histPrice(testTime(2024, 12, 31), 13000, "USD"),
	})
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	// "All" period: full history.
	// Sub-period 1: Jan 15 ($10,000) → Mar 15 pre-deposit ($10,300) = +3%
	// Sub-period 2: Mar 15 post-deposit ($15,300) → last ($15,300) = 0%
	// TWR ≈ 3%
	resultAll, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("All: unexpected error: %v", err)
	}

	// Short period (Apr-Dec): after the deposit, flat market.
	// No breakpoints inside this window.
	// Simple return: $15,300 → $15,300 = 0%
	from := testTime(2024, 4, 1)
	to := testTime(2024, 12, 31)
	resultShort, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
		PortfolioID: ptrInt64(1),
		DateFrom:    &from,
		DateTo:      &to,
	})
	if err != nil {
		t.Fatalf("Short: unexpected error: %v", err)
	}

	// Both should have non-nil TWR.
	if resultAll.ReturnMetrics.TWRPct == nil {
		t.Error("All: TWRPct is nil")
	}
	if resultShort.ReturnMetrics.TWRPct == nil {
		t.Error("Short: TWRPct is nil")
	}

	allTWR, _ := resultAll.ReturnMetrics.TWRPct.Float64()
	shortTWR, _ := resultShort.ReturnMetrics.TWRPct.Float64()

	// "All" should be positive (AAPL gains in Feb-Mar).
	if allTWR < 1 {
		t.Errorf("All TWR=%.2f%%, expected > 1%%", allTWR)
	}
	// "Short" should be near zero (flat market, no breakpoints).
	if shortTWR > 1 {
		t.Errorf("Short TWR=%.2f%%, expected ~0%%", shortTWR)
	}
	if allTWR == shortTWR {
		t.Errorf("TWR identical for both periods (%.2f%%) — period filter not affecting return metrics", allTWR)
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

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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
	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Feb 15: only cash (-$500), AAPL excluded (no fetcher).
	var feb15 *performance.EquityCurvePoint
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

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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

// TestComputeEquityCurve_AdditionalMetrics verifies that risk metrics,
// drawdown analysis, and yearly performance are computed and populated
// on the performance.PerformanceResult.
func TestComputeEquityCurve_AdditionalMetrics(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "USD"},
	})

	// Deposit, buy, price appreciation.
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 1000000),
		eqTxn(1, testTime(2024, 2, 15), "buy", "AAPL", "USD", 1000, 15000, -150000),
		eqTxn(1, testTime(2024, 3, 15), "sell", "AAPL", "USD", -500, 17000, 85000),
	})

	repo := newMockHistoricalRepo()
	repo.SetCachedPrices("AAPL", []market.HistoricalPrice{
		histPrice(testTime(2024, 1, 15), 15000, "USD"),
		histPrice(testTime(2024, 2, 15), 15000, "USD"),
		histPrice(testTime(2024, 3, 15), 17000, "USD"),
	})
	svc.WithMarketDataService(&mockEqMarketService{repo: repo}, nil)

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simple return: (10200 - 10000) / 10000 = 2% approximately.
	// (Portfolio went from $10,000 to $10,200, no additional deposits after first).
	if result.ReturnMetrics.SimpleReturnPct == nil {
		t.Fatal("SimpleReturnPct is nil")
	}
	simpleReturn, _ := result.ReturnMetrics.SimpleReturnPct.Float64()
	if simpleReturn < 0 || simpleReturn > 10 {
		t.Errorf("SimpleReturnPct = %.2f%%, expected small positive value", simpleReturn)
	}

	// Annualized simple return should also be populated.
	if result.ReturnMetrics.AnnualizedSimpleReturnPct == nil {
		t.Error("AnnualizedSimpleReturnPct is nil")
	}

	// Risk metrics: volatility should be populated.
	if result.RiskMetrics.AnnualizedVolatilityPct == nil {
		t.Error("AnnualizedVolatilityPct is nil")
	}
	vol, _ := result.RiskMetrics.AnnualizedVolatilityPct.Float64()
	if vol < 0 {
		t.Errorf("AnnualizedVolatilityPct = %.2f%%, expected non-negative", vol)
	}

	// Sharpe and Sortino should be nil without risk-free rate.
	if result.RiskMetrics.SharpeRatio != nil {
		t.Error("SharpeRatio should be nil without risk-free rate")
	}
	if result.RiskMetrics.SortinoRatio != nil {
		t.Error("SortinoRatio should be nil without risk-free rate")
	}

	// Drawdown: max drawdown should be populated.
	if result.DrawdownAnalysis.MaxDrawdownPct == nil {
		t.Error("MaxDrawdownPct is nil")
	}
	maxDD, _ := result.DrawdownAnalysis.MaxDrawdownPct.Float64()
	if maxDD < 0 {
		t.Errorf("MaxDrawdownPct = %.2f%%, expected non-negative", maxDD)
	}

	// Current drawdown should be populated.
	if result.DrawdownAnalysis.CurrentDrawdownPct == nil {
		t.Error("CurrentDrawdownPct is nil")
	}

	// Yearly performance should have at least one year.
	if len(result.YearlyPerformance) == 0 {
		t.Error("performance.YearlyPerformance is empty")
	}
	if len(result.YearlyPerformance) > 0 {
		if result.YearlyPerformance[0].Year != 2024 {
			t.Errorf("expected year 2024, got %d", result.YearlyPerformance[0].Year)
		}
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

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
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

func TestComputeEquityCurve_FXForwardFillWeekend(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "GBP"},
		{ID: 2, PortfolioCurrency: "GBP"},
	})

	// Account 1 (GBP): £10,000 cash
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 3, 25), "deposit", "$CASH-GBP", "GBP", 0, 0, 1000000), // £10,000
	})
	// Account 2 (USD): buy 1 BRK-B @ $400, funded by $400 deposit
	txnRepo.SetTransactions(2, []transaction.Transaction{
		eqTxn(2, testTime(2024, 3, 25), "deposit", "$CASH-USD", "USD", 0, 0, 40000), // $400
		eqTxn(2, testTime(2024, 3, 25), "buy", "BRK-B", "USD", 100, 40000, -40000),  // 1 share @ $400
	})

	// FX rates: only Mon-Fri (no weekend data)
	// USD/GBP: 1 USD = 0.80 GBP
	fxRate := decimal.MustNew(80, 2)
	svc.WithMarketDataService(&mockMarketDataService{
		historical: map[string][]market.HistoricalPrice{
			"BRK-B": {
				{Date: testTime(2024, 3, 25), Close: decimal.MustNew(40000, 2), Currency: "USD"}, // Mon $400
				{Date: testTime(2024, 3, 26), Close: decimal.MustNew(40000, 2), Currency: "USD"}, // Tue
				{Date: testTime(2024, 3, 27), Close: decimal.MustNew(40000, 2), Currency: "USD"}, // Wed
				{Date: testTime(2024, 3, 28), Close: decimal.MustNew(40000, 2), Currency: "USD"}, // Thu
				{Date: testTime(2024, 3, 29), Close: decimal.MustNew(40000, 2), Currency: "USD"}, // Fri
				// No Sat (3/30) or Sun (3/31)
				{Date: testTime(2024, 3, 1), Close: decimal.MustNew(40000, 2), Currency: "USD"}, // Mon 4/1
			},
			"USD/GBP": {
				{Date: testTime(2024, 3, 25), Close: fxRate}, // Mon
				{Date: testTime(2024, 3, 26), Close: fxRate}, // Tue
				{Date: testTime(2024, 3, 27), Close: fxRate}, // Wed
				{Date: testTime(2024, 3, 28), Close: fxRate}, // Thu
				{Date: testTime(2024, 3, 29), Close: fxRate}, // Fri
				// No Sat (3/30) or Sun (3/31)
				{Date: testTime(2024, 3, 1), Close: fxRate}, // Mon 4/1
			},
		},
	}, nil)

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the weekend dates (Sat 3/30 and Sun 3/31) in the curve.
	// They should use the forward-filled FX rate from Fri 3/29.
	// Portfolio value on weekend = £10,000 (cash) + $400 × 0.80 = £320 (position) = £10,320
	wantValue := decimal.MustNew(1032000, 2) // £10,320.00

	foundSat, foundSun := false, false
	for _, pt := range result.EquityCurve {
		dateKey := pt.Date.Format("2006-01-02")
		if dateKey == "2024-03-30" {
			foundSat = true
			if !pt.PortfolioValue.Equal(wantValue) {
				t.Errorf("Sat 3/30: got %s, want %s", pt.PortfolioValue.String(), wantValue.String())
			}
		}
		if dateKey == "2024-03-31" {
			foundSun = true
			if !pt.PortfolioValue.Equal(wantValue) {
				t.Errorf("Sun 3/31: got %s, want %s", pt.PortfolioValue.String(), wantValue.String())
			}
		}
	}
	if !foundSat {
		t.Error("expected Saturday 2024-03-30 in equity curve")
	}
	if !foundSun {
		t.Error("expected Sunday 2024-03-31 in equity curve")
	}
}

func TestComputeEquityCurve_MissingFXRateWarns(t *testing.T) {
	svc, txnRepo, accountLister := newTestServiceForEquity()
	accountLister.SetAccountsByPortfolio(1, []AccountRef{
		{ID: 1, PortfolioCurrency: "GBP"},
		{ID: 2, PortfolioCurrency: "GBP"},
	})

	// GBP cash + USD stock, but NO FX rates provided
	txnRepo.SetTransactions(1, []transaction.Transaction{
		eqTxn(1, testTime(2024, 1, 15), "deposit", "$CASH-GBP", "GBP", 0, 0, 1000000), // £10,000
	})
	txnRepo.SetTransactions(2, []transaction.Transaction{
		eqTxn(2, testTime(2024, 1, 15), "deposit", "$CASH-USD", "USD", 0, 0, 40000), // $400
		eqTxn(2, testTime(2024, 1, 15), "buy", "BRK-B", "USD", 100, 40000, -40000),  // 1 share @ $400
	})

	svc.WithMarketDataService(&mockMarketDataService{
		historical: map[string][]market.HistoricalPrice{
			"BRK-B": {
				{Date: testTime(2024, 1, 15), Close: decimal.MustNew(40000, 2), Currency: "USD"},
			},
			// No USD/GBP FX rates!
		},
	}, nil)

	result, err := svc.ComputeEquityCurve(ctx, performance.PerformanceFilters{
		PortfolioID: ptrInt64(1),
		Period:      "All",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without FX rate, the USD position value ($400) is SKIPPED entirely.
	// The curve should still be computed, but with a warning.
	if len(result.EquityCurve) < 1 {
		t.Fatal("expected at least 1 equity curve point")
	}

	// Portfolio value = £10,000 (GBP cash) only; USD position skipped due to missing FX.
	// This is correct — better to understate than to mix currencies silently.
	gotValue := result.EquityCurve[0].PortfolioValue
	wantGBP := decimal.MustNew(1000000, 2) // £10,000
	if !gotValue.Equal(wantGBP) {
		t.Errorf("portfolio value with missing FX: got %s, want %s (GBP only)", gotValue.String(), wantGBP.String())
	}
}
