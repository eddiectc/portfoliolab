package wisdomtree

import (
	"context"
	_ "embed"
	"strings"
	"testing"
)

//go:embed testdata/wmgt_page_cycletls.html
var e2eMainPage string

//go:embed testdata/wmgt_modal_all_holdings.html
var e2eModalPage string

// TestExtract_EndToEnd validates the full Extract flow (Matcher → Client →
// extractFromHTMLWithModals → all parsers) against real WisdomTree HTML.
// No network calls — uses embedded test data from a real CycleTLS scrape.
func TestExtract_EndToEnd(t *testing.T) {
	e := NewExtractor()
	e.client = &Client{minDelay: 0}

	e.client.SetFetchFunc(func(url string) (string, error) {
		if strings.Contains(url, "all-holdings") {
			return e2eModalPage, nil
		}
		return e2eMainPage, nil
	})

	result, err := e.Extract(context.Background(), "https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt---wisdomtree-megatrends-ucits-etf---usd-acc")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// FundInfo
	if result.FundInfo == nil {
		t.Fatal("FundInfo is nil")
	}
	if result.FundInfo.Symbol != "WMGT" {
		t.Errorf("expected symbol WMGT, got %s", result.FundInfo.Symbol)
	}
	if result.FundInfo.Name == "" {
		t.Error("FundInfo.Name is empty")
	}

	// AsOfDate
	if result.AsOfDate.IsZero() {
		t.Error("AsOfDate is zero")
	}

	// Holdings (from modal, with tickers)
	if len(result.Holdings) < 500 {
		t.Errorf("expected at least 500 holdings, got %d", len(result.Holdings))
	}
	// First holding should have a ticker from the modal
	if len(result.Holdings) > 0 && result.Holdings[0].Symbol == "" {
		t.Errorf("first holding missing symbol (modal should provide tickers): %s", result.Holdings[0].Name)
	}

	// NavHistory
	if len(result.NavHistory) < 100 {
		t.Errorf("expected at least 100 NAV points, got %d", len(result.NavHistory))
	}

	// Themes
	if len(result.Themes) < 10 {
		t.Errorf("expected at least 10 themes, got %d", len(result.Themes))
	}

	// Sectors — the critical check
	if len(result.Sectors) < 10 {
		t.Errorf("expected at least 10 sectors, got %d", len(result.Sectors))
	} else {
		// Verify known sectors exist
		sectorNames := make(map[string]bool)
		for _, s := range result.Sectors {
			sectorNames[s.Sector] = true
		}
		expectedSectors := []string{"Information Technology", "Industrials", "Health Care"}
		for _, name := range expectedSectors {
			if !sectorNames[name] {
				t.Errorf("expected sector %q not found in %v", name, sectorNames)
			}
		}
	}

	// CountryAllocation
	if len(result.CountryAllocation) < 20 {
		t.Errorf("expected at least 20 countries, got %d", len(result.CountryAllocation))
	}
	if result.CountryAllocation[0].Country != "United States" {
		t.Errorf("expected first country United States, got %s", result.CountryAllocation[0].Country)
	}

	// MarketCap
	if result.MarketCap == nil {
		t.Error("MarketCap is nil")
	} else {
		if result.MarketCap.Total == 0 {
			t.Error("MarketCap.Total is zero")
		}
		if result.MarketCap.Large == 0 {
			t.Error("MarketCap.Large is zero")
		}
	}

	// Characteristics
	if result.Characteristics == nil {
		t.Error("Characteristics is nil")
	} else {
		if result.Characteristics.PriceToEarnings == 0 {
			t.Error("Characteristics.PriceToEarnings is zero")
		}
		if result.Characteristics.DividendYield == 0 {
			t.Error("Characteristics.DividendYield is zero")
		}
	}

	// FundProfile
	if result.FundProfile == nil {
		t.Error("FundProfile is nil")
	} else {
		if result.FundProfile.TotalNetAssets == 0 {
			t.Error("FundProfile.TotalNetAssets is zero")
		}
		if result.FundProfile.AnnualExpenseRatio == 0 {
			t.Error("FundProfile.AnnualExpenseRatio is zero")
		}
		if result.FundProfile.Isin == "" {
			t.Error("FundProfile.Isin is empty")
		}
		if result.FundProfile.BaseCurrency == "" {
			t.Error("FundProfile.BaseCurrency is empty")
		}
	}

	t.Logf("Extract: symbol=%s, holdings=%d (with tickers), nav=%d, themes=%d, sectors=%d, countries=%d, AUM=%.0f",
		result.FundInfo.Symbol, len(result.Holdings), len(result.NavHistory),
		len(result.Themes), len(result.Sectors), len(result.CountryAllocation),
		result.FundProfile.TotalNetAssets)
}
