package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eddiectc/portfoliolab/internal/domain/account"
	"github.com/eddiectc/portfoliolab/internal/domain/allocation"
	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
)

// setupAlloc creates a portfolio, account, and symbol mappings for allocation tests.
func setupAlloc(t *testing.T) (*sql.DB, http.Handler, int64, int64) {
	t.Helper()
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Alloc Test", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var p portfolio.Portfolio
	_ = json.NewDecoder(w.Body).Decode(&p)
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
	var a account.Account
	_ = json.NewDecoder(w.Body).Decode(&a)
	accountID := a.ID

	// Create symbol mappings
	for _, sym := range []string{"AAPL", "MSFT", "GOOGL"} {
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

// createTransactionWithCurrency creates a transaction with a specified currency.
func createTransactionWithCurrency(t *testing.T, router http.Handler, accountID int64, date, txType, symbol string, quantity int, priceCents int, netCashCents int, currency string) {
	t.Helper()
	body := fmt.Sprintf(
		`{"account_id":%d,"date":"%s","type":"%s","symbol":"%s","quantity":%d,"price":%d,"currency":"%s","net_cash":%d}`,
		accountID, date, txType, symbol, quantity, priceCents, currency, netCashCents,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create transaction %s %s: expected 201, got %d: %s", txType, symbol, w.Code, w.Body.String())
	}
}

// insertAllocMarketData inserts current prices for allocation test symbols.
func insertAllocMarketData(t *testing.T, db *sql.DB) {
	t.Helper()

	prices := map[string]string{
		"AAPL":  "180.00",
		"MSFT":  "420.00",
		"GOOGL": "150.00",
	}

	for sym, price := range prices {
		_, err := db.Exec(
			`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
			 VALUES (?, ?, 'USD', 'stock', 'yahoo', '')`,
			sym, price,
		)
		if err != nil {
			t.Fatalf("insert market data %s: %v", sym, err)
		}
	}
}

func TestAllocation_Basic(t *testing.T) {
	db, router, _, accountID := setupAlloc(t)

	// Deposit $100,000
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)

	// Buy 100 AAPL @ $180 = $18,000
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 18000, -1800000)

	// Buy 50 MSFT @ $420 = $21,000
	createTransaction(t, router, accountID, "2025-01-15", "buy", "MSFT", 50, 42000, -2100000)

	// Insert market data (current prices)
	insertAllocMarketData(t, db)

	// GET /api/allocation
	req := httptest.NewRequest(http.MethodGet, "/api/allocation", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result allocation.AllocationResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Should have AAPL, MSFT, and Cash rows
	if len(result.Rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(result.Rows))
	}

	// Find AAPL and MSFT
	var aaplPct, msftPct string
	for _, row := range result.Rows {
		if row.Symbol == "AAPL" {
			aaplPct = row.AllocationPct.String()
		}
		if row.Symbol == "MSFT" {
			msftPct = row.AllocationPct.String()
		}
	}

	// AAPL: 100 * $180 = $18,000; MSFT: 50 * $420 = $21,000
	// Cash: $10,000,000 cents - $1,800,000 - $2,100,000 = $6,100,000 cents = $61,000
	// Total = $18,000 + $21,000 + $61,000 = $100,000
	// AAPL: 18%, MSFT: 21%, Cash: 61%
	if aaplPct == "" {
		t.Error("expected AAPL row in allocation")
	}
	if msftPct == "" {
		t.Error("expected MSFT row in allocation")
	}

	// Cash row should be present
	if result.CashRow == nil {
		t.Error("expected cash row in allocation")
	} else if result.CashRow.Symbol != "$CASH" {
		t.Errorf("expected cash symbol '$CASH', got %q", result.CashRow.Symbol)
	}

	// Base currency should be USD
	if result.BaseCurrency != "USD" {
		t.Errorf("expected base currency 'USD', got %q", result.BaseCurrency)
	}

	// Market data should be available
	if !result.MarketDataAvailable {
		t.Error("expected market data to be available")
	}
}

func TestAllocation_TargetSaveAndGet(t *testing.T) {
	_, router, portfolioID, _ := setupAlloc(t)

	// Save target: AAPL 40%, MSFT 30%, $CASH 30%
	body := json.RawMessage(`[
		{"symbol": "AAPL", "target_pct": "40.0"},
		{"symbol": "MSFT", "target_pct": "30.0"},
		{"symbol": "$CASH", "target_pct": "30.0"}
	]`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var statusResp struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(w.Body).Decode(&statusResp)
	if statusResp.Status != "saved" {
		t.Errorf("expected status 'saved', got %q", statusResp.Status)
	}

	// GET target allocations
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var targets []allocation.TargetAllocation
	if err := json.NewDecoder(w.Body).Decode(&targets); err != nil {
		t.Fatalf("decode targets: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	// Verify each target
	targetMap := make(map[string]string)
	for _, ta := range targets {
		targetMap[ta.Symbol] = ta.TargetPct.String()
	}

	if pct, ok := targetMap["AAPL"]; !ok || pct != "40.0" {
		t.Errorf("expected AAPL target 40.0, got %q", pct)
	}
	if pct, ok := targetMap["MSFT"]; !ok || pct != "30.0" {
		t.Errorf("expected MSFT target 30.0, got %q", pct)
	}
	if pct, ok := targetMap["$CASH"]; !ok || pct != "30.0" {
		t.Errorf("expected $CASH target 30.0, got %q", pct)
	}
}

func TestAllocation_TargetInvalidSum(t *testing.T) {
	_, router, portfolioID, _ := setupAlloc(t)

	// Save target with sum ≠ 100 (AAPL 50% + MSFT 20% = 70%)
	body := json.RawMessage(`[
		{"symbol": "AAPL", "target_pct": "50.0"},
		{"symbol": "MSFT", "target_pct": "20.0"}
	]`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid target sum, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "target_sum_not_100" {
		t.Errorf("expected error code target_sum_not_100, got %q", errResp.Code)
	}
}

func TestAllocation_TargetInvalidPct(t *testing.T) {
	_, router, portfolioID, _ := setupAlloc(t)

	// Save target with negative percentage
	body := json.RawMessage(`[
		{"symbol": "AAPL", "target_pct": "-10.0"},
		{"symbol": "MSFT", "target_pct": "110.0"}
	]`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid target pct, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocation_Drift(t *testing.T) {
	db, router, portfolioID, accountID := setupAlloc(t)

	// Deposit and buy
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 18000, -1800000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "MSFT", 50, 42000, -2100000)
	insertAllocMarketData(t, db)

	// Save target: AAPL 50%, MSFT 30%, $CASH 20%
	body := json.RawMessage(`[
		{"symbol": "AAPL", "target_pct": "50.0"},
		{"symbol": "MSFT", "target_pct": "30.0"},
		{"symbol": "$CASH", "target_pct": "20.0"}
	]`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// GET drift
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/allocation/drift?portfolio_id=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var drift allocation.DriftResult
	if err := json.NewDecoder(w.Body).Decode(&drift); err != nil {
		t.Fatalf("decode drift: %v", err)
	}

	if !drift.HasTarget {
		t.Error("expected has_target to be true")
	}

	// Should have drift rows for AAPL, MSFT, and Cash
	if len(drift.Rows) < 3 {
		t.Fatalf("expected at least 3 drift rows, got %d", len(drift.Rows))
	}

	// AAPL actual ~18%, target 50% → drift ~-32% (underweight, not balanced)
	for _, row := range drift.Rows {
		if row.Symbol == "AAPL" {
			if row.IsBalanced {
				t.Error("expected AAPL to be unbalanced (drift > 5%)")
			}
			// Drift should be negative (actual < target)
			if !row.DriftPct.IsNeg() {
				t.Error("expected AAPL drift to be negative (underweight)")
			}
		}
	}
}

func TestAllocation_Rebalance(t *testing.T) {
	db, router, portfolioID, accountID := setupAlloc(t)

	// Deposit and buy
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 18000, -1800000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "MSFT", 50, 42000, -2100000)
	insertAllocMarketData(t, db)

	// Save target with significant drift: AAPL 50%, MSFT 30%, $CASH 20%
	body := json.RawMessage(`[
		{"symbol": "AAPL", "target_pct": "50.0"},
		{"symbol": "MSFT", "target_pct": "30.0"},
		{"symbol": "$CASH", "target_pct": "20.0"}
	]`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// GET rebalance
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/allocation/rebalance?portfolio_id=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var rebalance allocation.RebalanceResult
	if err := json.NewDecoder(w.Body).Decode(&rebalance); err != nil {
		t.Fatalf("decode rebalance: %v", err)
	}

	// Should have suggestions (AAPL and MSFT are far from target)
	if len(rebalance.Suggestions) == 0 {
		t.Error("expected rebalancing suggestions for significant drift")
	}

	// Should not be balanced
	if rebalance.IsBalanced {
		t.Error("expected portfolio to not be balanced")
	}

	// Suggestions should include buy for underweight symbols
	for _, s := range rebalance.Suggestions {
		if s.Symbol == "AAPL" && s.Direction != "buy" {
			t.Errorf("expected AAPL direction 'buy' (underweight), got %q", s.Direction)
		}
		if s.Symbol == "MSFT" && s.Direction != "buy" {
			t.Errorf("expected MSFT direction 'buy' (underweight), got %q", s.Direction)
		}
		// Shares should be positive
		if s.Shares.IsNeg() {
			t.Errorf("expected positive shares for %s, got %s", s.Symbol, s.Shares.String())
		}
	}
}

func TestAllocation_MultiCurrencyCash(t *testing.T) {
	db, router, _, _ := setupAlloc(t)

	// Create a GBP portfolio
	body := json.RawMessage(`{"name": "GBP Portfolio", "currency": "GBP"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create GBP portfolio: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var gp portfolio.Portfolio
	_ = json.NewDecoder(w.Body).Decode(&gp)
	gbpPortfolioID := gp.ID

	// Create GBP account
	body = json.RawMessage(`{"name": "GBP Account", "portfolio_id": ` + fmt.Sprintf("%d", gbpPortfolioID) + `}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create GBP account: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var ga account.Account
	_ = json.NewDecoder(w.Body).Decode(&ga)
	gbpAccountID := ga.ID

	// Insert GBP→USD FX rate (1 GBP = 1.27 USD)
	_, err := db.Exec(
		`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
		 VALUES (?, ?, 'USD', 'fx', 'yahoo', '')`,
		"GBPUSD", "1.27",
	)
	if err != nil {
		t.Fatalf("insert FX rate: %v", err)
	}

	// Deposit £10,000 GBP (use GBP currency for this portfolio)
	createTransactionWithCurrency(t, router, gbpAccountID, "2025-01-01", "deposit", "$CASH-GBP", 10000, 1, 1000000, "GBP")

	// Buy AAPL (in GBP account, priced in GBP)
	createTransactionWithCurrency(t, router, gbpAccountID, "2025-01-15", "buy", "AAPL", 10, 18000, -180000, "GBP")
	insertAllocMarketData(t, db)

	// GET allocation for GBP portfolio
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/allocation?portfolio_ids=%d", gbpPortfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result allocation.AllocationResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Base currency should be GBP
	if result.BaseCurrency != "GBP" {
		t.Errorf("expected base currency 'GBP', got %q", result.BaseCurrency)
	}

	// Should have AAPL row and cash row
	if len(result.Rows) == 0 {
		t.Error("expected at least 1 allocation row for GBP portfolio")
	}
	if result.CashRow == nil {
		t.Error("expected cash row for GBP portfolio")
	}
}

func TestAllocation_WebPage_Renders200(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db, router, _, accountID := setupAlloc(t)

	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 18000, -1800000)
	insertAllocMarketData(t, db)

	req := httptest.NewRequest(http.MethodGet, "/allocation", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", contentType)
	}
}

func TestAllocation_EmptyPortfolio(t *testing.T) {
	_, router, _, _ := setupAlloc(t)

	// GET /api/allocation with no transactions
	req := httptest.NewRequest(http.MethodGet, "/api/allocation", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result allocation.AllocationResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Should have an empty-state message
	if result.Message == "" {
		t.Error("expected empty-state message for portfolio with no positions")
	}

	// No rows
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestAllocation_DeleteTarget(t *testing.T) {
	_, router, portfolioID, _ := setupAlloc(t)

	// Save target first
	body := json.RawMessage(`[
		{"symbol": "AAPL", "target_pct": "50.0"},
		{"symbol": "MSFT", "target_pct": "50.0"}
	]`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify target exists
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get target: expected 200, got %d", w.Code)
	}

	var targets []allocation.TargetAllocation
	_ = json.NewDecoder(w.Body).Decode(&targets)
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets before delete, got %d", len(targets))
	}

	// DELETE all targets
	req = httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var statusResp struct {
		Status string `json:"status"`
		Scope  string `json:"scope"`
	}
	_ = json.NewDecoder(w.Body).Decode(&statusResp)
	if statusResp.Status != "deleted" {
		t.Errorf("expected status 'deleted', got %q", statusResp.Status)
	}
	if statusResp.Scope != "all" {
		t.Errorf("expected scope 'all', got %q", statusResp.Scope)
	}

	// Verify targets are gone
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get target after delete: expected 200, got %d", w.Code)
	}

	var remaining []allocation.TargetAllocation
	_ = json.NewDecoder(w.Body).Decode(&remaining)
	if len(remaining) != 0 {
		t.Errorf("expected 0 targets after delete, got %d", len(remaining))
	}
}

func TestAllocation_WebPage_Empty_Renders200(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	_, router, _, _ := setupAlloc(t)

	// No transactions — empty portfolio
	req := httptest.NewRequest(http.MethodGet, "/allocation", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocation_WebPage_WithDrift_Renders200(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db, router, portfolioID, accountID := setupAlloc(t)

	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 18000, -1800000)
	insertAllocMarketData(t, db)

	// Save target
	body := json.RawMessage(`[
		{"symbol": "AAPL", "target_pct": "50.0"},
		{"symbol": "$CASH", "target_pct": "50.0"}
	]`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// GET /allocation?portfolio_ids=X (single portfolio → shows drift section)
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/allocation?portfolio_ids=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", contentType)
	}
}

func TestAllocation_NoPortfolios(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// GET /api/allocation with no portfolios at all
	req := httptest.NewRequest(http.MethodGet, "/api/allocation", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result allocation.AllocationResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Should have an empty-state message
	if result.Message == "" {
		t.Error("expected empty-state message when no portfolios exist")
	}
}

func TestAllocation_MissingPortfolioID(t *testing.T) {
	_, router, _, _ := setupAlloc(t)

	// GET /api/allocation/target without portfolio_id
	req := httptest.NewRequest(http.MethodGet, "/api/allocation/target", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing portfolio_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocation_WebPage_SaveTarget_Redirects(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	_, router, portfolioID, _ := setupAlloc(t)

	// POST /allocation/target (web form submission)
	form := fmt.Sprintf("portfolio_id=%d&symbol_0=AAPL&target_pct_0=50.0&symbol_1=MSFT&target_pct_1=50.0", portfolioID)
	req := httptest.NewRequest(http.MethodPost, "/allocation/target", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Should redirect (303 See Other) after saving
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect after saving target, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocation_WebPage_SaveTarget_InvalidRedirect(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	_, router, portfolioID, _ := setupAlloc(t)

	// POST /allocation/target with sum ≠ 100
	form := fmt.Sprintf("portfolio_id=%d&symbol_0=AAPL&target_pct_0=30.0&symbol_1=MSFT&target_pct_1=20.0", portfolioID)
	req := httptest.NewRequest(http.MethodPost, "/allocation/target", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Should redirect with error flash
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect (with error flash), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocation_WebPage_DeleteTarget_Redirects(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	_, router, portfolioID, _ := setupAlloc(t)

	// POST /allocation/target/delete
	form := fmt.Sprintf("portfolio_id=%d", portfolioID)
	req := httptest.NewRequest(http.MethodPost, "/allocation/target/delete", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect after deleting target, got %d: %s", w.Code, w.Body.String())
	}
}
