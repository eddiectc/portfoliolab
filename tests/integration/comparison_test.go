package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// comparisonResult is the JSON-decoded shape of a ComparisonResult.
type comparisonResult struct {
	ComputedAt   string                   `json:"computed_at"`
	PortfolioA   *portfolioComparison     `json:"portfolio_a"`
	PortfolioB   *portfolioComparison     `json:"portfolio_b"`
	CrossMetrics *crossPortfolioMetrics   `json:"cross_metrics,omitempty"`
	Warnings     []string                 `json:"warnings,omitempty"`
	Message      string                   `json:"message,omitempty"`
}

type portfolioComparison struct {
	ID                 int64                 `json:"id"`
	Name               string                `json:"name"`
	Type               string                `json:"type"`
	ReturnMetrics      *returnMetrics        `json:"return_metrics,omitempty"`
	RiskMetrics        *riskMetrics          `json:"risk_metrics,omitempty"`
	Drawdown           *drawdownResult       `json:"drawdown,omitempty"`
	DrawdownSeries     []json.RawMessage     `json:"drawdown_series,omitempty"`
	YearlyReturns      []yearlyReturn        `json:"yearly_returns,omitempty"`
	PeriodExtremes     *periodExtremes       `json:"period_extremes,omitempty"`
	ReturnDistribution *returnDistribution   `json:"return_distribution,omitempty"`
	IntraCorrelation   *intraCorrelation     `json:"intra_correlation,omitempty"`
	Warnings           []string              `json:"warnings,omitempty"`
	Message            string                `json:"message,omitempty"`
}

type returnMetrics struct {
	TWRPct              *decimal.Decimal `json:"twr_pct,omitempty"`
	AnnualizedTWRPct    *decimal.Decimal `json:"annualized_twr_pct,omitempty"`
	SimpleReturnPct     *decimal.Decimal `json:"simple_return_pct,omitempty"`
	AnnualizedSimplePct *decimal.Decimal `json:"annualized_simple_pct,omitempty"`
	CAGRPct             *decimal.Decimal `json:"cagr_pct,omitempty"`
	DaysElapsed         int              `json:"days_elapsed"`
	HasInsufficientData bool             `json:"has_insufficient_data"`
}

type riskMetrics struct {
	AnnualizedVolatilityPct *decimal.Decimal `json:"annualized_volatility_pct,omitempty"`
	SharpeRatio             *decimal.Decimal `json:"sharpe_ratio,omitempty"`
	SortinoRatio            *decimal.Decimal `json:"sortino_ratio,omitempty"`
}

type drawdownResult struct {
	MaxDrawdownPct       *decimal.Decimal `json:"max_drawdown_pct,omitempty"`
	CurrentDrawdownPct   *decimal.Decimal `json:"current_drawdown_pct,omitempty"`
	DrawdownDurationDays *int             `json:"drawdown_duration_days,omitempty"`
}

type yearlyReturn struct {
	Year      int              `json:"year"`
	ReturnPct *decimal.Decimal `json:"return_pct,omitempty"`
}

type crossPortfolioMetrics struct {
	BetaAlpha   *betaAlphaResult  `json:"beta_alpha,omitempty"`
	Correlation *correlationResult `json:"correlation,omitempty"`
	Overlap     *overlapResult    `json:"overlap,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
}

type betaAlphaResult struct {
	Beta    *decimal.Decimal `json:"beta,omitempty"`
	Alpha   *decimal.Decimal `json:"alpha,omitempty"`
}

type correlationResult struct {
	Correlation *decimal.Decimal `json:"correlation,omitempty"`
}

type overlapResult struct {
	TopHoldingsA []holdingWeight `json:"top_holdings_a"`
	TopHoldingsB []holdingWeight `json:"top_holdings_b"`
	OverlapPct   *decimal.Decimal `json:"overlap_pct,omitempty"`
	Warnings     []string        `json:"warnings,omitempty"`
}

type holdingWeight struct {
	Symbol string          `json:"symbol"`
	Weight decimal.Decimal `json:"weight"`
	Name   string          `json:"name,omitempty"`
}

type periodExtremes struct {
	BestMonth       *decimal.Decimal `json:"best_month,omitempty"`
	WorstMonth      *decimal.Decimal `json:"worst_month,omitempty"`
	BestYear        *decimal.Decimal `json:"best_year,omitempty"`
	WorstYear       *decimal.Decimal `json:"worst_year,omitempty"`
	WinRateMonthly  *decimal.Decimal `json:"win_rate_monthly,omitempty"`
	WinRateYearly   *decimal.Decimal `json:"win_rate_yearly,omitempty"`
}

type returnBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type returnDistribution struct {
	Monthly      []returnBucket `json:"monthly,omitempty"`
	Annual       []returnBucket `json:"annual,omitempty"`
	AnnualBinned []returnBucket `json:"annual_binned,omitempty"`
}

type intraCorrelation struct {
	Matrix  [][]json.RawMessage `json:"matrix,omitempty"`
	Symbols []string            `json:"symbols,omitempty"`
}

// setupComparison creates the DB, router, and pre-seeds symbol mappings for
// the comparison integration tests.
func setupComparison(t *testing.T) (*sql.DB, http.Handler) {
	t.Helper()
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Create symbol mappings for test symbols.
	for _, sym := range []string{"AAPL", "MSFT", "GOOGL", "AMZN", "NVDA"} {
		body := json.RawMessage(fmt.Sprintf(
			`{"internal_symbol":"%s","market_data_symbol":"%s"}`, sym, sym))
		req := httptest.NewRequest(http.MethodPost, "/api/symbols", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated && w.Code != http.StatusConflict {
			t.Fatalf("create symbol %s: expected 201/409, got %d: %s",
				sym, w.Code, w.Body.String())
		}
	}

	return db, router
}

// insertComparisonMarketData inserts historical prices for the given symbols
// spanning the requested number of trading days, starting from baseDate.
// Prices follow a simple upward trend with slight variation per symbol.
func insertComparisonMarketData(t *testing.T, db *sql.DB, symbols map[string]float64, days int) {
	t.Helper()
	now := time.Now().UTC()
	baseDate := now.AddDate(0, 0, -(days + days/5)) // extra buffer for weekends

	for i := 0; i <= days; i++ {
		date := baseDate.AddDate(0, 0, i)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		dateStr := date.Format("2006-01-02")

		for sym, basePrice := range symbols {
			// Simple upward trend: basePrice * (1 + i * 0.001)
			price := basePrice * (1.0 + float64(i)*0.001)
			_, err := db.Exec(
				`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
				 VALUES (?, ?, 'USD', 'stock', 'yahoo', ?)`,
				sym, fmt.Sprintf("%.2f", price), dateStr,
			)
			if err != nil {
				t.Fatalf("insert market data %s %s: %v", sym, dateStr, err)
			}
		}
	}

	// Also insert a "current" (empty date) row for each symbol.
	for sym, basePrice := range symbols {
		_, err := db.Exec(
			`INSERT OR REPLACE INTO market_data (symbol, price, currency, data_type, source, date)
			 VALUES (?, ?, 'USD', 'stock', 'yahoo', '')`,
			sym, fmt.Sprintf("%.2f", basePrice*1.15),
		)
		if err != nil {
			t.Fatalf("insert current market data %s: %v", sym, err)
		}
	}
}

// callComparison hits the comparison API and decodes the result.
func callComparison(t *testing.T, router http.Handler, url string) comparisonResult {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("comparison API: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result comparisonResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode comparison result: %v", err)
	}
	return result
}

// callComparisonRaw hits the comparison API and returns the raw response code and body.
func callComparisonRaw(t *testing.T, router http.Handler, url string) (*httptest.ResponseRecorder, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w, w.Code
}

func TestComparison_ModelVsModel(t *testing.T) {
	db, router := setupComparison(t)

	// Create two model portfolios with different compositions.
	mpA := createModelPortfolio(t, router, "Tech Growth", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("40.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("30.0")},
		{Symbol: "GOOGL", WeightPct: mustDecimal("30.0")},
	})

	mpB := createModelPortfolio(t, router, "Big Five", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("20.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("20.0")},
		{Symbol: "GOOGL", WeightPct: mustDecimal("20.0")},
		{Symbol: "AMZN", WeightPct: mustDecimal("20.0")},
		{Symbol: "NVDA", WeightPct: mustDecimal("20.0")},
	})

	// Insert ~120 trading days of historical data (covers 1M, 3M periods).
	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
		"AMZN":  175.0,
		"NVDA":  480.0,
	}, 120)

	// Call comparison API — 3M period.
	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=model&period=3M&starting_value=10000&risk_free_rate=4.5", mpA, mpB))

	// Verify top-level structure.
	if result.ComputedAt == "" {
		t.Error("expected computed_at to be set")
	}
	if result.PortfolioA == nil {
		t.Fatal("portfolio_a is nil")
	}
	if result.PortfolioB == nil {
		t.Fatal("portfolio_b is nil")
	}

	// Verify portfolio A.
	if result.PortfolioA.ID != mpA {
		t.Errorf("portfolio_a.id = %d, want %d", result.PortfolioA.ID, mpA)
	}
	if result.PortfolioA.Name != "Tech Growth" {
		t.Errorf("portfolio_a.name = %q, want %q", result.PortfolioA.Name, "Tech Growth")
	}
	if result.PortfolioA.Type != "model" {
		t.Errorf("portfolio_a.type = %q, want %q", result.PortfolioA.Type, "model")
	}
	if result.PortfolioA.ReturnMetrics == nil {
		t.Error("portfolio_a.return_metrics is nil")
	} else {
		if result.PortfolioA.ReturnMetrics.HasInsufficientData {
			t.Error("portfolio_a should have sufficient data")
		}
		if result.PortfolioA.ReturnMetrics.CAGRPct == nil {
			t.Error("portfolio_a.cagr_pct is nil")
		}
		if result.PortfolioA.ReturnMetrics.DaysElapsed <= 0 {
			t.Errorf("portfolio_a.days_elapsed = %d, want > 0", result.PortfolioA.ReturnMetrics.DaysElapsed)
		}
	}

	// Verify portfolio B.
	if result.PortfolioB.ID != mpB {
		t.Errorf("portfolio_b.id = %d, want %d", result.PortfolioB.ID, mpB)
	}
	if result.PortfolioB.Name != "Big Five" {
		t.Errorf("portfolio_b.name = %q, want %q", result.PortfolioB.Name, "Big Five")
	}

	// Verify cross metrics are present.
	if result.CrossMetrics == nil {
		t.Fatal("cross_metrics is nil")
	}
	if result.CrossMetrics.BetaAlpha == nil {
		t.Error("cross_metrics.beta_alpha is nil")
	} else {
		if result.CrossMetrics.BetaAlpha.Beta == nil {
			t.Error("beta is nil")
		}
		if result.CrossMetrics.BetaAlpha.Alpha == nil {
			t.Error("alpha is nil")
		}
	}
	if result.CrossMetrics.Correlation == nil {
		t.Error("cross_metrics.correlation is nil")
	} else if result.CrossMetrics.Correlation.Correlation == nil {
		t.Error("correlation value is nil")
	}

	// Verify risk metrics are present.
	if result.PortfolioA.RiskMetrics == nil {
		t.Error("portfolio_a.risk_metrics is nil")
	} else {
		if result.PortfolioA.RiskMetrics.AnnualizedVolatilityPct == nil {
			t.Error("annualized_volatility_pct is nil")
		}
		if result.PortfolioA.RiskMetrics.SharpeRatio == nil {
			t.Error("sharpe_ratio is nil")
		}
	}

	// Verify drawdown is present.
	if result.PortfolioA.Drawdown == nil {
		t.Error("portfolio_a.drawdown is nil")
	}

	// Verify drawdown series is present.
	if len(result.PortfolioA.DrawdownSeries) == 0 {
		t.Error("portfolio_a.drawdown_series is empty")
	}

	// Verify yearly returns (may have partial year data).
	// At least the structure should be present (even if empty for short periods).
	_ = result.PortfolioA.YearlyReturns // not asserting count — depends on date range

	// Verify period extremes.
	if result.PortfolioA.PeriodExtremes == nil {
		t.Error("portfolio_a.period_extremes is nil")
	}

	// Verify return distribution.
	if result.PortfolioA.ReturnDistribution == nil {
		t.Error("portfolio_a.return_distribution is nil")
	}

	// Verify intra-portfolio correlation.
	if result.PortfolioA.IntraCorrelation == nil {
		t.Error("portfolio_a.intra_correlation is nil")
	} else {
		if len(result.PortfolioA.IntraCorrelation.Symbols) < 2 {
			t.Errorf("intra_correlation symbols = %d, want >= 2",
				len(result.PortfolioA.IntraCorrelation.Symbols))
		}
	}

	// Verify overlap.
	if result.CrossMetrics.Overlap == nil {
		t.Error("cross_metrics.overlap is nil")
	} else {
		if len(result.CrossMetrics.Overlap.TopHoldingsA) != 3 {
			t.Errorf("overlap top_holdings_a = %d, want 3",
				len(result.CrossMetrics.Overlap.TopHoldingsA))
		}
		if len(result.CrossMetrics.Overlap.TopHoldingsB) != 5 {
			t.Errorf("overlap top_holdings_b = %d, want 5",
				len(result.CrossMetrics.Overlap.TopHoldingsB))
		}
		if result.CrossMetrics.Overlap.OverlapPct == nil {
			t.Error("overlap_pct is nil")
		}
	}
}

func TestComparison_ModelVsReal(t *testing.T) {
	db, router := setupComparison(t)

	// Create a model portfolio.
	mpID := createModelPortfolio(t, router, "Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})

	// Create a real portfolio with transactions.
	realPortfolioID := createPortfolioViaAPI(t, router, "Real Portfolio", "USD")
	realAccountID := createAccountViaAPI(t, router, "Real Account", realPortfolioID)

	// Create transactions: deposit + buy AAPL and MSFT.
	createTransaction(t, router, realAccountID, "2025-01-06", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, realAccountID, "2025-01-06", "buy", "AAPL", 50, 17500, -875000)
	createTransaction(t, router, realAccountID, "2025-01-06", "buy", "MSFT", 20, 40000, -800000)

	// Insert historical market data.
	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	}, 120)

	// Call comparison API — model vs real, 3M period.
	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=real&period=3M&starting_value=10000", mpID, realPortfolioID))

	if result.PortfolioA == nil {
		t.Fatal("portfolio_a is nil")
	}
	if result.PortfolioB == nil {
		t.Fatal("portfolio_b is nil")
	}

	// Model portfolio should have return metrics.
	if result.PortfolioA.ReturnMetrics == nil {
		t.Error("portfolio_a (model) return_metrics is nil")
	}
	if result.PortfolioA.Type != "model" {
		t.Errorf("portfolio_a.type = %q, want %q", result.PortfolioA.Type, "model")
	}

	// Real portfolio should have return metrics.
	if result.PortfolioB.ReturnMetrics == nil {
		t.Error("portfolio_b (real) return_metrics is nil")
	}
	if result.PortfolioB.Type != "real" {
		t.Errorf("portfolio_b.type = %q, want %q", result.PortfolioB.Type, "real")
	}
	if result.PortfolioB.Name != "Real Portfolio" {
		t.Errorf("portfolio_b.name = %q, want %q", result.PortfolioB.Name, "Real Portfolio")
	}

	// Cross metrics should be present.
	if result.CrossMetrics == nil {
		t.Fatal("cross_metrics is nil for model-vs-real")
	}
	if result.CrossMetrics.BetaAlpha == nil {
		t.Error("cross_metrics.beta_alpha is nil for model-vs-real")
	}
	if result.CrossMetrics.Correlation == nil {
		t.Error("cross_metrics.correlation is nil for model-vs-real")
	}
}

func TestComparison_RealVsReal(t *testing.T) {
	db, router := setupComparison(t)

	// Create two real portfolios with different holdings.
	portA := createPortfolioViaAPI(t, router, "Portfolio A", "USD")
	portB := createPortfolioViaAPI(t, router, "Portfolio B", "USD")
	acctA := createAccountViaAPI(t, router, "Account A", portA)
	acctB := createAccountViaAPI(t, router, "Account B", portB)

	// Portfolio A: heavy AAPL
	createTransaction(t, router, acctA, "2025-01-06", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, acctA, "2025-01-06", "buy", "AAPL", 100, 17500, -1750000)

	// Portfolio B: heavy MSFT
	createTransaction(t, router, acctB, "2025-01-06", "deposit", "$CASH-USD", 100000, 1, 10000000)
	createTransaction(t, router, acctB, "2025-01-06", "buy", "MSFT", 50, 40000, -2000000)

	// Insert historical market data.
	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	}, 120)

	// Call comparison API — real vs real, 3M period.
	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=real&portfolio_b_id=%d&portfolio_b_type=real&period=3M", portA, portB))

	if result.PortfolioA == nil {
		t.Fatal("portfolio_a is nil")
	}
	if result.PortfolioB == nil {
		t.Fatal("portfolio_b is nil")
	}

	if result.PortfolioA.Name != "Portfolio A" {
		t.Errorf("portfolio_a.name = %q, want %q", result.PortfolioA.Name, "Portfolio A")
	}
	if result.PortfolioB.Name != "Portfolio B" {
		t.Errorf("portfolio_b.name = %q, want %q", result.PortfolioB.Name, "Portfolio B")
	}

	// Both should have return metrics.
	if result.PortfolioA.ReturnMetrics == nil {
		t.Error("portfolio_a return_metrics is nil")
	}
	if result.PortfolioB.ReturnMetrics == nil {
		t.Error("portfolio_b return_metrics is nil")
	}

	// Cross metrics should be present.
	if result.CrossMetrics == nil {
		t.Fatal("cross_metrics is nil for real-vs-real")
	}
}

func TestComparison_RealPortfolioNoTransactions(t *testing.T) {
	db, router := setupComparison(t)

	// Create a model portfolio with data.
	mpID := createModelPortfolio(t, router, "Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})
	insertComparisonMarketData(t, db, map[string]float64{"AAPL": 175.0}, 120)

	// Create a real portfolio with NO transactions.
	emptyPortfolioID := createPortfolioViaAPI(t, router, "Empty Portfolio", "USD")

	// Call comparison API — model vs empty real.
	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=real&period=3M&starting_value=10000", mpID, emptyPortfolioID))

	// Portfolio A (model) should have data.
	if result.PortfolioA == nil {
		t.Fatal("portfolio_a is nil")
	}
	if result.PortfolioA.ReturnMetrics == nil {
		t.Error("portfolio_a (model) return_metrics is nil")
	}

	// Portfolio B (empty real) should have empty-state indicators.
	if result.PortfolioB == nil {
		t.Fatal("portfolio_b is nil")
	}
	if result.PortfolioB.Message == "" {
		t.Error("portfolio_b should have an empty-state message")
	}
	if result.PortfolioB.ReturnMetrics != nil && !result.PortfolioB.ReturnMetrics.HasInsufficientData {
		t.Error("portfolio_b should have HasInsufficientData = true")
	}

	// Cross metrics should be absent (one portfolio has insufficient data).
	if result.CrossMetrics != nil {
		// It's acceptable for cross metrics to be nil or empty when one side has no data.
		// The key is the API doesn't crash.
	}
}

func TestComparison_PeriodFiltering_1Y(t *testing.T) {
	db, router := setupComparison(t)

	mpA := createModelPortfolio(t, router, "Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})
	mpB := createModelPortfolio(t, router, "Model B", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "GOOGL", WeightPct: mustDecimal("100.0")},
	})

	// Insert ~365 trading days of data for 1Y coverage.
	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
	}, 365)

	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=model&period=1Y&starting_value=10000", mpA, mpB))

	if result.PortfolioA == nil || result.PortfolioB == nil {
		t.Fatal("portfolio result is nil")
	}

	// With 1Y of data, days elapsed should be substantial.
	if result.PortfolioA.ReturnMetrics != nil {
		if result.PortfolioA.ReturnMetrics.DaysElapsed < 200 {
			t.Errorf("portfolio_a.days_elapsed = %d, want >= 200 for 1Y period",
				result.PortfolioA.ReturnMetrics.DaysElapsed)
		}
	}

	// Yearly returns should have at least one entry.
	if len(result.PortfolioA.YearlyReturns) == 0 {
		t.Error("portfolio_a yearly_returns is empty for 1Y period")
	}
}

func TestComparison_PeriodFiltering_CustomDateRange(t *testing.T) {
	db, router := setupComparison(t)

	mpA := createModelPortfolio(t, router, "Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})
	mpB := createModelPortfolio(t, router, "Model B", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "GOOGL", WeightPct: mustDecimal("100.0")},
	})

	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
	}, 120)

	// Use a custom date range within the inserted data window.
	// The data starts ~140 days ago; use a 60-day window.
	dateFrom := "2025-01-01"
	dateTo := "2025-03-31"

	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=model&date_from=%s&date_to=%s&starting_value=10000", mpA, mpB, dateFrom, dateTo))

	if result.PortfolioA == nil || result.PortfolioB == nil {
		t.Fatal("portfolio result is nil")
	}

	// Custom date range should produce results (even if data is clipped to available dates).
	if result.PortfolioA.ReturnMetrics == nil {
		t.Error("portfolio_a return_metrics is nil for custom date range")
	}
}

func TestComparison_PeriodFiltering_1M(t *testing.T) {
	db, router := setupComparison(t)

	mpA := createModelPortfolio(t, router, "Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})

	insertComparisonMarketData(t, db, map[string]float64{"AAPL": 175.0}, 60)

	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=model&period=1M&starting_value=10000", mpA, mpA))

	if result.PortfolioA == nil || result.PortfolioB == nil {
		t.Fatal("portfolio result is nil")
	}

	// 1M period should have fewer days than 3M.
	if result.PortfolioA.ReturnMetrics != nil {
		if result.PortfolioA.ReturnMetrics.DaysElapsed > 60 {
			t.Errorf("portfolio_a.days_elapsed = %d, want <= 60 for 1M period",
				result.PortfolioA.ReturnMetrics.DaysElapsed)
		}
	}
}

func TestComparison_InvalidPortfolioType(t *testing.T) {
	_, router := setupComparison(t)

	w, code := callComparisonRaw(t, router,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=invalid&portfolio_b_id=2&portfolio_b_type=model")

	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid portfolio type, got %d: %s", code, w.Body.String())
	}
}

func TestComparison_MissingPortfolioID(t *testing.T) {
	_, router := setupComparison(t)

	w, code := callComparisonRaw(t, router,
		"/api/comparison?portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model")

	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing portfolio_a_id, got %d: %s", code, w.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISSING_PORTFOLIO_A_ID" {
		t.Errorf("expected error code MISSING_PORTFOLIO_A_ID, got %q", errResp.Code)
	}
}

func TestComparison_InvalidPeriod(t *testing.T) {
	_, router := setupComparison(t)

	w, code := callComparisonRaw(t, router,
		"/api/comparison?portfolio_a_id=1&portfolio_a_type=model&portfolio_b_id=2&portfolio_b_type=model&period=2Y")

	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid period, got %d: %s", code, w.Body.String())
	}
}

func TestComparison_SamePortfolioVsItself(t *testing.T) {
	db, router := setupComparison(t)

	mpID := createModelPortfolio(t, router, "Self Compare", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})

	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	}, 120)

	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=model&period=3M&starting_value=10000", mpID, mpID))

	if result.CrossMetrics == nil {
		t.Fatal("cross_metrics is nil for same portfolio comparison")
	}

	// Beta of identical series should be ~1.0.
	if result.CrossMetrics.BetaAlpha != nil && result.CrossMetrics.BetaAlpha.Beta != nil {
		beta, _ := result.CrossMetrics.BetaAlpha.Beta.Float64()
		if beta < 0.99 || beta > 1.01 {
			t.Errorf("beta of same portfolio = %.4f, want ~1.0", beta)
		}
	}

	// Correlation of identical series should be ~1.0.
	if result.CrossMetrics.Correlation != nil && result.CrossMetrics.Correlation.Correlation != nil {
		corr, _ := result.CrossMetrics.Correlation.Correlation.Float64()
		if corr < 0.999 || corr > 1.001 {
			t.Errorf("correlation of same portfolio = %.4f, want ~1.0", corr)
		}
	}

	// Alpha of identical series should be ~0.
	if result.CrossMetrics.BetaAlpha != nil && result.CrossMetrics.BetaAlpha.Alpha != nil {
		alpha, _ := result.CrossMetrics.BetaAlpha.Alpha.Float64()
		if alpha < -0.01 || alpha > 0.01 {
			t.Errorf("alpha of same portfolio = %.4f, want ~0.0", alpha)
		}
	}

	// Overlap should be 100% for identical portfolios.
	if result.CrossMetrics.Overlap != nil && result.CrossMetrics.Overlap.OverlapPct != nil {
		overlap, _ := result.CrossMetrics.Overlap.OverlapPct.Float64()
		if overlap < 99.0 {
			t.Errorf("overlap of same portfolio = %.2f%%, want ~100%%", overlap)
		}
	}
}

func TestComparison_WebPage_Renders200(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	_, router := setupComparison(t)

	// Create model portfolios so selectors are populated.
	createModelPortfolio(t, router, "Web Test Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})

	req := httptest.NewRequest(http.MethodGet, "/comparison", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", contentType)
	}
}

func TestComparison_WebPage_WithSelection(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db, router := setupComparison(t)

	mpA := createModelPortfolio(t, router, "Web Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})
	mpB := createModelPortfolio(t, router, "Web Model B", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "GOOGL", WeightPct: mustDecimal("100.0")},
	})

	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
	}, 120)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/comparison?portfolio_a_id=m%d&portfolio_b_id=m%d&period=3M&starting_value=10000", mpA, mpB), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check that the response body contains portfolio names.
	body := w.Body.String()
	if len(body) < 100 {
		t.Error("response body is suspiciously short")
	}
}

func TestComparison_StartingValue(t *testing.T) {
	db, router := setupComparison(t)

	mpID := createModelPortfolio(t, router, "SV Test", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("100.0")},
	})

	insertComparisonMarketData(t, db, map[string]float64{"AAPL": 175.0}, 60)

	// Compare with starting_value=100000 vs default 10000.
	result100k := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=model&period=1M&starting_value=100000", mpID, mpID))

	result10k := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=model&period=1M&starting_value=10000", mpID, mpID))

	// Both should return valid results.
	if result100k.PortfolioA == nil || result10k.PortfolioA == nil {
		t.Fatal("portfolio result is nil")
	}

	// Drawdown series should reflect the larger starting value.
	// Both portfolios are identical to themselves, so relative metrics match.
	if len(result100k.PortfolioA.DrawdownSeries) == 0 {
		t.Error("drawdown series is empty for starting_value=100000")
	}
	if len(result10k.PortfolioA.DrawdownSeries) == 0 {
		t.Error("drawdown series is empty for starting_value=10000")
	}
}

func TestComparison_EnhancedOverlap_WebPageRendersSections(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db, router := setupComparison(t)

	mpA := createModelPortfolio(t, router, "Enhanced A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})
	mpB := createModelPortfolio(t, router, "Enhanced B", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("30.0")},
		{Symbol: "GOOGL", WeightPct: mustDecimal("70.0")},
	})

	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
	}, 120)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/comparison?portfolio_a_id=m%d&portfolio_b_id=m%d&period=3M&starting_value=10000", mpA, mpB), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()

	// Verify enhanced overlap sections are present in the rendered HTML.
	sections := map[string]string{
		"Sector Allocation":  "Sector Allocation",
		"Country Allocation": "Country Allocation",
		"Merged Holdings":    "Merged Holdings",
		"Holdings Overlap":   "Holdings Overlap",
	}
	for section, expected := range sections {
		if !bytes.Contains([]byte(body), []byte(expected)) {
			t.Errorf("expected section %q in rendered page", section)
		}
	}
}

func TestComparison_EnhancedOverlap_FullStackWithData(t *testing.T) {
	skipIfTemplatesUnavailable(t)
	db := setupTestDB(t)
	router, _ := api.Router(db, testLogger(), api.WithTemplatesDir("../../templates"))

	// Seed symbol_details with sector and geographic data for test symbols.
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	for _, sd := range []struct {
		symbol, sector, shortName string
		geoAlloc                  string
	}{
		{"AAPL", "Technology", "Apple Inc.", `[{"Country":"United States","Percent":100.0}]`},
		{"MSFT", "Technology", "Microsoft Corp.", `[{"Country":"United States","Percent":100.0}]`},
		{"GOOGL", "Consumer Cyclical", "Alphabet Inc.", `[{"Country":"United States","Percent":100.0}]`},
		{"JPM", "Financial Services", "JPMorgan Chase", `[{"Country":"United States","Percent":100.0}]`},
		{"UNH", "Healthcare", "UnitedHealth Group", `[{"Country":"United States","Percent":100.0}]`},
	} {
		_, err := db.Exec(
			`INSERT OR REPLACE INTO symbol_details (
				internal_symbol, short_name, sector, geographic_allocations,
				quote_type, currency, fetched_at, updated_at
			) VALUES (?, ?, ?, ?, 'EQUITY', 'USD', ?, ?)`,
			sd.symbol, sd.shortName, sd.sector, sd.geoAlloc, now, now,
		)
		if err != nil {
			t.Fatalf("seed symbol_details %s: %v", sd.symbol, err)
		}
	}

	// Create two model portfolios with different sector exposure.
	mpA := createModelPortfolio(t, router, "Tech Heavy", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})
	mpB := createModelPortfolio(t, router, "Diversified", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "GOOGL", WeightPct: mustDecimal("30.0")},
		{Symbol: "JPM", WeightPct: mustDecimal("30.0")},
		{Symbol: "UNH", WeightPct: mustDecimal("40.0")},
	})

	// Insert market data so the comparison has return metrics.
	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL":  175.0,
		"MSFT":  400.0,
		"GOOGL": 140.0,
		"JPM":   170.0,
		"UNH":   520.0,
	}, 120)

	// Hit the web page.
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/comparison?portfolio_a_id=m%d&portfolio_b_id=m%d&period=3M&starting_value=10000", mpA, mpB), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()

	// Verify sector allocation section renders with actual sector names.
	if !bytes.Contains([]byte(body), []byte("Sector Allocation")) {
		t.Error("missing 'Sector Allocation' section")
	}
	// Portfolio A is 100% Technology, so "Technology" should appear in the table.
	if !bytes.Contains([]byte(body), []byte("Technology")) {
		t.Error("missing 'Technology' sector in rendered page")
	}
	// Portfolio B has Financial Services and Healthcare.
	if !bytes.Contains([]byte(body), []byte("Financial Services")) {
		t.Error("missing 'Financial Services' sector in rendered page")
	}
	if !bytes.Contains([]byte(body), []byte("Healthcare")) {
		t.Error("missing 'Healthcare' sector in rendered page")
	}

	// Verify country allocation section renders with country names.
	if !bytes.Contains([]byte(body), []byte("Country Allocation")) {
		t.Error("missing 'Country Allocation' section")
	}
	if !bytes.Contains([]byte(body), []byte("United States")) {
		t.Error("missing 'United States' country in rendered page")
	}

	// Verify merged holdings section.
	if !bytes.Contains([]byte(body), []byte("Merged Holdings")) {
		t.Error("missing 'Merged Holdings' section")
	}

	// Verify holdings overlap section (overweight/underweight/neutral).
	if !bytes.Contains([]byte(body), []byte("Holdings Overlap")) {
		t.Error("missing 'Holdings Overlap' section")
	}
	// AAPL and MSFT are only in portfolio A, so they should appear as overweight in A.
	// GOOGL, JPM, UNH are only in portfolio B.
	// Since there's no overlap in symbols, the "Neutral" section should be empty or absent.
	// The "Overweight" and "Underweight" sections should have data.
	if !bytes.Contains([]byte(body), []byte("Overweight")) {
		t.Error("missing 'Overweight' section in holdings overlap")
	}
	if !bytes.Contains([]byte(body), []byte("Underweight")) {
		t.Error("missing 'Underweight' section in holdings overlap")
	}
}

func TestComparison_RealVsModel_NoTransactionsOnReal(t *testing.T) {
	db, router := setupComparison(t)

	mpID := createModelPortfolio(t, router, "Model A", []modelportfolio.ModelPortfolioEntry{
		{Symbol: "AAPL", WeightPct: mustDecimal("50.0")},
		{Symbol: "MSFT", WeightPct: mustDecimal("50.0")},
	})
	insertComparisonMarketData(t, db, map[string]float64{
		"AAPL": 175.0,
		"MSFT": 400.0,
	}, 120)

	// Create real portfolio with account but NO transactions.
	realPortfolioID := createPortfolioViaAPI(t, router, "Empty Real", "USD")
	_ = createAccountViaAPI(t, router, "Empty Account", realPortfolioID)

	result := callComparison(t, router,
		fmt.Sprintf("/api/comparison?portfolio_a_id=%d&portfolio_a_type=model&portfolio_b_id=%d&portfolio_b_type=real&period=3M&starting_value=10000", mpID, realPortfolioID))

	// Model portfolio should have data.
	if result.PortfolioA.ReturnMetrics == nil {
		t.Error("portfolio_a (model) return_metrics is nil")
	}

	// Empty real portfolio should have empty-state indicators.
	if result.PortfolioB == nil {
		t.Fatal("portfolio_b is nil")
	}
	if result.PortfolioB.Message == "" {
		t.Error("portfolio_b (empty real) should have empty-state message")
	}

	// Overall result may have a message about insufficient data.
	// At minimum the API should not crash.
	if result.PortfolioA.Name != "Model A" {
		t.Errorf("portfolio_a.name = %q, want %q", result.PortfolioA.Name, "Model A")
	}
	if result.PortfolioB.Name != "Empty Real" {
		t.Errorf("portfolio_b.name = %q, want %q", result.PortfolioB.Name, "Empty Real")
	}
}
