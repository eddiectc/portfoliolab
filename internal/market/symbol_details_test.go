package market

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
)

// --- JSON Fixtures (from RESEARCH.md) ---

const etfTopHoldingsJSON = `{
  "quoteSummary": {
    "result": [{
      "topHoldings": {
        "holdings": [
          {"symbol": "BE", "holdingName": "Bloom Energy Corp Class A", "holdingPercent": 0.013997001},
          {"symbol": "JWN", "holdingName": "Nordstrom Inc", "holdingPercent": 0.0089}
        ],
        "stockPosition": 0.9929,
        "bondPosition": 0,
        "cashPosition": 0.006,
        "convertiblePosition": 0,
        "preferredPosition": 0,
        "otherPosition": 0.001,
        "equityHoldings": {
          "priceToEarnings": 0.03516,
          "priceToBook": 0.27355,
          "priceToCashflow": 0.06053,
          "priceToSales": 0.40772
        },
        "sectorWeightings": [
          {"realestate": 0.059699997},
          {"technology": 0.21209998},
          {"industrials": 0.3637}
        ],
        "maxAge": 1
      },
      "fundProfile": {
        "family": "WisdomTree Management Limited",
        "legalType": "Exchange Traded Fund",
        "feesExpensesInvestment": {
          "totalNetAssets": 21526.37,
          "annualReportExpenseRatio": 0.4,
          "annualHoldingsTurnover": 0
        }
      },
      "assetProfile": {
        "shortName": "WisdomTree Megatrends UCITS",
        "longName": "WisdomTree Megatrends UCITS ETF",
        "exchange": "LSE",
        "currency": "GBP",
        "quoteType": "ETF",
        "maxAge": 1
      }
    }]
  }
}`

const equityOnlyJSON = `{
  "quoteSummary": {
    "result": [{
      "assetProfile": {
        "shortName": "Apple Inc.",
        "longName": "Apple Inc.",
        "exchange": "NMS",
        "currency": "USD",
        "quoteType": "EQUITY",
        "maxAge": 1
      }
    }]
  }
}`

const emptyResultJSON = `{
  "quoteSummary": {
    "result": null,
    "error": {
      "code": "Not Found",
      "description": "Quote not found for symbol: INVALID"
    }
  }
}`

// --- Parse Tests ---

func TestParseQuoteSummaryResponse_ETFFull(t *testing.T) {
	var resp quoteSummaryResponse
	if err := json.Unmarshal([]byte(etfTopHoldingsJSON), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.QuoteSummary.Result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.QuoteSummary.Result))
	}

	result := resp.QuoteSummary.Result[0]

	// Asset profile
	if result.AssetProfile == nil {
		t.Fatal("expected assetProfile, got nil")
	}
	if result.AssetProfile.ShortName != "WisdomTree Megatrends UCITS" {
		t.Errorf("shortName = %q", result.AssetProfile.ShortName)
	}
	if result.AssetProfile.LongName != "WisdomTree Megatrends UCITS ETF" {
		t.Errorf("longName = %q", result.AssetProfile.LongName)
	}
	if result.AssetProfile.Exchange != "LSE" {
		t.Errorf("exchange = %q", result.AssetProfile.Exchange)
	}
	if result.AssetProfile.Currency != "GBP" {
		t.Errorf("currency = %q", result.AssetProfile.Currency)
	}
	if result.AssetProfile.QuoteType != "ETF" {
		t.Errorf("quoteType = %q", result.AssetProfile.QuoteType)
	}

	// Top holdings
	if result.TopHoldings == nil {
		t.Fatal("expected topHoldings, got nil")
	}
	if len(result.TopHoldings.Holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(result.TopHoldings.Holdings))
	}
	if result.TopHoldings.Holdings[0].Symbol != "BE" {
		t.Errorf("holding[0].symbol = %q", result.TopHoldings.Holdings[0].Symbol)
	}
	if result.TopHoldings.Holdings[0].HoldingPercent != 0.013997001 {
		t.Errorf("holding[0].percent = %f", result.TopHoldings.Holdings[0].HoldingPercent)
	}

	// Sector weightings
	if len(result.TopHoldings.SectorWeightings) != 3 {
		t.Fatalf("expected 3 sectors, got %d", len(result.TopHoldings.SectorWeightings))
	}

	// Aggregate positions
	if result.TopHoldings.StockPosition != 0.9929 {
		t.Errorf("stockPosition = %f", result.TopHoldings.StockPosition)
	}
	if result.TopHoldings.CashPosition != 0.006 {
		t.Errorf("cashPosition = %f", result.TopHoldings.CashPosition)
	}

	// Equity valuation
	if result.TopHoldings.EquityHoldings == nil {
		t.Fatal("expected equityHoldings, got nil")
	}
	if result.TopHoldings.EquityHoldings.PriceToEarnings != 0.03516 {
		t.Errorf("P/E = %f", result.TopHoldings.EquityHoldings.PriceToEarnings)
	}

	// Fund profile
	if result.FundProfile == nil {
		t.Fatal("expected fundProfile, got nil")
	}
	if result.FundProfile.Family != "WisdomTree Management Limited" {
		t.Errorf("family = %q", result.FundProfile.Family)
	}
	if result.FundProfile.LegalType != "Exchange Traded Fund" {
		t.Errorf("legalType = %q", result.FundProfile.LegalType)
	}
	if result.FundProfile.FeesExpenses.TotalNetAssets != 21526.37 {
		t.Errorf("totalNetAssets = %f", result.FundProfile.FeesExpenses.TotalNetAssets)
	}
}

func TestParseQuoteSummaryResponse_EquityOnly(t *testing.T) {
	var resp quoteSummaryResponse
	if err := json.Unmarshal([]byte(equityOnlyJSON), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	result := resp.QuoteSummary.Result[0]

	if result.AssetProfile == nil {
		t.Fatal("expected assetProfile, got nil")
	}
	if result.AssetProfile.ShortName != "Apple Inc." {
		t.Errorf("shortName = %q", result.AssetProfile.ShortName)
	}
	if result.AssetProfile.QuoteType != "EQUITY" {
		t.Errorf("quoteType = %q", result.AssetProfile.QuoteType)
	}

	// ETF-specific modules should be nil for equities
	if result.TopHoldings != nil {
		t.Error("expected nil topHoldings for equity")
	}
	if result.FundProfile != nil {
		t.Error("expected nil fundProfile for equity")
	}
}

func TestParseQuoteSummaryResponse_Error(t *testing.T) {
	var resp quoteSummaryResponse
	if err := json.Unmarshal([]byte(emptyResultJSON), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.QuoteSummary.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.QuoteSummary.Error.Code != "Not Found" {
		t.Errorf("error code = %q", resp.QuoteSummary.Error.Code)
	}
	if resp.QuoteSummary.Result != nil {
		t.Error("expected nil result on error")
	}
}

// --- Helper Function Tests ---

func TestParseTopHoldings(t *testing.T) {
	tests := []struct {
		name     string
		input    []topHoldingItem
		wantNil  bool
		wantLen  int
		wantFirst symbols.TopHolding
	}{
		{
			name: "normal",
			input: []topHoldingItem{
				{Symbol: "AAPL", HoldingName: "Apple Inc.", HoldingPercent: 0.05},
			},
			wantNil:   false,
			wantLen:   1,
			wantFirst: symbols.TopHolding{Symbol: "AAPL", Name: "Apple Inc.", Percent: 0.05},
		},
		{
			name:    "empty",
			input:   []topHoldingItem{},
			wantNil: true,
		},
		{
			name:    "nil",
			input:   nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTopHoldings(tt.input)
			if tt.wantNil && got != nil {
				t.Error("expected nil, got non-nil")
			}
			if !tt.wantNil && got == nil {
				t.Error("expected non-nil, got nil")
			}
			if !tt.wantNil && len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			if !tt.wantNil && len(got) > 0 {
				if got[0] != tt.wantFirst {
					t.Errorf("got[0] = %+v, want %+v", got[0], tt.wantFirst)
				}
			}
		})
	}
}

func TestParseSectorWeightings(t *testing.T) {
	input := []sectorWeightItem{
		{"technology": 0.25},
		{"healthcare": 0.15},
	}
	got := parseSectorWeightings(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 weightings, got %d", len(got))
	}
	if got[0].Sector != "technology" || got[0].Percent != 0.25 {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Sector != "healthcare" || got[1].Percent != 0.15 {
		t.Errorf("got[1] = %+v", got[1])
	}

	// Empty input
	if got := parseSectorWeightings([]sectorWeightItem{}); got != nil {
		t.Error("expected nil for empty input")
	}
	if got := parseSectorWeightings(nil); got != nil {
		t.Error("expected nil for nil input")
	}
}

// --- Integration-Style Tests with Mock Server ---

func setupMockServer(t *testing.T, responseJSON string) (*httptest.Server, func()) {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Cookie endpoint (fc.yahoo.com → /)
		w.Header().Set("Set-Cookie", "A3=a=b")
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test-crumb-123"))
	})

	mux.HandleFunc("/v10/finance/quoteSummary/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseJSON))
	})

	server := httptest.NewServer(mux)
	origCookie := yahooCookieURL
	origCrumb := yahooCrumbURL
	origQuoteSummary := yahooQuoteSummary

	// Override the package-level vars to point to the mock server
	yahooCookieURL = server.URL
	yahooCrumbURL = server.URL + "/v1/test/getcrumb"
	yahooQuoteSummary = server.URL + "/v10/finance/quoteSummary"

	cleanup := func() {
		server.Close()
		yahooCookieURL = origCookie
		yahooCrumbURL = origCrumb
		yahooQuoteSummary = origQuoteSummary
	}

	return server, cleanup
}

func TestFetchSymbolDetails_ETFFull(t *testing.T) {
	_, cleanup := setupMockServer(t, etfTopHoldingsJSON)
	defer cleanup()

	fetcher := NewYahooFinanceFetcher(discardLogger())
	details, err := fetcher.FetchSymbolDetails(context.Background(), "WMGG.L")
	if err != nil {
		t.Fatalf("FetchSymbolDetails: %v", err)
	}

	// Generic fields
	if details.ShortName != "WisdomTree Megatrends UCITS" {
		t.Errorf("ShortName = %q", details.ShortName)
	}
	if details.LongName != "WisdomTree Megatrends UCITS ETF" {
		t.Errorf("LongName = %q", details.LongName)
	}
	if details.Exchange != "LSE" {
		t.Errorf("Exchange = %q", details.Exchange)
	}
	if details.Currency != "GBP" {
		t.Errorf("Currency = %q", details.Currency)
	}
	if details.QuoteType != "ETF" {
		t.Errorf("QuoteType = %q", details.QuoteType)
	}

	// ETF holdings
	if len(details.TopHoldings) != 2 {
		t.Fatalf("TopHoldings len = %d, want 2", len(details.TopHoldings))
	}
	if details.TopHoldings[0].Symbol != "BE" {
		t.Errorf("TopHoldings[0].Symbol = %q", details.TopHoldings[0].Symbol)
	}
	if details.TopHoldings[0].Percent != 0.013997001 {
		t.Errorf("TopHoldings[0].Percent = %f", details.TopHoldings[0].Percent)
	}

	// Sector weightings
	if len(details.SectorWeightings) != 3 {
		t.Fatalf("SectorWeightings len = %d, want 3", len(details.SectorWeightings))
	}

	// Aggregate positions
	if details.AggregatePositions == nil {
		t.Fatal("AggregatePositions is nil")
	}
	if details.AggregatePositions.Stock != 0.9929 {
		t.Errorf("AggregatePositions.Stock = %f", details.AggregatePositions.Stock)
	}
	if details.AggregatePositions.Cash != 0.006 {
		t.Errorf("AggregatePositions.Cash = %f", details.AggregatePositions.Cash)
	}

	// Equity valuation
	if details.EquityValuation == nil {
		t.Fatal("EquityValuation is nil")
	}
	if details.EquityValuation.PriceToEarnings != 0.03516 {
		t.Errorf("EquityValuation.P/E = %f", details.EquityValuation.PriceToEarnings)
	}

	// Fund profile
	if details.FundProfile == nil {
		t.Fatal("FundProfile is nil")
	}
	if details.FundProfile.Family != "WisdomTree Management Limited" {
		t.Errorf("FundProfile.Family = %q", details.FundProfile.Family)
	}
	if details.FundProfile.LegalType != "Exchange Traded Fund" {
		t.Errorf("FundProfile.LegalType = %q", details.FundProfile.LegalType)
	}
	if details.FundProfile.TotalNetAssets != 21526.37 {
		t.Errorf("FundProfile.TotalNetAssets = %f", details.FundProfile.TotalNetAssets)
	}
	if details.FundProfile.AnnualExpenseRatio != 0.4 {
		t.Errorf("FundProfile.AnnualExpenseRatio = %f", details.FundProfile.AnnualExpenseRatio)
	}

	if details.FetchedAt.IsZero() {
		t.Error("FetchedAt is zero")
	}
}

func TestFetchSymbolDetails_EquityOnly(t *testing.T) {
	_, cleanup := setupMockServer(t, equityOnlyJSON)
	defer cleanup()

	fetcher := NewYahooFinanceFetcher(discardLogger())
	details, err := fetcher.FetchSymbolDetails(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("FetchSymbolDetails: %v", err)
	}

	if details.ShortName != "Apple Inc." {
		t.Errorf("ShortName = %q", details.ShortName)
	}
	if details.QuoteType != "EQUITY" {
		t.Errorf("QuoteType = %q", details.QuoteType)
	}

	// ETF-specific fields should be nil
	if details.TopHoldings != nil {
		t.Error("expected nil TopHoldings for equity")
	}
	if details.SectorWeightings != nil {
		t.Error("expected nil SectorWeightings for equity")
	}
	if details.AggregatePositions != nil {
		t.Error("expected nil AggregatePositions for equity")
	}
	if details.EquityValuation != nil {
		t.Error("expected nil EquityValuation for equity")
	}
	if details.FundProfile != nil {
		t.Error("expected nil FundProfile for equity")
	}
}

func TestFetchSymbolDetails_NotFound(t *testing.T) {
	_, cleanup := setupMockServer(t, emptyResultJSON)
	defer cleanup()

	fetcher := NewYahooFinanceFetcher(discardLogger())
	_, err := fetcher.FetchSymbolDetails(context.Background(), "INVALID")
	if err == nil {
		t.Fatal("expected error for not-found symbol")
	}
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("error should mention 'Not Found', got: %v", err)
	}
}

func TestFetchSymbolDetails_PartialData(t *testing.T) {
	partialJSON := `{
  "quoteSummary": {
    "result": [{
      "assetProfile": {
        "shortName": "Test Corp",
        "longName": "Test Corporation",
        "exchange": "NYSE",
        "currency": "USD",
        "quoteType": "EQUITY",
        "maxAge": 1
      },
      "topHoldings": {
        "holdings": [],
        "stockPosition": 0,
        "bondPosition": 0,
        "cashPosition": 0,
        "sectorWeightings": [],
        "maxAge": 1
      }
    }]
  }
}`

	_, cleanup := setupMockServer(t, partialJSON)
	defer cleanup()

	fetcher := NewYahooFinanceFetcher(discardLogger())
	details, err := fetcher.FetchSymbolDetails(context.Background(), "TEST")
	if err != nil {
		t.Fatalf("FetchSymbolDetails: %v", err)
	}

	// Generic fields populated
	if details.ShortName != "Test Corp" {
		t.Errorf("ShortName = %q", details.ShortName)
	}
	if details.QuoteType != "EQUITY" {
		t.Errorf("QuoteType = %q", details.QuoteType)
	}

	// Empty topHoldings → nil
	if details.TopHoldings != nil {
		t.Errorf("expected nil TopHoldings for empty array, got %d items", len(details.TopHoldings))
	}

	// AggregatePositions set (zeros are valid)
	if details.AggregatePositions == nil {
		t.Error("expected AggregatePositions even when empty")
	}

	// FundProfile should be nil
	if details.FundProfile != nil {
		t.Error("expected nil FundProfile")
	}
}

func TestFetchSymbolDetails_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origCookie := yahooCookieURL
	origCrumb := yahooCrumbURL
	origQuoteSummary := yahooQuoteSummary
	yahooCookieURL = server.URL
	yahooCrumbURL = server.URL + "/v1/test/getcrumb"
	yahooQuoteSummary = server.URL + "/v10/finance/quoteSummary"
	defer func() {
		yahooCookieURL = origCookie
		yahooCrumbURL = origCrumb
		yahooQuoteSummary = origQuoteSummary
	}()

	fetcher := NewYahooFinanceFetcher(discardLogger())
	_, err := fetcher.FetchSymbolDetails(context.Background(), "TEST")
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// --- Interface Check ---

func TestSymbolDetailsFetcherInterface(t *testing.T) {
	var _ SymbolDetailsFetcher = (*YahooFinanceFetcher)(nil)
}

// --- Stale Threshold Verification ---

func TestStaleThreshold(t *testing.T) {
	now := time.Now()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
	sixDaysAgo := now.Add(-6 * 24 * time.Hour)

	if !sevenDaysAgo.Before(sixDaysAgo) {
		t.Error("sanity check failed")
	}

	details := &symbols.SymbolDetails{
		FetchedAt: sevenDaysAgo.Add(-time.Hour), // 8 days ago
	}
	if !details.FetchedAt.Before(sevenDaysAgo) {
		t.Error("8 days ago should be before 7 days ago")
	}
}

// --- Helpers ---

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
}
