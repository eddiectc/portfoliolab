package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/optimization"
)

// validHrpPeriods is the set of accepted period values.
var validHrpPeriods = map[string]bool{
	"1Y": true, "3Y": true, "5Y": true,
}

// validHrpPeriodNames lists the accepted period values for error messages.
var validHrpPeriodNames = []string{"1Y", "3Y", "5Y"}

// hrpService defines the methods the handler needs from the HRP service.
type hrpService interface {
	optimizationCommonService
	ComputeHrp(ctx context.Context, req hierarchicalriskparity.ComputeHrpRequest) (*hierarchicalriskparity.ServiceResult, error)
}

// ensure the HRP domain service satisfies the handler's consumer interface.
var _ hrpService = (*hierarchicalriskparity.Service)(nil)

// HrpHandler handles HTTP requests for hierarchical risk parity computation.
type HrpHandler struct {
	OptimizationCommon
	svc hrpService
}

// NewHrpHandler creates a new HRP HTTP handler.
func NewHrpHandler(svc hrpService) *HrpHandler {
	return &HrpHandler{
		OptimizationCommon: OptimizationCommon{svc: svc},
		svc:                svc,
	}
}

// RegisterRoutes mounts HRP routes on the given router.
func (h *HrpHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/api/hrp/compute", h.HandleComputeHrp)
	r.Post("/api/hrp/save", h.HandleSaveHrpAsModelPortfolio)
	r.Get("/api/hrp/symbols", h.HandleGetCandidateSymbols)
	r.Get("/api/hrp/portfolio/{id}/symbols", h.HandleGetPortfolioSymbols)
	r.Get("/api/hrp/model-portfolio/{id}/symbols", h.HandleGetModelPortfolioSymbols)
}

// computeHrpRequest is the JSON request body for POST /api/hrp/compute.
type computeHrpRequest struct {
	Symbols      []string `json:"symbols"`
	Period       string   `json:"period"`
	BaseCurrency string   `json:"base_currency"`
}

// computeHrpResponse is the JSON response for POST /api/hrp/compute.
type computeHrpResponse struct {
	Result          *hierarchicalriskparity.HrpResult    `json:"result"`
	Warnings        []string                             `json:"warnings,omitempty"`
	ExcludedSymbols []string                             `json:"excluded_symbols,omitempty"`
	SymbolDataSpan  map[string]optimization.DataSpan `json:"symbol_data_span,omitempty"`
}

// HandleSaveHrpAsModelPortfolio handles POST /api/hrp/save.
func (h *HrpHandler) HandleSaveHrpAsModelPortfolio(w http.ResponseWriter, r *http.Request) {
	h.OptimizationCommon.HandleSaveAsModelPortfolio(w, r, "/hrp", "")
}

// HandleComputeHrp handles POST /api/hrp/compute.
// Request body: {"symbols": ["AAPL", "MSFT"], "period": "3Y"}
// Response: four HRP allocations, dendrograms, warnings.
func (h *HrpHandler) HandleComputeHrp(w http.ResponseWriter, r *http.Request) {
	var req computeHrpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	// Validate symbols count.
	if len(req.Symbols) < 2 {
		writeJSONError(w, http.StatusBadRequest, "INSUFFICIENT_SYMBOLS",
			"at least 2 symbols are required")
		return
	}
	if len(req.Symbols) > 20 {
		writeJSONError(w, http.StatusBadRequest, "TOO_MANY_SYMBOLS",
			"maximum 20 symbols supported")
		return
	}

	// Validate period.
	period := req.Period
	if period == "" {
		period = "3Y"
	}
	if !validHrpPeriods[period] {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PERIOD",
			"invalid period: "+period+", must be one of: "+strings.Join(validHrpPeriodNames, ", "))
		return
	}

	// Call service.
	serviceReq := hierarchicalriskparity.ComputeHrpRequest{
		Symbols:      req.Symbols,
		Period:       period,
		BaseCurrency: req.BaseCurrency,
	}

	result, err := h.svc.ComputeHrp(r.Context(), serviceReq)
	if err != nil {
		h.handleComputeError(w, err)
		return
	}

	resp := computeHrpResponse{
		Result:          result.Result,
		Warnings:        result.Warnings,
		ExcludedSymbols: result.ExcludedSymbols,
		SymbolDataSpan:  result.SymbolDataSpan,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *HrpHandler) handleComputeError(w http.ResponseWriter, err error) {
	switch {
	case err == hierarchicalriskparity.ErrInsufficientSymbols:
		writeJSONError(w, http.StatusBadRequest, "INSUFFICIENT_SYMBOLS", err.Error())
	case err == hierarchicalriskparity.ErrTooManySymbols:
		writeJSONError(w, http.StatusBadRequest, "TOO_MANY_SYMBOLS", err.Error())
	case err == hierarchicalriskparity.ErrInsufficientData:
		writeJSONError(w, http.StatusBadRequest, "INSUFFICIENT_DATA", err.Error())
	case err == hierarchicalriskparity.ErrNumericalFailure:
		writeJSONError(w, http.StatusBadRequest, "NUMERICAL_FAILURE", err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute HRP")
	}
}
