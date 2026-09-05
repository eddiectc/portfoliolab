package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/eddiectc/portfoliolab/internal/domain/portfolio"
	"github.com/eddiectc/portfoliolab/internal/domain/position"
)

// portfolioLister abstracts portfolio listing for base currency resolution.
type portfolioLister interface {
	List(ctx context.Context, limit, offset int) ([]portfolio.Portfolio, error)
}

// PositionHandler handles HTTP requests for position queries and recalculation.
type PositionHandler struct {
	service      *position.Service
	portfolioSvc portfolioLister
}

// NewPositionHandler creates a new position HTTP handler.
func NewPositionHandler(service *position.Service, portfolioSvc portfolioLister) *PositionHandler {
	return &PositionHandler{service: service, portfolioSvc: portfolioSvc}
}

// RegisterRoutes mounts position routes on the given router.
func (h *PositionHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/positions", h.HandleListOpen)
	r.Get("/api/positions/summary", h.HandleOpenSummary)
	r.Get("/api/positions/closed", h.HandleListClosed)
	r.Get("/api/positions/closed/summary", h.HandleClosedSummary)
	r.Post("/api/positions/recalculate", h.HandleRecalculate)
	r.Get("/api/lots/{lot_id}", h.HandleGetLot)
}

// HandleListOpen handles GET /api/positions (open positions).
// Returns positions enriched with current market data (price, market value, unrealized P&L).
func (h *PositionHandler) HandleListOpen(w http.ResponseWriter, r *http.Request) {
	filters, limit, offset := parsePositionListParams(r.URL.Query())

	enriched, err := h.computeOpenPositions(r.Context(), filters, limit, offset, "")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list positions")
		return
	}

	writeJSON(w, http.StatusOK, enriched)
}

// computeOpenPositions fetches and enriches open positions. Used by both API and web handlers.
func (h *PositionHandler) computeOpenPositions(ctx context.Context, filters position.ListFilters, limit, offset int, baseCurrency string) ([]position.PositionWithMarket, error) {
	items, err := h.service.GetOpenPositionsFiltered(ctx, filters, limit, offset)
	if err != nil {
		return nil, err
	}

	enriched := h.service.EnrichWithMarketData(ctx, items, baseCurrency)
	if enriched == nil {
		enriched = []position.PositionWithMarket{}
	}
	return enriched, nil
}

// HandleOpenSummary handles GET /api/positions/summary (aggregated totals for open positions).
// Optional query param: base_currency (auto-resolves from first portfolio if omitted).
func (h *PositionHandler) HandleOpenSummary(w http.ResponseWriter, r *http.Request) {
	filters, _, _ := parsePositionListParams(r.URL.Query())

	baseCurrency := r.URL.Query().Get("base_currency")
	if baseCurrency == "" {
		var err error
		baseCurrency, err = h.resolveBaseCurrency(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "NO_BASE_CURRENCY", err.Error())
			return
		}
	}

	summary, err := h.service.GetOpenPositionsSummary(r.Context(), filters, baseCurrency)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute summary")
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// HandleListClosed handles GET /api/positions/closed (closed positions).
func (h *PositionHandler) HandleListClosed(w http.ResponseWriter, r *http.Request) {
	filters, limit, offset := parsePositionListParams(r.URL.Query())

	items, err := h.computeClosedPositions(r.Context(), filters, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list positions")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// computeClosedPositions fetches closed positions. Used by both API and web handlers.
func (h *PositionHandler) computeClosedPositions(ctx context.Context, filters position.ListFilters, limit, offset int) ([]position.Position, error) {
	items, err := h.service.GetClosedPositionsFiltered(ctx, filters, limit, offset)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []position.Position{}
	}
	return items, nil
}

// HandleClosedSummary handles GET /api/positions/closed/summary (aggregated totals for closed positions).
// Optional query param: base_currency (auto-resolves from first portfolio if omitted).
func (h *PositionHandler) HandleClosedSummary(w http.ResponseWriter, r *http.Request) {
	filters, _, _ := parsePositionListParams(r.URL.Query())

	baseCurrency := r.URL.Query().Get("base_currency")
	if baseCurrency == "" {
		var err error
		baseCurrency, err = h.resolveBaseCurrency(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "NO_BASE_CURRENCY", err.Error())
			return
		}
	}

	summary, err := h.service.GetClosedPositionsSummary(r.Context(), filters, baseCurrency)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute summary")
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// HandleGetLot handles GET /api/lots/{lot_id}.
func (h *PositionHandler) HandleGetLot(w http.ResponseWriter, r *http.Request) {
	lotID := strings.TrimSpace(chi.URLParam(r, "lot_id"))
	if lotID == "" {
		writeJSONError(w, http.StatusBadRequest, "INVALID_LOT_ID", "empty lot ID")
		return
	}

	details, err := h.service.GetLotDetails(r.Context(), lotID)
	if err != nil {
		h.handleLotError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, details)
}

// HandleRecalculate handles POST /api/positions/recalculate.
// Accepts optional query params: account_id, portfolio_id.
// If neither is provided, recalculates all accounts.
func (h *PositionHandler) HandleRecalculate(w http.ResponseWriter, r *http.Request) {
	_, scope, err := h.doRecalculate(r)
	if err != nil {
		if parseErr(err) {
			writeJSONError(w, http.StatusBadRequest, "INVALID_ID", err.Error())
			return
		}
		h.handleRecalcError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recalculated", "scope": scope})
}

// parseErr checks if the error is a parse error (invalid ID) vs a service error.
func parseErr(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "invalid account_id") || strings.Contains(err.Error(), "invalid portfolio_id"))
}

// doRecalculate executes the recalculation based on query params.
// Returns (id int64, scope string, error). The web handler can call this
// to reuse the same logic with flash messages instead of JSON responses.
// id is the account_id or portfolio_id if specified, 0 otherwise.
func (h *PositionHandler) doRecalculate(r *http.Request) (int64, string, error) {
	query := r.URL.Query()

	if v := query.Get("account_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, "", errors.New("invalid account_id")
		}
		if err := h.service.RecalculateAccount(r.Context(), id); err != nil {
			return 0, "", err
		}
		return id, "account " + v, nil
	}

	if v := query.Get("portfolio_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, "", errors.New("invalid portfolio_id")
		}
		if err := h.service.RecalculatePortfolio(r.Context(), id); err != nil {
			return 0, "", err
		}
		return id, "portfolio " + v, nil
	}

	// No filter → recalculate all.
	if err := h.service.RecalculateAll(r.Context()); err != nil {
		return 0, "", err
	}
	return 0, "all accounts", nil
}

func (h *PositionHandler) handleLotError(w http.ResponseWriter, err error) {
	if errors.Is(err, position.ErrLotNotFound) {
		writeJSONError(w, http.StatusNotFound, "LOT_NOT_FOUND", "lot not found")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func (h *PositionHandler) handleRecalcError(w http.ResponseWriter, err error) {
	if errors.Is(err, position.ErrAccountNotFound) {
		writeJSONError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	if errors.Is(err, position.ErrPortfolioNotFound) {
		writeJSONError(w, http.StatusNotFound, "PORTFOLIO_NOT_FOUND", "portfolio not found")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

// resolveBaseCurrency determines the base currency from portfolios.
// Returns an error if no portfolios exist or if portfolios have different currencies.
func (h *PositionHandler) resolveBaseCurrency(ctx context.Context) (string, error) {
	portfolios, err := h.portfolioSvc.List(ctx, 0, 0)
	if err != nil {
		return "", errors.New("failed to resolve base currency")
	}
	if len(portfolios) == 0 {
		return "", errors.New("no base currency available — add a portfolio first")
	}
	// All portfolios must share the same base currency.
	baseCurrency := portfolios[0].Currency
	for _, p := range portfolios[1:] {
		if p.Currency != baseCurrency {
			return "", errors.New("portfolios have different base currencies — specify base_currency parameter")
		}
	}
	return baseCurrency, nil
}

// parsePositionListParams extracts filters and pagination from query params.
func parsePositionListParams(query url.Values) (position.ListFilters, int, int) {
	limit, offset := parsePagination(query)

	var filters position.ListFilters

	if v := query.Get("account_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filters.AccountID = &n
		}
	}
	if v := query.Get("portfolio_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filters.PortfolioID = &n
		}
	}
	if ids := query["account_ids"]; len(ids) > 0 {
		var accountIDs []int64
		for _, v := range ids {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				accountIDs = append(accountIDs, n)
			}
		}
		if len(accountIDs) > 0 {
			filters.AccountIDs = &accountIDs
		}
	}

	return filters, limit, offset
}
