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
			data_source_url     TEXT,
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
			sector              TEXT,
			top_holdings        TEXT,
			sector_weightings   TEXT,
			aggregate_positions TEXT,
			fund_profile        TEXT,
			equity_valuation         TEXT,
			geographic_allocations   TEXT,
			market_cap_breakdown     TEXT,
			themes                   TEXT,
			extractor_as_of_date     TEXT,
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
			{Symbol: "BE", Name: "Bloom Energy Corp", Percent: 1.4},
			{Symbol: "TSLA", Name: "Tesla Inc", Percent: 2.5},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 21.2},
			{Sector: "industrials", Percent: 36.4},
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
	if got.TopHoldings[0].Percent != 1.4 {
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
	if got.GeographicAllocations != nil {
		t.Error("expected nil GeographicAllocations")
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

	// List stale — should return empty because query starts from symbol_mappings
	// (no mapping for NO_MAPPING means it won't appear)
	staleList, err := repo.ListStale(ctx, now)
	if err != nil {
		t.Fatalf("ListStale: %v", err)
	}
	if len(staleList) != 0 {
		t.Errorf("expected 0 stale (no mapping), got %d", len(staleList))
	}
}

func TestSymbolDetailsRepository_ListStale_IncludesMissing(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	ctx := context.Background()
	now := time.Now()

	// Insert symbol mappings — one with details, one without
	_, err := db.Exec(`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol) VALUES ('AAPL', 'AAPL'), ('MSFT', 'MSFT'), ('WMGG.L', 'WMGG.L')`)
	if err != nil {
		t.Fatalf("insert symbol mappings: %v", err)
	}

	// AAPL has stale details (8 days ago)
	stale := &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		FetchedAt:      now.Add(-8 * 24 * time.Hour),
	}
	repo.Upsert(ctx, stale)

	// WMGG.L has fresh details (2 days ago)
	fresh := &symbol.SymbolDetails{
		InternalSymbol: "WMGG.L",
		ShortName:      "WisdomTree Megatrends",
		FetchedAt:      now.Add(-2 * 24 * time.Hour),
	}
	repo.Upsert(ctx, fresh)

	// MSFT has a mapping but NO details row

	// List stale (older than 7 days) — should include AAPL (stale) and MSFT (missing)
	threshold := now.Add(-7 * 24 * time.Hour)
	staleList, err := repo.ListStale(ctx, threshold)
	if err != nil {
		t.Fatalf("ListStale: %v", err)
	}
	if len(staleList) != 2 {
		t.Fatalf("expected 2 symbols (1 stale + 1 missing), got %d", len(staleList))
	}

	// Verify AAPL (stale) and MSFT (missing) are present
	found := make(map[string]symbol.StaleSymbol)
	for _, s := range staleList {
		found[s.InternalSymbol] = s
	}

	aapl, ok := found["AAPL"]
	if !ok {
		t.Error("expected AAPL in stale list")
	} else if aapl.MarketDataSymbol != "AAPL" {
		t.Errorf("expected AAPL market_data_symbol, got %q", aapl.MarketDataSymbol)
	}

	msft, ok := found["MSFT"]
	if !ok {
		t.Error("expected MSFT in stale list (missing details)")
	} else if msft.FetchedAt.IsZero() {
		// FetchedAt should be zero for missing details
	} else {
		t.Errorf("expected zero FetchedAt for missing MSFT, got %v", msft.FetchedAt)
	}

	// WMGG.L should NOT be in the list (fresh)
	if _, ok := found["WMGG.L"]; ok {
		t.Error("WMGG.L should not be in stale list (fresh)")
	}
}

func TestSymbolDetailsRepository_GeographicAllocations_MultiElement(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "VWRP.L",
		ShortName:      "iShares Global Clean Energy",
		QuoteType:      "ETF",
		FetchedAt:      now,
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 35},
			{Country: "China", Percent: 22},
			{Country: "Denmark", Percent: 10},
		},
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VWRP.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if len(got.GeographicAllocations) != 3 {
		t.Fatalf("expected 3 geographic allocations, got %d", len(got.GeographicAllocations))
	}
	if got.GeographicAllocations[0].Country != "United States" {
		t.Errorf("expected first country 'United States', got %q", got.GeographicAllocations[0].Country)
	}
	if got.GeographicAllocations[0].Percent != 35 {
		t.Errorf("expected first percent 0.35, got %f", got.GeographicAllocations[0].Percent)
	}
	if got.GeographicAllocations[1].Country != "China" {
		t.Errorf("expected second country 'China', got %q", got.GeographicAllocations[1].Country)
	}
	if got.GeographicAllocations[2].Country != "Denmark" {
		t.Errorf("expected third country 'Denmark', got %q", got.GeographicAllocations[2].Country)
	}
}

func TestSymbolDetailsRepository_GeographicAllocations_SingleElement(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		QuoteType:      "EQUITY",
		FetchedAt:      now,
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 100},
		},
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if len(got.GeographicAllocations) != 1 {
		t.Fatalf("expected 1 geographic allocation, got %d", len(got.GeographicAllocations))
	}
	if got.GeographicAllocations[0].Country != "United States" {
		t.Errorf("expected country 'United States', got %q", got.GeographicAllocations[0].Country)
	}
	if got.GeographicAllocations[0].Percent != 100 {
		t.Errorf("expected percent 1.0, got %f", got.GeographicAllocations[0].Percent)
	}
}

func TestSymbolDetailsRepository_GeographicAllocations_NilStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "NOGEO",
		ShortName:      "No Geo Data",
		QuoteType:      "EQUITY",
		FetchedAt:      now,
		// GeographicAllocations not set (nil)
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "NOGEO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.GeographicAllocations != nil {
		t.Errorf("expected nil GeographicAllocations, got %d items", len(got.GeographicAllocations))
	}
}

func TestSymbolDetailsRepository_GeographicAllocations_EmptySliceStoredAsEmpty(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()
	details := &symbol.SymbolDetails{
		InternalSymbol: "EMPTYGEO",
		ShortName:      "Empty Geo",
		QuoteType:      "EQUITY",
		FetchedAt:      now,
		GeographicAllocations: []symbol.GeographicAllocation{},
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "EMPTYGEO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	// Empty slice marshals to "[]" which is valid JSON and unmarshals back to empty slice
	if len(got.GeographicAllocations) != 0 {
		t.Errorf("expected empty GeographicAllocations, got %d items", len(got.GeographicAllocations))
	}
}

func TestSymbolDetailsRepository_GeographicAllocations_Overwrite(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	now := time.Now()

	// First upsert with one set of allocations
	details := &symbol.SymbolDetails{
		InternalSymbol: "VWRP.L",
		ShortName:      "iShares Global Clean Energy",
		FetchedAt:      now,
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 35},
		},
	}
	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert (first): %v", err)
	}

	// Second upsert with different allocations
	updated := &symbol.SymbolDetails{
		InternalSymbol: "VWRP.L",
		ShortName:      "iShares Global Clean Energy",
		FetchedAt:      now.Add(24 * time.Hour),
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 40},
			{Country: "China", Percent: 25},
		},
	}
	err = repo.Upsert(context.Background(), updated)
	if err != nil {
		t.Fatalf("Upsert (second): %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VWRP.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if len(got.GeographicAllocations) != 2 {
		t.Fatalf("expected 2 geographic allocations, got %d", len(got.GeographicAllocations))
	}
	if got.GeographicAllocations[0].Percent != 40 {
		t.Errorf("expected first percent 40, got %f", got.GeographicAllocations[0].Percent)
	}
	if got.GeographicAllocations[1].Country != "China" {
		t.Errorf("expected second country 'China', got %q", got.GeographicAllocations[1].Country)
	}
}

func TestSymbolDetailsRepository_ExtractorAsOfDate_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	asOfDate := time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC)
	details := &symbol.SymbolDetails{
		InternalSymbol:      "WMGG.L",
		ShortName:           "WisdomTree Megatrends",
		ExtractorAsOfDate:   asOfDate,
		FetchedAt:           time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.ExtractorAsOfDate.IsZero() {
		t.Error("expected non-zero ExtractorAsOfDate")
	} else if !got.ExtractorAsOfDate.Equal(asOfDate) {
		t.Errorf("expected ExtractorAsOfDate %v, got %v", asOfDate, got.ExtractorAsOfDate)
	}
}

func TestSymbolDetailsRepository_ExtractorAsOfDate_ZeroStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol:    "VOO",
		ShortName:         "Vanguard S&P 500",
		ExtractorAsOfDate: time.Time{}, // zero = not set (Yahoo data)
		FetchedAt:         time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if !got.ExtractorAsOfDate.IsZero() {
		t.Errorf("expected zero ExtractorAsOfDate for Yahoo data, got %v", got.ExtractorAsOfDate)
	}
}

func TestSymbolDetailsRepository_MarketCapBreakdown_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "WMGT",
		ShortName:      "WisdomTree Mid Cap Growth",
		MarketCapBreakdown: &symbol.MarketCapBreakdown{
			Total: 100,
			Large: 15.2,
			Mid:   68.5,
			Small: 16.3,
		},
		FetchedAt: time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "WMGT")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.MarketCapBreakdown == nil {
		t.Fatal("expected non-nil MarketCapBreakdown")
	}
	if got.MarketCapBreakdown.Total != 100 {
		t.Errorf("expected total 100, got %f", got.MarketCapBreakdown.Total)
	}
	if got.MarketCapBreakdown.Large != 15.2 {
		t.Errorf("expected large 15.2, got %f", got.MarketCapBreakdown.Large)
	}
	if got.MarketCapBreakdown.Mid != 68.5 {
		t.Errorf("expected mid 68.5, got %f", got.MarketCapBreakdown.Mid)
	}
	if got.MarketCapBreakdown.Small != 16.3 {
		t.Errorf("expected small 16.3, got %f", got.MarketCapBreakdown.Small)
	}
}

func TestSymbolDetailsRepository_MarketCapBreakdown_NilStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol:     "VOO",
		ShortName:          "Vanguard S&P 500",
		MarketCapBreakdown: nil,
		FetchedAt:          time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.MarketCapBreakdown != nil {
		t.Errorf("expected nil MarketCapBreakdown, got %+v", got.MarketCapBreakdown)
	}
}

func TestSymbolDetailsRepository_Themes_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "WMGG.L",
		ShortName:      "WisdomTree Megatrends",
		Themes: []symbol.ThemeBreakdown{
			{Name: "Technology", Percent: 42.5},
			{Name: "Consumer Discretionary", Percent: 28.3},
			{Name: "Healthcare", Percent: 15.1},
		},
		FetchedAt: time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if len(got.Themes) != 3 {
		t.Fatalf("expected 3 themes, got %d", len(got.Themes))
	}
	if got.Themes[0].Name != "Technology" || got.Themes[0].Percent != 42.5 {
		t.Errorf("expected first theme Technology 42.5, got %s %f", got.Themes[0].Name, got.Themes[0].Percent)
	}
	if got.Themes[1].Name != "Consumer Discretionary" || got.Themes[1].Percent != 28.3 {
		t.Errorf("expected second theme Consumer Discretionary 28.3, got %s %f", got.Themes[1].Name, got.Themes[1].Percent)
	}
	if got.Themes[2].Name != "Healthcare" || got.Themes[2].Percent != 15.1 {
		t.Errorf("expected third theme Healthcare 15.1, got %s %f", got.Themes[2].Name, got.Themes[2].Percent)
	}
}

func TestSymbolDetailsRepository_Themes_NilStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500",
		Themes:         nil,
		FetchedAt:      time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.Themes != nil {
		t.Errorf("expected nil Themes, got %+v", got.Themes)
	}
}

// isErrNotFound checks if the error is ErrNotFound.
func isErrNotFound(err error) bool {
	return err == ErrNotFound
}
