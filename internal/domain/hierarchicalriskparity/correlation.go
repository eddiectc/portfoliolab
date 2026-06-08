package hierarchicalriskparity

import (
	"codeberg.org/eddiectc/portfoliolab/internal/domain/stats"
)

// ComputeCorrelationMatrix computes the N×N Pearson correlation matrix from
// an aligned returns matrix. Each row of aligned is one trading date; each
// column is one symbol.
//
// The matrix is symmetric with 1.0 on the diagonal.
// If any symbol has zero variance, its row/column is set to 0 (except diagonal = 1).
func ComputeCorrelationMatrix(aligned [][]float64) [][]float64 {
	n := len(aligned[0]) // number of symbols
	corr := make([][]float64, n)
	for i := range corr {
		corr[i] = make([]float64, n)
		corr[i][i] = 1.0
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			r, _ := stats.PearsonCorrelation(alignedCol(aligned, i), alignedCol(aligned, j))
			corr[i][j] = r
			corr[j][i] = r
		}
	}

	return corr
}

// alignedCol extracts a single column from the aligned returns matrix.
func alignedCol(aligned [][]float64, col int) []float64 {
	n := len(aligned)
	result := make([]float64, n)
	for i := 0; i < n; i++ {
		result[i] = aligned[i][col]
	}
	return result
}
