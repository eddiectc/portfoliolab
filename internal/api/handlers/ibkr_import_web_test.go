package handlers

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/ibkrimport"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
)

// ---- Setup ----

func setupImportWebHandler(t *testing.T) (*ImportWebHandler, *mockImportService, *mockAccountRepoForTx, *mockAccountChecker, *mockSymbolRepoForImport) {
	t.Helper()

	importSvc := newMockImportService()
	accountRepo := newMockAccountRepoForTx()
	accountChecker := &mockAccountChecker{}
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	symbolRepo := newMockSymbolRepoForImport()
	symbolSvc := symbolmapping.NewService(symbolRepo)
	renderer := newTestRenderer(t)

	return NewImportWebHandler(importSvc, accountSvc, symbolSvc, renderer), importSvc, accountRepo, accountChecker, symbolRepo
}

// mockAccountChecker always says accounts 1-3 exist.
type mockAccountChecker struct{}

func (m *mockAccountChecker) AccountExists(_ context.Context, id int64) bool {
	return id >= 1 && id <= 10
}

// buildImportMultipartForm creates a multipart form body with an XML file and account_id.
func buildImportMultipartForm(xmlContent, accountID string) (body *bytes.Buffer, contentType string) {
	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("xml_file", "report.xml")
	part.Write([]byte(xmlContent))
	writer.WriteField("account_id", accountID)
	writer.Close()
	return body, writer.FormDataContentType()
}

// ---- HandleImportPage tests ----

func TestImportWebHandleImportPage_RendersUploadPage(t *testing.T) {
	handler, _, accountRepo, _, _ := setupImportWebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	r := httptest.NewRequest(http.MethodGet, "/transactions/import/ibkr", nil)
	w := httptest.NewRecorder()

	handler.HandleImportPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	checkContains("title", "Import IBKR Flex XML")
	checkContains("account select", `id="account_id"`)
	checkContains("file input", `id="xml_file"`)
	checkContains("submit button", "Preview Import")
	checkContains("supported types", "Supported Transaction Types")
	checkContains("trades info", "Stocks")
	checkContains("cash transactions info", "Dividends")
	checkContains("back link", "Back to Transactions")
	checkContains("closing html", "</html>")
}

func TestImportWebHandleImportPage_NoAccounts(t *testing.T) {
	handler, _, _, _, _ := setupImportWebHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/transactions/import/ibkr", nil)
	w := httptest.NewRecorder()

	handler.HandleImportPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Import IBKR Flex XML") {
		t.Error("missing title")
	}
}

// ---- HandleImportPost tests ----

func TestImportWebHandleImportPost_ValidXML(t *testing.T) {
	handler, importSvc, accountRepo, _, _ := setupImportWebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	preview := &ibkrimport.PreviewResponse{
		Importable: []ibkrimport.PreviewTransaction{
			{Date: "20240115", Type: "buy", Symbol: "AAPL", Quantity: "10", Price: "150.00", Currency: "USD", NetCash: "1500.00", ExternalReference: "T001", Description: "Bought 10 AAPL"},
		},
		Skipped: []ibkrimport.SkippedTransaction{
			{ExternalReference: "T002", Reason: "unmapped symbol: BROKERXYZ", BrokerSymbol: "BROKERXYZ"},
		},
		ImportableCount: 1,
		SkippedCount:    1,
		Errored:         []ibkrimport.ErroredTransaction{},
	}
	importSvc.WithPreview(preview, nil)

	body, contentType := buildImportMultipartForm(validXML(), "1")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr", body)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleImportPost(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	resp := w.Body.String()
	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(resp, text) {
			t.Errorf("preview page missing %s: %q", label, text)
		}
	}

	checkContains("title", "Import Preview")
	checkContains("importable count", "1")
	checkContains("skipped count", "1")
	checkContains("unified table", "import-table")
	checkContains("confirm button", "Confirm Import")
	checkContains("xml data input", `id="xml-data-input"`)
}

func TestImportWebHandleImportPost_UnifiedTableAndSymbols(t *testing.T) {
	handler, importSvc, accountRepo, _, symbolRepo := setupImportWebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	// Pre-populate existing symbols in the symbol repo
	symbolRepo.Create(nil, &symbolmapping.SymbolMapping{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"})
	symbolRepo.Create(nil, &symbolmapping.SymbolMapping{InternalSymbol: "MSFT", MarketDataSymbol: "MSFT"})

	// Multiple skipped with same/different broker symbols + one without broker symbol
	preview := &ibkrimport.PreviewResponse{
		Importable: []ibkrimport.PreviewTransaction{
			{Date: "20240115", Type: "buy", Symbol: "AAPL", Quantity: "10", Price: "150.00", Currency: "USD", NetCash: "1500.00", ExternalReference: "T005", Description: "Bought 10 AAPL"},
		},
		Skipped: []ibkrimport.SkippedTransaction{
			{ExternalReference: "T001", Reason: "unmapped symbol: STTHY", BrokerSymbol: "STTHY", Date: "20240115", Type: "buy", Symbol: "STTHY", Quantity: "100", Price: "50.00", Currency: "USD", NetCash: "5000.00", Description: "STTHY buy"},
			{ExternalReference: "T002", Reason: "unmapped symbol: STTHY", BrokerSymbol: "STTHY", Date: "20240116", Type: "sell", Symbol: "STTHY", Quantity: "50", Price: "52.00", Currency: "USD", NetCash: "2600.00", Description: "STTHY sell"},
			{ExternalReference: "T003", Reason: "unmapped symbol: AAPL", BrokerSymbol: "AAPL", Date: "20240117", Type: "buy", Symbol: "AAPL", Quantity: "5", Price: "160.00", Currency: "USD", NetCash: "800.00", Description: "AAPL buy"},
			{ExternalReference: "T004", Reason: "duplicate transaction", Date: "20240118", Type: "buy", Symbol: "MSFT", Quantity: "20", Price: "300.00", Currency: "USD", NetCash: "6000.00", Description: "MSFT buy"},
		},
		ImportableCount: 1,
		SkippedCount:    4,
		Errored:         []ibkrimport.ErroredTransaction{},
	}
	importSvc.WithPreview(preview, nil)

	body, contentType := buildImportMultipartForm(validXML(), "1")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr", body)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleImportPost(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	resp := w.Body.String()
	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(resp, text) {
			t.Errorf("preview page missing %s: %q", label, text)
		}
	}

	// Verify unified table renders all skipped rows individually
	checkContains("skipped row T001", "T001")
	checkContains("skipped row T002", "T002")
	checkContains("skipped row T003", "T003")
	checkContains("skipped row T004", "T004")
	checkContains("importable row T005", "T005")

	// Verify skipped rows have status attribute and class
	checkContains("skipped status", `data-status="skipped"`)
	checkContains("skipped row class", "skipped-row")

	// Verify resolve button appears for unmapped symbols with broker symbol
	checkContains("resolve button", "Resolve Symbol")
	checkContains("broker symbol data attr STTHY", `data-broker-symbol="STTHY"`)
	checkContains("broker symbol data attr AAPL", `data-broker-symbol="AAPL"`)

	// Verify reason column shows the skip reason
	checkContains("reason STTHY", "unmapped symbol: STTHY")
	checkContains("reason duplicate", "duplicate transaction")

	// Verify existing symbols embedded as JSON
	checkContains("existing symbols data", `id="existing-symbols-data"`)
	checkContains("AAPL in symbols", `"AAPL"`)
	checkContains("MSFT in symbols", `"MSFT"`)

	// Verify no grouped sections (old structure removed)
	if strings.Contains(resp, "Unmapped Symbols") {
		t.Error("should not contain 'Unmapped Symbols' section header")
	}
	if strings.Contains(resp, "Other Skipped") {
		t.Error("should not contain 'Other Skipped' section header")
	}
}

func TestImportWebHandleImportPost_InvalidXML(t *testing.T) {
	handler, importSvc, accountRepo, _, _ := setupImportWebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	importSvc.WithPreview(nil, ibkrimport.ErrInvalidXML)

	body, contentType := buildImportMultipartForm("<invalid>", "1")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr", body)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleImportPost(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render with error), got %d", w.Code)
	}

	resp := w.Body.String()
	if !strings.Contains(resp, "alert-error") {
		t.Error("expected error alert on re-render")
	}
	if !strings.Contains(resp, "Invalid XML") {
		t.Error("expected 'Invalid XML' error message")
	}
}

func TestImportWebHandleImportPost_AccountNotFound(t *testing.T) {
	handler, importSvc, accountRepo, _, _ := setupImportWebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	importSvc.WithPreview(nil, ibkrimport.ErrAccountNotFound)

	body, contentType := buildImportMultipartForm(validXML(), "999")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr", body)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleImportPost(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render with error), got %d", w.Code)
	}

	resp := w.Body.String()
	if !strings.Contains(resp, "account not found") {
		t.Error("expected account not found error")
	}
}

func TestImportWebHandleImportPost_MissingAccount(t *testing.T) {
	handler, _, _, _, _ := setupImportWebHandler(t)

	body, contentType := buildImportMultipartForm(validXML(), "")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr", body)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleImportPost(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render with error), got %d", w.Code)
	}

	resp := w.Body.String()
	if !strings.Contains(resp, "Please select an account") {
		t.Error("expected missing account error")
	}
}

// ---- HandleConfirmPost tests ----

func TestImportWebHandleConfirmPost_Success(t *testing.T) {
	handler, importSvc, _, _, _ := setupImportWebHandler(t)

	result := &ibkrimport.ImportResult{
		CreatedCount: 5,
		SkippedCount: 2,
	}
	importSvc.WithImport(result, nil)

	// Build form with base64-encoded XML data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("account_id", "1")
	writer.WriteField("xml_data", "dGVzdC14bWwtZGF0YQ==") // base64 of "test-xml-data"
	writer.Close()

	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr/confirm", body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.HandleConfirmPost(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected %d (redirect), got %d", http.StatusSeeOther, w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/transactions" {
		t.Errorf("expected redirect to /transactions, got %q", location)
	}
}

func TestImportWebHandleConfirmPost_MissingXMLData(t *testing.T) {
	handler, _, _, _, _ := setupImportWebHandler(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("account_id", "1")
	writer.Close()

	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr/confirm", body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.HandleConfirmPost(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImportWebHandleConfirmPost_MissingAccountID(t *testing.T) {
	handler, _, _, _, _ := setupImportWebHandler(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("xml_data", "dGVzdC14bWwtZGF0YQ==")
	writer.Close()

	r := httptest.NewRequest(http.MethodPost, "/transactions/import/ibkr/confirm", body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.HandleConfirmPost(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- RegisterRoutes tests ----

func TestImportWebRegisterRoutes(t *testing.T) {
	handler, importSvc, _, _, _ := setupImportWebHandler(t)
	importSvc.WithPreview(&ibkrimport.PreviewResponse{
		Importable: []ibkrimport.PreviewTransaction{},
		Skipped:    []ibkrimport.SkippedTransaction{},
		Errored:    []ibkrimport.ErroredTransaction{},
	}, nil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	expectedRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/transactions/import/ibkr"},
		{http.MethodPost, "/transactions/import/ibkr"},
		{http.MethodPost, "/transactions/import/ibkr/confirm"},
	}

	for _, e := range expectedRoutes {
		var req *http.Request
		if e.method == http.MethodGet {
			req = httptest.NewRequest(e.method, e.path, nil)
		} else {
			body, contentType := buildImportMultipartForm(validXML(), "1")
			req = httptest.NewRequest(e.method, e.path, body)
			req.Header.Set("Content-Type", contentType)
		}
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Errorf("expected route %s %s to be registered (got 404)", e.method, e.path)
		}
	}
}

// ---- Helper: checkContains (used in tests above) ----

func checkContains(t *testing.T, label, body, text string) {
	t.Helper()
	if !strings.Contains(body, text) {
		t.Errorf("page missing %s: %q", label, text)
	}
}

// mockSymbolRepoForImport is a minimal mock of the symbol mapping repository
// used for the import web handler tests.
type mockSymbolRepoForImport struct {
	mappings map[int64]*symbolmapping.SymbolMapping
	nextID   int64
}

func newMockSymbolRepoForImport() *mockSymbolRepoForImport {
	return &mockSymbolRepoForImport{
		mappings: make(map[int64]*symbolmapping.SymbolMapping),
		nextID:   1,
	}
}

func (m *mockSymbolRepoForImport) Create(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	sm.ID = m.nextID
	m.nextID++
	now := time.Now()
	if sm.CreatedAt.IsZero() {
		sm.CreatedAt = now
	}
	if sm.UpdatedAt.IsZero() {
		sm.UpdatedAt = now
	}
	m.mappings[sm.ID] = sm
	return nil
}

func (m *mockSymbolRepoForImport) GetByID(_ context.Context, id int64) (*symbolmapping.SymbolMapping, error) {
	sm, ok := m.mappings[id]
	if !ok {
		return nil, symbolmapping.ErrNotFound
	}
	cp := *sm
	return &cp, nil
}

func (m *mockSymbolRepoForImport) GetAll(_ context.Context, _, _ int) ([]symbolmapping.SymbolMapping, error) {
	result := make([]symbolmapping.SymbolMapping, 0, len(m.mappings))
	for _, sm := range m.mappings {
		cp := *sm
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockSymbolRepoForImport) ListAll(_ context.Context) ([]symbolmapping.SymbolMapping, error) {
	result := make([]symbolmapping.SymbolMapping, 0, len(m.mappings))
	for _, sm := range m.mappings {
		cp := *sm
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockSymbolRepoForImport) GetByInternalSymbol(_ context.Context, symbol string) (*symbolmapping.SymbolMapping, error) {
	for _, sm := range m.mappings {
		if sm.InternalSymbol == symbol {
			cp := *sm
			return &cp, nil
		}
	}
	return nil, symbolmapping.ErrNotFound
}

func (m *mockSymbolRepoForImport) GetByMarketDataSymbol(_ context.Context, symbol string) (*symbolmapping.SymbolMapping, error) {
	for _, sm := range m.mappings {
		if sm.MarketDataSymbol == symbol {
			cp := *sm
			return &cp, nil
		}
	}
	return nil, symbolmapping.ErrNotFound
}

func (m *mockSymbolRepoForImport) Update(_ context.Context, sm *symbolmapping.SymbolMapping) error {
	m.mappings[sm.ID] = sm
	return nil
}

func (m *mockSymbolRepoForImport) Delete(_ context.Context, id int64) error {
	if _, ok := m.mappings[id]; !ok {
		return symbolmapping.ErrNotFound
	}
	delete(m.mappings, id)
	return nil
}

func (m *mockSymbolRepoForImport) AddBrokerSymbol(_ context.Context, id int64, _, _ string) error {
	return nil
}

func (m *mockSymbolRepoForImport) GetBrokerSymbolByBroker(_ context.Context, _, _ string) (*symbolmapping.BrokerSymbol, error) {
	return nil, symbolmapping.ErrNotFound
}

func (m *mockSymbolRepoForImport) AddMarketDataSymbol(_ context.Context, id int64, _ string) error {
	return nil
}

func (m *mockSymbolRepoForImport) RemoveMarketDataSymbol(_ context.Context, id int64) error {
	return nil
}

func (m *mockSymbolRepoForImport) HasReferencingTransactions(_ context.Context, _ int64) (bool, error) {
	return false, nil
}
