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
	urls map[string]string    // internal_symbol -> data_source_url
	ids  map[string]int64     // internal_symbol -> id
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
	name     string
	result   *extractor.ExtractResult
	err      error
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

	fetcher.details = &symbol.SymbolDetails{
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
	fetcher.details = &symbol.SymbolDetails{
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

// --- Extractor Routing Tests ---

func TestService_FetchAndStore_RoutesToExtractor_WhenURLSet(t *testing.T) {
	svc, repo, _ := newTestService()

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
				PriceToEarnings: 25.3,
				PriceToBook:     4.2,
			},
			NavHistory: []extractor.NavPoint{
				{Date: "2024-01-15", NAV: 45.20},
				{Date: "2024-01-16", NAV: 45.50},
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
	if details.ExtractorAsOfDate.IsZero() {
		t.Error("expected non-zero ExtractorAsOfDate")
	} else if !details.ExtractorAsOfDate.Equal(time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected ExtractorAsOfDate 2024-03-29, got %v", details.ExtractorAsOfDate)
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
	svc, repo, _ := newTestService()

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
	svc, repo, _ := newTestService()

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
	svc, _, _ := newTestService()

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			FundInfo: &extractor.FundInfo{Name: "Test Fund"},
			NavHistory: []extractor.NavPoint{
				{Date: "2024-01-15", NAV: 45.20},
				{Date: "2024-01-16", NAV: 45.50},
				{Date: "2024-01-17", NAV: 46.00},
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
	svc, repo, _ := newTestService()

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			FundInfo: &extractor.FundInfo{Name: "Test Fund"},
			NavHistory: []extractor.NavPoint{
				{Date: "2024-01-15", NAV: 45.20},
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
	svc, _, _ := newTestService()

	reg := extractor.NewRegistry()
	mockExt := &mockExtractor{
		name: "wisdomtree",
		result: &extractor.ExtractResult{
			FundInfo: &extractor.FundInfo{Name: "Test Fund"},
			NavHistory: []extractor.NavPoint{
				{Date: "2024-01-15", NAV: 45.20},
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
		name       string
		symbol     string
		setupRepo  func() DataSourceURLSource
		wantURL    string
		wantErr    bool
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
			name:       "empty when no repo",
			symbol:     "VOO",
			setupRepo:  nil,
			wantURL:    "",
			wantErr:    false,
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
			PriceToEarnings: 25.3,
			PriceToBook:     4.2,
			PriceToCashflow: 18.0,
			PriceToSales:    5.5,
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

// --- Decimal conversion test ---

func TestService_storeNavHistory_DecimalConversion(t *testing.T) {
	svc, _, _ := newTestService()

	marketDataRepo := newMockMarketDataRepo()
	svc.WithMarketDataRepo(marketDataRepo)

	navPoints := []extractor.NavPoint{
		{Date: "2024-01-15", NAV: 45.20},
		{Date: "2024-01-16", NAV: 100.005},
	}

	err := svc.storeNavHistory(context.Background(), "WMGG.L", "GBP", navPoints)
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
