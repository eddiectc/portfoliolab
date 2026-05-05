package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/account"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/symbolmapping"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
)

// mockTxRepoForWeb is a minimal in-memory transaction repository for web handler tests.
type mockTxRepoForWeb struct {
	items    map[int64]transaction.Transaction
	accNames map[int64]string
	nextID   int64
}

func newMockTxRepoForWeb() *mockTxRepoForWeb {
	return &mockTxRepoForWeb{
		items:    make(map[int64]transaction.Transaction),
		accNames: make(map[int64]string),
		nextID:   1,
	}
}

// setAccountName sets the account name for a given ID.
func (m *mockTxRepoForWeb) setAccountName(id int64, name string) {
	m.accNames[id] = name
}

func (m *mockTxRepoForWeb) Create(_ context.Context, t *transaction.Transaction) error {
	t.ID = m.nextID
	m.nextID++
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	m.items[t.ID] = *t
	return nil
}

func (m *mockTxRepoForWeb) GetByID(_ context.Context, id int64) (*transaction.Transaction, error) {
	t, ok := m.items[id]
	if !ok {
		return nil, transaction.ErrNotFound
	}
	return &t, nil
}

func (m *mockTxRepoForWeb) List(_ context.Context, filters transaction.ListFilters, limit, offset int) ([]transaction.Transaction, error) {
	all := make([]transaction.Transaction, 0, len(m.items))
	for _, t := range m.items {
		all = append(all, t)
	}
	result := filterTxForWeb(all, filters)
	if len(result) <= offset {
		return []transaction.Transaction{}, nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockTxRepoForWeb) ListWithAccount(_ context.Context, filters transaction.ListFilters, limit, offset int) ([]transaction.TransactionWithAccount, error) {
	all := make([]transaction.Transaction, 0, len(m.items))
	for _, t := range m.items {
		all = append(all, t)
	}
	result := filterTxForWeb(all, filters)

	// Sort: date DESC, id ASC for deterministic ordering
	sort.Slice(result, func(i, j int) bool {
		if result[i].Date != result[j].Date {
			return result[i].Date.After(result[j].Date)
		}
		return result[i].ID < result[j].ID
	})

	if len(result) <= offset {
		return []transaction.TransactionWithAccount{}, nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	out := make([]transaction.TransactionWithAccount, len(result))
	for i, t := range result {
		out[i] = transaction.TransactionWithAccount{
			Transaction: t,
			AccountName: m.accNames[t.AccountID],
		}
	}
	return out, nil
}

func (m *mockTxRepoForWeb) Update(_ context.Context, t *transaction.Transaction) error {
	if _, ok := m.items[t.ID]; !ok {
		return transaction.ErrNotFound
	}
	m.items[t.ID] = *t
	return nil
}

func (m *mockTxRepoForWeb) Delete(_ context.Context, id int64) error {
	if _, ok := m.items[id]; !ok {
		return transaction.ErrNotFound
	}
	delete(m.items, id)
	return nil
}

func filterTxForWeb(items []transaction.Transaction, f transaction.ListFilters) []transaction.Transaction {
	result := make([]transaction.Transaction, 0, len(items))
	for _, t := range items {
		if f.AccountID != nil && t.AccountID != *f.AccountID {
			continue
		}
		if f.Symbol != nil && t.Symbol != *f.Symbol {
			continue
		}
		if f.Type != nil && t.Type != *f.Type {
			continue
		}
		if f.DateFrom != nil && t.Date.Before(*f.DateFrom) {
			continue
		}
		if f.DateTo != nil && t.Date.After(*f.DateTo) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// mockAccountRepoForTx is a minimal in-memory account repository for transaction web handler tests.
type mockAccountRepoForTx struct {
	accounts map[int64]*account.Account
	nextID   int64
}

func newMockAccountRepoForTx() *mockAccountRepoForTx {
	return &mockAccountRepoForTx{
		accounts: make(map[int64]*account.Account),
		nextID:   1,
	}
}

func (m *mockAccountRepoForTx) Create(_ context.Context, a *account.Account) error {
	a.ID = m.nextID
	m.nextID++
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = now
	}
	m.accounts[a.ID] = a
	return nil
}

func (m *mockAccountRepoForTx) GetByID(_ context.Context, id int64) (*account.Account, error) {
	a, ok := m.accounts[id]
	if !ok {
		return nil, account.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (m *mockAccountRepoForTx) GetAll(_ context.Context, _, _ int) ([]account.Account, error) {
	result := make([]account.Account, 0, len(m.accounts))
	for _, a := range m.accounts {
		cp := *a
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockAccountRepoForTx) GetByPortfolio(_ context.Context, portfolioID int64, _, _ int) ([]account.Account, error) {
	result := make([]account.Account, 0)
	for _, a := range m.accounts {
		if a.PortfolioID == portfolioID {
			cp := *a
			result = append(result, cp)
		}
	}
	return result, nil
}

func (m *mockAccountRepoForTx) GetByName(_ context.Context, name string) (*account.Account, error) {
	for _, a := range m.accounts {
		if a.Name == name {
			cp := *a
			return &cp, nil
		}
	}
	return nil, account.ErrNotFound
}

func (m *mockAccountRepoForTx) Update(_ context.Context, a *account.Account) error {
	m.accounts[a.ID] = a
	return nil
}

func (m *mockAccountRepoForTx) Delete(_ context.Context, id int64) error {
	if _, ok := m.accounts[id]; !ok {
		return account.ErrNotFound
	}
	delete(m.accounts, id)
	return nil
}

// mockSymbolRepoForTx is a minimal in-memory symbol mapping repository for transaction web handler tests.
type mockSymbolRepoForTx struct {
	mappings map[int64]*symbolmapping.SymbolMapping
	nextID   int64
}

func newMockSymbolRepoForTx() *mockSymbolRepoForTx {
	return &mockSymbolRepoForTx{
		mappings: make(map[int64]*symbolmapping.SymbolMapping),
		nextID:   1,
	}
}

func (m *mockSymbolRepoForTx) Create(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	sm.ID = m.nextID
	m.nextID++
	now := time.Now()
	if sm.CreatedAt.IsZero() {
		sm.CreatedAt = now
	}
	if sm.UpdatedAt.IsZero() {
		sm.UpdatedAt = now
	}
	m.mappings[sm.ID] = sm
	return nil
}

func (m *mockSymbolRepoForTx) GetByID(_ context.Context, id int64) (*symbolmapping.SymbolMapping, error) {
	sm, ok := m.mappings[id]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	cp := *sm
	return &cp, nil
}

func (m *mockSymbolRepoForTx) GetAll(_ context.Context, _, _ int) ([]symbolmapping.SymbolMapping, error) {
	result := make([]symbolmapping.SymbolMapping, 0, len(m.mappings))
	for _, sm := range m.mappings {
		cp := *sm
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockSymbolRepoForTx) GetByInternalSymbol(_ context.Context, symbol string) (*symbolmapping.SymbolMapping, error) {
	for _, sm := range m.mappings {
		if sm.InternalSymbol == symbol {
			cp := *sm
			return &cp, nil
		}
	}
	return nil, symbolmapping.ErrNotFound
}

func (m *mockSymbolRepoForTx) GetByMarketDataSymbol(_ context.Context, symbol string) (*symbolmapping.SymbolMapping, error) {
	for _, sm := range m.mappings {
		if sm.MarketDataSymbol == symbol {
			cp := *sm
			return &cp, nil
		}
	}
	return nil, symbolmapping.ErrNotFound
}

func (m *mockSymbolRepoForTx) Update(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	m.mappings[sm.ID] = sm
	return nil
}

func (m *mockSymbolRepoForTx) Delete(_ context.Context, id int64) error {
	if _, ok := m.mappings[id]; !ok {
		return symbolmapping.ErrNotFound
	}
	delete(m.mappings, id)
	return nil
}

func (m *mockSymbolRepoForTx) AddBrokerSymbol(_ context.Context, id int64, brokerName, brokerSymbol string) error {
	sm, ok := m.mappings[id]
	if !ok {
		return symbolmapping.ErrNotFound
	}
	sm.BrokerSymbols = append(sm.BrokerSymbols, symbolmapping.BrokerSymbol{
		BrokerName:   brokerName,
		BrokerSymbol: brokerSymbol,
	})
	return nil
}

func (m *mockSymbolRepoForTx) GetBrokerSymbolByBroker(_ context.Context, _, _ string) (*symbolmapping.BrokerSymbol, error) {
	return nil, symbolmapping.ErrNotFound
}

func (m *mockSymbolRepoForTx) HasReferencingTransactions(_ context.Context, _ int64) (bool, error) {
	return false, nil
}

// mockTxAccountChecker always says accounts 1-3 exist.
type mockTxAccountChecker struct{}

func (m *mockTxAccountChecker) AccountExists(_ context.Context, id int64) bool {
	return id >= 1 && id <= 10
}

// mockTxSymbolChecker always says common symbols exist.
type mockTxSymbolChecker struct{}

func (m *mockTxSymbolChecker) SymbolExists(_ context.Context, symbol string) bool {
	return true
}

// mockTxSymbolCreator no-op symbol creator.
type mockTxSymbolCreator struct{}

func (m *mockTxSymbolCreator) CreateSymbol(_ context.Context, _, _ string) error {
	return nil
}

// setupTransactionWebHandler creates a web handler with real services backed by mock repos.
// Returns the handler, transaction service, account repo, symbol repo, and txRepo so callers can seed data.
func setupTransactionWebHandler(t *testing.T) (*TransactionWebHandler, *transaction.Service, *mockAccountRepoForTx, *mockSymbolRepoForTx, *mockTxRepoForWeb) {
	t.Helper()

	txRepo := newMockTxRepoForWeb()
	accountChecker := &mockTxAccountChecker{}
	symbolChecker := &mockTxSymbolChecker{}
	symbolCreator := &mockTxSymbolCreator{}
	txSvc := transaction.NewService(txRepo, accountChecker, symbolChecker, symbolCreator)

	accountRepo := newMockAccountRepoForTx()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	symbolRepo := newMockSymbolRepoForTx()
	symbolSvc := symbolmapping.NewService(symbolRepo)

	renderer := newTestRenderer(t)
	handler := NewTransactionWebHandler(txSvc, accountSvc, symbolSvc, renderer)

	return handler, txSvc, accountRepo, symbolRepo, txRepo
}

// registerAccountName registers an account name in the txRepo mock so ListWithAccount returns it.
func registerAccountName(txRepo *mockTxRepoForWeb, acc *account.Account) {
	txRepo.setAccountName(acc.ID, acc.Name)
}

// -- HandleListPage tests --

// TestTxHandleListPage_EmptyList verifies GET /transactions renders with empty state.
func TestTxHandleListPage_EmptyList(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Transactions") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "Add Transaction") {
		t.Error("missing 'Add Transaction' link")
	}
	if !strings.Contains(body, "No transactions yet") {
		t.Error("expected empty state message")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestTxHandleListPage_WithData verifies transactions appear in the list.
func TestTxHandleListPage_WithData(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	registerAccountName(txRepo, acc)

	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})

	r := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "AAPL") {
		t.Error("expected symbol in list")
	}
	if !strings.Contains(body, "buy") {
		t.Error("expected type in list")
	}
	if !strings.Contains(body, "IBKR") {
		t.Error("expected account name in list")
	}
}

// TestTxHandleListPage_WithFilters verifies filter params are parsed and applied.
func TestTxHandleListPage_WithFilters(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	registerAccountName(txRepo, acc)

	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})

	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-02-01",
		Type:      "sell",
		Symbol:    "GOOG",
		Quantity:  decimal.MustNew(500, 2),
		Price:     decimal.MustNew(14000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(700000, 2),
	})

	r := httptest.NewRequest(http.MethodGet, "/transactions?type=buy", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "AAPL") {
		t.Error("expected AAPL in filtered list")
	}
	if strings.Contains(body, "GOOG") {
		t.Error("should not contain GOOG when filtered by type=buy")
	}
}

// TestTxHandleListPage_WithPagination verifies pagination links are generated.
func TestTxHandleListPage_WithPagination(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	registerAccountName(txRepo, acc)

	// Create 25 transactions to trigger pagination (limit is 20)
	for i := 0; i < 25; i++ {
		txSvc.Create(nil, transaction.CreateRequest{
			AccountID: 1,
			Date:      "2024-01-15",
			Type:      "buy",
			Symbol:    "AAPL",
			Quantity:  decimal.MustNew(1000, 2),
			Price:     decimal.MustNew(15000, 2),
			Currency:  "USD",
			NetCash:   decimal.MustNew(-1500000, 2),
		})
	}

	r := httptest.NewRequest(http.MethodGet, "/transactions", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Next") {
		t.Error("expected Next pagination link when more data exists")
	}
}

// TestTxHandleListPage_FilterByAccount verifies filtering by account_id.
func TestTxHandleListPage_FilterByAccount(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc1, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	acc2, _ := accountSvc.Create(nil, account.CreateRequest{Name: "Fidelity", PortfolioID: 1})
	registerAccountName(txRepo, acc1)
	registerAccountName(txRepo, acc2)

	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 2,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "GOOG",
		Quantity:  decimal.MustNew(500, 2),
		Price:     decimal.MustNew(14000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-700000, 2),
	})

	r := httptest.NewRequest(http.MethodGet, "/transactions?account_id=1", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "AAPL") {
		t.Error("expected AAPL in filtered list")
	}
	if strings.Contains(body, "GOOG") {
		t.Error("should not contain GOOG when filtered by account_id=1")
	}
}

// TestTxHandleListPage_FilterBySymbol verifies filtering by symbol.
func TestTxHandleListPage_FilterBySymbol(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	registerAccountName(txRepo, acc)

	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "GOOG",
		Quantity:  decimal.MustNew(500, 2),
		Price:     decimal.MustNew(14000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-700000, 2),
	})

	r := httptest.NewRequest(http.MethodGet, "/transactions?symbol=AAPL", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "AAPL") {
		t.Error("expected AAPL in filtered list")
	}
	if strings.Contains(body, "GOOG") {
		t.Error("should not contain GOOG when filtered by symbol=AAPL")
	}
}

// TestTxHandleListPage_FilterByDateRange verifies filtering by date range.
func TestTxHandleListPage_FilterByDateRange(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	registerAccountName(txRepo, acc)

	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-06-01",
		Type:      "buy",
		Symbol:    "GOOG",
		Quantity:  decimal.MustNew(500, 2),
		Price:     decimal.MustNew(14000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-700000, 2),
	})

	r := httptest.NewRequest(http.MethodGet, "/transactions?date_from=2024-03-01&date_to=2024-12-31", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	// Check for transaction rows (not placeholder text which always has "e.g. AAPL")
	if strings.Contains(body, "150.00") {
		t.Error("should not contain AAPL price (150.00) when filtered from Mar-Dec")
	}
	if !strings.Contains(body, "140.00") {
		t.Error("expected GOOG price (140.00) in filtered list")
	}
}

// TestTxHandleListPage_CombinedFilters verifies multiple filters work together.
func TestTxHandleListPage_CombinedFilters(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	registerAccountName(txRepo, acc)

	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-20",
		Type:      "sell",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(500, 2),
		Price:     decimal.MustNew(16000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(800000, 2),
	})
	txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-25",
		Type:      "buy",
		Symbol:    "GOOG",
		Quantity:  decimal.MustNew(500, 2),
		Price:     decimal.MustNew(14000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-700000, 2),
	})

	r := httptest.NewRequest(http.MethodGet, "/transactions?symbol=AAPL&type=buy", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	// Check for specific transaction data (not dropdown options)
	// The sell transaction has price 160.00, GOOG has price 140.00
	if strings.Contains(body, "160.00") {
		t.Error("should not contain sell price (160.00) when filtered by type=buy")
	}
	if strings.Contains(body, "140.00") {
		t.Error("should not contain GOOG price (140.00) when filtered by symbol=AAPL")
	}
}

// TestTxHandleListPage_PaginationPreservesFilters verifies filter params persist across pagination.
func TestTxHandleListPage_PaginationPreservesFilters(t *testing.T) {
	handler, txSvc, accountRepo, _, txRepo := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	acc, _ := accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})
	registerAccountName(txRepo, acc)

	for i := 0; i < 25; i++ {
		txSvc.Create(nil, transaction.CreateRequest{
			AccountID: 1,
			Date:      "2024-01-15",
			Type:      "buy",
			Symbol:    "AAPL",
			Quantity:  decimal.MustNew(1000, 2),
			Price:     decimal.MustNew(15000, 2),
			Currency:  "USD",
			NetCash:   decimal.MustNew(-1500000, 2),
		})
	}

	r := httptest.NewRequest(http.MethodGet, "/transactions?type=buy&page=2", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	// The "Previous" link should include the type=buy filter
	if !strings.Contains(body, "Previous") {
		t.Error("expected Previous link on page 2")
	}
	// Check that filter params are preserved in pagination links
	// (URL-encoded as type%3dbuy in href attributes)
	if !strings.Contains(body, "type%3dbuy") {
		t.Error("expected type=buy filter preserved in pagination links")
	}
}

// -- HandleNewPage tests --

// TestTxHandleNewPage_RendersForm verifies GET /transactions/new renders a complete form.
func TestTxHandleNewPage_RendersForm(t *testing.T) {
	_, txSvc, accountRepo, symbolRepo, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	symbolSvc := symbolmapping.NewService(symbolRepo)
	symbolSvc.Create(nil, symbolmapping.CreateRequest{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
	})

	handler2 := NewTransactionWebHandler(txSvc, accountSvc, symbolSvc, newTestRenderer(t))

	r := httptest.NewRequest(http.MethodGet, "/transactions/new", nil)
	w := httptest.NewRecorder()

	handler2.HandleNewPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "New Transaction") {
		t.Error("missing title")
	}
	if !strings.Contains(body, `id="account_id"`) {
		t.Error("missing account select")
	}
	if !strings.Contains(body, `id="symbol"`) {
		t.Error("missing symbol input")
	}
	if !strings.Contains(body, `id="net_cash"`) {
		t.Error("missing net_cash input")
	}
	if !strings.Contains(body, "Create Transaction") {
		t.Error("missing submit button text")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// -- HandleCreatePage tests --

// TestTxHandleCreatePage_ValidCreate verifies valid form submission redirects.
func TestTxHandleCreatePage_ValidCreate(t *testing.T) {
	handler, _, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=AAPL&quantity=10.00&price=150.00&currency=USD&net_cash=-1500.00")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/transactions/") {
		t.Errorf("expected redirect to transaction detail, got %q", location)
	}
}

// TestTxHandleCreatePage_MissingFields shows validation errors.
func TestTxHandleCreatePage_MissingFields(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	body := strings.NewReader("account_id=1&date=&type=&symbol=&quantity=&price=&currency=&net_cash=")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "New Transaction") {
		t.Error("expected form to re-render with title")
	}
	// Should show an error message
	if !strings.Contains(bodyStr, "alert-error") {
		t.Error("expected error alert on form re-render")
	}
}

// TestTxHandleCreatePage_InvalidType shows validation error for bad type.
func TestTxHandleCreatePage_InvalidType(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	body := strings.NewReader("account_id=1&date=2024-01-15&type=invalid&symbol=AAPL&quantity=10&price=150&currency=USD&net_cash=-1500")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Invalid transaction type") {
		t.Error("expected invalid type error")
	}
}

// TestTxHandleCreatePage_MissingNetCash shows validation error for zero net cash.
func TestTxHandleCreatePage_MissingNetCash(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=AAPL&quantity=10&price=150&currency=USD&net_cash=0")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Net cash is required") {
		t.Error("expected net cash error")
	}
}

// TestTxHandleCreatePage_ZeroQuantity shows validation error for zero quantity.
func TestTxHandleCreatePage_ZeroQuantity(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=AAPL&quantity=0&price=150&currency=USD&net_cash=-1500")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Quantity must be non-zero") {
		t.Error("expected quantity error")
	}
}

// TestTxHandleCreatePage_LowercaseCurrency shows validation error for lowercase currency.
func TestTxHandleCreatePage_LowercaseCurrency(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=AAPL&quantity=10&price=150&currency=usd&net_cash=-1500")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Invalid currency") {
		t.Error("expected currency error")
	}
}

// TestTxHandleCreatePage_ExternalFieldTooLong shows validation error for long external fields.
func TestTxHandleCreatePage_ExternalFieldTooLong(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	longSystem := strings.Repeat("x", 101)
	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=AAPL&quantity=10&price=150&currency=USD&net_cash=-1500&external_system=" + longSystem)
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "at most 100 characters") {
		t.Error("expected external field length error")
	}
}

// TestTxHandleCreatePage_InvalidDate shows validation error for bad date format.
func TestTxHandleCreatePage_InvalidDate(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	body := strings.NewReader("account_id=1&date=not-a-date&type=buy&symbol=AAPL&quantity=10&price=150&currency=USD&net_cash=-1500")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Invalid date format") {
		t.Error("expected date format error")
	}
}

// TestTxHandleCreatePage_NonExistentSymbol shows validation error for unknown symbol.
func TestTxHandleCreatePage_NonExistentSymbol(t *testing.T) {
	_, _, accountRepo, symbolRepo, _ := setupTransactionWebHandler(t)

	// Use a strict symbol checker that only knows specific symbols
	symbolChecker := &mockTxSymbolCheckerStrict{symbols: map[string]bool{"AAPL": true}}
	symbolCreator := &mockTxSymbolCreator{}
	txSvc2 := transaction.NewService(newMockTxRepoForWeb(), &mockTxAccountChecker{}, symbolChecker, symbolCreator)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	symbolSvc := symbolmapping.NewService(symbolRepo)
	handler2 := NewTransactionWebHandler(txSvc2, accountSvc, symbolSvc, newTestRenderer(t))

	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=UNKNOWN&quantity=10&price=150&currency=USD&net_cash=-1500")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler2.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Symbol not found") {
		t.Error("expected symbol not found error")
	}
}

// mockTxSymbolCheckerStrict only allows symbols in its map.
type mockTxSymbolCheckerStrict struct {
	symbols map[string]bool
}

func (m *mockTxSymbolCheckerStrict) SymbolExists(_ context.Context, symbol string) bool {
	return m.symbols[symbol]
}

// TestTxHandleCreatePage_CashSymbolMismatch shows validation error for $CASH currency mismatch.
func TestTxHandleCreatePage_CashSymbolMismatch(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	// $CASH-USD symbol with EUR currency — should fail
	body := strings.NewReader("account_id=1&date=2024-01-15&type=deposit&symbol=$CASH-USD&quantity=1000&price=1&currency=EUR&net_cash=1000")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Invalid currency") {
		t.Error("expected currency mismatch error")
	}
}

// TestTxHandleCreatePage_NegativeQuantity verifies negative quantity is accepted (short selling).
func TestTxHandleCreatePage_NegativeQuantity(t *testing.T) {
	handler, _, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	body := strings.NewReader("account_id=1&date=2024-01-15&type=sell&symbol=AAPL&quantity=-10&price=150&currency=USD&net_cash=1500")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}
}

// TestTxHandleCreatePage_DepositWithCashSymbol verifies deposit with $CASH-{currency} works.
func TestTxHandleCreatePage_DepositWithCashSymbol(t *testing.T) {
	handler, _, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	body := strings.NewReader("account_id=1&date=2024-01-15&type=deposit&symbol=$CASH-USD&quantity=1000&price=1&currency=USD&net_cash=1000")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}
}

// TestTxHandleCreatePage_PreservesValues verifies submitted values are preserved on error.
func TestTxHandleCreatePage_PreservesValues(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=AAPL&quantity=10&price=150&currency=usd&net_cash=-1500")
	r := httptest.NewRequest(http.MethodPost, "/transactions", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, `value="AAPL"`) {
		t.Error("expected symbol to be preserved")
	}
	if !strings.Contains(bodyStr, `value="10"`) {
		t.Error("expected quantity to be preserved")
	}
}

// -- HandleDetailPage tests --

// TestTxHandleDetailPage_RendersDetail verifies GET /transactions/{id} renders properly.
func TestTxHandleDetailPage_RendersDetail(t *testing.T) {
	handler, txSvc, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	tx, err := txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	idStr := strconv.FormatInt(tx.ID, 10)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", idStr)
	r := httptest.NewRequest(http.MethodGet, "/transactions/"+idStr, nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "AAPL") {
		t.Error("expected symbol in detail page")
	}
	if !strings.Contains(body, "IBKR") {
		t.Error("expected account name in detail page")
	}
	if !strings.Contains(body, "Edit") {
		t.Error("expected edit link")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestTxHandleDetailPage_NotFound returns 404 for non-existent transaction.
func TestTxHandleDetailPage_NotFound(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodGet, "/transactions/999", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

// -- HandleEditPage tests --

// TestTxHandleEditPage_RendersForm verifies GET /transactions/{id}/edit renders pre-populated form.
func TestTxHandleEditPage_RendersForm(t *testing.T) {
	handler, txSvc, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	tx, err := txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	idStr := strconv.FormatInt(tx.ID, 10)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", idStr)
	r := httptest.NewRequest(http.MethodGet, "/transactions/"+idStr+"/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleEditPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Edit Transaction") {
		t.Error("missing title")
	}
	if !strings.Contains(body, `value="AAPL"`) {
		t.Error("expected pre-filled symbol")
	}
	if !strings.Contains(body, "Save Changes") {
		t.Error("missing submit button text")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestTxHandleEditPage_NotFound returns 404 for non-existent transaction.
func TestTxHandleEditPage_NotFound(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodGet, "/transactions/999/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleEditPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

// -- HandleEditPost tests --

// TestTxHandleEditPost_ValidUpdate verifies valid edit submission redirects.
func TestTxHandleEditPost_ValidUpdate(t *testing.T) {
	handler, txSvc, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	tx, err := txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	idStr := strconv.FormatInt(tx.ID, 10)
	body := strings.NewReader("account_id=1&date=2024-02-01&type=sell&symbol=AAPL&quantity=10.00&price=160.00&currency=USD&net_cash=-1600.00")
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", idStr)
	r := httptest.NewRequest(http.MethodPost, "/transactions/"+idStr+"/edit", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleEditPost(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/transactions/") {
		t.Errorf("expected redirect to transaction detail, got %q", location)
	}
}

// TestTxHandleEditPost_InvalidData shows validation errors on form re-render.
func TestTxHandleEditPost_InvalidData(t *testing.T) {
	handler, txSvc, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	tx, err := txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	idStr := strconv.FormatInt(tx.ID, 10)
	body := strings.NewReader("account_id=1&date=2024-01-15&type=invalid&symbol=AAPL&quantity=10&price=150&currency=USD&net_cash=-1500")
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", idStr)
	r := httptest.NewRequest(http.MethodPost, "/transactions/"+idStr+"/edit", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleEditPost(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Invalid transaction type") {
		t.Error("expected invalid type error")
	}
}

// TestTxHandleEditPost_UserCurrencyOverride verifies user-entered currency is accepted.
func TestTxHandleEditPost_UserCurrencyOverride(t *testing.T) {
	handler, txSvc, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	tx, err := txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	idStr := strconv.FormatInt(tx.ID, 10)
	// Change currency to EUR
	body := strings.NewReader("account_id=1&date=2024-01-15&type=buy&symbol=AAPL&quantity=10.00&price=150.00&currency=EUR&net_cash=-1500.00")
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", idStr)
	r := httptest.NewRequest(http.MethodPost, "/transactions/"+idStr+"/edit", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleEditPost(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d (redirect), got %d", http.StatusSeeOther, resp.StatusCode)
	}
}

// -- HandleDeletePage tests --

// TestTxHandleDeletePage_Success verifies POST /transactions/{id}/delete redirects.
func TestTxHandleDeletePage_Success(t *testing.T) {
	handler, txSvc, accountRepo, _, _ := setupTransactionWebHandler(t)

	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	tx, err := txSvc.Create(nil, transaction.CreateRequest{
		AccountID: 1,
		Date:      "2024-01-15",
		Type:      "buy",
		Symbol:    "AAPL",
		Quantity:  decimal.MustNew(1000, 2),
		Price:     decimal.MustNew(15000, 2),
		Currency:  "USD",
		NetCash:   decimal.MustNew(-1500000, 2),
	})
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	idStr := strconv.FormatInt(tx.ID, 10)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", idStr)
	r := httptest.NewRequest(http.MethodPost, "/transactions/"+idStr+"/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/transactions" {
		t.Errorf("expected redirect to /transactions, got %q", location)
	}
}

// TestTxHandleDeletePage_NotFound redirects without error for non-existent transaction.
func TestTxHandleDeletePage_NotFound(t *testing.T) {
	handler, _, _, _, _ := setupTransactionWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodPost, "/transactions/999/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d (redirect), got %d", http.StatusSeeOther, resp.StatusCode)
	}
}
