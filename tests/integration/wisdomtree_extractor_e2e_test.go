package integration

import (
	"bytes"
	"context"
	"encoding/json"
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/config"
	"codeberg.org/eddiectc/portfoliolab/internal/data"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/blackrock"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/dimensional"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/imgp"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/vanguard"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/wisdomtree"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

//go:embed testdata/wmgt_page_cycletls.html
var wmgtMainPage string

//go:embed testdata/wmgt_modal_all_holdings.html
var wmgtModalPage string

// TestWisdomTree_ServiceLayerRoundTrip validates the full service-layer path:
// Extract → dispatcher.Dispatch → extractResultToSymbolDetails → Upsert → API GET.
// Uses embedded real HTML (no network calls).
func TestWisdomTree_ServiceLayerRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	// New-format URL (dispatch only — fetch is mocked with the embedded v1 page).
	sourceURL := "https://www.wisdomtree.com/gb/products/equities/wmgt---wisdomtree-megatrends-ucits-etf---usd-acc"
	internalSymbol := "WMGT.L"

	// Insert symbol mapping
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, internalSymbol, "WMGT.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Set up the dispatcher with a WisdomTree extractor that uses mock fetch
	wtExtractor := wisdomtree.NewExtractor()
	wtClient := wisdomtree.NewClient()
	wtClient.SetFetchFunc(func(url string) (string, error) {
		if strings.Contains(url, "all-holdings") {
			return wmgtModalPage, nil
		}
		return wmgtMainPage, nil
	})
	wtExtractor.SetClient(wtClient)

	reg := extractor.NewRegistry()
	reg.Register(wtExtractor)
	dispatcher := extractor.NewDispatcher(reg)

	// Build the service layer the same way the router does
	symbolDetailsRepo := data.NewSymbolDetailsRepository(db)
	yahooFetcher := market.NewYahooFinanceFetcher(testLogger())
	symbolDetailsSvc := symbols.NewService(symbolDetailsRepo, yahooFetcher)
	symbolDetailsSvc.WithExtractorDispatcher(dispatcher)
	symbolDetailsSvc.WithDataSourceURLRepo(data.NewSymbolMappingDataSourceURLAdapter(
		data.NewSymbolMappingRepository(db)))

	// Call FetchAndStore — the same path the background job uses
	if err := symbolDetailsSvc.FetchAndStore(context.Background(), internalSymbol, "WMGT.L"); err != nil {
		t.Fatalf("FetchAndStore failed: %v", err)
	}

	// Read back via repo
	read, err := symbolDetailsRepo.GetByInternalSymbol(context.Background(), internalSymbol)
	if err != nil {
		t.Fatalf("GetByInternalSymbol failed: %v", err)
	}

	// The critical check: sectors survived the service-layer mapping + DB round-trip
	if len(read.SectorWeightings) == 0 {
		t.Fatalf("sector_weightings empty after FetchAndStore round-trip")
	}
	t.Logf("Repo round-trip: sectors=%d, themes=%d, holdings=%d, countries=%d",
		len(read.SectorWeightings), len(read.Themes), len(read.TopHoldings),
		len(read.GeographicAllocations))
}

// TestWisdomTree_FullAPIRoundTrip validates the complete API path:
// FetchAndStore → GET /api/symbols/{id} includes sector_weightings.
func TestWisdomTree_FullAPIRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(),
		api.WithTemplatesDir("../../templates"),
		api.WithExtractorConfig(config.ExtractorConfig{}))

	// New-format URL (dispatch only — fetch is mocked with the embedded v1 page).
	sourceURL := "https://www.wisdomtree.com/gb/products/equities/wmgt---wisdomtree-megatrends-ucits-etf---usd-acc"
	internalSymbol := "WMGT.L"

	// Create symbol via API
	body := json.RawMessage(`{"internal_symbol": "WMGT.L", "market_data_symbol": "WMGT.L"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	time.Sleep(100 * time.Millisecond)

	var symID int64
	err := db.QueryRow("SELECT id FROM symbol_mappings WHERE internal_symbol = ?", internalSymbol).Scan(&symID)
	if err != nil {
		t.Fatalf("query symbol id: %v", err)
	}

	// Set data_source_url
	_, err = db.Exec(`UPDATE symbol_mappings SET data_source_url = ? WHERE internal_symbol = ?`,
		sourceURL, internalSymbol)
	if err != nil {
		t.Fatalf("update data_source_url: %v", err)
	}

	// The router already registered WisdomTree extractor — inject mock fetch into it
	// We need to access the dispatcher's registry to set the mock client.
	// Since we can't access internal state, we use TouchFetchedAt + direct service call.
	// Instead, call FetchAndStore directly through the service layer.
	// Build a service with mocked extractor (same as the router's but with mock fetch).
	wtExtractor := wisdomtree.NewExtractor()
	wtClient := wisdomtree.NewClient()
	wtClient.SetFetchFunc(func(url string) (string, error) {
		if strings.Contains(url, "all-holdings") {
			return wmgtModalPage, nil
		}
		return wmgtMainPage, nil
	})
	wtExtractor.SetClient(wtClient)

	reg := extractor.NewRegistry()
	reg.Register(wtExtractor)
	reg.Register(blackrock.NewExtractor())
	reg.Register(dimensional.NewExtractor())
	reg.Register(imgp.NewExtractor())
	reg.Register(vanguard.NewExtractor())
	dispatcher := extractor.NewDispatcher(reg)

	symbolDetailsRepo := data.NewSymbolDetailsRepository(db)
	yahooFetcher := market.NewYahooFinanceFetcher(testLogger())
	symbolDetailsSvc := symbols.NewService(symbolDetailsRepo, yahooFetcher)
	symbolDetailsSvc.WithExtractorDispatcher(dispatcher)
	symbolDetailsSvc.WithDataSourceURLRepo(data.NewSymbolMappingDataSourceURLAdapter(
		data.NewSymbolMappingRepository(db)))

	// FetchAndStore — same path as the background refresh job
	if err := symbolDetailsSvc.FetchAndStore(context.Background(), internalSymbol, "WMGT.L"); err != nil {
		t.Fatalf("FetchAndStore failed: %v", err)
	}

	// GET via API — verify sector_weightings is present
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/symbols/%d", symID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("GET symbol: expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	detailsRaw, ok := resp["symbol_details"]
	if !ok || detailsRaw == nil {
		t.Fatal("expected symbol_details in response")
	}
	detailsMap := detailsRaw.(map[string]interface{})

	// The critical check: sector_weightings present and non-empty
	sectorsRaw, ok := detailsMap["sector_weightings"]
	if !ok || sectorsRaw == nil {
		t.Fatal("sector_weightings missing or null in API response")
	}
	sectors, ok := sectorsRaw.([]interface{})
	t.Logf("sector_weightings: %+v", sectors);
	if !ok {
		t.Fatalf("sector_weightings is not an array: %T", sectorsRaw)
	}
	if len(sectors) == 0 {
		t.Fatalf("sector_weightings is empty array in API response")
	}

	// Verify themes also present
	themesRaw, ok := detailsMap["theme_breakdown"]
	if !ok || themesRaw == nil {
		t.Fatal("theme_breakdown missing in API response")
	}
	themes := themesRaw.([]interface{})

	// Verify holdings present
	holdingsRaw, ok := detailsMap["top_holdings"]
	if !ok || holdingsRaw == nil {
		t.Fatal("top_holdings missing in API response")
	}
	holdings := holdingsRaw.([]interface{})

	t.Logf("API round-trip OK: sectors=%d, themes=%d, holdings=%d",
		len(sectors), len(themes), len(holdings))
}
