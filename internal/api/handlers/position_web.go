package handlers

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// PositionFilter holds parsed filter parameters from query string for position pages.
type PositionFilter struct {
	AccountID   string
	PortfolioID string
}

// QueryParams serializes non-empty filter fields into a URL query fragment
// like "&account_id=1&portfolio_id=2". Returns "" if all fields are empty.
// Implements web.FilterEncoder for type-safe query preservation in templates.
func (f PositionFilter) QueryParams() string {
	var parts []string
	if f.AccountID != "" {
		parts = append(parts, "account_id="+f.AccountID)
	}
	if f.PortfolioID != "" {
		parts = append(parts, "portfolio_id="+f.PortfolioID)
	}
	if len(parts) == 0 {
		return ""
	}
	return "&" + strings.Join(parts, "&")
}

// PaginationQuery returns a complete query string with the given page number
// and all non-empty filter fields, properly URL-encoded for use in href attributes.
// e.g., "?page=2&account_id=1"
func (f PositionFilter) PaginationQuery(page int) string {
	values := url.Values{}
	values.Set("page", strconv.Itoa(page))
	if f.AccountID != "" {
		values.Set("account_id", f.AccountID)
	}
	if f.PortfolioID != "" {
		values.Set("portfolio_id", f.PortfolioID)
	}
	return "?" + values.Encode()
}

// positionListPageData is the data struct for the closed positions template.
type positionListPageData struct {
	web.PageData
	Positions    []position.Position
	Accounts     []account.Account
	Filter       PositionFilter
	BaseCurrency string
	Summary      closedPositionSummary
	Page         int
	HasPrev      bool
	HasNext      bool
}

// closedPositionSummary holds aggregated totals for the closed positions summary panel.
type closedPositionSummary struct {
	TotalRealizedPnLB string // total realized P&L in base currency
	HasFxErrors       bool   // true if any position missing FX rate
}

// positionSummary holds aggregated totals for the open positions summary panel.
type positionSummary struct {
	TotalCostBasisBase  string
	TotalMktValueBase   string
	TotalUnrealizedPnLB string
	TotalUnrealizedPnLP string // total unrealized P&L %
	HasFxErrors         bool   // true if any position missing FX rate
}

// openPositionListPageData is the data struct for the open positions template,
// with positions enriched with market data.
type openPositionListPageData struct {
	web.PageData
	Positions       []position.PositionWithMarket
	Accounts        []account.Account
	Filter          PositionFilter
	BaseCurrency    string
	Summary         positionSummary
	Page            int
	HasPrev         bool
	HasNext         bool
	CacheStatus     marketcache.CacheStatus
	HasCacheStatus  bool
	LastRefreshText string
}

// lotDetailPageData is the data struct for the lot detail template.
type lotDetailPageData struct {
	web.PageData
	Lot       position.LotWithDetails
	BackHref  string
	BackLabel string
}

// PositionWebHandler handles server-rendered position pages.
type PositionWebHandler struct {
	apiHandler   *PositionHandler
	positionSvc  *position.Service
	accountSvc   *account.Service
	portfolioSvc *portfolio.Service
	marketCache  cacheStatusProvider
	renderer     *web.Renderer
}

// NewPositionWebHandler creates a new position web handler.
func NewPositionWebHandler(apiHandler *PositionHandler, positionSvc *position.Service, accountSvc *account.Service, portfolioSvc *portfolio.Service, marketCache cacheStatusProvider, renderer *web.Renderer) *PositionWebHandler {
	return &PositionWebHandler{
		apiHandler:   apiHandler,
		positionSvc:  positionSvc,
		accountSvc:   accountSvc,
		portfolioSvc: portfolioSvc,
		marketCache:  marketCache,
		renderer:     renderer,
	}
}

// RegisterRoutes mounts web position routes on the given router.
// Note: more specific routes (with sub-paths) must be registered before catch-all routes.
func (h *PositionWebHandler) RegisterRoutes(r *chi.Mux) {
	// Specific routes first
	r.Post("/positions/recalculate", h.HandleRecalculate)
	r.Get("/lots/{lot_id}", h.HandleLotDetail)
	r.Get("/positions/closed", h.HandleClosedPositions)
	// Catch-all route last
	r.Get("/positions", h.HandleOpenPositions)
}

// HandleOpenPositions renders GET /positions (open positions list with filters and pagination).
// Positions are enriched with current market data (price, market value, unrealized P&L).
func (h *PositionWebHandler) HandleOpenPositions(w http.ResponseWriter, r *http.Request) {
	filter := parsePositionFilter(r.URL.Query())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := defaultPageLimit
	offset := (page - 1) * limit

	// Build domain filters
	var domainFilters position.ListFilters
	filterToDomain(filter, &domainFilters)

	// Determine base currency from filter or first portfolio.
	baseCurrency := h.resolveBaseCurrency(r.Context(), domainFilters)

	// Fetch positions via shared API handler method.
	enriched, err := h.apiHandler.computeOpenPositions(r.Context(), domainFilters, limit, offset, baseCurrency)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Compute summary from ALL positions (not just current page).
	summary, err := h.positionSvc.GetOpenPositionsSummary(r.Context(), domainFilters, baseCurrency)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Fetch accounts for filter dropdown.
	accounts, _ := h.accountSvc.ListAll(r.Context())

	// Cache status for aggregate indicator.
	var cacheStatus marketcache.CacheStatus
	var hasCacheStatus bool
	var lastRefreshText string
	if h.marketCache != nil {
		cacheStatus = h.marketCache.GetStatus()
		hasCacheStatus = true
		if !cacheStatus.LastRefresh.IsZero() {
			lastRefreshText = formatLastRefresh(cacheStatus.LastRefresh)
		}
	}

	data := openPositionListPageData{
		PageData: web.PageData{
			Title: "Open Positions",
			Flash: getFlash(w, r),
		},
		Positions:       enriched,
		Accounts:        accounts,
		Filter:          filter,
		BaseCurrency:    baseCurrency,
		Summary:         toPositionSummary(summary),
		Page:            page,
		HasPrev:         page > 1,
		HasNext:         len(enriched) == limit,
		CacheStatus:     cacheStatus,
		HasCacheStatus:  hasCacheStatus,
		LastRefreshText: lastRefreshText,
	}

	if data.Positions == nil {
		data.Positions = []position.PositionWithMarket{}
	}
	if data.Accounts == nil {
		data.Accounts = []account.Account{}
	}

	if err := h.renderer.Render(w, "position/open", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleClosedPositions renders GET /positions/closed (closed positions list with filters and pagination).
func (h *PositionWebHandler) HandleClosedPositions(w http.ResponseWriter, r *http.Request) {
	filter := parsePositionFilter(r.URL.Query())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := defaultPageLimit
	offset := (page - 1) * limit

	// Build domain filters
	var domainFilters position.ListFilters
	filterToDomain(filter, &domainFilters)

	// Determine base currency from filter or first portfolio.
	baseCurrency := h.resolveBaseCurrency(r.Context(), domainFilters)

	// Fetch positions via shared API handler method.
	items, err := h.apiHandler.computeClosedPositions(r.Context(), domainFilters, limit, offset)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Compute summary from ALL closed positions (not just current page).
	summary, err := h.positionSvc.GetClosedPositionsSummary(r.Context(), domainFilters, baseCurrency)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Fetch accounts for filter dropdown.
	accounts, _ := h.accountSvc.ListAll(r.Context())

	data := positionListPageData{
		PageData: web.PageData{
			Title: "Closed Positions",
			Flash: getFlash(w, r),
		},
		Positions:    items,
		Accounts:     accounts,
		Filter:       filter,
		BaseCurrency: baseCurrency,
		Summary:      toClosedPositionSummary(summary),
		Page:         page,
		HasPrev:      page > 1,
		HasNext:      len(items) == limit,
	}

	if data.Positions == nil {
		data.Positions = []position.Position{}
	}
	if data.Accounts == nil {
		data.Accounts = []account.Account{}
	}

	if err := h.renderer.Render(w, "position/closed", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleLotDetail renders GET /lots/{lot_id} (lot detail with consumptions and transactions).
func (h *PositionWebHandler) HandleLotDetail(w http.ResponseWriter, r *http.Request) {
	lotID := strings.TrimSpace(chi.URLParam(r, "lot_id"))
	if lotID == "" {
		http.NotFound(w, r)
		return
	}

	details, err := h.positionSvc.GetLotDetails(r.Context(), lotID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := lotDetailPageData{
		PageData: web.PageData{
			Title: "Lot " + lotID,
			Flash: getFlash(w, r),
		},
		Lot:       *details,
		BackHref:  "/positions",
		BackLabel: "Back to Positions",
	}

	if err := h.renderer.Render(w, "position/lot_detail", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleRecalculate handles POST /positions/recalculate (manual recalc trigger with flash message).
// Delegates to the API handler for the actual recalculation logic.
func (h *PositionWebHandler) HandleRecalculate(w http.ResponseWriter, r *http.Request) {
	_, scope, err := h.apiHandler.doRecalculate(r)
	if err != nil {
		setFlash(w, "Recalculation failed: "+err.Error())
		http.Redirect(w, r, "/positions", http.StatusSeeOther)
		return
	}
	setFlash(w, "Positions recalculated for "+scope)
	http.Redirect(w, r, "/positions", http.StatusSeeOther)
}

// parsePositionFilter extracts filter parameters from query string.
func parsePositionFilter(query url.Values) PositionFilter {
	return PositionFilter{
		AccountID:   query.Get("account_id"),
		PortfolioID: query.Get("portfolio_id"),
	}
}

// filterToDomain converts a PositionFilter (string-based for template state)
// to domain ListFilters (int64-based for service calls).
func filterToDomain(filter PositionFilter, out *position.ListFilters) {
	if filter.AccountID != "" {
		if n, err := strconv.ParseInt(filter.AccountID, 10, 64); err == nil {
			out.AccountID = &n
		}
	}
	if filter.PortfolioID != "" {
		if n, err := strconv.ParseInt(filter.PortfolioID, 10, 64); err == nil {
			out.PortfolioID = &n
		}
	}
}

// resolveBaseCurrency determines the portfolio base currency for display.
// If a specific portfolio is filtered, uses its currency. Otherwise uses the
// first portfolio's currency as default.
func (h *PositionWebHandler) resolveBaseCurrency(ctx context.Context, filters position.ListFilters) string {
	if filters.PortfolioID != nil {
		if p, err := h.portfolioSvc.Get(ctx, *filters.PortfolioID); err == nil {
			return p.Currency
		}
	}
	// No portfolio filter — get all portfolios and use the first one.
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil || len(portfolios) == 0 {
		return ""
	}
	return portfolios[0].Currency
}

// toPositionSummary converts a service-level OpenPositionSummary to the
// positionSummary struct used by the template.
func toPositionSummary(s position.OpenPositionSummary) positionSummary {
	// Compute total unrealized P&L % = total_unrealized_pnl_base / total_cost_basis_base × 100.
	var pnlPct string
	if !s.TotalCostBasisBase.Equal(decimal.Zero) {
		pct, _ := s.TotalUnrealizedPnLB.Quo(s.TotalCostBasisBase)
		pct, _ = pct.Mul(decimal.MustNew(10000, 2))
		pnlPct = pct.String()
	}

	return positionSummary{
		TotalCostBasisBase:  s.TotalCostBasisBase.String(),
		TotalMktValueBase:   s.TotalMktValueBase.String(),
		TotalUnrealizedPnLB: s.TotalUnrealizedPnLB.String(),
		TotalUnrealizedPnLP: pnlPct,
		HasFxErrors:         s.HasFxErrors,
	}
}

// toClosedPositionSummary converts a service-level ClosedPositionSummary to the
// closedPositionSummary struct used by the template.
func toClosedPositionSummary(s position.ClosedPositionSummary) closedPositionSummary {
	return closedPositionSummary{
		TotalRealizedPnLB: s.TotalRealizedPnLB.String(),
		HasFxErrors:       s.HasFxErrors,
	}
}
