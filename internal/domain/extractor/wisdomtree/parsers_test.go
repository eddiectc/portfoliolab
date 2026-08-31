package wisdomtree

import (
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

func TestParseHoldingsFromAPI(t *testing.T) {
	t.Run("full mapping with cash and currency filter", func(t *testing.T) {
		figi := "BBG000BBJQV0"
		sector := "Information Technology"
		ticker := "NVDA UQ"
		records := []holdingRecord{
			{
				SecurityName:    "Nvidia Corp",
				SecurityTicker:  &ticker,
				Wgt:             0.1476209845990145,
				DT:              "2026-08-27T00:00:00.000Z",
				AssetGroup:      "EQ",
				MarketValueBase: 7003317.62,
				Shares:          30719,
				SectorName:      &sector,
				Figi:            &figi,
			},
			{
				SecurityName:   "CASH W-O",
				SecurityTicker: nil, // cash: no ticker, filtered out
			},
			{
				SecurityName:   "JAPANESE YEN",
				SecurityTicker: nil, // currency basket: no ticker, filtered out
			},
		}
		got := ParseHoldingsFromAPI(records)
		if len(got) != 1 {
			t.Fatalf("got %d holdings, want 1 (cash and currency filtered)", len(got))
		}
		h := got[0]
		if h.Symbol != "NVDA" || h.Name != "Nvidia Corp" {
			t.Errorf("got %+v, want NVDA/Nvidia Corp", h)
		}
		if h.Percent != 14.7620984599 { // wgt 0.1476209845990145 * 100, float artifacts stripped
			t.Errorf("Percent = %v, want 14.7620984599 (wgt*100)", h.Percent)
		}
		if h.AsOfDate != "2026-08-27" {
			t.Errorf("AsOfDate = %q, want 2026-08-27", h.AsOfDate)
		}
		if h.AssetClass != "Equity" {
			t.Errorf("AssetClass = %q, want Equity", h.AssetClass)
		}
		if h.MarketValue != 7003317.62 || h.Shares != 30719 {
			t.Errorf("MarketValue/Shares = %v/%v", h.MarketValue, h.Shares)
		}
		if h.Sector != "Information Technology" {
			t.Errorf("Sector = %q", h.Sector)
		}
		if h.ISIN != "BBG000BBJQV0" {
			t.Errorf("ISIN (FIGI) = %q", h.ISIN)
		}
	})

	t.Run("whitespace-only ticker is filtered", func(t *testing.T) {
		ticker := "   "
		got := ParseHoldingsFromAPI([]holdingRecord{{SecurityName: "CASH", SecurityTicker: &ticker}})
		if len(got) != 0 {
			t.Errorf("got %d holdings, want 0", len(got))
		}
	})

	t.Run("asset group mapping and unknown passthrough", func(t *testing.T) {
		ticker := "X UQ"
		got := ParseHoldingsFromAPI([]holdingRecord{
			{SecurityTicker: &ticker, AssetGroup: "BD"},
			{SecurityTicker: &ticker, AssetGroup: "CA"},
			{SecurityTicker: &ticker, AssetGroup: "DER"},
			{SecurityTicker: &ticker, AssetGroup: "XXX"},
		})
		want := []string{"Bond", "Cash", "Derivative", "XXX"}
		for i, h := range got {
			if h.AssetClass != want[i] {
				t.Errorf("record %d AssetClass = %q, want %q", i, h.AssetClass, want[i])
			}
		}
	})
}

func TestFundInfoFromHistory(t *testing.T) {
	t.Run("latest record, ticker suffix stripped", func(t *testing.T) {
		info, err := FundInfoFromHistory([]historyPoint{
			{Name: "Old Name", Ticker: "OLD UQ"},
			{Name: "WisdomTree US Quality Growth UCITS ETF - USD Acc", Ticker: "QGRW LN"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Symbol != "QGRW" {
			t.Errorf("Symbol = %q, want QGRW", info.Symbol)
		}
		if info.Name != "WisdomTree US Quality Growth UCITS ETF - USD Acc" {
			t.Errorf("Name = %q", info.Name)
		}
	})

	t.Run("empty history", func(t *testing.T) {
		if _, err := FundInfoFromHistory(nil); err == nil {
			t.Error("expected error for empty history")
		}
	})

	t.Run("record without ticker", func(t *testing.T) {
		if _, err := FundInfoFromHistory([]historyPoint{{Name: "X"}}); err == nil {
			t.Error("expected error for missing ticker")
		}
	})
}

func TestParseNavHistoryFromAPI(t *testing.T) {
	t.Run("dates, scaling, currency", func(t *testing.T) {
		pts, err := ParseNavHistoryFromAPI([]historyPoint{
			{DT: "2024-04-16T00:00:00.000Z", NAV: 25.039, AUM: 1000},
			{DT: "2026-08-28T00:00:00.000Z", NAV: 42.9737, AUM: 47442.9648},
		}, "USD")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pts) != 2 {
			t.Fatalf("got %d points, want 2", len(pts))
		}
		if pts[0].Date.Format("2006-01-02") != "2024-04-16" {
			t.Errorf("first date = %v", pts[0].Date)
		}
		if pts[1].NAV.String() != "42.9737" {
			t.Errorf("last NAV = %v, want 42.9737", pts[1].NAV)
		}
		if pts[1].Currency != "USD" {
			t.Errorf("currency = %q, want USD", pts[1].Currency)
		}
	})

	t.Run("empty history", func(t *testing.T) {
		if _, err := ParseNavHistoryFromAPI(nil, "USD"); err == nil {
			t.Error("expected error for empty history")
		}
	})

	t.Run("bad date fails", func(t *testing.T) {
		if _, err := ParseNavHistoryFromAPI([]historyPoint{{DT: "not-a-date", NAV: 1}}, "USD"); err == nil {
			t.Error("expected error for unparsable date")
		}
	})
}

func TestLatestAUM(t *testing.T) {
	t.Run("thousands to whole units", func(t *testing.T) {
		aum, err := LatestAUM([]historyPoint{{AUM: 47442.9648}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if aum != 47442965 {
			t.Errorf("AUM = %v, want 47442965", aum)
		}
	})

	t.Run("empty history", func(t *testing.T) {
		if _, err := LatestAUM(nil); err == nil {
			t.Error("expected error for empty history")
		}
	})
}

// qgrwFlight decodes the captured QGRW tables page once per test process.
func qgrwFlight(t *testing.T) *Flight {
	t.Helper()
	return DecodeFlight(loadFixture(t, "flight_qgrw_tables.html"))
}

func ezFlight(t *testing.T) *Flight {
	t.Helper()
	return DecodeFlight(loadFixture(t, "flight_ezm_tables.html"))
}

func TestParseFundProfileFromFlight(t *testing.T) {
	t.Run("qgrw UCITS page", func(t *testing.T) {
		p, err := ParseFundProfileFromFlight(qgrwFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("profile is nil")
		}
		if p.Isin != "IE000YGEAK03" {
			t.Errorf("Isin = %q", p.Isin)
		}
		if p.AssetClassification != "Equities" {
			t.Errorf("AssetClassification = %q", p.AssetClassification)
		}
		if p.BaseCurrency != "USD" {
			t.Errorf("BaseCurrency = %q", p.BaseCurrency)
		}
		if p.DistributionStrategy != "Accumulating" {
			t.Errorf("DistributionStrategy = %q", p.DistributionStrategy)
		}
		if p.InceptionDate.Format("2006-01-02") != "2024-04-16" {
			t.Errorf("InceptionDate = %v", p.InceptionDate)
		}
		if p.AnnualExpenseRatio != 0.0033 {
			t.Errorf("AnnualExpenseRatio = %v, want 0.0033 (0.33%% TER)", p.AnnualExpenseRatio)
		}
		if p.Domicile != "Ireland" {
			t.Errorf("Domicile = %q", p.Domicile)
		}
		if p.IssuingCompany != "WisdomTree Issuer ICAV" {
			t.Errorf("IssuingCompany = %q", p.IssuingCompany)
		}
		if p.Custodian == "" || p.FundManager == "" {
			t.Errorf("Custodian/FundManager = %q/%q, want non-empty", p.Custodian, p.FundManager)
		}
	})

	t.Run("ezm US page (no ISIN, no Fees table)", func(t *testing.T) {
		p, err := ParseFundProfileFromFlight(ezFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("profile is nil")
		}
		if p.Isin != "" || p.BaseCurrency != "" {
			t.Errorf("US page should leave Isin/BaseCurrency empty, got %q/%q", p.Isin, p.BaseCurrency)
		}
		if p.InceptionDate.Format("2006-01-02") != "2007-02-23" {
			t.Errorf("InceptionDate = %v", p.InceptionDate)
		}
		if p.AnnualExpenseRatio != 0.0038 {
			t.Errorf("AnnualExpenseRatio = %v, want 0.0038 (0.38%% expense ratio)", p.AnnualExpenseRatio)
		}
	})

	t.Run("no overview tables", func(t *testing.T) {
		p, err := ParseFundProfileFromFlight(&Flight{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p != nil {
			t.Errorf("got %+v, want nil", p)
		}
	})
}

func TestParseCountryAllocationFromFlight(t *testing.T) {
	t.Run("qgrw", func(t *testing.T) {
		got, err := ParseCountryAllocationFromFlight(qgrwFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 4 {
			t.Fatalf("got %d countries, want 4", len(got))
		}
		if got[0].Country != "United States" || got[0].Percent != 99.54 {
			t.Errorf("top country = %+v, want United States 99.54", got[0])
		}
	})

	t.Run("ezm", func(t *testing.T) {
		got, err := ParseCountryAllocationFromFlight(ezFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 7 {
			t.Fatalf("got %d countries, want 7", len(got))
		}
		if got[0].Country != "United States" || got[0].Percent != 96.84 {
			t.Errorf("top country = %+v, want United States 96.84", got[0])
		}
	})

	t.Run("absent table", func(t *testing.T) {
		got, err := ParseCountryAllocationFromFlight(&Flight{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestParseMarketCapFromFlight(t *testing.T) {
	t.Run("qgrw (trillions)", func(t *testing.T) {
		mc, err := ParseMarketCapFromFlight(qgrwFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mc == nil {
			t.Fatal("market cap is nil")
		}
		if mc.Total != 34.81 || mc.Large != 100.00 || mc.Mid != 0 || mc.Small != 0 {
			t.Errorf("MarketCap = %+v", mc)
		}
	})

	t.Run("ezm (billion-scale fund)", func(t *testing.T) {
		mc, err := ParseMarketCapFromFlight(ezFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mc == nil {
			t.Fatal("market cap is nil")
		}
		if mc.Total != 3.86 || mc.Large != 38.49 || mc.Mid != 61.39 || mc.Small != 0.12 {
			t.Errorf("MarketCap = %+v", mc)
		}
	})

	t.Run("absent table", func(t *testing.T) {
		mc, err := ParseMarketCapFromFlight(&Flight{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mc != nil {
			t.Errorf("got %+v, want nil", mc)
		}
	})
}

func TestParseFundCharacteristicsFromFlight(t *testing.T) {
	t.Run("qgrw", func(t *testing.T) {
		c, err := ParseFundCharacteristicsFromFlight(qgrwFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c == nil {
			t.Fatal("characteristics is nil")
		}
		if c.DividendYield != 0.35 || c.PriceToEarnings != 31.43 {
			t.Errorf("DY/PE = %v/%v, want 0.35/31.43", c.DividendYield, c.PriceToEarnings)
		}
		if !c.HasCharacteristic(extractor.CharacteristicDividendYield) ||
			!c.HasCharacteristic(extractor.CharacteristicPriceToEarnings) {
			t.Errorf("FieldsPresent = %v, want DY and PE bits set", c.FieldsPresent)
		}
	})

	t.Run("ezm", func(t *testing.T) {
		c, err := ParseFundCharacteristicsFromFlight(ezFlight(t))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c == nil {
			t.Fatal("characteristics is nil")
		}
		if c.DividendYield != 1.52 || c.PriceToEarnings != 16.64 {
			t.Errorf("DY/PE = %v/%v, want 1.52/16.64", c.DividendYield, c.PriceToEarnings)
		}
		if !c.HasCharacteristic(extractor.CharacteristicDividendYield) ||
			!c.HasCharacteristic(extractor.CharacteristicPriceToEarnings) {
			t.Errorf("FieldsPresent = %v, want DY and PE bits set", c.FieldsPresent)
		}
	})

	t.Run("partial table sets only present bits", func(t *testing.T) {
		f := &Flight{tables: []*Table{{
			FirstColumn: "Portfolio Characteristics",
			Rows:        []KV{{Label: "Dividend Yield", Value: "0.35%"}},
		}}}
		c, err := ParseFundCharacteristicsFromFlight(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c == nil {
			t.Fatal("characteristics is nil")
		}
		if c.DividendYield != 0.35 {
			t.Errorf("DividendYield = %v, want 0.35", c.DividendYield)
		}
		if !c.HasCharacteristic(extractor.CharacteristicDividendYield) {
			t.Error("dividend yield bit not set")
		}
		if c.HasCharacteristic(extractor.CharacteristicPriceToEarnings) {
			t.Error("price-to-earnings bit set, want clear")
		}
	})

	t.Run("table with no parseable values", func(t *testing.T) {
		f := &Flight{tables: []*Table{{
			FirstColumn: "Portfolio Characteristics",
			Rows:        []KV{{Label: "Price/Earnings", Value: "n/a"}},
		}}}
		c, err := ParseFundCharacteristicsFromFlight(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c != nil {
			t.Errorf("got %+v, want nil", c)
		}
	})

	t.Run("absent table", func(t *testing.T) {
		c, err := ParseFundCharacteristicsFromFlight(&Flight{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c != nil {
			t.Errorf("got %+v, want nil", c)
		}
	})
}

func TestParseSectorsAndThemesFromFlight(t *testing.T) {
	t.Run("qgrw has sectors, no themes", func(t *testing.T) {
		sectors, err := ParseSectorsFromFlight(DecodeFlight(loadFixture(t, "flight_qgrw_sector.html")))
		if err != nil {
			t.Fatalf("sectors: %v", err)
		}
		if len(sectors) != 9 {
			t.Fatalf("got %d sectors, want 9", len(sectors))
		}
		if sectors[0].Sector != "Information Technology" || sectors[0].Percent != 58.4027 {
			t.Errorf("top sector = %+v", sectors[0])
		}
		if sectors[0].Date != "2026-08-27" {
			t.Errorf("sector date = %q", sectors[0].Date)
		}
		themes, err := ParseThemesFromFlight(DecodeFlight(loadFixture(t, "flight_qgrw_sector.html")))
		if err != nil {
			t.Fatalf("themes: %v", err)
		}
		if themes != nil {
			t.Errorf("themes = %v, want nil", themes)
		}
	})

	t.Run("wmgt themes", func(t *testing.T) {
		themes, err := ParseThemesFromFlight(DecodeFlight(loadFixture(t, "flight_wmgt_theme.html")))
		if err != nil {
			t.Fatalf("themes: %v", err)
		}
		if len(themes) != 19 {
			t.Fatalf("got %d themes, want 19", len(themes))
		}
		if themes[0].Name != "Grid Infrastructure" || themes[0].Percent != 8.1002 {
			t.Errorf("top theme = %+v", themes[0])
		}
	})

	t.Run("absent sections", func(t *testing.T) {
		if s, err := ParseSectorsFromFlight(&Flight{}); err != nil || s != nil {
			t.Errorf("sectors = %v, %v; want nil, nil", s, err)
		}
		if s, err := ParseThemesFromFlight(&Flight{}); err != nil || s != nil {
			t.Errorf("themes = %v, %v; want nil, nil", s, err)
		}
	})
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		in      string
		wantDay string
		wantErr bool
	}{
		{"22 May 2026", "2026-05-22", false},
		{"1 May 2026", "2026-05-01", false},
		{"02 Jun 2026", "2026-06-02", false},
		{"2 Jun 2026", "2026-06-02", false},
		{"22/05/2026", "2026-05-22", false},
		{"2/5/2026", "2026-05-02", false},
		{"2026-05-22", "2026-05-22", false},
		{"2/23/2007", "2007-02-23", false}, // US month-first
		{"  22 May 2026  ", "2026-05-22", false},
		{"not a date", "", true},
	}
	for _, tt := range tests {
		got, err := parseDate(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseDate(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDate(%q): %v", tt.in, err)
			continue
		}
		if got.Format("2006-01-02") != tt.wantDay {
			t.Errorf("parseDate(%q) = %s, want %s", tt.in, got.Format("2006-01-02"), tt.wantDay)
		}
	}
}

func TestParseNumber(t *testing.T) {
	tests := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"34.81", 34.81, true},
		{"1,234.56", 1234.56, true},
		{"0.35%", 0.35, true},
		{"€1.20", 1.20, true},
		{"$100", 100, true},
		{"  42  ", 42, true},
		{"", 0, false},
		{"n/a", 0, false},
	}
	for _, tt := range tests {
		got, err := parseNumber(tt.in)
		if !tt.ok {
			if err == nil {
				t.Errorf("parseNumber(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNumber(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseNumber(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestExtractTicker(t *testing.T) {
	tests := []struct{ in, want string }{
		{"NVDA UQ", "NVDA"},
		{"US5128073062", "US5128073062"}, // CUSIP kept as-is
		{"  AAPL UQ  ", "AAPL"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractTicker(tt.in); got != tt.want {
			t.Errorf("extractTicker(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestParseAsOfDate covers the NAV table header date parsing (both site
// regions: UCITS "As of 28/08/2026", US "As of 8/28/2026").
func TestParseAsOfDate(t *testing.T) {
	f := qgrwFlight(t)
	tbl := f.Table("Net Asset Value")
	if tbl == nil {
		t.Fatal("no Net Asset Value table in qgrw fixture")
	}
	d, ok := tbl.AsOfDate()
	if !ok {
		t.Fatal("no as-of date")
	}
	if d.Format("2006-01-02") != "2026-08-28" {
		t.Errorf("as-of = %v, want 2026-08-28", d)
	}

	fe := ezFlight(t)
	tbl = fe.Table("Net Asset Value")
	if tbl == nil {
		t.Fatal("no Net Asset Value table in ezm fixture")
	}
	d, ok = tbl.AsOfDate()
	if !ok {
		t.Fatal("no as-of date")
	}
	if d.Format("2006-01-02") != "2026-08-28" {
		t.Errorf("as-of = %v, want 2026-08-28 (US format 8/28/2026)", d)
	}
}
