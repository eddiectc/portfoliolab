package wisdomtree

// Parsers for the new WisdomTree site (2026 relaunch). Data sources:
//
//   - fund holdings and NAV/AUM history come from the undocumented JSON API
//     (client.go: FundHoldings / FundHistory, RESEARCH.md §5)
//   - overview, fees, country allocation, market capitalisation, fund
//     characteristics, sector and theme breakdowns come from the React
//     Flight payload embedded in the fund page (flight.go)
//
// All parsers return the shared extractor.FundProfile-family contract.
// Optional sections (missing on some funds) return nil, nil; required data
// returns an error. Unit conventions: percentages are stored as numbers the
// site displays ("0.33%" → 0.33, "14.76%" → 14.76); AUM is in whole units of
// the fund's base currency.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"github.com/govalues/decimal"
)

// ParseHoldingsFromAPI converts fund-holdings API rows into holdings.
//
// Rows without a security ticker are cash-like positions (cash, currency
// baskets, money-market instruments) and are skipped: the API omits the
// ticker exactly for the rows the old site's name-keyword filter excluded,
// while keyword matching mis-filtered real tickers such as FirstCash
// Holdings (name contains "CASH"). Percent is the API weight fraction
// scaled to the percentage convention (0.1476 → 14.76). The identifier slot
// carries the row's FIGI (the API does not return per-holding ISINs).
func ParseHoldingsFromAPI(records []holdingRecord) []extractor.Holding {
	holdings := make([]extractor.Holding, 0, len(records))
	for _, r := range records {
		if r.SecurityTicker == nil || strings.TrimSpace(*r.SecurityTicker) == "" {
			continue // cash / currency / derivative position
		}
		asOf := r.DT // "2026-08-27T00:00:00.000Z" → "2026-08-27"
		if len(asOf) >= 10 {
			asOf = asOf[:10]
		}
		holding := extractor.Holding{
			Symbol:      extractTicker(*r.SecurityTicker),
			Name:        r.SecurityName,
			Percent:     round10(r.Wgt * 100),
			AsOfDate:    asOf,
			AssetClass:  assetGroupLabel(r.AssetGroup),
			MarketValue: r.MarketValueBase,
			Shares:      r.Shares,
		}
		if r.SectorName != nil {
			holding.Sector = *r.SectorName
		}
		if r.Figi != nil {
			holding.ISIN = *r.Figi // FIGI; "-" slot convention for identifiers
		}
		holdings = append(holdings, holding)
	}
	return holdings
}

// assetGroupLabel maps the API's asset-group codes to display labels.
// Unknown codes are passed through rather than dropped.
func assetGroupLabel(code string) string {
	switch code {
	case "EQ":
		return "Equity"
	case "BD":
		return "Bond"
	case "CA":
		return "Cash"
	case "DER":
		return "Derivative"
	default:
		return code
	}
}

// FundInfoFromHistory derives the fund's name and ticker from the latest
// fund-history record (the API returns no dedicated fund-info endpoint).
func FundInfoFromHistory(points []historyPoint) (*extractor.FundInfo, error) {
	if len(points) == 0 {
		return nil, fmt.Errorf("fund-history returned no records")
	}
	latest := points[len(points)-1]
	if latest.Ticker == "" {
		return nil, fmt.Errorf("fund-history record has no ticker")
	}
	return &extractor.FundInfo{
		Name:   latest.Name,
		Symbol: strings.Fields(latest.Ticker)[0],
	}, nil
}

// ParseNavHistoryFromAPI converts fund-history API records (ascending since
// inception) into NAV points. Currency is the currency the NAV is quoted in
// (the fund's base currency from the page, or USD when absent).
func ParseNavHistoryFromAPI(points []historyPoint, currency string) ([]extractor.NavPoint, error) {
	if len(points) == 0 {
		return nil, fmt.Errorf("fund-history returned no records")
	}
	nav := make([]extractor.NavPoint, 0, len(points))
	for i, p := range points {
		d, err := time.Parse(time.RFC3339, p.DT)
		if err != nil {
			return nil, fmt.Errorf("row %d: parse date %q: %w", i, p.DT, err)
		}
		nav = append(nav, extractor.NavPoint{
			Date:     d,
			NAV:      decimal.MustParse(fmt.Sprintf("%.4f", p.NAV)),
			Currency: currency,
		})
	}
	return nav, nil
}

// LatestAUM returns the most recent AUM from fund-history records, in whole
// units of the fund's base currency. The API reports thousands
// (47442.9648 → $47,442,965, matching the page's "Total AUM of fund" and
// the US pages' "Total Assets (000)").
func LatestAUM(points []historyPoint) (float64, error) {
	if len(points) == 0 {
		return 0, fmt.Errorf("fund-history returned no records")
	}
	aumThousands := points[len(points)-1].AUM
	return float64(int64(aumThousands*1e3 + 0.5)), nil
}

// ParseFundProfileFromFlight extracts fund metadata from the flight-embedded
// tables (Product Overview, Fees, Structure, Key Service Providers). Both
// regions are supported: UCITS pages carry ISIN/base currency in the
// overview and the TER in the Fees table; US pages carry the expense ratio
// in the overview and have no ISIN or Fees table. Returns nil, nil when the
// page carries no overview data.
func ParseFundProfileFromFlight(f *Flight) (*extractor.FundProfile, error) {
	ov := f.Table("Product Overview")
	fees := f.Table("Fees")
	if ov == nil && fees == nil {
		return nil, nil
	}
	p := &extractor.FundProfile{}
	if ov != nil {
		p.Isin = tableValue(ov, "ISIN")
		p.AssetClassification = tableValue(ov, "Asset Class")
		p.BaseCurrency = tableValue(ov, "Base Currency")
		p.DistributionStrategy = tableValue(ov, "Use of Income")
		if v, ok := ov.Value("Inception Date"); ok {
			if d, err := parseDate(v); err == nil {
				p.InceptionDate = d
			}
		}
	}
	// TER: UCITS pages report it in the Fees table; US pages embed the
	// expense ratio in the overview table.
	terRaw := tableValue(fees, "Total expense ratio (TER)")
	if terRaw == "" {
		terRaw = tableValue(ov, "Expense Ratio")
	}
	if terRaw != "" {
		if v, err := parseNumber(terRaw); err == nil {
			p.AnnualExpenseRatio = v / 100 // site displays percent; contract wants a fraction
		}
	}
	if structure := f.Table("Structure"); structure != nil {
		p.LegalType = tableValue(structure, "Legal Form")
		p.ProductStructure = tableValue(structure, "Structure")
		p.Methodology = tableValue(structure, "Replication Method")
		p.Domicile = tableValue(structure, "Domicile")
		p.IssuingCompany = tableValue(structure, "Issuer")
	}
	if ksp := f.Table("Key Service Providers Table"); ksp != nil {
		p.Custodian = tableValue(ksp, "Custodian")
		p.FundManager = tableValue(ksp, "Fund Manager")
	}
	return p, nil
}

// ParseCountryAllocationFromFlight extracts the fund-level country
// allocation from the Country Allocation table, in page order (descending by
// weight). Returns nil, nil when the table is absent.
func ParseCountryAllocationFromFlight(f *Flight) ([]extractor.CountryAllocation, error) {
	t := f.Table("Country Allocation Table", "Country")
	if t == nil {
		return nil, nil
	}
	var countries []extractor.CountryAllocation
	for _, kv := range t.Rows {
		v, err := parseNumber(kv.Value)
		if err != nil || kv.Label == "" {
			continue
		}
		countries = append(countries, extractor.CountryAllocation{
			Country: kv.Label,
			Percent: v,
		})
	}
	return countries, nil
}

// ParseMarketCapFromFlight extracts the market capitalisation breakdown.
// Total is in the displayed unit (trillions of USD); Large/Mid/Small are
// percentages of the fund. The component labels are embedded in React
// element cells, so they are matched by prefix. Returns nil, nil when the
// table is absent.
func ParseMarketCapFromFlight(f *Flight) (*extractor.MarketCapBreakdown, error) {
	t := f.Table("Market Capitalisation", "Market Capitalization")
	if t == nil {
		return nil, nil
	}
	b := &extractor.MarketCapBreakdown{}
	if v, err := parseNumber(tableValuePrefix(t, "Total Market Capitali")); err == nil {
		b.Total = v
	}
	for _, kv := range t.Rows {
		v, err := parseNumber(kv.Value)
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(kv.Label, "Large Cap"):
			b.Large = v
		case strings.HasPrefix(kv.Label, "Mid Cap"):
			b.Mid = v
		case strings.HasPrefix(kv.Label, "Small Cap"):
			b.Small = v
		}
	}
	if b.Total == 0 && b.Large == 0 && b.Mid == 0 && b.Small == 0 {
		return nil, nil
	}
	return b, nil
}

// ParseFundCharacteristicsFromFlight extracts valuation metrics from the
// portfolio characteristics table. UCITS pages label the table "Fund
// characteristic" (asterisked dividend yield); US pages label it "Portfolio
// Characteristics" (plain dividend yield). All values are in the displayed
// units (percentages as numbers, ratios as displayed). Returns nil, nil when
// the table is absent or carries no usable values.
func ParseFundCharacteristicsFromFlight(f *Flight) (*extractor.FundCharacteristics, error) {
	t := f.Table("Portfolio Characteristics Table", "Fund characteristic", "Portfolio Characteristics")
	if t == nil {
		return nil, nil
	}
	c := &extractor.FundCharacteristics{}
	set := func(flag extractor.CharacteristicsFieldsMask, dst *float64, labels ...string) {
		v, err := parseNumber(tableValueAnyRaw(t, labels...))
		if err != nil {
			return
		}
		*dst = v
		c.FieldsPresent |= flag
	}
	set(extractor.CharacteristicDividendYield, &c.DividendYield, "*Dividend Yield", "Dividend Yield")
	set(extractor.CharacteristicPriceToEarnings, &c.PriceToEarnings, "Price/Earnings")
	set(extractor.CharacteristicEstimatedPriceToEarnings, &c.EstimatedPriceToEarnings, "Estimated Price/Earnings")
	set(extractor.CharacteristicPriceToBook, &c.PriceToBook, "Price/Book")
	set(extractor.CharacteristicPriceToCashflow, &c.PriceToCashflow, "Price/Cash Flow")
	set(extractor.CharacteristicPriceToSales, &c.PriceToSales, "Price/Sales")
	if c.FieldsPresent == 0 {
		return nil, nil
	}
	return c, nil
}

// ParseSectorsFromFlight extracts the Sector Breakdown ranking section.
// Weights are API fractions scaled to the percentage convention; the date is
// the section's data date (one date shared by all entries). Returns nil, nil
// when the section is absent.
func ParseSectorsFromFlight(f *Flight) ([]extractor.SectorWeighting, error) {
	s := f.SectionByClassification(RankClassificationSector)
	if s == nil {
		return nil, nil
	}
	sectors := make([]extractor.SectorWeighting, 0, len(s.Entries))
	for _, e := range s.Entries {
		sectors = append(sectors, extractor.SectorWeighting{
			Sector:  e.Name,
			Percent: round10(e.Weight * 100),
			Date:    entryDate(e.Date),
		})
	}
	return sectors, nil
}

// ParseThemesFromFlight extracts the Theme Breakdown ranking section.
// Returns nil, nil when the section is absent (not all funds have themes,
// e.g. QGRW).
func ParseThemesFromFlight(f *Flight) ([]extractor.Theme, error) {
	s := f.SectionByClassification(RankClassificationTheme)
	if s == nil {
		return nil, nil
	}
	themes := make([]extractor.Theme, 0, len(s.Entries))
	for _, e := range s.Entries {
		themes = append(themes, extractor.Theme{
			Name:    e.Name,
			Percent: round10(e.Weight * 100),
		})
	}
	return themes, nil
}

// entryDate converts a ranking entry's ISO data date to the "2006-01-02"
// display format; unparseable dates are passed through unchanged.
// round10 rounds to 10 decimal places to strip float64 multiplication
// artifacts (0.584027*100 = 58.402699999999996) while preserving the
// site's full weight precision.
func round10(v float64) float64 {
	return math.Round(v*1e10) / 1e10
}

func entryDate(iso string) string {
	if d, err := time.Parse(time.RFC3339, iso); err == nil {
		return d.Format("2006-01-02")
	}
	return iso
}

// tableValue returns the table's row value for an exact label, or "" when
// absent.
func tableValue(t *Table, label string) string {
	if t == nil {
		return ""
	}
	v, _ := t.Value(label)
	return v
}

// tableValuePrefix returns the value of the first row whose label starts
// with the given prefix (spelling variants across site regions), or "" when
// absent.
func tableValuePrefix(t *Table, prefix string) string {
	if t == nil {
		return ""
	}
	for _, kv := range t.Rows {
		if strings.HasPrefix(kv.Label, prefix) {
			return kv.Value
		}
	}
	return ""
}

// tableValueAnyRaw returns the raw value of the first matching label.
func tableValueAnyRaw(t *Table, labels ...string) string {
	for _, l := range labels {
		if v, ok := t.Value(l); ok {
			return v
		}
	}
	return ""
}

// parseNumber extracts a numeric value from a string, handling %, €, and
// commas.
func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, "€", "")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")

	return strconv.ParseFloat(s, 64)
}

// parseDate tries the date formats found on WisdomTree pages. UCITS pages
// are day-first ("16 April 2024", "27/08/2026"); US pages are month-first
// ("2/23/2007").
func parseDate(dateStr string) (time.Time, error) {
	// Strip leading/trailing whitespace
	dateStr = strings.TrimSpace(dateStr)

	formats := []string{
		"02 January 2006", // "22 May 2026"
		"2 January 2006",  // "1 May 2026"
		"02 Jan 2006",     // "02 Jun 2026" (abbreviated month)
		"2 Jan 2006",      // "2 Jun 2026" (abbreviated month, single digit day)
		"02/01/2006",      // "22/05/2026" (dd/mm/YYYY)
		"2/1/2006",        // "2/5/2026" (d/m/YYYY)
		"2006-01-02",      // "2026-05-22"
		"1/2/2006",        // "2/23/2007" (M/D/YYYY, US pages)
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized date format: %q", dateStr)
}

// extractTicker strips the Bloomberg market suffix from a ticker string.
// "NVDA UQ" -> "NVDA", "US5128073062" -> "US5128073062" (CUSIP kept as-is)
func extractTicker(ticker string) string {
	ticker = strings.TrimSpace(ticker)
	if ticker == "" {
		return ""
	}
	// Bloomberg format: "TICKER SUFFIX" (e.g. "NVDA UQ", "AAPL UQ")
	// CUSIP format: "US5128073062" (no space)
	// Strip the suffix part after the space
	if idx := strings.Index(ticker, " "); idx > 0 {
		return strings.TrimSpace(ticker[:idx])
	}
	return ticker
}
