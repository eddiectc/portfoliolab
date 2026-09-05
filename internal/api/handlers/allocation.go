package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/eddiectc/portfoliolab/internal/domain/allocation"
)

// allocationService defines the methods the handler needs from the allocation service.
type allocationService interface {
	ComputeAllocation(ctx context.Context, filter allocation.AllocationFilter) (*allocation.AllocationResult, error)
	GetTargetAllocation(ctx context.Context, portfolioID int64) ([]allocation.TargetAllocation, error)
	SaveTargetAllocation(ctx context.Context, portfolioID int64, entries []allocation.TargetEntry) error
	DeleteTargetAllocation(ctx context.Context, portfolioID int64, symbol string) error
	DeleteAllTargetAllocations(ctx context.Context, portfolioID int64) error
	ComputeDrift(ctx context.Context, filter allocation.AllocationFilter, portfolioID int64) (*allocation.DriftResult, error)
	ComputeRebalancingSuggestions(ctx context.Context, filter allocation.AllocationFilter, portfolioID int64) (*allocation.RebalanceResult, error)
}

// AllocationHandler handles HTTP requests for allocation queries, target CRUD, drift, and rebalancing.
type AllocationHandler struct {
	service allocationService
}

// NewAllocationHandler creates a new allocation HTTP handler.
func NewAllocationHandler(service allocationService) *AllocationHandler {
	return &AllocationHandler{service: service}
}

// RegisterRoutes mounts allocation routes on the given router.
func (h *AllocationHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/allocation", h.HandleAllocation)
	r.Get("/api/allocation/target", h.HandleGetTarget)
	r.Post("/api/allocation/target", h.HandleSaveTarget)
	r.Delete("/api/allocation/target", h.HandleDeleteTarget)
	r.Get("/api/allocation/drift", h.HandleDrift)
	r.Get("/api/allocation/rebalance", h.HandleRebalance)
}

// HandleAllocation handles GET /api/allocation.
// Optional query param: portfolio_ids (comma-separated, empty = all portfolios).
// Returns the current allocation breakdown for the selected portfolios.
func (h *AllocationHandler) HandleAllocation(w http.ResponseWriter, r *http.Request) {
	filter := parseAllocationFilter(r.URL.Query())

	result, err := h.service.ComputeAllocation(r.Context(), filter)
	if err != nil {
		h.handleAllocationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleGetTarget handles GET /api/allocation/target.
// Required query param: portfolio_id.
// Returns saved target allocations for the portfolio.
func (h *AllocationHandler) HandleGetTarget(w http.ResponseWriter, r *http.Request) {
	portfolioID, err := parseRequiredPortfolioID(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "MISSING_PORTFOLIO_ID", "portfolio_id is required")
		return
	}

	targets, err := h.service.GetTargetAllocation(r.Context(), portfolioID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get target allocations")
		return
	}

	writeJSON(w, http.StatusOK, targets)
}

// HandleSaveTarget handles POST /api/allocation/target.
// Required query param: portfolio_id.
// Request body: array of {symbol, target_pct} entries.
// Validates that each pct is in [0, 100] and all sum to exactly 100.
func (h *AllocationHandler) HandleSaveTarget(w http.ResponseWriter, r *http.Request) {
	portfolioID, err := parseRequiredPortfolioID(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "MISSING_PORTFOLIO_ID", "portfolio_id is required")
		return
	}

	var entries []allocation.TargetEntry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	if err := h.service.SaveTargetAllocation(r.Context(), portfolioID, entries); err != nil {
		h.handleTargetError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// HandleDeleteTarget handles DELETE /api/allocation/target.
// Required query param: portfolio_id.
// Optional query param: symbol (if omitted, deletes all targets for the portfolio).
func (h *AllocationHandler) HandleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	portfolioID, err := parseRequiredPortfolioID(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "MISSING_PORTFOLIO_ID", "portfolio_id is required")
		return
	}

	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))

	var scope string
	if symbol == "" {
		if err := h.service.DeleteAllTargetAllocations(r.Context(), portfolioID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete target allocations")
			return
		}
		scope = "all"
	} else {
		if err := h.service.DeleteTargetAllocation(r.Context(), portfolioID, symbol); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete target allocation")
			return
		}
		scope = symbol
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "scope": scope})
}

// HandleDrift handles GET /api/allocation/drift.
// Required query param: portfolio_id.
// Returns drift comparison between actual and target allocation.
func (h *AllocationHandler) HandleDrift(w http.ResponseWriter, r *http.Request) {
	portfolioID, err := parseRequiredPortfolioID(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "MISSING_PORTFOLIO_ID", "portfolio_id is required")
		return
	}

	filter := allocation.AllocationFilter{PortfolioIDs: []int64{portfolioID}}

	result, err := h.service.ComputeDrift(r.Context(), filter, portfolioID)
	if err != nil {
		h.handleAllocationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleRebalance handles GET /api/allocation/rebalance.
// Required query param: portfolio_id.
// Returns rebalancing suggestions to close the gap between actual and target.
func (h *AllocationHandler) HandleRebalance(w http.ResponseWriter, r *http.Request) {
	portfolioID, err := parseRequiredPortfolioID(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "MISSING_PORTFOLIO_ID", "portfolio_id is required")
		return
	}

	filter := allocation.AllocationFilter{PortfolioIDs: []int64{portfolioID}}

	result, err := h.service.ComputeRebalancingSuggestions(r.Context(), filter, portfolioID)
	if err != nil {
		h.handleAllocationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// --- Error handlers ---

func (h *AllocationHandler) handleAllocationError(w http.ResponseWriter, err error) {
	if errors.Is(err, allocation.ErrZeroTotalValue) {
		writeJSONError(w, http.StatusBadRequest, "ZERO_TOTAL_VALUE", "portfolio has zero or negative total value")
		return
	}
	if errors.Is(err, allocation.ErrMixedCurrencies) {
		writeJSONError(w, http.StatusBadRequest, "MIXED_CURRENCIES", "selected portfolios have mixed base currencies")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func (h *AllocationHandler) handleTargetError(w http.ResponseWriter, err error) {
	if errors.Is(err, allocation.ErrInvalidTargetPct) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_TARGET_PCT", "target percentage must be between 0 and 100")
		return
	}
	if errors.Is(err, allocation.ErrTargetSumNot100) {
		writeJSONError(w, http.StatusBadRequest, "TARGET_SUM_NOT_100", "target percentages must sum to exactly 100")
		return
	}
	if allocErr := new(allocation.AllocationError); errors.As(err, &allocErr) {
		writeJSONError(w, http.StatusBadRequest, allocErr.Code, allocErr.Message)
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

// --- Parameter parsing ---

// parseAllocationFilter extracts allocation filter criteria from query params.
// Supports portfolio_ids as a comma-separated list. Empty means all portfolios.
func parseAllocationFilter(query url.Values) allocation.AllocationFilter {
	var filter allocation.AllocationFilter

	if v := query.Get("portfolio_ids"); v != "" {
		var ids []int64
		for _, part := range strings.Split(v, ",") {
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

// parseRequiredPortfolioID extracts a required single portfolio_id from query params.
func parseRequiredPortfolioID(query url.Values) (int64, error) {
	v := query.Get("portfolio_id")
	if v == "" {
		return 0, errors.New("missing portfolio_id")
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}
