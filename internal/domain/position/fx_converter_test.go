package position

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

var fxCtx = context.Background()

// --- Mocks ---

type mockFxRepo struct {
	bySymbolDate map[string]map[string]*market.MarketData // symbol → date → MarketData
	currentFx    map[string]*market.MarketData             // pair → MarketData
	upsertCalls  []*market.MarketData
}

func newMockFxRepo() *mockFxRepo {
	return &mockFxRepo{
		bySymbolDate: make(map[string]map[string]*market.MarketData),
		currentFx:    make(map[string]*market.MarketData),
	}
}

func (m *mockFxRepo) setHistorical(symbol, date string, md *market.MarketData) {
	if m.bySymbolDate[symbol] == nil {
		m.bySymbolDate[symbol] = make(map[string]*market.MarketData)
	}
	m.bySymbolDate[symbol][date] = md
}

func (m *mockFxRepo) setCurrentFx(pair string, md *market.MarketData) {
	m.currentFx[pair] = md
}

func (m *mockFxRepo) GetLatest(_ context.Context, symbol string) (*market.MarketData, error) {
	if m.bySymbolDate[symbol] != nil {
		if md, ok := m.bySymbolDate[symbol][""]; ok {
			return md, nil
		}
	}
	return nil, nil
}

func (m *mockFxRepo) GetBySourceAndDate(_ context.Context, symbol, _source, date string) (*market.MarketData, error) {
	if m.bySymbolDate[symbol] != nil {
		if md, ok := m.bySymbolDate[symbol][date]; ok {
			return md, nil
		}
	}
	return nil, nil
}

func (m *mockFxRepo) Upsert(_ context.Context, md *market.MarketData) error {
	m.upsertCalls = append(m.upsertCalls, md)
	return nil
}

func (m *mockFxRepo) GetCurrentFxRate(_ context.Context, pair string) (*market.MarketData, error) {
	if md, ok := m.currentFx[pair]; ok {
		return md, nil
	}
	return nil, nil
}

type mockFxFetcher struct {
	rates map[string]*market.FxRate
	err   error
}

func newMockFxFetcher() *mockFxFetcher {
	return &mockFxFetcher{
		rates: make(map[string]*market.FxRate),
	}
}

func (m *mockFxFetcher) setRate(pair string, rate decimal.Decimal) {
	base, quote, _ := market.ParseFxPair(pair)
	m.rates[pair] = &market.FxRate{
		Pair:          pair,
		BaseCurrency:  base,
		QuoteCurrency: quote,
		Rate:          rate,
		FetchedAt:     time.Now(),
	}
}

func (m *mockFxFetcher) FetchRate(_ context.Context, pair string) (*market.FxRate, error) {
	if m.err != nil {
		return nil, m.err
	}
	if rate, ok := m.rates[pair]; ok {
		return rate, nil
	}
	return nil, fmt.Errorf("no rate for %s", pair)
}

// --- ConvertPnlToBase tests ---

func TestConvertPnlToBase_SameCurrency(t *testing.T) {
	pnl := decimal.MustNew(10000, 2) // +100.00
	converted, rateUsed, isFallback := ConvertPnlToBase(pnl, "USD", "USD", nil, false)

	if !converted.Equal(pnl) {
		t.Errorf("expected unchanged P&L %s, got %s", pnl.String(), converted.String())
	}
	if rateUsed != nil {
		t.Errorf("expected nil rate, got %v", rateUsed)
	}
	if isFallback {
		t.Error("expected no fallback")
	}
}

func TestConvertPnlToBase_ConvertsToBase(t *testing.T) {
	pnl := decimal.MustNew(10000, 2) // +100.00 GBP
	rate := &market.FxRate{
		Pair: "GBP/USD", BaseCurrency: "GBP", QuoteCurrency: "USD",
		Rate: decimal.MustNew(12500, 4), // 1.2500
	}
	converted, rateUsed, isFallback := ConvertPnlToBase(pnl, "GBP", "USD", rate, false)

	// Mul combines scales: pnl (scale 2) * rate (scale 4) → scale 6
	// 100.00 * 1.2500 = 125.000000
	want := decimal.MustNew(125000000, 6)
	if !converted.Equal(want) {
		t.Errorf("expected %s, got %s", want.String(), converted.String())
	}
	if rateUsed == nil || !rateUsed.Equal(rate.Rate) {
		t.Errorf("expected rate %s, got %v", rate.Rate.String(), rateUsed)
	}
	if isFallback {
		t.Error("expected no fallback")
	}
}

func TestConvertPnlToBase_NoRateReturnsFallback(t *testing.T) {
	pnl := decimal.MustNew(10000, 2)
	converted, rateUsed, isFallback := ConvertPnlToBase(pnl, "GBP", "USD", nil, false)

	if !converted.Equal(pnl) {
		t.Errorf("expected original P&L %s, got %s", pnl.String(), converted.String())
	}
	if rateUsed != nil {
		t.Errorf("expected nil rate, got %v", rateUsed)
	}
	if !isFallback {
		t.Error("expected fallback")
	}
}

func TestConvertPnlToBase_NegativePnL(t *testing.T) {
	pnl := decimal.MustNew(-5000, 2) // -50.00 GBP
	rate := &market.FxRate{
		Pair: "GBP/USD", BaseCurrency: "GBP", QuoteCurrency: "USD",
		Rate: decimal.MustNew(12000, 4), // 1.2000
	}
	converted, _, _ := ConvertPnlToBase(pnl, "GBP", "USD", rate, false)

	// scale 2 * scale 4 = scale 6
	want := decimal.MustNew(-60000000, 6) // -50.00 * 1.2000 = -60.000000
	if !converted.Equal(want) {
		t.Errorf("expected %s, got %s", want.String(), converted.String())
	}
}

func TestConvertPnlToBase_ZeroPnL(t *testing.T) {
	pnl := decimal.Zero
	rate := &market.FxRate{
		Pair: "GBP/USD", BaseCurrency: "GBP", QuoteCurrency: "USD",
		Rate: decimal.MustNew(12500, 4),
	}
	converted, _, _ := ConvertPnlToBase(pnl, "GBP", "USD", rate, false)

	if !converted.Equal(decimal.Zero) {
		t.Errorf("expected zero, got %s", converted.String())
	}
}

// --- FxConverter tests ---

func TestFxConverter_GetRateForDate_HistoricalInDB(t *testing.T) {
	repo := newMockFxRepo()
	repo.setHistorical("GBP/USD", "2024-03-15", &market.MarketData{
		Symbol: "GBP/USD", Price: decimal.MustNew(12500, 4), DataType: "fx",
		Currency: "USD", Source: "yahoo", Date: "2024-03-15",
		FetchedAt: time.Now(),
	})

	fetcher := newMockFxFetcher()
	converter := NewFxConverter(repo, fetcher, nil)

	rate, found := converter.GetRateForDate(fxCtx, "GBP/USD", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	if !found {
		t.Error("expected historical rate found")
	}
	if rate == nil {
		t.Fatal("expected non-nil rate")
	}
	if !rate.Rate.Equal(decimal.MustNew(12500, 4)) {
		t.Errorf("expected rate 1.2500, got %s", rate.Rate.String())
	}
	// Should not have fetched from provider.
	if len(repo.upsertCalls) > 0 {
		t.Error("expected no upsert (historical rate was in DB)")
	}
}

func TestFxConverter_GetRateForDate_FetchesOnDemand(t *testing.T) {
	repo := newMockFxRepo()
	fetcher := newMockFxFetcher()
	fetcher.setRate("GBP/USD", decimal.MustNew(12600, 4))

	converter := NewFxConverter(repo, fetcher, nil)

	rate, found := converter.GetRateForDate(fxCtx, "GBP/USD", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	if found {
		t.Error("expected not found (on-demand fetch is not historical)")
	}
	if rate == nil {
		t.Fatal("expected non-nil rate")
	}
	if !rate.Rate.Equal(decimal.MustNew(12600, 4)) {
		t.Errorf("expected rate 1.2600, got %s", rate.Rate.String())
	}
	// Should have cached the fetched rate.
	if len(repo.upsertCalls) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(repo.upsertCalls))
	}
	if repo.upsertCalls[0].Date != "2024-03-15" {
		t.Errorf("expected cached date 2024-03-15, got %s", repo.upsertCalls[0].Date)
	}
}

func TestFxConverter_GetRateForDate_FallsBackToSpot(t *testing.T) {
	repo := newMockFxRepo()
	// No historical rate, but current spot available.
	repo.setCurrentFx("GBP/USD", &market.MarketData{
		Symbol: "GBP/USD", Price: decimal.MustNew(12700, 4), DataType: "fx",
		Currency: "USD", Source: "yahoo", Date: "",
		FetchedAt: time.Now(),
	})
	fetcher := newMockFxFetcher()
	fetcher.err = fmt.Errorf("fetch unavailable") // force fetch failure

	converter := NewFxConverter(repo, fetcher, nil)

	rate, found := converter.GetRateForDate(fxCtx, "GBP/USD", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	if found {
		t.Error("expected not found (spot rate is a fallback)")
	}
	if rate == nil {
		t.Fatal("expected non-nil rate (from spot fallback)")
	}
	if !rate.Rate.Equal(decimal.MustNew(12700, 4)) {
		t.Errorf("expected rate 1.2700, got %s", rate.Rate.String())
	}
}

func TestFxConverter_GetRateForDate_NoRateAvailable(t *testing.T) {
	repo := newMockFxRepo()
	fetcher := newMockFxFetcher()
	fetcher.err = fmt.Errorf("fetch unavailable")

	converter := NewFxConverter(repo, fetcher, nil)

	rate, found := converter.GetRateForDate(fxCtx, "GBP/USD", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	if found {
		t.Error("expected not found")
	}
	if rate != nil {
		t.Errorf("expected nil rate, got %v", rate)
	}
}

func TestFxConverter_GetCurrentRate_FromCache(t *testing.T) {
	repo := newMockFxRepo()
	repo.setCurrentFx("EUR/USD", &market.MarketData{
		Symbol: "EUR/USD", Price: decimal.MustNew(10800, 4), DataType: "fx",
		Currency: "USD", Source: "yahoo", Date: "",
		FetchedAt: time.Now(),
	})
	fetcher := newMockFxFetcher()

	converter := NewFxConverter(repo, fetcher, nil)

	rate, found := converter.GetCurrentRate(fxCtx, "EUR/USD")

	if !found {
		t.Error("expected rate found")
	}
	if rate == nil {
		t.Fatal("expected non-nil rate")
	}
	if !rate.Rate.Equal(decimal.MustNew(10800, 4)) {
		t.Errorf("expected rate 1.0800, got %s", rate.Rate.String())
	}
}

func TestFxConverter_GetCurrentRate_FetchesAndCaches(t *testing.T) {
	repo := newMockFxRepo()
	fetcher := newMockFxFetcher()
	fetcher.setRate("EUR/USD", decimal.MustNew(10900, 4))

	converter := NewFxConverter(repo, fetcher, nil)

	rate, found := converter.GetCurrentRate(fxCtx, "EUR/USD")

	if !found {
		t.Error("expected rate found")
	}
	if rate == nil {
		t.Fatal("expected non-nil rate")
	}
	if !rate.Rate.Equal(decimal.MustNew(10900, 4)) {
		t.Errorf("expected rate 1.0900, got %s", rate.Rate.String())
	}
	// Should have cached.
	if len(repo.upsertCalls) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(repo.upsertCalls))
	}
	if repo.upsertCalls[0].Date != "" {
		t.Errorf("expected cached date '' (current), got %q", repo.upsertCalls[0].Date)
	}
}

// --- BuildFxPair tests ---

func TestBuildFxPair_SameCurrency(t *testing.T) {
	pair := BuildFxPair("USD", "USD")
	if pair != "" {
		t.Errorf("expected empty pair, got %q", pair)
	}
}

func TestBuildFxPair_DifferentCurrency(t *testing.T) {
	pair := BuildFxPair("GBP", "USD")
	if pair != "GBP/USD" {
		t.Errorf("expected GBP/USD, got %q", pair)
	}
}

func TestBuildFxPair_EurToGbp(t *testing.T) {
	pair := BuildFxPair("EUR", "GBP")
	if pair != "EUR/GBP" {
		t.Errorf("expected EUR/GBP, got %q", pair)
	}
}
