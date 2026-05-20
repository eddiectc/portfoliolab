package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/allocation"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// AllocationFilter holds parsed filter parameters for the allocation page.
type AllocationFilter struct {
	PortfolioIDs []int64 // comma-separated from query; empty = all
}

// QueryParams serializes non-empty filter fields into a URL query fragment
// like "&portfolio_ids=1,2". Returns "" if all fields are empty.
func (f AllocationFilter) QueryParams() string {
	if len(f.PortfolioIDs) == 0 {
		return ""
	}
	parts := make([]string, len(f.PortfolioIDs))
	for i, id := range f.PortfolioIDs {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return "&portfolio_ids=" + strings.Join(parts, ",")
}

// allocationPageData is the data struct for the allocation page template.
type allocationPageData struct {
	web.PageData
	Allocation        *allocation.AllocationResult
	Drift             *allocation.DriftResult
	Rebalance         *allocation.RebalanceResult
	Targets           []allocation.TargetAllocation
	SaveError         string
	Portfolios        []portfolio.Portfolio
	Symbols           []symbolmapping.SymbolMapping
	ModelPortfolios   []modelportfolio.ModelPortfolioSummary // for the "load model" dropdown
	SelectedPortfolio string                                 // single portfolio ID for drift/rebalance
	Filter            AllocationFilter
	BaseCurrency      string
	LastUpdatedText   string
	DriftWarning      string // shown when drift computation fails
	RebalanceWarning  string // shown when rebalance computation fails
	TargetWarning     string // shown when target fetch fails
}

// modelPortfolioSelector defines the methods needed to fetch model portfolios for the dropdown.
type modelPortfolioSelector interface {
	GetAllForSelector(ctx context.Context) ([]modelportfolio.ModelPortfolioSummary, error)
}

// AllocationWebHandler handles server-rendered allocation pages.
type AllocationWebHandler struct {
	apiHandler        *AllocationHandler
	portfolioSvc      *portfolio.Service
	symbolSvc         *symbolmapping.Service
	allocSvc          allocationService
	modelPortfolioSvc modelPortfolioSelector
	renderer          *web.Renderer
}

// NewAllocationWebHandler creates a new allocation web handler.
func NewAllocationWebHandler(apiHandler *AllocationHandler, portfolioSvc *portfolio.Service, symbolSvc *symbolmapping.Service, allocSvc allocationService, modelPortfolioSvc modelPortfolioSelector, renderer *web.Renderer) *AllocationWebHandler {
	return &AllocationWebHandler{
		apiHandler:        apiHandler,
		portfolioSvc:      portfolioSvc,
		symbolSvc:         symbolSvc,
		allocSvc:          allocSvc,
		modelPortfolioSvc: modelPortfolioSvc,
		renderer:          renderer,
	}
}

// RegisterRoutes mounts web allocation routes on the given router.
func (h *AllocationWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/allocation/target/delete", h.HandleDeleteTarget)
	r.Post("/allocation/target", h.HandleSaveTarget)
	r.Get("/allocation", h.HandleAllocation)
}

// HandleAllocation renders GET /allocation (allocation page).
// Optional query param: portfolio_ids (comma-separated, empty = all portfolios).
// Single portfolio: shows allocation + drift + rebalance + target editing.
// Multiple portfolios: shows allocation only.
func (h *AllocationWebHandler) HandleAllocation(w http.ResponseWriter, r *http.Request) {
	filter := parseWebAllocationFilter(r.URL.Query())

	// Fetch portfolios for dropdown.
	portfolios := h.fetchPortfolios(r.Context())

	// Fetch symbols for autocomplete.
	symbols := h.fetchSymbols(r.Context())

	// Fetch model portfolios for dropdown.
	modelPortfolios := h.fetchModelPortfolios(r.Context())

	// Compute allocation.
	allocFilter := toDomainFilter(filter)
	result, err := h.allocSvc.ComputeAllocation(r.Context(), allocFilter)
	if err != nil {
		data := h.buildPageData(w, r, filter, portfolios, symbols, modelPortfolios, nil, nil, nil, nil, "An error occurred while computing allocation data.", "", "", "", "")
		if err := h.renderer.Render(w, "allocation/list", data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Determine if single portfolio is selected (for drift/rebalance/target).
	selectedPortfolioID := h.selectedSinglePortfolio(filter)

	var (
		drift            *allocation.DriftResult
		rebalance        *allocation.RebalanceResult
		targets          []allocation.TargetAllocation
		driftWarning     string
		rebalanceWarning string
		targetWarning    string
	)

	if selectedPortfolioID != "" {
		pid, _ := strconv.ParseInt(selectedPortfolioID, 10, 64)

		// Compute drift.
		if dr, drErr := h.allocSvc.ComputeDrift(r.Context(), allocFilter, pid); drErr == nil {
			drift = dr
		} else {
			driftWarning = "Could not compute drift: " + drErr.Error()
		}

		// Compute rebalancing suggestions.
		if rb, rbErr := h.allocSvc.ComputeRebalancingSuggestions(r.Context(), allocFilter, pid); rbErr == nil {
			rebalance = rb
		} else {
			rebalanceWarning = "Could not compute rebalancing suggestions: " + rbErr.Error()
		}

		// Fetch targets.
		if tgts, tgErr := h.allocSvc.GetTargetAllocation(r.Context(), pid); tgErr == nil {
			targets = tgts
		} else {
			targetWarning = "Could not load target allocation: " + tgErr.Error()
		}
	}

	data := h.buildPageData(w, r, filter, portfolios, symbols, modelPortfolios, result, drift, rebalance, targets, "", selectedPortfolioID, driftWarning, rebalanceWarning, targetWarning)

	if err := h.renderer.Render(w, "allocation/list", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// HandleSaveTarget handles POST /allocation/target (save target allocation, redirect with flash).
func (h *AllocationWebHandler) HandleSaveTarget(w http.ResponseWriter, r *http.Request) {
	portfolioIDStr := r.FormValue("portfolio_id")
	if portfolioIDStr == "" {
		setFlash(w, "portfolio_id is required")
		http.Redirect(w, r, "/allocation", http.StatusSeeOther)
		return
	}
	portfolioID, err := strconv.ParseInt(portfolioIDStr, 10, 64)
	if err != nil {
		setFlash(w, "invalid portfolio_id")
		http.Redirect(w, r, "/allocation", http.StatusSeeOther)
		return
	}

	// Parse target entries from form: symbol_N, target_pct_N
	var entries []allocation.TargetEntry
	i := 0
	for {
		symbol := r.FormValue("symbol_" + strconv.Itoa(i))
		if symbol == "" {
			break
		}
		pctStr := r.FormValue("target_pct_" + strconv.Itoa(i))
		pct, err := decimal.Parse(pctStr)
		if err != nil {
			setFlash(w, "invalid percentage for "+symbol+": "+err.Error())
			http.Redirect(w, r, "/allocation?portfolio_ids="+portfolioIDStr, http.StatusSeeOther)
			return
		}
		entries = append(entries, allocation.TargetEntry{
			Symbol:    symbol,
			TargetPct: pct,
		})
		i++
	}

	if err := h.allocSvc.SaveTargetAllocation(r.Context(), portfolioID, entries); err != nil {
		setFlash(w, "Failed to save targets: "+err.Error())
		http.Redirect(w, r, "/allocation?portfolio_ids="+portfolioIDStr, http.StatusSeeOther)
		return
	}

	setFlash(w, "Target allocations saved")
	http.Redirect(w, r, "/allocation?portfolio_ids="+portfolioIDStr, http.StatusSeeOther)
}

// HandleDeleteTarget handles POST /allocation/target/delete (delete all targets, redirect with flash).
func (h *AllocationWebHandler) HandleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	portfolioIDStr := r.FormValue("portfolio_id")
	if portfolioIDStr == "" {
		setFlash(w, "portfolio_id is required")
		http.Redirect(w, r, "/allocation", http.StatusSeeOther)
		return
	}
	portfolioID, err := strconv.ParseInt(portfolioIDStr, 10, 64)
	if err != nil {
		setFlash(w, "invalid portfolio_id")
		http.Redirect(w, r, "/allocation", http.StatusSeeOther)
		return
	}

	if err := h.allocSvc.DeleteAllTargetAllocations(r.Context(), portfolioID); err != nil {
		setFlash(w, "Failed to delete targets: "+err.Error())
		http.Redirect(w, r, "/allocation?portfolio_ids="+portfolioIDStr, http.StatusSeeOther)
		return
	}

	setFlash(w, "Target allocations deleted")
	http.Redirect(w, r, "/allocation?portfolio_ids="+portfolioIDStr, http.StatusSeeOther)
}

// --- Helpers ---

// parseWebAllocationFilter extracts allocation filter from query params.
func parseWebAllocationFilter(query map[string][]string) AllocationFilter {
	var filter AllocationFilter
	if vals, ok := query["portfolio_ids"]; ok && len(vals) > 0 {
		var ids []int64
		for _, part := range strings.Split(vals[0], ",") {
			part = strings.TrimSpace(part)
			if n, err := strconv.ParseInt(part, 10, 64); err == nil {
				ids = append(ids, n)
			}
		}
		if len(ids) > 0 {
			filter.PortfolioIDs = ids
		}
	}
	return filter
}

// toDomainFilter converts a web AllocationFilter to a domain AllocationFilter.
func toDomainFilter(f AllocationFilter) allocation.AllocationFilter {
	return allocation.AllocationFilter{PortfolioIDs: f.PortfolioIDs}
}

// selectedSinglePortfolio returns the single portfolio ID string if exactly one
// portfolio is selected, or "" if none or multiple are selected.
func (h *AllocationWebHandler) selectedSinglePortfolio(filter AllocationFilter) string {
	if len(filter.PortfolioIDs) == 1 {
		return strconv.FormatInt(filter.PortfolioIDs[0], 10)
	}
	return ""
}

// fetchPortfolios returns all portfolios for the selector dropdown.
func (h *AllocationWebHandler) fetchPortfolios(ctx context.Context) []portfolio.Portfolio {
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		slog.Warn("failed to fetch portfolios for allocation dropdown", "error", err)
		return []portfolio.Portfolio{}
	}
	if portfolios == nil {
		return []portfolio.Portfolio{}
	}
	return portfolios
}

// fetchSymbols returns all internal symbols for the autocomplete datalist.
func (h *AllocationWebHandler) fetchSymbols(ctx context.Context) []symbolmapping.SymbolMapping {
	symbols, err := h.symbolSvc.List(ctx, 0, 0)
	if err != nil {
		slog.Warn("failed to fetch symbols for allocation autocomplete", "error", err)
		return []symbolmapping.SymbolMapping{}
	}
	if symbols == nil {
		return []symbolmapping.SymbolMapping{}
	}
	return symbols
}

// fetchModelPortfolios returns model portfolio summaries for the dropdown selector.
func (h *AllocationWebHandler) fetchModelPortfolios(ctx context.Context) []modelportfolio.ModelPortfolioSummary {
	if h.modelPortfolioSvc == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	summaries, err := h.modelPortfolioSvc.GetAllForSelector(ctx)
	if err != nil {
		slog.Warn("failed to fetch model portfolios for allocation dropdown", "error", err)
		return []modelportfolio.ModelPortfolioSummary{}
	}
	if summaries == nil {
		return []modelportfolio.ModelPortfolioSummary{}
	}
	return summaries
}

// buildPageData assembles the allocation page data struct.
func (h *AllocationWebHandler) buildPageData(
	w http.ResponseWriter, r *http.Request,
	filter AllocationFilter,
	portfolios []portfolio.Portfolio,
	symbols []symbolmapping.SymbolMapping,
	modelPortfolios []modelportfolio.ModelPortfolioSummary,
	alloc *allocation.AllocationResult,
	drift *allocation.DriftResult,
	rebalance *allocation.RebalanceResult,
	targets []allocation.TargetAllocation,
	errorMsg, selectedPortfolio, driftWarning, rebalanceWarning, targetWarning string,
) allocationPageData {
	// Format last updated time.
	var lastUpdatedText string
	var baseCurrency string
	if alloc != nil {
		if !alloc.LastUpdated.IsZero() {
			lastUpdatedText = alloc.LastUpdated.Format("2006-01-02 15:04:05 MST")
		}
		baseCurrency = alloc.BaseCurrency
	}

	return allocationPageData{
		PageData:          web.PageData{Title: "Allocation", Flash: getFlash(w, r)},
		Allocation:        alloc,
		Drift:             drift,
		Rebalance:         rebalance,
		Targets:           targets,
		Portfolios:        portfolios,
		Symbols:           symbols,
		ModelPortfolios:   modelPortfolios,
		SelectedPortfolio: selectedPortfolio,
		Filter:            filter,
		BaseCurrency:      baseCurrency,
		LastUpdatedText:   lastUpdatedText,
		SaveError:         errorMsg,
		DriftWarning:      driftWarning,
		RebalanceWarning:  rebalanceWarning,
		TargetWarning:     targetWarning,
	}
}

// serializeDriftData converts drift rows to JSON for client-side use.
func serializeDriftData(drift *allocation.DriftResult) string {
	if drift == nil || len(drift.Rows) == 0 {
		return "{}"
	}
	b, err := json.Marshal(drift)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// serializeRebalanceData converts rebalance result to JSON for client-side use.
func serializeRebalanceData(rebalance *allocation.RebalanceResult) string {
	if rebalance == nil || len(rebalance.Suggestions) == 0 {
		return "{}"
	}
	b, err := json.Marshal(rebalance)
	if err != nil {
		return "{}"
	}
	return string(b)
}
