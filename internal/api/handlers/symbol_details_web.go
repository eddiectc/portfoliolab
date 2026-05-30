package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// displayHolding is a template-friendly holding with pre-formatted percentage.
type displayHolding struct {
	Symbol  string
	Name    string
	Percent string // e.g. "1.40%"
}

// displaySector is a template-friendly sector weighting with pre-formatted percentage.
type displaySector struct {
	Sector  string
	Percent string // e.g. "25.50%"
}

// displayAggregatePositions is a template-friendly aggregate positions with pre-formatted percentages.
type displayAggregatePositions struct {
	Stock       string
	Bond        string
	Cash        string
	Convertible string
	Preferred   string
	Other       string
}

// displayFundProfile is a template-friendly fund profile with pre-formatted values.
type displayFundProfile struct {
	Family         string
	LegalType      string
	NetAssets      string // e.g. "1,234.56B"
	ExpenseRatio   string // e.g. "0.03%"
	Turnover       string // e.g. "35%"
	InceptionDate  string // e.g. "2018-03-15"
	Isin           string // e.g. "LU2951555585"
	ShareClassName string // e.g. "R USD UCITS ETF"
	OngoingCharges string // e.g. "0.75%"
}

// displayGeographicAllocation is a template-friendly geographic allocation with pre-formatted percentage.
type displayGeographicAllocation struct {
	Country string
	Percent string // e.g. "45.20%"
}

// displayTheme is a template-friendly theme breakdown with pre-formatted percentage.
type displayTheme struct {
	Name    string
	Percent string // e.g. "25.50%"
}

// displayRiskMeasures is a template-friendly risk measures with pre-formatted values.
type displayRiskMeasures struct {
	Volatility    string // e.g. "9.16%" or "—"
	SharpeRatio   string // e.g. "2.52" or "—"
	InfoRatio     string // e.g. "0.35" or "—"
	Beta          string // e.g. "0.85" or "—"
	Correlation   string // e.g. "0.92" or "—"
	TrackingError string // e.g. "3.45%" or "—"
}

// displayAssetClassEntry is a template-friendly asset class allocation entry.
type displayAssetClassEntry struct {
	AssetClass string
	Percent    string // e.g. "-5.20%"
}

// displayRegionDerivativeEntry is a template-friendly regional derivative exposure entry.
type displayRegionDerivativeEntry struct {
	Region  string
	Percent string // e.g. "45.20%"
}

// displayCurrencyDerivativeEntry is a template-friendly currency derivative exposure entry.
type displayCurrencyDerivativeEntry struct {
	Currency string
	Percent  string // e.g. "-3.50%"
}

// displayMarketCapBreakdown is a template-friendly market cap breakdown.
type displayMarketCapBreakdown struct {
	Total string // e.g. "450.00B"
	Large string // e.g. "75.00%"
	Mid   string // e.g. "20.00%"
	Small string // e.g. "5.00%"
}

// displayEquityValuation is a template-friendly equity valuation with pre-formatted values.
type displayEquityValuation struct {
	PriceToEarnings          string // e.g. "15.20"
	EstimatedPriceToEarnings string // e.g. "12.50"
	PriceToBook              string // e.g. "2.50"
	PriceToCashflow          string // e.g. "8.30"
	PriceToSales             string // e.g. "3.10"
	DividendYield            string // e.g. "1.50%"
}

// navPriceChartDataPoint is a single data point for the NAV vs Price chart.
type navPriceChartDataPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// symbolDetailsPageData is the data struct for the symbol details template.
type symbolDetailsPageData struct {
	web.PageData
	Symbol            symbolmapping.SymbolMapping
	Details           *symbolDetailsDisplay
	HasDetails        bool
	HasQuote          bool
	QuotePrice        string
	QuoteCurrency     string
	BackHref          string
	BackLabel         string
	Stale             bool
	FetchedText       string
	NavPriceChartData string // pre-serialized JSON for ECharts NAV vs Price chart
	HasChartData      bool
}

// symbolDetailsDisplay is a template-friendly version of symbol.SymbolDetails
// with pre-formatted values.
type symbolDetailsDisplay struct {
	InternalSymbol                string
	ShortName                     string
	LongName                      string
	Exchange                      string
	Currency                      string
	QuoteType                     string
	TopHoldings                   []displayHolding
	SectorWeightings              []displaySector
	AggregatePositions            *displayAggregatePositions
	FundProfile                   *displayFundProfile
	GeographicAllocations         []displayGeographicAllocation
	MarketCapBreakdown            *displayMarketCapBreakdown
	EquityValuation               *displayEquityValuation
	Themes                        []displayTheme
	RiskMeasures                  *displayRiskMeasures
	AssetClassAllocation          []displayAssetClassEntry
	EquityDerivativesByRegion     []displayRegionDerivativeEntry
	CurrencyDerivativesAllocation []displayCurrencyDerivativeEntry
	ExtractorAsOfDate             string // formatted "as of" date; empty when from Yahoo
}

// navHistorySource provides access to cached NAV history and stock price data.
type navHistorySource interface {
	GetNavHistoryBySymbol(ctx context.Context, symbol string) ([]market.HistoricalPrice, error)
	GetHistoricalPricesBySymbol(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
}

// SymbolDetailsWebHandler handles server-rendered symbol details pages.
type SymbolDetailsWebHandler struct {
	symbolMappingSvc *symbolmapping.Service
	detailsSvc       *symbols.Service
	fetcher          market.MarketDataFetcher
	navSource        navHistorySource
	renderer         *web.Renderer
}

// NewSymbolDetailsWebHandler creates a new symbol details web handler.
func NewSymbolDetailsWebHandler(symbolMappingSvc *symbolmapping.Service, detailsSvc *symbols.Service, fetcher market.MarketDataFetcher, navSource navHistorySource, renderer *web.Renderer) *SymbolDetailsWebHandler {
	return &SymbolDetailsWebHandler{
		symbolMappingSvc: symbolMappingSvc,
		detailsSvc:       detailsSvc,
		fetcher:          fetcher,
		navSource:        navSource,
		renderer:         renderer,
	}
}

// RegisterRoutes mounts web symbol details routes on the given router.
func (h *SymbolDetailsWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/symbols/{id}/details", h.HandleDetailsPage)
}

// HandleDetailsPage renders GET /symbols/{id}/details showing cached symbol details + live price.
func (h *SymbolDetailsWebHandler) HandleDetailsPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if h.symbolMappingSvc == nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	sm, err := h.symbolMappingSvc.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := symbolDetailsPageData{
		PageData: web.PageData{
			Title: "Symbol Details — " + sm.InternalSymbol,
			Flash: getFlash(w, r),
		},
		Symbol:    *sm,
		BackHref:  "/symbols",
		BackLabel: "Back to Symbols",
	}

	// Fetch cached symbol details
	if h.detailsSvc != nil {
		details, err := h.detailsSvc.GetByInternalSymbol(r.Context(), sm.InternalSymbol)
		if err == nil && details != nil {
			data.Details = toDisplayDetails(details)
			data.HasDetails = true
			data.Stale = time.Since(details.FetchedAt) > symbols.StaleThreshold
			data.FetchedText = formatFetchedAt(details.FetchedAt)
		}
	}

	// Fetch live quote
	if h.fetcher != nil {
		quote, err := h.fetcher.FetchQuote(r.Context(), sm.MarketDataSymbol)
		if err == nil && quote != nil {
			data.QuotePrice = quote.Price.String()
			data.QuoteCurrency = quote.Currency
			data.HasQuote = true
		}
		// On error, HasQuote stays false — page still renders with cached data
	}

	// Fetch NAV history and stock prices for the chart
	if h.navSource != nil && data.HasDetails {
		ctx := r.Context()
		navPrices, err := h.navSource.GetNavHistoryBySymbol(ctx, sm.InternalSymbol)
		if err == nil && len(navPrices) > 0 {
			// Also fetch stock prices for comparison
			stockPrices, stockErr := h.navSource.GetHistoricalPricesBySymbol(
				ctx, sm.MarketDataSymbol,
				time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Now(),
			)
			if stockErr != nil {
				stockPrices = nil
			}
			chartJSON, jsonErr := serializeNavPriceChartData(navPrices, stockPrices)
			if jsonErr == nil {
				data.NavPriceChartData = chartJSON
				data.HasChartData = true
			}
		}
	}

	if err := h.renderer.Render(w, "symbol_details/view", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// toDisplayDetails converts a SymbolDetails to a template-friendly display struct.
func toDisplayDetails(details *symbol.SymbolDetails) *symbolDetailsDisplay {
	isExtractorData := !details.ExtractorAsOfDate.IsZero()

	dd := &symbolDetailsDisplay{
		InternalSymbol: details.InternalSymbol,
		ShortName:      details.ShortName,
		LongName:       details.LongName,
		Exchange:       details.Exchange,
		Currency:       details.Currency,
		QuoteType:      details.QuoteType,
	}
	if isExtractorData {
		dd.ExtractorAsOfDate = details.ExtractorAsOfDate.Format("2006-01-02")
	}

	// Holdings — always include all; template shows top 10 with expand link
	for _, h := range details.TopHoldings {
		dd.TopHoldings = append(dd.TopHoldings, displayHolding{
			Symbol:  h.Symbol,
			Name:    h.Name,
			Percent: fmt.Sprintf("%.2f%%", h.Percent),
		})
	}

	// Sector weightings (sorted desc)
	sorted := make([]symbol.SectorWeighting, len(details.SectorWeightings))
	copy(sorted, details.SectorWeightings)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Percent > sorted[j].Percent
	})
	for _, s := range sorted {
		dd.SectorWeightings = append(dd.SectorWeightings, displaySector{
			Sector:  s.Sector,
			Percent: fmt.Sprintf("%.2f%%", s.Percent),
		})
	}

	// Aggregate positions
	if details.AggregatePositions != nil {
		dd.AggregatePositions = &displayAggregatePositions{
			Stock:       fmt.Sprintf("%.2f%%", details.AggregatePositions.Stock*100),
			Bond:        fmt.Sprintf("%.2f%%", details.AggregatePositions.Bond*100),
			Cash:        fmt.Sprintf("%.2f%%", details.AggregatePositions.Cash*100),
			Convertible: fmt.Sprintf("%.2f%%", details.AggregatePositions.Convertible*100),
			Preferred:   fmt.Sprintf("%.2f%%", details.AggregatePositions.Preferred*100),
			Other:       fmt.Sprintf("%.2f%%", details.AggregatePositions.Other*100),
		}
	}

	// Fund profile
	if details.FundProfile != nil {
		displayProfile := &displayFundProfile{
			Family:         details.FundProfile.Family,
			LegalType:      details.FundProfile.LegalType,
			NetAssets:      formatLargeNumber(details.FundProfile.TotalNetAssets),
			ExpenseRatio:   fmt.Sprintf("%.2f%%", details.FundProfile.AnnualExpenseRatio*100),
			Turnover:       fmt.Sprintf("%.0f%%", details.FundProfile.AnnualHoldingsTurnover*100),
			Isin:           details.FundProfile.Isin,
			ShareClassName: details.FundProfile.ShareClassName,
		}
		if !details.FundProfile.InceptionDate.IsZero() {
			displayProfile.InceptionDate = details.FundProfile.InceptionDate.Format("2006-01-02")
		}
		if details.FundProfile.OngoingCharges > 0 {
			displayProfile.OngoingCharges = fmt.Sprintf("%.2f%%", details.FundProfile.OngoingCharges)
		}
		dd.FundProfile = displayProfile
	}

	// Geographic allocations (sorted by percent descending)
	if len(details.GeographicAllocations) > 0 {
		sorted := make([]symbol.GeographicAllocation, len(details.GeographicAllocations))
		copy(sorted, details.GeographicAllocations)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Percent > sorted[j].Percent
		})
		for _, g := range sorted {
			dd.GeographicAllocations = append(dd.GeographicAllocations, displayGeographicAllocation{
				Country: g.Country,
				Percent: fmt.Sprintf("%.2f%%", g.Percent),
			})
		}
	}

	// Market cap breakdown
	if details.MarketCapBreakdown != nil {
		dd.MarketCapBreakdown = &displayMarketCapBreakdown{
			Total: formatLargeNumber(details.MarketCapBreakdown.Total),
			Large: fmt.Sprintf("%.2f%%", details.MarketCapBreakdown.Large),
			Mid:   fmt.Sprintf("%.2f%%", details.MarketCapBreakdown.Mid),
			Small: fmt.Sprintf("%.2f%%", details.MarketCapBreakdown.Small),
		}
	}

	// Equity valuation
	if details.EquityValuation != nil {
		ev := details.EquityValuation
		dd.EquityValuation = &displayEquityValuation{
			PriceToEarnings:          formatFloat(ev.PriceToEarnings),
			EstimatedPriceToEarnings: formatFloat(ev.EstimatedPriceToEarnings),
			PriceToBook:              formatFloat(ev.PriceToBook),
			PriceToCashflow:          formatFloat(ev.PriceToCashflow),
			PriceToSales:             formatFloat(ev.PriceToSales),
			DividendYield:            fmt.Sprintf("%.2f%%", ev.DividendYield),
		}
	}

	// Themes (sorted by percent desc)
	if len(details.Themes) > 0 {
		sorted := make([]symbol.ThemeBreakdown, len(details.Themes))
		copy(sorted, details.Themes)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Percent > sorted[j].Percent
		})
		for _, th := range sorted {
			dd.Themes = append(dd.Themes, displayTheme{
				Name:    th.Name,
				Percent: fmt.Sprintf("%.2f%%", th.Percent),
			})
		}
	}

	// Risk measures
	if details.RiskMeasures != nil && details.RiskMeasures.FieldsPresent != 0 {
		rm := details.RiskMeasures
		displayRM := &displayRiskMeasures{}
		if rm.HasField(symbol.SymbolRiskFieldVolatility) {
			displayRM.Volatility = fmt.Sprintf("%.2f%%", rm.Volatility)
		} else {
			displayRM.Volatility = "—"
		}
		if rm.HasField(symbol.SymbolRiskFieldSharpeRatio) {
			displayRM.SharpeRatio = fmt.Sprintf("%.2f", rm.SharpeRatio)
		} else {
			displayRM.SharpeRatio = "—"
		}
		if rm.HasField(symbol.SymbolRiskFieldInfoRatio) {
			displayRM.InfoRatio = fmt.Sprintf("%.2f", rm.InfoRatio)
		} else {
			displayRM.InfoRatio = "—"
		}
		if rm.HasField(symbol.SymbolRiskFieldBeta) {
			displayRM.Beta = fmt.Sprintf("%.2f", rm.Beta)
		} else {
			displayRM.Beta = "—"
		}
		if rm.HasField(symbol.SymbolRiskFieldCorrelation) {
			displayRM.Correlation = fmt.Sprintf("%.2f", rm.Correlation)
		} else {
			displayRM.Correlation = "—"
		}
		if rm.HasField(symbol.SymbolRiskFieldTrackingError) {
			displayRM.TrackingError = fmt.Sprintf("%.2f%%", rm.TrackingError)
		} else {
			displayRM.TrackingError = "—"
		}
		dd.RiskMeasures = displayRM
	}

	// Asset class allocation
	if len(details.AssetClassAllocation) > 0 {
		for _, ac := range details.AssetClassAllocation {
			dd.AssetClassAllocation = append(dd.AssetClassAllocation, displayAssetClassEntry{
				AssetClass: ac.AssetClass,
				Percent:    fmt.Sprintf("%.2f%%", ac.Percent),
			})
		}
	}

	// Equity derivatives by region
	if len(details.EquityDerivativesByRegion) > 0 {
		for _, rd := range details.EquityDerivativesByRegion {
			dd.EquityDerivativesByRegion = append(dd.EquityDerivativesByRegion, displayRegionDerivativeEntry{
				Region:  rd.Region,
				Percent: fmt.Sprintf("%.2f%%", rd.Percent),
			})
		}
	}

	// Currency derivatives allocation
	if len(details.CurrencyDerivativesAllocation) > 0 {
		for _, cd := range details.CurrencyDerivativesAllocation {
			dd.CurrencyDerivativesAllocation = append(dd.CurrencyDerivativesAllocation, displayCurrencyDerivativeEntry{
				Currency: cd.Currency,
				Percent:  fmt.Sprintf("%.2f%%", cd.Percent),
			})
		}
	}

	return dd
}

// formatFetchedAt returns a human-readable "Last updated" string.
func formatFetchedAt(t time.Time) string {
	diff := time.Since(t)
	if diff < time.Minute {
		return "Updated just now"
	}
	if diff < time.Hour {
		mins := int(diff.Minutes())
		return "Updated " + strconv.Itoa(mins) + "m ago"
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return "Updated " + strconv.Itoa(hours) + "h ago"
	}
	days := int(diff.Hours() / 24)
	return "Updated " + strconv.Itoa(days) + "d ago"
}

// formatLargeNumber formats a large float64 value with suffix (e.g. "1,234.56B").
func formatLargeNumber(val float64) string {
	if val >= 1e12 {
		return fmt.Sprintf("%.2fT", val/1e12)
	}
	if val >= 1e9 {
		return fmt.Sprintf("%.2fB", val/1e9)
	}
	if val >= 1e6 {
		return fmt.Sprintf("%.2fM", val/1e6)
	}
	return fmt.Sprintf("%.2f", val)
}

// formatFloat formats a float64 with 2 decimal places, showing "—" for zero.
func formatFloat(val float64) string {
	if val == 0 {
		return "—"
	}
	return fmt.Sprintf("%.2f", val)
}

// navPriceChartData is the JSON structure for the ECharts NAV vs Price chart.
type navPriceChartData struct {
	NavDates    []string  `json:"navDates"`
	NavValues   []float64 `json:"navValues"`
	PriceDates  []string  `json:"priceDates"`
	PriceValues []float64 `json:"priceValues"`
}

// serializeNavPriceChartData converts NAV and stock price history to JSON for
// ECharts consumption. Both series are displayed as-is without interpolation.
func serializeNavPriceChartData(navPrices, stockPrices []market.HistoricalPrice) (string, error) {
	data := &navPriceChartData{}

	for _, p := range navPrices {
		data.NavDates = append(data.NavDates, p.Date.Format("2006-01-02"))
		val, ok := p.Close.Float64()
		if !ok {
			return "", fmt.Errorf("convert NAV price for %s", p.Date.Format("2006-01-02"))
		}
		data.NavValues = append(data.NavValues, val)
	}

	for _, p := range stockPrices {
		data.PriceDates = append(data.PriceDates, p.Date.Format("2006-01-02"))
		val, ok := p.Close.Float64()
		if !ok {
			return "", fmt.Errorf("convert stock price for %s", p.Date.Format("2006-01-02"))
		}
		data.PriceValues = append(data.PriceValues, val)
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("serialize chart data: %w", err)
	}
	return string(bytes), nil
}
