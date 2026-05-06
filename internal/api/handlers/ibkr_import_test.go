package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/ibkrimport"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/symbolmapping"
)

// ---- Mocks ----

type mockImportService struct {
	previewResp   *ibkrimport.PreviewResponse
	previewErr    error
	importResp    *ibkrimport.ImportResult
	importErr     error
	createSymErr  error
	addBrokerErr  error
}

func newMockImportService() *mockImportService {
	return &mockImportService{}
}

func (m *mockImportService) WithPreview(resp *ibkrimport.PreviewResponse, err error) *mockImportService {
	m.previewResp = resp
	m.previewErr = err
	return m
}

func (m *mockImportService) WithImport(resp *ibkrimport.ImportResult, err error) *mockImportService {
	m.importResp = resp
	m.importErr = err
	return m
}

func (m *mockImportService) Preview(_ context.Context, _ []byte, _ int64) (*ibkrimport.PreviewResponse, error) {
	return m.previewResp, m.previewErr
}

func (m *mockImportService) ConfirmImport(_ context.Context, _ []byte, _ int64) (*ibkrimport.ImportResult, error) {
	return m.importResp, m.importErr
}

func (m *mockImportService) CreateSymbol(_ context.Context, _, _ string) error {
	return m.createSymErr
}

func (m *mockImportService) AddBrokerSymbolMapping(_ context.Context, _, _, _ string) error {
	return m.addBrokerErr
}

type mockSymbolService struct {
	createResp *symbolmapping.SymbolMapping
	createErr  error
}

func newMockSymbolService() *mockSymbolService {
	return &mockSymbolService{}
}

func (m *mockSymbolService) WithCreate(resp *symbolmapping.SymbolMapping, err error) *mockSymbolService {
	m.createResp = resp
	m.createErr = err
	return m
}

func (m *mockSymbolService) Create(_ context.Context, _ symbolmapping.CreateRequest) (*symbolmapping.SymbolMapping, error) {
	return m.createResp, m.createErr
}

// buildMultipartForm creates a multipart form body with an XML file and optional fields.
func buildMultipartForm(xmlContent, accountID string) (body *bytes.Buffer, contentType string) {
	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("xml_file", "report.xml")
	part.Write([]byte(xmlContent))
	writer.WriteField("account_id", accountID)
	writer.Close()
	return body, writer.FormDataContentType()
}

func validXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<SecurityOwnerActivityReport>
  <Statements>
    <Statement>
      <Trades>
        <Trade tradeDate="20240115" transactionID="T001" assetCategory="STK" subCategory="COMMON" symbol="AAPL" buySell="BUY" tradePrice="150.00" quantity="10" currency="USD" netCash="-1500.00" proceeds="1500.00" ibCommission="1.00" description="Bought 10 AAPL"/>
      </Trades>
      <CashTransactions/>
      <Transfers/>
    </Statement>
  </Statements>
</SecurityOwnerActivityReport>`
}

// ---- HandlePreview Tests ----

func TestImportHandlePreview_Success(t *testing.T) {
	preview := &ibkrimport.PreviewResponse{
		Importable: []ibkrimport.PreviewTransaction{
			{Date: "2024-01-15", Type: "buy", Symbol: "AAPL", Quantity: "10", Price: "150.00", Currency: "USD", NetCash: "1500.00", ExternalReference: "T001", Description: "Bought 10 AAPL"},
		},
		ImportableCount: 1,
		Skipped:         []ibkrimport.SkippedTransaction{},
		Errored:         []ibkrimport.ErroredTransaction{},
	}

	importSvc := newMockImportService().WithPreview(preview, nil)
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm(validXML(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ibkrimport.PreviewResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ImportableCount != 1 {
		t.Errorf("expected 1 importable, got %d", resp.ImportableCount)
	}
}

func TestImportHandlePreview_InvalidXML(t *testing.T) {
	importSvc := newMockImportService().WithPreview(nil, ibkrimport.ErrInvalidXML)
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm("<invalid>", "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_XML" {
		t.Errorf("expected INVALID_XML, got %q", errResp.Code)
	}
}

func TestImportHandlePreview_AccountNotFound(t *testing.T) {
	importSvc := newMockImportService().WithPreview(nil, ibkrimport.ErrAccountNotFound)
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm(validXML(), "999")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestImportHandlePreview_MissingAccountID(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm(validXML(), "")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImportHandlePreview_MissingXMLFile(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("account_id", "1")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- HandleConfirm Tests ----

func TestImportHandleConfirm_Success(t *testing.T) {
	result := &ibkrimport.ImportResult{
		CreatedCount: 5,
		SkippedCount: 2,
	}

	importSvc := newMockImportService().WithImport(result, nil)
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm(validXML(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ibkrimport.ImportResult
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.CreatedCount != 5 {
		t.Errorf("expected 5 created, got %d", resp.CreatedCount)
	}
	if resp.SkippedCount != 2 {
		t.Errorf("expected 2 skipped, got %d", resp.SkippedCount)
	}
}

func TestImportHandleConfirm_AllDuplicates(t *testing.T) {
	result := &ibkrimport.ImportResult{
		CreatedCount: 0,
		SkippedCount: 10,
	}

	importSvc := newMockImportService().WithImport(result, nil)
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm(validXML(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp ibkrimport.ImportResult
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.CreatedCount != 0 {
		t.Errorf("expected 0 created, got %d", resp.CreatedCount)
	}
}

func TestImportHandleConfirm_InternalError(t *testing.T) {
	importSvc := newMockImportService().WithImport(nil, fmt.Errorf("batch create failed"))
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm(validXML(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestImportHandleConfirm_MissingAccountID(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartForm(validXML(), "")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- HandleCreateSymbol Tests ----

func TestImportHandleCreateSymbol_Success(t *testing.T) {
	sm := &symbolmapping.SymbolMapping{
		ID:             42,
		InternalSymbol: "AAPL",
		MarketDataSymbol: "AAPL",
	}

	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService().WithCreate(sm, nil)
	handler := NewImportHandler(importSvc, symbolSvc)

	body := `{"internal_symbol":"AAPL","market_data_symbol":"AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var resp symbolmapping.SymbolMapping
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.InternalSymbol != "AAPL" {
		t.Errorf("expected 'AAPL', got %q", resp.InternalSymbol)
	}
}

func TestImportHandleCreateSymbol_InvalidBody(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/symbols", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImportHandleCreateSymbol_SymbolExists(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService().WithCreate(nil, symbolmapping.ErrInternalSymbolExists)
	handler := NewImportHandler(importSvc, symbolSvc)

	body := `{"internal_symbol":"AAPL","market_data_symbol":"AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestImportHandleCreateSymbol_InvalidSymbol(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService().WithCreate(nil, symbolmapping.ErrInvalidSymbol)
	handler := NewImportHandler(importSvc, symbolSvc)

	body := `{"internal_symbol":"","market_data_symbol":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- HandleAddBrokerSymbol Tests ----

func TestImportHandleAddBrokerSymbol_Success(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body := `{"broker_name":"IBKR","broker_symbol":"AAPL","internal_symbol":"AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/broker-symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestImportHandleAddBrokerSymbol_InvalidBody(t *testing.T) {
	importSvc := newMockImportService()
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/broker-symbols", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImportHandleAddBrokerSymbol_SymbolNotFound(t *testing.T) {
	importSvc := newMockImportService()
	importSvc.addBrokerErr = symbolmapping.ErrNotFound
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body := `{"broker_name":"IBKR","broker_symbol":"AAPL","internal_symbol":"XYZZY"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/broker-symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "SYMBOL_NOT_FOUND" {
		t.Errorf("expected SYMBOL_NOT_FOUND, got %q", errResp.Code)
	}
}

func TestImportHandleAddBrokerSymbol_BrokerSymbolExists(t *testing.T) {
	importSvc := newMockImportService()
	importSvc.addBrokerErr = symbolmapping.ErrBrokerSymbolExists
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	body := `{"broker_name":"IBKR","broker_symbol":"AAPL","internal_symbol":"AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/broker-symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}

	var errResp APIError
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "BROKER_SYMBOL_EXISTS" {
		t.Errorf("expected BROKER_SYMBOL_EXISTS, got %q", errResp.Code)
	}
}

// ---- RegisterRoutes Tests ----

func TestImportRegisterRoutes(t *testing.T) {
	importSvc := newMockImportService()
	importSvc.previewResp = &ibkrimport.PreviewResponse{
		Importable:      []ibkrimport.PreviewTransaction{},
		Skipped:         []ibkrimport.SkippedTransaction{},
		Errored:         []ibkrimport.ErroredTransaction{},
	}
	symbolSvc := newMockSymbolService()
	handler := NewImportHandler(importSvc, symbolSvc)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Verify each route responds (not 404)
	expectedRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/transactions/import/ibkr/preview"},
		{http.MethodPost, "/api/transactions/import/ibkr/confirm"},
		{http.MethodPost, "/api/transactions/import/ibkr/symbols"},
		{http.MethodPost, "/api/transactions/import/ibkr/broker-symbols"},
	}

	for _, e := range expectedRoutes {
		body, contentType := buildMultipartForm(validXML(), "1")
		req := httptest.NewRequest(e.method, e.path, body)
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		// Routes should not return 404 — they may return other errors but the handler is wired
		if w.Code == http.StatusNotFound {
			t.Errorf("expected route %s %s to be registered (got 404)", e.method, e.path)
		}
	}
}

// ---- Error Response Format ----

func TestImportErrorResponseFormat(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		contentType string
		setup      func(*mockImportService)
		wantCode   int
		wantErrCode string
	}{
		{
			name:  "preview invalid XML",
			method: http.MethodPost,
			path:  "/api/transactions/import/ibkr/preview",
			setup: func(m *mockImportService) { m.previewErr = ibkrimport.ErrInvalidXML },
			wantCode: http.StatusBadRequest,
			wantErrCode: "INVALID_XML",
		},
		{
			name:  "preview account not found",
			method: http.MethodPost,
			path:  "/api/transactions/import/ibkr/preview",
			setup: func(m *mockImportService) { m.previewErr = ibkrimport.ErrAccountNotFound },
			wantCode: http.StatusNotFound,
			wantErrCode: "ACCOUNT_NOT_FOUND",
		},
		{
			name:  "confirm internal error",
			method: http.MethodPost,
			path:  "/api/transactions/import/ibkr/confirm",
			setup: func(m *mockImportService) { m.importErr = fmt.Errorf("batch create failed") },
			wantCode: http.StatusInternalServerError,
			wantErrCode: "IMPORT_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importSvc := newMockImportService()
			tt.setup(importSvc)
			symbolSvc := newMockSymbolService()
			handler := NewImportHandler(importSvc, symbolSvc)

			var body *bytes.Buffer
			var contentType string
			if tt.path == "/api/transactions/import/ibkr/preview" || tt.path == "/api/transactions/import/ibkr/confirm" {
				body, contentType = buildMultipartForm(validXML(), "1")
			} else {
				body = bytes.NewBufferString(tt.body)
				contentType = "application/json"
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", contentType)
			w := httptest.NewRecorder()

			switch tt.path {
			case "/api/transactions/import/ibkr/preview":
				handler.HandlePreview(w, req)
			case "/api/transactions/import/ibkr/confirm":
				handler.HandleConfirm(w, req)
			}

			if w.Code != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, w.Code)
			}

			var errResp APIError
			json.NewDecoder(w.Body).Decode(&errResp)
			if errResp.Code != tt.wantErrCode {
				t.Errorf("expected error code %q, got %q", tt.wantErrCode, errResp.Code)
			}
		})
	}
}
