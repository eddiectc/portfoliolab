package analysis

import (
	"sort"
	"strconv"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
	"github.com/govalues/decimal"
)

// ComputeStressTests estimates the portfolio impact for each predefined
// historical crisis scenario.
//
// For each scenario, the estimated portfolio return is computed as the sum of
// (sector return × portfolio weight in that sector). The dollar impact is
// the portfolio value multiplied by the estimated return.
//
// Sector contributions are tracked per scenario to show which sectors drive
// the estimated impact. Results are sorted by severity (most negative first).
//
// Unknown sector weight (from AllocationResult.UnknownWeightPct) is excluded
// from the calculation and noted in a warning.
func ComputeStressTests(sectorAllocation *AllocationResult, portfolioValue decimal.Decimal) *StressTestResult {
	if sectorAllocation == nil || len(sectorAllocation.Breakdown) == 0 {
		return &StressTestResult{
			Scenarios: []StressScenarioResult{},
			Message:   "No sector allocation data available. Stress testing requires sector allocation data.",
		}
	}

	var scenarios []StressScenarioResult
	var warnings []string

	if sectorAllocation.UnknownWeightPct > 0 {
		warnings = append(warnings,
			"Unknown sector weight "+formatPct(sectorAllocation.UnknownWeightPct)+" excluded from stress test calculation")
	}

	for _, scenario := range PredefinedScenarios {
		result := computeScenarioImpact(scenario, sectorAllocation.Breakdown, portfolioValue)
		scenarios = append(scenarios, result)
	}

	// Sort by estimated return (most negative first).
	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].EstimatedReturnPct < scenarios[j].EstimatedReturnPct
	})

	return &StressTestResult{
		Scenarios: scenarios,
		Warnings:  warnings,
	}
}

// computeScenarioImpact estimates the portfolio impact for a single scenario.
func computeScenarioImpact(scenario StressScenario, breakdown map[string]float64, portfolioValue decimal.Decimal) StressScenarioResult {
	var estimatedReturn float64
	sectorContributions := make(map[string]float64)

	for sector, weight := range breakdown {
		sectorReturn, ok := scenario.SectorReturns[sector]
		if !ok {
			// Sector not in this scenario's data — skip it.
			continue
		}
		contribution := weight * sectorReturn / 100.0
		estimatedReturn += contribution
		sectorContributions[sector] = stats.RoundTo2(contribution)
	}

	estimatedReturn = stats.RoundTo2(estimatedReturn)

	// Dollar impact: portfolio value × estimated return / 100.
	returnDecimal := decimal.MustParse(strconv.FormatFloat(estimatedReturn, 'f', 2, 64))
	dollarImpact, _ := portfolioValue.Mul(returnDecimal)
	dollarImpact, _ = dollarImpact.Quo(decimal.MustNew(100, 0))

	return StressScenarioResult{
		Name:                  scenario.Name,
		DateRange:             scenario.DateRange,
		EstimatedReturnPct:    estimatedReturn,
		EstimatedDollarImpact: dollarImpact,
		SectorContributions:   sectorContributions,
	}
}

// formatPct formats a percentage value for display in warnings.
func formatPct(v float64) string {
	return strconv.FormatFloat(stats.RoundTo2(v), 'f', 2, 64) + "%"
}
