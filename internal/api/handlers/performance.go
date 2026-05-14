package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/comparison"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/performance"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
)

// PerformanceHandler handles HTTP requests for portfolio performance analytics.
type PerformanceHandler struct {
	positionSvc   *position.Service
	marketService position.MarketDataService
}

// NewPerformanceHandler creates a new performance HTTP handler.
func NewPerformanceHandler(positionSvc *position.Service, marketService position.MarketDataService) *PerformanceHandler {
	return &PerformanceHandler{
		positionSvc:   positionSvc,
		marketService: marketService,
	}
}

// RegisterRoutes mounts performance routes on the given router.
func (h *PerformanceHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/performance", h.HandlePerformance)
	r.Post("/api/performance/refresh", h.HandleRefresh)
}

// HandlePerformance handles GET /api/performance.
// Query params: portfolio_id (optional), period (optional), benchmark (optional).
// Returns the equity curve, return metrics, base currency, benchmark data, and any warnings.
func (h *PerformanceHandler) HandlePerformance(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())

	// Validate benchmark ticker if provided.
	if filters.Benchmark != "" && !comparison.IsValidPredefined(filters.Benchmark) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_BENCHMARK", "benchmark ticker is not a predefined benchmark")
		return
	}

	result, err := h.positionSvc.ComputeEquityCurve(r.Context(), filters)
	if err != nil {
		h.handlePerformanceError(w, err)
		return
	}

	// Fetch benchmark data if requested.
	if filters.Benchmark != "" && h.marketService != nil {
		h.addBenchmarkData(r.Context(), result, filters)
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleRefresh handles POST /api/performance/refresh.
// Query params: portfolio_id (optional), period (optional).
// Refreshes current market data (prices + FX rates) for the visible period.
func (h *PerformanceHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())

	result, err := h.positionSvc.RefreshMarketData(r.Context(), filters)
	if err != nil {
		h.handleRefreshError(w, err)
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

func (h *PerformanceHandler) handleRefreshError(w http.ResponseWriter, err error) {
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

	return filters
}
