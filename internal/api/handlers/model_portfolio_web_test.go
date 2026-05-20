package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// testSymbolRepoForWeb is a minimal in-memory mock symbol repo for model portfolio web tests.
type testSymbolRepoForWeb struct {
	symbols map[string]*symbolmapping.SymbolMapping
	nextID  int64
}

func newTestSymbolRepoForWeb() *testSymbolRepoForWeb {
	return &testSymbolRepoForWeb{
		symbols: make(map[string]*symbolmapping.SymbolMapping),
		nextID:  1,
	}
}

func (r *testSymbolRepoForWeb) Create(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	r.nextID++
	sm.ID = r.nextID
	r.symbols[sm.InternalSymbol] = sm
	return nil
}

func (r *testSymbolRepoForWeb) GetByID(_ context.Context, id int64) (*symbolmapping.SymbolMapping, error) {
	for _, sm := range r.symbols {
		if sm.ID == id {
			cp := *sm
			return &cp, nil
		}
	}
	return nil, symbolmapping.ErrNotFound
}

func (r *testSymbolRepoForWeb) GetByInternalSymbol(_ context.Context, symbol string) (*symbolmapping.SymbolMapping, error) {
	sm, ok := r.symbols[symbol]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	cp := *sm
	return &cp, nil
}

func (r *testSymbolRepoForWeb) GetByMarketDataSymbol(_ context.Context, symbol string) (*symbolmapping.SymbolMapping, error) {
	for _, sm := range r.symbols {
		if sm.MarketDataSymbol == symbol {
			cp := *sm
			return &cp, nil
		}
	}
	return nil, symbolmapping.ErrNotFound
}

func (r *testSymbolRepoForWeb) GetAll(_ context.Context, limit, offset int) ([]symbolmapping.SymbolMapping, error) {
	var result []symbolmapping.SymbolMapping
	for _, sm := range r.symbols {
		cp := *sm
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

func (r *testSymbolRepoForWeb) Update(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	r.symbols[sm.InternalSymbol] = sm
	return nil
}

func (r *testSymbolRepoForWeb) Delete(_ context.Context, id int64) error {
	for sym, sm := range r.symbols {
		if sm.ID == id {
			delete(r.symbols, sym)
			return nil
		}
	}
	return symbolmapping.ErrNotFound
}

func (r *testSymbolRepoForWeb) AddBrokerSymbol(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func (r *testSymbolRepoForWeb) GetBrokerSymbolByBroker(_ context.Context, _, _ string) (*symbolmapping.BrokerSymbol, error) {
	return nil, symbolmapping.ErrNotFound
}

func (r *testSymbolRepoForWeb) HasReferencingTransactions(_ context.Context, _ int64) (bool, error) {
	return false, nil
}

// testModelPortfolioRepoForWeb is a minimal in-memory mock repo for model portfolio web tests.
type testModelPortfolioRepoForWeb struct {
	portfolios map[int64]*modelportfolio.ModelPortfolio
	names      map[string]int64
	nextID     int64
}

func newTestModelPortfolioRepoForWeb() *testModelPortfolioRepoForWeb {
	return &testModelPortfolioRepoForWeb{
		portfolios: make(map[int64]*modelportfolio.ModelPortfolio),
		names:      make(map[string]int64),
		nextID:     1,
	}
}

func (r *testModelPortfolioRepoForWeb) Create(_ context.Context, mp modelportfolio.ModelPortfolio) (modelportfolio.ModelPortfolio, error) {
	r.nextID++
	mp.ID = r.nextID
	r.portfolios[mp.ID] = &mp
	r.names[mp.Name] = mp.ID
	return mp, nil
}

func (r *testModelPortfolioRepoForWeb) GetByID(_ context.Context, id int64) (modelportfolio.ModelPortfolio, error) {
	mp, ok := r.portfolios[id]
	if !ok {
		return modelportfolio.ModelPortfolio{}, modelportfolio.ErrNotFound
	}
	cp := *mp
	return cp, nil
}

func (r *testModelPortfolioRepoForWeb) GetByName(_ context.Context, name string) (modelportfolio.ModelPortfolio, error) {
	id, ok := r.names[name]
	if !ok {
		return modelportfolio.ModelPortfolio{}, modelportfolio.ErrNotFound
	}
	mp := r.portfolios[id]
	cp := *mp
	return cp, nil
}

func (r *testModelPortfolioRepoForWeb) List(_ context.Context, limit, offset int) ([]modelportfolio.ModelPortfolio, error) {
	var result []modelportfolio.ModelPortfolio
	for _, mp := range r.portfolios {
		cp := *mp
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

func (r *testModelPortfolioRepoForWeb) ListAll(_ context.Context) ([]modelportfolio.ModelPortfolio, error) {
	var result []modelportfolio.ModelPortfolio
	for _, mp := range r.portfolios {
		cp := *mp
		result = append(result, cp)
	}
	return result, nil
}

func (r *testModelPortfolioRepoForWeb) Update(_ context.Context, mp modelportfolio.ModelPortfolio) (modelportfolio.ModelPortfolio, error) {
	oldName := r.portfolios[mp.ID].Name
	r.portfolios[mp.ID] = &mp
	delete(r.names, oldName)
	r.names[mp.Name] = mp.ID
	return mp, nil
}

func (r *testModelPortfolioRepoForWeb) Delete(_ context.Context, id int64) error {
	mp, ok := r.portfolios[id]
	if !ok {
		return modelportfolio.ErrNotFound
	}
	delete(r.names, mp.Name)
	delete(r.portfolios, id)
	return nil
}

func newTestMPRenderer(t *testing.T) *web.Renderer {
	t.Helper()
	renderer, err := web.NewRenderer(findTemplatesDir())
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}
	return renderer
}

func setupMPWebHandler(t *testing.T) (*ModelPortfolioWebHandler, *modelportfolio.Service, *testModelPortfolioRepoForWeb) {
	t.Helper()
	repo := newTestModelPortfolioRepoForWeb()
	svc := modelportfolio.NewService(repo, nil, nil)
	symbolRepo := newTestSymbolRepoForWeb()
	symbolSvc := symbolmapping.NewService(symbolRepo)
	renderer := newTestMPRenderer(t)
	return NewModelPortfolioWebHandler(svc, symbolSvc, renderer), svc, repo
}

// TestModelPortfolioHandleListPage_RendersEmptyState verifies GET /model-portfolios renders with empty state.
func TestModelPortfolioHandleListPage_RendersEmptyState(t *testing.T) {
	handler, _, _ := setupMPWebHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/model-portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(body, "Model Portfolios") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "No model portfolios yet") {
		t.Error("missing empty state message")
	}
	if !strings.Contains(body, "New Model Portfolio") {
		t.Error("missing 'New Model Portfolio' link")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
}

// TestModelPortfolioHandleListPage_WithPortfolios renders created portfolios in the list.
func TestModelPortfolioHandleListPage_WithPortfolios(t *testing.T) {
	handler, _, repo := setupMPWebHandler(t)

	repo.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:   1,
		Name: "60/40 Balanced",
		Entries: []modelportfolio.ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustNew(6000, 2)},
			{Symbol: "BND", WeightPct: decimal.MustNew(4000, 2)},
		},
		CreatedAt: "2025-01-15 10:30",
	}
	repo.names["60/40 Balanced"] = 1

	r := httptest.NewRequest(http.MethodGet, "/model-portfolios", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "60/40 Balanced") {
		t.Error("expected portfolio name in list")
	}
	if !strings.Contains(body, "AAPL") {
		t.Error("expected symbol AAPL in list")
	}
}

// TestModelPortfolioHandleNewPage_RendersCompleteForm verifies GET /model-portfolios/new renders a complete form.
func TestModelPortfolioHandleNewPage_RendersCompleteForm(t *testing.T) {
	handler := setupMPWebHandlerForNew(t)
	r := httptest.NewRequest(http.MethodGet, "/model-portfolios/new", nil)
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

	checkContains(t, "DOCTYPE", "<!DOCTYPE html>")
	checkContains(t, "title", "New Model Portfolio")
	checkContains(t, "form tag", `<form action="/model-portfolios" method="POST"`)
	checkContains(t, "name input", `id="name"`)
	checkContains(t, "name input required", `required`)
	checkContains(t, "symbol datalist", `id="symbol-list"`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "submit text", "Create Model Portfolio")
	checkContains(t, "cancel link", `href="/model-portfolios"`)
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing body", "</body>")
	checkContains(t, "closing html", "</html>")
}

func setupMPWebHandlerForNew(t *testing.T) *ModelPortfolioWebHandler {
	t.Helper()
	repo := newTestModelPortfolioRepoForWeb()
	svc := modelportfolio.NewService(repo, nil, nil)
	symbolRepo := newTestSymbolRepoForWeb()
	symbolRepo.symbols["AAPL"] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	symbolRepo.symbols["BND"] = &symbolmapping.SymbolMapping{ID: 2, InternalSymbol: "BND", MarketDataSymbol: "BND"}
	symbolSvc := symbolmapping.NewService(symbolRepo)
	renderer := newTestMPRenderer(t)
	return NewModelPortfolioWebHandler(svc, symbolSvc, renderer)
}

// TestModelPortfolioHandleCreatePage_ValidSubmission creates a model portfolio via form and verifies redirect.
func TestModelPortfolioHandleCreatePage_ValidSubmission(t *testing.T) {
	handler, _, _ := setupMPWebHandler(t)

	body := strings.NewReader("name=Test+Portfolio&symbol=AAPL&weight=60.00&symbol=BND&weight=40.00")
	r := httptest.NewRequest(http.MethodPost, "/model-portfolios", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/model-portfolios" {
		t.Errorf("expected redirect to /model-portfolios, got %q", location)
	}
}

// TestModelPortfolioHandleCreatePage_EmptyName shows validation error on the form.
func TestModelPortfolioHandleCreatePage_EmptyName(t *testing.T) {
	handler := setupMPWebHandlerForNew(t)
	body := strings.NewReader("name=&symbol=AAPL&weight=100.00")
	r := httptest.NewRequest(http.MethodPost, "/model-portfolios", body)
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

// TestModelPortfolioHandleCreatePage_InvalidWeightSum shows validation error on the form.
func TestModelPortfolioHandleCreatePage_InvalidWeightSum(t *testing.T) {
	handler := setupMPWebHandlerForNew(t)
	body := strings.NewReader("name=Bad+Weights&symbol=AAPL&weight=50.00&symbol=BND&weight=30.00")
	r := httptest.NewRequest(http.MethodPost, "/model-portfolios", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()
	if !strings.Contains(pageBody, "100") {
		t.Error("expected weight sum error message mentioning 100%")
	}
}

// TestModelPortfolioHandleCreatePage_DuplicateName shows conflict error on the form.
func TestModelPortfolioHandleCreatePage_DuplicateName(t *testing.T) {
	handler, _, repo := setupMPWebHandler(t)

	repo.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:      1,
		Name:    "Existing",
		Entries: []modelportfolio.ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)}},
	}
	repo.names["Existing"] = 1

	body := strings.NewReader("name=Existing&symbol=AAPL&weight=100.00")
	r := httptest.NewRequest(http.MethodPost, "/model-portfolios", body)
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

// TestModelPortfolioHandleEditPage_RendersCompleteForm verifies GET /model-portfolios/{id}/edit renders with pre-filled values.
func TestModelPortfolioHandleEditPage_RendersCompleteForm(t *testing.T) {
	handler, _, repo := setupMPWebHandler(t)

	repo.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:   1,
		Name: "My Model",
		Entries: []modelportfolio.ModelPortfolioEntry{
			{Symbol: "AAPL", WeightPct: decimal.MustNew(6000, 2)},
			{Symbol: "BND", WeightPct: decimal.MustNew(4000, 2)},
		},
	}
	repo.names["My Model"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/model-portfolios/1/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	w := httptest.NewRecorder()
	handler.HandleEditPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	checkContains := func(t *testing.T, label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("edit page missing %s: %q", label, text)
		}
	}

	checkContains(t, "title", "Edit Model Portfolio")
	checkContains(t, "pre-filled name", `value="My Model"`)
	checkContains(t, "submit text", "Save Changes")
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing html", "</html>")
}

// TestModelPortfolioHandleEditPage_NotFound returns 404 for non-existent portfolio.
func TestModelPortfolioHandleEditPage_NotFound(t *testing.T) {
	handler, _, _ := setupMPWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodGet, "/model-portfolios/999/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleEditPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

// TestModelPortfolioHandleEditPost_ValidSubmission updates a model portfolio and redirects.
func TestModelPortfolioHandleEditPost_ValidSubmission(t *testing.T) {
	handler, _, repo := setupMPWebHandler(t)

	repo.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:      1,
		Name:    "Old Name",
		Entries: []modelportfolio.ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)}},
	}
	repo.names["Old Name"] = 1

	body := strings.NewReader("name=New+Name&symbol=AAPL&weight=60.00&symbol=BND&weight=40.00")
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodPost, "/model-portfolios/1/edit", body)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleEditPost(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}
}

// TestModelPortfolioHandleDeletePage_Success redirects after successful delete.
func TestModelPortfolioHandleDeletePage_Success(t *testing.T) {
	handler, _, repo := setupMPWebHandler(t)

	repo.portfolios[1] = &modelportfolio.ModelPortfolio{
		ID:      1,
		Name:    "To Delete",
		Entries: []modelportfolio.ModelPortfolioEntry{{Symbol: "AAPL", WeightPct: decimal.MustNew(10000, 2)}},
	}
	repo.names["To Delete"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodPost, "/model-portfolios/1/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/model-portfolios" {
		t.Errorf("expected redirect to /model-portfolios, got %q", location)
	}
}

// TestModelPortfolioHandleDeletePage_NotFound returns 404 for non-existent portfolio.
func TestModelPortfolioHandleDeletePage_NotFound(t *testing.T) {
	handler, _, _ := setupMPWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodPost, "/model-portfolios/999/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

// TestModelPortfolioUserFriendlyError maps domain errors to user-friendly messages.
func TestModelPortfolioUserFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"invalid name", modelportfolio.ErrInvalidName, "Invalid name"},
		{"name exists", modelportfolio.ErrNameExists, "already exists"},
		{"empty entries", modelportfolio.ErrEmptyEntries, "At least one entry"},
		{"invalid weight", modelportfolio.ErrInvalidWeight, "greater than 0"},
		{"duplicate symbol", modelportfolio.ErrDuplicateSymbol, "duplicate symbols"},
		{"unknown error", modelportfolio.ErrNotFound, "not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := modelPortfolioUserFriendlyError(tt.err)
			if !strings.Contains(got, tt.want) {
				t.Errorf("modelPortfolioUserFriendlyError() = %q, want contains %q", got, tt.want)
			}
		})
	}
}
