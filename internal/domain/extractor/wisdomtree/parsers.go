package wisdomtree

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/govalues/decimal"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// ParseFundInfo extracts symbol and name from `var fundInfo<HASH> = {...}`.
func ParseFundInfo(html string) (*extractor.FundInfo, error) {
	re := regexp.MustCompile(`var\s+fundInfo\w*\s*=\s*\{[^}]*\}`)
	match := re.FindString(html)
	if match == "" {
		return nil, fmt.Errorf("fund info not found in page")
	}

	symbol, err := extractJSONString(match, "symbol")
	if err != nil {
		return nil, fmt.Errorf("parse fund info symbol: %w", err)
	}
	name, err := extractJSONString(match, "name")
	if err != nil {
		return nil, fmt.Errorf("parse fund info name: %w", err)
	}

	return &extractor.FundInfo{
		Symbol: symbol,
		Name:   name,
	}, nil
}

// ParseFundProfile extracts AUM, TER, Inception Date, Family, and Legal Type from HTML table rows.
// AnnualHoldingsTurnover is not available on WisdomTree pages and is left at zero.
func ParseFundProfile(html string) (*extractor.FundProfile, error) {
	profile := &extractor.FundProfile{}

	// AUM: <td>Total AUM of fund</td> ... <td>...€</td>
	aum, err := parseTableValue(html, `<td>Total AUM of fund</td>`)
	if err == nil {
		profile.TotalNetAssets = aum
	}

	// TER: <td class="key">TER</td> ... <td>...</td>
	// Value is a percentage string (e.g. "0.40%"); parseTableValue strips "%" and returns the raw number.
	// AnnualExpenseRatio convention is a fraction (0.004 for 0.4%), so divide by 100.
	ter, err := parseTableValue(html, `<td class="key">TER</td>`)
	if err == nil {
		profile.AnnualExpenseRatio = ter / 100
	}

	// Inception Date: <td class="key">Inception Date</td> ... <td>...</td>
	inceptionRaw, err := parseTableRawValue(html, `<td class="key">Inception Date</td>`)
	if err == nil {
		if t, err := parseDate(inceptionRaw); err == nil {
			profile.InceptionDate = t
		}
	}

	// Fund Umbrella (Family): <td class="key">Fund Umbrella</td> ... <td>...</td>
	familyRaw, err := parseTableRawValue(html, `<td class="key">Fund Umbrella</td>`)
	if err == nil {
		profile.Family = strings.TrimSpace(familyRaw)
	}

	// Legal Form (Legal Type): <td class="key">Legal Form</td> ... <td>...</td>
	legalRaw, err := parseTableRawValue(html, `<td class="key">Legal Form</td>`)
	if err == nil {
		profile.LegalType = strings.TrimSpace(legalRaw)
	}

	// ISIN: from Listings & Codes table — <td>ISIN</td> ... <td>IE000YGEAK03</td>
	isinRaw, err := parseTableRawValue(html, `<td>ISIN</td>`)
	if err == nil {
		profile.Isin = strings.TrimSpace(isinRaw)
	}

	if profile.TotalNetAssets == 0 && profile.AnnualExpenseRatio == 0 && profile.InceptionDate.IsZero() {
		return nil, nil // optional section — not present on all pages
	}

	return profile, nil
}

// ParseHoldings extracts holdings from `var fundHoldingsData = '...'`.
// CSV format: date,Weight,Security Description
// Filters out cash/currency positions.
// Deprecated: use ParseHoldingsWithModal for holdings with ticker/symbol data.
func ParseHoldings(html string) ([]extractor.Holding, error) {
	re := regexp.MustCompile(`var\s+fundHoldingsData\s*=\s*'((?:[^'\\]|\\.)*)'`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return nil, fmt.Errorf("fund holdings data not found")
	}

	csvData := unescapeJSString(match[1])
	reader := csv.NewReader(strings.NewReader(csvData))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse holdings CSV: %w", err)
	}

	var holdings []extractor.Holding
	for i, record := range records {
		if i == 0 {
			continue // skip header
		}
		if len(record) < 3 {
			continue
		}

		weight, err := strconv.ParseFloat(strings.TrimSpace(record[1]), 64)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(record[2])
		// Filter out cash/currency positions
		if isCashPosition(name) {
			continue
		}

		// Weight is a fraction (0.0137 = 1.37%), convert to percentage
		holdings = append(holdings, extractor.Holding{
			Name:    name,
			Percent: weight * 100,
		})
	}

	return holdings, nil
}

// modalHolding is the JSON structure from the all-holdings modal.
type modalHolding struct {
	CountryCode    string  `json:"CountryCode"`
	Weight         float64 `json:"Weight"`
	COBDate        string  `json:"COBDate"`
	IdentifierName string  `json:"IdentifierName"`
	IdentifierTicker string `json:"IdentifierTicker"`
	SharesPar      string  `json:"SharesPar"`
	MarketValue    float64 `json:"MarketValue"`
}

// ExtractModalURL extracts the all-holdings modal URL from the main page HTML.
// Pattern: data-href="https://www.wisdomtree.eu/en-gb/global/etf-details/modals/all-holdings?id={GUID}"
func ExtractModalURL(html string) string {
	re := regexp.MustCompile(`data-href="([^"]*all-holdings[^"]*)"`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return ""
	}
	return match[1]
}

// ParseHoldingsFromModal extracts holdings from the all-holdings modal page.
// The modal contains an embedded JSON array with ticker data.
// Ticker format is "NVDA UQ" (Bloomberg-style with market suffix); the suffix is stripped.
// Some entries have CUSIP instead of ticker (e.g. "US5128073062").
func ParseHoldingsFromModal(modalHTML string) ([]extractor.Holding, error) {
	// Extract the JSON array from the JavaScript source variable
	// Pattern: var source = [{...},{...},...];
	re := regexp.MustCompile(`var\s+source\s*=\s*(\[\s*\{[^\]]*\}\s*\])`)
	match := re.FindStringSubmatch(modalHTML)
	if match == nil || len(match) < 2 {
		return nil, fmt.Errorf("holdings JSON not found in modal")
	}

	var modalHoldings []modalHolding
	if err := json.Unmarshal([]byte(match[1]), &modalHoldings); err != nil {
		return nil, fmt.Errorf("parse holdings JSON: %w", err)
	}

	var holdings []extractor.Holding
	for _, h := range modalHoldings {
		if isCashPosition(h.IdentifierName) {
			continue
		}

		holding := extractor.Holding{
			Name:    h.IdentifierName,
			Percent: h.Weight * 100, // fraction to percentage
		}

		// Extract ticker, stripping Bloomberg market suffix (e.g. "NVDA UQ" -> "NVDA")
		if h.IdentifierTicker != "" {
			holding.Symbol = extractTicker(h.IdentifierTicker)
		}

		holdings = append(holdings, holding)
	}

	return holdings, nil
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

// ParseNavHistory extracts NAV history from `var fundMarketData<HASH> = '...'`.
// CSV format: date,fund_ticker,close_price_adj,volume_adj,nav,bmk_ticker_A,...
func ParseNavHistory(html string) ([]extractor.NavPoint, error) {
	re := regexp.MustCompile(`var\s+fundMarketData\w+\s*=\s*'((?:[^'\\]|\\.)*)'`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return nil, nil // optional section — not present on all pages
	}

	csvData := unescapeJSString(match[1])
	reader := csv.NewReader(strings.NewReader(csvData))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse nav CSV: %w", err)
	}

	var points []extractor.NavPoint
	for i, record := range records {
		if i == 0 {
			continue // skip header
		}
		if len(record) < 5 {
			continue
		}

		dateStr := strings.TrimSpace(record[0])
		navStr := strings.TrimSpace(record[4])
		if navStr == "" {
			continue
		}

		nav, err := strconv.ParseFloat(navStr, 64)
		if err != nil || nav == 0 {
			continue
		}

		// WisdomTree's fundMarketData CSV uses US M/D/YYYY format (month first),
		// despite the site being wisdomtree.eu/en-gb. The 12/13/2023 entry (Dec 13)
		// is unambiguous — day 13 can't be a month.
		date, err := time.Parse("2/1/2006", dateStr)
		if err != nil {
			return nil, fmt.Errorf("row %d: parse date %q: %w", i, dateStr, err)
		}

		points = append(points, extractor.NavPoint{
			Date: date,
			NAV:  decimal.MustParse(fmt.Sprintf("%.4f", nav)),
		})
	}

	return points, nil
}

// ParseThemes extracts themes from `var fundThemeData = '...'`.
// CSV format: date,Weight,Security Description
// Returns nil, nil if the section is not present on the page.
func ParseThemes(html string) ([]extractor.Theme, error) {
	re := regexp.MustCompile(`var\s+fundThemeData\s*=\s*'((?:[^'\\]|\\.)*)'`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return nil, nil // optional section — not present on all pages
	}

	csvData := unescapeJSString(match[1])
	reader := csv.NewReader(strings.NewReader(csvData))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse themes CSV: %w", err)
	}

	var themes []extractor.Theme
	for i, record := range records {
		if i == 0 {
			continue // skip header
		}
		if len(record) < 3 {
			continue
		}

		weight, err := strconv.ParseFloat(strings.TrimSpace(record[1]), 64)
		if err != nil {
			continue
		}

		name := strings.TrimSpace(record[2])
		// Weight is a fraction, convert to percentage
		themes = append(themes, extractor.Theme{
			Name:    name,
			Percent: weight * 100,
		})
	}

	return themes, nil
}

// ParseSectors extracts sectors from `var fundSectorsData = '...'`.
// CSV format: date,securityName,weight,Sector,wgtSector
// Aggregates to sector totals using the wgtSector column.
// Returns nil, nil if the section is not present on the page.
func ParseSectors(html string) ([]extractor.SectorWeighting, error) {
	re := regexp.MustCompile(`var\s+fundSectorsData\s*=\s*'((?:[^'\\]|\\.)*)'`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return nil, nil // optional section — not present on all pages
	}

	csvData := unescapeJSString(match[1])
	reader := csv.NewReader(strings.NewReader(csvData))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse sectors CSV: %w", err)
	}

	// Aggregate by sector.
	// wgtSector (column 4) is the sector total weight repeated for each security
	// in that sector. Take unique (Sector, wgtSector) pairs — do NOT sum.
	// wgtSector is a fraction (e.g. 0.3685 = 36.85%), convert to percentage.
	sectorMap := make(map[string]float64)
	for i, record := range records {
		if i == 0 {
			continue // skip header
		}
		if len(record) < 5 {
			continue
		}

		sector := strings.TrimSpace(record[3])
		weightStr := strings.TrimSpace(record[4])
		if sector == "" || weightStr == "" {
			continue
		}

		weight, err := strconv.ParseFloat(weightStr, 64)
		if err != nil {
			continue
		}

		// Only set if not already present (take first occurrence; all rows
		// in the same sector have the same wgtSector value)
		if _, exists := sectorMap[sector]; !exists {
			sectorMap[sector] = weight * 100 // fraction to percentage
		}
	}

	var sectors []extractor.SectorWeighting
	for sector, weight := range sectorMap {
		if weight > 0 {
			sectors = append(sectors, extractor.SectorWeighting{
				Sector:  sector,
				Percent: weight,
			})
		}
	}

	return sectors, nil
}

// ParseCountryAllocation extracts country allocation from the HTML table.
// Pattern: <td class="key">1. United States</td> ... <td class="value"><span>40.54%</span></td>
// Returns nil, nil if the section is not present on the page.
func ParseCountryAllocation(html string) ([]extractor.CountryAllocation, error) {
	// Extract the country allocation section (between id= and </section>)
	// Use [\s\S] instead of . to match across newlines (RE2 engine)
	sectionRe := regexp.MustCompile(`id="country-allocation-section"[^>]*>([\s\S]+?)</section`) //nolint:revive
	sectionMatch := sectionRe.FindStringSubmatch(html)
	if sectionMatch == nil || len(sectionMatch) < 2 {
		return nil, nil // optional section — not present on all pages
	}

	section := sectionMatch[1]

	// Match rows: <td class="key">N. Country</td> ... <td class="value">...weight...</td>
	rowRe := regexp.MustCompile(`<td class="key">([^<]+)</td>\s*<td class="value">[^<]*(?:<span[^>]*>)?([^<%]+)%`) //nolint:revive
	matches := rowRe.FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no country allocation rows found")
	}

	var countries []extractor.CountryAllocation
	for _, match := range matches {
		name := strings.TrimSpace(match[1])
		// Strip leading "N. " numbering
		if idx := strings.Index(name, "."); idx >= 0 {
			name = strings.TrimSpace(name[idx+1:])
		}
		weight, err := strconv.ParseFloat(strings.TrimSpace(match[2]), 64)
		if err != nil {
			continue
		}
		countries = append(countries, extractor.CountryAllocation{
			Country: name,
			Percent: weight,
		})
	}

	return countries, nil
}

// ParseMarketCap extracts market cap breakdown from the HTML table.
// Pattern: <td class="key">Total Market Capitalization ($ Trillion)</td> ... <td class="value">58.68</td>
// and: <td class="key shifted">Large Cap (&gt; $10 Billion)</td> ... <td class="value">64.42%</td>
func ParseMarketCap(html string) (*extractor.MarketCapBreakdown, error) {
	breakdown := &extractor.MarketCapBreakdown{}

	// Total Market Capitalization
	totalRe := regexp.MustCompile(`<td class="key">Total Market Capitalization[^<]*</td>\s*<td class="value">([^<]+)</td>`) //nolint:revive
	match := totalRe.FindStringSubmatch(html)
	if match != nil && len(match) > 1 {
		totalStr := strings.TrimSpace(match[1])
		totalStr = strings.ReplaceAll(totalStr, ",", "")
		total, err := strconv.ParseFloat(totalStr, 64)
		if err == nil {
			breakdown.Total = total
		}
	}

	// Large Cap
	largeRe := regexp.MustCompile(`<td class="key[^>]*">Large Cap[^<]*</td>\s*<td class="value">([^<%]+)%`) //nolint:revive
	match = largeRe.FindStringSubmatch(html)
	if match != nil && len(match) > 1 {
		weight, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		if err == nil {
			breakdown.Large = weight
		}
	}

	// Mid Cap
	midRe := regexp.MustCompile(`<td class="key[^>]*">Mid Cap[^<]*</td>\s*<td class="value">([^<%]+)%`) //nolint:revive
	match = midRe.FindStringSubmatch(html)
	if match != nil && len(match) > 1 {
		weight, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		if err == nil {
			breakdown.Mid = weight
		}
	}

	// Small Cap
	smallRe := regexp.MustCompile(`<td class="key[^>]*">Small Cap[^<]*</td>\s*<td class="value">([^<%]+)%`) //nolint:revive
	match = smallRe.FindStringSubmatch(html)
	if match != nil && len(match) > 1 {
		weight, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		if err == nil {
			breakdown.Small = weight
		}
	}

	if breakdown.Total == 0 && breakdown.Large == 0 && breakdown.Mid == 0 && breakdown.Small == 0 {
		return nil, nil // optional section — not present on all pages
	}

	return breakdown, nil
}

// ParseFundCharacteristics extracts fund characteristics from the HTML table.
// Pattern: <td class="key">*Dividend Yield</td> ... <td class="value">0.94</td>
func ParseFundCharacteristics(html string) (*extractor.FundCharacteristics, error) {
	chars := &extractor.FundCharacteristics{}

	// Extract the fund characteristics section (sibling table to market cap)
	// Look for the Fund Characteristics header
	sectionRe := regexp.MustCompile(`<th class="key">Fund Characteristics</th>[\s\S]*?</tbody>`) //nolint:revive
	sectionMatch := sectionRe.FindStringSubmatch(html)
	if sectionMatch == nil {
		return nil, nil // optional section — not present on all pages
	}

	section := sectionMatch[0]

	// Parse each characteristic
	pairs := map[string]*float64{
		"*Dividend Yield":          &chars.DividendYield,
		"Price/Earnings":           &chars.PriceToEarnings,
		"Estimated Price/Earnings": &chars.EstimatedPriceToEarnings,
		"Price/Book":               &chars.PriceToBook,
		"Price/Sales":              &chars.PriceToSales,
		"Price/Cash Flow":          &chars.PriceToCashflow,
	}

	for key, target := range pairs {
		escapedKey := regexp.QuoteMeta(key)
		re := regexp.MustCompile(`<td class="key">` + escapedKey + `</td>\s*<td class="value">([^<]+)</td>`) //nolint:revive
		match := re.FindStringSubmatch(section)
		if match != nil && len(match) > 1 {
			val, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
			if err == nil {
				*target = val
			}
		}
	}

	// Check if at least one value was parsed
	if chars.DividendYield == 0 && chars.PriceToEarnings == 0 && chars.PriceToBook == 0 {
		return nil, nil // optional section — data not parseable
	}

	return chars, nil
}

// ParseAsOfDate extracts the "as of" date from the Net Asset Value table header.
// Pattern: <th>Net Asset Value</th><th>22 May 2026</th>
func ParseAsOfDate(html string) (time.Time, error) {
	re := regexp.MustCompile(`<th>\s*Net Asset Value\s*</th>\s*<th[^>]*>([^<]+)</th>`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return time.Time{}, fmt.Errorf("as-of date not found")
	}

	dateStr := strings.TrimSpace(match[1])
	return parseDate(dateStr)
}

// parseDate tries multiple date formats common on WisdomTree pages.
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
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized date format: %q", dateStr)
}

// extractJSONString extracts a string value for a given key from a JS object literal.
func extractJSONString(obj, key string) (string, error) {
	re := regexp.MustCompile(`'` + regexp.QuoteMeta(key) + `'\s*:\s*'([^']*)'`)
	match := re.FindStringSubmatch(obj)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("key %q not found in object", key)
	}
	return match[1], nil
}

// parseTableValue extracts the value from a table row after the given label cell.
// Pattern: <td>label</td> ... <td>value</td>
func parseTableValue(html, labelCell string) (float64, error) {
	raw, err := parseTableRawValue(html, labelCell)
	if err != nil {
		return 0, err
	}
	return parseNumber(raw)
}

// parseTableRawValue extracts the raw string value from a table row.
// Handles both direct text (`<td>value</td>`) and span-wrapped (`<td><span>value</span></td>`).
// Also handles whitespace/newlines between tag and text (e.g. `<td class="key">\n  TER\n</td>`).
func parseTableRawValue(html, labelCell string) (string, error) {
	// labelCell is like `<td>Total AUM of fund</td>` or `<td class="key">TER</td>`
	// Extract the label text from between the > and </td>
	labelRe := regexp.MustCompile(`>[ \t\n\r]*([^<\n]+)[ \t\n\r]*</td>`)
	labelMatch := labelRe.FindStringSubmatch(labelCell)
	if labelMatch == nil {
		return "", fmt.Errorf("could not extract label from %s", labelCell)
	}
	labelText := labelMatch[1]

	// Build flexible regex: <td...> whitespace LABEL whitespace </td> ... <td...> value
	re := regexp.MustCompile(`<td[^>]*>[ \t\n\r]*` + regexp.QuoteMeta(labelText) + `[ \t\n\r]*</td>\s*<td[^>]*>(?:<span[^>]*>)?([^<]+)`)
	match := re.FindStringSubmatch(html)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("table value not found for %s", labelCell)
	}
	return strings.TrimSpace(match[1]), nil
}

// parseNumber extracts a numeric value from a string, handling %, €, and commas.
func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, "€", "")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")

	return strconv.ParseFloat(s, 64)
}

// unescapeJSString converts JS escape sequences to their Go equivalents.
func unescapeJSString(s string) string {
	s = strings.ReplaceAll(s, `\'`, `'`)
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\u0026`, "&")
	s = strings.ReplaceAll(s, `\u0025`, "%")
	s = strings.ReplaceAll(s, `\u002F`, "/")
	return s
}

// isCashPosition returns true if the holding name is a cash or currency position.
func isCashPosition(name string) bool {
	name = strings.ToUpper(name)
	cashKeywords := []string{
		"CASH", "CASH W-O", "EURO INCOME", "STERLING POUND", "US DOLLAR",
		"JAPANESE YEN", "AUSTRALIAN DOLLAR", "CANADIAN DOLLAR", "SWISS FRANC",
		"SWEDISH KRONA", "NORWEGIAN KRONE", "DANISH KRONE", "BRAZIL REAL",
		"KOREAN WON", "SINGAPORE DOLLAR", "HONG KONG DOLLAR", "INDONESIAN RUPIAH",
		"THAILAND BAHT", "MALAYSIAN RINGIT", "POLISH ZLOTY", "MEXICAN PESO",
		"PHILIPPINE PESO", "TURKISH LIRA", "SOUTH AFRICAN RAND",
		"CHINESE RENMINBI", "CHINESE RENIMBI", "CZECHOSLOVAKIAN",
		"CGT ADJ", "CGT", "CASH & CASH EQUIVALENTS",
	}

	for _, keyword := range cashKeywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}
