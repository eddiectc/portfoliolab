package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
)

// mockMarketCache simulates MarketCache for handler tests.
type mockMarketCache struct {
	refreshCalled bool
	status        marketcache.CacheStatus
}

func (m *mockMarketCache) RefreshAll(_ context.Context) {
	m.refreshCalled = true
	m.status.Refreshing = true
}

func (m *mockMarketCache) GetStatus() marketcache.CacheStatus {
	return m.status
}

func setupMarketDataHandler(t *testing.T) (*MarketDataHandler, *mockMarketCache) {
	t.Helper()
	mock := &mockMarketCache{
		status: marketcache.CacheStatus{
			LastRefresh:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			Refreshing:    false,
			FailedSymbols: []string{},
			TotalSymbols:  5,
		},
	}
	return NewMarketDataHandler(mock), mock
}

func TestHandleRefresh_Returns202(t *testing.T) {
	handler, mock := setupMarketDataHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/market-data/refresh", nil)
	w := httptest.NewRecorder()

	handler.HandleRefresh(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	if !mock.refreshCalled {
		t.Error("expected RefreshAll to be called")
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["message"] != "market data refresh started" {
		t.Errorf("unexpected message: %q", resp["message"])
	}
}

func TestHandleStatus_ReturnsCacheStatus(t *testing.T) {
	handler, mock := setupMarketDataHandler(t)
	mock.status.FailedSymbols = []string{"AAPL", "MSFT"}
	mock.status.TotalSymbols = 7

	req := httptest.NewRequest(http.MethodGet, "/api/market-data/status", nil)
	w := httptest.NewRecorder()

	handler.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var status marketcache.CacheStatus
	_ = json.NewDecoder(w.Body).Decode(&status)

	if status.TotalSymbols != 7 {
		t.Errorf("expected total_symbols 7, got %d", status.TotalSymbols)
	}
	if len(status.FailedSymbols) != 2 {
		t.Errorf("expected 2 failed symbols, got %d", len(status.FailedSymbols))
	}
	if status.Refreshing {
		t.Error("expected refreshing to be false, got true")
	}
}

func TestHandleStatus_EmptyCache(t *testing.T) {
	mock := &mockMarketCache{
		status: marketcache.CacheStatus{
			LastRefresh:   time.Time{},
			Refreshing:    false,
			FailedSymbols: []string{},
			TotalSymbols:  0,
		},
	}
	handler := NewMarketDataHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/market-data/status", nil)
	w := httptest.NewRecorder()

	handler.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var status marketcache.CacheStatus
	_ = json.NewDecoder(w.Body).Decode(&status)

	if status.TotalSymbols != 0 {
		t.Errorf("expected total_symbols 0, got %d", status.TotalSymbols)
	}
	if status.FailedSymbols == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestHandleStatus_ReflectsInProgressRefresh(t *testing.T) {
	mock := &mockMarketCache{
		status: marketcache.CacheStatus{
			LastRefresh:   time.Now().UTC(),
			Refreshing:    true,
			FailedSymbols: []string{},
			TotalSymbols:  3,
		},
	}
	handler := NewMarketDataHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/market-data/status", nil)
	w := httptest.NewRecorder()

	handler.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status marketcache.CacheStatus
	_ = json.NewDecoder(w.Body).Decode(&status)

	if !status.Refreshing {
		t.Error("expected refreshing to be true during in-progress refresh")
	}
}

func TestHandleRefresh_ThenStatus_ShowsRefreshing(t *testing.T) {
	mock := &mockMarketCache{
		status: marketcache.CacheStatus{
			LastRefresh:   time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC),
			Refreshing:    false,
			FailedSymbols: []string{},
			TotalSymbols:  4,
		},
	}
	handler := NewMarketDataHandler(mock)

	// Trigger refresh.
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/market-data/refresh", nil)
	refreshW := httptest.NewRecorder()
	handler.HandleRefresh(refreshW, refreshReq)

	if refreshW.Code != http.StatusAccepted {
		t.Errorf("expected 202 from refresh, got %d", refreshW.Code)
	}

	// Check status reflects the refresh.
	statusReq := httptest.NewRequest(http.MethodGet, "/api/market-data/status", nil)
	statusW := httptest.NewRecorder()
	handler.HandleStatus(statusW, statusReq)

	var status marketcache.CacheStatus
	_ = json.NewDecoder(statusW.Body).Decode(&status)

	if !status.Refreshing {
		t.Error("expected status to show refreshing=true after refresh call")
	}
}

func TestRoutesAreRegistered(t *testing.T) {
	mock := &mockMarketCache{
		status: marketcache.CacheStatus{TotalSymbols: 1},
	}
	r := chi.NewRouter()
	NewMarketDataHandler(mock).RegisterRoutes(r)

	// Verify POST /api/market-data/refresh is registered.
	req := httptest.NewRequest(http.MethodPost, "/api/market-data/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 for refresh route, got %d", w.Code)
	}

	// Verify GET /api/market-data/status is registered.
	req = httptest.NewRequest(http.MethodGet, "/api/market-data/status", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for status route, got %d", w.Code)
	}
}
