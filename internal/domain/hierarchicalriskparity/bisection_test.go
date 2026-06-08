package hierarchicalriskparity

import (
	"testing"
)

// --- Test findBisectionPoint ---

func TestFindBisectionPoint(t *testing.T) {
	tests := []struct {
		name    string
		indices []int
		cov     [][]float64
		want    int
	}{
		{
			name:    "two assets — only one split",
			indices: []int{0, 1},
			cov: [][]float64{
				{0.04, 0.01},
				{0.01, 0.09},
			},
			want: 1,
		},
		{
			name:    "three assets — isolate C from correlated A,B",
			indices: []int{0, 1, 2},
			cov: [][]float64{
				{0.04, 0.03, 0.001},
				{0.03, 0.09, 0.001},
				{0.001, 0.001, 0.01},
			},
			want: 2,
		},
		{
			name:    "four assets — two clear clusters",
			indices: []int{0, 1, 2, 3},
			cov: [][]float64{
				{0.04, 0.03, 0.001, 0.001},
				{0.03, 0.09, 0.001, 0.001},
				{0.001, 0.001, 0.01, 0.008},
				{0.001, 0.001, 0.008, 0.02},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findBisectionPoint(tt.indices, tt.cov)
			if got != tt.want {
				t.Errorf("split = %d, want %d", got, tt.want)
			}
		})
	}
}

// --- Test minVarianceWeights ---

func TestMinVarianceWeights(t *testing.T) {
	tests := []struct {
		name    string
		indices []int
		cov     [][]float64
		wantLen int
		wantW   []float64 // nil = don't check exact values
		wantSum float64   // 0 = don't check sum
	}{
		{
			name:    "two uncorrelated assets — different variances",
			indices: []int{0, 1},
			cov: [][]float64{
				{0.04, 0.0},
				{0.0, 0.01},
			},
			wantLen: 2,
			wantW:   []float64{0.2, 0.8},
			wantSum: 1.0,
		},
		{
			name:    "two equal-variance uncorrelated assets",
			indices: []int{0, 1},
			cov: [][]float64{
				{0.04, 0.0},
				{0.0, 0.04},
			},
			wantLen: 2,
			wantW:   []float64{0.5, 0.5},
			wantSum: 1.0,
		},
		{
			name:    "single asset",
			indices: []int{0},
			cov: [][]float64{
				{0.04, 0.01},
				{0.01, 0.09},
			},
			wantLen: 1,
			wantW:   []float64{1.0},
			wantSum: 1.0,
		},
		{
			name:    "subset of 4x4 matrix — indices [1,3]",
			indices: []int{1, 3},
			cov: [][]float64{
				{0.04, 0.01, 0.005, 0.002},
				{0.01, 0.09, 0.02, 0.01},
				{0.005, 0.02, 0.01, 0.003},
				{0.002, 0.01, 0.003, 0.02},
			},
			wantLen: 2,
			wantW:   []float64{1.0 / 9.0, 8.0 / 9.0},
			wantSum: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weights := minVarianceWeights(tt.indices, tt.cov)
			if len(weights) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(weights), tt.wantLen)
			}
			if tt.wantW != nil {
				for i, w := range tt.wantW {
					almostEqual(t, weights[i], w, 1e-6, "weight["+string(rune('0'+i))+"]")
				}
			}
			if tt.wantSum != 0 {
				sum := 0.0
				for _, w := range weights {
					sum += w
				}
				almostEqual(t, sum, tt.wantSum, 1e-6, "weight sum")
			}
		})
	}
}

// --- Test recursiveBisect ---

func TestRecursiveBisect(t *testing.T) {
	tests := []struct {
		name      string
		indices   []int
		cov       [][]float64
		wantLen   int
		wantW     map[int]float64 // nil = don't check exact values
		wantSum   float64         // 0 = don't check sum
		minWeight map[int]float64 // minimum expected weight for specific indices
	}{
		{
			name:    "two uncorrelated assets — different variances",
			indices: []int{0, 1},
			cov: [][]float64{
				{0.04, 0.0},
				{0.0, 0.01},
			},
			wantLen: 2,
			wantW:   map[int]float64{0: 0.2, 1: 0.8},
			wantSum: 1.0,
		},
		{
			name:    "two equal-variance uncorrelated assets",
			indices: []int{0, 1},
			cov: [][]float64{
				{0.04, 0.0},
				{0.0, 0.04},
			},
			wantLen: 2,
			wantW:   map[int]float64{0: 0.5, 1: 0.5},
			wantSum: 1.0,
		},
		{
			name:    "three assets — C low-variance uncorrelated",
			indices: []int{0, 1, 2},
			cov: [][]float64{
				{0.04, 0.03, 0.001},
				{0.03, 0.09, 0.001},
				{0.001, 0.001, 0.01},
			},
			wantLen:   3,
			wantSum:   1.0,
			minWeight: map[int]float64{2: 0.1},
		},
		{
			name:    "four assets — two clear clusters",
			indices: []int{0, 1, 2, 3},
			cov: [][]float64{
				{0.04, 0.03, 0.001, 0.001},
				{0.03, 0.09, 0.001, 0.001},
				{0.001, 0.001, 0.01, 0.008},
				{0.001, 0.001, 0.008, 0.02},
			},
			wantLen: 4,
			wantSum: 1.0,
		},
		{
			name:    "single asset",
			indices: []int{0},
			cov:     [][]float64{{0.04}},
			wantLen: 1,
			wantW:   map[int]float64{0: 1.0},
			wantSum: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weights := recursiveBisect(tt.indices, tt.cov)
			if len(weights) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(weights), tt.wantLen)
			}
			if tt.wantW != nil {
				for idx, w := range tt.wantW {
					almostEqual(t, weights[idx], w, 1e-6, "weight["+string(rune('0'+idx))+"]")
				}
			}
			if tt.wantSum != 0 {
				sum := 0.0
				for _, w := range weights {
					sum += w
				}
				almostEqual(t, sum, tt.wantSum, 1e-4, "weight sum")
			}
			for idx, w := range weights {
				if w < -1e-10 {
					t.Errorf("weight[%d] = %.10f, want >= 0", idx, w)
				}
			}
			for idx, minW := range tt.minWeight {
				if weights[idx] < minW {
					t.Errorf("weight[%d] = %.6f, expected >= %.4f", idx, weights[idx], minW)
				}
			}
		})
	}
}

// --- Test portfolioVariance ---

func TestPortfolioVariance(t *testing.T) {
	tests := []struct {
		name    string
		indices []int
		weights []float64
		cov     [][]float64
		want    float64
	}{
		{
			name:    "two assets 50/50",
			indices: []int{0, 1},
			weights: []float64{0.5, 0.5},
			cov: [][]float64{
				{0.04, 0.01},
				{0.01, 0.09},
			},
			want: 0.0375,
		},
		{
			name:    "single asset",
			indices: []int{0},
			weights: []float64{1.0},
			cov: [][]float64{
				{0.04, 0.01},
				{0.01, 0.09},
			},
			want: 0.04,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := portfolioVariance(tt.indices, tt.weights, tt.cov)
			almostEqual(t, got, tt.want, 1e-9, "portfolio variance")
		})
	}
}

// --- Test inverseMatrix ---

func TestInverseMatrix(t *testing.T) {
	tests := []struct {
		name    string
		matrix  [][]float64
		n       int
		wantErr bool
	}{
		{
			name: "2x2 identity",
			matrix: [][]float64{
				{1.0, 0.0},
				{0.0, 1.0},
			},
			n:       2,
			wantErr: false,
		},
		{
			name: "singular matrix",
			matrix: [][]float64{
				{1.0, 2.0},
				{2.0, 4.0},
			},
			n:       2,
			wantErr: true,
		},
		{
			name: "3x3 symmetric",
			matrix: [][]float64{
				{2.0, 1.0, 0.0},
				{1.0, 3.0, 1.0},
				{0.0, 1.0, 2.0},
			},
			n:       3,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := inverseMatrix(tt.matrix, tt.n)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			// Verify A * A⁻¹ ≈ I.
			for i := 0; i < tt.n; i++ {
				for j := 0; j < tt.n; j++ {
					sum := 0.0
					for k := 0; k < tt.n; k++ {
						sum += tt.matrix[i][k] * inv[k][j]
					}
					expected := 0.0
					if i == j {
						expected = 1.0
					}
					almostEqual(t, sum, expected, 1e-6, "identity check")
				}
			}
		})
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
