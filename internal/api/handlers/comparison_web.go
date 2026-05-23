package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/comparison"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// comparisonFilter holds parsed filter parameters for the comparison page.
type comparisonFilter struct {
	PortfolioAID   int64
	PortfolioAType comparison.PortfolioType
	PortfolioBID   int64
	PortfolioBType comparison.PortfolioType
	Period         string
	DateFrom       string
	DateTo         string
	BaseCurrency   string
	StartingValue  string
}

// yearlyReturnRow holds a merged yearly return for template display.
type yearlyReturnRow struct {
	Year    int
	ReturnA *float64
	ReturnB *float64
}

// comparisonPageData is the data struct for the comparison page template.
type comparisonPageData struct {
	web.PageData
	Result *comparison.ComparisonResult
	// Pre-serialized JSON for ECharts.
	ValueGrowthChartData    string
	DrawdownChartData       string
	AnnualReturnsChartData  string
	AnnualHistogramAChart   string
	AnnualHistogramBChart   string
	MonthlyHistogramAChart  string
	MonthlyHistogramBChart  string
	OverlapChartData        string
	CorrelationMatrixAChart string
	CorrelationMatrixBChart string
	// Merged yearly returns for the table.
	YearlyReturnsMerged []yearlyReturnRow
	// Selectors.
	Portfolios      []portfolio.Portfolio
	ModelPortfolios []modelportfolio.ModelPortfolioSummary
	// UI state.
	SelectedPortfolioA     string // combined: m123 or r456
	SelectedPortfolioAType string
	SelectedPortfolioB     string // combined: m123 or r456
	SelectedPortfolioBType string
	SelectedPeriod         string
	SelectedDateFrom       string
	SelectedDateTo         string
	SelectedBaseCurrency   string
	SelectedStartingValue  string
	// Period button URLs.
	PeriodURLs map[string]string
}

// comparisonModelPortfolioSelector defines the methods needed to fetch model
// portfolios for the dropdown selector.
type comparisonModelPortfolioSelector interface {
	GetAllForSelector(ctx context.Context) ([]modelportfolio.ModelPortfolioSummary, error)
}

// ComparisonWebHandler handles server-rendered comparison pages.
type ComparisonWebHandler struct {
	apiHandler        *ComparisonHandler
	portfolioSvc      *portfolio.Service
	modelPortfolioSvc comparisonModelPortfolioSelector
	renderer          *web.Renderer
}

// NewComparisonWebHandler creates a new comparison web handler.
func NewComparisonWebHandler(
	apiHandler *ComparisonHandler,
	portfolioSvc *portfolio.Service,
	modelPortfolioSvc comparisonModelPortfolioSelector,
	renderer *web.Renderer,
) *ComparisonWebHandler {
	return &ComparisonWebHandler{
		apiHandler:        apiHandler,
		portfolioSvc:      portfolioSvc,
		modelPortfolioSvc: modelPortfolioSvc,
		renderer:          renderer,
	}
}

// RegisterRoutes mounts web comparison routes on the given router.
func (h *ComparisonWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/comparison", h.HandleComparison)
}

// HandleComparison renders GET /comparison (portfolio comparison page).
func (h *ComparisonWebHandler) HandleComparison(w http.ResponseWriter, r *http.Request) {
	filter := parseComparisonFilter(r.URL.Query())

	// Fetch selectors.
	portfolios := h.fetchPortfolios(r.Context())
	modelPortfolios := h.fetchModelPortfolios(r.Context())

	// Default period.
	period := filter.Period
	if period == "" {
		period = "1Y"
	}

	// Build domain request.
	req := buildComparisonRequest(filter)

	var result *comparison.ComparisonResult
	var err error
	if h.apiHandler != nil {
		result, err = h.apiHandler.computeResult(r.Context(), req)
	}
	if err != nil {
		data := h.buildPageData(w, r, filter, portfolios, modelPortfolios, nil, period)
		data.Error = "An error occurred while computing comparison data."
		if err := h.renderer.Render(w, "comparison/index", data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	data := h.buildPageData(w, r, filter, portfolios, modelPortfolios, result, period)

	if err := h.renderer.Render(w, "comparison/index", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// buildPageData assembles the comparison page data struct with serialized chart data.
func (h *ComparisonWebHandler) buildPageData(
	w http.ResponseWriter, r *http.Request,
	filter comparisonFilter,
	portfolios []portfolio.Portfolio,
	modelPortfolios []modelportfolio.ModelPortfolioSummary,
	result *comparison.ComparisonResult,
	period string,
) comparisonPageData {
	// Serialize chart data.
	valueGrowthChart := serializeValueGrowthChartData(result, filter.StartingValue)
	drawdownChart := serializeDrawdownChartData(result)
	annualReturnsChart := serializeAnnualReturnsChartData(result)
	annualHistA := serializeAnnualFrequencyHistogram(result, "A")
	annualHistB := serializeAnnualFrequencyHistogram(result, "B")
	monthlyHistA := serializeMonthlyHistogram(result, "A")
	monthlyHistB := serializeMonthlyHistogram(result, "B")
	overlapChart := serializeOverlapChartData(result)
	corrMatrixA := serializeCorrelationMatrix(result, "A")
	corrMatrixB := serializeCorrelationMatrix(result, "B")
	mergedYearly := mergeYearlyReturns(result)

	return comparisonPageData{
		PageData:                web.PageData{Title: "Portfolio Comparison", Flash: getFlash(w, r)},
		Result:                  result,
		ValueGrowthChartData:    valueGrowthChart,
		DrawdownChartData:       drawdownChart,
		AnnualReturnsChartData:  annualReturnsChart,
		AnnualHistogramAChart:   annualHistA,
		AnnualHistogramBChart:   annualHistB,
		MonthlyHistogramAChart:  monthlyHistA,
		MonthlyHistogramBChart:  monthlyHistB,
		OverlapChartData:        overlapChart,
		CorrelationMatrixAChart: corrMatrixA,
		CorrelationMatrixBChart: corrMatrixB,
		YearlyReturnsMerged:     mergedYearly,
		Portfolios:              portfolios,
		ModelPortfolios:         modelPortfolios,
		SelectedPortfolioA:      portfolioPrefix(filter.PortfolioAType) + strconv.FormatInt(filter.PortfolioAID, 10),
		SelectedPortfolioAType:  string(filter.PortfolioAType),
		SelectedPortfolioB:      portfolioPrefix(filter.PortfolioBType) + strconv.FormatInt(filter.PortfolioBID, 10),
		SelectedPortfolioBType:  string(filter.PortfolioBType),
		SelectedPeriod:          period,
		SelectedDateFrom:        filter.DateFrom,
		SelectedDateTo:          filter.DateTo,
		SelectedBaseCurrency:    filter.BaseCurrency,
		SelectedStartingValue:   filter.StartingValue,
		PeriodURLs:              buildComparisonPeriodURLs(filter, period),
	}
}

// portfolioPrefix returns "m" for model portfolios and "r" for real portfolios.
func portfolioPrefix(t comparison.PortfolioType) string {
	if t == comparison.PortTypeReal {
		return "r"
	}
	return "m"
}

// parseComparisonFilter extracts comparison parameters from query params.
// Supports both legacy format (portfolio_a_id + portfolio_a_type) and
// new combined format (portfolio_a_id = "m123" or "r456").
func parseComparisonFilter(query map[string][]string) comparisonFilter {
	var filter comparisonFilter

	// Parse Portfolio A (combined format: m123 = model, r456 = real).
	if vals, ok := query["portfolio_a_id"]; ok && len(vals) > 0 && vals[0] != "" {
		v := vals[0]
		if len(v) > 1 {
			prefix := string(v[0])
			idStr := v[1:]
			if n, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				filter.PortfolioAID = n
				if prefix == "r" {
					filter.PortfolioAType = comparison.PortTypeReal
				} else {
					filter.PortfolioAType = comparison.PortTypeModel
				}
			}
		}
	}
	if filter.PortfolioAType == "" {
		filter.PortfolioAType = comparison.PortTypeModel
	}

	// Parse Portfolio B (combined format: m123 = model, r456 = real).
	if vals, ok := query["portfolio_b_id"]; ok && len(vals) > 0 && vals[0] != "" {
		v := vals[0]
		if len(v) > 1 {
			prefix := string(v[0])
			idStr := v[1:]
			if n, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				filter.PortfolioBID = n
				if prefix == "r" {
					filter.PortfolioBType = comparison.PortTypeReal
				} else {
					filter.PortfolioBType = comparison.PortTypeModel
				}
			}
		}
	}
	if filter.PortfolioBType == "" {
		filter.PortfolioBType = comparison.PortTypeModel
	}
	if vals, ok := query["period"]; ok && len(vals) > 0 && vals[0] != "" {
		filter.Period = vals[0]
	}
	if vals, ok := query["date_from"]; ok && len(vals) > 0 && vals[0] != "" {
		filter.DateFrom = vals[0]
	}
	if vals, ok := query["date_to"]; ok && len(vals) > 0 && vals[0] != "" {
		filter.DateTo = vals[0]
	}
	if vals, ok := query["base_currency"]; ok && len(vals) > 0 && vals[0] != "" {
		filter.BaseCurrency = vals[0]
	}
	if vals, ok := query["starting_value"]; ok && len(vals) > 0 && vals[0] != "" {
		filter.StartingValue = vals[0]
	}

	return filter
}

// buildComparisonRequest converts a web filter to a domain ComparisonRequest.
func buildComparisonRequest(f comparisonFilter) comparison.ComparisonRequest {
	return comparison.ComparisonRequest{
		PortfolioAID:   f.PortfolioAID,
		PortfolioAType: f.PortfolioAType,
		PortfolioBID:   f.PortfolioBID,
		PortfolioBType: f.PortfolioBType,
		Period:         f.Period,
		BaseCurrency:   f.BaseCurrency,
		StartingValue:  parseStartingValue(f.StartingValue),
	}
}

// parseStartingValue parses a starting value string, defaulting to 10000.
func parseStartingValue(s string) decimal.Decimal {
	if s == "" {
		return decimal.MustParse("10000")
	}
	if d, err := decimal.Parse(s); err == nil {
		return d
	}
	return decimal.MustParse("10000")
}

// fetchPortfolios returns all real portfolios for the selector dropdown.
func (h *ComparisonWebHandler) fetchPortfolios(ctx context.Context) []portfolio.Portfolio {
	if h.portfolioSvc == nil {
		return []portfolio.Portfolio{}
	}
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		slog.Warn("failed to fetch portfolios for comparison dropdown", "error", err)
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// fetchModelPortfolios returns model portfolio summaries for the dropdown selector.
func (h *ComparisonWebHandler) fetchModelPortfolios(ctx context.Context) []modelportfolio.ModelPortfolioSummary {
	if h.modelPortfolioSvc == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	summaries, err := h.modelPortfolioSvc.GetAllForSelector(ctx)
	if err != nil {
		slog.Warn("failed to fetch model portfolios for comparison dropdown", "error", err)
		return []modelportfolio.ModelPortfolioSummary{}
	}
	if summaries == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	return summaries
}

// buildComparisonPeriodURLs pre-builds the URL for each period button.
func buildComparisonPeriodURLs(filter comparisonFilter, selectedPeriod string) map[string]string {
	urls := make(map[string]string)
	periods := []string{"1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"}
	// Combined portfolio selector format: m123 = model, r456 = real.
	aPrefix := "m"
	if filter.PortfolioAType == comparison.PortTypeReal {
		aPrefix = "r"
	}
	bPrefix := "m"
	if filter.PortfolioBType == comparison.PortTypeReal {
		bPrefix = "r"
	}
	for _, p := range periods {
		url := "/comparison?"
		url += "portfolio_a_id=" + aPrefix + strconv.FormatInt(filter.PortfolioAID, 10)
		url += "&portfolio_b_id=" + bPrefix + strconv.FormatInt(filter.PortfolioBID, 10)
		url += "&period=" + p
		if filter.DateFrom != "" {
			url += "&date_from=" + filter.DateFrom
		}
		if filter.DateTo != "" {
			url += "&date_to=" + filter.DateTo
		}
		if filter.BaseCurrency != "" {
			url += "&base_currency=" + filter.BaseCurrency
		}
		if filter.StartingValue != "" {
			url += "&starting_value=" + filter.StartingValue
		}
		urls[p] = url
	}
	return urls
}

// --- Chart Serialization ---

// valueGrowthChartData holds JSON data for the value growth line chart.
type valueGrowthChartData struct {
	Dates      []string  `json:"dates"`
	PortfolioA []float64 `json:"portfolio_a"`
	PortfolioB []float64 `json:"portfolio_b"`
	NameA      string    `json:"name_a"`
	NameB      string    `json:"name_b"`
	StartValue float64   `json:"start_value"`
}

// serializeValueGrowthChartData converts equity curves to value growth chart JSON.
// Both curves are normalized to start from the user-specified starting value
// so they are directly comparable regardless of actual capital deployed.
// Dates are aligned to the intersection of both curves.
func serializeValueGrowthChartData(result *comparison.ComparisonResult, startingValueStr string) string {
	if result == nil || result.PortfolioA == nil || result.PortfolioB == nil {
		return "{}"
	}
	curveA := result.PortfolioA.ValueGrowthSeries
	curveB := result.PortfolioB.ValueGrowthSeries
	if len(curveA) == 0 && len(curveB) == 0 {
		return "{}"
	}

	// User-specified starting value, default 10000.
	startVal := 10000.0
	if sv := parseStartingValue(startingValueStr); sv.IsPos() {
		startVal, _ = sv.Float64()
	}

	// Build date-indexed maps for both curves (normalized to startVal).
	var baseA, baseB float64
	if len(curveA) > 0 {
		baseA, _ = curveA[0].PortfolioValue.Float64()
	}
	if len(curveB) > 0 {
		baseB, _ = curveB[0].PortfolioValue.Float64()
	}

	mapA := make(map[string]float64)
	for _, pt := range curveA {
		val, _ := pt.PortfolioValue.Float64()
		if baseA > 0 {
			val = val * startVal / baseA
		}
		mapA[pt.Date.Format("2006-01-02")] = val
	}
	mapB := make(map[string]float64)
	for _, pt := range curveB {
		val, _ := pt.PortfolioValue.Float64()
		if baseB > 0 {
			val = val * startVal / baseB
		}
		mapB[pt.Date.Format("2006-01-02")] = val
	}

	// Find intersection of dates.
	dateSet := make(map[string]bool)
	for d := range mapA {
		if _, ok := mapB[d]; ok {
			dateSet[d] = true
		}
	}
	// If no intersection, use all dates from A.
	if len(dateSet) == 0 {
		for d := range mapA {
			dateSet[d] = true
		}
	}

	// Sort dates.
	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	// Build aligned arrays.
	data := valueGrowthChartData{
		Dates:      dates,
		NameA:      result.PortfolioA.Name,
		NameB:      result.PortfolioB.Name,
		StartValue: startVal,
	}
	for _, d := range dates {
		data.PortfolioA = append(data.PortfolioA, mapA[d])
		data.PortfolioB = append(data.PortfolioB, mapB[d])
	}

	b, _ := json.Marshal(data)
	return string(b)
}

// drawdownChartData holds JSON data for the drawdown line chart.
type drawdownChartData struct {
	Dates      []string  `json:"dates"`
	PortfolioA []float64 `json:"portfolio_a"`
	PortfolioB []float64 `json:"portfolio_b"`
	NameA      string    `json:"name_a"`
	NameB      string    `json:"name_b"`
}

// serializeDrawdownChartData converts the comparison result to drawdown chart JSON.
func serializeDrawdownChartData(result *comparison.ComparisonResult) string {
	if result == nil || result.PortfolioA == nil || result.PortfolioB == nil {
		return "{}"
	}
	seriesA := result.PortfolioA.DrawdownSeries
	seriesB := result.PortfolioB.DrawdownSeries
	if len(seriesA) == 0 && len(seriesB) == 0 {
		return "{}"
	}

	// Collect all dates (union of both series).
	dateSet := make(map[string]bool)
	mapA := make(map[string]float64)
	mapB := make(map[string]float64)
	for _, p := range seriesA {
		key := p.Date.Format("2006-01-02")
		dateSet[key] = true
		v, _ := p.Pct.Float64()
		mapA[key] = v
	}
	for _, p := range seriesB {
		key := p.Date.Format("2006-01-02")
		dateSet[key] = true
		v, _ := p.Pct.Float64()
		mapB[key] = v
	}
	var dates []string
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	aVals := make([]float64, len(dates))
	bVals := make([]float64, len(dates))
	for i, d := range dates {
		// Negate so drawdown goes downward from 0% (inverted chart).
		// Drawdown data is stored as positive percentages (e.g. 15.50 = 15.50% below peak).
		// Chart displays negative values so the line drops below the 0% axis.
		aVals[i] = -mapA[d]
		bVals[i] = -mapB[d]
	}

	data := drawdownChartData{
		Dates:      dates,
		PortfolioA: aVals,
		PortfolioB: bVals,
		NameA:      portfolioName(result.PortfolioA),
		NameB:      portfolioName(result.PortfolioB),
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// annualReturnsChartData holds JSON data for the annual returns bar chart.
type annualReturnsChartData struct {
	Years   []string  `json:"years"`
	ReturnA []float64 `json:"return_a"`
	ReturnB []float64 `json:"return_b"`
	NameA   string    `json:"name_a"`
	NameB   string    `json:"name_b"`
}

// serializeAnnualReturnsChartData converts yearly returns to side-by-side bar chart JSON.
func serializeAnnualReturnsChartData(result *comparison.ComparisonResult) string {
	if result == nil || result.PortfolioA == nil || result.PortfolioB == nil {
		return "{}"
	}

	yearsA := make(map[string]float64)
	for _, yr := range result.PortfolioA.YearlyReturns {
		if yr.ReturnPct != nil {
			v, _ := yr.ReturnPct.Float64()
			yearsA[strconv.Itoa(yr.Year)] = v
		}
	}
	yearsB := make(map[string]float64)
	for _, yr := range result.PortfolioB.YearlyReturns {
		if yr.ReturnPct != nil {
			v, _ := yr.ReturnPct.Float64()
			yearsB[strconv.Itoa(yr.Year)] = v
		}
	}

	// Collect all years sorted.
	yearSet := make(map[string]bool)
	for y := range yearsA {
		yearSet[y] = true
	}
	for y := range yearsB {
		yearSet[y] = true
	}
	var years []string
	for y := range yearSet {
		years = append(years, y)
	}
	sort.Strings(years)

	returnA := make([]float64, len(years))
	returnB := make([]float64, len(years))
	for i, y := range years {
		returnA[i] = yearsA[y]
		returnB[i] = yearsB[y]
	}

	data := annualReturnsChartData{
		Years:   years,
		ReturnA: returnA,
		ReturnB: returnB,
		NameA:   portfolioName(result.PortfolioA),
		NameB:   portfolioName(result.PortfolioB),
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// monthlyHistogramData holds JSON data for the monthly return frequency histogram.
type monthlyHistogramData struct {
	Bins   []string  `json:"bins"`
	Counts []float64 `json:"counts"`
}

// serializeMonthlyHistogram converts the monthly return distribution to histogram JSON.
// portfolioSide is "A" or "B".
func serializeMonthlyHistogram(result *comparison.ComparisonResult, portfolioSide string) string {
	if result == nil {
		return "{}"
	}

	var dist *comparison.ReturnDistribution
	switch portfolioSide {
	case "A":
		if result.PortfolioA != nil && result.PortfolioA.ReturnDistribution != nil {
			dist = result.PortfolioA.ReturnDistribution
		}
	case "B":
		if result.PortfolioB != nil && result.PortfolioB.ReturnDistribution != nil {
			dist = result.PortfolioB.ReturnDistribution
		}
	}

	if dist == nil || len(dist.Monthly) == 0 {
		return "{}"
	}

	bins := make([]string, len(dist.Monthly))
	counts := make([]float64, len(dist.Monthly))
	for i, b := range dist.Monthly {
		bins[i] = b.Label
		counts[i] = float64(b.Count)
	}

	data := monthlyHistogramData{
		Bins:   bins,
		Counts: counts,
	}
	j, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(j)
}

// overlapChartData holds JSON data for the overlap visualization.
type overlapChartData struct {
	HoldingsA  []string  `json:"holdings_a"`
	WeightsA   []float64 `json:"weights_a"`
	HoldingsB  []string  `json:"holdings_b"`
	WeightsB   []float64 `json:"weights_b"`
	OverlapPct *float64  `json:"overlap_pct,omitempty"`
}

// serializeOverlapChartData converts overlap result to chart JSON.
func serializeOverlapChartData(result *comparison.ComparisonResult) string {
	if result == nil || result.CrossMetrics == nil || result.CrossMetrics.Overlap == nil {
		return "{}"
	}
	overlap := result.CrossMetrics.Overlap

	holdingsA := make([]string, len(overlap.TopHoldingsA))
	weightsA := make([]float64, len(overlap.TopHoldingsA))
	for i, h := range overlap.TopHoldingsA {
		holdingsA[i] = h.Symbol
		v, _ := h.Weight.Float64()
		weightsA[i] = v * 100 // fraction → percentage
	}

	holdingsB := make([]string, len(overlap.TopHoldingsB))
	weightsB := make([]float64, len(overlap.TopHoldingsB))
	for i, h := range overlap.TopHoldingsB {
		holdingsB[i] = h.Symbol
		v, _ := h.Weight.Float64()
		weightsB[i] = v * 100
	}

	var overlapPct *float64
	if overlap.OverlapPct != nil {
		v, _ := overlap.OverlapPct.Float64()
		overlapPct = &v
	}

	data := overlapChartData{
		HoldingsA:  holdingsA,
		WeightsA:   weightsA,
		HoldingsB:  holdingsB,
		WeightsB:   weightsB,
		OverlapPct: overlapPct,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// serializeAnnualFrequencyHistogram converts the annual return frequency
// distribution to histogram JSON. portfolioSide is "A" or "B".
func serializeAnnualFrequencyHistogram(result *comparison.ComparisonResult, portfolioSide string) string {
	if result == nil {
		return "{}"
	}

	var dist *comparison.ReturnDistribution
	switch portfolioSide {
	case "A":
		if result.PortfolioA != nil && result.PortfolioA.ReturnDistribution != nil {
			dist = result.PortfolioA.ReturnDistribution
		}
	case "B":
		if result.PortfolioB != nil && result.PortfolioB.ReturnDistribution != nil {
			dist = result.PortfolioB.ReturnDistribution
		}
	}

	if dist == nil || len(dist.AnnualBinned) == 0 {
		return "{}"
	}

	bins := make([]string, len(dist.AnnualBinned))
	counts := make([]float64, len(dist.AnnualBinned))
	for i, b := range dist.AnnualBinned {
		bins[i] = b.Label
		counts[i] = float64(b.Count)
	}

	data := monthlyHistogramData{
		Bins:   bins,
		Counts: counts,
	}
	j, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(j)
}

// correlationMatrixData holds JSON data for the correlation matrix heatmap.
type correlationMatrixData struct {
	Symbols []string     `json:"symbols"`
	Matrix  [][]*float64 `json:"matrix"`
	Name    string       `json:"name"`
}

// serializeCorrelationMatrix converts the intra-portfolio correlation matrix
// to heatmap JSON. portfolioSide is "A" or "B".
func serializeCorrelationMatrix(result *comparison.ComparisonResult, portfolioSide string) string {
	if result == nil {
		return "{}"
	}

	var corr *comparison.IntraPortfolioCorrelationResult
	switch portfolioSide {
	case "A":
		if result.PortfolioA != nil && result.PortfolioA.IntraCorrelation != nil {
			corr = result.PortfolioA.IntraCorrelation
		}
	case "B":
		if result.PortfolioB != nil && result.PortfolioB.IntraCorrelation != nil {
			corr = result.PortfolioB.IntraCorrelation
		}
	}

	if corr == nil || corr.Matrix == nil || len(corr.Symbols) == 0 {
		return "{}"
	}

	name := ""
	switch portfolioSide {
	case "A":
		name = portfolioName(result.PortfolioA)
	case "B":
		name = portfolioName(result.PortfolioB)
	}

	data := correlationMatrixData{
		Symbols: corr.Symbols,
		Matrix:  corr.Matrix,
		Name:    name,
	}
	j, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(j)
}

// portfolioName returns the name from a PortfolioComparison or "Portfolio N".
func portfolioName(pc *comparison.PortfolioComparison) string {
	if pc == nil {
		return ""
	}
	if pc.Name != "" {
		return pc.Name
	}
	return "Portfolio " + strconv.FormatInt(pc.ID, 10)
}

// mergeYearlyReturns merges yearly returns from both portfolios into a single
// sorted list for the comparison table.
func mergeYearlyReturns(result *comparison.ComparisonResult) []yearlyReturnRow {
	if result == nil {
		return nil
	}

	// Index by year.
	returnsA := make(map[int]*float64)
	if result.PortfolioA != nil {
		for _, yr := range result.PortfolioA.YearlyReturns {
			if yr.ReturnPct != nil {
				v, _ := yr.ReturnPct.Float64()
				returnsA[yr.Year] = &v
			}
		}
	}
	returnsB := make(map[int]*float64)
	if result.PortfolioB != nil {
		for _, yr := range result.PortfolioB.YearlyReturns {
			if yr.ReturnPct != nil {
				v, _ := yr.ReturnPct.Float64()
				returnsB[yr.Year] = &v
			}
		}
	}

	// Collect all years.
	yearSet := make(map[int]bool)
	for y := range returnsA {
		yearSet[y] = true
	}
	for y := range returnsB {
		yearSet[y] = true
	}

	rows := make([]yearlyReturnRow, 0, len(yearSet))
	for y := range yearSet {
		rows = append(rows, yearlyReturnRow{
			Year:    y,
			ReturnA: returnsA[y],
			ReturnB: returnsB[y],
		})
	}

	// Sort by year.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Year < rows[j-1].Year; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}

	return rows
}
