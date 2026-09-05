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

	"github.com/eddiectc/portfoliolab/internal/domain/brokerimport"
	"github.com/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"github.com/eddiectc/portfoliolab/internal/domain/trading212import"
)

// ---- Mocks ----

type mockTrading212ImportService struct {
	previewResp  *brokerimport.PreviewResponse
	previewErr   error
	importResp   *brokerimport.ImportResult
	importErr    error
	createSymErr error
	addBrokerErr error
}

func newMockTrading212ImportService() *mockTrading212ImportService {
	return &mockTrading212ImportService{}
}

func (m *mockTrading212ImportService) WithPreview(resp *brokerimport.PreviewResponse, err error) *mockTrading212ImportService {
	m.previewResp = resp
	m.previewErr = err
	return m
}

func (m *mockTrading212ImportService) WithImport(resp *brokerimport.ImportResult, err error) *mockTrading212ImportService {
	m.importResp = resp
	m.importErr = err
	return m
}

func (m *mockTrading212ImportService) Preview(_ context.Context, _ []byte, _ int64) (*brokerimport.PreviewResponse, error) {
	return m.previewResp, m.previewErr
}

func (m *mockTrading212ImportService) ConfirmImport(_ context.Context, _ []byte, _ int64) (*brokerimport.ImportResult, error) {
	return m.importResp, m.importErr
}

func (m *mockTrading212ImportService) CreateSymbol(_ context.Context, _, _ string) error {
	return m.createSymErr
}

func (m *mockTrading212ImportService) AddBrokerSymbolMapping(_ context.Context, _, _, _ string) error {
	return m.addBrokerErr
}

// buildMultipartCSVForm creates a multipart form body with a CSV file and optional fields.
func buildMultipartCSVForm(csvContent, accountID string) (body *bytes.Buffer, contentType string) {
	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("csv_file", "report.csv")
	_, _ = part.Write([]byte(csvContent))
	_ = writer.WriteField("account_id", accountID)
	_ = writer.Close()
	return body, writer.FormDataContentType()
}

func validCSV() string {
	return `Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Deposit,2026-01-05 09:00:00,,,,"Bank Transfer",019a0001-0001-0001-0001-000000000001,,,,,5000.00,"GBP"
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"`
}

// ---- HandlePreview Tests ----

func TestTrading212HandlePreview_Success(t *testing.T) {
	preview := &brokerimport.PreviewResponse{
		Importable: []brokerimport.PreviewTransaction{
			{Date: "2026-01-06", Type: "buy", Symbol: "AAPL", Quantity: "10", Price: "150.00", Currency: "GBP", NetCash: "-1500.00", ExternalReference: "EOF50000000001", Description: "Apple Inc."},
		},
		ImportableCount: 1,
		Skipped:         []brokerimport.SkippedTransaction{},
		Errored:         []brokerimport.ErroredTransaction{},
	}

	importSvc := newMockTrading212ImportService().WithPreview(preview, nil)
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm(validCSV(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp brokerimport.PreviewResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.ImportableCount != 1 {
		t.Errorf("expected 1 importable, got %d", resp.ImportableCount)
	}
}

func TestTrading212HandlePreview_InvalidCSV(t *testing.T) {
	importSvc := newMockTrading212ImportService().WithPreview(nil, trading212import.ErrInvalidCSV)
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm("not,csv\nvalid,data", "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_CSV" {
		t.Errorf("expected INVALID_CSV, got %q", errResp.Code)
	}
}

func TestTrading212HandlePreview_AccountNotFound(t *testing.T) {
	importSvc := newMockTrading212ImportService().WithPreview(nil, trading212import.ErrAccountNotFound)
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm(validCSV(), "999")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTrading212HandlePreview_MissingAccountID(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm(validCSV(), "")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTrading212HandlePreview_MissingCSVFile(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("account_id", "1")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/preview", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.HandlePreview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- HandleConfirm Tests ----

func TestTrading212HandleConfirm_Success(t *testing.T) {
	result := &brokerimport.ImportResult{
		CreatedCount: 5,
		SkippedCount: 2,
	}

	importSvc := newMockTrading212ImportService().WithImport(result, nil)
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm(validCSV(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp brokerimport.ImportResult
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.CreatedCount != 5 {
		t.Errorf("expected 5 created, got %d", resp.CreatedCount)
	}
	if resp.SkippedCount != 2 {
		t.Errorf("expected 2 skipped, got %d", resp.SkippedCount)
	}
}

func TestTrading212HandleConfirm_AllDuplicates(t *testing.T) {
	result := &brokerimport.ImportResult{
		CreatedCount: 0,
		SkippedCount: 10,
	}

	importSvc := newMockTrading212ImportService().WithImport(result, nil)
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm(validCSV(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp brokerimport.ImportResult
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.CreatedCount != 0 {
		t.Errorf("expected 0 created, got %d", resp.CreatedCount)
	}
}

func TestTrading212HandleConfirm_InternalError(t *testing.T) {
	importSvc := newMockTrading212ImportService().WithImport(nil, fmt.Errorf("batch create failed"))
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm(validCSV(), "1")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestTrading212HandleConfirm_MissingAccountID(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body, contentType := buildMultipartCSVForm(validCSV(), "")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()

	handler.HandleConfirm(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- HandleCreateSymbol Tests ----

func TestTrading212HandleCreateSymbol_Success(t *testing.T) {
	sm := &symbolmapping.SymbolMapping{
		ID:               42,
		InternalSymbol:   "QGRP",
		MarketDataSymbol: "QGRP.L",
	}

	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService().WithCreate(sm, nil)
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body := `{"internal_symbol":"QGRP","market_data_symbol":"QGRP.L"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var resp symbolmapping.SymbolMapping
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.InternalSymbol != "QGRP" {
		t.Errorf("expected 'QGRP', got %q", resp.InternalSymbol)
	}
}

func TestTrading212HandleCreateSymbol_InvalidBody(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/symbols", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTrading212HandleCreateSymbol_SymbolExists(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService().WithCreate(nil, symbolmapping.ErrInternalSymbolExists)
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body := `{"internal_symbol":"AAPL","market_data_symbol":"AAPL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestTrading212HandleCreateSymbol_InvalidSymbol(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService().WithCreate(nil, symbolmapping.ErrInvalidSymbol)
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body := `{"internal_symbol":"","market_data_symbol":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleCreateSymbol(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---- HandleAddBrokerSymbol Tests ----

func TestTrading212HandleAddBrokerSymbol_Success(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body := `{"broker_name":"Trading212","broker_symbol":"QGRP","internal_symbol":"QGRP"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/broker-symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTrading212HandleAddBrokerSymbol_InvalidBody(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/broker-symbols", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTrading212HandleAddBrokerSymbol_SymbolNotFound(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	importSvc.addBrokerErr = symbolmapping.ErrNotFound
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body := `{"broker_name":"Trading212","broker_symbol":"QGRP","internal_symbol":"XYZZY"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/broker-symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "SYMBOL_NOT_FOUND" {
		t.Errorf("expected SYMBOL_NOT_FOUND, got %q", errResp.Code)
	}
}

func TestTrading212HandleAddBrokerSymbol_BrokerSymbolExists(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	importSvc.addBrokerErr = symbolmapping.ErrBrokerSymbolExists
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	body := `{"broker_name":"Trading212","broker_symbol":"QGRP","internal_symbol":"QGRP"}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/trading212/broker-symbols", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAddBrokerSymbol(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "BROKER_SYMBOL_EXISTS" {
		t.Errorf("expected BROKER_SYMBOL_EXISTS, got %q", errResp.Code)
	}
}

// ---- RegisterRoutes Tests ----

func TestTrading212RegisterRoutes(t *testing.T) {
	importSvc := newMockTrading212ImportService()
	importSvc.previewResp = &brokerimport.PreviewResponse{
		Importable: []brokerimport.PreviewTransaction{},
		Skipped:    []brokerimport.SkippedTransaction{},
		Errored:    []brokerimport.ErroredTransaction{},
	}
	symbolSvc := newMockSymbolService()
	handler := NewTrading212ImportHandler(importSvc, symbolSvc)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	expectedRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/transactions/import/trading212/preview"},
		{http.MethodPost, "/api/transactions/import/trading212/confirm"},
		{http.MethodPost, "/api/transactions/import/trading212/symbols"},
		{http.MethodPost, "/api/transactions/import/trading212/broker-symbols"},
	}

	for _, e := range expectedRoutes {
		var req *http.Request
		if e.path == "/api/transactions/import/trading212/preview" || e.path == "/api/transactions/import/trading212/confirm" {
			body, contentType := buildMultipartCSVForm(validCSV(), "1")
			req = httptest.NewRequest(e.method, e.path, body)
			req.Header.Set("Content-Type", contentType)
		} else {
			req = httptest.NewRequest(e.method, e.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Errorf("expected route %s %s to be registered (got 404)", e.method, e.path)
		}
	}
}

// ---- Error Response Format ----

func TestTrading212ErrorResponseFormat(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		setup       func(*mockTrading212ImportService)
		wantCode    int
		wantErrCode string
	}{
		{
			name:        "preview invalid CSV",
			method:      http.MethodPost,
			path:        "/api/transactions/import/trading212/preview",
			setup:       func(m *mockTrading212ImportService) { m.previewErr = trading212import.ErrInvalidCSV },
			wantCode:    http.StatusBadRequest,
			wantErrCode: "INVALID_CSV",
		},
		{
			name:        "preview account not found",
			method:      http.MethodPost,
			path:        "/api/transactions/import/trading212/preview",
			setup:       func(m *mockTrading212ImportService) { m.previewErr = trading212import.ErrAccountNotFound },
			wantCode:    http.StatusNotFound,
			wantErrCode: "ACCOUNT_NOT_FOUND",
		},
		{
			name:        "confirm internal error",
			method:      http.MethodPost,
			path:        "/api/transactions/import/trading212/confirm",
			setup:       func(m *mockTrading212ImportService) { m.importErr = fmt.Errorf("batch create failed") },
			wantCode:    http.StatusInternalServerError,
			wantErrCode: "IMPORT_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importSvc := newMockTrading212ImportService()
			tt.setup(importSvc)
			symbolSvc := newMockSymbolService()
			handler := NewTrading212ImportHandler(importSvc, symbolSvc)

			body, contentType := buildMultipartCSVForm(validCSV(), "1")
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", contentType)
			w := httptest.NewRecorder()

			switch tt.path {
			case "/api/transactions/import/trading212/preview":
				handler.HandlePreview(w, req)
			case "/api/transactions/import/trading212/confirm":
				handler.HandleConfirm(w, req)
			}

			if w.Code != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, w.Code)
			}

			var errResp APIError
			_ = json.NewDecoder(w.Body).Decode(&errResp)
			if errResp.Code != tt.wantErrCode {
				t.Errorf("expected error code %q, got %q", tt.wantErrCode, errResp.Code)
			}
		})
	}
}
