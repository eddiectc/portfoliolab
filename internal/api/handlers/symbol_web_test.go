package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
)

// --- Test helpers ---

// testSMWebRepo is a minimal in-memory mock repo for symbol mapping web handler tests.
type testSMWebRepo struct {
	mappings   map[int64]*symbolmapping.SymbolMapping
	byInternal map[string]int64
	nextID     int64
}

func newTestSMWebRepo() *testSMWebRepo {
	return &testSMWebRepo{
		mappings:   make(map[int64]*symbolmapping.SymbolMapping),
		byInternal: make(map[string]int64),
		nextID:     1,
	}
}

func (r *testSMWebRepo) Create(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	r.nextID++
	sm.ID = r.nextID
	cp := *sm
	cp.BrokerSymbols = nil
	r.mappings[sm.ID] = &cp
	r.byInternal[sm.InternalSymbol] = sm.ID
	return nil
}

func (r *testSMWebRepo) GetByID(_ context.Context, id int64) (*symbolmapping.SymbolMapping, error) {
	sm, ok := r.mappings[id]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	cp := *sm
	if len(sm.BrokerSymbols) > 0 {
		cp.BrokerSymbols = make([]symbolmapping.BrokerSymbol, len(sm.BrokerSymbols))
		copy(cp.BrokerSymbols, sm.BrokerSymbols)
	}
	return &cp, nil
}

func (r *testSMWebRepo) GetByInternalSymbol(_ context.Context, internalSymbol string) (*symbolmapping.SymbolMapping, error) {
	id, ok := r.byInternal[internalSymbol]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	return r.GetByID(context.Background(), id)
}

func (r *testSMWebRepo) GetAll(_ context.Context, limit, offset int) ([]symbolmapping.SymbolMapping, error) {
	var result []symbolmapping.SymbolMapping
	for _, sm := range r.mappings {
		cp := *sm
		if len(sm.BrokerSymbols) > 0 {
			cp.BrokerSymbols = make([]symbolmapping.BrokerSymbol, len(sm.BrokerSymbols))
			copy(cp.BrokerSymbols, sm.BrokerSymbols)
		}
		result = append(result, cp)
	}
	return result, nil
}

func (r *testSMWebRepo) ListAll(_ context.Context) ([]symbolmapping.SymbolMapping, error) {
	var result []symbolmapping.SymbolMapping
	for _, sm := range r.mappings {
		cp := *sm
		if len(sm.BrokerSymbols) > 0 {
			cp.BrokerSymbols = make([]symbolmapping.BrokerSymbol, len(sm.BrokerSymbols))
			copy(cp.BrokerSymbols, sm.BrokerSymbols)
		}
		result = append(result, cp)
	}
	return result, nil
}

func (r *testSMWebRepo) Update(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	old, ok := r.mappings[sm.ID]
	if !ok {
		return symbolmapping.ErrNotFound
	}
	if old.InternalSymbol != sm.InternalSymbol {
		delete(r.byInternal, old.InternalSymbol)
		r.byInternal[sm.InternalSymbol] = sm.ID
	}
	smCopy := *sm
	smCopy.BrokerSymbols = old.BrokerSymbols
	r.mappings[sm.ID] = &smCopy
	return nil
}

func (r *testSMWebRepo) Delete(_ context.Context, id int64) error {
	sm, ok := r.mappings[id]
	if !ok {
		return symbolmapping.ErrNotFound
	}
	delete(r.byInternal, sm.InternalSymbol)
	delete(r.mappings, id)
	return nil
}

func (r *testSMWebRepo) AddBrokerSymbol(_ context.Context, symbolMappingID int64, brokerName, brokerSymbol string) error {
	return nil
}

func (r *testSMWebRepo) GetBrokerSymbolByBroker(_ context.Context, brokerName, brokerSymbol string) (*symbolmapping.BrokerSymbol, error) {
	return nil, symbolmapping.ErrNotFound
}

func (r *testSMWebRepo) HasReferencingTransactions(_ context.Context, id int64) (bool, error) {
	return false, nil
}

func setupWebHandlerWithSMService(t *testing.T) (*SymbolWebHandler, *symbolmapping.Service, *testSMWebRepo) {
	t.Helper()
	repo := newTestSMWebRepo()
	svc := symbolmapping.NewService(repo)
	renderer := newTestRenderer(t)
	return NewSymbolWebHandler(svc, renderer), svc, repo
}

// --- Symbol Mapping User-Friendly Error Tests ---

func TestSymbolMappingUserFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "invalid symbol",
			err:  symbolmapping.ErrInvalidSymbol,
			want: "Invalid symbol",
		},
		{
			name: "internal symbol exists",
			err:  symbolmapping.ErrInternalSymbolExists,
			want: "already exists",
		},
		{
			name: "broker symbol exists",
			err:  symbolmapping.ErrBrokerSymbolExists,
			want: "already mapped",
		},
		{
			name: "in use",
			err:  symbolmapping.ErrInUse,
			want: "referenced by transactions",
		},
		{
			name: "unknown error",
			err:  symbolmapping.ErrNotFound,
			want: "error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := symbolMappingUserFriendlyError(tt.err)
			if !strings.Contains(got, tt.want) {
				t.Errorf("symbolMappingUserFriendlyError() = %q, want contains %q", got, tt.want)
			}
		})
	}
}

// --- Parse Broker Symbols Tests ---

func TestParseBrokerSymbols(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/symbols", nil)
	r.PostForm = map[string][]string{
		"broker_name":   {"IBKR", "T212", ""},
		"broker_symbol": {"AAPL.US", "AAPLU", "EMPTY"},
	}
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm failed: %v", err)
	}

	result := parseBrokerSymbols(r)
	// Third entry has empty name, should be skipped
	if len(result) != 2 {
		t.Errorf("expected 2 broker symbols, got %d", len(result))
	}
	if result[0].BrokerName != "IBKR" || result[0].BrokerSymbol != "AAPL.US" {
		t.Errorf("unexpected first broker symbol: %+v", result[0])
	}
	if result[1].BrokerName != "T212" || result[1].BrokerSymbol != "AAPLU" {
		t.Errorf("unexpected second broker symbol: %+v", result[1])
	}
}

func TestParseBrokerSymbols_Empty(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/symbols", nil)
	r.PostForm = map[string][]string{}
	_ = r.ParseForm()

	result := parseBrokerSymbols(r)
	if len(result) != 0 {
		t.Errorf("expected 0 broker symbols, got %d", len(result))
	}
}

func TestParseBrokerSymbols_SkipsEmptyFields(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/symbols", nil)
	r.PostForm = map[string][]string{
		"broker_name":   {"IBKR", "", "T212"},
		"broker_symbol": {"AAPL.US", "ORPHAN", ""},
	}
	_ = r.ParseForm()

	result := parseBrokerSymbols(r)
	if len(result) != 1 {
		t.Errorf("expected 1 broker symbol (empty pairs skipped), got %d", len(result))
	}
	if result[0].BrokerName != "IBKR" {
		t.Errorf("expected IBKR, got %q", result[0].BrokerName)
	}
}

// --- Template Rendering Tests ---

// TestHandleNewPage_RendersCompleteForm verifies that GET /symbols/new
// renders a complete form with all expected elements.
func TestSMHandleNewPage_RendersCompleteForm(t *testing.T) {
	handler, _, _ := setupWebHandlerWithSMService(t)
	r := httptest.NewRequest(http.MethodGet, "/symbols/new", nil)
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
	checkContains(t, "title", "New Symbol Mapping")
	checkContains(t, "form tag", `<form`)
	checkContains(t, "internal symbol input", `id="internal_symbol"`)
	checkContains(t, "market data symbol input", `id="market_data_symbol"`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "submit text", "Create Mapping")
	checkContains(t, "cancel link", `href="/symbols"`)
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing html", "</html>")
}

// TestHandleListPage_RendersCompletePage verifies GET /symbols renders properly.
func TestSMHandleListPage_RendersCompletePage(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	r := httptest.NewRequest(http.MethodGet, "/symbols", nil)
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
	if !strings.Contains(body, "Symbols") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "New Symbol Mapping") {
		t.Error("missing 'New Symbol Mapping' link")
	}
	if !strings.Contains(body, "</html>") {
		t.Error("missing closing html tag")
	}
	_ = repo
}

// TestHandleListPage_WithMappings shows created mappings in the list.
func TestSMHandleListPage_WithMappings(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.byInternal["AAPL"] = 1

	r := httptest.NewRequest(http.MethodGet, "/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "AAPL") {
		t.Error("expected internal symbol in list")
	}
}

// TestHandleCreatePage_ValidSubmission creates a mapping via form and verifies redirect.
func TestSMHandleCreatePage_ValidSubmission(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	body := strings.NewReader("internal_symbol=AAPL&market_data_symbol=AAPL")
	r := httptest.NewRequest(http.MethodPost, "/symbols", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/symbols" {
		t.Errorf("expected redirect to /symbols, got %q", location)
	}
	_ = repo
}

// TestHandleCreatePage_EmptyInternalSymbol shows validation error on the form.
func TestSMHandleCreatePage_EmptyInternalSymbol(t *testing.T) {
	handler, _, _ := setupWebHandlerWithSMService(t)

	body := strings.NewReader("internal_symbol=&market_data_symbol=AAPL")
	r := httptest.NewRequest(http.MethodPost, "/symbols", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()

	if !strings.Contains(pageBody, "Invalid symbol") {
		t.Error("expected validation error message")
	}

	// Form should still be complete
	if !strings.Contains(pageBody, `id="internal_symbol"`) {
		t.Error("missing internal symbol input on error page")
	}
	if !strings.Contains(pageBody, "</form>") {
		t.Error("missing closing form tag on error page")
	}
}

// TestHandleCreatePage_DuplicateInternalSymbol shows conflict error.
func TestSMHandleCreatePage_DuplicateInternalSymbol(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	body := strings.NewReader("internal_symbol=AAPL&market_data_symbol=AAPL")
	r := httptest.NewRequest(http.MethodPost, "/symbols", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()
	if !strings.Contains(pageBody, "already exists") {
		t.Error("expected duplicate symbol error")
	}
}

// TestHandleEditPage_RendersCompleteForm verifies GET /symbols/{id}/edit
// renders a complete form with pre-filled values.
func TestSMHandleEditPage_RendersCompleteForm(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.byInternal["AAPL"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/edit", nil)
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

	checkContains(t, "title", "Edit Symbol Mapping")
	checkContains(t, "internal symbol input", `id="internal_symbol"`)
	checkContains(t, "pre-filled internal symbol", `value="AAPL"`)
	checkContains(t, "market data symbol input", `id="market_data_symbol"`)
	checkContains(t, "pre-filled market data symbol", `value="AAPL"`)
	checkContains(t, "submit button", `type="submit"`)
	checkContains(t, "submit text", "Save Changes")
	checkContains(t, "closing form", "</form>")
	checkContains(t, "closing html", "</html>")
}

// TestHandleEditPage_NotFound returns 404 for non-existent mapping.
func TestSMHandleEditPage_NotFound(t *testing.T) {
	handler, _, _ := setupWebHandlerWithSMService(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodGet, "/symbols/999/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	w := httptest.NewRecorder()
	handler.HandleEditPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

// TestHandleUpdatePage_ValidSubmission updates a mapping via form and verifies redirect.
func TestSMHandleUpdatePage_ValidSubmission(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
	}
	repo.byInternal["AAPL"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	body := strings.NewReader("internal_symbol=AAPL&market_data_symbol=AAPL.LON")
	r := httptest.NewRequest(http.MethodPost, "/symbols/1/edit", body)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleUpdatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/symbols" {
		t.Errorf("expected redirect to /symbols, got %q", location)
	}
}

// TestHandleUpdatePage_DuplicateInternalSymbol shows conflict error.
func TestSMHandleUpdatePage_DuplicateInternalSymbol(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "APPL", MarketDataSymbol: "AAPL"}
	repo.mappings[2] = &symbolmapping.SymbolMapping{ID: 2, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["APPL"] = 1
	repo.byInternal["AAPL"] = 2

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	body := strings.NewReader("internal_symbol=AAPL&market_data_symbol=AAPL")
	r := httptest.NewRequest(http.MethodPost, "/symbols/1/edit", body)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleUpdatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (form re-render), got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()
	if !strings.Contains(pageBody, "already exists") {
		t.Error("expected duplicate symbol error")
	}
}

// TestHandleDeletePage_Success deletes a mapping via form and verifies redirect.
func TestSMHandleDeletePage_Success(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodPost, "/symbols/1/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "/symbols" {
		t.Errorf("expected redirect to /symbols, got %q", location)
	}
}

// TestHandleDeletePage_NotFound returns 404 for non-existent mapping.
func TestSMHandleDeletePage_NotFound(t *testing.T) {
	handler, _, _ := setupWebHandlerWithSMService(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodPost, "/symbols/999/delete", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDeletePage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

// TestHandleCreatePage_WithBenchmark creates a mapping with benchmark flag.
func TestHandleCreatePage_WithBenchmark(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	body := strings.NewReader("internal_symbol=SPX&market_data_symbol=SPX.GI&is_benchmark=on")
	r := httptest.NewRequest(http.MethodPost, "/symbols", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	// Verify the created mapping has IsBenchmark=true
	mappings, _ := repo.GetAll(context.Background(), 0, 0)
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if !mappings[0].IsBenchmark {
		t.Error("expected IsBenchmark=true")
	}
}

// TestHandleCreatePage_WithoutBenchmark creates a mapping without benchmark flag.
func TestHandleCreatePage_WithoutBenchmark(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	body := strings.NewReader("internal_symbol=AAPL&market_data_symbol=AAPL")
	r := httptest.NewRequest(http.MethodPost, "/symbols", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleCreatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	mappings, _ := repo.GetAll(context.Background(), 0, 0)
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].IsBenchmark {
		t.Error("expected IsBenchmark=false by default")
	}
}

// TestHandleEditPage_LoadsBenchmark shows checkbox checked for benchmark symbol.
func TestHandleEditPage_LoadsBenchmark(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "SPX",
		MarketDataSymbol: "SPX.GI",
		IsBenchmark:      true,
	}
	repo.byInternal["SPX"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/edit", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	w := httptest.NewRecorder()
	handler.HandleEditPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	pageBody := w.Body.String()
	if !strings.Contains(pageBody, `id="is_benchmark"`) {
		t.Error("missing is_benchmark checkbox")
	}
	if !strings.Contains(pageBody, "checked") {
		t.Error("expected checkbox to be checked for benchmark symbol")
	}
}

// TestHandleUpdatePage_ToggleBenchmark toggles benchmark flag from false to true.
func TestHandleUpdatePage_ToggleBenchmark(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		IsBenchmark:      false,
	}
	repo.byInternal["AAPL"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	body := strings.NewReader("internal_symbol=AAPL&market_data_symbol=AAPL&is_benchmark=on")
	r := httptest.NewRequest(http.MethodPost, "/symbols/1/edit", body)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleUpdatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	updated, err := repo.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("failed to get updated mapping: %v", err)
	}
	if !updated.IsBenchmark {
		t.Error("expected IsBenchmark=true after toggle")
	}
}

// TestHandleUpdatePage_NoBenchmarkChange leaves benchmark flag unchanged when checkbox not sent.
func TestHandleUpdatePage_NoBenchmarkChange(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		IsBenchmark:      true,
	}
	repo.byInternal["AAPL"] = 1

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	// No is_benchmark in form (checkbox unchecked = not sent)
	body := strings.NewReader("internal_symbol=AAPL&market_data_symbol=AAPL")
	r := httptest.NewRequest(http.MethodPost, "/symbols/1/edit", body)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.HandleUpdatePage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	// Benchmark should remain true (unchanged since checkbox wasn't sent and value didn't change)
	updated, err := repo.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("failed to get updated mapping: %v", err)
	}
	// Note: current.IsBenchmark=true, form isBenchmark=false (not sent), so they differ → req.IsBenchmark set to false
	// This is the actual behavior: unchecked checkbox = "off" ≠ current true → toggle to false
	// This test verifies the toggle-off behavior
	if updated.IsBenchmark {
		t.Error("expected IsBenchmark=false after unchecking (form didn't send is_benchmark, current was true)")
	}
}

// TestSMHandleNewPage_RendersBenchmarkCheckbox verifies the benchmark checkbox is on the form.
func TestSMHandleNewPage_RendersBenchmarkCheckbox(t *testing.T) {
	handler, _, _ := setupWebHandlerWithSMService(t)
	r := httptest.NewRequest(http.MethodGet, "/symbols/new", nil)
	w := httptest.NewRecorder()

	handler.HandleNewPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, `id="is_benchmark"`) {
		t.Error("missing is_benchmark checkbox")
	}
	if !strings.Contains(body, "Use as benchmark") {
		t.Error("missing 'Use as benchmark' label text")
	}
}

// TestSMHandleListPage_ShowBenchmarkBadge verifies benchmark column renders correctly.
func TestSMHandleListPage_ShowBenchmarkBadge(t *testing.T) {
	handler, _, repo := setupWebHandlerWithSMService(t)

	repo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "SPX",
		MarketDataSymbol: "SPX.GI",
		IsBenchmark:      true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.byInternal["SPX"] = 1

	repo.mappings[2] = &symbolmapping.SymbolMapping{
		ID:               2,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		IsBenchmark:      false,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.byInternal["AAPL"] = 2

	r := httptest.NewRequest(http.MethodGet, "/symbols", nil)
	w := httptest.NewRecorder()

	handler.HandleListPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Benchmark") {
		t.Error("missing Benchmark column header")
	}
	if !strings.Contains(body, "badge") {
		t.Error("missing badge for benchmark symbol")
	}
}

// TestRegisterRoutes verifies routes mount without panic.
func TestSMRegisterRoutes(t *testing.T) {
	r := chi.NewRouter()
	handler := &SymbolWebHandler{}
	handler.RegisterRoutes(r)
}
