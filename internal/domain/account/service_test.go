package account

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// --- Mock PortfolioChecker ---

type mockPortfolioChecker struct {
	ids map[int64]bool
}

func newMockPortfolioChecker(ids ...int64) *mockPortfolioChecker {
	m := &mockPortfolioChecker{ids: make(map[int64]bool)}
	for _, id := range ids {
		m.ids[id] = true
	}
	return m
}

func (m *mockPortfolioChecker) PortfolioExists(_ context.Context, id int64) bool {
	return m.ids[id]
}

// --- Mock Repository ---

type mockRepo struct {
	accounts    map[int64]*Account
	names       map[string]int64 // name -> id
	byPortfolio map[int64][]int64 // portfolio_id -> account ids (ordered)
	nextID      int64
	err         error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		accounts:    make(map[int64]*Account),
		names:       make(map[string]int64),
		byPortfolio: make(map[int64][]int64),
		nextID:      1,
	}
}

func (m *mockRepo) Create(_ context.Context, a *Account) error {
	if m.err != nil {
		return m.err
	}
	m.nextID++
	a.ID = m.nextID
	m.accounts[a.ID] = a
	m.names[a.Name] = a.ID
	m.byPortfolio[a.PortfolioID] = append(m.byPortfolio[a.PortfolioID], a.ID)
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	a, ok := m.accounts[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (m *mockRepo) GetAll(_ context.Context, limit, offset int) ([]Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []Account
	for _, a := range m.accounts {
		cp := *a
		result = append(result, cp)
	}
	if offset > 0 {
		if offset >= len(result) {
			return []Account{}, nil
		}
		result = result[offset:]
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockRepo) GetByPortfolio(_ context.Context, portfolioID int64, limit, offset int) ([]Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	ids, ok := m.byPortfolio[portfolioID]
	if !ok {
		return []Account{}, nil
	}
	var result []Account
	for _, id := range ids {
		a := m.accounts[id]
		cp := *a
		result = append(result, cp)
	}
	if offset > 0 {
		if offset >= len(result) {
			return []Account{}, nil
		}
		result = result[offset:]
	}
	// Simulate SQL LIMIT 0 → empty result (catches service-layer bugs)
	if limit == 0 {
		return []Account{}, nil
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockRepo) GetByName(_ context.Context, name string) (*Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	id, ok := m.names[name]
	if !ok {
		return nil, ErrNotFound
	}
	a := m.accounts[id]
	cp := *a
	return &cp, nil
}

func (m *mockRepo) Update(_ context.Context, a *Account) error {
	if m.err != nil {
		return m.err
	}
	old, ok := m.accounts[a.ID]
	if !ok {
		return ErrNotFound
	}
	// Update name mapping
	if old.Name != a.Name {
		delete(m.names, old.Name)
		m.names[a.Name] = a.ID
	}
	// Update portfolio mapping
	if old.PortfolioID != a.PortfolioID {
		// Remove from old portfolio
		oldIDs := m.byPortfolio[old.PortfolioID]
		for i, id := range oldIDs {
			if id == a.ID {
				m.byPortfolio[old.PortfolioID] = append(oldIDs[:i], oldIDs[i+1:]...)
				break
			}
		}
		m.byPortfolio[a.PortfolioID] = append(m.byPortfolio[a.PortfolioID], a.ID)
	}
	m.accounts[a.ID] = a
	return nil
}

func (m *mockRepo) Delete(_ context.Context, id int64) error {
	if m.err != nil {
		return m.err
	}
	a, ok := m.accounts[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.names, a.Name)
	// Remove from portfolio
	ids := m.byPortfolio[a.PortfolioID]
	for i, aid := range ids {
		if aid == id {
			m.byPortfolio[a.PortfolioID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	delete(m.accounts, id)
	return nil
}

func newTestService(t *testing.T, portfolioIDs ...int64) (*Service, *mockRepo) {
	t.Helper()
	repo := newMockRepo()
	checker := newMockPortfolioChecker(portfolioIDs...)
	return NewService(repo, checker), repo
}

// --- Create Tests ---

func TestService_Create(t *testing.T) {
	svc, _ := newTestService(t, 1)

	a, err := svc.Create(context.Background(), CreateRequest{
		Name:        "IBKR",
		PortfolioID: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "IBKR" {
		t.Errorf("expected name 'IBKR', got %q", a.Name)
	}
	if a.PortfolioID != 1 {
		t.Errorf("expected portfolio_id 1, got %d", a.PortfolioID)
	}
	if a.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if a.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if a.UpdatedAt.IsZero() {
		t.Error("expected non-zero updated_at")
	}
}

func TestService_Create_InvalidName(t *testing.T) {
	svc, _ := newTestService(t, 1)

	tests := []struct {
		name string
		req  CreateRequest
	}{
		{"empty name", CreateRequest{Name: "", PortfolioID: 1}},
		{"too long name", CreateRequest{Name: string(make([]byte, 101)), PortfolioID: 1}},
		{"whitespace only", CreateRequest{Name: "   ", PortfolioID: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), tt.req)
			if !errors.Is(err, ErrInvalidName) {
				t.Errorf("expected ErrInvalidName, got %v", err)
			}
		})
	}
}

func TestService_Create_DuplicateName(t *testing.T) {
	svc, _ := newTestService(t, 1, 2)

	// Create first account
	_, err := svc.Create(context.Background(), CreateRequest{
		Name:        "IBKR",
		PortfolioID: 1,
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	// Try duplicate in different portfolio
	_, err = svc.Create(context.Background(), CreateRequest{
		Name:        "IBKR",
		PortfolioID: 2,
	})
	if !errors.Is(err, ErrNameExists) {
		t.Errorf("expected ErrNameExists (diff portfolio), got %v", err)
	}

	// Try duplicate in same portfolio
	_, err = svc.Create(context.Background(), CreateRequest{
		Name:        "IBKR",
		PortfolioID: 1,
	})
	if !errors.Is(err, ErrNameExists) {
		t.Errorf("expected ErrNameExists (same portfolio), got %v", err)
	}
}

func TestService_Create_NonExistentPortfolio(t *testing.T) {
	svc, _ := newTestService(t, 1)

	_, err := svc.Create(context.Background(), CreateRequest{
		Name:        "New Account",
		PortfolioID: 999,
	})
	if !errors.Is(err, ErrPortfolioNotFound) {
		t.Errorf("expected ErrPortfolioNotFound, got %v", err)
	}
}

func TestService_Create_NameTrimming(t *testing.T) {
	svc, _ := newTestService(t, 1)

	a, err := svc.Create(context.Background(), CreateRequest{
		Name:        "  IBKR  ",
		PortfolioID: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "IBKR" {
		t.Errorf("expected trimmed name 'IBKR', got %q", a.Name)
	}
}

// --- Get Tests ---

func TestService_Get(t *testing.T) {
	svc, repo := newTestService(t, 1)

	repo.accounts[5] = &Account{ID: 5, Name: "IBKR", PortfolioID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["IBKR"] = 5

	a, err := svc.Get(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "IBKR" {
		t.Errorf("expected 'IBKR', got %q", a.Name)
	}
	if a.PortfolioID != 1 {
		t.Errorf("expected portfolio_id 1, got %d", a.PortfolioID)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _ := newTestService(t, 1)

	_, err := svc.Get(context.Background(), 999)
	if err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

// --- List Tests ---

func TestService_List(t *testing.T) {
	svc, repo := newTestService(t, 1, 2)

	for i := 1; i <= 5; i++ {
		id := int64(i)
		repo.accounts[id] = &Account{ID: id, Name: fmt.Sprintf("Acc%d", i), PortfolioID: 1}
		repo.names[fmt.Sprintf("Acc%d", i)] = id
	}

	// List all
	accounts, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 5 {
		t.Errorf("expected 5 accounts, got %d", len(accounts))
	}

	// Paginated
	accounts, err = svc.List(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts with limit 2, got %d", len(accounts))
	}

	// With offset
	accounts, err = svc.List(context.Background(), 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 3 {
		t.Errorf("expected 3 accounts with limit 3 offset 2, got %d", len(accounts))
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, _ := newTestService(t, 1)

	accounts, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}

// --- ListByPortfolio Tests ---

func TestService_ListByPortfolio(t *testing.T) {
	svc, repo := newTestService(t, 1, 2)

	// Portfolio 1: Acc1, Acc3
	// Portfolio 2: Acc2
	for i := 1; i <= 3; i++ {
		id := int64(i)
		pID := int64(1)
		if i == 2 {
			pID = 2
		}
		repo.accounts[id] = &Account{ID: id, Name: fmt.Sprintf("Acc%d", i), PortfolioID: pID}
		repo.names[fmt.Sprintf("Acc%d", i)] = id
		repo.byPortfolio[pID] = append(repo.byPortfolio[pID], id)
	}

	accounts, err := svc.ListByPortfolio(context.Background(), 1, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 2 {
		t.Errorf("expected 2 accounts for portfolio 1, got %d", len(accounts))
	}
	for _, a := range accounts {
		if a.PortfolioID != 1 {
			t.Errorf("expected portfolio_id 1, got %d", a.PortfolioID)
		}
	}
}

func TestService_ListByPortfolio_NonExistent(t *testing.T) {
	svc, _ := newTestService(t, 1)

	accounts, err := svc.ListByPortfolio(context.Background(), 999, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts for non-existent portfolio, got %d", len(accounts))
	}
}

// --- Update Tests ---

func TestService_Update_Name(t *testing.T) {
	svc, repo := newTestService(t, 1)

	repo.accounts[3] = &Account{ID: 3, Name: "Old Broker", PortfolioID: 1}
	repo.names["Old Broker"] = 3

	newName := "New Broker"
	a, err := svc.Update(context.Background(), 3, UpdateRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "New Broker" {
		t.Errorf("expected name 'New Broker', got %q", a.Name)
	}
}

func TestService_Update_Portfolio(t *testing.T) {
	svc, repo := newTestService(t, 1, 2)

	repo.accounts[3] = &Account{ID: 3, Name: "Test", PortfolioID: 1}
	repo.names["Test"] = 3
	repo.byPortfolio[1] = append(repo.byPortfolio[1], 3)

	newPID := int64(2)
	a, err := svc.Update(context.Background(), 3, UpdateRequest{PortfolioID: &newPID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.PortfolioID != 2 {
		t.Errorf("expected portfolio_id 2, got %d", a.PortfolioID)
	}
}

func TestService_Update_Both(t *testing.T) {
	svc, repo := newTestService(t, 1, 2)

	repo.accounts[3] = &Account{ID: 3, Name: "Old", PortfolioID: 1}
	repo.names["Old"] = 3

	newName := "New"
	newPID := int64(2)
	a, err := svc.Update(context.Background(), 3, UpdateRequest{Name: &newName, PortfolioID: &newPID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "New" {
		t.Errorf("expected name 'New', got %q", a.Name)
	}
	if a.PortfolioID != 2 {
		t.Errorf("expected portfolio_id 2, got %d", a.PortfolioID)
	}
}

func TestService_Update_NoChanges(t *testing.T) {
	svc, repo := newTestService(t, 1)

	originalTime := time.Now()
	repo.accounts[3] = &Account{ID: 3, Name: "Stable", PortfolioID: 1, UpdatedAt: originalTime}
	repo.names["Stable"] = 3

	a, err := svc.Update(context.Background(), 3, UpdateRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "Stable" {
		t.Errorf("expected name 'Stable', got %q", a.Name)
	}
	if a.UpdatedAt != originalTime {
		t.Errorf("expected unchanged updated_at, got %v (was %v)", a.UpdatedAt, originalTime)
	}
}

func TestService_Update_DuplicateName(t *testing.T) {
	svc, repo := newTestService(t, 1)

	repo.accounts[1] = &Account{ID: 1, Name: "Alpha", PortfolioID: 1}
	repo.accounts[2] = &Account{ID: 2, Name: "Beta", PortfolioID: 1}
	repo.names["Alpha"] = 1
	repo.names["Beta"] = 2

	_, err := svc.Update(context.Background(), 2, UpdateRequest{Name: strPtr("Alpha")})
	if !errors.Is(err, ErrNameExists) {
		t.Errorf("expected ErrNameExists, got %v", err)
	}
}

func TestService_Update_EmptyName(t *testing.T) {
	svc, repo := newTestService(t, 1)

	repo.accounts[3] = &Account{ID: 3, Name: "Test", PortfolioID: 1}
	repo.names["Test"] = 3

	emptyName := ""
	_, err := svc.Update(context.Background(), 3, UpdateRequest{Name: &emptyName})
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

func TestService_Update_NonExistentPortfolio(t *testing.T) {
	svc, repo := newTestService(t, 1)

	repo.accounts[3] = &Account{ID: 3, Name: "Test", PortfolioID: 1}
	repo.names["Test"] = 3

	newPID := int64(999)
	_, err := svc.Update(context.Background(), 3, UpdateRequest{PortfolioID: &newPID})
	if !errors.Is(err, ErrPortfolioNotFound) {
		t.Errorf("expected ErrPortfolioNotFound, got %v", err)
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc, _ := newTestService(t, 1)

	newName := "Ghost"
	_, err := svc.Update(context.Background(), 999, UpdateRequest{Name: &newName})
	if err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

func TestService_Update_NameTrimming(t *testing.T) {
	svc, repo := newTestService(t, 1)

	repo.accounts[3] = &Account{ID: 3, Name: "Old", PortfolioID: 1}
	repo.names["Old"] = 3

	newName := "  Trimmed  "
	a, err := svc.Update(context.Background(), 3, UpdateRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "Trimmed" {
		t.Errorf("expected trimmed name 'Trimmed', got %q", a.Name)
	}
}

// --- Delete Tests ---

func TestService_Delete(t *testing.T) {
	svc, repo := newTestService(t, 1)

	repo.accounts[7] = &Account{ID: 7, Name: "Temporary", PortfolioID: 1}
	repo.names["Temporary"] = 7

	err := svc.Delete(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.Get(context.Background(), 7)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t, 1)

	err := svc.Delete(context.Background(), 999)
	if err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

func strPtr(s string) *string {
	return &s
}
