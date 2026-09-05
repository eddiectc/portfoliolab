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

	"github.com/eddiectc/portfoliolab/internal/domain/account"
	"github.com/eddiectc/portfoliolab/internal/domain/brokerimport"
	"github.com/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"github.com/eddiectc/portfoliolab/internal/domain/trading212import"
)

// ---- Setup ----

func setupTrading212WebHandler(t *testing.T) (*Trading212ImportWebHandler, *mockTrading212ImportService, *mockAccountRepoForTx, *mockAccountChecker, *mockSymbolRepoForImport) {
	t.Helper()

	importSvc := newMockTrading212ImportService()
	accountRepo := newMockAccountRepoForTx()
	accountChecker := &mockAccountChecker{}
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	symbolRepo := newMockSymbolRepoForImport()
	symbolSvc := symbolmapping.NewService(symbolRepo)
	renderer := newTestRenderer(t)

	return NewTrading212ImportWebHandler(importSvc, accountSvc, symbolSvc, renderer), importSvc, accountRepo, accountChecker, symbolRepo
}

// buildTrading212MultipartForm creates a multipart form body with a CSV file and account_id.
func buildTrading212MultipartForm(csvContent, accountID string) (body *bytes.Buffer, contentType string) {
	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("csv_file", "report.csv")
	_, _ = part.Write([]byte(csvContent))
	_ = writer.WriteField("account_id", accountID)
	_ = writer.Close()
	return body, writer.FormDataContentType()
}

// ---- HandleImportPage tests ----

func TestTrading212WebHandleImportPage_RendersUploadPage(t *testing.T) {
	handler, _, accountRepo, _, _ := setupTrading212WebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	_, _ = accountSvc.Create(context.TODO(), account.CreateRequest{Name: "Trading 212 ISA", PortfolioID: 1})

	r := httptest.NewRequest(http.MethodGet, "/transactions/import/trading212", nil)
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

	checkContains("title", "Import Trading 212 CSV")
	checkContains("account select", `id="account_id"`)
	checkContains("file input", `id="csv_file"`)
	checkContains("submit button", "Preview Import")
	checkContains("supported types", "Supported Transaction Types")
	checkContains("trades info", "Limit buy")
	checkContains("cash transactions info", "Deposit")
	checkContains("gbx note", "GBX")
	checkContains("back link", "Back to Transactions")
	checkContains("closing html", "</html>")
}

func TestTrading212WebHandleImportPage_NoAccounts(t *testing.T) {
	handler, _, _, _, _ := setupTrading212WebHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/transactions/import/trading212", nil)
	w := httptest.NewRecorder()

	handler.HandleImportPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Import Trading 212 CSV") {
		t.Error("missing title")
	}
}

// ---- HandleImportPost tests ----

func TestTrading212WebHandleImportPost_ValidCSV(t *testing.T) {
	handler, importSvc, accountRepo, _, _ := setupTrading212WebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	_, _ = accountSvc.Create(context.TODO(), account.CreateRequest{Name: "Trading 212 ISA", PortfolioID: 1})

	preview := &brokerimport.PreviewResponse{
		Importable: []brokerimport.PreviewTransaction{
			{Date: "2026-01-06", Type: "buy", Symbol: "AAPL", Quantity: "10", Price: "150.00", Currency: "GBP", NetCash: "-1500.00", ExternalReference: "EOF50000000001", Description: "Apple Inc."},
		},
		Skipped: []brokerimport.SkippedTransaction{
			{ExternalReference: "EOF50000000002", Reason: "unmapped symbol: QGRP", BrokerSymbol: "QGRP"},
		},
		ImportableCount: 1,
		SkippedCount:    1,
		Errored:         []brokerimport.ErroredTransaction{},
	}
	importSvc.WithPreview(preview, nil)

	body, contentType := buildTrading212MultipartForm(validCSV(), "1")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212", body)
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
	checkContains("csv data input", `id="csv-data-input"`)
}

func TestTrading212WebHandleImportPost_UnifiedTableAndSymbols(t *testing.T) {
	handler, importSvc, accountRepo, _, symbolRepo := setupTrading212WebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	_, _ = accountSvc.Create(context.TODO(), account.CreateRequest{Name: "Trading 212 ISA", PortfolioID: 1})

	// Pre-populate existing symbols in the symbol repo
	_ = symbolRepo.Create(context.TODO(), &symbolmapping.SymbolMapping{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"})
	_ = symbolRepo.Create(context.TODO(), &symbolmapping.SymbolMapping{InternalSymbol: "MSFT", MarketDataSymbol: "MSFT"})

	// Multiple skipped with same/different broker symbols + one without broker symbol
	preview := &brokerimport.PreviewResponse{
		Importable: []brokerimport.PreviewTransaction{
			{Date: "2026-01-06", Type: "buy", Symbol: "AAPL", Quantity: "10", Price: "150.00", Currency: "GBP", NetCash: "-1500.00", ExternalReference: "EOF50000000005", Description: "Apple Inc."},
		},
		Skipped: []brokerimport.SkippedTransaction{
			{ExternalReference: "EOF50000000001", Reason: "unmapped symbol: QGRP", BrokerSymbol: "QGRP", Date: "2026-01-06", Type: "buy", Symbol: "QGRP", Quantity: "5", Price: "60.00", Currency: "GBP", NetCash: "-300.00", Description: "iShares Global Infrastructure"},
			{ExternalReference: "EOF50000000002", Reason: "unmapped symbol: QGBP", BrokerSymbol: "QGBP", Date: "2026-01-07", Type: "buy", Symbol: "QGBP", Quantity: "10", Price: "130.00", Currency: "GBP", NetCash: "-1300.00", Description: "iShares UK Treasury Gilts"},
			{ExternalReference: "EOF50000000003", Reason: "duplicate — already imported", Date: "2026-01-08", Type: "deposit", Symbol: "$CASH-GBP", Quantity: "5000", Price: "1", Currency: "GBP", NetCash: "5000.00", Description: "Bank Transfer"},
		},
		ImportableCount: 1,
		SkippedCount:    3,
		Errored:         []brokerimport.ErroredTransaction{},
	}
	importSvc.WithPreview(preview, nil)

	body, contentType := buildTrading212MultipartForm(validCSV(), "1")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212", body)
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
	checkContains("skipped row EOF50000000001", "EOF50000000001")
	checkContains("skipped row EOF50000000002", "EOF50000000002")
	checkContains("skipped row EOF50000000003", "EOF50000000003")
	checkContains("importable row EOF50000000005", "EOF50000000005")

	// Verify skipped rows have status attribute and class
	checkContains("skipped status", `data-status="skipped"`)
	checkContains("skipped row class", "skipped-row")

	// Verify resolve button appears for unmapped symbols with broker symbol
	checkContains("resolve button", "Resolve Symbol")
	checkContains("broker symbol data attr QGRP", `data-broker-symbol="QGRP"`)
	checkContains("broker symbol data attr QGBP", `data-broker-symbol="QGBP"`)

	// Verify reason column shows the skip reason
	checkContains("reason QGRP", "unmapped symbol: QGRP")
	checkContains("reason duplicate", "duplicate — already imported")

	// Verify existing symbols embedded as JSON
	checkContains("existing symbols data", `id="existing-symbols-data"`)
	checkContains("AAPL in symbols", `"AAPL"`)
	checkContains("MSFT in symbols", `"MSFT"`)

	// Verify Trading 212 API endpoints in JS
	checkContains("trading212 symbols endpoint", "/api/transactions/import/trading212/symbols")
	checkContains("trading212 broker-symbols endpoint", "/api/transactions/import/trading212/broker-symbols")
	checkContains("broker name Trading212", "Trading212")
}

func TestTrading212WebHandleImportPost_InvalidCSV(t *testing.T) {
	handler, importSvc, accountRepo, _, _ := setupTrading212WebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	_, _ = accountSvc.Create(context.TODO(), account.CreateRequest{Name: "Trading 212 ISA", PortfolioID: 1})

	importSvc.WithPreview(nil, trading212import.ErrInvalidCSV)

	body, contentType := buildTrading212MultipartForm("not,csv\nvalid,data", "1")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212", body)
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
	if !strings.Contains(resp, "Invalid CSV") {
		t.Error("expected 'Invalid CSV' error message")
	}
}

func TestTrading212WebHandleImportPost_AccountNotFound(t *testing.T) {
	handler, importSvc, accountRepo, _, _ := setupTrading212WebHandler(t)
	accountSvc := account.NewService(accountRepo, &mockPortfolioCheckerForWeb{})
	handler.accountSvc = accountSvc

	_, _ = accountSvc.Create(context.TODO(), account.CreateRequest{Name: "Trading 212 ISA", PortfolioID: 1})

	importSvc.WithPreview(nil, trading212import.ErrAccountNotFound)

	body, contentType := buildTrading212MultipartForm(validCSV(), "999")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212", body)
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

func TestTrading212WebHandleImportPost_MissingAccount(t *testing.T) {
	handler, _, _, _, _ := setupTrading212WebHandler(t)

	body, contentType := buildTrading212MultipartForm(validCSV(), "")
	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212", body)
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

func TestTrading212WebHandleConfirmPost_Success(t *testing.T) {
	handler, importSvc, _, _, _ := setupTrading212WebHandler(t)

	result := &brokerimport.ImportResult{
		CreatedCount: 5,
		SkippedCount: 2,
	}
	importSvc.WithImport(result, nil)

	// Build form with base64-encoded CSV data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("account_id", "1")
	_ = writer.WriteField("csv_data", "dGVzdC1jc3YtZGF0YQ==") // base64 of "test-csv-data"
	_ = writer.Close()

	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212/confirm", body)
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

func TestTrading212WebHandleConfirmPost_MissingCSVData(t *testing.T) {
	handler, _, _, _, _ := setupTrading212WebHandler(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("account_id", "1")
	_ = writer.Close()

	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212/confirm", body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.HandleConfirmPost(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTrading212WebHandleConfirmPost_MissingAccountID(t *testing.T) {
	handler, _, _, _, _ := setupTrading212WebHandler(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("csv_data", "dGVzdC1jc3YtZGF0YQ==")
	_ = writer.Close()

	r := httptest.NewRequest(http.MethodPost, "/transactions/import/trading212/confirm", body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.HandleConfirmPost(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- RegisterRoutes tests ----

func TestTrading212WebRegisterRoutes(t *testing.T) {
	handler, importSvc, _, _, _ := setupTrading212WebHandler(t)
	importSvc.WithPreview(&brokerimport.PreviewResponse{
		Importable: []brokerimport.PreviewTransaction{},
		Skipped:    []brokerimport.SkippedTransaction{},
		Errored:    []brokerimport.ErroredTransaction{},
	}, nil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	expectedRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/transactions/import/trading212"},
		{http.MethodPost, "/transactions/import/trading212"},
		{http.MethodPost, "/transactions/import/trading212/confirm"},
	}

	for _, e := range expectedRoutes {
		var req *http.Request
		if e.method == http.MethodGet {
			req = httptest.NewRequest(e.method, e.path, nil)
		} else {
			body, contentType := buildTrading212MultipartForm(validCSV(), "1")
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
