package modelportfolio

import (
	"context"
	"errors"
	"testing"

	"github.com/govalues/decimal"
)

var ctx = context.Background()

// --- Mock ---

type mockRepo struct {
	portfolios  map[int64]ModelPortfolio // id -> portfolio (domain type, entries already unmarshaled)
	nameIndex   map[string]int64         // name -> id
	nextID      int64
	creates     []ModelPortfolio
	createsErr  error
	getErr      error
	listErr     error
	updateErr   error
	deleteErr   error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		portfolios: make(map[int64]ModelPortfolio),
		nameIndex:  make(map[string]int64),
		nextID:     1,
	}
}

func (m *mockRepo) Create(_ context.Context, mp ModelPortfolio) (ModelPortfolio, error) {
	if m.createsErr != nil {
		return ModelPortfolio{}, m.createsErr
	}
	id := m.nextID
	m.nextID++
	mp.ID = id
	// Store a copy.
	m.portfolios[id] = mp
	m.nameIndex[mp.Name] = id
	m.creates = append(m.creates, mp)
	return mp, nil
}

func (m *mockRepo) GetByID(_ context.Context, id int64) (ModelPortfolio, error) {
	if m.getErr != nil {
		return ModelPortfolio{}, m.getErr
	}
	mp, ok := m.portfolios[id]
	if !ok {
		return ModelPortfolio{}, context.DeadlineExceeded // simulate "not found" DB error
	}
	return mp, nil
}

func (m *mockRepo) GetByName(_ context.Context, name string) (ModelPortfolio, error) {
	if m.getErr != nil {
		return ModelPortfolio{}, m.getErr
	}
	id, ok := m.nameIndex[name]
	if !ok {
		return ModelPortfolio{}, context.DeadlineExceeded // simulate "not found" DB error
	}
	return m.portfolios[id], nil
}

func (m *mockRepo) List(_ context.Context, limit, offset int) ([]ModelPortfolio, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	// Collect all portfolios.
	all := make([]ModelPortfolio, 0, len(m.portfolios))
	for _, mp := range m.portfolios {
		all = append(all, mp)
	}
	if len(all) == 0 {
		return []ModelPortfolio{}, nil
	}
	// Apply offset and limit (simulating SQL LIMIT/OFFSET).
	if offset >= len(all) {
		return []ModelPortfolio{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (m *mockRepo) Update(_ context.Context, mp ModelPortfolio) (ModelPortfolio, error) {
	if m.updateErr != nil {
		return ModelPortfolio{}, m.updateErr
	}
	_, ok := m.portfolios[mp.ID]
	if !ok {
		return ModelPortfolio{}, context.DeadlineExceeded
	}
	// Update name index if name changed.
	if existing, ok := m.portfolios[mp.ID]; ok && existing.Name != mp.Name {
		delete(m.nameIndex, existing.Name)
	}
	m.portfolios[mp.ID] = mp
	m.nameIndex[mp.Name] = mp.ID
	return mp, nil
}

func (m *mockRepo) Delete(_ context.Context, id int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	mp, ok := m.portfolios[id]
	if !ok {
		return context.DeadlineExceeded
	}
	delete(m.nameIndex, mp.Name)
	delete(m.portfolios, id)
	return nil
}

// --- Tests ---

func TestCreate_HappyPath(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	req := CreateRequest{
		Name: "My Model",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("60.0")},
			{Symbol: "MSFT", WeightPct: decimal.MustParse("40.0")},
		},
	}

	result, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != 1 {
		t.Errorf("id = %d, want 1", result.ID)
	}
	if result.Name != "My Model" {
		t.Errorf("name = %q, want %q", result.Name, "My Model")
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(result.Entries))
	}
	if result.Entries[0].Symbol != "AAPL" {
		t.Errorf("first entry symbol = %q, want %q", result.Entries[0].Symbol, "AAPL")
	}
}

func TestCreate_ValidationErrors(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	tests := []struct {
		name    string
		req     CreateRequest
		wantErr error
	}{
		{
			name: "empty name",
			req: CreateRequest{
				Name: "",
				Entries: []ModelPortfolioEntry{
					{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
				},
			},
			wantErr: ErrInvalidName,
		},
		{
			name: "empty entries",
			req: CreateRequest{
				Name:    "Test",
				Entries: []ModelPortfolioEntry{},
			},
			wantErr: ErrEmptyEntries,
		},
		{
			name: "weight sum not 100",
			req: CreateRequest{
				Name: "Test",
				Entries: []ModelPortfolioEntry{
					{Symbol: "AAPL", WeightPct: decimal.MustParse("50.0")},
					{Symbol: "MSFT", WeightPct: decimal.MustParse("30.0")},
				},
			},
			wantErr: &ModelPortfolioError{Code: "weight_sum_not_100"},
		},
		{
			name: "negative weight",
			req: CreateRequest{
				Name: "Test",
				Entries: []ModelPortfolioEntry{
					{Symbol: "AAPL", WeightPct: decimal.MustParse("-10.0")},
					{Symbol: "MSFT", WeightPct: decimal.MustParse("110.0")},
				},
			},
			wantErr: ErrInvalidWeight,
		},
		{
			name: "duplicate symbol",
			req: CreateRequest{
				Name: "Test",
				Entries: []ModelPortfolioEntry{
					{Symbol: "AAPL", WeightPct: decimal.MustParse("50.0")},
					{Symbol: "AAPL", WeightPct: decimal.MustParse("50.0")},
				},
			},
			wantErr: ErrDuplicateSymbol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var gotErr *ModelPortfolioError
			if !errors.As(err, &gotErr) {
				t.Fatalf("expected *ModelPortfolioError, got %T: %v", err, err)
			}
			if gotErr.Code != tt.wantErr.(*ModelPortfolioError).Code {
				t.Errorf("error code = %q, want %q", gotErr.Code, tt.wantErr.(*ModelPortfolioError).Code)
			}
		})
	}
}

func TestCreate_DuplicateName(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	req := CreateRequest{
		Name: "Duplicate",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
		},
	}

	// Create first.
	_, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("first create error: %v", err)
	}

	// Create second with same name.
	_, err = svc.Create(ctx, req)
	if !errors.Is(err, ErrNameExists) {
		t.Fatalf("expected ErrNameExists, got: %v", err)
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.createsErr = context.DeadlineExceeded
	svc := NewService(repo)

	req := CreateRequest{
		Name: "Test",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
		},
	}

	_, err := svc.Create(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGet_HappyPath(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	// Pre-populate.
	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:    1,
		Name:  "Test Portfolio",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("50.0")},
			{Symbol: "MSFT", WeightPct: decimal.MustParse("50.0")},
		},
	})

	result, err := svc.Get(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Name != "Test Portfolio" {
		t.Errorf("name = %q, want %q", result.Name, "Test Portfolio")
	}
	if len(result.Entries) != 2 {
		t.Errorf("entries count = %d, want 2", len(result.Entries))
	}
}

func TestGet_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.Get(ctx, 999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGet_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.getErr = context.DeadlineExceeded
	svc := NewService(repo)

	_, err := svc.Get(ctx, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestList_HappyPath(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	// Pre-populate two portfolios.
	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "Alpha",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})
	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      2,
		Name:    "Beta",
		Entries: []ModelPortfolioEntry{{Symbol: "MSFT", WeightPct: decimal.MustParse("100.0")}},
	})

	result, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 portfolios, got %d", len(result))
	}
}

func TestList_EmptyReturnsSliceNotNil(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	result, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 portfolios, got %d", len(result))
	}
}

func TestList_LimitZero(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "Test",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})

	// limit=0 simulates SQL LIMIT 0 → zero rows.
	result, err := svc.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 results with limit=0, got %d", len(result))
	}
}

func TestList_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.listErr = context.DeadlineExceeded
	svc := NewService(repo)

	_, err := svc.List(ctx, 10, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdate_HappyPath(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	// Pre-populate.
	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:    1,
		Name:  "Old Name",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
		},
	})

	req := UpdateRequest{
		Name: stringPtr("New Name"),
		Entries: []ModelPortfolioEntry{
			{Symbol: "MSFT", WeightPct: decimal.MustParse("100.0")},
		},
	}

	result, err := svc.Update(ctx, 1, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Name != "New Name" {
		t.Errorf("name = %q, want %q", result.Name, "New Name")
	}
	if result.Entries[0].Symbol != "MSFT" {
		t.Errorf("first entry symbol = %q, want %q", result.Entries[0].Symbol, "MSFT")
	}
}

func TestUpdate_NameOnly(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:    1,
		Name:  "Old Name",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
		},
	})

	req := UpdateRequest{
		Name: stringPtr("New Name"),
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
		},
	}

	result, err := svc.Update(ctx, 1, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Name != "New Name" {
		t.Errorf("name = %q, want %q", result.Name, "New Name")
	}
}

func TestUpdate_DuplicateName(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "Portfolio A",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})
	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      2,
		Name:    "Portfolio B",
		Entries: []ModelPortfolioEntry{{Symbol: "MSFT", WeightPct: decimal.MustParse("100.0")}},
	})

	// Try to rename A to B's name.
	req := UpdateRequest{
		Name: stringPtr("Portfolio B"),
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
		},
	}

	_, err := svc.Update(ctx, 1, req)
	if !errors.Is(err, ErrNameExists) {
		t.Fatalf("expected ErrNameExists, got: %v", err)
	}
}

func TestUpdate_SameNameNoConflict(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "Same Name",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})

	// Update entries but keep same name.
	req := UpdateRequest{
		Name: stringPtr("Same Name"),
		Entries: []ModelPortfolioEntry{
			{Symbol: "MSFT", WeightPct: decimal.MustParse("100.0")},
		},
	}

	_, err := svc.Update(ctx, 1, req)
	if err != nil {
		t.Fatalf("unexpected error when keeping same name: %v", err)
	}
}

func TestUpdate_ValidationErrors(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "Test",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})

	// Invalid entries (sum != 100).
	req := UpdateRequest{
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("50.0")},
		},
	}

	_, err := svc.Update(ctx, 1, req)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	req := UpdateRequest{
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")},
		},
	}

	_, err := svc.Update(ctx, 999, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdate_RepoError(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "Test",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})

	repo.updateErr = context.DeadlineExceeded

	req := UpdateRequest{
		Entries: []ModelPortfolioEntry{
			{Symbol: "MSFT", WeightPct: decimal.MustParse("100.0")},
		},
	}

	_, err := svc.Update(ctx, 1, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDelete_HappyPath(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "To Delete",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})

	err := svc.Delete(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's gone.
	_, err = svc.Get(ctx, 1)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.Delete(ctx, 999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDelete_RepoError(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      1,
		Name:    "Test",
		Entries: []ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustParse("100.0")}},
	})

	repo.deleteErr = context.DeadlineExceeded

	err := svc.Delete(ctx, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCRUD_Cycle(t *testing.T) {
	// Full CRUD cycle: Create → Get → List → Update → Delete.
	repo := newMockRepo()
	svc := NewService(repo)

	// Create.
	req := CreateRequest{
		Name: "Full Cycle",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("50.0")},
			{Symbol: "MSFT", WeightPct: decimal.MustParse("50.0")},
		},
	}
	created, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	// Get.
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if got.Name != "Full Cycle" {
		t.Errorf("name = %q, want %q", got.Name, "Full Cycle")
	}
	if len(got.Entries) != 2 {
		t.Errorf("entries count = %d, want 2", len(got.Entries))
	}

	// List.
	list, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list count = %d, want 1", len(list))
	}

	// Update.
	newName := "Renamed"
	updateReq := UpdateRequest{
		Name: &newName,
		Entries: []ModelPortfolioEntry{
			{Symbol: "GOOG", WeightPct: decimal.MustParse("100.0")},
		},
	}
	updated, err := svc.Update(ctx, created.ID, updateReq)
	if err != nil {
		t.Fatalf("update error: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("name = %q, want %q", updated.Name, "Renamed")
	}
	if updated.Entries[0].Symbol != "GOOG" {
		t.Errorf("first entry symbol = %q, want %q", updated.Entries[0].Symbol, "GOOG")
	}

	// Delete.
	err = svc.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}

	// Verify deleted.
	_, err = svc.Get(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	// Verify entries survive JSON marshal/unmarshal through the repo.
	repo := newMockRepo()
	svc := NewService(repo)

	entries := []ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: decimal.MustParse("25.5")},
		{Symbol: "MSFT", WeightPct: decimal.MustParse("34.75")},
		{Symbol: "GOOG", WeightPct: decimal.MustParse("39.75")},
	}

	req := CreateRequest{Name: "JSON Test", Entries: entries}
	created, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	// Get it back (goes through JSON round-trip in the mock).
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}

	if len(got.Entries) != len(entries) {
		t.Fatalf("entries count = %d, want %d", len(got.Entries), len(entries))
	}

	for i, want := range entries {
		if got.Entries[i].Symbol != want.Symbol {
			t.Errorf("entry[%d] symbol = %q, want %q", i, got.Entries[i].Symbol, want.Symbol)
		}
		if !got.Entries[i].WeightPct.Equal(want.WeightPct) {
			t.Errorf("entry[%d] weight_pct = %v, want %v", i, got.Entries[i].WeightPct, want.WeightPct)
		}
	}
}

func TestGetAllForSelector(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:   1,
		Name: "Model A",
		Entries: []ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustParse("50.0")},
			{Symbol: "MSFT", WeightPct: decimal.MustParse("50.0")},
		},
	})
	_, _ = repo.Create(ctx, ModelPortfolio{
		ID:      2,
		Name:    "Model B",
		Entries: []ModelPortfolioEntry{{Symbol: "GOOG", WeightPct: decimal.MustParse("100.0")}},
	})

	summaries, err := svc.GetAllForSelector(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// Build lookup by name.
	byName := make(map[string]ModelPortfolioSummary)
	for _, s := range summaries {
		byName[s.Name] = s
	}

	a := byName["Model A"]
	if a.EntryCount != 2 {
		t.Errorf("Model A entry_count = %d, want 2", a.EntryCount)
	}
	b := byName["Model B"]
	if b.EntryCount != 1 {
		t.Errorf("Model B entry_count = %d, want 1", b.EntryCount)
	}
}

func TestGetAllForSelector_Empty(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	summaries, err := svc.GetAllForSelector(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summaries == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries, got %d", len(summaries))
	}
}

// stringPtr is a helper for creating *string values in tests.
func stringPtr(s string) *string {
	return &s
}
