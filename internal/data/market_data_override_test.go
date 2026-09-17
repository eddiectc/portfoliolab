package data

import (
	"context"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/marketdata"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// seedOverride inserts an override directly through the repository (no
// domain-service validation — that is covered in the domain package).
func seedOverride(t *testing.T, repo *MarketDataOverrideRepository, symbol, date string, dt marketdata.DataType, price, currency string) {
	t.Helper()
	p, err := decimal.Parse(price)
	if err != nil {
		t.Fatalf("parse price %q: %v", price, err)
	}
	err = repo.Upsert(context.Background(), &marketdata.Override{
		Symbol:   symbol,
		Date:     date,
		DataType: dt,
		Price:    p,
		Currency: currency,
	})
	if err != nil {
		t.Fatalf("seed override %s/%s/%s: %v", symbol, date, dt, err)
	}
}

func TestMarketDataOverrideRepository_UpsertAndGet(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	seedOverride(t, repo, "AAPL", "2026-05-07", marketdata.DataTypeStock, "185.50", "USD")

	got, err := repo.Get(ctx, "AAPL", "2026-05-07", marketdata.DataTypeStock)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected override, got nil")
	}
	if got.Symbol != "AAPL" || got.Date != "2026-05-07" || got.Currency != "USD" {
		t.Errorf("unexpected override: %+v", got)
	}
	if got.DataType != marketdata.DataTypeStock {
		t.Errorf("data type = %v, want stock", got.DataType)
	}
	if !got.Price.Equal(decimal.MustParse("185.50")) {
		t.Errorf("price = %v, want 185.50", got.Price)
	}
	if got.Source != marketdata.Source {
		t.Errorf("source = %q, want %q", got.Source, marketdata.Source)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not set: %+v", got)
	}

	// Misses: wrong date, wrong data type, wrong symbol.
	for _, tc := range []struct {
		symbol, date string
		dt           marketdata.DataType
	}{
		{"AAPL", "2026-05-08", marketdata.DataTypeStock},
		{"AAPL", "2026-05-07", marketdata.DataTypeFx},
		{"MSFT", "2026-05-07", marketdata.DataTypeStock},
	} {
		got, err := repo.Get(ctx, tc.symbol, tc.date, tc.dt)
		if err != nil {
			t.Fatalf("Get(%s, %s, %s): %v", tc.symbol, tc.date, tc.dt, err)
		}
		if got != nil {
			t.Errorf("Get(%s, %s, %s) = %+v, want nil", tc.symbol, tc.date, tc.dt, got)
		}
	}
}

func TestMarketDataOverrideRepository_UpsertReplaces(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	seedOverride(t, repo, "AAPL", "2026-05-07", marketdata.DataTypeStock, "185.50", "USD")
	seedOverride(t, repo, "AAPL", "2026-05-07", marketdata.DataTypeStock, "190.25", "USD")

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data_overrides`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row after replace, got %d", count)
	}

	got, err := repo.Get(ctx, "AAPL", "2026-05-07", marketdata.DataTypeStock)
	if err != nil || got == nil {
		t.Fatalf("Get after replace: %v, %+v", err, got)
	}
	if !got.Price.Equal(decimal.MustParse("190.25")) {
		t.Errorf("price = %v, want 190.25 (last write wins)", got.Price)
	}
}

func TestMarketDataOverrideRepository_List(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	// Seed out of order to verify the stable (symbol, date, data_type) order.
	seedOverride(t, repo, "MSFT", "2026-05-06", marketdata.DataTypeStock, "450.00", "USD")
	seedOverride(t, repo, "AAPL", "2026-05-08", marketdata.DataTypeStock, "190.00", "USD")
	seedOverride(t, repo, "AAPL", "2026-05-07", marketdata.DataTypeStock, "185.50", "USD")
	seedOverride(t, repo, "AAPL", "2026-05-07", marketdata.DataTypeFx, "2.50", "USD")
	seedOverride(t, repo, "GBP/USD", "2026-05-05", marketdata.DataTypeFx, "1.2700", "USD")

	all, err := repo.List(ctx, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantOrder := []struct{ symbol, date, dt string }{
		{"AAPL", "2026-05-07", "fx"},
		{"AAPL", "2026-05-07", "stock"},
		{"AAPL", "2026-05-08", "stock"},
		{"GBP/USD", "2026-05-05", "fx"},
		{"MSFT", "2026-05-06", "stock"},
	}
	if len(all) != len(wantOrder) {
		t.Fatalf("expected %d overrides, got %d", len(wantOrder), len(all))
	}
	for i, w := range wantOrder {
		if all[i].Symbol != w.symbol || all[i].Date != w.date || string(all[i].DataType) != w.dt {
			t.Errorf("override[%d] = %s %s %s, want %s %s %s",
				i, all[i].Symbol, all[i].Date, all[i].DataType, w.symbol, w.date, w.dt)
		}
	}

	// Symbol filter.
	sym := "AAPL"
	aapl, err := repo.List(ctx, &sym)
	if err != nil {
		t.Fatalf("List(AAPL): %v", err)
	}
	if len(aapl) != 3 {
		t.Fatalf("expected 3 AAPL overrides, got %d", len(aapl))
	}
	for _, o := range aapl {
		if o.Symbol != "AAPL" {
			t.Errorf("List(AAPL) returned %s", o.Symbol)
		}
	}

	// Empty result for an unknown symbol.
	other := "NOPE"
	none, err := repo.List(ctx, &other)
	if err != nil {
		t.Fatalf("List(NOPE): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 overrides for NOPE, got %d", len(none))
	}
}

func TestMarketDataOverrideRepository_Delete(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	seedOverride(t, repo, "AAPL", "2026-05-07", marketdata.DataTypeStock, "185.50", "USD")
	seedOverride(t, repo, "AAPL", "2026-05-07", marketdata.DataTypeFx, "2.50", "USD")

	ok, err := repo.Delete(ctx, "AAPL", "2026-05-07", marketdata.DataTypeStock)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !ok {
		t.Fatal("expected delete to report true")
	}

	// Second delete of the same key reports not-found.
	ok, err = repo.Delete(ctx, "AAPL", "2026-05-07", marketdata.DataTypeStock)
	if err != nil {
		t.Fatalf("Delete again: %v", err)
	}
	if ok {
		t.Fatal("expected second delete to report false")
	}

	// A different data type on the same date is untouched.
	ok, err = repo.Delete(ctx, "AAPL", "2026-05-07", marketdata.DataTypeFx)
	if err != nil {
		t.Fatalf("Delete fx: %v", err)
	}
	if !ok {
		t.Fatal("expected fx delete to report true")
	}
}

func TestMarketDataRepository_GetHistoricalPricesBySymbol_AppliesOverrides(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)
	orepo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	prices := []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), Close: decimal.MustParse("17000.00"), Currency: "USD"},
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: decimal.MustParse("17500.00"), Currency: "USD"},
		{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), Close: decimal.MustParse("18000.00"), Currency: "USD"},
	}
	if err := repo.UpsertHistoricalPrices(ctx, "AAPL", prices, "stock"); err != nil {
		t.Fatalf("seed prices: %v", err)
	}

	// Override the middle date, add a date with no provider row, and add an
	// out-of-range date that must be excluded.
	seedOverride(t, orepo, "AAPL", "2026-05-07", marketdata.DataTypeStock, "17555.55", "USD")
	seedOverride(t, orepo, "AAPL", "2026-05-05", marketdata.DataTypeStock, "16800.00", "USD")
	seedOverride(t, orepo, "AAPL", "2026-05-04", marketdata.DataTypeStock, "16000.00", "USD")

	got, err := repo.GetHistoricalPricesBySymbol(ctx, "AAPL",
		time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	want := []struct {
		date, price, currency string
	}{
		{"2026-05-05", "16800.00", "USD"}, // override-only date
		{"2026-05-06", "17000.00", "USD"}, // provider, unchanged
		{"2026-05-07", "17555.55", "USD"}, // replaced by override
		{"2026-05-08", "18000.00", "USD"}, // provider, unchanged
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d prices, got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if d := got[i].Date.Format("2006-01-02"); d != w.date {
			t.Errorf("price[%d] date = %s, want %s", i, d, w.date)
		}
		if !got[i].Close.Equal(decimal.MustParse(w.price)) {
			t.Errorf("price[%d] = %v, want %s", i, got[i].Close, w.price)
		}
		if got[i].Currency != w.currency {
			t.Errorf("price[%d] currency = %s, want %s", i, got[i].Currency, w.currency)
		}
	}
}

func TestMarketDataRepository_OverridesDoNotTouchProviderRows(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)
	orepo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	prices := []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: decimal.MustParse("17500.00"), Currency: "USD"},
	}
	if err := repo.UpsertHistoricalPrices(ctx, "AAPL", prices, "stock"); err != nil {
		t.Fatalf("seed prices: %v", err)
	}
	seedOverride(t, orepo, "AAPL", "2026-05-07", marketdata.DataTypeStock, "17555.55", "USD")

	// Reads resolve to the override…
	got, err := repo.GetHistoricalPricesBySymbol(ctx, "AAPL",
		time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(got) != 1 || !got[0].Close.Equal(decimal.MustParse("17555.55")) {
		t.Fatalf("expected overridden price, got %+v", got)
	}

	// …while the provider row stays untouched in the table.
	var price string
	if err := db.QueryRowContext(ctx,
		`SELECT price FROM market_data WHERE symbol = 'AAPL' AND date = '2026-05-07' AND source = 'yahoo'`).Scan(&price); err != nil {
		t.Fatalf("query provider row: %v", err)
	}
	if price != "17500.00" {
		t.Errorf("provider row price = %s, want 17500.00 (unmodified)", price)
	}
}

func TestMarketDataRepository_GetHistoricalFxRateOnOrBefore_OverrideWins(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)
	orepo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	rates := []market.HistoricalPrice{
		{Date: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC), Close: decimal.MustParse("1.2700"), Currency: "USD"},
	}
	if err := repo.UpsertHistoricalPrices(ctx, "GBP/USD", rates, "fx"); err != nil {
		t.Fatalf("seed rates: %v", err)
	}
	seedOverride(t, orepo, "GBP/USD", "2024-01-10", marketdata.DataTypeFx, "1.2800", "USD")

	// Date after the override date → override.
	got, err := repo.GetHistoricalFxRateOnOrBefore(ctx, "GBP/USD", "yahoo", "2024-01-12")
	if err != nil {
		t.Fatalf("GetHistoricalFxRateOnOrBefore(2024-01-12): %v", err)
	}
	if got == nil {
		t.Fatal("expected rate, got nil")
	}
	if !got.Price.Equal(decimal.MustParse("1.2800")) {
		t.Errorf("price = %v, want 1.2800 (override)", got.Price)
	}
	if got.Source != marketdata.Source {
		t.Errorf("source = %q, want %q", got.Source, marketdata.Source)
	}

	// Date before the override date → provider row.
	got, err = repo.GetHistoricalFxRateOnOrBefore(ctx, "GBP/USD", "yahoo", "2024-01-08")
	if err != nil {
		t.Fatalf("GetHistoricalFxRateOnOrBefore(2024-01-08): %v", err)
	}
	if got == nil {
		t.Fatal("expected rate, got nil")
	}
	if !got.Price.Equal(decimal.MustParse("1.2700")) {
		t.Errorf("price = %v, want 1.2700 (provider)", got.Price)
	}
	if got.Source != "yahoo" {
		t.Errorf("source = %q, want yahoo", got.Source)
	}

	// Pair with no data at all → nil.
	got, err = repo.GetHistoricalFxRateOnOrBefore(ctx, "EUR/USD", "yahoo", "2024-01-08")
	if err != nil {
		t.Fatalf("GetHistoricalFxRateOnOrBefore(EUR/USD): %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for EUR/USD, got %+v", got)
	}
}

func TestMarketDataRepository_GetNavHistoryBySymbol_AppliesOverrides(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)
	orepo := NewMarketDataOverrideRepository(db)
	ctx := context.Background()

	navs := []market.HistoricalPrice{
		{Date: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), Close: decimal.MustParse("1000.00"), Currency: "GBP"},
		{Date: time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), Close: decimal.MustParse("1010.00"), Currency: "GBP"},
	}
	if err := repo.UpsertHistoricalPrices(ctx, "iShares-GBP", navs, "nav"); err != nil {
		t.Fatalf("seed navs: %v", err)
	}
	seedOverride(t, orepo, "iShares-GBP", "2026-05-31", marketdata.DataTypeNav, "1012.50", "GBP")

	got, err := repo.GetNavHistoryBySymbol(ctx, "iShares-GBP")
	if err != nil {
		t.Fatalf("GetNavHistoryBySymbol: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 nav entries, got %d", len(got))
	}
	if !got[0].Close.Equal(decimal.MustParse("1000.00")) {
		t.Errorf("nav[0] = %v, want 1000.00 (provider)", got[0].Close)
	}
	if !got[1].Close.Equal(decimal.MustParse("1012.50")) {
		t.Errorf("nav[1] = %v, want 1012.50 (override)", got[1].Close)
	}
	if got[1].Currency != "GBP" {
		t.Errorf("nav[1] currency = %s, want GBP", got[1].Currency)
	}
}
