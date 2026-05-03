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

		CREATE TABLE IF NOT EXISTS goose_db_version (
			id INTEGER PRIMARY KEY,
			version_id INTEGER NOT NULL,
			is_applied INTEGER NOT NULL DEFAULT 1,
			tstamp TIMESTAMP DEFAULT (datetime('now'))
		);
		INSERT OR REPLACE INTO goose_db_version (version_id, is_applied) VALUES (2, 1);
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
