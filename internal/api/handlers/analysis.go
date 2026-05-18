package handlers

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/analysis"
)

// analysisService is the interface the handler depends on for computing analysis.
type analysisService interface {
	ComputeAnalysis(ctx context.Context, filters analysis.AnalysisFilters) (*analysis.AnalysisResult, error)
}

// AnalysisHandler handles HTTP requests for portfolio analysis.
type AnalysisHandler struct {
	svc analysisService
}

// NewAnalysisHandler creates a new analysis HTTP handler.
func NewAnalysisHandler(svc analysisService) *AnalysisHandler {
	return &AnalysisHandler{svc: svc}
}

// RegisterRoutes mounts analysis routes on the given router.
func (h *AnalysisHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/analysis", h.HandleAnalysis)
}

// HandleAnalysis handles GET /api/analysis.
// Query params: portfolio_id (optional), section (optional), period (optional).
// Returns the full portfolio analysis result or a filtered subset.
// Use ?section= to request a single section: overlap, correlation,
// sector_allocation, geographic_allocation, stress_test, factor_exposure.
// Use ?period= for correlation lookback: 1Y, 3Y, 5Y, 10Y (default: 1Y).
func (h *AnalysisHandler) HandleAnalysis(w http.ResponseWriter, r *http.Request) {
	filters := parseAnalysisFilters(r.URL.Query())

	result, err := h.svc.ComputeAnalysis(r.Context(), filters)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute analysis")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// parseAnalysisFilters extracts analysis filter criteria from query params.
func parseAnalysisFilters(query url.Values) analysis.AnalysisFilters {
	var filters analysis.AnalysisFilters

	if v := query.Get("portfolio_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filters.PortfolioID = &n
		}
	}

	if v := query.Get("section"); v != "" {
		filters.Section = v
	}

	if v := query.Get("period"); v != "" {
		filters.Period = v
	}

	return filters
}
