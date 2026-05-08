package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/arch-portfolio-lab/portfoliolab/internal/api"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// Use anonymous in-memory DB for test isolation (no shared cache)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Run migrations manually (goose not needed for in-memory)
	_, err = db.Exec(`
		PRAGMA foreign_keys = ON;

		CREATE TABLE IF NOT EXISTS portfolios (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			currency TEXT NOT NULL DEFAULT 'USD',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			portfolio_id INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (portfolio_id) REFERENCES portfolios(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_accounts_portfolio_id ON accounts(portfolio_id);

		CREATE TABLE IF NOT EXISTS symbol_mappings (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			internal_symbol     TEXT    NOT NULL UNIQUE,
			market_data_symbol  TEXT    NOT NULL,
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS broker_symbol_mappings (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol_mapping_id   INTEGER NOT NULL,
			broker_name         TEXT    NOT NULL,
			broker_symbol       TEXT    NOT NULL,
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (symbol_mapping_id) REFERENCES symbol_mappings(id) ON DELETE CASCADE,
			UNIQUE(broker_name, broker_symbol)
		);

		CREATE INDEX IF NOT EXISTS idx_broker_symbol_mappings_mapping_id
			ON broker_symbol_mappings(symbol_mapping_id);

		CREATE TABLE IF NOT EXISTS transactions (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id          INTEGER NOT NULL,
			date                TEXT    NOT NULL,
			type                TEXT    NOT NULL,
			symbol              TEXT    NOT NULL,
			quantity            TEXT    NOT NULL,
			price               TEXT    NOT NULL,
			currency            TEXT    NOT NULL,
			net_cash            TEXT,
			external_system     TEXT,
			external_reference  TEXT,
			lot_id              TEXT,
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_transactions_lot_id ON transactions(lot_id);

		CREATE INDEX IF NOT EXISTS idx_transactions_account_id ON transactions(account_id);
		CREATE INDEX IF NOT EXISTS idx_transactions_date ON transactions(date DESC);
		CREATE INDEX IF NOT EXISTS idx_transactions_symbol ON transactions(symbol);
		CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(type);
		CREATE INDEX IF NOT EXISTS idx_transactions_lot_id ON transactions(lot_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_external_ref
			ON transactions(external_system, external_reference)
			WHERE external_system IS NOT NULL AND external_reference IS NOT NULL;

		CREATE TABLE IF NOT EXISTS positions (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id        INTEGER NOT NULL,
			symbol            TEXT    NOT NULL,
			currency          TEXT    NOT NULL,
			quantity          TEXT    NOT NULL DEFAULT '0',
			cost_basis        TEXT    NOT NULL DEFAULT '0',
			avg_open_price    TEXT,
			avg_close_price   TEXT,
			realized_pnl      TEXT    NOT NULL DEFAULT '0',
			realized_pnl_base TEXT,
			open_date         TEXT    NOT NULL,
			close_date        TEXT,
			is_closed         INTEGER NOT NULL DEFAULT 0,
			created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at        TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_positions_account_symbol ON positions(account_id, symbol);
		CREATE INDEX IF NOT EXISTS idx_positions_is_closed ON positions(is_closed);

		CREATE TABLE IF NOT EXISTS lots (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			lot_id          TEXT    NOT NULL UNIQUE,
			account_id      INTEGER NOT NULL,
			symbol          TEXT    NOT NULL,
			lot_type        TEXT    NOT NULL,
			quantity        TEXT    NOT NULL,
			cost_basis      TEXT    NOT NULL DEFAULT '0',
			sell_price      TEXT,
			realized_pnl    TEXT    NOT NULL DEFAULT '0',
			open_date       TEXT    NOT NULL,
			close_date      TEXT,
			created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_lots_account_symbol ON lots(account_id, symbol);
		CREATE INDEX IF NOT EXISTS idx_lots_lot_id ON lots(lot_id);

		CREATE TABLE IF NOT EXISTS lot_consumptions (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			sell_lot_id         TEXT    NOT NULL,
			buy_lot_id          TEXT    NOT NULL,
			quantity_consumed   TEXT    NOT NULL,
			cost_basis_consumed TEXT    NOT NULL DEFAULT '0',
			realized_pnl        TEXT    NOT NULL DEFAULT '0',
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (sell_lot_id) REFERENCES lots(lot_id) ON DELETE CASCADE,
			FOREIGN KEY (buy_lot_id) REFERENCES lots(lot_id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_lot_consumptions_sell_lot ON lot_consumptions(sell_lot_id);
		CREATE INDEX IF NOT EXISTS idx_lot_consumptions_buy_lot ON lot_consumptions(buy_lot_id);

		CREATE TABLE IF NOT EXISTS market_data (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol      TEXT    NOT NULL,
			price       TEXT    NOT NULL,
			currency    TEXT    NOT NULL,
			data_type   TEXT    NOT NULL DEFAULT 'stock',
			source      TEXT    NOT NULL DEFAULT 'yahoo',
			date        TEXT,
			fetched_at  TEXT    NOT NULL DEFAULT (datetime('now')),
			created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
			UNIQUE(symbol, source, date)
		);

		CREATE INDEX IF NOT EXISTS idx_market_data_symbol ON market_data(symbol);
		CREATE INDEX IF NOT EXISTS idx_market_data_symbol_date ON market_data(symbol, date);

		CREATE TABLE IF NOT EXISTS goose_db_version (
			id INTEGER PRIMARY KEY,
			version_id INTEGER NOT NULL,
			is_applied INTEGER NOT NULL DEFAULT 1,
			tstamp TIMESTAMP DEFAULT (datetime('now'))
		);
		INSERT OR REPLACE INTO goose_db_version (version_id, is_applied) VALUES (10, 1);
	`)
	if err != nil {
		t.Fatalf("run test migrations: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

// testLogger returns a no-op logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestIntegration_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	router := api.Router(db, testLogger())

	body := `{"name": "Integration Test", "currency": "GBP"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	if p.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if p.Name != "Integration Test" {
		t.Errorf("expected 'Integration Test', got %q", p.Name)
	}

	// Get the portfolio by ID
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/portfolios/%d", p.ID), nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, getReq)

	if w2.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", w2.Code)
	}

	var got portfolio.Portfolio
	json.NewDecoder(w2.Body).Decode(&got)
	if got.Name != "Integration Test" {
		t.Errorf("expected 'Integration Test', got %q", got.Name)
	}
	if got.Currency != "GBP" {
		t.Errorf("expected 'GBP', got %q", got.Currency)
	}
}

func TestIntegration_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	router := api.Router(db, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var portfolios []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&portfolios)
	if len(portfolios) != 0 {
		t.Errorf("expected 0 portfolios, got %d", len(portfolios))
	}
}

func TestIntegration_CreateListDelete(t *testing.T) {
	db := setupTestDB(t)
	router := api.Router(db, testLogger())

	// Create two portfolios
	for _, name := range []string{"Portfolio A", "Portfolio B"} {
		body := json.RawMessage(`{"name": "` + name + `", "currency": "USD"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", name, w.Code)
		}
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var portfolios []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&portfolios)
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(portfolios))
	}

	// Delete first
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/portfolios/1", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, deleteReq)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w2.Code)
	}

	// List again — should have 1
	req = httptest.NewRequest(http.MethodGet, "/api/portfolios", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var remaining []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&remaining)
	if len(remaining) != 1 {
		t.Errorf("expected 1 portfolio after delete, got %d", len(remaining))
	}
}

func TestIntegration_Update(t *testing.T) {
	db := setupTestDB(t)
	router := api.Router(db, testLogger())

	// Create
	body := json.RawMessage(`{"name": "Original", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// Update
	updateBody := json.RawMessage(`{"name": "Updated", "currency": "EUR"}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/portfolios/1", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)
	if p.Name != "Updated" {
		t.Errorf("expected 'Updated', got %q", p.Name)
	}
	if p.Currency != "EUR" {
		t.Errorf("expected 'EUR', got %q", p.Currency)
	}
}

func TestIntegration_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	router := api.Router(db, testLogger())

	// Create first
	body := json.RawMessage(`{"name": "Unique", "currency": "USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create first: expected 201, got %d", w.Code)
	}

	// Try duplicate
	req = httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate name, got %d", w.Code)
	}
}

func TestIntegration_Pagination(t *testing.T) {
	db := setupTestDB(t)
	router := api.Router(db, testLogger())

	// Create 5 portfolios
	for i := 0; i < 5; i++ {
		body := json.RawMessage(`{"name": "P` + string(rune('0'+i)) + `", "currency": "USD"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create portfolio %d: expected 201, got %d", i, w.Code)
		}
	}

	// Paginated list: limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios?limit=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var portfolios []portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&portfolios)
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios with limit=2, got %d", len(portfolios))
	}
}
