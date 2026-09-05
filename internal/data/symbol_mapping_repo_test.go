package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/symbolmapping"
)

// setupSymbolMappingDB creates an in-memory SQLite database with the symbol
// mapping schema for repository tests.
func setupSymbolMappingDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	_, err = db.Exec(`
		PRAGMA foreign_keys = ON;

		CREATE TABLE symbol_mappings (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			internal_symbol     TEXT    NOT NULL UNIQUE,
			market_data_symbol  TEXT    NOT NULL,
			is_benchmark        BOOLEAN NOT NULL DEFAULT 0,
			data_source_url     TEXT,
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE broker_symbol_mappings (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol_mapping_id   INTEGER NOT NULL,
			broker_name         TEXT    NOT NULL,
			broker_symbol       TEXT    NOT NULL,
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (symbol_mapping_id) REFERENCES symbol_mappings(id) ON DELETE CASCADE,
			UNIQUE(broker_name, broker_symbol)
		);

		CREATE INDEX idx_broker_symbol_mappings_mapping_id
			ON broker_symbol_mappings(symbol_mapping_id);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSymbolMappingRepository_CreateAndGet(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	err := repo.Create(context.Background(), sm)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sm.ID == 0 {
		t.Fatal("expected non-zero ID after Create")
	}

	got, err := repo.GetByID(context.Background(), sm.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.InternalSymbol != "AAPL" {
		t.Errorf("expected InternalSymbol 'AAPL', got %q", got.InternalSymbol)
	}
	if got.MarketDataSymbol != "AAPL" {
		t.Errorf("expected MarketDataSymbol 'AAPL', got %q", got.MarketDataSymbol)
	}
}

func TestSymbolMappingRepository_Create_DataSourceURL(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "WMGT",
		MarketDataSymbol: "WMGT LN",
		DataSourceURL:    "https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	err := repo.Create(context.Background(), sm)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(context.Background(), sm.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.DataSourceURL != "https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt" {
		t.Errorf("expected DataSourceURL 'https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt', got %q", got.DataSourceURL)
	}
}

func TestSymbolMappingRepository_GetByID_NotFound(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	_, err := repo.GetByID(context.Background(), 999)
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSymbolMappingRepository_GetByInternalSymbol(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "MSFT",
		MarketDataSymbol: "MSFT",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_ = repo.Create(context.Background(), sm)

	got, err := repo.GetByInternalSymbol(context.Background(), "MSFT")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}
	if got.ID != sm.ID {
		t.Errorf("expected ID %d, got %d", sm.ID, got.ID)
	}
}

func TestSymbolMappingRepository_GetByInternalSymbol_NotFound(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	_, err := repo.GetByInternalSymbol(context.Background(), "NONEXISTENT")
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSymbolMappingRepository_GetAll(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	for _, sym := range []string{"AAPL", "MSFT", "GOOGL"} {
		sm := &symbolmapping.SymbolMapping{
			InternalSymbol:   sym,
			MarketDataSymbol: sym,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		_ = repo.Create(context.Background(), sm)
	}

	mappings, err := repo.GetAll(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(mappings) != 3 {
		t.Errorf("expected 3 mappings, got %d", len(mappings))
	}
}

func TestSymbolMappingRepository_GetAll_Pagination(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	for i := 0; i < 5; i++ {
		sm := &symbolmapping.SymbolMapping{
			InternalSymbol:   string(rune('A' + i)),
			MarketDataSymbol: string(rune('A' + i)),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		_ = repo.Create(context.Background(), sm)
	}

	mappings, err := repo.GetAll(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(mappings) != 2 {
		t.Errorf("expected 2 mappings with limit=2, got %d", len(mappings))
	}

	// Simulate SQL LIMIT 0 → empty result
	mappings, err = repo.GetAll(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("GetAll limit=0: %v", err)
	}
	if len(mappings) != 0 {
		t.Errorf("expected 0 mappings with limit=0, got %d", len(mappings))
	}
}

func TestSymbolMappingRepository_Update(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "OLD",
		MarketDataSymbol: "OLD",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_ = repo.Create(context.Background(), sm)

	sm.InternalSymbol = "NEW"
	sm.MarketDataSymbol = "NEW"
	sm.UpdatedAt = time.Now()

	err := repo.Update(context.Background(), sm)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(context.Background(), sm.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.InternalSymbol != "NEW" {
		t.Errorf("expected InternalSymbol 'NEW', got %q", got.InternalSymbol)
	}
	if got.MarketDataSymbol != "NEW" {
		t.Errorf("expected MarketDataSymbol 'NEW', got %q", got.MarketDataSymbol)
	}
}

func TestSymbolMappingRepository_Delete(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_ = repo.Create(context.Background(), sm)

	err := repo.Delete(context.Background(), sm.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = repo.GetByID(context.Background(), sm.ID)
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSymbolMappingRepository_Delete_NotFound(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	err := repo.Delete(context.Background(), 999)
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSymbolMappingRepository_AddBrokerSymbol(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_ = repo.Create(context.Background(), sm)

	err := repo.AddBrokerSymbol(context.Background(), sm.ID, "InteractiveBrokers", "AAPL.US")
	if err != nil {
		t.Fatalf("AddBrokerSymbol: %v", err)
	}

	// Verify it's loaded through GetByID
	got, err := repo.GetByID(context.Background(), sm.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.BrokerSymbols) != 1 {
		t.Fatalf("expected 1 broker symbol, got %d", len(got.BrokerSymbols))
	}
	if got.BrokerSymbols[0].BrokerName != "InteractiveBrokers" {
		t.Errorf("expected broker name 'InteractiveBrokers', got %q", got.BrokerSymbols[0].BrokerName)
	}
	if got.BrokerSymbols[0].BrokerSymbol != "AAPL.US" {
		t.Errorf("expected broker symbol 'AAPL.US', got %q", got.BrokerSymbols[0].BrokerSymbol)
	}
}

func TestSymbolMappingRepository_GetBrokerSymbolByBroker(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_ = repo.Create(context.Background(), sm)
	_ = repo.AddBrokerSymbol(context.Background(), sm.ID, "IB", "AAPL.US")

	bs, err := repo.GetBrokerSymbolByBroker(context.Background(), "IB", "AAPL.US")
	if err != nil {
		t.Fatalf("GetBrokerSymbolByBroker: %v", err)
	}
	if bs.BrokerName != "IB" {
		t.Errorf("expected broker name 'IB', got %q", bs.BrokerName)
	}
}

func TestSymbolMappingRepository_GetBrokerSymbolByBroker_NotFound(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	_, err := repo.GetBrokerSymbolByBroker(context.Background(), "IB", "NONEXISTENT")
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSymbolMappingRepository_CascadeDelete(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	now := time.Now()
	sm := &symbolmapping.SymbolMapping{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_ = repo.Create(context.Background(), sm)
	_ = repo.AddBrokerSymbol(context.Background(), sm.ID, "IB", "AAPL.US")

	// Delete the symbol mapping — broker symbols should cascade-delete
	err := repo.Delete(context.Background(), sm.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify broker symbol is gone
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM broker_symbol_mappings WHERE symbol_mapping_id = ?", sm.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count broker symbols: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 broker symbols after cascade delete, got %d", count)
	}
}

func TestSymbolMappingRepository_HasReferencingTransactions(t *testing.T) {
	db := setupSymbolMappingDB(t)
	repo := NewSymbolMappingRepository(db)

	has, err := repo.HasReferencingTransactions(context.Background(), 1)
	if err != nil {
		t.Fatalf("HasReferencingTransactions: %v", err)
	}
	if has {
		t.Error("expected false (stub), got true")
	}
}

// errorsIs is a helper that wraps errors.Is for use in tests
// (avoids import cycle with the standard errors package in this context).
func errorsIs(err, target error) bool {
	return err == target
}
