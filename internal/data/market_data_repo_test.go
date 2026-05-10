package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// setupMarketDataDB creates an in-memory SQLite database with the market_data
// schema for repository tests.
func setupMarketDataDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE market_data (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol    TEXT    NOT NULL,
			price     TEXT    NOT NULL,
			currency  TEXT    NOT NULL,
			data_type TEXT    NOT NULL,
			source    TEXT    NOT NULL DEFAULT 'yahoo',
			date      TEXT    NOT NULL DEFAULT '',
			fetched_at TEXT   NOT NULL DEFAULT (datetime('now')),
			created_at TEXT   NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT   NOT NULL DEFAULT (datetime('now')),
			UNIQUE(symbol, source, date)
		);

		CREATE INDEX idx_market_data_symbol ON market_data(symbol);
		CREATE INDEX idx_market_data_symbol_date ON market_data(symbol, date);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func TestMarketDataRepository_UpsertAndGetLatest(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	price, _ := decimal.NewFromFloat64(185.50)
	md := &market.MarketData{
		Symbol:    "AAPL",
		Price:     price,
		Currency:  "USD",
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "",
		FetchedAt: time.Now(),
	}

	err := repo.Upsert(context.Background(), md)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetLatest(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want %q", got.Symbol, "AAPL")
	}
	if got.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", got.Currency, "USD")
	}
	if got.DataType != "stock" {
		t.Errorf("DataType = %q, want %q", got.DataType, "stock")
	}
	if !got.Price.Equal(price) {
		t.Errorf("Price = %v, want %v", got.Price, price)
	}
}

func TestMarketDataRepository_GetLatest_NotFound(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	got, err := repo.GetLatest(context.Background(), "NONEXISTENT")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent symbol, got %+v", got)
	}
}

func TestMarketDataRepository_Upsert_OverwritesExisting(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert initial price
	price1, _ := decimal.NewFromFloat64(180.00)
	md1 := &market.MarketData{
		Symbol:    "AAPL",
		Price:     price1,
		Currency:  "USD",
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "",
		FetchedAt: time.Now(),
	}
	repo.Upsert(context.Background(), md1)

	// Upsert with new price (same symbol/source/date)
	price2, _ := decimal.NewFromFloat64(185.50)
	md2 := &market.MarketData{
		Symbol:    "AAPL",
		Price:     price2,
		Currency:  "USD",
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "",
		FetchedAt: time.Now(),
	}
	repo.Upsert(context.Background(), md2)

	got, err := repo.GetLatest(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if !got.Price.Equal(price2) {
		t.Errorf("Price = %v, want %v (expected upsert to overwrite)", got.Price, price2)
	}
}

func TestMarketDataRepository_GetBySourceAndDate(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	price, _ := decimal.NewFromFloat64(1.2734)
	md := &market.MarketData{
		Symbol:    "GBP/USD",
		Price:     price,
		Currency:  "USD",
		DataType:  "fx",
		Source:    "yahoo",
		Date:      "2026-05-08",
		FetchedAt: time.Now(),
	}
	repo.Upsert(context.Background(), md)

	got, err := repo.GetBySourceAndDate(context.Background(), "GBP/USD", "yahoo", "2026-05-08")
	if err != nil {
		t.Fatalf("GetBySourceAndDate: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Symbol != "GBP/USD" {
		t.Errorf("Symbol = %q, want %q", got.Symbol, "GBP/USD")
	}
	if !got.Price.Equal(price) {
		t.Errorf("Price = %v, want %v", got.Price, price)
	}
}

func TestMarketDataRepository_GetBySourceAndDate_NotFound(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	got, err := repo.GetBySourceAndDate(context.Background(), "GBP/USD", "yahoo", "2026-01-01")
	if err != nil {
		t.Fatalf("GetBySourceAndDate: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent entry, got %+v", got)
	}
}

func TestMarketDataRepository_GetCurrentFxRate(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	rate, _ := decimal.NewFromFloat64(1.2734)
	md := &market.MarketData{
		Symbol:    "GBP/USD",
		Price:     rate,
		Currency:  "USD",
		DataType:  "fx",
		Source:    "yahoo",
		Date:      "", // current (latest)
		FetchedAt: time.Now(),
	}
	repo.Upsert(context.Background(), md)

	got, err := repo.GetCurrentFxRate(context.Background(), "GBP", "USD")
	if err != nil {
		t.Fatalf("GetCurrentFxRate: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Symbol != "GBP/USD" {
		t.Errorf("Symbol = %q, want %q", got.Symbol, "GBP/USD")
	}
	if got.DataType != "fx" {
		t.Errorf("DataType = %q, want %q", got.DataType, "fx")
	}
	if !got.Price.Equal(rate) {
		t.Errorf("Price = %v, want %v", got.Price, rate)
	}
}

func TestMarketDataRepository_GetCurrentFxRate_NotFound(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	got, err := repo.GetCurrentFxRate(context.Background(), "GBP", "USD")
	if err != nil {
		t.Fatalf("GetCurrentFxRate: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent FX rate, got %+v", got)
	}
}

func TestMarketDataRepository_MultipleSources(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert same symbol with different sources
	for _, source := range []string{"yahoo", "custom"} {
		price, _ := decimal.NewFromFloat64(180.00)
		if source == "custom" {
			price2, _ := decimal.NewFromFloat64(181.00)
			price = price2
		}
		md := &market.MarketData{
			Symbol:    "AAPL",
			Price:     price,
			Currency:  "USD",
			DataType:  "stock",
			Source:    source,
			Date:      "",
			FetchedAt: time.Now(),
		}
		repo.Upsert(context.Background(), md)
	}

	// GetBySourceAndDate should distinguish sources
	got, err := repo.GetBySourceAndDate(context.Background(), "AAPL", "yahoo", "")
	if err != nil {
		t.Fatalf("GetBySourceAndDate(yahoo): %v", err)
	}
	priceYahoo, _ := decimal.NewFromFloat64(180.00)
	if !got.Price.Equal(priceYahoo) {
		t.Errorf("yahoo price = %v, want %v", got.Price, priceYahoo)
	}

	got, err = repo.GetBySourceAndDate(context.Background(), "AAPL", "custom", "")
	if err != nil {
		t.Fatalf("GetBySourceAndDate(custom): %v", err)
	}
	priceCustom, _ := decimal.NewFromFloat64(181.00)
	if !got.Price.Equal(priceCustom) {
		t.Errorf("custom price = %v, want %v", got.Price, priceCustom)
	}
}

func TestMarketDataRepository_DeleteStaleMarketData(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now()

	// Manually insert with old fetched_at to simulate stale data
	price1, _ := decimal.NewFromFloat64(180.00)
	db.Exec(`INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"AAPL", price1.String(), "USD", "stock", "yahoo", "", oldTime.Format(time.RFC3339))

	// Delete stale entries older than 1 hour ago
	threshold := time.Now().Add(-1 * time.Hour)
	rows, err := repo.DeleteStaleMarketData(context.Background(), "AAPL", threshold)
	if err != nil {
		t.Fatalf("DeleteStaleMarketData: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row deleted, got %d", rows)
	}

	// Insert a fresh entry
	price2, _ := decimal.NewFromFloat64(185.00)
	md2 := &market.MarketData{
		Symbol:    "AAPL",
		Price:     price2,
		Currency:  "USD",
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "",
		FetchedAt: newTime,
	}
	repo.Upsert(context.Background(), md2)

	// Verify fresh entry survives
	got, err := repo.GetLatest(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result after fresh upsert")
	}
	if !got.Price.Equal(price2) {
		t.Errorf("Price = %v, want %v", got.Price, price2)
	}
}

func TestMarketDataRepository_UpsertHistoricalPrices(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	prices := []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17000, 2), Currency: "USD"},
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17500, 2), Currency: "USD"},
		{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(18000, 2), Currency: "USD"},
	}

	err := repo.UpsertHistoricalPrices(context.Background(), "AAPL", prices, "stock")
	if err != nil {
		t.Fatalf("UpsertHistoricalPrices: %v", err)
	}

	// Verify each date was stored.
	for i, wantPrice := range prices {
		got, err := repo.GetBySourceAndDate(context.Background(), "AAPL", "yahoo", wantPrice.Date.Format("2006-01-02"))
		if err != nil {
			t.Fatalf("GetBySourceAndDate for date %d: %v", i, err)
		}
		if got == nil {
			t.Fatalf("expected non-nil result for date %s", wantPrice.Date.Format("2006-01-02"))
		}
		if !got.Price.Equal(wantPrice.Close) {
			t.Errorf("date %s: Price = %v, want %v", wantPrice.Date.Format("2006-01-02"), got.Price, wantPrice.Close)
		}
		if got.Currency != "USD" {
			t.Errorf("date %s: Currency = %q, want %q", wantPrice.Date.Format("2006-01-02"), got.Currency, "USD")
		}
		if got.DataType != "stock" {
			t.Errorf("date %s: DataType = %q, want %q", wantPrice.Date.Format("2006-01-02"), got.DataType, "stock")
		}
	}
}

func TestMarketDataRepository_UpsertHistoricalPrices_Empty(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	err := repo.UpsertHistoricalPrices(context.Background(), "AAPL", nil, "stock")
	if err != nil {
		t.Fatalf("UpsertHistoricalPrices with nil: %v", err)
	}

	err = repo.UpsertHistoricalPrices(context.Background(), "AAPL", []market.HistoricalPrice{}, "stock")
	if err != nil {
		t.Fatalf("UpsertHistoricalPrices with empty slice: %v", err)
	}
}

func TestMarketDataRepository_UpsertHistoricalPrices_OverwritesExisting(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert initial price for a date.
	initialPrice, _ := decimal.NewFromFloat64(170.00)
	md := &market.MarketData{
		Symbol:    "AAPL",
		Price:     initialPrice,
		Currency:  "USD",
		DataType:  "stock",
		Source:    "yahoo",
		Date:      "2026-05-08",
		FetchedAt: time.Now(),
	}
	repo.Upsert(context.Background(), md)

	// Upsert historical prices including the same date with a new price.
	prices := []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(18000, 2), Currency: "USD"},
	}
	err := repo.UpsertHistoricalPrices(context.Background(), "AAPL", prices, "stock")
	if err != nil {
		t.Fatalf("UpsertHistoricalPrices: %v", err)
	}

	got, err := repo.GetBySourceAndDate(context.Background(), "AAPL", "yahoo", "2026-05-08")
	if err != nil {
		t.Fatalf("GetBySourceAndDate: %v", err)
	}
	wantPrice, _ := decimal.NewFromFloat64(180.00)
	if !got.Price.Equal(wantPrice) {
		t.Errorf("Price = %v, want %v (expected upsert to overwrite)", got.Price, wantPrice)
	}
}

func TestMarketDataRepository_GetHistoricalPricesBySymbol(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert historical prices for AAPL.
	prices := []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17000, 2), Currency: "USD"},
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17500, 2), Currency: "USD"},
		{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(18000, 2), Currency: "USD"},
	}
	err := repo.UpsertHistoricalPrices(context.Background(), "AAPL", prices, "stock")
	if err != nil {
		t.Fatalf("UpsertHistoricalPrices: %v", err)
	}

	// Query the full range.
	got, err := repo.GetHistoricalPricesBySymbol(context.Background(), "AAPL",
		time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 prices, got %d", len(got))
	}
	for i, want := range prices {
		if !got[i].Close.Equal(want.Close) {
			t.Errorf("price[%d] = %v, want %v", i, got[i].Close, want.Close)
		}
		if !got[i].Date.Equal(want.Date) {
			t.Errorf("date[%d] = %v, want %v", i, got[i].Date, want.Date)
		}
		if got[i].Currency != want.Currency {
			t.Errorf("currency[%d] = %q, want %q", i, got[i].Currency, want.Currency)
		}
	}
}

func TestMarketDataRepository_GetHistoricalPricesBySymbol_PartialRange(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert 3 days of prices.
	prices := []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17000, 2), Currency: "USD"},
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17500, 2), Currency: "USD"},
		{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(18000, 2), Currency: "USD"},
	}
	repo.UpsertHistoricalPrices(context.Background(), "AAPL", prices, "stock")

	// Query only the middle day.
	got, err := repo.GetHistoricalPricesBySymbol(context.Background(), "AAPL",
		time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 price, got %d", len(got))
	}
	if !got[0].Close.Equal(decimal.MustNew(17500, 2)) {
		t.Errorf("price = %v, want %v", got[0].Close, decimal.MustNew(17500, 2))
	}
}

func TestMarketDataRepository_GetHistoricalPricesBySymbol_NoData(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	got, err := repo.GetHistoricalPricesBySymbol(context.Background(), "NONEXISTENT",
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 prices, got %d", len(got))
	}
}

func TestMarketDataRepository_GetHistoricalPricesBySymbol_ExcludesCurrent(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert a historical price and a current (date="") quote.
	histPrice := []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17500, 2), Currency: "USD"},
	}
	repo.UpsertHistoricalPrices(context.Background(), "AAPL", histPrice, "stock")

	currentPrice, _ := decimal.NewFromFloat64(185.00)
	repo.Upsert(context.Background(), &market.MarketData{
		Symbol:   "AAPL",
		Price:    currentPrice,
		Currency: "USD",
		DataType: "stock",
		Source:   "yahoo",
		Date:     "", // current
		FetchedAt: time.Now(),
	})

	got, err := repo.GetHistoricalPricesBySymbol(context.Background(), "AAPL",
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	// Should only return the historical price, not the current quote.
	if len(got) != 1 {
		t.Fatalf("expected 1 price (current quote excluded), got %d", len(got))
	}
}

func TestMarketDataRepository_GetLatestQuotesBatch(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert current quotes for two symbols.
	for _, sym := range []string{"AAPL", "MSFT"} {
		price, _ := decimal.NewFromFloat64(185.00)
		if sym == "MSFT" {
			price, _ = decimal.NewFromFloat64(420.00)
		}
		repo.Upsert(context.Background(), &market.MarketData{
			Symbol:   sym,
			Price:    price,
			Currency: "USD",
			DataType: "stock",
			Source:   "yahoo",
			Date:     "",
			FetchedAt: time.Now(),
		})
	}

	got := repo.GetLatestQuotesBatch(context.Background(), []string{"AAPL", "MSFT", "GOOG"})
	if len(got) != 2 {
		t.Fatalf("expected 2 quotes (GOOG missing), got %d", len(got))
	}

	wantAAPL, _ := decimal.NewFromFloat64(185.00)
	if !got["AAPL"].Price.Equal(wantAAPL) {
		t.Errorf("AAPL price = %v, want %v", got["AAPL"].Price, wantAAPL)
	}

	wantMSFT, _ := decimal.NewFromFloat64(420.00)
	if !got["MSFT"].Price.Equal(wantMSFT) {
		t.Errorf("MSFT price = %v, want %v", got["MSFT"].Price, wantMSFT)
	}

	if _, ok := got["GOOG"]; ok {
		t.Error("GOOG should be omitted (no cached data)")
	}
}

func TestMarketDataRepository_GetLatestQuotesBatch_Empty(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	got := repo.GetLatestQuotesBatch(context.Background(), []string{})
	if len(got) != 0 {
		t.Errorf("expected 0 quotes, got %d", len(got))
	}

	got = repo.GetLatestQuotesBatch(context.Background(), nil)
	if len(got) != 0 {
		t.Errorf("expected 0 quotes for nil, got %d", len(got))
	}
}

func TestMarketDataRepository_GetLatestPriceDatePerSymbol(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	// Insert historical prices for two symbols.
	repo.UpsertHistoricalPrices(context.Background(), "AAPL", []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(17000, 2), Currency: "USD"},
		{Date: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(18000, 2), Currency: "USD"},
	}, "stock")
	repo.UpsertHistoricalPrices(context.Background(), "MSFT", []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), Close: decimal.MustNew(41000, 2), Currency: "USD"},
	}, "stock")

	got := repo.GetLatestPriceDatePerSymbol(context.Background(), []string{"AAPL", "MSFT", "GOOG"})
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (GOOG missing), got %d", len(got))
	}

	wantAAPL := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	if !got["AAPL"].Equal(wantAAPL) {
		t.Errorf("AAPL latest = %v, want %v", *got["AAPL"], wantAAPL)
	}

	wantMSFT := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	if !got["MSFT"].Equal(wantMSFT) {
		t.Errorf("MSFT latest = %v, want %v", *got["MSFT"], wantMSFT)
	}

	if _, ok := got["GOOG"]; ok {
		t.Error("GOOG should be omitted (no cached data)")
	}
}

func TestMarketDataRepository_GetLatestPriceDatePerSymbol_Empty(t *testing.T) {
	db := setupMarketDataDB(t)
	repo := NewMarketDataRepository(db)

	got := repo.GetLatestPriceDatePerSymbol(context.Background(), []string{"NONEXISTENT"})
	if len(got) != 0 {
		t.Errorf("expected 0 entries, got %d", len(got))
	}
}
