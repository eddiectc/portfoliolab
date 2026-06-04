package efficientfrontier

import (
	"math"
)

// ComputeMinVariance computes the analytical global minimum variance portfolio
// weights using the closed-form solution:
//
//	w = Σ⁻¹·1 / (1'·Σ⁻¹·1)
//
// where Σ is the covariance matrix and 1 is a vector of ones.
// Returns the weight vector (fractions summing to 1.0) or nil if the
// covariance matrix is singular.
//
// The matrix is assumed to be symmetric positive semi-definite.
// A small regularization epsilon is added to the diagonal if the matrix
// is near-singular.
func ComputeMinVariance(covMatrix [][]float64, n int) []float64 {
	if n < 2 {
		return nil
	}

	// Try inversion with regularization fallback.
	inv, err := inverseSymmetric(covMatrix, n)
	if err != nil {
		// Try with regularization.
		regularized := regularize(covMatrix, n)
		inv, err = inverseSymmetric(regularized, n)
		if err != nil {
			return nil // singular even with regularization
		}
	}

	// Compute Σ⁻¹·1 (sum of each row of the inverse).
	sigmaInv1 := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := 0.0
		for j := 0; j < n; j++ {
			sum += inv[i][j]
		}
		sigmaInv1[i] = sum
	}

	// Compute 1'·Σ⁻¹·1 (sum of sigmaInv1).
	denom := 0.0
	for _, v := range sigmaInv1 {
		denom += v
	}

	if denom == 0 || math.IsNaN(denom) || math.IsInf(denom, 0) {
		return nil
	}

	// Normalize: w = Σ⁻¹·1 / (1'·Σ⁻¹·1).
	weights := make([]float64, n)
	for i := 0; i < n; i++ {
		weights[i] = sigmaInv1[i] / denom
	}

	// Clamp negative weights to zero and renormalize (numerical artifacts).
	// The analytical solution can produce tiny negative weights due to
	// floating point precision.
	totalNeg := 0.0
	for i, w := range weights {
		if w < 0 {
			totalNeg += w
			weights[i] = 0
		}
	}
	if totalNeg < 0 {
		sum := 0.0
		for _, w := range weights {
			sum += w
		}
		if sum > 0 {
			for i := range weights {
				weights[i] /= sum
			}
		}
	}

	return weights
}

// regularize adds a small epsilon to the diagonal of the covariance matrix
// to improve numerical stability.
func regularize(covMatrix [][]float64, n int) [][]float64 {
	result := make([][]float64, n)
	for i := 0; i < n; i++ {
		result[i] = make([]float64, n)
		copy(result[i], covMatrix[i])
	}

	// Epsilon is 1e-6 times the average diagonal element.
	var avgDiag float64
	for i := 0; i < n; i++ {
		avgDiag += covMatrix[i][i]
	}
	avgDiag /= float64(n)
	epsilon := avgDiag * 1e-6

	for i := 0; i < n; i++ {
		result[i][i] += epsilon
	}
	return result
}

// inverseSymmetric computes the inverse of a symmetric matrix using
// Gaussian elimination with partial pivoting. Returns an error if the
// matrix is singular.
func inverseSymmetric(matrix [][]float64, n int) ([][]float64, error) {
	// Augment matrix with identity: [A | I]
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, 2*n)
		copy(aug[i], matrix[i])
		aug[i][n+i] = 1.0
	}

	// Forward elimination with partial pivoting.
	for col := 0; col < n; col++ {
		// Find pivot.
		maxVal := math.Abs(aug[col][col])
		maxRow := col
		for row := col + 1; row < n; row++ {
			if math.Abs(aug[row][col]) > maxVal {
				maxVal = math.Abs(aug[row][col])
				maxRow = row
			}
		}

		// Check for singularity.
		if maxVal < 1e-12 {
			return nil, ErrSingularMatrix
		}

		// Swap rows.
		if maxRow != col {
			aug[col], aug[maxRow] = aug[maxRow], aug[col]
		}

		// Scale pivot row.
		pivot := aug[col][col]
		for j := 0; j < 2*n; j++ {
			aug[col][j] /= pivot
		}

		// Eliminate column in other rows.
		for row := 0; row < n; row++ {
			if row == col {
				continue
			}
			factor := aug[row][col]
			for j := 0; j < 2*n; j++ {
				aug[row][j] -= factor * aug[col][j]
			}
		}
	}

	// Extract inverse from right half.
	inv := make([][]float64, n)
	for i := 0; i < n; i++ {
		inv[i] = make([]float64, n)
		copy(inv[i], aug[i][n:])
	}

	return inv, nil
}
