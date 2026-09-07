package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
	"github.com/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/optimization"
	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
	"github.com/eddiectc/portfoliolab/internal/web"
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
		saveModelPortfolio: func(_ context.Context, _ modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
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
	// Shared optimization partial fields.
	FormID         string // "hrp"
	FormAction     string // "/hrp"
	FormButtonText string // "Compute HRP"
	SymbolHint     string // "Comma-separated symbols. Min 2, max 20."
	APIBase        string // "/api/hrp"
	RiskFreeRate   bool   // false for HRP
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
	SymbolDataSpan map[string]optimization.DataSpan
	// LeastDataSymbol is the symbol with the fewest trading days in the result.
	LeastDataSymbol string
	// LeastDataDays is the trading day count of the symbol with least data.
	LeastDataDays int
}

// HandleHrp renders GET /hrp.
func (h *HrpWebHandler) HandleHrp(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse symbols.
	symbols := parseOptimizationSymbols(query.Get("symbols"))

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
	portfolios := fetchOptimizationPortfolios(r.Context(), h.portfolioSvc, "hrp")
	modelPortfolios := fetchOptimizationModelPortfolios(r.Context(), h.modelPortfolioSvc, "hrp")
	candidateSymbols := fetchCandidateSymbolsFromService(r.Context(), h.apiHandler.svc, "hrp")

	// Compute HRP.
	var result *hierarchicalriskparity.HrpResult
	var warnings, excludedSymbols []string
	var symbolDataSpan map[string]optimization.DataSpan

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
	symbolDataSpan map[string]optimization.DataSpan,
) hrpPageData {
	// Find symbol with least data.
	leastDataSymbol, leastDataDays := findLeastDataSymbol(symbolDataSpan)

	data := hrpPageData{
		PageData: web.PageData{
			Title: "Hierarchical Risk Parity",
			Flash: getFlash(w, r),
		},
		FormID:               "hrp",
		FormAction:           "/hrp",
		FormButtonText:       "Compute HRP",
		SymbolHint:           "Comma-separated symbols. Min 2, max 20.",
		APIBase:              "/api/hrp",
		RiskFreeRate:         false,
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
	handleOptimizationWebSave(w, r, "/hrp", "/model-portfolios", "HRP Portfolio", h.saveModelPortfolio)
}

// hrpErrorMessage maps a computation error to a user-facing message.
func hrpErrorMessage(err error) string {
	switch err {
	case hierarchicalriskparity.ErrInsufficientSymbols:
		return "At least 2 symbols are required for HRP computation."
	case hierarchicalriskparity.ErrTooManySymbols:
		return "Too many symbols. Maximum 20 symbols supported."
	case hierarchicalriskparity.ErrInsufficientData:
		return "Insufficient price data for the selected period. Try a shorter period or different symbols."
	case hierarchicalriskparity.ErrNumericalFailure:
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

// --- Shared fetch helpers (used by both EF and HRP web handlers) ---

// fetchOptimizationPortfolios returns all portfolios for the selector dropdown.
func fetchOptimizationPortfolios(ctx context.Context, portfolioSvc *portfolio.Service, label string) []portfolio.Portfolio {
	if portfolioSvc == nil {
		return []portfolio.Portfolio{}
	}
	portfolios, err := portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		slog.Error(label+": fetch portfolios", "error", err)
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// optimizationModelPortfolioSelector defines the methods needed to fetch model
// portfolios for the dropdown selector.
type optimizationModelPortfolioSelector interface {
	GetAllForSelector(ctx context.Context) ([]modelportfolio.ModelPortfolioSummary, error)
}

// fetchOptimizationModelPortfolios returns model portfolio summaries for the dropdown.
func fetchOptimizationModelPortfolios(ctx context.Context, svc optimizationModelPortfolioSelector, label string) []modelportfolio.ModelPortfolioSummary {
	if svc == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	summaries, err := svc.GetAllForSelector(ctx)
	if err != nil {
		slog.Error(label+": fetch model portfolios", "error", err)
		return []modelportfolio.ModelPortfolioSummary{}
	}
	if summaries == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	return summaries
}
