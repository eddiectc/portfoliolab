package analysis

import (
	"github.com/eddiectc/portfoliolab/internal/domain/correlation"
	"github.com/eddiectc/portfoliolab/internal/market"
)

// ComputeCorrelation computes the pairwise Pearson correlation matrix from
// historical price series.
//
// Delegates to the shared correlation package and converts the result
// to the analysis-domain CorrelationResult type.
func ComputeCorrelation(prices map[string][]market.HistoricalPrice, period string) *CorrelationResult {
	result := correlation.Compute(correlation.Input{
		Prices: prices,
		Period: period,
	})

	return &CorrelationResult{
		Matrix:   result.Matrix,
		Symbols:  result.Symbols,
		Period:   result.Period,
		Warnings: result.Warnings,
		Message:  result.Message,
	}
}
