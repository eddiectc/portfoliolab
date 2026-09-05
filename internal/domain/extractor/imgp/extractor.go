package imgp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/eddiectc/portfoliolab/internal/domain/extractor"
)

// Name is the identifier for the iMGP extractor.
const Name = "imgp"

// Extractor extracts fund data from iMGP factsheet PDFs.
type Extractor struct {
	matcher *URLMatcher
	client  *Client
}

// NewExtractor creates a new iMGP extractor.
func NewExtractor() *Extractor {
	return &Extractor{
		matcher: NewURLMatcher(),
		client:  NewClient(),
	}
}

// Name returns the extractor identifier.
func (e *Extractor) Name() string {
	return Name
}

// Match checks if a URL belongs to an iMGP domain.
func (e *Extractor) Match(rawURL string) bool {
	return e.matcher.Match(rawURL)
}

// Extract fetches and parses data from an iMGP fund page.
// Flow: fetch HTML page → extract PDF URL → download PDF → extract text → parse all sections.
// Required sections: Fund Facts + Reference Date. Optional sections (risk, allocations)
// returning nil is acceptable.
func (e *Extractor) Extract(ctx context.Context, sourceURL string) (*extractor.ExtractResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 1. Fetch HTML page
	html, err := e.client.FetchPage(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}

	// 2. Extract FundInfo from HTML page
	fundInfo := parseFundInfoFromHTML(sourceURL, html)

	// 3. Extract PDF URL from HTML
	pdfURL, err := extractPDFURL(html, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("extract PDF URL: %w", err)
	}

	// 4. Download PDF
	pdfBytes, err := e.client.FetchPDF(pdfURL)
	if err != nil {
		return nil, fmt.Errorf("fetch PDF: %w", err)
	}

	// 5. Extract text from PDF
	pdfText, err := ExtractPDFText(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("extract PDF text: %w", err)
	}

	// 6. Parse required sections — Fund Facts.
	// US share class factsheets use month-first dates and omit ISIN, share
	// class and ongoing charges.
	profile, err := ParseFundFacts(pdfText, dateLayoutFor(sourceURL))
	if err != nil {
		return nil, fmt.Errorf("parse fund facts: %w", err)
	}

	// US share class factsheets list CUSIP instead of ISIN; back-fill the
	// ISIN from the fund page's structured data (already resolved into
	// fundInfo.Symbol, with the URL slug as fallback).
	if profile.Isin == "" {
		profile.Isin = fundInfo.Symbol
	}
	if profile.Isin == "" {
		return nil, fmt.Errorf("ISIN not found in factsheet or fund page for %s", sourceURL)
	}

	// 7. Parse required sections — Reference Date
	refDate, err := ParseReferenceDate(pdfText)
	if err != nil {
		return nil, fmt.Errorf("parse reference date: %w", err)
	}

	// 8. Parse optional sections — partial data + error is acceptable.
	// These sections may have data quality issues (e.g. label/percentage mismatch)
	// that the parser reports as errors while still returning paired data.
	// We accept the partial data and log the warning.
	risk, err := ParseRiskMeasures(pdfText)
	if err != nil {
		slog.Warn("imgp: parsing risk measures", "error", err)
	}

	assetClass, err := ParseAssetClassAllocation(pdfText)
	if err != nil {
		slog.Warn("imgp: parsing asset class allocation", "error", err)
	}

	equityRegions, err := ParseEquityDerivativesByRegion(pdfText)
	if err != nil {
		slog.Warn("imgp: parsing equity derivatives by region", "error", err)
	}

	currencyAlloc, err := ParseCurrencyDerivativesAllocation(pdfText)
	if err != nil {
		slog.Warn("imgp: parsing currency derivatives allocation", "error", err)
	}

	return &extractor.ExtractResult{
		AsOfDate:                      refDate,
		FundInfo:                      fundInfo,
		FundProfile:                   profile,
		RiskMeasures:                  risk,
		AssetClassAllocation:          assetClass,
		EquityDerivativesByRegion:     equityRegions,
		CurrencyDerivativesAllocation: currencyAlloc,
	}, nil
}

// SetClient sets the HTTP client for fetching pages and PDFs.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

// dateLayoutFor returns the factsheet date layout for the fund page region.
// US share class factsheets use month-first dates (05/07/2019 = 7 May 2019);
// EU factsheets use day-first (07/03/2025 = 7 March 2025).
func dateLayoutFor(sourceURL string) DateLayout {
	if u, err := url.Parse(sourceURL); err == nil && strings.HasPrefix(u.Path, "/us/") {
		return DateLayoutMonthFirst
	}
	return DateLayoutDayFirst
}

// parseFundInfoFromHTML extracts the fund symbol (ISIN) and fund name from the
// HTML fund page. The page embeds a `const fund = {...}` JSON object with the
// structured identity (ISIN, fund name) on both the EU and US page variants;
// the URL path and <title> are the fallbacks.
func parseFundInfoFromHTML(sourceURL, html string) *extractor.FundInfo {
	symbol, name := "", ""
	if f := parseFundPageJSON(html); f != nil {
		symbol, name = f.Isin, f.SubFundName
	}
	if symbol == "" {
		// /fund/LU2951555585 or /us/fund/US53700T8273-<slug>
		symbol = extractISINFromURL(sourceURL)
	}
	if name == "" {
		name = extractFundNameFromTitle(html)
	}

	return &extractor.FundInfo{Symbol: symbol, Name: name}
}

// extractISINFromURL extracts the ISIN from the fund page URL path.
// Expected format: https://www.imgp.com/fund/LU2951555585
func extractISINFromURL(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}

	// Must be a proper URL with scheme and host
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	// Path is typically /fund/{ISIN} (EU) or /us/fund/{ISIN}-{slug} (US).
	filename := filepath.Base(parsed.Path)
	if isISINLike(filename) {
		return filename
	}
	if idx := strings.IndexByte(filename, '-'); idx > 0 && isISINLike(filename[:idx]) {
		return filename[:idx]
	}
	return ""
}

// isISINLike reports whether s looks like an ISIN: a two-letter country
// prefix plus a 10-character body of digits and letters (e.g. LU2951555585,
// US53700T8273). Check digits are not validated.
func isISINLike(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i := 0; i < 2; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	for i := 2; i < 12; i++ {
		if !((s[i] >= '0' && s[i] <= '9') || (s[i] >= 'A' && s[i] <= 'Z')) {
			return false
		}
	}
	return true
}

// extractFundNameFromTitle extracts the fund name from the HTML <title> tag.
// iMGP page titles follow the pattern: "Fund Name | iMGP" or "Fund Name - iMGP"
func extractFundNameFromTitle(html string) string {
	// Find content between <title> and </title>
	start := strings.Index(html, "<title")
	if start < 0 {
		return ""
	}

	// Find the opening > after <title
	openTagEnd := strings.Index(html[start:], ">")
	if openTagEnd < 0 {
		return ""
	}

	titleStart := start + openTagEnd + 1
	titleEnd := strings.Index(html[titleStart:], "</title>")
	if titleEnd < 0 {
		return ""
	}

	title := strings.TrimSpace(html[titleStart : titleStart+titleEnd])

	// Remove the site suffix (" | iMGP", " | iM Global Partner", ...).
	for _, sep := range []string{" | iMGP", " - iMGP", " | IMGP", " - IMGP", " | iM Global Partner", " - iM Global Partner"} {
		if idx := strings.LastIndex(title, sep); idx >= 0 {
			title = strings.TrimSpace(title[:idx])
			break
		}
	}

	return title
}
