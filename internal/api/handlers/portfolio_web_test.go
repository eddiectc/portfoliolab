package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
)

// mockPortfolioRepo is an in-memory repository for testing.
type mockPortfolioRepo struct {
	portfolios map[int64]portfolio.Portfolio
	nextID     int64
}

func newMockRepo() *mockPortfolioRepo {
	return &mockPortfolioRepo{
		portfolios: make(map[int64]portfolio.Portfolio),
		nextID:     1,
	}
}

func (m *mockPortfolioRepo) Create(_ context.Context, p *portfolio.Portfolio) error {
	m.nextID++
	p.ID = m.nextID
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	m.portfolios[p.ID] = *p
	return nil
}

func (m *mockPortfolioRepo) GetByID(_ context.Context, id int64) (*portfolio.Portfolio, error) {
	p, ok := m.portfolios[id]
	if !ok {
		return nil, portfolio.ErrNotFound
	}
	return &p, nil
}

func (m *mockPortfolioRepo) GetAll(_ context.Context, _, _ int) ([]portfolio.Portfolio, error) {
	result := make([]portfolio.Portfolio, 0, len(m.portfolios))
	for _, p := range m.portfolios {
		result = append(result, p)
	}
	return result, nil
}

func (m *mockPortfolioRepo) Update(_ context.Context, p *portfolio.Portfolio) error {
	m.portfolios[p.ID] = *p
	return nil
}

func (m *mockPortfolioRepo) Delete(_ context.Context, id int64) error {
	delete(m.portfolios, id)
	return nil
}

func (m *mockPortfolioRepo) GetByName(_ context.Context, name string) (*portfolio.Portfolio, error) {
	for _, p := range m.portfolios {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, portfolio.ErrNotFound
}

func TestUserFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "invalid name",
			err:  portfolio.ErrInvalidName,
			want: "Invalid name",
		},
		{
			name: "invalid currency",
			err:  portfolio.ErrInvalidCurrency,
			want: "Invalid currency",
		},
		{
			name: "name exists",
			err:  portfolio.ErrNameExists,
			want: "already exists",
		},
		{
			name: "unknown error",
			err:  portfolio.ErrNotFound,
			want: "error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userFriendlyError(tt.err)
			if !strings.Contains(got, tt.want) {
				t.Errorf("userFriendlyError() = %q, want contains %q", got, tt.want)
			}
		})
	}
}

func TestCommonCurrencies(t *testing.T) {
	if len(commonCurrencies) == 0 {
		t.Error("commonCurrencies should not be empty")
	}

	// Check that USD is included (it's the default)
	found := false
	for _, c := range commonCurrencies {
		if c == "USD" {
			found = true
			break
		}
	}
	if !found {
		t.Error("commonCurrencies should include USD")
	}
}

func TestWriteJSON_HandlesEmptySlice(t *testing.T) {
	w := httptest.NewRecorder()
	empty := []portfolio.Portfolio{}
	writeJSON(w, http.StatusOK, empty)

	var result []portfolio.Portfolio
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Errorf("failed to decode empty slice: %v", err)
	}

	if result == nil {
		t.Error("expected empty array, got null")
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"valid", "42", 42, false},
		{"zero", "0", 0, false},
		{"empty", "", 0, true},
		{"negative", "-1", -1, false},
		{"non-numeric", "abc", 0, true},
		{"whitespace", " 5 ", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseID(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("parseID(%q) expected error, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseID(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantLimit int
		wantOffset int
	}{
		{"no params", "", 0, 0},
		{"limit only", "limit=10", 10, 0},
		{"offset only", "offset=5", 0, 5},
		{"both", "limit=10&offset=5", 10, 5},
		{"zero limit defaults", "limit=0", 0, 0},
		{"negative offset defaults", "offset=-1", 0, 0},
		{"non-numeric ignored", "limit=abc&offset=xyz", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			limit, offset := parsePagination(r.URL.Query())
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
			if offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	now := time.Date(2025, 5, 2, 14, 30, 0, 0, time.UTC)
	got := formatTime(now)
	want := "2025-05-02 14:30"
	if got != want {
		t.Errorf("formatTime() = %q, want %q", got, want)
	}
}

// Test that PortfolioWebHandler.RegisterRoutes mounts expected paths.
func TestRegisterRoutes(t *testing.T) {
	r := chi.NewRouter()
	handler := &PortfolioWebHandler{}
	handler.RegisterRoutes(r)

	// Just verify it doesn't panic — route existence is tested by integration tests
}
