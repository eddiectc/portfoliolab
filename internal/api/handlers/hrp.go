package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// validHrpPeriods is the set of accepted period values.
var validHrpPeriods = map[string]bool{
	"1Y": true, "3Y": true, "5Y": true,
}

// validHrpPeriodNames lists the accepted period values for error messages.
var validHrpPeriodNames = []string{"1Y", "3Y", "5Y"}

// hrpService defines the methods the handler needs from the HRP service.
type hrpService interface {
	ComputeHrp(ctx context.Context, req hierarchicalriskparity.ComputeHrpRequest) (*hierarchicalriskparity.ServiceResult, error)
	GetCandidateSymbols(ctx context.Context) ([]string, error)
	GetSymbolsFromPortfolio(ctx context.Context, portfolioID int64) ([]string, error)
	GetSymbolsFromModelPortfolio(ctx context.Context, modelPortfolioID int64) ([]string, error)
}

// HrpHandler handles HTTP requests for hierarchical risk parity computation.
type HrpHandler struct {
	svc                   hrpService
	modelPortfolioCreator modelPortfolioCreator
}

// NewHrpHandler creates a new HRP HTTP handler.
func NewHrpHandler(svc hrpService) *HrpHandler {
	return &HrpHandler{svc: svc}
}

// WithModelPortfolioCreator sets the model portfolio creator for saving
// optimized allocations as model portfolios.
func (h *HrpHandler) WithModelPortfolioCreator(creator modelPortfolioCreator) {
	h.modelPortfolioCreator = creator
}

// RegisterRoutes mounts HRP routes on the given router.
func (h *HrpHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/api/hrp/compute", h.HandleComputeHrp)
	r.Post("/api/hrp/save", h.HandleSaveAsModelPortfolio)
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
	Result          *hierarchicalriskparity.HrpResult             `json:"result"`
	Warnings        []string                                      `json:"warnings,omitempty"`
	ExcludedSymbols []string                                      `json:"excluded_symbols,omitempty"`
	SymbolDataSpan  map[string]hierarchicalriskparity.DataSpan    `json:"symbol_data_span,omitempty"`
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

// HandleGetCandidateSymbols handles GET /api/hrp/symbols.
// Returns all known internal symbols for autocomplete.
func (h *HrpHandler) HandleGetCandidateSymbols(w http.ResponseWriter, r *http.Request) {
	symbols, err := h.svc.GetCandidateSymbols(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve symbols")
		return
	}

	writeJSON(w, http.StatusOK, symbolsResponse{Symbols: symbols})
}

// HandleGetPortfolioSymbols handles GET /api/hrp/portfolio/{id}/symbols.
// Returns the distinct symbols held in a real portfolio.
func (h *HrpHandler) HandleGetPortfolioSymbols(w http.ResponseWriter, r *http.Request) {
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

// HandleGetModelPortfolioSymbols handles GET /api/hrp/model-portfolio/{id}/symbols.
// Returns the symbols in a model portfolio.
func (h *HrpHandler) HandleGetModelPortfolioSymbols(w http.ResponseWriter, r *http.Request) {
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

// saveHrpModelPortfolioRequest is the JSON request body for POST /api/hrp/save.
type saveHrpModelPortfolioRequest struct {
	Name    string                    `json:"name"`
	Entries []saveModelPortfolioEntry `json:"entries"`
}

// HandleSaveAsModelPortfolio handles POST /api/hrp/save.
// Saves an HRP allocation as a model portfolio.
// Request body: {"name": "HRP Ward Portfolio", "entries": [{"symbol": "AAPL", "weight": 0.3}, ...]}
// Response: created model portfolio.
func (h *HrpHandler) HandleSaveAsModelPortfolio(w http.ResponseWriter, r *http.Request) {
	if h.modelPortfolioCreator == nil {
		writeJSONError(w, http.StatusInternalServerError, "NOT_CONFIGURED", "model portfolio creator not configured")
		return
	}

	var req saveHrpModelPortfolioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, "INVALID_NAME", "name is required")
		return
	}
	if len(req.Entries) == 0 {
		writeJSONError(w, http.StatusBadRequest, "EMPTY_ENTRIES", "at least one entry is required")
		return
	}

	// Convert fraction weights (0.0-1.0) to percentages for model portfolio.
	entries := make([]modelportfolio.ModelPortfolioEntry, 0, len(req.Entries))
	for _, e := range req.Entries {
		entries = append(entries, modelportfolio.ModelPortfolioEntry{
			Symbol:    e.Symbol,
			WeightPct: decimal.MustParse(strconv.FormatFloat(e.Weight*100, 'f', 2, 64)),
		})
	}

	createReq := modelportfolio.CreateRequest{
		Name:    req.Name,
		Entries: entries,
	}

	mp, err := h.modelPortfolioCreator.Create(r.Context(), createReq)
	if err != nil {
		h.handleSaveError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, mp)
}

func (h *HrpHandler) handleSaveError(w http.ResponseWriter, err error) {
	switch {
	case err == modelportfolio.ErrNameExists:
		writeJSONError(w, http.StatusConflict, "NAME_EXISTS", err.Error())
	case err == modelportfolio.ErrInvalidName:
		writeJSONError(w, http.StatusBadRequest, "INVALID_NAME", err.Error())
	case err == modelportfolio.ErrWeightSumNot100:
		writeJSONError(w, http.StatusBadRequest, "WEIGHT_SUM_NOT_100", err.Error())
	case err == modelportfolio.ErrInvalidWeight:
		writeJSONError(w, http.StatusBadRequest, "INVALID_WEIGHT", err.Error())
	case err == modelportfolio.ErrDuplicateSymbol:
		writeJSONError(w, http.StatusBadRequest, "DUPLICATE_SYMBOL", err.Error())
	case err == modelportfolio.ErrEmptyEntries:
		writeJSONError(w, http.StatusBadRequest, "EMPTY_ENTRIES", err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save model portfolio")
	}
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
