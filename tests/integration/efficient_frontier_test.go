package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// setupEfficientFrontier creates the DB, router, and pre-seeds symbol mappings
// for efficient frontier integration tests.
func setupEfficientFrontier(t *testing.T, symbols []string) (*sql.DB, http.Handler) {
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

// insertFrontierMarketData inserts historical prices for the given symbols
// spanning ~252 trading days (1 year of data). Prices follow a simple
// upward trend with slight variation per symbol to produce a valid frontier.
func insertFrontierMarketData(t *testing.T, db *sql.DB, symbols map[string]float64) {
	t.Helper()
	now := time.Now().UTC()
	// Go back far enough to cover 252 trading days (~312 calendar days including weekends)
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

// insertPartialMarketData inserts historical prices for only some of the
// given symbols. The symbols in `fullData` get ~252 days, the symbols in
// `partialData` get only ~28 trading days (below the 60-day threshold).
func insertPartialMarketData(t *testing.T, db *sql.DB, fullData, partialData map[string]float64) {
	t.Helper()
	now := time.Now().UTC()

	// Full data: ~252 trading days (320 calendar days including weekends)
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
	// This is below the 60-day minimum threshold.
	// First, delete any existing data for partial symbols to ensure
	// they only have the limited data range.
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

// computeFrontierRequest is the JSON request body for POST /api/efficient-frontier/compute.
type computeFrontierRequest struct {
	Symbols      []string `json:"symbols"`
	Period       string   `json:"period"`
	RiskFreeRate float64  `json:"risk_free_rate,omitempty"`
}

// computeFrontierResponse is the JSON response for POST /api/efficient-frontier/compute.
type computeFrontierResponse struct {
	Result          *frontierResult `json:"result"`
	Warnings        []string        `json:"warnings,omitempty"`
	ExcludedSymbols []string        `json:"excluded_symbols,omitempty"`
}

type frontierResult struct {
	FrontierPoints []frontierPoint     `json:"frontier_points"`
	MaxSharpe      *optimizedPortfolio `json:"max_sharpe,omitempty"`
	MinVariance    *optimizedPortfolio `json:"min_variance,omitempty"`
	HighestReturn  *optimizedPortfolio `json:"highest_return,omitempty"`
	Symbols        []string            `json:"symbols"`
	TradingDays    int                 `json:"trading_days"`
	ComputedAt     string              `json:"computed_at"`
	Warnings       []string            `json:"warnings,omitempty"`
	Message        string              `json:"message,omitempty"`
}

type frontierPoint struct {
	ReturnPct     float64   `json:"return_pct"`
	VolatilityPct float64   `json:"volatility_pct"`
	SharpeRatio   float64   `json:"sharpe_ratio"`
	Weights       []float64 `json:"weights"`
}

type optimizedPortfolio struct {
	Name          string    `json:"name"`
	ReturnPct     float64   `json:"return_pct"`
	VolatilityPct float64   `json:"volatility_pct"`
	SharpeRatio   float64   `json:"sharpe_ratio"`
	Weights       []float64 `json:"weights"`
}

// callComputeFrontier sends a compute request and returns the decoded response.
func callComputeFrontier(t *testing.T, router http.Handler, req computeFrontierRequest) (*httptest.ResponseRecorder, computeFrontierResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	reqReq := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/compute", bytes.NewReader(body))
	reqReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, reqReq)

	var resp computeFrontierResponse
	if w.Code == http.StatusOK {
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v (body: %s)", err, w.Body.String())
		}
	}
	return w, resp
}

// sumWeights returns the sum of a weight slice.
func sumWeights(weights []float64) float64 {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	return sum
}

// almostEqual checks if two float64 values are approximately equal.
func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestEfficientFrontier_ComputeTwoSymbols(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	db, router := setupEfficientFrontier(t, symbols)

	insertFrontierMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	})

	w, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	// Verify frontier points are present.
	if len(resp.Result.FrontierPoints) == 0 {
		t.Error("expected frontier points to be non-empty")
	}

	// Verify key portfolios are present.
	if resp.Result.MaxSharpe == nil {
		t.Error("expected max_sharpe portfolio")
	}
	if resp.Result.MinVariance == nil {
		t.Error("expected min_variance portfolio")
	}
	if resp.Result.HighestReturn == nil {
		t.Error("expected highest_return portfolio")
	}

	// Verify symbols match.
	if len(resp.Result.Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(resp.Result.Symbols))
	}

	// Verify weights sum to ~1.0 for each frontier point.
	for i, pt := range resp.Result.FrontierPoints {
		sum := sumWeights(pt.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("frontier point %d: weights sum to %f, expected ~1.0", i, sum)
		}
	}

	// Verify key portfolio weights sum to ~1.0.
	for name, port := range map[string]*optimizedPortfolio{
		"max_sharpe":     resp.Result.MaxSharpe,
		"min_variance":   resp.Result.MinVariance,
		"highest_return": resp.Result.HighestReturn,
	} {
		if port == nil {
			continue
		}
		sum := sumWeights(port.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("%s: weights sum to %f, expected ~1.0", name, sum)
		}
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

func TestEfficientFrontier_ComputeThreePlusSymbols(t *testing.T) {
	symbols := []string{"AAPL", "MSFT", "GOOGL"}
	db, router := setupEfficientFrontier(t, symbols)

	insertFrontierMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
	})

	w, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	if len(resp.Result.Symbols) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(resp.Result.Symbols))
	}

	// All frontier points should have 3 weights summing to ~1.0.
	for i, pt := range resp.Result.FrontierPoints {
		if len(pt.Weights) != 3 {
			t.Errorf("frontier point %d: expected 3 weights, got %d", i, len(pt.Weights))
		}
		sum := sumWeights(pt.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("frontier point %d: weights sum to %f, expected ~1.0", i, sum)
		}
	}

	// Key portfolios should also have 3 weights.
	for name, port := range map[string]*optimizedPortfolio{
		"max_sharpe":     resp.Result.MaxSharpe,
		"min_variance":   resp.Result.MinVariance,
		"highest_return": resp.Result.HighestReturn,
	} {
		if port == nil {
			continue
		}
		if len(port.Weights) != 3 {
			t.Errorf("%s: expected 3 weights, got %d", name, len(port.Weights))
		}
		sum := sumWeights(port.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("%s: weights sum to %f, expected ~1.0", name, sum)
		}
	}
}

func TestEfficientFrontier_Error_AllSymbolsNoData(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	_, router := setupEfficientFrontier(t, symbols)
	// Don't insert any market data — symbols exist but no price data.

	w, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	// When no data is available for any symbol, the service returns 200
	// with a message (empty state), not an error.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil even with no data")
	}
	if resp.Result.Message == "" {
		t.Error("expected message for empty state")
	}
	// Frontier points should be empty when no data is available.
	if resp.Result.FrontierPoints != nil && len(resp.Result.FrontierPoints) > 0 {
		t.Error("expected no frontier points when all symbols have no data")
	}
}

func TestEfficientFrontier_Error_SingleSymbol(t *testing.T) {
	symbols := []string{"AAPL"}
	db, router := setupEfficientFrontier(t, symbols)
	insertFrontierMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
	})

	w, _ := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	// The handler validates at least 2 symbols at the HTTP level.
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

func TestEfficientFrontier_Warning_PartialData(t *testing.T) {
	symbols := []string{"AAPL", "MSFT", "GOOGL"}
	db, router := setupEfficientFrontier(t, symbols)

	// AAPL and MSFT get full data (~252 trading days), GOOGL gets only partial (~28 trading days)
	insertPartialMarketData(t, db,
		map[string]float64{
			"AAPL": 175.0,
			"MSFT": 400.0,
		},
		map[string]float64{
			"GOOGL": 140.0,
		},
	)

	w, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	// The engine checks min trading days across ALL symbols.
	// When any symbol has < 60 days, the computation returns a warning
	// with no frontier points (not a hard error).
	// Warnings are merged: engine warnings → resp.Warnings, service warnings also → resp.Warnings.
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
	if len(resp.Result.FrontierPoints) > 0 {
		t.Error("expected no frontier points when data is insufficient")
	}

	// All symbols are still in the result (the engine doesn't exclude individual symbols).
	if len(resp.Result.Symbols) != 3 {
		t.Errorf("expected 3 symbols in result, got %d: %v", len(resp.Result.Symbols), resp.Result.Symbols)
	}
}

func TestEfficientFrontier_Error_AllSymbolsMissingData(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	_, router := setupEfficientFrontier(t, symbols)
	// Don't insert any market data

	w, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	// Service returns 200 with empty-state message when all symbols have no data.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}
	if resp.Result.Message == "" {
		t.Error("expected message indicating no data available")
	}
	// All symbols should be excluded.
	if len(resp.ExcludedSymbols) != len(symbols) {
		t.Errorf("expected %d excluded symbols, got %d: %v", len(symbols), len(resp.ExcludedSymbols), resp.ExcludedSymbols)
	}
}

func TestEfficientFrontier_Error_NumericalFailure(t *testing.T) {
	symbols := []string{"SYM_A", "SYM_B"}
	db, router := setupEfficientFrontier(t, symbols)

	// Insert identical price data for both symbols — this produces a
	// perfectly correlated (singular) covariance matrix.
	now := time.Now().UTC()
	baseDate := now.AddDate(0, 0, -320)
	for i := 0; i <= 320; i++ {
		date := baseDate.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")
		// Both symbols get the exact same price — correlation = 1.0.
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

	w, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	// The engine handles singular matrices via regularization (epsilon on diagonal)
	// and grid search fallback. It succeeds silently — no hard error or warning.
	// This verifies the full stack doesn't crash on perfectly correlated assets.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}

	// Frontier should still be computed (regularization makes the matrix invertible).
	if len(resp.Result.FrontierPoints) == 0 {
		t.Error("expected frontier points even with perfectly correlated assets")
	}

	// Key portfolios should be present.
	if resp.Result.MaxSharpe == nil {
		t.Error("expected max_sharpe portfolio")
	}
	if resp.Result.MinVariance == nil {
		t.Error("expected min_variance portfolio")
	}

	// Weights should sum to ~1.0.
	for i, pt := range resp.Result.FrontierPoints {
		sum := sumWeights(pt.Weights)
		if !almostEqual(sum, 1.0, 0.01) {
			t.Errorf("frontier point %d: weights sum to %f, expected ~1.0", i, sum)
		}
	}
}

func TestEfficientFrontier_SaveAsModelPortfolio(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	db, router := setupEfficientFrontier(t, symbols)

	insertFrontierMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	})

	// Step 1: Compute frontier.
	_, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: symbols,
		Period:  "1Y",
	})

	if resp.Result == nil || resp.Result.MaxSharpe == nil {
		t.Fatal("expected frontier result with max_sharpe")
	}

	// Step 2: Save max_sharpe as model portfolio.
	// Filter out zero-weight entries (model portfolio requires weights > 0).
	type saveEntry struct {
		Symbol string  `json:"symbol"`
		Weight float64 `json:"weight"`
	}
	entries := make([]saveEntry, 0, len(resp.Result.MaxSharpe.Weights))
	for i, wt := range resp.Result.MaxSharpe.Weights {
		if wt > 0.001 { // Only include symbols with meaningful weight
			entries = append(entries, saveEntry{
				Symbol: resp.Result.Symbols[i],
				Weight: wt,
			})
		}
	}
	// Normalize weights to sum to 1.0 after filtering.
	totalWeight := 0.0
	for _, e := range entries {
		totalWeight += e.Weight
	}
	if totalWeight > 0 {
		for i := range entries {
			entries[i].Weight = entries[i].Weight / totalWeight
		}
	}

	saveReq := struct {
		Name    string      `json:"name"`
		Entries []saveEntry `json:"entries"`
	}{
		Name:    "Efficient Frontier Max Sharpe",
		Entries: entries,
	}
	body, _ := json.Marshal(saveReq)
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var mp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&mp)

	if mp.Name != "Efficient Frontier Max Sharpe" {
		t.Errorf("expected name 'Efficient Frontier Max Sharpe', got %q", mp.Name)
	}
	if len(mp.Entries) == 0 {
		t.Fatal("expected at least 1 entry in saved model portfolio")
	}
	// Verify entries match the filtered symbols from the frontier.
	entrySymbols := make(map[string]bool)
	for _, e := range mp.Entries {
		entrySymbols[e.Symbol] = true
	}
	for _, e := range entries {
		if !entrySymbols[e.Symbol] {
			t.Errorf("expected entry for symbol %q", e.Symbol)
		}
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
	if retrieved.Name != "Efficient Frontier Max Sharpe" {
		t.Errorf("expected name 'Efficient Frontier Max Sharpe', got %q", retrieved.Name)
	}
}

func TestEfficientFrontier_GetCandidateSymbols(t *testing.T) {
	symbols := []string{"AAPL", "MSFT", "GOOGL"}
	_, router := setupEfficientFrontier(t, symbols)

	req := httptest.NewRequest(http.MethodGet, "/api/efficient-frontier/symbols", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Symbols []string `json:"symbols"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	// Should include our test symbols.
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

func TestEfficientFrontier_TooManySymbols(t *testing.T) {
	symbols := []string{"AAPL", "MSFT", "GOOGL", "AMZN", "NVDA", "TSLA", "META", "JPM", "V", "JNJ", "PG"}
	_, router := setupEfficientFrontier(t, symbols)

	w, _ := callComputeFrontier(t, router, computeFrontierRequest{
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

func TestEfficientFrontier_CustomPeriod(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	db, router := setupEfficientFrontier(t, symbols)

	// Insert 3 years of data
	now := time.Now().UTC()
	baseDate := now.AddDate(-3, 0, 0)
	for i := 0; i <= 1095; i++ {
		date := baseDate.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")
		for sym, basePrice := range map[string]float64{"AAPL": 175.0, "MSFT": 400.0} {
			price := basePrice * (1.0 + float64(i)*0.0005)
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

	w, resp := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols:      symbols,
		Period:       "3Y",
		RiskFreeRate: 0.04,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if resp.Result == nil {
		t.Fatal("expected result to be non-nil")
	}
	if len(resp.Result.FrontierPoints) == 0 {
		t.Error("expected frontier points for 3Y period")
	}
	// Trading days should be ~756 for 3Y.
	if resp.Result.TradingDays < 600 || resp.Result.TradingDays > 900 {
		t.Errorf("expected trading days around 756 for 3Y, got %d", resp.Result.TradingDays)
	}
}

func TestEfficientFrontier_InvalidPeriod(t *testing.T) {
	_, router := setupEfficientFrontier(t, []string{"AAPL", "MSFT"})

	w, _ := callComputeFrontier(t, router, computeFrontierRequest{
		Symbols: []string{"AAPL", "MSFT"},
		Period:  "2Y", // Not a valid period
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

func TestEfficientFrontier_SaveAsModelPortfolio_InvalidName(t *testing.T) {
	_, router := setupEfficientFrontier(t, []string{"AAPL", "MSFT"})

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
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/save", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEfficientFrontier_SaveAsModelPortfolio_DuplicateName(t *testing.T) {
	symbols := []string{"AAPL", "MSFT"}
	_, router := setupEfficientFrontier(t, symbols)

	// Create a model portfolio first.
	createModelPortfolio(t, router, "Duplicate Test", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})

	// Try to save frontier result with same name.
	saveReq := struct {
		Name    string `json:"name"`
		Entries []struct {
			Symbol string  `json:"symbol"`
			Weight float64 `json:"weight"`
		} `json:"entries"`
	}{
		Name: "Duplicate Test",
		Entries: []struct {
			Symbol string  `json:"symbol"`
			Weight float64 `json:"weight"`
		}{
			{Symbol: "AAPL", Weight: 0.5},
			{Symbol: "MSFT", Weight: 0.5},
		},
	}
	body, _ := json.Marshal(saveReq)
	req := httptest.NewRequest(http.MethodPost, "/api/efficient-frontier/save", bytes.NewReader(body))
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
