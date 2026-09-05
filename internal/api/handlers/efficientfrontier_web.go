package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eddiectc/portfoliolab/internal/domain/efficientfrontier"
	"github.com/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/optimization"
	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
	"github.com/eddiectc/portfoliolab/internal/web"
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
	// Shared optimization partial fields.
	FormID         string // "frontier"
	FormAction     string // "/efficient-frontier"
	FormButtonText string // "Compute Frontier"
	SymbolHint     string // "Comma-separated symbols. Min 2, max 10."
	ApiBase        string // "/api/efficient-frontier"
	RiskFreeRate   bool   // true for frontier (shows risk-free rate field)
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
	// Key portfolio display values (annualized when Annualized is true).
	DisplayMaxSharpe     *displayPortfolio
	DisplayMinVariance   *displayPortfolio
	DisplayHighestReturn *displayPortfolio
	DisplayMaxSortino    *displayPortfolio
	DisplayMinDrawdown   *displayPortfolio
	// LeastDataSymbol is the symbol with the fewest trading days in the result.
	LeastDataSymbol string
	// LeastDataDays is the trading day count of the symbol with least data.
	LeastDataDays int
	// ExpectedTradingDays is the approximate expected trading days for the selected period.
	ExpectedTradingDays int
	// Annualized is true when return/volatility are displayed as annualized figures.
	Annualized bool
}

// displayPortfolio holds annualized or period return/vol for template display.
type displayPortfolio struct {
	Name           string
	ReturnPct      float64
	VolatilityPct  float64
	SharpeRatio    float64
	SortinoRatio   float64
	MaxDrawdownPct float64
}

// HandleEfficientFrontier renders GET /efficient-frontier.
func (h *EfficientFrontierWebHandler) HandleEfficientFrontier(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse symbols.
	symbols := parseOptimizationSymbols(query.Get("symbols"))

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
	portfolios := fetchOptimizationPortfolios(h.portfolioSvc, r.Context(), "efficient frontier")
	modelPortfolios := fetchOptimizationModelPortfolios(h.modelPortfolioSvc, r.Context(), "efficient frontier")
	candidateSymbols := fetchCandidateSymbolsFromService(h.apiHandler.svc, r.Context(), "efficient frontier")

	// Compute frontier.
	var result *efficientfrontier.FrontierResult
	var warnings, excludedSymbols []string
	var selectedPortfolio *efficientfrontier.OptimizedPortfolio
	var symbolDataSpan map[string]optimization.DataSpan

	if len(symbols) >= 2 {
		serviceReq := efficientfrontier.ComputeFrontierRequest{
			Symbols:      symbols,
			Period:       period,
			RiskFreeRate: riskFreeRate / 100.0,
			BaseCurrency: baseCurrency,
		}
		serviceResult, err := h.apiHandler.svc.ComputeFrontier(r.Context(), serviceReq)
		if err != nil {
			data := h.buildPageData(w, r, symbols, period, riskFreeRate, baseCurrency, portfolios, modelPortfolios, candidateSymbols, nil, nil, nil, nil, nil)
			data.Error = frontierErrorMessage(err)
			if err := h.renderer.Render(w, "efficient_frontier/index", data); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		result = serviceResult.Result
		warnings = serviceResult.Warnings
		excludedSymbols = serviceResult.ExcludedSymbols
		symbolDataSpan = serviceResult.SymbolDataSpan

		// Default selected portfolio is Max Sharpe.
		if result.MaxSharpe != nil {
			selectedPortfolio = result.MaxSharpe
		} else if result.MinVariance != nil {
			selectedPortfolio = result.MinVariance
		}
	}

	data := h.buildPageData(w, r, symbols, period, riskFreeRate, baseCurrency, portfolios, modelPortfolios, candidateSymbols, result, warnings, excludedSymbols, selectedPortfolio, symbolDataSpan)

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
	symbolDataSpan map[string]optimization.DataSpan,
) frontierPageData {
	// Find symbol with least data.
	leastDataSymbol, leastDataDays := findLeastDataSymbol(symbolDataSpan)

	// Compute expected trading days from the actual data date range.
	expectedDays := computeExpectedTradingDays(symbolDataSpan)

	// Determine whether to annualize: data must be at least 90% of expected.
	annualized := false
	retFactor, volFactor := 1.0, 1.0
	if expectedDays > 0 && result != nil {
		if float64(leastDataDays)/float64(expectedDays) >= 0.90 {
			annualized = true
			if result.TradingDays > 0 {
				retFactor = 252.0 / float64(result.TradingDays)
				volFactor = math.Sqrt(retFactor)
			}
		}
	}

	// Build chart data.
	frontierChart := serializeFrontierChartData(result, annualized)

	data := frontierPageData{
		PageData: web.PageData{
			Title: "Efficient Frontier",
			Flash: getFlash(w, r),
		},
		FormID:               "frontier",
		FormAction:           "/efficient-frontier",
		FormButtonText:       "Compute Frontier",
		SymbolHint:           "Comma-separated symbols. Min 2, max 10.",
		ApiBase:              "/api/efficient-frontier",
		RiskFreeRate:         true,
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
		LeastDataSymbol:      leastDataSymbol,
		LeastDataDays:        leastDataDays,
		ExpectedTradingDays:  expectedDays,
		Annualized:           annualized,
	}

	// Populate display portfolios with annualized values.
	populateDisplayPortfolios(&data, result, retFactor, volFactor)

	return data
}

func populateDisplayPortfolios(data *frontierPageData, result *efficientfrontier.FrontierResult, retFactor, volFactor float64) {
	if result == nil {
		return
	}
	if result.MaxSharpe != nil {
		data.DisplayMaxSharpe = &displayPortfolio{
			Name:           result.MaxSharpe.Name,
			ReturnPct:      result.MaxSharpe.ReturnPct * retFactor,
			VolatilityPct:  result.MaxSharpe.VolatilityPct * volFactor,
			SharpeRatio:    result.MaxSharpe.SharpeRatio,
			SortinoRatio:   result.MaxSharpe.SortinoRatio,
			MaxDrawdownPct: result.MaxSharpe.MaxDrawdownPct,
		}
	}
	if result.MinVariance != nil {
		data.DisplayMinVariance = &displayPortfolio{
			Name:           result.MinVariance.Name,
			ReturnPct:      result.MinVariance.ReturnPct * retFactor,
			VolatilityPct:  result.MinVariance.VolatilityPct * volFactor,
			SharpeRatio:    result.MinVariance.SharpeRatio,
			SortinoRatio:   result.MinVariance.SortinoRatio,
			MaxDrawdownPct: result.MinVariance.MaxDrawdownPct,
		}
	}
	if result.HighestReturn != nil {
		data.DisplayHighestReturn = &displayPortfolio{
			Name:           result.HighestReturn.Name,
			ReturnPct:      result.HighestReturn.ReturnPct * retFactor,
			VolatilityPct:  result.HighestReturn.VolatilityPct * volFactor,
			SharpeRatio:    result.HighestReturn.SharpeRatio,
			SortinoRatio:   result.HighestReturn.SortinoRatio,
			MaxDrawdownPct: result.HighestReturn.MaxDrawdownPct,
		}
	}
	if result.MaxSortino != nil {
		data.DisplayMaxSortino = &displayPortfolio{
			Name:           result.MaxSortino.Name,
			ReturnPct:      result.MaxSortino.ReturnPct * retFactor,
			VolatilityPct:  result.MaxSortino.VolatilityPct * volFactor,
			SharpeRatio:    result.MaxSortino.SharpeRatio,
			SortinoRatio:   result.MaxSortino.SortinoRatio,
			MaxDrawdownPct: result.MaxSortino.MaxDrawdownPct,
		}
	}
	if result.MinDrawdown != nil {
		data.DisplayMinDrawdown = &displayPortfolio{
			Name:           result.MinDrawdown.Name,
			ReturnPct:      result.MinDrawdown.ReturnPct * retFactor,
			VolatilityPct:  result.MinDrawdown.VolatilityPct * volFactor,
			SharpeRatio:    result.MinDrawdown.SharpeRatio,
			SortinoRatio:   result.MinDrawdown.SortinoRatio,
			MaxDrawdownPct: result.MinDrawdown.MaxDrawdownPct,
		}
	}
}

// computeExpectedTradingDays counts the weekdays (Mon-Fri) between
// the earliest start and latest end date across all symbol data spans.
// This gives the upper bound of trading days before accounting for holidays.
func computeExpectedTradingDays(dataSpan map[string]optimization.DataSpan) int {
	if len(dataSpan) == 0 {
		return 0
	}
	var earliestStart, latestEnd time.Time
	first := true
	for _, span := range dataSpan {
		start, err1 := time.Parse("2006-01-02", span.StartDate)
		end, err2 := time.Parse("2006-01-02", span.EndDate)
		if err1 != nil || err2 != nil {
			continue
		}
		if first {
			earliestStart = start
			latestEnd = end
			first = false
		} else {
			if start.Before(earliestStart) {
				earliestStart = start
			}
			if end.After(latestEnd) {
				latestEnd = end
			}
		}
	}
	if first {
		return 0
	}
	return countWeekdays(earliestStart, latestEnd)
}

// countWeekdays counts the number of weekdays (Mon-Fri) between start and end inclusive.
func countWeekdays(start, end time.Time) int {
	count := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		weekday := d.Weekday()
		if weekday != time.Saturday && weekday != time.Sunday {
			count++
		}
	}
	return count
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

// frontierPointData is a single frontier point with weights for click interaction.
type frontierPointData struct {
	Volatility     float64   `json:"volatility"`
	Return         float64   `json:"return"`
	SharpeRatio    float64   `json:"sharpe_ratio"`
	SortinoRatio   float64   `json:"sortino_ratio"`
	MaxDrawdownPct float64   `json:"max_drawdown_pct"`
	Weights        []float64 `json:"weights"`
}

// frontierChartData holds JSON data for the ECharts scatter plot.
type frontierChartData struct {
	// Frontier points with weights for click interaction.
	FrontierPoints []frontierPointData `json:"frontier_points"`
	// Key portfolios.
	MaxSharpe     *frontierKeyPortfolio `json:"max_sharpe,omitempty"`
	MinVariance   *frontierKeyPortfolio `json:"min_variance,omitempty"`
	HighestReturn *frontierKeyPortfolio `json:"highest_return,omitempty"`
	MaxSortino    *frontierKeyPortfolio `json:"max_sortino,omitempty"`
	MinDrawdown   *frontierKeyPortfolio `json:"min_drawdown,omitempty"`
	// Symbol names for allocation weight display.
	Symbols []string `json:"symbols"`
	// Expected returns per symbol as percentages (e.g. 15.0 = 15%).
	ExpectedReturns []float64 `json:"expected_returns,omitempty"`
	// CorrelationMatrix is the N×N correlation matrix for the heatmap.
	CorrelationMatrix [][]float64 `json:"correlation_matrix,omitempty"`
	// TradingDays is the number of trading days the data covers.
	TradingDays int `json:"trading_days"`
}

// frontierKeyPortfolio is a key portfolio point for the chart.
type frontierKeyPortfolio struct {
	Name           string    `json:"name"`
	Volatility     float64   `json:"volatility"`
	Return         float64   `json:"return"`
	SharpeRatio    float64   `json:"sharpe_ratio"`
	SortinoRatio   float64   `json:"sortino_ratio"`
	MaxDrawdownPct float64   `json:"max_drawdown_pct"`
	Weights        []float64 `json:"weights"`
}

// serializeFrontierChartData converts the frontier result to JSON for ECharts.
// If annualized is true, return and volatility are scaled to annual figures
// using the standard 252 trading days convention.
func serializeFrontierChartData(result *efficientfrontier.FrontierResult, annualized bool) string {
	if result == nil || len(result.FrontierPoints) == 0 {
		return "{}"
	}

	retFactor, volFactor := 1.0, 1.0
	if annualized && result.TradingDays > 0 {
		retFactor = 252.0 / float64(result.TradingDays)
		volFactor = math.Sqrt(retFactor)
	}

	// Build frontier points with weights for click interaction.
	points := make([]frontierPointData, 0, len(result.FrontierPoints))
	for _, pt := range result.FrontierPoints {
		points = append(points, frontierPointData{
			Volatility:     pt.VolatilityPct * volFactor,
			Return:         pt.ReturnPct * retFactor,
			SharpeRatio:    pt.SharpeRatio,
			SortinoRatio:   pt.SortinoRatio,
			MaxDrawdownPct: pt.MaxDrawdownPct,
			Weights:        pt.Weights,
		})
	}

	expectedReturns := make([]float64, len(result.ExpectedReturns))
	for i, r := range result.ExpectedReturns {
		expectedReturns[i] = r * retFactor
	}

	data := frontierChartData{
		FrontierPoints:    points,
		Symbols:           result.Symbols,
		ExpectedReturns:   expectedReturns,
		CorrelationMatrix: result.CorrelationMatrix,
		TradingDays:       result.TradingDays,
	}

	if result.MaxSharpe != nil {
		data.MaxSharpe = &frontierKeyPortfolio{
			Name:           result.MaxSharpe.Name,
			Volatility:     result.MaxSharpe.VolatilityPct * volFactor,
			Return:         result.MaxSharpe.ReturnPct * retFactor,
			SharpeRatio:    result.MaxSharpe.SharpeRatio,
			SortinoRatio:   result.MaxSharpe.SortinoRatio,
			MaxDrawdownPct: result.MaxSharpe.MaxDrawdownPct,
			Weights:        result.MaxSharpe.Weights,
		}
	}
	if result.MinVariance != nil {
		data.MinVariance = &frontierKeyPortfolio{
			Name:           result.MinVariance.Name,
			Volatility:     result.MinVariance.VolatilityPct * volFactor,
			Return:         result.MinVariance.ReturnPct * retFactor,
			SharpeRatio:    result.MinVariance.SharpeRatio,
			SortinoRatio:   result.MinVariance.SortinoRatio,
			MaxDrawdownPct: result.MinVariance.MaxDrawdownPct,
			Weights:        result.MinVariance.Weights,
		}
	}
	if result.HighestReturn != nil {
		data.HighestReturn = &frontierKeyPortfolio{
			Name:           result.HighestReturn.Name,
			Volatility:     result.HighestReturn.VolatilityPct * volFactor,
			Return:         result.HighestReturn.ReturnPct * retFactor,
			SharpeRatio:    result.HighestReturn.SharpeRatio,
			SortinoRatio:   result.HighestReturn.SortinoRatio,
			MaxDrawdownPct: result.HighestReturn.MaxDrawdownPct,
			Weights:        result.HighestReturn.Weights,
		}
	}
	if result.MaxSortino != nil {
		data.MaxSortino = &frontierKeyPortfolio{
			Name:           result.MaxSortino.Name,
			Volatility:     result.MaxSortino.VolatilityPct * volFactor,
			Return:         result.MaxSortino.ReturnPct * retFactor,
			SharpeRatio:    result.MaxSortino.SharpeRatio,
			SortinoRatio:   result.MaxSortino.SortinoRatio,
			MaxDrawdownPct: result.MaxSortino.MaxDrawdownPct,
			Weights:        result.MaxSortino.Weights,
		}
	}
	if result.MinDrawdown != nil {
		data.MinDrawdown = &frontierKeyPortfolio{
			Name:           result.MinDrawdown.Name,
			Volatility:     result.MinDrawdown.VolatilityPct * volFactor,
			Return:         result.MinDrawdown.ReturnPct * retFactor,
			SharpeRatio:    result.MinDrawdown.SharpeRatio,
			SortinoRatio:   result.MinDrawdown.SortinoRatio,
			MaxDrawdownPct: result.MinDrawdown.MaxDrawdownPct,
			Weights:        result.MinDrawdown.Weights,
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
	handleOptimizationWebSave(w, r, "/efficient-frontier", "/model-portfolios", "Optimized Portfolio", h.saveModelPortfolio)
}

// frontierErrorMessage maps a computation error to a user-facing message.
func frontierErrorMessage(err error) string {
	switch err {
	case efficientfrontier.ErrInsufficientSymbols:
		return "At least 2 symbols are required for frontier computation."
	case efficientfrontier.ErrTooManySymbols:
		return "Too many symbols. Maximum 10 symbols supported."
	case efficientfrontier.ErrInsufficientData:
		return "Insufficient price data for the selected period. Try a shorter period or different symbols."
	case efficientfrontier.ErrSingularMatrix:
		return "The covariance matrix is singular — likely caused by perfectly correlated assets. Try removing duplicate or highly correlated symbols."
	case efficientfrontier.ErrNumericalFailure:
		return "The optimization failed due to a numerical error. Try different symbols or a shorter period."
	default:
		return "An unexpected error occurred while computing the efficient frontier."
	}
}

// WithModelPortfolioCreator sets the model portfolio creator for saving.
func (h *EfficientFrontierWebHandler) WithModelPortfolioCreator(creator modelPortfolioCreator) {
	h.saveModelPortfolio = func(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error) {
		return creator.Create(ctx, req)
	}
}
