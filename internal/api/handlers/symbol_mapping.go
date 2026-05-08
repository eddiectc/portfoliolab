package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// SymbolMappingHandler handles HTTP requests for symbol mapping CRUD operations.
type SymbolMappingHandler struct {
	service *symbolmapping.Service
}

// NewSymbolMappingHandler creates a new symbol mapping HTTP handler.
func NewSymbolMappingHandler(service *symbolmapping.Service) *SymbolMappingHandler {
	return &SymbolMappingHandler{service: service}
}

// RegisterRoutes mounts symbol mapping routes on the given router.
func (h *SymbolMappingHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/symbol-mappings/preview", h.HandlePreview)
	r.Get("/api/symbol-mappings", h.HandleList)
	r.Post("/api/symbol-mappings", h.HandleCreate)
	r.Get("/api/symbol-mappings/{id}", h.HandleGet)
	r.Patch("/api/symbol-mappings/{id}", h.HandleUpdate)
	r.Delete("/api/symbol-mappings/{id}", h.HandleDelete)
	r.Post("/api/symbol-mappings/{id}/broker-symbols", h.HandleAddBrokerSymbol)
}

// HandleCreate handles POST /api/symbol-mappings.
func (h *SymbolMappingHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req symbolmapping.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	sm, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, sm)
}

// HandleList handles GET /api/symbol-mappings.
func (h *SymbolMappingHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r.URL.Query())

	mappings, err := h.service.List(r.Context(), limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list symbol mappings")
		return
	}

	if mappings == nil {
		mappings = []symbolmapping.SymbolMapping{}
	}

	writeJSON(w, http.StatusOK, mappings)
}

// HandleGet handles GET /api/symbol-mappings/{id}.
func (h *SymbolMappingHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol mapping ID")
		return
	}

	sm, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "SYMBOL_MAPPING_NOT_FOUND", "symbol mapping not found")
		return
	}

	writeJSON(w, http.StatusOK, sm)
}

// HandleUpdate handles PATCH /api/symbol-mappings/{id}.
func (h *SymbolMappingHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol mapping ID")
		return
	}

	var req symbolmapping.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	sm, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sm)
}

// HandleDelete handles DELETE /api/symbol-mappings/{id}.
func (h *SymbolMappingHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol mapping ID")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleAddBrokerSymbol handles POST /api/symbol-mappings/{id}/broker-symbols.
func (h *SymbolMappingHandler) HandleAddBrokerSymbol(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol mapping ID")
		return
	}

	var req symbolmapping.BrokerSymbolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	if err := h.service.AddBrokerSymbol(r.Context(), id, req); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PreviewResponse wraps a market quote with auto-correction detection.
// CorrectedSymbol is populated when the market data provider returns data
// for a different symbol than requested (e.g. "AAP" → "AAPL").
type PreviewResponse struct {
	Symbol          string          `json:"symbol"`
	Name            string          `json:"name"`
	Exchange        string          `json:"exchange"`
	Currency        string          `json:"currency"`
	LatestPrice     decimal.Decimal `json:"latest_price"`
	CorrectedSymbol string          `json:"corrected_symbol"`
}

// HandlePreview handles GET /api/symbol-mappings/preview?symbol=AAPL.
// Returns a market data quote for the given symbol with auto-correction detection.
func (h *SymbolMappingHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		writeJSONError(w, http.StatusBadRequest, "MISSING_SYMBOL", "symbol query parameter is required")
		return
	}

	quote, err := h.service.PreviewSymbol(r.Context(), symbol)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "PREVIEW_FAILED", "could not fetch market data preview")
		return
	}

	resp := h.toPreviewResponse(quote, symbol)
	writeJSON(w, http.StatusOK, resp)
}

// toPreviewResponse converts a market.MarketData to a PreviewResponse, detecting
// auto-correction when the returned symbol differs from the requested symbol.
func (h *SymbolMappingHandler) toPreviewResponse(data *market.MarketData, requestedSymbol string) PreviewResponse {
	resp := PreviewResponse{
		Symbol:      data.Symbol,
		Name:        "",
		Exchange:    "",
		Currency:    data.Currency,
		LatestPrice: data.Price,
	}

	// Detect auto-correction: if the market data provider returned a different
	// symbol than requested (case-insensitive comparison)
	if !strings.EqualFold(data.Symbol, requestedSymbol) {
		resp.CorrectedSymbol = data.Symbol
	}

	return resp
}

func (h *SymbolMappingHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, symbolmapping.ErrInvalidSymbol) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_SYMBOL", err.Error())
		return
	}
	if errors.Is(err, symbolmapping.ErrInternalSymbolExists) {
		writeJSONError(w, http.StatusConflict, "INTERNAL_SYMBOL_EXISTS", "a symbol mapping with this internal symbol already exists")
		return
	}
	if errors.Is(err, symbolmapping.ErrBrokerSymbolExists) {
		writeJSONError(w, http.StatusConflict, "BROKER_SYMBOL_EXISTS", "this broker symbol is already mapped to a different internal symbol")
		return
	}
	if errors.Is(err, symbolmapping.ErrInUse) {
		writeJSONError(w, http.StatusConflict, "SYMBOL_MAPPING_IN_USE", "symbol mapping is referenced by transactions and cannot be deleted")
		return
	}
	if errors.Is(err, symbolmapping.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "SYMBOL_MAPPING_NOT_FOUND", "symbol mapping not found")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
