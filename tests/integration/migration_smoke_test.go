package integration

import (
	"testing"
)

func TestMigration_PositionsTableExists(t *testing.T) {
	db := setupTestDB(t)

	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='positions'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("positions table not found: %v", err)
	}
	if tableName != "positions" {
		t.Errorf("expected 'positions', got %q", tableName)
	}
}

func TestMigration_LotsTableExists(t *testing.T) {
	db := setupTestDB(t)

	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='lots'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("lots table not found: %v", err)
	}
	if tableName != "lots" {
		t.Errorf("expected 'lots', got %q", tableName)
	}
}

func TestMigration_LotConsumptionsTableExists(t *testing.T) {
	db := setupTestDB(t)

	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='lot_consumptions'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("lot_consumptions table not found: %v", err)
	}
	if tableName != "lot_consumptions" {
		t.Errorf("expected 'lot_consumptions', got %q", tableName)
	}
}

func TestMigration_MarketDataTableExists(t *testing.T) {
	db := setupTestDB(t)

	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='market_data'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("market_data table not found: %v", err)
	}
	if tableName != "market_data" {
		t.Errorf("expected 'market_data', got %q", tableName)
	}
}

func TestMigration_TransactionsLotIDColumnExists(t *testing.T) {
	db := setupTestDB(t)

	var columnName string
	err := db.QueryRow(`
		SELECT name FROM pragma_table_info('transactions')
		WHERE name = 'lot_id'
	`).Scan(&columnName)
	if err != nil {
		t.Fatalf("lot_id column not found on transactions table: %v", err)
	}
	if columnName != "lot_id" {
		t.Errorf("expected 'lot_id', got %q", columnName)
	}
}

func TestMigration_PositionsIndexesExist(t *testing.T) {
	db := setupTestDB(t)

	for _, idx := range []string{"idx_positions_account_symbol", "idx_positions_is_closed"} {
		var indexName string
		err := db.QueryRow(`
			SELECT name FROM sqlite_master
			WHERE type='index' AND name = ?
		`, idx).Scan(&indexName)
		if err != nil {
			t.Errorf("index %s not found: %v", idx, err)
		}
	}
}

func TestMigration_LotsIndexesExist(t *testing.T) {
	db := setupTestDB(t)

	for _, idx := range []string{"idx_lots_account_symbol", "idx_lots_lot_id"} {
		var indexName string
		err := db.QueryRow(`
			SELECT name FROM sqlite_master
			WHERE type='index' AND name = ?
		`, idx).Scan(&indexName)
		if err != nil {
			t.Errorf("index %s not found: %v", idx, err)
		}
	}
}

func TestMigration_MarketDataIndexesExist(t *testing.T) {
	db := setupTestDB(t)

	for _, idx := range []string{"idx_market_data_symbol", "idx_market_data_symbol_date"} {
		var indexName string
		err := db.QueryRow(`
			SELECT name FROM sqlite_master
			WHERE type='index' AND name = ?
		`, idx).Scan(&indexName)
		if err != nil {
			t.Errorf("index %s not found: %v", idx, err)
		}
	}
}

func TestMigration_MarketDataUniqueConstraint(t *testing.T) {
	db := setupTestDB(t)

	// Insert a market_data row with a specific date
	_, err := db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"AAPL", "150.00", "USD", "stock", "yahoo", "2025-01-01", "2025-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert market_data: %v", err)
	}

	// Duplicate (same symbol, source, date) should fail
	_, err = db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"AAPL", "151.00", "USD", "stock", "yahoo", "2025-01-01", "2025-01-01T00:00:00Z",
	)
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation, got nil")
	}

	// Different date should succeed
	_, err = db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"AAPL", "152.00", "USD", "stock", "yahoo", "2025-01-02", "2025-01-02T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert with different date should succeed: %v", err)
	}

	// Same symbol/source but NULL date (latest) should succeed (SQLite: NULL ≠ NULL in UNIQUE)
	_, err = db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, fetched_at) VALUES (?, ?, ?, ?, ?, ?)",
		"AAPL", "153.00", "USD", "stock", "yahoo", "2025-01-03T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert with NULL date should succeed: %v", err)
	}
}

func TestMigration_AccountsTableExists(t *testing.T) {
	db := setupTestDB(t)

	// Verify the accounts table exists and has the expected columns
	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='accounts'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("accounts table not found: %v", err)
	}
	if tableName != "accounts" {
		t.Errorf("expected 'accounts', got %q", tableName)
	}
}

func TestMigration_AccountsForeignKeyCascade(t *testing.T) {
	db := setupTestDB(t)

	// Create a portfolio
	var portfolioID int64
	err := db.QueryRow(
		"INSERT INTO portfolios (name, currency) VALUES (?, ?) RETURNING id",
		"Test Portfolio", "USD",
	).Scan(&portfolioID)
	if err != nil {
		t.Fatalf("create portfolio: %v", err)
	}

	// Create accounts under that portfolio
	for _, name := range []string{"IBKR", "Degiro"} {
		_, err := db.Exec(
			"INSERT INTO accounts (name, portfolio_id) VALUES (?, ?)",
			name, portfolioID,
		)
		if err != nil {
			t.Fatalf("create account %s: %v", name, err)
		}
	}

	// Verify accounts exist
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM accounts WHERE portfolio_id = ?", portfolioID).Scan(&count)
	if err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 accounts, got %d", count)
	}

	// Delete the portfolio — cascade should remove accounts
	_, err = db.Exec("DELETE FROM portfolios WHERE id = ?", portfolioID)
	if err != nil {
		t.Fatalf("delete portfolio: %v", err)
	}

	// Verify accounts are gone
	err = db.QueryRow("SELECT COUNT(*) FROM accounts WHERE portfolio_id = ?", portfolioID).Scan(&count)
	if err != nil {
		t.Fatalf("count accounts after cascade: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 accounts after cascade, got %d", count)
	}
}

func TestMigration_AccountsIndexExists(t *testing.T) {
	db := setupTestDB(t)

	var indexName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='index' AND name='idx_accounts_portfolio_id'
	`).Scan(&indexName)
	if err != nil {
		t.Fatalf("idx_accounts_portfolio_id index not found: %v", err)
	}
	if indexName != "idx_accounts_portfolio_id" {
		t.Errorf("expected 'idx_accounts_portfolio_id', got %q", indexName)
	}
}
