package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
)

// mustDecimal parses a decimal string, panicking on error (test helper).
func mustDecimal(s string) decimal.Decimal {
	return decimal.MustParse(s)
}

// createModelPortfolio creates a model portfolio via API and returns its ID.
func createModelPortfolio(t *testing.T, router http.Handler, name string, entries []modelportfolio.ModelPortfolioEntry) int64 {
	t.Helper()
	body, _ := json.Marshal(modelportfolio.CreateRequest{
		Name:    name,
		Entries: entries,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create model portfolio %q: expected 201, got %d: %s", name, w.Code, w.Body.String())
	}
	var mp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&mp)
	return mp.ID
}

// createPortfolioViaAPI creates a portfolio via the API and returns its ID.
func createPortfolioViaAPI(t *testing.T, router http.Handler, name, currency string) int64 {
	t.Helper()
	body := json.RawMessage(`{"name": "` + name + `", "currency": "` + currency + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio %q: expected 201, got %d: %s", name, w.Code, w.Body.String())
	}
	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	return p.ID
}

// createAccountViaAPI creates an account via the API and returns its ID.
func createAccountViaAPI(t *testing.T, router http.Handler, name string, portfolioID int64) int64 {
	t.Helper()
	body := json.RawMessage(`{"name": "` + name + `", "portfolio_id": ` + fmt.Sprintf("%d", portfolioID) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account %q: expected 201, got %d: %s", name, w.Code, w.Body.String())
	}
	var a account.Account
	json.NewDecoder(w.Body).Decode(&a)
	return a.ID
}

// insertMarketDataRaw inserts market data rows directly into the DB.
func insertMarketDataRaw(t *testing.T, db *sql.DB, prices map[string]string) {
	t.Helper()
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

func TestModelPortfolio_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	id := createModelPortfolio(t, router, "Growth Model", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("40.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("30.0")},
		{Symbol: "GOOGL", WeightPct: mustDecimal("30.0")},
	})

	// GET the portfolio back
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/model-portfolios/%d", id), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var mp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&mp)

	if mp.Name != "Growth Model" {
		t.Errorf("expected name 'Growth Model', got %q", mp.Name)
	}
	if len(mp.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(mp.Entries))
	}
	if mp.Entries[0].Symbol != "AAPL" {
		t.Errorf("expected first symbol 'AAPL', got %q", mp.Entries[0].Symbol)
	}
}

func TestModelPortfolio_CreateInvalidWeights(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Weights sum to 70%, not 100%
	body := json.RawMessage(`{
		"name": "Bad Weights",
		"entries": [
			{"symbol": "AAPL", "weight_pct": "30.0"},
			{"symbol": "MSFT", "weight_pct": "40.0"}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid weight sum, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "weight_sum_not_100" {
		t.Errorf("expected error code 'weight_sum_not_100', got %q", errResp.Code)
	}
}

func TestModelPortfolio_CreateNegativeWeight(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	body := json.RawMessage(`{
		"name": "Negative Weight",
		"entries": [
			{"symbol": "AAPL", "weight_pct": "-10.0"},
			{"symbol": "MSFT", "weight_pct": "110.0"}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative weight, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelPortfolio_CreateWithInlineSymbol(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create a model portfolio with a symbol that doesn't exist yet (TSLA)
	id := createModelPortfolio(t, router, "New Symbol Model", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "TSLA", WeightPct: mustDecimal("50.0")},
	})

	// Verify portfolio was created
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/model-portfolios/%d", id), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var mp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&mp)
	if len(mp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(mp.Entries))
	}

	// Verify TSLA symbol was auto-created
	req = httptest.NewRequest(http.MethodGet, "/api/symbols", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 listing symbols, got %d: %s", w.Code, w.Body.String())
	}

	var symbols []struct {
		InternalSymbol string `json:"internal_symbol"`
	}
	json.NewDecoder(w.Body).Decode(&symbols)
	found := false
	for _, s := range symbols {
		if s.InternalSymbol == "TSLA" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected TSLA symbol to be auto-created")
	}
}

func TestModelPortfolio_Edit(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	id := createModelPortfolio(t, router, "Edit Test", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})

	// Update: change name and entries
	newName := "Updated Model"
	body, _ := json.Marshal(modelportfolio.UpdateRequest{
		Name: &newName,
		Entries: []modelportfolio.ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: mustDecimal("60.0")},
			{Symbol: "MSFT", WeightPct: mustDecimal("40.0")},
		},
	})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/model-portfolios/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var mp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&mp)

	if mp.Name != "Updated Model" {
		t.Errorf("expected name 'Updated Model', got %q", mp.Name)
	}
	if len(mp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(mp.Entries))
	}
	aaplPct := mp.Entries[0].WeightPct.String()
	if aaplPct != "60.0" {
		t.Errorf("expected AAPL weight 60.0, got %q", aaplPct)
	}
}

func TestModelPortfolio_Delete(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	id := createModelPortfolio(t, router, "Delete Me", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})

	// DELETE
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/model-portfolios/%d", id), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// GET should return 404
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/model-portfolios/%d", id), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelPortfolio_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	req := httptest.NewRequest(http.MethodGet, "/api/model-portfolios", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var portfolios []modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&portfolios)
	if portfolios == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(portfolios) != 0 {
		t.Errorf("expected 0 portfolios, got %d", len(portfolios))
	}
}

func TestModelPortfolio_CreateDuplicateName(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	createModelPortfolio(t, router, "Duplicate", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})

	// Try to create another with same name
	body := json.RawMessage(`{
		"name": "Duplicate",
		"entries": [{"symbol": "MSFT", "weight_pct": "100.0"}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/model-portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate name, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelPortfolio_ApplyAsTarget(t *testing.T) {
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create portfolio, account, symbols
	portfolioID := createPortfolioViaAPI(t, router, "Target Test", "USD")
	accountID := createAccountViaAPI(t, router, "Test Account", portfolioID)

	// Create symbols
	for _, sym := range []string{"AAPL", "MSFT", "GOOGL"} {
		body := json.RawMessage(fmt.Sprintf(`{"internal_symbol": "%s", "market_data_symbol": "%s"}`, sym, sym))
		req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create symbol %s: expected 201, got %d: %s", sym, w.Code, w.Body.String())
		}
	}

	// Deposit and buy
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 18000, -1800000)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "MSFT", 50, 42000, -2100000)

	// Insert market data
	insertMarketDataRaw(t, db, map[string]string{
		"AAPL":  "180.00",
		"MSFT":  "420.00",
		"GOOGL": "150.00",
	})

	// Create a model portfolio
	modelID := createModelPortfolio(t, router, "Target Model", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("40.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("30.0")},
		{Symbol: "$CASH", WeightPct: mustDecimal("30.0")},
	})

	// Fetch the model portfolio (simulates what the JS does on the allocation page)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/model-portfolios/%d", modelID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var mp modelportfolio.ModelPortfolio
	json.NewDecoder(w.Body).Decode(&mp)

	// Apply as target allocation (simulates user saving after loading model)
	targets := make([]struct {
		Symbol    string `json:"symbol"`
		TargetPct string `json:"target_pct"`
	}, len(mp.Entries))
	for i, e := range mp.Entries {
		targets[i] = struct {
			Symbol    string `json:"symbol"`
			TargetPct string `json:"target_pct"`
		}{Symbol: e.Symbol, TargetPct: e.WeightPct.String()}
	}
	targetBody, _ := json.Marshal(targets)
	req = httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), bytes.NewReader(targetBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify target was saved correctly
	req = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/allocation/target?portfolio_id=%d", portfolioID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get target: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var savedTargets []struct {
		Symbol    string `json:"symbol"`
		TargetPct string `json:"target_pct"`
	}
	json.NewDecoder(w.Body).Decode(&savedTargets)

	if len(savedTargets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(savedTargets))
	}

	targetMap := make(map[string]string)
	for _, ta := range savedTargets {
		targetMap[ta.Symbol] = ta.TargetPct
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

func TestModelPortfolio_WebPage_List_Renders200(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	req := httptest.NewRequest(http.MethodGet, "/model-portfolios", nil)
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

func TestModelPortfolio_WebPage_New_Renders200(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	req := httptest.NewRequest(http.MethodGet, "/model-portfolios/new", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelPortfolio_WebPage_Edit_Renders200(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	id := createModelPortfolio(t, router, "Edit Page Test", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/model-portfolios/%d/edit", id), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelPortfolio_WebPage_CreateRedirects(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	form := "name=Web+Create&symbol=AAPL&weight=50.0&symbol=MSFT&weight=50.0"
	req := httptest.NewRequest(http.MethodPost, "/model-portfolios", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect after create, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelPortfolio_WebPage_DeleteRedirects(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	id := createModelPortfolio(t, router, "Web Delete", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/model-portfolios/%d/delete", id), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect after delete, got %d: %s", w.Code, w.Body.String())
	}
}
