package allocation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/govalues/decimal"
)

// --- Mock ---

type mockTargetRepo struct {
	targets             map[int64][]TargetAllocation // portfolioID -> targets
	upserts             []TargetAllocation
	deleteSymbols       []string
	deleteAllPortfolios []int64
	getErr              error
	upsertErr           error
	deleteErr           error
}

func (m *mockTargetRepo) GetByPortfolio(_ context.Context, portfolioID int64) ([]TargetAllocation, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.targets == nil {
		return []TargetAllocation{}, nil
	}
	t := m.targets[portfolioID]
	if t == nil {
		return []TargetAllocation{}, nil
	}
	return t, nil
}

func (m *mockTargetRepo) Upsert(_ context.Context, ta TargetAllocation) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	if m.upserts == nil {
		m.upserts = []TargetAllocation{}
	}
	m.upserts = append(m.upserts, ta)
	// Simulate real behavior: store in map for subsequent GetByPortfolio calls.
	if m.targets == nil {
		m.targets = make(map[int64][]TargetAllocation)
	}
	// Remove existing entry for this portfolio+symbol.
	var existing []TargetAllocation
	for _, t := range m.targets[ta.PortfolioID] {
		if t.Symbol != ta.Symbol {
			existing = append(existing, t)
		}
	}
	existing = append(existing, ta)
	m.targets[ta.PortfolioID] = existing
	return nil
}

func (m *mockTargetRepo) DeleteBySymbol(_ context.Context, portfolioID int64, symbol string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if m.deleteSymbols == nil {
		m.deleteSymbols = []string{}
	}
	m.deleteSymbols = append(m.deleteSymbols, symbol)
	// Simulate real behavior: remove from map.
	if m.targets != nil {
		var remaining []TargetAllocation
		for _, t := range m.targets[portfolioID] {
			if t.Symbol != symbol {
				remaining = append(remaining, t)
			}
		}
		if len(remaining) == 0 {
			delete(m.targets, portfolioID)
		} else {
			m.targets[portfolioID] = remaining
		}
	}
	return nil
}

func (m *mockTargetRepo) DeleteByPortfolio(_ context.Context, portfolioID int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if m.deleteAllPortfolios == nil {
		m.deleteAllPortfolios = []int64{}
	}
	m.deleteAllPortfolios = append(m.deleteAllPortfolios, portfolioID)
	// Simulate real behavior: remove from map.
	if m.targets != nil {
		delete(m.targets, portfolioID)
	}
	return nil
}

// --- Tests ---

func TestSaveTargetAllocation_Valid(t *testing.T) {
	repo := &mockTargetRepo{}
	svc := NewService(nil, nil, repo)

	entries := []TargetEntry{
		{Symbol: "AAPL", TargetPct: decimal.MustParse("60.0")},
		{Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
	}

	err := svc.SaveTargetAllocation(ctx, 1, entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.upserts) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(repo.upserts))
	}

	if repo.upserts[0].Symbol != "AAPL" {
		t.Errorf("first upsert symbol = %q, want %q", repo.upserts[0].Symbol, "AAPL")
	}
	if !repo.upserts[0].TargetPct.Equal(decimal.MustParse("60.0")) {
		t.Errorf("first upsert target_pct = %v, want 60.0", repo.upserts[0].TargetPct)
	}
	if repo.upserts[0].PortfolioID != 1 {
		t.Errorf("first upsert portfolio_id = %d, want 1", repo.upserts[0].PortfolioID)
	}
}

func TestSaveTargetAllocation_Validation(t *testing.T) {
	repo := &mockTargetRepo{}
	svc := NewService(nil, nil, repo)

	tests := []struct {
		name    string
		entries []TargetEntry
		wantErr string
	}{
		{
			name: "negative percentage",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("-5.0")},
				{Symbol: "MSFT", TargetPct: decimal.MustParse("105.0")},
			},
			wantErr: "invalid_target_pct",
		},
		{
			name: "percentage over 100",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("101.0")},
				{Symbol: "MSFT", TargetPct: decimal.MustParse("-1.0")},
			},
			wantErr: "invalid_target_pct",
		},
		{
			name: "sum less than 100",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("30.0")},
				{Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
			},
			wantErr: "target_sum_not_100",
		},
		{
			name: "sum greater than 100",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("60.0")},
				{Symbol: "MSFT", TargetPct: decimal.MustParse("60.0")},
			},
			wantErr: "target_sum_not_100",
		},
		{
			name:    "empty entries",
			entries: []TargetEntry{},
			wantErr: "target_sum_not_100",
		},
		{
			name: "empty symbol",
			entries: []TargetEntry{
				{Symbol: "", TargetPct: decimal.MustParse("50.0")},
				{Symbol: "MSFT", TargetPct: decimal.MustParse("50.0")},
			},
			wantErr: "invalid_target_pct",
		},
		{
			name: "duplicate symbol",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("50.0")},
				{Symbol: "AAPL", TargetPct: decimal.MustParse("50.0")},
			},
			wantErr: "duplicate_symbol",
		},
		{
			name: "valid three entries",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("40.0")},
				{Symbol: "MSFT", TargetPct: decimal.MustParse("35.0")},
				{Symbol: "$CASH", TargetPct: decimal.MustParse("25.0")},
			},
			wantErr: "",
		},
		{
			name: "valid zero weight",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("100.0")},
				{Symbol: "MSFT", TargetPct: decimal.MustParse("0.0")},
			},
			wantErr: "",
		},
		{
			name: "exact 100 single entry",
			entries: []TargetEntry{
				{Symbol: "AAPL", TargetPct: decimal.MustParse("100.0")},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.SaveTargetAllocation(ctx, 1, tt.entries)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var allocErr *AllocationError
				if !errors.As(err, &allocErr) {
					t.Fatalf("expected *AllocationError, got %T: %v", err, err)
				}
				if allocErr.Code != tt.wantErr {
					t.Errorf("error code = %q, want %q", allocErr.Code, tt.wantErr)
				}
			}
		})
	}
}

func TestSaveTargetAllocation_SumErrorMessage(t *testing.T) {
	repo := &mockTargetRepo{}
	svc := NewService(nil, nil, repo)

	entries := []TargetEntry{
		{Symbol: "AAPL", TargetPct: decimal.MustParse("30.0")},
		{Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
	}

	err := svc.SaveTargetAllocation(ctx, 1, entries)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "70.0") {
		t.Errorf("error message should contain current total 70.0, got: %s", msg)
	}
	if !strings.Contains(msg, "30.0") {
		t.Errorf("error message should contain delta 30.0, got: %s", msg)
	}
}

func TestSaveTargetAllocation_RepoError(t *testing.T) {
	repo := &mockTargetRepo{upsertErr: context.DeadlineExceeded}
	svc := NewService(nil, nil, repo)

	entries := []TargetEntry{
		{Symbol: "AAPL", TargetPct: decimal.MustParse("100.0")},
	}

	err := svc.SaveTargetAllocation(ctx, 1, entries)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetTargetAllocation(t *testing.T) {
	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("60.0")},
				{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
			},
		},
	}
	svc := NewService(nil, nil, repo)

	targets, err := svc.GetTargetAllocation(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	if targets[0].Symbol != "AAPL" {
		t.Errorf("first target symbol = %q, want %q", targets[0].Symbol, "AAPL")
	}
	if !targets[0].TargetPct.Equal(decimal.MustParse("60.0")) {
		t.Errorf("first target target_pct = %v, want 60.0", targets[0].TargetPct)
	}
}

func TestGetTargetAllocation_Empty(t *testing.T) {
	repo := &mockTargetRepo{}
	svc := NewService(nil, nil, repo)

	targets, err := svc.GetTargetAllocation(ctx, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 0 {
		t.Errorf("expected 0 targets, got %d", len(targets))
	}
}

func TestGetTargetAllocation_RepoError(t *testing.T) {
	repo := &mockTargetRepo{getErr: context.DeadlineExceeded}
	svc := NewService(nil, nil, repo)

	_, err := svc.GetTargetAllocation(ctx, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeleteTargetAllocation(t *testing.T) {
	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("60.0")},
				{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
			},
		},
	}
	svc := NewService(nil, nil, repo)

	err := svc.DeleteTargetAllocation(ctx, 1, "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify AAPL was removed from the mock.
	remaining, _ := repo.GetByPortfolio(ctx, 1)
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining target, got %d", len(remaining))
	}
	if remaining[0].Symbol != "MSFT" {
		t.Errorf("remaining symbol = %q, want %q", remaining[0].Symbol, "MSFT")
	}
}

func TestDeleteTargetAllocation_RepoError(t *testing.T) {
	repo := &mockTargetRepo{deleteErr: context.DeadlineExceeded}
	svc := NewService(nil, nil, repo)

	err := svc.DeleteTargetAllocation(ctx, 1, "AAPL")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeleteAllTargetAllocations(t *testing.T) {
	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("60.0")},
				{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
			},
			2: {
				{PortfolioID: 2, Symbol: "GOOG", TargetPct: decimal.MustParse("100.0")},
			},
		},
	}
	svc := NewService(nil, nil, repo)

	err := svc.DeleteAllTargetAllocations(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify portfolio 1 targets are gone but portfolio 2 remains.
	remaining1, _ := repo.GetByPortfolio(ctx, 1)
	if len(remaining1) != 0 {
		t.Errorf("expected 0 targets for portfolio 1, got %d", len(remaining1))
	}

	remaining2, _ := repo.GetByPortfolio(ctx, 2)
	if len(remaining2) != 1 {
		t.Errorf("expected 1 target for portfolio 2, got %d", len(remaining2))
	}
}

func TestDeleteAllTargetAllocations_RepoError(t *testing.T) {
	repo := &mockTargetRepo{deleteErr: context.DeadlineExceeded}
	svc := NewService(nil, nil, repo)

	err := svc.DeleteAllTargetAllocations(ctx, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSaveThenGetTargetAllocation(t *testing.T) {
	// Full CRUD cycle: save targets, then retrieve them.
	repo := &mockTargetRepo{}
	svc := NewService(nil, nil, repo)

	entries := []TargetEntry{
		{Symbol: "AAPL", TargetPct: decimal.MustParse("50.0")},
		{Symbol: "MSFT", TargetPct: decimal.MustParse("30.0")},
		{Symbol: "$CASH", TargetPct: decimal.MustParse("20.0")},
	}

	err := svc.SaveTargetAllocation(ctx, 1, entries)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	targets, err := svc.GetTargetAllocation(ctx, 1)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}

	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	// Build map for easy lookup.
	targetMap := make(map[string]decimal.Decimal)
	for _, ta := range targets {
		targetMap[ta.Symbol] = ta.TargetPct
	}

	if pct, ok := targetMap["AAPL"]; !ok || !pct.Equal(decimal.MustParse("50.0")) {
		t.Errorf("AAPL target_pct = %v, want 50.0", pct)
	}
	if pct, ok := targetMap["MSFT"]; !ok || !pct.Equal(decimal.MustParse("30.0")) {
		t.Errorf("MSFT target_pct = %v, want 30.0", pct)
	}
	if pct, ok := targetMap["$CASH"]; !ok || !pct.Equal(decimal.MustParse("20.0")) {
		t.Errorf("Cash target_pct = %v, want 20.0", pct)
	}
}

func TestSaveTargetAllocation_UpdateExisting(t *testing.T) {
	// Save targets, then save again with different values.
	repo := &mockTargetRepo{}
	svc := NewService(nil, nil, repo)

	// Initial targets.
	entries1 := []TargetEntry{
		{Symbol: "AAPL", TargetPct: decimal.MustParse("60.0")},
		{Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
	}
	if err := svc.SaveTargetAllocation(ctx, 1, entries1); err != nil {
		t.Fatalf("initial save error: %v", err)
	}

	// Updated targets (AAPL reduced, MSFT increased).
	entries2 := []TargetEntry{
		{Symbol: "AAPL", TargetPct: decimal.MustParse("40.0")},
		{Symbol: "MSFT", TargetPct: decimal.MustParse("60.0")},
	}
	if err := svc.SaveTargetAllocation(ctx, 1, entries2); err != nil {
		t.Fatalf("update save error: %v", err)
	}

	targets, err := svc.GetTargetAllocation(ctx, 1)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	targetMap := make(map[string]decimal.Decimal)
	for _, ta := range targets {
		targetMap[ta.Symbol] = ta.TargetPct
	}

	if pct, ok := targetMap["AAPL"]; !ok || !pct.Equal(decimal.MustParse("40.0")) {
		t.Errorf("AAPL target_pct = %v, want 40.0", pct)
	}
	if pct, ok := targetMap["MSFT"]; !ok || !pct.Equal(decimal.MustParse("60.0")) {
		t.Errorf("MSFT target_pct = %v, want 60.0", pct)
	}
}
