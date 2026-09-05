package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/optimization"
)

// optimizationCommonService defines the methods shared by all optimization
// handlers (efficient frontier, HRP, etc.) for symbol lookup and data span.
type optimizationCommonService interface {
	GetCandidateSymbols(ctx context.Context) ([]string, error)
	GetSymbolsFromPortfolio(ctx context.Context, portfolioID int64) ([]string, error)
	GetSymbolsFromModelPortfolio(ctx context.Context, modelPortfolioID int64) ([]string, error)
}

// modelPortfolioCreator defines the method needed to create a model portfolio.
type modelPortfolioCreator interface {
	Create(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error)
}

// OptimizationCommon is embedded in optimization API handlers to share
// symbol-lookup routes and model-portfolio saving logic.
type OptimizationCommon struct {
	svc                   optimizationCommonService
	modelPortfolioCreator modelPortfolioCreator
}

// WithModelPortfolioCreator sets the model portfolio creator for saving
// optimized allocations as model portfolios.
func (o *OptimizationCommon) WithModelPortfolioCreator(creator modelPortfolioCreator) {
	o.modelPortfolioCreator = creator
}

// symbolsResponse is the JSON response for symbol list endpoints.
type symbolsResponse struct {
	Symbols []string `json:"symbols"`
}

// saveModelPortfolioEntry is a single entry in a save-as-model-portfolio request.
type saveModelPortfolioEntry struct {
	Symbol string  `json:"symbol"`
	Weight float64 `json:"weight"`
}

// HandleGetCandidateSymbols handles GET requests for candidate symbols.
// Returns all known internal symbols for autocomplete.
func (o *OptimizationCommon) HandleGetCandidateSymbols(w http.ResponseWriter, r *http.Request) {
	if o.svc == nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "service not configured")
		return
	}
	symbols, err := o.svc.GetCandidateSymbols(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve symbols")
		return
	}
	writeJSON(w, http.StatusOK, symbolsResponse{Symbols: symbols})
}

// HandleGetPortfolioSymbols handles GET requests for portfolio symbols.
// Returns the distinct symbols held in a real portfolio.
func (o *OptimizationCommon) HandleGetPortfolioSymbols(w http.ResponseWriter, r *http.Request) {
	if o.svc == nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "service not configured")
		return
	}
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid portfolio ID")
		return
	}
	symbols, err := o.svc.GetSymbolsFromPortfolio(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve portfolio symbols")
		return
	}
	writeJSON(w, http.StatusOK, symbolsResponse{Symbols: symbols})
}

// HandleGetModelPortfolioSymbols handles GET requests for model portfolio symbols.
// Returns the symbols in a model portfolio.
func (o *OptimizationCommon) HandleGetModelPortfolioSymbols(w http.ResponseWriter, r *http.Request) {
	if o.svc == nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "service not configured")
		return
	}
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid model portfolio ID")
		return
	}
	symbols, err := o.svc.GetSymbolsFromModelPortfolio(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve model portfolio symbols")
		return
	}
	writeJSON(w, http.StatusOK, symbolsResponse{Symbols: symbols})
}

// HandleSaveAsModelPortfolio handles POST requests to save an optimized
// allocation as a model portfolio.
//
// redirectURL is the path to redirect to on success (e.g. "/efficient-frontier").
// defaultName is the fallback portfolio name if none provided.
func (o *OptimizationCommon) HandleSaveAsModelPortfolio(w http.ResponseWriter, r *http.Request, redirectURL, defaultName string) {
	if o.modelPortfolioCreator == nil {
		writeJSONError(w, http.StatusInternalServerError, "NOT_CONFIGURED", "model portfolio creator not configured")
		return
	}

	var req struct {
		Name    string                    `json:"name"`
		Entries []saveModelPortfolioEntry `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		if defaultName != "" {
			req.Name = defaultName
		} else {
			writeJSONError(w, http.StatusBadRequest, "INVALID_NAME", "name is required")
			return
		}
	}
	if len(req.Entries) == 0 {
		writeJSONError(w, http.StatusBadRequest, "EMPTY_ENTRIES", "at least one entry is required")
		return
	}

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

	mp, err := o.modelPortfolioCreator.Create(r.Context(), createReq)
	if err != nil {
		handleModelPortfolioSaveError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, mp)
}

// handleModelPortfolioSaveError maps model portfolio creation errors to
// JSON error responses. Shared by all optimization handlers.
func handleModelPortfolioSaveError(w http.ResponseWriter, err error) {
	switch err {
	case modelportfolio.ErrNameExists:
		writeJSONError(w, http.StatusConflict, "NAME_EXISTS", err.Error())
	case modelportfolio.ErrInvalidName:
		writeJSONError(w, http.StatusBadRequest, "INVALID_NAME", err.Error())
	case modelportfolio.ErrWeightSumNot100:
		writeJSONError(w, http.StatusBadRequest, "WEIGHT_SUM_NOT_100", err.Error())
	case modelportfolio.ErrInvalidWeight:
		writeJSONError(w, http.StatusBadRequest, "INVALID_WEIGHT", err.Error())
	case modelportfolio.ErrDuplicateSymbol:
		writeJSONError(w, http.StatusBadRequest, "DUPLICATE_SYMBOL", err.Error())
	case modelportfolio.ErrEmptyEntries:
		writeJSONError(w, http.StatusBadRequest, "EMPTY_ENTRIES", err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save model portfolio")
	}
}

// --- Shared data span helpers ---

// findLeastDataSymbol returns the symbol with the fewest trading days
// and its trading day count from the data span map.
func findLeastDataSymbol(dataSpan map[string]optimization.DataSpan) (string, int) {
	if len(dataSpan) == 0 {
		return "", 0
	}
	var leastSymbol string
	leastDays := 1<<31 - 1 // max int32
	for sym, span := range dataSpan {
		if span.TradingDays < leastDays {
			leastDays = span.TradingDays
			leastSymbol = sym
		}
	}
	return leastSymbol, leastDays
}

// --- Shared web helpers ---

// parseOptimizationSymbols parses comma-separated symbols from query params.
func parseOptimizationSymbols(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var symbols []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			symbols = append(symbols, p)
		}
	}
	return symbols
}

// validBaseCurrencies is the set of accepted base currency values.
var validBaseCurrencies = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "JPY": true,
	"CHF": true, "CAD": true, "AUD": true, "CNY": true,
}

// serializeSliceForJS serializes a slice to JSON for embedding in JavaScript.
func serializeSliceForJS(data interface{}) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// fetchCandidateSymbolsFromService returns all known internal symbols for autocomplete.
func fetchCandidateSymbolsFromService(svc optimizationCommonService, ctx context.Context, label string) []string {
	if svc == nil {
		return []string{}
	}
	symbols, err := svc.GetCandidateSymbols(ctx)
	if err != nil {
		slog.Error(label+": fetch candidate symbols", "error", err)
		return []string{}
	}
	if symbols == nil {
		return []string{}
	}
	return symbols
}

// handleOptimizationWebSave handles POST requests to save an optimized
// allocation as a model portfolio from a web form.
//
// redirectURL is the path to redirect to on error.
// successRedirect is the path to redirect to on success (e.g. "/model-portfolios").
// defaultName is the fallback portfolio name if none provided.
func handleOptimizationWebSave(w http.ResponseWriter, r *http.Request, redirectURL, successRedirect, defaultName string,
	saveModelPortfolio func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error),
) {
	// Parse form data.
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = defaultName
	}

	// Parse weights from form: weight_0, weight_1, etc.
	weights := r.Form["weight"]
	symbols := r.Form["symbol"]

	if len(weights) == 0 || len(symbols) == 0 {
		setFlash(w, "No allocation data provided")
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	// Build entries — convert fraction weights to percentages.
	entries := make([]modelportfolio.ModelPortfolioEntry, 0, len(weights))
	for i, sym := range symbols {
		sym = strings.TrimSpace(sym)
		if i >= len(weights) {
			break
		}
		weightStr := strings.TrimSpace(weights[i])
		weight, err := strconv.ParseFloat(weightStr, 64)
		if err != nil || weight <= 0 {
			continue
		}
		entries = append(entries, modelportfolio.ModelPortfolioEntry{
			Symbol:    sym,
			WeightPct: decimal.MustParse(strconv.FormatFloat(weight*100, 'f', 2, 64)),
		})
	}

	if len(entries) == 0 {
		setFlash(w, "No valid allocation entries")
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	// Sort entries by symbol for consistent ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Symbol < entries[j].Symbol
	})

	req := modelportfolio.CreateRequest{
		Name:    name,
		Entries: entries,
	}

	mp, err := saveModelPortfolio(r.Context(), req)
	if err != nil {
		setFlash(w, "Failed to save model portfolio: "+err.Error())
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	setFlash(w, "Model portfolio \""+mp.Name+"\" created successfully")
	http.Redirect(w, r, successRedirect, http.StatusSeeOther)
}
