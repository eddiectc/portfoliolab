package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
)

// PortfolioHandler handles HTTP requests for portfolio CRUD operations.
type PortfolioHandler struct {
	service *portfolio.Service
}

// NewPortfolioHandler creates a new portfolio HTTP handler.
func NewPortfolioHandler(service *portfolio.Service) *PortfolioHandler {
	return &PortfolioHandler{service: service}
}

// RegisterRoutes mounts portfolio routes on the given router.
func (h *PortfolioHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/portfolios", h.HandleList)
	r.Post("/api/portfolios", h.HandleCreate)
	r.Get("/api/portfolios/{id}", h.HandleGet)
	r.Patch("/api/portfolios/{id}", h.HandleUpdate)
	r.Delete("/api/portfolios/{id}", h.HandleDelete)
}

// HandleCreate handles POST /api/portfolios.
func (h *PortfolioHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req portfolio.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	p, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

// HandleList handles GET /api/portfolios.
func (h *PortfolioHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r.URL.Query())

	portfolios, err := h.service.List(r.Context(), limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list portfolios")
		return
	}

	if portfolios == nil {
		portfolios = []portfolio.Portfolio{}
	}

	writeJSON(w, http.StatusOK, portfolios)
}

// HandleGet handles GET /api/portfolios/{id}.
func (h *PortfolioHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid portfolio ID")
		return
	}

	p, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "PORTFOLIO_NOT_FOUND", "portfolio not found")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

// HandleUpdate handles PATCH /api/portfolios/{id}.
func (h *PortfolioHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid portfolio ID")
		return
	}

	var req portfolio.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	p, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, p)
}

// HandleDelete handles DELETE /api/portfolios/{id}.
func (h *PortfolioHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid portfolio ID")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, "PORTFOLIO_NOT_FOUND", "portfolio not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PortfolioHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, portfolio.ErrInvalidName) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_NAME", err.Error())
		return
	}
	if errors.Is(err, portfolio.ErrInvalidCurrency) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_CURRENCY", err.Error())
		return
	}
	if errors.Is(err, portfolio.ErrNameExists) {
		writeJSONError(w, http.StatusConflict, "PORTFOLIO_NAME_EXISTS", "a portfolio with this name already exists")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func parseID(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty ID")
	}
	return strconv.ParseInt(s, 10, 64)
}

const defaultLimit = 50

func parsePagination(query url.Values) (int, int) {
	limit := defaultLimit
	offset := 0

	if v := query.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
		// n == 0 or negative → keep default
	}
	if v := query.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	return limit, offset
}
