package integration

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"codeberg.org/eddiectc/portfoliolab/internal/data"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/wisdomtree"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

//go:embed testdata/page_qgrw_wtclassid.txt
var qgrwPageWtClassID string

//go:embed testdata/flight_qgrw_tables.html
var qgrwFlightTables string

//go:embed testdata/flight_qgrw_sector.html
var qgrwFlightSector string

//go:embed testdata/holdings_qgrw.json
var qgrwHoldings string

//go:embed testdata/fund_history_qgrw.json
var qgrwHistory string

// qgrwSourceURL is the new-format product page the dispatcher must route to
// the WisdomTree extractor.
const qgrwSourceURL = "https://www.wisdomtree.com/gb/products/equities/wisdomtree-us-quality-growth-ucits-etf---usd-acc"

// fakeYahooFetcher implements market.SymbolDetailsFetcher without any network
// access. In the extractor path Yahoo is only used to source Exchange and
// Currency, so canned values are sufficient.
type fakeYahooFetcher struct{}

func (fakeYahooFetcher) FetchSymbolDetails(_ context.Context, _ string) (*symbol.SymbolDetails, error) {
	return &symbol.SymbolDetails{Exchange: "LSE", Currency: "USD"}, nil
}

// mockWisdomTreeClient returns a client that serves the captured QGRW
// fixtures (page + holdings + history API) for every fetch.
func mockWisdomTreeClient(t *testing.T) *wisdomtree.Client {
	t.Helper()
	page := qgrwPageWtClassID + qgrwFlightTables + qgrwFlightSector
	c := wisdomtree.NewClient()
	c.SetThrottle(func(time.Duration) {}) // no real waiting in tests
	c.SetFetchFunc(func(url string) (string, error) {
		switch {
		case strings.Contains(url, "/fund-holdings/"):
			return qgrwHoldings, nil
		case strings.Contains(url, "/fund-history/"):
			return qgrwHistory, nil
		default:
			return page, nil
		}
	})
	return c
}

// newWisdomTreeTestService builds a symbols service wired exactly like the
// router (dispatcher + URL adapter + market-data repo) but with a mocked
// WisdomTree client and a fake Yahoo fetcher, so the tests make no network
// calls at all.
func newWisdomTreeTestService(t *testing.T, db *sql.DB) *symbols.Service {
	t.Helper()
	wtExtractor := wisdomtree.NewExtractor()
	wtExtractor.SetClient(mockWisdomTreeClient(t))

	reg := extractor.NewRegistry()
	_ = reg.Register(wtExtractor)

	svc := symbols.NewService(data.NewSymbolDetailsRepository(db), fakeYahooFetcher{})
	svc.WithExtractorDispatcher(extractor.NewDispatcher(reg))
	svc.WithDataSourceURLRepo(data.NewSymbolMappingDataSourceURLAdapter(data.NewSymbolMappingRepository(db)))
	svc.WithMarketDataRepo(data.NewMarketDataRepository(db))
	return svc
}

// TestWisdomTree_ServiceLayerRoundTrip validates the full service-layer path:
// Extract → dispatcher.Dispatch → extractResultToSymbolDetails → Upsert →
// repo read, plus NAV history stored to market_data. Uses captured fixtures
// only (no network calls).
func TestWisdomTree_ServiceLayerRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	internalSymbol := "QGRW.L"

	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, internalSymbol, "QGRW.L", qgrwSourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	svc := newWisdomTreeTestService(t, db)
	if err := svc.FetchAndStore(context.Background(), internalSymbol, "QGRW.L"); err != nil {
		t.Fatalf("FetchAndStore failed: %v", err)
	}

	repo := data.NewSymbolDetailsRepository(db)
	read, err := repo.GetByInternalSymbol(context.Background(), internalSymbol)
	if err != nil {
		t.Fatalf("GetByInternalSymbol failed: %v", err)
	}

	// Yahoo-fetched envelope (fake fetcher)
	if read.Exchange != "LSE" || read.Currency != "USD" {
		t.Errorf("Exchange/Currency = %q/%q, want LSE/USD (fake Yahoo)", read.Exchange, read.Currency)
	}

	// Sectors: 9 entries, top Information Technology.
	if len(read.SectorWeightings) != 9 {
		t.Fatalf("SectorWeightings len = %d, want 9", len(read.SectorWeightings))
	}
	if read.SectorWeightings[0].Sector != "Information Technology" || read.SectorWeightings[0].Percent != 58.4027 {
		t.Errorf("top sector = %+v, want Information Technology 58.4027", read.SectorWeightings[0])
	}

	// Holdings: 100 entries, top NVDA.
	if len(read.TopHoldings) != 100 {
		t.Fatalf("TopHoldings len = %d, want 100", len(read.TopHoldings))
	}
	if read.TopHoldings[0].Symbol != "NVDA" {
		t.Errorf("top holding = %q, want NVDA", read.TopHoldings[0].Symbol)
	}

	// EquityValuation must survive the repo round-trip. Regression: the
	// wisdomtree parser must set the FieldsPresent mask so the service-layer
	// mapping keeps the section.
	if read.EquityValuation == nil {
		t.Fatal("EquityValuation is nil after round-trip")
	}
	if read.EquityValuation.PriceToEarnings != 31.43 {
		t.Errorf("PriceToEarnings = %v, want 31.43", read.EquityValuation.PriceToEarnings)
	}
	if read.EquityValuation.DividendYield != 0.35 {
		t.Errorf("DividendYield = %v, want 0.35", read.EquityValuation.DividendYield)
	}

	// FundProfile fields.
	if read.FundProfile == nil {
		t.Fatal("FundProfile is nil after round-trip")
	}
	if read.FundProfile.TotalNetAssets != 47442965 {
		t.Errorf("TotalNetAssets = %v, want 47442965", read.FundProfile.TotalNetAssets)
	}
	if read.FundProfile.AnnualExpenseRatio != 0.0033 {
		t.Errorf("AnnualExpenseRatio = %v, want 0.0033", read.FundProfile.AnnualExpenseRatio)
	}
	if got := read.FundProfile.InceptionDate.Format("2006-01-02"); got != "2024-04-16" {
		t.Errorf("InceptionDate = %q, want 2024-04-16", got)
	}

	// Geographic allocations: 4 entries, top United States.
	if len(read.GeographicAllocations) != 4 {
		t.Fatalf("GeographicAllocations len = %d, want 4", len(read.GeographicAllocations))
	}
	if read.GeographicAllocations[0].Country != "United States" || read.GeographicAllocations[0].Percent != 99.54 {
		t.Errorf("top country = %+v, want United States 99.54", read.GeographicAllocations[0])
	}

	// QGRW has no themes section — must round-trip as nil.
	if read.Themes != nil {
		t.Errorf("Themes = %v, want nil", read.Themes)
	}

	// NAV history stored to market_data.
	var navCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM market_data
		WHERE symbol = ? AND data_type = 'nav' AND source = 'wisdomtree'
	`, internalSymbol).Scan(&navCount); err != nil {
		t.Fatalf("count nav rows: %v", err)
	}
	if navCount != 601 {
		t.Errorf("nav rows = %d, want 601", navCount)
	}
	var latestDate string
	if err := db.QueryRow(`
		SELECT date FROM market_data
		WHERE symbol = ? AND data_type = 'nav' AND source = 'wisdomtree'
		ORDER BY date DESC LIMIT 1
	`, internalSymbol).Scan(&latestDate); err != nil {
		t.Fatalf("latest nav date: %v", err)
	}
	if latestDate != "2026-08-28" {
		t.Errorf("latest nav date = %q, want 2026-08-28", latestDate)
	}

	t.Logf("Repo round-trip: sectors=%d, holdings=%d, countries=%d, nav=%d",
		len(read.SectorWeightings), len(read.TopHoldings),
		len(read.GeographicAllocations), navCount)
}

// TestWisdomTree_FullAPIRoundTrip validates the complete API path:
// FetchAndStore → GET /api/symbols/{id} returns the extracted details
// (including equity_valuation and fund_profile). No network calls.
func TestWisdomTree_FullAPIRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	internalSymbol := "QGRW.L"

	// Create symbol via API.
	body := json.RawMessage(fmt.Sprintf(`{"internal_symbol": %q, "market_data_symbol": %q}`, internalSymbol, "QGRW.L"))
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	// Set data_source_url (dispatcher routes on it).
	if _, err := db.Exec(`UPDATE symbol_mappings SET data_source_url = ? WHERE internal_symbol = ?`,
		qgrwSourceURL, internalSymbol); err != nil {
		t.Fatalf("update data_source_url: %v", err)
	}

	var symID int64
	if err := db.QueryRow("SELECT id FROM symbol_mappings WHERE internal_symbol = ?", internalSymbol).Scan(&symID); err != nil {
		t.Fatalf("query symbol id: %v", err)
	}

	// FetchAndStore with mocked fetches (same path as the background job).
	svc := newWisdomTreeTestService(t, db)
	if err := svc.FetchAndStore(context.Background(), internalSymbol, "QGRW.L"); err != nil {
		t.Fatalf("FetchAndStore failed: %v", err)
	}

	// GET via API.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/symbols/%d", symID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("GET symbol: expected 200, got %d", w.Code)
	}

	var apiResp struct {
		SymbolDetails *struct {
			InternalSymbol   string                   `json:"internal_symbol"`
			Exchange         string                   `json:"exchange"`
			Currency         string                   `json:"currency"`
			TopHoldings      []symbol.TopHolding      `json:"top_holdings"`
			SectorWeightings []symbol.SectorWeighting `json:"sector_weightings"`
			ThemeBreakdown   []symbol.ThemeBreakdown  `json:"theme_breakdown"`
			FundProfile      *symbol.FundProfile      `json:"fund_profile"`
			EquityValuation  *symbol.EquityValuation  `json:"equity_valuation"`
		} `json:"symbol_details"`
	}
	if err := json.NewDecoder(w.Body).Decode(&apiResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if apiResp.SymbolDetails == nil {
		t.Fatal("expected symbol_details in response")
	}
	d := apiResp.SymbolDetails

	if len(d.SectorWeightings) != 9 {
		t.Errorf("sector_weightings len = %d, want 9", len(d.SectorWeightings))
	}
	if len(d.TopHoldings) != 100 {
		t.Errorf("top_holdings len = %d, want 100", len(d.TopHoldings))
	}
	if d.ThemeBreakdown != nil {
		t.Errorf("theme_breakdown = %v, want absent (QGRW has no themes)", d.ThemeBreakdown)
	}
	if d.FundProfile == nil || d.FundProfile.TotalNetAssets != 47442965 {
		t.Errorf("fund_profile = %+v, want TotalNetAssets 47442965", d.FundProfile)
	}
	if d.EquityValuation == nil {
		t.Fatal("equity_valuation missing from API response")
	}
	if d.EquityValuation.PriceToEarnings != 31.43 || d.EquityValuation.DividendYield != 0.35 {
		t.Errorf("equity_valuation PE/DY = %v/%v, want 31.43/0.35",
			d.EquityValuation.PriceToEarnings, d.EquityValuation.DividendYield)
	}

	t.Logf("API round-trip OK: sectors=%d, holdings=%d, equity_valuation=PE %.2f",
		len(d.SectorWeightings), len(d.TopHoldings), d.EquityValuation.PriceToEarnings)
}
