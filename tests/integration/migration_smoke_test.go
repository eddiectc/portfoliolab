package integration

import (
	"database/sql"
	"fmt"
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

func TestMigration_TransactionsDescriptionColumnExists(t *testing.T) {
	db := setupTestDB(t)

	var columnName string
	err := db.QueryRow(`
		SELECT name FROM pragma_table_info('transactions')
		WHERE name = 'description'
	`).Scan(&columnName)
	if err != nil {
		t.Fatalf("description column not found on transactions table: %v", err)
	}
	if columnName != "description" {
		t.Errorf("expected 'description', got %q", columnName)
	}
}

func TestMigration_PositionsFxRateColumnsExist(t *testing.T) {
	db := setupTestDB(t)

	for _, col := range []string{"fx_rate_used", "fx_rate_fallback"} {
		var name string
		err := db.QueryRow(`
			SELECT name FROM pragma_table_info('positions')
			WHERE name = ?
		`, col).Scan(&name)
		if err != nil {
			t.Errorf("column %s not found in positions: %v", col, err)
		}
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

	// Insert "latest" row (date = '')
	_, err = db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, fetched_at) VALUES (?, ?, ?, ?, ?, ?)",
		"AAPL", "153.00", "USD", "stock", "yahoo", "2025-01-03T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert with empty date should succeed: %v", err)
	}

	// Duplicate "latest" row (same symbol, source, empty date) should fail
	_, err = db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, fetched_at) VALUES (?, ?, ?, ?, ?, ?)",
		"AAPL", "154.00", "USD", "stock", "yahoo", "2025-01-04T00:00:00Z",
	)
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation on empty date, got nil")
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

func TestMigration_TargetAllocationsTableExists(t *testing.T) {
	db := setupTestDB(t)

	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='target_allocations'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("target_allocations table not found: %v", err)
	}
	if tableName != "target_allocations" {
		t.Errorf("expected 'target_allocations', got %q", tableName)
	}
}

func TestMigration_TargetAllocationsUniqueConstraint(t *testing.T) {
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

	// Insert first target allocation
	_, err = db.Exec(
		"INSERT INTO target_allocations (portfolio_id, symbol, target_pct) VALUES (?, ?, ?)",
		portfolioID, "AAPL", "50.00",
	)
	if err != nil {
		t.Fatalf("insert first target: %v", err)
	}

	// Duplicate (same portfolio_id + symbol) should fail
	_, err = db.Exec(
		"INSERT INTO target_allocations (portfolio_id, symbol, target_pct) VALUES (?, ?, ?)",
		portfolioID, "AAPL", "60.00",
	)
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation, got nil")
	}

	// Different symbol should succeed
	_, err = db.Exec(
		"INSERT INTO target_allocations (portfolio_id, symbol, target_pct) VALUES (?, ?, ?)",
		portfolioID, "GOOGL", "50.00",
	)
	if err != nil {
		t.Fatalf("insert different symbol should succeed: %v", err)
	}
}

func TestMigration_TargetAllocationsForeignKeyCascade(t *testing.T) {
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

	// Insert target allocations
	_, err = db.Exec(
		"INSERT INTO target_allocations (portfolio_id, symbol, target_pct) VALUES (?, ?, ?)",
		portfolioID, "AAPL", "50.00",
	)
	if err != nil {
		t.Fatalf("insert target: %v", err)
	}

	// Verify target exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM target_allocations WHERE portfolio_id = ?", portfolioID).Scan(&count)
	if err != nil {
		t.Fatalf("count targets: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 target, got %d", count)
	}

	// Delete the portfolio — cascade should remove target allocations
	_, err = db.Exec("DELETE FROM portfolios WHERE id = ?", portfolioID)
	if err != nil {
		t.Fatalf("delete portfolio: %v", err)
	}

	// Verify target is gone
	err = db.QueryRow("SELECT COUNT(*) FROM target_allocations WHERE portfolio_id = ?", portfolioID).Scan(&count)
	if err != nil {
		t.Fatalf("count targets after cascade: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 targets after cascade, got %d", count)
	}
}

func TestMigration_TargetAllocationsIndexExists(t *testing.T) {
	db := setupTestDB(t)

	var indexName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='index' AND name='idx_target_allocations_portfolio_id'
	`).Scan(&indexName)
	if err != nil {
		t.Fatalf("idx_target_allocations_portfolio_id index not found: %v", err)
	}
	if indexName != "idx_target_allocations_portfolio_id" {
		t.Errorf("expected 'idx_target_allocations_portfolio_id', got %q", indexName)
	}
}

func TestMigration_ModelPortfoliosTableExists(t *testing.T) {
	db := setupTestDB(t)

	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='model_portfolios'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("model_portfolios table not found: %v", err)
	}
	if tableName != "model_portfolios" {
		t.Errorf("expected 'model_portfolios', got %q", tableName)
	}
}

func TestMigration_ModelPortfoliosColumnsExist(t *testing.T) {
	db := setupTestDB(t)

	for _, col := range []string{"id", "name", "entries", "created_at", "updated_at"} {
		var name string
		err := db.QueryRow(`
			SELECT name FROM pragma_table_info('model_portfolios')
			WHERE name = ?
		`, col).Scan(&name)
		if err != nil {
			t.Errorf("column %s not found in model_portfolios: %v", col, err)
		}
	}
}

func TestMigration_ModelPortfoliosNameUniqueConstraint(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.Exec(
		"INSERT INTO model_portfolios (name, entries) VALUES (?, ?)",
		"Test Model", "[]",
	)
	if err != nil {
		t.Fatalf("insert first model portfolio: %v", err)
	}

	_, err = db.Exec(
		"INSERT INTO model_portfolios (name, entries) VALUES (?, ?)",
		"Test Model", "[]",
	)
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation on name, got nil")
	}
}

func TestMigration_MarketDataUpdatedAtColumnExists(t *testing.T) {
	db := setupTestDB(t)

	var columnName string
	err := db.QueryRow(`
		SELECT name FROM pragma_table_info('market_data')
		WHERE name='updated_at'
	`).Scan(&columnName)
	if err != nil {
		t.Fatalf("updated_at column not found in market_data: %v", err)
	}
	if columnName != "updated_at" {
		t.Errorf("expected 'updated_at', got %q", columnName)
	}
}

func TestMigration_ExtractorColumnsExist(t *testing.T) {
	db := setupTestDB(t)

	for _, tc := range []struct {
		table string
		col   string
	}{
		{"symbol_mappings", "data_source_url"},
		{"symbol_details", "extractor_as_of_date"},
	} {
		t.Run(fmt.Sprintf("%s.%s", tc.table, tc.col), func(t *testing.T) {
			var columnName string
			err := db.QueryRow(
				"SELECT name FROM pragma_table_info(?) WHERE name=?",
				tc.table, tc.col,
			).Scan(&columnName)
			if err != nil {
				t.Fatalf("column %s not found in %s: %v", tc.col, tc.table, err)
			}
			if columnName != tc.col {
				t.Errorf("expected %q, got %q", tc.col, columnName)
			}
		})
	}
}

func TestMigration_NavHistoryQuery(t *testing.T) {
	db := setupTestDB(t)

	// Insert NAV data points
	for _, date := range []string{"2026-01-01", "2026-01-02", "2026-01-03"} {
		_, err := db.Exec(
			"INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"WMGT LN", "50.00", "GBP", "nav", "wisdomtree", date, "2026-01-03T00:00:00Z",
		)
		if err != nil {
			t.Fatalf("insert nav data: %v", err)
		}
	}

	// Query using the same filter as GetNavHistoryBySymbol
	rows, err := db.Query(`
		SELECT symbol, price, data_type, source, date
		FROM market_data
		WHERE symbol = ?
		  AND data_type = 'nav'
		  AND date != ''
		ORDER BY date ASC
	`, "WMGT LN")
	if err != nil {
		t.Fatalf("query nav history: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var count int
	expected := []string{"2026-01-01", "2026-01-02", "2026-01-03"}
	for rows.Next() {
		var symbol, price, dataType, source, date string
		if err := rows.Scan(&symbol, &price, &dataType, &source, &date); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		if symbol != "WMGT LN" {
			t.Errorf("expected symbol WMGT LN, got %s", symbol)
		}
		if dataType != "nav" {
			t.Errorf("expected data_type nav, got %s", dataType)
		}
		if source != "wisdomtree" {
			t.Errorf("expected source wisdomtree, got %s", source)
		}
		if date != expected[count] {
			t.Errorf("expected date %s, got %s", expected[count], date)
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 nav rows, got %d", count)
	}
}

func TestMigration_NavDataIsolation(t *testing.T) {
	db := setupTestDB(t)

	// Insert stock price data
	_, err := db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"WMGT LN", "52.00", "GBP", "stock", "yahoo", "2026-01-01", "2026-01-03T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert stock data: %v", err)
	}

	// Insert NAV data for same symbol
	_, err = db.Exec(
		"INSERT INTO market_data (symbol, price, currency, data_type, source, date, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"WMGT LN", "50.00", "GBP", "nav", "wisdomtree", "2026-01-01", "2026-01-03T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert nav data: %v", err)
	}

	// Verify GetHistoricalPricesBySymbolAndRange excludes NAV
	rows, err := db.Query(`
		SELECT symbol, price, data_type FROM market_data
		WHERE symbol = ?
		  AND date >= '2026-01-01'
		  AND date <= '2026-01-01'
		  AND date != ''
		  AND data_type IN ('stock', 'fx')
		ORDER BY date ASC
	`, "WMGT LN")
	if err != nil {
		t.Fatalf("query prices: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var count int
	for rows.Next() {
		var symbol, price, dataType string
		if err := rows.Scan(&symbol, &price, &dataType); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		if dataType == "nav" {
			t.Error("NAV data leaked into stock/fx price query")
		}
		if price != "52.00" {
			t.Errorf("expected stock price 52.00, got %s", price)
		}
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 stock row, got %d", count)
	}
}

func TestMigration_StaleSymbolDetailsIncludesDataSourceURL(t *testing.T) {
	db := setupTestDB(t)

	// Create symbol mapping with data_source_url
	_, err := db.Exec(
		"INSERT INTO symbol_mappings (internal_symbol, market_data_symbol, is_benchmark, data_source_url, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"WMGT", "WMGT LN", 0, "https://www.wisdomtree.eu/en-gb/etfs/wmgt", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert symbol mapping: %v", err)
	}

	// Create stale symbol details (fetched_at older than threshold)
	_, err = db.Exec(
		"INSERT INTO symbol_details (internal_symbol, short_name, fetched_at) VALUES (?, ?, ?)",
		"WMGT", "WisdomTree", "2025-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert symbol details: %v", err)
	}

	// Query using same logic as ListStaleSymbolDetails
	rows, err := db.Query(`
		SELECT sm.internal_symbol, sm.market_data_symbol, sm.data_source_url, sd.fetched_at
		FROM symbol_mappings sm
		LEFT JOIN symbol_details sd ON sm.internal_symbol = sd.internal_symbol
		WHERE sd.fetched_at IS NULL OR sd.fetched_at < '2026-01-01T00:00:00Z'
		ORDER BY sd.fetched_at ASC
	`)
	if err != nil {
		t.Fatalf("query stale symbols: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var found bool
	for rows.Next() {
		var internalSymbol, marketDataSymbol, fetchedAt string
		var dataSourceURL sql.NullString
		if err := rows.Scan(&internalSymbol, &marketDataSymbol, &dataSourceURL, &fetchedAt); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		if internalSymbol == "WMGT" {
			found = true
			if !dataSourceURL.Valid || dataSourceURL.String != "https://www.wisdomtree.eu/en-gb/etfs/wmgt" {
				t.Errorf("expected data_source_url to be set, got valid=%v, value=%q", dataSourceURL.Valid, dataSourceURL.String)
			}
		}
	}
	if !found {
		t.Error("WMGT not found in stale symbol details results")
	}
}
