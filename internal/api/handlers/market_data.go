package handlers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
)

// marketCacheStatus exposes the subset of MarketCache needed by the handler.
type marketCacheStatus interface {
	RefreshAll(ctx context.Context)
	GetStatus() marketcache.CacheStatus
}

// MarketDataHandler handles HTTP requests for market data cache operations.
type MarketDataHandler struct {
	cache marketCacheStatus
}

// NewMarketDataHandler creates a new market data HTTP handler.
func NewMarketDataHandler(cache marketCacheStatus) *MarketDataHandler {
	return &MarketDataHandler{cache: cache}
}

// RegisterRoutes mounts market data routes on the given router.
func (h *MarketDataHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/api/market-data/refresh", h.HandleRefresh)
	r.Get("/api/market-data/status", h.HandleStatus)
}

// HandleRefresh handles POST /api/market-data/refresh.
// Triggers a full background refresh of all cached market data.
// Returns 202 Accepted immediately (operation runs asynchronously).
func (h *MarketDataHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	h.cache.RefreshAll(r.Context())
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "market data refresh started",
	})
}

// HandleStatus handles GET /api/market-data/status.
// Returns the current cache status as JSON.
func (h *MarketDataHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	status := h.cache.GetStatus()
	writeJSON(w, http.StatusOK, status)
}
