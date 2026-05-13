package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// histPrice creates a HistoricalPrice with the given close value (in cents).
func histPrice(dateStr string, closeCents int64, currency string) market.HistoricalPrice {
	pt, _ := time.Parse("2006-01-02", dateStr)
	return market.HistoricalPrice{
		Date:     pt,
		Close:    decimal.MustNew(closeCents, 2),
		Currency: currency,
	}
}

// insertMarketData inserts stock prices and FX rates into the market_data table.
func insertMarketData(t *testing.T, db *sql.DB, stockPrices, fxRates map[string][]market.HistoricalPrice) {
	t.Helper()
	for sym, prices := range stockPrices {
		for _, p := range prices {
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, ?, 'stock', 'yahoo', ?)`,
				sym, p.Close.String(), p.Currency, p.Date.Format("2006-01-02"),
			)
			if err != nil {
				t.Fatalf("insert market data %s: %v", sym, err)
			}
		}
	}
	for pair, prices := range fxRates {
		for _, p := range prices {
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, ?, 'fx', 'yahoo', ?)`,
				pair, p.Close.String(), p.Currency, p.Date.Format("2006-01-02"),
			)
			if err != nil {
				t.Fatalf("insert fx data %s: %v", pair, err)
			}
		}
	}
}

// setupPerf creates a portfolio + account via the API and returns identifiers.
func setupPerf(t *testing.T, baseCurrency, acctCurrency string) (*sql.DB, http.Handler, int64, int64) {
	t.Helper()
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Portfolio
	body := json.RawMessage(`{"name":"Perf","currency":"` + baseCurrency + `"}`)
	req := httptest.NewRequest("POST", "/api/portfolios", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create portfolio: %d %s", w.Code, w.Body.String())
	}
	var p portfolio.Portfolio
	json.NewDecoder(w.Body).Decode(&p)

	// Account
	body = json.RawMessage(`{"name":"Acct","portfolio_id":` + fmt.Sprintf("%d", p.ID) + `}`)
	req = httptest.NewRequest("POST", "/api/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create account: %d %s", w.Code, w.Body.String())
	}
	var a struct{ ID int64 }
	json.NewDecoder(w.Body).Decode(&a)

	// Symbol mappings
	for _, sym := range []string{"USDSTK", "GBPSTK"} {
		body = json.RawMessage(`{"internal_symbol":"` + sym + `","market_data_symbol":"` + sym + `"}`)
		req = httptest.NewRequest("POST", "/api/symbol-mappings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated && w.Code != http.StatusConflict {
			t.Fatalf("symbol mapping %s: %d", sym, w.Code)
		}
	}

	return db, router, p.ID, a.ID
}

// createTx creates a transaction via the API.
// Note: quantity, priceCents, netCashCents are passed as JSON numbers.
// The API parses them as decimal.Decimal at face value (e.g., 1000 → 1000.00).
// Price and net_cash use "cents" convention: priceCents=10000 → price=100.00.
func createTx(t *testing.T, router http.Handler, accountID int64, date, txType, symbol, currency string, quantity, priceCents, netCashCents int64) {
	t.Helper()
	body := fmt.Sprintf(
		`{"account_id":%d,"date":"%s","type":"%s","symbol":"%s","quantity":%d,"price":%d,"currency":"%s","net_cash":%d}`,
		accountID, date, txType, symbol, quantity, priceCents, currency, netCashCents,
	)
	req := httptest.NewRequest("POST", "/api/transactions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("tx %s %s: %d %s", txType, symbol, w.Code, w.Body.String())
	}
}

// getPerformance hits the performance API and decodes the result.
func getPerformance(t *testing.T, router http.Handler, portfolioID int64) position.PerformanceResult {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/performance?portfolio_id="+fmt.Sprintf("%d", portfolioID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("performance API: %d %s", w.Code, w.Body.String())
	}
	var result position.PerformanceResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return result
}

// assertTWRInRange checks TWRPct is non-nil and within [minPct, maxPct].
func assertTWRInRange(t *testing.T, got *decimal.Decimal, minPct, maxPct float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("TWRPct is nil, want value in [%.2f%%, %.2f%%]", minPct, maxPct)
	}
	v, _ := got.Float64()
	if v < minPct || v > maxPct {
		t.Errorf("TWRPct = %s%%, want [%.2f%%, %.2f%%]", got.String(), minPct, maxPct)
	}
}

// ---------------------------------------------------------------------------
// Test 1: Single-currency baseline (USD, no FX needed)
//
//   Day 1: deposit $1000 + buy 10 USDSTK @$100 → 10 shares, cash=$0
//   Day 3: deposit $1000                        → cash=$1000, 10 shares (CASH FLOW)
//   Day 4+: equity curve extends to today with forward-filled prices
//
// Prices: day1=$100, day2=$105, day3=$110, day4=$108 (forward-filled thereafter)
//
// Equity curve: day1=1000, day3(post-deposit)=2100, end=2080 (10×$108 + $1000 cash)
//
// TWR periods:
//   Period 1 (day1→day3): 1100/1000 - 1 = 10%
//   Period 2 (day3→end): 2080/2100 - 1 ≈ -0.95%
//   TWR = 1.10 × 0.9905 - 1 ≈ 8.95%
// Range: 8%–10%
// ---------------------------------------------------------------------------

func TestPerformance_TWR_SingleCurrency(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "USD", "USD")

	// Day 1: deposit + buy on same day (portfolio starts fully invested)
	createTx(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-01", "buy", "USDSTK", "USD", 10, 10000, -1000)
	// Day 3: additional deposit (cash flow breakpoint)
	createTx(t, router, accountID, "2025-01-03", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)

	insertMarketData(t, db, map[string][]market.HistoricalPrice{
		"USDSTK": {
			histPrice("2025-01-01", 10000, "USD"),
			histPrice("2025-01-02", 10500, "USD"),
			histPrice("2025-01-03", 11000, "USD"),
			histPrice("2025-01-04", 10800, "USD"),
		},
	}, nil)

	result := getPerformance(t, router, portfolioID)
	assertTWRInRange(t, result.ReturnMetrics.TWRPct, 8.0, 10.0)
	if result.ReturnMetrics.AnnualizedTWRPct == nil {
		t.Fatal("AnnualizedTWRPct is nil")
	}
	t.Logf("TWR=%s%%, AnnTWR=%s%%, points=%d",
		result.ReturnMetrics.TWRPct.String(), result.ReturnMetrics.AnnualizedTWRPct.String(), len(result.EquityCurve))
}

// ---------------------------------------------------------------------------
// Test 2: No additional cash flows — TWR == simple return
//
//   Day 1: deposit $1000 + buy 10 USDSTK @$100 → 10 shares, cash=$0
//   Day 4: end
//
// No cash flows after initial investment.
// TWR = simple return = (1080 - 1000) / 1000 = 8%
// Range: 7%–9%
// ---------------------------------------------------------------------------

func TestPerformance_TWR_NoCashFlows(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "USD", "USD")

	// Day 1: deposit + buy on same day
	createTx(t, router, accountID, "2025-01-01", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-01", "buy", "USDSTK", "USD", 10, 10000, -1000)

	insertMarketData(t, db, map[string][]market.HistoricalPrice{
		"USDSTK": {
			histPrice("2025-01-01", 10000, "USD"),
			histPrice("2025-01-02", 10500, "USD"),
			histPrice("2025-01-04", 10800, "USD"),
		},
	}, nil)

	result := getPerformance(t, router, portfolioID)
	assertTWRInRange(t, result.ReturnMetrics.TWRPct, 7.0, 9.0)
	t.Logf("TWR=%s%%, points=%d", result.ReturnMetrics.TWRPct.String(), len(result.EquityCurve))
}

// ---------------------------------------------------------------------------
// Test 3: Multi-currency happy path (GBP portfolio, GBP stock + USD cash)
//
//   Day 1: deposit £1000 + buy 10 GBPSTK @£100 → 10 shares, cash=£0
//   Day 3: deposit $1000                        → cash=£800 (at 0.80 USD/GBP (1 USD = 0.80 GBP)), 10 shares
//   Day 4+: equity curve extends to today with forward-filled prices
//
// GBPSTK: day1=£100, day2=£105, day3=£110, day4=£108 (forward-filled thereafter)
// FX (USD/GBP): 0.80 for all days
//
// Equity curve: day1=1000, day3(post-deposit)=1900, end=1880 (10×£108 + £800 cash)
//
// TWR periods:
//   Period 1 (day1→day3): 1100/1000 - 1 = 10%
//   Period 2 (day3→end): 1880/1900 - 1 ≈ -1.05%
//   TWR = 1.10 × 0.9895 - 1 ≈ 8.84%
// Range: 8%–10%
// ---------------------------------------------------------------------------

func TestPerformance_TWR_MultiCurrencyHappyPath(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	// Day 1: deposit + buy in GBP
	createTx(t, router, accountID, "2025-01-01", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-01", "buy", "GBPSTK", "GBP", 10, 10000, -1000)
	// Day 3: deposit in USD (foreign currency cash flow)
	createTx(t, router, accountID, "2025-01-03", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)

	insertMarketData(t, db,
		map[string][]market.HistoricalPrice{
			"GBPSTK": {
				histPrice("2025-01-01", 10000, "GBP"),
				histPrice("2025-01-02", 10500, "GBP"),
				histPrice("2025-01-03", 11000, "GBP"),
				histPrice("2025-01-04", 10800, "GBP"),
			},
		},
		map[string][]market.HistoricalPrice{
			"USD/GBP": {
				histPrice("2025-01-01", 80, "GBP"),
				histPrice("2025-01-02", 80, "GBP"),
				histPrice("2025-01-03", 80, "GBP"),
				histPrice("2025-01-04", 80, "GBP"),
			},
		},
	)

	result := getPerformance(t, router, portfolioID)
	assertTWRInRange(t, result.ReturnMetrics.TWRPct, 8.0, 10.0)
	if len(result.EquityCurve) == 0 {
		t.Fatal("equity curve is empty")
	}
	t.Logf("TWR=%s%%, points=%d", result.ReturnMetrics.TWRPct.String(), len(result.EquityCurve))
}

// ---------------------------------------------------------------------------
// Test 4: Missing FX rate in the middle of the period
//
// Same as test 3, but FX rate for day 2 is missing.
// The system should forward-fill from day 1.
// Since no USD cash exists on day 2, the missing rate doesn't affect
// the equity curve. TWR should be the same as the happy path.
// ---------------------------------------------------------------------------

func TestPerformance_TWR_MissingFXMiddle(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	createTx(t, router, accountID, "2025-01-01", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-01", "buy", "GBPSTK", "GBP", 10, 10000, -1000)
	createTx(t, router, accountID, "2025-01-03", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)

	insertMarketData(t, db,
		map[string][]market.HistoricalPrice{
			"GBPSTK": {
				histPrice("2025-01-01", 10000, "GBP"),
				histPrice("2025-01-02", 10500, "GBP"),
				histPrice("2025-01-03", 11000, "GBP"),
				histPrice("2025-01-04", 10800, "GBP"),
			},
		},
		map[string][]market.HistoricalPrice{
			"USD/GBP": {
				histPrice("2025-01-01", 80, "GBP"),
				// day 2 missing — no USD cash on day 2, so no conversion needed
				histPrice("2025-01-03", 80, "GBP"),
				histPrice("2025-01-04", 80, "GBP"),
			},
		},
	)

	result := getPerformance(t, router, portfolioID)
	assertTWRInRange(t, result.ReturnMetrics.TWRPct, 8.0, 10.0)
	t.Logf("TWR=%s%%, points=%d", result.ReturnMetrics.TWRPct.String(), len(result.EquityCurve))
}

// ---------------------------------------------------------------------------
// Test 5: Missing FX rate at the beginning (before USD cash arrives)
//
// FX rate for day 1–2 is missing. USD cash only arrives on day 3.
// The system should still work because FX is only needed when
// USD cash actually exists (day 3+).
//
// This tests that missing FX before foreign cash arrives doesn't
// cause silent currency mixing or incorrect values.
// ---------------------------------------------------------------------------

func TestPerformance_TWR_MissingFXBeginning(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	createTx(t, router, accountID, "2025-01-01", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-01", "buy", "GBPSTK", "GBP", 10, 10000, -1000)
	createTx(t, router, accountID, "2025-01-03", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)

	insertMarketData(t, db,
		map[string][]market.HistoricalPrice{
			"GBPSTK": {
				histPrice("2025-01-01", 10000, "GBP"),
				histPrice("2025-01-02", 10500, "GBP"),
				histPrice("2025-01-03", 11000, "GBP"),
				histPrice("2025-01-04", 10800, "GBP"),
			},
		},
		map[string][]market.HistoricalPrice{
			"USD/GBP": {
				// day 1–2 missing — no USD cash yet, should be fine
				histPrice("2025-01-03", 80, "GBP"),
				histPrice("2025-01-04", 80, "GBP"),
			},
		},
	)

	result := getPerformance(t, router, portfolioID)
	// TWR should be in the same range as happy path since FX is available
	// when USD cash actually exists.
	assertTWRInRange(t, result.ReturnMetrics.TWRPct, 8.0, 10.0)
	t.Logf("TWR=%s%%, warnings=%v", result.ReturnMetrics.TWRPct.String(), result.Warnings)
}

// ---------------------------------------------------------------------------
// Test 6: Missing FX rate at the end of the period
//
// FX rates for day 3–4 are missing.
// The system should forward-fill the last known rate (day 2).
// TWR should still be reasonable (same as happy path with constant rate).
// ---------------------------------------------------------------------------

func TestPerformance_TWR_MissingFXEnd(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	createTx(t, router, accountID, "2025-01-01", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-01", "buy", "GBPSTK", "GBP", 10, 10000, -1000)
	createTx(t, router, accountID, "2025-01-03", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)

	insertMarketData(t, db,
		map[string][]market.HistoricalPrice{
			"GBPSTK": {
				histPrice("2025-01-01", 10000, "GBP"),
				histPrice("2025-01-02", 10500, "GBP"),
				histPrice("2025-01-03", 11000, "GBP"),
				histPrice("2025-01-04", 10800, "GBP"),
			},
		},
		map[string][]market.HistoricalPrice{
			"USD/GBP": {
				histPrice("2025-01-01", 80, "GBP"),
				histPrice("2025-01-02", 80, "GBP"),
				// day 3–4 missing — should forward-fill from day 2
			},
		},
	)

	result := getPerformance(t, router, portfolioID)
	assertTWRInRange(t, result.ReturnMetrics.TWRPct, 8.0, 10.0)
	t.Logf("TWR=%s%%, points=%d", result.ReturnMetrics.TWRPct.String(), len(result.EquityCurve))
}

// ---------------------------------------------------------------------------
// Test 7: Multi-currency with both cash and stock in foreign currency
//
// GBP portfolio with:
//   - GBP cash deposit + GBP stock purchase (day 1)
//   - USD cash deposit (day 3)
//   - USD stock purchase (day 4)
//
// All FX rates present. Verifies the system handles mixed currencies
// across both cash balances and stock positions.
//
//   Day 1: deposit £1000 + buy 10 GBPSTK @£100 → 10 GBP shares, cash=£0
//   Day 3: deposit $1000                        → cash=$1000=£800, 10 GBP shares
//   Day 4: buy 10 USDSTK @$100                  → cash=$0, 10 GBP shares + 10 USD shares
//   Day 5+: equity curve extends to today with forward-filled prices
//
// GBPSTK: day1=£100, day3=£110, day5=£108 (forward-filled thereafter)
// USDSTK: day4=$100, day5=$102 (forward-filled thereafter)
// FX (USD/GBP): 0.80 for all days
//
// Equity curve: day1=1000, day3(post)=1900, day4(post-buy)=1900, end≈1896
//
// TWR breakpoints (deposits):
//   day1: pre=0 (initial deposit)
//   day3: pre=1100 (GBP position before USD deposit)
//
// TWR periods:
//   Period 1 (day1→day3): 1100/1000 - 1 = 10%
//   Period 2 (day3→end): ~1896/1900 - 1 ≈ -0.21%
//   TWR ≈ 9.79%
// Range: 8%–11%
// ---------------------------------------------------------------------------

func TestPerformance_TWR_MultiCurrencyCashAndStock(t *testing.T) {
	db, router, portfolioID, accountID := setupPerf(t, "GBP", "GBP")

	// Day 1: deposit + buy in GBP
	createTx(t, router, accountID, "2025-01-01", "deposit", "$CASH-GBP", "GBP", 1000, 1, 1000)
	createTx(t, router, accountID, "2025-01-01", "buy", "GBPSTK", "GBP", 10, 10000, -1000)
	// Day 3: deposit in USD
	createTx(t, router, accountID, "2025-01-03", "deposit", "$CASH-USD", "USD", 1000, 1, 1000)
	// Day 4: buy USD stock
	createTx(t, router, accountID, "2025-01-04", "buy", "USDSTK", "USD", 10, 10000, -1000)

	insertMarketData(t, db,
		map[string][]market.HistoricalPrice{
			"GBPSTK": {
				histPrice("2025-01-01", 10000, "GBP"),
				histPrice("2025-01-03", 11000, "GBP"),
				histPrice("2025-01-05", 10800, "GBP"),
			},
			"USDSTK": {
				histPrice("2025-01-04", 10000, "USD"),
				histPrice("2025-01-05", 10200, "USD"),
			},
		},
		map[string][]market.HistoricalPrice{
			"USD/GBP": {
				histPrice("2025-01-01", 80, "GBP"),
				histPrice("2025-01-03", 80, "GBP"),
				histPrice("2025-01-04", 80, "GBP"),
				histPrice("2025-01-05", 80, "GBP"),
			},
		},
	)

	result := getPerformance(t, router, portfolioID)
	assertTWRInRange(t, result.ReturnMetrics.TWRPct, 8.0, 11.0)
	if len(result.EquityCurve) == 0 {
		t.Fatal("equity curve is empty")
	}
	t.Logf("TWR=%s%%, points=%d", result.ReturnMetrics.TWRPct.String(), len(result.EquityCurve))
}

// TestBenchmarkChartRespectsPeriod verifies that the benchmark chart data
// in the web response is clipped to the selected period, not showing data
// from before the period start (e.g., "5Y" shouldn't show from 2000).
func TestBenchmarkChartRespectsPeriod(t *testing.T) {
	db, router, portfolioID, _ := setupPerf(t, "USD", "USD")

	// Create a buy transaction in 2024 (within the 5Y window)
	createTx(t, router, portfolioID, "2024-06-15", "buy", "USDSTK", "USD", 100, 10000, 1000000)

	// Insert benchmark data spanning 2000-now (wider than 5Y)
	var benchPrices []market.HistoricalPrice
	for y := 2000; y <= 2026; y++ {
		for m := 1; m <= 12; m++ {
			d := time.Date(y, time.Month(m), 15, 0, 0, 0, 0, time.UTC)
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			price := 1000 + (y-2000)*200 // increases over time
			benchPrices = append(benchPrices, histPrice(d.Format("2006-01-02"), int64(price*100), "USD"))
		}
	}
	insertMarketData(t, db,
		map[string][]market.HistoricalPrice{
			"USDSTK": {
				histPrice("2024-06-15", 10000, "USD"),
				histPrice("2024-12-31", 11000, "USD"),
			},
			"^GSPC": benchPrices,
		},
		map[string][]market.HistoricalPrice{},
	)

	// Hit /performance with period=5Y and benchmark
	req := httptest.NewRequest("GET", "/performance?portfolio_id="+fmt.Sprintf("%d", portfolioID)+"&period=5Y&benchmark=^GSPC", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("performance page: %d %s", w.Code, w.Body.String())
	}

	body := w.Body.String()

	// Extract benchmark chart data from the page (it's embedded as JSON in a script tag)
	// Look for benchRaw = JSON.parse('...') pattern
	const startMarker = "benchRaw = JSON.parse('"
	startIdx := bytes.Index([]byte(body), []byte(startMarker))
	if startIdx == -1 {
		t.Fatal("could not find benchmark chart data in page")
	}

	// Find the closing single-quote
	jsonStart := startIdx + len(startMarker)
	jsonEnd := bytes.IndexByte([]byte(body)[jsonStart:], 0x27)
	if jsonEnd == -1 {
		t.Fatal("could not find end of benchmark JSON")
	}
	benchmarkJSON := body[jsonStart : jsonStart+jsonEnd]

	// Template escapes JSON for HTML embedding (e.g., \u0022 for "), unescape it
	benchmarkJSON = strings.ReplaceAll(benchmarkJSON, `\u0022`, `"`)
	benchmarkJSON = strings.ReplaceAll(benchmarkJSON, `\u0027`, `'`)

	// Parse and check dates
	var benchData []struct{ Date string }
	if err := json.Unmarshal([]byte(benchmarkJSON), &benchData); err != nil {
		t.Fatalf("parse benchmark JSON: %v (json: %s)", err, benchmarkJSON[:min(200, len(benchmarkJSON))])
	}

	if len(benchData) == 0 {
		t.Fatal("benchmark chart data is empty")
	}

	// 5Y from now (2026) should start around 2021
	fiveYearsAgo := time.Now().UTC().AddDate(-5, 0, 0)

	// First point should be >= 5 years ago
	firstDate, err := time.Parse("2006-01-02", benchData[0].Date)
	if err != nil {
		t.Fatalf("parse first date %s: %v", benchData[0].Date, err)
	}
	if firstDate.Before(fiveYearsAgo) {
		t.Errorf("benchmark chart starts at %s which is before 5Y cutoff %s (chart shows data outside selected period)",
			benchData[0].Date, fiveYearsAgo.Format("2006-01-02"))
	}

	// Last point should be <= now
	lastDate, err := time.Parse("2006-01-02", benchData[len(benchData)-1].Date)
	if err != nil {
		t.Fatalf("parse last date %s: %v", benchData[len(benchData)-1].Date, err)
	}
	if lastDate.After(time.Now().UTC()) {
		t.Errorf("benchmark chart ends at %s which is after now", benchData[len(benchData)-1].Date)
	}

	t.Logf("benchmark chart: %d points, first=%s, last=%s, 5Y cutoff=%s",
		len(benchData), benchData[0].Date, benchData[len(benchData)-1].Date, fiveYearsAgo.Format("2006-01-02"))
}

// TestBenchmarkChartPeriodLongerThanPortfolio verifies that when the selected
// period (e.g. 5Y) is longer than the portfolio's actual date range (e.g. 1Y),
// the benchmark chart is clipped to the selected period, not extended to 2000.
func TestBenchmarkChartPeriodLongerThanPortfolio(t *testing.T) {
	db, router, portfolioID, _ := setupPerf(t, "USD", "USD")

	// Portfolio only has 1 year of data (2025)
	createTx(t, router, portfolioID, "2025-01-15", "buy", "USDSTK", "USD", 100, 10000, 1000000)

	// Benchmark data spans 2000-now
	var benchPrices []market.HistoricalPrice
	for y := 2000; y <= 2026; y++ {
		for m := 1; m <= 12; m++ {
			d := time.Date(y, time.Month(m), 15, 0, 0, 0, 0, time.UTC)
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			price := 1000 + (y-2000)*200
			benchPrices = append(benchPrices, histPrice(d.Format("2006-01-02"), int64(price*100), "USD"))
		}
	}
	insertMarketData(t, db,
		map[string][]market.HistoricalPrice{
			"USDSTK": {
				histPrice("2025-01-15", 10000, "USD"),
				histPrice("2025-12-31", 11000, "USD"),
			},
			"^GSPC": benchPrices,
		},
		map[string][]market.HistoricalPrice{},
	)

	// Test with "All" period — should show from 2000 (that's the filter)
	req := httptest.NewRequest("GET", "/performance?portfolio_id="+fmt.Sprintf("%d", portfolioID)+"&period=All&benchmark=^GSPC", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("performance All: %d %s", w.Code, w.Body.String())
	}
	benchData := extractBenchmarkData(t, w.Body.String())
	if len(benchData) == 0 {
		t.Fatal("benchmark chart data is empty for All period")
	}
	// "All" should include 2000 data
	firstDate, _ := time.Parse("2006-01-02", benchData[0].Date)
	if firstDate.Year() >= 2010 {
		t.Errorf("All period: first point %s should be near 2000, got year %d", benchData[0].Date, firstDate.Year())
	}
	t.Logf("All period: %d points, first=%s", len(benchData), benchData[0].Date)

	// Test with "1Y" period — should NOT show from 2000
	req = httptest.NewRequest("GET", "/performance?portfolio_id="+fmt.Sprintf("%d", portfolioID)+"&period=1Y&benchmark=^GSPC", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("performance 1Y: %d %s", w.Code, w.Body.String())
	}
	benchData = extractBenchmarkData(t, w.Body.String())
	if len(benchData) == 0 {
		t.Fatal("benchmark chart data is empty for 1Y period")
	}
	oneYearAgo := time.Now().UTC().AddDate(-1, 0, 0)
	firstDate, _ = time.Parse("2006-01-02", benchData[0].Date)
	if firstDate.Before(oneYearAgo) {
		t.Errorf("1Y period: first point %s is before 1Y cutoff %s", benchData[0].Date, oneYearAgo.Format("2006-01-02"))
	}
	t.Logf("1Y period: %d points, first=%s, cutoff=%s", len(benchData), benchData[0].Date, oneYearAgo.Format("2006-01-02"))
}

func extractBenchmarkData(t *testing.T, body string) []struct {
	Date  string
	Price string
} {
	t.Helper()
	const startMarker = "benchRaw = JSON.parse('"
	startIdx := bytes.Index([]byte(body), []byte(startMarker))
	if startIdx == -1 {
		t.Fatal("could not find benchmark chart data in page")
	}
	jsonStart := startIdx + len(startMarker)
	jsonEnd := bytes.IndexByte([]byte(body)[jsonStart:], 0x27)
	if jsonEnd == -1 {
		t.Fatal("could not find end of benchmark JSON")
	}
	benchmarkJSON := body[jsonStart : jsonStart+jsonEnd]
	benchmarkJSON = strings.ReplaceAll(benchmarkJSON, `\u0022`, `"`)
	benchmarkJSON = strings.ReplaceAll(benchmarkJSON, `\u0027`, `'`)

	var benchData []struct {
		Date  string
		Price string
	}
	if err := json.Unmarshal([]byte(benchmarkJSON), &benchData); err != nil {
		t.Fatalf("parse benchmark JSON: %v (json: %s)", err, benchmarkJSON[:min(200, len(benchmarkJSON))])
	}
	return benchData
}
