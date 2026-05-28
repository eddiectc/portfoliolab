package wisdomtree

import (
	_ "embed"
	"testing"
)

//go:embed testdata/wmgt_page_cycletls.html
var realHTML string

func TestParseCountryAllocation_Real(t *testing.T) {
	countries, err := ParseCountryAllocation(realHTML)
	if err != nil {
		t.Fatalf("ParseCountryAllocation failed: %v", err)
	}
	if len(countries) < 10 {
		t.Errorf("expected at least 10 countries, got %d", len(countries))
	}
	if countries[0].Country != "United States" {
		t.Errorf("expected first country United States, got %s", countries[0].Country)
	}
	t.Logf("Got %d countries: %s (%.2f%%), %s (%.2f%%), %s (%.2f%%)",
		len(countries),
		countries[0].Country, countries[0].Percent,
		countries[1].Country, countries[1].Percent,
		countries[2].Country, countries[2].Percent)
}

func TestParseMarketCap_Real(t *testing.T) {
	mc, err := ParseMarketCap(realHTML)
	if err != nil {
		t.Fatalf("ParseMarketCap failed: %v", err)
	}
	if mc.Total == 0 {
		t.Error("expected total > 0")
	}
	t.Logf("Market cap: total=%.2fT, large=%.2f%%, mid=%.2f%%, small=%.2f%%",
		mc.Total, mc.Large, mc.Mid, mc.Small)
}

func TestParseFundCharacteristics_Real(t *testing.T) {
	ch, err := ParseFundCharacteristics(realHTML)
	if err != nil {
		t.Fatalf("ParseFundCharacteristics failed: %v", err)
	}
	if ch.PriceToEarnings == 0 {
		t.Error("expected P/E > 0")
	}
	if ch.EstimatedPriceToEarnings == 0 {
		t.Error("expected Estimated P/E > 0")
	}
	t.Logf("Characteristics: PE=%.2f, EstPE=%.2f, PB=%.2f, PS=%.2f, PCF=%.2f, DY=%.2f",
		ch.PriceToEarnings, ch.EstimatedPriceToEarnings, ch.PriceToBook, ch.PriceToSales, ch.PriceToCashflow, ch.DividendYield)
}

func TestExtractFromHTML_Real(t *testing.T) {
	// Test individual parsers against real HTML.
	// Full extractFromHTML is not tested here because the real page
	// may not have all sections (e.g. fundSectorsData is missing on WMGT).
	// The atomic full extraction is covered by TestExtractFromHTML with sampleHTML.

	fundInfo, err := ParseFundInfo(realHTML)
	if err != nil {
		t.Fatalf("ParseFundInfo failed: %v", err)
	}
	if fundInfo.Symbol != "WMGT" {
		t.Errorf("expected symbol WMGT, got %s", fundInfo.Symbol)
	}

	holdings, err := ParseHoldings(realHTML)
	if err != nil {
		t.Fatalf("ParseHoldings failed: %v", err)
	}
	if len(holdings) < 100 {
		t.Errorf("expected at least 100 holdings, got %d", len(holdings))
	}

	profile, err := ParseFundProfile(realHTML)
	if err != nil {
		t.Fatalf("ParseFundProfile failed: %v", err)
	}
	if profile.TotalNetAssets == 0 {
		t.Error("expected AUM > 0")
	}
	if profile.AnnualExpenseRatio == 0 {
		t.Error("expected TER > 0")
	}

	themes, err := ParseThemes(realHTML)
	if err != nil {
		t.Fatalf("ParseThemes failed: %v", err)
	}
	if len(themes) < 5 {
		t.Errorf("expected at least 5 themes, got %d", len(themes))
	}

	asOf, err := ParseAsOfDate(realHTML)
	if err != nil {
		t.Fatalf("ParseAsOfDate failed: %v", err)
	}
	if asOf.IsZero() {
		t.Error("expected non-zero as-of date")
	}

	t.Logf("Real HTML: symbol=%s, holdings=%d, themes=%d, AUM=%.0f, TER=%.2f%%, asOf=%s",
		fundInfo.Symbol, len(holdings), len(themes),
		profile.TotalNetAssets, profile.AnnualExpenseRatio, asOf.Format("2006-01-02"))
}
