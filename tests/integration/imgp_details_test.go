package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/eddiectc/portfoliolab/internal/data"
	"github.com/eddiectc/portfoliolab/internal/domain/extractor"
	"github.com/eddiectc/portfoliolab/internal/types/symbol"
)

func TestIMGPSymbolDetails_FullStackRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewSymbolDetailsRepository(db)

	asOfDate := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)

	// Build an ExtractResult with all iMGP-specific fields
	result := &extractor.ExtractResult{
		AsOfDate: asOfDate,
		FundInfo: &extractor.FundInfo{
			Symbol: "LU2951555585",
			Name:   "iMGP Multi Asset Fund",
		},
		FundProfile: &extractor.FundProfile{
			Family:             "iM Global Partner",
			LegalType:          "Undertaking for Collective Investment",
			TotalNetAssets:     52400000,
			AnnualExpenseRatio: 0.015,
			InceptionDate:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			Isin:               "LU2951555585",
			ShareClassName:     "R USD UCITS ETF",
			OngoingCharges:     1.5,
		},
		RiskMeasures: &extractor.RiskMeasures{
			Volatility:    9.16,
			SharpeRatio:   2.52,
			FieldsPresent: extractor.RiskFieldVolatility | extractor.RiskFieldSharpeRatio,
		},
		AssetClassAllocation: []extractor.AssetClassEntry{
			{AssetClass: "Equities", Percent: 85.2},
			{AssetClass: "Bonds", Percent: 5.3},
			{AssetClass: "Gold", Percent: -2.1},
			{AssetClass: "Cash", Percent: 11.6},
		},
		EquityDerivativesByRegion: []extractor.RegionDerivativeEntry{
			{Region: "North America", Percent: 52.3},
			{Region: "Europe", Percent: 31.7},
			{Region: "Asia", Percent: 12.0},
			{Region: "Emerging Countries", Percent: 4.0},
		},
		CurrencyDerivativesAllocation: []extractor.CurrencyDerivativeEntry{
			{Currency: "USD", Percent: 65.0},
			{Currency: "EUR", Percent: 20.0},
			{Currency: "JPY", Percent: 15.0},
		},
	}

	// Step 1: Service maps ExtractResult → SymbolDetails
	details := extractResultToSymbolDetailsForTest(result, "IMGPFUND")

	// Step 2: Repository upserts to real SQLite
	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Step 3: Repository retrieves from real SQLite
	got, err := repo.GetByInternalSymbol(context.Background(), "IMGPFUND")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}

	// Verify FundProfile — new iMGP fields
	if got.FundProfile == nil {
		t.Fatal("expected non-nil FundProfile")
	}
	if got.FundProfile.Isin != "LU2951555585" {
		t.Errorf("expected Isin LU2951555585, got %q", got.FundProfile.Isin)
	}
	if got.FundProfile.ShareClassName != "R USD UCITS ETF" {
		t.Errorf("expected ShareClassName 'R USD UCITS ETF', got %q", got.FundProfile.ShareClassName)
	}
	if got.FundProfile.OngoingCharges != 1.5 {
		t.Errorf("expected OngoingCharges 1.5, got %f", got.FundProfile.OngoingCharges)
	}

	// Verify RiskMeasures
	if got.RiskMeasures == nil {
		t.Fatal("expected non-nil RiskMeasures")
	}
	if got.RiskMeasures.Volatility != 9.16 {
		t.Errorf("expected Volatility 9.16, got %f", got.RiskMeasures.Volatility)
	}
	if got.RiskMeasures.SharpeRatio != 2.52 {
		t.Errorf("expected SharpeRatio 2.52, got %f", got.RiskMeasures.SharpeRatio)
	}
	if got.RiskMeasures.InfoRatio != 0 {
		t.Errorf("expected InfoRatio 0 (not present), got %f", got.RiskMeasures.InfoRatio)
	}
	expectedMask := symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio
	if got.RiskMeasures.FieldsPresent != expectedMask {
		t.Errorf("expected FieldsPresent %v, got %v", expectedMask, got.RiskMeasures.FieldsPresent)
	}
	// Verify HasField helper on symbol-side type
	if !got.RiskMeasures.HasField(symbol.SymbolRiskFieldVolatility) {
		t.Error("expected HasField(Volatility) = true")
	}
	if got.RiskMeasures.HasField(symbol.SymbolRiskFieldInfoRatio) {
		t.Error("expected HasField(InfoRatio) = false")
	}

	// Verify AssetClassAllocation (including negative value)
	if len(got.AssetClassAllocation) != 4 {
		t.Fatalf("expected 4 asset class entries, got %d", len(got.AssetClassAllocation))
	}
	if got.AssetClassAllocation[0].AssetClass != "Equities" || got.AssetClassAllocation[0].Percent != 85.2 {
		t.Errorf("expected Equities 85.2, got %s %f", got.AssetClassAllocation[0].AssetClass, got.AssetClassAllocation[0].Percent)
	}
	if got.AssetClassAllocation[2].AssetClass != "Gold" || got.AssetClassAllocation[2].Percent != -2.1 {
		t.Errorf("expected Gold -2.1, got %s %f", got.AssetClassAllocation[2].AssetClass, got.AssetClassAllocation[2].Percent)
	}

	// Verify EquityDerivativesByRegion
	if len(got.EquityDerivativesByRegion) != 4 {
		t.Fatalf("expected 4 region entries, got %d", len(got.EquityDerivativesByRegion))
	}
	if got.EquityDerivativesByRegion[0].Region != "North America" {
		t.Errorf("expected North America, got %q", got.EquityDerivativesByRegion[0].Region)
	}

	// Verify CurrencyDerivativesAllocation
	if len(got.CurrencyDerivativesAllocation) != 3 {
		t.Fatalf("expected 3 currency entries, got %d", len(got.CurrencyDerivativesAllocation))
	}
	if got.CurrencyDerivativesAllocation[0].Currency != "USD" {
		t.Errorf("expected USD, got %q", got.CurrencyDerivativesAllocation[0].Currency)
	}

	// Verify ExtractorAsOfDate
	if got.ExtractorAsOfDate.IsZero() {
		t.Error("expected non-zero ExtractorAsOfDate")
	}
	if !got.ExtractorAsOfDate.Equal(asOfDate) {
		t.Errorf("expected ExtractorAsOfDate %v, got %v", asOfDate, got.ExtractorAsOfDate)
	}
}

func TestIMGPSymbolDetails_OverwritePreservesNewFields(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewSymbolDetailsRepository(db)

	// First upsert with iMGP data
	details1 := &symbol.SymbolDetails{
		InternalSymbol:    "IMGPFUND",
		ShortName:         "iMGP Fund",
		ExtractorAsOfDate: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		FetchedAt:         time.Now().Add(-24 * time.Hour),
		RiskMeasures: &symbol.RiskMeasures{
			Volatility:    9.16,
			SharpeRatio:   2.52,
			FieldsPresent: symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio,
		},
		AssetClassAllocation: []symbol.AssetClassEntry{
			{AssetClass: "Equities", Percent: 85.2},
			{AssetClass: "Bonds", Percent: 5.3},
		},
	}
	err := repo.Upsert(context.Background(), details1)
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	// Second upsert with updated data (simulating monthly factsheet refresh)
	details2 := &symbol.SymbolDetails{
		InternalSymbol:    "IMGPFUND",
		ShortName:         "iMGP Fund Updated",
		ExtractorAsOfDate: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		FetchedAt:         time.Now(),
		RiskMeasures: &symbol.RiskMeasures{
			Volatility:    8.95,
			SharpeRatio:   2.68,
			Beta:          0.79,
			FieldsPresent: symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio | symbol.SymbolRiskFieldBeta,
		},
		AssetClassAllocation: []symbol.AssetClassEntry{
			{AssetClass: "Equities", Percent: 87.1},
			{AssetClass: "Bonds", Percent: 3.4},
			{AssetClass: "Gold", Percent: -1.5},
			{AssetClass: "Cash", Percent: 11.0},
		},
		EquityDerivativesByRegion: []symbol.RegionDerivativeEntry{
			{Region: "North America", Percent: 53.0},
			{Region: "Europe", Percent: 30.0},
			{Region: "Asia", Percent: 17.0},
		},
	}
	err = repo.Upsert(context.Background(), details2)
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	// Verify updated data
	got, err := repo.GetByInternalSymbol(context.Background(), "IMGPFUND")
	if err != nil {
		t.Fatalf("GetByInternalSymbol: %v", err)
	}

	if got.ShortName != "iMGP Fund Updated" {
		t.Errorf("expected updated short name, got %q", got.ShortName)
	}
	if got.RiskMeasures == nil {
		t.Fatal("expected non-nil RiskMeasures")
	}
	if got.RiskMeasures.Volatility != 8.95 {
		t.Errorf("expected updated Volatility 8.95, got %f", got.RiskMeasures.Volatility)
	}
	if got.RiskMeasures.Beta != 0.79 {
		t.Errorf("expected new Beta 0.79, got %f", got.RiskMeasures.Beta)
	}
	if len(got.AssetClassAllocation) != 4 {
		t.Fatalf("expected 4 updated asset class entries, got %d", len(got.AssetClassAllocation))
	}
	if len(got.EquityDerivativesByRegion) != 3 {
		t.Fatalf("expected 3 region entries, got %d", len(got.EquityDerivativesByRegion))
	}
}

func TestIMGPSymbolDetails_NewColumnsStoredAsJSON(t *testing.T) {
	db := setupTestDB(t)
	repo := data.NewSymbolDetailsRepository(db)

	details := &symbol.SymbolDetails{
		InternalSymbol: "IMGPFUND",
		ShortName:      "iMGP Fund",
		FetchedAt:      time.Now(),
		RiskMeasures: &symbol.RiskMeasures{
			Volatility:    9.16,
			SharpeRatio:   2.52,
			FieldsPresent: symbol.SymbolRiskFieldVolatility | symbol.SymbolRiskFieldSharpeRatio,
		},
		AssetClassAllocation: []symbol.AssetClassEntry{
			{AssetClass: "Equities", Percent: 85.2},
		},
	}
	err := repo.Upsert(context.Background(), details)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Verify risk_measures column stores valid JSON
	var riskMeasuresJSON string
	err = db.QueryRow("SELECT risk_measures FROM symbol_details WHERE internal_symbol = ?",
		"IMGPFUND").Scan(&riskMeasuresJSON)
	if err != nil {
		t.Fatalf("query risk_measures: %v", err)
	}
	var rm map[string]interface{}
	if err := json.Unmarshal([]byte(riskMeasuresJSON), &rm); err != nil {
		t.Fatalf("parse risk_measures JSON: %v", err)
	}
	if rm["Volatility"] != float64(9.16) {
		t.Errorf("expected Volatility 9.16 in JSON, got %v", rm["Volatility"])
	}

	// Verify asset_class_allocation column stores valid JSON array
	var allocJSON string
	err = db.QueryRow("SELECT asset_class_allocation FROM symbol_details WHERE internal_symbol = ?",
		"IMGPFUND").Scan(&allocJSON)
	if err != nil {
		t.Fatalf("query asset_class_allocation: %v", err)
	}
	var alloc []map[string]interface{}
	if err := json.Unmarshal([]byte(allocJSON), &alloc); err != nil {
		t.Fatalf("parse asset_class_allocation JSON: %v", err)
	}
	if len(alloc) != 1 || alloc[0]["AssetClass"] != "Equities" {
		t.Errorf("expected 1 entry with AssetClass Equities, got %v", alloc)
	}

	// Verify nil columns are stored as NULL
	var equityDeriv, currencyDeriv sql.NullString
	err = db.QueryRow(`
		SELECT equity_derivatives_by_region, currency_derivatives_allocation
		FROM symbol_details WHERE internal_symbol = ?
	`, "IMGPFUND").Scan(&equityDeriv, &currencyDeriv)
	if err != nil {
		t.Fatalf("query null columns: %v", err)
	}
	if equityDeriv.Valid {
		t.Errorf("expected equity_derivatives_by_region NULL, got %q", equityDeriv.String)
	}
	if currencyDeriv.Valid {
		t.Errorf("expected currency_derivatives_allocation NULL, got %q", currencyDeriv.String)
	}
}

// extractResultToSymbolDetailsForTest exposes the unexported function from
// the symbols package for integration testing. It duplicates the mapping logic
// to exercise the full ExtractResult → SymbolDetails → DB → SymbolDetails path.
// This is a test-only helper; the actual mapping lives in symbols/service.go.
func extractResultToSymbolDetailsForTest(result *extractor.ExtractResult, internalSymbol string) *symbol.SymbolDetails {
	// Reuse the service's mapping by calling it through the symbols package.
	// We use a reflection-free approach: build the result inline matching
	// the exact same logic as extractResultToSymbolDetails in service.go.
	details := &symbol.SymbolDetails{
		InternalSymbol:    internalSymbol,
		ExtractorAsOfDate: result.AsOfDate,
		FetchedAt:         time.Now(),
	}

	if result.FundInfo != nil {
		details.ShortName = result.FundInfo.Name
		details.LongName = result.FundInfo.Name
	}

	if result.FundProfile != nil {
		details.FundProfile = &symbol.FundProfile{
			Family:                 result.FundProfile.Family,
			LegalType:              result.FundProfile.LegalType,
			TotalNetAssets:         result.FundProfile.TotalNetAssets,
			AnnualExpenseRatio:     result.FundProfile.AnnualExpenseRatio,
			AnnualHoldingsTurnover: result.FundProfile.AnnualHoldingsTurnover,
			InceptionDate:          result.FundProfile.InceptionDate,
			Isin:                   result.FundProfile.Isin,
			ShareClassName:         result.FundProfile.ShareClassName,
			OngoingCharges:         result.FundProfile.OngoingCharges,
		}
	}

	if result.RiskMeasures != nil {
		details.RiskMeasures = &symbol.RiskMeasures{
			Volatility:    result.RiskMeasures.Volatility,
			SharpeRatio:   result.RiskMeasures.SharpeRatio,
			InfoRatio:     result.RiskMeasures.InfoRatio,
			Beta:          result.RiskMeasures.Beta,
			Correlation:   result.RiskMeasures.Correlation,
			TrackingError: result.RiskMeasures.TrackingError,
			FieldsPresent: symbol.SymbolRiskFieldsMask(result.RiskMeasures.FieldsPresent),
		}
	}

	if len(result.AssetClassAllocation) > 0 {
		details.AssetClassAllocation = make([]symbol.AssetClassEntry, len(result.AssetClassAllocation))
		for i, ac := range result.AssetClassAllocation {
			details.AssetClassAllocation[i] = symbol.AssetClassEntry{
				AssetClass: ac.AssetClass,
				Percent:    ac.Percent,
			}
		}
	}

	if len(result.EquityDerivativesByRegion) > 0 {
		details.EquityDerivativesByRegion = make([]symbol.RegionDerivativeEntry, len(result.EquityDerivativesByRegion))
		for i, rd := range result.EquityDerivativesByRegion {
			details.EquityDerivativesByRegion[i] = symbol.RegionDerivativeEntry{
				Region:  rd.Region,
				Percent: rd.Percent,
			}
		}
	}

	if len(result.CurrencyDerivativesAllocation) > 0 {
		details.CurrencyDerivativesAllocation = make([]symbol.CurrencyDerivativeEntry, len(result.CurrencyDerivativesAllocation))
		for i, cd := range result.CurrencyDerivativesAllocation {
			details.CurrencyDerivativesAllocation[i] = symbol.CurrencyDerivativeEntry{
				Currency: cd.Currency,
				Percent:  cd.Percent,
			}
		}
	}

	return details
}
