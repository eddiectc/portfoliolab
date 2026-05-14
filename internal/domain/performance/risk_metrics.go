package performance

import (
	"math"

	"github.com/govalues/decimal"
)

// RiskMetrics holds risk and risk-adjusted return statistics derived from
// daily returns. AnnualizedVolatilityPct is the annualized standard deviation
// of daily returns, expressed as a percentage (e.g. 15.50 = 15.50%). Nil when
// fewer than 2 data points are available.
// SharpeRatio is the annualized Sharpe ratio: (annualizedReturn - riskFreeRate) /
// annualizedVolatility. Nil when risk-free rate is not provided, volatility is
// zero, or fewer than 2 data points. Expressed as a plain ratio (not percentage).
// SortinoRatio is the annualized Sortino ratio: (annualizedReturn - riskFreeRate) /
// annualizedDownsideDeviation. Nil when risk-free rate is not provided, downside
// deviation is zero, or fewer than 2 data points. Expressed as a plain ratio.
type RiskMetrics struct {
	AnnualizedVolatilityPct *decimal.Decimal `json:"annualized_volatility_pct,omitempty"`
	SharpeRatio             *decimal.Decimal `json:"sharpe_ratio,omitempty"`
	SortinoRatio            *decimal.Decimal `json:"sortino_ratio,omitempty"`
}

// ComputeRiskMetrics computes volatility, Sharpe ratio, and Sortino ratio from
// a sequence of daily returns.
//
// Annualized volatility = std-dev of daily returns × sqrt(252).
//
// Sharpe ratio = (mean daily return × 252 - riskFreeRate) / annualizedVol.
// Nil when riskFreeRatePct is nil (no risk-free rate available) or volatility
// is zero.
//
// Sortino ratio = (mean daily return × 252 - riskFreeRate) / downsideDev,
// where downsideDev = sqrt(mean of squared negative returns) × sqrt(252).
// Nil when riskFreeRatePct is nil or downside deviation is zero.
//
// Returns nil fields when fewer than 2 data points are available.
func ComputeRiskMetrics(dailyReturns []DailyReturn, riskFreeRatePct *decimal.Decimal) RiskMetrics {
	if len(dailyReturns) < 2 {
		return RiskMetrics{}
	}

	// Convert return percentages to ratios for computation.
	n := float64(len(dailyReturns))
	var returns []float64
	var sum float64

	for _, dr := range dailyReturns {
		rF, _ := dr.ReturnPct.Float64()
		ratio := rF / 100.0 // percentage → ratio
		returns = append(returns, ratio)
		sum += ratio
	}

	mean := sum / n

	// --- Annualized volatility ---
	var varianceSum float64
	for _, r := range returns {
		diff := r - mean
		varianceSum += diff * diff
	}
	stdDev := math.Sqrt(varianceSum / n)
	annualizedVol := stdDev * math.Sqrt(252.0)

	annualizedVolPct, _ := decimal.NewFromFloat64(annualizedVol * 100.0)
	annualizedVolPct = annualizedVolPct.Round(2)

	metrics := RiskMetrics{
		AnnualizedVolatilityPct: ptrDec(annualizedVolPct),
	}

	// --- Sharpe and Sortino (need risk-free rate) ---
	if riskFreeRatePct == nil {
		return metrics
	}

	riskFreeF, _ := riskFreeRatePct.Float64()
	riskFreeRatio := riskFreeF / 100.0 // annual percentage → annual ratio

	annualizedReturn := mean * 252.0
	excessReturn := annualizedReturn - riskFreeRatio

	// Daily risk-free rate for downside deviation comparison.
	dailyRiskFree := riskFreeRatio / 252.0

	// --- Sharpe ratio ---
	if annualizedVol > 0 {
		sharpe := excessReturn / annualizedVol
		sharpeDec, _ := decimal.NewFromFloat64(sharpe)
		sharpeDec = sharpeDec.Round(4)
		metrics.SharpeRatio = ptrDec(sharpeDec)
	}

	// --- Sortino ratio ---
	var negSum float64
	negCount := 0.0
	for _, r := range returns {
		if r < dailyRiskFree {
			diff := dailyRiskFree - r
			negSum += diff * diff
			negCount++
		}
	}

	if negCount > 0 {
		downsideDev := math.Sqrt(negSum/n) * math.Sqrt(252.0)
		if downsideDev > 0 {
			sortino := excessReturn / downsideDev
			sortinoDec, _ := decimal.NewFromFloat64(sortino)
			sortinoDec = sortinoDec.Round(4)
			metrics.SortinoRatio = ptrDec(sortinoDec)
		}
	}

	return metrics
}
