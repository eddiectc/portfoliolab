package hierarchicalriskparity

import (
	"math"
	"testing"
)

// --- Test CorrelationToDistance ---

func TestCorrelationToDistance(t *testing.T) {
	tests := []struct {
		name string
		corr [][]float64
		want [][]float64
	}{
		{
			name: "perfectly correlated → distance 0",
			corr: [][]float64{
				{1.0, 1.0},
				{1.0, 1.0},
			},
			want: [][]float64{
				{0.0, 0.0},
				{0.0, 0.0},
			},
		},
		{
			name: "uncorrelated → distance ~1.41",
			corr: [][]float64{
				{1.0, 0.0},
				{0.0, 1.0},
			},
			want: [][]float64{
				{0.0, math.Sqrt(2.0)},
				{math.Sqrt(2.0), 0.0},
			},
		},
		{
			name: "perfectly negative → distance 2",
			corr: [][]float64{
				{1.0, -1.0},
				{-1.0, 1.0},
			},
			want: [][]float64{
				{0.0, 2.0},
				{2.0, 0.0},
			},
		},
		{
			name: "partial correlation 0.5 → distance 1",
			corr: [][]float64{
				{1.0, 0.5},
				{0.5, 1.0},
			},
			want: [][]float64{
				{0.0, 1.0},
				{1.0, 0.0},
			},
		},
		{
			name: "three symbols mixed",
			corr: [][]float64{
				{1.0, 0.8, 0.0},
				{0.8, 1.0, -0.5},
				{0.0, -0.5, 1.0},
			},
			want: [][]float64{
				{0.0, math.Sqrt(0.4), math.Sqrt(2.0)},
				{math.Sqrt(0.4), 0.0, math.Sqrt(3.0)},
				{math.Sqrt(2.0), math.Sqrt(3.0), 0.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CorrelationToDistance(tt.corr)
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
						t.Errorf("dist[%d][%d] = %.6f, want %.6f", i, j, got[i][j], tt.want[i][j])
					}
				}
			}
		})
	}
}

func TestCorrelationToDistanceSymmetry(t *testing.T) {
	corr := [][]float64{
		{1.0, 0.3, -0.7},
		{0.3, 1.0, 0.5},
		{-0.7, 0.5, 1.0},
	}

	dist := CorrelationToDistance(corr)

	for i := 0; i < 3; i++ {
		if math.Abs(dist[i][i]) > 0.0001 {
			t.Errorf("dist[%d][%d] = %.6f, want 0", i, i, dist[i][i])
		}
		for j := i + 1; j < 3; j++ {
			if math.Abs(dist[i][j]-dist[j][i]) > 0.0001 {
				t.Errorf("dist[%d][%d] = %.6f != dist[%d][%d] = %.6f",
					i, j, dist[i][j], j, i, dist[j][i])
			}
		}
	}
}

func TestCorrelationToDistanceClamping(t *testing.T) {
	// Correlation slightly above 1.0 (floating-point artifact)
	// should clamp to distance 0.
	corr := [][]float64{
		{1.0, 1.0000001},
		{1.0000001, 1.0},
	}

	dist := CorrelationToDistance(corr)
	if math.Abs(dist[0][1]) > 0.0001 {
		t.Errorf("dist[0][1] = %.10f, want ~0 (clamped)", dist[0][1])
	}
}
