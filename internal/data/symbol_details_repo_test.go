package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// setupSymbolDetailsDB creates an in-memory SQLite database with the symbol
// details and symbol_mappings schema for repository tests.
func setupSymbolDetailsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	_, err = db.Exec(`
		PRAGMA foreign_keys = ON;

		CREATE TABLE symbol_mappings (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			internal_symbol     TEXT    NOT NULL UNIQUE,
			market_data_symbol  TEXT    NOT NULL,
			is_benchmark        BOOLEAN NOT NULL DEFAULT 0,
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE symbol_details (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			internal_symbol     TEXT    NOT NULL UNIQUE,
			short_name          TEXT,
			long_name           TEXT,
			exchange            TEXT,
			currency            TEXT,
			quote_type          TEXT,
			top_holdings        TEXT,
			sector_weightings   TEXT,
			aggregate_positions TEXT,
			fund_profile        TEXT,
			equity_valuation    TEXT,
			fetched_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func mustMarshalJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(b)
}

func TestSymbolDetailsRepository_UpsertAndGet(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		LongName:       "Apple Inc.",
		Exchange:       "NMS",
		Currency:       "USD",
		QuoteType:      "EQUITY",
		FetchedAt:      now,
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.InternalSymbol != "AAPL" {
		t.Errorf("expected InternalSymbol 'AAPL', got %q", got.InternalSymbol)
	}
	if got.ShortName != "Apple Inc." {
		t.Errorf("expected ShortName 'Apple Inc.', got %q", got.ShortName)
	}
	if got.Exchange != "NMS" {
		t.Errorf("expected Exchange 'NMS', got %q", got.Exchange)
	}
	if got.Currency != "USD" {
		t.Errorf("expected Currency 'USD', got %q", got.Currency)
	}
	if got.QuoteType != "EQUITY" {
		t.Errorf("expected QuoteType 'EQUITY', got %q", got.QuoteType)
	}
}

func TestSymbolDetailsRepository_UpsertWithETFData(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "WMGG.L",
		ShortName:      "WisdomTree Megatrends",
		LongName:       "WisdomTree Megatrends UCITS ETF",
		Exchange:       "LSE",
		Currency:       "GBP",
		QuoteType:      "ETF",
		TopHoldings: []symbol.TopHolding{
			{Symbol: "BE", Name: "Bloom Energy Corp", Percent: 0.014},
			{Symbol: "TSLA", Name: "Tesla Inc", Percent: 0.025},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 0.212},
			{Sector: "industrials", Percent: 0.364},
		},
		AggregatePositions: &symbol.AggregatePositions{
			Stock: 0.993,
			Cash:  0.006,
			Other: 0.001,
		},
		FundProfile: &symbol.FundProfile{
			Family:             "WisdomTree",
			LegalType:          "Exchange Traded Fund",
			TotalNetAssets:     21526.37,
			AnnualExpenseRatio: 0.004,
		},
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings: 0.035,
			PriceToBook:     0.274,
		},
		FetchedAt: now,
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.QuoteType != "ETF" {
		t.Errorf("expected QuoteType 'ETF', got %q", got.QuoteType)
	}
	if len(got.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(got.TopHoldings))
	}
	if got.TopHoldings[0].Symbol != "BE" {
		t.Errorf("expected first holding symbol 'BE', got %q", got.TopHoldings[0].Symbol)
	}
	if got.TopHoldings[0].Percent != 0.014 {
		t.Errorf("expected first holding percent 0.014, got %f", got.TopHoldings[0].Percent)
	}
	if len(got.SectorWeightings) != 2 {
		t.Fatalf("expected 2 sector weightings, got %d", len(got.SectorWeightings))
	}
	if got.AggregatePositions == nil {
		t.Fatal("expected non-nil AggregatePositions")
	}
	if got.AggregatePositions.Stock != 0.993 {
		t.Errorf("expected stock position 0.993, got %f", got.AggregatePositions.Stock)
	}
	if got.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if got.FundProfile.Family != "WisdomTree" {
		t.Errorf("expected family 'WisdomTree', got %q", got.FundProfile.Family)
	}
	if got.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
}

func TestSymbolDetailsRepository_UpsertOverwrites(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		Exchange:       "NMS",
		FetchedAt:      now,
	}
	repo.Upsert(context.Background(), details)

	// Upsert again with different data
	updated := &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple",
		Exchange:       "NASDAQ",
		Currency:       "USD",
		FetchedAt:      now.Add(24 * time.Hour),
	}
	err := repo.Upsert(context.Background(), updated)
	if err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.ShortName != "Apple" {
		t.Errorf("expected ShortName 'Apple', got %q", got.ShortName)
	}
	if got.Exchange != "NASDAQ" {
		t.Errorf("expected Exchange 'NASDAQ', got %q", got.Exchange)
	}
	if got.Currency != "USD" {
		t.Errorf("expected Currency 'USD', got %q", got.Currency)
	}
}

func TestSymbolDetailsRepository_GetByInternalSymbol_NotFound(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	_, err := repo.GetByInternalSymbol(context.Background(), "NONEXISTENT")
	if !isErrNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSymbolDetailsRepository_ListStale(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	// Also insert symbol mappings (needed for the JOIN in ListStale)
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol) VALUES ('AAPL', 'AAPL'), ('WMGG.L', 'WMGG.L')`)
	if err != nil {
		t.Fatalf("insert symbol mappings: %v", err)
	}

	now := time.Now()

	// Insert stale details (8 days ago)
	stale := &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		FetchedAt:      now.Add(-8 * 24 * time.Hour),
	}
	repo.Upsert(ctx, stale)

	// Insert fresh details (2 days ago)
	fresh := &symbol.SymbolDetails{
		InternalSymbol: "WMGG.L",
		ShortName:      "WisdomTree Megatrends",
		FetchedAt:      now.Add(-2 * 24 * time.Hour),
	}
	repo.Upsert(ctx, fresh)

	// List stale (older than 7 days)
	threshold := now.Add(-7 * 24 * time.Hour)
	staleList, err := repo.ListStale(ctx, threshold)
	if err != nil {
		t.Fatalf("ListStale: %v", err)
	}
	if len(staleList) != 1 {
		t.Fatalf("expected 1 stale symbol, got %d", len(staleList))
	}
	if staleList[0].InternalSymbol != "AAPL" {
		t.Errorf("expected stale symbol 'AAPL', got %q", staleList[0].InternalSymbol)
	}
	if staleList[0].MarketDataSymbol != "AAPL" {
		t.Errorf("expected market data symbol 'AAPL', got %q", staleList[0].MarketDataSymbol)
	}
}

func TestSymbolDetailsRepository_ListStale_None(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol) VALUES ('AAPL', 'AAPL')`)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		FetchedAt:      now,
	}
	repo.Upsert(ctx, details)

	// Threshold after fetched_at — nothing should be stale
	// (fetched_at < threshold: items fetched BEFORE the cutoff are stale)
	// Since fetched_at = now and threshold = now + 1h, now < now+1h is true.
	// So we need threshold BEFORE fetched_at:
	staleList, err := repo.ListStale(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("ListStale: %v", err)
	}
	if len(staleList) != 0 {
		t.Errorf("expected 0 stale symbols, got %d", len(staleList))
	}
}

func TestSymbolDetailsRepository_EmptyFieldsStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "XYZ",
		FetchedAt:      now,
		// All other fields empty
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "XYZ")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.ShortName != "" {
		t.Errorf("expected empty ShortName, got %q", got.ShortName)
	}
	if got.Exchange != "" {
		t.Errorf("expected empty Exchange, got %q", got.Exchange)
	}
	if len(got.TopHoldings) != 0 {
		t.Errorf("expected empty TopHoldings, got %d items", len(got.TopHoldings))
	}
	if got.AggregatePositions != nil {
		t.Error("expected nil AggregatePositions")
	}
	if got.FundProfile != nil {
		t.Error("expected nil FundProfile")
	}
}

func TestSymbolDetailsRepository_ListStale_NoMatchingMapping(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	ctx := context.Background()
	now := time.Now()

	// Insert symbol details without a corresponding symbol mapping
	details := &symbol.SymbolDetails{
		InternalSymbol: "NO_MAPPING",
		ShortName:      "No Mapping Symbol",
		FetchedAt:      now.Add(-30 * 24 * time.Hour),
	}
	repo.Upsert(ctx, details)

	// List stale — should return empty because the JOIN with symbol_mappings fails
	staleList, err := repo.ListStale(ctx, now)
	if err != nil {
		t.Fatalf("ListStale: %v", err)
	}
	if len(staleList) != 0 {
		t.Errorf("expected 0 stale (no mapping), got %d", len(staleList))
	}
}

// isErrNotFound checks if the error is ErrNotFound.
func isErrNotFound(err error) bool {
	return err == ErrNotFound
}
