package handlers

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/account"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/ibkrimport"
)

// ---- Setup ----

func setupImportWebHandler(t *testing.T) (*ImportWebHandler, *mockImportService, *mockAccountRepoForTx, *mockAccountChecker) {
	t.Helper()

	importSvc := newMockImportService()
	accountRepo := newMockAccountRepoForTx()
	accountChecker := &mockAccountChecker{}
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	renderer := newTestRenderer(t)

	return NewImportWebHandler(importSvc, accountSvc, renderer), importSvc, accountRepo, accountChecker
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
	handler, _, accountRepo, _ := setupImportWebHandler(t)
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
	handler, _, _, _ := setupImportWebHandler(t)

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
	handler, importSvc, accountRepo, _ := setupImportWebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	accountSvc.Create(nil, account.CreateRequest{Name: "IBKR", PortfolioID: 1})

	preview := &ibkrimport.PreviewResponse{
		Importable: []ibkrimport.PreviewTransaction{
			{Date: "2024-01-15", Type: "buy", Symbol: "AAPL", Quantity: "10", Price: "150.00", Currency: "USD", NetCash: "1500.00", ExternalReference: "T001", Description: "Bought 10 AAPL"},
		},
		Skipped: []ibkrimport.SkippedTransaction{
			{ExternalReference: "T002", Reason: "unmapped symbol"},
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
	checkContains("importable section", "Importable Transactions")
	checkContains("skipped section", "Skipped Transactions")
	checkContains("confirm button", "Confirm Import")
	checkContains("xml data input", `id="xml-data-input"`)
}

func TestImportWebHandleImportPost_InvalidXML(t *testing.T) {
	handler, importSvc, accountRepo, _ := setupImportWebHandler(t)
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
	handler, importSvc, accountRepo, _ := setupImportWebHandler(t)
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
	handler, _, _, _ := setupImportWebHandler(t)

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
	handler, importSvc, _, _ := setupImportWebHandler(t)

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
	handler, _, _, _ := setupImportWebHandler(t)

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
	handler, _, _, _ := setupImportWebHandler(t)

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
	handler, importSvc, _, _ := setupImportWebHandler(t)
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
