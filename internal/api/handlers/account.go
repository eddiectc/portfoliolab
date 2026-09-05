package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/eddiectc/portfoliolab/internal/domain/account"
)

// AccountHandler handles HTTP requests for account CRUD operations.
type AccountHandler struct {
	service *account.Service
}

// NewAccountHandler creates a new account HTTP handler.
func NewAccountHandler(service *account.Service) *AccountHandler {
	return &AccountHandler{service: service}
}

// RegisterRoutes mounts account routes on the given router.
func (h *AccountHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/accounts", h.HandleList)
	r.Post("/api/accounts", h.HandleCreate)
	r.Get("/api/accounts/{id}", h.HandleGet)
	r.Patch("/api/accounts/{id}", h.HandleUpdate)
	r.Delete("/api/accounts/{id}", h.HandleDelete)
}

// HandleCreate handles POST /api/accounts.
func (h *AccountHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req account.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	a, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, a)
}

// HandleList handles GET /api/accounts.
// Supports optional portfolio_id query parameter to filter accounts by portfolio.
func (h *AccountHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r.URL.Query())

	var accounts []account.Account
	var err error

	portfolioIDStr := r.URL.Query().Get("portfolio_id")
	if portfolioIDStr != "" {
		portfolioID, parseErr := strconv.ParseInt(portfolioIDStr, 10, 64)
		if parseErr != nil {
			writeJSONError(w, http.StatusBadRequest, "INVALID_PORTFOLIO_ID", "invalid portfolio_id parameter")
			return
		}
		accounts, err = h.service.ListByPortfolio(r.Context(), portfolioID, limit, offset)
	} else {
		accounts, err = h.service.List(r.Context(), limit, offset)
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list accounts")
		return
	}

	if accounts == nil {
		accounts = []account.Account{}
	}

	writeJSON(w, http.StatusOK, accounts)
}

// HandleGet handles GET /api/accounts/{id}.
func (h *AccountHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid account ID")
		return
	}

	a, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}

	writeJSON(w, http.StatusOK, a)
}

// HandleUpdate handles PATCH /api/accounts/{id}.
func (h *AccountHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid account ID")
		return
	}

	var req account.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	a, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, a)
}

// HandleDelete handles DELETE /api/accounts/{id}.
func (h *AccountHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid account ID")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AccountHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, account.ErrInvalidName) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_NAME", err.Error())
		return
	}
	if errors.Is(err, account.ErrNameExists) {
		writeJSONError(w, http.StatusConflict, "ACCOUNT_NAME_EXISTS", "an account with this name already exists")
		return
	}
	if errors.Is(err, account.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	if errors.Is(err, account.ErrPortfolioNotFound) {
		writeJSONError(w, http.StatusNotFound, "PORTFOLIO_NOT_FOUND", "portfolio not found")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
