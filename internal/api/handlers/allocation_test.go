package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/allocation"
)

// targetEntry is the JSON request body format for POST /api/allocation/target.
type targetEntry struct {
	Symbol    string  `json:"symbol"`
	TargetPct float64 `json:"target_pct"`
}

// --- Mock ---

type mockAllocationService struct {
	mu                      sync.RWMutex
	computeAllocationResult *allocation.AllocationResult
	computeAllocationErr    error
	getTargetResult         []allocation.TargetAllocation
	getTargetErr            error
	saveErr                 error
	deleteErr               error
	deleteAllErr            error
	driftResult             *allocation.DriftResult
	driftErr                error
	rebalanceResult         *allocation.RebalanceResult
	rebalanceErr            error
	lastFilter              allocation.AllocationFilter
	lastPortfolioID         int64
	lastEntries             []allocation.TargetEntry
	lastSymbol              string
}

func newMockAllocationService() *mockAllocationService {
	return &mockAllocationService{
		getTargetResult: []allocation.TargetAllocation{},
	}
}

func (m *mockAllocationService) ComputeAllocation(_ context.Context, filter allocation.AllocationFilter) (*allocation.AllocationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.lastFilter = filter
	if m.computeAllocationErr != nil {
		return nil, m.computeAllocationErr
	}
	return m.computeAllocationResult, nil
}

func (m *mockAllocationService) GetTargetAllocation(_ context.Context, portfolioID int64) ([]allocation.TargetAllocation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.lastPortfolioID = portfolioID
	if m.getTargetErr != nil {
		return nil, m.getTargetErr
	}
	result := make([]allocation.TargetAllocation, len(m.getTargetResult))
	copy(result, m.getTargetResult)
	return result, nil
}

func (m *mockAllocationService) SaveTargetAllocation(_ context.Context, portfolioID int64, entries []allocation.TargetEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPortfolioID = portfolioID
	m.lastEntries = entries
	return m.saveErr
}

func (m *mockAllocationService) DeleteTargetAllocation(_ context.Context, portfolioID int64, symbol string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPortfolioID = portfolioID
	m.lastSymbol = symbol
	return m.deleteErr
}

func (m *mockAllocationService) DeleteAllTargetAllocations(_ context.Context, portfolioID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPortfolioID = portfolioID
	return m.deleteAllErr
}

func (m *mockAllocationService) ComputeDrift(_ context.Context, filter allocation.AllocationFilter, portfolioID int64) (*allocation.DriftResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.lastFilter = filter
	m.lastPortfolioID = portfolioID
	if m.driftErr != nil {
		return nil, m.driftErr
	}
	return m.driftResult, nil
}

func (m *mockAllocationService) ComputeRebalancingSuggestions(_ context.Context, filter allocation.AllocationFilter, portfolioID int64) (*allocation.RebalanceResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.lastFilter = filter
	m.lastPortfolioID = portfolioID
	if m.rebalanceErr != nil {
		return nil, m.rebalanceErr
	}
	return m.rebalanceResult, nil
}

// --- Setup ---

func setupAllocationRouter(t *testing.T) (*chi.Mux, *mockAllocationService) {
	t.Helper()
	r := chi.NewRouter()
	svc := newMockAllocationService()
	h := NewAllocationHandler(svc)
	h.RegisterRoutes(r)
	return r, svc
}

// --- HandleAllocation Tests ---

func TestAllocHandleAllocation_Success(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.computeAllocationResult = &allocation.AllocationResult{
		Rows: []allocation.AllocationRow{
			{Symbol: "AAPL", AllocationPct: decimal.MustNew(600, 1), MarketValueBase: allocPtrDecimal(decimal.MustNew(60000, 2))},
			{Symbol: "MSFT", AllocationPct: decimal.MustNew(400, 1), MarketValueBase: allocPtrDecimal(decimal.MustNew(40000, 2))},
		},
		TotalValueBase: decimal.MustNew(100000, 2),
		BaseCurrency:   "USD",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result allocation.AllocationResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
	if result.TotalValueBase.String() != "1000.00" {
		t.Errorf("expected total 1000.00, got %s", result.TotalValueBase.String())
	}
}

func TestAllocHandleAllocation_WithPortfolioFilter(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.computeAllocationResult = &allocation.AllocationResult{
		Rows:         []allocation.AllocationRow{},
		BaseCurrency: "USD",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation?portfolio_ids=1,2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if len(svc.lastFilter.PortfolioIDs) != 2 {
		t.Errorf("expected 2 portfolio IDs, got %d", len(svc.lastFilter.PortfolioIDs))
	}
}

func TestAllocHandleAllocation_EmptyPortfolio(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.computeAllocationResult = &allocation.AllocationResult{
		BaseCurrency: "USD",
		Message:      "No open positions found",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result allocation.AllocationResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result.Message != "No open positions found" {
		t.Errorf("expected empty message, got %q", result.Message)
	}
}

func TestAllocHandleAllocation_ZeroTotalValue(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.computeAllocationErr = allocation.ErrZeroTotalValue

	req := httptest.NewRequest(http.MethodGet, "/api/allocation", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "ZERO_TOTAL_VALUE" {
		t.Errorf("expected ZERO_TOTAL_VALUE, got %q", errResp.Code)
	}
}

func TestAllocHandleAllocation_MixedCurrencies(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.computeAllocationErr = allocation.ErrMixedCurrencies

	req := httptest.NewRequest(http.MethodGet, "/api/allocation?portfolio_ids=1,2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MIXED_CURRENCIES" {
		t.Errorf("expected MIXED_CURRENCIES, got %q", errResp.Code)
	}
}

// --- HandleGetTarget Tests ---

func TestAllocHandleGetTarget_Success(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.getTargetResult = []allocation.TargetAllocation{
		{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustNew(600, 1)},
		{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustNew(400, 1)},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/target?portfolio_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var targets []allocation.TargetAllocation
	_ = json.NewDecoder(w.Body).Decode(&targets)
	if len(targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(targets))
	}
}

func TestAllocHandleGetTarget_NoPortfolioID(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/target", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "MISSING_PORTFOLIO_ID" {
		t.Errorf("expected MISSING_PORTFOLIO_ID, got %q", errResp.Code)
	}
}

func TestAllocHandleGetTarget_EmptyTargets(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.getTargetResult = []allocation.TargetAllocation{}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/target?portfolio_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var targets []allocation.TargetAllocation
	_ = json.NewDecoder(w.Body).Decode(&targets)
	if targets == nil {
		t.Error("expected empty array, got nil")
	}
}

// --- HandleSaveTarget Tests ---

func TestAllocHandleSaveTarget_Success(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	body := []targetEntry{
		{Symbol: "AAPL", TargetPct: 60.0},
		{Symbol: "MSFT", TargetPct: 40.0},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/allocation/target?portfolio_id=1",
		bytes.NewReader(mustMarshal(t, body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "saved" {
		t.Errorf("expected status 'saved', got %q", resp["status"])
	}
}

func TestAllocHandleSaveTarget_InvalidBody(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/allocation/target?portfolio_id=1",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_REQUEST" {
		t.Errorf("expected INVALID_REQUEST, got %q", errResp.Code)
	}
}

func TestAllocHandleSaveTarget_NoPortfolioID(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	body := []targetEntry{{Symbol: "AAPL", TargetPct: 100.0}}
	req := httptest.NewRequest(http.MethodPost, "/api/allocation/target",
		bytes.NewReader(mustMarshal(t, body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocHandleSaveTarget_InvalidPct(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.saveErr = allocation.ErrInvalidTargetPct

	body := []targetEntry{{Symbol: "AAPL", TargetPct: -5.0}}
	req := httptest.NewRequest(http.MethodPost, "/api/allocation/target?portfolio_id=1",
		bytes.NewReader(mustMarshal(t, body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "INVALID_TARGET_PCT" {
		t.Errorf("expected INVALID_TARGET_PCT, got %q", errResp.Code)
	}
}

func TestAllocHandleSaveTarget_SumNot100(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.saveErr = allocation.ErrTargetSumNot100

	body := []targetEntry{
		{Symbol: "AAPL", TargetPct: 60.0},
		{Symbol: "MSFT", TargetPct: 30.0},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/allocation/target?portfolio_id=1",
		bytes.NewReader(mustMarshal(t, body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "TARGET_SUM_NOT_100" {
		t.Errorf("expected TARGET_SUM_NOT_100, got %q", errResp.Code)
	}
}

func TestAllocHandleSaveTarget_CustomError(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.saveErr = &allocation.AllocationError{
		Code:    "target_sum_not_100",
		Message: "target percentages must sum to exactly 100% (current total: 90.00%, delta: -10.00%)",
	}

	body := []targetEntry{
		{Symbol: "AAPL", TargetPct: 60.0},
		{Symbol: "MSFT", TargetPct: 30.0},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/allocation/target?portfolio_id=1",
		bytes.NewReader(mustMarshal(t, body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var errResp APIError
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "target_sum_not_100" {
		t.Errorf("expected target_sum_not_100, got %q", errResp.Code)
	}
}

// --- HandleDeleteTarget Tests ---

func TestAllocHandleDeleteTarget_Single(t *testing.T) {
	r, svc := setupAllocationRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/allocation/target?portfolio_id=1&symbol=AAPL", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["scope"] != "AAPL" {
		t.Errorf("expected scope 'AAPL', got %q", resp["scope"])
	}
	if svc.lastPortfolioID != 1 {
		t.Errorf("expected portfolio_id 1, got %d", svc.lastPortfolioID)
	}
}

func TestAllocHandleDeleteTarget_All(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/allocation/target?portfolio_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["scope"] != "all" {
		t.Errorf("expected scope 'all', got %q", resp["scope"])
	}
}

func TestAllocHandleDeleteTarget_NoPortfolioID(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/allocation/target", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- HandleDrift Tests ---

func TestAllocHandleDrift_Success(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.driftResult = &allocation.DriftResult{
		Rows: []allocation.DriftRow{
			{Symbol: "AAPL", ActualPct: decimal.MustNew(650, 1), TargetPct: decimal.MustNew(600, 1), DriftPct: decimal.MustNew(50, 1), IsBalanced: false},
			{Symbol: "MSFT", ActualPct: decimal.MustNew(350, 1), TargetPct: decimal.MustNew(400, 1), DriftPct: decimal.MustNew(-50, 1), IsBalanced: false},
		},
		BaseCurrency: "USD",
		HasTarget:    true,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/drift?portfolio_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result allocation.DriftResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
	if !result.HasTarget {
		t.Error("expected HasTarget=true")
	}
}

func TestAllocHandleDrift_NoPortfolioID(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/drift", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocHandleDrift_NoTarget(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.driftResult = &allocation.DriftResult{
		Rows: []allocation.DriftRow{
			{Symbol: "AAPL", ActualPct: decimal.MustNew(1000, 1), TargetPct: decimal.Zero, DriftPct: decimal.MustNew(1000, 1), IsBalanced: false},
		},
		BaseCurrency: "USD",
		HasTarget:    false,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/drift?portfolio_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result allocation.DriftResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result.HasTarget {
		t.Error("expected HasTarget=false")
	}
}

// --- HandleRebalance Tests ---

func TestAllocHandleRebalance_Success(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.rebalanceResult = &allocation.RebalanceResult{
		Suggestions: []allocation.RebalanceSuggestion{
			{Symbol: "AAPL", Direction: "sell", Shares: decimal.MustNew(500, 2), DollarValue: decimal.MustNew(50000, 2)},
			{Symbol: "MSFT", Direction: "buy", Shares: decimal.MustNew(300, 2), DollarValue: decimal.MustNew(30000, 2)},
		},
		BaseCurrency:     "USD",
		TotalDollarValue: decimal.MustNew(80000, 2),
		IsBalanced:       false,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/rebalance?portfolio_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result allocation.RebalanceResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if len(result.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(result.Suggestions))
	}
	if result.IsBalanced {
		t.Error("expected IsBalanced=false")
	}
}

func TestAllocHandleRebalance_Balanced(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.rebalanceResult = &allocation.RebalanceResult{
		Suggestions:      []allocation.RebalanceSuggestion{},
		BaseCurrency:     "USD",
		TotalDollarValue: decimal.Zero,
		IsBalanced:       true,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/rebalance?portfolio_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result allocation.RebalanceResult
	_ = json.NewDecoder(w.Body).Decode(&result)
	if !result.IsBalanced {
		t.Error("expected IsBalanced=true")
	}
	if len(result.Suggestions) != 0 {
		t.Errorf("expected 0 suggestions when balanced, got %d", len(result.Suggestions))
	}
}

func TestAllocHandleRebalance_NoPortfolioID(t *testing.T) {
	r, _ := setupAllocationRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/allocation/rebalance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Route Registration Tests ---

func TestAllocRoutesRegistered(t *testing.T) {
	r, svc := setupAllocationRouter(t)
	svc.computeAllocationResult = &allocation.AllocationResult{BaseCurrency: "USD"}
	svc.getTargetResult = []allocation.TargetAllocation{}
	svc.driftResult = &allocation.DriftResult{BaseCurrency: "USD"}
	svc.rebalanceResult = &allocation.RebalanceResult{BaseCurrency: "USD", IsBalanced: true}

	tests := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/allocation"},
		{http.MethodGet, "/api/allocation/target?portfolio_id=1"},
		{http.MethodGet, "/api/allocation/drift?portfolio_id=1"},
		{http.MethodGet, "/api/allocation/rebalance?portfolio_id=1"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for %s %s, got %d", tc.method, tc.path, w.Code)
		}
	}
}

// --- parseAllocationFilter Tests ---

func TestParseAllocationFilter(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantLen int
		wantIDs []int64
	}{
		{"empty", "/api/allocation", 0, nil},
		{"single", "/api/allocation?portfolio_ids=1", 1, []int64{1}},
		{"multiple", "/api/allocation?portfolio_ids=1,2,3", 3, []int64{1, 2, 3}},
		{"with spaces", "/api/allocation?portfolio_ids=1%2C+2%2C+3", 3, []int64{1, 2, 3}},
		{"mixed valid/invalid", "/api/allocation?portfolio_ids=1,abc,3", 2, []int64{1, 3}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			filter := parseAllocationFilter(req.URL.Query())
			if len(filter.PortfolioIDs) != tc.wantLen {
				t.Errorf("expected %d IDs, got %d", tc.wantLen, len(filter.PortfolioIDs))
			}
			for i, wantID := range tc.wantIDs {
				if filter.PortfolioIDs[i] != wantID {
					t.Errorf("expected ID %d at index %d, got %d", wantID, i, filter.PortfolioIDs[i])
				}
			}
		})
	}
}

// --- Helper ---

func allocPtrDecimal(d decimal.Decimal) *decimal.Decimal {
	return &d
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return b
}
