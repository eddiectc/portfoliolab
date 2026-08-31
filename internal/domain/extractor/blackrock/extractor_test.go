package blackrock

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	if e.Name() != Name {
		t.Errorf("expected name %q, got %q", Name, e.Name())
	}
}

func TestExtractor_Match(t *testing.T) {
	e := NewExtractor()

	if !e.Match("https://www.ishares.com/uk/individual/en/products/270051/test") {
		t.Error("expected ishares.com/uk URL to match")
	}
	if e.Match("https://www.wisdomtree.eu/en-gb/etfs/wmgt") {
		t.Error("expected wisdomtree.eu URL to not match")
	}
}

func TestExtractor_Extract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleProductPageHTML))
	}))
	defer server.Close()

	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(fetchSampleByComponent(t, nil, nil))

	productURL := server.URL + "/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf"
	result, err := e.Extract(context.Background(), productURL)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.FundInfo == nil {
		t.Fatal("FundInfo is nil")
	}
	if result.FundProfile == nil {
		t.Fatal("FundProfile is nil")
	}

	// Fund identity (from page HTML)
	if result.FundInfo.Name != "iShares MSCI World Momentum Factor UCITS ETF" {
		t.Errorf("FundInfo.Name = %q", result.FundInfo.Name)
	}
	// Symbol comes from the keyFundFacts bbeqtick data point
	if result.FundInfo.Symbol != "IWMO LN" {
		t.Errorf("FundInfo.Symbol = %q, want %q", result.FundInfo.Symbol, "IWMO LN")
	}

	// Profile from the keyFundFacts JSON
	p := result.FundProfile
	if p.Isin != "IE00BP3QZ825" {
		t.Errorf("Isin = %q", p.Isin)
	}
	if p.TotalNetAssets != 6022456334 {
		t.Errorf("TotalNetAssets = %v", p.TotalNetAssets)
	}
	if !p.InceptionDate.Equal(time.Date(2014, 10, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("InceptionDate = %v", p.InceptionDate)
	}
	if p.Benchmark != "MSCI World Momentum index (Net)" {
		t.Errorf("Benchmark = %q", p.Benchmark)
	}
	if p.BaseCurrency != "USD" {
		t.Errorf("BaseCurrency = %q", p.BaseCurrency)
	}
	// TER is not in the JSON — it must come from the page HTML overlay
	if p.AnnualExpenseRatio != 0.0025 {
		t.Errorf("AnnualExpenseRatio = %v, want 0.0025 (fraction for 0.25%%, from page HTML)", p.AnnualExpenseRatio)
	}

	// Holdings from the holdings JSON (3 equities + 1 cash row)
	if len(result.Holdings) != 4 {
		t.Fatalf("len(Holdings) = %d, want 4", len(result.Holdings))
	}
	if result.Holdings[0].Symbol != "MU" {
		t.Errorf("Holdings[0].Symbol = %q", result.Holdings[0].Symbol)
	}
	if result.Holdings[0].ISIN != "US5951121038" {
		t.Errorf("Holdings[0].ISIN = %q (JSON provides per-holding ISINs)", result.Holdings[0].ISIN)
	}
	if result.Holdings[0].Percent != 6.57 {
		t.Errorf("Holdings[0].Percent = %v", result.Holdings[0].Percent)
	}
	if result.Holdings[0].Shares != 350601 {
		t.Errorf("Holdings[0].Shares = %v", result.Holdings[0].Shares)
	}
	if result.Holdings[0].Price != 971 {
		t.Errorf("Holdings[0].Price = %v", result.Holdings[0].Price)
	}
	cash := result.Holdings[3]
	if cash.Symbol != "USD" || cash.ISIN != "-" {
		t.Errorf("cash row = %+v", cash)
	}

	// Allocations derived from holdings
	sectors := map[string]float64{}
	for _, s := range result.Sectors {
		sectors[s.Sector] = s.Percent
	}
	if sectors["Information Technology"] != 10.43 {
		t.Errorf("IT sector = %v, want 10.43", sectors["Information Technology"])
	}
	if sectors["Cash"] != 0.05 {
		t.Errorf("Cash sector = %v, want 0.05", sectors["Cash"])
	}
	countries := map[string]float64{}
	for _, c := range result.CountryAllocation {
		countries[c.Country] = c.Percent
	}
	if countries["United States"] != 12.36 {
		t.Errorf("United States = %v, want 12.36", countries["United States"])
	}

	// As-of date: the holdings snapshot date (27/Aug/2026) wins over the
	// page-level date (29/May/2026)
	if !result.AsOfDate.Equal(time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AsOfDate = %v, want 2026-08-27 (holdings snapshot)", result.AsOfDate)
	}

	// Characteristics from the page HTML
	if result.Characteristics == nil {
		t.Fatal("Characteristics is nil")
	}
	if result.Characteristics.NumberOfHoldings != 352 {
		t.Errorf("NumberOfHoldings = %v", result.Characteristics.NumberOfHoldings)
	}
	if result.Characteristics.PriceToEarnings != 29.16 {
		t.Errorf("PriceToEarnings = %v", result.Characteristics.PriceToEarnings)
	}
}

func TestExtractor_Extract_Phase1Failure(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(func(url string) (string, error) {
		return "", fmt.Errorf("HTTP 404: not found")
	})

	_, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/999999/invalid-fund")
	if err == nil {
		t.Error("expected error for Phase 1 failure")
	}
}

func TestExtractor_Extract_Phase2Failure(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(fetchSampleByComponent(t, nil, map[string]error{
		"keyFundFacts": fmt.Errorf("HTTP 500: keyFundFacts error"),
	}))

	_, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err == nil {
		t.Error("expected error for Phase 2 failure")
	} else if !strings.Contains(err.Error(), "key fund facts") {
		t.Errorf("expected key fund facts error, got: %v", err)
	}
}

func TestExtractor_Extract_Phase3Failure(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(fetchSampleByComponent(t, nil, map[string]error{
		"holdings": fmt.Errorf("HTTP 500: holdings error"),
	}))

	_, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err == nil {
		t.Error("expected error for Phase 3 failure")
	} else if !strings.Contains(err.Error(), "holdings") {
		t.Errorf("expected holdings error, got: %v", err)
	}
}

func TestExtractor_Extract_EmptyHoldings(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(fetchSampleByComponent(t, map[string]string{
		"holdings": emptyHoldingsJSON,
	}, nil))

	result, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err != nil {
		t.Fatalf("expected success with empty holdings, got error: %v", err)
	}
	if len(result.Holdings) != 0 {
		t.Errorf("expected 0 holdings, got %d", len(result.Holdings))
	}
}

func TestExtractor_Extract_MissingProductDataConfig(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(func(url string) (string, error) {
		return sampleProductPageHTMLLegacy, nil
	})

	_, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err == nil {
		t.Error("expected error when the page has no product data API config")
	} else if !strings.Contains(err.Error(), "product data config") {
		t.Errorf("expected product data config error, got: %v", err)
	}
}

func TestExtractor_Extract_AsOfDatePageFallback(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(fetchSampleByComponent(t, map[string]string{
		"holdings": sampleHoldingsJSONWithoutAsOfDate,
	}, nil))

	result, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	// Without a holdings snapshot date, the page-level date is used
	if !result.AsOfDate.Equal(time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AsOfDate = %v, want 2026-05-29 (page fallback)", result.AsOfDate)
	}
}

func TestExtractor_Extract_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	e := NewExtractor()
	_, err := e.Extract(ctx, "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestEnsureSwitchLocale(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantQ    map[string]string // expected query params (all must be present)
		notWantQ string            // param that must NOT be added (already present case)
	}{
		{
			name: "adds params",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/test",
			wantQ: map[string]string{
				"switchLocale":         "y",
				"siteEntryPassthrough": "true",
			},
		},
		{
			name: "preserves existing params",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/test?foo=bar",
			wantQ: map[string]string{
				"foo":                  "bar",
				"switchLocale":         "y",
				"siteEntryPassthrough": "true",
			},
		},
		{
			name: "already has switchLocale",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/test?switchLocale=y&siteEntryPassthrough=true",
			wantQ: map[string]string{
				"switchLocale":         "y",
				"siteEntryPassthrough": "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureSwitchLocale(tt.url)
			gotURL, err := url.Parse(got)
			if err != nil {
				t.Fatalf("parsed result is not a valid URL: %v", err)
			}
			for k, v := range tt.wantQ {
				if gotURL.Query().Get(k) != v {
					t.Errorf("query param %q = %q, want %q", k, gotURL.Query().Get(k), v)
				}
			}
		})
	}
}

// fetchSampleByComponent routes product data API requests by their
// component query parameter and serves the matching sample payload.
// Overrides win over the defaults: bodyOverrides (string bodies) and
// errorOverrides (fetch errors) per component.
func fetchSampleByComponent(
	t *testing.T,
	bodyOverrides map[string]string,
	errorOverrides map[string]error,
) func(string) (string, error) {
	t.Helper()

	defaults := map[string]string{
		"keyFundFacts": sampleKeyFundFactsJSON,
		"holdings":     sampleHoldingsJSON,
	}

	return func(u string) (string, error) {
		comp := ""
		switch {
		case strings.Contains(u, "component=keyFundFacts"):
			comp = "keyFundFacts"
		case strings.Contains(u, "component=holdings"):
			comp = "holdings"
		}
		if comp == "" {
			return sampleProductPageHTML, nil
		}
		if err, ok := errorOverrides[comp]; ok {
			return "", err
		}
		if body, ok := bodyOverrides[comp]; ok {
			return body, nil
		}
		return defaults[comp], nil
	}
}

// --- Sample Data ---
//
// The page fixture mirrors the 2026-08 iShares "onedes" layout: the product
// data API config is embedded (HTML-entity-escaped, as served), key-facts
// values live in data-item table rows, and the legacy key-value tables are
// gone. The profile and holdings fixtures mirror the product data JSON API
// response envelope.

const sampleProductPageHTML = `
<html>
<head><title>iShares MSCI World Momentum Factor UCITS ETF | iShares UK</title></head>
<body>
<walrus-context contextid="pageConfig" value="{&quot;context&quot;:{&quot;productId&quot;:&quot;270051&quot;},&quot;services&quot;:{&quot;apiHost&quot;:&quot;https://www.blackrock.com/varnish-api/uk-retail01-product-data/product-data/api/v2/get-product-data?&quot;}}"></walrus-context>
<walrus-context contextid="componentConfig" value="{&quot;user_types&quot;:{&quot;individual&quot;:&quot;individual&quot;},&quot;productDataParams&quot;:{&quot;appSubType&quot;:&quot;ISHARES&quot;,&quot;appType&quot;:&quot;PRODUCT_PAGE&quot;,&quot;locale&quot;:&quot;en_GB&quot;,&quot;targetSite&quot;:&quot;ishares-uk&quot;,&quot;userType&quot;:&quot;individual&quot;}}"></walrus-context>
<h1>iShares MSCI World Momentum Factor UCITS ETF</h1>
<table class="key-facts">
<tr class="data-item oneds-body-m-compact col-totalNetAssetsFundLevel" data-itemid="keyFundFacts-row-totalNetAssetsFundLevel" data-itemname="totalNetFactsFundLevel" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Fund Level Net Assets</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">USD 6,022,456,334</td>
</tr>
<tr class="data-item oneds-body-m-compact col-totalNetAssets" data-itemid="keyFundFacts-row-totalNetAssets" data-itemname="totalNetAssets" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Net Assets</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">USD <!-- -->6,022,456,334</td>
</tr>
<tr class="data-item oneds-body-m-compact col-emeaMgt" data-itemid="keyFundFacts-row-emeaMgt" data-itemname="emeaMgt" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Annual Management Fee (TER)</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1"><span class="value">0.25%</span></td>
</tr>
</table>
<table class="characteristics">
<tr class="data-item oneds-body-m-compact col-numHoldings" data-itemid="characteristics-row-numHoldings" data-itemname="numHoldings" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Number of Holdings</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">352</td>
</tr>
<tr class="data-item oneds-body-m-compact col-priceEarnings" data-itemid="characteristics-row-priceEarnings" data-itemname="priceEarnings" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">P/E Ratio</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">29.16</td>
</tr>
<tr class="data-item oneds-body-m-compact col-priceBook" data-itemid="characteristics-row-priceBook" data-itemname="priceBook" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">P/B Ratio</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">3.89</td>
</tr>
</table>
<div class="as-of-date oneds-body-s-compact">as of 29/May/2026</div>
</body>
</html>
`

// sampleProductPageHTMLLegacy is a product page without the embedded product
// data API config (pre-redesign layout).
const sampleProductPageHTMLLegacy = `
<html>
<head><title>iShares MSCI World Momentum Factor UCITS ETF | iShares UK</title></head>
<body>
<h1>iShares MSCI World Momentum Factor UCITS ETF</h1>
<table class="key-facts">
<tr><td>ISIN</td><td>IE00BP3QZ825</td></tr>
<tr><td>Net Assets</td><td>USD 5,181,115,355 (as of 29/May/2026)</td></tr>
</table>
<h3>Fund Holdings as of,"29/May/2026"</h3>
</body>
</html>
`

const sampleKeyFundFactsJSON = `{
  "fundName": "iShares MSCI World Momentum Factor UCITS ETF",
  "componentsByNameMap": {
    "keyFundFacts": {
      "containersByNameMap": {
        "default": {
          "dataPointsByNameMap": {
            "isin": {"name": "isin", "formattedValue": "IE00BP3QZ825"},
            "totalNetAssets": {"name": "totalNetAssets", "formattedValue": "USD 6,022,456,334"},
            "inceptionDate": {"name": "inceptionDate", "formattedValue": "03/Oct/2014"},
            "assetClass": {"name": "assetClass", "formattedValue": "Equity"},
            "sfdr": {"name": "sfdr", "formattedValue": "Other"},
            "useOfProfitsCode": {"name": "useOfProfitsCode", "formattedValue": "Accumulating"},
            "domicile": {"name": "domicile", "formattedValue": "Ireland"},
            "rebalanceFrequency": {"name": "rebalanceFrequency", "formattedValue": "Quarterly"},
            "fundmanager": {"name": "fundmanager", "formattedValue": "BlackRock Asset Management Ireland Limited"},
            "fundCustodian": {"name": "fundCustodian", "formattedValue": "State Street Custodial Services (Ireland) Limited"},
            "bbeqtick": {"name": "bbeqtick", "formattedValue": "IWMO LN"},
            "indexSeriesName": {"name": "indexSeriesName", "formattedValue": "MSCI World Momentum index (Net)"},
            "productStructure": {"name": "productStructure", "formattedValue": "Physical"},
            "fundMethodologyTypeCode": {"name": "fundMethodologyTypeCode", "formattedValue": "Optimised"},
            "issuingCompany": {"name": "issuingCompany", "formattedValue": "iShares IV plc"},
            "baseCurrencyCode": {"name": "baseCurrencyCode", "formattedValue": "USD"}
          }
        }
      }
    }
  }
}`

const sampleHoldingsJSON = `{
  "fundName": "iShares MSCI World Momentum Factor UCITS ETF",
  "componentsByNameMap": {
    "holdings": {
      "containersByNameMap": {
        "all": {
          "dataPointsByNameMap": {
            "asOfDate": {"name": "asOfDate", "formattedValue": "27/Aug/2026", "value": "20260827"},
            "ticker": {"name": "ticker", "formattedValue": ["MU", "AAPL", "JNJ", "USD"]},
            "issueName": {"name": "issueName", "formattedValue": ["MICRON TECHNOLOGY", "APPLE", "JOHNSON & JOHNSON", "Cash"]},
            "holdingPercent": {"name": "holdingPercent", "formattedValue": ["6.57", "3.86", "1.93", "0.05"]},
            "sectorName": {"name": "sectorName", "formattedValue": ["Information Technology", "Information Technology", "Healthcare", "Cash"]},
            "assetClass": {"name": "assetClass", "formattedValue": ["Equity", "Equity", "Equity", "Cash"]},
            "marketValue": {"name": "marketValue", "formattedValue": ["340,433,571.00", "200,000,000.00", "100,000,000.00", "2,600,000.00"]},
            "notionalValue": {"name": "notionalValue", "formattedValue": ["340,433,571.00", "200,000,000.00", "100,000,000.00", "2,600,000.00"]},
            "unitsHeld": {"name": "unitsHeld", "formattedValue": ["350,601.00", "1,000,000.00", "500,000.00", "2,600,000.00"]},
            "unitPrice": {"name": "unitPrice", "formattedValue": ["971.00", "200.00", "200.00", "1.00"]},
            "countryOfRisk": {"name": "countryOfRisk", "formattedValue": ["United States", "United States", "United States", "Cash"]},
            "exchange": {"name": "exchange", "formattedValue": ["NASDAQ", "NASDAQ", "NYSE", ""]},
            "marketCurrencyCode": {"name": "marketCurrencyCode", "formattedValue": ["USD", "USD", "USD", "USD"]},
            "isin": {"name": "isin", "formattedValue": ["US5951121038", "US0378331005", "US4781601046", "-"]}
          }
        }
      }
    }
  }
}`

const sampleHoldingsAsOfDateLine = `            "asOfDate": {"name": "asOfDate", "formattedValue": "27/Aug/2026", "value": "20260827"},
`

// sampleHoldingsJSONWithoutAsOfDate is sampleHoldingsJSON with the asOfDate
// data point removed (exercises the page-level as-of date fallback).
var sampleHoldingsJSONWithoutAsOfDate = strings.Replace(sampleHoldingsJSON, sampleHoldingsAsOfDateLine, "", 1)

// emptyHoldingsJSON is a holdings response with no holding rows.
const emptyHoldingsJSON = `{
  "fundName": "iShares MSCI World Momentum Factor UCITS ETF",
  "componentsByNameMap": {
    "holdings": {
      "containersByNameMap": {
        "all": {
          "dataPointsByNameMap": {
            "asOfDate": {"name": "asOfDate", "formattedValue": "27/Aug/2026", "value": "20260827"},
            "ticker": {"name": "ticker", "formattedValue": []},
            "issueName": {"name": "issueName", "formattedValue": []}
          }
        }
      }
    }
  }
}`
