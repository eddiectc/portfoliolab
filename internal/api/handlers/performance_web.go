package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// performancePageData is the data struct for the performance page template.
type performancePageData struct {
	web.PageData
	Result              *position.PerformanceResult
	ChartData           string // pre-serialized JSON for ECharts
	CurrentValue        string // last equity curve point's portfolio value
	CurrentNetDeposit   string // last equity curve point's net deposit
	Portfolios          []portfolio.Portfolio
	SelectedPeriod      string
	SelectedPortfolioID string
	Error               string
	// Pre-built URLs for template safety (Go html/template is strict about expressions in URLs).
	RefreshURL  string
	PeriodURLs  map[string]string // period label -> full URL
}

// PerformanceWebHandler handles server-rendered performance pages.
type PerformanceWebHandler struct {
	positionSvc  *position.Service
	portfolioSvc *portfolio.Service
	renderer     *web.Renderer
}

// NewPerformanceWebHandler creates a new performance web handler.
func NewPerformanceWebHandler(positionSvc *position.Service, portfolioSvc *portfolio.Service, renderer *web.Renderer) *PerformanceWebHandler {
	return &PerformanceWebHandler{
		positionSvc:  positionSvc,
		portfolioSvc: portfolioSvc,
		renderer:     renderer,
	}
}

// RegisterRoutes mounts web performance routes on the given router.
func (h *PerformanceWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/performance/refresh", h.HandleRefresh)
	r.Get("/performance", h.HandlePerformance)
}

// HandlePerformance renders GET /performance (equity curve chart + return metrics).
func (h *PerformanceWebHandler) HandlePerformance(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())

	// Resolve selected portfolio ID for UI state.
	var selectedPortfolioID string
	if filters.PortfolioID != nil {
		selectedPortfolioID = strconv.FormatInt(*filters.PortfolioID, 10)
	}

	result, err := h.positionSvc.ComputeEquityCurve(r.Context(), filters)
	if err != nil {
		data := performancePageData{
			PageData:            web.PageData{Title: "Performance", Flash: getFlash(w, r)},
			Portfolios:          h.fetchPortfolios(r.Context()),
			SelectedPeriod:      filters.Period,
			SelectedPortfolioID: selectedPortfolioID,
			Error:               userFriendlyPerformanceError(err),
			RefreshURL:          buildRefreshURL(selectedPortfolioID, filters.Period),
			PeriodURLs:          buildPeriodURLs(selectedPortfolioID, filters.Period),
		}
		if err := h.renderer.Render(w, "performance/index", data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Extract last point for summary display.
	var currentValue, currentNetDeposit string
	if len(result.EquityCurve) > 0 {
		last := result.EquityCurve[len(result.EquityCurve)-1]
		currentValue = last.PortfolioValue.String()
		currentNetDeposit = last.NetDeposit.String()
	}

	data := performancePageData{
		PageData:            web.PageData{Title: "Performance", Flash: getFlash(w, r)},
		Result:              result,
		ChartData:           serializeChartData(result.EquityCurve),
		CurrentValue:        currentValue,
		CurrentNetDeposit:   currentNetDeposit,
		Portfolios:          h.fetchPortfolios(r.Context()),
		SelectedPeriod:      filters.Period,
		SelectedPortfolioID: selectedPortfolioID,
		RefreshURL:          buildRefreshURL(selectedPortfolioID, filters.Period),
		PeriodURLs:          buildPeriodURLs(selectedPortfolioID, filters.Period),
	}

	if err := h.renderer.Render(w, "performance/index", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// HandleRefresh handles POST /performance/refresh (manual market data refresh).
func (h *PerformanceWebHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	filters := parsePerformanceFilters(r.URL.Query())

	result, err := h.positionSvc.RefreshMarketData(r.Context(), filters)
	if err != nil {
		setFlash(w, "Refresh failed: "+err.Error())
		http.Redirect(w, r, "/performance"+r.URL.RawQuery, http.StatusSeeOther)
		return
	}

	// Build a summary message.
	var message string
	if len(result.SymbolsRefreshed) > 0 {
		message = "Refreshed " + strconv.Itoa(len(result.SymbolsRefreshed)) + " symbol(s)"
	} else {
		message = "No symbols to refresh"
	}
	if len(result.FxPairsRefreshed) > 0 {
		message += ", " + strconv.Itoa(len(result.FxPairsRefreshed)) + " FX pair(s)"
	}
	if len(result.FailedSymbols) > 0 {
		message += " (" + strconv.Itoa(len(result.FailedSymbols)) + " failed)"
	}

	setFlash(w, message)
	http.Redirect(w, r, "/performance"+r.URL.RawQuery, http.StatusSeeOther)
}

// fetchPortfolios returns all portfolios for the selector dropdown.
func (h *PerformanceWebHandler) fetchPortfolios(ctx context.Context) []portfolio.Portfolio {
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// userFriendlyPerformanceError returns a user-friendly message from a performance error.
func userFriendlyPerformanceError(err error) string {
	var posErr *position.PositionError
	if errors.As(err, &posErr) {
		switch posErr.Code {
		case "mismatched_currencies":
			return "Cannot combine portfolios with different base currencies. Select a single portfolio to view its performance."
		default:
			return posErr.Message
		}
	}
	return "An error occurred while computing performance data."
}

// chartDataPoint is the JSON-serializable format for ECharts.
type chartDataPoint struct {
	Date           string `json:"date"`
	PortfolioValue string `json:"portfolio_value"`
	NetDeposit     string `json:"net_deposit"`
}

// serializeChartData converts equity curve points to JSON for ECharts consumption.
func serializeChartData(points []position.EquityCurvePoint) string {
	if len(points) == 0 {
		return "[]"
	}
	data := make([]chartDataPoint, len(points))
	for i, p := range points {
		data[i] = chartDataPoint{
			Date:           p.Date.Format("2006-01-02"),
			PortfolioValue: p.PortfolioValue.String(),
			NetDeposit:     p.NetDeposit.String(),
		}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// buildRefreshURL constructs the POST target for the refresh button.
func buildRefreshURL(portfolioID, period string) string {
	url := "/performance/refresh"
	if portfolioID != "" {
		url += "?portfolio_id=" + portfolioID
		if period != "" && period != "All" {
			url += "&period=" + period
		}
	} else if period != "" && period != "All" {
		url += "?period=" + period
	}
	return url
}

// buildPeriodURLs pre-builds the URL for each period button.
func buildPeriodURLs(portfolioID, selectedPeriod string) map[string]string {
	urls := make(map[string]string)
	for _, p := range []string{"1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"} {
		url := "/performance"
		if portfolioID != "" {
			url += "?portfolio_id=" + portfolioID
			if p != "All" {
				url += "&period=" + p
			}
		} else if p != "All" {
			url += "?period=" + p
		}
		urls[p] = url
	}
	_ = selectedPeriod // used by template for active state
	return urls
}
