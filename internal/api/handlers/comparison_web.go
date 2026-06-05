package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/comparison"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// =============================================================================
// COMPARISON PAGE RULES — enforce for ALL new charts/tables:
//
// 1. TWR ONLY: All comparison data is cash-flow-independent (TWR-equivalent).
//    ValueGrowthSeries already uses NavPerUnit. Never use PortfolioValue —
//    it includes deposits/withdrawals and would be inconsistent with TWR.
//
// 2. DATE ALIGNMENT: Use intersectDates(mapA, mapB) for any side-by-side
//    time series. Never use union dates — only dates where BOTH portfolios
//    have data. This ensures both lines share the same x-axis.
//
// 3. STARTING VALUE: Use parseStartingValue(filter.StartingValue) for any
//    value normalization. The user-specified starting value (default 10000)
//    is the canonical reference — never use actual portfolio values.
//
// 4. CHARTS THAT DON'T NEED DATE ALIGNMENT: Histograms, correlation matrices,
//    overlap tables, and yearly aggregations work on pre-aggregated data and
//    are exempt from rule 2.
// =============================================================================

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
	RiskFreeRate   string
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
	AnnualHistogramChart    string
	MonthlyHistogramChart   string
	OverlapChartData        string
	SectorDriftChart        string
	CountryDriftChart       string
	MergedHoldingsData      string
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
	SelectedRiskFreeRate   string
	// Effective period (intersection of both portfolios' data ranges).
	EffectiveDateFrom *time.Time
	EffectiveDateTo   *time.Time
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
	annualHist := serializeAnnualFrequencyHistogramCombined(result)
	monthlyHist := serializeMonthlyHistogramCombined(result)
	overlapChart := serializeOverlapChartData(result)
	sectorDriftChart := serializeSectorDriftChart(result)
	countryDriftChart := serializeCountryDriftChart(result)
	mergedHoldings := serializeMergedHoldings(result)
	corrMatrixA := serializeCorrelationMatrix(result, "A")
	corrMatrixB := serializeCorrelationMatrix(result, "B")
	mergedYearly := mergeYearlyReturns(result)

	// Default starting value for display.
	startingValue := filter.StartingValue
	if startingValue == "" {
		startingValue = "10000"
	}

	// Default risk-free rate for display.
	riskFreeRate := filter.RiskFreeRate
	if riskFreeRate == "" {
		riskFreeRate = "0"
	}

	pd := comparisonPageData{
		PageData:                web.PageData{Title: "Portfolio Comparison", Flash: getFlash(w, r)},
		Result:                  result,
		ValueGrowthChartData:    valueGrowthChart,
		DrawdownChartData:       drawdownChart,
		AnnualReturnsChartData:  annualReturnsChart,
		AnnualHistogramChart:    annualHist,
		MonthlyHistogramChart:   monthlyHist,
		OverlapChartData:        overlapChart,
		SectorDriftChart:        sectorDriftChart,
		CountryDriftChart:       countryDriftChart,
		MergedHoldingsData:      mergedHoldings,
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
		SelectedStartingValue:   startingValue,
		SelectedRiskFreeRate:    riskFreeRate,
		PeriodURLs:              buildComparisonPeriodURLs(filter, period, riskFreeRate),
	}
	// Compute effective period as intersection of both portfolios' data ranges.
	if result != nil {
		pd.EffectiveDateFrom, pd.EffectiveDateTo = intersectEffectiveDates(
			result.PortfolioA,
			result.PortfolioB,
		)
	}
	return pd
}

// intersectEffectiveDates returns the overlapping date range of both portfolios.
// dateFrom = max(A.from, B.from), dateTo = min(A.to, B.to).
func intersectEffectiveDates(a, b *comparison.PortfolioComparison) (*time.Time, *time.Time) {
	var fromA, toA, fromB, toB time.Time
	var hasA, hasB bool
	if a != nil && a.EffectiveDateFrom != nil && a.EffectiveDateTo != nil {
		fromA = *a.EffectiveDateFrom
		toA = *a.EffectiveDateTo
		hasA = true
	}
	if b != nil && b.EffectiveDateFrom != nil && b.EffectiveDateTo != nil {
		fromB = *b.EffectiveDateFrom
		toB = *b.EffectiveDateTo
		hasB = true
	}
	if !hasA || !hasB {
		if hasA {
			return &fromA, &toA
		}
		if hasB {
			return &fromB, &toB
		}
		return nil, nil
	}
	// Intersection: later start, earlier end.
	dateFrom := fromA
	if fromB.After(fromA) {
		dateFrom = fromB
	}
	dateTo := toA
	if toB.Before(toA) {
		dateTo = toB
	}
	if dateFrom.After(dateTo) {
		// No overlap — fall back to A's range.
		return &fromA, &toA
	}
	return &dateFrom, &dateTo
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
	if vals, ok := query["risk_free_rate"]; ok && len(vals) > 0 && vals[0] != "" {
		filter.RiskFreeRate = vals[0]
	}

	return filter
}

// buildComparisonRequest converts a web filter to a domain ComparisonRequest.
func buildComparisonRequest(f comparisonFilter) comparison.ComparisonRequest {
	req := comparison.ComparisonRequest{
		PortfolioAID:   f.PortfolioAID,
		PortfolioAType: f.PortfolioAType,
		PortfolioBID:   f.PortfolioBID,
		PortfolioBType: f.PortfolioBType,
		Period:         f.Period,
		BaseCurrency:   f.BaseCurrency,
		StartingValue:  parseStartingValue(f.StartingValue),
	}
	if f.RiskFreeRate != "" {
		if d, err := decimal.Parse(f.RiskFreeRate); err == nil {
			req.RiskFreeRatePct = &d
		}
	}
	return req
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
func buildComparisonPeriodURLs(filter comparisonFilter, selectedPeriod string, riskFreeRate string) map[string]string {
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
		if riskFreeRate != "0" {
			url += "&risk_free_rate=" + riskFreeRate
		}
		urls[p] = url
	}
	return urls
}

// --- Chart Serialization ---

// intersectDates returns sorted dates present in both maps (intersection).
// This is the canonical way to align two portfolio time series on the comparison page.
// All side-by-side charts must use this to ensure both portfolios share the same date range.
func intersectDates(a, b map[string]float64) []string {
	dateSet := make(map[string]bool)
	for d := range a {
		if _, ok := b[d]; ok {
			dateSet[d] = true
		}
	}
	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	return dates
}

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
// ValueGrowthSeries is already TWR-equivalent (NavPerUnit in PortfolioValue).
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
	// ValueGrowthSeries PortfolioValue is already NavPerUnit (TWR-equivalent).
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

	// Only dates where both portfolios have data (intersection).
	dates := intersectDates(mapA, mapB)
	if len(dates) == 0 {
		return "{}"
	}

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

	// Build date-indexed maps.
	mapA := make(map[string]float64)
	for _, p := range seriesA {
		v, _ := p.Pct.Float64()
		mapA[p.Date.Format("2006-01-02")] = v
	}
	mapB := make(map[string]float64)
	for _, p := range seriesB {
		v, _ := p.Pct.Float64()
		mapB[p.Date.Format("2006-01-02")] = v
	}

	// Only dates where both portfolios have data (intersection).
	dates := intersectDates(mapA, mapB)
	if len(dates) == 0 {
		return "{}"
	}

	aVals := make([]float64, len(dates))
	bVals := make([]float64, len(dates))
	for i, d := range dates {
		// Negate so drawdown goes downward from 0% (inverted chart).
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

	// Only years where both portfolios have data (intersection).
	yearSet := make(map[string]bool)
	for y := range yearsA {
		if _, ok := yearsB[y]; ok {
			yearSet[y] = true
		}
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

// combinedHistogramData holds JSON for a combined histogram showing both portfolios.
type combinedHistogramData struct {
	Bins    []string  `json:"bins"`
	CountsA []float64 `json:"counts_a"`
	CountsB []float64 `json:"counts_b"`
	NameA   string    `json:"name_a"`
	NameB   string    `json:"name_b"`
}

// serializeMonthlyHistogramCombined produces a combined monthly return frequency
// histogram with both portfolios side-by-side.
func serializeMonthlyHistogramCombined(result *comparison.ComparisonResult) string {
	if result == nil {
		return "{}"
	}
	distA := getDistribution(result, "A")
	distB := getDistribution(result, "B")

	if distA == nil && distB == nil {
		return "{}"
	}

	bins, countsA, countsB := mergeHistogramBins(distA, distB, func(d *comparison.ReturnDistribution) []comparison.ReturnBucket {
		if d == nil {
			return nil
		}
		return d.Monthly
	})

	data := combinedHistogramData{
		Bins:    bins,
		CountsA: countsA,
		CountsB: countsB,
		NameA:   portfolioName(result.PortfolioA),
		NameB:   portfolioName(result.PortfolioB),
	}
	j, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(j)
}

// serializeAnnualFrequencyHistogramCombined produces a combined annual return
// frequency histogram with both portfolios side-by-side.
func serializeAnnualFrequencyHistogramCombined(result *comparison.ComparisonResult) string {
	if result == nil {
		return "{}"
	}
	distA := getDistribution(result, "A")
	distB := getDistribution(result, "B")

	if distA == nil && distB == nil {
		return "{}"
	}

	bins, countsA, countsB := mergeHistogramBins(distA, distB, func(d *comparison.ReturnDistribution) []comparison.ReturnBucket {
		if d == nil {
			return nil
		}
		return d.AnnualBinned
	})

	data := combinedHistogramData{
		Bins:    bins,
		CountsA: countsA,
		CountsB: countsB,
		NameA:   portfolioName(result.PortfolioA),
		NameB:   portfolioName(result.PortfolioB),
	}
	j, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(j)
}

// getDistribution returns the ReturnDistribution for the given portfolio side.
func getDistribution(result *comparison.ComparisonResult, side string) *comparison.ReturnDistribution {
	switch side {
	case "A":
		if result.PortfolioA != nil && result.PortfolioA.ReturnDistribution != nil {
			return result.PortfolioA.ReturnDistribution
		}
	case "B":
		if result.PortfolioB != nil && result.PortfolioB.ReturnDistribution != nil {
			return result.PortfolioB.ReturnDistribution
		}
	}
	return nil
}

// mergeHistogramBins merges two sets of histogram buckets into a shared bin list
// (union of labels) with parallel count slices for each portfolio.
func mergeHistogramBins(distA, distB *comparison.ReturnDistribution, extractor func(*comparison.ReturnDistribution) []comparison.ReturnBucket) ([]string, []float64, []float64) {
	bucketsA := extractor(distA)
	bucketsB := extractor(distB)

	// Build label -> count maps.
	mapA := make(map[string]float64)
	for _, b := range bucketsA {
		mapA[b.Label] = float64(b.Count)
	}
	mapB := make(map[string]float64)
	for _, b := range bucketsB {
		mapB[b.Label] = float64(b.Count)
	}

	// Union of labels, sorted.
	labelSet := make(map[string]bool)
	for l := range mapA {
		labelSet[l] = true
	}
	for l := range mapB {
		labelSet[l] = true
	}
	var bins []string
	for l := range labelSet {
		bins = append(bins, l)
	}
	sort.Slice(bins, func(i, j int) bool {
		return extractBinCenter(bins[i]) < extractBinCenter(bins[j])
	})

	countsA := make([]float64, len(bins))
	countsB := make([]float64, len(bins))
	for i, b := range bins {
		countsA[i] = mapA[b]
		countsB[i] = mapB[b]
	}

	return bins, countsA, countsB
}

// extractBinCenter parses the center value from a bin label like "-5% to 0%" for sorting.
func extractBinCenter(label string) float64 {
	// Parse first number from label.
	var result string
	inNum := false
	for _, r := range label {
		if r == '-' || (r >= '0' && r <= '9') {
			result += string(r)
			inNum = true
		} else if inNum {
			break
		}
	}
	if result == "" {
		return 0
	}
	v, err := strconv.ParseFloat(result, 64)
	if err != nil {
		return 0
	}
	return v
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

// driftChartData holds JSON data for a diverging bar chart showing the
// allocation drift (Portfolio A - Portfolio B) for each category.
type driftChartData struct {
	Categories []string  `json:"categories"`
	Values     []float64 `json:"values"` // positive = A > B, negative = B > A (percentage points)
	NameA      string    `json:"name_a"`
	NameB      string    `json:"name_b"`
	Note       string    `json:"note,omitempty"` // informational note (e.g. truncation)
}

// serializeSectorDriftChart produces a diverging bar chart JSON showing the
// sector allocation drift between the two portfolios (A - B in percentage points).
// Categories are sorted by absolute difference descending.
func serializeSectorDriftChart(result *comparison.ComparisonResult) string {
	if result == nil || result.CrossMetrics == nil || result.CrossMetrics.Overlap == nil {
		return "{}"
	}
	overlap := result.CrossMetrics.Overlap
	sectorA := overlap.SectorAllocationA
	sectorB := overlap.SectorAllocationB
	if sectorA == nil || sectorB == nil {
		return "{}"
	}

	drift := computeAllocationDrift(sectorA.Breakdown, sectorB.Breakdown)
	if len(drift.categories) == 0 {
		return "{}"
	}

	data := driftChartData{
		Categories: drift.categories,
		Values:     drift.values,
		NameA:      portfolioName(result.PortfolioA),
		NameB:      portfolioName(result.PortfolioB),
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// serializeCountryDriftChart produces a diverging bar chart JSON showing the
// country allocation drift between the two portfolios (A - B in percentage points).
// Categories are sorted by absolute difference descending, limited to top 15.
const countryDriftLimit = 15

func serializeCountryDriftChart(result *comparison.ComparisonResult) string {
	if result == nil || result.CrossMetrics == nil || result.CrossMetrics.Overlap == nil {
		return "{}"
	}
	overlap := result.CrossMetrics.Overlap
	countryA := overlap.CountryAllocationA
	countryB := overlap.CountryAllocationB
	if countryA == nil || countryB == nil {
		return "{}"
	}

	drift := computeAllocationDrift(countryA.Breakdown, countryB.Breakdown)
	if len(drift.categories) == 0 {
		return "{}"
	}

	var note string
	categories := drift.categories
	values := drift.values
	if len(categories) > countryDriftLimit {
		categories = categories[:countryDriftLimit]
		values = values[:countryDriftLimit]
		note = strconv.Itoa(len(drift.categories)-countryDriftLimit) + " country(s) omitted (below top " + strconv.Itoa(countryDriftLimit) + ")"
	}

	data := driftChartData{
		Categories: categories,
		Values:     values,
		NameA:      portfolioName(result.PortfolioA),
		NameB:      portfolioName(result.PortfolioB),
		Note:       note,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// allocationDriftEntry holds a category and its drift value.
type allocationDriftEntry struct {
	category string
	value    float64 // percentage points (A - B)
}

// allocationDrift holds sorted drift data for chart serialization.
type allocationDrift struct {
	categories []string
	values     []float64
}

// computeAllocationDrift computes the drift (A - B in percentage points) for
// each category present in either breakdown, sorted by absolute difference descending.
// Breakdown values are fractions (0.0-1.0); output is in percentage points.
func computeAllocationDrift(breakdownA, breakdownB map[string]float64) allocationDrift {
	// Union of categories.
	catSet := make(map[string]bool)
	for k := range breakdownA {
		catSet[k] = true
	}
	for k := range breakdownB {
		catSet[k] = true
	}

	entries := make([]allocationDriftEntry, 0, len(catSet))
	for cat := range catSet {
		valA := breakdownA[cat] * 100 // fraction → percentage
		valB := breakdownB[cat] * 100
		diff := valA - valB
		entries = append(entries, allocationDriftEntry{category: cat, value: diff})
	}

	// Sort by absolute difference descending, then alphabetically for ties.
	sort.Slice(entries, func(i, j int) bool {
		absI := entries[i].value
		if absI < 0 {
			absI = -absI
		}
		absJ := entries[j].value
		if absJ < 0 {
			absJ = -absJ
		}
		if absI != absJ {
			return absI > absJ
		}
		return entries[i].category < entries[j].category
	})

	categories := make([]string, len(entries))
	values := make([]float64, len(entries))
	for i, e := range entries {
		categories[i] = e.category
		values[i] = roundTo2(e.value)
	}
	return allocationDrift{categories: categories, values: values}
}

// mergedHoldingsData holds JSON data for the merged holdings table.
type mergedHoldingsData struct {
	Holdings []mergedHoldingRow `json:"holdings"`
	NameA    string             `json:"name_a"`
	NameB    string             `json:"name_b"`
}

type mergedHoldingRow struct {
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name,omitempty"`
	WeightAPct float64 `json:"weight_a_pct"` // percentage
	WeightBPct float64 `json:"weight_b_pct"` // percentage
	OverlapPct float64 `json:"overlap_pct"`  // percentage points
}

// serializeMergedHoldings converts merged holdings to JSON for the template.
func serializeMergedHoldings(result *comparison.ComparisonResult) string {
	if result == nil || result.CrossMetrics == nil || result.CrossMetrics.Overlap == nil {
		return "{}"
	}
	overlap := result.CrossMetrics.Overlap
	if len(overlap.MergedHoldings) == 0 {
		return "{}"
	}

	holdings := make([]mergedHoldingRow, len(overlap.MergedHoldings))
	for i, h := range overlap.MergedHoldings {
		wA, _ := h.WeightA.Float64()
		wB, _ := h.WeightB.Float64()
		holdings[i] = mergedHoldingRow{
			Symbol:     h.Symbol,
			Name:       h.Name,
			WeightAPct: roundTo2(wA * 100),
			WeightBPct: roundTo2(wB * 100),
			OverlapPct: h.OverlapPct,
		}
	}

	data := mergedHoldingsData{
		Holdings: holdings,
		NameA:    portfolioName(result.PortfolioA),
		NameB:    portfolioName(result.PortfolioB),
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// roundTo2 rounds a float64 to 2 decimal places.
// The sign-extraction pattern is needed because Go's int() truncates toward
// zero, so int(-5499.5) == -5499 (not -5500). By extracting the sign first,
// we always round the absolute value, then reapply the sign.
func roundTo2(v float64) float64 {
	sign := 1.0
	if v < 0 {
		v = -v
		sign = -1
	}
	return sign * float64(int(v*100+0.5)) / 100
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

	// Only years where both portfolios have data (intersection).
	yearSet := make(map[int]bool)
	for y := range returnsA {
		if _, ok := returnsB[y]; ok {
			yearSet[y] = true
		}
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
