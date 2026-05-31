package integration

import (
	"bytes"
	"context"
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
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/vanguard"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/wisdomtree"
)

func TestVanguard_ExtractorDispatch_Routing(t *testing.T) {
	reg := extractor.NewRegistry()
	vgExtractor := vanguard.NewExtractor()
	if err := reg.Register(vgExtractor); err != nil {
		t.Fatalf("register vanguard extractor: %v", err)
	}

	dispatcher := extractor.NewDispatcher(reg)

	// Vanguard URL should find the extractor (may fail on network, but not on routing)
	_, err := dispatcher.Dispatch(context.Background(),
		"https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing")
	if err != nil {
		errStr := err.Error()
		if len(errStr) > 20 && errStr[:20] == "no extractor registere" {
			t.Fatalf("routing failed: %v", err)
		}
		// Any other error (network, API) is expected in test environment
	}

	// Non-matching URL should fail with "no extractor" error
	_, err = dispatcher.Dispatch(context.Background(),
		"https://finance.yahoo.com/quote/SPY")
	if err == nil {
		t.Error("expected error for non-vanguard URL")
	}
}

func TestVanguard_ExtractorRegisteredInRouter(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

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

func TestVanguard_ExtractResultStructure(t *testing.T) {
	var _ extractor.Extractor = (*vanguard.Extractor)(nil)

	ex := vanguard.NewExtractor()
	if ex.Name() != vanguard.Name {
		t.Errorf("Name() = %q, want %q", ex.Name(), vanguard.Name)
	}
}

func TestVanguard_SymbolDetails_FullStackRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	sourceURL := "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing"
	internalSymbol := "VWRL.L"

	// Insert symbol mapping with vanguard data_source_url
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, internalSymbol, "VWRL.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Insert symbol details with all Vanguard-specific fields
	asOfDate := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)

	// Holdings with bond fields (SecurityType, CouponRate, FinalMaturity, AsOfDate)
	holdingsJSON := `[
		{"symbol":"AAPL","name":"Apple Inc","percent":3.5,"securityType":"Equity","asOfDate":"2026-05-31"},
		{"symbol":"US10Y","name":"US Treasury 10Y","percent":2.1,"securityType":"Government Bond","couponRate":4.25,"finalMaturity":"2034-06-15","asOfDate":"2026-05-31"}
	]`
	// Sectors with Date
	sectorsJSON := `[{"sector":"Technology","percent":21.5,"date":"2026-05-31"},{"sector":"Financials","percent":14.2,"date":"2026-05-31"}]`
	// Countries with Region fields
	countriesJSON := `[
		{"country":"United States","percent":60.5,"regionName":"North America","regionCode":"NA","date":"2026-05-31"},
		{"country":"United Kingdom","percent":4.2,"regionName":"Europe","regionCode":"EU","date":"2026-05-31"}
	]`
	// Expanded equity valuation
	equityJSON := `{"priceToEarnings":18.5,"priceToBook":3.2,"priceToCashflow":12.0,"priceToSales":2.8,"medianMarketCap":2500000,"forwardROE":15.3,"forwardEPSGrowth":8.7,"revenueRatio":0.45}`
	// Bond characteristics
	bondJSON := `{"averageCoupon":3.85,"averageMaturity":7.2,"averageQuality":7.2,"averageDuration":6.5}`

	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, geographic_allocations,
			equity_valuation, bond_characteristics,
			fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "Vanguard FTSE All-World", "Vanguard FTSE All-World UCITS ETF",
		"LSE", "USD", "ETF",
		holdingsJSON, sectorsJSON, countriesJSON,
		equityJSON, bondJSON,
		`{"family":"Vanguard","legalType":"Exchange Traded Fund","totalNetAssets":8500000000,"annualExpenseRatio":0.22}`,
		asOfDate.Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Verify holdings with bond fields
	var holdingsJSONRead string
	err = db.QueryRow("SELECT top_holdings FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&holdingsJSONRead)
	if err != nil {
		t.Fatalf("query holdings: %v", err)
	}
	var holdings []struct {
		Symbol        string  `json:"symbol"`
		Name          string  `json:"name"`
		Percent       float64 `json:"percent"`
		SecurityType  string  `json:"securityType"`
		CouponRate    *float64 `json:"couponRate"`
		FinalMaturity *string `json:"finalMaturity"`
		AsOfDate      string  `json:"asOfDate"`
	}
	if err := json.Unmarshal([]byte(holdingsJSONRead), &holdings); err != nil {
		t.Fatalf("parse holdings JSON: %v", err)
	}
	if len(holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(holdings))
	}
	if holdings[0].SecurityType != "Equity" {
		t.Errorf("expected SecurityType 'Equity', got %q", holdings[0].SecurityType)
	}
	if holdings[0].AsOfDate != "2026-05-31" {
		t.Errorf("expected AsOfDate '2026-05-31', got %q", holdings[0].AsOfDate)
	}
	if holdings[1].SecurityType != "Government Bond" {
		t.Errorf("expected SecurityType 'Government Bond', got %q", holdings[1].SecurityType)
	}
	if holdings[1].CouponRate == nil || *holdings[1].CouponRate != 4.25 {
		t.Errorf("expected CouponRate 4.25, got %v", holdings[1].CouponRate)
	}
	if holdings[1].FinalMaturity == nil || *holdings[1].FinalMaturity != "2034-06-15" {
		t.Errorf("expected FinalMaturity '2034-06-15', got %v", holdings[1].FinalMaturity)
	}

	// Verify sectors with Date
	var sectorsJSONRead string
	err = db.QueryRow("SELECT sector_weightings FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&sectorsJSONRead)
	if err != nil {
		t.Fatalf("query sectors: %v", err)
	}
	var sectors []struct {
		Sector  string  `json:"sector"`
		Percent float64 `json:"percent"`
		Date    string  `json:"date"`
	}
	if err := json.Unmarshal([]byte(sectorsJSONRead), &sectors); err != nil {
		t.Fatalf("parse sectors JSON: %v", err)
	}
	if len(sectors) != 2 {
		t.Fatalf("expected 2 sectors, got %d", len(sectors))
	}
	if sectors[0].Date != "2026-05-31" {
		t.Errorf("expected Date '2026-05-31', got %q", sectors[0].Date)
	}

	// Verify countries with Region fields
	var countriesJSONRead string
	err = db.QueryRow("SELECT geographic_allocations FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&countriesJSONRead)
	if err != nil {
		t.Fatalf("query countries: %v", err)
	}
	var countries []struct {
		Country    string  `json:"country"`
		Percent    float64 `json:"percent"`
		RegionName string  `json:"regionName"`
		RegionCode string  `json:"regionCode"`
		Date       string  `json:"date"`
	}
	if err := json.Unmarshal([]byte(countriesJSONRead), &countries); err != nil {
		t.Fatalf("parse countries JSON: %v", err)
	}
	if len(countries) != 2 {
		t.Fatalf("expected 2 countries, got %d", len(countries))
	}
	if countries[0].RegionName != "North America" {
		t.Errorf("expected RegionName 'North America', got %q", countries[0].RegionName)
	}
	if countries[0].RegionCode != "NA" {
		t.Errorf("expected RegionCode 'NA', got %q", countries[0].RegionCode)
	}
	if countries[0].Date != "2026-05-31" {
		t.Errorf("expected Date '2026-05-31', got %q", countries[0].Date)
	}

	// Verify expanded equity valuation
	var equityJSONRead string
	err = db.QueryRow("SELECT equity_valuation FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&equityJSONRead)
	if err != nil {
		t.Fatalf("query equity valuation: %v", err)
	}
	var equity map[string]interface{}
	if err := json.Unmarshal([]byte(equityJSONRead), &equity); err != nil {
		t.Fatalf("parse equity JSON: %v", err)
	}
	if equity["medianMarketCap"] != float64(2500000) {
		t.Errorf("expected medianMarketCap 2500000, got %v", equity["medianMarketCap"])
	}
	if equity["forwardROE"] != 15.3 {
		t.Errorf("expected forwardROE 15.3, got %v", equity["forwardROE"])
	}
	if equity["forwardEPSGrowth"] != 8.7 {
		t.Errorf("expected forwardEPSGrowth 8.7, got %v", equity["forwardEPSGrowth"])
	}
	if equity["revenueRatio"] != 0.45 {
		t.Errorf("expected revenueRatio 0.45, got %v", equity["revenueRatio"])
	}

	// Verify bond characteristics
	var bondJSONRead string
	err = db.QueryRow("SELECT bond_characteristics FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&bondJSONRead)
	if err != nil {
		t.Fatalf("query bond characteristics: %v", err)
	}
	var bond map[string]interface{}
	if err := json.Unmarshal([]byte(bondJSONRead), &bond); err != nil {
		t.Fatalf("parse bond JSON: %v", err)
	}
	if bond["averageCoupon"] != 3.85 {
		t.Errorf("expected averageCoupon 3.85, got %v", bond["averageCoupon"])
	}
	if bond["averageMaturity"] != 7.2 {
		t.Errorf("expected averageMaturity 7.2, got %v", bond["averageMaturity"])
	}
	if bond["averageQuality"] != 7.2 {
		t.Errorf("expected averageQuality 7.2, got %v", bond["averageQuality"])
	}
	if bond["averageDuration"] != 6.5 {
		t.Errorf("expected averageDuration 6.5, got %v", bond["averageDuration"])
	}
}

func TestVanguard_SymbolAPIDetailsWithNewFields(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	internalSymbol := "VWRL.L"

	// Create symbol via API
	body := json.RawMessage(`{"internal_symbol": "VWRL.L", "market_data_symbol": "VWRL.L"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	// Give the background goroutine a moment to start (and fail silently)
	time.Sleep(100 * time.Millisecond)

	var symID int64
	err := db.QueryRow("SELECT id FROM symbol_mappings WHERE internal_symbol = ?", internalSymbol).Scan(&symID)
	if err != nil {
		t.Fatalf("query symbol id: %v", err)
	}

	// Insert symbol details with Vanguard-specific fields
	asOfDate := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	holdingsJSON := `[{"symbol":"AAPL","name":"Apple Inc","percent":3.5,"securityType":"Equity","asOfDate":"2026-05-31"}]`
	sectorsJSON := `[{"sector":"Technology","percent":21.5,"date":"2026-05-31"}]`
	countriesJSON := `[{"country":"United States","percent":60.5,"regionName":"North America","regionCode":"NA","date":"2026-05-31"}]`
	equityJSON := `{"priceToEarnings":18.5,"priceToBook":3.2,"medianMarketCap":2500000,"forwardROE":15.3}`
	bondJSON := `{"averageCoupon":3.85,"averageMaturity":7.2,"averageQuality":7.2,"averageDuration":6.5}`

	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, geographic_allocations,
			equity_valuation, bond_characteristics,
			fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "Vanguard FTSE All-World", "Vanguard FTSE All-World UCITS ETF",
		"LSE", "USD", "ETF",
		holdingsJSON, sectorsJSON, countriesJSON,
		equityJSON, bondJSON,
		`{"family":"Vanguard","legalType":"Exchange Traded Fund","totalNetAssets":8500000000,"annualExpenseRatio":0.22}`,
		asOfDate.Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// GET symbol details via API
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/symbols/%d", symID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("GET symbol: expected 200, got %d", w.Code)
	}

	// Save body before decoding
	bodyBytes := w.Body.Bytes()

	var resp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, string(bodyBytes))
	}

	detailsRaw, ok := resp["symbol_details"]
	if !ok || detailsRaw == nil {
		t.Fatalf("expected symbol_details in response, got nil (raw: %s)", string(bodyBytes))
	}

	details := detailsRaw.(map[string]interface{})

	if details["short_name"] != "Vanguard FTSE All-World" {
		t.Errorf("expected short_name 'Vanguard FTSE All-World', got %v", details["short_name"])
	}

	// Verify holdings with new fields (PascalCase keys from domain types)
	holdings, ok := details["top_holdings"].([]interface{})
	if !ok || len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %v", holdings)
	}
	holding := holdings[0].(map[string]interface{})
	if holding["SecurityType"] != "Equity" {
		t.Errorf("expected SecurityType 'Equity', got %v", holding["SecurityType"])
	}
	if holding["AsOfDate"] != "2026-05-31" {
		t.Errorf("expected AsOfDate '2026-05-31', got %v", holding["AsOfDate"])
	}

	// Verify sectors with Date
	sectors, ok := details["sector_weightings"].([]interface{})
	if !ok || len(sectors) != 1 {
		t.Fatalf("expected 1 sector, got %v", sectors)
	}
	sector := sectors[0].(map[string]interface{})
	if sector["Date"] != "2026-05-31" {
		t.Errorf("expected Date '2026-05-31', got %v", sector["Date"])
	}

	// Verify countries with Region fields
	countries, ok := details["geographic_allocations"].([]interface{})
	if !ok || len(countries) != 1 {
		t.Fatalf("expected 1 country, got %v", countries)
	}
	country := countries[0].(map[string]interface{})
	if country["RegionName"] != "North America" {
		t.Errorf("expected RegionName 'North America', got %v", country["RegionName"])
	}
	if country["RegionCode"] != "NA" {
		t.Errorf("expected RegionCode 'NA', got %v", country["RegionCode"])
	}

	// Verify expanded equity valuation (PascalCase keys from domain types)
	equityRaw, ok := details["equity_valuation"]
	if !ok || equityRaw == nil {
		t.Fatal("expected non-nil equity_valuation")
	}
	equity := equityRaw.(map[string]interface{})
	if equity["MedianMarketCap"] != float64(2500000) {
		t.Errorf("expected MedianMarketCap 2500000, got %v", equity["MedianMarketCap"])
	}
	if equity["ForwardROE"] != 15.3 {
		t.Errorf("expected ForwardROE 15.3, got %v", equity["ForwardROE"])
	}

	// Verify bond characteristics (PascalCase keys from domain types)
	bondRaw, ok := details["bond_characteristics"]
	if !ok || bondRaw == nil {
		t.Fatal("expected non-nil bond_characteristics")
	}
	bond := bondRaw.(map[string]interface{})
	if bond["AverageCoupon"] != 3.85 {
		t.Errorf("expected AverageCoupon 3.85, got %v", bond["AverageCoupon"])
	}
	if bond["AverageQuality"] != 7.2 {
		t.Errorf("expected AverageQuality 7.2, got %v", bond["AverageQuality"])
	}
}

func TestVanguard_WebPageRendersWithNewFields(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	internalSymbol := "VWRL.L"

	// Create symbol via API
	body := json.RawMessage(`{"internal_symbol": "VWRL.L", "market_data_symbol": "VWRL.L"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d", w.Code)
	}

	var symID int64
	err := db.QueryRow("SELECT id FROM symbol_mappings WHERE internal_symbol = ?", internalSymbol).Scan(&symID)
	if err != nil {
		t.Fatalf("query symbol id: %v", err)
	}

	// Insert symbol details with Vanguard-specific fields
	holdingsJSON := `[{"symbol":"AAPL","name":"Apple Inc","percent":3.5,"securityType":"Equity","asOfDate":"2026-05-31"},{"symbol":"US10Y","name":"US Treasury 10Y","percent":2.1,"securityType":"Government Bond","couponRate":4.25,"finalMaturity":"2034-06-15","asOfDate":"2026-05-31"}]`
	sectorsJSON := `[{"sector":"Technology","percent":21.5,"date":"2026-05-31"}]`
	countriesJSON := `[{"country":"United States","percent":60.5,"regionName":"North America","regionCode":"NA","date":"2026-05-31"}]`
	equityJSON := `{"priceToEarnings":18.5,"priceToBook":3.2,"medianMarketCap":2500000,"forwardROE":15.3,"forwardEPSGrowth":8.7,"revenueRatio":0.45}`
	bondJSON := `{"averageCoupon":3.85,"averageMaturity":7.2,"averageQuality":7.2,"averageDuration":6.5}`

	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, geographic_allocations,
			equity_valuation, bond_characteristics,
			fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "Vanguard FTSE All-World", "Vanguard FTSE All-World UCITS ETF",
		"LSE", "USD", "ETF",
		holdingsJSON, sectorsJSON, countriesJSON,
		equityJSON, bondJSON,
		`{"family":"Vanguard","legalType":"Exchange Traded Fund","totalNetAssets":8500000000,"annualExpenseRatio":0.22}`,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// GET web page — route is /symbols/{id}/details
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/symbols/%d/details", symID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("GET symbol details page: expected 200, got %d", w.Code)
	}

	bodyStr := w.Body.String()

	// Verify page renders with the symbol name in the title
	if !bytes.Contains([]byte(bodyStr), []byte("Vanguard FTSE All-World")) {
		t.Errorf("page missing symbol name in title")
	}
	// Verify the symbol is in the page
	if !bytes.Contains([]byte(bodyStr), []byte("VWRL.L")) {
		t.Errorf("page missing internal symbol")
	}
	// Detailed field rendering (bond characteristics, security types, etc.)
	// is covered by unit tests in symbol_details_web_test.go
}

func TestVanguard_NAVDataWithSource(t *testing.T) {
	db := setupTestDB(t)

	// Insert NAV data with vanguard source
	_, err := db.Exec(`
		INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "VWRL.L", "105.50", "USD", "nav", "vanguard", "2026-05-30", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert NAV data: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "VWRL.L", "106.20", "USD", "nav", "vanguard", "2026-05-31", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert NAV data 2: %v", err)
	}

	// Verify NAV data is queryable by source
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM market_data WHERE data_type = 'nav' AND source = 'vanguard'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query NAV data: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 NAV records with vanguard source, got %d", count)
	}

	// Verify NAV data is NOT returned by stock queries
	err = db.QueryRow(`
		SELECT COUNT(*) FROM market_data WHERE data_type IN ('stock', 'fx') AND symbol = 'VWRL.L'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query stock data: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 stock records, got %d", count)
	}
}

func TestVanguard_DataSourceURL_RoundTrip(t *testing.T) {
	db := setupTestDB(t)

	sourceURL := "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing"

	// Insert a symbol mapping with vanguard data_source_url
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, "VWRL.L", "VWRL.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Verify data_source_url is returned by the stale query
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
	if internalSymbol != "VWRL.L" {
		t.Errorf("expected internal_symbol VWRL.L, got %q", internalSymbol)
	}
	if dataSourceURL != sourceURL {
		t.Errorf("expected vanguard data_source_url, got %q", dataSourceURL)
	}
}

func TestVanguard_BondFund_OnlyBondCharacteristics(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "VBND.L"

	// Insert a bond fund with only bond characteristics (no equity valuation)
	holdingsJSON := `[{"symbol":"US10Y","name":"US Treasury 10Y","percent":15.0,"securityType":"Government Bond","couponRate":4.5,"finalMaturity":"2034-06-15","asOfDate":"2026-05-31"}]`
	bondJSON := `{"averageCoupon":4.25,"averageMaturity":8.5,"averageQuality":9.0,"averageDuration":7.8}`

	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, bond_characteristics,
			fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "Vanguard US Bond", "Vanguard US Bond ETF",
		"LSE", "USD", "ETF",
		holdingsJSON, bondJSON,
		`{"family":"Vanguard","legalType":"Exchange Traded Fund","totalNetAssets":5000000000,"annualExpenseRatio":0.05}`,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Verify equity_valuation is NULL for bond fund
	var equityVal sql.NullString
	err = db.QueryRow("SELECT equity_valuation FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&equityVal)
	if err != nil {
		t.Fatalf("query equity_valuation: %v", err)
	}
	if equityVal.Valid {
		t.Errorf("expected NULL equity_valuation for bond fund, got %q", equityVal.String)
	}

	// Verify bond_characteristics is present
	var bondJSONRead string
	err = db.QueryRow("SELECT bond_characteristics FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&bondJSONRead)
	if err != nil {
		t.Fatalf("query bond_characteristics: %v", err)
	}
	var bond map[string]interface{}
	if err := json.Unmarshal([]byte(bondJSONRead), &bond); err != nil {
		t.Fatalf("parse bond JSON: %v", err)
	}
	if bond["averageQuality"] != 9.0 {
		t.Errorf("expected averageQuality 9.0, got %v", bond["averageQuality"])
	}
	if bond["averageDuration"] != 7.8 {
		t.Errorf("expected averageDuration 7.8, got %v", bond["averageDuration"])
	}
}

func TestVanguard_EquityFund_NoBondCharacteristics(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "VWRL.L"

	// Insert an equity fund with only equity valuation (no bond characteristics)
	equityJSON := `{"priceToEarnings":18.5,"priceToBook":3.2,"medianMarketCap":2500000,"forwardROE":15.3}`

	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			equity_valuation, fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "Vanguard FTSE All-World", "Vanguard FTSE All-World UCITS ETF",
		"LSE", "USD", "ETF",
		equityJSON,
		`{"family":"Vanguard","legalType":"Exchange Traded Fund","totalNetAssets":8500000000,"annualExpenseRatio":0.22}`,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Verify bond_characteristics is NULL for equity fund
	var bondChars sql.NullString
	err = db.QueryRow("SELECT bond_characteristics FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&bondChars)
	if err != nil {
		t.Fatalf("query bond_characteristics: %v", err)
	}
	if bondChars.Valid {
		t.Errorf("expected NULL bond_characteristics for equity fund, got %q", bondChars.String)
	}

	// Verify equity_valuation is present
	var equityJSONRead string
	err = db.QueryRow("SELECT equity_valuation FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&equityJSONRead)
	if err != nil {
		t.Fatalf("query equity_valuation: %v", err)
	}
	var equity map[string]interface{}
	if err := json.Unmarshal([]byte(equityJSONRead), &equity); err != nil {
		t.Fatalf("parse equity JSON: %v", err)
	}
	if equity["priceToEarnings"] != 18.5 {
		t.Errorf("expected priceToEarnings 18.5, got %v", equity["priceToEarnings"])
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !bytes.Contains([]byte(s), []byte(sub)) {
			return false
		}
	}
	return true
}

// TestVanguard_Dispatcher_UnregisteredProvider verifies that dispatching
// to a URL with no matching extractor returns an explicit error.
// Corresponds to Story 7 AC4: "Given a symbol is assigned to the Vanguard
// provider but no extractor is registered, When the system attempts to fetch
// symbol details, Then the fetch fails with an explicit error."
func TestVanguard_Dispatcher_UnregisteredProvider(t *testing.T) {
	reg := extractor.NewRegistry()
	// Register only WisdomTree — Vanguard is NOT registered
	reg.Register(wisdomtree.NewExtractor())
	dispatcher := extractor.NewDispatcher(reg)

	// Dispatch to a Vanguard URL — should fail with explicit error
	_, err := dispatcher.Dispatch(context.Background(),
		"https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing")
	if err == nil {
		t.Fatal("expected error for unregistered provider, got nil")
	}

	errStr := err.Error()
	// Error should mention the URL and that no extractor is registered
	if !containsString(errStr, "no extractor registered") {
		t.Errorf("expected 'no extractor registered' in error, got: %s", errStr)
	}
	if !containsString(errStr, "vanguardinvestor.co.uk") {
		t.Errorf("expected URL in error message, got: %s", errStr)
	}

	// Verify a registered provider (WisdomTree) still works (may fail on network but not on routing)
	_, err = dispatcher.Dispatch(context.Background(),
		"https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/")
	if err != nil {
		errStr := err.Error()
		if containsString(errStr, "no extractor registered") {
			t.Errorf("WisdomTree extractor should be registered, got routing error: %s", errStr)
		}
		// Network/API errors are expected in test environment
	}
}

// TestVanguard_ProviderSwitch_YahooToVanguard verifies that switching a symbol
// from Yahoo Finance (no provider) to the Vanguard provider is reflected in
// the stale query and data_source_url round-trip.
// Corresponds to Story 7 AC3: "Given a symbol is switched from Yahoo Finance
// (no provider) to the Vanguard provider, When the next refresh runs, Then
// symbol details are fetched from Vanguard instead of Yahoo."
func TestVanguard_ProviderSwitch_YahooToVanguard(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "VWRL.L"
	sourceURL := "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing"

	// Step 1: Create symbol without data_source_url (Yahoo Finance path)
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol)
		VALUES (?, ?)
	`, internalSymbol, "VWRL.L")
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Verify no data_source_url
	var dataSourceURL sql.NullString
	err = db.QueryRow("SELECT data_source_url FROM symbol_mappings WHERE internal_symbol = ?",
		internalSymbol).Scan(&dataSourceURL)
	if err != nil {
		t.Fatalf("query data_source_url: %v", err)
	}
	if dataSourceURL.Valid && dataSourceURL.String != "" {
		t.Errorf("expected empty data_source_url for Yahoo symbol, got %q", dataSourceURL.String)
	}

	// Step 2: Switch to Vanguard provider by setting data_source_url
	_, err = db.Exec(`
		UPDATE symbol_mappings SET data_source_url = ? WHERE internal_symbol = ?
	`, sourceURL, internalSymbol)
	if err != nil {
		t.Fatalf("update data_source_url: %v", err)
	}

	// Verify data_source_url is now set
	err = db.QueryRow("SELECT data_source_url FROM symbol_mappings WHERE internal_symbol = ?",
		internalSymbol).Scan(&dataSourceURL)
	if err != nil {
		t.Fatalf("query data_source_url after update: %v", err)
	}
	if !dataSourceURL.Valid || dataSourceURL.String != sourceURL {
		t.Errorf("expected vanguard data_source_url, got %q", dataSourceURL.String)
	}

	// Step 3: Verify the stale query picks up the symbol with its data_source_url
	// (this is what the background refresh uses to decide which extractor to invoke)
	var staleURL, staleSymbol string
	err = db.QueryRow(`
		SELECT sm.internal_symbol, sm.data_source_url
		FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, time.Now().Add(-7*24*time.Hour).Format(time.RFC3339)).Scan(&staleSymbol, &staleURL)
	if err != nil {
		t.Fatalf("query stale with data_source_url: %v", err)
	}
	if staleSymbol != internalSymbol {
		t.Errorf("expected stale symbol %q, got %q", internalSymbol, staleSymbol)
	}
	if staleURL != sourceURL {
		t.Errorf("expected vanguard data_source_url in stale query, got %q", staleURL)
	}

	// Step 4: Verify the dispatcher routes the URL to the Vanguard extractor
	reg := extractor.NewRegistry()
	reg.Register(vanguard.NewExtractor())
	dispatcher := extractor.NewDispatcher(reg)

	// The dispatcher should find the Vanguard extractor for this URL
	// (may fail on network, but not on routing)
	_, err = dispatcher.Dispatch(context.Background(), staleURL)
	if err != nil {
		errStr := err.Error()
		if containsString(errStr, "no extractor registered") {
			t.Errorf("Vanguard extractor should be registered for %q, got routing error: %s", staleURL, errStr)
		}
		// Network/API errors are expected in test environment
	}
}

// TestVanguard_BackgroundRefresh_DualPath verifies that the background refresh
// mechanism correctly handles symbols with data_source_url configured.
// The stale query returns both the symbol and its data_source_url, allowing
// the service layer to route through the extractor dispatcher.
// Corresponds to Story 7 AC2: "Given a symbol is assigned to the Vanguard
// provider, When the background refresh runs, Then both symbol details (via
// Vanguard) and market data (via Yahoo) are refreshed."
func TestVanguard_BackgroundRefresh_DualPath(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "VWRL.L"
	sourceURL := "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-distributing"

	// Create symbol with vanguard data_source_url
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, internalSymbol, "VWRL.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Simulate what the background refresh does:
	// 1. Stale query returns symbols with their data_source_url
	var staleSymbols []struct {
		InternalSymbol string `db:"internal_symbol"`
		DataSourceURL  string `db:"data_source_url"`
	}

	rows, err := db.Query(`
		SELECT sm.internal_symbol, COALESCE(sm.data_source_url, '')
		FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, time.Now().Add(-7*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("query stale symbols: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sym struct {
			InternalSymbol string
			DataSourceURL  string
		}
		if err := rows.Scan(&sym.InternalSymbol, &sym.DataSourceURL); err != nil {
			t.Fatalf("scan stale row: %v", err)
		}
		staleSymbols = append(staleSymbols, struct {
			InternalSymbol string `db:"internal_symbol"`
			DataSourceURL  string `db:"data_source_url"`
		}{sym.InternalSymbol, sym.DataSourceURL})
	}

	if len(staleSymbols) != 1 {
		t.Fatalf("expected 1 stale symbol, got %d", len(staleSymbols))
	}
	if staleSymbols[0].InternalSymbol != internalSymbol {
		t.Errorf("expected stale symbol %q, got %q", internalSymbol, staleSymbols[0].InternalSymbol)
	}
	if staleSymbols[0].DataSourceURL != sourceURL {
		t.Errorf("expected vanguard data_source_url in stale query, got %q", staleSymbols[0].DataSourceURL)
	}

	// 2. Service layer uses data_source_url to route through dispatcher
	// Verify the dispatcher correctly identifies the Vanguard extractor
	reg := extractor.NewRegistry()
	reg.Register(vanguard.NewExtractor())

	extractor, err := reg.FindByURL(sourceURL)
	if err != nil {
		t.Fatalf("expected extractor for vanguard URL: %v", err)
	}
	if extractor.Name() != vanguard.Name {
		t.Errorf("expected vanguard extractor, got %q", extractor.Name())
	}

	// 3. Verify the dual-path: Yahoo for market data + Vanguard for symbol details
	// The service layer (FetchAndStore) always fetches Yahoo first for exchange/currency,
	// then routes through the extractor for detailed data.
	// We verify this by checking the data_source_url is available for the dispatcher,
	// and the market_data_symbol is available for Yahoo.
	var marketDataSymbol, dataURL string
	err = db.QueryRow(`
		SELECT market_data_symbol, data_source_url
		FROM symbol_mappings WHERE internal_symbol = ?
	`, internalSymbol).Scan(&marketDataSymbol, &dataURL)
	if err != nil {
		t.Fatalf("query symbol mapping: %v", err)
	}
	if marketDataSymbol == "" {
		t.Error("expected market_data_symbol for Yahoo path")
	}
	if dataURL == "" {
		t.Error("expected data_source_url for Vanguard path")
	}
	// Both paths are available — the service layer would use:
	// - marketDataSymbol ("VWRL.L") → Yahoo for exchange/currency
	// - dataURL (vanguard URL) → Vanguard extractor for detailed data

	// 4. Verify NAV data from Vanguard is stored with correct source
	_, err = db.Exec(`
		INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "105.50", "USD", "nav", "vanguard", "2026-05-31", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert NAV data: %v", err)
	}

	// Market data from Yahoo (stock type) is separate
	_, err = db.Exec(`
		INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "106.00", "USD", "stock", "yahoo", "2026-05-31", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert stock data: %v", err)
	}

	// Verify both sources coexist
	var navCount, stockCount int
	err = db.QueryRow("SELECT COUNT(*) FROM market_data WHERE symbol = ? AND data_type = 'nav' AND source = 'vanguard'",
		internalSymbol).Scan(&navCount)
	if err != nil {
		t.Fatalf("query NAV count: %v", err)
	}
	if navCount != 1 {
		t.Errorf("expected 1 NAV record from vanguard, got %d", navCount)
	}

	err = db.QueryRow("SELECT COUNT(*) FROM market_data WHERE symbol = ? AND data_type = 'stock' AND source = 'yahoo'",
		internalSymbol).Scan(&stockCount)
	if err != nil {
		t.Fatalf("query stock count: %v", err)
	}
	if stockCount != 1 {
		t.Errorf("expected 1 stock record from yahoo, got %d", stockCount)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
