package blackrock

import (
	"encoding/json"
	"fmt"
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

// ParseAsOfDate extracts the "as of" date from the holdings section.
// Pattern: "Fund Holdings as of" followed by a date in the page.
func ParseAsOfDate(html string) (time.Time, error) {
	re := regexp.MustCompile(`Fund Holdings as of["\s,]*([0-9]{1,2}/[A-Za-z]+/[0-9]{4})`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return time.Time{}, fmt.Errorf("holdings as-of date not found")
	}

	return parseIShareDate(match[1])
}

// --- Phase 2: JSON API Parsers ---

// holdingsJSONResponse is the top-level JSON response from the holdings API.
type holdingsJSONResponse struct {
	AsOfDate string       `json:"asOfDate"`
	AaData   [][]jsonNode `json:"aaData"`
}

// jsonNode represents a cell in the holdings data that can be either a string
// or an object with "display" and "raw" values.
type jsonNode struct {
	Display string  `json:"display"`
	Raw     float64 `json:"raw"`
	Value   string  `json:"value"`
	str     string  // set when the cell is a plain string
}

// UnmarshalJSON handles both string and object cells in the holdings data.
func (n *jsonNode) UnmarshalJSON(data []byte) error {
	// Try as object first
	var obj struct {
		Display string  `json:"display"`
		Raw     float64 `json:"raw"`
		Value   string  `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		n.Display = obj.Display
		n.Raw = obj.Raw
		n.Value = obj.Value
		return nil
	}
	// Fall back to string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("jsonNode: cannot unmarshal as object or string: %w", err)
	}
	n.str = s
	return nil
}

// ParseHoldings parses the holdings JSON API response.
// Each row in aaData is a 13-field array:
// [0] Ticker, [1] Name, [2] Sector, [3] Asset Class,
// [4] Market Value (object), [5] Weight (object), [6] Notional Value (object),
// [7] Shares (object), [8] Identifier (CUSIP/ISIN), [9] Price (object),
// [10] Location, [11] Exchange, [12] Market Currency
// Returns holdings list and the asOfDate from the response.
func ParseHoldings(jsonData string) ([]extractor.Holding, string, error) {
	// Strip UTF-8 BOM
	jsonData = strings.TrimPrefix(jsonData, "\xef\xbb\xbf")

	var resp holdingsJSONResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		return nil, "", fmt.Errorf("unmarshal holdings JSON: %w", err)
	}

	if len(resp.AaData) == 0 {
		return nil, resp.AsOfDate, nil // empty holdings is valid
	}

	var holdings []extractor.Holding
	for _, row := range resp.AaData {
		if len(row) < 13 {
			continue
		}

		ticker := getStringValue(row[0])
		name := getStringValue(row[1])
		sector := getStringValue(row[2])
		assetClass := getStringValue(row[3])

		marketValue := row[4].Raw
		weight := row[5].Raw
		notionalValue := row[6].Raw
		shares := row[7].Raw
		identifier := getStringValue(row[8])
		price := row[9].Raw
		location := getStringValue(row[10])
		exchange := getStringValue(row[11])
		marketCurrency := getStringValue(row[12])

		holdings = append(holdings, extractor.Holding{
			Symbol:         ticker,
			Name:           name,
			Percent:        weight,
			Sector:         sector,
			AssetClass:     assetClass,
			MarketValue:    marketValue,
			NotionalValue:  notionalValue,
			Shares:         shares,
			Price:          price,
			Identifier:     identifier,
			Location:       location,
			Exchange:       exchange,
			MarketCurrency: marketCurrency,
		})
	}

	return holdings, resp.AsOfDate, nil
}

// DeriveSectorAllocation aggregates holdings by sector and returns sorted sector weights.
func DeriveSectorAllocation(holdings []extractor.Holding) ([]extractor.SectorWeighting, error) {
	sectorMap := make(map[string]float64)

	for _, h := range holdings {
		if h.Sector == "" {
			continue
		}
		// Use absolute weight for aggregation (negative weights are short positions)
		sectorMap[h.Sector] += h.Percent
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
		countryMap[h.Location] += h.Percent
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

// --- Helpers ---

// parseKeyValue extracts a value from a key-value table row in the HTML.
// Pattern: <td>Key</td> ... <td>Value</td>
// Handles whitespace and newlines in table cells.
func parseKeyValue(html, key string) (string, error) {
	// Match: <td...> whitespace KEY whitespace </td> ... <td...> VALUE ... </td>
	re := regexp.MustCompile(`<td[^>]*>[ \t\n\r]*` + regexp.QuoteMeta(key) + `[ \t\n\r]*</td>\s*<td[^>]*>([^<]+(?:<[^/][^>]*>[^<]*)*)`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("key %q not found", key)
	}

	val := match[1]
	// Strip inline HTML tags
	tagRe := regexp.MustCompile(`<[^>]+>`)
	val = tagRe.ReplaceAllString(val, "")
	return strings.TrimSpace(val), nil
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

// getStringValue extracts a string value from a jsonNode.
func getStringValue(node jsonNode) string {
	if node.str != "" {
		return node.str
	}
	if node.Value != "" {
		return node.Value
	}
	if node.Display != "" {
		return node.Display
	}
	return ""
}

// mathRound rounds a float64 to the given number of decimal places.
func mathRound(val float64, places int) float64 {
	pow := 1.0
	for i := 0; i < places; i++ {
		pow *= 10
	}
	return float64(int(val*pow+0.5)) / pow
}
