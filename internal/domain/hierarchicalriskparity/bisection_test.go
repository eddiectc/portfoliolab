package hierarchicalriskparity

import (
	"math"
	"testing"
)

// --- Test findBisectionPoint ---

func TestFindBisectionPointTwoAssets(t *testing.T) {
	// Two assets: only one possible split (after index 0).
	cov := [][]float64{
		{0.04, 0.01},
		{0.01, 0.09},
	}
	indices := []int{0, 1}

	split := findBisectionPoint(indices, cov)
	if split != 1 {
		t.Errorf("split = %d, want 1", split)
	}
}

func TestFindBisectionPointThreeAssets(t *testing.T) {
	// Three assets: A(0) correlated with B(1), both uncorrelated with C(2).
	// Best split should be after A,B (split=2) to isolate C.
	cov := [][]float64{
		{0.04, 0.03, 0.001},
		{0.03, 0.09, 0.001},
		{0.001, 0.001, 0.01},
	}
	indices := []int{0, 1, 2}

	split := findBisectionPoint(indices, cov)
	if split != 2 {
		t.Errorf("split = %d, want 2 (isolate C from A,B)", split)
	}
}

func TestFindBisectionPointFourAssetsTwoClusters(t *testing.T) {
	// Four assets: A(0), B(1) correlated; C(2), D(3) correlated;
	// cross-cluster covariance is small.
	// Best split should be after B (split=2).
	cov := [][]float64{
		{0.04, 0.03, 0.001, 0.001},
		{0.03, 0.09, 0.001, 0.001},
		{0.001, 0.001, 0.01, 0.008},
		{0.001, 0.001, 0.008, 0.02},
	}
	indices := []int{0, 1, 2, 3}

	split := findBisectionPoint(indices, cov)
	if split != 2 {
		t.Errorf("split = %d, want 2", split)
	}
}

// --- Test minVarianceWeights ---

func TestMinVarianceWeightsTwoAssets(t *testing.T) {
	// Two uncorrelated assets with different variances.
	// Min variance should weight the lower-variance asset more.
	cov := [][]float64{
		{0.04, 0.0},
		{0.0, 0.01},
	}
	indices := []int{0, 1}

	weights := minVarianceWeights(indices, cov)
	if len(weights) != 2 {
		t.Fatalf("len = %d, want 2", len(weights))
	}

	// For diagonal covariance: w_i ∝ 1/var_i
	// w_0 ∝ 1/0.04 = 25, w_1 ∝ 1/0.01 = 100
	// w_0 = 25/125 = 0.2, w_1 = 100/125 = 0.8
	almostEqual(t, weights[0], 0.2, 1e-6, "weight[0]")
	almostEqual(t, weights[1], 0.8, 1e-6, "weight[1]")

	// Weights should sum to 1.
	sum := weights[0] + weights[1]
	almostEqual(t, sum, 1.0, 1e-6, "weight sum")
}

func TestMinVarianceWeightsEqualVariance(t *testing.T) {
	// Two identical uncorrelated assets → equal weights.
	cov := [][]float64{
		{0.04, 0.0},
		{0.0, 0.04},
	}
	indices := []int{0, 1}

	weights := minVarianceWeights(indices, cov)
	almostEqual(t, weights[0], 0.5, 1e-6, "weight[0]")
	almostEqual(t, weights[1], 0.5, 1e-6, "weight[1]")
}

func TestMinVarianceWeightsSingleAsset(t *testing.T) {
	cov := [][]float64{
		{0.04, 0.01},
		{0.01, 0.09},
	}
	indices := []int{0}

	weights := minVarianceWeights(indices, cov)
	if len(weights) != 1 || math.Abs(weights[0]-1.0) > 1e-9 {
		t.Errorf("got %v, want [1.0]", weights)
	}
}

func TestMinVarianceWeightsSubset(t *testing.T) {
	// Use a 4x4 covariance matrix but only indices [1, 3].
	cov := [][]float64{
		{0.04, 0.01, 0.005, 0.002},
		{0.01, 0.09, 0.02, 0.01},
		{0.005, 0.02, 0.01, 0.003},
		{0.002, 0.01, 0.003, 0.02},
	}
	indices := []int{1, 3}

	weights := minVarianceWeights(indices, cov)
	if len(weights) != 2 {
		t.Fatalf("len = %d, want 2", len(weights))
	}

	// Sub-matrix for indices [1, 3]:
	// [[0.09, 0.01], [0.01, 0.02]]
	// Inverse: det = 0.09*0.02 - 0.01*0.01 = 0.0018 - 0.0001 = 0.0017
	// inv = 1/0.0017 * [[0.02, -0.01], [-0.01, 0.09]]
	// sigmaInv1 = [0.02/0.0017 - 0.01/0.0017, -0.01/0.0017 + 0.09/0.0017]
	//           = [(0.02-0.01)/0.0017, (0.09-0.01)/0.0017]
	//           = [0.01/0.0017, 0.08/0.0017]
	// denom = 0.09/0.0017
	// w_0 = 0.01/0.09 = 1/9 ≈ 0.1111
	// w_1 = 0.08/0.09 = 8/9 ≈ 0.8889
	almostEqual(t, weights[0], 1.0/9.0, 1e-6, "weight[0]")
	almostEqual(t, weights[1], 8.0/9.0, 1e-6, "weight[1]")

	sum := weights[0] + weights[1]
	almostEqual(t, sum, 1.0, 1e-6, "weight sum")
}

// --- Test recursiveBisect ---

func TestRecursiveBisectTwoAssets(t *testing.T) {
	// Two uncorrelated assets with different variances.
	cov := [][]float64{
		{0.04, 0.0},
		{0.0, 0.01},
	}
	indices := []int{0, 1}

	weights := recursiveBisect(indices, cov)

	if len(weights) != 2 {
		t.Fatalf("len = %d, want 2", len(weights))
	}

	// With two uncorrelated assets, inverse-variance allocation:
	// left = {0}, right = {1}
	// leftVar = 0.04, rightVar = 0.01
	// leftBudget = (1/0.04) / (1/0.04 + 1/0.01) = 25 / (25 + 100) = 25/125 = 0.2
	// rightBudget = 100/125 = 0.8
	almostEqual(t, weights[0], 0.2, 1e-6, "weight[0]")
	almostEqual(t, weights[1], 0.8, 1e-6, "weight[1]")

	// Weights should sum to 1.
	sum := weights[0] + weights[1]
	almostEqual(t, sum, 1.0, 1e-6, "weight sum")

	// All weights should be non-negative.
	for idx, w := range weights {
		if w < 0 {
			t.Errorf("weight[%d] = %.6f, want >= 0", idx, w)
		}
	}
}

func TestRecursiveBisectTwoEqualAssets(t *testing.T) {
	// Two identical uncorrelated assets → equal weights.
	cov := [][]float64{
		{0.04, 0.0},
		{0.0, 0.04},
	}
	indices := []int{0, 1}

	weights := recursiveBisect(indices, cov)
	almostEqual(t, weights[0], 0.5, 1e-6, "weight[0]")
	almostEqual(t, weights[1], 0.5, 1e-6, "weight[1]")
}

func TestRecursiveBisectThreeAssets(t *testing.T) {
	// Three assets: A(0), B(1) correlated; C(2) independent.
	// Covariance matrix (annualized daily):
	cov := [][]float64{
		{0.04, 0.03, 0.001},
		{0.03, 0.09, 0.001},
		{0.001, 0.001, 0.01},
	}
	indices := []int{0, 1, 2}

	weights := recursiveBisect(indices, cov)

	if len(weights) != 3 {
		t.Fatalf("len = %d, want 3", len(weights))
	}

	// Weights should sum to 1.
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	almostEqual(t, sum, 1.0, 1e-4, "weight sum")

	// All weights should be non-negative.
	for idx, w := range weights {
		if w < -1e-10 {
			t.Errorf("weight[%d] = %.10f, want >= 0", idx, w)
		}
	}

	// C(2) has low variance and is uncorrelated, so it should get a meaningful weight.
	if weights[2] < 0.1 {
		t.Errorf("weight[2] = %.6f, expected significant allocation for low-variance asset", weights[2])
	}
}

func TestRecursiveBisectFourAssetsTwoClusters(t *testing.T) {
	// Four assets in two clear clusters.
	cov := [][]float64{
		{0.04, 0.03, 0.001, 0.001},
		{0.03, 0.09, 0.001, 0.001},
		{0.001, 0.001, 0.01, 0.008},
		{0.001, 0.001, 0.008, 0.02},
	}
	indices := []int{0, 1, 2, 3}

	weights := recursiveBisect(indices, cov)

	if len(weights) != 4 {
		t.Fatalf("len = %d, want 4", len(weights))
	}

	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	almostEqual(t, sum, 1.0, 1e-4, "weight sum")

	for idx, w := range weights {
		if w < -1e-10 {
			t.Errorf("weight[%d] = %.10f, want >= 0", idx, w)
		}
	}
}

func TestRecursiveBisectSingleAsset(t *testing.T) {
	cov := [][]float64{{0.04}}
	indices := []int{0}

	weights := recursiveBisect(indices, cov)
	if len(weights) != 1 {
		t.Fatalf("len = %d, want 1", len(weights))
	}
	if math.Abs(weights[0]-1.0) > 1e-9 {
		t.Errorf("weight[0] = %.10f, want 1.0", weights[0])
	}
}

// --- Test portfolioVariance ---

func TestPortfolioVarianceTwoAssets(t *testing.T) {
	cov := [][]float64{
		{0.04, 0.01},
		{0.01, 0.09},
	}
	indices := []int{0, 1}
	weights := []float64{0.5, 0.5}

	variance := portfolioVariance(indices, weights, cov)
	// w' * Sigma * w = 0.5*0.5*0.04 + 0.5*0.5*0.01 + 0.5*0.5*0.01 + 0.5*0.5*0.09
	//                = 0.01 + 0.0025 + 0.0025 + 0.0225 = 0.0375
	almostEqual(t, variance, 0.0375, 1e-9, "portfolio variance")
}

func TestPortfolioVarianceSingleAsset(t *testing.T) {
	cov := [][]float64{
		{0.04, 0.01},
		{0.01, 0.09},
	}
	indices := []int{0}
	weights := []float64{1.0}

	variance := portfolioVariance(indices, weights, cov)
	almostEqual(t, variance, 0.04, 1e-9, "portfolio variance")
}

// --- Test inverseMatrix ---

func TestInverseMatrixIdentity(t *testing.T) {
	matrix := [][]float64{
		{1.0, 0.0},
		{0.0, 1.0},
	}
	inv, err := inverseMatrix(matrix, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify A * A⁻¹ ≈ I.
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			sum := 0.0
			for k := 0; k < 2; k++ {
				sum += matrix[i][k] * inv[k][j]
			}
			expected := 0.0
			if i == j {
				expected = 1.0
			}
			almostEqual(t, sum, expected, 1e-6, "identity check")
		}
	}
}

func TestInverseMatrixSingular(t *testing.T) {
	matrix := [][]float64{
		{1.0, 2.0},
		{2.0, 4.0},
	}
	_, err := inverseMatrix(matrix, 2)
	if err == nil {
		t.Error("expected error for singular matrix, got nil")
	}
}

func TestInverseMatrix3x3(t *testing.T) {
	matrix := [][]float64{
		{2.0, 1.0, 0.0},
		{1.0, 3.0, 1.0},
		{0.0, 1.0, 2.0},
	}
	inv, err := inverseMatrix(matrix, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify A * A⁻¹ ≈ I.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			sum := 0.0
			for k := 0; k < 3; k++ {
				sum += matrix[i][k] * inv[k][j]
			}
			expected := 0.0
			if i == j {
				expected = 1.0
			}
			almostEqual(t, sum, expected, 1e-6, "3x3 identity check")
		}
	}
}

// --- Test regularizeMatrix ---

func TestRegularizeMatrix(t *testing.T) {
	matrix := [][]float64{
		{0.04, 0.01},
		{0.01, 0.09},
	}
	regularized := regularizeMatrix(matrix, 2)

	// Diagonal should be increased slightly.
	if regularized[0][0] <= matrix[0][0] {
		t.Errorf("diagonal[0] not increased")
	}
	if regularized[1][1] <= matrix[1][1] {
		t.Errorf("diagonal[1] not increased")
	}
	// Off-diagonal should be unchanged.
	if regularized[0][1] != matrix[0][1] {
		t.Errorf("off-diagonal changed")
	}
}
