package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
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
	Family       string
	LegalType    string
	NetAssets    string // e.g. "1,234.56B"
	ExpenseRatio string // e.g. "0.03%"
	Turnover     string // e.g. "35%"
}

// displayGeographicAllocation is a template-friendly geographic allocation with pre-formatted percentage.
type displayGeographicAllocation struct {
	Country string
	Percent string // e.g. "45.20%"
}

// symbolDetailsPageData is the data struct for the symbol details template.
type symbolDetailsPageData struct {
	web.PageData
	Symbol        symbolmapping.SymbolMapping
	Details       *symbolDetailsDisplay
	HasDetails    bool
	HasQuote      bool
	QuotePrice    string
	QuoteCurrency string
	BackHref      string
	BackLabel     string
	Stale         bool
	FetchedText   string
}

// symbolDetailsDisplay is a template-friendly version of symbol.SymbolDetails
// with pre-formatted values.
type symbolDetailsDisplay struct {
	InternalSymbol          string
	ShortName               string
	LongName                string
	Exchange                string
	Currency                string
	QuoteType               string
	TopHoldings             []displayHolding
	SectorWeightings        []displaySector
	AggregatePositions      *displayAggregatePositions
	FundProfile             *displayFundProfile
	GeographicAllocations   []displayGeographicAllocation
}

// SymbolDetailsWebHandler handles server-rendered symbol details pages.
type SymbolDetailsWebHandler struct {
	symbolMappingSvc *symbolmapping.Service
	detailsSvc       *symbols.Service
	fetcher          market.MarketDataFetcher
	renderer         *web.Renderer
}

// NewSymbolDetailsWebHandler creates a new symbol details web handler.
func NewSymbolDetailsWebHandler(symbolMappingSvc *symbolmapping.Service, detailsSvc *symbols.Service, fetcher market.MarketDataFetcher, renderer *web.Renderer) *SymbolDetailsWebHandler {
	return &SymbolDetailsWebHandler{
		symbolMappingSvc: symbolMappingSvc,
		detailsSvc:       detailsSvc,
		fetcher:          fetcher,
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

	if err := h.renderer.Render(w, "symbol_details/view", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// toDisplayDetails converts a SymbolDetails to a template-friendly display struct.
func toDisplayDetails(details *symbol.SymbolDetails) *symbolDetailsDisplay {
	dd := &symbolDetailsDisplay{
		InternalSymbol: details.InternalSymbol,
		ShortName:      details.ShortName,
		LongName:       details.LongName,
		Exchange:       details.Exchange,
		Currency:       details.Currency,
		QuoteType:      details.QuoteType,
	}

	// Top holdings (limit to 10)
	holdings := details.TopHoldings
	if len(holdings) > 10 {
		holdings = holdings[:10]
	}
	for _, h := range holdings {
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
		dd.FundProfile = &displayFundProfile{
			Family:       details.FundProfile.Family,
			LegalType:    details.FundProfile.LegalType,
			NetAssets:    formatLargeNumber(details.FundProfile.TotalNetAssets),
			ExpenseRatio: fmt.Sprintf("%.2f%%", details.FundProfile.AnnualExpenseRatio*100),
			Turnover:     fmt.Sprintf("%.0f%%", details.FundProfile.AnnualHoldingsTurnover*100),
		}
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
