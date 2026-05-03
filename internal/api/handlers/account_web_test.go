package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/account"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
)

// mockAccountRepo is an in-memory repository for testing account web handlers.
type mockAccountRepo struct {
	accounts    map[int64]*account.Account
	names       map[string]int64
	byPortfolio map[int64][]int64
	nextID      int64
}

func newMockAccountRepo() *mockAccountRepo {
	return &mockAccountRepo{
		accounts:    make(map[int64]*account.Account),
		names:       make(map[string]int64),
		byPortfolio: make(map[int64][]int64),
		nextID:      1,
	}
}

func (m *mockAccountRepo) Create(_ context.Context, a *account.Account) error {
	m.nextID++
	a.ID = m.nextID
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = now
	}
	m.accounts[a.ID] = a
	m.names[a.Name] = a.ID
	m.byPortfolio[a.PortfolioID] = append(m.byPortfolio[a.PortfolioID], a.ID)
	return nil
}

func (m *mockAccountRepo) GetByID(_ context.Context, id int64) (*account.Account, error) {
	a, ok := m.accounts[id]
	if !ok {
		return nil, account.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (m *mockAccountRepo) GetAll(_ context.Context, _, _ int) ([]account.Account, error) {
	result := make([]account.Account, 0, len(m.accounts))
	for _, a := range m.accounts {
		cp := *a
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockAccountRepo) GetByPortfolio(_ context.Context, portfolioID int64, _, _ int) ([]account.Account, error) {
	ids := m.byPortfolio[portfolioID]
	result := make([]account.Account, 0, len(ids))
	for _, id := range ids {
		a := m.accounts[id]
		cp := *a
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockAccountRepo) GetByName(_ context.Context, name string) (*account.Account, error) {
	id, ok := m.names[name]
	if !ok {
		return nil, account.ErrNotFound
	}
	a := m.accounts[id]
	cp := *a
	return &cp, nil
}

func (m *mockAccountRepo) Update(_ context.Context, a *account.Account) error {
	m.accounts[a.ID] = a
	return nil
}

func (m *mockAccountRepo) Delete(_ context.Context, id int64) error {
	a, ok := m.accounts[id]
	if !ok {
		return account.ErrNotFound
	}
	delete(m.accounts, id)
	delete(m.names, a.Name)
	return nil
}

// mockPortfolioCheckerForWeb always says portfolio 1 exists.
type mockPortfolioCheckerForWeb struct{}

func (m *mockPortfolioCheckerForWeb) PortfolioExists(_ context.Context, id int64) bool {
	return id > 0
}

func newAccountWebHandler(t *testing.T) (*AccountWebHandler, *mockAccountRepo) {
	t.Helper()
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)

	renderer := newTestRenderer(t)
	return NewAccountWebHandler(accountSvc, portfolioSvc, renderer), accountRepo
}

// TestAccountHandleListPage_RendersCompletePage verifies GET /accounts renders properly.
func TestAccountHandleListPage_RendersCompletePage(t *testing.T) {
	handler, _ := newAccountWebHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/accounts", nil)
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
	if !strings.Contains(body, "Accounts") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "New Account") {
		t.Error("missing 'New Account' link")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestAccountHandleListPage_WithAccounts shows created accounts in the list.
func TestAccountHandleListPage_WithAccounts(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioSvc.Create(nil, portfolio.CreateRequest{Name: "Main", Currency: "USD"})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 2})

	r := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "IBKR") {
		t.Error("expected account name in list")
	}
	if !strings.Contains(body, "Main") {
		t.Error("expected portfolio name in list")
	}
}

// TestAccountHandleNewPage_RendersCompleteForm verifies GET /accounts/new renders a complete form.
func TestAccountHandleNewPage_RendersCompleteForm(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioSvc.Create(nil, portfolio.CreateRequest{Name: "Main", Currency: "USD"})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	r := httptest.NewRequest(http.MethodGet, "/accounts/new", nil)
	w := httptest.NewRecorder()

	handler.HandleNewPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	checkContains := func(t *testing.T, label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	checkContains(t, "title", "New Account")
	checkContains(t, "form tag", `<form action="/accounts" method="POST"`)
	checkContains(t, "name input", `id="name"`)
	checkContains(t, "portfolio select", `id="portfolio_id"`)
	checkContains(t, "portfolio option", `value="2"`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "submit text", "Create Account")
	checkContains(t, "cancel link", `href="/accounts"`)
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing html", "</html>")
}

// TestAccountHandleCreatePage_ValidSubmission creates an account via form and verifies redirect.
func TestAccountHandleCreatePage_ValidSubmission(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioSvc.Create(nil, portfolio.CreateRequest{Name: "Main", Currency: "USD"})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	body := strings.NewReader("name=IBKR&portfolio_id=2")
	r := httptest.NewRequest(http.MethodPost, "/accounts", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/portfolios/2" {
		t.Errorf("expected redirect to /portfolios/2, got %q", location)
	}
}

// TestAccountHandleCreatePage_EmptyName shows validation error on the form.
func TestAccountHandleCreatePage_EmptyName(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioSvc.Create(nil, portfolio.CreateRequest{Name: "Main", Currency: "USD"})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	body := strings.NewReader("name=&portfolio_id=2")
	r := httptest.NewRequest(http.MethodPost, "/accounts", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()
	if !strings.Contains(pageBody, "Invalid name") {
		t.Error("expected validation error message")
	}
}

// TestAccountHandleCreatePage_DuplicateName shows conflict error on the form.
func TestAccountHandleCreatePage_DuplicateName(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioSvc.Create(nil, portfolio.CreateRequest{Name: "Main", Currency: "USD"})

	accountSvc.Create(nil, account.CreateRequest{Name: "Existing", PortfolioID: 2})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	body := strings.NewReader("name=Existing&portfolio_id=2")
	r := httptest.NewRequest(http.MethodPost, "/accounts", body)
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

// TestAccountHandleDetailPage_RendersCompletePage verifies GET /accounts/{id} renders properly.
func TestAccountHandleDetailPage_RendersCompletePage(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioSvc.Create(nil, portfolio.CreateRequest{Name: "Main", Currency: "USD"})

	accountSvc.Create(nil, account.CreateRequest{Name: "Fidelity", PortfolioID: 2})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "2")
	r := httptest.NewRequest(http.MethodGet, "/accounts/2", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	w := httptest.NewRecorder()
	handler.HandleDetailPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Fidelity") {
		t.Error("expected account name in detail page")
	}
	if !strings.Contains(body, "Main") {
		t.Error("expected portfolio name in detail page")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestAccountHandleDetailPage_NotFound returns 404 for non-existent account.
func TestAccountHandleDetailPage_NotFound(t *testing.T) {
	handler, _ := newAccountWebHandler(t)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodGet, "/accounts/999", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

// TestAccountHandleEditPage_RendersCompleteForm verifies GET /accounts/{id}/edit
// renders a complete form with pre-filled values.
func TestAccountHandleEditPage_RendersCompleteForm(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioSvc.Create(nil, portfolio.CreateRequest{Name: "Main", Currency: "USD"})

	accountSvc.Create(nil, account.CreateRequest{Name: "Vanguard", PortfolioID: 2})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "2")
	r := httptest.NewRequest(http.MethodGet, "/accounts/2/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	w := httptest.NewRecorder()
	handler.HandleEditPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()

	checkContains := func(t *testing.T, label, text string) {
		t.Helper()
		if !strings.Contains(pageBody, text) {
			t.Errorf("edit page missing %s: %q", label, text)
		}
	}

	checkContains(t, "title", "Edit Account")
	checkContains(t, "name input", `id="name"`)
	checkContains(t, "pre-filled name", `value="Vanguard"`)
	checkContains(t, "portfolio hidden input", `name="portfolio_id"`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "submit text", "Save Changes")
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing html", "</html>")
}

// TestAccountHandleDeletePage_Success verifies POST /accounts/{id}/delete redirects.
func TestAccountHandleDeletePage_Success(t *testing.T) {
	accountRepo := newMockAccountRepo()
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})

	portfolioRepo := newMockRepo()
	portfolioSvc := portfolio.NewService(portfolioRepo)

	accountSvc.Create(nil, account.CreateRequest{Name: "ToDelete", PortfolioID: 1})

	renderer := newTestRenderer(t)
	handler := NewAccountWebHandler(accountSvc, portfolioSvc, renderer)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "2")
	r := httptest.NewRequest(http.MethodPost, "/accounts/2/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/accounts" {
		t.Errorf("expected redirect to /accounts, got %q", location)
	}
}

// TestAccountHandleDeletePage_NotFound returns 404 for non-existent account.
func TestAccountHandleDeletePage_NotFound(t *testing.T) {
	handler, _ := newAccountWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodPost, "/accounts/999/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}
