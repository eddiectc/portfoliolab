package handlers

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/analysis"
)

// validSectionNames lists the accepted section values for error messages.
var validSectionNames = []string{
	string(analysis.SectionOverlap),
	string(analysis.SectionCorrelation),
	string(analysis.SectionSectorAllocation),
	string(analysis.SectionGeographicAllocation),
	string(analysis.SectionStressTest),
	string(analysis.SectionFactorExposure),
}

// validPeriodNames lists the accepted period values for error messages.
var validPeriodNames = []string{"3M", "6M", "1Y", "3Y", "5Y", "10Y"}

// validSections is the set of accepted section filter values.
var validSections = map[string]bool{
	string(analysis.SectionOverlap):              true,
	string(analysis.SectionCorrelation):          true,
	string(analysis.SectionSectorAllocation):     true,
	string(analysis.SectionGeographicAllocation): true,
	string(analysis.SectionStressTest):           true,
	string(analysis.SectionFactorExposure):       true,
}

// validPeriods is the set of accepted period filter values.
var validPeriods = map[string]bool{
	"3M": true, "6M": true, "1Y": true, "3Y": true, "5Y": true, "10Y": true,
}

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

	// Validate section filter.
	if filters.Section != "" && !validSections[filters.Section] {
		writeJSONError(w, http.StatusBadRequest, "INVALID_SECTION",
			"invalid section: "+filters.Section+", must be one of: "+strings.Join(validSectionNames, ", "))
		return
	}

	// Validate period filter.
	if filters.Period != "" && !validPeriods[filters.Period] {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PERIOD",
			"invalid period: "+filters.Period+", must be one of: "+strings.Join(validPeriodNames, ", "))
		return
	}

	result, err := h.computeResult(r.Context(), filters)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute analysis")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// computeResult computes the analysis result for the given filters.
// Shared between the API handler and the web handler.
func (h *AnalysisHandler) computeResult(ctx context.Context, filters analysis.AnalysisFilters) (*analysis.AnalysisResult, error) {
	return h.svc.ComputeAnalysis(ctx, filters)
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
