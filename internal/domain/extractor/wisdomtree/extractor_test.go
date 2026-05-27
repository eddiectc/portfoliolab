package wisdomtree

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

	if !e.Match("https://www.wisdomtree.eu/en-gb/etfs/wmgt") {
		t.Error("expected wisdomtree.eu URL to match")
	}
	if e.Match("https://www.vanguard.com/etfs/vo") {
		t.Error("expected vanguard.com URL to not match")
	}
}

func TestExtractor_Extract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sampleHTML))
	}))
	defer server.Close()

	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(func(url string) (string, error) {
		resp, err := server.Client().Get(url)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	})

	result, err := e.Extract(context.Background(), server.URL+"/etfs/wmgt")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.FundInfo == nil {
		t.Error("FundInfo is nil")
	}
	if len(result.Holdings) == 0 {
		t.Error("Holdings is empty")
	}
	if len(result.NavHistory) == 0 {
		t.Error("NavHistory is empty")
	}
	if len(result.Themes) == 0 {
		t.Error("Themes is empty")
	}
	if len(result.Sectors) == 0 {
		t.Error("Sectors is empty")
	}
	if len(result.CountryAllocation) == 0 {
		t.Error("CountryAllocation is empty")
	}
	if result.MarketCap == nil {
		t.Error("MarketCap is nil")
	}
	if result.Characteristics == nil {
		t.Error("Characteristics is nil")
	}
}

func TestExtractor_Extract_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(func(url string) (string, error) {
		resp, err := server.Client().Get(url)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	})

	_, err := e.Extract(context.Background(), server.URL+"/etfs/wmgt")
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestExtractor_Extract_Atomic(t *testing.T) {
	// HTML with fundInfo but no holdings data should fail entirely
	html := `var fundInfo = {'symbol':'WMGT', 'name':'Test'}` +
		`<td>Total AUM of fund</td><td>1.5</td>` +
		`<td class="key">TER</td><td>0.40</td>` +
		`<th>Net Asset Value</th><th>22 May 2026</th>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer server.Close()

	e := NewExtractor()
	e.client = &Client{minDelay: 0}
	e.client.SetFetchFunc(func(url string) (string, error) {
		resp, err := server.Client().Get(url)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	})

	_, err := e.Extract(context.Background(), server.URL+"/etfs/wmgt")
	if err == nil {
		t.Error("expected error for missing holdings data (atomic extraction)")
	}
}

// Minimal sample HTML for integration test
const sampleHTML = `
<script>
var fundInfo = {'symbol':'WMGT', 'name':'WisdomTree Megatrends UCITS ETF'};
var fundHoldingsData = 'date,Weight,Security Description\n5/11/2026,0.0137223,"Prysmian SpA"\n5/11/2026,0.0132832,"Bloom Energy Corp"\n5/11/2026,0.0025826,"CASH W-O"';
var fundMarketDataB123 = 'date,fund_ticker,close_price_adj,volume_adj,nav\n5/11/2026,WMGT LN,,,45.846\n5/8/2026,WMGT LN,,,45.1556';
var fundThemeData = 'date,Weight,Security Description\n5/8/2026,0.0884090,"Grid Infrastructure"\n5/8/2026,0.0834350,"Sustainable Energy Storage"';
var fundSectorsData = 'date,securityName,weight,Sector,wgtSector\n5/11/2026,"Prysmian",0.0137,"Industrials",0.5\n5/11/2026,"Bloom",0.0132,"Technology",0.3';
</script>
<table>
<tr><td>Total AUM of fund</td><td><span class="value currency positive">$1,500,000</span></td></tr>
<tr><td class="key">TER</td><td>0.40%</td></tr>
<tr><td class="key">Inception Date</td><td>01/06/2023</td></tr>
<tr><td class="key">Fund Umbrella</td><td>WisdomTree Issuer ICAV</td></tr>
<tr><td class="key">Legal Form</td><td>Irish Collective Asset-management Vehicle (ICAV)</td></tr>
</table>
<table>
<tr><th>Net Asset Value</th><th>22 May 2026</th></tr>
</table>
<section id="country-allocation-section">
<table class="table table-striped-customized">
<thead><tr><th class="key">Country</th><th class="value">Weight</th></tr></thead>
<tbody>
<tr><td class="key">1. United States</td><td class="value"><span class="value percent positive">40.54%</span></td></tr>
<tr><td class="key">2. United Kingdom</td><td class="value"><span class="value percent positive">12.30%</span></td></tr>
<tr><td class="key">3. Japan</td><td class="value"><span class="value percent positive">8.15%</span></td></tr>
</tbody>
</table>
</section>
<section id="fund-facts-section">
<table class="table table-striped-customized">
<thead><tr><th class="key">Market Capitalization</th><th class="value">As of 22 May 2026</th></tr></thead>
<tbody>
<tr><td class="key">Total Market Capitalization ($ Trillion)</td><td class="value">58.68</td></tr>
<tr><td colspan="2"><strong>Fund MarketCap Breakdown</strong></td></tr>
<tr><td class="key shifted">Large Cap (&gt; $10 Billion)</td><td class="value">64.42%</td></tr>
<tr><td class="key shifted">Mid Cap ($2-$10 Billion)</td><td class="value">25.18%</td></tr>
<tr><td class="key shifted">Small Cap (&lt; $2 Billion)</td><td class="value">10.40%</td></tr>
</tbody>
</table>
<table class="table table-striped-customized">
<thead><tr><th class="key">Fund Characteristics</th><th class="value">As of 22 May 2026</th></tr></thead>
<tbody>
<tr><td class="key">*Dividend Yield</td><td class="value">0.94</td></tr>
<tr><td class="key">Price/Earnings</td><td class="value">69.64</td></tr>
<tr><td class="key">Price/Book</td><td class="value">4.06</td></tr>
<tr><td class="key">Price/Sales</td><td class="value">2.61</td></tr>
<tr><td class="key">Price/Cash Flow</td><td class="value">28.69</td></tr>
</tbody>
</table>
</section>
`

func TestExtractFromHTML(t *testing.T) {
	result, err := extractFromHTML(sampleHTML)
	if err != nil {
		t.Fatalf("extractFromHTML failed: %v", err)
	}

	// FundInfo
	if result.FundInfo.Symbol != "WMGT" {
		t.Errorf("expected symbol WMGT, got %s", result.FundInfo.Symbol)
	}
	if result.FundInfo.Name != "WisdomTree Megatrends UCITS ETF" {
		t.Errorf("unexpected name: %s", result.FundInfo.Name)
	}

	// Holdings (cash filtered out)
	if len(result.Holdings) != 2 {
		t.Errorf("expected 2 holdings (cash filtered), got %d", len(result.Holdings))
	}
	if len(result.Holdings) > 0 {
		if result.Holdings[0].Name != "Prysmian SpA" {
			t.Errorf("unexpected first holding: %s", result.Holdings[0].Name)
		}
	}

	// NAV
	if len(result.NavHistory) != 2 {
		t.Errorf("expected 2 nav points, got %d", len(result.NavHistory))
	}

	// Themes
	if len(result.Themes) != 2 {
		t.Errorf("expected 2 themes, got %d", len(result.Themes))
	}

	// Sectors
	if len(result.Sectors) != 2 {
		t.Errorf("expected 2 sectors, got %d", len(result.Sectors))
	}

	// AsOfDate
	if result.AsOfDate.IsZero() {
		t.Error("AsOfDate is zero")
	}

	// Country Allocation
	if len(result.CountryAllocation) == 0 {
		t.Error("CountryAllocation is empty")
	}
	if len(result.CountryAllocation) > 0 {
		if result.CountryAllocation[0].Country != "United States" {
			t.Errorf("expected first country United States, got %s", result.CountryAllocation[0].Country)
		}
		if result.CountryAllocation[0].Percent != 40.54 {
			t.Errorf("expected first country percent 40.54, got %f", result.CountryAllocation[0].Percent)
		}
	}

	// Market Cap
	if result.MarketCap == nil {
		t.Error("MarketCap is nil")
	} else {
		if result.MarketCap.Total != 58.68 {
			t.Errorf("expected market cap total 58.68, got %f", result.MarketCap.Total)
		}
		if result.MarketCap.Large != 64.42 {
			t.Errorf("expected large cap 64.42, got %f", result.MarketCap.Large)
		}
	}

	// Characteristics
	if result.Characteristics == nil {
		t.Error("Characteristics is nil")
	} else {
		if result.Characteristics.DividendYield != 0.94 {
			t.Errorf("expected dividend yield 0.94, got %f", result.Characteristics.DividendYield)
		}
		if result.Characteristics.PriceToEarnings != 69.64 {
			t.Errorf("expected P/E 69.64, got %f", result.Characteristics.PriceToEarnings)
		}
	}

	// FundProfile Family and LegalType
	if result.FundProfile.Family != "WisdomTree Issuer ICAV" {
		t.Errorf("expected family 'WisdomTree Issuer ICAV', got %q", result.FundProfile.Family)
	}
	if result.FundProfile.LegalType != "Irish Collective Asset-management Vehicle (ICAV)" {
		t.Errorf("unexpected legal type: %q", result.FundProfile.LegalType)
	}
}
