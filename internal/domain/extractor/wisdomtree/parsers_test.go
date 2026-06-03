package wisdomtree

import (
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

func TestParseFundInfo(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    *extractor.FundInfo
		wantErr bool
	}{
		{
			name: "basic fund info",
			html: `var fundInfo = {'symbol':'WMGT', 'name':'WisdomTree Megatrends UCITS ETF'}`,
			want: &extractor.FundInfo{
				Symbol: "WMGT",
				Name:   "WisdomTree Megatrends UCITS ETF",
			},
		},
		{
			name: "fund info with hash suffix",
			html: `var fundInfoA1B2C3 = {'symbol':'WMST', 'name':'WisdomTree STOXX Europe'}`,
			want: &extractor.FundInfo{
				Symbol: "WMST",
				Name:   "WisdomTree STOXX Europe",
			},
		},
		{
			name:    "missing fund info",
			html:    `<script>var somethingElse = 'data'</script>`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty html",
			html:    ``,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFundInfo(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Symbol != tt.want.Symbol {
				t.Errorf("symbol = %q, want %q", got.Symbol, tt.want.Symbol)
			}
			if got.Name != tt.want.Name {
				t.Errorf("name = %q, want %q", got.Name, tt.want.Name)
			}
		})
	}
}

func TestParseHoldings(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantLen int
		wantErr bool
	}{
		{
			name: "basic holdings with cash filter",
			html: `var fundHoldingsData = 'date,Weight,Security Description\n5/11/2026,0.0137,"Apple Inc"\n5/11/2026,0.0025,"CASH W-O"\n5/11/2026,0.0100,"Microsoft Corp"'`,
			wantLen: 2, // cash filtered out
		},
		{
			name: "empty holdings (header only)",
			html: `var fundHoldingsData = 'date,Weight,Security Description'`,
			wantLen: 0,
		},
		{
			name:    "missing holdings data",
			html:    `<script>var otherData = 'something'</script>`,
			wantErr: true,
		},
		{
			name: "holdings with escaped quotes",
			html: `var fundHoldingsData = 'date,Weight,Security Description\n5/11/2026,0.0100,\"Johnson \u0026 Johnson"'`,
			wantLen: 1,
		},
		{
			name: "currency positions filtered",
			html: `var fundHoldingsData = 'date,Weight,Security Description\n5/11/2026,0.001,"APPLE"\n5/11/2026,0.0005,"JAPANESE YEN"\n5/11/2026,0.0003,"SWISS FRANC"'`,
			wantLen: 1, // only Apple remains
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHoldings(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d holdings, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseNavHistory(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantLen int
		wantNil bool
		wantErr bool
	}{
		{
			name: "basic nav history",
			html: `var fundMarketDataX1 = 'date,fund_ticker,close_price_adj,volume_adj,nav\n5/11/2026,WMGT LN,,,45.846\n5/8/2026,WMGT LN,,,45.1556'`,
			wantLen: 2,
		},
		{
			name: "nav with empty nav values skipped",
			html: `var fundMarketDataA = 'date,fund_ticker,close_price_adj,volume_adj,nav\n5/11/2026,WMGT LN,,,45.846\n5/8/2026,WMGT LN,,,'`,
			wantLen: 1, // empty nav skipped
		},
		{
			name:    "missing nav data",
			html:    `<script>var other = 'data'</script>`,
			wantNil: true, // optional section — nil, nil when not found
		},
		{
			name: "nav with hash suffix",
			html: `var fundMarketDataB123 = 'date,fund_ticker,close_price_adj,volume_adj,nav\n5/11/2026,WMGT LN,,,45.846'`,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNavHistory(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil result, got %d items", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d nav points, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseThemes(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantLen int
		wantNil bool
		wantErr bool
	}{
		{
			name: "basic themes",
			html: `var fundThemeData = 'date,Weight,Security Description\n5/8/2026,0.0884,"Grid Infrastructure"\n5/8/2026,0.0834,"Sustainable Energy"'`,
			wantLen: 2,
		},
		{
			name:    "missing theme data",
			html:    `<script>var other = 'data'</script>`,
			wantNil: true,
		},
		{
			name: "single theme",
			html: `var fundThemeData = 'date,Weight,Security Description\n5/8/2026,1.0,"AI"'`,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseThemes(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil result, got %d items", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d themes, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseSectors(t *testing.T) {
	tests := []struct {
		name       string
		html       string
		wantLen    int
		wantValues map[string]float64 // sector name -> expected percent
		wantNil    bool
		wantErr    bool
	}{
		{
			name: "basic sectors — wgtSector repeated, take first",
			html: `var fundSectorsData = 'date,securityName,weight,Sector,wgtSector\n5/11/2026,"A",0.01,"Technology",0.05\n5/11/2026,"B",0.02,"Technology",0.05\n5/11/2026,"C",0.01,"Healthcare",0.12'`,
			wantLen: 2,
			wantValues: map[string]float64{
				"Technology": 5.0,   // 0.05 * 100
				"Healthcare": 12.0, // 0.12 * 100
			},
		},
		{
			name:    "missing sectors data",
			html:    `<script>var other = 'data'</script>`,
			wantNil: true,
		},
		{
			name: "empty sector names skipped",
			html: `var fundSectorsData = 'date,securityName,weight,Sector,wgtSector\n5/11/2026,"A",0.01,"",0.5\n5/11/2026,"B",0.02,"Tech",0.3'`,
			wantLen: 1, // empty sector skipped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSectors(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil result, got %d items", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d sectors, want %d", len(got), tt.wantLen)
			}
			if tt.wantValues != nil {
				gotMap := make(map[string]float64)
				for _, s := range got {
					gotMap[s.Sector] = s.Percent
				}
				for sector, wantPercent := range tt.wantValues {
					if gotPercent, ok := gotMap[sector]; !ok {
						t.Errorf("missing sector %q", sector)
					} else if gotPercent != wantPercent {
						t.Errorf("%s: got %.2f%%, want %.2f%%", sector, gotPercent, wantPercent)
					}
				}
			}
		})
	}
}

func TestParseAsOfDate(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    time.Time
		wantErr bool
	}{
		{
			name: "standard format",
			html: `<th>Net Asset Value</th><th>22 May 2026</th>`,
			want: time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "abbreviated month",
			html: `<th>Net Asset Value</th><th>02 Jun 2026</th>`,
			want: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "abbreviated month single digit day",
			html: `<th>Net Asset Value</th><th>1 Jun 2026</th>`,
			want: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "with whitespace",
			html: `<th>  Net Asset Value  </th>  <th> 22 May 2026 </th>`,
			want: time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "missing as-of date",
			html:    `<th>Something Else</th><th>22 May 2026</th>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAsOfDate(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFundProfile(t *testing.T) {
	tests := []struct {
		name          string
		html          string
		wantAUM       float64
		wantTER       float64
		wantFamily    string
		wantLegalType string
		wantIsin      string
		wantNil       bool
		wantErr       bool
	}{
		{
			name: "complete profile",
			html: `<table>
<tr><td>Total AUM of fund</td><td><span class="value currency positive">$60,368,055</span></td></tr>
<tr><td class="key">TER</td><td>0.40%</td></tr>
<tr><td class="key">Inception Date</td><td>01/06/2023</td></tr>
<tr><td class="key">Fund Umbrella</td><td>WisdomTree Issuer ICAV</td></tr>
<tr><td class="key">Legal Form</td><td>ICAV</td></tr>
</table>
<table>
<tr><td>ISIN</td><td>IE000YGEAK03</td></tr>
</table>`,
			wantAUM:       60368055,
			wantTER:       0.0040,
			wantFamily:    "WisdomTree Issuer ICAV",
			wantLegalType: "ICAV",
			wantIsin:      "IE000YGEAK03",
		},
		{
			name: "minimal profile (AUM only)",
			html: `<table>
<tr><td>Total AUM of fund</td><td>$1,234,567</td></tr>
</table>`,
			wantAUM: 1234567,
		},
		{
			name: "multiline key cells",
			html: `<table>
<tr><td class="key">
								TER
							</td><td>0.50%</td></tr>
<tr><td class="key">
								Fund Umbrella
							</td><td>Test Fund</td></tr>
</table>`,
			wantTER:  0.0050,
			wantFamily: "Test Fund",
		},
		{
			name:    "no profile data",
			html:    `<table><tr><td>other</td><td>data</td></tr></table>`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFundProfile(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Error("expected nil result")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.TotalNetAssets != tt.wantAUM {
				t.Errorf("AUM = %f, want %f", got.TotalNetAssets, tt.wantAUM)
			}
			if got.AnnualExpenseRatio != tt.wantTER {
				t.Errorf("TER = %f, want %f", got.AnnualExpenseRatio, tt.wantTER)
			}
			if got.Family != tt.wantFamily {
				t.Errorf("Family = %q, want %q", got.Family, tt.wantFamily)
			}
			if got.LegalType != tt.wantLegalType {
				t.Errorf("LegalType = %q, want %q", got.LegalType, tt.wantLegalType)
			}
			if got.Isin != tt.wantIsin {
				t.Errorf("Isin = %q, want %q", got.Isin, tt.wantIsin)
			}
		})
	}
}

func TestParseCountryAllocation(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantLen int
		wantNil bool
		wantErr bool
	}{
		{
			name: "basic countries with numbering",
			html: `<section id="country-allocation-section">
<table>
<tbody>
<tr><td class="key">1. United States</td><td class="value"><span>40.54%</span></td></tr>
<tr><td class="key">2. Japan</td><td class="value"><span>8.15%</span></td></tr>
</tbody>
</table>
</section>`,
			wantLen: 2,
		},
		{
			name:    "missing section",
			html:    `<section id="other-section"></section>`,
			wantNil: true,
		},
		{
			name:    "empty section",
			html:    `<section id="country-allocation-section"></section>`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCountryAllocation(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil result, got %d items", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d countries, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseMarketCap(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		wantTotal float64
		wantNil   bool
		wantErr   bool
	}{
		{
			name: "complete market cap",
			html: `<section id="fund-facts-section">
<tr><td class="key">Total Market Capitalization ($ Trillion)</td><td class="value">58.68</td></tr>
<tr><td class="key shifted">Large Cap (&gt; $10 Billion)</td><td class="value">64.42%</td></tr>
<tr><td class="key shifted">Mid Cap ($2-$10 Billion)</td><td class="value">25.18%</td></tr>
<tr><td class="key shifted">Small Cap (&lt; $2 Billion)</td><td class="value">10.40%</td></tr>
</section>`,
			wantTotal: 58.68,
		},
		{
			name:    "missing data",
			html:    `<section id="fund-facts-section">nothing here</section>`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMarketCap(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Error("expected nil result")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Total != tt.wantTotal {
				t.Errorf("Total = %f, want %f", got.Total, tt.wantTotal)
			}
		})
	}
}

func TestParseFundCharacteristics(t *testing.T) {
	tests := []struct {
		name               string
		html               string
		wantPE             float64
		wantEstimatedPE    float64
		wantNil            bool
		wantErr            bool
	}{
		{
			name: "complete characteristics",
			html: `<table>
<thead><tr><th class="key">Fund Characteristics</th><th class="value">As of 22 May 2026</th></tr></thead>
<tbody>
<tr><td class="key">*Dividend Yield</td><td class="value">0.94</td></tr>
<tr><td class="key">Price/Earnings</td><td class="value">69.64</td></tr>
<tr><td class="key">Estimated Price/Earnings</td><td class="value">34.56</td></tr>
<tr><td class="key">Price/Book</td><td class="value">4.06</td></tr>
<tr><td class="key">Price/Sales</td><td class="value">2.61</td></tr>
<tr><td class="key">Price/Cash Flow</td><td class="value">28.69</td></tr>
</tbody>
</table>`,
			wantPE:          69.64,
			wantEstimatedPE: 34.56,
		},
		{
			name: "partial characteristics",
			html: `<table>
<thead><tr><th class="key">Fund Characteristics</th><th class="value">As of 22 May 2026</th></tr></thead>
<tbody>
<tr><td class="key">Price/Earnings</td><td class="value">15.20</td></tr>
<tr><td class="key">Price/Book</td><td class="value">2.10</td></tr>
</tbody>
</table>`,
			wantPE: 15.20,
		},
		{
			name:    "missing section",
			html:    `<section id="other">no characteristics here</section>`,
			wantNil: true,
		},
		{
			name: "negative values",
			html: `<table>
<thead><tr><th class="key">Fund Characteristics</th><th class="value">As of 22 May 2026</th></tr></thead>
<tbody>
<tr><td class="key">Price/Earnings</td><td class="value">-5.20</td></tr>
<tr><td class="key">Price/Book</td><td class="value">1.50</td></tr>
</tbody>
</table>`,
			wantPE: -5.20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFundCharacteristics(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Error("expected nil result")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.PriceToEarnings != tt.wantPE {
				t.Errorf("PriceToEarnings = %f, want %f", got.PriceToEarnings, tt.wantPE)
			}
			if got.EstimatedPriceToEarnings != tt.wantEstimatedPE {
				t.Errorf("EstimatedPriceToEarnings = %f, want %f", got.EstimatedPriceToEarnings, tt.wantEstimatedPE)
			}
		})
	}
}

func TestIsCashPosition(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"cash", "CASH W-O", true},
		{"euro income", "EURO INCOME A/C", true},
		{"japanese yen", "JAPANESE YEN", true},
		{"chinese renimbi", "CHINESE RENIMBI", true},
		{"cgt adj", "BRL CGT ADJ", true},
		{"regular holding", "Apple Inc", false},
		{"microsoft", "Microsoft Corp", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCashPosition(tt.input)
			if got != tt.expected {
				t.Errorf("isCashPosition(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractModalURL(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "standard modal url",
			html:     `<a data-href="https://www.wisdomtree.eu/en-gb/global/etf-details/modals/all-holdings?id={8B845B79-F55C-4B6A-8D67-CA84E1C19C5B}">`,
			expected: "https://www.wisdomtree.eu/en-gb/global/etf-details/modals/all-holdings?id={8B845B79-F55C-4B6A-8D67-CA84E1C19C5B}",
		},
		{
			name:     "missing modal url",
			html:     `<div>no modal here</div>`,
			expected: "",
		},
		{
			name:     "empty html",
			html:     ``,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractModalURL(tt.html)
			if got != tt.expected {
				t.Errorf("ExtractModalURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractTicker(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"bloomberg format", "NVDA UQ", "NVDA"},
		{"bloomberg with US", "MSFT US", "MSFT"},
		{"bloomberg with UN", "LLY UN", "LLY"},
		{"cusip", "US5128073062", "US5128073062"},
		{"simple ticker", "AAPL", "AAPL"},
		{"empty", "", ""},
		{"with spaces", "  AMD US  ", "AMD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTicker(tt.input)
			if got != tt.expected {
				t.Errorf("extractTicker(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseHoldingsFromModal(t *testing.T) {
	modalHTML := `<script>
var source = [
{"CountryCode":"US ","Weight":0.1426250,"COBDate":"2026-06-02T00:00:00","IdentifierName":"Nvidia Corp","IdentifierTicker":"NVDA UQ","FIGI":null,"SharesPar":"26576","MarketValue":5921664.32,"ContractType":null},
{"CountryCode":"US ","Weight":0.1228107,"COBDate":"2026-06-02T00:00:00","IdentifierName":"Apple Inc","IdentifierTicker":"AAPL UQ","FIGI":null,"SharesPar":"16177","MarketValue":5098990.40,"ContractType":null},
{"CountryCode":"US ","Weight":0.0114614,"COBDate":"2026-06-02T00:00:00","IdentifierName":"LAM RESEARCH CORP","IdentifierTicker":"US5128073062","FIGI":null,"SharesPar":"1423","MarketValue":475865.43,"ContractType":null},
{"CountryCode":"   ","Weight":0.0011587,"COBDate":"2026-06-02T00:00:00","IdentifierName":"CASH W-O","IdentifierTicker":null,"FIGI":null,"SharesPar":"0","MarketValue":null,"ContractType":null}
];
</script>`

	t.Run("basic modal holdings", func(t *testing.T) {
		got, err := ParseHoldingsFromModal(modalHTML)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("expected 3 holdings (cash filtered), got %d", len(got))
		}

		// Check first holding
		if got[0].Name != "Nvidia Corp" {
			t.Errorf("holding[0].Name = %q, want %q", got[0].Name, "Nvidia Corp")
		}
		if got[0].Symbol != "NVDA" {
			t.Errorf("holding[0].Symbol = %q, want %q", got[0].Symbol, "NVDA")
		}
		if got[0].Percent != 14.2625 {
			t.Errorf("holding[0].Percent = %f, want %f", got[0].Percent, 14.2625)
		}

		// Check CUSIP ticker preserved
		if got[2].Symbol != "US5128073062" {
			t.Errorf("holding[2].Symbol = %q, want %q", got[2].Symbol, "US5128073062")
		}
	})

	t.Run("missing JSON", func(t *testing.T) {
		_, err := ParseHoldingsFromModal(`<div>no JSON here</div>`)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestUnescapeJSString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ampersand", `Hello\u0026World`, "Hello&World"},
		{"escaped quote", `\"F5, Inc\"`, `"F5, Inc"`},
		{"escaped single quote", `it\'s`, "it's"},
		{"newline", `line1\\nline2`, "line1\nline2"},
		{"percent", `\u0025`, "%"},
		{"slash", `\u002F`, "/"},
		{"backslash", `\\`, `\`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeJSString(tt.input)
			if got != tt.expected {
				t.Errorf("unescapeJSString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
