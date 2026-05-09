package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
)

// PerformanceHandler handles HTTP requests for portfolio performance analytics.
type PerformanceHandler struct {
	positionSvc *position.Service
}

// NewPerformanceHandler creates a new performance HTTP handler.
func NewPerformanceHandler(positionSvc *position.Service) *PerformanceHandler {
	return &PerformanceHandler{positionSvc: positionSvc}
}

// RegisterRoutes mounts performance routes on the given router.
func (h *PerformanceHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/performance", h.HandlePerformance)
	r.Post("/api/performance/refresh", h.HandleRefresh)
}

// HandlePerformance handles GET /api/performance.
// Query params: portfolio_id (optional), period (optional).
// Returns the equity curve, return metrics, base currency, and any warnings.
func (h *PerformanceHandler) HandlePerformance(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())

	result, err := h.positionSvc.ComputeEquityCurve(r.Context(), filters)
	if err != nil {
		h.handlePerformanceError(w, err)
		return
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

// parsePerformanceFilters extracts performance filter criteria from query params.
func parsePerformanceFilters(query url.Values) position.PerformanceFilters {
	var filters position.PerformanceFilters

	if v := query.Get("portfolio_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filters.PortfolioID = &n
		}
	}

	if v := query.Get("period"); v != "" {
		filters.Period = v
	}

	return filters
}
