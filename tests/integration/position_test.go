package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
)

// setupPos creates a portfolio, account, and symbol mapping for position tests.
func setupPos(t *testing.T) (db *sql.DB, router http.Handler, portfolioID int64, accountID int64) {
	t.Helper()
	db = setupTestDB(t)
	router = api.Router(db, testLogger())

	// Create portfolio
	body := json.RawMessage(`{"name": "Test Portfolio", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}
	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	portfolioID = p.ID

	// Create account
	body = json.RawMessage(`{"name": "Test Account", "portfolio_id": ` + fmt.Sprintf("%d", portfolioID) + `}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d", w.Code)
	}
	var a account.Account
	json.NewDecoder(w.Body).Decode(&a)
	accountID = a.ID

	// Create symbol mapping for AAPL
	body = json.RawMessage(`{"internal_symbol": "AAPL", "market_data_symbol": "AAPL"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/symbol-mappings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol mapping: expected 201, got %d", w.Code)
	}

	return db, router, portfolioID, accountID
}

func createTransaction(t *testing.T, router http.Handler, accountID int64, date, txType, symbol string, quantity int, priceCents int, netCashCents int) transaction.Transaction {
	t.Helper()
	body := fmt.Sprintf(
		`{"account_id":%d,"date":"%s","type":"%s","symbol":"%s","quantity":%d,"price":%d,"currency":"USD","net_cash":%d}`,
		accountID, date, txType, symbol, quantity, priceCents, netCashCents,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create transaction %s %s: expected 201, got %d: %s", txType, symbol, w.Code, w.Body.String())
	}
	var tx transaction.Transaction
	json.NewDecoder(w.Body).Decode(&tx)
	return tx
}

// TestPosition_FullRecalcFlow verifies: create transactions → positions computed → verify via API.
func TestPosition_FullRecalcFlow(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Deposit
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 10000, 1, 1000000)

	// Buy 10 AAPL @ $150 (price=15000 cents, net_cash=-150000 cents = -$1,500)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 15000, -150000)

	// Sell 3 AAPL @ $170 (price=17000 cents, net_cash=51000 cents = $510)
	createTransaction(t, router, accountID, "2025-03-20", "sell", "AAPL", -3, 17000, 51000)

	// List open positions
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)

	// Should have AAPL open position + cash position
	foundAAPL := false
	foundCash := false
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			foundAAPL = true
			if p.Quantity.String() != "7" {
				t.Errorf("expected AAPL quantity 7, got %q", p.Quantity.String())
			}
			if p.IsClosed {
				t.Error("expected AAPL position to be open")
			}
		}
		if p.Symbol == "$CASH-USD" {
			foundCash = true
			// Cash: deposit 1000000 - buy 150000 + sell 51000 = 901000 (net_cash sum in cents)
			if p.Quantity.String() != "901000" {
				t.Errorf("expected cash balance 901000, got %q", p.Quantity.String())
			}
		}
	}

	if !foundAAPL {
		t.Error("expected AAPL open position")
	}
	if !foundCash {
		t.Error("expected $CASH-USD position")
	}
}

// TestPosition_FIFOWithRealSQL verifies FIFO matching with real database.
func TestPosition_FIFOWithRealSQL(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Buy lot 1: 100 shares @ $150 (Jan)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 15000, -1500000)

	// Buy lot 2: 50 shares @ $160 (Feb)
	createTransaction(t, router, accountID, "2025-02-20", "buy", "AAPL", 50, 16000, -800000)

	// Sell 120 shares @ $170 (Mar) — consumes all 100 from lot 1 + 20 from lot 2
	createTransaction(t, router, accountID, "2025-03-20", "sell", "AAPL", -120, 17000, 2040000)

	// List open positions
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)

	// Should have 1 AAPL open position with 30 shares remaining
	foundAAPL := false
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			foundAAPL = true
			if p.Quantity.String() != "30" {
				t.Errorf("expected AAPL quantity 30, got %q", p.Quantity.String())
			}
			if p.IsClosed {
				t.Error("expected AAPL position to be open")
			}
		}
	}
	if !foundAAPL {
		t.Error("expected AAPL open position with 30 shares remaining")
	}
}

// TestPosition_CashTracking verifies cash position through deposits/withdrawals/trades.
func TestPosition_CashTracking(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Deposit $10,000
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 10000, 1, 1000000)

	// Withdraw $2,000
	createTransaction(t, router, accountID, "2025-02-01", "withdrawal", "$CASH-USD", 2000, 1, -200000)

	// Buy 10 AAPL @ $150 (costs $1,500)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 15000, -150000)

	// Sell 5 AAPL @ $170 (proceeds $850)
	createTransaction(t, router, accountID, "2025-03-20", "sell", "AAPL", -5, 17000, 85000)

	// List open positions
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)

	// Cash: 1000000 - 200000 - 150000 + 85000 = 735000 (net_cash sum in cents)
	for _, p := range positions {
		if p.Symbol == "$CASH-USD" {
			if p.Quantity.String() != "735000" {
				t.Errorf("expected cash balance 735000, got %q", p.Quantity.String())
			}
			return
		}
	}
	t.Error("expected $CASH-USD position")
}

// TestPosition_Transitions verifies open → closed → open again.
func TestPosition_Transitions(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Buy 100 AAPL @ $150 (Jan)
	createTransaction(t, router, accountID, "2025-01-01", "buy", "AAPL", 100, 15000, -1500000)

	// Sell all 100 @ $170 (Feb) — closes position
	createTransaction(t, router, accountID, "2025-02-01", "sell", "AAPL", -100, 17000, 1700000)

	// Buy 50 more @ $180 (Mar) — opens new position
	createTransaction(t, router, accountID, "2025-03-01", "buy", "AAPL", 50, 18000, -900000)

	// List open positions — should have 1 open (50 shares)
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var openPositions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&openPositions)

	aaplOpen := false
	for _, p := range openPositions {
		if p.Symbol == "AAPL" {
			aaplOpen = true
			if p.Quantity.String() != "50" {
				t.Errorf("expected open AAPL quantity 50, got %q", p.Quantity.String())
			}
		}
	}
	if !aaplOpen {
		t.Error("expected open AAPL position")
	}

	// List closed positions — should have 1 closed (100 shares)
	req = httptest.NewRequest(http.MethodGet, "/api/positions/closed", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var closedPositions []position.Position
	json.NewDecoder(w.Body).Decode(&closedPositions)

	aaplClosed := false
	for _, p := range closedPositions {
		if p.Symbol == "AAPL" {
			aaplClosed = true
			if p.Quantity.String() != "100" {
				t.Errorf("expected closed AAPL quantity 100, got %q", p.Quantity.String())
			}
			if !p.IsClosed {
				t.Error("expected closed position to be marked closed")
			}
		}
	}
	if !aaplClosed {
		t.Error("expected closed AAPL position")
	}
}

// TestPosition_EmptyAccount verifies empty account returns empty position lists.
func TestPosition_EmptyAccount(t *testing.T) {
	_, router, _, _ := setupPos(t)

	// List open positions
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)
	// Should include cash positions from the account (if any) or be empty
	// With no transactions, should be empty or just the account's cash
	if len(positions) != 0 {
		t.Errorf("expected 0 open positions, got %d", len(positions))
	}

	// List closed positions
	req = httptest.NewRequest(http.MethodGet, "/api/positions/closed", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var closed []position.Position
	json.NewDecoder(w.Body).Decode(&closed)
	if len(closed) != 0 {
		t.Errorf("expected 0 closed positions, got %d", len(closed))
	}
}

// TestPosition_Recalculate verifies manual recalculation endpoint.
func TestPosition_Recalculate(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Create some transactions
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 15000, -150000)

	// Manual recalculate for this account
	recalcBody := fmt.Sprintf(`{"account_id": %d}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate", bytes.NewBufferString(recalcBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("recalculate: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify positions are still correct after manual recalc
	req = httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)

	foundAAPL := false
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			foundAAPL = true
			if p.Quantity.String() != "10" {
				t.Errorf("expected AAPL quantity 10 after recalc, got %q", p.Quantity.String())
			}
		}
	}
	if !foundAAPL {
		t.Error("expected AAPL position after manual recalc")
	}
}

// TestPosition_RecalculateIdempotent verifies running recalc twice produces same result.
func TestPosition_RecalculateIdempotent(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Create transactions
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 15000, -150000)
	createTransaction(t, router, accountID, "2025-03-20", "sell", "AAPL", -3, 17000, 51000)

	// Recalculate first time
	recalcBody := fmt.Sprintf(`{"account_id": %d}`, accountID)
	req := httptest.NewRequest(http.MethodPost, "/api/positions/recalculate", bytes.NewBufferString(recalcBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first recalculate: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Get positions after first recalc
	req = httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var positions1 []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions1)

	// Recalculate second time
	req = httptest.NewRequest(http.MethodPost, "/api/positions/recalculate", bytes.NewBufferString(recalcBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second recalculate: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Get positions after second recalc
	req = httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var positions2 []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions2)

	// Compare: same number of positions, same quantities
	if len(positions1) != len(positions2) {
		t.Fatalf("position count changed after second recalc: %d vs %d", len(positions1), len(positions2))
	}

	for i := range positions1 {
		if positions1[i].Symbol != positions2[i].Symbol {
			t.Errorf("position %d symbol mismatch: %q vs %q", i, positions1[i].Symbol, positions2[i].Symbol)
		}
		if positions1[i].Quantity.String() != positions2[i].Quantity.String() {
			t.Errorf("position %d quantity mismatch: %q vs %q", i, positions1[i].Quantity.String(), positions2[i].Quantity.String())
		}
	}
}

// TestPosition_FilterByAccount verifies position filtering by account.
func TestPosition_FilterByAccount(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Create a second account
	acctBody := json.RawMessage(`{"name": "Account 2", "portfolio_id": 1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(acctBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account 2: expected 201, got %d", w.Code)
	}
	var a2 account.Account
	json.NewDecoder(w.Body).Decode(&a2)

	// Buy on account 1
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 15000, -150000)

	// Buy on account 2
	createTransaction(t, router, a2.ID, "2025-01-15", "buy", "AAPL", 20, 15000, -300000)

	// Filter by account 1
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/positions?account_id=%d", accountID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)

	// Should only have positions from account 1
	aaplCount := 0
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			aaplCount++
			if p.Quantity.String() != "10" {
				t.Errorf("expected quantity 10 for account %d, got %q", accountID, p.Quantity.String())
			}
		}
	}
	if aaplCount != 1 {
		t.Errorf("expected 1 AAPL position for account %d, got %d", accountID, aaplCount)
	}
}

// TestPosition_ClosedPositions verifies closed positions endpoint.
func TestPosition_ClosedPositions(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Buy 100 @ $150
	createTransaction(t, router, accountID, "2025-01-01", "buy", "AAPL", 100, 15000, -1500000)

	// Sell all 100 @ $170
	createTransaction(t, router, accountID, "2025-02-01", "sell", "AAPL", -100, 17000, 1700000)

	// Open positions should be empty (no AAPL)
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var openPositions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&openPositions)
	for _, p := range openPositions {
		if p.Symbol == "AAPL" {
			t.Error("expected no open AAPL position (all shares sold)")
		}
	}

	// Closed positions should have 1 AAPL
	req = httptest.NewRequest(http.MethodGet, "/api/positions/closed", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var closedPositions []position.Position
	json.NewDecoder(w.Body).Decode(&closedPositions)

	found := false
	for _, p := range closedPositions {
		if p.Symbol == "AAPL" {
			found = true
			if !p.IsClosed {
				t.Error("expected closed position")
			}
			if p.Quantity.String() != "100" {
				t.Errorf("expected quantity 100, got %q", p.Quantity.String())
			}
		}
	}
	if !found {
		t.Error("expected closed AAPL position")
	}
}

// TestPosition_LotDetail verifies lot detail endpoint.
func TestPosition_LotDetail(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Buy with explicit lot_id
	lotID := "LOT-TEST-001"
	body := fmt.Sprintf(
		`{"account_id":%d,"date":"2025-01-15","type":"buy","symbol":"AAPL","quantity":10,"price":15000,"currency":"USD","net_cash":-150000,"lot_id":"%s"}`,
		accountID, lotID,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create buy: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Get lot detail
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/lots/%s", lotID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get lot: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var lotResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&lotResp)

	if lotType, ok := lotResp["lot_type"].(string); !ok || lotType != "buy" {
		t.Errorf("expected lot_type 'buy', got %v", lotType)
	}
	if quantity, ok := lotResp["quantity"].(string); !ok || quantity != "10" {
		t.Errorf("expected quantity '10', got %v", quantity)
	}
}

// TestPosition_CascadeDelete verifies positions are cleaned up when account is deleted.
func TestPosition_CascadeDelete(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Create a buy transaction (triggers position creation)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 10, 15000, -150000)

	// Verify position exists
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)
	aaplFound := false
	for _, p := range positions {
		if p.Symbol == "AAPL" {
			aaplFound = true
		}
	}
	if !aaplFound {
		t.Fatal("expected AAPL position before account delete")
	}

	// Delete the account
	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/accounts/%d", accountID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, deleteReq)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete account: expected 204, got %d", w.Code)
	}

	// Verify positions are gone
	req = httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var remaining []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&remaining)
	for _, p := range remaining {
		if p.Symbol == "AAPL" {
			t.Error("expected AAPL position to be deleted with account")
		}
	}
}

// TestPosition_MultipleCycles verifies multiple open-to-close cycles.
func TestPosition_MultipleCycles(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Cycle 1: buy 100, sell 100
	createTransaction(t, router, accountID, "2025-01-01", "buy", "AAPL", 100, 15000, -1500000)
	createTransaction(t, router, accountID, "2025-02-01", "sell", "AAPL", -100, 17000, 1700000)

	// Cycle 2: buy 50, sell 50
	createTransaction(t, router, accountID, "2025-03-01", "buy", "AAPL", 50, 18000, -900000)
	createTransaction(t, router, accountID, "2025-04-01", "sell", "AAPL", -50, 19000, 950000)

	// No open positions
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var openPositions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&openPositions)
	for _, p := range openPositions {
		if p.Symbol == "AAPL" {
			t.Error("expected no open AAPL position after two full cycles")
		}
	}

	// Two closed positions
	req = httptest.NewRequest(http.MethodGet, "/api/positions/closed", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var closedPositions []position.Position
	json.NewDecoder(w.Body).Decode(&closedPositions)

	closedCount := 0
	for _, p := range closedPositions {
		if p.Symbol == "AAPL" {
			closedCount++
		}
	}
	if closedCount != 2 {
		t.Errorf("expected 2 closed AAPL positions, got %d", closedCount)
	}
}

// TestPosition_DividendWithNoOpenPosition verifies dividend affects cash even without open share position.
func TestPosition_DividendWithNoOpenPosition(t *testing.T) {
	_, router, _, accountID := setupPos(t)

	// Deposit
	createTransaction(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", 10000, 1, 1000000)

	// Buy then sell all (position closed)
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 15000, -1500000)
	createTransaction(t, router, accountID, "2025-02-01", "sell", "AAPL", -100, 17000, 1700000)

	// Dividend on AAPL (no open position)
	createTransaction(t, router, accountID, "2025-03-01", "dividend", "AAPL", 1, 300, 30000)

	// Check cash position
	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var positions []position.PositionWithMarket
	json.NewDecoder(w.Body).Decode(&positions)

	// Cash: deposit 1000000 - buy 1500000 + sell 1700000 + dividend 30000 = 1230000 (net_cash sum in cents)
	for _, p := range positions {
		if p.Symbol == "$CASH-USD" {
			if p.Quantity.String() != "1230000" {
				t.Errorf("expected cash balance 1230000, got %q", p.Quantity.String())
			}
			return
		}
	}
	t.Error("expected $CASH-USD position")
}

// TestPosition_FxRatePersisted verifies that FxRateUsed and FxRateFallback
// are persisted to the database during recalculation and survive a reload.
func TestPosition_FxRatePersisted(t *testing.T) {
	db, router, _, accountID := setupPos(t)

	// Create a closed position: buy then sell all
	createTransaction(t, router, accountID, "2025-01-15", "buy", "AAPL", 100, 15000, -1500000)
	createTransaction(t, router, accountID, "2025-03-20", "sell", "AAPL", -100, 17000, 1700000)

	// Query the database directly to check fx_rate_used is persisted
	var fxRateUsed string
	var fxRateFallback bool
	err := db.QueryRow(`
		SELECT fx_rate_used, fx_rate_fallback FROM positions
		WHERE account_id = ? AND symbol = 'AAPL' AND is_closed = 1
	`, accountID).Scan(&fxRateUsed, &fxRateFallback)
	if err != nil {
		t.Fatalf("query fx_rate_used: %v", err)
	}

	// Same-currency position (USD/USD) should have fx_rate_used = '1'
	if fxRateUsed != "1" {
		t.Errorf("expected fx_rate_used = '1' for same-currency closed position, got %q", fxRateUsed)
	}
	if fxRateFallback {
		t.Error("expected fx_rate_fallback = false for same-currency position, got true")
	}
}
