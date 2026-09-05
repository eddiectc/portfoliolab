package imgp

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eddiectc/portfoliolab/internal/domain/extractor"
)

// --- Extractor Name and Match ---

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	if e.Name() != Name {
		t.Errorf("expected name %q, got %q", Name, e.Name())
	}
}

func TestExtractor_Match(t *testing.T) {
	e := NewExtractor()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"imgp.com domain", "https://www.imgp.com/fund/LU2951555585", true},
		{"imgp.com without www", "https://imgp.com/fund/LU2951555585", true},
		{"subdomain.imgp.com", "https://sub.imgp.com/page", true},
		{"other domain", "https://www.wisdomtree.eu/en-gb/etfs/wmgt", false},
		{"vanguard", "https://www.vanguard.com/etfs/vo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Match(tt.url)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// --- Extract success ---

func TestExtractor_Extract_Success(t *testing.T) {
	mock := newMockClient()
	mock.setupPageFetch(sampleHTMLWithFactsheetLink(), nil)

	realPDF, err := loadSamplePDF(t)
	if err != nil {
		t.Skipf("sample PDF not available: %v", err)
	}
	mock.setupPDFFetch(realPDF, nil)

	e := NewExtractor()
	e.SetClient(mock.Client)

	result, err := e.Extract(context.Background(), "https://www.imgp.com/fund/LU2951555585")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Verify required fields
	if result.FundInfo == nil {
		t.Error("FundInfo is nil")
	} else {
		if result.FundInfo.Symbol != "LU2951555585" {
			t.Errorf("FundInfo.Symbol = %q, want %q", result.FundInfo.Symbol, "LU2951555585")
		}
		if result.FundInfo.Name == "" {
			t.Error("FundInfo.Name is empty")
		}
	}

	if result.FundProfile == nil {
		t.Error("FundProfile is nil")
	} else {
		if result.FundProfile.Isin != "LU2951555585" {
			t.Errorf("FundProfile.Isin = %q, want %q", result.FundProfile.Isin, "LU2951555585")
		}
		if result.FundProfile.TotalNetAssets == 0 {
			t.Error("FundProfile.TotalNetAssets is zero")
		}
	}

	if result.AsOfDate.IsZero() {
		t.Error("AsOfDate is zero")
	}

	// Optional fields — may be present or nil
	if result.RiskMeasures != nil {
		t.Logf("RiskMeasures present: Vol=%.2f, Sharpe=%.2f",
			result.RiskMeasures.Volatility, result.RiskMeasures.SharpeRatio)
	}
	t.Logf("AssetClassAllocation: %d entries", len(result.AssetClassAllocation))
	t.Logf("EquityDerivativesByRegion: %d entries", len(result.EquityDerivativesByRegion))
	t.Logf("CurrencyDerivativesAllocation: %d entries", len(result.CurrencyDerivativesAllocation))

	// Verify calls were made
	if mock.pageCalls != 1 {
		t.Errorf("expected 1 page fetch, got %d", mock.pageCalls)
	}
	if mock.pdfCalls != 1 {
		t.Errorf("expected 1 PDF fetch, got %d", mock.pdfCalls)
	}
}

// --- Extract success (US share class) ---
// US factsheets use month-first dates, CUSIP instead of ISIN, and
// "Gross Expense Ratio" instead of "Management Fees".
func TestExtractor_Extract_US(t *testing.T) {
	mock := newMockClient()
	mock.setupPageFetch(sampleUSHTML(), nil)

	pdfBytes, err := loadSamplePDFNamed(t, "DBMF_FACTSHEETS_EN.pdf")
	if err != nil {
		t.Skipf("sample PDF not available: %v", err)
	}
	mock.setupPDFFetch(pdfBytes, nil)

	e := NewExtractor()
	e.SetClient(mock.Client)

	result, err := e.Extract(context.Background(), "https://www.imgp.com/us/fund/US53700T8273-imgp-dbi-managed-futures-strategy-etf")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Fund identity comes from the page's structured JSON.
	if result.FundInfo == nil {
		t.Fatal("FundInfo is nil")
	}
	if result.FundInfo.Symbol != "US53700T8273" {
		t.Errorf("FundInfo.Symbol = %q, want %q", result.FundInfo.Symbol, "US53700T8273")
	}
	if result.FundInfo.Name != "iMGP DBi Managed Futures Strategy ETF" {
		t.Errorf("FundInfo.Name = %q, want %q", result.FundInfo.Name, "iMGP DBi Managed Futures Strategy ETF")
	}

	if result.FundProfile == nil {
		t.Fatal("FundProfile is nil")
	}
	// The US factsheet lists CUSIP, not ISIN — the ISIN is back-filled from
	// the fund page.
	if result.FundProfile.Isin != "US53700T8273" {
		t.Errorf("FundProfile.Isin = %q, want %q (back-filled)", result.FundProfile.Isin, "US53700T8273")
	}
	// 05/07/2019 is month-first on US factsheets: 7 May 2019, not 5 July.
	wantInception := time.Date(2019, 5, 7, 0, 0, 0, 0, time.UTC)
	if !result.FundProfile.InceptionDate.Equal(wantInception) {
		t.Errorf("InceptionDate = %v, want %v", result.FundProfile.InceptionDate, wantInception)
	}
	// Gross Expense Ratio 0.85% stored as fraction.
	if math.Abs(result.FundProfile.AnnualExpenseRatio-0.0085) > 1e-12 {
		t.Errorf("AnnualExpenseRatio = %v, want 0.0085", result.FundProfile.AnnualExpenseRatio)
	}
	if result.FundProfile.ShareClassName != "" {
		t.Errorf("ShareClassName = %q, want empty (US factsheets omit it)", result.FundProfile.ShareClassName)
	}
	if result.FundProfile.TotalNetAssets != 3_900_000_000 {
		t.Errorf("TotalNetAssets = %.0f, want 3900000000", result.FundProfile.TotalNetAssets)
	}

	if result.RiskMeasures == nil {
		t.Error("RiskMeasures is nil (US factsheet has a risk measures section)")
	} else {
		if math.Abs(result.RiskMeasures.Volatility-12.39) > 1e-9 {
			t.Errorf("Volatility = %v, want 12.39", result.RiskMeasures.Volatility)
		}
		if math.Abs(result.RiskMeasures.SharpeRatio-0.35) > 1e-9 {
			t.Errorf("SharpeRatio = %v, want 0.35", result.RiskMeasures.SharpeRatio)
		}
	}
}

// --- Extract failures ---

func TestExtractor_Extract_PageFetchError(t *testing.T) {
	mock := newMockClient()
	mock.setupPageFetch("", errors.New("network error"))

	e := NewExtractor()
	e.SetClient(mock.Client)

	_, err := e.Extract(context.Background(), "https://www.imgp.com/fund/LU2951555585")
	if err == nil {
		t.Fatal("expected error for page fetch failure")
	}
	if !strings.Contains(err.Error(), "fetch page") {
		t.Errorf("error %q should contain 'fetch page'", err.Error())
	}
}

func TestExtractor_Extract_NoFactsheetLink(t *testing.T) {
	mock := newMockClient()
	mock.setupPageFetch(`<html><body>no factsheet link</body></html>`, nil)

	e := NewExtractor()
	e.SetClient(mock.Client)

	_, err := e.Extract(context.Background(), "https://www.imgp.com/fund/LU2951555585")
	if err == nil {
		t.Fatal("expected error when no factsheet link found")
	}
	if !strings.Contains(err.Error(), "extract PDF URL") {
		t.Errorf("error %q should contain 'extract PDF URL'", err.Error())
	}
}

func TestExtractor_Extract_PDFFetchError(t *testing.T) {
	mock := newMockClient()
	mock.setupPageFetch(sampleHTMLWithFactsheetLink(), nil)
	mock.setupPDFFetch(nil, errors.New("PDF not found"))

	e := NewExtractor()
	e.SetClient(mock.Client)

	_, err := e.Extract(context.Background(), "https://www.imgp.com/fund/LU2951555585")
	if err == nil {
		t.Fatal("expected error for PDF fetch failure")
	}
	if !strings.Contains(err.Error(), "fetch PDF") {
		t.Errorf("error %q should contain 'fetch PDF'", err.Error())
	}
}

func TestExtractor_Extract_InvalidPDF(t *testing.T) {
	mock := newMockClient()
	mock.setupPageFetch(sampleHTMLWithFactsheetLink(), nil)
	mock.setupPDFFetch([]byte("not a valid PDF"), nil)

	e := NewExtractor()
	e.SetClient(mock.Client)

	_, err := e.Extract(context.Background(), "https://www.imgp.com/fund/LU2951555585")
	if err == nil {
		t.Fatal("expected error for invalid PDF")
	}
	if !strings.Contains(err.Error(), "extract PDF text") {
		t.Errorf("error %q should contain 'extract PDF text'", err.Error())
	}
}

func TestExtractor_Extract_OptionalSectionPartial(t *testing.T) {
	// Uses the real PDF which has partial risk measures (only Volatility + Sharpe)
	// This tests that partial optional data is accepted without failure.
	mock := newMockClient()
	mock.setupPageFetch(sampleHTMLWithFactsheetLink(), nil)

	realPDF, err := loadSamplePDF(t)
	if err != nil {
		t.Skipf("sample PDF not available: %v", err)
	}
	mock.setupPDFFetch(realPDF, nil)

	e := NewExtractor()
	e.SetClient(mock.Client)

	result, err := e.Extract(context.Background(), "https://www.imgp.com/fund/LU2951555585")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// RiskMeasures should be present but partial
	if result.RiskMeasures == nil {
		t.Error("RiskMeasures should be present (partial data is acceptable)")
	} else {
		if !result.RiskMeasures.HasField(extractor.RiskFieldVolatility) {
			t.Error("Volatility should be present")
		}
	}

	if len(result.AssetClassAllocation) == 0 {
		t.Error("AssetClassAllocation should have entries")
	}
	if len(result.EquityDerivativesByRegion) == 0 {
		t.Error("EquityDerivativesByRegion should have entries")
	}
}

// --- Context cancellation ---

func TestExtractor_Extract_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	e := NewExtractor()
	mock := newMockClient()
	mock.setupPageFetch("html", nil)
	e.SetClient(mock.Client)

	_, err := e.Extract(ctx, "https://www.imgp.com/fund/LU2951555585")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	// Verify no fetch was made — the initial select catches cancellation before any I/O
	if mock.pageCalls != 0 {
		t.Error("page should not have been fetched when context is cancelled")
	}
}

// --- parseFundInfoFromHTML ---

func TestParseFundInfoFromHTML(t *testing.T) {
	tests := []struct {
		name       string
		sourceURL  string
		html       string
		wantSymbol string
		wantName   string
	}{
		{
			name:       "standard URL and title",
			sourceURL:  "https://www.imgp.com/fund/LU2951555585",
			html:       `<html><head><title>DBi Managed Futures Fund | iMGP</title></head></html>`,
			wantSymbol: "LU2951555585",
			wantName:   "DBi Managed Futures Fund",
		},
		{
			name:       "title with dash separator",
			sourceURL:  "https://www.imgp.com/fund/IE00BXYZ1234",
			html:       `<html><head><title>Some Fund Name - iMGP</title></head></html>`,
			wantSymbol: "IE00BXYZ1234",
			wantName:   "Some Fund Name",
		},
		{
			name:       "title without iMGP suffix",
			sourceURL:  "https://www.imgp.com/fund/LU1234567890",
			html:       `<html><head><title>Plain Fund Name</title></head></html>`,
			wantSymbol: "LU1234567890",
			wantName:   "Plain Fund Name",
		},
		{
			name:       "no title tag",
			sourceURL:  "https://www.imgp.com/fund/LU1234567890",
			html:       `<html><head></head></html>`,
			wantSymbol: "LU1234567890",
			wantName:   "",
		},
		{
			name:       "no title tag at all",
			sourceURL:  "https://www.imgp.com/fund/LU1234567890",
			html:       `<html><body>no head</body></html>`,
			wantSymbol: "LU1234567890",
			wantName:   "",
		},
		{
			name:       "page JSON takes precedence over URL and title",
			sourceURL:  "https://www.imgp.com/fund/LU1234567890",
			html:       `<html><head><title>Wrong Name | iMGP</title></head><body>const fund = {"sub_fund_name": "Real Fund Name", "isin": "LU9999999999"};</body></html>`,
			wantSymbol: "LU9999999999",
			wantName:   "Real Fund Name",
		},
		{
			name:       "US slug URL and title fallback",
			sourceURL:  "https://www.imgp.com/us/fund/US53700T8273-imgp-dbi-managed-futures-strategy-etf",
			html:       `<html><head><title>iMGP DBi Managed Futures Strategy ETF | iM Global Partner US</title></head></html>`,
			wantSymbol: "US53700T8273",
			wantName:   "iMGP DBi Managed Futures Strategy ETF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := parseFundInfoFromHTML(tt.sourceURL, tt.html)
			if info.Symbol != tt.wantSymbol {
				t.Errorf("Symbol = %q, want %q", info.Symbol, tt.wantSymbol)
			}
			if info.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", info.Name, tt.wantName)
			}
		})
	}
}

// --- extractISINFromURL ---

func TestExtractISINFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"standard path", "https://www.imgp.com/fund/LU2951555585", "LU2951555585"},
		{"with query params", "https://www.imgp.com/fund/IE00BXYZ1234?lang=en", "IE00BXYZ1234"},
		{"without www", "https://imgp.com/fund/LU1234567890", "LU1234567890"},
		{"US slug", "https://www.imgp.com/us/fund/US53700T8273-imgp-dbi-managed-futures-strategy-etf", "US53700T8273"},
		{"slug without ISIN", "https://www.imgp.com/fund/some-fund-name", ""},
		{"invalid URL", "not a url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractISINFromURL(tt.url)
			if got != tt.want {
				t.Errorf("extractISINFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// --- extractFundNameFromTitle ---

func TestExtractFundNameFromTitle(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"pipe separator", `<html><title>Fund Name | iMGP</title></html>`, "Fund Name"},
		{"dash separator", `<html><title>Fund Name - iMGP</title></html>`, "Fund Name"},
		{"no suffix", `<html><title>Fund Name</title></html>`, "Fund Name"},
		{"no title tag", `<html></html>`, ""},
		{"empty title", `<html><title></title></html>`, ""},
		{"extra whitespace", `<html><title>  Fund Name  | iMGP  </title></html>`, "Fund Name"},
		{"uppercase IMGP", `<html><title>Fund Name | IMGP</title></html>`, "Fund Name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFundNameFromTitle(tt.html)
			if got != tt.want {
				t.Errorf("extractFundNameFromTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Helpers ---

// mockClient wraps Client with tracking.
type mockClient struct {
	*Client
	htmlResp  string
	htmlErr   error
	pdfResp   []byte
	pdfErr    error
	pageCalls int
	pdfCalls  int
}

func newMockClient() *mockClient {
	mc := &mockClient{
		Client: NewClient(),
	}
	mc.SetThrottle(func(time.Duration) {}) // no real waiting in tests
	return mc
}

func (m *mockClient) setupPageFetch(html string, err error) {
	m.htmlResp = html
	m.htmlErr = err
	m.SetFetchFunc(func(url string) (string, error) {
		m.pageCalls++
		return m.htmlResp, m.htmlErr
	})
}

func (m *mockClient) setupPDFFetch(pdf []byte, err error) {
	m.pdfResp = pdf
	m.pdfErr = err
	m.SetFetchPDFFunc(func(url string) ([]byte, error) {
		m.pdfCalls++
		return m.pdfResp, m.pdfErr
	})
}

// sampleHTMLWithFactsheetLink returns HTML with a factsheet PDF link.
func sampleHTMLWithFactsheetLink() string {
	return `<html><head><title>DBi Managed Futures Fund | iMGP</title></head>
<body>
<a href="https://www.imgp.com/uploads/factsheets/LU2951555585_FACTSHEETS_EN.pdf" target="_blank">Factsheet</a>
</body></html>`
}

// sampleUSHTML returns a US fund page with the `const fund` JSON object and
// a factsheet link.
func sampleUSHTML() string {
	return `<html><head><title>iMGP DBi Managed Futures Strategy ETF | iM Global Partner US</title></head>
<body>
<script>const fund = {"sub_fund_name": "iMGP DBi Managed Futures Strategy ETF", "share_class_name": "ETF USD", "isin": "US53700T8273", "cusip_code": "56170L828", "management_fee_us": 0.85};</script>
<a href="https://www.imgp.com/uploads/factsheets/DBMF_FACTSHEETS_EN.pdf" target="_blank">Factsheet</a>
</body></html>`
}

// loadSamplePDF reads the EU sample PDF from the feature samples directory.
func loadSamplePDF(t *testing.T) ([]byte, error) {
	t.Helper()
	return loadSamplePDFNamed(t, "LU2951555585_FACTSHEETS_EN.pdf")
}

// loadSamplePDFNamed reads a named sample PDF from the feature samples directory.
func loadSamplePDFNamed(t *testing.T, filename string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(filepath.Join("..", "..", "..", "..", "features", "f024_imgp-scraper", "samples", filename))
}

// --- parseFundPageJSON ---

func TestParseFundPageJSON(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantIsin string
		wantName string
		wantNil  bool
	}{
		{
			name:     "valid JSON",
			html:     `<body>const fund = {"sub_fund_name": "A Fund", "isin": "LU1234567890"};</body>`,
			wantIsin: "LU1234567890",
			wantName: "A Fund",
		},
		{
			name:    "missing marker",
			html:    `<body>no fund json here</body>`,
			wantNil: true,
		},
		{
			name:    "invalid JSON after marker",
			html:    `<body>const fund = {not json};</body>`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFundPageJSON(tt.html)
			if tt.wantNil {
				if f != nil {
					t.Fatalf("expected nil, got %+v", f)
				}
				return
			}
			if f == nil {
				t.Fatal("expected non-nil, got nil")
			}
			if f.Isin != tt.wantIsin {
				t.Errorf("Isin = %q, want %q", f.Isin, tt.wantIsin)
			}
			if f.SubFundName != tt.wantName {
				t.Errorf("SubFundName = %q, want %q", f.SubFundName, tt.wantName)
			}
		})
	}
}
