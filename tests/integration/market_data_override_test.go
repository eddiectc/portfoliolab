package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/data"
	"github.com/eddiectc/portfoliolab/internal/domain/marketdata"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// seedMarketData inserts a provider market data row directly.
func seedMarketData(t *testing.T, repo *data.MarketDataRepository, m *market.MarketData) {
	t.Helper()
	if err := repo.Upsert(context.Background(), m); err != nil {
		t.Fatalf("seed %s %s %s: %v", m.Symbol, m.DataType, m.Date, err)
	}
}

// putOverride sends PUT /api/market-data/overrides with the given JSON body.
func putOverride(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/market-data/overrides", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestMarketDataOverride_CorrectsCorruptedBar is the spec's primary scenario:
// a wrong provider bar is corrected by an override; the provider row stays
// intact and raw reads still see it.
func TestMarketDataOverride_CorrectsCorruptedBar(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	router := newTestRouter(t, db)
	ctx := context.Background()

	seedMarketData(t, repo, &market.MarketData{
		Symbol: "XNAQ.L", Price: decimal.MustParse("41.99"), Currency: "GBP",
		DataType: "stock", Source: "yahoo", Date: "2025-10-24", FetchedAt: time.Now(),
	})

	w := putOverride(t, router, `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.02,"currency":"GBP"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var stored marketdata.Override
	if err := json.NewDecoder(w.Body).Decode(&stored); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if stored.Source != "override" {
		t.Errorf("expected source 'override', got %q", stored.Source)
	}
	if !stored.Price.Equal(decimal.MustParse("44.02")) {
		t.Errorf("expected stored price 44.02, got %s", stored.Price)
	}

	start, _ := time.Parse("2006-01-02", "2025-10-01")
	end, _ := time.Parse("2006-01-02", "2025-10-31")
	prices, err := repo.GetHistoricalPricesBySymbol(ctx, "XNAQ.L", start, end)
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}
	if !prices[0].Close.Equal(decimal.MustParse("44.02")) {
		t.Errorf("expected corrected price 44.02, got %s", prices[0].Close)
	}

	// The provider row coexists untouched; raw reads see the original value.
	raw, err := repo.GetBySourceAndDate(ctx, "XNAQ.L", "yahoo", "2025-10-24")
	if err != nil {
		t.Fatalf("GetBySourceAndDate: %v", err)
	}
	if raw == nil || !raw.Price.Equal(decimal.MustParse("41.99")) {
		t.Errorf("expected provider row 41.99 to remain, got %+v", raw)
	}
}

// TestMarketDataOverride_OverrideOnlyDate verifies that an override on a date
// the provider has no row for still appears in the price history (union of
// dates).
func TestMarketDataOverride_OverrideOnlyDate(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	router := newTestRouter(t, db)
	ctx := context.Background()

	seedMarketData(t, repo, &market.MarketData{
		Symbol: "XNAQ.L", Price: decimal.MustParse("50.00"), Currency: "GBP",
		DataType: "stock", Source: "yahoo", Date: "2025-10-20", FetchedAt: time.Now(),
	})
	w := putOverride(t, router, `{"symbol":"XNAQ.L","date":"2025-10-22","data_type":"stock","price":51.50,"currency":"GBP"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	start, _ := time.Parse("2006-01-02", "2025-10-01")
	end, _ := time.Parse("2006-01-02", "2025-10-31")
	prices, err := repo.GetHistoricalPricesBySymbol(ctx, "XNAQ.L", start, end)
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(prices) != 2 {
		t.Fatalf("expected 2 prices (provider + override-only date), got %d", len(prices))
	}
	if !prices[0].Date.Equal(time.Date(2025, 10, 20, 0, 0, 0, 0, time.UTC)) ||
		!prices[0].Close.Equal(decimal.MustParse("50.00")) {
		t.Errorf("first price: expected 2025-10-20 50.00, got %s %s", prices[0].Date, prices[0].Close)
	}
	if !prices[1].Date.Equal(time.Date(2025, 10, 22, 0, 0, 0, 0, time.UTC)) ||
		!prices[1].Close.Equal(decimal.MustParse("51.50")) {
		t.Errorf("second price: expected 2025-10-22 51.50, got %s %s", prices[1].Date, prices[1].Close)
	}
}

// TestMarketDataOverride_SurvivesRefresh verifies that re-fetching the same
// date (manual full-refresh semantics) does not displace the override.
func TestMarketDataOverride_SurvivesRefresh(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	router := newTestRouter(t, db)
	ctx := context.Background()

	seedMarketData(t, repo, &market.MarketData{
		Symbol: "XNAQ.L", Price: decimal.MustParse("41.99"), Currency: "GBP",
		DataType: "stock", Source: "yahoo", Date: "2025-10-24", FetchedAt: time.Now(),
	})
	w := putOverride(t, router, `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.02,"currency":"GBP"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	// A refresh upserts the provider row again with a different (still wrong)
	// value for the same date.
	if err := repo.Upsert(ctx, &market.MarketData{
		Symbol: "XNAQ.L", Price: decimal.MustParse("42.50"), Currency: "GBP",
		DataType: "stock", Source: "yahoo", Date: "2025-10-24", FetchedAt: time.Now(),
	}); err != nil {
		t.Fatalf("refresh upsert: %v", err)
	}

	start, _ := time.Parse("2006-01-02", "2025-10-01")
	end, _ := time.Parse("2006-01-02", "2025-10-31")
	prices, err := repo.GetHistoricalPricesBySymbol(ctx, "XNAQ.L", start, end)
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(prices) != 1 || !prices[0].Close.Equal(decimal.MustParse("44.02")) {
		t.Errorf("expected override 44.02 to survive refresh, got %+v", prices)
	}
}

// TestMarketDataOverride_FxOnOrBefore verifies FX override precedence in the
// forward-fill read: the override wins for D >= override date, the provider
// rate wins before it.
func TestMarketDataOverride_FxOnOrBefore(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	router := newTestRouter(t, db)
	ctx := context.Background()

	seedMarketData(t, repo, &market.MarketData{
		Symbol: "GBP/USD", Price: decimal.MustParse("1.2500"), Currency: "USD",
		DataType: "fx", Source: "yahoo", Date: "2024-01-10", FetchedAt: time.Now(),
	})
	w := putOverride(t, router, `{"symbol":"GBP/USD","date":"2024-01-15","data_type":"fx","price":1.3000,"currency":"USD"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	cases := []struct {
		date   string
		want   string
		source string
	}{
		{"2024-01-12", "1.2500", "yahoo"},    // before the override: provider
		{"2024-01-15", "1.3000", "override"}, // on the override date
		{"2024-02-01", "1.3000", "override"}, // after: forward-filled
	}
	for _, tc := range cases {
		got, err := repo.GetHistoricalFxRateOnOrBefore(ctx, "GBP/USD", "yahoo", tc.date)
		if err != nil {
			t.Fatalf("GetHistoricalFxRateOnOrBefore(%s): %v", tc.date, err)
		}
		if got == nil || !got.Price.Equal(decimal.MustParse(tc.want)) || got.Source != tc.source {
			t.Errorf("date %s: expected %s (source %s), got %+v", tc.date, tc.want, tc.source, got)
		}
	}
}

// TestMarketDataOverride_NavHistory verifies NAV overrides are reflected in
// the NAV read path (symbol detail page).
func TestMarketDataOverride_NavHistory(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	router := newTestRouter(t, db)
	ctx := context.Background()

	seedMarketData(t, repo, &market.MarketData{
		Symbol: "WMGG.L", Price: decimal.MustParse("100.00"), Currency: "GBP",
		DataType: "nav", Source: "wisdomtree", Date: "2024-01-15", FetchedAt: time.Now(),
	})
	w := putOverride(t, router, `{"symbol":"WMGG.L","date":"2024-01-15","data_type":"nav","price":101.25,"currency":"GBP"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	prices, err := repo.GetNavHistoryBySymbol(ctx, "WMGG.L")
	if err != nil {
		t.Fatalf("GetNavHistoryBySymbol: %v", err)
	}
	if len(prices) != 1 || !prices[0].Close.Equal(decimal.MustParse("101.25")) {
		t.Errorf("expected NAV override 101.25, got %+v", prices)
	}
}

// TestMarketDataOverride_LiveQuoteUnaffected verifies overrides never leak
// into live-quote reads.
func TestMarketDataOverride_LiveQuoteUnaffected(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	router := newTestRouter(t, db)
	ctx := context.Background()

	seedMarketData(t, repo, &market.MarketData{
		Symbol: "XNAQ.L", Price: decimal.MustParse("51.00"), Currency: "GBP",
		DataType: "stock", Source: "yahoo", Date: "", FetchedAt: time.Now(),
	})
	w := putOverride(t, router, `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.02,"currency":"GBP"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	latest, err := repo.GetLatest(ctx, "XNAQ.L")
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest == nil || !latest.Price.Equal(decimal.MustParse("51.00")) {
		t.Errorf("expected live quote 51.00, got %+v", latest)
	}

	// No override row may ever be written to market_data.
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data WHERE source = 'override'`).Scan(&count); err != nil {
		t.Fatalf("count override rows: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 override rows in market_data, got %d", count)
	}
}

// TestMarketDataOverride_UpdateReplaces verifies a PUT on an existing key
// replaces the override (last write wins).
func TestMarketDataOverride_UpdateReplaces(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	body := `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.02,"currency":"GBP"}`
	if w := putOverride(t, router, body); w.Code != http.StatusOK {
		t.Fatalf("first PUT: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	body = `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":45.10,"currency":"GBP"}`
	w := putOverride(t, router, body)
	if w.Code != http.StatusOK {
		t.Fatalf("second PUT: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/market-data/overrides", nil)
	wl := httptest.NewRecorder()
	router.ServeHTTP(wl, req)
	if wl.Code != http.StatusOK {
		t.Fatalf("GET list: expected 200, got %d", wl.Code)
	}
	var items []marketdata.Override
	if err := json.NewDecoder(wl.Body).Decode(&items); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 override, got %d", len(items))
	}
	if !items[0].Price.Equal(decimal.MustParse("45.10")) {
		t.Errorf("expected replacement price 45.10, got %s", items[0].Price)
	}
}

// TestMarketDataOverride_Remove verifies DELETE reverts reads to the provider
// value, returns 404 when nothing matches, and is scoped to (symbol, data
// type) — other overrides are untouched.
func TestMarketDataOverride_Remove(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewMarketDataRepository(db)
	router := newTestRouter(t, db)
	ctx := context.Background()

	seedMarketData(t, repo, &market.MarketData{
		Symbol: "XNAQ.L", Price: decimal.MustParse("41.99"), Currency: "GBP",
		DataType: "stock", Source: "yahoo", Date: "2025-10-24", FetchedAt: time.Now(),
	})
	if w := putOverride(t, router, `{"symbol":"XNAQ.L","date":"2025-10-24","data_type":"stock","price":44.02,"currency":"GBP"}`); w.Code != http.StatusOK {
		t.Fatalf("PUT stock override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	// An override on another symbol (FX series) shares the date; removing the
	// stock override must not touch it.
	if w := putOverride(t, router, `{"symbol":"GBP/USD","date":"2025-10-24","data_type":"fx","price":1.30,"currency":"USD"}`); w.Code != http.StatusOK {
		t.Fatalf("PUT fx override: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	del := func(dataType string) int {
		req := httptest.NewRequest(http.MethodDelete,
			fmt.Sprintf("/api/market-data/overrides?symbol=XNAQ.L&date=2025-10-24&data_type=%s", dataType), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	if code := del("stock"); code != http.StatusNoContent {
		t.Fatalf("DELETE stock: expected 204, got %d", code)
	}

	start, _ := time.Parse("2006-01-02", "2025-10-01")
	end, _ := time.Parse("2006-01-02", "2025-10-31")
	prices, err := repo.GetHistoricalPricesBySymbol(ctx, "XNAQ.L", start, end)
	if err != nil {
		t.Fatalf("GetHistoricalPricesBySymbol: %v", err)
	}
	if len(prices) != 1 || !prices[0].Close.Equal(decimal.MustParse("41.99")) {
		t.Errorf("expected reads to revert to provider 41.99, got %+v", prices)
	}

	if code := del("stock"); code != http.StatusNotFound {
		t.Fatalf("DELETE again: expected 404, got %d", code)
	}
	// A different data type must 404 even when an override exists on the same
	// date under another symbol.
	if code := del("nav"); code != http.StatusNotFound {
		t.Fatalf("DELETE nav (none exists): expected 404, got %d", code)
	}

	// The FX override on the other symbol is still present.
	req := httptest.NewRequest(http.MethodGet, "/api/market-data/overrides?symbol=GBP/USD", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var items []marketdata.Override
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(items) != 1 || items[0].DataType != marketdata.DataTypeFx {
		t.Errorf("expected the GBP/USD fx override to survive, got %+v", items)
	}
}

// TestMarketDataOverride_ValidationOverHTTP verifies service validation
// errors surface as 400s with the matching error codes.
func TestMarketDataOverride_ValidationOverHTTP(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	futureDate := time.Now().UTC().AddDate(0, 1, 0).Format("2006-01-02")
	cases := []struct {
		name string
		body string
		code string
	}{
		{"zero price", `{"symbol":"X","date":"2024-01-01","data_type":"stock","price":0,"currency":"GBP"}`, "INVALID_PRICE"},
		{"negative price", `{"symbol":"X","date":"2024-01-01","data_type":"stock","price":-1,"currency":"GBP"}`, "INVALID_PRICE"},
		{"unparseable date", `{"symbol":"X","date":"2024-13-01","data_type":"stock","price":1.5,"currency":"GBP"}`, "INVALID_DATE"},
		{"future date", fmt.Sprintf(`{"symbol":"X","date":"%s","data_type":"stock","price":1.5,"currency":"GBP"}`, futureDate), "INVALID_DATE"},
		{"unknown data type", `{"symbol":"X","date":"2024-01-01","data_type":"bogus","price":1.5,"currency":"GBP"}`, "INVALID_DATA_TYPE"},
		{"empty symbol", `{"symbol":"","date":"2024-01-01","data_type":"stock","price":1.5,"currency":"GBP"}`, "INVALID_SYMBOL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := putOverride(t, router, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (%s)", w.Code, w.Body.String())
			}
			var errResp struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if errResp.Code != tc.code {
				t.Errorf("expected code %s, got %s", tc.code, errResp.Code)
			}
		})
	}

	// Nothing was stored.
	req := httptest.NewRequest(http.MethodGet, "/api/market-data/overrides", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var items []marketdata.Override
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 stored overrides after validation failures, got %d", len(items))
	}
}

// TestMarketDataOverride_ListOrderAndFilter verifies stable list ordering
// (symbol, date, data_type) and the ?symbol= filter.
func TestMarketDataOverride_ListOrderAndFilter(t *testing.T) {
	db := setupTestDB(t)
	router := newTestRouter(t, db)

	// Insert deliberately out of order.
	bodies := []string{
		`{"symbol":"B","date":"2024-01-02","data_type":"stock","price":2.0,"currency":"GBP"}`,
		`{"symbol":"A","date":"2024-01-01","data_type":"stock","price":1.0,"currency":"GBP"}`,
		`{"symbol":"B","date":"2024-01-01","data_type":"nav","price":1.5,"currency":"GBP"}`,
	}
	for _, b := range bodies {
		if w := putOverride(t, router, b); w.Code != http.StatusOK {
			t.Fatalf("PUT %s: expected 200, got %d (%s)", b, w.Code, w.Body.String())
		}
	}

	list := func(query string) []marketdata.Override {
		req := httptest.NewRequest(http.MethodGet, "/api/market-data/overrides"+query, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET list: expected 200, got %d", w.Code)
		}
		var items []marketdata.Override
		if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		return items
	}

	items := list("")
	if len(items) != 3 {
		t.Fatalf("expected 3 overrides, got %d", len(items))
	}
	want := [][3]string{
		{"A", "2024-01-01", "stock"},
		{"B", "2024-01-01", "nav"},
		{"B", "2024-01-02", "stock"},
	}
	for i, key := range want {
		got := [3]string{items[i].Symbol, items[i].Date, string(items[i].DataType)}
		if got != key {
			t.Errorf("position %d: expected %v, got %v", i, key, got)
		}
	}

	filtered := list("?symbol=B")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 overrides for B, got %d", len(filtered))
	}
	for _, o := range filtered {
		if o.Symbol != "B" {
			t.Errorf("expected only B, got %q", o.Symbol)
		}
	}
}
