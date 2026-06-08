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

	"codeberg.org/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// hrpPeriods lists the accepted period values.
var hrpPeriods = []string{"1Y", "3Y", "5Y"}

// HrpWebHandler handles server-rendered HRP pages.
type HrpWebHandler struct {
	apiHandler         *HrpHandler
	portfolioSvc       *portfolio.Service
	modelPortfolioSvc  hrpModelPortfolioSelector
	renderer           *web.Renderer
	saveModelPortfolio func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error)
}

// hrpModelPortfolioSelector defines the methods needed to fetch model
// portfolios for the dropdown selector.
type hrpModelPortfolioSelector interface {
	GetAllForSelector(ctx context.Context) ([]modelportfolio.ModelPortfolioSummary, error)
}

// ensure the model portfolio service satisfies the web handler's consumer interface.
var _ hrpModelPortfolioSelector = (*modelportfolio.Service)(nil)

// NewHrpWebHandler creates a new HRP web handler.
func NewHrpWebHandler(
	apiHandler *HrpHandler,
	portfolioSvc *portfolio.Service,
	modelPortfolioSvc hrpModelPortfolioSelector,
	renderer *web.Renderer,
) *HrpWebHandler {
	return &HrpWebHandler{
		apiHandler:        apiHandler,
		portfolioSvc:      portfolioSvc,
		modelPortfolioSvc: modelPortfolioSvc,
		renderer:          renderer,
		saveModelPortfolio: func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
			return modelportfolio.ModelPortfolio{}, nil
		},
	}
}

// RegisterRoutes mounts web HRP routes on the given router.
func (h *HrpWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/hrp", h.HandleHrp)
	r.Post("/hrp/save", h.HandleSaveAsModelPortfolio)
}

// hrpPageData is the data struct for the HRP page template.
type hrpPageData struct {
	web.PageData
	// Pre-serialized JSON for ECharts dendrograms.
	HrpChartData string
	// HRP result data.
	Result          *hierarchicalriskparity.HrpResult
	Warnings        []string
	ExcludedSymbols []string
	// Candidate symbols for autocomplete.
	CandidateSymbols []string
	// Portfolios and model portfolios for "copy from" dropdowns.
	Portfolios      []portfolio.Portfolio
	ModelPortfolios []modelportfolio.ModelPortfolioSummary
	// Pre-serialized JSON for JS dropdown data.
	PortfoliosJSON      string
	ModelPortfoliosJSON string
	// UI state.
	SelectedSymbols      []string
	SelectedPeriod       string
	SelectedBaseCurrency string
	// Period button URLs.
	PeriodURLs map[string]string
	// SymbolDataSpan is the actual data coverage per symbol.
	SymbolDataSpan map[string]hierarchicalriskparity.DataSpan
	// LeastDataSymbol is the symbol with the fewest trading days in the result.
	LeastDataSymbol string
	// LeastDataDays is the trading day count of the symbol with least data.
	LeastDataDays int
}

// HandleHrp renders GET /hrp.
func (h *HrpWebHandler) HandleHrp(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse symbols.
	symbols := parseHrpSymbols(query.Get("symbols"))

	// Parse period.
	period := query.Get("period")
	if period == "" || !validHrpPeriods[period] {
		period = "3Y"
	}

	// Parse base currency.
	baseCurrency := query.Get("base_currency")
	if baseCurrency != "" && !validBaseCurrencies[baseCurrency] {
		baseCurrency = ""
	}

	// Fetch selectors.
	portfolios := h.fetchPortfolios(r.Context())
	modelPortfolios := h.fetchModelPortfolios(r.Context())
	candidateSymbols := h.fetchCandidateSymbols(r.Context())

	// Compute HRP.
	var result *hierarchicalriskparity.HrpResult
	var warnings, excludedSymbols []string
	var symbolDataSpan map[string]hierarchicalriskparity.DataSpan

	if len(symbols) >= 2 {
		serviceReq := hierarchicalriskparity.ComputeHrpRequest{
			Symbols:      symbols,
			Period:       period,
			BaseCurrency: baseCurrency,
		}
		serviceResult, err := h.apiHandler.svc.ComputeHrp(r.Context(), serviceReq)
		if err != nil {
			data := h.buildPageData(w, r, symbols, period, baseCurrency, portfolios, modelPortfolios, candidateSymbols, nil, nil, nil, nil)
			data.Error = hrpErrorMessage(err)
			if err := h.renderer.Render(w, "hierarchical_risk_parity/index", data); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		result = serviceResult.Result
		warnings = serviceResult.Warnings
		excludedSymbols = serviceResult.ExcludedSymbols
		symbolDataSpan = serviceResult.SymbolDataSpan
	}

	data := h.buildPageData(w, r, symbols, period, baseCurrency, portfolios, modelPortfolios, candidateSymbols, result, warnings, excludedSymbols, symbolDataSpan)

	if err := h.renderer.Render(w, "hierarchical_risk_parity/index", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// buildPageData assembles the HRP page data struct with serialized chart data.
func (h *HrpWebHandler) buildPageData(
	w http.ResponseWriter, r *http.Request,
	symbols []string, period string, baseCurrency string,
	portfolios []portfolio.Portfolio,
	modelPortfolios []modelportfolio.ModelPortfolioSummary,
	candidateSymbols []string,
	result *hierarchicalriskparity.HrpResult,
	warnings, excludedSymbols []string,
	symbolDataSpan map[string]hierarchicalriskparity.DataSpan,
) hrpPageData {
	// Find symbol with least data.
	leastDataSymbol, leastDataDays := findHrpLeastDataSymbol(symbolDataSpan)

	data := hrpPageData{
		PageData: web.PageData{
			Title: "Hierarchical Risk Parity",
			Flash: getFlash(w, r),
		},
		HrpChartData:         serializeHrpChartData(result),
		Result:               result,
		Warnings:             warnings,
		ExcludedSymbols:      excludedSymbols,
		CandidateSymbols:     candidateSymbols,
		Portfolios:           portfolios,
		ModelPortfolios:      modelPortfolios,
		PortfoliosJSON:       serializeSliceForJS(portfolios),
		ModelPortfoliosJSON:  serializeSliceForJS(modelPortfolios),
		SelectedSymbols:      symbols,
		SelectedPeriod:       period,
		SelectedBaseCurrency: baseCurrency,
		PeriodURLs:           buildHrpPeriodURLs(symbols, period, baseCurrency),
		SymbolDataSpan:       symbolDataSpan,
		LeastDataSymbol:      leastDataSymbol,
		LeastDataDays:        leastDataDays,
	}

	return data
}

// findHrpLeastDataSymbol returns the symbol with the fewest trading days
// and its trading day count from the data span map.
func findHrpLeastDataSymbol(dataSpan map[string]hierarchicalriskparity.DataSpan) (string, int) {
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

// parseHrpSymbols parses comma-separated symbols from query params.
func parseHrpSymbols(s string) []string {
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

// buildHrpPeriodURLs pre-builds the URL for each period button.
func buildHrpPeriodURLs(symbols []string, currentPeriod string, baseCurrency string) map[string]string {
	urls := make(map[string]string)
	for _, p := range hrpPeriods {
		url := "/hrp"
		hasQuery := false
		if len(symbols) > 0 {
			url += "?symbols=" + strings.Join(symbols, ",")
			hasQuery = true
		}
		if p != currentPeriod {
			if hasQuery {
				url += "&period=" + p
			} else {
				url += "?period=" + p
				hasQuery = true
			}
		}
		if baseCurrency != "" {
			if hasQuery {
				url += "&base_currency=" + baseCurrency
			} else {
				url += "?base_currency=" + baseCurrency
			}
		}
		urls[p] = url
	}
	return urls
}

// fetchPortfolios returns all portfolios for the selector dropdown.
func (h *HrpWebHandler) fetchPortfolios(ctx context.Context) []portfolio.Portfolio {
	if h.portfolioSvc == nil {
		return []portfolio.Portfolio{}
	}
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		slog.Error("hrp: fetch portfolios", "error", err)
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// fetchModelPortfolios returns model portfolio summaries for the dropdown.
func (h *HrpWebHandler) fetchModelPortfolios(ctx context.Context) []modelportfolio.ModelPortfolioSummary {
	if h.modelPortfolioSvc == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	summaries, err := h.modelPortfolioSvc.GetAllForSelector(ctx)
	if err != nil {
		slog.Error("hrp: fetch model portfolios", "error", err)
		return []modelportfolio.ModelPortfolioSummary{}
	}
	if summaries == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	return summaries
}

// fetchCandidateSymbols returns all known internal symbols for autocomplete.
func (h *HrpWebHandler) fetchCandidateSymbols(ctx context.Context) []string {
	if h.apiHandler == nil {
		return []string{}
	}
	symbols, err := h.apiHandler.svc.GetCandidateSymbols(ctx)
	if err != nil {
		slog.Error("hrp: fetch candidate symbols", "error", err)
		return []string{}
	}
	if symbols == nil {
		return []string{}
	}
	return symbols
}

// hrpChartData holds JSON data for the ECharts dendrogram rendering.
type hrpChartData struct {
	// Allocations are the four HRP allocations with weights and dendrograms.
	Allocations []hrpAllocationData `json:"allocations"`
	// Symbol names in computation order.
	Symbols []string `json:"symbols"`
	// TradingDays is the number of trading days the data covers.
	TradingDays int `json:"trading_days"`
}

// hrpAllocationData holds a single allocation's weights and dendrogram.
type hrpAllocationData struct {
	// Method is the linkage method name (e.g. "single", "complete").
	Method string `json:"method"`
	// Weights maps each symbol to its allocation weight as a percentage.
	Weights map[string]float64 `json:"weights"`
	// Dendrogram is the tree structure for the ECharts tree chart.
	Dendrogram *hierarchicalriskparity.DendrogramNode `json:"dendrogram,omitempty"`
}

// serializeHrpChartData converts the HRP result to JSON for ECharts dendrograms.
func serializeHrpChartData(result *hierarchicalriskparity.HrpResult) string {
	if result == nil || len(result.Allocations) == 0 {
		return "{}"
	}

	allocations := make([]hrpAllocationData, 0, len(result.Allocations))
	for _, alloc := range result.Allocations {
		// Convert fraction weights to percentages for display.
		weightsPct := make(map[string]float64, len(alloc.Weights))
		for sym, w := range alloc.Weights {
			weightsPct[sym] = w * 100
		}
		allocations = append(allocations, hrpAllocationData{
			Method:     alloc.Method,
			Weights:    weightsPct,
			Dendrogram: alloc.Dendrogram,
		})
	}

	data := hrpChartData{
		Allocations: allocations,
		Symbols:     result.Symbols,
		TradingDays: result.TradingDays,
	}

	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// HandleSaveAsModelPortfolio handles POST /hrp/save.
// Saves the selected HRP allocation as a model portfolio.
func (h *HrpWebHandler) HandleSaveAsModelPortfolio(w http.ResponseWriter, r *http.Request) {
	// Parse form data.
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "HRP Portfolio"
	}

	// Parse weights from form: weight_0, weight_1, etc.
	weights := r.Form["weight"]
	symbols := r.Form["symbol"]

	if len(weights) == 0 || len(symbols) == 0 {
		setFlash(w, "No allocation data provided")
		http.Redirect(w, r, "/hrp", http.StatusSeeOther)
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
		// Weight is a fraction (0.0-1.0) from the HRP, convert to percentage.
		entries = append(entries, modelportfolio.ModelPortfolioEntry{
			Symbol:    sym,
			WeightPct: decimal.MustParse(strconv.FormatFloat(weight*100, 'f', 2, 64)),
		})
	}

	if len(entries) == 0 {
		setFlash(w, "No valid allocation entries")
		http.Redirect(w, r, "/hrp", http.StatusSeeOther)
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

	mp, err := h.saveModelPortfolio(r.Context(), req)
	if err != nil {
		setFlash(w, "Failed to save model portfolio: "+err.Error())
		http.Redirect(w, r, "/hrp", http.StatusSeeOther)
		return
	}

	setFlash(w, "Model portfolio \""+mp.Name+"\" created successfully")
	http.Redirect(w, r, "/model-portfolios", http.StatusSeeOther)
}

// hrpErrorMessage maps a computation error to a user-facing message.
func hrpErrorMessage(err error) string {
	switch {
	case err == hierarchicalriskparity.ErrInsufficientSymbols:
		return "At least 2 symbols are required for HRP computation."
	case err == hierarchicalriskparity.ErrTooManySymbols:
		return "Too many symbols. Maximum 20 symbols supported."
	case err == hierarchicalriskparity.ErrInsufficientData:
		return "Insufficient price data for the selected period. Try a shorter period or different symbols."
	case err == hierarchicalriskparity.ErrNumericalFailure:
		return "The computation failed due to a numerical error. Try different symbols or a shorter period."
	default:
		return "An unexpected error occurred while computing the hierarchical risk parity."
	}
}

// WithModelPortfolioCreator sets the model portfolio creator for saving.
func (h *HrpWebHandler) WithModelPortfolioCreator(creator modelPortfolioCreator) {
	h.saveModelPortfolio = func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
		return creator.Create(ctx, req)
	}
}
