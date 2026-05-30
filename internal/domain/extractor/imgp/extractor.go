package imgp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
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

	// 6. Parse required sections — Fund Facts
	profile, err := ParseFundFacts(pdfText)
	if err != nil {
		return nil, fmt.Errorf("parse fund facts: %w", err)
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
		AsOfDate:                    refDate,
		FundInfo:                    fundInfo,
		FundProfile:                 profile,
		RiskMeasures:                risk,
		AssetClassAllocation:        assetClass,
		EquityDerivativesByRegion:   equityRegions,
		CurrencyDerivativesAllocation: currencyAlloc,
	}, nil
}

// SetClient sets the HTTP client for fetching pages and PDFs.
func (e *Extractor) SetClient(c *Client) {
	e.client = c
}

// parseFundInfoFromHTML extracts the fund symbol (ISIN from URL path) and fund name
// (from the page title tag) from the HTML fund page.
func parseFundInfoFromHTML(sourceURL, html string) *extractor.FundInfo {
	info := &extractor.FundInfo{}

	// Extract ISIN from URL path: /fund/LU2951555585
	info.Symbol = extractISINFromURL(sourceURL)

	// Extract fund name from <title> tag
	info.Name = extractFundNameFromTitle(html)

	return info
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

	// Path is typically /fund/{ISIN}
	filename := filepath.Base(parsed.Path)
	// ISIN is 2 letters + 10 digits (12 chars total)
	if len(filename) == 12 {
		return filename
	}
	return filename
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

	// Remove the " | iMGP" or " - iMGP" suffix
	for _, sep := range []string{" | iMGP", " - iMGP", " | IMGP", " - IMGP"} {
		if idx := strings.LastIndex(title, sep); idx >= 0 {
			title = strings.TrimSpace(title[:idx])
			break
		}
	}

	return title
}
