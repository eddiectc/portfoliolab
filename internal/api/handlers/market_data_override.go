package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eddiectc/portfoliolab/internal/domain/marketdata"
)

// MarketDataOverrideHandler handles HTTP requests for user-set market data
// overrides.
type MarketDataOverrideHandler struct {
	service *marketdata.Service
}

// NewMarketDataOverrideHandler creates a new market data override HTTP
// handler.
func NewMarketDataOverrideHandler(service *marketdata.Service) *MarketDataOverrideHandler {
	return &MarketDataOverrideHandler{service: service}
}

// RegisterRoutes mounts the market data override routes on the given router.
func (h *MarketDataOverrideHandler) RegisterRoutes(r *chi.Mux) {
	r.Put("/api/market-data/overrides", h.HandleSet)
	r.Get("/api/market-data/overrides", h.HandleList)
	r.Delete("/api/market-data/overrides", h.HandleDelete)
}

// HandleSet handles PUT /api/market-data/overrides.
func (h *MarketDataOverrideHandler) HandleSet(w http.ResponseWriter, r *http.Request) {
	var req marketdata.SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	o, err := h.service.Set(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, o)
}

// HandleList handles GET /api/market-data/overrides.
func (h *MarketDataOverrideHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	var symbol *string
	if v := r.URL.Query().Get("symbol"); v != "" {
		s := v
		symbol = &s
	}

	items, err := h.service.List(r.Context(), symbol)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list market data overrides")
		return
	}

	if items == nil {
		items = []marketdata.Override{}
	}

	writeJSON(w, http.StatusOK, items)
}

// HandleDelete handles DELETE /api/market-data/overrides.
func (h *MarketDataOverrideHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	symbol := query.Get("symbol")
	date := query.Get("date")
	dataType := query.Get("data_type")

	if symbol == "" || date == "" || dataType == "" {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "symbol, date and data_type query parameters are required")
		return
	}

	if err := h.service.Remove(r.Context(), symbol, date, marketdata.DataType(dataType)); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *MarketDataOverrideHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, marketdata.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "OVERRIDE_NOT_FOUND", "override not found")
		return
	}
	if errors.Is(err, marketdata.ErrInvalidSymbol) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_SYMBOL", err.Error())
		return
	}
	if errors.Is(err, marketdata.ErrInvalidDataType) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_DATA_TYPE", err.Error())
		return
	}
	if errors.Is(err, marketdata.ErrInvalidPrice) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PRICE", err.Error())
		return
	}
	if errors.Is(err, marketdata.ErrInvalidDate) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_DATE", err.Error())
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
