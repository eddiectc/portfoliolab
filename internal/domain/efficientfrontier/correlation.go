package efficientfrontier

import (
	"math"
)

// ComputeCorrelationMatrix computes the N×N Pearson correlation matrix
// from an N×N covariance matrix (row-major).
//
// Formula: corr(i,j) = cov(i,j) / sqrt(var(i) * var(j))
// Returns the symmetric correlation matrix with 1.0 on the diagonal.
// If any diagonal element is zero, the corresponding row/column is set to 0.
func ComputeCorrelationMatrix(covMatrix [][]float64, n int) [][]float64 {
	corr := make([][]float64, n)
	for i := range corr {
		corr[i] = make([]float64, n)
	}

	// Precompute standard deviations (sqrt of diagonal).
	stdDev := make([]float64, n)
	for i := 0; i < n; i++ {
		stdDev[i] = math.Sqrt(covMatrix[i][i])
	}

	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			switch {
			case i == j:
				corr[i][j] = 1.0
			case stdDev[i] == 0 || stdDev[j] == 0:
				corr[i][j] = 0
			default:
				corr[i][j] = covMatrix[i][j] / (stdDev[i] * stdDev[j])
			}
			corr[j][i] = corr[i][j] // symmetric
		}
	}

	return corr
}
