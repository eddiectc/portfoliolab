package efficientfrontier

import (
	"math"
	"testing"
)

func TestComputeCorrelationMatrix(t *testing.T) {
	tests := []struct {
		name      string
		covMatrix [][]float64
		n         int
		wantDiag  float64 // expected diagonal value
		wantOff   float64 // expected off-diagonal (approx)
		epsilon   float64
	}{
		{
			name: "uncorrelated assets",
			covMatrix: [][]float64{
				{0.04, 0.0},
				{0.0, 0.09},
			},
			n:        2,
			wantDiag: 1.0,
			wantOff:  0.0,
			epsilon:  0.0001,
		},
		{
			name: "perfectly correlated",
			covMatrix: [][]float64{
				{0.04, 0.06},
				{0.06, 0.09},
			},
			// corr = 0.06 / (0.2 * 0.3) = 0.06 / 0.06 = 1.0
			n:        2,
			wantDiag: 1.0,
			wantOff:  1.0,
			epsilon:  0.0001,
		},
		{
			name: "negatively correlated",
			covMatrix: [][]float64{
				{0.04, -0.02},
				{-0.02, 0.01},
			},
			// corr = -0.02 / (0.2 * 0.1) = -0.02 / 0.02 = -1.0
			n:        2,
			wantDiag: 1.0,
			wantOff:  -1.0,
			epsilon:  0.0001,
		},
		{
			name: "partial correlation",
			covMatrix: [][]float64{
				{0.04, 0.01},
				{0.01, 0.09},
			},
			// corr = 0.01 / (0.2 * 0.3) = 0.01 / 0.06 = 0.1667
			n:        2,
			wantDiag: 1.0,
			wantOff:  0.1667,
			epsilon:  0.001,
		},
		{
			name: "3x3 mixed correlations",
			covMatrix: [][]float64{
				{0.04, 0.01, -0.005},
				{0.01, 0.09, 0.02},
				{-0.005, 0.02, 0.01},
			},
			n:        3,
			wantDiag: 1.0,
			// Just check diagonal and symmetry
			epsilon: 0.0001,
		},
		{
			name: "zero variance on diagonal",
			covMatrix: [][]float64{
				{0.0, 0.0},
				{0.0, 0.09},
			},
			n:        2,
			wantDiag: 1.0, // diagonal[1][1] should be 1.0
			wantOff:  0.0, // row 0 should be zeros
			epsilon:  0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corr := ComputeCorrelationMatrix(tt.covMatrix, tt.n)

			// Check dimensions.
			if len(corr) != tt.n {
				t.Errorf("rows = %d, want %d", len(corr), tt.n)
			}
			for i := range corr {
				if len(corr[i]) != tt.n {
					t.Errorf("row[%d] cols = %d, want %d", i, len(corr[i]), tt.n)
				}
			}

			// Check diagonal is 1.0 (unless zero variance).
			for i := 0; i < tt.n; i++ {
				if tt.covMatrix[i][i] > 0 {
					if math.Abs(corr[i][i]-tt.wantDiag) > tt.epsilon {
						t.Errorf("corr[%d][%d] = %.4f, want %.4f", i, i, corr[i][i], tt.wantDiag)
					}
				}
			}

			// Check symmetry.
			for i := 0; i < tt.n; i++ {
				for j := i + 1; j < tt.n; j++ {
					if math.Abs(corr[i][j]-corr[j][i]) > tt.epsilon {
						t.Errorf("corr[%d][%d] = %.4f != corr[%d][%d] = %.4f",
							i, j, corr[i][j], j, i, corr[j][i])
					}
				}
			}

			// Check off-diagonal for 2x2 cases.
			if tt.n == 2 && tt.covMatrix[0][0] > 0 && tt.covMatrix[1][1] > 0 {
				if math.Abs(corr[0][1]-tt.wantOff) > tt.epsilon {
					t.Errorf("corr[0][1] = %.4f, want %.4f", corr[0][1], tt.wantOff)
				}
			}
		})
	}
}
