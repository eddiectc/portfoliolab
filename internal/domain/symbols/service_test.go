package symbols

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// --- Mock Repository ---

type mockRepo struct {
	details map[string]*market.SymbolDetails
	stale   []market.StaleSymbol
	err     error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		details: make(map[string]*market.SymbolDetails),
	}
}

func (m *mockRepo) Upsert(_ context.Context, d *market.SymbolDetails) error {
	if m.err != nil {
		return m.err
	}
	m.details[d.InternalSymbol] = d
	return nil
}

func (m *mockRepo) GetByInternalSymbol(_ context.Context, internalSymbol string) (*market.SymbolDetails, error) {
	if m.err != nil {
		return nil, m.err
	}
	d, ok := m.details[internalSymbol]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *d
	return &cp, nil
}

func (m *mockRepo) ListStale(_ context.Context, _ time.Time) ([]market.StaleSymbol, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]market.StaleSymbol, len(m.stale))
	copy(result, m.stale)
	return result, nil
}

// --- Mock Fetcher ---

type mockFetcher struct {
	details *market.SymbolDetails
	err     error
}

func (m *mockFetcher) FetchSymbolDetails(_ context.Context, _ string) (*market.SymbolDetails, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.details == nil {
		return nil, nil
	}
	cp := *m.details
	return &cp, nil
}

func newTestService() (*Service, *mockRepo, *mockFetcher) {
	repo := newMockRepo()
	fetcher := &mockFetcher{}
	return NewService(repo, fetcher), repo, fetcher
}

// --- FetchAndStore Tests ---

func TestService_FetchAndStore_Success(t *testing.T) {
	svc, repo, fetcher := newTestService()

	fetcher.details = &market.SymbolDetails{
		ShortName:  "Vanguard S&P 500 ETF",
		LongName:   "Vanguard S&P 500 ETF",
		Exchange:   "PCX",
		Currency:   "USD",
		QuoteType:  "ETF",
		FetchedAt:  time.Now(),
	}

	err := svc.FetchAndStore(context.Background(), "VOO", "VOO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify stored with internal symbol
	details, err := repo.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("expected details to be stored: %v", err)
	}
	if details.InternalSymbol != "VOO" {
		t.Errorf("expected internal_symbol 'VOO', got %q", details.InternalSymbol)
	}
	if details.ShortName != "Vanguard S&P 500 ETF" {
		t.Errorf("expected short_name 'Vanguard S&P 500 ETF', got %q", details.ShortName)
	}
}

func TestService_FetchAndStore_FetchFails(t *testing.T) {
	svc, repo, fetcher := newTestService()

	fetcher.err = fmt.Errorf("yahoo returned 404")

	err := svc.FetchAndStore(context.Background(), "FAKE", "FAKE")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify nothing stored
	_, err = repo.GetByInternalSymbol(context.Background(), "FAKE")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected no details stored, got %v", err)
	}
}

func TestService_FetchAndStore_StoreFails(t *testing.T) {
	svc, repo, fetcher := newTestService()

	fetcher.details = &market.SymbolDetails{
		ShortName:  "Test",
		FetchedAt:  time.Now(),
	}
	repo.err = fmt.Errorf("db connection lost")

	err := svc.FetchAndStore(context.Background(), "TEST", "TEST")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_FetchAndStore_PartialData(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Simulate partial data (no ETF-specific fields)
	fetcher.details = &market.SymbolDetails{
		ShortName:  "Apple Inc.",
		LongName:   "Apple Inc.",
		Exchange:   "NMS",
		Currency:   "USD",
		QuoteType:  "EQUITY",
		FetchedAt:  time.Now(),
	}

	err := svc.FetchAndStore(context.Background(), "AAPL", "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	details, _ := repo.GetByInternalSymbol(context.Background(), "AAPL")
	if details.TopHoldings != nil {
		t.Error("expected nil TopHoldings for equity")
	}
	if details.FundProfile != nil {
		t.Error("expected nil FundProfile for equity")
	}
	if details.ShortName != "Apple Inc." {
		t.Errorf("expected short_name 'Apple Inc.', got %q", details.ShortName)
	}
}

// --- GetByInternalSymbol Tests ---

func TestService_GetByInternalSymbol_Success(t *testing.T) {
	svc, repo, _ := newTestService()

	repo.details["VOO"] = &market.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500 ETF",
		Exchange:       "PCX",
		Currency:       "USD",
		QuoteType:      "ETF",
		FetchedAt:      time.Now(),
	}

	details, err := svc.GetByInternalSymbol(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details.InternalSymbol != "VOO" {
		t.Errorf("expected 'VOO', got %q", details.InternalSymbol)
	}
	if details.ShortName != "Vanguard S&P 500 ETF" {
		t.Errorf("expected 'Vanguard S&P 500 ETF', got %q", details.ShortName)
	}
}

func TestService_GetByInternalSymbol_NotFound(t *testing.T) {
	svc, _, _ := newTestService()

	_, err := svc.GetByInternalSymbol(context.Background(), "NOEXIST")
	if err == nil {
		t.Error("expected error for non-existent symbol, got nil")
	}
}

// --- GetStaleSymbols Tests ---

func TestService_GetStaleSymbols_HasStale(t *testing.T) {
	svc, repo, _ := newTestService()

	repo.stale = []market.StaleSymbol{
		{InternalSymbol: "VOO", MarketDataSymbol: "VOO", FetchedAt: time.Now().AddDate(0, 0, -8)},
		{InternalSymbol: "AAPL", MarketDataSymbol: "AAPL", FetchedAt: time.Now().AddDate(0, 0, -10)},
	}

	stale, err := svc.GetStaleSymbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 2 {
		t.Errorf("expected 2 stale symbols, got %d", len(stale))
	}
	if stale[0].InternalSymbol != "VOO" {
		t.Errorf("expected first stale 'VOO', got %q", stale[0].InternalSymbol)
	}
	if stale[1].InternalSymbol != "AAPL" {
		t.Errorf("expected second stale 'AAPL', got %q", stale[1].InternalSymbol)
	}
}

func TestService_GetStaleSymbols_NoneStale(t *testing.T) {
	svc, repo, _ := newTestService()

	repo.stale = []market.StaleSymbol{}

	stale, err := svc.GetStaleSymbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 stale symbols, got %d", len(stale))
	}
}

func TestService_GetStaleSymbols_RepoError(t *testing.T) {
	svc, repo, _ := newTestService()

	repo.err = fmt.Errorf("query failed")

	_, err := svc.GetStaleSymbols(context.Background())
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// --- RefreshSymbol Tests ---

func TestService_RefreshSymbol_Success(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Initially stale data
	repo.details["VOO"] = &market.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Old Name",
		FetchedAt:      time.Now().AddDate(0, 0, -8),
	}

	// Fetcher returns fresh data
	fetcher.details = &market.SymbolDetails{
		ShortName:  "Updated Name",
		Exchange:   "PCX",
		Currency:   "USD",
		QuoteType:  "ETF",
		FetchedAt:  time.Now(),
	}

	err := svc.RefreshSymbol(context.Background(), "VOO", "VOO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify updated
	details, _ := repo.GetByInternalSymbol(context.Background(), "VOO")
	if details.ShortName != "Updated Name" {
		t.Errorf("expected 'Updated Name', got %q", details.ShortName)
	}
}

func TestService_RefreshSymbol_FetchFails(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Existing data
	repo.details["VOO"] = &market.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Existing Data",
		FetchedAt:      time.Now().AddDate(0, 0, -8),
	}

	fetcher.err = fmt.Errorf("network error")

	err := svc.RefreshSymbol(context.Background(), "VOO", "VOO")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Existing data preserved (fetch failed before store)
	details, _ := repo.GetByInternalSymbol(context.Background(), "VOO")
	if details.ShortName != "Existing Data" {
		t.Errorf("expected existing data preserved, got %q", details.ShortName)
	}
}
