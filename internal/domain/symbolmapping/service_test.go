package symbolmapping

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- Mock Repository ---

type mockRepo struct {
	mappings      map[int64]*SymbolMapping
	byInternal    map[string]int64 // internal_symbol -> id
	brokerSymbols map[int64][]*BrokerSymbol // mapping_id -> broker symbols
	byBroker      map[string]*BrokerSymbol // "brokerName|brokerSymbol" -> BrokerSymbol
	nextID        int64
	err           error
	inUseIDs      map[int64]bool // IDs that have referencing transactions
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		mappings:      make(map[int64]*SymbolMapping),
		byInternal:    make(map[string]int64),
		brokerSymbols: make(map[int64][]*BrokerSymbol),
		byBroker:      make(map[string]*BrokerSymbol),
		nextID:        1,
		inUseIDs:      make(map[int64]bool),
	}
}

func (m *mockRepo) Create(_ context.Context, sm *SymbolMapping) error {
	if m.err != nil {
		return m.err
	}
	m.nextID++
	sm.ID = m.nextID
	cp := *sm
	cp.BrokerSymbols = nil // don't copy broker symbols on create
	m.mappings[sm.ID] = &cp
	m.byInternal[sm.InternalSymbol] = sm.ID
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id int64) (*SymbolMapping, error) {
	if m.err != nil {
		return nil, m.err
	}
	sm, ok := m.mappings[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *sm
	// Deep copy broker symbols
	if len(sm.BrokerSymbols) > 0 {
		cp.BrokerSymbols = make([]BrokerSymbol, len(sm.BrokerSymbols))
		copy(cp.BrokerSymbols, sm.BrokerSymbols)
	}
	return &cp, nil
}

func (m *mockRepo) GetByInternalSymbol(_ context.Context, internalSymbol string) (*SymbolMapping, error) {
	if m.err != nil {
		return nil, m.err
	}
	id, ok := m.byInternal[internalSymbol]
	if !ok {
		return nil, ErrNotFound
	}
	sm := m.mappings[id]
	cp := *sm
	if len(sm.BrokerSymbols) > 0 {
		cp.BrokerSymbols = make([]BrokerSymbol, len(sm.BrokerSymbols))
		copy(cp.BrokerSymbols, sm.BrokerSymbols)
	}
	return &cp, nil
}

func (m *mockRepo) GetAll(_ context.Context, limit, offset int) ([]SymbolMapping, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []SymbolMapping
	for _, sm := range m.mappings {
		cp := *sm
		if len(sm.BrokerSymbols) > 0 {
			cp.BrokerSymbols = make([]BrokerSymbol, len(sm.BrokerSymbols))
			copy(cp.BrokerSymbols, sm.BrokerSymbols)
		}
		result = append(result, cp)
	}
	if offset > 0 {
		if offset >= len(result) {
			return []SymbolMapping{}, nil
		}
		result = result[offset:]
	}
	// Simulate SQL LIMIT 0 → empty result (catches service-layer bugs)
	if limit == 0 {
		return []SymbolMapping{}, nil
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockRepo) Update(_ context.Context, sm *SymbolMapping) error {
	if m.err != nil {
		return m.err
	}
	old, ok := m.mappings[sm.ID]
	if !ok {
		return ErrNotFound
	}
	// Update internal symbol index
	if old.InternalSymbol != sm.InternalSymbol {
		delete(m.byInternal, old.InternalSymbol)
		m.byInternal[sm.InternalSymbol] = sm.ID
	}
	// Update the mapping (keep broker symbols from old)
	smCopy := *sm
	smCopy.BrokerSymbols = old.BrokerSymbols // preserve broker symbols
	m.mappings[sm.ID] = &smCopy
	return nil
}

func (m *mockRepo) Delete(_ context.Context, id int64) error {
	if m.err != nil {
		return m.err
	}
	sm, ok := m.mappings[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.byInternal, sm.InternalSymbol)
	delete(m.mappings, id)
	// Clean up broker symbols
	for _, bs := range m.brokerSymbols[id] {
		key := fmt.Sprintf("%s|%s", bs.BrokerName, bs.BrokerSymbol)
		delete(m.byBroker, key)
	}
	delete(m.brokerSymbols, id)
	return nil
}

func (m *mockRepo) AddBrokerSymbol(_ context.Context, symbolMappingID int64, brokerName, brokerSymbol string) error {
	if m.err != nil {
		return m.err
	}
	bs := &BrokerSymbol{
		ID:           m.nextID + 1000, // offset to avoid ID collision
		SymbolID:     symbolMappingID,
		BrokerName:   brokerName,
		BrokerSymbol: brokerSymbol,
		CreatedAt:    time.Now(),
	}
	m.nextID++
	m.brokerSymbols[symbolMappingID] = append(m.brokerSymbols[symbolMappingID], bs)
	key := fmt.Sprintf("%s|%s", brokerName, brokerSymbol)
	m.byBroker[key] = bs

	// Also update the mapping's BrokerSymbols slice
	if sm, ok := m.mappings[symbolMappingID]; ok {
		sm.BrokerSymbols = append(sm.BrokerSymbols, *bs)
	}
	return nil
}

func (m *mockRepo) GetBrokerSymbolByBroker(_ context.Context, brokerName, brokerSymbol string) (*BrokerSymbol, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := fmt.Sprintf("%s|%s", brokerName, brokerSymbol)
	bs, ok := m.byBroker[key]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *bs
	return &cp, nil
}

func (m *mockRepo) HasReferencingTransactions(_ context.Context, id int64) (bool, error) {
	return m.inUseIDs[id], nil
}

func (m *mockRepo) markInUse(id int64) {
	m.inUseIDs[id] = true
}

func newTestService(t *testing.T) (*Service, *mockRepo) {
	t.Helper()
	repo := newMockRepo()
	return NewService(repo), repo
}

// mockQuoteFetcher simulates a market.MarketDataFetcher for tests.
type mockQuoteFetcher struct {
	data map[string]*market.MarketData
	err  error
}

func (m *mockQuoteFetcher) FetchQuote(_ context.Context, symbol string) (*market.MarketData, error) {
	if m.err != nil {
		return nil, m.err
	}
	if d, ok := m.data[symbol]; ok {
		cp := *d
		return &cp, nil
	}
	return nil, fmt.Errorf("symbol not found: %s", symbol)
}

func (m *mockQuoteFetcher) FetchFxRate(_ context.Context, baseCurrency, quoteCurrency string) (*market.MarketData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockQuoteFetcher) FetchQuotesBatch(_ context.Context, symbols []string) map[string]*market.MarketData {
	result := make(map[string]*market.MarketData)
	if m.err != nil {
		return result
	}
	for _, sym := range symbols {
		if d, ok := m.data[sym]; ok {
			cp := *d
			result[sym] = &cp
		}
	}
	return result
}

// --- Create Tests ---

func TestService_Create(t *testing.T) {
	svc, _ := newTestService(t)

	sm, err := svc.Create(context.Background(), CreateRequest{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", sm.InternalSymbol)
	}
	if sm.MarketDataSymbol != "AAPL" {
		t.Errorf("expected market data symbol 'AAPL', got %q", sm.MarketDataSymbol)
	}
	if sm.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if sm.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestService_Create_WithBrokerSymbols(t *testing.T) {
	svc, _ := newTestService(t)

	sm, err := svc.Create(context.Background(), CreateRequest{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		BrokerSymbols: []BrokerSymbolRequest{
			{BrokerName: "IBKR", BrokerSymbol: "AAPL.US"},
			{BrokerName: "T212", BrokerSymbol: "AAPLU"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sm.BrokerSymbols) != 2 {
		t.Fatalf("expected 2 broker symbols, got %d", len(sm.BrokerSymbols))
	}
	// Check broker symbols are present (order not guaranteed)
	foundIBKR, foundT212 := false, false
	for _, bs := range sm.BrokerSymbols {
		if bs.BrokerName == "IBKR" && bs.BrokerSymbol == "AAPL.US" {
			foundIBKR = true
		}
		if bs.BrokerName == "T212" && bs.BrokerSymbol == "AAPLU" {
			foundT212 = true
		}
	}
	if !foundIBKR {
		t.Error("expected IBKR broker symbol")
	}
	if !foundT212 {
		t.Error("expected T212 broker symbol")
	}
}

func TestService_Create_InvalidInternalSymbol(t *testing.T) {
	svc, _ := newTestService(t)

	tests := []struct {
		name string
		req  CreateRequest
	}{
		{"empty internal symbol", CreateRequest{InternalSymbol: "", MarketDataSymbol: "AAPL"}},
		{"whitespace internal symbol", CreateRequest{InternalSymbol: "   ", MarketDataSymbol: "AAPL"}},
		{"too long internal symbol", CreateRequest{InternalSymbol: string(make([]byte, 21)), MarketDataSymbol: "AAPL"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), tt.req)
			if !errors.Is(err, ErrInvalidSymbol) {
				t.Errorf("expected ErrInvalidSymbol, got %v", err)
			}
		})
	}
}

func TestService_Create_InvalidMarketDataSymbol(t *testing.T) {
	svc, _ := newTestService(t)

	tests := []struct {
		name string
		req  CreateRequest
	}{
		{"empty market data symbol", CreateRequest{InternalSymbol: "AAPL", MarketDataSymbol: ""}},
		{"whitespace market data symbol", CreateRequest{InternalSymbol: "AAPL", MarketDataSymbol: "   "}},
		{"too long market data symbol", CreateRequest{InternalSymbol: "AAPL", MarketDataSymbol: string(make([]byte, 21))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), tt.req)
			if !errors.Is(err, ErrInvalidSymbol) {
				t.Errorf("expected ErrInvalidSymbol, got %v", err)
			}
		})
	}
}

func TestService_Create_DuplicateInternalSymbol(t *testing.T) {
	svc, _ := newTestService(t)

	// Create first mapping
	_, err := svc.Create(context.Background(), CreateRequest{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	// Try duplicate
	_, err = svc.Create(context.Background(), CreateRequest{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL.LON",
	})
	if !errors.Is(err, ErrInternalSymbolExists) {
		t.Errorf("expected ErrInternalSymbolExists, got %v", err)
	}
}

func TestService_Create_SymbolTrimming(t *testing.T) {
	svc, _ := newTestService(t)

	sm, err := svc.Create(context.Background(), CreateRequest{
		InternalSymbol:   "  AAPL  ",
		MarketDataSymbol: "  AAPL  ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected trimmed internal symbol 'AAPL', got %q", sm.InternalSymbol)
	}
	if sm.MarketDataSymbol != "AAPL" {
		t.Errorf("expected trimmed market data symbol 'AAPL', got %q", sm.MarketDataSymbol)
	}
}

func TestService_Create_SkipsEmptyBrokerSymbols(t *testing.T) {
	svc, _ := newTestService(t)

	sm, err := svc.Create(context.Background(), CreateRequest{
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		BrokerSymbols: []BrokerSymbolRequest{
			{BrokerName: "IBKR", BrokerSymbol: "AAPL.US"},
			{BrokerName: "", BrokerSymbol: ""},       // should be skipped
			{BrokerName: "T212", BrokerSymbol: ""},   // should be skipped (empty symbol)
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sm.BrokerSymbols) != 1 {
		t.Errorf("expected 1 broker symbol (empty ones skipped), got %d", len(sm.BrokerSymbols))
	}
}

// --- Get Tests ---

func TestService_Get(t *testing.T) {
	svc, repo := newTestService(t)

	// Seed a mapping
	repo.mappings[1] = &SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.byInternal["AAPL"] = 1

	sm, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected 'AAPL', got %q", sm.InternalSymbol)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	_, err := svc.Get(context.Background(), 999)
	if err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

// --- List Tests ---

func TestService_List(t *testing.T) {
	svc, repo := newTestService(t)

	// Seed mappings
	for i := 1; i <= 5; i++ {
		id := int64(i)
		internal := fmt.Sprintf("SYM%d", i)
		repo.mappings[id] = &SymbolMapping{
			ID:               id,
			InternalSymbol:   internal,
			MarketDataSymbol: internal,
		}
		repo.byInternal[internal] = id
	}

	// List all
	mappings, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 5 {
		t.Errorf("expected 5 mappings, got %d", len(mappings))
	}

	// Paginated
	mappings, err = svc.List(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 2 {
		t.Errorf("expected 2 mappings with limit 2, got %d", len(mappings))
	}

	// With offset
	mappings, err = svc.List(context.Background(), 3, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 3 {
		t.Errorf("expected 3 mappings with limit 3 offset 2, got %d", len(mappings))
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, _ := newTestService(t)

	mappings, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 0 {
		t.Errorf("expected 0 mappings, got %d", len(mappings))
	}
}

// --- Update Tests ---

func TestService_Update_MarketDataSymbol(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		UpdatedAt:        time.Now(),
	}
	repo.byInternal["AAPL"] = 1

	newSymbol := "AAPL.LON"
	sm, err := svc.Update(context.Background(), 1, UpdateRequest{MarketDataSymbol: &newSymbol})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.MarketDataSymbol != "AAPL.LON" {
		t.Errorf("expected market data symbol 'AAPL.LON', got %q", sm.MarketDataSymbol)
	}
}

func TestService_Update_InternalSymbol(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{
		ID:               1,
		InternalSymbol:   "APPL",
		MarketDataSymbol: "AAPL",
	}
	repo.byInternal["APPL"] = 1

	newSymbol := "AAPL"
	sm, err := svc.Update(context.Background(), 1, UpdateRequest{InternalSymbol: &newSymbol})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", sm.InternalSymbol)
	}
}

func TestService_Update_InternalSymbolToExisting(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "APPL", MarketDataSymbol: "AAPL"}
	repo.mappings[2] = &SymbolMapping{ID: 2, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["APPL"] = 1
	repo.byInternal["AAPL"] = 2

	// Try to change APPL -> AAPL (already exists)
	newSymbol := "AAPL"
	_, err := svc.Update(context.Background(), 1, UpdateRequest{InternalSymbol: &newSymbol})
	if !errors.Is(err, ErrInternalSymbolExists) {
		t.Errorf("expected ErrInternalSymbolExists, got %v", err)
	}
}

func TestService_Update_InvalidInternalSymbol(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	empty := ""
	_, err := svc.Update(context.Background(), 1, UpdateRequest{InternalSymbol: &empty})
	if !errors.Is(err, ErrInvalidSymbol) {
		t.Errorf("expected ErrInvalidSymbol, got %v", err)
	}
}

func TestService_Update_InvalidMarketDataSymbol(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	empty := ""
	_, err := svc.Update(context.Background(), 1, UpdateRequest{MarketDataSymbol: &empty})
	if !errors.Is(err, ErrInvalidSymbol) {
		t.Errorf("expected ErrInvalidSymbol, got %v", err)
	}
}

func TestService_Update_NoChanges(t *testing.T) {
	svc, repo := newTestService(t)

	originalTime := time.Now()
	repo.mappings[1] = &SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		UpdatedAt:        originalTime,
	}
	repo.byInternal["AAPL"] = 1

	sm, err := svc.Update(context.Background(), 1, UpdateRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected internal symbol 'AAPL', got %q", sm.InternalSymbol)
	}
	if sm.UpdatedAt != originalTime {
		t.Errorf("expected unchanged updated_at, got %v (was %v)", sm.UpdatedAt, originalTime)
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	newSymbol := "AAPL"
	_, err := svc.Update(context.Background(), 999, UpdateRequest{InternalSymbol: &newSymbol})
	if err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

func TestService_Update_SymbolTrimming(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "APPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["APPL"] = 1

	newSymbol := "  AAPL  "
	sm, err := svc.Update(context.Background(), 1, UpdateRequest{InternalSymbol: &newSymbol})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected trimmed internal symbol 'AAPL', got %q", sm.InternalSymbol)
	}
}

// --- Delete Tests ---

func TestService_Delete_Unused(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	err := svc.Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's gone
	_, err = svc.Get(context.Background(), 1)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_Delete_InUse(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	repo.markInUse(1) // has referencing transactions

	err := svc.Delete(context.Background(), 1)
	if !errors.Is(err, ErrInUse) {
		t.Errorf("expected ErrInUse, got %v", err)
	}

	// Verify it's still there
	sm, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected mapping to still exist, got error: %v", err)
	}
	if sm.InternalSymbol != "AAPL" {
		t.Errorf("expected 'AAPL', got %q", sm.InternalSymbol)
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	err := svc.Delete(context.Background(), 999)
	if err == nil {
		t.Error("expected error for non-existent ID, got nil")
	}
}

// --- AddBrokerSymbol Tests ---

func TestService_AddBrokerSymbol(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	err := svc.AddBrokerSymbol(context.Background(), 1, BrokerSymbolRequest{
		BrokerName:   "IBKR",
		BrokerSymbol: "AAPL.US",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify broker symbol was added
	sm, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("get after add: %v", err)
	}
	if len(sm.BrokerSymbols) != 1 {
		t.Errorf("expected 1 broker symbol, got %d", len(sm.BrokerSymbols))
	}
	if sm.BrokerSymbols[0].BrokerName != "IBKR" {
		t.Errorf("expected broker name 'IBKR', got %q", sm.BrokerSymbols[0].BrokerName)
	}
}

func TestService_AddBrokerSymbol_AlreadyMappedToDifferentInternalSymbol(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.mappings[2] = &SymbolMapping{ID: 2, InternalSymbol: "AAPL-ALT", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1
	repo.byInternal["AAPL-ALT"] = 2

	// Add broker symbol to mapping 1
	repo.brokerSymbols[1] = append(repo.brokerSymbols[1], &BrokerSymbol{
		ID: 1001, SymbolID: 1, BrokerName: "IBKR", BrokerSymbol: "AAPL.US",
	})
	repo.byBroker["IBKR|AAPL.US"] = &BrokerSymbol{
		ID: 1001, SymbolID: 1, BrokerName: "IBKR", BrokerSymbol: "AAPL.US",
	}
	repo.mappings[1].BrokerSymbols = append(repo.mappings[1].BrokerSymbols, BrokerSymbol{
		ID: 1001, SymbolID: 1, BrokerName: "IBKR", BrokerSymbol: "AAPL.US",
	})

	// Try to add same broker symbol to mapping 2
	err := svc.AddBrokerSymbol(context.Background(), 2, BrokerSymbolRequest{
		BrokerName:   "IBKR",
		BrokerSymbol: "AAPL.US",
	})
	if !errors.Is(err, ErrBrokerSymbolExists) {
		t.Errorf("expected ErrBrokerSymbolExists, got %v", err)
	}
}

func TestService_AddBrokerSymbol_InvalidBrokerName(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	err := svc.AddBrokerSymbol(context.Background(), 1, BrokerSymbolRequest{
		BrokerName:   "",
		BrokerSymbol: "AAPL.US",
	})
	if !errors.Is(err, ErrInvalidSymbol) {
		t.Errorf("expected ErrInvalidSymbol for empty broker name, got %v", err)
	}
}

func TestService_AddBrokerSymbol_InvalidBrokerSymbol(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	err := svc.AddBrokerSymbol(context.Background(), 1, BrokerSymbolRequest{
		BrokerName:   "IBKR",
		BrokerSymbol: "",
	})
	if !errors.Is(err, ErrInvalidSymbol) {
		t.Errorf("expected ErrInvalidSymbol for empty broker symbol, got %v", err)
	}
}

func TestService_AddBrokerSymbol_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	err := svc.AddBrokerSymbol(context.Background(), 999, BrokerSymbolRequest{
		BrokerName:   "IBKR",
		BrokerSymbol: "AAPL.US",
	})
	if err == nil {
		t.Error("expected error for non-existent mapping, got nil")
	}
}

func TestService_AddBrokerSymbol_SameMappingIsNoOp(t *testing.T) {
	svc, repo := newTestService(t)

	repo.mappings[1] = &SymbolMapping{ID: 1, InternalSymbol: "AAPL", MarketDataSymbol: "AAPL"}
	repo.byInternal["AAPL"] = 1

	// Add broker symbol
	err := svc.AddBrokerSymbol(context.Background(), 1, BrokerSymbolRequest{
		BrokerName:   "IBKR",
		BrokerSymbol: "AAPL.US",
	})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}

	// Add same broker symbol to same mapping — should be a no-op
	err = svc.AddBrokerSymbol(context.Background(), 1, BrokerSymbolRequest{
		BrokerName:   "IBKR",
		BrokerSymbol: "AAPL.US",
	})
	if err != nil {
		t.Fatalf("duplicate add to same mapping should be no-op, got %v", err)
	}

	// Should still have only 1 broker symbol
	sm, _ := svc.Get(context.Background(), 1)
	if len(sm.BrokerSymbols) != 1 {
		t.Errorf("expected 1 broker symbol (no-op on duplicate), got %d", len(sm.BrokerSymbols))
	}
}

// --- PreviewSymbol Tests ---

func TestService_PreviewSymbol_NoFetcher(t *testing.T) {
	_, repo := newTestService(t)
	_ = repo
	// Service created without WithMarketDataFetcher
	svc := NewService(newMockRepo())

	_, err := svc.PreviewSymbol(context.Background(), "AAPL")
	if !errors.Is(err, ErrPreviewFailed) {
		t.Errorf("expected ErrPreviewFailed, got %v", err)
	}
}

func TestService_PreviewSymbol_Success(t *testing.T) {
	repo := newMockRepo()
	fetcher := &mockQuoteFetcher{
		data: map[string]*market.MarketData{
			"AAPL": {Symbol: "AAPL", Price: decimal.MustNew(17850, 2), Currency: "USD", DataType: "stock", Source: "yahoo", Date: ""},
		},
	}
	svc := NewService(repo, WithMarketDataFetcher(fetcher))

	data, err := svc.PreviewSymbol(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got %s", data.Symbol)
	}
	if !data.Price.Equal(decimal.MustNew(17850, 2)) {
		t.Errorf("expected price 178.50, got %s", data.Price.String())
	}
	if data.Currency != "USD" {
		t.Errorf("expected currency USD, got %s", data.Currency)
	}
}

func TestService_PreviewSymbol_FetchError(t *testing.T) {
	repo := newMockRepo()
	fetcher := &mockQuoteFetcher{
		err: fmt.Errorf("network timeout"),
	}
	svc := NewService(repo, WithMarketDataFetcher(fetcher))

	_, err := svc.PreviewSymbol(context.Background(), "AAPL")
	if !errors.Is(err, ErrPreviewFailed) {
		t.Errorf("expected ErrPreviewFailed, got %v", err)
	}
}

func TestService_PreviewSymbol_SymbolNotFound(t *testing.T) {
	repo := newMockRepo()
	fetcher := &mockQuoteFetcher{
		data: map[string]*market.MarketData{},
	}
	svc := NewService(repo, WithMarketDataFetcher(fetcher))

	_, err := svc.PreviewSymbol(context.Background(), "INVALID")
	if !errors.Is(err, ErrPreviewFailed) {
		t.Errorf("expected ErrPreviewFailed, got %v", err)
	}
}
