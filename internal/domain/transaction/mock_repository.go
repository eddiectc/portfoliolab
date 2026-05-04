package transaction

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/govalues/decimal"
)

// mockRepository is an in-memory implementation of Repository for testing.
// It simulates real repository behavior: filtering, pagination, and auto-increment IDs.
type mockRepository struct {
	mu       sync.RWMutex
	items    map[int64]Transaction
	nextID   int64
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		items:  make(map[int64]Transaction),
		nextID: 1,
	}
}

func (m *mockRepository) Create(_ context.Context, t *Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t.ID = m.nextID
	m.nextID++
	m.items[t.ID] = *t
	return nil
}

func (m *mockRepository) GetByID(_ context.Context, id int64) (*Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (m *mockRepository) List(_ context.Context, filters ListFilters, limit, offset int) ([]Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Collect all items
	all := make([]Transaction, 0, len(m.items))
	for _, t := range m.items {
		all = append(all, t)
	}

	// Apply filters
	result := filterTransactions(all, filters)

	// Sort: date DESC, symbol ASC, type ASC, id ASC
	sort.Slice(result, func(i, j int) bool {
		if result[i].Date != result[j].Date {
			return result[i].Date.After(result[j].Date)
		}
		if result[i].Symbol != result[j].Symbol {
			return result[i].Symbol < result[j].Symbol
		}
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].ID < result[j].ID
	})

	// Apply pagination
	if len(result) <= offset {
		return []Transaction{}, nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

func (m *mockRepository) Update(_ context.Context, t *Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[t.ID]; !ok {
		return ErrNotFound
	}
	m.items[t.ID] = *t
	return nil
}

func (m *mockRepository) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return ErrNotFound
	}
	delete(m.items, id)
	return nil
}

// filterTransactions applies ListFilters to a slice of transactions.
func filterTransactions(items []Transaction, f ListFilters) []Transaction {
	result := make([]Transaction, 0, len(items))
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

// mockAccountChecker is an in-memory account existence checker.
type mockAccountChecker struct {
	ids map[int64]bool
}

func newMockAccountChecker(accountIDs ...int64) *mockAccountChecker {
	m := &mockAccountChecker{ids: make(map[int64]bool)}
	for _, id := range accountIDs {
		m.ids[id] = true
	}
	return m
}

func (m *mockAccountChecker) AccountExists(_ context.Context, id int64) bool {
	return m.ids[id]
}

// mockSymbolChecker is an in-memory symbol existence checker.
type mockSymbolChecker struct {
	mu      sync.RWMutex
	symbols map[string]bool
}

func newMockSymbolChecker(symbols ...string) *mockSymbolChecker {
	m := &mockSymbolChecker{symbols: make(map[string]bool)}
	for _, s := range symbols {
		m.symbols[s] = true
	}
	return m
}

func (m *mockSymbolChecker) SymbolExists(_ context.Context, symbol string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.symbols[symbol]
}

// mockSymbolCreator creates symbols in the mock symbol checker.
type mockSymbolCreator struct {
	checker *mockSymbolChecker
}

func newMockSymbolCreator(checker *mockSymbolChecker) *mockSymbolCreator {
	return &mockSymbolCreator{checker: checker}
}

func (m *mockSymbolCreator) CreateSymbol(_ context.Context, internalSymbol, _ string) error {
	m.checker.mu.Lock()
	defer m.checker.mu.Unlock()
	m.checker.symbols[internalSymbol] = true
	return nil
}

// helper to create a transaction for tests.
func mustParseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// tx creates a Transaction with auto-timestamps for testing.
func tx(accountID int64, date, typ, symbol, currency string, qty, price decimal.Decimal, netCash *decimal.Decimal) *Transaction {
	now := time.Now()
	return &Transaction{
		AccountID: accountID,
		Date:      mustParseDate(date),
		Type:      typ,
		Symbol:    symbol,
		Quantity:  qty,
		Price:     price,
		Currency:  currency,
		NetCash:   netCash,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// dec is shorthand for decimal.MustNew.
func dec(value int64, scale int) decimal.Decimal {
	return decimal.MustNew(value, scale)
}

// decp returns a pointer to a decimal.
func decp(value int64, scale int) *decimal.Decimal {
	val := decimal.MustNew(value, scale)
	return &val
}
