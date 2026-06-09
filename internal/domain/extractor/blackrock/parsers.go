package blackrock

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// --- Phase 1: Product Page HTML Parsers ---

// ParseFundIdentity extracts name and symbol from the product page HTML.
func ParseFundIdentity(html string) (*extractor.FundInfo, error) {
	// Try <h1> tag first
	h1Re := regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
	match := h1Re.FindStringSubmatch(html)
	name := ""
	if match != nil && len(match) > 1 {
		name = strings.TrimSpace(match[1])
	}

	// Fallback to <title> tag
	if name == "" {
		titleRe := regexp.MustCompile(`<title>([^<]+)</title>`)
		match = titleRe.FindStringSubmatch(html)
		if match != nil && len(match) > 1 {
			name = strings.TrimSpace(match[1])
			// Strip " | iShares UK" suffix
			if idx := strings.Index(name, " | iShares"); idx > 0 {
				name = strings.TrimSpace(name[:idx])
			}
		}
	}

	if name == "" {
		return nil, fmt.Errorf("fund name not found in page")
	}

	// Extract symbol from ISIN (used as identifier until we parse the Key Facts)
	// The actual ticker is in the Key Facts as "Bloomberg Ticker"
	return &extractor.FundInfo{
		Name: name,
	}, nil
}

// ParseFundProfile extracts Key Facts from the product page HTML table.
func ParseFundProfile(html string) (*extractor.FundProfile, error) {
	profile := &extractor.FundProfile{}

	// Net Assets: "Net Assets" -> "USD 5,181,115,355 (as of 29/May/2026)"
	if val, err := parseKeyValue(html, "Net Assets"); err == nil {
		profile.TotalNetAssets = extractAUM(val)
	}

	// Inception Date
	if val, err := parseKeyValue(html, "Inception Date"); err == nil {
		if t, err := parseIShareDate(val); err == nil {
			profile.InceptionDate = t
		}
	}

	// Asset Class
	if val, err := parseKeyValue(html, "Asset Class"); err == nil {
		profile.AssetClassification = strings.TrimSpace(val)
	}

	// SFDR Classification
	if val, err := parseKeyValue(html, "SFDR Classification"); err == nil {
		profile.SFDRClassification = strings.TrimSpace(val)
	}

	// Total Expense Ratio
	if val, err := parseKeyValue(html, "Total Expense Ratio"); err == nil {
		profile.AnnualExpenseRatio = parsePercentValue(val)
	}

	// Use of Income (Distribution Strategy)
	if val, err := parseKeyValue(html, "Use of Income"); err == nil {
		profile.DistributionStrategy = strings.TrimSpace(val)
	}

	// Domicile
	if val, err := parseKeyValue(html, "Domicile"); err == nil {
		profile.Domicile = strings.TrimSpace(val)
	}

	// Rebalance Frequency
	if val, err := parseKeyValue(html, "Rebalance Frequency"); err == nil {
		profile.RebalanceFrequency = strings.TrimSpace(val)
	}

	// Fund Manager
	if val, err := parseKeyValue(html, "Fund Manager"); err == nil {
		profile.FundManager = strings.TrimSpace(val)
	}

	// Custodian
	if val, err := parseKeyValue(html, "Custodian"); err == nil {
		profile.Custodian = strings.TrimSpace(val)
	}

	// Bloomberg Ticker
	if val, err := parseKeyValue(html, "Bloomberg Ticker"); err == nil {
		profile.BenchmarkTicker = strings.TrimSpace(val)
	}

	// Benchmark Index
	if val, err := parseKeyValue(html, "Benchmark Index"); err == nil {
		profile.Benchmark = strings.TrimSpace(val)
	}

	// ISIN
	if val, err := parseKeyValue(html, "ISIN"); err == nil {
		profile.Isin = strings.TrimSpace(val)
	}

	// Product Structure
	if val, err := parseKeyValue(html, "Product Structure"); err == nil {
		profile.ProductStructure = strings.TrimSpace(val)
	}

	// Methodology
	if val, err := parseKeyValue(html, "Methodology"); err == nil {
		profile.Methodology = strings.TrimSpace(val)
	}

	// Issuing Company
	if val, err := parseKeyValue(html, "Issuing Company"); err == nil {
		profile.IssuingCompany = strings.TrimSpace(val)
	}

	// Validate: at least some key fields should be present
	if profile.Isin == "" && profile.TotalNetAssets == 0 {
		return nil, fmt.Errorf("fund profile data missing (no ISIN or AUM found)")
	}

	return profile, nil
}

// ParseFundCharacteristics extracts portfolio characteristics from the HTML.
// Handles both equity funds (P/E, P/B) and bond funds (YTM, duration).
func ParseFundCharacteristics(html string) (*extractor.FundCharacteristics, error) {
	chars := &extractor.FundCharacteristics{}

	// Number of Holdings
	if val, err := parseKeyValue(html, "Number of Holdings"); err == nil {
		if n, err := strconv.Atoi(cleanNumber(val)); err == nil {
			chars.NumberOfHoldings = n
			chars.FieldsPresent |= extractor.CharacteristicNumberOfHoldings
		}
	}

	// P/E Ratio (equity funds)
	if val, err := parseKeyValue(html, "P/E Ratio"); err == nil {
		chars.PriceToEarnings = parseFloatValue(val)
		chars.FieldsPresent |= extractor.CharacteristicPriceToEarnings
	}

	// P/B Ratio (equity funds)
	if val, err := parseKeyValue(html, "P/B Ratio"); err == nil {
		chars.PriceToBook = parseFloatValue(val)
		chars.FieldsPresent |= extractor.CharacteristicPriceToBook
	}

	// 3y Beta
	if val, err := parseKeyValue(html, "3y Beta"); err == nil {
		chars.Beta3Y = parseFloatValue(val)
		chars.FieldsPresent |= extractor.CharacteristicBeta3Y
	}

	// Standard Deviation (3y)
	if val, err := parseKeyValue(html, "Standard Deviation (3y)"); err == nil {
		chars.StandardDeviation3Y = parseFloatValue(val)
		chars.FieldsPresent |= extractor.CharacteristicStandardDeviation3Y
	}

	// Bond-specific fields

	// Yield to Maturity
	if val, err := parseKeyValue(html, "Yield to Maturity"); err == nil {
		chars.AverageCoupon = parseFloatValue(val)
		chars.FieldsPresent |= extractor.CharacteristicAverageCoupon
	}

	// Weighted Average YTM
	if val, err := parseKeyValue(html, "Weighted Average YTM"); err == nil {
		// Store in AverageCoupon if YTM not already set (both map to similar concept)
		if chars.AverageCoupon == 0 {
			chars.AverageCoupon = parseFloatValue(val)
			chars.FieldsPresent |= extractor.CharacteristicAverageCoupon
		}
	}

	// Weighted Avg Maturity
	if val, err := parseKeyValue(html, "Weighted Avg Maturity"); err == nil {
		chars.AverageMaturity = parseFloatValue(val)
		chars.FieldsPresent |= extractor.CharacteristicAverageMaturity
	}

	// Modified Duration
	if val, err := parseKeyValue(html, "Modified Duration"); err == nil {
		chars.AverageDuration = parseFloatValue(val)
		chars.FieldsPresent |= extractor.CharacteristicAverageDuration
	}

	// Effective Duration
	if val, err := parseKeyValue(html, "Effective Duration"); err == nil {
		if chars.AverageDuration == 0 {
			chars.AverageDuration = parseFloatValue(val)
			chars.FieldsPresent |= extractor.CharacteristicAverageDuration
		}
	}

	// Validate: at least one characteristic should be present
	if chars.FieldsPresent == 0 && chars.NumberOfHoldings == 0 {
		return nil, nil // optional section
	}

	return chars, nil
}

// ParseComponentID extracts the component ID from the holdings download link.
// Pattern: <a href=".../1506575576011.ajax?fileType=csv...">
func ParseComponentID(html string) (string, error) {
	re := regexp.MustCompile(`/(\d+)\.ajax\?fileType=csv`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("component ID not found in page (holdings download link missing)")
	}
	return match[1], nil
}

// ParseAsOfDate extracts the "as of" date from the page.
// Tries the old "Fund Holdings as of" pattern first, then falls back to
// the new div-based as-of-date elements in product-data-item blocks.
// Returns zero time if no date found (the JSON API provides its own asOfDate).
func ParseAsOfDate(html string) (time.Time, error) {
	// Try old "Fund Holdings as of" pattern first
	oldRe := regexp.MustCompile(`Fund Holdings as of["\s,]*([0-9]{1,2}/[A-Za-z]+/[0-9]{4})`)
	match := oldRe.FindStringSubmatch(html)
	if match != nil && len(match) > 1 {
		return parseIShareDate(match[1])
	}

	// Try new div-based as-of-date elements: <div class="as-of-date">as of DD/Mon/YYYY</div>
	// or <p class="as-of-date">as of DD/Mon/YYYY</p>
	newRe := regexp.MustCompile(`<[^>]*class="as-of-date"[^>]*>\s*as of\s*([0-9]{1,2}/[A-Za-z]+/[0-9]{4})`)
	match = newRe.FindStringSubmatch(html)
	if match != nil && len(match) > 1 {
		return parseIShareDate(match[1])
	}

	// Not found — return zero time; the JSON API provides its own asOfDate
	return time.Time{}, nil
}

// --- Phase 2: CSV Parsers ---

// csvRowLookup provides column-name-based access to a CSV row.
type csvRowLookup struct {
	cols map[string]int
	row  []string
}

func (r csvRowLookup) str(name string) string {
	if idx, ok := r.cols[name]; ok && idx < len(r.row) {
		return strings.TrimSpace(r.row[idx])
	}
	return ""
}

func (r csvRowLookup) float(name string) float64 {
	s := r.str(name)
	if s == "" || s == "-" {
		return 0
	}
	// Strip currency prefix (e.g. "USD 126,187,569.90")
	if idx := strings.Index(s, " "); idx > 0 {
		s = s[idx+1:]
	}
	s = strings.ReplaceAll(s, ",", "")
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

// ParseHoldings parses the holdings CSV download.
// CSV format: title row ("Fund Holdings as of,\"DD/Mon/YYYY\""), blank row,
// header row, then data rows. Columns are read by name from the header,
// so the parser is resilient to column reordering or insertion.
// Returns holdings list and the as-of date string from the title row.
func ParseHoldings(csvData string) ([]extractor.Holding, string, error) {
	// Strip UTF-8 BOM
	csvData = strings.TrimPrefix(csvData, "\xef\xbb\xbf")

	reader := csv.NewReader(strings.NewReader(csvData))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // variable field counts: title (2), header (13), data (13)

	var (
		cols      map[string]int
		asOfDate  string
		hdrFound  bool
	)

	var holdings []extractor.Holding

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read CSV row: %w", err)
		}

		// Skip blank rows
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}

		// First non-blank row is the title with as-of date
		// Identified by "Fund Holdings" prefix (CSV reader strips quotes)
		if !hdrFound && strings.HasPrefix(row[0], "Fund Holdings") {
			combined := strings.Join(row, " ")
			// Extract date from 'Fund Holdings as of, 29/May/2026'
			dateRe := regexp.MustCompile(`([0-9]{1,2}/[A-Za-z]+/[0-9]{4})`)
			if match := dateRe.FindStringSubmatch(combined); match != nil && len(match) > 1 {
				asOfDate = match[1]
			}
			continue
		}

		// Next non-blank row is the header
		if !hdrFound {
			cols = make(map[string]int)
			for i, name := range row {
				cols[strings.TrimSpace(name)] = i
			}
			hdrFound = true
			continue
		}

		// Data row — read columns by name
		lookup := csvRowLookup{cols: cols, row: row}

		holdings = append(holdings, extractor.Holding{
			Symbol:         lookup.str("Ticker"),
			Name:           lookup.str("Name"),
			Percent:        lookup.float("Weight (%)"),
			Sector:         lookup.str("Sector"),
			AssetClass:     lookup.str("Asset Class"),
			MarketValue:    lookup.float("Market Value"),
			NotionalValue:  lookup.float("Notional Value"),
			Shares:         lookup.float("Shares"),
			Price:          lookup.float("Price"),
			ISIN:           "-", // CSV does not include CUSIP/ISIN column
			Location:       lookup.str("Location"),
			Exchange:       lookup.str("Exchange"),
			MarketCurrency: lookup.str("Market Currency"),
		})
	}

	return holdings, asOfDate, nil
}

// DeriveSectorAllocation aggregates holdings by sector and returns sorted sector weights.
func DeriveSectorAllocation(holdings []extractor.Holding) ([]extractor.SectorWeighting, error) {
	sectorMap := make(map[string]float64)

	for _, h := range holdings {
		if h.Sector == "" {
			continue
		}
		// Use absolute weight for aggregation (negative weights are short positions)
		sectorMap[h.Sector] += math.Abs(h.Percent)
	}

	var sectors []extractor.SectorWeighting
	for sector, weight := range sectorMap {
		// Round to 2 decimal places
		weight = mathRound(weight, 2)
		sectors = append(sectors, extractor.SectorWeighting{
			Sector:  sector,
			Percent: weight,
		})
	}

	// Sort descending by weight
	sort.Slice(sectors, func(i, j int) bool {
		return sectors[i].Percent > sectors[j].Percent
	})

	return sectors, nil
}

// DeriveCountryAllocation aggregates holdings by location (country) and returns sorted country weights.
func DeriveCountryAllocation(holdings []extractor.Holding) ([]extractor.CountryAllocation, error) {
	countryMap := make(map[string]float64)

	for _, h := range holdings {
		if h.Location == "" {
			continue
		}
		// Use absolute weight for aggregation (negative weights are short positions)
		countryMap[h.Location] += math.Abs(h.Percent)
	}

	var countries []extractor.CountryAllocation
	for country, weight := range countryMap {
		weight = mathRound(weight, 2)
		countries = append(countries, extractor.CountryAllocation{
			Country: country,
			Percent: weight,
		})
	}

	// Sort descending by weight
	sort.Slice(countries, func(i, j int) bool {
		return countries[i].Percent > countries[j].Percent
	})

	return countries, nil
}

// colClassForKey maps old key-value label names to their new CSS class names
// used in the div-based product-data-item layout.
var colClassForKey = map[string]string{
	"Net Assets":            "totalNetAssets",
	"Inception Date":        "inceptionDate",
	"Asset Class":           "assetClass",
	"SFDR Classification":   "sfdr",
	"Use of Income":         "useOfProfitsCode",
	"Domicile":              "domicile",
	"Rebalance Frequency":   "rebalanceFrequency",
	"Fund Manager":          "fundmanager",
	"Custodian":             "fundCustodian",
	"Bloomberg Ticker":      "bbeqtick",
	"Benchmark Index":       "indexSeriesName",
	"ISIN":                  "isin",
	"Product Structure":     "productStructure",
	"Methodology":           "fundMethodologyTypeCode",
	"Issuing Company":       "issuingCompany",
	"Number of Holdings":    "numHoldings",
	"P/E Ratio":             "priceEarnings",
	"P/B Ratio":             "priceBook",
	"3y Beta":               "threeYrBetaFund",
	"Standard Deviation (3y)": "volatilitySourced3YrAnnualized",
}

// parseKeyValue extracts a value from a key-value table row in the HTML.
// Tries the old <td> table format first, then falls back to the new
// div-based product-data-item format.
func parseKeyValue(html, key string) (string, error) {
	// Try old <td> table format first
	val, err := parseKeyValueFromTable(html, key)
	if err == nil {
		return val, nil
	}

	// Fall back to new div-based format
	colClass, ok := colClassForKey[key]
	if !ok {
		return "", fmt.Errorf("key %q not found", key)
	}
	val, err = parseKeyValueFromDiv(html, colClass)
	if err != nil {
		return "", fmt.Errorf("key %q not found", key)
	}
	return val, nil
}

// parseKeyValueFromTable extracts a value from <td>Key</td><td>Value</td> format.
func parseKeyValueFromTable(html, key string) (string, error) {
	re := regexp.MustCompile(`<td[^>]*>[ \t\n\r]*` + regexp.QuoteMeta(key) + `[ \t\n\r]*</td>\s*<td[^>]*>([^<]+(?:<[^/][^>]*>[^<]*)*)`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("key %q not found in table", key)
	}

	val := match[1]
	tagRe := regexp.MustCompile(`<[^>]+>`)
	val = tagRe.ReplaceAllString(val, "")
	return strings.TrimSpace(val), nil
}

// parseKeyValueFromDiv extracts a value from the new div-based product-data-item format.
// Pattern: <div class="product-data-item col-xxx"><div class="caption">...</div><div class="data">VALUE</div></div>
// Strategy: find the column marker, then extract the <div class="data"> value from the next occurrence.
// This avoids regex issues with deeply nested divs in the caption section.
func parseKeyValueFromDiv(html, colClass string) (string, error) {
	// Find the position of this column class marker
	marker := `col-` + colClass
	idx := strings.Index(html, marker)
	if idx == -1 {
		return "", fmt.Errorf("product-data-item col-%s not found", colClass)
	}

	// From this position, find the next <div class="data"> and extract its value
	remaining := html[idx:]
	dataRe := regexp.MustCompile(`<div class="data">([^<]+)`)
	match := dataRe.FindStringSubmatch(remaining)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("data div not found in col-%s", colClass)
	}

	return strings.TrimSpace(match[1]), nil
}

// extractAUM parses AUM from strings like "USD 5,181,115,355 (as of 29/May/2026)".
func extractAUM(s string) float64 {
	// Extract numeric part (with commas) before any parenthetical
	re := regexp.MustCompile(`([\d,]+)`)
	match := re.FindStringSubmatch(s)
	if match == nil {
		return 0
	}
	numStr := strings.ReplaceAll(match[1], ",", "")
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}
	return val
}

// parsePercentValue parses percentage values like "0.25%" or "5.59%".
func parsePercentValue(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

// parseFloatValue extracts a float from a value string, stripping %, commas, and parenthetical dates.
func parseFloatValue(s string) float64 {
	s = strings.TrimSpace(s)
	// Remove parenthetical date suffix like " (as of 29/May/2026)"
	if idx := strings.Index(s, " (as of"); idx > 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, ",", "")
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return val
}

// cleanNumber strips non-numeric characters except minus sign for integer parsing.
func cleanNumber(s string) string {
	// Remove parenthetical date suffix
	if idx := strings.Index(s, " (as of"); idx > 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, ",", "")
	return strings.TrimSpace(s)
}

// parseIShareDate parses dates in iShares format: "29/May/2026" or "30/Apr/2026".
func parseIShareDate(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)

	formats := []string{
		"02/Jan/2006", // "29/May/2026"
		"2/Jan/2006",  // "1/May/2026"
		"20060102",    // "20260529" (JSON API format)
		"2006-01-02",  // "2026-05-29"
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized iShares date format: %q", dateStr)
}

// mathRound rounds a float64 to the given number of decimal places.
func mathRound(val float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(val*pow) / pow
}
