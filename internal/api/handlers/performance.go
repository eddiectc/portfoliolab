package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/comparison"
	"github.com/eddiectc/portfoliolab/internal/domain/performance"
	"github.com/eddiectc/portfoliolab/internal/domain/position"
	"github.com/eddiectc/portfoliolab/internal/market"
)

// benchmarkValidator checks if a market data symbol is a user-defined benchmark.
type benchmarkValidator interface {
	IsBenchmark(ctx context.Context, marketDataSymbol string) (bool, error)
}

// PerformanceHandler handles HTTP requests for portfolio performance analytics.
type PerformanceHandler struct {
	positionSvc   *position.Service
	marketService position.MarketDataService
	validator     benchmarkValidator
}

// NewPerformanceHandler creates a new performance HTTP handler.
func NewPerformanceHandler(positionSvc *position.Service, marketService position.MarketDataService) *PerformanceHandler {
	return &PerformanceHandler{
		positionSvc:   positionSvc,
		marketService: marketService,
	}
}

// WithBenchmarkValidator sets the benchmark validator for the handler.
func (h *PerformanceHandler) WithBenchmarkValidator(v benchmarkValidator) {
	h.validator = v
}

// RegisterRoutes mounts performance routes on the given router.
func (h *PerformanceHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/performance", h.HandlePerformance)
	r.Post("/api/performance/refresh", h.HandleRefresh)
}

// HandlePerformance handles GET /api/performance.
// Query params: portfolio_id (optional), period (optional), benchmark (optional), mode (optional), fields (optional).
// Returns the equity curve, return metrics, base currency, benchmark data, monthly returns, and any warnings.
// Use ?fields= to request subsets: equity_curve,metrics,risk,drawdown,yearly,monthly,benchmark,nav
func (h *PerformanceHandler) HandlePerformance(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())
	fields := parseFields(r.URL.Query())

	// Validate benchmark ticker if provided.
	if filters.Benchmark != "" {
		if h.validator == nil {
			writeJSONError(w, http.StatusBadRequest, "INVALID_BENCHMARK", "benchmark validation not configured")
			return
		}
		ok, err := h.validator.IsBenchmark(r.Context(), filters.Benchmark)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to validate benchmark")
			return
		}
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "INVALID_BENCHMARK", "benchmark ticker is not a user-defined benchmark")
			return
		}
	}

	result, err := h.computeResult(r.Context(), filters)
	if err != nil {
		h.handlePerformanceError(w, err)
		return
	}

	// Filter fields if requested.
	if len(fields) > 0 {
		filterPerformanceResult(result, fields)
	}

	writeJSON(w, http.StatusOK, result)
}

// computeResult computes the full PerformanceResult including equity curve,
// benchmark data, and monthly returns. Used by both API and web handlers.
func (h *PerformanceHandler) computeResult(ctx context.Context, filters performance.PerformanceFilters) (*performance.PerformanceResult, error) {
	result, err := h.positionSvc.ComputeEquityCurve(ctx, filters)
	if err != nil {
		return nil, err
	}

	// Fetch benchmark data if requested.
	if filters.Benchmark != "" && h.marketService != nil {
		h.addBenchmarkData(ctx, result, filters)
	}

	// Compute monthly returns (needs benchmark prices if benchmark selected).
	result.MonthlyReturns = computeMonthlyReturnsFromCurve(result.EquityCurve, result.BenchmarkPrices, filters.Benchmark)

	return result, nil
}

// HandleRefresh handles POST /api/performance/refresh.
// Query params: portfolio_id (optional), period (optional).
// Refreshes current market data (prices + FX rates) for the visible period.
func (h *PerformanceHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())

	result, err := h.positionSvc.RefreshMarketData(r.Context(), filters)
	if err != nil {
		h.handleRefreshError(w)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *PerformanceHandler) handlePerformanceError(w http.ResponseWriter, err error) {
	var posErr *position.PositionError
	if errors.As(err, &posErr) {
		switch posErr.Code {
		case "mismatched_currencies":
			writeJSONError(w, http.StatusBadRequest, "MISMATCHED_CURRENCIES", posErr.Message)
		default:
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		}
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func (h *PerformanceHandler) handleRefreshError(w http.ResponseWriter) {
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

// addBenchmarkData fetches cached benchmark prices and computes MWR for the
// selected benchmark, populating the benchmark fields on the result.
func (h *PerformanceHandler) addBenchmarkData(ctx context.Context, result *performance.PerformanceResult, filters performance.PerformanceFilters) {
	ticker := filters.Benchmark
	result.BenchmarkTicker = ticker

	dateFrom, dateTo := determineDateRange(filters)
	if dateFrom.IsZero() {
		// No lower bound — fetch from earliest available.
		dateFrom = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if dateTo.IsZero() {
		dateTo = time.Now().UTC()
	}

	prices, err := h.marketService.GetHistoricalPrices(ctx, ticker, dateFrom, dateTo)
	if err != nil {
		result.BenchmarkWarning = "failed to fetch benchmark data"
		return
	}

	if len(prices) == 0 {
		result.BenchmarkWarning = "no cached data available for benchmark"
		return
	}

	result.BenchmarkPrices = prices
	if len(prices) > 0 {
		result.BenchmarkCurrency = prices[0].Currency
	}

	mwr := comparison.ComputeMWRForPeriod(prices, dateFrom, dateTo)
	result.BenchmarkMWRPct = mwr
}

// parseFields parses the fields query parameter into a set of field group names.
// Empty return means "all fields".
func parseFields(query url.Values) map[string]bool {
	v := query.Get("fields")
	if v == "" {
		return nil
	}
	fields := make(map[string]bool)
	for _, f := range strings.Split(v, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			fields[f] = true
		}
	}
	return fields
}

// filterPerformanceResult zeroes out fields not in the requested set.
func filterPerformanceResult(result *performance.PerformanceResult, fields map[string]bool) {
	if !fields["equity_curve"] {
		result.EquityCurve = nil
	}
	if !fields["metrics"] {
		result.ReturnMetrics = performance.ReturnMetrics{}
		result.BaseCurrency = ""
		result.Warnings = nil
	}
	if !fields["risk"] {
		result.RiskMetrics = performance.RiskMetrics{}
	}
	if !fields["drawdown"] {
		result.DrawdownAnalysis = performance.DrawdownAnalysis{}
	}
	if !fields["yearly"] {
		result.YearlyPerformance = nil
	}
	if !fields["monthly"] {
		result.MonthlyReturns = nil
	}
	if !fields["benchmark"] {
		result.BenchmarkTicker = ""
		result.BenchmarkPrices = nil
		result.BenchmarkMWRPct = nil
		result.BenchmarkCurrency = ""
		result.BenchmarkWarning = ""
	}
	if !fields["nav"] {
		result.NavSummary = nil
	}
}

// determineDateRange parses the period string into a date range, mirroring
// the logic in position/equity_curve.go.
func determineDateRange(filters performance.PerformanceFilters) (time.Time, time.Time) {
	now := time.Now().UTC()

	if filters.DateFrom != nil && filters.DateTo != nil {
		return *filters.DateFrom, *filters.DateTo
	}

	dateTo := now
	if filters.DateTo != nil {
		dateTo = *filters.DateTo
	}

	if filters.DateFrom != nil {
		return *filters.DateFrom, dateTo
	}

	period := filters.Period
	if period == "" || period == "All" {
		return time.Time{}, dateTo
	}

	var dateFrom time.Time
	switch period {
	case "1W":
		dateFrom = now.AddDate(0, 0, -7)
	case "1M":
		dateFrom = now.AddDate(0, -1, 0)
	case "3M":
		dateFrom = now.AddDate(0, -3, 0)
	case "1Y":
		dateFrom = now.AddDate(-1, 0, 0)
	case "3Y":
		dateFrom = now.AddDate(-3, 0, 0)
	case "5Y":
		dateFrom = now.AddDate(-5, 0, 0)
	case "YTD":
		dateFrom = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default:
		dateFrom = time.Time{}
	}

	return dateFrom, dateTo
}

// parsePerformanceFilters extracts performance filter criteria from query params.
func parsePerformanceFilters(query url.Values) performance.PerformanceFilters {
	var filters performance.PerformanceFilters

	if v := query.Get("portfolio_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filters.PortfolioID = &n
		}
	}

	if v := query.Get("period"); v != "" {
		filters.Period = v
	}

	if v := query.Get("benchmark"); v != "" {
		filters.Benchmark = v
	}

	if v := query.Get("mode"); v != "" {
		filters.Mode = v
	}

	if v := query.Get("date_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filters.DateFrom = &t
		}
	}

	if v := query.Get("date_to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filters.DateTo = &t
		}
	}

	return filters
}

// computeMonthlyTWR computes the Time-Weighted Return for a single month
// from its equity curve points. It detects cash flow dates by finding
// changes in NetDeposit between consecutive points, computes the pre-cash-flow
// portfolio value (PV - incremental_net_deposit), and geometrically links
// sub-period returns.
//
// Sub-periods: start → pre-CF[0], post-CF[0] → pre-CF[1], ..., post-CF[n] → end.
// The post-CF value on a cash flow date is the PortfolioValue from the equity
// curve (which includes the cash flow). The pre-CF value is PV - delta_ND.
//
// Returns nil if the month has fewer than 2 points or the start value is
// non-positive.
func computeMonthlyTWR(points []performance.EquityCurvePoint) *decimal.Decimal {
	if len(points) < 2 {
		return nil
	}

	startPV := points[0].PortfolioValue
	if !startPV.IsPos() {
		return nil
	}

	startPVF, _ := startPV.Float64()
	endPVF, _ := points[len(points)-1].PortfolioValue.Float64()

	// Detect cash flow indices: points where NetDeposit changed from previous.
	var cfIndices []int
	prevND, _ := points[0].NetDeposit.Float64()
	for i := 1; i < len(points); i++ {
		nd, _ := points[i].NetDeposit.Float64()
		if nd != prevND {
			cfIndices = append(cfIndices, i)
			prevND = nd
		}
	}

	// No cash flows: simple return.
	if len(cfIndices) == 0 {
		if startPVF <= 0 {
			return nil
		}
		ratio := endPVF / startPVF
		retPct, _ := decimal.NewFromFloat64((ratio - 1.0) * 100.0)
		result := retPct.Round(2)
		return &result
	}

	// Geometrically link sub-period return ratios.
	product := 1.0

	// Previous "from" value for sub-period computation.
	// Initially the start of the month.
	fromVal := startPVF

	for _, cfIdx := range cfIndices {
		// Pre-cash-flow PV: PV_on_day - incremental_net_deposit.
		// incremental_ND = ND[cfIdx] - ND[cfIdx-1]
		// pre_CF_PV = PV[cfIdx] - (ND[cfIdx] - ND[cfIdx-1])
		cfPV, _ := points[cfIdx].PortfolioValue.Float64()
		cfND, _ := points[cfIdx].NetDeposit.Float64()
		var prevCFND float64
		if cfIdx > 0 {
			prevCFND, _ = points[cfIdx-1].NetDeposit.Float64()
		}
		incrementalND := cfND - prevCFND
		preCFPV := cfPV - incrementalND

		// Sub-period return: fromVal → preCFPV.
		if fromVal > 0 && preCFPV > 0 {
			product *= preCFPV / fromVal
		} else {
			return nil
		}

		// Next sub-period starts from the post-cash-flow PV (includes the CF).
		fromVal = cfPV
	}

	// Final sub-period: last post-CF (or start if no CFs reached) → end of month.
	if fromVal > 0 && endPVF > 0 {
		product *= endPVF / fromVal
	} else {
		return nil
	}

	// TWR = product - 1, expressed as percentage.
	twrPct, _ := decimal.NewFromFloat64((product - 1.0) * 100.0)
	result := twrPct.Round(2)
	return &result
}

// computeMonthlyReturnsFromCurve computes monthly returns for the portfolio
// equity curve and optionally for the benchmark. Produces yearReturnData
// rows (one per year) for the heatmap template.
//
// Portfolio monthly return: Time-Weighted Return (TWR) that isolates
// investment performance from deposit/withdrawal timing by splitting
// the month at cash flow dates and geometrically linking sub-period returns.
// Benchmark monthly return: uses comparison.ComputeMonthlyReturns on benchmark prices.
// Diff: portfolio return - benchmark return (empty string if no benchmark).
func computeMonthlyReturnsFromCurve(curve []performance.EquityCurvePoint, benchPrices []market.HistoricalPrice, benchmarkTicker string) []performance.YearlyMonthlyReturns {
	if len(curve) == 0 {
		return nil
	}

	// Group equity curve points by year-month for TWR computation.
	portfolioMonths := make(map[string][]performance.EquityCurvePoint)
	for _, pt := range curve {
		key := pt.Date.Format("2006-01")
		portfolioMonths[key] = append(portfolioMonths[key], pt)
	}

	// Compute benchmark monthly returns if benchmark selected.
	var benchMonthly map[string]*decimal.Decimal
	if benchmarkTicker != "" && len(benchPrices) > 0 {
		benchMonthly = comparison.ComputeMonthlyReturns(benchPrices)
	}

	// Only include months that have portfolio data.
	// Benchmark-only months (before portfolio started) are excluded.
	monthSet := make(map[string]bool)
	for key := range portfolioMonths {
		monthSet[key] = true
	}

	// Sort month keys.
	months := make([]string, 0, len(monthSet))
	for key := range monthSet {
		months = append(months, key)
	}
	sort.Strings(months)

	// Group month data by year.
	type monthCell struct {
		portfolioRet *decimal.Decimal
		benchRet     *decimal.Decimal
		diff         *decimal.Decimal
	}
	yearMonths := make(map[int]map[int]monthCell) // year -> month -> cell
	for _, key := range months {
		year, _ := strconv.Atoi(key[:4])
		month, _ := strconv.Atoi(key[5:7])

		if yearMonths[year] == nil {
			yearMonths[year] = make(map[int]monthCell)
		}

		cell := monthCell{}

		// Portfolio return — TWR scoped to this month.
		if pts, ok := portfolioMonths[key]; ok {
			cell.portfolioRet = computeMonthlyTWR(pts)
		}

		// Benchmark return.
		if benchmarkTicker != "" {
			if bRet, ok := benchMonthly[key]; ok {
				cell.benchRet = bRet
				// Diff = portfolio - benchmark.
				if cell.portfolioRet != nil {
					diff, _ := cell.portfolioRet.Sub(*bRet)
					diffRounded := diff.Round(2)
					cell.diff = &diffRounded
				}
			}
		}

		yearMonths[year][month] = cell
	}

	// Build result rows sorted by year.
	years := make([]int, 0, len(yearMonths))
	for y := range yearMonths {
		years = append(years, y)
	}
	sort.Ints(years)

	result := make([]performance.YearlyMonthlyReturns, 0, len(years))
	for _, year := range years {
		monthsMap := make(map[int]performance.MonthlyReturn)
		for month, cell := range yearMonths[year] {
			monthsMap[month] = performance.MonthlyReturn{
				ReturnPct:          cell.portfolioRet,
				BenchmarkReturnPct: cell.benchRet,
				DiffPct:            cell.diff,
			}
		}
		result = append(result, performance.YearlyMonthlyReturns{
			Year:   year,
			Months: monthsMap,
		})
	}

	return result
}
