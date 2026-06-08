package hierarchicalriskparity

import (
	"math"
	"testing"
)

// --- Test ComputeCorrelationMatrix ---

func TestComputeCorrelationMatrix(t *testing.T) {
	tests := []struct {
		name    string
		aligned [][]float64
		want    [][]float64
	}{
		{
			name: "perfectly correlated (identical series)",
			aligned: [][]float64{
				{0.01, 0.01},
				{-0.02, -0.02},
				{0.03, 0.03},
			},
			want: [][]float64{
				{1.0, 1.0},
				{1.0, 1.0},
			},
		},
		{
			name: "perfectly negatively correlated",
			aligned: [][]float64{
				{0.01, -0.01},
				{-0.02, 0.02},
				{0.03, -0.03},
			},
			want: [][]float64{
				{1.0, -1.0},
				{-1.0, 1.0},
			},
		},
		{
			name: "uncorrelated series",
			aligned: [][]float64{
				{0.01, 0.0},
				{-0.02, 0.0},
				{0.03, 0.0},
				{0.01, 0.0},
				{-0.01, 0.0},
			},
			want: [][]float64{
				{1.0, 0.0},
				{0.0, 1.0},
			},
		},
		{
			name: "three symbols — A=B, C independent of both",
			aligned: [][]float64{
				{1.0, 1.0, 0.0},
				{2.0, 2.0, 1.0},
				{3.0, 3.0, 0.0},
				{4.0, 4.0, 1.0},
				{5.0, 5.0, 0.0},
			},
			want: [][]float64{
				{1.0, 1.0, 0.0},
				{1.0, 1.0, 0.0},
				{0.0, 0.0, 1.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeCorrelationMatrix(tt.aligned)
			n := len(tt.want)
			if len(got) != n {
				t.Fatalf("rows = %d, want %d", len(got), n)
			}
			for i := 0; i < n; i++ {
				if len(got[i]) != n {
					t.Fatalf("cols in row %d = %d, want %d", i, len(got[i]), n)
				}
				for j := 0; j < n; j++ {
					if math.Abs(got[i][j]-tt.want[i][j]) > 0.0001 {
						t.Errorf("corr[%d][%d] = %.6f, want %.6f", i, j, got[i][j], tt.want[i][j])
					}
				}
			}
		})
	}
}

func TestComputeCorrelationMatrixSymmetry(t *testing.T) {
	aligned := [][]float64{
		{0.01, -0.02, 0.03},
		{-0.01, 0.02, -0.01},
		{0.02, -0.01, 0.01},
		{0.01, 0.01, -0.02},
		{-0.02, -0.01, 0.02},
	}

	corr := ComputeCorrelationMatrix(aligned)

	// Check symmetry.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if math.Abs(corr[i][j]-corr[j][i]) > 0.0001 {
				t.Errorf("corr[%d][%d] = %.6f != corr[%d][%d] = %.6f",
					i, j, corr[i][j], j, i, corr[j][i])
			}
		}
	}

	// Check diagonal.
	for i := 0; i < 3; i++ {
		if math.Abs(corr[i][i]-1.0) > 0.0001 {
			t.Errorf("corr[%d][%d] = %.6f, want 1.0", i, i, corr[i][i])
		}
	}
}
