package efficientfrontier

import (
	"math"
	"testing"
)

// --- Test ComputeMinVariance ---

func TestComputeMinVariance(t *testing.T) {
	tests := []struct {
		name       string
		covMatrix  [][]float64
		n          int
		wantNil    bool
		wantSum    float64
		sumEpsilon float64
		wantAllPos bool
	}{
		{
			name: "2x2 identity — equal weights",
			covMatrix: [][]float64{
				{1.0, 0.0},
				{0.0, 1.0},
			},
			n:          2,
			wantNil:    false,
			wantSum:    1.0,
			sumEpsilon: 0.01,
			wantAllPos: true,
		},
		{
			name: "2x2 correlated — unequal weights",
			covMatrix: [][]float64{
				{4.0, 2.0},
				{2.0, 1.0},
			},
			n:          2,
			wantNil:    false,
			wantSum:    1.0,
			sumEpsilon: 0.01,
			wantAllPos: true,
		},
		{
			name: "3x3 diagonal — inverse proportional to variance",
			covMatrix: [][]float64{
				{4.0, 0.0, 0.0},
				{0.0, 1.0, 0.0},
				{0.0, 0.0, 0.25},
			},
			n:          3,
			wantNil:    false,
			wantSum:    1.0,
			sumEpsilon: 0.01,
			wantAllPos: true,
		},
		{
			name: "singular matrix — regularization produces equal weights",
			covMatrix: [][]float64{
				{1.0, 1.0},
				{1.0, 1.0},
			},
			n:          2,
			wantNil:    false, // regularization makes it invertible
			wantSum:    1.0,
			sumEpsilon: 0.01,
			wantAllPos: true,
		},
		{
			name:      "single element returns nil",
			covMatrix: [][]float64{{1.0}},
			n:         1,
			wantNil:   true,
		},
		{
			name: "3x3 realistic covariance",
			covMatrix: [][]float64{
				{0.04, 0.01, 0.005},
				{0.01, 0.09, 0.02},
				{0.005, 0.02, 0.01},
			},
			n:          3,
			wantNil:    false,
			wantSum:    1.0,
			sumEpsilon: 0.01,
			wantAllPos: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeMinVariance(tt.covMatrix, tt.n)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Errorf("expected non-nil weights")
				return
			}
			if len(got) != tt.n {
				t.Errorf("weights len = %d, want %d", len(got), tt.n)
			}

			// Check sum ≈ 1.0.
			sum := 0.0
			for _, w := range got {
				sum += w
			}
			if math.Abs(sum-tt.wantSum) > tt.sumEpsilon {
				t.Errorf("weights sum = %.6f, want %.6f (±%.6f)", sum, tt.wantSum, tt.sumEpsilon)
			}

			if tt.wantAllPos {
				for i, w := range got {
					if w < -0.001 {
						t.Errorf("weight[%d] = %.6f, want >= 0", i, w)
					}
				}
			}
		})
	}
}

// --- Test inverseSymmetric ---

func TestInverseSymmetric(t *testing.T) {
	tests := []struct {
		name    string
		matrix  [][]float64
		n       int
		wantErr bool
	}{
		{
			name: "2x2 identity — inverse is identity",
			matrix: [][]float64{
				{1.0, 0.0},
				{0.0, 1.0},
			},
			n:       2,
			wantErr: false,
		},
		{
			name: "2x2 diagonal",
			matrix: [][]float64{
				{2.0, 0.0},
				{0.0, 3.0},
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
			name: "3x3 invertible",
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
			inv, err := inverseSymmetric(tt.matrix, tt.n)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
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
					if math.Abs(sum-expected) > 1e-6 {
						t.Errorf("(A*A⁻¹)[%d][%d] = %.10f, want %.10f", i, j, sum, expected)
					}
				}
			}
		})
	}
}

// --- Test regularize ---

func TestRegularize(t *testing.T) {
	matrix := [][]float64{
		{0.04, 0.01},
		{0.01, 0.09},
	}
	regularized := regularize(matrix, 2)

	// Diagonal should be increased slightly.
	if regularized[0][0] <= matrix[0][0] {
		t.Errorf("diagonal[0] not increased: %.10f <= %.10f", regularized[0][0], matrix[0][0])
	}
	if regularized[1][1] <= matrix[1][1] {
		t.Errorf("diagonal[1] not increased: %.10f <= %.10f", regularized[1][1], matrix[1][1])
	}
	// Off-diagonal should be unchanged.
	if regularized[0][1] != matrix[0][1] {
		t.Errorf("off-diagonal changed: %.10f != %.10f", regularized[0][1], matrix[0][1])
	}
}
