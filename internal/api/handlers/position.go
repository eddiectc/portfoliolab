package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/position"
)

// PositionHandler handles HTTP requests for position queries and recalculation.
type PositionHandler struct {
	service *position.Service
}

// NewPositionHandler creates a new position HTTP handler.
func NewPositionHandler(service *position.Service) *PositionHandler {
	return &PositionHandler{service: service}
}

// RegisterRoutes mounts position routes on the given router.
func (h *PositionHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/positions", h.HandleListOpen)
	r.Get("/api/positions/closed", h.HandleListClosed)
	r.Post("/api/positions/recalculate", h.HandleRecalculate)
	r.Get("/api/lots/{lot_id}", h.HandleGetLot)
}

// HandleListOpen handles GET /api/positions (open positions).
func (h *PositionHandler) HandleListOpen(w http.ResponseWriter, r *http.Request) {
	filters, limit, offset := parsePositionListParams(r.URL.Query())

	items, err := h.service.GetOpenPositionsFiltered(r.Context(), filters, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list positions")
		return
	}

	if items == nil {
		items = []position.Position{}
	}

	writeJSON(w, http.StatusOK, items)
}

// HandleListClosed handles GET /api/positions/closed (closed positions).
func (h *PositionHandler) HandleListClosed(w http.ResponseWriter, r *http.Request) {
	filters, limit, offset := parsePositionListParams(r.URL.Query())

	items, err := h.service.GetClosedPositionsFiltered(r.Context(), filters, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list positions")
		return
	}

	if items == nil {
		items = []position.Position{}
	}

	writeJSON(w, http.StatusOK, items)
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
	query := r.URL.Query()

	if v := query.Get("account_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid account_id")
			return
		}
		if err := h.service.RecalculateAccount(r.Context(), id); err != nil {
			h.handleRecalcError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "recalculated", "account_id": v})
		return
	}

	if v := query.Get("portfolio_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid portfolio_id")
			return
		}
		if err := h.service.RecalculatePortfolio(r.Context(), id); err != nil {
			h.handleRecalcError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "recalculated", "portfolio_id": v})
		return
	}

	// No filter → recalculate all.
	if err := h.service.RecalculateAll(r.Context()); err != nil {
		h.handleRecalcError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recalculated", "scope": "all"})
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
