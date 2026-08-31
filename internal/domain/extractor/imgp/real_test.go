package imgp

import (
	_ "embed"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
)

//go:embed testdata/lu2951555585_factsheet.txt
var realPDFText string

func TestParseFundFacts_Real(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	profile, err := ParseFundFacts(realPDFText, DateLayoutDayFirst)
	if err != nil {
		t.Fatalf("ParseFundFacts failed: %v", err)
	}
	if profile.TotalNetAssets == 0 {
		t.Error("expected TotalNetAssets > 0")
	}
	if profile.Isin == "" {
		t.Error("expected Isin to be populated")
	}
	if profile.ShareClassName == "" {
		t.Error("expected ShareClassName to be populated")
	}
	t.Logf("FundFacts: AUM=%.2f, ISIN=%s, ShareClass=%s, OngoingCharges=%.2f%%",
		profile.TotalNetAssets, profile.Isin, profile.ShareClassName, profile.OngoingCharges)
}

func TestParseRiskMeasures_Real(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	risk, err := ParseRiskMeasures(realPDFText)
	if err != nil {
		t.Fatalf("ParseRiskMeasures failed: %v", err)
	}
	if risk == nil {
		t.Fatal("RiskMeasures is nil")
	}
	// This fund is < 1 year old, so only Volatility and Sharpe are present.
	if !risk.HasField(extractor.RiskFieldVolatility) {
		t.Error("expected Volatility to be present")
	}
	if !risk.HasField(extractor.RiskFieldSharpeRatio) {
		t.Error("expected SharpeRatio to be present")
	}
	// These fields are absent for this fund.
	if risk.HasField(extractor.RiskFieldInfoRatio) {
		t.Error("InfoRatio should be absent for this fund")
	}
	t.Logf("RiskMeasures: Vol=%.2f, Sharpe=%.2f, FieldsPresent=%d",
		risk.Volatility, risk.SharpeRatio, risk.FieldsPresent)
}

func TestParseAssetClassAllocation_Real(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	alloc, err := ParseAssetClassAllocation(realPDFText)
	if err != nil {
		t.Fatalf("ParseAssetClassAllocation failed: %v", err)
	}
	if len(alloc) == 0 {
		t.Error("expected at least one asset class entry")
	}
	t.Logf("AssetClassAllocation: %d entries", len(alloc))
	for _, entry := range alloc {
		t.Logf("  %s: %.2f%%", entry.AssetClass, entry.Percent)
	}
}

func TestParseEquityDerivativesByRegion_Real(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	derivs, err := ParseEquityDerivativesByRegion(realPDFText)
	if err != nil {
		t.Fatalf("ParseEquityDerivativesByRegion failed: %v", err)
	}
	if len(derivs) == 0 {
		t.Error("expected at least one region entry")
	}
	t.Logf("EquityDerivativesByRegion: %d entries", len(derivs))
	for _, entry := range derivs {
		t.Logf("  %s: %.2f%%", entry.Region, entry.Percent)
	}
}

func TestParseCurrencyDerivativesAllocation_Real(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	alloc, err := ParseCurrencyDerivativesAllocation(realPDFText)
	if err != nil {
		// This fund has a data quality issue: 10 labels but 9 percentages.
		// Parser returns partial data + error. This is expected.
		t.Logf("ParseCurrencyDerivativesAllocation returned expected error: %v", err)
	}
	if len(alloc) == 0 {
		t.Error("expected at least one currency entry (even with partial data)")
	}
	t.Logf("CurrencyDerivativesAllocation: %d entries", len(alloc))
	for _, entry := range alloc {
		t.Logf("  %s: %.2f%%", entry.Currency, entry.Percent)
	}
}

func TestParseReferenceDate_Real(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	date, err := ParseReferenceDate(realPDFText)
	if err != nil {
		t.Fatalf("ParseReferenceDate failed: %v", err)
	}
	if date.IsZero() {
		t.Error("expected non-zero reference date")
	}
	// Sanity check: date should be within last 2 years
	twoYearsAgo := time.Now().AddDate(-2, 0, 0)
	if date.Before(twoYearsAgo) {
		t.Errorf("reference date %s is more than 2 years old", date.Format("2006-01-02"))
	}
	t.Logf("ReferenceDate: %s", date.Format("2006-01-02"))
}
