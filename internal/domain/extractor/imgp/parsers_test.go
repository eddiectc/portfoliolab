package imgp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

// --- ParseFundFacts ---

func TestParseFundFacts(t *testing.T) {
	pdfText := loadFixture(t, "lu2951555585_factsheet.txt")

	profile, err := ParseFundFacts(pdfText)
	if err != nil {
		t.Fatalf("ParseFundFacts: %v", err)
	}

	// Fund Size: 439.2 Mn USD = 439,200,000
	if profile.TotalNetAssets != 439_200_000 {
		t.Errorf("TotalNetAssets = %.0f, want 439200000", profile.TotalNetAssets)
	}

	// Inception Date: 07/03/2025
	expectedInception, _ := time.Parse("02/01/2006", "07/03/2025")
	if !profile.InceptionDate.Equal(expectedInception) {
		t.Errorf("InceptionDate = %v, want %v", profile.InceptionDate, expectedInception)
	}

	// ISIN
	if profile.Isin != "LU2951555585" {
		t.Errorf("Isin = %q, want %q", profile.Isin, "LU2951555585")
	}

	// Share Class
	if profile.ShareClassName != "R USD UCITS ETF" {
		t.Errorf("ShareClassName = %q, want %q", profile.ShareClassName, "R USD UCITS ETF")
	}

	// Management Fees
	if profile.AnnualExpenseRatio != 0.55 {
		t.Errorf("AnnualExpenseRatio = %.2f, want 0.55", profile.AnnualExpenseRatio)
	}

	// Ongoing Charges
	if profile.OngoingCharges != 0.75 {
		t.Errorf("OngoingCharges = %.2f, want 0.75", profile.OngoingCharges)
	}
}

func TestParseFundFacts_MissingSection(t *testing.T) {
	_, err := ParseFundFacts("some random text without fund facts")
	if err == nil {
		t.Error("expected error for missing fund facts section")
	}
}

// --- ParseRiskMeasures ---

func TestParseRiskMeasures(t *testing.T) {
	pdfText := loadFixture(t, "lu2951555585_factsheet.txt")

	risk, err := ParseRiskMeasures(pdfText)
	if err != nil {
		t.Fatalf("ParseRiskMeasures: %v", err)
	}

	// This fund has Volatility and Sharpe but other fields are absent
	if risk == nil {
		t.Fatal("expected non-nil risk measures")
	}

	if risk.Volatility != 9.16 {
		t.Errorf("Volatility = %.2f, want 9.16", risk.Volatility)
	}
	if !risk.HasField(extractor.RiskFieldVolatility) {
		t.Error("expected Volatility field to be marked as present")
	}

	if risk.SharpeRatio != 2.52 {
		t.Errorf("SharpeRatio = %.2f, want 2.52", risk.SharpeRatio)
	}
	if !risk.HasField(extractor.RiskFieldSharpeRatio) {
		t.Error("expected SharpeRatio field to be marked as present")
	}

	// Other fields should be 0 AND marked as absent
	if risk.HasField(extractor.RiskFieldInfoRatio) {
		t.Error("InfoRatio should be marked as absent")
	}
	if risk.HasField(extractor.RiskFieldBeta) {
		t.Error("Beta should be marked as absent")
	}
	if risk.HasField(extractor.RiskFieldCorrelation) {
		t.Error("Correlation should be marked as absent")
	}
	if risk.HasField(extractor.RiskFieldTrackingError) {
		t.Error("TrackingError should be marked as absent")
	}
	if risk.AllFieldsPresent() {
		t.Error("AllFieldsPresent should be false for partial data")
	}
}

func TestParseRiskMeasures_AllFields(t *testing.T) {
	// Synthesize text with all risk measures present
	text := `Measure of Risk Annualized risk measures Fund Volatility
(1Y)
 5.23% Sharpe Ratio
(1Y)
 1.85 Annualized risk measures Information Ratio
(1Y)
 0.45 Beta
(1Y)
 0.78 Correlation
(1Y)
 0.92 Tracking Error
(1Y)
 2.15%`

	risk, err := ParseRiskMeasures(text)
	if err != nil {
		t.Fatalf("ParseRiskMeasures: %v", err)
	}
	if risk == nil {
		t.Fatal("expected non-nil risk measures")
	}

	if risk.Volatility != 5.23 {
		t.Errorf("Volatility = %.2f, want 5.23", risk.Volatility)
	}
	if risk.SharpeRatio != 1.85 {
		t.Errorf("SharpeRatio = %.2f, want 1.85", risk.SharpeRatio)
	}
	if risk.InfoRatio != 0.45 {
		t.Errorf("InfoRatio = %.2f, want 0.45", risk.InfoRatio)
	}
	if risk.Beta != 0.78 {
		t.Errorf("Beta = %.2f, want 0.78", risk.Beta)
	}
	if risk.Correlation != 0.92 {
		t.Errorf("Correlation = %.2f, want 0.92", risk.Correlation)
	}
	if risk.TrackingError != 2.15 {
		t.Errorf("TrackingError = %.2f, want 2.15", risk.TrackingError)
	}

	// All fields should be marked as present
	if !risk.AllFieldsPresent() {
		t.Error("AllFieldsPresent should be true when all six fields are parsed")
	}
	for _, field := range []extractor.RiskFieldsMask{
		extractor.RiskFieldVolatility,
		extractor.RiskFieldSharpeRatio,
		extractor.RiskFieldInfoRatio,
		extractor.RiskFieldBeta,
		extractor.RiskFieldCorrelation,
		extractor.RiskFieldTrackingError,
	} {
		if !risk.HasField(field) {
			t.Errorf("expected field %d to be marked as present", field)
		}
	}
}

func TestParseRiskMeasures_MissingSection(t *testing.T) {
	risk, err := ParseRiskMeasures("no risk data here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if risk != nil {
		t.Errorf("expected nil for missing section, got %+v", risk)
	}
}

// --- ParseAssetClassAllocation ---

func TestParseAssetClassAllocation(t *testing.T) {
	pdfText := loadFixture(t, "lu2951555585_factsheet.txt")

	entries, err := ParseAssetClassAllocation(pdfText)
	if err != nil {
		t.Fatalf("ParseAssetClassAllocation: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected non-empty asset class allocation")
	}

	// Expected: Bonds -25.7%, Gold 4.3%, Oil 15.4%, Equities 20.3%
	// The order in the PDF is: Bonds, Gold, Oil, Equities
	expected := map[string]float64{
		"Bonds":    -25.7,
		"Gold":      4.3,
		"Oil":       15.4,
		"Equities":  20.3,
	}

	for _, entry := range entries {
		want, ok := expected[entry.AssetClass]
		if !ok {
			t.Errorf("unexpected asset class: %q", entry.AssetClass)
			continue
		}
		if entry.Percent != want {
			t.Errorf("%s: percent = %.1f, want %.1f", entry.AssetClass, entry.Percent, want)
		}
		delete(expected, entry.AssetClass)
	}

	for k := range expected {
		t.Errorf("missing asset class: %q", k)
	}
}

func TestParseAssetClassAllocation_MissingSection(t *testing.T) {
	entries, err := ParseAssetClassAllocation("no derivatives allocation here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for missing section, got %+v", entries)
	}
}

func TestParseAssetClassAllocation_CountMismatch(t *testing.T) {
	// Section with 3 labels but 5 percentages — should return partial data + error
	text := `Derivatives Allocation
Bonds
Gold
Oil
-10
0
10
-25.7%
4.3%
15.4%
20.3%
5.0%`

	entries, err := ParseAssetClassAllocation(text)
	if err == nil {
		t.Fatal("expected error for label/percentage count mismatch")
	}
	if !strings.Contains(err.Error(), "count mismatch") {
		t.Errorf("expected count mismatch error, got: %v", err)
	}
	// Should still return paired entries (3 labels paired with first 3 percentages)
	if len(entries) != 3 {
		t.Errorf("expected 3 paired entries, got %d", len(entries))
	}
}

// --- ParseEquityDerivativesByRegion ---

func TestParseEquityDerivativesByRegion(t *testing.T) {
	pdfText := loadFixture(t, "lu2951555585_factsheet.txt")

	entries, err := ParseEquityDerivativesByRegion(pdfText)
	if err != nil {
		t.Fatalf("ParseEquityDerivativesByRegion: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected non-empty equity derivatives by region")
	}

	// Expected values from the PDF:
	// Cash & Others: 0%, Asia ex Japan: 0.2%, Japan: 0.3%, Europe ex-EMU: 0.5%,
	// EMU: 0.6%, North America: 2.9%, Emerging Countries: 15.8%
	expected := map[string]float64{
		"Cash & Others":        0.0,
		"Asia ex Japan":        0.2,
		"Japan":                0.3,
		"Europe ex-EMU":        0.5,
		"EMU":                  0.6,
		"North America":        2.9,
		"Emerging Countries": 15.8,
	}

	if len(entries) != len(expected) {
		t.Fatalf("expected %d region entries, got %d", len(expected), len(entries))
	}

	for _, entry := range entries {
		want, ok := expected[entry.Region]
		if !ok {
			t.Errorf("unexpected region: %q", entry.Region)
			continue
		}
		if entry.Percent != want {
			t.Errorf("%s: percent = %.1f, want %.1f", entry.Region, entry.Percent, want)
		}
		delete(expected, entry.Region)
	}

	for k := range expected {
		t.Errorf("missing region: %q", k)
	}
}

func TestParseEquityDerivativesByRegion_MissingSection(t *testing.T) {
	entries, err := ParseEquityDerivativesByRegion("no equity derivatives here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for missing section, got %+v", entries)
	}
}

func TestParseEquityDerivativesByRegion_CountMismatch(t *testing.T) {
	// Section with 3 labels but 5 percentages — should return partial data + error
	text := `Equity Derivatives by Region
North
America
Japan
0
10
20
15.8%
2.9%
0.3%
5.0%
10.0%`

	entries, err := ParseEquityDerivativesByRegion(text)
	if err == nil {
		t.Fatal("expected error for label/percentage count mismatch")
	}
	if !strings.Contains(err.Error(), "count mismatch") {
		t.Errorf("expected count mismatch error, got: %v", err)
	}
	// Should still return paired entries
	if len(entries) != 2 {
		t.Errorf("expected 2 paired entries, got %d", len(entries))
	}
}

// --- ParseCurrencyDerivativesAllocation ---

func TestParseCurrencyDerivativesAllocation(t *testing.T) {
	pdfText := loadFixture(t, "lu2951555585_factsheet.txt")

	entries, err := ParseCurrencyDerivativesAllocation(pdfText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected non-empty currency derivatives allocation")
	}

	// Debug: print what we got
	for _, e := range entries {
		t.Logf("  %s: %.1f%%", e.Currency, e.Percent)
	}

	// Expected values from the PDF (sorted by absolute value, descending):
	// JPY: -89.4%, EUR: 68.3%, EM FX: 12.8%, USD: 4.8%, Other DM FX: 3%,
	// GBP: 0.2%, AUD: 0.1%, CHF: 0.1%, SEK: 0%
	// Note: "Other" + "DM FX" merged into "Other DM FX" by post-processing

	// Verify entries are sorted by value descending (positives first, negatives last)
	for i := 1; i < len(entries); i++ {
		if entries[i].Percent > entries[i-1].Percent {
			t.Errorf("entries not sorted: %s (%.1f%%) before %s (%.1f%%)",
				entries[i-1].Currency, entries[i-1].Percent,
				entries[i].Currency, entries[i].Percent)
		}
	}

	// Verify key values
	foundJPY := false
	foundEUR := false
	foundOtherDMFX := false
	for _, e := range entries {
		if strings.EqualFold(e.Currency, "JPY") && e.Percent == -89.4 {
			foundJPY = true
		}
		if strings.EqualFold(e.Currency, "EUR") && e.Percent == 68.3 {
			foundEUR = true
		}
		if strings.EqualFold(e.Currency, "Other DM FX") && e.Percent == 3.0 {
			foundOtherDMFX = true
		}
	}

	if !foundJPY {
		t.Error("expected JPY at -89.4%")
	}
	if !foundEUR {
		t.Error("expected EUR at 68.3%")
	}
	if !foundOtherDMFX {
		t.Error("expected Other DM FX at 3.0%")
	}
}

func TestParseCurrencyDerivativesAllocation_MissingSection(t *testing.T) {
	entries, err := ParseCurrencyDerivativesAllocation("no currency derivatives here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil for missing section, got %+v", entries)
	}
}

func TestParseCurrencyDerivativesAllocation_CountMismatch(t *testing.T) {
	// Section with 2 labels but 4 percentages — should return partial data + error
	text := `Currency Derivatives Allocation
USD
EUR
-50
0
50
12.8%
68.3%
5.0%
10.0%`

	entries, err := ParseCurrencyDerivativesAllocation(text)
	if err == nil {
		t.Fatal("expected error for label/percentage count mismatch")
	}
	if !strings.Contains(err.Error(), "count mismatch") {
		t.Errorf("expected count mismatch error, got: %v", err)
	}
	// Should still return paired entries
	if len(entries) != 2 {
		t.Errorf("expected 2 paired entries, got %d", len(entries))
	}
}

// --- ParseReferenceDate ---

func TestParseReferenceDate(t *testing.T) {
	pdfText := loadFixture(t, "lu2951555585_factsheet.txt")

	date, err := ParseReferenceDate(pdfText)
	if err != nil {
		t.Fatalf("ParseReferenceDate: %v", err)
	}

	// "Fact Sheet – April 30, 2026"
	expected, _ := time.Parse("January 2, 2006", "April 30, 2026")
	if !date.Equal(expected) {
		t.Errorf("reference date = %v, want %v", date, expected)
	}
}

func TestParseReferenceDate_Missing(t *testing.T) {
	_, err := ParseReferenceDate("no factsheet header here")
	if err == nil {
		t.Error("expected error for missing reference date")
	}
}

func TestParseReferenceDate_Formats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantYear int
		wantMon  time.Month
		wantDay  int
	}{
		{"long month", "Fact Sheet – April 30, 2026 MARKETING", 2026, time.April, 30},
		{"dash separator", "Fact Sheet - March 15, 2025 MARKETING", 2025, time.March, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date, err := ParseReferenceDate(tt.input)
			if err != nil {
				t.Fatalf("ParseReferenceDate: %v", err)
			}
			if date.Year() != tt.wantYear || date.Month() != tt.wantMon || date.Day() != tt.wantDay {
				t.Errorf("got %v, want %d-%02d-%02d", date, tt.wantYear, tt.wantMon, tt.wantDay)
			}
		})
	}
}

// --- ExtractPDFText ---

func TestExtractPDFText(t *testing.T) {
	// Read the raw PDF bytes
	pdfBytes, err := os.ReadFile(filepath.Join("testdata", "..", "..", "..", "..", "..", "features", "f024_imgp-scraper", "samples", "LU2951555585_FACTSHEETS_EN.pdf"))
	if err != nil {
		t.Skipf("sample PDF not found: %v", err)
	}

	text, err := ExtractPDFText(pdfBytes)
	if err != nil {
		t.Fatalf("ExtractPDFText: %v", err)
	}

	if len(text) == 0 {
		t.Error("expected non-empty text from PDF")
	}

	// Verify some known content is present
	mustContain := []string{
		"Fact Sheet",
		"Fund Facts",
		"LU2951555585",
		"Equity Derivatives by Region",
		"Currency Derivatives Allocation",
	}

	for _, s := range mustContain {
		if !strings.Contains(text, s) {
			t.Errorf("PDF text missing expected content: %q", s)
		}
	}
}

func TestExtractPDFText_InvalidPDF(t *testing.T) {
	_, err := ExtractPDFText([]byte("not a valid PDF"))
	if err == nil {
		t.Error("expected error for invalid PDF")
	}
}

// --- parseDate ---

func TestParseDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // "2006-01-02" format
	}{
		{"dd/mm/yyyy", "07/03/2025", "2025-03-07"},
		{"Month DD, YYYY", "April 30, 2026", "2026-04-30"},
		{"DD-MM-YYYY", "30-04-2026", "2026-04-30"},
		{"YYYY-MM-DD", "2026-04-30", "2026-04-30"},
		{"DD/MM/YY", "07/03/25", "2025-03-07"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDate(tt.input)
			if err != nil {
				t.Fatalf("parseDate(%q): %v", tt.input, err)
			}
			if got.Format("2006-01-02") != tt.want {
				t.Errorf("parseDate(%q) = %s, want %s", tt.input, got.Format("2006-01-02"), tt.want)
			}
		})
	}
}

func TestParseDate_Invalid(t *testing.T) {
	_, err := parseDate("not a date")
	if err == nil {
		t.Error("expected error for invalid date")
	}
}

// --- extractFundSize ---

func TestExtractFundSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"millions", "Fund Size 439.2 Mn USD", 439_200_000, false},
		{"billions", "Fund Size 1.5 Bn USD", 1_500_000_000, false},
		{"million spelled", "Fund Size 500 Million EUR", 500_000_000, false},
		{"missing", "Fund Size not available", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFundSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractFundSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractFundSize() = %.0f, want %.0f", got, tt.want)
			}
		})
	}
}

// --- extractISIN ---

func TestExtractISIN(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"standard", "ISIN LU2951555585", "LU2951555585", false},
		{"with surrounding text", "foo ISIN US1234567890 bar", "US1234567890", false},
		{"missing", "no ISIN here", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractISIN(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractISIN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractISIN() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- extractPercent ---

func TestExtractPercent(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		input   string
		want    float64
		wantErr bool
	}{
		{"management fees", "Management Fees", "Management Fees 0.55% Ongoing Charges 0.75%", 0.55, false},
		{"ongoing charges", "Ongoing Charges", "Management Fees 0.55% Ongoing Charges 0.75%", 0.75, false},
		{"missing", "TER", "no TER here", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractPercent(tt.input, tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPercent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractPercent() = %.2f, want %.2f", got, tt.want)
			}
		})
	}
}

// --- extractInceptionDate ---

func TestExtractInceptionDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // "2006-01-02" format
	}{
		{"standard dd/mm/yyyy", "Inception Date of theShare Class 07/03/2025", "2025-03-07"},
		{"with extra spacing", "Inception Date  of the Share Class 15/12/2023", "2023-12-15"},
		{"dash separator", "Inception Date 15-12-2023", "2023-12-15"},
		{"two-digit year", "Inception Date 07/03/25", "2025-03-07"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractInceptionDate(tt.input)
			if err != nil {
				t.Fatalf("extractInceptionDate(%q): %v", tt.input, err)
			}
			if got.Format("2006-01-02") != tt.want {
				t.Errorf("extractInceptionDate(%q) = %s, want %s", tt.input, got.Format("2006-01-02"), tt.want)
			}
		})
	}
}

func TestExtractInceptionDate_Missing(t *testing.T) {
	_, err := extractInceptionDate("no inception date here")
	if err == nil {
		t.Error("expected error for missing inception date")
	}
}

// --- extractShareClass ---

func TestExtractShareClass(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard with Classification delimiter", "Share Class R USD UCITS ETF Classification SFDR 6", "R USD UCITS ETF"},
		{"with Cut-off delimiter", "Share Class A EUR Cut-off Time TD 12:00", "A EUR"},
		{"with SRRI delimiter", "Share Class I USD UCITS ETF SRRI 5/7", "I USD UCITS ETF"},

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractShareClass(tt.input)
			if err != nil {
				t.Fatalf("extractShareClass(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("extractShareClass(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractShareClass_Missing(t *testing.T) {
	_, err := extractShareClass("no share class here")
	if err == nil {
		t.Error("expected error for missing share class")
	}
}

// --- Integration: full extraction from fixture ---

func TestFullExtractionFromFixture(t *testing.T) {
	pdfText := loadFixture(t, "lu2951555585_factsheet.txt")

	// Parse all sections
	profile, err := ParseFundFacts(pdfText)
	if err != nil {
		t.Errorf("ParseFundFacts: %v", err)
	}

	risk, err := ParseRiskMeasures(pdfText)
	if err != nil {
		t.Errorf("ParseRiskMeasures: %v", err)
	}

	assetClass, err := ParseAssetClassAllocation(pdfText)
	if err != nil {
		t.Errorf("ParseAssetClassAllocation: %v", err)
	}

	equityRegions, err := ParseEquityDerivativesByRegion(pdfText)
	if err != nil {
		t.Errorf("ParseEquityDerivativesByRegion: %v", err)
	}

	currencyAlloc, err := ParseCurrencyDerivativesAllocation(pdfText)
	if err != nil {
		// Fixture has 10 labels but only 9 percentages — expected partial data + warning
		t.Logf("ParseCurrencyDerivativesAllocation (expected partial): %v", err)
	}

	refDate, err := ParseReferenceDate(pdfText)
	if err != nil {
		t.Errorf("ParseReferenceDate: %v", err)
	}

	// Build ExtractResult
	result := &extractor.ExtractResult{
		Source:                      "imgp",
		AsOfDate:                    refDate,
		FundProfile:                 profile,
		RiskMeasures:                risk,
		AssetClassAllocation:        assetClass,
		EquityDerivativesByRegion:   equityRegions,
		CurrencyDerivativesAllocation: currencyAlloc,
	}

	// Verify required fields are present
	if result.FundProfile == nil {
		t.Error("FundProfile should not be nil (required)")
	} else {
		t.Logf("FundProfile: AUM=%.0f, ISIN=%s, ShareClass=%s",
			result.FundProfile.TotalNetAssets, result.FundProfile.Isin, result.FundProfile.ShareClassName)
	}
	if result.AsOfDate.IsZero() {
		t.Error("AsOfDate should not be zero (required)")
	}

	// Optional fields may be nil but should not cause errors
	if result.RiskMeasures != nil {
		t.Logf("RiskMeasures: Vol=%.2f, Sharpe=%.2f",
			result.RiskMeasures.Volatility, result.RiskMeasures.SharpeRatio)
	}
	t.Logf("AssetClassAllocation: %d entries", len(result.AssetClassAllocation))
	t.Logf("EquityDerivativesByRegion: %d entries", len(result.EquityDerivativesByRegion))
	t.Logf("CurrencyDerivativesAllocation: %d entries", len(result.CurrencyDerivativesAllocation))
}
