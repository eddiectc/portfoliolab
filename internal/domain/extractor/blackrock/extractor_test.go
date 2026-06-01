package blackrock

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
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
		w.Write([]byte(sampleProductPageHTML))
	}))
	defer server.Close()

	e := NewExtractor()
	e.client = &Client{minDelay: 0}

	requestCount := 0
	e.client.SetFetchFunc(func(url string) (string, error) {
		requestCount++
		if requestCount == 1 {
			return sampleProductPageHTML, nil
		}
		return sampleHoldingsJSON, nil
	})

	result, err := e.Extract(context.Background(), server.URL+"/product")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.FundInfo == nil {
		t.Error("FundInfo is nil")
	}
	if result.FundProfile == nil {
		t.Error("FundProfile is nil")
	}
	if len(result.Holdings) == 0 {
		t.Error("Holdings is empty")
	}
	if len(result.Sectors) == 0 {
		t.Error("Sectors is empty")
	}
	if len(result.CountryAllocation) == 0 {
		t.Error("CountryAllocation is empty")
	}
	if result.Characteristics == nil {
		t.Error("Characteristics is nil")
	}
	if result.AsOfDate.IsZero() {
		t.Error("AsOfDate is zero")
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

	requestCount := 0
	e.client.SetFetchFunc(func(url string) (string, error) {
		requestCount++
		if requestCount == 1 {
			return sampleProductPageHTML, nil
		}
		return "", fmt.Errorf("HTTP 500: holdings API error")
	})

	_, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err == nil {
		t.Error("expected error for Phase 2 failure")
	}
}

func TestExtractor_Extract_EmptyHoldings(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}

	emptyHoldingsJSON := `{"asOfDate": "20260529", "aaData": []}`

	requestCount := 0
	e.client.SetFetchFunc(func(url string) (string, error) {
		requestCount++
		if requestCount == 1 {
			return sampleProductPageHTML, nil
		}
		return emptyHoldingsJSON, nil
	})

	result, err := e.Extract(context.Background(), "https://www.ishares.com/uk/individual/en/products/270051/test")
	if err != nil {
		t.Fatalf("expected success with empty holdings, got error: %v", err)
	}
	if len(result.Holdings) != 0 {
		t.Errorf("expected 0 holdings, got %d", len(result.Holdings))
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
				"switchLocale":           "y",
				"siteEntryPassthrough": "true",
			},
		},
		{
			name: "preserves existing params",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/test?foo=bar",
			wantQ: map[string]string{
				"foo":                    "bar",
				"switchLocale":           "y",
				"siteEntryPassthrough": "true",
			},
		},
		{
			name: "already has switchLocale",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/test?switchLocale=y&siteEntryPassthrough=true",
			wantQ: map[string]string{
				"switchLocale":           "y",
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

// --- Sample Data ---

const sampleProductPageHTML = `
<html>
<head><title>iShares MSCI World Momentum Factor UCITS ETF | iShares UK</title></head>
<body>
<h1>iShares MSCI World Momentum Factor UCITS ETF</h1>
<table class="key-facts">
<tr><td>Net Assets</td><td>USD 5,181,115,355 (as of 29/May/2026)</td></tr>
<tr><td>Inception Date</td><td>03/Oct/2014</td></tr>
<tr><td>Asset Class</td><td>Equity</td></tr>
<tr><td>SFDR Classification</td><td>Other</td></tr>
<tr><td>Total Expense Ratio</td><td>0.25%</td></tr>
<tr><td>Use of Income</td><td>Accumulating</td></tr>
<tr><td>Domicile</td><td>Ireland</td></tr>
<tr><td>Rebalance Frequency</td><td>Quarterly</td></tr>
<tr><td>Fund Manager</td><td>BlackRock Asset Management Ireland Limited</td></tr>
<tr><td>Custodian</td><td>State Street Custodial Services (Ireland) Limited</td></tr>
<tr><td>Bloomberg Ticker</td><td>IWMO LN</td></tr>
<tr><td>Benchmark Index</td><td>MSCI World Momentum Index (Net)</td></tr>
<tr><td>ISIN</td><td>IE00BP3QZ825</td></tr>
<tr><td>Product Structure</td><td>Physical</td></tr>
<tr><td>Methodology</td><td>Optimised</td></tr>
<tr><td>Issuing Company</td><td>iShares IV plc</td></tr>
</table>
<table class="characteristics">
<tr><td>Number of Holdings</td><td>352 (as of 29/May/2026)</td></tr>
<tr><td>P/E Ratio</td><td>29.16 (as of 29/May/2026)</td></tr>
<tr><td>P/B Ratio</td><td>3.89 (as of 29/May/2026)</td></tr>
<tr><td>3y Beta</td><td>0.999 (as of 30/Apr/2026)</td></tr>
<tr><td>Standard Deviation (3y)</td><td>16.62% (as of 30/Apr/2026)</td></tr>
</table>
<h3>Fund Holdings as of,"29/May/2026"</h3>
<a href="/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf/1506575576011.ajax?fileType=csv&fileName=IWMO_holdings&dataType=fund">Download Holdings</a>
</body>
</html>
`

const sampleHoldingsJSON = `{
  "asOfDate": "20260529",
  "aaData": [
    [
      "MU",
      "MICRON TECHNOLOGY INC",
      "Information Technology",
      "Equity",
      {"display": "USD 340,433,571.00", "raw": 340433571},
      {"display": "6.57", "raw": 6.57076},
      {"display": "340,433,571.00", "raw": 340433571},
      {"display": "350,601.00", "raw": 350601},
      "US5951121038",
      {"display": "971.00", "raw": 971},
      "United States",
      "NASDAQ",
      "USD"
    ],
    [
      "AAPL",
      "APPLE INC",
      "Information Technology",
      "Equity",
      {"display": "USD 200,000,000.00", "raw": 200000000},
      {"display": "3.86", "raw": 3.86},
      {"display": "200,000,000.00", "raw": 200000000},
      {"display": "1,000,000.00", "raw": 1000000},
      "US0378331005",
      {"display": "200.00", "raw": 200},
      "United States",
      "NASDAQ",
      "USD"
    ],
    [
      "JNJ",
      "JOHNSON & JOHNSON",
      "Healthcare",
      "Equity",
      {"display": "USD 100,000,000.00", "raw": 100000000},
      {"display": "1.93", "raw": 1.93},
      {"display": "100,000,000.00", "raw": 100000000},
      {"display": "500,000.00", "raw": 500000},
      "US4781601046",
      {"display": "200.00", "raw": 200},
      "United States",
      "NYSE",
      "USD"
    ]
  ]
}`
