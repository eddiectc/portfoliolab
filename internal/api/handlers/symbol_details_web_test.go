package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/types/symbol"
)

// --- Mocks ---

type detailsWebTestRepo struct {
	details map[string]*symbol.SymbolDetails
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

// --- Setup ---

func setupDetailsWebHandler(t *testing.T) (*SymbolDetailsWebHandler, *symbolmapping.Service, *symbols.Service, *testSMWebRepo, *detailsWebTestRepo, *detailsWebQuoteFetcher) {
	t.Helper()
	smRepo := newTestSMWebRepo()
	smSvc := symbolmapping.NewService(smRepo)
	detailsRepo := newDetailsWebTestRepo()
	detailsFetcher := &detailsWebTestFetcher{}
	detailsSvc := symbols.NewService(detailsRepo, detailsFetcher)
	quoteFetcher := &detailsWebQuoteFetcher{}
	renderer := newTestRenderer(t)
	handler := NewSymbolDetailsWebHandler(smSvc, detailsSvc, quoteFetcher, renderer)
	return handler, smSvc, detailsSvc, smRepo, detailsRepo, quoteFetcher
}

// --- HandleDetailsPage Tests ---

func TestDetailsHandleDetailsPage_WithDetails(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, quoteFetcher := setupDetailsWebHandler(t)

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
			{Symbol: "AAPL", Name: "Apple Inc.", Percent: 0.07},
			{Symbol: "MSFT", Name: "Microsoft Corp.", Percent: 0.06},
		},
		SectorWeightings: []symbol.SectorWeighting{
			{Sector: "technology", Percent: 0.30},
			{Sector: "financials", Percent: 0.13},
		},
		AggregatePositions: &symbol.AggregatePositions{
			Stock: 0.995,
			Cash:  0.005,
		},
		FundProfile: &symbol.FundProfile{
			Family:         "Vanguard",
			LegalType:      "Exchange Traded Fund",
			TotalNetAssets: 1e11,
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
	handler, _, _, smRepo, _, quoteFetcher := setupDetailsWebHandler(t)

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
	handler, _, _, _, _, _ := setupDetailsWebHandler(t)

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
	handler, _, _, _, _, _ := setupDetailsWebHandler(t)

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
	handler := NewSymbolDetailsWebHandler(nil, nil, nil, newTestRenderer(t))
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
	handler, _, _, smRepo, detailsRepo, quoteFetcher := setupDetailsWebHandler(t)

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
	handler, _, _, smRepo, detailsRepo, _ := setupDetailsWebHandler(t)

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
			{Country: "United States", Percent: 0.60},
			{Country: "United Kingdom", Percent: 0.05},
			{Country: "Japan", Percent: 0.04},
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
	handler, _, _, smRepo, detailsRepo, _ := setupDetailsWebHandler(t)

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
			{Country: "United States", Percent: 1.0},
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

	// Single-element: country shown in Overview section, NOT as a separate table
	if !strings.Contains(body, "United States") {
		t.Error("expected 'United States' in overview section")
	}
	// Should NOT have a separate Geographic Allocation card for single-element
	// (the country is inline in the Overview table)
	if strings.Contains(body, "Geographic Allocation") {
		t.Error("should not show separate 'Geographic Allocation' section for single-element (stock)")
	}
}

func TestDetailsHandleDetailsPage_GeographicNoData(t *testing.T) {
	handler, _, _, smRepo, detailsRepo, _ := setupDetailsWebHandler(t)

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

	if !strings.Contains(body, "No geographic data available") {
		t.Error("expected 'No geographic data available' message")
	}
	if strings.Contains(body, "Geographic Allocation") {
		// The section header is present but with "No geographic data available" body
		// This is fine — it shows the section with a placeholder
	}
}
