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

	"codeberg.org/eddiectc/portfoliolab/internal/domain/efficientfrontier"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// frontierPeriods lists the accepted period values.
var frontierPeriods = []string{"1Y", "3Y", "5Y"}

// frontierDefaultRiskFreeRate is the default risk-free rate (4.5%).
const frontierDefaultRiskFreeRate = 4.5

// EfficientFrontierWebHandler handles server-rendered efficient frontier pages.
type EfficientFrontierWebHandler struct {
	apiHandler         *EfficientFrontierHandler
	portfolioSvc       *portfolio.Service
	modelPortfolioSvc  frontierModelPortfolioSelector
	renderer           *web.Renderer
	saveModelPortfolio func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error)
}

// frontierModelPortfolioSelector defines the methods needed to fetch model
// portfolios for the dropdown selector.
type frontierModelPortfolioSelector interface {
	GetAllForSelector(ctx context.Context) ([]modelportfolio.ModelPortfolioSummary, error)
}

// NewEfficientFrontierWebHandler creates a new efficient frontier web handler.
func NewEfficientFrontierWebHandler(
	apiHandler *EfficientFrontierHandler,
	portfolioSvc *portfolio.Service,
	modelPortfolioSvc frontierModelPortfolioSelector,
	renderer *web.Renderer,
) *EfficientFrontierWebHandler {
	return &EfficientFrontierWebHandler{
		apiHandler:        apiHandler,
		portfolioSvc:      portfolioSvc,
		modelPortfolioSvc: modelPortfolioSvc,
		renderer:          renderer,
		saveModelPortfolio: func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
			return modelportfolio.ModelPortfolio{}, nil
		},
	}
}

// RegisterRoutes mounts web efficient frontier routes on the given router.
func (h *EfficientFrontierWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/efficient-frontier", h.HandleEfficientFrontier)
	r.Post("/efficient-frontier/save", h.HandleSaveAsModelPortfolio)
}

// frontierPageData is the data struct for the efficient frontier page template.
type frontierPageData struct {
	web.PageData
	// Pre-serialized JSON for ECharts.
	FrontierChartData string
	// Frontier result data.
	Result          *efficientfrontier.FrontierResult
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
	SelectedRiskFreeRate string
	SelectedBaseCurrency string
	// Period button URLs.
	PeriodURLs map[string]string
	// Selected key portfolio (for display after clicking a point).
	SelectedPortfolio *efficientfrontier.OptimizedPortfolio
}

// HandleEfficientFrontier renders GET /efficient-frontier.
func (h *EfficientFrontierWebHandler) HandleEfficientFrontier(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse symbols.
	symbols := parseFrontierSymbols(query.Get("symbols"))

	// Parse period.
	period := query.Get("period")
	if period == "" || !validFrontierPeriods[period] {
		period = "1Y"
	}

	// Parse risk-free rate.
	riskFreeRate := parseRiskFreeRate(query.Get("risk_free_rate"))

	// Parse base currency.
	baseCurrency := query.Get("base_currency")
	if baseCurrency != "" && !validBaseCurrencies[baseCurrency] {
		baseCurrency = ""
	}

	// Fetch selectors.
	portfolios := h.fetchPortfolios(r.Context())
	modelPortfolios := h.fetchModelPortfolios(r.Context())
	candidateSymbols := h.fetchCandidateSymbols(r.Context())

	// Compute frontier.
	var result *efficientfrontier.FrontierResult
	var warnings, excludedSymbols []string
	var selectedPortfolio *efficientfrontier.OptimizedPortfolio

	if len(symbols) >= 2 {
		serviceReq := efficientfrontier.ComputeFrontierRequest{
			Symbols:      symbols,
			Period:       period,
			RiskFreeRate: riskFreeRate / 100.0,
			BaseCurrency: baseCurrency,
		}
		serviceResult, err := h.apiHandler.svc.ComputeFrontier(r.Context(), serviceReq)
		if err != nil {
				data := h.buildPageData(w, r, symbols, period, riskFreeRate, baseCurrency, portfolios, modelPortfolios, candidateSymbols, nil, nil, nil, nil)
			data.Error = frontierErrorMessage(err)
			if err := h.renderer.Render(w, "efficient_frontier/index", data); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		result = serviceResult.Result
		warnings = serviceResult.Warnings
		excludedSymbols = serviceResult.ExcludedSymbols

		// Default selected portfolio is Max Sharpe.
		if result.MaxSharpe != nil {
			selectedPortfolio = result.MaxSharpe
		} else if result.MinVariance != nil {
			selectedPortfolio = result.MinVariance
		}
	}

	data := h.buildPageData(w, r, symbols, period, riskFreeRate, baseCurrency, portfolios, modelPortfolios, candidateSymbols, result, warnings, excludedSymbols, selectedPortfolio)

	if err := h.renderer.Render(w, "efficient_frontier/index", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// buildPageData assembles the frontier page data struct with serialized chart data.
func (h *EfficientFrontierWebHandler) buildPageData(
	w http.ResponseWriter, r *http.Request,
	symbols []string, period string, riskFreeRate float64, baseCurrency string,
	portfolios []portfolio.Portfolio,
	modelPortfolios []modelportfolio.ModelPortfolioSummary,
	candidateSymbols []string,
	result *efficientfrontier.FrontierResult,
	warnings, excludedSymbols []string,
	selectedPortfolio *efficientfrontier.OptimizedPortfolio,
) frontierPageData {
	frontierChart := serializeFrontierChartData(result)

	return frontierPageData{
		PageData: web.PageData{
			Title: "Efficient Frontier",
			Flash: getFlash(w, r),
		},
		FrontierChartData:    frontierChart,
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
		SelectedRiskFreeRate: strconv.FormatFloat(riskFreeRate, 'f', 1, 64),
		SelectedBaseCurrency: baseCurrency,
		PeriodURLs:           buildFrontierPeriodURLs(symbols, period, riskFreeRate, baseCurrency),
		SelectedPortfolio:    selectedPortfolio,
	}
}

// parseFrontierSymbols parses comma-separated symbols from query params.
func parseFrontierSymbols(s string) []string {
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

// parseRiskFreeRate parses a risk-free rate percentage from query params.
func parseRiskFreeRate(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return frontierDefaultRiskFreeRate
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || v > 50 {
		return frontierDefaultRiskFreeRate
	}
	return v
}

// buildFrontierPeriodURLs pre-builds the URL for each period button.
var validBaseCurrencies = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "JPY": true,
	"CHF": true, "CAD": true, "AUD": true, "CNY": true,
}

func buildFrontierPeriodURLs(symbols []string, _ string, riskFreeRate float64, baseCurrency string) map[string]string {
	urls := make(map[string]string)
	for _, p := range frontierPeriods {
		url := "/efficient-frontier"
		hasQuery := false
		if len(symbols) > 0 {
			url += "?symbols=" + strings.Join(symbols, ",")
			hasQuery = true
		}
		if p != "1Y" {
			if hasQuery {
				url += "&period=" + p
			} else {
				url += "?period=" + p
				hasQuery = true
			}
		}
		rfr := strconv.FormatFloat(riskFreeRate, 'f', 1, 64)
		if rfr != "4.5" {
			if hasQuery {
				url += "&risk_free_rate=" + rfr
			} else {
				url += "?risk_free_rate=" + rfr
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
func (h *EfficientFrontierWebHandler) fetchPortfolios(ctx context.Context) []portfolio.Portfolio {
	if h.portfolioSvc == nil {
		return []portfolio.Portfolio{}
	}
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		slog.Error("efficient frontier: fetch portfolios", "error", err)
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// fetchModelPortfolios returns model portfolio summaries for the dropdown.
func (h *EfficientFrontierWebHandler) fetchModelPortfolios(ctx context.Context) []modelportfolio.ModelPortfolioSummary {
	if h.modelPortfolioSvc == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	summaries, err := h.modelPortfolioSvc.GetAllForSelector(ctx)
	if err != nil {
		slog.Error("efficient frontier: fetch model portfolios", "error", err)
		return []modelportfolio.ModelPortfolioSummary{}
	}
	if summaries == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	return summaries
}

// fetchCandidateSymbols returns all known internal symbols for autocomplete.
func (h *EfficientFrontierWebHandler) fetchCandidateSymbols(ctx context.Context) []string {
	if h.apiHandler == nil {
		return []string{}
	}
	symbols, err := h.apiHandler.svc.GetCandidateSymbols(ctx)
	if err != nil {
		slog.Error("efficient frontier: fetch candidate symbols", "error", err)
		return []string{}
	}
	if symbols == nil {
		return []string{}
	}
	return symbols
}

// frontierPointData is a single frontier point with weights for click interaction.
type frontierPointData struct {
	Volatility  float64   `json:"volatility"`
	Return      float64   `json:"return"`
	SharpeRatio float64   `json:"sharpe_ratio"`
	Weights     []float64 `json:"weights"`
}

// frontierChartData holds JSON data for the ECharts scatter plot.
type frontierChartData struct {
	// Frontier points with weights for click interaction.
	FrontierPoints []frontierPointData `json:"frontier_points"`
	// Key portfolios.
	MaxSharpe     *frontierKeyPortfolio `json:"max_sharpe,omitempty"`
	MinVariance   *frontierKeyPortfolio `json:"min_variance,omitempty"`
	HighestReturn *frontierKeyPortfolio `json:"highest_return,omitempty"`
	// Symbol names for allocation weight display.
	Symbols []string `json:"symbols"`
	// Expected returns per symbol as percentages (e.g. 15.0 = 15%).
	ExpectedReturns []float64 `json:"expected_returns,omitempty"`
	// TradingDays is the number of trading days the data covers.
	TradingDays int `json:"trading_days"`
}

// frontierKeyPortfolio is a key portfolio point for the chart.
type frontierKeyPortfolio struct {
	Name        string    `json:"name"`
	Volatility  float64   `json:"volatility"`
	Return      float64   `json:"return"`
	SharpeRatio float64   `json:"sharpe_ratio"`
	Weights     []float64 `json:"weights"`
}

// serializeFrontierChartData converts the frontier result to JSON for ECharts.
func serializeFrontierChartData(result *efficientfrontier.FrontierResult) string {
	if result == nil || len(result.FrontierPoints) == 0 {
		return "{}"
	}

	// Build frontier points with weights for click interaction.
	points := make([]frontierPointData, 0, len(result.FrontierPoints))
	for _, pt := range result.FrontierPoints {
		points = append(points, frontierPointData{
			Volatility:  pt.VolatilityPct,
			Return:      pt.ReturnPct,
			SharpeRatio: pt.SharpeRatio,
			Weights:     pt.Weights,
		})
	}

	data := frontierChartData{
		FrontierPoints:    points,
		Symbols:           result.Symbols,
		ExpectedReturns:   result.ExpectedReturns,
		TradingDays:       result.TradingDays,
	}

	if result.MaxSharpe != nil {
		data.MaxSharpe = &frontierKeyPortfolio{
			Name:        result.MaxSharpe.Name,
			Volatility:  result.MaxSharpe.VolatilityPct,
			Return:      result.MaxSharpe.ReturnPct,
			SharpeRatio: result.MaxSharpe.SharpeRatio,
			Weights:     result.MaxSharpe.Weights,
		}
	}
	if result.MinVariance != nil {
		data.MinVariance = &frontierKeyPortfolio{
			Name:        result.MinVariance.Name,
			Volatility:  result.MinVariance.VolatilityPct,
			Return:      result.MinVariance.ReturnPct,
			SharpeRatio: result.MinVariance.SharpeRatio,
			Weights:     result.MinVariance.Weights,
		}
	}
	if result.HighestReturn != nil {
		data.HighestReturn = &frontierKeyPortfolio{
			Name:        result.HighestReturn.Name,
			Volatility:  result.HighestReturn.VolatilityPct,
			Return:      result.HighestReturn.ReturnPct,
			SharpeRatio: result.HighestReturn.SharpeRatio,
			Weights:     result.HighestReturn.Weights,
		}
	}

	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// HandleSaveAsModelPortfolio handles POST /efficient-frontier/save.
// Saves the selected frontier portfolio as a model portfolio.
func (h *EfficientFrontierWebHandler) HandleSaveAsModelPortfolio(w http.ResponseWriter, r *http.Request) {
	// Parse form data.
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "Optimized Portfolio"
	}

	// Parse weights from form: weight_0, weight_1, etc.
	weights := r.Form["weight"]
	symbols := r.Form["symbol"]

	if len(weights) == 0 || len(symbols) == 0 {
		setFlash(w, "No allocation data provided")
		http.Redirect(w, r, "/efficient-frontier", http.StatusSeeOther)
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
		// Weight is a fraction (0.0-1.0) from the frontier, convert to percentage.
		entries = append(entries, modelportfolio.ModelPortfolioEntry{
			Symbol:    sym,
			WeightPct: decimal.MustParse(strconv.FormatFloat(weight*100, 'f', 2, 64)),
		})
	}

	if len(entries) == 0 {
		setFlash(w, "No valid allocation entries")
		http.Redirect(w, r, "/efficient-frontier", http.StatusSeeOther)
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
		http.Redirect(w, r, "/efficient-frontier", http.StatusSeeOther)
		return
	}

	setFlash(w, "Model portfolio \""+mp.Name+"\" created successfully")
	http.Redirect(w, r, "/model-portfolios", http.StatusSeeOther)
}

// frontierErrorMessage maps a computation error to a user-facing message.
func frontierErrorMessage(err error) string {
	switch {
	case err == efficientfrontier.ErrInsufficientSymbols:
		return "At least 2 symbols are required for frontier computation."
	case err == efficientfrontier.ErrTooManySymbols:
		return "Too many symbols. Maximum 10 symbols supported."
	case err == efficientfrontier.ErrInsufficientData:
		return "Insufficient price data for the selected period. Try a shorter period or different symbols."
	case err == efficientfrontier.ErrSingularMatrix:
		return "The covariance matrix is singular — likely caused by perfectly correlated assets. Try removing duplicate or highly correlated symbols."
	case err == efficientfrontier.ErrNumericalFailure:
		return "The optimization failed due to a numerical error. Try different symbols or a shorter period."
	default:
		return "An unexpected error occurred while computing the efficient frontier."
	}
}

// modelPortfolioCreator defines the method needed to create a model portfolio.
type modelPortfolioCreator interface {
	Create(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error)
}

// WithModelPortfolioCreator sets the model portfolio creator for saving.
func (h *EfficientFrontierWebHandler) WithModelPortfolioCreator(creator modelPortfolioCreator) {
	h.saveModelPortfolio = func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
		return creator.Create(ctx, req)
	}
}

// serializeSliceForJS serializes a slice to JSON for embedding in JavaScript.
func serializeSliceForJS(data interface{}) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "[]"
	}
	return string(b)
}
