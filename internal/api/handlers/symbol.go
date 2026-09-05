package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"github.com/eddiectc/portfoliolab/internal/domain/symbols"
	"github.com/eddiectc/portfoliolab/internal/market"
	"github.com/eddiectc/portfoliolab/internal/types/symbol"
)

// SymbolHandler handles HTTP requests for symbol CRUD operations.
type SymbolHandler struct {
	service        *symbolmapping.Service
	detailsService *symbols.Service
}

// NewSymbolHandler creates a new symbol HTTP handler.
func NewSymbolHandler(service *symbolmapping.Service, detailsService *symbols.Service) *SymbolHandler {
	return &SymbolHandler{
		service:        service,
		detailsService: detailsService,
	}
}

// RegisterRoutes mounts symbol routes on the given router.
func (h *SymbolHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/symbols/preview", h.HandlePreview)
	r.Get("/api/symbols", h.HandleList)
	r.Post("/api/symbols", h.HandleCreate)
	r.Get("/api/symbols/{id}", h.HandleGet)
	r.Patch("/api/symbols/{id}", h.HandleUpdate)
	r.Delete("/api/symbols/{id}", h.HandleDelete)
	r.Post("/api/symbols/{id}/broker-symbols", h.HandleAddBrokerSymbol)
}

// SymbolGetResponse is the enriched response for GET /api/symbols/{id}.
type SymbolGetResponse struct {
	ID               int64                        `json:"id"`
	InternalSymbol   string                       `json:"internal_symbol"`
	MarketDataSymbol string                       `json:"market_data_symbol"`
	IsBenchmark      bool                         `json:"is_benchmark"`
	DataSourceURL    string                       `json:"data_source_url"`
	BrokerSymbols    []symbolmapping.BrokerSymbol `json:"broker_symbols"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	SymbolDetails    *SymbolDetailsResponse       `json:"symbol_details"`
}

// SymbolDetailsResponse is the API representation of cached symbol details.
type SymbolDetailsResponse struct {
	InternalSymbol        string                        `json:"internal_symbol"`
	ShortName             string                        `json:"short_name"`
	LongName              string                        `json:"long_name"`
	Exchange              string                        `json:"exchange"`
	Currency              string                        `json:"currency"`
	QuoteType             string                        `json:"quote_type"`
	TopHoldings           []symbol.TopHolding           `json:"top_holdings,omitempty"`
	SectorWeightings      []symbol.SectorWeighting      `json:"sector_weightings,omitempty"`
	AggregatePositions    *symbol.AggregatePositions    `json:"aggregate_positions,omitempty"`
	FundProfile           *symbol.FundProfile           `json:"fund_profile,omitempty"`
	EquityValuation       *symbol.EquityValuation       `json:"equity_valuation,omitempty"`
	BondCharacteristics   *symbol.BondCharacteristics   `json:"bond_characteristics,omitempty"`
	GeographicAllocations []symbol.GeographicAllocation `json:"geographic_allocations,omitempty"`
	MarketCapBreakdown    *symbol.MarketCapBreakdown    `json:"market_cap_breakdown,omitempty"`
	ThemeBreakdown        []symbol.ThemeBreakdown       `json:"theme_breakdown,omitempty"`
	ExtractorAsOfDate     *time.Time                    `json:"extractor_as_of_date,omitempty"`
	FetchedAt             time.Time                     `json:"fetched_at"`
}

// HandleCreate handles POST /api/symbols.
func (h *SymbolHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
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

// HandleList handles GET /api/symbols.
func (h *SymbolHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r.URL.Query())

	mappings, err := h.service.List(r.Context(), limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list symbols")
		return
	}

	if mappings == nil {
		mappings = []symbolmapping.SymbolMapping{}
	}

	writeJSON(w, http.StatusOK, mappings)
}

// HandleGet handles GET /api/symbols/{id}.
func (h *SymbolHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol ID")
		return
	}

	sm, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "SYMBOL_NOT_FOUND", "symbol not found")
		return
	}

	resp := h.toSymbolGetResponse(sm)

	// Enrich with cached symbol details
	if h.detailsService != nil {
		details, err := h.detailsService.GetByInternalSymbol(r.Context(), sm.InternalSymbol)
		if err == nil && details != nil {
			resp.SymbolDetails = toSymbolDetailsResponse(details)
		}
		// On error (not found), symbol_details is simply null
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleUpdate handles PATCH /api/symbols/{id}.
func (h *SymbolHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol ID")
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

// HandleDelete handles DELETE /api/symbols/{id}.
func (h *SymbolHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol ID")
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleAddBrokerSymbol handles POST /api/symbols/{id}/broker-symbols.
func (h *SymbolHandler) HandleAddBrokerSymbol(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid symbol ID")
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

// HandlePreview handles GET /api/symbols/preview?symbol=AAPL.
// Returns a market data quote for the given symbol with auto-correction detection.
func (h *SymbolHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
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
func (h *SymbolHandler) toPreviewResponse(data *market.MarketData, requestedSymbol string) PreviewResponse {
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

func (h *SymbolHandler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, symbolmapping.ErrInvalidSymbol) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_SYMBOL", err.Error())
		return
	}
	if errors.Is(err, symbolmapping.ErrInternalSymbolExists) {
		writeJSONError(w, http.StatusConflict, "INTERNAL_SYMBOL_EXISTS", "a symbol with this internal symbol already exists")
		return
	}
	if errors.Is(err, symbolmapping.ErrBrokerSymbolExists) {
		writeJSONError(w, http.StatusConflict, "BROKER_SYMBOL_EXISTS", "this broker symbol is already mapped to a different internal symbol")
		return
	}
	if errors.Is(err, symbolmapping.ErrInUse) {
		writeJSONError(w, http.StatusConflict, "SYMBOL_IN_USE", "symbol is referenced by transactions and cannot be deleted")
		return
	}
	if errors.Is(err, symbolmapping.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "SYMBOL_NOT_FOUND", "symbol not found")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func (h *SymbolHandler) toSymbolGetResponse(sm *symbolmapping.SymbolMapping) SymbolGetResponse {
	resp := SymbolGetResponse{
		ID:               sm.ID,
		InternalSymbol:   sm.InternalSymbol,
		MarketDataSymbol: sm.MarketDataSymbol,
		IsBenchmark:      sm.IsBenchmark,
		DataSourceURL:    sm.DataSourceURL,
		BrokerSymbols:    sm.BrokerSymbols,
		CreatedAt:        sm.CreatedAt,
		UpdatedAt:        sm.UpdatedAt,
	}
	if resp.BrokerSymbols == nil {
		resp.BrokerSymbols = []symbolmapping.BrokerSymbol{}
	}
	return resp
}

func toSymbolDetailsResponse(details *symbol.SymbolDetails) *SymbolDetailsResponse {
	// Sort geographic allocations by percent descending
	allocs := make([]symbol.GeographicAllocation, len(details.GeographicAllocations))
	copy(allocs, details.GeographicAllocations)
	sort.Slice(allocs, func(i, j int) bool {
		return allocs[i].Percent > allocs[j].Percent
	})

	resp := &SymbolDetailsResponse{
		InternalSymbol:        details.InternalSymbol,
		ShortName:             details.ShortName,
		LongName:              details.LongName,
		Exchange:              details.Exchange,
		Currency:              details.Currency,
		QuoteType:             details.QuoteType,
		TopHoldings:           details.TopHoldings,
		SectorWeightings:      details.SectorWeightings,
		AggregatePositions:    details.AggregatePositions,
		FundProfile:           details.FundProfile,
		EquityValuation:       details.EquityValuation,
		BondCharacteristics:   details.BondCharacteristics,
		GeographicAllocations: allocs,
		MarketCapBreakdown:    details.MarketCapBreakdown,
		ThemeBreakdown:        details.Themes,
		FetchedAt:             details.FetchedAt,
	}
	if !details.ExtractorAsOfDate.IsZero() {
		resp.ExtractorAsOfDate = &details.ExtractorAsOfDate
	}
	return resp
}
