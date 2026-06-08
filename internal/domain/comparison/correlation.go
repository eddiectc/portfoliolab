package comparison

import (
	"codeberg.org/eddiectc/portfoliolab/internal/domain/correlation"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// IntraPortfolioCorrelationInput holds the data needed to compute the
// correlation matrix for symbols within a single portfolio.
type IntraPortfolioCorrelationInput struct {
	// Prices maps each symbol to its historical price series (sorted ASC).
	Prices map[string][]market.HistoricalPrice
	// Period is the lookback period for correlation (e.g. "1Y", "3Y", "5Y").
	// Supported: "3M", "6M", "1Y", "3Y", "5Y", "10Y". Defaults to "1Y".
	Period string
}

// IntraPortfolioCorrelationResult holds the correlation matrix for symbols
// within a portfolio. Matrix is N×N where N = len(Symbols). Matrix[i][j]
// is a pointer to the correlation between Symbols[i] and Symbols[j], or nil
// when there is insufficient overlapping data.
type IntraPortfolioCorrelationResult struct {
	Matrix   [][]*float64 `json:"matrix,omitempty"`
	Symbols  []string     `json:"symbols"`
	Period   string       `json:"period"`
	Warnings []string     `json:"warnings,omitempty"`
	Message  string       `json:"message,omitempty"`
}

// ComputeIntraPortfolioCorrelation computes the pairwise Pearson correlation
// matrix from historical price series for the symbols in a portfolio.
//
// Delegates to the shared correlation package and converts the result
// to the comparison-domain IntraPortfolioCorrelationResult type.
func ComputeIntraPortfolioCorrelation(input IntraPortfolioCorrelationInput) *IntraPortfolioCorrelationResult {
	result := correlation.Compute(correlation.Input{
		Prices: input.Prices,
		Period: input.Period,
	})

	return &IntraPortfolioCorrelationResult{
		Matrix:   result.Matrix,
		Symbols:  result.Symbols,
		Period:   result.Period,
		Warnings: result.Warnings,
		Message:  result.Message,
	}
}
