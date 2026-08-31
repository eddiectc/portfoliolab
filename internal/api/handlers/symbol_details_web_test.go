package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// --- Mocks ---

type detailsWebTestRepo struct {
	details           map[string]*symbol.SymbolDetails
	touchFetchedAt    bool
	touchFetchedAtSym string
}

func newDetailsWebTestRepo() *detailsWebTestRepo {
	return &detailsWebTestRepo{details: make(map[string]*symbol.SymbolDetails)}
}

func (r *detailsWebTestRepo) Upsert(_ context.Context, d *symbol.SymbolDetails) error {
	r.details[d.InternalSymbol] = d
	return nil
}

func (r *detailsWebTestRepo) GetByInternalSymbol(_ context.Context, s string) (*symbol.SymbolDetails, error) {
	d, ok := r.details[s]
	if !ok {
		return nil, symbols.ErrNotFound
	}
	cp := *d
	return &cp, nil
}

func (r *detailsWebTestRepo) ListStale(_ context.Context, _ time.Time) ([]symbol.StaleSymbol, error) {
	return nil, nil
}

func (r *detailsWebTestRepo) TouchFetchedAt(_ context.Context, sym string) error {
	r.touchFetchedAt = true
	r.touchFetchedAtSym = sym
	return nil
}

type detailsWebTestFetcher struct {
	details *symbol.SymbolDetails
	err     error
}

func (f *detailsWebTestFetcher) FetchSymbolDetails(_ context.Context, _ string) (*symbol.SymbolDetails, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.details == nil {
		return nil, nil
	}
	cp := *f.details
	return &cp, nil
}

type detailsWebQuoteFetcher struct {
	quote *market.MarketData
	err   error
}

func (f *detailsWebQuoteFetcher) FetchQuote(_ context.Context, _ string) (*market.MarketData, error) {
	return f.quote, f.err
}

func (f *detailsWebQuoteFetcher) FetchFxRate(_ context.Context, _, _ string) (*market.MarketData, error) {
	return nil, nil
}

func (f *detailsWebQuoteFetcher) FetchQuotesBatch(_ context.Context, _ []string) map[string]*market.MarketData {
	return nil
}

func (f *detailsWebQuoteFetcher) FetchHistoricalPricesBatch(_ context.Context, _ []string, _, _ time.Time) (map[string][]market.HistoricalPrice, []string) {
	return nil, nil
}

type detailsWebNavSource struct {
	navPrices      []market.HistoricalPrice
	stockPrices    []market.HistoricalPrice
	navErr         error
	stockPricesErr error
}

func (s *detailsWebNavSource) GetNavHistoryBySymbol(_ context.Context, _ string) ([]market.HistoricalPrice, error) {
	return s.navPrices, s.navErr
}

func (s *detailsWebNavSource) GetHistoricalPricesBySymbol(_ context.Context, _ string, _, _ time.Time) ([]market.HistoricalPrice, error) {
	return s.stockPrices, s.stockPricesErr
}

// --- Setup ---

func setupDetailsWebHandler(t *testing.T) (*SymbolDetailsWebHandler, *symbolmapping.Service, *symbols.Service, *testSMWebRepo, *detailsWebTestRepo, *detailsWebQuoteFetcher, *detailsWebNavSource) {
	t.Helper()
	smRepo := newTestSMWebRepo()
	smSvc := symbolmapping.NewService(smRepo)
	detailsRepo := newDetailsWebTestRepo()
	detailsFetcher := &detailsWebTestFetcher{}
	detailsSvc := symbols.NewService(detailsRepo, detailsFetcher)
	quoteFetcher := &detailsWebQuoteFetcher{}
	navSource := &detailsWebNavSource{}
	renderer := newTestRenderer(t)
	handler := NewSymbolDetailsWebHandler(smSvc, detailsSvc, quoteFetcher, navSource, renderer)
	return handler, smSvc, detailsSvc, smRepo, detailsRepo, quoteFetcher, navSource
}

// --- HandleDetailsPage Tests ---

func TestDetailsHandleDetailsPage_WithDetails(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, quoteFetcher, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VOO",
		MarketDataSymbol: "VOO",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VOO"] = 1

	detailsRepo.details["VOO"] = &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500 ETF",
		LongName:       "Vanguard S&P 500 ETF",
		Exchange:       "NYSE",
		Currency:       "USD",
		QuoteType:      "ETF",
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 7},
			{Symbol: "MSFT", Name: "Microsoft Corp.", Percent: 6},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 30},
			{Sector: "financials", Percent: 13},
		},
		AggregatePositions: &symbol.AggregatePositions{
			Stock: 0.995,
			Cash:  0.005,
		},
		FundProfile: &symbol.FundProfile{
			Family:             "Vanguard",
			LegalType:          "Exchange Traded Fund",
			TotalNetAssets:     1e11,
			AnnualExpenseRatio: 0.0003,
		},
		FetchedAt: time.Now().Add(-2 * time.Hour),
	}

	price, _ := decimal.NewFromFloat64(450.50)
	quoteFetcher.quote = &market.MarketData{
		Symbol:   "VOO",
		Price:    price,
		Currency: "USD",
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}
	checkNotContains := func(label, text string) {
		t.Helper()
		if strings.Contains(body, text) {
			t.Errorf("page should not contain %s: %q", label, text)
		}
	}

	checkContains("title", "Symbol Details — VOO")
	checkContains("name", "Vanguard S&amp;P 500 ETF")
	checkContains("exchange", "NYSE")
	checkContains("currency", "USD")
	checkContains("type", "ETF")
	checkContains("live price", "450.5")
	checkContains("holdings header", "Top 10 Holdings")
	checkContains("sector header", "Sector Weightings")
	checkContains("aggregate header", "Aggregate Positions")
	checkContains("fund profile header", "Fund Profile")
	checkContains("family", "Vanguard")
	checkContains("back link", "/symbols")
	checkNotContains("edit link", "/symbols/1/edit")
}

func TestDetailsHandleDetailsPage_NoDetails(t *testing.T) {
	handler, _, _, smRepo, _, quoteFetcher, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["AAPL"] = 1

	// No details in repo

	price, _ := decimal.NewFromFloat64(178.25)
	quoteFetcher.quote = &market.MarketData{
		Symbol:   "AAPL",
		Price:    price,
		Currency: "USD",
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	if !strings.Contains(body, "No symbol details available") {
		t.Error("expected 'No symbol details available' message")
	}
	if strings.Contains(body, "Top 10 Holdings") {
		t.Error("should not show holdings section when no details")
	}
	if !strings.Contains(body, "178.25") {
		t.Error("should still show live price even without cached details")
	}
}

func TestDetailsHandleDetailsPage_NotFound(t *testing.T) {
	handler, _, _, _, _, _, _ := setupDetailsWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	r := httptest.NewRequest(http.MethodGet, "/symbols/999/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestDetailsHandleDetailsPage_InvalidID(t *testing.T) {
	handler, _, _, _, _, _, _ := setupDetailsWebHandler(t)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "not-a-number")
	r := httptest.NewRequest(http.MethodGet, "/symbols/not-a-number/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestDetailsHandleDetailsPage_NoQuoteFetcher(t *testing.T) {
	handler := NewSymbolDetailsWebHandler(nil, nil, nil, nil, newTestRenderer(t))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// With no service configured, the handler returns 500 (not a panic)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	req := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Returns 500 because symbolMappingSvc is nil — no crash/panic
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 (nil service), got %d", w.Code)
	}
}

func TestDetailsHandleDetailsPage_StaleIndicator(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, quoteFetcher, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VOO",
		MarketDataSymbol: "VOO",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VOO"] = 1

	// Details fetched 10 days ago (stale)
	detailsRepo.details["VOO"] = &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500 ETF",
		QuoteType:      "ETF",
		FetchedAt:      time.Now().Add(-10 * 24 * time.Hour),
	}

	price, _ := decimal.NewFromFloat64(450.50)
	quoteFetcher.quote = &market.MarketData{
		Symbol:   "VOO",
		Price:    price,
		Currency: "USD",
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Stale") {
		t.Error("expected stale indicator for details older than 7 days")
	}
	if !strings.Contains(body, "10d ago") {
		t.Error("expected '10d ago' in fetched text")
	}
}

// --- Helper function tests ---

func TestFormatFetchedAt_JustNow(t *testing.T) {
	got := formatFetchedAt(time.Now())
	if got != "Updated just now" {
		t.Errorf("expected 'Updated just now', got %q", got)
	}
}

func TestFormatFetchedAt_Minutes(t *testing.T) {
	got := formatFetchedAt(time.Now().Add(-5 * time.Minute))
	if !strings.Contains(got, "5m ago") {
		t.Errorf("expected '5m ago', got %q", got)
	}
}

func TestFormatFetchedAt_Hours(t *testing.T) {
	got := formatFetchedAt(time.Now().Add(-3 * time.Hour))
	if !strings.Contains(got, "3h ago") {
		t.Errorf("expected '3h ago', got %q", got)
	}
}

func TestFormatFetchedAt_Days(t *testing.T) {
	got := formatFetchedAt(time.Now().Add(-5 * 24 * time.Hour))
	if !strings.Contains(got, "5d ago") {
		t.Errorf("expected '5d ago', got %q", got)
	}
}

func TestFormatLargeNumber(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{1e12, "1.00T"},
		{1.5e12, "1.50T"},
		{1e9, "1.00B"},
		{2.5e9, "2.50B"},
		{1e6, "1.00M"},
		{500e6, "500.00M"},
		{1234.56, "1234.56"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatLargeNumber(tt.input)
			if got != tt.want {
				t.Errorf("formatLargeNumber(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- RegisterRoutes test ---

func TestDetailsRegisterRoutes(t *testing.T) {
	r := chi.NewRouter()
	handler := &SymbolDetailsWebHandler{}
	handler.RegisterRoutes(r)
}

// --- Geographic allocation tests ---

func TestDetailsHandleDetailsPage_GeographicMultiElement(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VWRP.L",
		MarketDataSymbol: "VWRP.L",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VWRP.L"] = 1

	detailsRepo.details["VWRP.L"] = &symbol.SymbolDetails{
		InternalSymbol: "VWRP.L",
		ShortName:      "Vanguard FTSE All-World UCITS ETF",
		QuoteType:      "ETF",
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 60},
			{Country: "United Kingdom", Percent: 5},
			{Country: "Japan", Percent: 4},
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Geographic Allocation") {
		t.Error("expected 'Geographic Allocation' section header")
	}
	if !strings.Contains(body, "United States") {
		t.Error("expected 'United States' in geographic table")
	}
	if !strings.Contains(body, "60.00%") {
		t.Error("expected '60.00%' for United States")
	}
	if !strings.Contains(body, "United Kingdom") {
		t.Error("expected 'United Kingdom' in geographic table")
	}
	if !strings.Contains(body, "5.00%") {
		t.Error("expected '5.00%' for United Kingdom")
	}
	if !strings.Contains(body, "Japan") {
		t.Error("expected 'Japan' in geographic table")
	}
	if !strings.Contains(body, "4.00%") {
		t.Error("expected '4.00%' for Japan")
	}

	// Verify sort order: United States (60%) appears before United Kingdom (5%)
	usIdx := strings.Index(body, "United States")
	ukIdx := strings.Index(body, "United Kingdom")
	if usIdx > ukIdx {
		t.Error("geographic allocations should be sorted by percent descending")
	}
}

func TestDetailsHandleDetailsPage_GeographicSingleElement(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "AAPL",
		MarketDataSymbol: "AAPL",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["AAPL"] = 1

	detailsRepo.details["AAPL"] = &symbol.SymbolDetails{
		InternalSymbol: "AAPL",
		ShortName:      "Apple Inc.",
		QuoteType:      "EQUITY",
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 100},
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Single-element: country shown as a "Country" row in the Overview section
	if !strings.Contains(body, "<th>Country</th>") {
		t.Error("expected '<th>Country</th>' row in overview section")
	}
	if !strings.Contains(body, "United States") {
		t.Error("expected 'United States' value in overview section")
	}
	// Should NOT have a separate Geographic Allocation card for single-element
	// (the country is inline in the Overview table)
	if strings.Contains(body, "Geographic Allocation") {
		t.Error("should not show separate 'Geographic Allocation' section for single-element (stock)")
	}
}

func TestDetailsHandleDetailsPage_GeographicNoData(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VOO",
		MarketDataSymbol: "VOO",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VOO"] = 1

	detailsRepo.details["VOO"] = &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500 ETF",
		QuoteType:      "ETF",
		// No geographic allocations
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Geographic Allocation") {
		t.Error("expected 'Geographic Allocation' section header for ETF with no data")
	}
	if !strings.Contains(body, "No geographic data available") {
		t.Error("expected 'No geographic data available' message for ETF with no data")
	}
}

func TestDetailsHandleDetailsPage_GeographicNoData_NonETF_Suppressed(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "MSFT",
		MarketDataSymbol: "MSFT",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["MSFT"] = 1

	detailsRepo.details["MSFT"] = &symbol.SymbolDetails{
		InternalSymbol: "MSFT",
		ShortName:      "Microsoft Corporation",
		QuoteType:      "EQUITY",
		// No geographic allocations, no country
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Non-ETF with no geographic data: "No geographic data available" card is suppressed
	if strings.Contains(body, "No geographic data available") {
		t.Error("should not show 'No geographic data available' for non-ETF (stock)")
	}
}

// --- Extractor data display tests ---

func TestDetailsHandleDetailsPage_ExtractorData_FullSections(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, navSource := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "WMGT",
		MarketDataSymbol: "WMGT",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["WMGT"] = 1

	asOfDate := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	detailsRepo.details["WMGT"] = &symbol.SymbolDetails{
		InternalSymbol:    "WMGT",
		ShortName:         "WisdomTree Germany Hedged",
		LongName:          "WisdomTree Germany Hedged UCITS ETF",
		Exchange:          "XETRA",
		Currency:          "EUR",
		QuoteType:         "ETF",
		ExtractorAsOfDate: asOfDate,
		TopHoldings: []symbol.TopHolding{
			{Symbol: "SAP", Name: "SAP SE", Percent: 4.5},
			{Symbol: "SIE", Name: "Siemens AG", Percent: 3.8},
			{Symbol: "ALV", Name: "Allianz SE", Percent: 3.2},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 20},
			{Sector: "industrials", Percent: 15},
		},
		MarketCapBreakdown: &symbol.MarketCapBreakdown{
			Total: 2.5e11,
			Large: 75,
			Mid:   20,
			Small: 5,
		},
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings:          14.5,
			EstimatedPriceToEarnings: 12.0,
			PriceToBook:              2.3,
			PriceToCashflow:          9.8,
			PriceToSales:             2.1,
			DividendYield:            2.5,
		},
		Themes: []symbol.ThemeBreakdown{
			{Name: "Large Cap", Percent: 80},
			{Name: "Developed Markets", Percent: 100},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "Germany", Percent: 70},
			{Country: "Switzerland", Percent: 10},
		},
		FundProfile: &symbol.FundProfile{
			Family:             "WisdomTree",
			LegalType:          "UCITS",
			TotalNetAssets:     500e6,
			AnnualExpenseRatio: 0.002,
			InceptionDate:      time.Date(2018, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	// NAV history for chart
	price1, _ := decimal.NewFromFloat64(25.50)
	price2, _ := decimal.NewFromFloat64(26.00)
	navSource.navPrices = []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), Close: price1, Currency: "EUR"},
		{Date: time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC), Close: price2, Currency: "EUR"},
	}
	navSource.stockPrices = []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC), Close: price2, Currency: "EUR"},
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}
	checkNotContains := func(label, text string) {
		t.Helper()
		if strings.Contains(body, text) {
			t.Errorf("page should not contain %s: %q", label, text)
		}
	}

	// Holdings header always says "Top 10 Holdings" (expandable)
	checkContains("holdings header", "Top 10 Holdings")
	checkContains("holding 1", "SAP")
	checkContains("holding 2", "Siemens")

	// Market cap section
	checkContains("market cap header", "Market Capitalization")
	checkContains("market cap total", "250.00B")
	checkContains("market cap large", "75.00%")

	// Fund characteristics section
	checkContains("characteristics header", "Fund Characteristics")
	checkContains("pe ratio", "14.50")
	checkContains("pb ratio", "2.30")
	checkContains("dividend yield", "2.50%")

	// Theme breakdown section
	checkContains("theme header", "Theme Breakdown")
	checkContains("theme 1", "Large Cap")
	checkContains("theme 2", "Developed Markets")

	// Country Allocation (renamed from Geographic Allocation for extractor data)
	checkContains("country allocation header", "Country Allocation")
	checkNotContains("geographic allocation header", "Geographic Allocation")

	// As of date
	checkContains("as of date", "2026-05-22")

	// Inception date
	checkContains("inception date", "2018-03-15")

	// NAV chart
	checkContains("chart container", "nav-price-chart")
	checkContains("echarts script", "echarts.min.js")
}

func TestDetailsHandleDetailsPage_YahooData_Top10Holdings(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VOO",
		MarketDataSymbol: "VOO",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VOO"] = 1

	// Yahoo data: no ExtractorAsOfDate
	detailsRepo.details["VOO"] = &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500 ETF",
		QuoteType:      "ETF",
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 7},
			{Symbol: "MSFT", Name: "Microsoft Corp.", Percent: 6},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 30},
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Yahoo data: holdings header should say "Top 10 Holdings"
	if !strings.Contains(body, "Top 10 Holdings") {
		t.Error("expected 'Top 10 Holdings' header for Yahoo data")
	}
	// Should NOT show extractor-specific sections
	if strings.Contains(body, "Market Capitalization") {
		t.Error("should not show Market Capitalization for Yahoo data")
	}
	if strings.Contains(body, "Fund Characteristics") {
		t.Error("should not show Fund Characteristics for Yahoo data")
	}
	if strings.Contains(body, "Theme Breakdown") {
		t.Error("should not show Theme Breakdown for Yahoo data")
	}
	if strings.Contains(body, "nav-price-chart") {
		t.Error("should not show NAV chart for Yahoo data without NAV history")
	}
	// Should NOT show "As of" date
	if strings.Contains(body, "As of") {
		t.Error("should not show 'As of' date for Yahoo data")
	}
}

func TestDetailsHandleDetailsPage_ExtractorData_NoNavChart(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, navSource := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "WMGT",
		MarketDataSymbol: "WMGT",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["WMGT"] = 1

	asOfDate := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	detailsRepo.details["WMGT"] = &symbol.SymbolDetails{
		InternalSymbol:    "WMGT",
		ShortName:         "WisdomTree Germany Hedged",
		QuoteType:         "ETF",
		ExtractorAsOfDate: asOfDate,
		TopHoldings: []symbol.TopHolding{
			{Symbol: "SAP", Name: "SAP SE", Percent: 4.5},
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	// No NAV history available
	navSource.navPrices = nil

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Should show holdings section
	if !strings.Contains(body, "Top 10 Holdings") {
		t.Error("expected 'Top 10 Holdings' header")
	}
	if !strings.Contains(body, "2026-05-22") {
		t.Error("expected 'As of' date")
	}
	// Should NOT show chart when no NAV history
	if strings.Contains(body, "nav-price-chart") {
		t.Error("should not show NAV chart when no NAV history available")
	}
}

func TestToDisplayDetails_ExtractorData_FullHoldings(t *testing.T) {
	asOfDate := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	details := &symbol.SymbolDetails{
		InternalSymbol:    "WMGT",
		ExtractorAsOfDate: asOfDate,
		TopHoldings: []symbol.TopHolding{
			{Symbol: "H1", Name: "Holding 1", Percent: 1},
			{Symbol: "H2", Name: "Holding 2", Percent: 2},
			{Symbol: "H3", Name: "Holding 3", Percent: 3},
		},
		MarketCapBreakdown: &symbol.MarketCapBreakdown{
			Total: 1e9,
			Large: 80,
			Mid:   15,
			Small: 5,
		},
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings:          15,
			EstimatedPriceToEarnings: 12.5,
			PriceToBook:              0, // zero value
			DividendYield:            2.5,
		},
		Themes: []symbol.ThemeBreakdown{
			{Name: "Theme A", Percent: 60},
			{Name: "Theme B", Percent: 40},
		},
		FundProfile: &symbol.FundProfile{
			InceptionDate: time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	dd := toDisplayDetails(details)

	// All holdings included (template handles top-10 display)
	if len(dd.TopHoldings) != 3 {
		t.Errorf("expected 3 holdings, got %d", len(dd.TopHoldings))
	}

	// Extractor as of date
	if dd.ExtractorAsOfDate != "2026-05-22" {
		t.Errorf("expected ExtractorAsOfDate '2026-05-22', got %q", dd.ExtractorAsOfDate)
	}

	// Market cap
	if dd.MarketCapBreakdown == nil {
		t.Error("expected MarketCapBreakdown to be set")
	} else {
		if dd.MarketCapBreakdown.Total != "1.00B" {
			t.Errorf("expected Total '1.00B', got %q", dd.MarketCapBreakdown.Total)
		}
		if dd.MarketCapBreakdown.Large != "80.00%" {
			t.Errorf("expected Large '80.00%%', got %q", dd.MarketCapBreakdown.Large)
		}
	}

	// Equity valuation
	if dd.EquityValuation == nil {
		t.Error("expected EquityValuation to be set")
	} else {
		if dd.EquityValuation.PriceToEarnings != "15.00" {
			t.Errorf("expected P/E '15.00', got %q", dd.EquityValuation.PriceToEarnings)
		}
		if dd.EquityValuation.EstimatedPriceToEarnings != "12.50" {
			t.Errorf("expected Estimated P/E '12.50', got %q", dd.EquityValuation.EstimatedPriceToEarnings)
		}
		if dd.EquityValuation.PriceToBook != "—" {
			t.Errorf("expected P/B '—' for zero, got %q", dd.EquityValuation.PriceToBook)
		}
		if dd.EquityValuation.DividendYield != "2.50%" {
			t.Errorf("expected DividendYield '2.50%%', got %q", dd.EquityValuation.DividendYield)
		}
	}

	// Themes
	if len(dd.Themes) != 2 {
		t.Errorf("expected 2 themes, got %d", len(dd.Themes))
	}

	// Inception date
	if dd.FundProfile == nil || dd.FundProfile.InceptionDate != "2020-01-15" {
		t.Errorf("expected InceptionDate '2020-01-15', got %q", dd.FundProfile.InceptionDate)
	}
}

func TestToDisplayDetails_FundProfile_WithIMGPSpecificFields(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "LU2951555585",
		FundProfile: &symbol.FundProfile{
			Family:             "iMGP",
			LegalType:          "UCITS",
			TotalNetAssets:     500e6,
			AnnualExpenseRatio: 0.002,
			InceptionDate:      time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC),
			Isin:               "LU2951555585",
			ShareClassName:     "R USD UCITS ETF",
			OngoingCharges:     0.75,
		},
	}

	dd := toDisplayDetails(details)

	if dd.FundProfile == nil {
		t.Fatal("expected FundProfile to be set")
	}

	if dd.FundProfile.NetAssets != "500.00M" {
		t.Errorf("expected NetAssets '500.00M', got %q", dd.FundProfile.NetAssets)
	}
	if dd.FundProfile.InceptionDate != "2023-06-15" {
		t.Errorf("expected InceptionDate '2023-06-15', got %q", dd.FundProfile.InceptionDate)
	}
	if dd.FundProfile.Isin != "LU2951555585" {
		t.Errorf("expected Isin 'LU2951555585', got %q", dd.FundProfile.Isin)
	}
	if dd.FundProfile.ShareClassName != "R USD UCITS ETF" {
		t.Errorf("expected ShareClassName 'R USD UCITS ETF', got %q", dd.FundProfile.ShareClassName)
	}
	if dd.FundProfile.OngoingCharges != "0.75%" {
		t.Errorf("expected OngoingCharges '0.75%%', got %q", dd.FundProfile.OngoingCharges)
	}
}

func TestToDisplayDetails_FundProfile_WithoutIMGPSpecificFields(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		FundProfile: &symbol.FundProfile{
			Family:             "Vanguard",
			TotalNetAssets:     1e11,
			AnnualExpenseRatio: 0.0003,
		},
	}

	dd := toDisplayDetails(details)

	if dd.FundProfile == nil {
		t.Fatal("expected FundProfile to be set")
	}

	// iMGP-specific fields should be empty strings (not displayed in template)
	if dd.FundProfile.Isin != "" {
		t.Errorf("expected empty Isin, got %q", dd.FundProfile.Isin)
	}
	if dd.FundProfile.ShareClassName != "" {
		t.Errorf("expected empty ShareClassName, got %q", dd.FundProfile.ShareClassName)
	}
	if dd.FundProfile.OngoingCharges != "" {
		t.Errorf("expected empty OngoingCharges, got %q", dd.FundProfile.OngoingCharges)
	}
}

func TestToDisplayDetails_YahooData_LimitedHoldings(t *testing.T) {
	// Generate 15 holdings
	holdings := make([]symbol.TopHolding, 15)
	for i := 0; i < 15; i++ {
		holdings[i] = symbol.TopHolding{
			Symbol:  fmt.Sprintf("H%02d", i+1),
			Name:    fmt.Sprintf("Holding %d", i+1),
			Percent: float64(15 - i),
		}
	}

	details := &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		TopHoldings:    holdings,
		// No ExtractorAsOfDate (Yahoo data)
		FetchedAt: time.Now(),
	}

	dd := toDisplayDetails(details)

	// All holdings included (template handles top-10 display + expand)
	if len(dd.TopHoldings) != 15 {
		t.Errorf("expected 15 holdings, got %d", len(dd.TopHoldings))
	}

	// No extractor as of date
	if dd.ExtractorAsOfDate != "" {
		t.Errorf("expected empty ExtractorAsOfDate for Yahoo data, got %q", dd.ExtractorAsOfDate)
	}
}

// --- Risk measures display tests ---

func TestToDisplayDetails_RiskMeasures_PartialFields(t *testing.T) {
	// Simulates a new fund with only Volatility and Sharpe Ratio populated
	details := &symbol.SymbolDetails{
		InternalSymbol: "LU2951555585",
		RiskMeasures: &symbol.RiskMeasures{
			Volatility:    9.16,
			SharpeRatio:   2.52,
			FieldsPresent: symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio,
		},
	}

	dd := toDisplayDetails(details)

	if dd.RiskMeasures == nil {
		t.Fatal("expected RiskMeasures to be set")
	}

	if dd.RiskMeasures.Volatility != "9.16%" {
		t.Errorf("expected Volatility '9.16%%', got %q", dd.RiskMeasures.Volatility)
	}
	if dd.RiskMeasures.SharpeRatio != "2.52" {
		t.Errorf("expected SharpeRatio '2.52', got %q", dd.RiskMeasures.SharpeRatio)
	}
	// Absent fields should show em-dash
	if dd.RiskMeasures.InfoRatio != "—" {
		t.Errorf("expected InfoRatio '—', got %q", dd.RiskMeasures.InfoRatio)
	}
	if dd.RiskMeasures.Beta != "—" {
		t.Errorf("expected Beta '—', got %q", dd.RiskMeasures.Beta)
	}
	if dd.RiskMeasures.Correlation != "—" {
		t.Errorf("expected Correlation '—', got %q", dd.RiskMeasures.Correlation)
	}
	if dd.RiskMeasures.TrackingError != "—" {
		t.Errorf("expected TrackingError '—', got %q", dd.RiskMeasures.TrackingError)
	}
}

func TestToDisplayDetails_RiskMeasures_AllFields(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "WMGT",
		RiskMeasures: &symbol.RiskMeasures{
			Volatility:    14.50,
			SharpeRatio:   0.85,
			InfoRatio:     0.35,
			Beta:          1.12,
			Correlation:   0.92,
			TrackingError: 3.45,
			FieldsPresent: symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio |
				symbol.SymbolRiskFieldInfoRatio | symbol.SymbolRiskFieldBeta |
				symbol.SymbolRiskFieldCorrelation | symbol.SymbolRiskFieldTrackingError,
		},
	}

	dd := toDisplayDetails(details)

	if dd.RiskMeasures == nil {
		t.Fatal("expected RiskMeasures to be set")
	}

	if dd.RiskMeasures.Volatility != "14.50%" {
		t.Errorf("expected Volatility '14.50%%', got %q", dd.RiskMeasures.Volatility)
	}
	if dd.RiskMeasures.SharpeRatio != "0.85" {
		t.Errorf("expected SharpeRatio '0.85', got %q", dd.RiskMeasures.SharpeRatio)
	}
	if dd.RiskMeasures.InfoRatio != "0.35" {
		t.Errorf("expected InfoRatio '0.35', got %q", dd.RiskMeasures.InfoRatio)
	}
	if dd.RiskMeasures.Beta != "1.12" {
		t.Errorf("expected Beta '1.12', got %q", dd.RiskMeasures.Beta)
	}
	if dd.RiskMeasures.Correlation != "0.92" {
		t.Errorf("expected Correlation '0.92', got %q", dd.RiskMeasures.Correlation)
	}
	if dd.RiskMeasures.TrackingError != "3.45%" {
		t.Errorf("expected TrackingError '3.45%%', got %q", dd.RiskMeasures.TrackingError)
	}
}

func TestToDisplayDetails_RiskMeasures_NoFieldsPresent(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "TEST",
		RiskMeasures: &symbol.RiskMeasures{
			FieldsPresent: 0, // non-nil but no fields populated
		},
	}

	dd := toDisplayDetails(details)

	if dd.RiskMeasures != nil {
		t.Error("expected RiskMeasures to be nil when no fields are present")
	}
}

func TestToDisplayDetails_RiskMeasures_Nil(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "TEST",
		RiskMeasures:   nil,
	}

	dd := toDisplayDetails(details)

	if dd.RiskMeasures != nil {
		t.Error("expected RiskMeasures to be nil when source is nil")
	}
}

// --- Asset class allocation display tests ---

func TestToDisplayDetails_AssetClassAllocation(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "TEST",
		AssetClassAllocation: []symbol.AssetClassEntry{
			{AssetClass: "Equities", Percent: 85.50},
			{AssetClass: "Bonds", Percent: 5.20},
			{AssetClass: "Gold", Percent: -3.50},
			{AssetClass: "Oil", Percent: -2.20},
			{AssetClass: "Cash", Percent: 15.00},
		},
	}

	dd := toDisplayDetails(details)

	if len(dd.AssetClassAllocation) != 5 {
		t.Fatalf("expected 5 asset class entries, got %d", len(dd.AssetClassAllocation))
	}

	if dd.AssetClassAllocation[0].AssetClass != "Equities" {
		t.Errorf("expected 'Equities', got %q", dd.AssetClassAllocation[0].AssetClass)
	}
	if dd.AssetClassAllocation[0].Percent != "85.50%" {
		t.Errorf("expected '85.50%%', got %q", dd.AssetClassAllocation[0].Percent)
	}
	// Negative value
	if dd.AssetClassAllocation[2].Percent != "-3.50%" {
		t.Errorf("expected '-3.50%%' for Gold, got %q", dd.AssetClassAllocation[2].Percent)
	}
}

func TestToDisplayDetails_AssetClassAllocation_Empty(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol:       "TEST",
		AssetClassAllocation: []symbol.AssetClassEntry{},
	}

	dd := toDisplayDetails(details)

	if len(dd.AssetClassAllocation) != 0 {
		t.Errorf("expected 0 asset class entries, got %d", len(dd.AssetClassAllocation))
	}
}

// --- Equity derivatives by region display tests ---

func TestToDisplayDetails_EquityDerivativesByRegion(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "TEST",
		EquityDerivativesByRegion: []symbol.RegionDerivativeEntry{
			{Region: "North America", Percent: 45.20},
			{Region: "Europe", Percent: 35.80},
			{Region: "Asia", Percent: 15.00},
			{Region: "Emerging Countries", Percent: -6.00},
		},
	}

	dd := toDisplayDetails(details)

	if len(dd.EquityDerivativesByRegion) != 4 {
		t.Fatalf("expected 4 region entries, got %d", len(dd.EquityDerivativesByRegion))
	}

	if dd.EquityDerivativesByRegion[0].Region != "North America" {
		t.Errorf("expected 'North America', got %q", dd.EquityDerivativesByRegion[0].Region)
	}
	if dd.EquityDerivativesByRegion[0].Percent != "45.20%" {
		t.Errorf("expected '45.20%%', got %q", dd.EquityDerivativesByRegion[0].Percent)
	}
	// Negative value
	if dd.EquityDerivativesByRegion[3].Percent != "-6.00%" {
		t.Errorf("expected '-6.00%%', got %q", dd.EquityDerivativesByRegion[3].Percent)
	}
}

// --- Currency derivatives allocation display tests ---

func TestToDisplayDetails_CurrencyDerivativesAllocation(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "TEST",
		CurrencyDerivativesAllocation: []symbol.CurrencyDerivativeEntry{
			{Currency: "USD", Percent: 50.00},
			{Currency: "EUR", Percent: 30.00},
			{Currency: "JPY", Percent: -5.00},
			{Currency: "GBP", Percent: 25.00},
		},
	}

	dd := toDisplayDetails(details)

	if len(dd.CurrencyDerivativesAllocation) != 4 {
		t.Fatalf("expected 4 currency entries, got %d", len(dd.CurrencyDerivativesAllocation))
	}

	if dd.CurrencyDerivativesAllocation[0].Currency != "USD" {
		t.Errorf("expected 'USD', got %q", dd.CurrencyDerivativesAllocation[0].Currency)
	}
	if dd.CurrencyDerivativesAllocation[0].Percent != "50.00%" {
		t.Errorf("expected '50.00%%', got %q", dd.CurrencyDerivativesAllocation[0].Percent)
	}
	// Negative value
	if dd.CurrencyDerivativesAllocation[2].Percent != "-5.00%" {
		t.Errorf("expected '-5.00%%', got %q", dd.CurrencyDerivativesAllocation[2].Percent)
	}
}

// --- Full page integration test for iMGP sections ---

func TestDetailsHandleDetailsPage_IMGPSections(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "LU2951555585",
		MarketDataSymbol: "LU2951555585.L",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["LU2951555585"] = 1

	asOfDate := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	detailsRepo.details["LU2951555585"] = &symbol.SymbolDetails{
		InternalSymbol:    "LU2951555585",
		ShortName:         "iMGP Fund",
		LongName:          "iM Global Partner Fund",
		Exchange:          "LUX",
		Currency:          "EUR",
		QuoteType:         "ETF",
		ExtractorAsOfDate: asOfDate,
		RiskMeasures: &symbol.RiskMeasures{
			Volatility:    9.16,
			SharpeRatio:   2.52,
			FieldsPresent: symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio,
		},
		AssetClassAllocation: []symbol.AssetClassEntry{
			{AssetClass: "Equities", Percent: 85.50},
			{AssetClass: "Bonds", Percent: 5.20},
			{AssetClass: "Gold", Percent: -3.50},
		},
		EquityDerivativesByRegion: []symbol.RegionDerivativeEntry{
			{Region: "North America", Percent: 45.20},
			{Region: "Europe", Percent: 35.80},
			{Region: "Asia", Percent: 19.00},
		},
		CurrencyDerivativesAllocation: []symbol.CurrencyDerivativeEntry{
			{Currency: "USD", Percent: 50.00},
			{Currency: "EUR", Percent: 30.00},
			{Currency: "JPY", Percent: -5.00},
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	// Risk Measures section
	checkContains("risk measures header", "Risk Measures")
	checkContains("volatility", "9.16%")
	checkContains("sharpe ratio", "2.52")
	// Absent fields (InfoRatio, Beta, Correlation, TrackingError) render as em-dash
	// This is verified by the toDisplayDetails unit tests; here we check the section renders

	// Asset Class Allocation section
	checkContains("asset class header", "Asset Class Allocation")
	checkContains("equities", "Equities")
	checkContains("equities percent", "85.50%")
	checkContains("gold negative", "-3.50%")

	// Equity Derivatives by Region section
	checkContains("equity derivatives header", "Equity Derivatives by Region")
	checkContains("north america", "North America")
	checkContains("na percent", "45.20%")
	checkContains("europe", "Europe")
	checkContains("asia", "Asia")

	// Currency Derivatives Allocation section
	checkContains("currency derivatives header", "Currency Derivatives Allocation")
	checkContains("usd", "USD")
	checkContains("usd percent", "50.00%")
	checkContains("jpy negative", "-5.00%")

	// As of date shown in sections
	checkContains("as of date", "2026-05-22")
}

func TestDetailsHandleDetailsPage_IMGPSections_Absent(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VOO",
		MarketDataSymbol: "VOO",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VOO"] = 1

	// Standard Yahoo data — no iMGP-specific fields
	detailsRepo.details["VOO"] = &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		ShortName:      "Vanguard S&P 500 ETF",
		QuoteType:      "ETF",
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 7},
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// iMGP-specific sections should NOT appear
	if strings.Contains(body, "Risk Measures") {
		t.Error("should not show 'Risk Measures' section for non-iMGP data")
	}
	if strings.Contains(body, "Asset Class Allocation") {
		t.Error("should not show 'Asset Class Allocation' section for non-iMGP data")
	}
	if strings.Contains(body, "Equity Derivatives by Region") {
		t.Error("should not show 'Equity Derivatives by Region' section for non-iMGP data")
	}
	if strings.Contains(body, "Currency Derivatives Allocation") {
		t.Error("should not show 'Currency Derivatives Allocation' section for non-iMGP data")
	}
}

// --- Vanguard-specific display tests ---

func TestDetailsHandleDetailsPage_VanguardFields(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, navSource := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VWRL",
		MarketDataSymbol: "VWRL",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VWRL"] = 1

	couponRate := 3.50
	finalMaturity := "2030"
	asOfDate := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	detailsRepo.details["VWRL"] = &symbol.SymbolDetails{
		InternalSymbol:    "VWRL",
		ShortName:         "Vanguard FTSE All-World UCITS ETF",
		LongName:          "Vanguard FTSE All-World UCITS ETF USD Distributing",
		Exchange:          "LSE",
		Currency:          "USD",
		QuoteType:         "ETF",
		ExtractorAsOfDate: asOfDate,
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 3.8, SecurityType: "Common Stock", AsOfDate: "2026-05-21"},
			{Symbol: "MSFT", Name: "Microsoft Corp.", Percent: 3.5, SecurityType: "Common Stock", AsOfDate: "2026-05-21"},
			{Symbol: "BND1", Name: "US Treasury Bond", Percent: 1.2, SecurityType: "Government Bond", CouponRate: &couponRate, FinalMaturity: &finalMaturity, AsOfDate: "2026-05-21"},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 25.5, Date: "2026-05-22"},
			{Sector: "financials", Percent: 13.2, Date: "2026-05-22"},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 60.0, RegionName: "Developed Markets", RegionCode: "DM", Date: "2026-05-22"},
			{Country: "United Kingdom", Percent: 5.0, RegionName: "Developed Markets", RegionCode: "DM", Date: "2026-05-22"},
			{Country: "China", Percent: 4.0, RegionName: "Emerging Markets", RegionCode: "EM", Date: "2026-05-22"},
		},
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings:          18.5,
			EstimatedPriceToEarnings: 15.0,
			PriceToBook:              3.2,
			PriceToCashflow:          10.5,
			PriceToSales:             2.8,
			DividendYield:            1.8,
			MedianMarketCap:          450e9,
			ForwardROE:               18.5,
			ForwardEPSGrowth:         12.3,
			RevenueRatio:             1.05,
		},
		BondCharacteristics: &symbol.BondCharacteristics{
			AverageCoupon:   3.50,
			AverageMaturity: 7.50,
			AverageQuality:  7.20,
			AverageDuration: 6.80,
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	// NAV history for chart
	price1, _ := decimal.NewFromFloat64(10.50)
	price2, _ := decimal.NewFromFloat64(10.75)
	navSource.navPrices = []market.HistoricalPrice{
		{Date: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), Close: price1, Currency: "USD"},
		{Date: time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC), Close: price2, Currency: "USD"},
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}
	checkNotContains := func(label, text string) {
		t.Helper()
		if strings.Contains(body, text) {
			t.Errorf("page should not contain %s: %q", label, text)
		}
	}

	// Holdings: new columns (Type, Coupon, Maturity)
	checkContains("holdings type header", "<th>Type</th>")
	checkContains("holdings coupon header", "<th class=\"text-right\">Coupon</th>")
	checkContains("holdings maturity header", "<th class=\"text-right\">Maturity</th>")
	checkContains("equity security type", "Common Stock")
	checkContains("bond security type", "Government Bond")
	checkContains("coupon rate", "3.50%")
	checkContains("final maturity", "2030")
	checkContains("holdings as of date", "As of 2026-05-21")

	// Sector weightings: per-section date
	checkContains("sector as of date", "As of 2026-05-22")
	checkContains("sector technology", "technology")

	// Country allocation: region column + per-section date
	checkContains("country allocation header", "Country Allocation")
	checkContains("region header", "<th>Region</th>")
	checkContains("developed markets region", "Developed Markets")
	checkContains("emerging markets region", "Emerging Markets")
	checkContains("country as of date", "As of 2026-05-22")

	// Fund characteristics: expanded equity fields
	checkContains("fund characteristics header", "Fund Characteristics")
	checkContains("median market cap", "Median Market Cap")
	checkContains("median market cap value", "450.00B")
	checkContains("forward roe", "Forward ROE")
	checkContains("forward roe value", "18.50%")
	checkContains("forward eps growth", "Forward EPS Growth")
	checkContains("forward eps growth value", "12.30%")
	checkContains("revenue ratio", "Revenue / Prior Year")
	checkContains("revenue ratio value", "1.05")

	// Bond characteristics section (conditional — present because data exists)
	checkContains("bond characteristics header", "Bond Characteristics")
	checkContains("average coupon", "Average Coupon")
	checkContains("average coupon value", "3.50%")
	checkContains("average maturity", "Average Maturity (years)")
	checkContains("average maturity value", "7.50")
	checkContains("average quality", "Average Quality")
	checkContains("average quality value", "7.20")
	checkContains("average duration", "Average Duration (years)")
	checkContains("average duration value", "6.80")

	// NAV chart
	checkContains("chart container", "nav-price-chart")

	// Backward compat: existing fields still work
	checkContains("pe ratio", "18.50")
	checkContains("pb ratio", "3.20")
	checkContains("dividend yield", "1.80%")

	// Should NOT show em-dash for equity holdings' bond fields
	checkNotContains("apple coupon dash", "<td class=\"text-right\">—</td>"+"<td class=\"text-right\">3.80%</td>")
}

func TestToDisplayDetails_VanguardFields(t *testing.T) {
	couponRate := 3.50
	finalMaturity := "2030"
	asOfDate := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	details := &symbol.SymbolDetails{
		InternalSymbol:    "VWRL",
		ExtractorAsOfDate: asOfDate,
		TopHoldings: []symbol.TopHolding{
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 3.8, SecurityType: "Common Stock", AsOfDate: "2026-05-21"},
			{Symbol: "BND1", Name: "US Treasury", Percent: 1.2, SecurityType: "Government Bond", CouponRate: &couponRate, FinalMaturity: &finalMaturity, AsOfDate: "2026-05-21"},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 25.5, Date: "2026-05-22"},
		},
		GeographicAllocations: []symbol.GeographicAllocation{
			{Country: "United States", Percent: 60.0, RegionName: "Developed Markets", RegionCode: "DM", Date: "2026-05-22"},
		},
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings:  18.5,
			MedianMarketCap:  450e9,
			ForwardROE:       18.5,
			ForwardEPSGrowth: 12.3,
			RevenueRatio:     1.05,
		},
		BondCharacteristics: &symbol.BondCharacteristics{
			AverageCoupon:   3.50,
			AverageMaturity: 7.50,
			AverageQuality:  7.20,
			AverageDuration: 6.80,
		},
	}

	dd := toDisplayDetails(details)

	// Holdings: new fields mapped
	if len(dd.TopHoldings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(dd.TopHoldings))
	}
	if dd.TopHoldings[0].SecurityType != "Common Stock" {
		t.Errorf("expected SecurityType 'Common Stock', got %q", dd.TopHoldings[0].SecurityType)
	}
	if dd.TopHoldings[0].AsOfDate != "2026-05-21" {
		t.Errorf("expected AsOfDate '2026-05-21', got %q", dd.TopHoldings[0].AsOfDate)
	}
	if dd.TopHoldings[0].CouponRate != "" {
		t.Errorf("expected empty CouponRate for equity, got %q", dd.TopHoldings[0].CouponRate)
	}
	if dd.TopHoldings[1].CouponRate != "3.50%" {
		t.Errorf("expected CouponRate '3.50%%', got %q", dd.TopHoldings[1].CouponRate)
	}
	if dd.TopHoldings[1].FinalMaturity != "2030" {
		t.Errorf("expected FinalMaturity '2030', got %q", dd.TopHoldings[1].FinalMaturity)
	}

	// Sectors: Date mapped
	if len(dd.SectorWeightings) != 1 {
		t.Fatalf("expected 1 sector, got %d", len(dd.SectorWeightings))
	}
	if dd.SectorWeightings[0].Date != "2026-05-22" {
		t.Errorf("expected Date '2026-05-22', got %q", dd.SectorWeightings[0].Date)
	}

	// Geographic: Region + Date mapped
	if len(dd.GeographicAllocations) != 1 {
		t.Fatalf("expected 1 geo allocation, got %d", len(dd.GeographicAllocations))
	}
	if dd.GeographicAllocations[0].RegionName != "Developed Markets" {
		t.Errorf("expected RegionName 'Developed Markets', got %q", dd.GeographicAllocations[0].RegionName)
	}
	if dd.GeographicAllocations[0].RegionCode != "DM" {
		t.Errorf("expected RegionCode 'DM', got %q", dd.GeographicAllocations[0].RegionCode)
	}
	if dd.GeographicAllocations[0].Date != "2026-05-22" {
		t.Errorf("expected Date '2026-05-22', got %q", dd.GeographicAllocations[0].Date)
	}

	// Equity valuation: new fields
	if dd.EquityValuation == nil {
		t.Fatal("expected EquityValuation to be set")
	}
	if dd.EquityValuation.MedianMarketCap != "450.00B" {
		t.Errorf("expected MedianMarketCap '450.00B', got %q", dd.EquityValuation.MedianMarketCap)
	}
	if dd.EquityValuation.ForwardROE != "18.50%" {
		t.Errorf("expected ForwardROE '18.50%%', got %q", dd.EquityValuation.ForwardROE)
	}
	if dd.EquityValuation.ForwardEPSGrowth != "12.30%" {
		t.Errorf("expected ForwardEPSGrowth '12.30%%', got %q", dd.EquityValuation.ForwardEPSGrowth)
	}
	if dd.EquityValuation.RevenueRatio != "1.05" {
		t.Errorf("expected RevenueRatio '1.05', got %q", dd.EquityValuation.RevenueRatio)
	}

	// Bond characteristics
	if dd.BondCharacteristics == nil {
		t.Fatal("expected BondCharacteristics to be set")
	}
	if dd.BondCharacteristics.AverageCoupon != "3.50%" {
		t.Errorf("expected AverageCoupon '3.50%%', got %q", dd.BondCharacteristics.AverageCoupon)
	}
	if dd.BondCharacteristics.AverageMaturity != "7.50" {
		t.Errorf("expected AverageMaturity '7.50', got %q", dd.BondCharacteristics.AverageMaturity)
	}
	if dd.BondCharacteristics.AverageQuality != "7.20" {
		t.Errorf("expected AverageQuality '7.20', got %q", dd.BondCharacteristics.AverageQuality)
	}
	if dd.BondCharacteristics.AverageDuration != "6.80" {
		t.Errorf("expected AverageDuration '6.80', got %q", dd.BondCharacteristics.AverageDuration)
	}
}

func TestToDisplayDetails_BondCharacteristics_Nil(t *testing.T) {
	// Equity fund — no bond characteristics
	details := &symbol.SymbolDetails{
		InternalSymbol: "VOO",
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings: 15.0,
		},
		BondCharacteristics: nil,
	}

	dd := toDisplayDetails(details)

	if dd.BondCharacteristics != nil {
		t.Error("expected BondCharacteristics to be nil for equity fund")
	}
}

func TestToDisplayDetails_EquityValuation_ZeroNewFields(t *testing.T) {
	// Existing extractor data without new Vanguard-specific fields
	details := &symbol.SymbolDetails{
		InternalSymbol: "WMGT",
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings: 14.5,
			PriceToBook:     2.3,
			// New fields are zero
		},
	}

	dd := toDisplayDetails(details)

	if dd.EquityValuation == nil {
		t.Fatal("expected EquityValuation to be set")
	}
	// Zero values render as em-dash
	if dd.EquityValuation.MedianMarketCap != "—" {
		t.Errorf("expected MedianMarketCap '—' for zero, got %q", dd.EquityValuation.MedianMarketCap)
	}
	if dd.EquityValuation.ForwardROE != "—" {
		t.Errorf("expected ForwardROE '—' for zero, got %q", dd.EquityValuation.ForwardROE)
	}
	if dd.EquityValuation.ForwardEPSGrowth != "—" {
		t.Errorf("expected ForwardEPSGrowth '—' for zero, got %q", dd.EquityValuation.ForwardEPSGrowth)
	}
	if dd.EquityValuation.RevenueRatio != "—" {
		t.Errorf("expected RevenueRatio '—' for zero, got %q", dd.EquityValuation.RevenueRatio)
	}
	// Existing fields still work
	if dd.EquityValuation.PriceToEarnings != "14.50" {
		t.Errorf("expected PriceToEarnings '14.50', got %q", dd.EquityValuation.PriceToEarnings)
	}
}

// --- BlackRock/iShares-specific display tests ---

func TestToDisplayDetails_BlackRockFundProfile(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "ISEW.L",
		FundProfile: &symbol.FundProfile{
			Family:             "iShares",
			LegalType:          "UCITS",
			TotalNetAssets:     50e9,
			AnnualExpenseRatio: 0.004,
			Isin:               "IE00B4L5Y983",
			Benchmark:          "S&P 500 Index",
			SFDRClassification: "Article 8",
			Domicile:           "Ireland",
			RebalanceFrequency: "Quarterly",
			ProductStructure:   "Physical",
			Methodology:        "Representative",
			FundManager:        "BlackRock Asset Management Ireland Limited",
			Custodian:          "State Street Custodial Services (Ireland) Limited",
			IssuingCompany:     "iShares IV plc",
			BenchmarkTicker:    "SPX",
		},
	}

	dd := toDisplayDetails(details)

	if dd.FundProfile == nil {
		t.Fatal("expected FundProfile to be set")
	}

	if dd.FundProfile.SFDRClassification != "Article 8" {
		t.Errorf("expected SFDRClassification 'Article 8', got %q", dd.FundProfile.SFDRClassification)
	}
	if dd.FundProfile.Domicile != "Ireland" {
		t.Errorf("expected Domicile 'Ireland', got %q", dd.FundProfile.Domicile)
	}
	if dd.FundProfile.RebalanceFrequency != "Quarterly" {
		t.Errorf("expected RebalanceFrequency 'Quarterly', got %q", dd.FundProfile.RebalanceFrequency)
	}
	if dd.FundProfile.ProductStructure != "Physical" {
		t.Errorf("expected ProductStructure 'Physical', got %q", dd.FundProfile.ProductStructure)
	}
	if dd.FundProfile.Methodology != "Representative" {
		t.Errorf("expected Methodology 'Representative', got %q", dd.FundProfile.Methodology)
	}
	if dd.FundProfile.FundManager != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("expected FundManager, got %q", dd.FundProfile.FundManager)
	}
	if dd.FundProfile.Custodian != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("expected Custodian, got %q", dd.FundProfile.Custodian)
	}
	if dd.FundProfile.IssuingCompany != "iShares IV plc" {
		t.Errorf("expected IssuingCompany 'iShares IV plc', got %q", dd.FundProfile.IssuingCompany)
	}
	if dd.FundProfile.BenchmarkTicker != "SPX" {
		t.Errorf("expected BenchmarkTicker 'SPX', got %q", dd.FundProfile.BenchmarkTicker)
	}
}

func TestToDisplayDetails_BlackRockEquityValuation(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "ISEW.L",
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings:     15.2,
			DividendYield:       1.5,
			Beta3Y:              0.85,
			StandardDeviation3Y: 9.16,
			NumberOfHoldings:    352,
		},
	}

	dd := toDisplayDetails(details)

	if dd.EquityValuation == nil {
		t.Fatal("expected EquityValuation to be set")
	}
	if dd.EquityValuation.Beta3Y != "0.85" {
		t.Errorf("expected Beta3Y '0.85', got %q", dd.EquityValuation.Beta3Y)
	}
	if dd.EquityValuation.StandardDeviation3Y != "9.16%" {
		t.Errorf("expected StandardDeviation3Y '9.16%%', got %q", dd.EquityValuation.StandardDeviation3Y)
	}
	if dd.EquityValuation.NumberOfHoldings != "352" {
		t.Errorf("expected NumberOfHoldings '352', got %q", dd.EquityValuation.NumberOfHoldings)
	}
}

func TestToDisplayDetails_BlackRockHoldings(t *testing.T) {
	details := &symbol.SymbolDetails{
		InternalSymbol: "ISEW.L",
		TopHoldings: []symbol.TopHolding{
			{
				Symbol:         "AAPL",
				Name:           "Apple Inc.",
				Percent:        7.04,
				Sector:         "Information Technology",
				AssetClass:     "Equity",
				MarketValue:    120e9,
				NotionalValue:  118e9,
				Shares:         350000,
				Price:          337.14,
				ISIN:           "037833100",
				Location:       "United States",
				Exchange:       "NASDAQ",
				MarketCurrency: "USD",
			},
		},
	}

	dd := toDisplayDetails(details)

	if len(dd.TopHoldings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(dd.TopHoldings))
	}

	h := dd.TopHoldings[0]
	if h.Sector != "Information Technology" {
		t.Errorf("expected Sector 'Information Technology', got %q", h.Sector)
	}
	if h.AssetClass != "Equity" {
		t.Errorf("expected AssetClass 'Equity', got %q", h.AssetClass)
	}
	if h.MarketValue != "120.00B" {
		t.Errorf("expected MarketValue '120.00B', got %q", h.MarketValue)
	}
	if h.NotionalValue != "118.00B" {
		t.Errorf("expected NotionalValue '118.00B', got %q", h.NotionalValue)
	}
	if h.Shares != "350000.00" {
		t.Errorf("expected Shares '350000.00', got %q", h.Shares)
	}
	if h.Price != "337.14" {
		t.Errorf("expected Price '337.14', got %q", h.Price)
	}
	if h.ISIN != "037833100" {
		t.Errorf("expected ISIN '037833100', got %q", h.ISIN)
	}
	if h.Location != "United States" {
		t.Errorf("expected Location 'United States', got %q", h.Location)
	}
	if h.Exchange != "NASDAQ" {
		t.Errorf("expected Exchange 'NASDAQ', got %q", h.Exchange)
	}
	if h.MarketCurrency != "USD" {
		t.Errorf("expected MarketCurrency 'USD', got %q", h.MarketCurrency)
	}
}

func TestDetailsHandleDetailsPage_BlackRockSections(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "ISEW.L",
		MarketDataSymbol: "ISEW.L",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["ISEW.L"] = 1

	asOfDate := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	detailsRepo.details["ISEW.L"] = &symbol.SymbolDetails{
		InternalSymbol:    "ISEW.L",
		ShortName:         "iShares Core S&P 500 UCITS ETF USD (Acc)",
		LongName:          "iShares Core S&P 500 UCITS ETF USD (Acc)",
		Exchange:          "LSE",
		Currency:          "GBP",
		QuoteType:         "ETF",
		ExtractorAsOfDate: asOfDate,
		FundProfile: &symbol.FundProfile{
			Family:             "iShares",
			LegalType:          "UCITS",
			TotalNetAssets:     50e9,
			AnnualExpenseRatio: 0.004,
			Isin:               "IE00B4L5Y983",
			Benchmark:          "S&P 500 Index",
			SFDRClassification: "Article 8",
			Domicile:           "Ireland",
			RebalanceFrequency: "Quarterly",
			ProductStructure:   "Physical",
			Methodology:        "Representative",
			FundManager:        "BlackRock Asset Management Ireland Limited",
			Custodian:          "State Street Custodial Services (Ireland) Limited",
			IssuingCompany:     "iShares IV plc",
			BenchmarkTicker:    "SPX",
		},
		EquityValuation: &symbol.EquityValuation{
			PriceToEarnings:     15.2,
			DividendYield:       1.5,
			Beta3Y:              0.85,
			StandardDeviation3Y: 9.16,
			NumberOfHoldings:    352,
		},
		TopHoldings: []symbol.TopHolding{
			{
				Symbol:   "AAPL",
				Name:     "Apple Inc.",
				Percent:  7.04,
				Sector:   "Information Technology",
				Exchange: "NASDAQ",
			},
			{
				Symbol:   "MSFT",
				Name:     "Microsoft Corp.",
				Percent:  6.50,
				Sector:   "Information Technology",
				Exchange: "NASDAQ",
			},
		},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	r := httptest.NewRequest(http.MethodGet, "/symbols/1/details", nil)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	handler.HandleDetailsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	// Fund Profile section — new BlackRock fields
	checkContains("sfdr classification", "Article 8")
	checkContains("domicile", "Ireland")
	checkContains("rebalance frequency", "Quarterly")
	checkContains("product structure", "Physical")
	checkContains("methodology", "Representative")
	checkContains("fund manager", "BlackRock Asset Management Ireland Limited")
	checkContains("custodian", "State Street Custodial Services (Ireland) Limited")
	checkContains("issuing company", "iShares IV plc")
	checkContains("benchmark ticker", "SPX")

	// Fund Characteristics — new BlackRock fields
	checkContains("beta 3Y", "0.85")
	checkContains("standard deviation", "9.16%")
	checkContains("number of holdings", "352")

	// Holdings table — sector column
	checkContains("sector header", "Sector")
	checkContains("sector value", "Information Technology")

	// Existing fields still present
	checkContains("pe ratio", "15.20")
	checkContains("dividend yield", "1.50%")
	checkContains("holdings header", "Top 10 Holdings")
}

func TestHandleRefreshDetails_Success(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _, _ := setupDetailsWebHandler(t)

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VOO",
		MarketDataSymbol: "VOO",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VOO"] = 1

	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	req := httptest.NewRequest(http.MethodPost, "/symbols/1/details/refresh", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if loc != "/symbols/1/details" {
		t.Errorf("expected redirect to /symbols/1/details, got %s", loc)
	}

	if !detailsRepo.touchFetchedAt {
		t.Error("expected TouchFetchedAt to be called")
	}
	if detailsRepo.touchFetchedAtSym != "VOO" {
		t.Errorf("expected TouchFetchedAt for VOO, got %s", detailsRepo.touchFetchedAtSym)
	}
}

func TestHandleRefreshDetails_NotFound(t *testing.T) {
	handler, _, _, _, _, _, _ := setupDetailsWebHandler(t)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "999")
	req := httptest.NewRequest(http.MethodPost, "/symbols/999/details/refresh", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleRefreshDetails_NoDetailsService(t *testing.T) {
	handler, _, _, smRepo, _, _, _ := setupDetailsWebHandler(t)
	handler.detailsSvc = nil

	smRepo.mappings[1] = &symbolmapping.SymbolMapping{
		ID:               1,
		InternalSymbol:   "VOO",
		MarketDataSymbol: "VOO",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	smRepo.byInternal["VOO"] = 1

	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "1")
	req := httptest.NewRequest(http.MethodPost, "/symbols/1/details/refresh", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, ctx))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}
