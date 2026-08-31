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
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// setupHrp creates the DB, router, and pre-seeds symbol mappings
// for HRP integration tests.
func setupHrp(t *testing.T, symbols []string) (*sql.DB, http.Handler) {
	t.Helper()
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	for _, sym := range symbols {
		body := json.RawMessage(fmt.Sprintf(
			`{"internal_symbol":"%s","market_data_symbol":"%s"}`, sym, sym))
		req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated && w.Code != http.StatusConflict {
			t.Fatalf("create symbol %s: expected 201/409, got %d: %s",
				sym, w.Code, w.Body.String())
		}
	}

	return db, router
}

// insertHrpMarketData inserts historical prices for the given symbols
// spanning ~252 trading days (1 year of data). Prices follow a simple
// upward trend with slight variation per symbol.
func insertHrpMarketData(t *testing.T, db *sql.DB, symbols map[string]float64) {
	t.Helper()
	now := time.Now().UTC()
	baseDate := now.AddDate(0, 0, -320)

	for i := 0; i <= 320; i++ {
		date := baseDate.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")

		for sym, basePrice := range symbols {
			// Simple upward trend with slight daily variation
			price := basePrice * (1.0 + float64(i)*0.001)
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, 'USD', 'stock', 'yahoo', ?)`,
				sym, fmt.Sprintf("%.2f", price), dateStr,
			)
			if err != nil {
				t.Fatalf("insert market data %s %s: %v", sym, dateStr, err)
			}
		}
	}

	// Also insert a "current" (empty date) row for each symbol.
	for sym, basePrice := range symbols {
		_, err := db.Exec(
			`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
			 VALUES (?, ?, 'USD', 'stock', 'yahoo', '')`,
			sym, fmt.Sprintf("%.2f", basePrice*1.3),
		)
		if err != nil {
			t.Fatalf("insert current market data %s: %v", sym, err)
		}
	}
}

// insertHrpPartialMarketData inserts full data for some symbols and
// limited data (~28 trading days) for others.
func insertHrpPartialMarketData(t *testing.T, db *sql.DB, fullData, partialData map[string]float64) {
	t.Helper()
	now := time.Now().UTC()

	// Full data: ~252 trading days
	fullBase := now.AddDate(0, 0, -320)
	for i := 0; i <= 320; i++ {
		date := fullBase.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")
		for sym, basePrice := range fullData {
			price := basePrice * (1.0 + float64(i)*0.001)
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, 'USD', 'stock', 'yahoo', ?)`,
				sym, fmt.Sprintf("%.2f", price), dateStr,
			)
			if err != nil {
				t.Fatalf("insert market data %s %s: %v", sym, dateStr, err)
			}
		}
	}

	// Partial data: only ~28 trading days (40 calendar days)
	for sym := range partialData {
		_, err := db.Exec(`DELETE FROM market_data WHERE symbol = ?`, sym)
		if err != nil {
			t.Fatalf("delete existing data for %s: %v", sym, err)
		}
	}
	partialBase := now.AddDate(0, 0, -40)
	for i := 0; i <= 40; i++ {
		date := partialBase.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")
		for sym, basePrice := range partialData {
			price := basePrice * (1.0 + float64(i)*0.001)
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, 'USD', 'stock', 'yahoo', ?)`,
				sym, fmt.Sprintf("%.2f", price), dateStr,
			)
			if err != nil {
				t.Fatalf("insert partial data %s %s: %v", sym, dateStr, err)
			}
		}
	}
}

// computeHrpRequest is the JSON request body for POST /api/hrp/compute.
type computeHrpRequest struct {
	Symbols      []string `json:"symbols"`
	Period       string   `json:"period"`
	BaseCurrency string   `json:"base_currency,omitempty"`
}

// computeHrpResponse is the JSON response for POST /api/hrp/compute.
type computeHrpResponse struct {
	Result          *hrpResult `json:"result"`
	Warnings        []string   `json:"warnings,omitempty"`
	ExcludedSymbols []string   `json:"excluded_symbols,omitempty"`
}

type hrpResult struct {
	Allocations []hrpAllocation `json:"allocations"`
	Symbols     []string        `json:"symbols"`
	TradingDays int             `json:"trading_days"`
	ComputedAt  string          `json:"computed_at"`
	Warnings    []string        `json:"warnings,omitempty"`
	Message     string          `json:"message,omitempty"`
}

type hrpAllocation struct {
	Method     string             `json:"method"`
	Weights    map[string]float64 `json:"weights"`
	Dendrogram *hrpDendrogramNode `json:"dendrogram,omitempty"`
}

type hrpDendrogramNode struct {
	Name     string               `json:"name"`
	Children []*hrpDendrogramNode `json:"children,omitempty"`
	Distance float64              `json:"distance,omitempty"`
}

// callComputeHrp sends a compute request and returns the decoded response.
func callComputeHrp(t *testing.T, router http.Handler, req computeHrpRequest) (*httptest.ResponseRecorder, computeHrpResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	reqReq := httptest.NewRequest(http.MethodPost, "/api/hrp/compute", bytes.NewReader(body))
	reqReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, reqReq)

	var resp computeHrpResponse
	if w.Code == http.StatusOK {
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v (body: %s)", err, w.Body.String())
		}
	}
	return w, resp
}

// sumMapWeights returns the sum of all values in a weight map.
func sumMapWeights(weights map[string]float64) float64 {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	return sum
}

// countDendrogramLeaves counts leaf nodes in a dendrogram tree.
func countDendrogramLeaves(node *hrpDendrogramNode) int {
	if node == nil {
		return 0
	}
	if len(node.Children) == 0 {
		return 1
	}
	count := 0
	for _, child := range node.Children {
		count += countDendrogramLeaves(child)
	}
	return count
}

// --- Tests ---

func TestHRP_ComputeTwoSymbols(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	db, router := setupHrp(t, symbols)

	insertHrpMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	})

	w, resp := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	// Verify four allocations (one per linkage method).
	if len(resp.Result.Allocations) != 4 {
		t.Fatalf("expected 4 allocations, got %d", len(resp.Result.Allocations))
	}

	// Verify each allocation.
	expectedMethods := map[string]bool{"single": false, "complete": false, "average": false, "ward": false}
	for _, alloc := range resp.Result.Allocations {
		expectedMethods[alloc.Method] = true

		// Weights should cover both symbols.
		if len(alloc.Weights) != 2 {
			t.Errorf("allocation %q: expected 2 weights, got %d", alloc.Method, len(alloc.Weights))
		}

		// Weights should sum to ~1.0.
		sum := sumMapWeights(alloc.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("allocation %q: weights sum to %f, expected ~1.0", alloc.Method, sum)
		}

		// All weights should be non-negative.
		for sym, w := range alloc.Weights {
			if w < 0 {
				t.Errorf("allocation %q: weight[%q] = %f, expected >= 0", alloc.Method, sym, w)
			}
		}

		// Dendrogram should have 2 leaves.
		if alloc.Dendrogram != nil {
			leaves := countDendrogramLeaves(alloc.Dendrogram)
			if leaves != 2 {
				t.Errorf("allocation %q: dendrogram leaves = %d, expected 2", alloc.Method, leaves)
			}
		}
	}

	// Verify all four methods are present.
	for method, found := range expectedMethods {
		if !found {
			t.Errorf("expected allocation method %q not found", method)
		}
	}

	// Verify symbols match.
	if len(resp.Result.Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(resp.Result.Symbols))
	}

	// Verify trading days is reasonable (~252 for 1Y).
	if resp.Result.TradingDays < 200 || resp.Result.TradingDays > 300 {
		t.Errorf("expected trading days around 252, got %d", resp.Result.TradingDays)
	}

	// Verify computed_at is set.
	if resp.Result.ComputedAt == "" {
		t.Error("expected computed_at to be set")
	}
}

func TestHRP_ComputeFiveSymbols(t *testing.T) {
	symbols := []string{"AAPL", "MSFT", "GOOGL", "AMZN", "NVDA"}
	db, router := setupHrp(t, symbols)

	insertHrpMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
		"AMZN":  180.0,
		"NVDA":  800.0,
	})

	w, resp := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	if len(resp.Result.Allocations) != 4 {
		t.Fatalf("expected 4 allocations, got %d", len(resp.Result.Allocations))
	}

	for _, alloc := range resp.Result.Allocations {
		if len(alloc.Weights) != 5 {
			t.Errorf("allocation %q: expected 5 weights, got %d", alloc.Method, len(alloc.Weights))
		}
		sum := sumMapWeights(alloc.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("allocation %q: weights sum to %f, expected ~1.0", alloc.Method, sum)
		}
		if alloc.Dendrogram != nil {
			leaves := countDendrogramLeaves(alloc.Dendrogram)
			if leaves != 5 {
				t.Errorf("allocation %q: dendrogram leaves = %d, expected 5", alloc.Method, leaves)
			}
		}
	}
}

func TestHRP_Error_SingleSymbol(t *testing.T) {
	symbols := []string{"AAPL"}
	db, router := setupHrp(t, symbols)
	insertHrpMarketData(t, db, map[string]float64{"AAPL": 175.0})

	w, _ := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INSUFFICIENT_SYMBOLS" {
		t.Errorf("expected error code INSUFFICIENT_SYMBOLS, got %q", errResp.Code)
	}
}

func TestHRP_Error_AllSymbolsNoData(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	_, router := setupHrp(t, symbols)
	// Don't insert any market data.

	w, resp := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	// Service returns 200 with empty-state message.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}
	if resp.Result.Message == "" {
		t.Error("expected message for empty state")
	}
	if len(resp.ExcludedSymbols) != 2 {
		t.Errorf("expected 2 excluded symbols, got %d: %v", len(resp.ExcludedSymbols), resp.ExcludedSymbols)
	}
}

func TestHRP_Error_TooManySymbols(t *testing.T) {
	symbols := []string{"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8", "S9", "S10",
		"S11", "S12", "S13", "S14", "S15", "S16", "S17", "S18", "S19", "S20", "S21"}
	_, router := setupHrp(t, symbols)

	w, _ := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "TOO_MANY_SYMBOLS" {
		t.Errorf("expected error code TOO_MANY_SYMBOLS, got %q", errResp.Code)
	}
}

func TestHRP_Error_InvalidPeriod(t *testing.T) {
	_, router := setupHrp(t, []string{"AAPL", "MSFT"})

	w, _ := callComputeHrp(t, router, computeHrpRequest{
		Symbols: []string{"AAPL", "MSFT"},
		Period:  "2Y", // Not valid
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_PERIOD" {
		t.Errorf("expected error code INVALID_PERIOD, got %q", errResp.Code)
	}
}

func TestHRP_Warning_PartialData(t *testing.T) {
	symbols := []string{"AAPL", "MSFT", "GOOGL"}
	db, router := setupHrp(t, symbols)

	insertHrpPartialMarketData(t, db,
		map[string]float64{
			"AAPL": 175.0,
			"MSFT": 400.0,
		},
		map[string]float64{
			"GOOGL": 140.0,
		},
	)

	w, resp := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	// Should have warnings about insufficient data for GOOGL.
	hasWarning := false
	for _, w := range resp.Warnings {
		if len(w) > 0 {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Error("expected warnings about insufficient data")
	}
}

func TestHRP_SaveAsModelPortfolio(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	db, router := setupHrp(t, symbols)

	insertHrpMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	})

	// Step 1: Compute HRP.
	_, resp := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if resp.Result == nil || len(resp.Result.Allocations) == 0 {
		t.Fatal("expected HRP result with allocations")
	}

	// Step 2: Save the "single" linkage allocation as a model portfolio.
	var alloc *hrpAllocation
	for i := range resp.Result.Allocations {
		if resp.Result.Allocations[i].Method == "single" {
			alloc = &resp.Result.Allocations[i]
			break
		}
	}
	if alloc == nil {
		t.Fatal("expected 'single' linkage allocation")
	}

	type saveEntry struct {
		Symbol string  `json:"symbol"`
		Weight float64 `json:"weight"`
	}
	entries := make([]saveEntry, 0, len(alloc.Weights))
	for sym, weight := range alloc.Weights {
		if weight > 0.001 {
			entries = append(entries, saveEntry{Symbol: sym, Weight: weight})
		}
	}

	saveReq := struct {
		Name    string      `json:"name"`
		Entries []saveEntry `json:"entries"`
	}{
		Name:    "HRP Single Linkage",
		Entries: entries,
	}
	body, _ := json.Marshal(saveReq)
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var mp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&mp)

	if mp.Name != "HRP Single Linkage" {
		t.Errorf("expected name 'HRP Single Linkage', got %q", mp.Name)
	}
	if len(mp.Entries) == 0 {
		t.Fatal("expected at least 1 entry in saved model portfolio")
	}

	// Verify the saved portfolio can be retrieved.
	getReq := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/model-portfolios/%d", mp.ID), nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, getReq)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var retrieved modelportfolio.ModelPortfolio
	json.NewDecoder(w2.Body).Decode(&retrieved)
	if retrieved.Name != "HRP Single Linkage" {
		t.Errorf("expected name 'HRP Single Linkage', got %q", retrieved.Name)
	}
}

func TestHRP_GetCandidateSymbols(t *testing.T) {
	symbols := []string{"AAPL", "MSFT", "GOOGL"}
	_, router := setupHrp(t, symbols)

	req := httptest.NewRequest(http.MethodGet, "/api/hrp/symbols", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Symbols []string `json:"symbols"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	found := make(map[string]bool)
	for _, s := range resp.Symbols {
		found[s] = true
	}
	for _, sym := range symbols {
		if !found[sym] {
			t.Errorf("expected %s in candidate symbols", sym)
		}
	}
}

func TestHRP_DefaultPeriod(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	db, router := setupHrp(t, symbols)

	insertHrpMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	})

	// Don't specify period — should default to 3Y.
	w, resp := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}
	if len(resp.Result.Allocations) != 4 {
		t.Fatalf("expected 4 allocations, got %d", len(resp.Result.Allocations))
	}
}

func TestHRP_SaveAsModelPortfolio_InvalidName(t *testing.T) {
	_, router := setupHrp(t, []string{"AAPL", "MSFT"})

	saveReq := struct {
		Name    string `json:"name"`
		Entries []struct {
			Symbol string  `json:"symbol"`
			Weight float64 `json:"weight"`
		} `json:"entries"`
	}{
		Name: "",
		Entries: []struct {
			Symbol string  `json:"symbol"`
			Weight float64 `json:"weight"`
		}{
			{Symbol: "AAPL", Weight: 0.5},
			{Symbol: "MSFT", Weight: 0.5},
		},
	}
	body, _ := json.Marshal(saveReq)
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHRP_SaveAsModelPortfolio_DuplicateName(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	_, router := setupHrp(t, symbols)

	// Create a model portfolio first.
	createModelPortfolio(t, router, "Duplicate HRP Test", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})

	saveReq := struct {
		Name    string `json:"name"`
		Entries []struct {
			Symbol string  `json:"symbol"`
			Weight float64 `json:"weight"`
		} `json:"entries"`
	}{
		Name: "Duplicate HRP Test",
		Entries: []struct {
			Symbol string  `json:"symbol"`
			Weight float64 `json:"weight"`
		}{
			{Symbol: "AAPL", Weight: 0.5},
			{Symbol: "MSFT", Weight: 0.5},
		},
	}
	body, _ := json.Marshal(saveReq)
	req := httptest.NewRequest(http.MethodPost, "/api/hrp/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "NAME_EXISTS" {
		t.Errorf("expected error code NAME_EXISTS, got %q", errResp.Code)
	}
}

func TestHRP_PerfectlyCorrelatedAssets(t *testing.T) {
	symbols := []string{"SYM_A", "SYM_B"}
	db, router := setupHrp(t, symbols)

	// Insert identical price data — perfectly correlated (singular covariance).
	now := time.Now().UTC()
	baseDate := now.AddDate(0, 0, -320)
	for i := 0; i <= 320; i++ {
		date := baseDate.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")
		price := fmt.Sprintf("%.2f", 100.0+float64(i)*0.5)
		for _, sym := range symbols {
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, 'USD', 'stock', 'yahoo', ?)`,
				sym, price, dateStr,
			)
			if err != nil {
				t.Fatalf("insert market data %s %s: %v", sym, dateStr, err)
			}
		}
	}

	w, resp := callComputeHrp(t, router, computeHrpRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	// HRP handles singular matrices via regularization fallback.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	if len(resp.Result.Allocations) != 4 {
		t.Fatalf("expected 4 allocations, got %d", len(resp.Result.Allocations))
	}

	// All allocations should have valid weights summing to ~1.0.
	for _, alloc := range resp.Result.Allocations {
		sum := sumMapWeights(alloc.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("allocation %q: weights sum to %f, expected ~1.0", alloc.Method, sum)
		}
	}
}
