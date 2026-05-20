package portfolio

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// mockRepo is a simple in-memory mock for testing the service.
type mockRepo struct {
	portfolios map[int64]*Portfolio
	names      map[string]int64 // name -> id
	nextID     int64
	err        error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		portfolios: make(map[int64]*Portfolio),
		names:      make(map[string]int64),
		nextID:     1,
	}
}

func (m *mockRepo) Create(_ context.Context, p *Portfolio) error {
	if m.err != nil {
		return m.err
	}
	m.nextID++
	p.ID = m.nextID
	m.portfolios[p.ID] = p
	m.names[p.Name] = p.ID
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id int64) (*Portfolio, error) {
	if m.err != nil {
		return nil, m.err
	}
	p, ok := m.portfolios[id]
	if !ok {
		return nil, ErrNotFound
	}
	// Return a copy
	cp := *p
	return &cp, nil
}

func (m *mockRepo) GetAll(_ context.Context, limit, offset int) ([]Portfolio, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []Portfolio
	for _, p := range m.portfolios {
		cp := *p
		result = append(result, cp)
	}
	if offset > 0 {
		if offset >= len(result) {
			return []Portfolio{}, nil
		}
		result = result[offset:]
	}
	// Simulate SQL LIMIT 0 → empty result (catches service-layer bugs)
	if limit == 0 {
		return []Portfolio{}, nil
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockRepo) ListAll(_ context.Context) ([]Portfolio, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []Portfolio
	for _, p := range m.portfolios {
		cp := *p
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockRepo) Update(_ context.Context, p *Portfolio) error {
	if m.err != nil {
		return m.err
	}
	// Update name mapping
	if existingID, ok := m.names[p.Name]; ok && existingID != p.ID {
		return ErrNameExists
	}
	old := m.portfolios[p.ID]
	if old == nil {
		return ErrNotFound
	}
	delete(m.names, old.Name)
	m.portfolios[p.ID] = p
	m.names[p.Name] = p.ID
	return nil
}

func (m *mockRepo) Delete(_ context.Context, id int64) error {
	if m.err != nil {
		return m.err
	}
	p, ok := m.portfolios[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.names, p.Name)
	delete(m.portfolios, id)
	return nil
}

func (m *mockRepo) GetByName(_ context.Context, name string) (*Portfolio, error) {
	if m.err != nil {
		return nil, m.err
	}
	id, ok := m.names[name]
	if !ok {
		return nil, ErrNotFound
	}
	p := m.portfolios[id]
	cp := *p
	return &cp, nil
}

func newTestService(t *testing.T) (*Service, *mockRepo) {
	t.Helper()
	repo := newMockRepo()
	return NewService(repo), repo
}

func TestService_Create(t *testing.T) {
	svc, _ := newTestService(t)

	p, err := svc.Create(context.Background(), CreateRequest{
		Name:     "Test Portfolio",
		Currency: "USD",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Test Portfolio" {
		t.Errorf("expected name 'Test Portfolio', got %q", p.Name)
	}
	if p.Currency != "USD" {
		t.Errorf("expected currency 'USD', got %q", p.Currency)
	}
	if p.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestService_Create_DefaultCurrency(t *testing.T) {
	svc, _ := newTestService(t)

	p, err := svc.Create(context.Background(), CreateRequest{
		Name:     "Savings",
		Currency: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Currency != "USD" {
		t.Errorf("expected currency 'USD' (default), got %q", p.Currency)
	}
}

func TestService_Create_InvalidName(t *testing.T) {
	svc, _ := newTestService(t)

	tests := []struct {
		name string
		req  CreateRequest
	}{
		{"empty name", CreateRequest{Name: "", Currency: "USD"}},
		{"too long name", CreateRequest{Name: string(make([]byte, 101)), Currency: "USD"}},
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

func TestService_Create_InvalidCurrency(t *testing.T) {
	svc, _ := newTestService(t)

	tests := []struct {
		name     string
		currency string
	}{
		{"too short", "US"},
		{"too long", "USDA"},
		{"lowercase", "usd"},
		{"with numbers", "US1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), CreateRequest{
				Name:     "Test",
				Currency: tt.currency,
			})
			if !errors.Is(err, ErrInvalidCurrency) {
				t.Errorf("expected ErrInvalidCurrency for %q, got %v", tt.currency, err)
			}
		})
	}
}

func TestService_Create_DuplicateName(t *testing.T) {
	svc, repo := newTestService(t)
	_ = repo // repo used implicitly through svc

	// Create first portfolio
	_, err := svc.Create(context.Background(), CreateRequest{
		Name:     "Dup",
		Currency: "USD",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	// Try to create duplicate
	_, err = svc.Create(context.Background(), CreateRequest{
		Name:     "Dup",
		Currency: "EUR",
	})
	if !errors.Is(err, ErrNameExists) {
		t.Errorf("expected ErrNameExists, got %v", err)
	}
}

func TestService_Create_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.err = fmt.Errorf("db error")
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateRequest{
		Name:     "Test",
		Currency: "USD",
	})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestService_Get(t *testing.T) {
	svc, repo := newTestService(t)

	// Seed a portfolio
	repo.portfolios[1] = &Portfolio{ID: 1, Name: "Test", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["Test"] = 1

	p, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Test" {
		t.Errorf("expected 'Test', got %q", p.Name)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	_, err := svc.Get(context.Background(), 999)
	if err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

func TestService_List(t *testing.T) {
	svc, repo := newTestService(t)

	// Seed portfolios
	for i := 1; i <= 5; i++ {
		repo.portfolios[int64(i)] = &Portfolio{ID: int64(i), Name: fmt.Sprintf("P%d", i), Currency: "USD"}
		repo.names[fmt.Sprintf("P%d", i)] = int64(i)
	}

	// List all
	portfolios, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(portfolios) != 5 {
		t.Errorf("expected 5 portfolios, got %d", len(portfolios))
	}

	// Paginated
	portfolios, err = svc.List(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios with limit 2, got %d", len(portfolios))
	}
}

func TestService_Update(t *testing.T) {
	svc, repo := newTestService(t)

	// Seed
	repo.portfolios[1] = &Portfolio{ID: 1, Name: "Old", Currency: "USD"}
	repo.names["Old"] = 1

	newName := "New"
	p, err := svc.Update(context.Background(), 1, UpdateRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "New" {
		t.Errorf("expected name 'New', got %q", p.Name)
	}
}

func TestService_Update_Currency(t *testing.T) {
	svc, repo := newTestService(t)

	repo.portfolios[1] = &Portfolio{ID: 1, Name: "Test", Currency: "USD"}
	repo.names["Test"] = 1

	newCurrency := "EUR"
	p, err := svc.Update(context.Background(), 1, UpdateRequest{Currency: &newCurrency})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Currency != "EUR" {
		t.Errorf("expected currency 'EUR', got %q", p.Currency)
	}
}

func TestService_Update_InvalidName(t *testing.T) {
	svc, repo := newTestService(t)

	repo.portfolios[1] = &Portfolio{ID: 1, Name: "Test", Currency: "USD"}
	repo.names["Test"] = 1

	emptyName := ""
	_, err := svc.Update(context.Background(), 1, UpdateRequest{Name: &emptyName})
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

func TestService_Update_DuplicateName(t *testing.T) {
	svc, repo := newTestService(t)

	repo.portfolios[1] = &Portfolio{ID: 1, Name: "A", Currency: "USD"}
	repo.portfolios[2] = &Portfolio{ID: 2, Name: "B", Currency: "USD"}
	repo.names["A"] = 1
	repo.names["B"] = 2

	// Try to rename A to B
	_, err := svc.Update(context.Background(), 1, UpdateRequest{Name: strPtr("B")})
	if !errors.Is(err, ErrNameExists) {
		t.Errorf("expected ErrNameExists, got %v", err)
	}
}

func TestService_Delete(t *testing.T) {
	svc, repo := newTestService(t)

	repo.portfolios[1] = &Portfolio{ID: 1, Name: "Test", Currency: "USD"}
	repo.names["Test"] = 1

	err := svc.Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's gone
	_, err = svc.Get(context.Background(), 1)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_Update_NoChanges(t *testing.T) {
	svc, repo := newTestService(t)

	originalTime := time.Now()
	repo.portfolios[1] = &Portfolio{ID: 1, Name: "Stable", Currency: "USD", UpdatedAt: originalTime}
	repo.names["Stable"] = 1

	// Update with no fields provided
	p, err := svc.Update(context.Background(), 1, UpdateRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Stable" {
		t.Errorf("expected name 'Stable', got %q", p.Name)
	}
	if p.UpdatedAt != originalTime {
		t.Errorf("expected unchanged updated_at, got %v (was %v)", p.UpdatedAt, originalTime)
	}
}

func strPtr(s string) *string {
	return &s
}
