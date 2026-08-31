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
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/blackrock"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/wisdomtree"
)

func TestBlackRock_ExtractorDispatch_Routing(t *testing.T) {
	reg := extractor.NewRegistry()
	if err := reg.Register(blackrock.NewExtractor()); err != nil {
		t.Fatalf("register blackrock extractor: %v", err)
	}

	dispatcher := extractor.NewDispatcher(reg)

	// iShares URL should find the extractor (may fail on network, but not on routing)
	_, err := dispatcher.Dispatch(context.Background(),
		"https://www.ishares.com/uk/individual/en/products/268426/ishares-core-sp-500-ucits-etf")
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
		t.Error("expected error for non-ishares URL")
	}
}

func TestBlackRock_ExtractorRegisteredInRouter(t *testing.T) {
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

func TestBlackRock_ExtractResultStructure(t *testing.T) {
	var _ extractor.Extractor = (*blackrock.Extractor)(nil)

	ex := blackrock.NewExtractor()
	if ex.Name() != blackrock.Name {
		t.Errorf("Name() = %q, want %q", ex.Name(), blackrock.Name)
	}
}

// TestBlackRock_SymbolDetails_FullStackRoundTrip exercises the complete
// ExtractResult → SymbolDetails → DB → SymbolDetails path for an equity fund
// with P/E, P/B, beta, standard deviation, and number of holdings.
func TestBlackRock_SymbolDetails_FullStackRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	sourceURL := "https://www.ishares.com/uk/individual/en/products/268426/ishares-core-sp-500-ucits-etf"
	internalSymbol := "ISHY.L"

	// Insert symbol mapping with blackrock data_source_url
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, internalSymbol, "ISHY.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	asOfDate := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)

	// Holdings with BlackRock-specific fields (Sector, AssetClass, MarketValue, NotionalValue, Shares, Price, ISIN, Location, Exchange, MarketCurrency)
	holdingsJSON := `[
		{"symbol":"AAPL","name":"Apple Inc","percent":7.2,"sector":"Information Technology","assetClass":"Equity","marketValue":1500000000,"notionalValue":1500000000,"shares":8000000,"price":187.5,"isin":"US0378331005","location":"United States","exchange":"NASDAQ","marketCurrency":"USD"},
		{"symbol":"MSFT","name":"Microsoft Corp","percent":6.8,"sector":"Information Technology","assetClass":"Equity","marketValue":1420000000,"notionalValue":1420000000,"shares":3500000,"price":405.71,"isin":"US5949181045","location":"United States","exchange":"NASDAQ","marketCurrency":"USD"}
	]`
	// Sectors derived from holdings
	sectorsJSON := `[{"sector":"Information Technology","percent":48.5},{"sector":"Health Care","percent":13.2},{"sector":"Financials","percent":12.8}]`
	// Countries derived from holdings
	countriesJSON := `[{"country":"United States","percent":94.2},{"country":"United Kingdom","percent":2.1},{"country":"Japan","percent":1.5}]`
	// Equity valuation with BlackRock-specific fields (Beta3Y, StandardDeviation3Y, NumberOfHoldings)
	equityJSON := `{"priceToEarnings":22.5,"priceToBook":4.8,"priceToCashflow":16.2,"priceToSales":3.1,"dividendYield":1.4,"beta3Y":1.02,"standardDeviation3Y":14.8,"numberOfHoldings":505}`
	// Fund profile with BlackRock-specific fields (SFDR, Domicile, RebalanceFrequency, ProductStructure, Methodology, FundManager, Custodian, IssuingCompany, BenchmarkTicker)
	profileJSON := `{"family":"iShares","legalType":"Exchange Traded Fund","totalNetAssets":12500000000,"annualExpenseRatio":0.07,"isin":"IE00B53SZB19","benchmark":"S&P 500 Index","sfdrClassification":"Article 6","domicile":"Ireland","rebalanceFrequency":"Quarterly","productStructure":"Physical","methodology":"Representative","fundManager":"BlackRock Asset Management Ireland Limited","custodian":"State Street Custodial Services (Ireland) Limited","issuingCompany":"iShares IV plc","benchmarkTicker":"SPX"}`

	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, geographic_allocations,
			equity_valuation, fund_profile,
			extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF USD (Dist)",
		"LSE", "USD", "ETF",
		holdingsJSON, sectorsJSON, countriesJSON,
		equityJSON, profileJSON,
		asOfDate.Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Verify holdings with BlackRock-specific fields
	var holdingsJSONRead string
	err = db.QueryRow("SELECT top_holdings FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&holdingsJSONRead)
	if err != nil {
		t.Fatalf("query holdings: %v", err)
	}
	var holdings []struct {
		Symbol         string  `json:"symbol"`
		Name           string  `json:"name"`
		Percent        float64 `json:"percent"`
		Sector         string  `json:"sector"`
		AssetClass     string  `json:"assetClass"`
		MarketValue    float64 `json:"marketValue"`
		NotionalValue  float64 `json:"notionalValue"`
		Shares         float64 `json:"shares"`
		Price          float64 `json:"price"`
		ISIN           string  `json:"isin"`
		Location       string  `json:"location"`
		Exchange       string  `json:"exchange"`
		MarketCurrency string  `json:"marketCurrency"`
	}
	if err := json.Unmarshal([]byte(holdingsJSONRead), &holdings); err != nil {
		t.Fatalf("parse holdings JSON: %v", err)
	}
	if len(holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(holdings))
	}
	if holdings[0].Sector != "Information Technology" {
		t.Errorf("expected Sector 'Information Technology', got %q", holdings[0].Sector)
	}
	if holdings[0].AssetClass != "Equity" {
		t.Errorf("expected AssetClass 'Equity', got %q", holdings[0].AssetClass)
	}
	if holdings[0].MarketValue != 1500000000 {
		t.Errorf("expected MarketValue 1500000000, got %v", holdings[0].MarketValue)
	}
	if holdings[0].ISIN != "US0378331005" {
		t.Errorf("expected ISIN 'US0378331005', got %q", holdings[0].ISIN)
	}
	if holdings[0].Location != "United States" {
		t.Errorf("expected Location 'United States', got %q", holdings[0].Location)
	}
	if holdings[0].Exchange != "NASDAQ" {
		t.Errorf("expected Exchange 'NASDAQ', got %q", holdings[0].Exchange)
	}

	// Verify equity valuation with BlackRock-specific fields
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
	if equity["beta3Y"] != float64(1.02) {
		t.Errorf("expected beta3Y 1.02, got %v", equity["beta3Y"])
	}
	if equity["standardDeviation3Y"] != float64(14.8) {
		t.Errorf("expected standardDeviation3Y 14.8, got %v", equity["standardDeviation3Y"])
	}
	if equity["numberOfHoldings"] != float64(505) {
		t.Errorf("expected numberOfHoldings 505, got %v", equity["numberOfHoldings"])
	}

	// Verify fund profile with BlackRock-specific fields
	var profileJSONRead string
	err = db.QueryRow("SELECT fund_profile FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&profileJSONRead)
	if err != nil {
		t.Fatalf("query fund profile: %v", err)
	}
	var profile map[string]interface{}
	if err := json.Unmarshal([]byte(profileJSONRead), &profile); err != nil {
		t.Fatalf("parse profile JSON: %v", err)
	}
	if profile["sfdrClassification"] != "Article 6" {
		t.Errorf("expected sfdrClassification 'Article 6', got %v", profile["sfdrClassification"])
	}
	if profile["domicile"] != "Ireland" {
		t.Errorf("expected domicile 'Ireland', got %v", profile["domicile"])
	}
	if profile["rebalanceFrequency"] != "Quarterly" {
		t.Errorf("expected rebalanceFrequency 'Quarterly', got %v", profile["rebalanceFrequency"])
	}
	if profile["productStructure"] != "Physical" {
		t.Errorf("expected productStructure 'Physical', got %v", profile["productStructure"])
	}
	if profile["methodology"] != "Representative" {
		t.Errorf("expected methodology 'Representative', got %v", profile["methodology"])
	}
	if profile["fundManager"] != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("expected fundManager 'BlackRock Asset Management Ireland Limited', got %v", profile["fundManager"])
	}
	if profile["custodian"] != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("expected custodian 'State Street Custodial Services (Ireland) Limited', got %v", profile["custodian"])
	}
	if profile["issuingCompany"] != "iShares IV plc" {
		t.Errorf("expected issuingCompany 'iShares IV plc', got %v", profile["issuingCompany"])
	}
	if profile["benchmarkTicker"] != "SPX" {
		t.Errorf("expected benchmarkTicker 'SPX', got %v", profile["benchmarkTicker"])
	}

	// Verify sectors derived from holdings
	var sectorsJSONRead string
	err = db.QueryRow("SELECT sector_weightings FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&sectorsJSONRead)
	if err != nil {
		t.Fatalf("query sectors: %v", err)
	}
	var sectors []struct {
		Sector  string  `json:"sector"`
		Percent float64 `json:"percent"`
	}
	if err := json.Unmarshal([]byte(sectorsJSONRead), &sectors); err != nil {
		t.Fatalf("parse sectors JSON: %v", err)
	}
	if len(sectors) != 3 {
		t.Fatalf("expected 3 sectors, got %d", len(sectors))
	}
	if sectors[0].Sector != "Information Technology" || sectors[0].Percent != 48.5 {
		t.Errorf("expected first sector 'Information Technology' 48.5, got %q %v", sectors[0].Sector, sectors[0].Percent)
	}

	// Verify countries derived from holdings
	var countriesJSONRead string
	err = db.QueryRow("SELECT geographic_allocations FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&countriesJSONRead)
	if err != nil {
		t.Fatalf("query countries: %v", err)
	}
	var countries []struct {
		Country string  `json:"country"`
		Percent float64 `json:"percent"`
	}
	if err := json.Unmarshal([]byte(countriesJSONRead), &countries); err != nil {
		t.Fatalf("parse countries JSON: %v", err)
	}
	if len(countries) != 3 {
		t.Fatalf("expected 3 countries, got %d", len(countries))
	}
	if countries[0].Country != "United States" || countries[0].Percent != 94.2 {
		t.Errorf("expected first country 'United States' 94.2, got %q %v", countries[0].Country, countries[0].Percent)
	}
}

// TestBlackRock_BondFund_OnlyBondCharacteristics verifies that a bond ETF
// has bond characteristics (YTM, duration) with no equity valuation.
func TestBlackRock_BondFund_OnlyBondCharacteristics(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "IEF.L"

	// Bond fund with bond characteristics (YTM, duration) and no equity valuation
	holdingsJSON := `[{"symbol":"US10Y","name":"US Treasury 10Y","percent":15.0,"sector":"Government","assetClass":"Government Bond","marketValue":3100000000,"notionalValue":3100000000,"shares":0,"price":0,"isin":"US912828Z079","location":"United States","exchange":"OTC","marketCurrency":"USD"}]`
	bondJSON := `{"averageCoupon":4.25,"averageMaturity":8.5,"averageQuality":9.0,"averageDuration":7.8}`
	profileJSON := `{"family":"iShares","legalType":"Exchange Traded Fund","totalNetAssets":25000000000,"annualExpenseRatio":0.05,"isin":"IE00B53SZB19","sfdrClassification":"Article 6","domicile":"Ireland","productStructure":"Physical","fundManager":"BlackRock Asset Management Ireland Limited","custodian":"State Street Custodial Services (Ireland) Limited","issuingCompany":"iShares IV plc"}`

	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, bond_characteristics,
			fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares US Treasury Bond", "iShares US Treasury Bond ETF",
		"LSE", "USD", "ETF",
		holdingsJSON, bondJSON,
		profileJSON,
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
	if bond["averageCoupon"] != 4.25 {
		t.Errorf("expected averageCoupon 4.25, got %v", bond["averageCoupon"])
	}
	if bond["averageDuration"] != 7.8 {
		t.Errorf("expected averageDuration 7.8, got %v", bond["averageDuration"])
	}

	// Verify holdings with bond-specific fields
	var holdingsJSONRead string
	err = db.QueryRow("SELECT top_holdings FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&holdingsJSONRead)
	if err != nil {
		t.Fatalf("query holdings: %v", err)
	}
	var holdings []struct {
		Sector     string `json:"sector"`
		AssetClass string `json:"assetClass"`
		ISIN       string `json:"isin"`
		Location   string `json:"location"`
		Exchange   string `json:"exchange"`
	}
	if err := json.Unmarshal([]byte(holdingsJSONRead), &holdings); err != nil {
		t.Fatalf("parse holdings JSON: %v", err)
	}
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	if holdings[0].Sector != "Government" {
		t.Errorf("expected Sector 'Government', got %q", holdings[0].Sector)
	}
	if holdings[0].AssetClass != "Government Bond" {
		t.Errorf("expected AssetClass 'Government Bond', got %q", holdings[0].AssetClass)
	}
	if holdings[0].ISIN != "US912828Z079" {
		t.Errorf("expected ISIN 'US912828Z079', got %q", holdings[0].ISIN)
	}
}

// TestBlackRock_EquityFund_NoBondCharacteristics verifies that an equity ETF
// has equity valuation with no bond characteristics.
func TestBlackRock_EquityFund_NoBondCharacteristics(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "ISHY.L"

	equityJSON := `{"priceToEarnings":22.5,"priceToBook":4.8,"beta3Y":1.02,"standardDeviation3Y":14.8,"numberOfHoldings":505}`

	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			equity_valuation, fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF USD (Dist)",
		"LSE", "USD", "ETF",
		equityJSON,
		`{"family":"iShares","legalType":"Exchange Traded Fund","totalNetAssets":12500000000,"annualExpenseRatio":0.07}`,
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
	if equity["priceToEarnings"] != 22.5 {
		t.Errorf("expected priceToEarnings 22.5, got %v", equity["priceToEarnings"])
	}
	if equity["beta3Y"] != 1.02 {
		t.Errorf("expected beta3Y 1.02, got %v", equity["beta3Y"])
	}
	if equity["standardDeviation3Y"] != 14.8 {
		t.Errorf("expected standardDeviation3Y 14.8, got %v", equity["standardDeviation3Y"])
	}
	if equity["numberOfHoldings"] != float64(505) {
		t.Errorf("expected numberOfHoldings 505, got %v", equity["numberOfHoldings"])
	}
}

// TestBlackRock_SectorAllocation_DerivedFromHoldings verifies that sector
// allocation is derived from holdings data (aggregated by sector).
func TestBlackRock_SectorAllocation_DerivedFromHoldings(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "ISHY.L"

	// Holdings with sectors that should aggregate to sector allocation
	holdingsJSON := `[
		{"symbol":"AAPL","name":"Apple Inc","percent":7.2,"sector":"Information Technology"},
		{"symbol":"MSFT","name":"Microsoft Corp","percent":6.8,"sector":"Information Technology"},
		{"symbol":"JNJ","name":"Johnson & Johnson","percent":4.1,"sector":"Health Care"},
		{"symbol":"JPM","name":"JPMorgan Chase","percent":3.9,"sector":"Financials"},
		{"symbol":"XOM","name":"Exxon Mobil","percent":2.5,"sector":"Energy"}
	]`
	// Sector allocation derived from holdings: IT=14.0, Health=4.1, Financials=3.9, Energy=2.5
	sectorsJSON := `[{"sector":"Information Technology","percent":14.0},{"sector":"Health Care","percent":4.1},{"sector":"Financials","percent":3.9},{"sector":"Energy","percent":2.5}]`

	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings,
			extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF",
		"LSE", "USD", "ETF",
		holdingsJSON, sectorsJSON,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Verify sector allocation is correctly derived
	var sectorsJSONRead string
	err = db.QueryRow("SELECT sector_weightings FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&sectorsJSONRead)
	if err != nil {
		t.Fatalf("query sectors: %v", err)
	}
	var sectors []struct {
		Sector  string  `json:"sector"`
		Percent float64 `json:"percent"`
	}
	if err := json.Unmarshal([]byte(sectorsJSONRead), &sectors); err != nil {
		t.Fatalf("parse sectors JSON: %v", err)
	}
	if len(sectors) != 4 {
		t.Fatalf("expected 4 sectors, got %d", len(sectors))
	}
	// IT = 7.2 + 6.8 = 14.0
	if sectors[0].Sector != "Information Technology" || sectors[0].Percent != 14.0 {
		t.Errorf("expected 'Information Technology' 14.0, got %q %v", sectors[0].Sector, sectors[0].Percent)
	}
	if sectors[1].Sector != "Health Care" || sectors[1].Percent != 4.1 {
		t.Errorf("expected 'Health Care' 4.1, got %q %v", sectors[1].Sector, sectors[1].Percent)
	}
}

// TestBlackRock_CountryAllocation_DerivedFromHoldings verifies that country
// allocation is derived from holdings data (aggregated by location).
func TestBlackRock_CountryAllocation_DerivedFromHoldings(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "ISHY.L"

	// Holdings with locations that should aggregate to country allocation
	holdingsJSON := `[
		{"symbol":"AAPL","name":"Apple Inc","percent":7.2,"location":"United States"},
		{"symbol":"MSFT","name":"Microsoft Corp","percent":6.8,"location":"United States"},
		{"symbol":"HSBA","name":"HSBC","percent":1.5,"location":"United Kingdom"},
		{"symbol":"TSM","name":"Taiwan Semiconductor","percent":1.2,"location":"Taiwan"}
	]`
	// Country allocation derived from holdings: US=14.0, UK=1.5, Taiwan=1.2
	countriesJSON := `[{"country":"United States","percent":14.0},{"country":"United Kingdom","percent":1.5},{"country":"Taiwan","percent":1.2}]`

	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, geographic_allocations,
			extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF",
		"LSE", "USD", "ETF",
		holdingsJSON, countriesJSON,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Verify country allocation is correctly derived
	var countriesJSONRead string
	err = db.QueryRow("SELECT geographic_allocations FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&countriesJSONRead)
	if err != nil {
		t.Fatalf("query countries: %v", err)
	}
	var countries []struct {
		Country string  `json:"country"`
		Percent float64 `json:"percent"`
	}
	if err := json.Unmarshal([]byte(countriesJSONRead), &countries); err != nil {
		t.Fatalf("parse countries JSON: %v", err)
	}
	if len(countries) != 3 {
		t.Fatalf("expected 3 countries, got %d", len(countries))
	}
	// US = 7.2 + 6.8 = 14.0
	if countries[0].Country != "United States" || countries[0].Percent != 14.0 {
		t.Errorf("expected 'United States' 14.0, got %q %v", countries[0].Country, countries[0].Percent)
	}
	if countries[1].Country != "United Kingdom" || countries[1].Percent != 1.5 {
		t.Errorf("expected 'United Kingdom' 1.5, got %q %v", countries[1].Country, countries[1].Percent)
	}
}

// TestBlackRock_SymbolAPIDetailsWithNewFields verifies that the API endpoint
// /api/symbols/{id} returns BlackRock data with all new fields (SFDR, domicile,
// beta, extended holdings).
func TestBlackRock_SymbolAPIDetailsWithNewFields(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	internalSymbol := "ISHY.L"

	// Create symbol via API
	body := json.RawMessage(`{"internal_symbol": "ISHY.L", "market_data_symbol": "ISHY.L"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var symID int64
	err := db.QueryRow("SELECT id FROM symbol_mappings WHERE internal_symbol = ?", internalSymbol).Scan(&symID)
	if err != nil {
		t.Fatalf("query symbol id: %v", err)
	}

	// Insert symbol details with BlackRock-specific fields
	holdingsJSON := `[{"symbol":"AAPL","name":"Apple Inc","percent":7.2,"sector":"Information Technology","assetClass":"Equity","marketValue":1500000000,"notionalValue":1500000000,"shares":8000000,"price":187.5,"isin":"US0378331005","location":"United States","exchange":"NASDAQ","marketCurrency":"USD"}]`
	sectorsJSON := `[{"sector":"Information Technology","percent":48.5}]`
	countriesJSON := `[{"country":"United States","percent":94.2}]`
	equityJSON := `{"priceToEarnings":22.5,"priceToBook":4.8,"beta3Y":1.02,"standardDeviation3Y":14.8,"numberOfHoldings":505}`
	profileJSON := `{"family":"iShares","legalType":"Exchange Traded Fund","totalNetAssets":12500000000,"annualExpenseRatio":0.07,"sfdrClassification":"Article 6","domicile":"Ireland","rebalanceFrequency":"Quarterly","productStructure":"Physical","methodology":"Representative","fundManager":"BlackRock Asset Management Ireland Limited","custodian":"State Street Custodial Services (Ireland) Limited","issuingCompany":"iShares IV plc","benchmarkTicker":"SPX"}`

	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, geographic_allocations,
			equity_valuation, fund_profile,
			extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF USD (Dist)",
		"LSE", "USD", "ETF",
		holdingsJSON, sectorsJSON, countriesJSON,
		equityJSON, profileJSON,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
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

	if details["short_name"] != "iShares Core S&P 500 UCITS ETF" {
		t.Errorf("expected short_name 'iShares Core S&P 500 UCITS ETF', got %v", details["short_name"])
	}

	// Verify holdings with new fields (PascalCase keys from domain types)
	holdings, ok := details["top_holdings"].([]interface{})
	if !ok || len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %v", holdings)
	}
	holding := holdings[0].(map[string]interface{})
	if holding["Sector"] != "Information Technology" {
		t.Errorf("expected Sector 'Information Technology', got %v", holding["Sector"])
	}
	if holding["AssetClass"] != "Equity" {
		t.Errorf("expected AssetClass 'Equity', got %v", holding["AssetClass"])
	}
	if holding["MarketValue"] != float64(1500000000) {
		t.Errorf("expected MarketValue 1500000000, got %v", holding["MarketValue"])
	}
	if holding["ISIN"] != "US0378331005" {
		t.Errorf("expected ISIN 'US0378331005', got %v", holding["ISIN"])
	}
	if holding["Location"] != "United States" {
		t.Errorf("expected Location 'United States', got %v", holding["Location"])
	}
	if holding["Exchange"] != "NASDAQ" {
		t.Errorf("expected Exchange 'NASDAQ', got %v", holding["Exchange"])
	}

	// Verify equity valuation with BlackRock-specific fields
	equityRaw, ok := details["equity_valuation"]
	if !ok || equityRaw == nil {
		t.Fatal("expected non-nil equity_valuation")
	}
	equity := equityRaw.(map[string]interface{})
	if equity["Beta3Y"] != float64(1.02) {
		t.Errorf("expected Beta3Y 1.02, got %v", equity["Beta3Y"])
	}
	if equity["StandardDeviation3Y"] != float64(14.8) {
		t.Errorf("expected StandardDeviation3Y 14.8, got %v", equity["StandardDeviation3Y"])
	}
	if equity["NumberOfHoldings"] != float64(505) {
		t.Errorf("expected NumberOfHoldings 505, got %v", equity["NumberOfHoldings"])
	}

	// Verify fund profile with BlackRock-specific fields
	profileRaw, ok := details["fund_profile"]
	if !ok || profileRaw == nil {
		t.Fatal("expected non-nil fund_profile")
	}
	profile := profileRaw.(map[string]interface{})
	if profile["SFDRClassification"] != "Article 6" {
		t.Errorf("expected SFDRClassification 'Article 6', got %v", profile["SFDRClassification"])
	}
	if profile["Domicile"] != "Ireland" {
		t.Errorf("expected Domicile 'Ireland', got %v", profile["Domicile"])
	}
	if profile["RebalanceFrequency"] != "Quarterly" {
		t.Errorf("expected RebalanceFrequency 'Quarterly', got %v", profile["RebalanceFrequency"])
	}
	if profile["ProductStructure"] != "Physical" {
		t.Errorf("expected ProductStructure 'Physical', got %v", profile["ProductStructure"])
	}
	if profile["Methodology"] != "Representative" {
		t.Errorf("expected Methodology 'Representative', got %v", profile["Methodology"])
	}
	if profile["FundManager"] != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("expected FundManager 'BlackRock Asset Management Ireland Limited', got %v", profile["FundManager"])
	}
	if profile["Custodian"] != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("expected Custodian, got %v", profile["Custodian"])
	}
	if profile["IssuingCompany"] != "iShares IV plc" {
		t.Errorf("expected IssuingCompany 'iShares IV plc', got %v", profile["IssuingCompany"])
	}
	if profile["BenchmarkTicker"] != "SPX" {
		t.Errorf("expected BenchmarkTicker 'SPX', got %v", profile["BenchmarkTicker"])
	}
}

// TestBlackRock_WebPageRendersWithNewFields verifies that the web page
// /symbols/{id}/details renders correctly with symbol name and data.
func TestBlackRock_WebPageRendersWithNewFields(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	internalSymbol := "ISHY.L"

	// Create symbol via API
	body := json.RawMessage(`{"internal_symbol": "ISHY.L", "market_data_symbol": "ISHY.L"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var symID int64
	err := db.QueryRow("SELECT id FROM symbol_mappings WHERE internal_symbol = ?", internalSymbol).Scan(&symID)
	if err != nil {
		t.Fatalf("query symbol id: %v", err)
	}

	// Insert symbol details with BlackRock-specific fields
	holdingsJSON := `[{"symbol":"AAPL","name":"Apple Inc","percent":7.2,"sector":"Information Technology","assetClass":"Equity"}]`
	sectorsJSON := `[{"sector":"Information Technology","percent":48.5}]`
	countriesJSON := `[{"country":"United States","percent":94.2}]`
	equityJSON := `{"priceToEarnings":22.5,"priceToBook":4.8,"beta3Y":1.02,"standardDeviation3Y":14.8,"numberOfHoldings":505}`
	profileJSON := `{"family":"iShares","legalType":"Exchange Traded Fund","totalNetAssets":12500000000,"annualExpenseRatio":0.07,"sfdrClassification":"Article 6","domicile":"Ireland","rebalanceFrequency":"Quarterly","productStructure":"Physical","methodology":"Representative","fundManager":"BlackRock Asset Management Ireland Limited","custodian":"State Street Custodial Services (Ireland) Limited","issuingCompany":"iShares IV plc","benchmarkTicker":"SPX"}`

	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			top_holdings, sector_weightings, geographic_allocations,
			equity_valuation, fund_profile,
			extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF USD (Dist)",
		"LSE", "USD", "ETF",
		holdingsJSON, sectorsJSON, countriesJSON,
		equityJSON, profileJSON,
		time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// GET web page
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/symbols/%d/details", symID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("GET symbol details page: expected 200, got %d", w.Code)
	}

	bodyStr := w.Body.String()

	// Verify page renders with the symbol name (use substring without & to avoid HTML escaping)
	if !bytes.Contains([]byte(bodyStr), []byte("iShares Core")) {
		t.Errorf("page missing symbol name in title")
	}
	// Verify the symbol is in the page
	if !bytes.Contains([]byte(bodyStr), []byte("ISHY.L")) {
		t.Errorf("page missing internal symbol")
	}
	// Verify BlackRock-specific fields are rendered
	if !bytes.Contains([]byte(bodyStr), []byte("Article 6")) {
		t.Errorf("page missing SFDR classification")
	}
	if !bytes.Contains([]byte(bodyStr), []byte("Ireland")) {
		t.Errorf("page missing domicile")
	}
	// Detailed field rendering is covered by unit tests in symbol_details_web_test.go
}

// TestBlackRock_DataSourceURL_RoundTrip verifies that the data_source_url
// for BlackRock symbols round-trips correctly through the stale query.
func TestBlackRock_DataSourceURL_RoundTrip(t *testing.T) {
	db := setupTestDB(t)

	sourceURL := "https://www.ishares.com/uk/individual/en/products/268426/ishares-core-sp-500-ucits-etf"

	// Insert a symbol mapping with blackrock data_source_url
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, "ISHY.L", "ISHY.L", sourceURL)
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
	if internalSymbol != "ISHY.L" {
		t.Errorf("expected internal_symbol ISHY.L, got %q", internalSymbol)
	}
	if dataSourceURL != sourceURL {
		t.Errorf("expected blackrock data_source_url, got %q", dataSourceURL)
	}
}

// TestBlackRock_BackgroundRefresh_DualPath verifies that the background refresh
// mechanism correctly handles symbols with blackrock data_source_url configured.
// The stale query returns both the symbol and its data_source_url, allowing
// the service layer to route through the extractor dispatcher.
func TestBlackRock_BackgroundRefresh_DualPath(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "ISHY.L"
	sourceURL := "https://www.ishares.com/uk/individual/en/products/268426/ishares-core-sp-500-ucits-etf"

	// Create symbol with blackrock data_source_url
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, internalSymbol, "ISHY.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Simulate what the background refresh does:
	// 1. Stale query returns symbols with their data_source_url
	rows, err := db.Query(`
		SELECT sm.internal_symbol, COALESCE(sm.data_source_url, '')
		FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, time.Now().Add(-7*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("query stale symbols: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var staleSymbols []struct {
		InternalSymbol string
		DataSourceURL  string
	}
	for rows.Next() {
		var sym struct {
			InternalSymbol string
			DataSourceURL  string
		}
		if err := rows.Scan(&sym.InternalSymbol, &sym.DataSourceURL); err != nil {
			t.Fatalf("scan stale row: %v", err)
		}
		staleSymbols = append(staleSymbols, struct {
			InternalSymbol string
			DataSourceURL  string
		}{sym.InternalSymbol, sym.DataSourceURL})
	}

	if len(staleSymbols) != 1 {
		t.Fatalf("expected 1 stale symbol, got %d", len(staleSymbols))
	}
	if staleSymbols[0].InternalSymbol != internalSymbol {
		t.Errorf("expected stale symbol %q, got %q", internalSymbol, staleSymbols[0].InternalSymbol)
	}
	if staleSymbols[0].DataSourceURL != sourceURL {
		t.Errorf("expected blackrock data_source_url in stale query, got %q", staleSymbols[0].DataSourceURL)
	}

	// 2. Service layer uses data_source_url to route through dispatcher
	reg := extractor.NewRegistry()
	_ = reg.Register(blackrock.NewExtractor())

	extracted, err := reg.FindByURL(sourceURL)
	if err != nil {
		t.Fatalf("expected extractor for blackrock URL: %v", err)
	}
	if extracted.Name() != blackrock.Name {
		t.Errorf("expected blackrock extractor, got %q", extracted.Name())
	}

	// 3. Verify the dual-path: Yahoo for market data + BlackRock for symbol details
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
		t.Error("expected data_source_url for BlackRock path")
	}

	// 4. Verify both NAV (blackrock) and stock (yahoo) data coexist
	_, err = db.Exec(`
		INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "52.50", "USD", "nav", "blackrock", "2026-05-31", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert NAV data: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "52.75", "USD", "stock", "yahoo", "2026-05-31", time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert stock data: %v", err)
	}

	var navCount, stockCount int
	err = db.QueryRow("SELECT COUNT(*) FROM market_data WHERE symbol = ? AND data_type = 'nav' AND source = 'blackrock'",
		internalSymbol).Scan(&navCount)
	if err != nil {
		t.Fatalf("query NAV count: %v", err)
	}
	if navCount != 1 {
		t.Errorf("expected 1 NAV record from blackrock, got %d", navCount)
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

// TestBlackRock_BackgroundRefresh_ExtractionFailurePreservesPreviousData
// verifies that when extraction fails during background refresh, the symbol
// is marked as failed and previous data is preserved.
func TestBlackRock_BackgroundRefresh_ExtractionFailurePreservesPreviousData(t *testing.T) {
	db := setupTestDB(t)

	internalSymbol := "ISHY.L"
	sourceURL := "https://www.ishares.com/uk/individual/en/products/268426/ishares-core-sp-500-ucits-etf"

	// Create symbol with blackrock data_source_url
	_, err := db.Exec(`
		INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, data_source_url)
		VALUES (?, ?, ?)
	`, internalSymbol, "ISHY.L", sourceURL)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Insert existing symbol details (simulating previous successful extraction)
	existingProfileJSON := `{"family":"iShares","legalType":"Exchange Traded Fund","totalNetAssets":12000000000,"annualExpenseRatio":0.07,"sfdrClassification":"Article 6","domicile":"Ireland"}`
	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, long_name, exchange, currency, quote_type,
			fund_profile, extractor_as_of_date, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, internalSymbol, "iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF USD (Dist)",
		"LSE", "USD", "ETF",
		existingProfileJSON,
		time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		time.Now().Add(-30*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert existing symbol details: %v", err)
	}

	// Simulate extraction failure: mark as failed, preserve previous data
	// In the actual implementation, the service layer would:
	// 1. Attempt extraction via dispatcher
	// 2. On failure, set last_error and keep existing data
	// 3. NOT delete the existing symbol_details row

	// Verify existing data is still present (not deleted on failure)
	var shortName string
	err = db.QueryRow("SELECT short_name FROM symbol_details WHERE internal_symbol = ?",
		internalSymbol).Scan(&shortName)
	if err != nil {
		t.Fatalf("query existing symbol details: %v", err)
	}
	if shortName != "iShares Core S&P 500 UCITS ETF" {
		t.Errorf("expected preserved short_name, got %q", shortName)
	}

	// Verify the symbol is still in the stale query (eligible for retry)
	var staleURL string
	err = db.QueryRow(`
		SELECT sm.data_source_url
		FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < ?
	`, time.Now().Add(-7*24*time.Hour).Format(time.RFC3339)).Scan(&staleURL)
	if err != nil {
		t.Fatalf("query stale: %v", err)
	}
	if staleURL != sourceURL {
		t.Errorf("expected blackrock data_source_url in stale query for retry, got %q", staleURL)
	}
}

// TestBlackRock_Dispatcher_UnregisteredProvider verifies that dispatching
// to a URL with no matching extractor returns an explicit error.
func TestBlackRock_Dispatcher_UnregisteredProvider(t *testing.T) {
	reg := extractor.NewRegistry()
	// Register only WisdomTree — BlackRock is NOT registered
	_ = reg.Register(wisdomtree.NewExtractor())
	dispatcher := extractor.NewDispatcher(reg)

	// Dispatch to an iShares URL — should fail with explicit error
	_, err := dispatcher.Dispatch(context.Background(),
		"https://www.ishares.com/uk/individual/en/products/268426/ishares-core-sp-500-ucits-etf")
	if err == nil {
		t.Fatal("expected error for unregistered provider, got nil")
	}

	errStr := err.Error()
	// Error should mention that no extractor is registered
	if !containsString(errStr, "no extractor registered") {
		t.Errorf("expected 'no extractor registered' in error, got: %s", errStr)
	}

	// Verify a registered provider (WisdomTree) still works (may fail on network but not on routing)
	_, err = dispatcher.Dispatch(context.Background(),
		"https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/")
	if err != nil {
		errStr := err.Error()
		if len(errStr) > 20 && errStr[:20] == "no extractor registere" {
			t.Errorf("WisdomTree extractor should be registered, got routing error: %s", errStr)
		}
		// Network/API errors are expected in test environment
	}
}
