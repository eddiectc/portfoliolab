package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/comparison"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// cacheStatusProvider exposes the subset of MarketCache needed by web handlers.
type cacheStatusProvider interface {
	GetStatus() marketcache.CacheStatus
	RefreshAll(ctx context.Context)
}

// monthlyReturnData holds one row for the monthly return heatmap.
type monthlyReturnData struct {
	Year            int
	Month           int
	PortfolioReturn string
	BenchmarkReturn string
	Diff            string
}

// performancePageData is the data struct for the performance page template.
type performancePageData struct {
	web.PageData
	Result              *position.PerformanceResult
	ChartData           string // pre-serialized JSON for ECharts
	CurrentValue        string // last equity curve point's portfolio value
	CurrentNetDeposit   string // last equity curve point's net deposit
	Portfolios          []portfolio.Portfolio
	SelectedPeriod      string
	SelectedPortfolioID string
	Error               string
	// Cache status for the aggregate status indicator.
	CacheStatus     marketcache.CacheStatus
	HasCacheStatus  bool
	LastRefreshText string
	StaleSymbols    []string
	// Pre-built URLs for template safety (Go html/template is strict about expressions in URLs).
	RefreshURL  string
	PeriodURLs  map[string]string // period label -> full URL
	// Benchmark fields.
	SelectedBenchmark   string
	BenchmarkNames      map[string]string // ticker -> display name for dropdown
	BenchmarkTicker     string
	BenchmarkChartData  string // JSON-serialized benchmark prices for ECharts
	BenchmarkMWRPct     *decimal.Decimal
	BenchmarkCurrency   string
	BenchmarkWarning    string
	BenchmarkURLs       map[string]string // option label -> full URL
	MonthlyReturns      []monthlyReturnData
}

// PerformanceWebHandler handles server-rendered performance pages.
type PerformanceWebHandler struct {
	positionSvc   *position.Service
	portfolioSvc  *portfolio.Service
	marketCache   cacheStatusProvider
	marketService position.MarketDataService
	renderer      *web.Renderer
}

// NewPerformanceWebHandler creates a new performance web handler.
func NewPerformanceWebHandler(positionSvc *position.Service, portfolioSvc *portfolio.Service, marketCache cacheStatusProvider, marketService position.MarketDataService, renderer *web.Renderer) *PerformanceWebHandler {
	return &PerformanceWebHandler{
		positionSvc:   positionSvc,
		portfolioSvc:  portfolioSvc,
		marketCache:   marketCache,
		marketService: marketService,
		renderer:      renderer,
	}
}

// RegisterRoutes mounts web performance routes on the given router.
func (h *PerformanceWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/performance/refresh", h.HandleRefresh)
	r.Get("/performance", h.HandlePerformance)
}

// HandlePerformance renders GET /performance (equity curve chart + return metrics).
func (h *PerformanceWebHandler) HandlePerformance(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())

	// Validate benchmark if provided.
	benchmark := filters.Benchmark
	if benchmark != "" && !comparison.IsValidPredefined(benchmark) {
		// Silently drop invalid benchmark — just show no benchmark.
		benchmark = ""
	}

	// Resolve selected portfolio ID for UI state.
	var selectedPortfolioID string
	if filters.PortfolioID != nil {
		selectedPortfolioID = strconv.FormatInt(*filters.PortfolioID, 10)
	}

	result, err := h.positionSvc.ComputeEquityCurve(r.Context(), filters)
	if err != nil {
		// Cache status for aggregate indicator (available even on error).
		var cacheStatus marketcache.CacheStatus
		var hasCacheStatus bool
		var lastRefreshText string
		if h.marketCache != nil {
			cacheStatus = h.marketCache.GetStatus()
			hasCacheStatus = true
			if !cacheStatus.LastRefresh.IsZero() {
				lastRefreshText = formatLastRefresh(cacheStatus.LastRefresh)
			}
		}

		data := performancePageData{
			PageData:            web.PageData{Title: "Performance", Flash: getFlash(w, r)},
			Portfolios:          h.fetchPortfolios(r.Context()),
			SelectedPeriod:      filters.Period,
			SelectedPortfolioID: selectedPortfolioID,
			SelectedBenchmark:   benchmark,
			BenchmarkNames:      comparison.GetPredefined(),
			Error:               userFriendlyPerformanceError(err),
			CacheStatus:         cacheStatus,
			HasCacheStatus:      hasCacheStatus,
			LastRefreshText:     lastRefreshText,
			RefreshURL:          buildRefreshURL(selectedPortfolioID, benchmark),
			PeriodURLs:          buildPeriodURLs(selectedPortfolioID, filters.Period, benchmark),
			BenchmarkURLs:       buildBenchmarkURLs(benchmark, selectedPortfolioID, filters.Period),
		}
		if err := h.renderer.Render(w, "performance/index", data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Extract last point for summary display.
	var currentValue, currentNetDeposit string
	if len(result.EquityCurve) > 0 {
		last := result.EquityCurve[len(result.EquityCurve)-1]
		currentValue = last.PortfolioValue.String()
		currentNetDeposit = last.NetDeposit.String()
	}

	// Cache status for aggregate indicator.
	var cacheStatus marketcache.CacheStatus
	var hasCacheStatus bool
	var lastRefreshText string
	var staleSymbols []string
	if h.marketCache != nil {
		cacheStatus = h.marketCache.GetStatus()
		hasCacheStatus = true
		if !cacheStatus.LastRefresh.IsZero() {
			lastRefreshText = formatLastRefresh(cacheStatus.LastRefresh)
		}
		staleSymbols = extractStaleSymbols(result.Warnings)
	}

	// Fetch benchmark data if selected.
	var benchmarkChartData string
	var benchmarkMWRPct *decimal.Decimal
	var benchmarkCurrency, benchmarkWarning string
	var benchmarkPrices []market.HistoricalPrice
	if benchmark != "" && h.marketService != nil {
		benchmarkChartData, benchmarkMWRPct, benchmarkCurrency, benchmarkWarning, benchmarkPrices = h.fetchBenchmarkData(r.Context(), benchmark, filters)
	}

	// Compute monthly returns for heatmap.
	monthlyReturns := computeMonthlyReturnsFromCurve(result.EquityCurve, benchmarkPrices, benchmark)

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance", Flash: getFlash(w, r)},
		Result:              result,
		ChartData:           serializeChartData(result.EquityCurve),
		CurrentValue:        currentValue,
		CurrentNetDeposit:   currentNetDeposit,
		Portfolios:          h.fetchPortfolios(r.Context()),
		SelectedPeriod:      filters.Period,
		SelectedPortfolioID: selectedPortfolioID,
		CacheStatus:         cacheStatus,
		HasCacheStatus:      hasCacheStatus,
		LastRefreshText:     lastRefreshText,
		StaleSymbols:        staleSymbols,
		RefreshURL:          buildRefreshURL(selectedPortfolioID, benchmark),
		PeriodURLs:          buildPeriodURLs(selectedPortfolioID, filters.Period, benchmark),
		// Benchmark fields.
		SelectedBenchmark:   benchmark,
		BenchmarkNames:      comparison.GetPredefined(),
		BenchmarkTicker:     benchmark,
		BenchmarkChartData:  benchmarkChartData,
		BenchmarkMWRPct:     benchmarkMWRPct,
		BenchmarkCurrency:   benchmarkCurrency,
		BenchmarkWarning:    benchmarkWarning,
		BenchmarkURLs:       buildBenchmarkURLs(benchmark, selectedPortfolioID, filters.Period),
		MonthlyReturns:      monthlyReturns,
	}

	if err := h.renderer.Render(w, "performance/index", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// fetchBenchmarkData fetches cached benchmark prices, computes MWR, and
// serializes chart data for the template. Also returns raw prices for
// monthly return computation.
func (h *PerformanceWebHandler) fetchBenchmarkData(ctx context.Context, ticker string, filters position.PerformanceFilters) (chartData string, mwrPct *decimal.Decimal, currency, warning string, prices []market.HistoricalPrice) {
	dateFrom, dateTo := determineDateRange(filters)
	if dateFrom.IsZero() {
		dateFrom = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if dateTo.IsZero() {
		dateTo = time.Now().UTC()
	}

	prices, err := h.marketService.GetHistoricalPrices(ctx, ticker, dateFrom, dateTo)
	if err != nil {
		return "[]", nil, "", "failed to fetch benchmark data", nil
	}

	if len(prices) == 0 {
		return "[]", nil, "", "no cached data available for benchmark", nil
	}

	currency = prices[0].Currency

	mwrPct = comparison.ComputeMWRForPeriod(prices, dateFrom, dateTo)

	chartData = serializeBenchmarkChartData(prices)

	return chartData, mwrPct, currency, "", prices
}

// benchmarkChartDataPoint is the JSON-serializable format for ECharts benchmark series.
type benchmarkChartDataPoint struct {
	Date  string `json:"date"`
	Price string `json:"price"`
}

// serializeBenchmarkChartData converts benchmark historical prices to JSON for ECharts.
func serializeBenchmarkChartData(prices []market.HistoricalPrice) string {
	if len(prices) == 0 {
		return "[]"
	}
	data := make([]benchmarkChartDataPoint, len(prices))
	for i, p := range prices {
		data[i] = benchmarkChartDataPoint{
			Date:  p.Date.Format("2006-01-02"),
			Price: p.Close.String(),
		}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// HandleRefresh handles POST /performance/refresh (manual full market data refresh).
// Triggers a background refresh of ALL symbols via the market cache.
func (h *PerformanceWebHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if h.marketCache != nil {
		h.marketCache.RefreshAll(r.Context())
		setFlash(w, "Market data refresh started")
	} else {
		setFlash(w, "Market data refresh is not available")
	}
	http.Redirect(w, r, "/performance", http.StatusSeeOther)
}

// fetchPortfolios returns all portfolios for the selector dropdown.
func (h *PerformanceWebHandler) fetchPortfolios(ctx context.Context) []portfolio.Portfolio {
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// userFriendlyPerformanceError returns a user-friendly message from a performance error.
func userFriendlyPerformanceError(err error) string {
	var posErr *position.PositionError
	if errors.As(err, &posErr) {
		switch posErr.Code {
		case "mismatched_currencies":
			return "Cannot combine portfolios with different base currencies. Select a single portfolio to view its performance."
		default:
			return posErr.Message
		}
	}
	return "An error occurred while computing performance data."
}

// chartDataPoint is the JSON-serializable format for ECharts.
type chartDataPoint struct {
	Date           string `json:"date"`
	PortfolioValue string `json:"portfolio_value"`
	NetDeposit     string `json:"net_deposit"`
}

// serializeChartData converts equity curve points to JSON for ECharts consumption.
func serializeChartData(points []position.EquityCurvePoint) string {
	if len(points) == 0 {
		return "[]"
	}
	data := make([]chartDataPoint, len(points))
	for i, p := range points {
		data[i] = chartDataPoint{
			Date:           p.Date.Format("2006-01-02"),
			PortfolioValue: p.PortfolioValue.String(),
			NetDeposit:     p.NetDeposit.String(),
		}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// buildRefreshURL constructs the POST target for the refresh button.
func buildRefreshURL(portfolioID, benchmark string) string {
	url := "/performance/refresh"
	hasQuery := false
	if portfolioID != "" {
		url += "?portfolio_id=" + portfolioID
		hasQuery = true
	}
	if benchmark != "" {
		if hasQuery {
			url += "&benchmark=" + benchmark
		} else {
			url += "?benchmark=" + benchmark
		}
	}
	return url
}

// extractStaleSymbols extracts symbol names from staleness warnings.
// Warnings follow the pattern "stale market data for SYMBOL (...)" or
// "missing market data for SYMBOL".
func extractStaleSymbols(warnings []string) []string {
	var symbols []string
	for _, w := range warnings {
		if strings.HasPrefix(w, "stale market data for ") {
			sym := strings.TrimPrefix(w, "stale market data for ")
			if idx := strings.Index(sym, " ("); idx > 0 {
				symbols = append(symbols, sym[:idx])
			}
		} else if strings.HasPrefix(w, "missing market data for ") {
			symbols = append(symbols, strings.TrimPrefix(w, "missing market data for "))
		}
	}
	return symbols
}

// formatLastRefresh formats a time as a human-readable "X ago" string.
func formatLastRefresh(t time.Time) string {
	elapsed := time.Since(t)
	if elapsed < 30*time.Second {
		return "Just now"
	}
	if elapsed < time.Minute {
		return "Updated " + strconv.Itoa(int(elapsed.Seconds())) + "s ago"
	}
	if elapsed < time.Hour {
		return "Updated " + strconv.Itoa(int(elapsed.Minutes())) + "m ago"
	}
	if elapsed < 24*time.Hour {
		return "Updated " + strconv.Itoa(int(elapsed.Hours())) + "h ago"
	}
	days := int(elapsed.Hours() / 24)
	if days == 1 {
		return "Updated 1d ago"
	}
	return "Updated " + strconv.Itoa(days) + "d ago"
}

// buildPeriodURLs pre-builds the URL for each period button, preserving
// portfolio_id and benchmark params.
func buildPeriodURLs(portfolioID, selectedPeriod, benchmark string) map[string]string {
	urls := make(map[string]string)
	for _, p := range []string{"1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"} {
		url := "/performance"
		hasQuery := false
		if portfolioID != "" {
			url += "?portfolio_id=" + portfolioID
			hasQuery = true
		}
		if p != "All" {
			if hasQuery {
				url += "&period=" + p
			} else {
				url += "?period=" + p
				hasQuery = true
			}
		}
		if benchmark != "" {
			if hasQuery {
				url += "&benchmark=" + benchmark
			} else {
				url += "?benchmark=" + benchmark
			}
		}
		urls[p] = url
	}
	_ = selectedPeriod // used by template for active state
	return urls
}

// buildBenchmarkURLs pre-builds the URL for each benchmark option in the
// dropdown, preserving portfolio_id and period params.
func buildBenchmarkURLs(selectedBenchmark, portfolioID, period string) map[string]string {
	urls := make(map[string]string)
	predefined := comparison.GetPredefined()

	// "None" option — no benchmark param.
	url := "/performance"
	hasQuery := false
	if portfolioID != "" {
		url += "?portfolio_id=" + portfolioID
		hasQuery = true
	}
	if period != "" && period != "All" {
		if hasQuery {
			url += "&period=" + period
		} else {
			url += "?period=" + period
			hasQuery = true
		}
	}
	urls["None"] = url

	// Each predefined benchmark.
	for ticker, name := range predefined {
		url := "/performance"
		hasQuery := false
		if portfolioID != "" {
			url += "?portfolio_id=" + portfolioID
			hasQuery = true
		}
		if period != "" && period != "All" {
			if hasQuery {
				url += "&period=" + period
			} else {
				url += "?period=" + period
				hasQuery = true
			}
		}
		if hasQuery {
			url += "&benchmark=" + ticker
		} else {
			url += "?benchmark=" + ticker
		}
		labels := name + " (" + ticker + ")"
		urls[labels] = url
	}

	_ = selectedBenchmark // used by template for active state
	return urls
}

// computeMonthlyReturnsFromCurve computes monthly returns for the portfolio
// equity curve and optionally for the benchmark. Produces monthlyReturnData
// rows for the heatmap template.
//
// Portfolio monthly return: (end_value / start_value - 1) * 100 for each month.
// Benchmark monthly return: uses comparison.ComputeMonthlyReturns on benchmark prices.
// Diff: portfolio return - benchmark return (empty string if no benchmark).
func computeMonthlyReturnsFromCurve(curve []position.EquityCurvePoint, benchPrices []market.HistoricalPrice, benchmarkTicker string) []monthlyReturnData {
	if len(curve) == 0 {
		return nil
	}

	// Group equity curve by year-month: collect first and last value per month.
	type monthRange struct {
		first decimal.Decimal
		last  decimal.Decimal
	}
	portfolioMonths := make(map[string]*monthRange)
	for _, pt := range curve {
		key := pt.Date.Format("2006-01")
		if existing, ok := portfolioMonths[key]; ok {
			existing.last = pt.PortfolioValue
		} else {
			portfolioMonths[key] = &monthRange{
				first: pt.PortfolioValue,
				last:  pt.PortfolioValue,
			}
		}
	}

	// Compute benchmark monthly returns if benchmark selected.
	var benchMonthly map[string]*decimal.Decimal
	if benchmarkTicker != "" && len(benchPrices) > 0 {
		benchMonthly = comparison.ComputeMonthlyReturns(benchPrices)
	}

	// Collect all month keys from both sources.
	monthSet := make(map[string]bool)
	for key := range portfolioMonths {
		monthSet[key] = true
	}
	for key := range benchMonthly {
		monthSet[key] = true
	}

	// Sort month keys.
	months := make([]string, 0, len(monthSet))
	for key := range monthSet {
		months = append(months, key)
	}
	// Simple sort: YYYY-MM strings sort lexicographically.
	for i := 0; i < len(months); i++ {
		for j := i + 1; j < len(months); j++ {
			if months[i] > months[j] {
				months[i], months[j] = months[j], months[i]
			}
		}
	}

	// Build result rows.
	result := make([]monthlyReturnData, 0, len(months))
	for _, key := range months {
		year, _ := strconv.Atoi(key[:4])
		month, _ := strconv.Atoi(key[5:7])

		row := monthlyReturnData{
			Year:  year,
			Month: month,
		}

		// Portfolio return.
		if mr, ok := portfolioMonths[key]; ok {
			if mr.first.IsPos() {
				firstF, _ := mr.first.Float64()
				lastF, _ := mr.last.Float64()
				ratio := lastF / firstF
				retPct, _ := decimal.NewFromFloat64((ratio - 1.0) * 100.0)
				val := retPct.Round(2)
				row.PortfolioReturn = val.String()
			}
		}

		// Benchmark return.
		if benchmarkTicker != "" {
			if bRet, ok := benchMonthly[key]; ok {
				row.BenchmarkReturn = bRet.String()
				// Diff = portfolio - benchmark.
				if row.PortfolioReturn != "" {
					pRet, _ := decimal.Parse(row.PortfolioReturn)
					diff, _ := pRet.Sub(*bRet)
					diffRounded := diff.Round(2)
					row.Diff = diffRounded.String()
				}
			}
		}

		result = append(result, row)
	}

	return result
}
