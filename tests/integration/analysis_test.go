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

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
)

// setupAnalysis creates a portfolio, account, and symbol mappings for analysis tests.
func setupAnalysis(t *testing.T) (*sql.DB, http.Handler, int64, int64) {
	t.Helper()
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create portfolio
	body := json.RawMessage(`{"name": "Analysis Test", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	portfolioID := p.ID

	// Create account
	body = json.RawMessage(`{"name": "Test Account", "portfolio_id": ` + fmt.Sprintf("%d", portfolioID) + `}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var a struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(w.Body).Decode(&a)
	accountID := a.ID

	// Create symbol mappings
	for _, sym := range []string{"AAPL", "MSFT", "VOO"} {
		body = json.RawMessage(fmt.Sprintf(`{"internal_symbol": "%s", "market_data_symbol": "%s"}`, sym, sym))
		req = httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create symbol %s: expected 201, got %d: %s", sym, w.Code, w.Body.String())
		}
	}

	return db, router, portfolioID, accountID
}

// insertAnalysisMarketData inserts current prices and historical prices for correlation.
func insertAnalysisMarketData(t *testing.T, db *sql.DB) {
	t.Helper()

	// Current prices (empty date = "latest" row, used for position enrichment)
	for _, sym := range []struct {
		sym  string
		price string
	}{
		{"AAPL", "180.00"},
		{"MSFT", "420.00"},
		{"VOO", "480.00"},
	} {
		_, err := db.Exec(
			`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
			 VALUES (?, ?, 'USD', 'stock', 'yahoo', '')`,
			sym.sym, sym.price,
		)
		if err != nil {
			t.Fatalf("insert market data %s: %v", sym.sym, err)
		}
	}

	// Historical prices for correlation (60 days, starting ~90 days ago)
	// Must be within time.Now()-365 days to fall in the query range.
	now := time.Now().UTC()
	baseDate := now.AddDate(0, 0, -90)
	for i := 0; i < 60; i++ {
		// Skip weekends
		date := baseDate.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")

		// AAPL: slight upward trend with noise
		aaplPrice := fmt.Sprintf("%.2f", 170.0+float64(i)*0.5)
		// MSFT: correlated with AAPL (similar direction)
		msftPrice := fmt.Sprintf("%.2f", 400.0+float64(i)*0.8)
		// VOO: less correlated (different pattern)
		vooPrice := fmt.Sprintf("%.2f", 470.0+float64(i)*0.3)

		for _, entry := range []struct {
			sym   string
			price string
		}{
			{"AAPL", aaplPrice},
			{"MSFT", msftPrice},
			{"VOO", vooPrice},
		} {
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, 'USD', 'stock', 'yahoo', ?)`,
				entry.sym, entry.price, dateStr,
			)
			if err != nil {
				t.Fatalf("insert historical %s %s: %v", entry.sym, dateStr, err)
			}
		}
	}
}

// insertAnalysisSymbolDetails inserts symbol details for AAPL, MSFT (stocks) and VOO (ETF).
func insertAnalysisSymbolDetails(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Now().Format(time.RFC3339)

	// AAPL — stock with sector
	_, err := db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, quote_type, sector,
			equity_valuation, fund_profile, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"AAPL", "Apple Inc", "EQUITY", "Technology",
		`{"priceToEarnings":30.0,"priceToBook":45.0,"priceToCashflow":22.0,"priceToSales":8.0}`,
		`{"totalNetAssets":3000000000000}`,
		now,
	)
	if err != nil {
		t.Fatalf("insert AAPL details: %v", err)
	}

	// MSFT — stock with sector
	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, quote_type, sector,
			equity_valuation, fund_profile, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"MSFT", "Microsoft Corp", "EQUITY", "Technology",
		`{"priceToEarnings":35.0,"priceToBook":12.0,"priceToCashflow":25.0,"priceToSales":12.0}`,
		`{"totalNetAssets":3100000000000}`,
		now,
	)
	if err != nil {
		t.Fatalf("insert MSFT details: %v", err)
	}

	// VOO — ETF with holdings, sector weightings
	_, err = db.Exec(`
		INSERT INTO symbol_details (
			internal_symbol, short_name, quote_type,
			top_holdings, sector_weightings, aggregate_positions,
			fund_profile, equity_valuation, geographic_allocations, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"VOO", "Vanguard S&P 500 ETF", "ETF",
		`[
			{"symbol":"AAPL","name":"Apple Inc","percent":700},
			{"symbol":"MSFT","name":"Microsoft Corp","percent":650},
			{"symbol":"GOOGL","name":"Alphabet Inc","percent":400},
			{"symbol":"AMZN","name":"Amazon.com Inc","percent":350},
			{"symbol":"NVDA","name":"NVIDIA Corp","percent":300}
		]`,
		`[
			{"sector":"Technology","percent":3000},
			{"sector":"Healthcare","percent":1300},
			{"sector":"Financials","percent":1200},
			{"sector":"Consumer Discretionary","percent":1000},
			{"sector":"Communication Services","percent":800},
			{"sector":"Industrials","percent":800},
			{"sector":"Consumer Staples","percent":600},
			{"sector":"Energy","percent":400},
			{"sector":"Utilities","percent":200},
			{"sector":"Real Estate","percent":200},
			{"sector":"Materials","percent":100}
		]`,
		`{"stock":9800,"bond":0,"cash":100,"convertible":0,"preferred":0,"other":900}`,
		`{"family":"Vanguard","legalType":"Exchange Traded Fund","totalNetAssets":400000000000,"annualExpenseRatio":0.03,"annualHoldingsTurnover":3.0}`,
		`{"priceToEarnings":24.0,"priceToBook":5.0,"priceToCashflow":16.0,"priceToSales":4.0}`,
		`[{"region":"United States","percent":9400},{"region":"Other","percent":600}]`,
		now,
	)
	if err != nil {
		t.Fatalf("insert VOO details: %v", err)
	}
}

func TestAnalysis_FullPortfolio(t *testing.T) {
	db, router, portfolioID, accountID := setupAnalysis(t)

	// Create transactions: buy AAPL, MSFT, VOO
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 17000, -170000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "MSFT", 5, 40000, -200000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "VOO", 10, 47000, -470000)

	// Insert market data (current prices + historical for correlation)
	insertAnalysisMarketData(t, db)

	// Insert symbol details
	insertAnalysisSymbolDetails(t, db)

	// Call analysis API
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d&period=3M", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		PortfolioID          int64             `json:"portfolio_id"`
		ComputedAt           string            `json:"computed_at"`
		Overlap              *json.RawMessage  `json:"overlap"`
		Correlation          *json.RawMessage  `json:"correlation"`
		SectorAllocation     *json.RawMessage  `json:"sector_allocation"`
		GeographicAllocation *json.RawMessage  `json:"geographic_allocation"`
		StressTest           *json.RawMessage  `json:"stress_test"`
		FactorExposure       *json.RawMessage  `json:"factor_exposure"`
		Warnings             []string          `json:"warnings"`
		Message              string            `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Verify structure
	if result.PortfolioID != portfolioID {
		t.Errorf("expected portfolio_id %d, got %d", portfolioID, result.PortfolioID)
	}
	if result.ComputedAt == "" {
		t.Error("expected computed_at to be set")
	}
	if result.Message != "" {
		t.Errorf("expected no message for valid analysis, got %q", result.Message)
	}

	// All sections should be non-null for a portfolio with positions and data
	if result.Overlap == nil {
		t.Error("expected overlap section to be non-null")
	}
	if result.Correlation == nil {
		t.Error("expected correlation section to be non-null")
	}
	if result.SectorAllocation == nil {
		t.Error("expected sector_allocation section to be non-null")
	}
	if result.GeographicAllocation == nil {
		t.Error("expected geographic_allocation section to be non-null")
	}
	if result.StressTest == nil {
		t.Error("expected stress_test section to be non-null")
	}
	if result.FactorExposure == nil {
		t.Error("expected factor_exposure section to be non-null")
	}
}

func TestAnalysis_NoPositions(t *testing.T) {
	_, router, portfolioID, _ := setupAnalysis(t)
	// Don't create any transactions

	// Call analysis API
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Message    string `json:"message"`
		Overlap    *struct{} `json:"overlap"`
		Warnings   []string `json:"warnings"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Message == "" {
		t.Error("expected empty-state message for no positions")
	}
	if result.Overlap != nil {
		t.Error("expected overlap to be null for no positions")
	}
}

func TestAnalysis_SectionFilter(t *testing.T) {
	db, router, portfolioID, accountID := setupAnalysis(t)

	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 17000, -170000)
	insertAnalysisMarketData(t, db)
	insertAnalysisSymbolDetails(t, db)

	// Request only sector_allocation
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d&section=sector_allocation", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		SectorAllocation *json.RawMessage `json:"sector_allocation"`
		Overlap          *struct{}        `json:"overlap"`
		Correlation      *struct{}        `json:"correlation"`
		StressTest       *struct{}        `json:"stress_test"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.SectorAllocation == nil {
		t.Error("expected sector_allocation to be non-null when filtered")
	}
	// Other sections should be omitted (omitempty on nil pointer)
	if result.Overlap != nil {
		t.Error("expected overlap to be omitted when filtering sector_allocation")
	}
	if result.Correlation != nil {
		t.Error("expected correlation to be omitted when filtering sector_allocation")
	}
}

func TestAnalysis_InvalidSection(t *testing.T) {
	_, router, portfolioID, _ := setupAnalysis(t)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d&section=foobar", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid section, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnalysis_InvalidPeriod(t *testing.T) {
	_, router, portfolioID, _ := setupAnalysis(t)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d&period=2Y", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid period, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnalysis_StressTestScenarios(t *testing.T) {
	db, router, portfolioID, accountID := setupAnalysis(t)

	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "VOO", 10, 47000, -470000)
	insertAnalysisMarketData(t, db)
	insertAnalysisSymbolDetails(t, db)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d&section=stress_test", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		StressTest *struct {
			Scenarios []struct {
				Name                string `json:"name"`
				EstimatedReturnPct  float64 `json:"estimated_return_pct"`
				EstimatedDollarImpact string `json:"estimated_dollar_impact"`
				SectorContributions map[string]float64 `json:"sector_contributions"`
			} `json:"scenarios"`
			Warnings []string `json:"warnings"`
		} `json:"stress_test"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.StressTest == nil {
		t.Fatal("expected stress_test section")
	}
	if len(result.StressTest.Scenarios) != 6 {
		t.Errorf("expected 6 stress scenarios, got %d", len(result.StressTest.Scenarios))
	}

	// All scenarios should have negative returns (crisis scenarios)
	for _, s := range result.StressTest.Scenarios {
		if s.EstimatedReturnPct >= 0 {
			t.Errorf("scenario %q should have negative return, got %.2f%%", s.Name, s.EstimatedReturnPct)
		}
		if s.EstimatedDollarImpact == "" {
			t.Errorf("scenario %q should have dollar impact", s.Name)
		}
		if len(s.SectorContributions) == 0 {
			t.Errorf("scenario %q should have sector contributions", s.Name)
		}
	}

	// Verify sorted by severity (most negative first)
	if len(result.StressTest.Scenarios) >= 2 {
		first := result.StressTest.Scenarios[0].EstimatedReturnPct
		second := result.StressTest.Scenarios[1].EstimatedReturnPct
		if first > second {
			t.Errorf("scenarios should be sorted by severity, first=%.2f > second=%.2f", first, second)
		}
	}
}

func TestAnalysis_OverlapWithETFs(t *testing.T) {
	db, router, portfolioID, accountID := setupAnalysis(t)

	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 17000, -170000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "VOO", 10, 47000, -470000)
	insertAnalysisMarketData(t, db)
	insertAnalysisSymbolDetails(t, db)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d&section=overlap", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Overlap *struct {
			PairwiseMatrix         []struct{} `json:"pairwise_matrix"`
			TopConcentratedStocks  []struct {
				Symbol        string   `json:"symbol"`
				TotalWeightPct float64 `json:"total_weight_pct"`
				HeldByETFs    []string `json:"held_by_etfs"`
			} `json:"top_concentrated_stocks"`
			Message string `json:"message"`
		} `json:"overlap"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Overlap == nil {
		t.Fatal("expected overlap section")
	}

	// With 1 ETF (VOO) and 1 stock (AAPL), pairwise matrix should be empty
	// (pairwise requires 2+ ETFs)
	if len(result.Overlap.PairwiseMatrix) != 0 {
		t.Errorf("expected 0 pairwise entries for 1 ETF, got %d", len(result.Overlap.PairwiseMatrix))
	}

	// Top concentrated stocks should include VOO's holdings
	if len(result.Overlap.TopConcentratedStocks) == 0 {
		t.Error("expected concentrated stocks from VOO holdings")
	}
}

func TestAnalysis_CorrelationMatrix(t *testing.T) {
	db, router, portfolioID, accountID := setupAnalysis(t)

	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 17000, -170000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "MSFT", 5, 40000, -200000)
	insertAnalysisMarketData(t, db)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/analysis?portfolio_id=%d&section=correlation&period=3M", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result struct {
		Correlation *struct {
			Matrix   [][]json.RawMessage `json:"matrix"`
			Symbols  []string            `json:"symbols"`
			Period   string              `json:"period"`
			Warnings []string            `json:"warnings"`
		} `json:"correlation"`
		Warnings []string `json:"warnings"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Correlation == nil {
		t.Fatal("expected correlation section")
	}
	if len(result.Correlation.Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d (warnings: %v, top-level warnings: %v)",
			len(result.Correlation.Symbols), result.Correlation.Warnings, result.Warnings)
	}
	if result.Correlation.Period != "3M" {
		t.Errorf("expected period 3M, got %q", result.Correlation.Period)
	}
	if len(result.Correlation.Matrix) != 2 {
		t.Errorf("expected 2x2 matrix, got %dxN", len(result.Correlation.Matrix))
	}
}


