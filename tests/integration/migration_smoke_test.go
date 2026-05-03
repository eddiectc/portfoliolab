package integration

import (
	"testing"
)

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
