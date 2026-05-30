package imgp

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

// ExtractPDFText extracts plain text from a PDF byte slice.
func ExtractPDFText(pdfBytes []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return "", fmt.Errorf("open PDF: %w", err)
	}

	var pages []string
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extract text from page %d: %w", i, err)
		}
		pages = append(pages, text)
	}

	return strings.Join(pages, "\n"), nil
}

// ParseFundFacts extracts fund facts from the PDF text.
// Required: Fund Size, Inception Date, ISIN, Management Fees.
// Returns nil, nil for FundInfo (fund identity comes from the HTML page, not PDF).
func ParseFundFacts(pdfText string) (*extractor.FundProfile, error) {
	profile := &extractor.FundProfile{}

	// Fund Facts section is a dense paragraph embedded in the text (not a standalone line).
	// Pattern: "...Fund Facts Fund Size 439.2 Mn USD Inception Date of theShare Class 07/03/2025 ..."
	// Extract the paragraph containing "Fund Facts"
	factsSection := extractParagraphWithMarker(pdfText, "Fund Facts")
	if factsSection == "" {
		return nil, fmt.Errorf("fund facts section not found")
	}

	// Fund Size: "Fund Size 439.2 Mn USD"
	aum, err := extractFundSize(factsSection)
	if err != nil {
		return nil, fmt.Errorf("parse fund size: %w", err)
	}
	profile.TotalNetAssets = aum

	// Inception Date: "Inception Date of theShare Class 07/03/2025"
	inception, err := extractInceptionDate(factsSection)
	if err != nil {
		return nil, fmt.Errorf("parse inception date: %w", err)
	}
	profile.InceptionDate = inception

	// ISIN: "ISIN LU2951555585"
	isin, err := extractISIN(factsSection)
	if err != nil {
		return nil, fmt.Errorf("parse ISIN: %w", err)
	}
	profile.Isin = isin

	// Share Class: "Share Class R USD UCITS ETF"
	shareClass, err := extractShareClass(factsSection)
	if err != nil {
		return nil, fmt.Errorf("parse share class: %w", err)
	}
	profile.ShareClassName = shareClass

	// Management Fees: "Management Fees 0.55%"
	mgmtFees, err := extractPercent(factsSection, "Management Fees")
	if err != nil {
		return nil, fmt.Errorf("parse management fees: %w", err)
	}
	profile.AnnualExpenseRatio = mgmtFees

	// Ongoing Charges: "Ongoing Charges 0.75%"
	ongoingCharges, err := extractPercent(factsSection, "Ongoing Charges")
	if err != nil {
		return nil, fmt.Errorf("parse ongoing charges: %w", err)
	}
	profile.OngoingCharges = ongoingCharges

	return profile, nil
}

// ParseRiskMeasures extracts risk metrics from the PDF text.
// Returns nil, nil if the section is not present or has no parseable data.
// FieldsPresent bitmask tracks which fields were actually parsed — a zero value
// on a field means it was absent from the source, not genuinely zero.
func ParseRiskMeasures(pdfText string) (*extractor.RiskMeasures, error) {
	risk := &extractor.RiskMeasures{}

	// Risk measures are in the "Measure of Risk" / "Annualized risk measures" section
	// Pattern: "Fund Volatility \n(1Y)\n 9.16% Sharpe Ratio \n(1Y)\n 2.52"
	// Values may be absent (e.g. for new funds)

	// Volatility: "Fund Volatility \n(1Y)\n 9.16%"
	volatility, err := extractRiskMeasure(pdfText, "Fund Volatility")
	if err == nil {
		risk.Volatility = volatility
		risk.FieldsPresent |= extractor.RiskFieldVolatility
	}

	// Sharpe Ratio: "Sharpe Ratio \n(1Y)\n 2.52"
	sharpe, err := extractRiskMeasure(pdfText, "Sharpe Ratio")
	if err == nil {
		risk.SharpeRatio = sharpe
		risk.FieldsPresent |= extractor.RiskFieldSharpeRatio
	}

	// Information Ratio: "Information Ratio \n(1Y)\n"
	infoRatio, err := extractRiskMeasure(pdfText, "Information Ratio")
	if err == nil {
		risk.InfoRatio = infoRatio
		risk.FieldsPresent |= extractor.RiskFieldInfoRatio
	}

	// Beta: "Beta \n(1Y)\n"
	beta, err := extractRiskMeasure(pdfText, "Beta")
	if err == nil {
		risk.Beta = beta
		risk.FieldsPresent |= extractor.RiskFieldBeta
	}

	// Correlation: "Correlation \n(1Y)\n"
	correlation, err := extractRiskMeasure(pdfText, "Correlation")
	if err == nil {
		risk.Correlation = correlation
		risk.FieldsPresent |= extractor.RiskFieldCorrelation
	}

	// Tracking Error: "Tracking Error \n(1Y)\n"
	trackingError, err := extractRiskMeasure(pdfText, "Tracking Error")
	if err == nil {
		risk.TrackingError = trackingError
		risk.FieldsPresent |= extractor.RiskFieldTrackingError
	}

	// Check if at least one value was parsed
	if risk.FieldsPresent == 0 {
		return nil, nil // optional section — no data parsed
	}

	return risk, nil
}

// ParseAssetClassAllocation extracts the "Derivatives Allocation" section
// (asset class breakdown: Equities, Bonds, Gold, Oil).
// Returns nil, nil if the section is not present.
func ParseAssetClassAllocation(pdfText string) ([]extractor.AssetClassEntry, error) {
	// "Derivatives Allocation" section contains asset class labels followed by percentages
	// Pattern: "Derivatives Allocation \nBonds\nGold\nOil\nEquities\n-30\n-20\n-10\n0\n10\n20\n30\n-25.7%\n4.3%\n15.4%\n20.3%"
	// Labels come first, then axis numbers, then percentage values

	section := extractSection(pdfText, "Derivatives Allocation")
	if section == "" {
		return nil, nil // optional section
	}

	labels, percentages := extractLabelsAndPercentages(section)
	if len(labels) == 0 || len(percentages) == 0 {
		return nil, nil // optional section — no data
	}

	pairs := pairAlignAndSort(labels, percentages)
	entries := make([]extractor.AssetClassEntry, len(pairs))
	for i, p := range pairs {
		entries[i] = extractor.AssetClassEntry{AssetClass: p.label, Percent: p.pct}
	}

	if len(labels) != len(percentages) {
		return entries, fmt.Errorf("asset class allocation: label/percentage count mismatch (%d labels, %d percentages) — paired %d entries",
			len(labels), len(percentages), len(entries))
	}

	return entries, nil
}

// ParseEquityDerivativesByRegion extracts regional exposure from equity derivatives.
// Returns nil, nil if the section is not present.
func ParseEquityDerivativesByRegion(pdfText string) ([]extractor.RegionDerivativeEntry, error) {
	// "Equity Derivatives by Region" section
	// Labels: Cash & Others, Asia ex Japan, Japan, Europe ex-EMU, EMU, North America, Emerging Countries
	// Note: labels may have newlines within them (e.g. "Cash\n&\nOthers")

	section := extractSection(pdfText, "Equity Derivatives by Region")
	if section == "" {
		return nil, nil // optional section
	}

	// For region labels, we need to handle multi-line labels like "Cash\n&\nOthers"
	// Strategy: extract known region patterns and percentages separately
	labels, percentages := extractRegionLabelsAndPercentages(section)
	if len(labels) == 0 || len(percentages) == 0 {
		return nil, nil // optional section — no data
	}

	pairs := pairAlignAndSort(labels, percentages)
	entries := make([]extractor.RegionDerivativeEntry, len(pairs))
	for i, p := range pairs {
		entries[i] = extractor.RegionDerivativeEntry{Region: p.label, Percent: p.pct}
	}

	if len(labels) != len(percentages) {
		return entries, fmt.Errorf("equity derivatives by region: label/percentage count mismatch (%d labels, %d percentages) — paired %d entries",
			len(labels), len(percentages), len(entries))
	}

	return entries, nil
}

// ParseCurrencyDerivativesAllocation extracts currency exposure from derivatives.
// Returns nil, nil if the section is not present.
func ParseCurrencyDerivativesAllocation(pdfText string) ([]extractor.CurrencyDerivativeEntry, error) {
	// "Currency Derivatives Allocation" section
	// Labels: JPY, SEK, AUD, CHF, GBP, Other DM FX, USD, EM FX, EUR
	// Note: "Other DM FX" may be split across two lines by PDF text extraction

	section := extractSection(pdfText, "Currency Derivatives Allocation")
	if section == "" {
		return nil, nil // optional section
	}

	labels, percentages := extractLabelsAndPercentages(section)
	if len(labels) == 0 || len(percentages) == 0 {
		return nil, nil // optional section — no data
	}

	// Post-process: merge "Other" + "DM FX" into "Other DM FX" if they appear
	// consecutively (PDF text extraction splits multi-word labels across lines).
	labels, percentages = mergeSplitLabels(labels, percentages)

	pairs := pairAlignAndSort(labels, percentages)
	entries := make([]extractor.CurrencyDerivativeEntry, len(pairs))
	for i, p := range pairs {
		entries[i] = extractor.CurrencyDerivativeEntry{Currency: p.label, Percent: p.pct}
	}

	if len(labels) != len(percentages) {
		return entries, fmt.Errorf("currency derivatives allocation: label/percentage count mismatch (%d labels, %d percentages) — paired %d entries",
			len(labels), len(percentages), len(entries))
	}

	return entries, nil
}

// mergeSplitLabels merges consecutive labels that form a known compound label
// (e.g. "Other" + "DM FX" → "Other DM FX"). This reduces label count by 1
// while keeping percentage count unchanged, fixing PDF extraction artefacts
// where multi-word chart labels are split across lines but share one percentage.
func mergeSplitLabels(labels []string, percentages []float64) ([]string, []float64) {
	// Known split patterns: [label1, label2] → merged
	splits := map[string]map[string]string{
		"Other": {"DM FX": "Other DM FX"},
	}

	mergedLabels := make([]string, 0, len(labels))

	for i := 0; i < len(labels); i++ {
		if i+1 < len(labels) {
			if mergeMap, ok := splits[labels[i]]; ok {
				if merged, ok := mergeMap[labels[i+1]]; ok {
					mergedLabels = append(mergedLabels, merged)
					i++ // skip next label
					continue
				}
			}
		}
		mergedLabels = append(mergedLabels, labels[i])
	}

	return mergedLabels, percentages
}

// ParseReferenceDate extracts the "as of" date from the factsheet header.
// Pattern: "Fact Sheet – April 30, 2026" or "Fact Sheet - April 30, 2026"
func ParseReferenceDate(pdfText string) (time.Time, error) {
	// "Fact Sheet – April 30, 2026" at the top of page 1
	re := regexp.MustCompile(`Fact Sheet\s*[–\-]\s*(.+?)\s*(?:MARKETING|$)`)
	match := re.FindStringSubmatch(pdfText)
	if match == nil || len(match) < 2 {
		return time.Time{}, fmt.Errorf("reference date not found in factsheet header")
	}

	dateStr := strings.TrimSpace(match[1])
	return parseDate(dateStr)
}

// --- Helper functions ---

// extractSection extracts text between a section header and the next major section.
// Uses word-boundary matching to avoid substring collisions (e.g. "Derivatives Allocation"
// vs "Currency Derivatives Allocation"). Returns the section text or empty string if not found.
func extractSection(text, header string) string {
	// Find the header as a standalone line (not a substring of a longer header)
	// Match: newline + header + space/newline, or start-of-string + header + space/newline
	idx := findSectionStart(text, header)
	if idx < 0 {
		return ""
	}

	// Start after the header
	start := idx + len(header)
	remaining := text[start:]

	// Find the next section header
	end := findNextSectionEnd(remaining, header)

	return strings.TrimSpace(remaining[:end])
}

// extractParagraphWithMarker finds the paragraph (line) containing the given marker
// and returns everything from the marker to the end of that paragraph. Used for
// dense text sections like "Fund Facts" that are embedded within other text.
func extractParagraphWithMarker(text, marker string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, marker) {
			idx := strings.Index(line, marker)
			return line[idx:]
		}
	}
	return ""
}

// findSectionStart finds the start of a section header as a standalone line,
// avoiding substring matches (e.g. "Derivatives Allocation" should not match
// inside "Currency Derivatives Allocation").
func findSectionStart(text, header string) int {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == header || strings.TrimSpace(line) == header+" " {
			// Calculate the byte offset of this line
			offset := 0
			for j := 0; j < i; j++ {
				offset += len(lines[j]) + 1 // +1 for newline
			}
			return offset
		}
	}
	return -1
}

// findNextSectionEnd finds the end of the current section by looking for the next
// standalone section header.
func findNextSectionEnd(remaining, currentHeader string) int {
	knownHeaders := []string{
		"Portfolio Breakdown", "Performance as of", "Important information",
		"Glossary", "Measure of Risk", "Performance by Month",
		"Contact", "About the Fund", "Fund Facts",
		"Equity Derivatives by Region", "Currency Derivatives Allocation",
		"Derivatives Allocation", "Fixed Income Derivatives Duration",
	}

	lines := strings.Split(remaining, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, h := range knownHeaders {
			if trimmed == h || trimmed == h+" " {
				// Found next section header
				offset := 0
				for j := 0; j < i; j++ {
					offset += len(lines[j]) + 1
				}
				return offset
			}
		}
	}
	return len(remaining)
}

// extractFundSize parses "439.2 Mn USD" into a float64 in millions.
func extractFundSize(section string) (float64, error) {
	re := regexp.MustCompile(`Fund Size\s+([\d,.]+)\s*(Mn|Bn|Million|Billion)\s*\w+`)
	match := re.FindStringSubmatch(section)
	if match == nil || len(match) < 3 {
		return 0, fmt.Errorf("fund size not found")
	}

	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse fund size value: %w", err)
	}

	multiplier := 1.0
	switch strings.ToLower(match[2]) {
	case "mn", "million":
		multiplier = 1_000_000
	case "bn", "billion":
		multiplier = 1_000_000_000
	}

	return value * multiplier, nil
}

// extractInceptionDate parses "Inception Date of theShare Class 07/03/2025".
func extractInceptionDate(section string) (time.Time, error) {
	re := regexp.MustCompile(`Inception Date[^0-9]*([\d]{1,2}[/\-][\d]{1,2}[/\-][\d]{2,4})`)
	match := re.FindStringSubmatch(section)
	if match == nil || len(match) < 2 {
		return time.Time{}, fmt.Errorf("inception date not found")
	}

	return parseDate(match[1])
}

// extractISIN parses "ISIN LU2951555585".
func extractISIN(section string) (string, error) {
	re := regexp.MustCompile(`ISIN\s+([A-Z]{2}\d{10})`)
	match := re.FindStringSubmatch(section)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("ISIN not found")
	}

	return match[1], nil
}

// extractShareClass parses "Share Class R USD UCITS ETF".
func extractShareClass(section string) (string, error) {
	re := regexp.MustCompile(`Share Class\s+([A-Z][^\n]+?)\s+(?:Classification|Cut-off|SRRI|$)`)
	match := re.FindStringSubmatch(section)
	if match == nil || len(match) < 2 {
		return "", fmt.Errorf("share class not found")
	}
	return strings.TrimSpace(match[1]), nil
}

// extractPercent extracts a percentage value after a given label.
func extractPercent(section, label string) (float64, error) {
	re := regexp.MustCompile(regexp.QuoteMeta(label) + `\s+([\d.]+)%`)
	match := re.FindStringSubmatch(section)
	if match == nil || len(match) < 2 {
		return 0, fmt.Errorf("%s not found", label)
	}

	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", label, err)
	}

	return value, nil
}

// extractRiskMeasure extracts a numeric value after a risk measure label.
// Risk measures appear as: "Fund Volatility \n(1Y)\n 9.16%" or "Sharpe Ratio \n(1Y)\n 2.52"
// Returns 0 if the value is absent (empty after the label).
func extractRiskMeasure(text, label string) (float64, error) {
	// Pattern: label followed by optional whitespace/newlines/(1Y)/more whitespace, then optional number
	escaped := regexp.QuoteMeta(label)
	re := regexp.MustCompile(escaped + `\s+(?:\([^)]*\)\s+)?([\d.-]+)%?`)
	match := re.FindStringSubmatch(text)
	if match == nil || len(match) < 2 {
		return 0, fmt.Errorf("%s value not found", label)
	}

	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", label, err)
	}

	return value, nil
}

// extractLabelsAndPercentages extracts category labels and percentage values from a chart section.
// Labels come first (single-word), then axis numbers, then percentages.
// Used for currency derivatives and asset class allocation where labels are single words.
func extractLabelsAndPercentages(section string) ([]string, []float64) {
	lines := strings.Split(section, "\n")
	var labels []string
	var percentages []float64
	phase := phaseLabels // phaseLabels → phaseAxis → phasePercents

	// Percentage pattern (including negative)
	pctRe := regexp.MustCompile(`^([\d.-]+)%$`)
	// Pure integer (axis label)
	intRe := regexp.MustCompile(`^-?\d+$`)
	// Whitespace/empty
	blankRe := regexp.MustCompile(`^\s*$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if blankRe.MatchString(line) {
			continue
		}

		// Check if it's a percentage value
		if pctRe.MatchString(line) {
			phase = phasePercents
			valStr := strings.TrimSuffix(line, "%")
			val, err := strconv.ParseFloat(valStr, 64)
			if err == nil {
				percentages = append(percentages, val)
			}
			continue
		}

		// Check if it's a pure integer (axis number) — skip
		if intRe.MatchString(line) {
			phase = phaseAxis
			continue
		}

		// Single-word labels (single-word token, not starting with digit)
		if phase == phaseLabels && len(line) > 0 && len(line) < 30 && !isNumber(line) {
			labels = append(labels, line)
		}
	}

	return labels, percentages
}

// phase tracking constants
type parsePhase int

const (
	phaseLabels  parsePhase = iota // collecting labels
	phaseAxis                      // axis numbers (skip)
	phasePercents                  // percentage values
)

// isNumber checks if a string looks like a number.
func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// pairAlignAndSort creates entries from labels and float64 percentages,
// pairing sequentially from the start (dropping extra labels at the end when
// counts mismatch), and sorting by absolute percentage descending (biggest first).
// When the PDF extraction drops a percentage (e.g. "0%" misread as axis value),
// sequential pairing preserves correct label→value mapping for the matched entries.
func pairAlignAndSort(labels []string, percentages []float64) []struct{ label string; pct float64 } {
	minLen := len(labels)
	if len(percentages) < minLen {
		minLen = len(percentages)
	}

	type pair struct{ label string; pct float64 }
	pairs := make([]pair, minLen)

	for i := 0; i < minLen; i++ {
		pairs[i] = pair{labels[i], percentages[i]}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return math.Abs(pairs[i].pct) > math.Abs(pairs[j].pct)
	})

	result := make([]struct{ label string; pct float64 }, len(pairs))
	for i, p := range pairs {
		result[i] = struct{ label string; pct float64 }{label: p.label, pct: p.pct}
	}
	return result
}

// extractRegionLabelsAndPercentages handles multi-line region labels
// like "Cash\n&\nOthers" by combining consecutive tokens into region names.
// Uses known region patterns to build labels from the raw token stream.
func extractRegionLabelsAndPercentages(section string) ([]string, []float64) {
	lines := strings.Split(section, "\n")
	var rawTokens []string
	var percentages []float64

	pctRe := regexp.MustCompile(`^([\d.-]+)%$`)
	intRe := regexp.MustCompile(`^-?\d+$`)
	blankRe := regexp.MustCompile(`^\s*$`)
	phase := phaseLabels

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if blankRe.MatchString(line) {
			continue
		}

		if pctRe.MatchString(line) {
			phase = phasePercents
			valStr := strings.TrimSuffix(line, "%")
			val, err := strconv.ParseFloat(valStr, 64)
			if err == nil {
				percentages = append(percentages, val)
			}
			continue
		}

		if intRe.MatchString(line) {
			phase = phaseAxis
			continue
		}

		if phase == phaseLabels && len(line) > 0 && !isNumber(line) {
			rawTokens = append(rawTokens, line)
		}
	}

	// Combine raw tokens into region labels using known patterns
	// Expected: Cash, &, Others, Asia, ex, Japan, Japan, Europe, ex-EMU, EMU, North, America, Emerging, Countries
	// Result: Cash & Others, Asia ex Japan, Japan, Europe ex-EMU, EMU, North America, Emerging Countries
	var labels []string
	i := 0
	for i < len(rawTokens) {
		token := rawTokens[i]

		// "Cash & Others" pattern: word + & + word
		if i+2 < len(rawTokens) && rawTokens[i+1] == "&" {
			labels = append(labels, token+" & "+rawTokens[i+2])
			i += 3
			continue
		}

		// "Asia ex Japan" pattern: word + ex + word
		if i+2 < len(rawTokens) && rawTokens[i+1] == "ex" {
			labels = append(labels, token+" ex "+rawTokens[i+2])
			i += 3
			continue
		}

		// "Europe ex-EMU" pattern: word + ex-EMU (already combined)
		// Just take it as-is
		if i+1 < len(rawTokens) && rawTokens[i+1] == "ex-EMU" {
			labels = append(labels, token+" ex-EMU")
			i += 2
			continue
		}

		// Multi-word regions: "North America", "Emerging Countries"
		// Heuristic: if next token starts with uppercase and current token
		// is a known prefix, combine them
		if i+1 < len(rawTokens) {
			combined := token + " " + rawTokens[i+1]
			knownMultiWord := map[string]bool{
				"North America": true, "Emerging Countries": true,
			}
			if knownMultiWord[combined] {
				labels = append(labels, combined)
				i += 2
				continue
			}
		}

		// Single-word label
		labels = append(labels, token)
		i++
	}

	return labels, percentages
}

// parseDate tries multiple date formats common on iMGP factsheets.
func parseDate(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)

	formats := []string{
		"02/01/2006",   // "07/03/2025"
		"2/1/2006",     // "7/3/2025"
		"02/01/06",     // "07/03/25"
		"2/1/06",       // "7/3/25"
		"January 2, 2006", // "April 30, 2026"
		"Jan 2, 2006",      // "Apr 30, 2026"
		"2 January 2006",   // "30 April 2026"
		"02 January 2006",  // "30 April 2026"
		"2006-01-02",       // "2026-04-30"
		"02-01-2006",       // "30-04-2026"
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized date format: %q", dateStr)
}
