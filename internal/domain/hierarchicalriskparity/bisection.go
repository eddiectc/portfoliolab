package hierarchicalriskparity

import "math"

// findBisectionPoint finds the split point in the sorted indices that
// minimizes the cross-group covariance. The split divides indices into
// indices[0:split] and indices[split:len].
//
// Cross-group covariance is the sum of all pairwise covariances between
// assets in group 1 and assets in group 2.
func findBisectionPoint(indices []int, covMatrix [][]float64) int {
	n := len(indices)
	if n < 2 {
		return 1
	}

	bestSplit := 1
	bestCrossCov := math.Inf(1)

	for split := 1; split < n; split++ {
		crossCov := 0.0
		for i := 0; i < split; i++ {
			for j := split; j < n; j++ {
				crossCov += covMatrix[indices[i]][indices[j]]
			}
		}
		if crossCov < bestCrossCov {
			bestCrossCov = crossCov
			bestSplit = split
		}
	}

	return bestSplit
}

// minVarianceWeights computes the analytical minimum variance weights for
// a subset of assets identified by indices. Uses the covariance matrix
// with matrix inversion (same approach as efficient frontier).
//
// Returns weights indexed by position in the indices slice (not by original
// symbol index). Weights sum to 1.0 and are non-negative.
func minVarianceWeights(indices []int, covMatrix [][]float64) []float64 {
	n := len(indices)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return []float64{1.0}
	}

	// Extract sub-covariance matrix.
	subCov := make([][]float64, n)
	for i := 0; i < n; i++ {
		subCov[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			subCov[i][j] = covMatrix[indices[i]][indices[j]]
		}
	}

	// Try inversion; fall back to regularization then equal weights.
	weights := minVarianceFromInverse(subCov, n)
	if weights == nil {
		regularized := regularizeMatrix(subCov, n)
		weights = minVarianceFromInverse(regularized, n)
	}
	if weights == nil {
		// Equal weights fallback.
		weights = make([]float64, n)
		for i := range weights {
			weights[i] = 1.0 / float64(n)
		}
	}

	return weights
}

// minVarianceFromInverse computes the analytical minimum variance weights
// from an already-inverted covariance matrix. Returns nil if the matrix
// is singular or the result is invalid.
func minVarianceFromInverse(covMatrix [][]float64, n int) []float64 {
	inv, err := inverseMatrix(covMatrix, n)
	if err != nil {
		return nil
	}

	// Compute SigmaInv1 = inv * ones (sum of each row of the inverse).
	sigmaInv1 := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			sigmaInv1[i] += inv[i][j]
		}
	}

	// Compute denom = 1' * SigmaInv1.
	denom := 0.0
	for _, v := range sigmaInv1 {
		denom += v
	}

	if denom == 0 || math.IsNaN(denom) || math.IsInf(denom, 0) {
		return nil
	}

	// Normalize: w = SigmaInv1 / denom.
	weights := make([]float64, n)
	for i := 0; i < n; i++ {
		weights[i] = sigmaInv1[i] / denom
	}

	// Clamp negative weights to zero and renormalize (numerical artifacts).
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

// recursiveBisect performs the recursive bisection step of HRP.
// It takes the sorted indices (from quasi-diagonalization) and the
// covariance matrix, and returns a map of original symbol index → weight.
//
// The algorithm:
//  1. Find the optimal bisection point (minimizing cross-group covariance)
//  2. Split into two groups
//  3. Compute min-variance portfolio for each group to get its variance
//  4. Allocate capital using inverse-variance weighting
//     (w_group = (1/sigma²_group) / sum(1/sigma²))
//  5. Recurse within each group
//  6. Scale leaf weights by the group budget from each level
func recursiveBisect(indices []int, covMatrix [][]float64) map[int]float64 {
	weights := make(map[int]float64)

	if len(indices) == 1 {
		weights[indices[0]] = 1.0
		return weights
	}

	// Find the optimal bisection point.
	split := findBisectionPoint(indices, covMatrix)

	// Split into two groups.
	left := make([]int, split)
	copy(left, indices[:split])
	right := make([]int, len(indices)-split)
	copy(right, indices[split:])

	// Compute minimum variance weights within each group.
	leftW := minVarianceWeights(left, covMatrix)
	rightW := minVarianceWeights(right, covMatrix)

	// Compute the variance of each group's min-variance portfolio.
	leftVar := portfolioVariance(left, leftW, covMatrix)
	rightVar := portfolioVariance(right, rightW, covMatrix)

	// Inverse-variance allocation (standard HRP capital allocation).
	// Each group's budget is proportional to 1/variance.
	leftInvVar := 1.0 / leftVar
	rightInvVar := 1.0 / rightVar
	totalInvVar := leftInvVar + rightInvVar

	leftBudget := leftInvVar / totalInvVar
	rightBudget := rightInvVar / totalInvVar

	// Recurse on each group.
	leftResult := recursiveBisect(left, covMatrix)
	rightResult := recursiveBisect(right, covMatrix)

	// Scale weights by group budget.
	for idx, w := range leftResult {
		weights[idx] = w * leftBudget
	}
	for idx, w := range rightResult {
		weights[idx] = w * rightBudget
	}

	return weights
}

// portfolioVariance computes w' * Sigma * w for the given indices and weights.
func portfolioVariance(indices []int, weights []float64, covMatrix [][]float64) float64 {
	n := len(indices)
	var variance float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			variance += weights[i] * weights[j] * covMatrix[indices[i]][indices[j]]
		}
	}
	return variance
}

// inverseMatrix computes the inverse of a matrix using Gaussian elimination
// with partial pivoting. Returns an error if the matrix is singular.
func inverseMatrix(matrix [][]float64, n int) ([][]float64, error) {
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
			return nil, ErrNumericalFailure
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

// regularizeMatrix adds a small epsilon to the diagonal of the matrix
// to improve numerical stability.
func regularizeMatrix(matrix [][]float64, n int) [][]float64 {
	result := make([][]float64, n)
	for i := 0; i < n; i++ {
		result[i] = make([]float64, n)
		copy(result[i], matrix[i])
	}

	var avgDiag float64
	for i := 0; i < n; i++ {
		avgDiag += matrix[i][i]
	}
	avgDiag /= float64(n)
	epsilon := avgDiag * 1e-6

	for i := 0; i < n; i++ {
		result[i][i] += epsilon
	}
	return result
}
