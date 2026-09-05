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

	"github.com/eddiectc/portfoliolab/internal/domain/marketcache"
	"github.com/eddiectc/portfoliolab/internal/domain/performance"
	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/position"
	"github.com/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/eddiectc/portfoliolab/internal/web"
)

// cacheStatusProvider exposes the subset of MarketCache needed by web handlers.
type cacheStatusProvider interface {
	GetStatus() marketcache.CacheStatus
	RefreshAll(ctx context.Context)
}

// performancePageData is the data struct for the performance page template.
type performancePageData struct {
	web.PageData
	Result              *performance.PerformanceResult
	ChartData           string // pre-serialized JSON for ECharts (equity mode)
	NavChartData        string // pre-serialized JSON for ECharts (NAV mode, normalized to 100%)
	CurrentValue        string // last equity curve point's portfolio value
	CurrentNetDeposit   string // last equity curve point's net deposit
	Portfolios          []portfolio.Portfolio
	SelectedPeriod      string
	SelectedPortfolioID string
	SelectedMode        string // "equity" (default) or "nav"
	Error               string
	// Cache status for the aggregate status indicator.
	CacheStatus     marketcache.CacheStatus
	HasCacheStatus  bool
	LastRefreshText string
	StaleSymbols    []string
	// Pre-built URLs for template safety (Go html/template is strict about expressions in URLs).
	RefreshURL string
	PeriodURLs map[string]string // period label -> full URL
	ModeURLs   map[string]string // mode label -> full URL
	// Benchmark fields.
	SelectedBenchmark  string
	BenchmarkNames     map[string]string // ticker -> display name for dropdown
	BenchmarkTicker    string
	BenchmarkChartData string // JSON-serialized benchmark prices for ECharts
	BenchmarkMWRPct    *decimal.Decimal
	BenchmarkCurrency  string
	BenchmarkWarning   string
	BenchmarkURLs      map[string]string // option label -> full URL
	MonthlyReturns     []performance.YearlyMonthlyReturns
}

// benchmarkSymbolLister exposes the subset of symbol mapping repository needed
// to list user-defined benchmarks.
type benchmarkSymbolLister interface {
	ListBenchmarks(ctx context.Context) ([]symbolmapping.SymbolMapping, error)
}

// PerformanceWebHandler handles server-rendered performance pages.
type PerformanceWebHandler struct {
	apiHandler      *PerformanceHandler
	portfolioSvc    *portfolio.Service
	marketCache     cacheStatusProvider
	benchmarkLister benchmarkSymbolLister
	renderer        *web.Renderer
}

// NewPerformanceWebHandler creates a new performance web handler.
func NewPerformanceWebHandler(apiHandler *PerformanceHandler, portfolioSvc *portfolio.Service, marketCache cacheStatusProvider, benchmarkLister benchmarkSymbolLister, renderer *web.Renderer) *PerformanceWebHandler {
	return &PerformanceWebHandler{
		apiHandler:      apiHandler,
		portfolioSvc:    portfolioSvc,
		marketCache:     marketCache,
		benchmarkLister: benchmarkLister,
		renderer:        renderer,
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

	// Build benchmark names map from user-defined benchmarks.
	benchmarkNames := h.loadBenchmarkNames(r.Context())

	// Validate benchmark if provided.
	benchmark := filters.Benchmark
	if benchmark != "" {
		// Silently drop if not in user-defined benchmarks.
		if _, ok := benchmarkNames[benchmark]; !ok {
			benchmark = ""
		}
	}

	// Resolve selected mode for UI state.
	mode := filters.Mode
	if mode != "nav" && mode != "equity" {
		mode = "equity"
	}

	// Resolve selected portfolio ID for UI state.
	var selectedPortfolioID string
	if filters.PortfolioID != nil {
		selectedPortfolioID = strconv.FormatInt(*filters.PortfolioID, 10)
	}

	result, err := h.apiHandler.computeResult(r.Context(), filters)
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
			SelectedMode:        mode,
			SelectedBenchmark:   benchmark,
			BenchmarkNames:      benchmarkNames,
			Error:               userFriendlyPerformanceError(err),
			CacheStatus:         cacheStatus,
			HasCacheStatus:      hasCacheStatus,
			LastRefreshText:     lastRefreshText,
			RefreshURL:          buildRefreshURL(selectedPortfolioID, benchmark, mode),
			PeriodURLs:          buildPeriodURLs(selectedPortfolioID, filters.Period, benchmark, mode),
			BenchmarkURLs:       buildBenchmarkURLs(benchmarkNames, benchmark, selectedPortfolioID, filters.Period, mode),
			ModeURLs:            buildModeURLs(selectedPortfolioID, filters.Period, benchmark, mode),
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

	// Compute NAV chart data for NAV mode.
	var navChartData string
	if mode == "nav" {
		navChartData = computeNavChartData(result.EquityCurve)
	}

	// Serialize benchmark chart data for ECharts (presentation only).
	// Clip to portfolio date range so both chart lines start/end at the same dates.
	var benchmarkChartData string
	if benchmark != "" && len(result.BenchmarkPrices) > 0 {
		clipped := clipToPortfolioRange(result.BenchmarkPrices, result.EquityCurve)
		benchmarkChartData = serializeBenchmarkChartData(clipped)
	}

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance", Flash: getFlash(w, r)},
		Result:              result,
		ChartData:           serializeChartData(result.EquityCurve),
		NavChartData:        navChartData,
		CurrentValue:        currentValue,
		CurrentNetDeposit:   currentNetDeposit,
		Portfolios:          h.fetchPortfolios(r.Context()),
		SelectedPeriod:      filters.Period,
		SelectedPortfolioID: selectedPortfolioID,
		SelectedMode:        mode,
		CacheStatus:         cacheStatus,
		HasCacheStatus:      hasCacheStatus,
		LastRefreshText:     lastRefreshText,
		StaleSymbols:        staleSymbols,
		RefreshURL:          buildRefreshURL(selectedPortfolioID, benchmark, mode),
		PeriodURLs:          buildPeriodURLs(selectedPortfolioID, filters.Period, benchmark, mode),
		ModeURLs:            buildModeURLs(selectedPortfolioID, filters.Period, benchmark, mode),
		// Benchmark fields (from result, computed by API handler).
		SelectedBenchmark:  benchmark,
		BenchmarkNames:     benchmarkNames,
		BenchmarkTicker:    result.BenchmarkTicker,
		BenchmarkChartData: benchmarkChartData,
		BenchmarkMWRPct:    result.BenchmarkMWRPct,
		BenchmarkCurrency:  result.BenchmarkCurrency,
		BenchmarkWarning:   result.BenchmarkWarning,
		BenchmarkURLs:      buildBenchmarkURLs(benchmarkNames, benchmark, selectedPortfolioID, filters.Period, mode),
		MonthlyReturns:     result.MonthlyReturns,
	}

	if err := h.renderer.Render(w, "performance/index", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// clipToPortfolioRange clips benchmark prices to the portfolio date range
// so both chart lines start and end at the same dates for easy comparison.
func clipToPortfolioRange(prices []market.HistoricalPrice, curve []performance.EquityCurvePoint) []market.HistoricalPrice {
	if len(curve) == 0 {
		return prices
	}
	dateFrom := curve[0].Date
	dateTo := curve[len(curve)-1].Date

	var clipped []market.HistoricalPrice
	for _, p := range prices {
		if !p.Date.Before(dateFrom) && !p.Date.After(dateTo) {
			clipped = append(clipped, p)
		}
	}
	return clipped
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

// loadBenchmarkNames queries user-defined benchmarks and returns a map of
// market_data_symbol → display name (internal_symbol).
func (h *PerformanceWebHandler) loadBenchmarkNames(ctx context.Context) map[string]string {
	names := make(map[string]string)
	if h.benchmarkLister == nil {
		return names
	}
	mappings, err := h.benchmarkLister.ListBenchmarks(ctx)
	if err != nil {
		return names // gracefully degrade on error
	}
	for _, m := range mappings {
		names[m.MarketDataSymbol] = m.InternalSymbol
	}
	return names
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
func serializeChartData(points []performance.EquityCurvePoint) string {
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
func buildRefreshURL(portfolioID, benchmark, mode string) string {
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
			hasQuery = true
		}
	}
	if mode != "" && mode != "equity" {
		if hasQuery {
			url += "&mode=" + mode
		} else {
			url += "?mode=" + mode
		}
	}
	return url
}

// buildModeURLs pre-builds the URL for each mode tab, preserving
// portfolio_id, period, and benchmark params.
func buildModeURLs(portfolioID, period, benchmark, selectedMode string) map[string]string {
	urls := make(map[string]string)
	for _, m := range []string{"equity", "nav"} {
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
		if benchmark != "" {
			if hasQuery {
				url += "&benchmark=" + benchmark
			} else {
				url += "?benchmark=" + benchmark
				hasQuery = true
			}
		}
		if m != "equity" {
			if hasQuery {
				url += "&mode=" + m
			} else {
				url += "?mode=" + m
			}
		}
		urls[m] = url
	}
	_ = selectedMode // used by template for active state
	return urls
}

// navChartDataPoint is the JSON-serializable format for NAV-mode ECharts.
type navChartDataPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"` // NAV per unit (actual value)
}

// computeNavChartData converts equity curve points to NAV-mode chart data
// using the actual NavPerUnit value on each point.
func computeNavChartData(points []performance.EquityCurvePoint) string {
	if len(points) == 0 {
		return "[]"
	}
	data := make([]navChartDataPoint, len(points))
	for i, p := range points {
		var val float64
		if p.NavPerUnit != nil {
			val, _ = p.NavPerUnit.Float64()
		}
		data[i] = navChartDataPoint{
			Date:  p.Date.Format("2006-01-02"),
			Value: val,
		}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "[]"
	}
	return string(b)
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
// portfolio_id, benchmark, and mode params.
func buildPeriodURLs(portfolioID, selectedPeriod, benchmark, mode string) map[string]string {
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
				hasQuery = true
			}
		}
		if mode != "" && mode != "equity" {
			if hasQuery {
				url += "&mode=" + mode
			} else {
				url += "?mode=" + mode
			}
		}
		urls[p] = url
	}
	_ = selectedPeriod // used by template for active state
	return urls
}

// buildBenchmarkURLs pre-builds the URL for each benchmark option,
// preserving portfolio_id, period, and mode params.
// benchmarkNames maps market_data_symbol → display name (internal_symbol).
func buildBenchmarkURLs(benchmarkNames map[string]string, selectedBenchmark, portfolioID, period, mode string) map[string]string {
	urls := make(map[string]string)

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
	if mode != "" && mode != "equity" {
		if hasQuery {
			url += "&mode=" + mode
		} else {
			url += "?mode=" + mode
		}
	}
	urls["None"] = url

	// Each user-defined benchmark.
	for ticker, name := range benchmarkNames {
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
		if mode != "" && mode != "equity" {
			url += "&mode=" + mode
		}
		labels := name + " (" + ticker + ")"
		urls[labels] = url
	}

	_ = selectedBenchmark // used by template for active state
	return urls
}
