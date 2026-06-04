package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/efficientfrontier"
)

// validFrontierPeriods is the set of accepted period values.
var validFrontierPeriods = map[string]bool{
	"1Y": true, "3Y": true, "5Y": true,
}

// validFrontierPeriodNames lists the accepted period values for error messages.
var validFrontierPeriodNames = []string{"1Y", "3Y", "5Y"}

// efficientFrontierService defines the methods the handler needs from the
// efficient frontier service.
type efficientFrontierService interface {
	ComputeFrontier(ctx context.Context, req efficientfrontier.ComputeFrontierRequest) (*efficientfrontier.ServiceResult, error)
	GetCandidateSymbols(ctx context.Context) ([]string, error)
	GetSymbolsFromPortfolio(ctx context.Context, portfolioID int64) ([]string, error)
	GetSymbolsFromModelPortfolio(ctx context.Context, modelPortfolioID int64) ([]string, error)
}

// EfficientFrontierHandler handles HTTP requests for efficient frontier computation.
type EfficientFrontierHandler struct {
	svc efficientFrontierService
}

// NewEfficientFrontierHandler creates a new efficient frontier HTTP handler.
func NewEfficientFrontierHandler(svc efficientFrontierService) *EfficientFrontierHandler {
	return &EfficientFrontierHandler{svc: svc}
}

// RegisterRoutes mounts efficient frontier routes on the given router.
func (h *EfficientFrontierHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/api/efficient-frontier/compute", h.HandleComputeFrontier)
	r.Get("/api/efficient-frontier/symbols", h.HandleGetCandidateSymbols)
	r.Get("/api/efficient-frontier/portfolio/{id}/symbols", h.HandleGetPortfolioSymbols)
	r.Get("/api/efficient-frontier/model-portfolio/{id}/symbols", h.HandleGetModelPortfolioSymbols)
}

// computeFrontierRequest is the JSON request body for POST /api/efficient-frontier/compute.
type computeFrontierRequest struct {
	Symbols      []string `json:"symbols"`
	Period       string   `json:"period"`
	RiskFreeRate float64  `json:"risk_free_rate"`
}

// computeFrontierResponse is the JSON response for POST /api/efficient-frontier/compute.
type computeFrontierResponse struct {
	Result          *efficientfrontier.FrontierResult `json:"result"`
	Warnings        []string                          `json:"warnings,omitempty"`
	ExcludedSymbols []string                          `json:"excluded_symbols,omitempty"`
}

// symbolsResponse is the JSON response for symbol list endpoints.
type symbolsResponse struct {
	Symbols []string `json:"symbols"`
}

// HandleComputeFrontier handles POST /api/efficient-frontier/compute.
// Request body: {"symbols": ["AAPL", "MSFT"], "period": "1Y", "risk_free_rate": 0.045}
// Response: frontier points, key portfolios, warnings.
func (h *EfficientFrontierHandler) HandleComputeFrontier(w http.ResponseWriter, r *http.Request) {
	var req computeFrontierRequest
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
	if len(req.Symbols) > 10 {
		writeJSONError(w, http.StatusBadRequest, "TOO_MANY_SYMBOLS",
			"maximum 10 symbols supported")
		return
	}

	// Validate period.
	period := req.Period
	if period == "" {
		period = "1Y"
	}
	if !validFrontierPeriods[period] {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PERIOD",
			"invalid period: "+period+", must be one of: "+strings.Join(validFrontierPeriodNames, ", "))
		return
	}

	// Call service.
	serviceReq := efficientfrontier.ComputeFrontierRequest{
		Symbols:      req.Symbols,
		Period:       period,
		RiskFreeRate: req.RiskFreeRate,
	}

	result, err := h.svc.ComputeFrontier(r.Context(), serviceReq)
	if err != nil {
		h.handleComputeError(w, err)
		return
	}

	resp := computeFrontierResponse{
		Result:          result.Result,
		Warnings:        result.Warnings,
		ExcludedSymbols: result.ExcludedSymbols,
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleGetCandidateSymbols handles GET /api/efficient-frontier/symbols.
// Returns all known internal symbols for autocomplete.
func (h *EfficientFrontierHandler) HandleGetCandidateSymbols(w http.ResponseWriter, r *http.Request) {
	symbols, err := h.svc.GetCandidateSymbols(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve symbols")
		return
	}

	writeJSON(w, http.StatusOK, symbolsResponse{Symbols: symbols})
}

// HandleGetPortfolioSymbols handles GET /api/efficient-frontier/portfolio/{id}/symbols.
// Returns the distinct symbols held in a real portfolio.
func (h *EfficientFrontierHandler) HandleGetPortfolioSymbols(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid portfolio ID")
		return
	}

	symbols, err := h.svc.GetSymbolsFromPortfolio(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve portfolio symbols")
		return
	}

	writeJSON(w, http.StatusOK, symbolsResponse{Symbols: symbols})
}

// HandleGetModelPortfolioSymbols handles GET /api/efficient-frontier/model-portfolio/{id}/symbols.
// Returns the symbols in a model portfolio.
func (h *EfficientFrontierHandler) HandleGetModelPortfolioSymbols(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid model portfolio ID")
		return
	}

	symbols, err := h.svc.GetSymbolsFromModelPortfolio(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve model portfolio symbols")
		return
	}

	writeJSON(w, http.StatusOK, symbolsResponse{Symbols: symbols})
}

func (h *EfficientFrontierHandler) handleComputeError(w http.ResponseWriter, err error) {
	switch {
	case err == efficientfrontier.ErrInsufficientSymbols:
		writeJSONError(w, http.StatusBadRequest, "INSUFFICIENT_SYMBOLS", err.Error())
	case err == efficientfrontier.ErrTooManySymbols:
		writeJSONError(w, http.StatusBadRequest, "TOO_MANY_SYMBOLS", err.Error())
	case err == efficientfrontier.ErrInsufficientData:
		writeJSONError(w, http.StatusBadRequest, "INSUFFICIENT_DATA", err.Error())
	case err == efficientfrontier.ErrSingularMatrix:
		writeJSONError(w, http.StatusBadRequest, "SINGULAR_MATRIX", err.Error())
	case err == efficientfrontier.ErrNumericalFailure:
		writeJSONError(w, http.StatusBadRequest, "NUMERICAL_FAILURE", err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute efficient frontier")
	}
}
