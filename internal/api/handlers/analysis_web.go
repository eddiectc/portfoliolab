package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/analysis"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// AnalysisWebHandler handles server-rendered analysis pages.
type AnalysisWebHandler struct {
	apiHandler    *AnalysisHandler
	portfolioSvc  *portfolio.Service
	renderer      *web.Renderer
}

// NewAnalysisWebHandler creates a new analysis web handler.
func NewAnalysisWebHandler(apiHandler *AnalysisHandler, portfolioSvc *portfolio.Service, renderer *web.Renderer) *AnalysisWebHandler {
	return &AnalysisWebHandler{
		apiHandler:   apiHandler,
		portfolioSvc: portfolioSvc,
		renderer:     renderer,
	}
}

// RegisterRoutes mounts web analysis routes on the given router.
func (h *AnalysisWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/analysis", h.HandleAnalysis)
}

// HandleAnalysis renders GET /analysis (portfolio analysis page).
func (h *AnalysisWebHandler) HandleAnalysis(w http.ResponseWriter, r *http.Request) {
	filters := parseAnalysisFilters(r.URL.Query())

	// Resolve selected portfolio ID for UI state.
	var selectedPortfolioID string
	if filters.PortfolioID != nil {
		selectedPortfolioID = strconv.FormatInt(*filters.PortfolioID, 10)
	}

	// Resolve selected period for UI state.
	period := filters.Period
	if period == "" {
		period = "1Y"
	}

	result, err := h.apiHandler.computeResult(r.Context(), filters)
	if err != nil {
		data := analysisPageData{
			PageData:            web.PageData{Title: "Portfolio Analysis", Flash: getFlash(w, r)},
			Portfolios:          h.fetchPortfolios(r.Context()),
			SelectedPortfolioID: selectedPortfolioID,
			SelectedPeriod:      period,
			SelectedSection:     filters.Section,
			Error:               "An error occurred while computing analysis data.",
			PeriodURLs:          buildAnalysisPeriodURLs(selectedPortfolioID, period, filters.Section),
			SectionURLs:         buildAnalysisSectionURLs(selectedPortfolioID, period, filters.Section),
		}
		if err := h.renderer.Render(w, "analysis/index", data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Serialize chart data for ECharts.
	correlationData := serializeCorrelationData(result)
	sectorChartData := serializeAllocationChartData(result.SectorAllocation)
	geographicChartData := serializeAllocationChartData(result.GeographicAllocation)

	data := analysisPageData{
		PageData:              web.PageData{Title: "Portfolio Analysis", Flash: getFlash(w, r)},
		Result:                result,
		CorrelationChartData:  correlationData,
		SectorChartData:       sectorChartData,
		GeographicChartData:   geographicChartData,
		Portfolios:            h.fetchPortfolios(r.Context()),
		SelectedPortfolioID:   selectedPortfolioID,
		SelectedPeriod:        period,
		SelectedSection:       filters.Section,
		PeriodURLs:            buildAnalysisPeriodURLs(selectedPortfolioID, period, filters.Section),
		SectionURLs:           buildAnalysisSectionURLs(selectedPortfolioID, period, filters.Section),
	}

	if err := h.renderer.Render(w, "analysis/index", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// analysisPageData is the data struct for the analysis page template.
type analysisPageData struct {
	web.PageData
	Result                *analysis.AnalysisResult
	CorrelationChartData  string // pre-serialized JSON for ECharts heatmap
	SectorChartData       string // pre-serialized JSON for ECharts bar chart
	GeographicChartData   string // pre-serialized JSON for ECharts bar chart
	Portfolios            []portfolio.Portfolio
	SelectedPortfolioID   string
	SelectedPeriod        string
	SelectedSection       string
	Error                 string
	PeriodURLs            map[string]string
	SectionURLs           map[string]string
}

// fetchPortfolios returns all portfolios for the selector dropdown.
func (h *AnalysisWebHandler) fetchPortfolios(ctx context.Context) []portfolio.Portfolio {
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// correlationHeatmapData is the JSON format for the ECharts heatmap.
type correlationHeatmapData struct {
	Symbols []string          `json:"symbols"`
	Matrix  []heatmapCellData `json:"matrix"`
	Period  string            `json:"period"`
}

type heatmapCellData struct {
	SymbolA string   `json:"symbol_a"`
	SymbolB string   `json:"symbol_b"`
	Value   *float64 `json:"value"` // nil = insufficient data, UI renders as "-"
}

// serializeCorrelationData converts the correlation result to JSON for ECharts.
func serializeCorrelationData(result *analysis.AnalysisResult) string {
	if result == nil || result.Correlation == nil || len(result.Correlation.Matrix) == 0 {
		return "{}"
	}
	corr := result.Correlation
	symbols := corr.Symbols
	matrix := corr.Matrix
	n := len(symbols)

	// Build matrix data for ECharts heatmap: [[x, y, value], ...]
	var cells []heatmapCellData
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			cells = append(cells, heatmapCellData{
				SymbolA: symbols[i],
				SymbolB: symbols[j],
				Value:   matrix[i][j],
			})
		}
	}

	data := correlationHeatmapData{
		Symbols: symbols,
		Matrix:  cells,
		Period:  corr.Period,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// allocationBarData is the JSON format for allocation bar charts.
type allocationBarData struct {
	Categories []string `json:"categories"`
	Values     []float64 `json:"values"`
}

// serializeAllocationChartData converts an allocation result to sorted bar chart data.
func serializeAllocationChartData(result *analysis.AllocationResult) string {
	if result == nil || len(result.Breakdown) == 0 {
		return "{}"
	}

	// Sort by weight descending.
	type kv struct {
		Key   string
		Value float64
	}
	var pairs []kv
	for k, v := range result.Breakdown {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[j].Value
	})

	categories := make([]string, len(pairs))
	values := make([]float64, len(pairs))
	for i, p := range pairs {
		categories[i] = p.Key
		values[i] = p.Value
	}

	data := allocationBarData{
		Categories: categories,
		Values:     values,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// buildAnalysisPeriodURLs pre-builds the URL for each period button.
func buildAnalysisPeriodURLs(portfolioID, selectedPeriod, section string) map[string]string {
	urls := make(map[string]string)
	periods := []string{"1Y", "3Y", "5Y", "10Y"}
	for _, p := range periods {
		url := "/analysis"
		hasQuery := false
		if portfolioID != "" {
			url += "?portfolio_id=" + portfolioID
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
		if section != "" {
			if hasQuery {
				url += "&section=" + section
			} else {
				url += "?section=" + section
			}
		}
		urls[p] = url
	}
	_ = selectedPeriod
	return urls
}

// buildAnalysisSectionURLs pre-builds the URL for each section tab.
func buildAnalysisSectionURLs(portfolioID, period, selectedSection string) map[string]string {
	urls := make(map[string]string)
	sections := []struct {
		key string
		val analysis.AnalysisSection
	}{
		{"overlap", analysis.SectionOverlap},
		{"correlation", analysis.SectionCorrelation},
		{"sector", analysis.SectionSectorAllocation},
		{"geographic", analysis.SectionGeographicAllocation},
		{"stress", analysis.SectionStressTest},
		{"factor", analysis.SectionFactorExposure},
	}
	for _, s := range sections {
		url := "/analysis"
		hasQuery := false
		if portfolioID != "" {
			url += "?portfolio_id=" + portfolioID
			hasQuery = true
		}
		if period != "" && period != "1Y" {
			if hasQuery {
				url += "&period=" + period
			} else {
				url += "?period=" + period
				hasQuery = true
			}
		}
		if hasQuery {
			url += "&section=" + string(s.val)
		} else {
			url += "?section=" + string(s.val)
		}
		urls[s.key] = url
	}
	// "all" section (no section filter).
	url := "/analysis"
	if portfolioID != "" {
		url += "?portfolio_id=" + portfolioID
	}
	if period != "" && period != "1Y" {
		url += "&period=" + period
	}
	urls["all"] = url
	_ = selectedSection
	return urls
}
