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
			risk_measures            TEXT,
			asset_class_allocation   TEXT,
			equity_derivatives_by_region TEXT,
			currency_derivatives_allocation TEXT,
			bond_characteristics      TEXT,
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
		InternalSymbol:        "EMPTYGEO",
		ShortName:             "Empty Geo",
		QuoteType:             "EQUITY",
		FetchedAt:             now,
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
		InternalSymbol:    "WMGG.L",
		ShortName:         "WisdomTree Megatrends",
		ExtractorAsOfDate: asOfDate,
		FetchedAt:         time.Now(),
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

func TestSymbolDetailsRepository_RiskMeasures_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "IMGPFUND",
		ShortName:      "iMGP Fund",
		RiskMeasures: &symbol.RiskMeasures{
			Volatility:    9.16,
			SharpeRatio:   2.52,
			InfoRatio:     0.35,
			Beta:          0.82,
			Correlation:   0.91,
			TrackingError: 3.45,
			FieldsPresent: symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio | symbol.SymbolRiskFieldInfoRatio | symbol.SymbolRiskFieldBeta | symbol.SymbolRiskFieldCorrelation | symbol.SymbolRiskFieldTrackingError,
		},
		FetchedAt: time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "IMGPFUND")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.RiskMeasures == nil {
		t.Fatal("expected non-nil RiskMeasures")
	}
	if got.RiskMeasures.Volatility != 9.16 {
		t.Errorf("expected Volatility 9.16, got %f", got.RiskMeasures.Volatility)
	}
	if got.RiskMeasures.SharpeRatio != 2.52 {
		t.Errorf("expected SharpeRatio 2.52, got %f", got.RiskMeasures.SharpeRatio)
	}
	if got.RiskMeasures.InfoRatio != 0.35 {
		t.Errorf("expected InfoRatio 0.35, got %f", got.RiskMeasures.InfoRatio)
	}
	if got.RiskMeasures.Beta != 0.82 {
		t.Errorf("expected Beta 0.82, got %f", got.RiskMeasures.Beta)
	}
	if got.RiskMeasures.Correlation != 0.91 {
		t.Errorf("expected Correlation 0.91, got %f", got.RiskMeasures.Correlation)
	}
	if got.RiskMeasures.TrackingError != 3.45 {
		t.Errorf("expected TrackingError 3.45, got %f", got.RiskMeasures.TrackingError)
	}
	expectedMask := symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio | symbol.SymbolRiskFieldInfoRatio | symbol.SymbolRiskFieldBeta | symbol.SymbolRiskFieldCorrelation | symbol.SymbolRiskFieldTrackingError
	if got.RiskMeasures.FieldsPresent != expectedMask {
		t.Errorf("expected FieldsPresent %v, got %v", expectedMask, got.RiskMeasures.FieldsPresent)
	}
}

func TestSymbolDetailsRepository_RiskMeasures_NilStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500",
		RiskMeasures:   nil,
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
	if got.RiskMeasures != nil {
		t.Errorf("expected nil RiskMeasures, got %+v", got.RiskMeasures)
	}
}

func TestSymbolDetailsRepository_AssetClassAllocation_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "IMGPFUND",
		ShortName:      "iMGP Fund",
		AssetClassAllocation: []symbol.AssetClassEntry{
			{AssetClass: "Equities", Percent: 85.2},
			{AssetClass: "Bonds", Percent: 5.3},
			{AssetClass: "Gold", Percent: -2.1},
			{AssetClass: "Cash", Percent: 11.6},
		},
		FetchedAt: time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "IMGPFUND")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if len(got.AssetClassAllocation) != 4 {
		t.Fatalf("expected 4 asset class entries, got %d", len(got.AssetClassAllocation))
	}
	if got.AssetClassAllocation[0].AssetClass != "Equities" || got.AssetClassAllocation[0].Percent != 85.2 {
		t.Errorf("expected Equities 85.2, got %s %f", got.AssetClassAllocation[0].AssetClass, got.AssetClassAllocation[0].Percent)
	}
	if got.AssetClassAllocation[2].AssetClass != "Gold" || got.AssetClassAllocation[2].Percent != -2.1 {
		t.Errorf("expected Gold -2.1, got %s %f", got.AssetClassAllocation[2].AssetClass, got.AssetClassAllocation[2].Percent)
	}
}

func TestSymbolDetailsRepository_AssetClassAllocation_NilStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol:       "VOO",
		ShortName:            "Vanguard S&P 500",
		AssetClassAllocation: nil,
		FetchedAt:            time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.AssetClassAllocation != nil {
		t.Errorf("expected nil AssetClassAllocation, got %+v", got.AssetClassAllocation)
	}
}

func TestSymbolDetailsRepository_EquityDerivativesByRegion_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "IMGPFUND",
		ShortName:      "iMGP Fund",
		EquityDerivativesByRegion: []symbol.RegionDerivativeEntry{
			{Region: "North America", Percent: 52.3},
			{Region: "Europe", Percent: 31.7},
			{Region: "Asia", Percent: 12.0},
			{Region: "Emerging Countries", Percent: 4.0},
		},
		FetchedAt: time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "IMGPFUND")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if len(got.EquityDerivativesByRegion) != 4 {
		t.Fatalf("expected 4 region entries, got %d", len(got.EquityDerivativesByRegion))
	}
	if got.EquityDerivativesByRegion[0].Region != "North America" || got.EquityDerivativesByRegion[0].Percent != 52.3 {
		t.Errorf("expected North America 52.3, got %s %f", got.EquityDerivativesByRegion[0].Region, got.EquityDerivativesByRegion[0].Percent)
	}
	if got.EquityDerivativesByRegion[3].Region != "Emerging Countries" || got.EquityDerivativesByRegion[3].Percent != 4.0 {
		t.Errorf("expected Emerging Countries 4.0, got %s %f", got.EquityDerivativesByRegion[3].Region, got.EquityDerivativesByRegion[3].Percent)
	}
}

func TestSymbolDetailsRepository_EquityDerivativesByRegion_NilStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol:            "VOO",
		ShortName:                 "Vanguard S&P 500",
		EquityDerivativesByRegion: nil,
		FetchedAt:                 time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.EquityDerivativesByRegion != nil {
		t.Errorf("expected nil EquityDerivativesByRegion, got %+v", got.EquityDerivativesByRegion)
	}
}

func TestSymbolDetailsRepository_CurrencyDerivativesAllocation_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "IMGPFUND",
		ShortName:      "iMGP Fund",
		CurrencyDerivativesAllocation: []symbol.CurrencyDerivativeEntry{
			{Currency: "USD", Percent: 65.0},
			{Currency: "EUR", Percent: 20.0},
			{Currency: "JPY", Percent: 15.0},
		},
		FetchedAt: time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "IMGPFUND")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if len(got.CurrencyDerivativesAllocation) != 3 {
		t.Fatalf("expected 3 currency entries, got %d", len(got.CurrencyDerivativesAllocation))
	}
	if got.CurrencyDerivativesAllocation[0].Currency != "USD" || got.CurrencyDerivativesAllocation[0].Percent != 65.0 {
		t.Errorf("expected USD 65.0, got %s %f", got.CurrencyDerivativesAllocation[0].Currency, got.CurrencyDerivativesAllocation[0].Percent)
	}
	if got.CurrencyDerivativesAllocation[2].Currency != "JPY" || got.CurrencyDerivativesAllocation[2].Percent != 15.0 {
		t.Errorf("expected JPY 15.0, got %s %f", got.CurrencyDerivativesAllocation[2].Currency, got.CurrencyDerivativesAllocation[2].Percent)
	}
}

func TestSymbolDetailsRepository_VanguardFields_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	couponRate := 3.25
	finalMaturity := "2035-06-15"

	details := &symbol.SymbolDetails{
		InternalSymbol: "VWRL.L",
		ShortName:      "Vanguard FTSE All-World",
		LongName:       "Vanguard FTSE All-World UCITS ETF USD Distributing",
		Exchange:       "LSE",
		Currency:       "GBP",
		QuoteType:      "ETF",
		FetchedAt:      time.Now(),
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 3.8, SecurityType: "Common Stock", AsOfDate: "2024-03-29"},
			{Symbol: "US09200A99", Name: "US Treasury Note 2.5%", Percent: 0.5, SecurityType: "Government Bond", CouponRate: &couponRate, FinalMaturity: &finalMaturity, AsOfDate: "2024-03-29"},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 22.5, Date: "2024-03-29"},
			{Sector: "financial_services", Percent: 15.3, Date: "2024-03-29"},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 58.2, RegionName: "North America", RegionCode: "NA", Date: "2024-03-29"},
			{Country: "United Kingdom", Percent: 4.1, RegionName: "Europe", RegionCode: "EU", Date: "2024-03-29"},
			{Country: "China", Percent: 3.0, RegionName: "Asia Pacific", RegionCode: "AP", Date: "2024-03-29"},
		},
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings:  21.5,
			PriceToBook:      4.2,
			MedianMarketCap:  1850.0,
			ForwardROE:       18.3,
			ForwardEPSGrowth: 12.1,
			RevenueRatio:     1.05,
		},
		BondCharacteristics: &symbol.BondCharacteristics{
			AverageCoupon:   3.15,
			AverageMaturity: 7.5,
			AverageQuality:  6.8,
			AverageDuration: 6.2,
		},
		ExtractorAsOfDate: time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VWRL.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}

	// Verify holdings with new fields
	if len(got.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(got.TopHoldings))
	}
	if got.TopHoldings[0].SecurityType != "Common Stock" {
		t.Errorf("expected SecurityType 'Common Stock', got %q", got.TopHoldings[0].SecurityType)
	}
	if got.TopHoldings[0].AsOfDate != "2024-03-29" {
		t.Errorf("expected AsOfDate '2024-03-29', got %q", got.TopHoldings[0].AsOfDate)
	}
	if got.TopHoldings[1].SecurityType != "Government Bond" {
		t.Errorf("expected SecurityType 'Government Bond', got %q", got.TopHoldings[1].SecurityType)
	}
	if got.TopHoldings[1].CouponRate == nil || *got.TopHoldings[1].CouponRate != 3.25 {
		t.Errorf("expected CouponRate 3.25, got %v", got.TopHoldings[1].CouponRate)
	}
	if got.TopHoldings[1].FinalMaturity == nil || *got.TopHoldings[1].FinalMaturity != "2035-06-15" {
		t.Errorf("expected FinalMaturity '2035-06-15', got %v", got.TopHoldings[1].FinalMaturity)
	}

	// Verify sectors with Date
	if len(got.SectorWeightings) != 2 {
		t.Fatalf("expected 2 sectors, got %d", len(got.SectorWeightings))
	}
	if got.SectorWeightings[0].Date != "2024-03-29" {
		t.Errorf("expected sector Date '2024-03-29', got %q", got.SectorWeightings[0].Date)
	}

	// Verify countries with region fields
	if len(got.GeographicAllocations) != 3 {
		t.Fatalf("expected 3 countries, got %d", len(got.GeographicAllocations))
	}
	if got.GeographicAllocations[0].RegionName != "North America" {
		t.Errorf("expected RegionName 'North America', got %q", got.GeographicAllocations[0].RegionName)
	}
	if got.GeographicAllocations[0].RegionCode != "NA" {
		t.Errorf("expected RegionCode 'NA', got %q", got.GeographicAllocations[0].RegionCode)
	}
	if got.GeographicAllocations[0].Date != "2024-03-29" {
		t.Errorf("expected Date '2024-03-29', got %q", got.GeographicAllocations[0].Date)
	}

	// Verify expanded equity valuation
	if got.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
	if got.EquityValuation.MedianMarketCap != 1850.0 {
		t.Errorf("expected MedianMarketCap 1850.0, got %f", got.EquityValuation.MedianMarketCap)
	}
	if got.EquityValuation.ForwardROE != 18.3 {
		t.Errorf("expected ForwardROE 18.3, got %f", got.EquityValuation.ForwardROE)
	}
	if got.EquityValuation.ForwardEPSGrowth != 12.1 {
		t.Errorf("expected ForwardEPSGrowth 12.1, got %f", got.EquityValuation.ForwardEPSGrowth)
	}
	if got.EquityValuation.RevenueRatio != 1.05 {
		t.Errorf("expected RevenueRatio 1.05, got %f", got.EquityValuation.RevenueRatio)
	}

	// Verify bond characteristics
	if got.BondCharacteristics == nil {
		t.Fatal("expected non-nil BondCharacteristics")
	}
	if got.BondCharacteristics.AverageCoupon != 3.15 {
		t.Errorf("expected AverageCoupon 3.15, got %f", got.BondCharacteristics.AverageCoupon)
	}
	if got.BondCharacteristics.AverageMaturity != 7.5 {
		t.Errorf("expected AverageMaturity 7.5, got %f", got.BondCharacteristics.AverageMaturity)
	}
	if got.BondCharacteristics.AverageQuality != 6.8 {
		t.Errorf("expected AverageQuality 6.8, got %f", got.BondCharacteristics.AverageQuality)
	}
	if got.BondCharacteristics.AverageDuration != 6.2 {
		t.Errorf("expected AverageDuration 6.2, got %f", got.BondCharacteristics.AverageDuration)
	}

	// Verify ExtractorAsOfDate
	if got.ExtractorAsOfDate.IsZero() {
		t.Error("expected non-zero ExtractorAsOfDate")
	}
}

func TestSymbolDetailsRepository_BackwardCompatibility_WisdomTreeData(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	// Simulate old WisdomTree-style JSON (without new Vanguard fields)
	// Uses PascalCase keys — Go's default json.Marshal behavior (no explicit tags)
	oldHoldings := `[{"Symbol":"BE","Name":"Bloom Energy Corp","Percent":1.4},{"Symbol":"TSLA","Name":"Tesla Inc","Percent":2.5}]`
	oldSectors := `[{"Sector":"technology","Percent":21.2},{"Sector":"industrials","Percent":36.4}]`
	oldCountries := `[{"Country":"United States","Percent":45.2},{"Country":"Japan","Percent":8.1}]`
	oldEquityVal := `{"PriceToEarnings":0.035,"PriceToBook":0.274}`

	// Insert raw JSON directly (simulating old data already in DB)
	_, err := db.Exec(`
		INSERT INTO symbol_details (internal_symbol, short_name, quote_type, top_holdings, sector_weightings, geographic_allocations, equity_valuation, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, datetime('now'))
	`, "WMGG.L", "WisdomTree Megatrends", "ETF", oldHoldings, oldSectors, oldCountries, oldEquityVal)
	if err != nil {
		t.Fatalf("insert old data: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}

	// Verify old fields still deserialize correctly
	if got.ShortName != "WisdomTree Megatrends" {
		t.Errorf("expected ShortName 'WisdomTree Megatrends', got %q", got.ShortName)
	}
	if got.QuoteType != "ETF" {
		t.Errorf("expected QuoteType 'ETF', got %q", got.QuoteType)
	}

	// Holdings: old fields present, new fields empty/nil
	if len(got.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(got.TopHoldings))
	}
	if got.TopHoldings[0].Symbol != "BE" {
		t.Errorf("expected Symbol 'BE', got %q", got.TopHoldings[0].Symbol)
	}
	if got.TopHoldings[0].Percent != 1.4 {
		t.Errorf("expected Percent 1.4, got %f", got.TopHoldings[0].Percent)
	}
	if got.TopHoldings[0].SecurityType != "" {
		t.Errorf("expected empty SecurityType (backward compat), got %q", got.TopHoldings[0].SecurityType)
	}
	if got.TopHoldings[0].CouponRate != nil {
		t.Errorf("expected nil CouponRate (backward compat), got %v", got.TopHoldings[0].CouponRate)
	}

	// Sectors: old fields present, Date empty
	if len(got.SectorWeightings) != 2 {
		t.Fatalf("expected 2 sectors, got %d", len(got.SectorWeightings))
	}
	if got.SectorWeightings[0].Sector != "technology" {
		t.Errorf("expected Sector 'technology', got %q", got.SectorWeightings[0].Sector)
	}
	if got.SectorWeightings[0].Percent != 21.2 {
		t.Errorf("expected Percent 21.2, got %f", got.SectorWeightings[0].Percent)
	}
	if got.SectorWeightings[0].Date != "" {
		t.Errorf("expected empty Date (backward compat), got %q", got.SectorWeightings[0].Date)
	}

	// Countries: old fields present, region fields empty
	if len(got.GeographicAllocations) != 2 {
		t.Fatalf("expected 2 countries, got %d", len(got.GeographicAllocations))
	}
	if got.GeographicAllocations[0].Country != "United States" {
		t.Errorf("expected Country 'United States', got %q", got.GeographicAllocations[0].Country)
	}
	if got.GeographicAllocations[0].Percent != 45.2 {
		t.Errorf("expected Percent 45.2, got %f", got.GeographicAllocations[0].Percent)
	}
	if got.GeographicAllocations[0].RegionName != "" {
		t.Errorf("expected empty RegionName (backward compat), got %q", got.GeographicAllocations[0].RegionName)
	}
	if got.GeographicAllocations[0].RegionCode != "" {
		t.Errorf("expected empty RegionCode (backward compat), got %q", got.GeographicAllocations[0].RegionCode)
	}

	// EquityValuation: old fields present, new fields zero
	if got.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
	if got.EquityValuation.PriceToEarnings != 0.035 {
		t.Errorf("expected PriceToEarnings 0.035, got %f", got.EquityValuation.PriceToEarnings)
	}
	if got.EquityValuation.MedianMarketCap != 0 {
		t.Errorf("expected zero MedianMarketCap (backward compat), got %f", got.EquityValuation.MedianMarketCap)
	}

	// BondCharacteristics: not set in old data
	if got.BondCharacteristics != nil {
		t.Errorf("expected nil BondCharacteristics (backward compat), got %+v", got.BondCharacteristics)
	}
}

func TestSymbolDetailsRepository_BlackRockFields_RoundTrip(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	asOfDate := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	details := &symbol.SymbolDetails{
		InternalSymbol: "ISUS.L",
		ShortName:      "iShares Core USD Treasury Bond",
		LongName:       "iShares Core USD Treasury Bond UCITS Fund",
		Exchange:       "LSE",
		Currency:       "GBP",
		QuoteType:      "ETF",
		FetchedAt:      time.Now(),
		FundProfile: &symbol.FundProfile{
			Family:               "iShares",
			LegalType:            "UCITS",
			TotalNetAssets:       15234.56,
			AnnualExpenseRatio:   0.05,
			Isin:                 "IE00B53HZB01",
			Benchmark:            "ICE US Treasury Broad Index",
			AssetClassification:  "Fixed Income",
			DistributionStrategy: "Distributing",
			MarketRegionFocus:    "Global",
			SFDRClassification:   "Article 6",
			Domicile:             "Ireland",
			RebalanceFrequency:   "Quarterly",
			ProductStructure:     "Physical",
			Methodology:          "Representative",
			FundManager:          "BlackRock Asset Management Ireland Limited",
			Custodian:            "State Street Custodial Services (Ireland) Limited",
			IssuingCompany:       "iShares IV plc",
			BenchmarkTicker:      "IB000BM0001",
		},
		EquityValuation: &symbol.EquityValuation{
			Beta3Y:              0.85,
			StandardDeviation3Y: 4.12,
			NumberOfHoldings:    12,
		},
		TopHoldings: []symbol.TopHolding{
			{
				Symbol:         "912828D57",
				Name:           "United States Treasury Note 0.38% 20/05/2027",
				Percent:        5.23,
				Sector:         "Government",
				AssetClass:     "Bond",
				MarketValue:    125000000,
				NotionalValue:  126500000,
				ISIN:           "912828D57",
				Location:       "United States",
				Exchange:       "OTC",
				MarketCurrency: "USD",
			},
			{
				Symbol:         "912828Y37",
				Name:           "United States Treasury Note 1.88% 31/05/2026",
				Percent:        4.87,
				Sector:         "Government",
				AssetClass:     "Bond",
				MarketValue:    118000000,
				NotionalValue:  119200000,
				Shares:         115000000,
				Price:          1.0365,
				ISIN:           "912828Y37",
				Location:       "United States",
				Exchange:       "OTC",
				MarketCurrency: "USD",
			},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "government", Percent: 98.5},
			{Sector: "cash", Percent: 1.5},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 98.5},
			{Country: "Cash", Percent: 1.5},
		},
		ExtractorAsOfDate: asOfDate,
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "ISUS.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}

	// Verify FundProfile new fields
	if got.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if got.FundProfile.SFDRClassification != "Article 6" {
		t.Errorf("expected SFDRClassification 'Article 6', got %q", got.FundProfile.SFDRClassification)
	}
	if got.FundProfile.Domicile != "Ireland" {
		t.Errorf("expected Domicile 'Ireland', got %q", got.FundProfile.Domicile)
	}
	if got.FundProfile.RebalanceFrequency != "Quarterly" {
		t.Errorf("expected RebalanceFrequency 'Quarterly', got %q", got.FundProfile.RebalanceFrequency)
	}
	if got.FundProfile.ProductStructure != "Physical" {
		t.Errorf("expected ProductStructure 'Physical', got %q", got.FundProfile.ProductStructure)
	}
	if got.FundProfile.Methodology != "Representative" {
		t.Errorf("expected Methodology 'Representative', got %q", got.FundProfile.Methodology)
	}
	if got.FundProfile.FundManager != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("expected FundManager, got %q", got.FundProfile.FundManager)
	}
	if got.FundProfile.Custodian != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("expected Custodian, got %q", got.FundProfile.Custodian)
	}
	if got.FundProfile.IssuingCompany != "iShares IV plc" {
		t.Errorf("expected IssuingCompany, got %q", got.FundProfile.IssuingCompany)
	}
	if got.FundProfile.BenchmarkTicker != "IB000BM0001" {
		t.Errorf("expected BenchmarkTicker 'IB000BM0001', got %q", got.FundProfile.BenchmarkTicker)
	}

	// Verify EquityValuation new fields
	if got.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
	if got.EquityValuation.Beta3Y != 0.85 {
		t.Errorf("expected Beta3Y 0.85, got %f", got.EquityValuation.Beta3Y)
	}
	if got.EquityValuation.StandardDeviation3Y != 4.12 {
		t.Errorf("expected StandardDeviation3Y 4.12, got %f", got.EquityValuation.StandardDeviation3Y)
	}
	if got.EquityValuation.NumberOfHoldings != 12 {
		t.Errorf("expected NumberOfHoldings 12, got %d", got.EquityValuation.NumberOfHoldings)
	}

	// Verify TopHoldings new fields
	if len(got.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(got.TopHoldings))
	}
	if got.TopHoldings[0].Sector != "Government" {
		t.Errorf("expected Sector 'Government', got %q", got.TopHoldings[0].Sector)
	}
	if got.TopHoldings[0].AssetClass != "Bond" {
		t.Errorf("expected AssetClass 'Bond', got %q", got.TopHoldings[0].AssetClass)
	}
	if got.TopHoldings[0].MarketValue != 125000000 {
		t.Errorf("expected MarketValue 125000000, got %f", got.TopHoldings[0].MarketValue)
	}
	if got.TopHoldings[0].NotionalValue != 126500000 {
		t.Errorf("expected NotionalValue 126500000, got %f", got.TopHoldings[0].NotionalValue)
	}
	if got.TopHoldings[0].ISIN != "912828D57" {
		t.Errorf("expected ISIN '912828D57', got %q", got.TopHoldings[0].ISIN)
	}
	if got.TopHoldings[0].Location != "United States" {
		t.Errorf("expected Location 'United States', got %q", got.TopHoldings[0].Location)
	}
	if got.TopHoldings[0].Exchange != "OTC" {
		t.Errorf("expected Exchange 'OTC', got %q", got.TopHoldings[0].Exchange)
	}
	if got.TopHoldings[0].MarketCurrency != "USD" {
		t.Errorf("expected MarketCurrency 'USD', got %q", got.TopHoldings[0].MarketCurrency)
	}
	// Second holding with Shares and Price
	if got.TopHoldings[1].Shares != 115000000 {
		t.Errorf("expected Shares 115000000, got %f", got.TopHoldings[1].Shares)
	}
	if got.TopHoldings[1].Price != 1.0365 {
		t.Errorf("expected Price 1.0365, got %f", got.TopHoldings[1].Price)
	}

	// Verify existing fields still work
	if got.FundProfile.Family != "iShares" {
		t.Errorf("expected Family 'iShares', got %q", got.FundProfile.Family)
	}
	if got.FundProfile.Isin != "IE00B53HZB01" {
		t.Errorf("expected Isin 'IE00B53HZB01', got %q", got.FundProfile.Isin)
	}
	if got.ShortName != "iShares Core USD Treasury Bond" {
		t.Errorf("expected ShortName, got %q", got.ShortName)
	}
}

func TestSymbolDetailsRepository_BackwardCompatibility_BlackRockFields(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	// Simulate old WisdomTree-style JSON (without any BlackRock-specific fields)
	oldHoldings := `[{"Symbol":"BE","Name":"Bloom Energy Corp","Percent":1.4},{"Symbol":"TSLA","Name":"Tesla Inc","Percent":2.5}]`
	oldSectors := `[{"Sector":"technology","Percent":21.2},{"Sector":"industrials","Percent":36.4}]`
	oldCountries := `[{"Country":"United States","Percent":45.2},{"Country":"Japan","Percent":8.1}]`
	oldFundProfile := `{"Family":"WisdomTree","LegalType":"Exchange Traded Fund","TotalNetAssets":21526.37,"AnnualExpenseRatio":0.4,"Isin":"GB00BFNMHK52"}`
	oldEquityVal := `{"PriceToEarnings":25.3,"PriceToBook":4.2,"DividendYield":0.5}`

	_, err := db.Exec(`
		INSERT INTO symbol_details (internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, geographic_allocations, fund_profile, equity_valuation, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, datetime('now'))
	`, "WMGG.L", "WisdomTree Megatrends", "WisdomTree Megatrends UCITS ETF",
		"LSE", "GBP", "ETF",
		oldHoldings, oldSectors, oldCountries, oldFundProfile, oldEquityVal)
	if err != nil {
		t.Fatalf("insert old data: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}

	// Verify old fields still deserialize correctly
	if got.ShortName != "WisdomTree Megatrends" {
		t.Errorf("expected ShortName 'WisdomTree Megatrends', got %q", got.ShortName)
	}
	if got.QuoteType != "ETF" {
		t.Errorf("expected QuoteType 'ETF', got %q", got.QuoteType)
	}

	// Holdings: old fields present, new BlackRock fields empty/zero
	if len(got.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(got.TopHoldings))
	}
	if got.TopHoldings[0].Symbol != "BE" {
		t.Errorf("expected Symbol 'BE', got %q", got.TopHoldings[0].Symbol)
	}
	if got.TopHoldings[0].Percent != 1.4 {
		t.Errorf("expected Percent 1.4, got %f", got.TopHoldings[0].Percent)
	}
	// New BlackRock fields should be empty/zero
	if got.TopHoldings[0].Sector != "" {
		t.Errorf("expected empty Sector (backward compat), got %q", got.TopHoldings[0].Sector)
	}
	if got.TopHoldings[0].AssetClass != "" {
		t.Errorf("expected empty AssetClass (backward compat), got %q", got.TopHoldings[0].AssetClass)
	}
	if got.TopHoldings[0].MarketValue != 0 {
		t.Errorf("expected zero MarketValue (backward compat), got %f", got.TopHoldings[0].MarketValue)
	}
	if got.TopHoldings[0].Location != "" {
		t.Errorf("expected empty Location (backward compat), got %q", got.TopHoldings[0].Location)
	}
	if got.TopHoldings[0].Exchange != "" {
		t.Errorf("expected empty Exchange (backward compat), got %q", got.TopHoldings[0].Exchange)
	}

	// FundProfile: old fields present, new BlackRock fields empty
	if got.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if got.FundProfile.Family != "WisdomTree" {
		t.Errorf("expected Family 'WisdomTree', got %q", got.FundProfile.Family)
	}
	if got.FundProfile.Isin != "GB00BFNMHK52" {
		t.Errorf("expected Isin 'GB00BFNMHK52', got %q", got.FundProfile.Isin)
	}
	// New BlackRock fields should be empty
	if got.FundProfile.SFDRClassification != "" {
		t.Errorf("expected empty SFDRClassification (backward compat), got %q", got.FundProfile.SFDRClassification)
	}
	if got.FundProfile.Domicile != "" {
		t.Errorf("expected empty Domicile (backward compat), got %q", got.FundProfile.Domicile)
	}
	if got.FundProfile.FundManager != "" {
		t.Errorf("expected empty FundManager (backward compat), got %q", got.FundProfile.FundManager)
	}
	if got.FundProfile.Custodian != "" {
		t.Errorf("expected empty Custodian (backward compat), got %q", got.FundProfile.Custodian)
	}
	if got.FundProfile.IssuingCompany != "" {
		t.Errorf("expected empty IssuingCompany (backward compat), got %q", got.FundProfile.IssuingCompany)
	}
	if got.FundProfile.RebalanceFrequency != "" {
		t.Errorf("expected empty RebalanceFrequency (backward compat), got %q", got.FundProfile.RebalanceFrequency)
	}
	if got.FundProfile.ProductStructure != "" {
		t.Errorf("expected empty ProductStructure (backward compat), got %q", got.FundProfile.ProductStructure)
	}
	if got.FundProfile.Methodology != "" {
		t.Errorf("expected empty Methodology (backward compat), got %q", got.FundProfile.Methodology)
	}
	if got.FundProfile.BenchmarkTicker != "" {
		t.Errorf("expected empty BenchmarkTicker (backward compat), got %q", got.FundProfile.BenchmarkTicker)
	}

	// EquityValuation: old fields present, new BlackRock fields zero
	if got.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
	if got.EquityValuation.PriceToEarnings != 25.3 {
		t.Errorf("expected PriceToEarnings 25.3, got %f", got.EquityValuation.PriceToEarnings)
	}
	if got.EquityValuation.Beta3Y != 0 {
		t.Errorf("expected zero Beta3Y (backward compat), got %f", got.EquityValuation.Beta3Y)
	}
	if got.EquityValuation.StandardDeviation3Y != 0 {
		t.Errorf("expected zero StandardDeviation3Y (backward compat), got %f", got.EquityValuation.StandardDeviation3Y)
	}
	if got.EquityValuation.NumberOfHoldings != 0 {
		t.Errorf("expected zero NumberOfHoldings (backward compat), got %d", got.EquityValuation.NumberOfHoldings)
	}

	// Sectors and countries still work
	if len(got.SectorWeightings) != 2 {
		t.Fatalf("expected 2 sectors, got %d", len(got.SectorWeightings))
	}
	if len(got.GeographicAllocations) != 2 {
		t.Fatalf("expected 2 countries, got %d", len(got.GeographicAllocations))
	}
}

func TestSymbolDetailsRepository_CurrencyDerivativesAllocation_NilStoredAsNull(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol:                "VOO",
		ShortName:                     "Vanguard S&P 500",
		CurrencyDerivativesAllocation: nil,
		FetchedAt:                     time.Now(),
	}

	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.CurrencyDerivativesAllocation != nil {
		t.Errorf("expected nil CurrencyDerivativesAllocation, got %+v", got.CurrencyDerivativesAllocation)
	}
}

func TestSymbolDetailsRepository_TouchFetchedAt(t *testing.T) {
	db := setupSymbolDetailsDB(t)
	repo := NewSymbolDetailsRepository(db)

	// Insert symbol mapping
	_, err := db.ExecContext(context.Background(),
		"INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))",
		"VOO", "VOO")
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Insert details with recent fetched_at
	details := &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500",
		FetchedAt:      time.Now(),
	}
	err = repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Verify it's not stale
	stale, err := repo.ListStale(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("ListStale before touch: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected no stale symbols before touch, got %d", len(stale))
	}

	// Touch fetched_at
	err = repo.TouchFetchedAt(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("TouchFetchedAt: %v", err)
	}

	// Verify it's now stale
	stale, err = repo.ListStale(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ListStale after touch: %v", err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale symbol after touch, got %d", len(stale))
	} else if stale[0].InternalSymbol != "VOO" {
		t.Errorf("expected VOO, got %s", stale[0].InternalSymbol)
	}
}
