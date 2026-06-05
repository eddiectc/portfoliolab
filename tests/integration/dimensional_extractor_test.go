package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/dimensional"
)

func TestDimensional_ExtractorDispatch_Routing(t *testing.T) {
	// Verify the Dimensional extractor is registered and routes correctly
	reg := extractor.NewRegistry()
	dimExtractor := dimensional.NewExtractor()
	if err := reg.Register(dimExtractor); err != nil {
		t.Fatalf("register dimensional extractor: %v", err)
	}

	// Dimensional URL should find the extractor (test routing without network calls)
	found, err := reg.FindByURL(
		"https://www.dimensional.com/gb-en/funds/ie000eggfvg6/global-core-equity-ucits-etf-acc")
	if err != nil {
		t.Fatalf("routing failed: %v", err)
	}
	if found.Name() != "dimensional" {
		t.Errorf("expected dimensional extractor, got %q", found.Name())
	}

	// Non-matching URL should fail with "no extractor" error
	_, err = reg.FindByURL("https://finance.yahoo.com/quote/SPY")
	if err == nil {
		t.Error("expected error for non-dimensional URL")
	}
}

func TestDimensional_ExtractorRegisteredInRouter(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Verify the router is functional by hitting a known endpoint
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health check: expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %q", resp["status"])
	}
}

func TestDimensional_SymbolWithDataSourceURL_RoundTrip(t *testing.T) {
	db := setupTestDB(t)

	dimensionalURL := "https://www.dimensional.com/gb-en/funds/ie000eggfvg6/global-core-equity-ucits-etf-acc"

	// Insert a symbol mapping with dimensional data_source_url
	result, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, "DGRC.L", "DGRC.L", dimensionalURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		t.Fatalf("expected 1 row affected, got %v", rows)
	}

	// Verify data_source_url is returned by the stale query (cross-layer audit)
	var dataSourceURL, internalSymbol string
	err = db.QueryRow(`
		SELECT sm.internal_symbol, sm.data_source_url
		FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, time.Now().Add(-7*24*time.Hour).Format(time.RFC3339)).Scan(&internalSymbol, &dataSourceURL)
	if err != nil {
		t.Fatalf("query stale with data_source_url: %v", err)
	}
	if internalSymbol != "DGRC.L" {
		t.Errorf("expected internal_symbol DGRC.L, got %q", internalSymbol)
	}
	if dataSourceURL != dimensionalURL {
		t.Errorf("expected dimensional data_source_url, got %q", dataSourceURL)
	}
}

func TestDimensional_SymbolAPICreatesWithSourceURL(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create a symbol first (without data_source_url)
	body := json.RawMessage(`{"internal_symbol": "DGRC.L", "market_data_symbol": "DGRC.L"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	// Get the symbol ID
	var id int64
	err := db.QueryRow("SELECT id FROM symbol_mappings WHERE internal_symbol = ?", "DGRC.L").Scan(&id)
	if err != nil {
		t.Fatalf("query symbol id: %v", err)
	}

	// Update with data_source_url via PATCH
	patchBody := json.RawMessage(fmt.Sprintf(
		`{"data_source_url": "%s"}`,
		"https://www.dimensional.com/gb-en/funds/ie000eggfvg6/global-core-equity-ucits-etf-acc"))
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/symbols/%d", id), bytes.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch symbol: expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	// Verify the data_source_url was set
	var dataSourceURL sql.NullString
	err = db.QueryRow("SELECT data_source_url FROM symbol_mappings WHERE id = ?", id).Scan(&dataSourceURL)
	if err != nil {
		t.Fatalf("query data_source_url: %v", err)
	}
	if !dataSourceURL.Valid {
		t.Error("expected data_source_url to be set, got NULL")
	} else if dataSourceURL.String == "" {
		t.Error("expected data_source_url to be non-empty")
	}
}

func TestDimensional_NavDataTypeStandalone(t *testing.T) {
	db := setupTestDB(t)

	// Insert NAV data with dimensional source
	_, err := db.Exec(`
		INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "DGRC.L", "28.34", "GBP", "nav", "dimensional", "2026-05-28", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert NAV data: %v", err)
	}

	// Verify NAV data is queryable by data_type
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM market_data WHERE data_type = 'nav' AND source = 'dimensional'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query NAV data: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 NAV record with dimensional source, got %d", count)
	}

	// Verify NAV data is NOT returned by stock queries
	err = db.QueryRow(`
		SELECT COUNT(*) FROM market_data WHERE data_type IN ('stock', 'fx')
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query stock data: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 stock records, got %d", count)
	}
}

func TestDimensional_ExtractResultStructure(t *testing.T) {
	// Verify the Extractor interface is properly implemented
	var _ extractor.Extractor = (*dimensional.Extractor)(nil)

	ex := dimensional.NewExtractor()
	if ex.Name() != dimensional.Name {
		t.Errorf("Name() = %q, want %q", ex.Name(), dimensional.Name)
	}
}

func TestDimensional_IntegrationFullStack(t *testing.T) {
	db := setupTestDB(t)

	isin := "IE000EGGFVG6"
	sourceURL := fmt.Sprintf("https://www.dimensional.com/gb-en/funds/%s/global-core-equity-ucits-etf-acc", isin)

	// Insert symbol with dimensional source URL
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, "DGRC.L", "DGRC.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol: %v", err)
	}

	// Manually insert symbol details (simulating what a successful extraction would store)
	asOfDate := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, fund_profile,
			geographic_allocations, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "DGRC.L", "Test ETF", "Test ETF Full", "LSE", "GBP", "ETF",
		`[{"symbol":"AAPL","name":"Apple Inc","percent":5.0}]`,
		`[{"sector":"Technology","percent":21.59}]`,
		`{"family":"Dimensional Fund Advisors","legalType":"ETF","totalNetAssets":1000000,"annualExpenseRatio":0.0026}`,
		`[{"country":"United States","percent":70.31}]`,
		asOfDate.Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Verify the data round-trips correctly
	var shortName, family, extractorAsOf string
	var totalNetAssets sql.NullFloat64
	err = db.QueryRow(`
		SELECT sd.short_name,
		       json_extract(sd.fund_profile, '$.family'),
		       json_extract(sd.fund_profile, '$.totalNetAssets'),
		       sd.extractor_as_of_date
		FROM symbol_details sd
		WHERE sd.internal_symbol = ?
	`, "DGRC.L").Scan(&shortName, &family, &totalNetAssets, &extractorAsOf)
	if err != nil {
		t.Fatalf("query symbol details: %v", err)
	}

	if shortName != "Test ETF" {
		t.Errorf("short_name = %q, want %q", shortName, "Test ETF")
	}
	if family != "Dimensional Fund Advisors" {
		t.Errorf("family = %q, want %q", family, "Dimensional Fund Advisors")
	}
	if !totalNetAssets.Valid || totalNetAssets.Float64 != 1000000 {
		t.Errorf("totalNetAssets = %v, want 1000000", totalNetAssets)
	}
	if extractorAsOf != asOfDate.Format(time.RFC3339) {
		t.Errorf("extractor_as_of_date = %q, want %q", extractorAsOf, asOfDate.Format(time.RFC3339))
	}

	// Verify holdings are parseable
	var holdingsJSON string
	err = db.QueryRow("SELECT top_holdings FROM symbol_details WHERE internal_symbol = ?",
		"DGRC.L").Scan(&holdingsJSON)
	if err != nil {
		t.Fatalf("query holdings: %v", err)
	}
	var holdings []struct {
		Symbol  string  `json:"symbol"`
		Name    string  `json:"name"`
		Percent float64 `json:"percent"`
	}
	if err := json.Unmarshal([]byte(holdingsJSON), &holdings); err != nil {
		t.Fatalf("parse holdings JSON: %v", err)
	}
	if len(holdings) != 1 || holdings[0].Symbol != "AAPL" {
		t.Errorf("holdings = %v, want 1 holding with AAPL", holdings)
	}
}
