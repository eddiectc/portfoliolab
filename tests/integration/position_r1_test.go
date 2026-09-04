package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/govalues/decimal"
)

// createDecimalTxn creates a transaction with non-integer decimal values
// (the generic createTransaction helper only takes integer cents).
func createDecimalTxn(t *testing.T, router http.Handler, accountID int64, date, txType, symbol, quantity, price, netCash, currency string) {
	t.Helper()
	body := fmt.Sprintf(
		`{"account_id":%d,"date":"%s","type":"%s","symbol":"%s","quantity":%s,"price":%s,"currency":"%s","net_cash":%s}`,
		accountID, date, txType, symbol, quantity, price, currency, netCash,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create transaction %s %s: expected 201, got %d: %s", txType, symbol, w.Code, w.Body.String())
	}
}

func createSymbolMapping(t *testing.T, router http.Handler, symbol string) {
	t.Helper()
	body := json.RawMessage(fmt.Sprintf(`{"internal_symbol": %q, "market_data_symbol": %q}`, symbol, symbol))
	req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol mapping %s: expected 201, got %d: %s", symbol, w.Code, w.Body.String())
	}
}

// TestPositionR1_VWRPPartialSell is the regression scenario for revision R1:
// buy 666 VWRP.L @ £105.00 (2024-07-01), sell 18 VWRP.L @ £144.60 (2025-07-15).
//
// Expected per SPEC R1:
//   - open row:   648 shares, cost basis −68,040.00
//   - closed row: 18 shares, realized P&L +712.80, 2024-07-01 → 2025-07-15
//   - cash: the 2,602.80 of sale proceeds flows in exactly once (no
//     double counting into both cash and the closed row)
func TestPositionR1_VWRPPartialSell(t *testing.T) {
	db, router, _, accountID := setupPos(t)
	createSymbolMapping(t, router, "VWRP.L")

	createDecimalTxn(t, router, accountID, "2024-07-01", "buy", "VWRP.L", "666", "105.00", "-69930.00", "GBP")
	createDecimalTxn(t, router, accountID, "2025-07-15", "sell", "VWRP.L", "-18", "144.60", "2602.80", "GBP")

	// --- Open row: 648 shares, cost attributed from the unconsumed buy lot.
	var openQty, openCost string
	err := db.QueryRow(`
		SELECT quantity, cost_basis FROM positions
		WHERE account_id = ? AND symbol = 'VWRP.L' AND is_closed = 0
	`, accountID).Scan(&openQty, &openCost)
	if err != nil {
		t.Fatalf("query open VWRP.L position: %v", err)
	}
	if got, want := decimal.MustParse(openQty), decimal.MustNew(648, 0); !got.Equal(want) {
		t.Errorf("open quantity: expected %s, got %s", want, got)
	}
	if got, want := decimal.MustParse(openCost), decimal.MustNew(-6804000, 2); !got.Equal(want) {
		t.Errorf("open cost basis: expected %s, got %s", want, got)
	}

	// --- Closed row: 18 shares, cost 1,890.00, P&L 712.80, buy date → sell date.
	var closedQty, closedPnl, closedCost, closedOpen, closedClose string
	err = db.QueryRow(`
		SELECT quantity, realized_pnl, cost_basis, open_date, close_date FROM positions
		WHERE account_id = ? AND symbol = 'VWRP.L' AND is_closed = 1
	`, accountID).Scan(&closedQty, &closedPnl, &closedCost, &closedOpen, &closedClose)
	if err != nil {
		t.Fatalf("query closed VWRP.L position: %v", err)
	}
	if got, want := decimal.MustParse(closedQty), decimal.MustNew(18, 0); !got.Equal(want) {
		t.Errorf("closed quantity: expected %s, got %s", want, got)
	}
	if got, want := decimal.MustParse(closedPnl), decimal.MustNew(71280, 2); !got.Equal(want) {
		t.Errorf("closed realized P&L: expected %s, got %s", want, got)
	}
	if got, want := decimal.MustParse(closedCost), decimal.MustNew(-189000, 2); !got.Equal(want) {
		t.Errorf("closed cost basis: expected %s, got %s", want, got)
	}
	if !strings.HasPrefix(closedOpen, "2024-07-01") {
		t.Errorf("closed open_date: expected 2024-07-01…, got %s", closedOpen)
	}
	if !strings.HasPrefix(closedClose, "2025-07-15") {
		t.Errorf("closed close_date: expected 2025-07-15…, got %s", closedClose)
	}

	// --- Cash: exactly the net_cash sum. (−69,930.00) + 2,602.80 = −67,327.20.
	// The proceeds appear once here — not again in the closed row.
	var cashQty string
	err = db.QueryRow(`
		SELECT quantity FROM positions WHERE account_id = ? AND symbol = '$CASH-GBP'
	`, accountID).Scan(&cashQty)
	if err != nil {
		t.Fatalf("query cash position: %v", err)
	}
	if got, want := decimal.MustParse(cashQty), decimal.MustNew(-6732720, 2); !got.Equal(want) {
		t.Errorf("cash balance: expected %s, got %s", want, got)
	}

	// --- Web: the closed positions page renders the partial-sale row.
	skipIfTemplatesUnavailable(t)
	req := httptest.NewRequest(http.MethodGet, "/positions/closed", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /positions/closed: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "VWRP.L") {
		t.Error("closed positions page does not include the VWRP.L partial-sale row")
	}

	// --- Web: the open positions page shows the reduced open position.
	req = httptest.NewRequest(http.MethodGet, "/positions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /positions: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "648") {
		t.Error("open positions page does not show the reduced 648-share VWRP.L position")
	}
}

// TestPositionR1_FullCloseUnchanged verifies the fully-closed case still
// produces exactly one closed row and no open row.
func TestPositionR1_FullCloseUnchanged(t *testing.T) {
	db, router, _, accountID := setupPos(t)

	// Buy 100 @ $150, sell all 100 @ $170.
	createTransaction(t, router, accountID, "2025-01-01", "buy", "AAPL", 100, 15000, -1500000)
	createTransaction(t, router, accountID, "2025-02-01", "sell", "AAPL", -100, 17000, 1700000)

	var openCount, closedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM positions WHERE account_id = ? AND symbol = 'AAPL' AND is_closed = 0`, accountID).Scan(&openCount); err != nil {
		t.Fatalf("count open: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM positions WHERE account_id = ? AND symbol = 'AAPL' AND is_closed = 1`, accountID).Scan(&closedCount); err != nil {
		t.Fatalf("count closed: %v", err)
	}
	if openCount != 0 {
		t.Errorf("expected 0 open AAPL positions, got %d", openCount)
	}
	if closedCount != 1 {
		t.Errorf("expected 1 closed AAPL position, got %d", closedCount)
	}
}

// TestPositionR1_TwoPartialSells verifies two partial sells produce two
// closed rows (one per sell lot) plus the reduced open position.
func TestPositionR1_TwoPartialSells(t *testing.T) {
	db, router, _, accountID := setupPos(t)

	// Buy 100 @ $150; sell 20 @ $160 (May); sell 30 @ $170 (June).
	createTransaction(t, router, accountID, "2025-01-01", "buy", "AAPL", 100, 15000, -1500000)
	createTransaction(t, router, accountID, "2025-05-01", "sell", "AAPL", -20, 16000, 320000)
	createTransaction(t, router, accountID, "2025-06-01", "sell", "AAPL", -30, 17000, 510000)

	var openQty string
	if err := db.QueryRow(`SELECT quantity FROM positions WHERE account_id = ? AND symbol = 'AAPL' AND is_closed = 0`, accountID).Scan(&openQty); err != nil {
		t.Fatalf("query open position: %v", err)
	}
	if got, want := decimal.MustParse(openQty), decimal.MustNew(50, 0); !got.Equal(want) {
		t.Errorf("open quantity: expected %s, got %s", want, got)
	}
	// Open cost: 50/100 × 1,500,000 = 750,000
	var openCost string
	if err := db.QueryRow(`SELECT cost_basis FROM positions WHERE account_id = ? AND symbol = 'AAPL' AND is_closed = 0`, accountID).Scan(&openCost); err != nil {
		t.Fatalf("query open cost: %v", err)
	}
	if got, want := decimal.MustParse(openCost), decimal.MustNew(-75000000, 2); !got.Equal(want) {
		t.Errorf("open cost basis: expected %s, got %s", want, got)
	}

	// Two closed rows, one per sell lot.
	var closedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM positions WHERE account_id = ? AND symbol = 'AAPL' AND is_closed = 1`, accountID).Scan(&closedCount); err != nil {
		t.Fatalf("count closed: %v", err)
	}
	if closedCount != 2 {
		t.Fatalf("expected 2 closed positions, got %d", closedCount)
	}

	rows, err := db.Query(`
		SELECT quantity, realized_pnl, close_date FROM positions
		WHERE account_id = ? AND symbol = 'AAPL' AND is_closed = 1 ORDER BY close_date
	`, accountID)
	if err != nil {
		t.Fatalf("query closed rows: %v", err)
	}
	defer rows.Close()

	type closedRow struct {
		qty, pnl string
		close    string
	}
	var got []closedRow
	for rows.Next() {
		var r closedRow
		if err := rows.Scan(&r.qty, &r.pnl, &r.close); err != nil {
			t.Fatalf("scan closed row: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 closed rows, got %d", len(got))
	}
	// First sell: 20 @ 16,000 → P&L 20×(16,000−15,000) = +200,000
	if got, want := decimal.MustParse(got[0].pnl), decimal.MustNew(2000000, 2); !got.Equal(want) {
		t.Errorf("first closed P&L: expected %s, got %s", want, got)
	}
	if !strings.HasPrefix(got[0].close, "2025-05-01") {
		t.Errorf("first close date: expected 2025-05-01…, got %s", got[0].close)
	}
	// Second sell: 30 @ 17,000 → P&L 30×(17,000−15,000) = +600,000
	if got, want := decimal.MustParse(got[1].pnl), decimal.MustNew(6000000, 2); !got.Equal(want) {
		t.Errorf("second closed P&L: expected %s, got %s", want, got)
	}
	if !strings.HasPrefix(got[1].close, "2025-06-01") {
		t.Errorf("second close date: expected 2025-06-01…, got %s", got[1].close)
	}
}
