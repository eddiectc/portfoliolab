package blackrock

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// --- Phase 1: HTML Parser Tests ---

func TestParseFundIdentity(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    string
		wantErr bool
	}{
		{
			name:    "h1 tag",
			html:    `<h1>iShares MSCI World Momentum Factor UCITS ETF</h1>`,
			want:    "iShares MSCI World Momentum Factor UCITS ETF",
			wantErr: false,
		},
		{
			name:    "title tag fallback",
			html:    `<title>iShares MSCI World Momentum Factor UCITS ETF | iShares UK</title>`,
			want:    "iShares MSCI World Momentum Factor UCITS ETF",
			wantErr: false,
		},
		{
			name:    "title tag without suffix",
			html:    `<title>iShares Core ETF</title>`,
			want:    "iShares Core ETF",
			wantErr: false,
		},
		{
			name:    "missing name",
			html:    `<html><body>no fund name here</body></html>`,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFundIdentity(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.want {
				t.Errorf("name = %q, want %q", got.Name, tt.want)
			}
		})
	}
}

func TestParseFundProfile(t *testing.T) {
	html := `<table class="key-facts">
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
</table>`

	profile, err := ParseFundProfile(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.TotalNetAssets != 5181115355 {
		t.Errorf("TotalNetAssets = %f, want 5181115355", profile.TotalNetAssets)
	}
	if profile.InceptionDate.Year() != 2014 || profile.InceptionDate.Month() != 10 || profile.InceptionDate.Day() != 3 {
		t.Errorf("InceptionDate = %v, want 2014-10-03", profile.InceptionDate)
	}
	if profile.AssetClassification != "Equity" {
		t.Errorf("AssetClassification = %q, want Equity", profile.AssetClassification)
	}
	if profile.SFDRClassification != "Other" {
		t.Errorf("SFDRClassification = %q, want Other", profile.SFDRClassification)
	}
	if profile.AnnualExpenseRatio != 0.0025 {
		t.Errorf("AnnualExpenseRatio = %f, want 0.0025 (fraction for 0.25%%)", profile.AnnualExpenseRatio)
	}
	if profile.DistributionStrategy != "Accumulating" {
		t.Errorf("DistributionStrategy = %q, want Accumulating", profile.DistributionStrategy)
	}
	if profile.Domicile != "Ireland" {
		t.Errorf("Domicile = %q, want Ireland", profile.Domicile)
	}
	if profile.RebalanceFrequency != "Quarterly" {
		t.Errorf("RebalanceFrequency = %q, want Quarterly", profile.RebalanceFrequency)
	}
	if profile.FundManager != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("FundManager = %q", profile.FundManager)
	}
	if profile.Custodian != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("Custodian = %q", profile.Custodian)
	}
	if profile.BenchmarkTicker != "IWMO LN" {
		t.Errorf("BenchmarkTicker = %q, want IWMO LN", profile.BenchmarkTicker)
	}
	if profile.Benchmark != "MSCI World Momentum Index (Net)" {
		t.Errorf("Benchmark = %q", profile.Benchmark)
	}
	if profile.Isin != "IE00BP3QZ825" {
		t.Errorf("Isin = %q, want IE00BP3QZ825", profile.Isin)
	}
	if profile.ProductStructure != "Physical" {
		t.Errorf("ProductStructure = %q, want Physical", profile.ProductStructure)
	}
	if profile.Methodology != "Optimised" {
		t.Errorf("Methodology = %q, want Optimised", profile.Methodology)
	}
	if profile.IssuingCompany != "iShares IV plc" {
		t.Errorf("IssuingCompany = %q, want iShares IV plc", profile.IssuingCompany)
	}
}

func TestParseFundProfile_MissingData(t *testing.T) {
	_, err := ParseFundProfile(`<table><tr><td>Other</td><td>Data</td></tr></table>`)
	if err == nil {
		t.Error("expected error for missing ISIN and AUM")
	}
}

func TestParseFundProfile_DivFormat(t *testing.T) {
	html := `<div class="product-data-list">
<div class="product-data-item col-totalNetAssets "><div class="caption" data-label="" data-hasContent="no">Net Assets<div class="as-of-date">as of 29/May/2026</div></div><div class="data">USD 5,181,115,355</div></div>
<div class="product-data-item col-inceptionDate "><div class="caption" data-label="" data-hasContent="no">Inception Date<div class="as-of-date"></div></div><div class="data">03/Oct/2014</div></div>
<div class="product-data-item col-assetClass "><div class="caption" data-label="" data-hasContent="no">Asset Class<div class="as-of-date"></div></div><div class="data">Equity</div></div>
<div class="product-data-item col-sfdr "><div class="caption" data-label="" data-hasContent="no">SFDR Classification<div class="as-of-date"></div></div><div class="data">Other</div></div>
<div class="product-data-item col-useOfProfitsCode "><div class="caption" data-label="" data-hasContent="no">Use of Income<div class="as-of-date"></div></div><div class="data">Accumulating</div></div>
<div class="product-data-item col-domicile "><div class="caption" data-label="" data-hasContent="no">Domicile<div class="as-of-date"></div></div><div class="data">Ireland</div></div>
<div class="product-data-item col-rebalanceFrequency "><div class="caption" data-label="" data-hasContent="no">Rebalance Frequency<div class="as-of-date"></div></div><div class="data">Quarterly</div></div>
<div class="product-data-item col-fundmanager "><div class="caption" data-label="" data-hasContent="no">Fund Manager<div class="as-of-date"></div></div><div class="data">BlackRock Asset Management Ireland Limited</div></div>
<div class="product-data-item col-fundCustodian "><div class="caption" data-label="" data-hasContent="no">Custodian<div class="as-of-date"></div></div><div class="data">State Street Custodial Services (Ireland) Limited</div></div>
<div class="product-data-item col-bbeqtick "><div class="caption" data-label="" data-hasContent="no">Bloomberg Ticker<div class="as-of-date"></div></div><div class="data">IWMO LN</div></div>
<div class="product-data-item col-indexSeriesName "><div class="caption" data-label="" data-hasContent="no">Benchmark Index<div class="as-of-date"></div></div><div class="data">MSCI World Momentum Index (Net)</div></div>
<div class="product-data-item col-isin "><div class="caption" data-label="" data-hasContent="no">ISIN<div class="as-of-date"></div></div><div class="data">IE00BP3QZ825</div></div>
<div class="product-data-item col-productStructure "><div class="caption" data-label="" data-hasContent="no">Product Structure<div class="as-of-date"></div></div><div class="data">Physical</div></div>
<div class="product-data-item col-fundMethodologyTypeCode "><div class="caption" data-label="" data-hasContent="no">Methodology<div class="as-of-date"></div></div><div class="data">Optimised</div></div>
<div class="product-data-item col-issuingCompany "><div class="caption" data-label="" data-hasContent="no">Issuing Company<div class="as-of-date"></div></div><div class="data">iShares IV plc</div></div>
</div>`

	profile, err := ParseFundProfile(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.TotalNetAssets != 5181115355 {
		t.Errorf("TotalNetAssets = %f, want 5181115355", profile.TotalNetAssets)
	}
	if profile.InceptionDate.Year() != 2014 || profile.InceptionDate.Month() != 10 || profile.InceptionDate.Day() != 3 {
		t.Errorf("InceptionDate = %v, want 2014-10-03", profile.InceptionDate)
	}
	if profile.AssetClassification != "Equity" {
		t.Errorf("AssetClassification = %q, want Equity", profile.AssetClassification)
	}
	if profile.SFDRClassification != "Other" {
		t.Errorf("SFDRClassification = %q, want Other", profile.SFDRClassification)
	}
	if profile.DistributionStrategy != "Accumulating" {
		t.Errorf("DistributionStrategy = %q, want Accumulating", profile.DistributionStrategy)
	}
	if profile.Domicile != "Ireland" {
		t.Errorf("Domicile = %q, want Ireland", profile.Domicile)
	}
	if profile.RebalanceFrequency != "Quarterly" {
		t.Errorf("RebalanceFrequency = %q, want Quarterly", profile.RebalanceFrequency)
	}
	if profile.FundManager != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("FundManager = %q", profile.FundManager)
	}
	if profile.Custodian != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("Custodian = %q", profile.Custodian)
	}
	if profile.BenchmarkTicker != "IWMO LN" {
		t.Errorf("BenchmarkTicker = %q, want IWMO LN", profile.BenchmarkTicker)
	}
	if profile.Benchmark != "MSCI World Momentum Index (Net)" {
		t.Errorf("Benchmark = %q", profile.Benchmark)
	}
	if profile.Isin != "IE00BP3QZ825" {
		t.Errorf("Isin = %q, want IE00BP3QZ825", profile.Isin)
	}
	if profile.ProductStructure != "Physical" {
		t.Errorf("ProductStructure = %q, want Physical", profile.ProductStructure)
	}
	if profile.Methodology != "Optimised" {
		t.Errorf("Methodology = %q, want Optimised", profile.Methodology)
	}
	if profile.IssuingCompany != "iShares IV plc" {
		t.Errorf("IssuingCompany = %q, want iShares IV plc", profile.IssuingCompany)
	}
	// Total Expense Ratio is not in the new page layout — should be 0
	if profile.AnnualExpenseRatio != 0 {
		t.Errorf("AnnualExpenseRatio = %f, want 0 (not present in new layout)", profile.AnnualExpenseRatio)
	}
}

func TestParseFundCharacteristics_DivFormat(t *testing.T) {
	html := `<div class="product-data-list">
<div class="product-data-item col-numHoldings "><div class="caption">Number of Holdings<div class="as-of-date">as of 29/May/2026</div></div><div class="data">352</div></div>
<div class="product-data-item col-priceEarnings "><div class="caption">P/E Ratio<div class="as-of-date">as of 29/May/2026</div></div><div class="data">29.16</div></div>
<div class="product-data-item col-priceBook "><div class="caption">P/B Ratio<div class="as-of-date">as of 29/May/2026</div></div><div class="data">3.89</div></div>
<div class="product-data-item col-threeYrBetaFund "><div class="caption">3y Beta<div class="as-of-date">as of 30/Apr/2026</div></div><div class="data">0.999</div></div>
<div class="product-data-item col-volatilitySourced3YrAnnualized "><div class="caption">Standard Deviation (3y)<div class="as-of-date">as of 30/Apr/2026</div></div><div class="data">16.62%</div></div>
</div>`

	chars, err := ParseFundCharacteristics(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chars.NumberOfHoldings != 352 {
		t.Errorf("NumberOfHoldings = %d, want 352", chars.NumberOfHoldings)
	}
	if chars.PriceToEarnings != 29.16 {
		t.Errorf("PriceToEarnings = %f, want 29.16", chars.PriceToEarnings)
	}
	if chars.PriceToBook != 3.89 {
		t.Errorf("PriceToBook = %f, want 3.89", chars.PriceToBook)
	}
	if chars.Beta3Y != 0.999 {
		t.Errorf("Beta3Y = %f, want 0.999", chars.Beta3Y)
	}
	if chars.StandardDeviation3Y != 16.62 {
		t.Errorf("StandardDeviation3Y = %f, want 16.62", chars.StandardDeviation3Y)
	}
}

func TestParseFundCharacteristics_Equity(t *testing.T) {
	html := `<table>
<tr><td>Number of Holdings</td><td>352 (as of 29/May/2026)</td></tr>
<tr><td>P/E Ratio</td><td>29.16 (as of 29/May/2026)</td></tr>
<tr><td>P/B Ratio</td><td>3.89 (as of 29/May/2026)</td></tr>
<tr><td>3y Beta</td><td>0.999 (as of 30/Apr/2026)</td></tr>
<tr><td>Standard Deviation (3y)</td><td>16.62% (as of 30/Apr/2026)</td></tr>
</table>`

	chars, err := ParseFundCharacteristics(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chars.NumberOfHoldings != 352 {
		t.Errorf("NumberOfHoldings = %d, want 352", chars.NumberOfHoldings)
	}
	if chars.PriceToEarnings != 29.16 {
		t.Errorf("PriceToEarnings = %f, want 29.16", chars.PriceToEarnings)
	}
	if chars.PriceToBook != 3.89 {
		t.Errorf("PriceToBook = %f, want 3.89", chars.PriceToBook)
	}
	if chars.Beta3Y != 0.999 {
		t.Errorf("Beta3Y = %f, want 0.999", chars.Beta3Y)
	}
	if chars.StandardDeviation3Y != 16.62 {
		t.Errorf("StandardDeviation3Y = %f, want 16.62", chars.StandardDeviation3Y)
	}

	// Verify bitmask
	if !chars.HasCharacteristic(extractor.CharacteristicNumberOfHoldings) {
		t.Error("Expected CharacteristicNumberOfHoldings to be set")
	}
	if !chars.HasCharacteristic(extractor.CharacteristicPriceToEarnings) {
		t.Error("Expected CharacteristicPriceToEarnings to be set")
	}
	if !chars.HasCharacteristic(extractor.CharacteristicBeta3Y) {
		t.Error("Expected CharacteristicBeta3Y to be set")
	}
}

func TestParseFundCharacteristics_Bond(t *testing.T) {
	html := `<table>
<tr><td>Number of Holdings</td><td>1224 (as of 30/Apr/2026)</td></tr>
<tr><td>Standard Deviation (3y)</td><td>4.96% (as of 30/Apr/2026)</td></tr>
<tr><td>Yield to Maturity</td><td>5.59 (as of 30/Apr/2026)</td></tr>
<tr><td>Weighted Average YTM</td><td>5.49% (as of 30/Apr/2026)</td></tr>
<tr><td>Weighted Avg Maturity</td><td>7.63 (as of 30/Apr/2026)</td></tr>
<tr><td>Modified Duration</td><td>5.13 (as of 30/Apr/2026)</td></tr>
<tr><td>Effective Duration</td><td>5.17 (as of 30/Apr/2026)</td></tr>
<tr><td>3y Beta</td><td>0.997 (as of 30/Apr/2026)</td></tr>
</table>`

	chars, err := ParseFundCharacteristics(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chars.NumberOfHoldings != 1224 {
		t.Errorf("NumberOfHoldings = %d, want 1224", chars.NumberOfHoldings)
	}
	if chars.StandardDeviation3Y != 4.96 {
		t.Errorf("StandardDeviation3Y = %f, want 4.96", chars.StandardDeviation3Y)
	}
	if chars.AverageCoupon != 5.59 {
		t.Errorf("AverageCoupon (YTM) = %f, want 5.59", chars.AverageCoupon)
	}
	if chars.AverageMaturity != 7.63 {
		t.Errorf("AverageMaturity = %f, want 7.63", chars.AverageMaturity)
	}
	if chars.AverageDuration != 5.13 {
		t.Errorf("AverageDuration = %f, want 5.13", chars.AverageDuration)
	}
	if chars.Beta3Y != 0.997 {
		t.Errorf("Beta3Y = %f, want 0.997", chars.Beta3Y)
	}
}

func TestParseFundCharacteristics_Empty(t *testing.T) {
	chars, err := ParseFundCharacteristics(`<table><tr><td>Other</td><td>Data</td></tr></table>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chars != nil {
		t.Error("expected nil for empty characteristics")
	}
}

func TestParseComponentID(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    string
		wantErr bool
	}{
		{
			name: "standard component ID",
			html: `<a href="/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf/1506575576011.ajax?fileType=csv&fileName=IWMO_holdings&dataType=fund">Download`,
			want: "1506575576011",
		},
		{
			name: "shorter component ID",
			html: `<a href="/uk/individual/en/products/123456/test/9876543210.ajax?fileType=csv">Download`,
			want: "9876543210",
		},
		{
			name:    "missing component ID",
			html:    `<a href="/uk/individual/en/products/123456/test">No download link</a>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseComponentID(tt.html)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
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
			name: "standard format (old table)",
			html: `<h3>Fund Holdings as of,"29/May/2026"</h3>`,
			want: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "with space (old table)",
			html: `<h3>Fund Holdings as of "30/Apr/2026"</h3>`,
			want: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "div as-of-date",
			html: `<div class="as-of-date">as of 29/May/2026</div>`,
			want: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "p as-of-date",
			html: `<p class="as-of-date">as of 31/Mar/2026</p>`,
			want: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "product-data-item as-of-date",
			html: `<div class="product-data-item col-totalNetAssets "><div class="caption"><div class="as-of-date">as of 29/May/2026</div></div><div class="data">USD 5,181,115,355</div></div>`,
			want: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "missing as-of date returns zero",
			html: `<h3>Fund Holdings</h3>`,
			want: time.Time{},
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

// --- Phase 2: JSON Parser Tests ---

func TestParseHoldings(t *testing.T) {
	csvData := `Fund Holdings as of,"29/May/2026"

Ticker,Name,Type,Sector,Asset Class,Market Value,Weight (%),Notional Value,Shares,Price,Location,Exchange,Market Currency
"MU","MICRON TECHNOLOGY INC","EQUITY","Information Technology","Equity","340,433,571.00","6.57076","340,433,571.00","350,601.00","971.00","United States","NASDAQ","USD"
"AAPL","APPLE INC","EQUITY","Information Technology","Equity","200,000,000.00","3.86","200,000,000.00","1,000,000.00","200.00","United States","NASDAQ","USD"
"","USD CASH","CASH","Cash","Cash","-1,000,000.00","-0.02","-1,000,000.00","1,000,000.00","1.00","","","USD"`

	holdings, asOfDate, err := ParseHoldings(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(holdings) != 3 {
		t.Fatalf("expected 3 holdings, got %d", len(holdings))
	}

	// First holding (MU)
	if holdings[0].Symbol != "MU" {
		t.Errorf("holding[0].Symbol = %q, want MU", holdings[0].Symbol)
	}
	if holdings[0].Name != "MICRON TECHNOLOGY INC" {
		t.Errorf("holding[0].Name = %q", holdings[0].Name)
	}
	if holdings[0].Percent != 6.57076 {
		t.Errorf("holding[0].Percent = %f, want 6.57076", holdings[0].Percent)
	}
	if holdings[0].Sector != "Information Technology" {
		t.Errorf("holding[0].Sector = %q", holdings[0].Sector)
	}
	if holdings[0].AssetClass != "Equity" {
		t.Errorf("holding[0].AssetClass = %q", holdings[0].AssetClass)
	}
	if holdings[0].MarketValue != 340433571 {
		t.Errorf("holding[0].MarketValue = %f, want 340433571", holdings[0].MarketValue)
	}
	if holdings[0].NotionalValue != 340433571 {
		t.Errorf("holding[0].NotionalValue = %f, want 340433571", holdings[0].NotionalValue)
	}
	if holdings[0].Shares != 350601 {
		t.Errorf("holding[0].Shares = %f, want 350601", holdings[0].Shares)
	}
	if holdings[0].ISIN != "-" {
		t.Errorf("holding[0].ISIN = %q, want -", holdings[0].ISIN)
	}
	if holdings[0].Price != 971 {
		t.Errorf("holding[0].Price = %f, want 971", holdings[0].Price)
	}
	if holdings[0].Location != "United States" {
		t.Errorf("holding[0].Location = %q", holdings[0].Location)
	}
	if holdings[0].Exchange != "NASDAQ" {
		t.Errorf("holding[0].Exchange = %q", holdings[0].Exchange)
	}
	if holdings[0].MarketCurrency != "USD" {
		t.Errorf("holding[0].MarketCurrency = %q", holdings[0].MarketCurrency)
	}

	// Cash position
	if holdings[2].Name != "USD CASH" {
		t.Errorf("holding[2].Name = %q, want USD CASH", holdings[2].Name)
	}
	if holdings[2].Percent != -0.02 {
		t.Errorf("holding[2].Percent = %f, want -0.02", holdings[2].Percent)
	}
	if holdings[2].ISIN != "-" {
		t.Errorf("holding[2].ISIN = %q, want -", holdings[2].ISIN)
	}

	// As-of date
	if asOfDate != "29/May/2026" {
		t.Errorf("asOfDate = %q, want 29/May/2026", asOfDate)
	}
}

func TestParseHoldings_BOM(t *testing.T) {
	// UTF-8 BOM prefix
	csvData := "\xef\xbb\xbfFund Holdings as of,\"29/May/2026\"\n\nTicker,Name,Type,Sector,Asset Class,Market Value,Weight (%),Notional Value,Shares,Price,Location,Exchange,Market Currency\n\"MU\",\"Test\",\"EQUITY\",\"Tech\",\"Equity\",\"100\",\"1.0\",\"100\",\"10\",\"10\",\"US\",\"NASDAQ\",\"USD\""
	holdings, _, err := ParseHoldings(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(holdings) != 1 {
		t.Errorf("expected 1 holding with BOM, got %d", len(holdings))
	}
}

func TestParseHoldings_Empty(t *testing.T) {
	csvData := `Fund Holdings as of,"29/May/2026"

Ticker,Name,Type,Sector,Asset Class,Market Value,Weight (%),Notional Value,Shares,Price,Location,Exchange,Market Currency`
	holdings, _, err := ParseHoldings(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(holdings) != 0 {
		t.Errorf("expected 0 holdings, got %d", len(holdings))
	}
}

func TestParseHoldings_NoHeaderRow(t *testing.T) {
	// CSV with data but no title or header row — should return 0 holdings
	csvData := `MU,Micron,EQUITY,Tech,Equity,100,1.0,100,10,10,US,NASDAQ,USD`
	holdings, _, err := ParseHoldings(csvData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(holdings) != 0 {
		t.Errorf("expected 0 holdings without header row, got %d", len(holdings))
	}
}

func TestDeriveSectorAllocation(t *testing.T) {
	holdings := []extractor.Holding{
		{Symbol: "MU", Sector: "Information Technology", Percent: 6.57},
		{Symbol: "AAPL", Sector: "Information Technology", Percent: 3.86},
		{Symbol: "JNJ", Sector: "Healthcare", Percent: 4.20},
		{Symbol: "JPM", Sector: "Financials", Percent: 2.10},
		{Symbol: "USD CASH", Sector: "", Percent: -0.02},
	}

	sectors, err := DeriveSectorAllocation(holdings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sectors) != 3 {
		t.Fatalf("expected 3 sectors, got %d", len(sectors))
	}

	// Sorted descending
	if sectors[0].Sector != "Information Technology" {
		t.Errorf("first sector = %q, want Information Technology", sectors[0].Sector)
	}
	if sectors[0].Percent != 10.43 {
		t.Errorf("first sector percent = %f, want 10.43", sectors[0].Percent)
	}
	if sectors[1].Sector != "Healthcare" {
		t.Errorf("second sector = %q, want Healthcare", sectors[1].Sector)
	}
	if sectors[2].Sector != "Financials" {
		t.Errorf("third sector = %q, want Financials", sectors[2].Sector)
	}
}

func TestDeriveSectorAllocation_NegativeWeight(t *testing.T) {
	// Negative-weight holding with a sector (e.g. short position) should use absolute weight
	holdings := []extractor.Holding{
		{Symbol: "MU", Sector: "Information Technology", Percent: 6.57},
		{Symbol: "SHORT", Sector: "Information Technology", Percent: -2.00},
		{Symbol: "JNJ", Sector: "Healthcare", Percent: 4.20},
	}

	sectors, err := DeriveSectorAllocation(holdings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sectors[0].Sector != "Information Technology" {
		t.Errorf("first sector = %q, want Information Technology", sectors[0].Sector)
	}
	// 6.57 + |-2.00| = 8.57
	if sectors[0].Percent != 8.57 {
		t.Errorf("first sector percent = %f, want 8.57", sectors[0].Percent)
	}
}

func TestDeriveCountryAllocation(t *testing.T) {
	holdings := []extractor.Holding{
		{Symbol: "MU", Location: "United States", Percent: 6.57},
		{Symbol: "AAPL", Location: "United States", Percent: 3.86},
		{Symbol: "NVS", Location: "Netherlands", Percent: 2.50},
		{Symbol: "T", Location: "United States", Percent: 1.80},
		{Symbol: "Cash", Location: "", Percent: -0.02},
	}

	countries, err := DeriveCountryAllocation(holdings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(countries) != 2 {
		t.Fatalf("expected 2 countries, got %d", len(countries))
	}

	if countries[0].Country != "United States" {
		t.Errorf("first country = %q, want United States", countries[0].Country)
	}
	if countries[0].Percent != 12.23 {
		t.Errorf("first country percent = %f, want 12.23", countries[0].Percent)
	}
	if countries[1].Country != "Netherlands" {
		t.Errorf("second country = %q, want Netherlands", countries[1].Country)
	}
}

func TestDeriveCountryAllocation_NegativeWeight(t *testing.T) {
	// Negative-weight holding with a location (e.g. short position) should use absolute weight
	holdings := []extractor.Holding{
		{Symbol: "MU", Location: "United States", Percent: 6.57},
		{Symbol: "SHORT", Location: "United States", Percent: -2.00},
		{Symbol: "NVS", Location: "Netherlands", Percent: 2.50},
	}

	countries, err := DeriveCountryAllocation(holdings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if countries[0].Country != "United States" {
		t.Errorf("first country = %q, want United States", countries[0].Country)
	}
	// 6.57 + |-2.00| = 8.57
	if countries[0].Percent != 8.57 {
		t.Errorf("first country percent = %f, want 8.57", countries[0].Percent)
	}
}

func TestParseIShareDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "dd/Mon/yyyy",
			input: "29/May/2026",
			want:  time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "d/Mon/yyyy",
			input: "1/May/2026",
			want:  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "yyyymmdd",
			input: "20260529",
			want:  time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "yyyy-mm-dd",
			input: "2026-05-29",
			want:  time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "invalid",
			input:   "not a date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIShareDate(tt.input)
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

func TestExtractAUM(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
	}{
		{"standard", "USD 5,181,115,355 (as of 29/May/2026)", 5181115355},
		{"no currency", "1,234,567 (as of 01/Jan/2026)", 1234567},
		{"small", "EUR 500,000", 500000},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAUM(tt.input)
			if got != tt.want {
				t.Errorf("extractAUM(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFloatValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
	}{
		{"with date suffix", "29.16 (as of 29/May/2026)", 29.16},
		{"with percent", "16.62%", 16.62},
		{"plain", "0.999", 0.999},
		{"with comma", "1,234.56", 1234.56},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFloatValue(tt.input)
			if got != tt.want {
				t.Errorf("parseFloatValue(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestMathRound(t *testing.T) {
	tests := []struct {
		name   string
		val    float64
		places int
		want   float64
	}{
		{"positive", 10.435, 2, 10.44},
		{"negative", -0.025, 2, -0.03},
		{"zero", 0, 2, 0},
		{"large", 1234567.895, 2, 1234567.90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mathRound(tt.val, tt.places)
			if got != tt.want {
				t.Errorf("mathRound(%f, %d) = %f, want %f", tt.val, tt.places, got, tt.want)
			}
		})
	}
}

// --- Product Data JSON API (2026-08 redesign) ---

func TestParseProductDataConfig(t *testing.T) {
	cfg, err := ParseProductDataConfig(sampleProductPageHTML)
	if err != nil {
		t.Fatalf("ParseProductDataConfig failed: %v", err)
	}
	if cfg.APIHost != "https://www.blackrock.com/varnish-api/uk-retail01-product-data/product-data/api/v2/get-product-data" {
		t.Errorf("APIHost = %q (trailing ? must be stripped)", cfg.APIHost)
	}
	if cfg.AppSubType != "ISHARES" || cfg.AppType != "PRODUCT_PAGE" {
		t.Errorf("AppSubType/AppType = %q/%q", cfg.AppSubType, cfg.AppType)
	}
	if cfg.Locale != "en_GB" || cfg.TargetSite != "ishares-uk" || cfg.UserType != "individual" {
		t.Errorf("Locale/TargetSite/UserType = %q/%q/%q", cfg.Locale, cfg.TargetSite, cfg.UserType)
	}
}

func TestParseProductDataConfig_Missing(t *testing.T) {
	for name, page := range map[string]string{
		"no config":    "<html><body><h1>Fund</h1></body></html>",
		"only apiHost": `<script>{"services":{"apiHost":"https://x/api?"}}</script>`,
		"only params":  `<script>{"productDataParams":{"appSubType":"ISHARES","locale":"en_GB","targetSite":"ishares-uk","userType":"individual"}}</script>`,
		"incomplete":   `<script>{"apiHost":"https://x/api","productDataParams":{"appSubType":"ISHARES"}}</script>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProductDataConfig(page); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParsePortfolioID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
		fail bool
	}{
		{
			name: "product url",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/ishares-msci-world-momentum-factor-ucits-etf",
			want: "270051",
		},
		{
			name: "with query string",
			url:  "https://www.ishares.com/uk/individual/en/products/270051/fund?switchLocale=y",
			want: "270051",
		},
		{
			name: "not found page",
			url:  "https://www.ishares.com/uk/individual/en/404",
			fail: true,
		},
		{
			name: "no products segment",
			url:  "https://www.ishares.com/uk/individual/en/funds",
			fail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePortfolioID(tt.url)
			if tt.fail {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePortfolioID failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildProductDataURL(t *testing.T) {
	cfg, err := ParseProductDataConfig(sampleProductPageHTML)
	if err != nil {
		t.Fatalf("ParseProductDataConfig failed: %v", err)
	}

	u, err := BuildProductDataURL(cfg, "270051", "holdings")
	if err != nil {
		t.Fatalf("BuildProductDataURL failed: %v", err)
	}

	want := "https://www.blackrock.com/varnish-api/uk-retail01-product-data/product-data/api/v2/get-product-data?" +
		"appSubType=ISHARES&appType=PRODUCT_PAGE&component=holdings&locale=en_GB&" +
		"portfolioId=270051&targetSite=ishares-uk&userType=individual"
	if u != want {
		t.Errorf("URL = %q, want %q", u, want)
	}

	if _, err := BuildProductDataURL(nil, "270051", "holdings"); err == nil {
		t.Error("expected error for nil config")
	}
}

func TestParseFundProfileFromJSON(t *testing.T) {
	profile, err := ParseFundProfileFromJSON(sampleKeyFundFactsJSON)
	if err != nil {
		t.Fatalf("ParseFundProfileFromJSON failed: %v", err)
	}

	if profile.Isin != "IE00BP3QZ825" {
		t.Errorf("Isin = %q", profile.Isin)
	}
	if profile.TotalNetAssets != 6022456334 {
		t.Errorf("TotalNetAssets = %v", profile.TotalNetAssets)
	}
	if !profile.InceptionDate.Equal(time.Date(2014, 10, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("InceptionDate = %v", profile.InceptionDate)
	}
	if profile.AssetClassification != "Equity" {
		t.Errorf("AssetClassification = %q", profile.AssetClassification)
	}
	if profile.SFDRClassification != "Other" {
		t.Errorf("SFDRClassification = %q", profile.SFDRClassification)
	}
	if profile.DistributionStrategy != "Accumulating" {
		t.Errorf("DistributionStrategy = %q", profile.DistributionStrategy)
	}
	if profile.Domicile != "Ireland" {
		t.Errorf("Domicile = %q", profile.Domicile)
	}
	if profile.RebalanceFrequency != "Quarterly" {
		t.Errorf("RebalanceFrequency = %q", profile.RebalanceFrequency)
	}
	if profile.FundManager != "BlackRock Asset Management Ireland Limited" {
		t.Errorf("FundManager = %q", profile.FundManager)
	}
	if profile.Custodian != "State Street Custodial Services (Ireland) Limited" {
		t.Errorf("Custodian = %q", profile.Custodian)
	}
	if profile.BenchmarkTicker != "IWMO LN" {
		t.Errorf("BenchmarkTicker = %q", profile.BenchmarkTicker)
	}
	if profile.Benchmark != "MSCI World Momentum index (Net)" {
		t.Errorf("Benchmark = %q", profile.Benchmark)
	}
	if profile.ProductStructure != "Physical" {
		t.Errorf("ProductStructure = %q", profile.ProductStructure)
	}
	if profile.Methodology != "Optimised" {
		t.Errorf("Methodology = %q", profile.Methodology)
	}
	if profile.IssuingCompany != "iShares IV plc" {
		t.Errorf("IssuingCompany = %q", profile.IssuingCompany)
	}
	if profile.BaseCurrency != "USD" {
		t.Errorf("BaseCurrency = %q", profile.BaseCurrency)
	}
	// TER is not part of keyFundFacts
	if profile.AnnualExpenseRatio != 0 {
		t.Errorf("AnnualExpenseRatio = %v, want 0 (not in JSON)", profile.AnnualExpenseRatio)
	}
}

func TestParseFundProfileFromJSON_Errors(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		match string
	}{
		{
			name:  "invalid json",
			body:  "{not json",
			match: "parse product data response",
		},
		{
			name:  "component missing",
			body:  `{"componentsByNameMap":{"other":{"containersByNameMap":{"default":{"dataPointsByNameMap":{}}}}}}`,
			match: "keyFundFacts component not found",
		},
		{
			name:  "no identifying data",
			body:  `{"componentsByNameMap":{"keyFundFacts":{"containersByNameMap":{"default":{"dataPointsByNameMap":{"domicile":{"name":"domicile","formattedValue":"Ireland"}}}}}}}`,
			match: "fund profile data missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFundProfileFromJSON(tt.body)
			if err == nil {
				t.Fatal("expected error")
			}
			if !containsStr(err.Error(), tt.match) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.match)
			}
		})
	}
}

func TestParseHoldingsFromJSON(t *testing.T) {
	holdings, asOf, err := ParseHoldingsFromJSON(sampleHoldingsJSON)
	if err != nil {
		t.Fatalf("ParseHoldingsFromJSON failed: %v", err)
	}

	if len(holdings) != 4 {
		t.Fatalf("len(holdings) = %d, want 4", len(holdings))
	}

	mu := holdings[0]
	if mu.Symbol != "MU" || mu.Name != "MICRON TECHNOLOGY" {
		t.Errorf("holdings[0] = %+v", mu)
	}
	if mu.Percent != 6.57 || mu.Shares != 350601 || mu.Price != 971 {
		t.Errorf("holdings[0] percent/shares/price = %v/%v/%v", mu.Percent, mu.Shares, mu.Price)
	}
	if mu.MarketValue != 340433571 || mu.NotionalValue != 340433571 {
		t.Errorf("holdings[0] market/notional = %v/%v", mu.MarketValue, mu.NotionalValue)
	}
	if mu.Sector != "Information Technology" || mu.AssetClass != "Equity" {
		t.Errorf("holdings[0] sector/assetClass = %q/%q", mu.Sector, mu.AssetClass)
	}
	if mu.ISIN != "US5951121038" {
		t.Errorf("holdings[0] ISIN = %q", mu.ISIN)
	}
	if mu.Location != "United States" || mu.Exchange != "NASDAQ" || mu.MarketCurrency != "USD" {
		t.Errorf("holdings[0] location/exchange/ccy = %q/%q/%q", mu.Location, mu.Exchange, mu.MarketCurrency)
	}

	cash := holdings[3]
	if cash.Symbol != "USD" || cash.Name != "Cash" || cash.ISIN != "-" {
		t.Errorf("cash row = %+v", cash)
	}

	if !asOf.Equal(time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("asOf = %v, want 2026-08-27", asOf)
	}
}

func TestParseHoldingsFromJSON_Empty(t *testing.T) {
	holdings, asOf, err := ParseHoldingsFromJSON(emptyHoldingsJSON)
	if err != nil {
		t.Fatalf("ParseHoldingsFromJSON failed: %v", err)
	}
	if len(holdings) != 0 {
		t.Errorf("len(holdings) = %d, want 0", len(holdings))
	}
	if asOf.IsZero() {
		t.Error("asOf is zero, want 2026-08-27")
	}
}

func TestParseHoldingsFromJSON_MissingAsOfDate(t *testing.T) {
	holdings, asOf, err := ParseHoldingsFromJSON(sampleHoldingsJSONWithoutAsOfDate)
	if err != nil {
		t.Fatalf("ParseHoldingsFromJSON failed: %v", err)
	}
	if len(holdings) != 4 {
		t.Errorf("len(holdings) = %d, want 4", len(holdings))
	}
	if !asOf.IsZero() {
		t.Errorf("asOf = %v, want zero", asOf)
	}
}

func TestParseHoldingsFromJSON_Errors(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		match string
	}{
		{
			name:  "invalid json",
			body:  "nope",
			match: "parse product data response",
		},
		{
			name:  "component missing",
			body:  `{"componentsByNameMap":{}}`,
			match: "holdings component not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseHoldingsFromJSON(tt.body)
			if err == nil {
				t.Fatal("expected error")
			}
			if !containsStr(err.Error(), tt.match) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.match)
			}
		})
	}
}

// --- Data-item table rows (2026-08 redesign) ---

const sampleDataItemRowsHTML = `
<table>
<tr class="data-item oneds-body-m-compact col-totalNetAssetsFundLevel" data-itemid="keyFundFacts-row-totalNetAssetsFundLevel" data-itemname="totalNetAssetsFundLevel" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Fund Level Net Assets</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">USD 9,999,999,999</td>
</tr>
<tr class="data-item oneds-body-m-compact col-totalNetAssets" data-itemid="keyFundFacts-row-totalNetAssets" data-itemname="totalNetAssets" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Net Assets</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">USD <!-- -->6,022,456,334</td>
</tr>
<tr class="data-item oneds-body-m-compact col-emeaMgt" data-itemid="keyFundFacts-row-emeaMgt" data-itemname="emeaMgt" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Annual Management Fee (TER)</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1"><span class="value">0.25%</span></td>
</tr>
<tr class="data-item oneds-body-m-compact col-numHoldings" data-itemid="characteristics-row-numHoldings" data-itemname="numHoldings" data-itemtype="key-value">
<th class="caption" tabindex="-1"><div class="caption-wrapper"><span class="label">Number of Holdings</span></div></th>
<td class="data oneds-body-l-bold" tabindex="-1">352</td>
</tr>
</table>
`

func TestParseKeyValue_DataItemRows(t *testing.T) {
	// Span-wrapped value
	ter, err := parseKeyValue(sampleDataItemRowsHTML, "Total Expense Ratio")
	if err != nil {
		t.Fatalf("Total Expense Ratio: %v", err)
	}
	if ter != "0.25%" {
		t.Errorf("TER = %q, want %q", ter, "0.25%")
	}

	// Plain value with an SSR comment artifact
	aum, err := parseKeyValue(sampleDataItemRowsHTML, "Net Assets")
	if err != nil {
		t.Fatalf("Net Assets: %v", err)
	}
	if aum != "USD 6,022,456,334" {
		t.Errorf("Net Assets = %q (SSR comment must be stripped, FundLevel row must not be read)", aum)
	}

	// Plain value, no span
	nh, err := parseKeyValue(sampleDataItemRowsHTML, "Number of Holdings")
	if err != nil {
		t.Fatalf("Number of Holdings: %v", err)
	}
	if nh != "352" {
		t.Errorf("Number of Holdings = %q", nh)
	}

	// Missing key
	if _, err := parseKeyValue(sampleDataItemRowsHTML, "ISIN"); err == nil {
		t.Error("expected error for missing key")
	}
}

func TestParseFundProfile_DataItemRows(t *testing.T) {
	// A full profile expressed as data-item rows (TER, ISIN, AUM, inception)
	html := `
<table>
<tr class="data-item col-emeaMgt"><th class="caption"><span class="label">Total Expense Ratio</span></th><td class="data">0.25%</td></tr>
<tr class="data-item col-isin"><th class="caption"><span class="label">ISIN</span></th><td class="data">IE00BP3QZ825</td></tr>
<tr class="data-item col-totalNetAssets"><th class="caption"><span class="label">Net Assets</span></th><td class="data">USD 6,022,456,334</td></tr>
<tr class="data-item col-inceptionDate"><th class="caption"><span class="label">Inception Date</span></th><td class="data">03/Oct/2014</td></tr>
<tr class="data-item col-sfdr"><th class="caption"><span class="label">SFDR Classification</span></th><td class="data">Other</td></tr>
<tr class="data-item col-assetClass"><th class="caption"><span class="label">Asset Class</span></th><td class="data">Equity</td></tr>
</table>`

	profile, err := ParseFundProfile(html)
	if err != nil {
		t.Fatalf("ParseFundProfile failed: %v", err)
	}
	if profile.Isin != "IE00BP3QZ825" {
		t.Errorf("Isin = %q", profile.Isin)
	}
	if profile.TotalNetAssets != 6022456334 {
		t.Errorf("TotalNetAssets = %v", profile.TotalNetAssets)
	}
	if profile.AnnualExpenseRatio != 0.0025 {
		t.Errorf("AnnualExpenseRatio = %v, want 0.0025 (fraction for 0.25%%)", profile.AnnualExpenseRatio)
	}
	if profile.BenchmarkTicker != "" {
		t.Errorf("BenchmarkTicker = %q, want empty (not in rows)", profile.BenchmarkTicker)
	}
}

func TestParseAsOfDate_ExtraClasses(t *testing.T) {
	html := `<div class="as-of-date oneds-body-s-compact">as of 29/May/2026</div>`
	got, err := ParseAsOfDate(html)
	if err != nil {
		t.Fatalf("ParseAsOfDate failed: %v", err)
	}
	if !got.Equal(time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("got %v, want 2026-05-29", got)
	}
}

func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}
