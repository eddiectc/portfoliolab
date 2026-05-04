package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
)

// TransactionHandler handles HTTP requests for transaction CRUD operations.
type TransactionHandler struct {
	service *transaction.Service
}

// NewTransactionHandler creates a new transaction HTTP handler.
func NewTransactionHandler(service *transaction.Service) *TransactionHandler {
	return &TransactionHandler{service: service}
}

// RegisterRoutes mounts transaction routes on the given router.
func (h *TransactionHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/transactions", h.HandleList)
	r.Post("/api/transactions", h.HandleCreate)
	r.Get("/api/transactions/{id}", h.HandleGet)
	r.Patch("/api/transactions/{id}", h.HandleUpdate)
	r.Delete("/api/transactions/{id}", h.HandleDelete)
}

// HandleCreate handles POST /api/transactions.
func (h *TransactionHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req transaction.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	t, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

// HandleList handles GET /api/transactions.
func (h *TransactionHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	filters, limit, offset := parseTransactionListParams(r.URL.Query())

	items, err := h.service.List(r.Context(), filters, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list transactions")
		return
	}

	if items == nil {
		items = []transaction.Transaction{}
	}

	writeJSON(w, http.StatusOK, items)
}

// HandleGet handles GET /api/transactions/{id}.
func (h *TransactionHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid transaction ID")
		return
	}

	t, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaction not found")
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// HandleUpdate handles PATCH /api/transactions/{id}.
func (h *TransactionHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid transaction ID")
		return
	}

	var req transaction.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	t, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// HandleDelete handles DELETE /api/transactions/{id}.
func (h *TransactionHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid transaction ID")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaction not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TransactionHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, transaction.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaction not found")
		return
	}
	if errors.Is(err, transaction.ErrAccountNotFound) {
		writeJSONError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	if errors.Is(err, transaction.ErrSymbolNotFound) {
		writeJSONError(w, http.StatusBadRequest, "SYMBOL_NOT_FOUND", "symbol not found")
		return
	}
	if errors.Is(err, transaction.ErrInvalidSymbol) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_SYMBOL", err.Error())
		return
	}
	if errors.Is(err, transaction.ErrInvalidPrice) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PRICE", err.Error())
		return
	}
	if errors.Is(err, transaction.ErrInvalidCurrency) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_CURRENCY", err.Error())
		return
	}
	if errors.Is(err, transaction.ErrInvalidType) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_TYPE", err.Error())
		return
	}
	if errors.Is(err, transaction.ErrInvalidQuantity) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_QUANTITY", err.Error())
		return
	}
	if errors.Is(err, transaction.ErrInvalidDate) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_DATE", err.Error())
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

// parseTransactionListParams extracts filters and pagination from query params.
func parseTransactionListParams(query url.Values) (transaction.ListFilters, int, int) {
	limit, offset := parsePagination(query)

	var filters transaction.ListFilters

	if v := query.Get("account_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filters.AccountID = &n
		}
	}
	if v := query.Get("symbol"); v != "" {
		s := strings.TrimSpace(v)
		filters.Symbol = &s
	}
	if v := query.Get("type"); v != "" {
		t := strings.TrimSpace(v)
		filters.Type = &t
	}
	if v := query.Get("date_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filters.DateFrom = &t
		}
	}
	if v := query.Get("date_to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filters.DateTo = &t
		}
	}

	return filters, limit, offset
}
