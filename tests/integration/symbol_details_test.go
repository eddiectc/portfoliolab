package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
)

func TestSymbolDetails_CreateAndEnrich(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create a symbol via API
	body := json.RawMessage(`{"internal_symbol": "TESTSYM", "market_data_symbol": "TEST"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	// Give the background goroutine a moment to start (and fail silently)
	time.Sleep(100 * time.Millisecond)

	// GET without details — should return 200 with null symbol_details
	getReq := httptest.NewRequest(http.MethodGet, "/api/symbols/1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("GET symbol: expected 200, got %d", w.Code)
	}

	var resp struct {
		ID               int64  `json:"id"`
		InternalSymbol   string `json:"internal_symbol"`
		MarketDataSymbol string `json:"market_data_symbol"`
		SymbolDetails    *struct {
			ShortName string `json:"short_name"`
		} `json:"symbol_details"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.InternalSymbol != "TESTSYM" {
		t.Errorf("expected internal_symbol TESTSYM, got %q", resp.InternalSymbol)
	}
	if resp.SymbolDetails != nil {
		t.Error("expected nil symbol_details before cache populated")
	}

	// Manually insert symbol details (simulating what the background fetch would do)
	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, aggregate_positions, fund_profile, equity_valuation,
			fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "TESTSYM", "Test Symbol", "Test Symbol Full", "NYSE", "USD", "EQUITY",
		`[{"symbol":"AAPL","name":"Apple Inc","percent":0.05}]`,
		`[{"sector":"technology","percent":0.60}]`,
		`{"stock":0.95,"bond":0,"cash":0.05,"convertible":0,"preferred":0,"other":0}`,
		`{"family":"Test Family","legalType":"Exchange Traded Fund","totalNetAssets":1000000,"annualExpenseRatio":0.03,"annualHoldingsTurnover":0.25}`,
		`{"priceToEarnings":0.02,"priceToBook":0.15,"priceToCashflow":0.05,"priceToSales":0.30}`,
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// GET again — should now include symbol_details
	getReq = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/symbols/%d", resp.ID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("GET symbol after details: expected 200, got %d", w.Code)
	}

	var resp2 struct {
		SymbolDetails *struct {
			InternalSymbol string  `json:"internal_symbol"`
			ShortName      string  `json:"short_name"`
			LongName       string  `json:"long_name"`
			Exchange       string  `json:"exchange"`
			Currency       string  `json:"currency"`
			QuoteType      string  `json:"quote_type"`
			TopHoldings    []struct {
				Symbol  string  `json:"symbol"`
				Name    string  `json:"name"`
				Percent float64 `json:"percent"`
			} `json:"top_holdings"`
			SectorWeightings []struct {
				Sector  string  `json:"sector"`
				Percent float64 `json:"percent"`
			} `json:"sector_weightings"`
			AggregatePositions *struct {
				Stock float64 `json:"stock"`
				Bond  float64 `json:"bond"`
				Cash  float64 `json:"cash"`
			} `json:"aggregate_positions"`
			FundProfile *struct {
				Family           string  `json:"family"`
				LegalType        string  `json:"legalType"`
				TotalNetAssets   float64 `json:"totalNetAssets"`
				ExpenseRatio     float64 `json:"annualExpenseRatio"`
			} `json:"fund_profile"`
			EquityValuation *struct {
				PriceToEarnings float64 `json:"priceToEarnings"`
			} `json:"equity_valuation"`
		} `json:"symbol_details"`
	}
	json.NewDecoder(w.Body).Decode(&resp2)

	if resp2.SymbolDetails == nil {
		t.Fatal("expected symbol_details to be populated after cache insert")
	}
	details := resp2.SymbolDetails

	if details.ShortName != "Test Symbol" {
		t.Errorf("expected short_name 'Test Symbol', got %q", details.ShortName)
	}
	if details.Exchange != "NYSE" {
		t.Errorf("expected exchange NYSE, got %q", details.Exchange)
	}
	if details.Currency != "USD" {
		t.Errorf("expected currency USD, got %q", details.Currency)
	}
	if details.QuoteType != "EQUITY" {
		t.Errorf("expected quote_type EQUITY, got %q", details.QuoteType)
	}
	if len(details.TopHoldings) != 1 || details.TopHoldings[0].Symbol != "AAPL" {
		t.Errorf("expected 1 holding with symbol AAPL, got %v", details.TopHoldings)
	}
	if len(details.SectorWeightings) != 1 || details.SectorWeightings[0].Sector != "technology" {
		t.Errorf("expected 1 sector weighting, got %v", details.SectorWeightings)
	}
	if details.AggregatePositions == nil || details.AggregatePositions.Stock != 0.95 {
		t.Errorf("expected stock position 0.95, got %v", details.AggregatePositions)
	}
	if details.FundProfile == nil || details.FundProfile.Family != "Test Family" {
		t.Errorf("expected fund family 'Test Family', got %v", details.FundProfile)
	}
	if details.EquityValuation == nil || details.EquityValuation.PriceToEarnings != 0.02 {
		t.Errorf("expected P/E 0.02, got %v", details.EquityValuation)
	}
}

func TestSymbolDetails_ListExcludesDetails(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create a symbol
	body := json.RawMessage(`{"internal_symbol": "LISTSYM", "market_data_symbol": "LIST"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d", w.Code)
	}

	// GET list — should return lean response (no symbol_details field)
	listReq := httptest.NewRequest(http.MethodGet, "/api/symbols", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, listReq)
	if w.Code != http.StatusOK {
		t.Fatalf("list symbols: expected 200, got %d", w.Code)
	}

	var symbols []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&symbols)
	if len(symbols) == 0 {
		t.Fatal("expected at least 1 symbol in list")
	}
	// List response should NOT have symbol_details
	if _, hasDetails := symbols[0]["symbol_details"]; hasDetails {
		t.Error("list response should not include symbol_details")
	}
}

func TestSymbolDetails_StaleRefresh_PicksUpMissingDetails(t *testing.T) {
	db := setupTestDB(t)

	// Insert a symbol mapping directly (avoiding background fetch)
	_, err := db.Exec(`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol) VALUES (?, ?)`,
		"STALESYM", "STALE")
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Verify the symbol exists but has no details
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM symbol_details WHERE internal_symbol = ?", "STALESYM").Scan(&count)
	if err != nil {
		t.Fatalf("query symbol_details: %v", err)
	}
	if count != 0 {
		t.Fatal("expected 0 symbol details (no fetch performed)")
	}

	// The stale refresh query (LEFT JOIN) should pick up this symbol
	var staleCount int
	threshold := time.Now().Add(-8 * 24 * time.Hour).Format(time.RFC3339)
	err = db.QueryRow(`
		SELECT COUNT(*) FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, threshold).Scan(&staleCount)
	if err != nil {
		t.Fatalf("query stale: %v", err)
	}
	if staleCount != 1 {
		t.Errorf("expected 1 stale symbol, got %d", staleCount)
	}

	// Insert details with old fetched_at — should also be picked up
	_, err = db.Exec(`
		INSERT INTO symbol_details (internal_symbol, short_name, fetched_at)
		VALUES (?, ?, ?)
	`, "OLDDET", "Old Detail", time.Now().Add(-10*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert old details: %v", err)
	}
	// Also need a mapping for it
	_, err = db.Exec(`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol) VALUES (?, ?)`,
		"OLDDET", "OLD")
	if err != nil {
		t.Fatalf("insert old mapping: %v", err)
	}

	err = db.QueryRow(`
		SELECT COUNT(*) FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, threshold).Scan(&staleCount)
	if err != nil {
		t.Fatalf("query stale after old insert: %v", err)
	}
	if staleCount != 2 {
		t.Errorf("expected 2 stale symbols, got %d", staleCount)
	}

	// Insert fresh details — should NOT be picked up
	_, err = db.Exec(`
		INSERT INTO symbol_details (internal_symbol, short_name, fetched_at)
		VALUES (?, ?, ?)
	`, "FRESHDET", "Fresh Detail", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert fresh details: %v", err)
	}
	_, err = db.Exec(`INSERT INTO symbol_mappings (internal_symbol, market_data_symbol) VALUES (?, ?)`,
		"FRESHDET", "FRESH")
	if err != nil {
		t.Fatalf("insert fresh mapping: %v", err)
	}

	err = db.QueryRow(`
		SELECT COUNT(*) FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, threshold).Scan(&staleCount)
	if err != nil {
		t.Fatalf("query stale after fresh insert: %v", err)
	}
	if staleCount != 2 {
		t.Errorf("expected 2 stale symbols (fresh excluded), got %d", staleCount)
	}
}
