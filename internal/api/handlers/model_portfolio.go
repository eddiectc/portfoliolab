package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
)

// modelPortfolioService defines the methods the handler needs from the model portfolio service.
type modelPortfolioService interface {
	Create(ctx context.Context, req modelportfolio.CreateRequest) (modelportfolio.ModelPortfolio, error)
	Get(ctx context.Context, id int64) (modelportfolio.ModelPortfolio, error)
	List(ctx context.Context, limit, offset int) ([]modelportfolio.ModelPortfolio, error)
	Update(ctx context.Context, id int64, req modelportfolio.UpdateRequest) (modelportfolio.ModelPortfolio, error)
	Delete(ctx context.Context, id int64) error
}

// ModelPortfolioHandler handles HTTP requests for model portfolio CRUD operations.
type ModelPortfolioHandler struct {
	service modelPortfolioService
}

// NewModelPortfolioHandler creates a new model portfolio HTTP handler.
func NewModelPortfolioHandler(service modelPortfolioService) *ModelPortfolioHandler {
	return &ModelPortfolioHandler{service: service}
}

// RegisterRoutes mounts model portfolio routes on the given router.
func (h *ModelPortfolioHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/model-portfolios", h.HandleList)
	r.Post("/api/model-portfolios", h.HandleCreate)
	r.Get("/api/model-portfolios/{id}", h.HandleGet)
	r.Patch("/api/model-portfolios/{id}", h.HandleUpdate)
	r.Delete("/api/model-portfolios/{id}", h.HandleDelete)
}

// HandleCreate handles POST /api/model-portfolios.
func (h *ModelPortfolioHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req modelportfolio.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	mp, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, mp)
}

// HandleList handles GET /api/model-portfolios.
func (h *ModelPortfolioHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r.URL.Query())

	modelPortfolios, err := h.service.List(r.Context(), limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list model portfolios")
		return
	}

	if modelPortfolios == nil {
		modelPortfolios = []modelportfolio.ModelPortfolio{}
	}

	writeJSON(w, http.StatusOK, modelPortfolios)
}

// HandleGet handles GET /api/model-portfolios/{id}.
func (h *ModelPortfolioHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid model portfolio ID")
		return
	}

	mp, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "MODEL_PORTFOLIO_NOT_FOUND", "model portfolio not found")
		return
	}

	writeJSON(w, http.StatusOK, mp)
}

// HandleUpdate handles PATCH /api/model-portfolios/{id}.
func (h *ModelPortfolioHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid model portfolio ID")
		return
	}

	var req modelportfolio.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	mp, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, mp)
}

// HandleDelete handles DELETE /api/model-portfolios/{id}.
func (h *ModelPortfolioHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid model portfolio ID")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, "MODEL_PORTFOLIO_NOT_FOUND", "model portfolio not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ModelPortfolioHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, modelportfolio.ErrInvalidName) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_NAME", err.Error())
		return
	}
	if errors.Is(err, modelportfolio.ErrNameExists) {
		writeJSONError(w, http.StatusConflict, "MODEL_PORTFOLIO_NAME_EXISTS", "a model portfolio with this name already exists")
		return
	}
	if errors.Is(err, modelportfolio.ErrEmptyEntries) {
		writeJSONError(w, http.StatusBadRequest, "EMPTY_ENTRIES", err.Error())
		return
	}
	if errors.Is(err, modelportfolio.ErrInvalidWeight) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_WEIGHT", err.Error())
		return
	}
	if errors.Is(err, modelportfolio.ErrDuplicateSymbol) {
		writeJSONError(w, http.StatusBadRequest, "DUPLICATE_SYMBOL", err.Error())
		return
	}
	if mpErr := new(modelportfolio.ModelPortfolioError); errors.As(err, &mpErr) {
		writeJSONError(w, http.StatusBadRequest, mpErr.Code, mpErr.Message)
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
