package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// testPortfolioRepoForWeb is a minimal in-memory mock repo for portfolio web handler tests.
type testPortfolioRepoForWeb struct {
	portfolios map[int64]*portfolio.Portfolio
	names      map[string]int64 // name -> id
	nextID     int64
}

func newTestPortfolioRepoForWeb() *testPortfolioRepoForWeb {
	return &testPortfolioRepoForWeb{
		portfolios: make(map[int64]*portfolio.Portfolio),
		names:      make(map[string]int64),
		nextID:     1,
	}
}

func (r *testPortfolioRepoForWeb) Create(_ context.Context, p *portfolio.Portfolio) error {
	r.nextID++
	p.ID = r.nextID
	r.portfolios[p.ID] = p
	r.names[p.Name] = p.ID
	return nil
}

func (r *testPortfolioRepoForWeb) GetByID(_ context.Context, id int64) (*portfolio.Portfolio, error) {
	p, ok := r.portfolios[id]
	if !ok {
		return nil, portfolio.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *testPortfolioRepoForWeb) GetAll(_ context.Context, limit, offset int) ([]portfolio.Portfolio, error) {
	var result []portfolio.Portfolio
	for _, p := range r.portfolios {
		cp := *p
		result = append(result, cp)
	}
	if offset > 0 && offset < len(result) {
		result = result[offset:]
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *testPortfolioRepoForWeb) ListAll(_ context.Context) ([]portfolio.Portfolio, error) {
	var result []portfolio.Portfolio
	for _, p := range r.portfolios {
		cp := *p
		result = append(result, cp)
	}
	return result, nil
}

func (r *testPortfolioRepoForWeb) Update(_ context.Context, p *portfolio.Portfolio) error {
	r.portfolios[p.ID] = p
	return nil
}

func (r *testPortfolioRepoForWeb) Delete(_ context.Context, id int64) error {
	p, ok := r.portfolios[id]
	if !ok {
		return portfolio.ErrNotFound
	}
	delete(r.names, p.Name)
	delete(r.portfolios, id)
	return nil
}

func (r *testPortfolioRepoForWeb) GetByName(_ context.Context, name string) (*portfolio.Portfolio, error) {
	id, ok := r.names[name]
	if !ok {
		return nil, portfolio.ErrNotFound
	}
	p := r.portfolios[id]
	cp := *p
	return &cp, nil
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
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{"no params", "", 50, 0},
		{"limit only", "limit=10", 10, 0},
		{"offset only", "offset=5", 50, 5},
		{"both", "limit=10&offset=5", 10, 5},
		{"zero limit defaults", "limit=0", 50, 0},
		{"negative offset defaults", "offset=-1", 50, 0},
		{"non-numeric ignored", "limit=abc&offset=xyz", 50, 0},
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

// --- Template rendering tests ---

// findTemplatesDir walks up from the current directory to find the templates/ dir.
// This works regardless of whether tests run from the package dir or module root.
func findTemplatesDir() string {
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(dir + "/templates"); err == nil {
			// Return the relative path from CWD to the found templates directory
			rel, _ := filepath.Rel(cwd, dir+"/templates")
			return rel
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fallback: try relative paths from common test working directories
	for _, candidate := range []string{"../../../templates", "../../templates", "../templates", "templates"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "templates" // best effort
}

func newTestRenderer(t *testing.T) *web.Renderer {
	t.Helper()
	renderer, err := web.NewRenderer(findTemplatesDir())
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}
	return renderer
}

func newTestWebHandler(t *testing.T) *PortfolioWebHandler {
	t.Helper()
	repo := newTestPortfolioRepoForWeb()
	svc := portfolio.NewService(repo)
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	renderer := newTestRenderer(t)
	return NewPortfolioWebHandler(svc, accountSvc, renderer)
}

// setupWebHandlerWithService creates a web handler with a real service backed by a mock repo.
// Returns the handler, service, and mock repo so callers can seed data.
func setupWebHandlerWithService(t *testing.T) (*PortfolioWebHandler, *portfolio.Service, *testPortfolioRepoForWeb) {
	t.Helper()
	repo := newTestPortfolioRepoForWeb()
	svc := portfolio.NewService(repo)
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	renderer := newTestRenderer(t)
	return NewPortfolioWebHandler(svc, accountSvc, renderer), svc, repo
}

// TestHandleNewPage_RendersCompleteForm verifies that GET /portfolios/new
// renders a complete form with all expected elements (input, select, button).
// This is a regression test for the bug where missing struct fields caused
// the template to panic and the response to be truncated.
func TestHandleNewPage_RendersCompleteForm(t *testing.T) {
	handler := newTestWebHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/portfolios/new", nil)
	w := httptest.NewRecorder()

	handler.HandleNewPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	// Verify the page structure is complete
	checkContains := func(t *testing.T, label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	checkContains(t, "DOCTYPE", "<!DOCTYPE html>")
	checkContains(t, "title", "New Portfolio")
	checkContains(t, "form tag", `<form action="/portfolios" method="POST"`)
	checkContains(t, "name input", `id="name"`)
	checkContains(t, "name input type", `type="text"`)
	checkContains(t, "name input required", `required`)
	checkContains(t, "currency select", `id="currency"`)
	checkContains(t, "USD option", `<option value="USD"`)
	checkContains(t, "EUR option", `<option value="EUR"`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "submit text", "Create Portfolio")
	checkContains(t, "cancel link", `href="/portfolios"`)
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing body", "</body>")
	checkContains(t, "closing html", "</html>")
}

// TestHandleCreatePage_ValidSubmission creates a portfolio via form and verifies redirect.
func TestHandleCreatePage_ValidSubmission(t *testing.T) {
	handler, _, repo := setupWebHandlerWithService(t)
	_ = repo

	body := strings.NewReader("name=Test+Portfolio&currency=GBP")
	r := httptest.NewRequest(http.MethodPost, "/portfolios", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/portfolios" {
		t.Errorf("expected redirect to /portfolios, got %q", location)
	}
}

// TestHandleCreatePage_EmptyName shows validation error on the form.
func TestHandleCreatePage_EmptyName(t *testing.T) {
	handler := newTestWebHandler(t)
	body := strings.NewReader("name=&currency=USD")
	r := httptest.NewRequest(http.MethodPost, "/portfolios", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()

	// Should show the form again with error
	if !strings.Contains(pageBody, "Invalid name") {
		t.Error("expected validation error message, got: " + pageBody[:200])
	}

	// Form should still be complete (regression: error path must also have all fields)
	checkContains := func(t *testing.T, label, text string) {
		t.Helper()
		if !strings.Contains(pageBody, text) {
			t.Errorf("error page missing %s: %q", label, text)
		}
	}

	checkContains(t, "name input", `id="name"`)
	checkContains(t, "currency select", `id="currency"`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing html", "</html>")
}

// TestHandleCreatePage_InvalidCurrency shows validation error on the form.
func TestHandleCreatePage_InvalidCurrency(t *testing.T) {
	handler := newTestWebHandler(t)
	body := strings.NewReader("name=Test&currency=XX")
	r := httptest.NewRequest(http.MethodPost, "/portfolios", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()
	if !strings.Contains(pageBody, "Invalid currency") {
		t.Error("expected currency validation error")
	}

	// The submitted name should be preserved
	if !strings.Contains(pageBody, `value="Test"`) {
		t.Error("expected submitted name to be preserved in form")
	}
}

// TestHandleCreatePage_DuplicateName shows conflict error on the form.
func TestHandleCreatePage_DuplicateName(t *testing.T) {
	handler, _, repo := setupWebHandlerWithService(t)

	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Existing", Currency: "USD"}
	repo.names["Existing"] = 1

	body := strings.NewReader("name=Existing&currency=EUR")
	r := httptest.NewRequest(http.MethodPost, "/portfolios", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()
	if !strings.Contains(pageBody, "already exists") {
		t.Error("expected duplicate name error")
	}
}

// TestHandleEditPage_RendersCompleteForm verifies GET /portfolios/{id}/edit
// renders a complete form with pre-filled values.
func TestHandleEditPage_RendersCompleteForm(t *testing.T) {
	handler, _, repo := setupWebHandlerWithService(t)

	repo.portfolios[2] = &portfolio.Portfolio{ID: 2, Name: "My Portfolio", Currency: "CHF", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.names["My Portfolio"] = 2

	// Manually set the URL param via chi context
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "2")
	r := httptest.NewRequest(http.MethodGet, "/portfolios/2/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	w := httptest.NewRecorder()
	handler.HandleEditPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()

	// Form should be complete
	checkContains := func(t *testing.T, label, text string) {
		t.Helper()
		if !strings.Contains(pageBody, text) {
			t.Errorf("edit page missing %s: %q", label, text)
		}
	}

	checkContains(t, "title", "Edit Portfolio")
	checkContains(t, "name input", `id="name"`)
	checkContains(t, "pre-filled name", `value="My Portfolio"`)
	checkContains(t, "currency select", `id="currency"`)
	checkContains(t, "selected currency", `selected`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "submit text", "Save Changes")
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing html", "</html>")
}

// TestHandleListPage_RendersCompletePage verifies GET /portfolios renders properly.
func TestHandleListPage_RendersCompletePage(t *testing.T) {
	handler, _, _ := setupWebHandlerWithService(t)

	r := httptest.NewRequest(http.MethodGet, "/portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(body, "Portfolios") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "New Portfolio") {
		t.Error("missing 'New Portfolio' link")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestHandleListPage_WithPortfolios shows created portfolios in the list.
func TestHandleListPage_WithPortfolios(t *testing.T) {
	handler, _, repo := setupWebHandlerWithService(t)

	repo.portfolios[1] = &portfolio.Portfolio{ID: 1, Name: "Main", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	r := httptest.NewRequest(http.MethodGet, "/portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Main") {
		t.Error("expected portfolio name in list")
	}
	if !strings.Contains(body, "USD") {
		t.Error("expected portfolio currency in list")
	}
}

// TestHandleDetailPage_RendersCompletePage verifies GET /portfolios/{id} renders properly.
func TestHandleDetailPage_RendersCompletePage(t *testing.T) {
	handler, _, repo := setupWebHandlerWithService(t)

	repo.portfolios[2] = &portfolio.Portfolio{ID: 2, Name: "Detail Test", Currency: "JPY", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "2")
	r := httptest.NewRequest(http.MethodGet, "/portfolios/2", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	w := httptest.NewRecorder()
	handler.HandleDetailPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Detail Test") {
		t.Error("expected portfolio name in detail page")
	}
	if !strings.Contains(body, "JPY") {
		t.Error("expected portfolio currency in detail page")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestHandleDetailPage_NotFound returns 404 for non-existent portfolio.
func TestHandleDetailPage_NotFound(t *testing.T) {
	handler, _, _ := setupWebHandlerWithService(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodGet, "/portfolios/999", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}
