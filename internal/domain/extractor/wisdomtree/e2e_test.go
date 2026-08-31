package wisdomtree

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// e2eFixture is the captured data for one fund. The page body is the
// concatenation of the partial captures of the same page (each fetch
// streamed only part of the flight payload: the wtClassID capture plus the
// tables and sector-section captures). The holdings and history JSON files
// are the captured API responses for the page's wtClassID.
type e2eFixture struct {
	name      string
	url       string
	wtClassID int
	pages     []string
	holdings  string
	history   string
}

var e2eFixtures = []e2eFixture{
	{
		name:      "qgrw",
		url:       "https://www.wisdomtree.com/gb/products/equities/wisdomtree-us-quality-growth-ucits-etf---usd-acc",
		wtClassID: 49567173,
		pages:     []string{"page_qgrw_wtclassid.txt", "flight_qgrw_tables.html", "flight_qgrw_sector.html"},
		holdings:  "holdings_qgrw.json",
		history:   "fund_history_qgrw.json",
	},
	{
		name:      "ezm",
		url:       "https://www.wisdomtree.com/us/products/equity/ezm",
		wtClassID: 1000518,
		pages:     []string{"page_ezm_wtclassid.txt", "flight_ezm_tables.html", "flight_ezm_sector.html"},
		holdings:  "holdings_ezm.json",
		history:   "fund_history_ezm.json",
	},
}

// TestExtract_EndToEnd runs the full pipeline (page fetch → wtClassID →
// fund-holdings/fund-history APIs → React Flight decode → ExtractResult)
// against captured fixtures for both site regions.
func TestExtract_EndToEnd(t *testing.T) {
	for _, fx := range e2eFixtures {
		t.Run(fx.name, func(t *testing.T) {
			var page strings.Builder
			for _, p := range fx.pages {
				_, _ = page.WriteString(loadFixture(t, p))
			}
			holdings := loadFixture(t, fx.holdings)
			history := loadFixture(t, fx.history)

			var fetched []string
			e := NewExtractor()
			e.client.minDelay = 0
			e.client.SetFetchFunc(func(url string) (string, error) {
				fetched = append(fetched, url)
				switch {
				case strings.Contains(url, "/fund-holdings/"):
					return holdings, nil
				case strings.Contains(url, "/fund-history/"):
					return history, nil
				default:
					return page.String(), nil
				}
			})

			result, err := e.Extract(context.Background(), fx.url)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}

			// Page first, then the two API calls built from the extracted
			// wtClassID.
			wantAPI1 := fmt.Sprintf("https://www.wisdomtree.com/api/fund-holdings/%d", fx.wtClassID)
			wantAPI2 := fmt.Sprintf("https://www.wisdomtree.com/api/fund-history/%d", fx.wtClassID)
			if len(fetched) != 3 || fetched[0] != fx.url ||
				fetched[1] != wantAPI1 || fetched[2] != wantAPI2 {
				t.Fatalf("fetch order = %v, want [%s %s %s]", fetched, fx.url, wantAPI1, wantAPI2)
			}

			if result.Source != "wisdomtree" {
				t.Errorf("Source = %q, want %q", result.Source, "wisdomtree")
			}
			if got := result.AsOfDate.Format("2006-01-02"); got != "2026-08-28" {
				t.Errorf("AsOfDate = %q, want 2026-08-28", got)
			}
			if got, want := len(result.NavHistory), len(parseHistoryFixture(t, fx.history)); got != want {
				t.Errorf("NavHistory len = %d, want %d", got, want)
			}
			nav := result.NavHistory[len(result.NavHistory)-1]
			if got := nav.Date.Format("2006-01-02"); got != "2026-08-28" {
				t.Errorf("last NavHistory date = %q, want 2026-08-28", got)
			}
			if nav.NAV.String() == "" {
				t.Error("last NavHistory NAV is empty")
			}
			if result.FundProfile == nil {
				t.Fatal("FundProfile is nil")
			}
			assertProfile(t, result, fx.name)
			assertHoldings(t, result, fx.name)
		})
	}
}

// parseHistoryFixture parses a captured fund-history API response.
func parseHistoryFixture(t *testing.T, name string) []historyPoint {
	t.Helper()
	var points []historyPoint
	if err := json.Unmarshal([]byte(loadFixture(t, name)), &points); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return points
}

func assertProfile(t *testing.T, result *extractor.ExtractResult, name string) {
	t.Helper()
	p := result.FundProfile
	switch name {
	case "qgrw":
		if p.Isin != "IE000YGEAK03" {
			t.Errorf("Isin = %q, want IE000YGEAK03", p.Isin)
		}
		if p.AssetClassification != "Equities" {
			t.Errorf("AssetClassification = %q, want Equities", p.AssetClassification)
		}
		if p.BaseCurrency != "USD" {
			t.Errorf("BaseCurrency = %q, want USD", p.BaseCurrency)
		}
		if p.DistributionStrategy != "Accumulating" {
			t.Errorf("DistributionStrategy = %q, want Accumulating", p.DistributionStrategy)
		}
		if got := p.InceptionDate.Format("2006-01-02"); got != "2024-04-16" {
			t.Errorf("InceptionDate = %q, want 2024-04-16", got)
		}
		if p.AnnualExpenseRatio != 0.0033 {
			t.Errorf("AnnualExpenseRatio = %v, want 0.0033", p.AnnualExpenseRatio)
		}
		if p.LegalType != "Irish Collective Asset-management Vehicle (ICAV)" {
			t.Errorf("LegalType = %q", p.LegalType)
		}
		if p.Domicile != "Ireland" {
			t.Errorf("Domicile = %q, want Ireland", p.Domicile)
		}
		if p.IssuingCompany != "WisdomTree Issuer ICAV" {
			t.Errorf("IssuingCompany = %q, want WisdomTree Issuer ICAV", p.IssuingCompany)
		}
		if p.Custodian != "The Bank of New York Mellon SA/NV, Dublin Branch" {
			t.Errorf("Custodian = %q", p.Custodian)
		}
		if p.FundManager != "Irish Life Investment Managers" {
			t.Errorf("FundManager = %q", p.FundManager)
		}
		if p.TotalNetAssets != 47442965 {
			t.Errorf("TotalNetAssets = %v, want 47442965", p.TotalNetAssets)
		}
		if result.FundInfo.Name != "WisdomTree US Quality Growth UCITS ETF - USD Acc" {
			t.Errorf("FundInfo.Name = %q", result.FundInfo.Name)
		}
		if result.FundInfo.Symbol != "QGRW" {
			t.Errorf("FundInfo.Symbol = %q, want QGRW", result.FundInfo.Symbol)
		}
		nav := result.NavHistory[len(result.NavHistory)-1]
		if nav.NAV.String() != "42.9737" {
			t.Errorf("last NAV = %v, want 42.9737", nav.NAV)
		}
		if nav.Currency != "USD" {
			t.Errorf("NAV currency = %q, want USD", nav.Currency)
		}
		if result.MarketCap == nil {
			t.Fatal("MarketCap is nil")
		}
		if result.MarketCap.Total != 34.81 || result.MarketCap.Large != 100.00 ||
			result.MarketCap.Mid != 0 || result.MarketCap.Small != 0 {
			t.Errorf("MarketCap = %+v, want Total 34.81 Large 100 Mid 0 Small 0", result.MarketCap)
		}
		if result.Characteristics == nil {
			t.Fatal("Characteristics is nil")
		}
		if result.Characteristics.DividendYield != 0.35 {
			t.Errorf("DividendYield = %v, want 0.35", result.Characteristics.DividendYield)
		}
		if result.Characteristics.PriceToEarnings != 31.43 {
			t.Errorf("PriceToEarnings = %v, want 31.43", result.Characteristics.PriceToEarnings)
		}
		assertCountryRow(t, result, "United States", 99.54)
		if len(result.CountryAllocation) != 4 {
			t.Errorf("CountryAllocation len = %d, want 4", len(result.CountryAllocation))
		}
		assertSectorTop(t, result, "Information Technology", 58.4027, "2026-08-27")
		if len(result.Sectors) != 9 {
			t.Errorf("Sectors len = %d, want 9", len(result.Sectors))
		}
		if result.Themes != nil {
			t.Errorf("Themes = %v, want nil", result.Themes)
		}
	case "ezm":
		if p.Isin != "" || p.AssetClassification != "" || p.BaseCurrency != "" {
			t.Errorf("US page should leave Isin/AssetClassification/BaseCurrency empty, got %q/%q/%q",
				p.Isin, p.AssetClassification, p.BaseCurrency)
		}
		if p.LegalType != "" || p.Custodian != "" {
			t.Errorf("US page should leave LegalType/Custodian empty, got %q/%q", p.LegalType, p.Custodian)
		}
		if got := p.InceptionDate.Format("2006-01-02"); got != "2007-02-23" {
			t.Errorf("InceptionDate = %q, want 2007-02-23", got)
		}
		if p.AnnualExpenseRatio != 0.0038 {
			t.Errorf("AnnualExpenseRatio = %v, want 0.0038", p.AnnualExpenseRatio)
		}
		if p.TotalNetAssets != 946609717 {
			t.Errorf("TotalNetAssets = %v, want 946609717", p.TotalNetAssets)
		}
		if result.FundInfo.Name != "WisdomTree U.S. MidCap Fund" {
			t.Errorf("FundInfo.Name = %q", result.FundInfo.Name)
		}
		if result.FundInfo.Symbol != "EZM" {
			t.Errorf("FundInfo.Symbol = %q, want EZM", result.FundInfo.Symbol)
		}
		nav := result.NavHistory[len(result.NavHistory)-1]
		if nav.NAV.String() != "76.0329" {
			t.Errorf("last NAV = %v, want 76.0329", nav.NAV)
		}
		if nav.Currency != "USD" {
			t.Errorf("NAV currency = %q, want USD (default when base currency absent)", nav.Currency)
		}
		if result.MarketCap == nil {
			t.Fatal("MarketCap is nil")
		}
		mc := result.MarketCap
		if mc.Total != 3.86 || mc.Large != 38.49 || mc.Mid != 61.39 || mc.Small != 0.12 {
			t.Errorf("MarketCap = %+v, want Total 3.86 Large 38.49 Mid 61.39 Small 0.12", mc)
		}
		if result.Characteristics == nil {
			t.Fatal("Characteristics is nil")
		}
		if result.Characteristics.DividendYield != 1.52 {
			t.Errorf("DividendYield = %v, want 1.52", result.Characteristics.DividendYield)
		}
		if result.Characteristics.PriceToEarnings != 16.64 {
			t.Errorf("PriceToEarnings = %v, want 16.64", result.Characteristics.PriceToEarnings)
		}
		assertCountryRow(t, result, "United States", 96.84)
		if len(result.CountryAllocation) != 7 {
			t.Errorf("CountryAllocation len = %d, want 7", len(result.CountryAllocation))
		}
		assertSectorTop(t, result, "Financials", 19.322, "2026-08-28")
		if len(result.Sectors) != 12 {
			t.Errorf("Sectors len = %d, want 12", len(result.Sectors))
		}
		if result.Themes != nil {
			t.Errorf("Themes = %v, want nil", result.Themes)
		}
		// Regression: the currency basket row FirstCash Inc. (no ticker in the
		// API response) must not be dropped by the cash filter.
		found := false
		for _, h := range result.Holdings {
			if strings.Contains(h.Name, "FirstCash") {
				found = true
			}
		}
		if !found {
			t.Error("FirstCash Inc. holding missing — cash filter too aggressive?")
		}
	}
}

func assertHoldings(t *testing.T, result *extractor.ExtractResult, name string) {
	t.Helper()
	wantCount := map[string]int{"qgrw": 100, "ezm": 506}[name]
	wantDate := map[string]string{"qgrw": "2026-08-27", "ezm": "2026-08-28"}[name]
	if got := len(result.Holdings); got != wantCount {
		t.Fatalf("Holdings len = %d, want %d", got, wantCount)
	}
	for _, h := range result.Holdings {
		if h.AssetClass != "Equity" {
			t.Errorf("holding %s AssetClass = %q, want Equity", h.Symbol, h.AssetClass)
			break
		}
		if h.AsOfDate != wantDate {
			t.Errorf("holding %s AsOfDate = %q, want %s", h.Symbol, h.AsOfDate, wantDate)
			break
		}
		if h.Symbol == "" {
			t.Errorf("holding %q has empty Symbol", h.Name)
			break
		}
	}
	switch name {
	case "qgrw":
		first := result.Holdings[0]
		if first.Symbol != "NVDA" || first.Name != "Nvidia Corp" {
			t.Errorf("first holding = %+v, want NVDA/Nvidia Corp", first)
		}
		if first.Percent != 14.7620984599 {
			t.Errorf("first Percent = %v, want 14.7620984599", first.Percent)
		}
		if first.ISIN != "BBG000BBJQV0" {
			t.Errorf("first ISIN = %q, want BBG000BBJQV0 (FIGI)", first.ISIN)
		}
	case "ezm":
		top := result.Holdings[0]
		if top.Symbol != "VTRS" || top.Percent != 1.415342593 {
			t.Errorf("top holding = %s %v, want VTRS 1.415342593", top.Symbol, top.Percent)
		}
	}
}

// TestWMGT_Sections covers the WMGT-specific paths with the partial captures:
// the space-after-colon wtClassID variant, the sector and theme ranking
// sections, and the 893-row holdings parse. The full-page tables were not
// captured for WMGT, so a full Extract run is not possible for it.
func TestWMGT_Sections(t *testing.T) {
	sectorPage := loadFixture(t, "flight_wmgt_sector.html")
	themePage := loadFixture(t, "flight_wmgt_theme.html")

	if id, err := ExtractWtClassID(sectorPage); err != nil || id != 46987205 {
		t.Errorf("ExtractWtClassID(sector) = %d, %v; want 46987205 (space variant)", id, err)
	}
	if id, err := ExtractWtClassID(themePage); err != nil || id != 46987205 {
		t.Errorf("ExtractWtClassID(theme) = %d, %v; want 46987205", id, err)
	}

	sectors, err := ParseSectorsFromFlight(DecodeFlight(sectorPage))
	if err != nil {
		t.Fatalf("ParseSectorsFromFlight: %v", err)
	}
	if len(sectors) != 11 {
		t.Fatalf("Sectors len = %d, want 11", len(sectors))
	}
	if sectors[0].Sector != "Information Technology" || sectors[0].Percent != 31.8072 {
		t.Errorf("top sector = %+v, want Information Technology 31.8072", sectors[0])
	}
	if sectors[0].Date != "2026-08-27" {
		t.Errorf("sector date = %q, want 2026-08-27", sectors[0].Date)
	}

	themes, err := ParseThemesFromFlight(DecodeFlight(themePage))
	if err != nil {
		t.Fatalf("ParseThemesFromFlight: %v", err)
	}
	if len(themes) != 19 {
		t.Fatalf("Themes len = %d, want 19", len(themes))
	}
	if themes[0].Name != "Grid Infrastructure" || themes[0].Percent != 8.1002 {
		t.Errorf("top theme = %+v, want Grid Infrastructure 8.1002", themes[0])
	}

	c := NewClient()
	c.minDelay = 0
	c.SetFetchFunc(func(url string) (string, error) {
		if strings.Contains(url, "/fund-holdings/") {
			return loadFixture(t, "holdings_wmgt.json"), nil
		}
		if strings.Contains(url, "/fund-history/") {
			return loadFixture(t, "fund_history_wmgt.json"), nil
		}
		t.Fatalf("unexpected fetch %q", url)
		return "", nil
	})
	records, err := c.FundHoldings(context.Background(), 46987205)
	if err != nil {
		t.Fatalf("FundHoldings: %v", err)
	}
	holdings := ParseHoldingsFromAPI(records)
	if len(holdings) != 893 {
		t.Errorf("WMGT holdings len = %d, want 893 (920 rows minus 27 no-ticker rows)", len(holdings))
	}

	points, err := c.FundHistory(context.Background(), 46987205)
	if err != nil {
		t.Fatalf("FundHistory: %v", err)
	}
	aum, err := LatestAUM(points)
	if err != nil {
		t.Fatalf("LatestAUM: %v", err)
	}
	if aum != 66226762 {
		t.Errorf("LatestAUM = %v, want 66226762 (66226.7618 thousands)", aum)
	}
}

func assertCountryRow(t *testing.T, result *extractor.ExtractResult, name string, percent float64) {
	t.Helper()
	for _, c := range result.CountryAllocation {
		if c.Country == name {
			if c.Percent != percent {
				t.Errorf("country %s percent = %v, want %v", name, c.Percent, percent)
			}
			return
		}
	}
	t.Errorf("country %q not found in CountryAllocation", name)
}

func assertSectorTop(t *testing.T, result *extractor.ExtractResult, name string, percent float64, date string) {
	t.Helper()
	if len(result.Sectors) == 0 {
		t.Errorf("Sectors is empty, want top %q", name)
		return
	}
	top := result.Sectors[0]
	if top.Sector != name {
		t.Errorf("top sector = %q, want %q", top.Sector, name)
	}
	if top.Percent != percent {
		t.Errorf("top sector percent = %v, want %v", top.Percent, percent)
	}
	if top.Date != date {
		t.Errorf("top sector date = %q, want %q", top.Date, date)
	}
}
