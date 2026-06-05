// Package stats provides shared statistical and financial computation functions
// used across the application. All functions in this package operate on float64
// slices and use standard conventions:
//
//   - Returns and volatility are expressed as ratios (e.g. 0.15 = 15%), not percentages
//   - Annualization uses 252 trading days per year
//   - Sample statistics (n-1 denominator) are preferred over population statistics
//
// # Shared Functions — Use These, Don't Reinvent
//
// Both the comparison/performance and efficient-frontier modules must use these
// shared functions for financial metrics. Do NOT create new calculation functions
// in domain-specific packages.
//
// Return & Volatility:
//   - AnnualizedReturn(dailyReturns)     — arithmetic mean × 252, ratio output
//   - AnnualizedVolatility(dailyReturns) — sample stddev × sqrt(252), ratio output
//
// Risk-Adjusted Metrics:
//   - SharpeRatio(ret, vol, riskFree)    — all inputs as annualized ratios
//   - SortinoRatio(ret, volDown, rf)     — downside deviation variant
//
// Portfolio Math:
//   - PortfolioReturn(expectedReturns, weights) — weighted sum w'μ
//   - PortfolioVolatility(covMatrix, weights)   — sqrt(w'Σw)
//   - CovarianceMatrix(alignedReturns)          — sample cov, annualized
//
// Correlation & Alignment:
//   - PearsonCorrelation(x, y)          — correlation coefficient
//   - AlignSeries(mapA, mapB)           — align two key→value series
//
// Rounding:
//   - RoundTo2(v) / RoundTo4(v)         — decimal rounding helpers
package stats

import (
	"math"
)

const (
	// TradingDaysPerYear is the standard number of trading days used for
	// annualizing returns and volatility.
	TradingDaysPerYear = 252
)

// AnnualizedReturn computes the annualized expected return from a series of
// daily returns expressed as ratios (e.g. 0.01 = 1%).
//
// Formula: mean(daily returns) × 252
// Result is returned as a ratio (e.g. 0.15 = 15% annualized).
// Returns 0 when fewer than 2 observations.
func AnnualizedReturn(dailyReturns []float64) float64 {
	n := len(dailyReturns)
	if n < 2 {
		return 0
	}
	sum := 0.0
	for _, r := range dailyReturns {
		sum += r
	}
	return (sum / float64(n)) * float64(TradingDaysPerYear)
}

// AnnualizedVolatility computes the annualized volatility from a series of
// daily returns expressed as ratios (e.g. 0.01 = 1%).
//
// Formula: sampleStdDev(daily returns) × sqrt(252)
// Uses sample standard deviation (n-1 denominator).
// Result is returned as a ratio (e.g. 0.15 = 15% annualized).
// Returns 0 when fewer than 2 observations.
func AnnualizedVolatility(dailyReturns []float64) float64 {
	n := len(dailyReturns)
	if n < 2 {
		return 0
	}

	// Compute mean.
	sum := 0.0
	for _, r := range dailyReturns {
		sum += r
	}
	mean := sum / float64(n)

	// Compute sample variance (n-1).
	varianceSum := 0.0
	for _, r := range dailyReturns {
		diff := r - mean
		varianceSum += diff * diff
	}
	variance := varianceSum / float64(n-1)

	// Annualize: daily stddev * sqrt(252).
	return math.Sqrt(variance) * math.Sqrt(float64(TradingDaysPerYear))
}

// SharpeRatio computes the Sharpe ratio from annualized return, annualized
// volatility, and annualized risk-free rate, all expressed as ratios
// (e.g. 0.15 = 15%, 0.045 = 4.5%).
//
// Formula: (return - riskFree) / volatility
// Returns 0 when volatility is zero or negative.
func SharpeRatio(annualizedReturn, annualizedVol, riskFreeRate float64) float64 {
	if annualizedVol <= 0 {
		return 0
	}
	return (annualizedReturn - riskFreeRate) / annualizedVol
}

// SortinoRatio computes the Sortino ratio from annualized return, annualized
// downside deviation, and annualized risk-free rate, all expressed as ratios.
//
// Formula: (return - riskFree) / downsideDeviation
// Returns 0 when downside deviation is zero or negative.
func SortinoRatio(annualizedReturn, downsideDev, riskFreeRate float64) float64 {
	if downsideDev <= 0 {
		return 0
	}
	return (annualizedReturn - riskFreeRate) / downsideDev
}

// DownsideDeviation computes the annualized downside deviation from daily
// returns (ratios) relative to a daily risk-free rate (ratio).
//
// Formula: sqrt(mean((min(r - rf, 0))^2)) × sqrt(252)
// Only negative excess returns (below risk-free) contribute.
func DownsideDeviation(dailyReturns []float64, dailyRiskFree float64) float64 {
	n := len(dailyReturns)
	if n < 2 {
		return 0
	}

	negSum := 0.0
	for _, r := range dailyReturns {
		if r < dailyRiskFree {
			diff := dailyRiskFree - r
			negSum += diff * diff
		}
	}

	// Use n (not n-1) for downside deviation — standard convention.
	return math.Sqrt(negSum/float64(n)) * math.Sqrt(float64(TradingDaysPerYear))
}

// PortfolioReturn computes the expected return of a portfolio given
// annualized expected returns (ratios) and weights (fractions summing to 1).
// Result is returned as a ratio.
func PortfolioReturn(expectedReturns []float64, weights []float64) float64 {
	ret := 0.0
	for i, w := range weights {
		ret += w * expectedReturns[i]
	}
	return ret
}

// PortfolioVolatility computes the annualized volatility of a portfolio
// given an annualized covariance matrix (ratios) and weights (fractions
// summing to 1).
// Result is returned as a ratio.
func PortfolioVolatility(covMatrix [][]float64, weights []float64) float64 {
	n := len(weights)
	variance := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			variance += weights[i] * weights[j] * covMatrix[i][j]
		}
	}
	return math.Sqrt(variance)
}

// AnnualizedCovarianceMatrix computes the sample covariance matrix from a set
// of aligned daily returns (ratios). Each row is one observation date,
// each column is one asset.
//
// The matrix is annualized: daily covariance × 252.
// Returns the N×N symmetric matrix (row-major) where N is the number of assets.
func AnnualizedCovarianceMatrix(aligned [][]float64) [][]float64 {
	if len(aligned) < 2 {
		return nil
	}
	n := len(aligned[0]) // number of assets
	m := len(aligned)    // number of observations

	cov := make([][]float64, n)
	for i := range cov {
		cov[i] = make([]float64, n)
	}

	// Compute means per asset.
	means := make([]float64, n)
	for k := 0; k < m; k++ {
		for i := 0; i < n; i++ {
			means[i] += aligned[k][i]
		}
	}
	for i := 0; i < n; i++ {
		means[i] /= float64(m)
	}

	// Compute sample covariance (n-1) and annualize by 252.
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			sum := 0.0
			for k := 0; k < m; k++ {
				di := aligned[k][i] - means[i]
				dj := aligned[k][j] - means[j]
				sum += di * dj
			}
			annualizedCov := (sum / float64(m-1)) * float64(TradingDaysPerYear)
			cov[i][j] = annualizedCov
			cov[j][i] = annualizedCov // symmetric
		}
	}

	return cov
}

// PearsonCorrelation computes the Pearson correlation coefficient of two
// equally-lengthed float64 series. Returns the coefficient and sample count.
// If either series has zero variance, returns 0.
func PearsonCorrelation(x, y []float64) (float64, int) {
	n := len(x)
	if n == 0 || n != len(y) {
		return 0, 0
	}

	// Compute means.
	sumX, sumY := 0.0, 0.0
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	// Compute numerator and denominators.
	num := 0.0
	sumDx2 := 0.0
	sumDy2 := 0.0
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		num += dx * dy
		sumDx2 += dx * dx
		sumDy2 += dy * dy
	}

	if sumDx2 == 0 || sumDy2 == 0 {
		return 0, n
	}

	return num / math.Sqrt(sumDx2*sumDy2), n
}

// AlignSeries takes two key→value maps and produces aligned float64 slices
// (matching keys in the same order) plus the overlap count.
// x always corresponds to map a, y to map b.
// Keys are compared as strings; the caller is responsible for providing
// consistent key formatting (e.g. date strings, timestamps).
func AlignSeries(a, b map[string]float64) ([]float64, []float64, int) {
	// Build a walk order from the smaller map for lookup efficiency.
	var mapFromA bool // true if the lookup map is a
	var lookup map[string]float64
	var walk map[string]float64

	if len(a) <= len(b) {
		mapFromA = true
		lookup = a
		walk = b
	} else {
		mapFromA = false
		lookup = b
		walk = a
	}

	x := make([]float64, 0, len(lookup))
	y := make([]float64, 0, len(lookup))

	for key, walkVal := range walk {
		mapped, ok := lookup[key]
		if !ok {
			continue
		}
		if mapFromA {
			// walk is b, lookup is a: mapped=a, walkVal=b
			x = append(x, mapped)
			y = append(y, walkVal)
		} else {
			// walk is a, lookup is b: walkVal=a, mapped=b
			x = append(x, walkVal)
			y = append(y, mapped)
		}
	}

	return x, y, len(x)
}

// RoundTo2 rounds a float64 to 2 decimal places.
// Normalizes -0 to 0 to avoid JSON serializing as -0.
func RoundTo2(v float64) float64 {
	result := math.Round(v*100) / 100
	if result == 0 {
		return 0 // normalize -0 to 0
	}
	return result
}

// RoundTo4 rounds a float64 to 4 decimal places.
func RoundTo4(v float64) float64 {
	return math.Round(v*10000) / 10000
}


