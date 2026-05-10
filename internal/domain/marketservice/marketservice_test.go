package marketservice

import (
	"context"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

var ctx = context.Background()

// --- Mocks ---

type mockFetcher struct {
	quotes   map[string]*market.MarketData
	fxRates  map[string]*market.MarketData // key: "BASE/QUOTE"
	fxErr    error
	err      error
}

func (m *mockFetcher) FetchQuotesBatch(_ context.Context, symbols []string) map[string]*market.MarketData {
	result := make(map[string]*market.MarketData)
	if m.err != nil {
		return result
	}
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			result[sym] = q
		}
	}
	return result
}

func (m *mockFetcher) FetchFxRate(_ context.Context, base, quote string) (*market.MarketData, error) {
	if m.fxErr != nil {
		return nil, m.fxErr
	}
	if m.fxRates == nil {
		return nil, nil
	}
	pair := market.FormatFxPair(base, quote)
	if md, ok := m.fxRates[pair]; ok {
		return md, nil
	}
	return nil, nil
}

type mockRepo struct {
	quotes       map[string]*market.MarketData
	fxRates      map[string]*market.MarketData
	historical   map[string][]market.HistoricalPrice
	latestDates  map[string]*time.Time
	upserted     []*market.MarketData
	upsertErr    error
	getHistErr   error
}

func (m *mockRepo) GetLatestQuotesBatch(_ context.Context, symbols []string) map[string]*market.MarketData {
	result := make(map[string]*market.MarketData)
	if m.quotes == nil {
		return result
	}
	for _, sym := range symbols {
		if q, ok := m.quotes[sym]; ok {
			result[sym] = q
		}
	}
	return result
}

func (m *mockRepo) GetHistoricalPricesBySymbol(_ context.Context, symbol string, _, _ time.Time) ([]market.HistoricalPrice, error) {
	if m.getHistErr != nil {
		return nil, m.getHistErr
	}
	if m.historical == nil {
		return nil, nil
	}
	prices, ok := m.historical[symbol]
	if !ok {
		return nil, nil
	}
	return prices, nil
}

func (m *mockRepo) GetLatestPriceDatePerSymbol(_ context.Context, symbols []string) map[string]*time.Time {
	result := make(map[string]*time.Time)
	if m.latestDates == nil {
		return result
	}
	for _, sym := range symbols {
		if t, ok := m.latestDates[sym]; ok {
			result[sym] = t
		}
	}
	return result
}

func (m *mockRepo) Upsert(_ context.Context, md *market.MarketData) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserted = append(m.upserted, md)
	return nil
}

func (m *mockRepo) GetCurrentFxRate(_ context.Context, base, quote string) (*market.MarketData, error) {
	if m.fxRates == nil {
		return nil, nil
	}
	pair := market.FormatFxPair(base, quote)
	if md, ok := m.fxRates[pair]; ok {
		return md, nil
	}
	return nil, nil
}

func (m *mockRepo) GetBySourceAndDate(_ context.Context, symbol, _, date string) (*market.MarketData, error) {
	if m.fxRates == nil {
		return nil, nil
	}
	// Simple lookup: key is "BASE/QUOTE" or "BASE/QUOTE:date"
	if md, ok := m.fxRates[symbol]; ok {
		if date == "" || md.Date == date {
			return md, nil
		}
	}
	if md, ok := m.fxRates[symbol+":"+date]; ok {
		return md, nil
	}
	return nil, nil
}

// --- GetQuotes tests ---

func TestGetQuotes_FromCache(t *testing.T) {
	price := decimal.MustNew(15000, 2)
	svc := New(nil, &mockRepo{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: price, Currency: "USD"},
		},
	})

	result := svc.GetQuotes(ctx, []string{"AAPL", "MSFT"})

	if len(result) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(result))
	}
	if result["AAPL"] == nil || !result["AAPL"].Price.Equal(price) {
		t.Errorf("expected AAPL price %s, got %v", price.String(), result["AAPL"])
	}
	if _, ok := result["MSFT"]; ok {
		t.Error("expected MSFT omitted (not in cache)")
	}
}

func TestGetQuotes_NoRepo(t *testing.T) {
	svc := New(nil, nil)

	result := svc.GetQuotes(ctx, []string{"AAPL"})

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestGetQuotes_EmptySymbols(t *testing.T) {
	svc := New(nil, &mockRepo{quotes: map[string]*market.MarketData{}})

	result := svc.GetQuotes(ctx, []string{})

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

// --- GetHistoricalPrices tests ---

func TestGetHistoricalPrices_FromCache(t *testing.T) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	price := decimal.MustNew(15000, 2)
	svc := New(nil, &mockRepo{
		historical: map[string][]market.HistoricalPrice{
			"AAPL": {{Date: date, Close: price, Currency: "USD"}},
		},
	})

	prices, err := svc.GetHistoricalPrices(ctx, "AAPL", date.AddDate(0, 0, -1), date.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}
	if !prices[0].Close.Equal(price) {
		t.Errorf("expected price %s, got %s", price.String(), prices[0].Close.String())
	}
}

func TestGetHistoricalPrices_NoData(t *testing.T) {
	svc := New(nil, &mockRepo{})

	prices, err := svc.GetHistoricalPrices(ctx, "AAPL", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prices != nil {
		t.Errorf("expected nil, got %v", prices)
	}
}

func TestGetHistoricalPrices_NoRepo(t *testing.T) {
	svc := New(nil, nil)

	prices, err := svc.GetHistoricalPrices(ctx, "AAPL", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prices != nil {
		t.Errorf("expected nil, got %v", prices)
	}
}

// --- GetLatestPriceDatePerSymbol tests ---

func TestGetLatestPriceDatePerSymbol(t *testing.T) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	svc := New(nil, &mockRepo{
		latestDates: map[string]*time.Time{
			"AAPL": &date,
		},
	})

	result := svc.GetLatestPriceDatePerSymbol(ctx, []string{"AAPL", "MSFT"})

	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result["AAPL"] == nil || !result["AAPL"].Equal(date) {
		t.Errorf("expected date %v, got %v", date, result["AAPL"])
	}
	if _, ok := result["MSFT"]; ok {
		t.Error("expected MSFT omitted")
	}
}

func TestGetLatestPriceDatePerSymbol_NoRepo(t *testing.T) {
	svc := New(nil, nil)

	result := svc.GetLatestPriceDatePerSymbol(ctx, []string{"AAPL"})

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

// --- RefreshQuotes tests ---

func TestRefreshQuotes_Success(t *testing.T) {
	price := decimal.MustNew(15000, 2)
	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: price, Currency: "USD"},
			"MSFT": {Symbol: "MSFT", Price: price, Currency: "USD"},
		},
	}
	repo := &mockRepo{}
	svc := New(fetcher, repo)

	result := svc.RefreshQuotes(ctx, []string{"AAPL", "MSFT"})

	if len(result.Refreshed) != 2 {
		t.Errorf("expected 2 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d: %v", len(result.Failed), result.Failed)
	}
	if len(repo.upserted) != 2 {
		t.Errorf("expected 2 upserted, got %d", len(repo.upserted))
	}
}

func TestRefreshQuotes_PartialFailure(t *testing.T) {
	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(15000, 2), Currency: "USD"},
			// MSFT not in map → fetch fails
		},
	}
	repo := &mockRepo{}
	svc := New(fetcher, repo)

	result := svc.RefreshQuotes(ctx, []string{"AAPL", "MSFT"})

	if len(result.Refreshed) != 1 || result.Refreshed[0] != "AAPL" {
		t.Errorf("expected [AAPL] refreshed, got %v", result.Refreshed)
	}
	if len(result.Failed) != 1 || result.Failed[0] != "MSFT" {
		t.Errorf("expected [MSFT] failed, got %v", result.Failed)
	}
}

func TestRefreshQuotes_NoFetcher(t *testing.T) {
	svc := New(nil, &mockRepo{})

	result := svc.RefreshQuotes(ctx, []string{"AAPL"})

	if len(result.Refreshed) != 0 {
		t.Errorf("expected 0 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestRefreshQuotes_EmptySymbols(t *testing.T) {
	fetcher := &mockFetcher{quotes: map[string]*market.MarketData{}}
	svc := New(fetcher, &mockRepo{})

	result := svc.RefreshQuotes(ctx, []string{})

	if len(result.Refreshed) != 0 {
		t.Errorf("expected 0 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

func TestRefreshQuotes_UpsertError(t *testing.T) {
	fetcher := &mockFetcher{
		quotes: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(15000, 2), Currency: "USD"},
		},
	}
	repo := &mockRepo{upsertErr: assertErr{}}
	svc := New(fetcher, repo)

	result := svc.RefreshQuotes(ctx, []string{"AAPL"})

	if len(result.Refreshed) != 0 {
		t.Errorf("expected 0 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 1 || result.Failed[0] != "AAPL" {
		t.Errorf("expected [AAPL] failed, got %v", result.Failed)
	}
}

// --- FX tests ---

func TestGetCurrentFxRate_FromCache(t *testing.T) {
	rate := decimal.MustNew(13000, 2)
	svc := New(nil, &mockRepo{
		fxRates: map[string]*market.MarketData{
			"GBP/USD": {Symbol: "GBP/USD", Price: rate, Currency: "USD", DataType: "fx"},
		},
	})

	fx, err := svc.GetCurrentFxRate(ctx, "GBP", "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fx == nil {
		t.Fatal("expected non-nil rate")
	}
	if !fx.Rate.Equal(rate) {
		t.Errorf("expected rate %s, got %s", rate.String(), fx.Rate.String())
	}
	if fx.BaseCurrency != "GBP" || fx.QuoteCurrency != "USD" {
		t.Errorf("unexpected pair: %s/%s", fx.BaseCurrency, fx.QuoteCurrency)
	}
}

func TestGetCurrentFxRate_NotCached(t *testing.T) {
	svc := New(nil, &mockRepo{})

	fx, err := svc.GetCurrentFxRate(ctx, "GBP", "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fx != nil {
		t.Errorf("expected nil, got %v", fx)
	}
}

func TestGetCurrentFxRate_NoRepo(t *testing.T) {
	svc := New(nil, nil)

	fx, err := svc.GetCurrentFxRate(ctx, "GBP", "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fx != nil {
		t.Errorf("expected nil, got %v", fx)
	}
}

func TestGetHistoricalFxRate_FromCache(t *testing.T) {
	rate := decimal.MustNew(12800, 2)
	svc := New(nil, &mockRepo{
		fxRates: map[string]*market.MarketData{
			"GBP/USD:2024-01-15": {Symbol: "GBP/USD", Price: rate, Currency: "USD", DataType: "fx", Date: "2024-01-15"},
		},
	})

	fx, err := svc.GetHistoricalFxRate(ctx, "GBP", "USD", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fx == nil {
		t.Fatal("expected non-nil rate")
	}
	if !fx.Rate.Equal(rate) {
		t.Errorf("expected rate %s, got %s", rate.String(), fx.Rate.String())
	}
}

func TestGetHistoricalFxRate_NotCached(t *testing.T) {
	svc := New(nil, &mockRepo{})

	fx, err := svc.GetHistoricalFxRate(ctx, "GBP", "USD", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fx != nil {
		t.Errorf("expected nil, got %v", fx)
	}
}

func TestRefreshFxRates_Success(t *testing.T) {
	rate := decimal.MustNew(13000, 2)
	fetcher := &mockFetcher{
		fxRates: map[string]*market.MarketData{
			"GBP/USD": {Symbol: "GBP/USD", Price: rate, Currency: "USD", DataType: "fx"},
			"EUR/USD": {Symbol: "EUR/USD", Price: rate, Currency: "USD", DataType: "fx"},
		},
	}
	repo := &mockRepo{}
	svc := New(fetcher, repo)

	pairs := []FxPair{
		{BaseCurrency: "GBP", QuoteCurrency: "USD"},
		{BaseCurrency: "EUR", QuoteCurrency: "USD"},
	}
	result := svc.RefreshFxRates(ctx, pairs)

	if len(result.Refreshed) != 2 {
		t.Errorf("expected 2 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
	if len(repo.upserted) != 2 {
		t.Errorf("expected 2 upserted, got %d", len(repo.upserted))
	}
}

func TestRefreshFxRates_PartialFailure(t *testing.T) {
	fetcher := &mockFetcher{
		fxRates: map[string]*market.MarketData{
			"GBP/USD": {Symbol: "GBP/USD", Price: decimal.MustNew(13000, 2), Currency: "USD"},
			// EUR/USD not in map → fetch fails
		},
	}
	repo := &mockRepo{}
	svc := New(fetcher, repo)

	pairs := []FxPair{
		{BaseCurrency: "GBP", QuoteCurrency: "USD"},
		{BaseCurrency: "EUR", QuoteCurrency: "USD"},
	}
	result := svc.RefreshFxRates(ctx, pairs)

	if len(result.Refreshed) != 1 {
		t.Errorf("expected 1 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestRefreshFxRates_NoFetcher(t *testing.T) {
	svc := New(nil, &mockRepo{})

	pairs := []FxPair{{BaseCurrency: "GBP", QuoteCurrency: "USD"}}
	result := svc.RefreshFxRates(ctx, pairs)

	if len(result.Refreshed) != 0 {
		t.Errorf("expected 0 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestRefreshFxRates_EmptyPairs(t *testing.T) {
	fetcher := &mockFetcher{fxRates: map[string]*market.MarketData{}}
	svc := New(fetcher, &mockRepo{})

	result := svc.RefreshFxRates(ctx, []FxPair{})

	if len(result.Refreshed) != 0 {
		t.Errorf("expected 0 refreshed, got %d", len(result.Refreshed))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "assert error" }
