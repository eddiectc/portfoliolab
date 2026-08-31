package symbols

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
	"github.com/govalues/decimal"
)

// --- Mock Repository ---

type mockRepo struct {
	details map[string]*symbol.SymbolDetails
	stale   []symbol.StaleSymbol
	err     error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		details: make(map[string]*symbol.SymbolDetails),
	}
}

func (m *mockRepo) Upsert(_ context.Context, d *symbol.SymbolDetails) error {
	if m.err != nil {
		return m.err
	}
	m.details[d.InternalSymbol] = d
	return nil
}

func (m *mockRepo) GetByInternalSymbol(_ context.Context, internalSymbol string) (*symbol.SymbolDetails, error) {
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

func (m *mockRepo) ListStale(_ context.Context, _ time.Time) ([]symbol.StaleSymbol, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]symbol.StaleSymbol, len(m.stale))
	copy(result, m.stale)
	return result, nil
}

func (m *mockRepo) TouchFetchedAt(_ context.Context, _ string) error {
	return m.err
}

// --- Mock Fetcher ---

type mockFetcher struct {
	details *symbol.SymbolDetails
	err     error
}

func (m *mockFetcher) FetchSymbolDetails(_ context.Context, _ string) (*symbol.SymbolDetails, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.details == nil {
		return nil, nil
	}
	cp := *m.details
	return &cp, nil
}

// --- Mock DataSourceURLSource ---

type mockDataSourceURLRepo struct {
	urls map[string]string // internal_symbol -> data_source_url
	ids  map[string]int64  // internal_symbol -> id
	err  error
}

func newMockDataSourceURLRepo() *mockDataSourceURLRepo {
	return &mockDataSourceURLRepo{
		urls: make(map[string]string),
		ids:  make(map[string]int64),
	}
}

func (m *mockDataSourceURLRepo) GetDataSourceURLByInternalSymbol(_ context.Context, internalSymbol string) (string, int64, error) {
	if m.err != nil {
		return "", 0, m.err
	}
	id, ok := m.ids[internalSymbol]
	if !ok {
		return "", 0, fmt.Errorf("symbol mapping not found for %s", internalSymbol)
	}
	return m.urls[internalSymbol], id, nil
}

func (m *mockDataSourceURLRepo) UpdateDataSourceURL(_ context.Context, id int64, url string) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

// --- Mock MarketDataRepository ---

type mockMarketDataRepo struct {
	entries []*market.MarketData
	err     error
}

func newMockMarketDataRepo() *mockMarketDataRepo {
	return &mockMarketDataRepo{}
}

func (m *mockMarketDataRepo) Upsert(_ context.Context, md *market.MarketData) error {
	if m.err != nil {
		return m.err
	}
	m.entries = append(m.entries, md)
	return nil
}

// --- Mock Extractor ---

type mockExtractor struct {
	name   string
	result *extractor.ExtractResult
	err    error
}

func (m *mockExtractor) Name() string { return m.name }
func (m *mockExtractor) Extract(_ context.Context, _ string) (*extractor.ExtractResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}
func (m *mockExtractor) Match(rawURL string) bool {
	return true // match any URL for simplicity
}

func newTestService() (*Service, *mockRepo, *mockFetcher) {
	repo := newMockRepo()
	fetcher := &mockFetcher{}
	return NewService(repo, fetcher), repo, fetcher
}

// --- FetchAndStore Tests ---

func TestService_FetchAndStore_Success(t *testing.T) {
	svc, repo, fetcher := newTestService()

	fetcher.details = &symbol.SymbolDetails{
		ShortName: "Vanguard S&P 500 ETF",
		LongName:  "Vanguard S&P 500 ETF",
		Exchange:  "PCX",
		Currency:  "USD",
		QuoteType: "ETF",
		FetchedAt: time.Now(),
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

	fetcher.details = &symbol.SymbolDetails{
		ShortName: "Test",
		FetchedAt: time.Now(),
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
	fetcher.details = &symbol.SymbolDetails{
		ShortName: "Apple Inc.",
		LongName:  "Apple Inc.",
		Exchange:  "NMS",
		Currency:  "USD",
		QuoteType: "EQUITY",
		FetchedAt: time.Now(),
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

// --- Extractor Routing Tests ---

func TestService_FetchAndStore_RoutesToExtractor_WhenURLSet(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Yahoo provides Exchange/Currency (source of truth)
	fetcher.details = &symbol.SymbolDetails{
		Exchange: "LSE",
		Currency: "USD",
	}

	// Set up extractor with WisdomTree-style data
	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			AsOfDate: time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC),
			FundInfo: &extractor.FundInfo{
				Symbol: "WMGG.L",
				Name:   "WisdomTree Megatrends",
			},
			FundProfile: &extractor.FundProfile{
				Family:             "WisdomTree",
				LegalType:          "Exchange Traded Fund",
				TotalNetAssets:     21526.37,
				AnnualExpenseRatio: 0.4,
				InceptionDate:      time.Date(2018, 11, 20, 0, 0, 0, 0, time.UTC),
			},
			Holdings: []extractor.Holding{
				{Symbol: "TSLA", Name: "Tesla Inc", Percent: 2.5},
				{Symbol: "MSFT", Name: "Microsoft Corp", Percent: 1.8},
			},
			Sectors: []extractor.SectorWeighting{
				{Sector: "technology", Percent: 21.2},
				{Sector: "industrials", Percent: 36.4},
			},
			CountryAllocation: []extractor.CountryAllocation{
				{Country: "United States", Percent: 65.0},
				{Country: "United Kingdom", Percent: 15.0},
			},
			Characteristics: &extractor.FundCharacteristics{
				PriceToEarnings:          25.3,
				EstimatedPriceToEarnings: 18.7,
				PriceToBook:              4.2,
				FieldsPresent:            extractor.CharacteristicPriceToEarnings | extractor.CharacteristicEstimatedPriceToEarnings | extractor.CharacteristicPriceToBook,
			},
			NavHistory: []extractor.NavPoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4520, 2)},
				{Date: time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4550, 2)},
			},
		},
	}
	reg.Register(mockExt)
	dispatcher := extractor.NewDispatcher(reg)

	// Set up data source URL repo
	urlRepo := newMockDataSourceURLRepo()
	urlRepo.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
	urlRepo.ids["WMGG.L"] = 1

	// Set up market data repo for NAV
	marketDataRepo := newMockMarketDataRepo()

	svc.WithExtractorDispatcher(dispatcher)
	svc.WithDataSourceURLRepo(urlRepo)
	svc.WithMarketDataRepo(marketDataRepo)

	err := svc.FetchAndStore(context.Background(), "WMGG.L", "WMGG.L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify details stored
	details, err := repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if err != nil {
		t.Fatalf("expected details to be stored: %v", err)
	}
	if details.ShortName != "WisdomTree Megatrends" {
		t.Errorf("expected short_name 'WisdomTree Megatrends', got %q", details.ShortName)
	}
	if details.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if details.FundProfile.Family != "WisdomTree" {
		t.Errorf("expected family 'WisdomTree', got %q", details.FundProfile.Family)
	}
	if len(details.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(details.TopHoldings))
	}
	if details.TopHoldings[0].Symbol != "TSLA" {
		t.Errorf("expected first holding 'TSLA', got %q", details.TopHoldings[0].Symbol)
	}
	if len(details.SectorWeightings) != 2 {
		t.Fatalf("expected 2 sectors, got %d", len(details.SectorWeightings))
	}
	if len(details.GeographicAllocations) != 2 {
		t.Fatalf("expected 2 countries, got %d", len(details.GeographicAllocations))
	}
	if details.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
	if details.EquityValuation.PriceToEarnings != 25.3 {
		t.Errorf("expected P/E 25.3, got %f", details.EquityValuation.PriceToEarnings)
	}
	if details.EquityValuation.EstimatedPriceToEarnings != 18.7 {
		t.Errorf("expected Estimated P/E 18.7, got %f", details.EquityValuation.EstimatedPriceToEarnings)
	}
	if details.ExtractorAsOfDate.IsZero() {
		t.Error("expected non-zero ExtractorAsOfDate")
	} else if !details.ExtractorAsOfDate.Equal(time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected ExtractorAsOfDate 2024-03-29, got %v", details.ExtractorAsOfDate)
	}
	// Exchange/Currency always from Yahoo, never from extractor.
	if details.Exchange != "LSE" {
		t.Errorf("expected Exchange 'LSE' (from Yahoo), got %q", details.Exchange)
	}
	if details.Currency != "USD" {
		t.Errorf("expected Currency 'USD' (from Yahoo), got %q", details.Currency)
	}
}

func TestService_FetchAndStore_UsesYahoo_WhenNoURL(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Set up dispatcher but NO data source URL configured
	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name:   "wisdomtree",
		result: &extractor.ExtractResult{},
	}
	reg.Register(mockExt)
	dispatcher := extractor.NewDispatcher(reg)

	urlRepo := newMockDataSourceURLRepo()
	// Don't set any URL — symbol falls back to Yahoo
	urlRepo.ids["VOO"] = 1

	svc.WithExtractorDispatcher(dispatcher)
	svc.WithDataSourceURLRepo(urlRepo)

	fetcher.details = &symbol.SymbolDetails{
		ShortName: "Vanguard S&P 500 ETF",
		FetchedAt: time.Now(),
	}

	err := svc.FetchAndStore(context.Background(), "VOO", "VOO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	details, _ := repo.GetByInternalSymbol(context.Background(), "VOO")
	if details.ShortName != "Vanguard S&P 500 ETF" {
		t.Errorf("expected Yahoo data, got %q", details.ShortName)
	}
}

func TestService_FetchAndStore_RoutesToExtractor_WhenDispatcherSet(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Yahoo provides Exchange/Currency
	fetcher.details = &symbol.SymbolDetails{
		Exchange: "LSE",
		Currency: "USD",
	}

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			FundInfo: &extractor.FundInfo{Name: "Extractor Fund"},
		},
	}
	reg.Register(mockExt)
	dispatcher := extractor.NewDispatcher(reg)

	urlRepo := newMockDataSourceURLRepo()
	urlRepo.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
	urlRepo.ids["WMGG.L"] = 1

	svc.WithExtractorDispatcher(dispatcher)
	svc.WithDataSourceURLRepo(urlRepo)

	err := svc.FetchAndStore(context.Background(), "WMGG.L", "WMGG.L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	details, _ := repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if details.ShortName != "Extractor Fund" {
		t.Errorf("expected extractor data, got %q", details.ShortName)
	}
}

func TestService_FetchAndStore_ExtractorError_NotPersisted(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Yahoo succeeds but extractor fails
	fetcher.details = &symbol.SymbolDetails{
		Exchange: "LSE",
		Currency: "USD",
	}

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		err:  fmt.Errorf("page not found"),
	}
	reg.Register(mockExt)
	dispatcher := extractor.NewDispatcher(reg)

	urlRepo := newMockDataSourceURLRepo()
	urlRepo.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
	urlRepo.ids["WMGG.L"] = 1

	svc.WithExtractorDispatcher(dispatcher)
	svc.WithDataSourceURLRepo(urlRepo)

	err := svc.FetchAndStore(context.Background(), "WMGG.L", "WMGG.L")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify nothing persisted
	_, err = repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if !errors.Is(err, ErrNotFound) {
		t.Error("expected no details stored after extractor error")
	}
}

// --- NAV History Tests ---

func TestService_FetchAndStore_NAVHistoryStored(t *testing.T) {
	svc, _, fetcher := newTestService()

	// Yahoo provides Exchange/Currency
	fetcher.details = &symbol.SymbolDetails{
		Exchange: "LSE",
		Currency: "USD",
	}

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			FundInfo: &extractor.FundInfo{Name: "Test Fund"},
			NavHistory: []extractor.NavPoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4520, 2)},
				{Date: time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4550, 2)},
				{Date: time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4600, 2)},
			},
		},
	}
	reg.Register(mockExt)
	dispatcher := extractor.NewDispatcher(reg)

	urlRepo := newMockDataSourceURLRepo()
	urlRepo.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
	urlRepo.ids["WMGG.L"] = 1

	marketDataRepo := newMockMarketDataRepo()
	svc.WithExtractorDispatcher(dispatcher)
	svc.WithDataSourceURLRepo(urlRepo)
	svc.WithMarketDataRepo(marketDataRepo)

	err := svc.FetchAndStore(context.Background(), "WMGG.L", "WMGG.L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify NAV entries stored
	if len(marketDataRepo.entries) != 3 {
		t.Fatalf("expected 3 NAV entries, got %d", len(marketDataRepo.entries))
	}
	for i, entry := range marketDataRepo.entries {
		if entry.Symbol != "WMGG.L" {
			t.Errorf("entry %d: expected symbol 'WMGG.L', got %q", i, entry.Symbol)
		}
		if entry.DataType != "nav" {
			t.Errorf("entry %d: expected data_type 'nav', got %q", i, entry.DataType)
		}
		if entry.Source != "wisdomtree" {
			t.Errorf("entry %d: expected source 'wisdomtree', got %q", i, entry.Source)
		}
	}
	// Check dates
	expectedDates := []string{"2024-01-15", "2024-01-16", "2024-01-17"}
	for i, date := range expectedDates {
		if marketDataRepo.entries[i].Date != date {
			t.Errorf("entry %d: expected date %q, got %q", i, date, marketDataRepo.entries[i].Date)
		}
	}
}

func TestService_FetchAndStore_NAVHistoryNotStored_WhenNoMarketDataRepo(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Yahoo provides Exchange/Currency
	fetcher.details = &symbol.SymbolDetails{
		Exchange: "LSE",
		Currency: "USD",
	}

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			FundInfo: &extractor.FundInfo{Name: "Test Fund"},
			NavHistory: []extractor.NavPoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4520, 2)},
			},
		},
	}
	reg.Register(mockExt)
	dispatcher := extractor.NewDispatcher(reg)

	urlRepo := newMockDataSourceURLRepo()
	urlRepo.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
	urlRepo.ids["WMGG.L"] = 1

	// Don't set market data repo
	svc.WithExtractorDispatcher(dispatcher)
	svc.WithDataSourceURLRepo(urlRepo)
	// No WithMarketDataRepo

	err := svc.FetchAndStore(context.Background(), "WMGG.L", "WMGG.L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Details should still be stored
	_, err = repo.GetByInternalSymbol(context.Background(), "WMGG.L")
	if err != nil {
		t.Error("expected details to be stored even without market data repo")
	}
}

func TestService_FetchAndStore_NAVHistoryNotStored_WhenYahooRoute(t *testing.T) {
	svc, _, fetcher := newTestService()

	marketDataRepo := newMockMarketDataRepo()
	svc.WithMarketDataRepo(marketDataRepo)

	fetcher.details = &symbol.SymbolDetails{
		ShortName: "Vanguard S&P 500 ETF",
		FetchedAt: time.Now(),
	}

	err := svc.FetchAndStore(context.Background(), "VOO", "VOO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Yahoo route doesn't produce NAV history
	if len(marketDataRepo.entries) != 0 {
		t.Errorf("expected 0 NAV entries for Yahoo route, got %d", len(marketDataRepo.entries))
	}
}

func TestService_FetchAndStore_NAVHistoryStoreFails(t *testing.T) {
	svc, _, fetcher := newTestService()

	// Yahoo provides Exchange/Currency
	fetcher.details = &symbol.SymbolDetails{
		Exchange: "LSE",
		Currency: "USD",
	}

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			FundInfo: &extractor.FundInfo{Name: "Test Fund"},
			NavHistory: []extractor.NavPoint{
				{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4520, 2)},
			},
		},
	}
	reg.Register(mockExt)
	dispatcher := extractor.NewDispatcher(reg)

	urlRepo := newMockDataSourceURLRepo()
	urlRepo.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
	urlRepo.ids["WMGG.L"] = 1

	marketDataRepo := newMockMarketDataRepo()
	marketDataRepo.err = fmt.Errorf("db connection lost")
	svc.WithExtractorDispatcher(dispatcher)
	svc.WithDataSourceURLRepo(urlRepo)
	svc.WithMarketDataRepo(marketDataRepo)

	err := svc.FetchAndStore(context.Background(), "WMGG.L", "WMGG.L")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- DataSourceURL Tests ---

func TestService_GetDataSourceURL(t *testing.T) {
	tests := []struct {
		name      string
		symbol    string
		setupRepo func() DataSourceURLSource
		wantURL   string
		wantErr   bool
	}{
		{
			name:   "success",
			symbol: "WMGG.L",
			setupRepo: func() DataSourceURLSource {
				r := newMockDataSourceURLRepo()
				r.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
				r.ids["WMGG.L"] = 1
				return r
			},
			wantURL: "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/",
		},
		{
			name:   "empty when no URL configured",
			symbol: "VOO",
			setupRepo: func() DataSourceURLSource {
				r := newMockDataSourceURLRepo()
				r.ids["VOO"] = 1
				return r
			},
			wantURL: "",
		},
		{
			name:      "empty when no repo",
			symbol:    "VOO",
			setupRepo: nil,
			wantURL:   "",
			wantErr:   false,
		},
		{
			name:   "error when repo fails",
			symbol: "WMGG.L",
			setupRepo: func() DataSourceURLSource {
				r := newMockDataSourceURLRepo()
				r.ids["WMGG.L"] = 1
				r.err = fmt.Errorf("db error")
				return r
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := newTestService()
			if tt.setupRepo != nil {
				svc.WithDataSourceURLRepo(tt.setupRepo())
			}
			url, err := svc.GetDataSourceURL(context.Background(), tt.symbol)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if url != tt.wantURL {
				t.Errorf("expected URL %q, got %q", tt.wantURL, url)
			}
		})
	}
}

func TestService_SetDataSourceURL(t *testing.T) {
	tests := []struct {
		name      string
		symbol    string
		url       string
		setupRepo func() DataSourceURLSource
		wantErr   bool
	}{
		{
			name:   "success",
			symbol: "WMGG.L",
			url:    "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/",
			setupRepo: func() DataSourceURLSource {
				r := newMockDataSourceURLRepo()
				r.ids["WMGG.L"] = 1
				return r
			},
		},
		{
			name:      "error when no repo",
			symbol:    "WMGG.L",
			url:       "https://example.com",
			setupRepo: nil,
			wantErr:   true,
		},
		{
			name:   "error when symbol not found",
			symbol: "NONEXISTENT",
			url:    "https://example.com",
			setupRepo: func() DataSourceURLSource {
				return newMockDataSourceURLRepo()
			},
			wantErr: true,
		},
		{
			name:   "clear URL",
			symbol: "WMGG.L",
			url:    "",
			setupRepo: func() DataSourceURLSource {
				r := newMockDataSourceURLRepo()
				r.urls["WMGG.L"] = "https://www.wisdomtree.com/uk/en/ics/etfs/WMGG/"
				r.ids["WMGG.L"] = 1
				return r
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := newTestService()
			if tt.setupRepo != nil {
				svc.WithDataSourceURLRepo(tt.setupRepo())
			}
			err := svc.SetDataSourceURL(context.Background(), tt.symbol, tt.url)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- extractResultToSymbolDetails Tests ---

func TestService_extractResultToSymbolDetails_FullResult(t *testing.T) {
	asOfDate := time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC)
	result := &extractor.ExtractResult{
		AsOfDate: asOfDate,
		FundInfo: &extractor.FundInfo{
			Symbol: "WMGG.L",
			Name:   "WisdomTree Megatrends",
		},
		FundProfile: &extractor.FundProfile{
			Family:             "WisdomTree",
			LegalType:          "Exchange Traded Fund",
			TotalNetAssets:     21526.37,
			AnnualExpenseRatio: 0.4,
			InceptionDate:      time.Date(2018, 11, 20, 0, 0, 0, 0, time.UTC),
		},
		Holdings: []extractor.Holding{
			{Symbol: "TSLA", Name: "Tesla Inc", Percent: 2.5},
		},
		Sectors: []extractor.SectorWeighting{
			{Sector: "technology", Percent: 21.2},
		},
		CountryAllocation: []extractor.CountryAllocation{
			{Country: "United States", Percent: 65.0},
		},
		Characteristics: &extractor.FundCharacteristics{
			PriceToEarnings:          25.3,
			EstimatedPriceToEarnings: 19.1,
			PriceToBook:              4.2,
			PriceToCashflow:          18.0,
			PriceToSales:             5.5,
			FieldsPresent:            extractor.CharacteristicPriceToEarnings | extractor.CharacteristicEstimatedPriceToEarnings | extractor.CharacteristicPriceToBook | extractor.CharacteristicPriceToCashflow | extractor.CharacteristicPriceToSales,
		},
		MarketCap: &extractor.MarketCapBreakdown{
			Total: 100,
			Large: 15.2,
			Mid:   68.5,
			Small: 16.3,
		},
		Themes: []extractor.Theme{
			{Name: "Technology", Percent: 42.5},
			{Name: "Consumer Discretionary", Percent: 28.3},
		},
	}

	details := extractResultToSymbolDetails(result, "WMGG.L")

	if details.InternalSymbol != "WMGG.L" {
		t.Errorf("expected internal symbol 'WMGG.L', got %q", details.InternalSymbol)
	}
	if details.ShortName != "WisdomTree Megatrends" {
		t.Errorf("expected short name 'WisdomTree Megatrends', got %q", details.ShortName)
	}
	if len(details.TopHoldings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(details.TopHoldings))
	}
	if details.TopHoldings[0].Symbol != "TSLA" {
		t.Errorf("expected holding symbol 'TSLA', got %q", details.TopHoldings[0].Symbol)
	}
	if details.TopHoldings[0].Percent != 2.5 {
		t.Errorf("expected holding percent 2.5, got %f", details.TopHoldings[0].Percent)
	}
	if len(details.SectorWeightings) != 1 {
		t.Fatalf("expected 1 sector, got %d", len(details.SectorWeightings))
	}
	if details.SectorWeightings[0].Sector != "technology" {
		t.Errorf("expected sector 'technology', got %q", details.SectorWeightings[0].Sector)
	}
	if len(details.GeographicAllocations) != 1 {
		t.Fatalf("expected 1 country, got %d", len(details.GeographicAllocations))
	}
	if details.GeographicAllocations[0].Country != "United States" {
		t.Errorf("expected country 'United States', got %q", details.GeographicAllocations[0].Country)
	}
	if details.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if details.FundProfile.Family != "WisdomTree" {
		t.Errorf("expected family 'WisdomTree', got %q", details.FundProfile.Family)
	}
	if details.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
	if details.EquityValuation.PriceToEarnings != 25.3 {
		t.Errorf("expected P/E 25.3, got %f", details.EquityValuation.PriceToEarnings)
	}
	if details.EquityValuation.EstimatedPriceToEarnings != 19.1 {
		t.Errorf("expected Estimated P/E 19.1, got %f", details.EquityValuation.EstimatedPriceToEarnings)
	}
	if details.ExtractorAsOfDate.IsZero() {
		t.Error("expected non-zero ExtractorAsOfDate")
	} else if !details.ExtractorAsOfDate.Equal(asOfDate) {
		t.Errorf("expected ExtractorAsOfDate %v, got %v", asOfDate, details.ExtractorAsOfDate)
	}
	if details.MarketCapBreakdown == nil {
		t.Fatal("expected non-nil MarketCapBreakdown")
	}
	if details.MarketCapBreakdown.Total != 100 {
		t.Errorf("expected total 100, got %f", details.MarketCapBreakdown.Total)
	}
	if details.MarketCapBreakdown.Large != 15.2 {
		t.Errorf("expected large 15.2, got %f", details.MarketCapBreakdown.Large)
	}
	if details.MarketCapBreakdown.Mid != 68.5 {
		t.Errorf("expected mid 68.5, got %f", details.MarketCapBreakdown.Mid)
	}
	if details.MarketCapBreakdown.Small != 16.3 {
		t.Errorf("expected small 16.3, got %f", details.MarketCapBreakdown.Small)
	}
	if len(details.Themes) != 2 {
		t.Fatalf("expected 2 themes, got %d", len(details.Themes))
	}
	if details.Themes[0].Name != "Technology" || details.Themes[0].Percent != 42.5 {
		t.Errorf("expected first theme Technology 42.5, got %s %f", details.Themes[0].Name, details.Themes[0].Percent)
	}
	if details.Themes[1].Name != "Consumer Discretionary" || details.Themes[1].Percent != 28.3 {
		t.Errorf("expected second theme Consumer Discretionary 28.3, got %s %f", details.Themes[1].Name, details.Themes[1].Percent)
	}
}

func TestService_extractResultToSymbolDetails_IMGPFIELDS(t *testing.T) {
	asOfDate := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	result := &extractor.ExtractResult{
		AsOfDate: asOfDate,
		FundInfo: &extractor.FundInfo{
			Symbol: "LU2951555585",
			Name:   "iMGP Multi Asset Fund",
		},
		FundProfile: &extractor.FundProfile{
			Family:             "iM Global Partner",
			LegalType:          "Undertaking for Collective Investment",
			TotalNetAssets:     52400000,
			AnnualExpenseRatio: 0.015,
			InceptionDate:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			Isin:               "LU2951555585",
			ShareClassName:     "R USD UCITS ETF",
			OngoingCharges:     1.5,
		},
		RiskMeasures: &extractor.RiskMeasures{
			Volatility:    9.16,
			SharpeRatio:   2.52,
			FieldsPresent: extractor.RiskFieldVolatility | extractor.RiskFieldSharpeRatio,
		},
		AssetClassAllocation: []extractor.AssetClassEntry{
			{AssetClass: "Equities", Percent: 85.2},
			{AssetClass: "Bonds", Percent: 5.3},
			{AssetClass: "Gold", Percent: -2.1},
			{AssetClass: "Cash", Percent: 11.6},
		},
		EquityDerivativesByRegion: []extractor.RegionDerivativeEntry{
			{Region: "North America", Percent: 52.3},
			{Region: "Europe", Percent: 31.7},
			{Region: "Asia", Percent: 12.0},
			{Region: "Emerging Countries", Percent: 4.0},
		},
		CurrencyDerivativesAllocation: []extractor.CurrencyDerivativeEntry{
			{Currency: "USD", Percent: 65.0},
			{Currency: "EUR", Percent: 20.0},
			{Currency: "JPY", Percent: 15.0},
		},
	}

	details := extractResultToSymbolDetails(result, "IMGPFUND")

	// FundProfile — new iMGP fields
	if details.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if details.FundProfile.Isin != "LU2951555585" {
		t.Errorf("expected Isin LU2951555585, got %q", details.FundProfile.Isin)
	}
	if details.FundProfile.ShareClassName != "R USD UCITS ETF" {
		t.Errorf("expected ShareClassName 'R USD UCITS ETF', got %q", details.FundProfile.ShareClassName)
	}
	if details.FundProfile.OngoingCharges != 1.5 {
		t.Errorf("expected OngoingCharges 1.5, got %f", details.FundProfile.OngoingCharges)
	}

	// RiskMeasures
	if details.RiskMeasures == nil {
		t.Fatal("expected non-nil RiskMeasures")
	}
	if details.RiskMeasures.Volatility != 9.16 {
		t.Errorf("expected Volatility 9.16, got %f", details.RiskMeasures.Volatility)
	}
	if details.RiskMeasures.SharpeRatio != 2.52 {
		t.Errorf("expected SharpeRatio 2.52, got %f", details.RiskMeasures.SharpeRatio)
	}
	if details.RiskMeasures.InfoRatio != 0 {
		t.Errorf("expected InfoRatio 0 (not present), got %f", details.RiskMeasures.InfoRatio)
	}
	expectedMask := symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio
	if details.RiskMeasures.FieldsPresent != expectedMask {
		t.Errorf("expected FieldsPresent %v, got %v", expectedMask, details.RiskMeasures.FieldsPresent)
	}
	// Verify HasField helper works
	if !details.RiskMeasures.HasField(symbol.SymbolRiskFieldVolatility) {
		t.Error("expected HasField(Volatility) = true")
	}
	if details.RiskMeasures.HasField(symbol.SymbolRiskFieldInfoRatio) {
		t.Error("expected HasField(InfoRatio) = false")
	}
	if details.RiskMeasures.AllFieldsPresent() {
		t.Error("expected AllFieldsPresent() = false (only 2 fields present)")
	}

	// AssetClassAllocation
	if len(details.AssetClassAllocation) != 4 {
		t.Fatalf("expected 4 asset class entries, got %d", len(details.AssetClassAllocation))
	}
	if details.AssetClassAllocation[0].AssetClass != "Equities" || details.AssetClassAllocation[0].Percent != 85.2 {
		t.Errorf("expected Equities 85.2, got %s %f", details.AssetClassAllocation[0].AssetClass, details.AssetClassAllocation[0].Percent)
	}
	if details.AssetClassAllocation[2].AssetClass != "Gold" || details.AssetClassAllocation[2].Percent != -2.1 {
		t.Errorf("expected Gold -2.1, got %s %f", details.AssetClassAllocation[2].AssetClass, details.AssetClassAllocation[2].Percent)
	}

	// EquityDerivativesByRegion
	if len(details.EquityDerivativesByRegion) != 4 {
		t.Fatalf("expected 4 region entries, got %d", len(details.EquityDerivativesByRegion))
	}
	if details.EquityDerivativesByRegion[0].Region != "North America" {
		t.Errorf("expected North America, got %q", details.EquityDerivativesByRegion[0].Region)
	}
	if details.EquityDerivativesByRegion[3].Region != "Emerging Countries" {
		t.Errorf("expected Emerging Countries, got %q", details.EquityDerivativesByRegion[3].Region)
	}

	// CurrencyDerivativesAllocation
	if len(details.CurrencyDerivativesAllocation) != 3 {
		t.Fatalf("expected 3 currency entries, got %d", len(details.CurrencyDerivativesAllocation))
	}
	if details.CurrencyDerivativesAllocation[0].Currency != "USD" {
		t.Errorf("expected USD, got %q", details.CurrencyDerivativesAllocation[0].Currency)
	}
	if details.CurrencyDerivativesAllocation[2].Currency != "JPY" {
		t.Errorf("expected JPY, got %q", details.CurrencyDerivativesAllocation[2].Currency)
	}

	// ExtractorAsOfDate
	if details.ExtractorAsOfDate.IsZero() {
		t.Error("expected non-zero ExtractorAsOfDate")
	}
	if !details.ExtractorAsOfDate.Equal(asOfDate) {
		t.Errorf("expected ExtractorAsOfDate %v, got %v", asOfDate, details.ExtractorAsOfDate)
	}
}

func TestService_extractResultToSymbolDetails_IMGPOptionalFieldsAbsent(t *testing.T) {
	// Only required fields (FundProfile + AsOfDate), no optional sections
	result := &extractor.ExtractResult{
		AsOfDate: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		FundInfo: &extractor.FundInfo{Name: "iMGP Fund"},
		FundProfile: &extractor.FundProfile{
			Family:         "iM Global Partner",
			TotalNetAssets: 10000000,
			Isin:           "LU1234567890",
		},
		// RiskMeasures, AssetClassAllocation, etc. all nil
	}

	details := extractResultToSymbolDetails(result, "IMGPFUND")

	if details.RiskMeasures != nil {
		t.Error("expected nil RiskMeasures")
	}
	if len(details.AssetClassAllocation) != 0 {
		t.Errorf("expected empty AssetClassAllocation, got %d", len(details.AssetClassAllocation))
	}
	if len(details.EquityDerivativesByRegion) != 0 {
		t.Errorf("expected empty EquityDerivativesByRegion, got %d", len(details.EquityDerivativesByRegion))
	}
	if len(details.CurrencyDerivativesAllocation) != 0 {
		t.Errorf("expected empty CurrencyDerivativesAllocation, got %d", len(details.CurrencyDerivativesAllocation))
	}
	// FundProfile should still be populated
	if details.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if details.FundProfile.Isin != "LU1234567890" {
		t.Errorf("expected Isin LU1234567890, got %q", details.FundProfile.Isin)
	}
}

func TestService_extractResultToSymbolDetails_VanguardFields(t *testing.T) {
	couponRate := 3.25
	finalMaturity := "2035-06-15"
	asOfDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	result := &extractor.ExtractResult{
		AsOfDate: asOfDate,
		FundInfo: &extractor.FundInfo{
			Symbol: "VWRL.L",
			Name:   "Vanguard FTSE All-World UCITS ETF",
		},
		Holdings: []extractor.Holding{
			{
				Symbol:        "AAPL",
				Name:          "Apple Inc",
				Percent:       0.5,
				SecurityType:  "Common Stock",
				CouponRate:    nil,
				FinalMaturity: nil,
				AsOfDate:      "2024-12-31",
			},
			{
				Symbol:        "US-10Y",
				Name:          "US Treasury 10Y",
				Percent:       0.3,
				SecurityType:  "Government Bond",
				CouponRate:    &couponRate,
				FinalMaturity: &finalMaturity,
				AsOfDate:      "2024-12-31",
			},
		},
		Sectors: []extractor.SectorWeighting{
			{Sector: "technology", Percent: 25.5, Date: "2024-12-31"},
			{Sector: "financials", Percent: 15.2, Date: "2024-12-31"},
		},
		CountryAllocation: []extractor.CountryAllocation{
			{Country: "United States", Percent: 60.0, RegionName: "Developed Markets", RegionCode: "DEV", Date: "2024-12-31"},
			{Country: "China", Percent: 5.0, RegionName: "Emerging Markets", RegionCode: "EM", Date: "2024-12-31"},
		},
		Characteristics: &extractor.FundCharacteristics{
			PriceToEarnings:          20.5,
			EstimatedPriceToEarnings: 18.0,
			PriceToBook:              3.5,
			MedianMarketCap:          150.0,
			ForwardROE:               15.2,
			ForwardEPSGrowth:         10.5,
			RevenueRatio:             1.08,
			FieldsPresent: extractor.CharacteristicPriceToEarnings |
				extractor.CharacteristicMedianMarketCap |
				extractor.CharacteristicForwardROE |
				extractor.CharacteristicForwardEPSGrowth |
				extractor.CharacteristicRevenueRatio,
		},
	}

	details := extractResultToSymbolDetails(result, "VWRL.L")

	// Holdings — new fields
	if len(details.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(details.TopHoldings))
	}
	if details.TopHoldings[0].SecurityType != "Common Stock" {
		t.Errorf("expected SecurityType 'Common Stock', got %q", details.TopHoldings[0].SecurityType)
	}
	if details.TopHoldings[0].CouponRate != nil {
		t.Errorf("expected nil CouponRate for stock, got %v", *details.TopHoldings[0].CouponRate)
	}
	if details.TopHoldings[0].FinalMaturity != nil {
		t.Errorf("expected nil FinalMaturity for stock, got %v", *details.TopHoldings[0].FinalMaturity)
	}
	if details.TopHoldings[0].AsOfDate != "2024-12-31" {
		t.Errorf("expected AsOfDate '2024-12-31', got %q", details.TopHoldings[0].AsOfDate)
	}
	if details.TopHoldings[1].SecurityType != "Government Bond" {
		t.Errorf("expected SecurityType 'Government Bond', got %q", details.TopHoldings[1].SecurityType)
	}
	if details.TopHoldings[1].CouponRate == nil || *details.TopHoldings[1].CouponRate != 3.25 {
		t.Errorf("expected CouponRate 3.25, got %v", details.TopHoldings[1].CouponRate)
	}
	if details.TopHoldings[1].FinalMaturity == nil || *details.TopHoldings[1].FinalMaturity != "2035-06-15" {
		t.Errorf("expected FinalMaturity '2035-06-15', got %v", details.TopHoldings[1].FinalMaturity)
	}

	// Sectors — Date field
	if details.SectorWeightings[0].Date != "2024-12-31" {
		t.Errorf("expected sector Date '2024-12-31', got %q", details.SectorWeightings[0].Date)
	}
	if details.SectorWeightings[1].Sector != "financials" || details.SectorWeightings[1].Percent != 15.2 {
		t.Errorf("expected second sector financials 15.2, got %s %f", details.SectorWeightings[1].Sector, details.SectorWeightings[1].Percent)
	}

	// Countries — RegionName, RegionCode, Date
	if details.GeographicAllocations[0].RegionName != "Developed Markets" {
		t.Errorf("expected RegionName 'Developed Markets', got %q", details.GeographicAllocations[0].RegionName)
	}
	if details.GeographicAllocations[0].RegionCode != "DEV" {
		t.Errorf("expected RegionCode 'DEV', got %q", details.GeographicAllocations[0].RegionCode)
	}
	if details.GeographicAllocations[0].Date != "2024-12-31" {
		t.Errorf("expected Date '2024-12-31', got %q", details.GeographicAllocations[0].Date)
	}
	if details.GeographicAllocations[1].RegionName != "Emerging Markets" {
		t.Errorf("expected RegionName 'Emerging Markets', got %q", details.GeographicAllocations[1].RegionName)
	}

	// Equity valuation — new fields
	if details.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation")
	}
	if details.EquityValuation.MedianMarketCap != 150.0 {
		t.Errorf("expected MedianMarketCap 150.0, got %f", details.EquityValuation.MedianMarketCap)
	}
	if details.EquityValuation.ForwardROE != 15.2 {
		t.Errorf("expected ForwardROE 15.2, got %f", details.EquityValuation.ForwardROE)
	}
	if details.EquityValuation.ForwardEPSGrowth != 10.5 {
		t.Errorf("expected ForwardEPSGrowth 10.5, got %f", details.EquityValuation.ForwardEPSGrowth)
	}
	if details.EquityValuation.RevenueRatio != 1.08 {
		t.Errorf("expected RevenueRatio 1.08, got %f", details.EquityValuation.RevenueRatio)
	}

	// Bond characteristics — nil for equity fund (no bond fields set)
	if details.BondCharacteristics != nil {
		t.Error("expected nil BondCharacteristics for equity fund")
	}
}

func TestService_extractResultToSymbolDetails_BlackRockFields(t *testing.T) {
	result := &extractor.ExtractResult{
		AsOfDate: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		FundInfo: &extractor.FundInfo{
			Symbol: "ISUS.L",
			Name:   "iShares Core USD Total Bond Market UCITS ETF USD (Acc)",
		},
		FundProfile: &extractor.FundProfile{
			Family:               "iShares",
			LegalType:            "Exchange Traded Fund",
			TotalNetAssets:       25000000000,
			AnnualExpenseRatio:   0.08,
			InceptionDate:        time.Date(2012, 9, 25, 0, 0, 0, 0, time.UTC),
			Isin:                 "IE00B53HDB03",
			Benchmark:            "Bloomberg US Universal Treasury Index",
			AssetClassification:  "Fixed Income",
			DistributionStrategy: "ACUM",
			SFDRClassification:   "Article 6",
			Domicile:             "Ireland",
			RebalanceFrequency:   "Quarterly",
			ProductStructure:     "Physical",
			Methodology:          "Representative",
			FundManager:          "BlackRock Asset Management Ireland Limited",
			Custodian:            "State Street Custodial Services (Ireland) Limited",
			IssuingCompany:       "iShares IV plc",
			BenchmarkTicker:      "LBU000IW Index",
		},
		Holdings: []extractor.Holding{
			{
				Symbol:         "US-10Y",
				Name:           "US Treasury 10Y",
				Percent:        18.5,
				SecurityType:   "Government Bond",
				Sector:         "Government",
				AssetClass:     "Fixed Income",
				MarketValue:    4625000000,
				NotionalValue:  4700000000,
				Shares:         0,
				Price:          98.5,
				ISIN:           "912828ZT0",
				Location:       "United States",
				Exchange:       "OTC",
				MarketCurrency: "USD",
			},
			{
				Symbol:         "AAPL",
				Name:           "Apple Inc",
				Percent:        0.5,
				SecurityType:   "Common Stock",
				Sector:         "Information Technology",
				AssetClass:     "Equity",
				MarketValue:    125000000,
				NotionalValue:  125000000,
				Shares:         625000,
				Price:          200.0,
				ISIN:           "037833100",
				Location:       "United States",
				Exchange:       "NASDAQ",
				MarketCurrency: "USD",
			},
			{
				Symbol:         "CASH",
				Name:           "Cash",
				Percent:        0.2,
				Sector:         "",
				AssetClass:     "Cash",
				MarketValue:    50000000,
				NotionalValue:  50000000,
				Shares:         0,
				Price:          0,
				ISIN:           "-",
				Location:       "",
				Exchange:       "",
				MarketCurrency: "USD",
			},
		},
		Sectors: []extractor.SectorWeighting{
			{Sector: "Government", Percent: 85.0},
			{Sector: "Corporate", Percent: 12.0},
			{Sector: "Agency", Percent: 3.0},
		},
		CountryAllocation: []extractor.CountryAllocation{
			{Country: "United States", Percent: 98.5},
			{Country: "Cash", Percent: 1.5},
		},
		Characteristics: &extractor.FundCharacteristics{
			PriceToEarnings:     0,
			AverageCoupon:       4.25,
			AverageMaturity:     8.5,
			AverageQuality:      7.8,
			AverageDuration:     6.2,
			Beta3Y:              0.02,
			StandardDeviation3Y: 5.8,
			NumberOfHoldings:    8542,
			FieldsPresent: extractor.CharacteristicAverageCoupon |
				extractor.CharacteristicAverageMaturity |
				extractor.CharacteristicAverageQuality |
				extractor.CharacteristicAverageDuration |
				extractor.CharacteristicBeta3Y |
				extractor.CharacteristicStandardDeviation3Y |
				extractor.CharacteristicNumberOfHoldings,
		},
	}

	details := extractResultToSymbolDetails(result, "ISUS.L")

	// FundProfile — BlackRock-specific fields
	if details.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if details.FundProfile.SFDRClassification != "Article 6" {
		t.Errorf("expected SFDRClassification 'Article 6', got %q", details.FundProfile.SFDRClassification)
	}
	if details.FundProfile.Domicile != "Ireland" {
		t.Errorf("expected Domicile 'Ireland', got %q", details.FundProfile.Domicile)
	}
	if details.FundProfile.RebalanceFrequency != "Quarterly" {
		t.Errorf("expected RebalanceFrequency 'Quarterly', got %q", details.FundProfile.RebalanceFrequency)
	}
	if details.FundProfile.ProductStructure != "Physical" {
		t.Errorf("expected ProductStructure 'Physical', got %q", details.FundProfile.ProductStructure)
	}
	if details.FundProfile.Methodology != "Representative" {
		t.Errorf("expected Methodology 'Representative', got %q", details.FundProfile.Methodology)
	}
	if details.FundProfile.FundManager != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("expected FundManager, got %q", details.FundProfile.FundManager)
	}
	if details.FundProfile.Custodian != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("expected Custodian, got %q", details.FundProfile.Custodian)
	}
	if details.FundProfile.IssuingCompany != "iShares IV plc" {
		t.Errorf("expected IssuingCompany 'iShares IV plc', got %q", details.FundProfile.IssuingCompany)
	}
	if details.FundProfile.BenchmarkTicker != "LBU000IW Index" {
		t.Errorf("expected BenchmarkTicker, got %q", details.FundProfile.BenchmarkTicker)
	}

	// Holdings — BlackRock-specific fields
	if len(details.TopHoldings) != 3 {
		t.Fatalf("expected 3 holdings, got %d", len(details.TopHoldings))
	}
	// First holding (bond)
	h0 := details.TopHoldings[0]
	if h0.Sector != "Government" {
		t.Errorf("expected Sector 'Government', got %q", h0.Sector)
	}
	if h0.AssetClass != "Fixed Income" {
		t.Errorf("expected AssetClass 'Fixed Income', got %q", h0.AssetClass)
	}
	if h0.MarketValue != 4625000000 {
		t.Errorf("expected MarketValue 4625000000, got %f", h0.MarketValue)
	}
	if h0.NotionalValue != 4700000000 {
		t.Errorf("expected NotionalValue 4700000000, got %f", h0.NotionalValue)
	}
	if h0.Price != 98.5 {
		t.Errorf("expected Price 98.5, got %f", h0.Price)
	}
	if h0.ISIN != "912828ZT0" {
		t.Errorf("expected ISIN '912828ZT0', got %q", h0.ISIN)
	}
	if h0.Location != "United States" {
		t.Errorf("expected Location 'United States', got %q", h0.Location)
	}
	if h0.Exchange != "OTC" {
		t.Errorf("expected Exchange 'OTC', got %q", h0.Exchange)
	}
	if h0.MarketCurrency != "USD" {
		t.Errorf("expected MarketCurrency 'USD', got %q", h0.MarketCurrency)
	}
	// Second holding (equity)
	h1 := details.TopHoldings[1]
	if h1.Shares != 625000 {
		t.Errorf("expected Shares 625000, got %f", h1.Shares)
	}
	if h1.Exchange != "NASDAQ" {
		t.Errorf("expected Exchange 'NASDAQ', got %q", h1.Exchange)
	}
	// Third holding (cash)
	h2 := details.TopHoldings[2]
	if h2.ISIN != "-" {
		t.Errorf("expected ISIN '-' for cash, got %q", h2.ISIN)
	}
	if h2.AssetClass != "Cash" {
		t.Errorf("expected AssetClass 'Cash', got %q", h2.AssetClass)
	}

	// Bond characteristics — populated (bond fund)
	if details.BondCharacteristics == nil {
		t.Fatal("expected non-nil BondCharacteristics for bond fund")
	}
	if details.BondCharacteristics.AverageCoupon != 4.25 {
		t.Errorf("expected AverageCoupon 4.25, got %f", details.BondCharacteristics.AverageCoupon)
	}
	if details.BondCharacteristics.AverageDuration != 6.2 {
		t.Errorf("expected AverageDuration 6.2, got %f", details.BondCharacteristics.AverageDuration)
	}

	// Equity valuation — nil for bond fund (no equity fields present)
	if details.EquityValuation != nil {
		t.Errorf("expected nil EquityValuation for bond fund, got %+v", details.EquityValuation)
	}

	// Sectors and country allocation
	if len(details.SectorWeightings) != 3 {
		t.Errorf("expected 3 sectors, got %d", len(details.SectorWeightings))
	}
	if len(details.GeographicAllocations) != 2 {
		t.Errorf("expected 2 countries, got %d", len(details.GeographicAllocations))
	}
}

func TestService_extractResultToSymbolDetails_BlackRockEquityFund(t *testing.T) {
	result := &extractor.ExtractResult{
		AsOfDate: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		FundInfo: &extractor.FundInfo{
			Symbol: "ISF.L",
			Name:   "iShares FTSE 100 UCITS ETF GBP (Dist)",
		},
		FundProfile: &extractor.FundProfile{
			Family:               "iShares",
			LegalType:            "Exchange Traded Fund",
			TotalNetAssets:       5000000000,
			AnnualExpenseRatio:   0.07,
			InceptionDate:        time.Date(2000, 5, 3, 0, 0, 0, 0, time.UTC),
			Isin:                 "IE00B4K4B820",
			Benchmark:            "FTSE 100 Total Return Index",
			AssetClassification:  "Equity",
			DistributionStrategy: "INCM",
			MarketRegionFocus:    "UK",
			SFDRClassification:   "Article 6",
			Domicile:             "Ireland",
			RebalanceFrequency:   "Quarterly",
			ProductStructure:     "Physical",
			Methodology:          "Representative",
			FundManager:          "BlackRock Asset Management Ireland Limited",
			Custodian:            "State Street Custodial Services (Ireland) Limited",
			IssuingCompany:       "iShares IV plc",
			BenchmarkTicker:      "XFLT10 Index",
		},
		Holdings: []extractor.Holding{
			{
				Symbol:         "AZN.L",
				Name:           "AstraZeneca PLC",
				Percent:        5.2,
				SecurityType:   "Common Stock",
				Sector:         "Health Care",
				AssetClass:     "Equity",
				MarketValue:    260000000,
				NotionalValue:  260000000,
				Shares:         1200000,
				Price:          216.67,
				ISIN:           "GB0009895292",
				Location:       "United Kingdom",
				Exchange:       "LSE",
				MarketCurrency: "GBP",
			},
		},
		Sectors: []extractor.SectorWeighting{
			{Sector: "Health Care", Percent: 18.5},
			{Sector: "Financials", Percent: 16.2},
			{Sector: "Consumer Staples", Percent: 14.8},
		},
		CountryAllocation: []extractor.CountryAllocation{
			{Country: "United Kingdom", Percent: 100.0},
		},
		Characteristics: &extractor.FundCharacteristics{
			PriceToEarnings:          12.5,
			EstimatedPriceToEarnings: 11.8,
			PriceToBook:              1.9,
			PriceToCashflow:          9.2,
			DividendYield:            3.8,
			Beta3Y:                   0.95,
			StandardDeviation3Y:      16.2,
			NumberOfHoldings:         105,
			FieldsPresent: extractor.CharacteristicPriceToEarnings |
				extractor.CharacteristicEstimatedPriceToEarnings |
				extractor.CharacteristicPriceToBook |
				extractor.CharacteristicPriceToCashflow |
				extractor.CharacteristicDividendYield |
				extractor.CharacteristicBeta3Y |
				extractor.CharacteristicStandardDeviation3Y |
				extractor.CharacteristicNumberOfHoldings,
		},
	}

	details := extractResultToSymbolDetails(result, "ISF.L")

	// FundProfile — BlackRock-specific fields
	if details.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if details.FundProfile.SFDRClassification != "Article 6" {
		t.Errorf("expected SFDRClassification 'Article 6', got %q", details.FundProfile.SFDRClassification)
	}
	if details.FundProfile.Domicile != "Ireland" {
		t.Errorf("expected Domicile 'Ireland', got %q", details.FundProfile.Domicile)
	}
	if details.FundProfile.BenchmarkTicker != "XFLT10 Index" {
		t.Errorf("expected BenchmarkTicker, got %q", details.FundProfile.BenchmarkTicker)
	}
	// Existing fields still work
	if details.FundProfile.AssetClassification != "Equity" {
		t.Errorf("expected AssetClassification 'Equity', got %q", details.FundProfile.AssetClassification)
	}
	if details.FundProfile.DistributionStrategy != "INCM" {
		t.Errorf("expected DistributionStrategy 'INCM', got %q", details.FundProfile.DistributionStrategy)
	}

	// Equity valuation — populated for equity fund (P/E present)
	if details.EquityValuation == nil {
		t.Fatal("expected non-nil EquityValuation for equity fund")
	}
	if details.EquityValuation.PriceToEarnings != 12.5 {
		t.Errorf("expected P/E 12.5, got %f", details.EquityValuation.PriceToEarnings)
	}
	if details.EquityValuation.Beta3Y != 0.95 {
		t.Errorf("expected Beta3Y 0.95, got %f", details.EquityValuation.Beta3Y)
	}
	if details.EquityValuation.StandardDeviation3Y != 16.2 {
		t.Errorf("expected StandardDeviation3Y 16.2, got %f", details.EquityValuation.StandardDeviation3Y)
	}
	if details.EquityValuation.NumberOfHoldings != 105 {
		t.Errorf("expected NumberOfHoldings 105, got %d", details.EquityValuation.NumberOfHoldings)
	}
	// Existing fields still work
	if details.EquityValuation.EstimatedPriceToEarnings != 11.8 {
		t.Errorf("expected Estimated P/E 11.8, got %f", details.EquityValuation.EstimatedPriceToEarnings)
	}
	if details.EquityValuation.DividendYield != 3.8 {
		t.Errorf("expected DividendYield 3.8, got %f", details.EquityValuation.DividendYield)
	}

	// Holdings — BlackRock-specific fields
	if len(details.TopHoldings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(details.TopHoldings))
	}
	h := details.TopHoldings[0]
	if h.Sector != "Health Care" {
		t.Errorf("expected Sector 'Health Care', got %q", h.Sector)
	}
	if h.AssetClass != "Equity" {
		t.Errorf("expected AssetClass 'Equity', got %q", h.AssetClass)
	}
	if h.Shares != 1200000 {
		t.Errorf("expected Shares 1200000, got %f", h.Shares)
	}
	if h.MarketValue != 260000000 {
		t.Errorf("expected MarketValue 260000000, got %f", h.MarketValue)
	}
	if h.ISIN != "GB0009895292" {
		t.Errorf("expected ISIN 'GB0009895292', got %q", h.ISIN)
	}
	if h.Location != "United Kingdom" {
		t.Errorf("expected Location 'United Kingdom', got %q", h.Location)
	}
	if h.Exchange != "LSE" {
		t.Errorf("expected Exchange 'LSE', got %q", h.Exchange)
	}
	if h.MarketCurrency != "GBP" {
		t.Errorf("expected MarketCurrency 'GBP', got %q", h.MarketCurrency)
	}
	// Existing fields still work
	if h.Symbol != "AZN.L" {
		t.Errorf("expected Symbol 'AZN.L', got %q", h.Symbol)
	}
	if h.Percent != 5.2 {
		t.Errorf("expected Percent 5.2, got %f", h.Percent)
	}

	// Bond characteristics — nil for equity fund
	if details.BondCharacteristics != nil {
		t.Errorf("expected nil BondCharacteristics for equity fund, got %+v", details.BondCharacteristics)
	}
}

func TestService_extractResultToSymbolDetails_BondFundCharacteristics(t *testing.T) {
	result := &extractor.ExtractResult{
		AsOfDate: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
		FundInfo: &extractor.FundInfo{
			Symbol: "VAGT.L",
			Name:   "Vanguard Aggregate Bond UCITS ETF",
		},
		Characteristics: &extractor.FundCharacteristics{
			AverageCoupon:   3.5,
			AverageMaturity: 8.2,
			AverageQuality:  7.5,
			AverageDuration: 6.1,
			FieldsPresent: extractor.CharacteristicAverageCoupon |
				extractor.CharacteristicAverageMaturity |
				extractor.CharacteristicAverageQuality |
				extractor.CharacteristicAverageDuration,
		},
	}

	details := extractResultToSymbolDetails(result, "VAGT.L")

	// Equity valuation — nil for bond fund (no equity fields present)
	if details.EquityValuation != nil {
		t.Errorf("expected nil EquityValuation for bond fund, got %+v", details.EquityValuation)
	}

	// Bond characteristics — populated
	if details.BondCharacteristics == nil {
		t.Fatal("expected non-nil BondCharacteristics for bond fund")
	}
	if details.BondCharacteristics.AverageCoupon != 3.5 {
		t.Errorf("expected AverageCoupon 3.5, got %f", details.BondCharacteristics.AverageCoupon)
	}
	if details.BondCharacteristics.AverageMaturity != 8.2 {
		t.Errorf("expected AverageMaturity 8.2, got %f", details.BondCharacteristics.AverageMaturity)
	}
	if details.BondCharacteristics.AverageQuality != 7.5 {
		t.Errorf("expected AverageQuality 7.5, got %f", details.BondCharacteristics.AverageQuality)
	}
	if details.BondCharacteristics.AverageDuration != 6.1 {
		t.Errorf("expected AverageDuration 6.1, got %f", details.BondCharacteristics.AverageDuration)
	}
}

func TestService_extractResultToSymbolDetails_EmptyHoldings(t *testing.T) {
	result := &extractor.ExtractResult{
		FundInfo: &extractor.FundInfo{Name: "Empty Fund"},
		Holdings: []extractor.Holding{},
	}

	details := extractResultToSymbolDetails(result, "EMPTY")

	// Empty holdings should map to nil slice (not created by the if len > 0 guard)
	if details.TopHoldings != nil {
		t.Errorf("expected nil TopHoldings for empty input, got %d items", len(details.TopHoldings))
	}
}

func TestService_extractResultToSymbolDetails_EmptyResult(t *testing.T) {
	result := &extractor.ExtractResult{}

	details := extractResultToSymbolDetails(result, "WMGG.L")

	if details.InternalSymbol != "WMGG.L" {
		t.Errorf("expected internal symbol 'WMGG.L', got %q", details.InternalSymbol)
	}
	if details.ShortName != "" {
		t.Errorf("expected empty short name, got %q", details.ShortName)
	}
	if len(details.TopHoldings) != 0 {
		t.Errorf("expected empty holdings, got %d", len(details.TopHoldings))
	}
	if details.FundProfile != nil {
		t.Error("expected nil FundProfile")
	}
	if details.EquityValuation != nil {
		t.Error("expected nil EquityValuation")
	}
}

// --- GetByInternalSymbol Tests ---

func TestService_GetByInternalSymbol_Success(t *testing.T) {
	svc, repo, _ := newTestService()

	repo.details["VOO"] = &symbol.SymbolDetails{
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

	repo.stale = []symbol.StaleSymbol{
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

	repo.stale = []symbol.StaleSymbol{}

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

func TestService_GetStaleSymbols_ExcludesCash(t *testing.T) {
	svc, repo, _ := newTestService()

	repo.stale = []symbol.StaleSymbol{
		{InternalSymbol: "$CASH-GBP", MarketDataSymbol: "GBPUSD=X", FetchedAt: time.Now().AddDate(0, 0, -8)},
		{InternalSymbol: "VOO", MarketDataSymbol: "VOO", FetchedAt: time.Now().AddDate(0, 0, -8)},
	}

	stale, err := svc.GetStaleSymbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale symbol (cash excluded), got %d", len(stale))
	}
	if stale[0].InternalSymbol != "VOO" {
		t.Errorf("expected stale 'VOO', got %q", stale[0].InternalSymbol)
	}
}

// --- RefreshSymbol Tests ---

func TestService_RefreshSymbol_Success(t *testing.T) {
	svc, repo, fetcher := newTestService()

	// Initially stale data
	repo.details["VOO"] = &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Old Name",
		FetchedAt:      time.Now().AddDate(0, 0, -8),
	}

	// Fetcher returns fresh data
	fetcher.details = &symbol.SymbolDetails{
		ShortName: "Updated Name",
		Exchange:  "PCX",
		Currency:  "USD",
		QuoteType: "ETF",
		FetchedAt: time.Now(),
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
	repo.details["VOO"] = &symbol.SymbolDetails{
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

func TestService_TouchFetchedAt_Success(t *testing.T) {
	svc, repo, _ := newTestService()

	// Insert existing details
	repo.details["VOO"] = &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500",
		FetchedAt:      time.Now().AddDate(0, 0, -8),
	}

	err := svc.TouchFetchedAt(context.Background(), "VOO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_TouchFetchedAt_Error(t *testing.T) {
	repo := newMockRepo()
	repo.err = fmt.Errorf("db error")
	fetcher := &mockFetcher{details: &symbol.SymbolDetails{InternalSymbol: "VOO"}}
	svc := NewService(repo, fetcher)

	err := svc.TouchFetchedAt(context.Background(), "VOO")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Decimal conversion test ---

func TestService_storeNavHistory_DecimalConversion(t *testing.T) {
	svc, _, _ := newTestService()

	marketDataRepo := newMockMarketDataRepo()
	svc.WithMarketDataRepo(marketDataRepo)

	navPoints := []extractor.NavPoint{
		{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(4520, 2)},
		{Date: time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(100005, 3)},
	}

	err := svc.storeNavHistory(context.Background(), "WMGG.L", "GBP", navPoints, "wisdomtree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(marketDataRepo.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(marketDataRepo.entries))
	}

	// Verify decimal conversion
	expected1 := decimal.MustNew(4520, 2)
	if !marketDataRepo.entries[0].Price.Equal(expected1) {
		t.Errorf("expected price %s, got %s", expected1.String(), marketDataRepo.entries[0].Price.String())
	}

	expected2 := decimal.MustParse("100.005")
	if !marketDataRepo.entries[1].Price.Equal(expected2) {
		t.Errorf("expected price %s, got %s", expected2.String(), marketDataRepo.entries[1].Price.String())
	}

	// Verify currency
	if marketDataRepo.entries[0].Currency != "GBP" {
		t.Errorf("expected currency 'GBP', got %q", marketDataRepo.entries[0].Currency)
	}
}

// --- NAV Currency Matching Test ---

func TestService_storeNavHistory_CurrencyFromNavPoint(t *testing.T) {
	svc, _, _ := newTestService()

	marketDataRepo := newMockMarketDataRepo()
	svc.WithMarketDataRepo(marketDataRepo)

	// NAV points carry their own currency (e.g. Vanguard returns NAV in
	// the fund's base currency USD, even though the symbol lists in GBP)
	navPoints := []extractor.NavPoint{
		{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(10500, 2), Currency: "USD"},
		{Date: time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(10550, 2), Currency: "USD"},
		{Date: time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC), NAV: decimal.MustNew(10600, 2)}, // no currency — should fallback
	}

	// Symbol currency is GBP (from Yahoo Finance listing)
	err := svc.storeNavHistory(context.Background(), "VWRP.L", "GBP", navPoints, "vanguard")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(marketDataRepo.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(marketDataRepo.entries))
	}

	// Points with their own currency should use it
	if marketDataRepo.entries[0].Currency != "USD" {
		t.Errorf("entry 0: expected currency 'USD', got %q", marketDataRepo.entries[0].Currency)
	}
	if marketDataRepo.entries[1].Currency != "USD" {
		t.Errorf("entry 1: expected currency 'USD', got %q", marketDataRepo.entries[1].Currency)
	}

	// Point without currency should fallback to symbol currency
	if marketDataRepo.entries[2].Currency != "GBP" {
		t.Errorf("entry 2: expected fallback currency 'GBP', got %q", marketDataRepo.entries[2].Currency)
	}
}
