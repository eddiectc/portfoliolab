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

// positionListPageData is the data struct for the position list templates.
type positionListPageData struct {
	web.PageData
	Positions    []position.Position
	Accounts     []account.Account
	Filter       PositionFilter
	BaseCurrency string
	Page         int
	HasPrev      bool
	HasNext      bool
}

// positionSummary holds aggregated totals for the position summary panel.
type positionSummary struct {
	TotalCostBasisBase  string
	TotalMktValueBase   string
	TotalUnrealizedPnLB string
	TotalUnrealizedPnLP string // total unrealized P&L %
}

// openPositionListPageData is the data struct for the open positions template,
// with positions enriched with market data.
type openPositionListPageData struct {
	web.PageData
	Positions    []position.PositionWithMarket
	Accounts     []account.Account
	Filter       PositionFilter
	BaseCurrency string
	Summary      positionSummary
	Page         int
	HasPrev      bool
	HasNext      bool
}

// lotDetailPageData is the data struct for the lot detail template.
type lotDetailPageData struct {
	web.PageData
	Lot          position.LotWithDetails
	BackHref     string
	BackLabel    string
}

// PositionWebHandler handles server-rendered position pages.
type PositionWebHandler struct {
	positionSvc  *position.Service
	accountSvc   *account.Service
	portfolioSvc *portfolio.Service
	renderer     *web.Renderer
}

// NewPositionWebHandler creates a new position web handler.
func NewPositionWebHandler(positionSvc *position.Service, accountSvc *account.Service, portfolioSvc *portfolio.Service, renderer *web.Renderer) *PositionWebHandler {
	return &PositionWebHandler{
		positionSvc:  positionSvc,
		accountSvc:   accountSvc,
		portfolioSvc: portfolioSvc,
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
	if filter.AccountID != "" {
		if n, err := strconv.ParseInt(filter.AccountID, 10, 64); err == nil {
			domainFilters.AccountID = &n
		}
	}
	if filter.PortfolioID != "" {
		if n, err := strconv.ParseInt(filter.PortfolioID, 10, 64); err == nil {
			domainFilters.PortfolioID = &n
		}
	}

	items, err := h.positionSvc.GetOpenPositionsFiltered(r.Context(), domainFilters, limit, offset)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Determine base currency from filter or first portfolio.
	baseCurrency := h.resolveBaseCurrency(r.Context(), domainFilters)

	// Enrich with market data (current price, market value, unrealized P&L).
	enriched := h.positionSvc.EnrichWithMarketData(r.Context(), items, baseCurrency)

	// Fetch accounts for filter dropdown.
	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)

	data := openPositionListPageData{
		PageData: web.PageData{
			Title: "Open Positions",
			Flash: getFlash(w, r),
		},
		Positions:    enriched,
		Accounts:     accounts,
		Filter:       filter,
		BaseCurrency: baseCurrency,
		Summary:      computePositionSummary(enriched),
		Page:         page,
		HasPrev:      page > 1,
		HasNext:      len(enriched) == limit,
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
	if filter.AccountID != "" {
		if n, err := strconv.ParseInt(filter.AccountID, 10, 64); err == nil {
			domainFilters.AccountID = &n
		}
	}
	if filter.PortfolioID != "" {
		if n, err := strconv.ParseInt(filter.PortfolioID, 10, 64); err == nil {
			domainFilters.PortfolioID = &n
		}
	}

	items, err := h.positionSvc.GetClosedPositionsFiltered(r.Context(), domainFilters, limit, offset)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Determine base currency from filter or first portfolio.
	baseCurrency := h.resolveBaseCurrency(r.Context(), domainFilters)

	// Fetch accounts for filter dropdown.
	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)

	data := positionListPageData{
		PageData: web.PageData{
			Title: "Closed Positions",
			Flash: getFlash(w, r),
		},
		Positions:    items,
		Accounts:     accounts,
		Filter:       filter,
		BaseCurrency: baseCurrency,
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
// Accepts optional query params: account_id, portfolio_id.
// If neither is provided, recalculates all accounts.
func (h *PositionWebHandler) HandleRecalculate(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var scope string

	if v := query.Get("account_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			setFlash(w, "Invalid account ID")
			http.Redirect(w, r, "/positions", http.StatusSeeOther)
			return
		}
		if err := h.positionSvc.RecalculateAccount(r.Context(), id); err != nil {
			setFlash(w, "Recalculation failed: "+err.Error())
			http.Redirect(w, r, "/positions", http.StatusSeeOther)
			return
		}
		scope = "account " + v
	} else if v := query.Get("portfolio_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			setFlash(w, "Invalid portfolio ID")
			http.Redirect(w, r, "/positions", http.StatusSeeOther)
			return
		}
		if err := h.positionSvc.RecalculatePortfolio(r.Context(), id); err != nil {
			setFlash(w, "Recalculation failed: "+err.Error())
			http.Redirect(w, r, "/positions", http.StatusSeeOther)
			return
		}
		scope = "portfolio " + v
	} else {
		if err := h.positionSvc.RecalculateAll(r.Context()); err != nil {
			setFlash(w, "Recalculation failed: "+err.Error())
			http.Redirect(w, r, "/positions", http.StatusSeeOther)
			return
		}
		scope = "all accounts"
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

// computePositionSummary aggregates position totals for the summary panel.
func computePositionSummary(positions []position.PositionWithMarket) positionSummary {
	var (
		totalCostBasisBase  decimal.Decimal
		totalMktValueBase   decimal.Decimal
		totalUnrealizedPnLB decimal.Decimal
	)

	for _, p := range positions {
		if !p.MarketDataAvailable {
			continue
		}
		if p.CostBasisBase != nil {
			totalCostBasisBase, _ = totalCostBasisBase.Add(*p.CostBasisBase)
		}
		if p.MarketValueBase != nil {
			totalMktValueBase, _ = totalMktValueBase.Add(*p.MarketValueBase)
		}
		if p.UnrealizedPnLBase != nil {
			totalUnrealizedPnLB, _ = totalUnrealizedPnLB.Add(*p.UnrealizedPnLBase)
		}
	}

	// Compute total unrealized P&L % = total_unrealized_pnl_base / total_cost_basis_base × 100.
	var pnlPct string
	if !totalCostBasisBase.Equal(decimal.Zero) {
		pct, _ := totalUnrealizedPnLB.Quo(totalCostBasisBase)
		pct, _ = pct.Mul(decimal.MustNew(10000, 2))
		pnlPct = pct.String()
	}

	return positionSummary{
		TotalCostBasisBase:  totalCostBasisBase.String(),
		TotalMktValueBase:   totalMktValueBase.String(),
		TotalUnrealizedPnLB: totalUnrealizedPnLB.String(),
		TotalUnrealizedPnLP: pnlPct,
	}
}
