package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

// setupTransactionDB creates an in-memory SQLite database with the transactions
// schema (and accounts table for FK) for repository tests.
func setupTransactionDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	_, err = db.Exec(`
		PRAGMA foreign_keys = ON;

		CREATE TABLE portfolios (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			name      TEXT    NOT NULL,
			currency  TEXT    NOT NULL,
			created_at TEXT   NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT   NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE accounts (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			name         TEXT    NOT NULL,
			portfolio_id INTEGER NOT NULL,
			created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at   TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (portfolio_id) REFERENCES portfolios(id) ON DELETE CASCADE
		);

		CREATE TABLE transactions (
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
			created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
		);

		CREATE INDEX idx_transactions_account_id ON transactions(account_id);
		CREATE INDEX idx_transactions_date ON transactions(date);
		CREATE INDEX idx_transactions_symbol ON transactions(symbol);
		CREATE INDEX idx_transactions_type ON transactions(type);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	// Seed a portfolio and account for FK
	_, err = db.Exec(`INSERT INTO portfolios (name, currency) VALUES ('Test', 'USD')`)
	if err != nil {
		t.Fatalf("seed portfolio: %v", err)
	}
	_, err = db.Exec(`INSERT INTO accounts (name, portfolio_id) VALUES ('Test Account', 1)`)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	return db
}

func newTestTransaction(id int64, accountID int64, date string, txType, symbol, currency string, quantity, price, netCash decimal.Decimal) *transaction.Transaction {
	return &transaction.Transaction{
		ID:        id,
		AccountID: accountID,
		Date:      mustParseTime(date),
		Type:      txType,
		Symbol:    symbol,
		Quantity:  quantity,
		Price:     price,
		Currency:  currency,
		NetCash:   netCash,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// --- CRUD Tests ---

func TestTransactionRepository_CreateAndGet(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	netCash := decimal.MustNew(15000, 2) // 150.00
	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), netCash)

	err := repo.Create(context.Background(), txn)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if txn.ID == 0 {
		t.Fatal("expected non-zero ID after Create")
	}

	got, err := repo.GetByID(context.Background(), txn.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", got.Symbol)
	}
	if !got.Quantity.Equal(decimal.MustNew(10, 0)) {
		t.Errorf("expected Quantity 10, got %q", got.Quantity.String())
	}
	if !got.Price.Equal(decimal.MustNew(15000, 2)) {
		t.Errorf("expected Price 150, got %q", got.Price.String())
	}
	if !got.NetCash.Equal(decimal.MustNew(15000, 2)) {
		t.Errorf("expected NetCash 150, got %q", got.NetCash.String())
	}
}

func TestTransactionRepository_Create_WithExternalFields(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	extSys := "IBKR"
	extRef := "TXN-12345"
	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	txn.ExternalSystem = &extSys
	txn.ExternalReference = &extRef

	err := repo.Create(context.Background(), txn)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(context.Background(), txn.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ExternalSystem == nil || *got.ExternalSystem != "IBKR" {
		t.Errorf("expected ExternalSystem 'IBKR', got %v", got.ExternalSystem)
	}
	if got.ExternalReference == nil || *got.ExternalReference != "TXN-12345" {
		t.Errorf("expected ExternalReference 'TXN-12345', got %v", got.ExternalReference)
	}
}

func TestTransactionRepository_GetByID_NotFound(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	_, err := repo.GetByID(context.Background(), 999)
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTransactionRepository_Update(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn)

	// Update price and quantity
	txn.Price = decimal.MustNew(16000, 2) // 160.00
	txn.Quantity = decimal.MustNew(12, 0)
	txn.UpdatedAt = time.Now()

	err := repo.Update(context.Background(), txn)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(context.Background(), txn.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if !got.Price.Equal(decimal.MustNew(16000, 2)) {
		t.Errorf("expected Price 160, got %q", got.Price.String())
	}
	if !got.Quantity.Equal(decimal.MustNew(12, 0)) {
		t.Errorf("expected Quantity 12, got %q", got.Quantity.String())
	}
}

func TestTransactionRepository_Delete(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn)

	err := repo.Delete(context.Background(), txn.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = repo.GetByID(context.Background(), txn.ID)
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestTransactionRepository_Delete_NotFound(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	err := repo.Delete(context.Background(), 999)
	if !errorsIs(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- Decimal Round-Trip Tests ---

func TestTransactionRepository_DecimalRoundTrip(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	tests := []struct {
		name     string
		quantity decimal.Decimal
		price    decimal.Decimal
	}{
		{"integer qty", decimal.MustNew(100, 0), decimal.MustNew(15000, 2)},
		{"fractional qty", decimal.MustNew(1234, 2), decimal.MustNew(75, 2)},
		{"negative qty (short)", decimal.MustNew(-50, 0), decimal.MustNew(20000, 2)},
		{"large price", decimal.MustNew(1, 0), decimal.MustNew(1785000, 2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "TEST", "USD",
				tt.quantity, tt.price, decimal.Zero)

			err := repo.Create(context.Background(), txn)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, err := repo.GetByID(context.Background(), txn.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}

			if !got.Quantity.Equal(tt.quantity) {
				t.Errorf("quantity mismatch: want %s, got %s", tt.quantity.String(), got.Quantity.String())
			}
			if !got.Price.Equal(tt.price) {
				t.Errorf("price mismatch: want %s, got %s", tt.price.String(), got.Price.String())
			}
		})
	}
}

// --- List Tests ---

func TestTransactionRepository_List_NoFilters(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	for i := 0; i < 5; i++ {
		txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
			decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
		repo.Create(context.Background(), txn)
	}

	items, err := repo.List(context.Background(), transaction.ListFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items, got %d", len(items))
	}
}

func TestTransactionRepository_List_Pagination(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	for i := 0; i < 5; i++ {
		txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
			decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
		repo.Create(context.Background(), txn)
	}

	items, err := repo.List(context.Background(), transaction.ListFilters{}, 2, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items with limit=2, got %d", len(items))
	}

	// LIMIT 0 → empty result (real SQL behavior)
	items, err = repo.List(context.Background(), transaction.ListFilters{}, 0, 0)
	if err != nil {
		t.Fatalf("List limit=0: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items with limit=0, got %d", len(items))
	}
}

func TestTransactionRepository_List_ByAccount(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Create a second account
	_, err := db.Exec(`INSERT INTO accounts (name, portfolio_id) VALUES ('Second', 1)`)
	if err != nil {
		t.Fatalf("seed second account: %v", err)
	}

	// Transactions on account 1
	for i := 0; i < 3; i++ {
		txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
			decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
		repo.Create(context.Background(), txn)
	}
	// Transactions on account 2
	for i := 0; i < 2; i++ {
		txn := newTestTransaction(0, 2, "2025-01-15T00:00:00Z", "buy", "MSFT", "USD",
			decimal.MustNew(5, 0), decimal.MustNew(30000, 2), decimal.Zero)
		repo.Create(context.Background(), txn)
	}

	accountID := int64(1)
	items, err := repo.List(context.Background(), transaction.ListFilters{AccountID: &accountID}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items for account 1, got %d", len(items))
	}
	for _, item := range items {
		if item.AccountID != 1 {
			t.Errorf("expected AccountID 1, got %d", item.AccountID)
		}
	}
}

func TestTransactionRepository_List_BySymbol(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn1)

	txn2 := newTestTransaction(0, 1, "2025-01-16T00:00:00Z", "buy", "MSFT", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(30000, 2), decimal.Zero)
	repo.Create(context.Background(), txn2)

	symbol := "AAPL"
	items, err := repo.List(context.Background(), transaction.ListFilters{Symbol: &symbol}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item for AAPL, got %d", len(items))
	}
	if items[0].Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", items[0].Symbol)
	}
}

func TestTransactionRepository_List_ByType(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn1)

	txn2 := newTestTransaction(0, 1, "2025-01-16T00:00:00Z", "sell", "AAPL", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(16000, 2), decimal.Zero)
	repo.Create(context.Background(), txn2)

	txType := "sell"
	items, err := repo.List(context.Background(), transaction.ListFilters{Type: &txType}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 sell transaction, got %d", len(items))
	}
}

func TestTransactionRepository_List_ByDateRange(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Jan 10
	txn1 := newTestTransaction(0, 1, "2025-01-10T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn1)

	// Jan 20
	txn2 := newTestTransaction(0, 1, "2025-01-20T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn2)

	// Feb 1
	txn3 := newTestTransaction(0, 1, "2025-02-01T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn3)

	from := mustParseTime("2025-01-15T00:00:00Z")
	to := mustParseTime("2025-01-31T00:00:00Z")
	items, err := repo.List(context.Background(), transaction.ListFilters{DateFrom: &from, DateTo: &to}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item in date range, got %d", len(items))
	}
}

func TestTransactionRepository_List_AccountAndSymbol(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn1)

	txn2 := newTestTransaction(0, 1, "2025-01-16T00:00:00Z", "buy", "MSFT", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(30000, 2), decimal.Zero)
	repo.Create(context.Background(), txn2)

	accountID := int64(1)
	symbol := "AAPL"
	items, err := repo.List(context.Background(), transaction.ListFilters{
		AccountID: &accountID, Symbol: &symbol,
	}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestTransactionRepository_List_AllFilters(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn1)

	txn2 := newTestTransaction(0, 1, "2025-01-20T00:00:00Z", "sell", "AAPL", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(16000, 2), decimal.Zero)
	repo.Create(context.Background(), txn2)

	txn3 := newTestTransaction(0, 1, "2025-01-25T00:00:00Z", "buy", "MSFT", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(30000, 2), decimal.Zero)
	repo.Create(context.Background(), txn3)

	accountID := int64(1)
	symbol := "AAPL"
	txType := "buy"
	from := mustParseTime("2025-01-01T00:00:00Z")
	to := mustParseTime("2025-01-31T00:00:00Z")
	items, err := repo.List(context.Background(), transaction.ListFilters{
		AccountID: &accountID, Symbol: &symbol, Type: &txType,
		DateFrom: &from, DateTo: &to,
	}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item with all filters, got %d", len(items))
	}
}

func TestTransactionRepository_List_EmptyResult(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	symbol := "NONEXISTENT"
	items, err := repo.List(context.Background(), transaction.ListFilters{Symbol: &symbol}, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestTransactionRepository_ListWithAccount_Basic(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Rename the seeded account
	_, err := db.Exec("UPDATE accounts SET name = 'Broker A' WHERE id = 1")
	if err != nil {
		t.Fatalf("update account: %v", err)
	}

	// Insert a transaction
	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.MustNew(-150000, 2))
	err = repo.Create(context.Background(), txn)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].AccountName != "Broker A" {
		t.Errorf("expected AccountName 'Broker A', got %q", items[0].AccountName)
	}
	if items[0].Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", items[0].Symbol)
	}
}

func TestTransactionRepository_ListWithAccount_FilterByAccount(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Rename seeded account and add a second one
	_, err := db.Exec("UPDATE accounts SET name = 'Broker A' WHERE id = 1")
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	_, err = db.Exec("INSERT INTO accounts (name, portfolio_id) VALUES ('Broker B', 1)")
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	// Transaction on account 1
	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.MustNew(-150000, 2))
	err = repo.Create(context.Background(), txn1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Transaction on account 2
	txn2 := newTestTransaction(0, 2, "2025-01-16T00:00:00Z", "buy", "MSFT", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(30000, 2), decimal.MustNew(-150000, 2))
	err = repo.Create(context.Background(), txn2)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	accountID := int64(1)
	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{AccountID: &accountID}, 10, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].AccountName != "Broker A" {
		t.Errorf("expected AccountName 'Broker A', got %q", items[0].AccountName)
	}
	if items[0].Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", items[0].Symbol)
	}
}

func TestTransactionRepository_ListWithAccount_MultipleAccounts(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Rename seeded account and add a second one
	_, err := db.Exec("UPDATE accounts SET name = 'Broker A' WHERE id = 1")
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	_, err = db.Exec("INSERT INTO accounts (name, portfolio_id) VALUES ('Broker B', 1)")
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.MustNew(-150000, 2))
	err = repo.Create(context.Background(), txn1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	txn2 := newTestTransaction(0, 2, "2025-01-16T00:00:00Z", "buy", "MSFT", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(30000, 2), decimal.MustNew(-150000, 2))
	err = repo.Create(context.Background(), txn2)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Most recent first
	if items[0].AccountName != "Broker B" {
		t.Errorf("expected first item AccountName 'Broker B', got %q", items[0].AccountName)
	}
	if items[1].AccountName != "Broker A" {
		t.Errorf("expected second item AccountName 'Broker A', got %q", items[1].AccountName)
	}
}

func TestTransactionRepository_ListWithAccount_WithFilters(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Rename seeded account
	_, err := db.Exec("UPDATE accounts SET name = 'Broker A' WHERE id = 1")
	if err != nil {
		t.Fatalf("update account: %v", err)
	}

	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.MustNew(-150000, 2))
	err = repo.Create(context.Background(), txn1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	txn2 := newTestTransaction(0, 1, "2025-01-16T00:00:00Z", "sell", "AAPL", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(16000, 2), decimal.MustNew(80000, 2))
	err = repo.Create(context.Background(), txn2)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Filter by symbol and type
	symbol := "AAPL"
	xtype := "buy"
	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{Symbol: &symbol, Type: &xtype}, 10, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Type != "buy" {
		t.Errorf("expected Type 'buy', got %q", items[0].Type)
	}
	if items[0].AccountName != "Broker A" {
		t.Errorf("expected AccountName 'Broker A', got %q", items[0].AccountName)
	}
}

func TestTransactionRepository_ListWithAccount_Pagination(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Create 3 transactions on the seeded account
	for i := 0; i < 3; i++ {
		txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
			decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.MustNew(-150000, 2))
		err := repo.Create(context.Background(), txn)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Page 1: limit 2, offset 0
	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{}, 2, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items on page 1, got %d", len(items))
	}

	// Page 2: limit 2, offset 2
	items, err = repo.ListWithAccount(context.Background(), transaction.ListFilters{}, 2, 2)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item on page 2, got %d", len(items))
	}
}

func TestTransactionRepository_ListWithAccount_AllFilters(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	_, err := db.Exec("UPDATE accounts SET name = 'Broker A' WHERE id = 1")
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	_, err = db.Exec("INSERT INTO accounts (name, portfolio_id) VALUES ('Broker B', 1)")
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	// Account 1, AAPL, buy, Jan 15
	txn1 := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.MustNew(-150000, 2))
	repo.Create(context.Background(), txn1)

	// Account 1, AAPL, sell, Jan 20
	txn2 := newTestTransaction(0, 1, "2025-01-20T00:00:00Z", "sell", "AAPL", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(16000, 2), decimal.MustNew(80000, 2))
	repo.Create(context.Background(), txn2)

	// Account 2, MSFT, buy, Jan 25
	txn3 := newTestTransaction(0, 2, "2025-01-25T00:00:00Z", "buy", "MSFT", "USD",
		decimal.MustNew(5, 0), decimal.MustNew(30000, 2), decimal.MustNew(-150000, 2))
	repo.Create(context.Background(), txn3)

	accountID := int64(1)
	symbol := "AAPL"
	txType := "buy"
	from := mustParseTime("2025-01-01T00:00:00Z")
	to := mustParseTime("2025-01-31T00:00:00Z")
	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{
		AccountID: &accountID, Symbol: &symbol, Type: &txType,
		DateFrom: &from, DateTo: &to,
	}, 10, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item with all filters, got %d", len(items))
	}
	if items[0].AccountName != "Broker A" {
		t.Errorf("expected AccountName 'Broker A', got %q", items[0].AccountName)
	}
	if items[0].Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", items[0].Symbol)
	}
	if items[0].Type != "buy" {
		t.Errorf("expected Type 'buy', got %q", items[0].Type)
	}
}

func TestTransactionRepository_ListWithAccount_EmptyResult(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	symbol := "NONEXISTENT"
	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{Symbol: &symbol}, 10, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestTransactionRepository_ListWithAccount_ZeroLimit(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn)

	// LIMIT 0 → empty result (real SQL behavior)
	items, err := repo.ListWithAccount(context.Background(), transaction.ListFilters{}, 0, 0)
	if err != nil {
		t.Fatalf("ListWithAccount: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items with limit=0, got %d", len(items))
	}
}

// --- ExternalReferenceExists Tests ---

func TestTransactionRepository_ExternalReferenceExists_Found(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	extSys := "IBKR"
	extRef := "TXN-12345"
	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	txn.ExternalSystem = &extSys
	txn.ExternalReference = &extRef
	repo.Create(context.Background(), txn)

	if !repo.ExternalReferenceExists(context.Background(), "IBKR", "TXN-12345") {
		t.Error("expected ExternalReferenceExists to return true")
	}
}

func TestTransactionRepository_ExternalReferenceExists_NotFound(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	if repo.ExternalReferenceExists(context.Background(), "IBKR", "NONEXISTENT") {
		t.Error("expected ExternalReferenceExists to return false for non-existent reference")
	}
}

func TestTransactionRepository_ExternalReferenceExists_DifferentSystem(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	extSys := "IBKR"
	extRef := "TXN-12345"
	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	txn.ExternalSystem = &extSys
	txn.ExternalReference = &extRef
	repo.Create(context.Background(), txn)

	// Same reference but different system should not match
	if repo.ExternalReferenceExists(context.Background(), "DEGIRO", "TXN-12345") {
		t.Error("expected ExternalReferenceExists to return false for different external system")
	}
}

func TestTransactionRepository_ExternalReferenceExists_EmptyTable(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	if repo.ExternalReferenceExists(context.Background(), "IBKR", "TXN-12345") {
		t.Error("expected ExternalReferenceExists to return false on empty table")
	}
}

func TestTransactionRepository_ExternalReferenceExists_WithoutExternalFields(t *testing.T) {
	db := setupTransactionDB(t)
	repo := NewTransactionRepository(db)

	// Create a transaction without external fields
	txn := newTestTransaction(0, 1, "2025-01-15T00:00:00Z", "buy", "AAPL", "USD",
		decimal.MustNew(10, 0), decimal.MustNew(15000, 2), decimal.Zero)
	repo.Create(context.Background(), txn)

	// Should not match a transaction without external fields
	if repo.ExternalReferenceExists(context.Background(), "IBKR", "TXN-12345") {
		t.Error("expected ExternalReferenceExists to return false when no transaction has those external fields")
	}
}
