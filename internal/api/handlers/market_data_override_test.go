package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/marketdata"
)

// --- Test helpers ---

// mockOverrideStore is a hand-written in-memory mock of marketdata.Store for
// override handler tests.
type mockOverrideStore struct {
	rows      map[string]*marketdata.Override
	upsertErr error
}

func overrideKey(symbol, date string, dt marketdata.DataType) string {
	return symbol + "|" + date + "|" + string(dt)
}

func newMockOverrideStore() *mockOverrideStore {
	return &mockOverrideStore{rows: make(map[string]*marketdata.Override)}
}

func (m *mockOverrideStore) seed(symbol, date string, dt marketdata.DataType, price decimal.Decimal, currency string) {
	m.rows[overrideKey(symbol, date, dt)] = &marketdata.Override{
		Symbol:   symbol,
		Date:     date,
		DataType: dt,
		Price:    price,
		Currency: currency,
		Source:   marketdata.Source,
	}
}

func (m *mockOverrideStore) get(symbol, date string, dt marketdata.DataType) *marketdata.Override {
	return m.rows[overrideKey(symbol, date, dt)]
}

func (m *mockOverrideStore) Upsert(_ context.Context, o *marketdata.Override) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	cp := *o
	m.rows[overrideKey(o.Symbol, o.Date, o.DataType)] = &cp
	return nil
}

func (m *mockOverrideStore) Delete(_ context.Context, symbol, date string, dt marketdata.DataType) (bool, error) {
	key := overrideKey(symbol, date, dt)
	if _, ok := m.rows[key]; !ok {
		return false, nil
	}
	delete(m.rows, key)
	return true, nil
}

func (m *mockOverrideStore) List(_ context.Context, symbol *string) ([]marketdata.Override, error) {
	var out []marketdata.Override
	for _, o := range m.rows {
		if symbol != nil && o.Symbol != *symbol {
			continue
		}
		cp := *o
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Symbol != out[j].Symbol {
			return out[i].Symbol < out[j].Symbol
		}
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].DataType < out[j].DataType
	})
	return out, nil
}

func newOverrideTestRouter(store *mockOverrideStore) http.Handler {
	r := chi.NewRouter()
	NewMarketDataOverrideHandler(marketdata.NewService(store)).RegisterRoutes(r)
	return r
}

func putOverride(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/market-data/overrides", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func getOverrides(t *testing.T, h http.Handler, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/market-data/overrides"+rawQuery, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func deleteOverride(t *testing.T, h http.Handler, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/market-data/overrides?"+rawQuery, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decodeAPIError(t *testing.T, w *httptest.ResponseRecorder) APIError {
	t.Helper()
	var e APIError
	if err := json.NewDecoder(w.Body).Decode(&e); err != nil {
		t.Fatalf("failed to decode error response %q: %v", w.Body.String(), err)
	}
	return e
}

// --- Tests ---

func TestOverrideSet_HappyPath(t *testing.T) {
	store := newMockOverrideStore()
	h := newOverrideTestRouter(store)

	w := putOverride(t, h, `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.02,"currency":"GBP"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Symbol   string          `json:"symbol"`
		Date     string          `json:"date"`
		DataType string          `json:"data_type"`
		Price    decimal.Decimal `json:"price"`
		Currency string          `json:"currency"`
		Source   string          `json:"source"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response %q: %v", w.Body.String(), err)
	}
	if resp.Symbol != "XNAQ.L" || resp.Date != "2025-10-24" || resp.DataType != "stock" || resp.Currency != "GBP" {
		t.Errorf("unexpected response fields: %+v", resp)
	}
	if !resp.Price.Equal(decimal.MustParse("44.02")) {
		t.Errorf("expected price 44.02, got %s", resp.Price.String())
	}
	if resp.Source != "override" {
		t.Errorf(`expected source "override", got %q`, resp.Source)
	}

	stored := store.get("XNAQ.L", "2025-10-24", marketdata.DataTypeStock)
	if stored == nil {
		t.Fatal("expected the store to contain the upserted override")
	}
	if !stored.Price.Equal(decimal.MustParse("44.02")) {
		t.Errorf("expected stored price 44.02, got %s", stored.Price.String())
	}
}

func TestOverrideSet_ReplaceExisting(t *testing.T) {
	store := newMockOverrideStore()
	store.seed("XNAQ.L", "2025-10-24", marketdata.DataTypeStock, decimal.MustParse("44.00"), "GBP")
	h := newOverrideTestRouter(store)

	w := putOverride(t, h, `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.05,"currency":"GBP"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(store.rows) != 1 {
		t.Fatalf("expected one row after replace, got %d", len(store.rows))
	}
	stored := store.get("XNAQ.L", "2025-10-24", marketdata.DataTypeStock)
	if !stored.Price.Equal(decimal.MustParse("44.05")) {
		t.Errorf("expected price 44.05, got %s", stored.Price.String())
	}
}

func TestOverrideSet_InvalidJSON(t *testing.T) {
	store := newMockOverrideStore()
	h := newOverrideTestRouter(store)

	w := putOverride(t, h, `{"symbol": `)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if e := decodeAPIError(t, w); e.Code != "INVALID_REQUEST" {
		t.Errorf(`expected code INVALID_REQUEST, got %q`, e.Code)
	}
	if len(store.rows) != 0 {
		t.Error("expected no rows to be stored for an invalid body")
	}
}

func TestOverrideSet_ValidationErrors(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode string
	}{
		{"empty symbol", `{"symbol":"","date":"2025-10-24","data_type":"stock","price":44,"currency":"GBP"}`, "INVALID_SYMBOL"},
		{"unknown data type", `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"etf","price":44,"currency":"GBP"}`, "INVALID_DATA_TYPE"},
		{"zero price", `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":0,"currency":"GBP"}`, "INVALID_PRICE"},
		{"negative price", `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":-1.5,"currency":"GBP"}`, "INVALID_PRICE"},
		{"malformed date", `{"symbol":"XNAQ.L","date":"24/10/2025","data_type":"stock","price":44,"currency":"GBP"}`, "INVALID_DATE"},
		{"future date", `{"symbol":"XNAQ.L","date":"` + time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02") + `","data_type":"stock","price":44,"currency":"GBP"}`, "INVALID_DATE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockOverrideStore()
			h := newOverrideTestRouter(store)

			w := putOverride(t, h, tc.body)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if e := decodeAPIError(t, w); e.Code != tc.wantCode {
				t.Errorf("expected code %s, got %q", tc.wantCode, e.Code)
			}
			if len(store.rows) != 0 {
				t.Error("expected no rows to be stored on validation failure")
			}
		})
	}
}

func TestOverrideSet_InternalError(t *testing.T) {
	store := newMockOverrideStore()
	store.upsertErr = errors.New("disk on fire")
	h := newOverrideTestRouter(store)

	w := putOverride(t, h, `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.02,"currency":"GBP"}`)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if e := decodeAPIError(t, w); e.Code != "INTERNAL_ERROR" {
		t.Errorf(`expected code INTERNAL_ERROR, got %q`, e.Code)
	}
}

func TestOverrideList_All(t *testing.T) {
	store := newMockOverrideStore()
	store.seed("XNAQ.L", "2025-10-24", marketdata.DataTypeStock, decimal.MustParse("44.02"), "GBP")
	store.seed("GBP/USD", "2025-10-24", marketdata.DataTypeFx, decimal.MustParse("1.28"), "")
	store.seed("XNAQ.L", "2025-10-23", marketdata.DataTypeStock, decimal.MustParse("43.9"), "GBP")
	h := newOverrideTestRouter(store)

	w := getOverrides(t, h, "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var items []marketdata.Override
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode %q: %v", w.Body.String(), err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 overrides, got %d", len(items))
	}
	wantOrder := []string{
		"GBP/USD|2025-10-24|fx",
		"XNAQ.L|2025-10-23|stock",
		"XNAQ.L|2025-10-24|stock",
	}
	for i, want := range wantOrder {
		got := items[i].Symbol + "|" + items[i].Date + "|" + string(items[i].DataType)
		if got != want {
			t.Errorf("position %d: expected %s, got %s", i, want, got)
		}
	}
}

func TestOverrideList_BySymbol(t *testing.T) {
	store := newMockOverrideStore()
	store.seed("XNAQ.L", "2025-10-24", marketdata.DataTypeStock, decimal.MustParse("44.02"), "GBP")
	store.seed("GBP/USD", "2025-10-24", marketdata.DataTypeFx, decimal.MustParse("1.28"), "")
	h := newOverrideTestRouter(store)

	w := getOverrides(t, h, "?symbol="+url.QueryEscape("GBP/USD"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var items []marketdata.Override
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode %q: %v", w.Body.String(), err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 override, got %d", len(items))
	}
	if items[0].Symbol != "GBP/USD" {
		t.Errorf(`expected symbol GBP/USD, got %q`, items[0].Symbol)
	}
}

func TestOverrideList_Empty(t *testing.T) {
	store := newMockOverrideStore()
	h := newOverrideTestRouter(store)

	w := getOverrides(t, h, "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var items []marketdata.Override
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode %q: %v", w.Body.String(), err)
	}
	if items == nil {
		t.Error("expected an empty array, not null")
	}
	if len(items) != 0 {
		t.Errorf("expected 0 overrides, got %d", len(items))
	}
}

func TestOverrideDelete_RemovesOverride(t *testing.T) {
	store := newMockOverrideStore()
	store.seed("XNAQ.L", "2025-10-24", marketdata.DataTypeStock, decimal.MustParse("44.02"), "GBP")
	h := newOverrideTestRouter(store)

	w := deleteOverride(t, h, "symbol="+url.QueryEscape("XNAQ.L")+"&date=2025-10-24&data_type=stock")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if len(w.Body.Bytes()) != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
	if len(store.rows) != 0 {
		t.Error("expected the override to be deleted")
	}
}

func TestOverrideDelete_MissingParams(t *testing.T) {
	store := newMockOverrideStore()
	h := newOverrideTestRouter(store)

	cases := []struct {
		name  string
		query string
	}{
		{"missing date", "symbol=XNAQ.L&data_type=stock"},
		{"missing data_type", "symbol=XNAQ.L&date=2025-10-24"},
		{"missing symbol", "date=2025-10-24&data_type=stock"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := deleteOverride(t, h, tc.query)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if e := decodeAPIError(t, w); e.Code != "INVALID_REQUEST" {
				t.Errorf(`expected code INVALID_REQUEST, got %q`, e.Code)
			}
		})
	}
}

func TestOverrideDelete_NotFound(t *testing.T) {
	store := newMockOverrideStore()
	h := newOverrideTestRouter(store)

	w := deleteOverride(t, h, "symbol=XNAQ.L&date=2025-10-24&data_type=stock")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if e := decodeAPIError(t, w); e.Code != "OVERRIDE_NOT_FOUND" {
		t.Errorf(`expected code OVERRIDE_NOT_FOUND, got %q`, e.Code)
	}
}

func TestOverrideDelete_OtherDataTypeStill404(t *testing.T) {
	store := newMockOverrideStore()
	store.seed("XNAQ.L", "2025-10-24", marketdata.DataTypeStock, decimal.MustParse("44.02"), "GBP")
	h := newOverrideTestRouter(store)

	w := deleteOverride(t, h, "symbol=XNAQ.L&date=2025-10-24&data_type=nav")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if e := decodeAPIError(t, w); e.Code != "OVERRIDE_NOT_FOUND" {
		t.Errorf(`expected code OVERRIDE_NOT_FOUND, got %q`, e.Code)
	}
	if len(store.rows) != 1 {
		t.Error("expected the unrelated override to remain")
	}
}
