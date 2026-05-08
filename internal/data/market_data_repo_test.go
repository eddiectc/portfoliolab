package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/market"
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

	got, err := repo.GetCurrentFxRate(context.Background(), "GBP/USD")
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

	got, err := repo.GetCurrentFxRate(context.Background(), "GBP/USD")
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
