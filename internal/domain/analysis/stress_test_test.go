package analysis

import (
	"strconv"
	"strings"
	"testing"

	"github.com/govalues/decimal"
)

// --- ComputeStressTests tests ---

func TestComputeStressTests_HappyPath(t *testing.T) {
	// Portfolio: 50% Information Technology, 30% Financials, 20% Health Care
	// Use a known scenario to verify computation.
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Information Technology": 50.0,
			"Financials":             30.0,
			"Health Care":            20.0,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(allocation, portfolioValue)

	if result.Message != "" {
		t.Errorf("unexpected message: %q", result.Message)
	}
	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if len(result.Scenarios) != len(PredefinedScenarios) {
		t.Fatalf("expected %d scenarios, got %d", len(PredefinedScenarios), len(result.Scenarios))
	}

	// Verify sorted by severity (most negative first).
	for i := 1; i < len(result.Scenarios); i++ {
		if result.Scenarios[i].EstimatedReturnPct < result.Scenarios[i-1].EstimatedReturnPct {
			t.Errorf("scenarios not sorted by severity: scenario %d (%.2f) < scenario %d (%.2f)",
				i, result.Scenarios[i].EstimatedReturnPct, i-1, result.Scenarios[i-1].EstimatedReturnPct)
		}
	}

	// Verify first scenario has expected structure.
	first := result.Scenarios[0]
	if first.Name == "" {
		t.Error("scenario name is empty")
	}
	if first.DateRange == "" {
		t.Error("scenario date_range is empty")
	}
	if len(first.SectorContributions) == 0 {
		t.Error("sector contributions is empty")
	}

	// Verify dollar impact is computed correctly.
	// For a $100k portfolio, a -30% return should be ~-$30k.
	// (Exact value depends on which scenario is first.)
	expectedImpact, _ := portfolioValue.Mul(decimal.MustParse(strconvFixedPct(first.EstimatedReturnPct)))
	expectedImpact, _ = expectedImpact.Quo(decimal.MustNew(100, 0))
	if !first.EstimatedDollarImpact.Equal(expectedImpact) {
		t.Errorf("dollar impact = %s, want %s", first.EstimatedDollarImpact.String(), expectedImpact.String())
	}
}

func TestComputeStressTests_SingleSector(t *testing.T) {
	// 100% in one sector — impact should equal that sector's return for each scenario.
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Energy": 100.0,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("50000")

	result := ComputeStressTests(allocation, portfolioValue)

	if len(result.Scenarios) != len(PredefinedScenarios) {
		t.Fatalf("expected %d scenarios, got %d", len(PredefinedScenarios), len(result.Scenarios))
	}

	// Find the 2020 COVID scenario (Energy had -35% return).
	for _, s := range result.Scenarios {
		if s.Name == "2020 COVID Crash" {
			if s.EstimatedReturnPct != -35.0 {
				t.Errorf("2020 COVID return = %.2f, want -35.0", s.EstimatedReturnPct)
			}
			// $50k × -35% / 100 = -$17,500
			wantImpact := decimal.MustParse("-17500")
			if !s.EstimatedDollarImpact.Equal(wantImpact) {
				t.Errorf("dollar impact = %s, want %s", s.EstimatedDollarImpact.String(), wantImpact.String())
			}
			return
		}
	}
	t.Error("2020 COVID Crash scenario not found")
}

func TestComputeStressTests_EqualWeight(t *testing.T) {
	// Equal weight across 3 sectors (using GICS names matching the JSON data).
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Information Technology": 33.33,
			"Health Care":            33.33,
			"Energy":                 33.34,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(allocation, portfolioValue)

	if len(result.Scenarios) != len(PredefinedScenarios) {
		t.Fatalf("expected %d scenarios, got %d", len(PredefinedScenarios), len(result.Scenarios))
	}

	// Each scenario should have contributions from all 3 sectors.
	for _, s := range result.Scenarios {
		if len(s.SectorContributions) != 3 {
			t.Errorf("scenario %q has %d sector contributions, want 3", s.Name, len(s.SectorContributions))
		}
	}
}

func TestComputeStressTests_UnknownSectorWeight(t *testing.T) {
	// Some weight in unknown sector — should generate a warning and exclude it.
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Technology":  60.0,
			"Health Care": 30.0,
		},
		UnknownWeightPct: 10.0,
	}
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(allocation, portfolioValue)

	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "10.00%") {
		t.Errorf("warning should mention 10.00%% unknown weight: %q", result.Warnings[0])
	}

	// The calculation should only use the known sectors (60% + 30% = 90% total).
	// Returns should be proportionally lower than if 100% was known.
}

func TestComputeStressTests_ZeroPortfolioValue(t *testing.T) {
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Technology":  50.0,
			"Health Care": 50.0,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("0")

	result := ComputeStressTests(allocation, portfolioValue)

	if len(result.Scenarios) != len(PredefinedScenarios) {
		t.Fatalf("expected %d scenarios, got %d", len(PredefinedScenarios), len(result.Scenarios))
	}

	// All dollar impacts should be 0.
	for _, s := range result.Scenarios {
		if !s.EstimatedDollarImpact.Equal(decimal.MustParse("0")) {
			t.Errorf("scenario %q dollar impact = %s, want 0", s.Name, s.EstimatedDollarImpact.String())
		}
	}
}

func TestComputeStressTests_NegativePortfolioValue(t *testing.T) {
	// Edge case: negative portfolio value (e.g., short positions net negative).
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Information Technology": 100.0,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("-50000")

	result := ComputeStressTests(allocation, portfolioValue)

	// Dollar impact should be negative of what it would be for positive value.
	// A -78% return on -$50k = +$39k (inverse relationship).
	for _, s := range result.Scenarios {
		if s.Name == "2000 Dot-Com Bubble" {
			// Information Technology had -78% return in 2000.
			// -$50k × -78% / 100 = +$39k
			wantImpact := decimal.MustParse("39000")
			if !s.EstimatedDollarImpact.Equal(wantImpact) {
				t.Errorf("dollar impact = %s, want %s", s.EstimatedDollarImpact.String(), wantImpact.String())
			}
			return
		}
	}
}

func TestComputeStressTests_EmptyAllocation(t *testing.T) {
	allocation := &AllocationResult{
		Breakdown:        map[string]float64{},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(allocation, portfolioValue)

	if result.Message == "" {
		t.Error("expected empty-state message")
	}
	if len(result.Scenarios) != 0 {
		t.Errorf("expected no scenarios, got %d", len(result.Scenarios))
	}
}

func TestComputeStressTests_NilAllocation(t *testing.T) {
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(nil, portfolioValue)

	if result.Message == "" {
		t.Error("expected empty-state message")
	}
	if len(result.Scenarios) != 0 {
		t.Errorf("expected no scenarios, got %d", len(result.Scenarios))
	}
}

func TestComputeStressTests_SectorNotInScenario(t *testing.T) {
	// Use a sector name that doesn't appear in any scenario's data.
	// The sector should be skipped without error.
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Information Technology": 50.0,
			"Nonexistent Sector":     50.0,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(allocation, portfolioValue)

	if len(result.Scenarios) != len(PredefinedScenarios) {
		t.Fatalf("expected %d scenarios, got %d", len(PredefinedScenarios), len(result.Scenarios))
	}

	// Each scenario should only have contributions from Information Technology (the valid sector).
	for _, s := range result.Scenarios {
		if _, ok := s.SectorContributions["Nonexistent Sector"]; ok {
			t.Errorf("scenario %q should not include 'Nonexistent Sector'", s.Name)
		}
		if _, ok := s.SectorContributions["Information Technology"]; !ok {
			t.Errorf("scenario %q should include 'Information Technology'", s.Name)
		}
	}
}

func TestComputeStressTests_SortedBySeverity(t *testing.T) {
	// Verify that results are sorted most-negative return first.
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Technology":       25.0,
			"Financials":       25.0,
			"Health Care":      25.0,
			"Consumer Staples": 25.0,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(allocation, portfolioValue)

	for i := 1; i < len(result.Scenarios); i++ {
		if result.Scenarios[i].EstimatedReturnPct < result.Scenarios[i-1].EstimatedReturnPct {
			t.Errorf("not sorted: scenario %d (%.2f) < scenario %d (%.2f)",
				i, result.Scenarios[i].EstimatedReturnPct, i-1, result.Scenarios[i-1].EstimatedReturnPct)
		}
	}
}

func TestComputeStressTests_MixedPositiveNegativeReturns(t *testing.T) {
	// 2022 scenario has Energy at +10%. A 100% Energy portfolio should show positive return.
	allocation := &AllocationResult{
		Breakdown: map[string]float64{
			"Energy": 100.0,
		},
		UnknownWeightPct: 0,
	}
	portfolioValue := decimal.MustParse("100000")

	result := ComputeStressTests(allocation, portfolioValue)

	for _, s := range result.Scenarios {
		if s.Name == "2022 Market Decline" {
			// Energy had +10% in 2022.
			if s.EstimatedReturnPct != 10.0 {
				t.Errorf("2022 return = %.2f, want 10.0", s.EstimatedReturnPct)
			}
			// $100k × 10% / 100 = $10k
			wantImpact := decimal.MustParse("10000")
			if !s.EstimatedDollarImpact.Equal(wantImpact) {
				t.Errorf("dollar impact = %s, want %s", s.EstimatedDollarImpact.String(), wantImpact.String())
			}
			return
		}
	}
	t.Error("2022 Market Decline scenario not found")
}

// --- LoadPredefinedScenarios tests ---

func TestLoadPredefinedScenarios(t *testing.T) {
	scenarios, err := LoadPredefinedScenarios()
	if err != nil {
		t.Fatalf("LoadPredefinedScenarios error: %v", err)
	}

	if len(scenarios) != 6 {
		t.Fatalf("expected 6 scenarios, got %d", len(scenarios))
	}

	// Verify each scenario has required fields.
	for _, s := range scenarios {
		if s.Name == "" {
			t.Error("scenario has empty name")
		}
		if s.DateRange == "" {
			t.Errorf("scenario %q has empty date_range", s.Name)
		}
		if len(s.SectorReturns) == 0 {
			t.Errorf("scenario %q has no sector returns", s.Name)
		}
	}
}

func TestPredefinedScenarios_Init(t *testing.T) {
	// Verify that PredefinedScenarios was populated at init time.
	if len(PredefinedScenarios) == 0 {
		t.Error("PredefinedScenarios is empty — init() may have failed")
	}
	if len(PredefinedScenarios) != 6 {
		t.Errorf("PredefinedScenarios has %d entries, want 6", len(PredefinedScenarios))
	}
}

// Helper: strconvFixedPct formats a float64 percentage for decimal parsing.
func strconvFixedPct(v float64) string {
	return strconv.FormatFloat(roundTo2(v), 'f', 2, 64)
}
