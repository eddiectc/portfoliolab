package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/eddiectc/portfoliolab/internal/domain/account"
	"github.com/eddiectc/portfoliolab/internal/domain/ibkrimport"
	"github.com/eddiectc/portfoliolab/internal/domain/transaction"
)

// loadIBKRSampleXML reads the IBKR sample XML fixture from the testdata directory.
func loadIBKRSampleXML(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/ibkr_sample.xml")
	if err != nil {
		t.Fatalf("read IBKR sample XML: %v", err)
	}
	return data
}

// buildImportForm creates a multipart form body with an XML file and account_id.
func buildImportForm(t *testing.T, xmlData []byte, accountID string) (body *bytes.Buffer, contentType string) {
	t.Helper()
	body = &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("xml_file", "report.xml")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, err = part.Write(xmlData)
	if err != nil {
		t.Fatalf("write xml data: %v", err)
	}
	err = writer.WriteField("account_id", accountID)
	if err != nil {
		t.Fatalf("write account_id: %v", err)
	}
	err = writer.Close()
	if err != nil {
		t.Fatalf("close form: %v", err)
	}
	return body, writer.FormDataContentType()
}

// setupIBKR creates a portfolio, account, and symbol mappings needed for IBKR import tests.
func setupIBKR(t *testing.T) (db *sql.DB, router http.Handler, accountID int64) {
	t.Helper()
	db = setupTestDB(t)
	router = newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Test Portfolio", "currency": "GBP"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create account
	acctBody := json.RawMessage(`{"name": "IBKR", "portfolio_id": 1}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(acctBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d", w.Code)
	}
	var a account.Account
	_ = json.NewDecoder(w.Body).Decode(&a)

	// Create symbol mappings for symbols in the sample XML: AAPL, STHY
	for _, sym := range []string{"AAPL", "STHY"} {
		smBody := json.RawMessage(fmt.Sprintf(`{"internal_symbol": "%s", "market_data_symbol": "%s"}`, sym, sym))
		req = httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(smBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create symbol mapping %s: expected 201, got %d", sym, w.Code)
		}
	}

	return db, router, a.ID
}

// countTransactions queries the transactions table for a given account.
func countTransactions(t *testing.T, db *sql.DB, accountID int64) int {
	t.Helper()
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM transactions WHERE account_id = ?", accountID).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	return count
}

func TestIBKRImport_FullFlow(t *testing.T) {
	db, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	// Step 1: Preview
	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("preview: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var preview ibkrimport.PreviewResponse
	_ = json.NewDecoder(w.Body).Decode(&preview)

	// Sample XML: 6 trades (5 STK + 1 FX→2 entries) + 7 cash + 2 transfers = 16 importable
	if preview.ImportableCount != 16 {
		t.Errorf("expected 16 importable, got %d (skipped: %d, errored: %d)",
			preview.ImportableCount, preview.SkippedCount, preview.ErroredCount)
	}
	if preview.SkippedCount != 0 {
		t.Errorf("expected 0 skipped, got %d", preview.SkippedCount)
	}
	if preview.ErroredCount != 0 {
		t.Errorf("expected 0 errored, got %d", preview.ErroredCount)
	}

	// Step 2: Confirm
	body, contentType = buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req = httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result ibkrimport.ImportResult
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result.CreatedCount != 16 {
		t.Errorf("expected 16 created, got %d (skipped: %d)", result.CreatedCount, result.SkippedCount)
	}

	// Step 3: Verify transactions in DB
	count := countTransactions(t, db, accountID)
	if count != 16 {
		t.Errorf("expected 16 transactions in DB, got %d", count)
	}

	// Step 4: Verify external_system and external_reference are set
	var extSystem, extRef string
	err := db.QueryRow(
		"SELECT external_system, external_reference FROM transactions WHERE account_id = ? LIMIT 1",
		accountID,
	).Scan(&extSystem, &extRef)
	if err != nil {
		t.Fatalf("query external fields: %v", err)
	}
	if extSystem != "IBKR" {
		t.Errorf("expected external_system 'IBKR', got %q", extSystem)
	}
	if extRef == "" {
		t.Error("expected non-empty external_reference")
	}
}

func TestIBKRImport_DuplicateDetection(t *testing.T) {
	_, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	// First import
	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first import: expected 200, got %d", w.Code)
	}

	var result ibkrimport.ImportResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result.CreatedCount != 16 {
		t.Fatalf("expected 16 created on first import, got %d", result.CreatedCount)
	}

	// Re-import same file — all should be skipped
	body, contentType = buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req = httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("second preview: expected 200, got %d", w.Code)
	}

	var preview ibkrimport.PreviewResponse
	_ = json.NewDecoder(w.Body).Decode(&preview)

	if preview.ImportableCount != 0 {
		t.Errorf("expected 0 importable on re-import, got %d", preview.ImportableCount)
	}
	// 15 source records (6 trades + 7 cash + 2 transfers), FX counts as 1 skip
	if preview.SkippedCount != 15 {
		t.Errorf("expected 15 skipped on re-import, got %d", preview.SkippedCount)
	}

	// Confirm re-import — should create 0
	body, contentType = buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req = httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("second confirm: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	_ = json.NewDecoder(w.Body).Decode(&result)
	if result.CreatedCount != 0 {
		t.Errorf("expected 0 created on re-import, got %d", result.CreatedCount)
	}
}

func TestIBKRImport_InvalidXML(t *testing.T) {
	_, router, accountID := setupIBKR(t)

	body, contentType := buildImportForm(t, []byte("<not valid xml"), fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestIBKRImport_EmptyXML(t *testing.T) {
	_, router, accountID := setupIBKR(t)

	body, contentType := buildImportForm(t, []byte(""), fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestIBKRImport_AccountNotFound(t *testing.T) {
	_, router, _ := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	body, contentType := buildImportForm(t, xmlData, "999")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestIBKRImport_FXTradesCreateTwoTransactions(t *testing.T) {
	db, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d", w.Code)
	}

	// The sample XML has 1 FX trade (GBP.USD) which should produce 2 transactions:
	// withdrawal in GBP and deposit in USD, with composite external references
	var withdrawalRef, depositRef string
	err := db.QueryRow(`
		SELECT external_reference FROM transactions
		WHERE account_id = ? AND external_reference LIKE '%_fx_withdrawal'
		LIMIT 1
	`, accountID).Scan(&withdrawalRef)
	if err != nil {
		t.Fatalf("query FX withdrawal ref: %v", err)
	}
	err = db.QueryRow(`
		SELECT external_reference FROM transactions
		WHERE account_id = ? AND external_reference LIKE '%_fx_deposit'
		LIMIT 1
	`, accountID).Scan(&depositRef)
	if err != nil {
		t.Fatalf("query FX deposit ref: %v", err)
	}

	if withdrawalRef == "" {
		t.Error("expected FX withdrawal transaction with composite reference")
	}
	if depositRef == "" {
		t.Error("expected FX deposit transaction with composite reference")
	}
}

func TestIBKRImport_CashTransactionClassification(t *testing.T) {
	db, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d", w.Code)
	}

	// Verify cash transaction types exist in the DB
	expectedTypes := map[string]bool{
		"dividend": false,
		"interest": false,
		"tax":      false,
		"fee":      false,
		"deposit":  false,
	}

	rows, err := db.Query(`
		SELECT type FROM transactions
		WHERE account_id = ? AND type IN ('dividend', 'interest', 'tax', 'fee', 'deposit')
	`, accountID)
	if err != nil {
		t.Fatalf("query types: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatalf("scan type: %v", err)
		}
		expectedTypes[typ] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	for typ, found := range expectedTypes {
		if !found {
			t.Errorf("expected cash transaction type %q not found", typ)
		}
	}
}

func TestIBKRImport_TransferClassification(t *testing.T) {
	db, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d", w.Code)
	}

	// Sample XML has 2 transfers: 1 IN (deposit) and 1 OUT (withdrawal)
	// Count deposits and withdrawals (includes cash txns too, so check by external ref)
	var depositCount, withdrawalCount int
	err := db.QueryRow(`
		SELECT
			SUM(CASE WHEN type = 'deposit' THEN 1 ELSE 0 END),
			SUM(CASE WHEN type = 'withdrawal' THEN 1 ELSE 0 END)
		FROM transactions
		WHERE account_id = ?
			AND external_reference IN ('30000000020', '30000000021')
	`, accountID).Scan(&depositCount, &withdrawalCount)
	if err != nil {
		t.Fatalf("query transfer types: %v", err)
	}

	if depositCount != 1 {
		t.Errorf("expected 1 transfer deposit, got %d", depositCount)
	}
	if withdrawalCount != 1 {
		t.Errorf("expected 1 transfer withdrawal, got %d", withdrawalCount)
	}
}

func TestIBKRImport_UnmappedSymbolsSkipped(t *testing.T) {
	// Create setup WITHOUT symbol mappings — all trade symbols should be skipped
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Test Portfolio", "currency": "GBP"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create account
	acctBody := json.RawMessage(`{"name": "IBKR", "portfolio_id": 1}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(acctBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d", w.Code)
	}
	var a account.Account
	_ = json.NewDecoder(w.Body).Decode(&a)

	xmlData := loadIBKRSampleXML(t)

	// Preview without any symbol mappings
	formBody, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", a.ID))
	req = httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", formBody)
	req.Header.Set("Content-Type", contentType)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("preview: expected 200, got %d", w.Code)
	}

	var preview ibkrimport.PreviewResponse
	_ = json.NewDecoder(w.Body).Decode(&preview)

	// 5 STK trades should be skipped (unmapped), 1 FX trade is importable (2 entries, no symbol needed)
	// 7 cash txns: 1 dividend (needs symbol, skipped), 6 others (no symbol needed, importable)
	// 2 transfers: importable
	// So: 2 FX + 6 cash + 2 transfers = 10 importable, 5+1 = 6 skipped
	if preview.ImportableCount != 10 {
		t.Errorf("expected 10 importable without symbol mappings, got %d (skipped: %d, errored: %d)",
			preview.ImportableCount, preview.SkippedCount, preview.ErroredCount)
	}
	if preview.SkippedCount != 6 {
		t.Errorf("expected 6 skipped without symbol mappings, got %d", preview.SkippedCount)
	}

	// Verify skipped reasons include "unmapped symbol" with broker symbol
	unmappedCount := 0
	for _, s := range preview.Skipped {
		if strings.HasPrefix(s.Reason, "unmapped symbol:") {
			unmappedCount++
		}
	}
	if unmappedCount != 6 {
		t.Errorf("expected 6 'unmapped symbol' skips, got %d", unmappedCount)
	}
}

func TestIBKRImport_BrokerSymbolMapping(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Create portfolio
	body := json.RawMessage(`{"name": "Test Portfolio", "currency": "GBP"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: expected 201, got %d", w.Code)
	}

	// Create account
	acctBody := json.RawMessage(`{"name": "IBKR", "portfolio_id": 1}`)
	req = httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(acctBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: expected 201, got %d", w.Code)
	}
	var a account.Account
	_ = json.NewDecoder(w.Body).Decode(&a)

	// Create internal symbol "AAPL" but NO broker symbol mapping
	smBody := json.RawMessage(`{"internal_symbol": "AAPL", "market_data_symbol": "AAPL"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(smBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create symbol mapping: expected 201, got %d", w.Code)
	}

	xmlData := loadIBKRSampleXML(t)

	// Preview: AAPL trades should resolve via direct symbol match (symbol in XML = "AAPL" = internal symbol)
	formBody, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", a.ID))
	req = httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", formBody)
	req.Header.Set("Content-Type", contentType)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("preview: expected 200, got %d", w.Code)
	}

	var preview ibkrimport.PreviewResponse
	_ = json.NewDecoder(w.Body).Decode(&preview)

	// AAPL trades should be importable via direct match, STHY trades skipped (unmapped)
	// 2 AAPL trades + 1 FX + 6 cash (non-dividend) + 2 transfers = 11 importable
	// 2 STHY trades + 1 STHY dividend = 3 skipped
	aaplCount := 0
	for _, p := range preview.Importable {
		if p.Symbol == "AAPL" {
			aaplCount++
		}
	}
	if aaplCount != 2 {
		t.Errorf("expected 2 AAPL trades importable via direct match, got %d", aaplCount)
	}
}

func TestIBKRImport_UniqueIndexPreventsDuplicates(t *testing.T) {
	db, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	// First import
	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first import: expected 200, got %d", w.Code)
	}

	// Try to manually insert a duplicate at the DB level — should fail due to unique index
	_, err := db.Exec(`
		INSERT INTO transactions (account_id, date, type, symbol, quantity, price, currency,
			net_cash, external_system, external_reference)
		VALUES (?, datetime('now'), 'buy', 'AAPL', '1', '100', 'USD', '-100', 'IBKR', '30000000001')
	`, accountID)
	if err == nil {
		t.Error("expected unique index violation, got nil error")
	}
}

func TestIBKRImport_StockAndETFTrades(t *testing.T) {
	db, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d", w.Code)
	}

	// Verify buy and sell transactions exist
	var buyCount, sellCount int
	err := db.QueryRow(`
		SELECT
			SUM(CASE WHEN type = 'buy' THEN 1 ELSE 0 END),
			SUM(CASE WHEN type = 'sell' THEN 1 ELSE 0 END)
		FROM transactions
		WHERE account_id = ?
	`, accountID).Scan(&buyCount, &sellCount)
	if err != nil {
		t.Fatalf("query trade types: %v", err)
	}

	if buyCount == 0 {
		t.Error("expected at least 1 buy transaction")
	}
	if sellCount == 0 {
		t.Error("expected at least 1 sell transaction")
	}
}

func TestIBKRImport_NegativeQuantityOnSells(t *testing.T) {
	db, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d", w.Code)
	}

	// Verify sell transactions have negative quantity (as IBKR reports them)
	var quantity string
	err := db.QueryRow(`
		SELECT quantity FROM transactions
		WHERE account_id = ? AND type = 'sell' LIMIT 1
	`, accountID).Scan(&quantity)
	if err != nil {
		t.Fatalf("query sell quantity: %v", err)
	}
	if quantity[0] != '-' {
		t.Errorf("expected negative quantity for sell, got %q", quantity)
	}
}

func TestIBKRImport_MissingAccountID(t *testing.T) {
	_, router, _ := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	body, contentType := buildImportForm(t, xmlData, "")
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/preview", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing account_id, got %d", w.Code)
	}
}

func TestIBKRImport_ListTransactionsAfterImport(t *testing.T) {
	_, router, accountID := setupIBKR(t)
	xmlData := loadIBKRSampleXML(t)

	// Import
	body, contentType := buildImportForm(t, xmlData, fmt.Sprintf("%d", accountID))
	req := httptest.NewRequest(http.MethodPost, "/api/transactions/import/ibkr/confirm", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: expected 200, got %d", w.Code)
	}

	// List transactions via API — should include imported ones
	req = httptest.NewRequest(http.MethodGet, "/api/transactions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var txns []transaction.Transaction
	_ = json.NewDecoder(w.Body).Decode(&txns)

	// Should have at least 16 transactions
	if len(txns) < 16 {
		t.Errorf("expected at least 16 transactions, got %d", len(txns))
	}

	// Verify imported transactions have external_system set
	importedCount := 0
	for _, tx := range txns {
		if tx.ExternalSystem != nil && *tx.ExternalSystem == "IBKR" {
			importedCount++
		}
	}
	if importedCount != 16 {
		t.Errorf("expected 16 transactions with external_system='IBKR', got %d", importedCount)
	}
}
