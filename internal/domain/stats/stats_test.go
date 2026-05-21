package stats

import (
	"math"
	"testing"
)

func approxEqual(t *testing.T, got, want float64, epsilon float64) bool {
	t.Helper()
	if math.Abs(got-want) <= epsilon {
		return true
	}
	t.Errorf("got %v, want ~%v (epsilon %v)", got, want, epsilon)
	return false
}

// --- PearsonCorrelation ---

func TestPearsonCorrelation(t *testing.T) {
	tests := []struct {
		name     string
		x        []float64
		y        []float64
		wantCorr float64
		wantN    int
	}{
		{
			name:     "perfect positive correlation",
			x:        []float64{1, 2, 3, 4, 5},
			y:        []float64{2, 4, 6, 8, 10},
			wantCorr: 1.0,
			wantN:    5,
		},
		{
			name:     "perfect negative correlation",
			x:        []float64{1, 2, 3, 4, 5},
			y:        []float64{10, 8, 6, 4, 2},
			wantCorr: -1.0,
			wantN:    5,
		},
		{
			name:     "negative correlation",
			x:        []float64{1, 2, 3, 4, 5},
			y:        []float64{5, 4, 3, 2, 1},
			wantCorr: -1.0,
			wantN:    5,
		},
		{
			name:     "zero variance in x",
			x:        []float64{5, 5, 5, 5},
			y:        []float64{1, 2, 3, 4},
			wantCorr: 0.0,
			wantN:    4,
		},
		{
			name:     "zero variance in y",
			x:        []float64{1, 2, 3, 4},
			y:        []float64{5, 5, 5, 5},
			wantCorr: 0.0,
			wantN:    4,
		},
		{
			name:     "empty",
			x:        []float64{},
			y:        []float64{},
			wantCorr: 0.0,
			wantN:    0,
		},
		{
			name:     "length mismatch",
			x:        []float64{1, 2, 3},
			y:        []float64{1, 2},
			wantCorr: 0.0,
			wantN:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := PearsonCorrelation(tt.x, tt.y)
			if n != tt.wantN {
				t.Errorf("n = %d, want %d", n, tt.wantN)
			}
			if tt.wantCorr == 0 && got == 0 {
				return // exact zero match
			}
			approxEqual(t, got, tt.wantCorr, 0.01)
		})
	}
}

// --- AlignSeries ---

func TestAlignSeries(t *testing.T) {
	tests := []struct {
		name      string
		a         map[string]float64
		b         map[string]float64
		wantOverlap int
		// We verify alignment by checking that paired values correspond
		// to the same keys.
	}{
		{
			name: "full overlap",
			a:    map[string]float64{"2024-01-01": 1.0, "2024-01-02": 2.0, "2024-01-03": 3.0},
			b:    map[string]float64{"2024-01-01": 10.0, "2024-01-02": 20.0, "2024-01-03": 30.0},
			wantOverlap: 3,
		},
		{
			name: "partial overlap",
			a:    map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4},
			b:    map[string]float64{"c": 30, "d": 40, "e": 50, "f": 60},
			wantOverlap: 2,
		},
		{
			name: "no overlap",
			a:    map[string]float64{"a": 1, "b": 2},
			b:    map[string]float64{"c": 3, "d": 4},
			wantOverlap: 0,
		},
		{
			name:      "one empty",
			a:         map[string]float64{},
			b:         map[string]float64{"a": 1, "b": 2},
			wantOverlap: 0,
		},
		{
			name:      "both empty",
			a:         map[string]float64{},
			b:         map[string]float64{},
			wantOverlap: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, overlap := AlignSeries(tt.a, tt.b)
			if overlap != tt.wantOverlap {
				t.Errorf("overlap = %d, want %d", overlap, tt.wantOverlap)
			}
			if len(x) != overlap || len(y) != overlap {
				t.Errorf("len(x)=%d, len(y)=%d, want both=%d", len(x), len(y), overlap)
			}

			// Verify x values come from a and y values come from b,
			// and they correspond to the same keys.
			for i := 0; i < overlap; i++ {
				found := false
				for key, aVal := range tt.a {
					if aVal == x[i] && tt.b[key] == y[i] {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("pair x[%d]=%v, y[%d]=%v not found in same key", i, x[i], i, y[i])
				}
			}
		})
	}
}

// --- RoundTo2 ---

func TestRoundTo2(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"normal", 3.14159, 3.14},
		{"round up", 2.676, 2.68},
		{"negative", -1.234, -1.23},
		{"negative zero", -0.004, 0.0},
		{"exact zero", 0.0, 0.0},
		{"whole number", 5.0, 5.0},
		{"three decimal rounds to zero", -0.001, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RoundTo2(tt.in)
			if got != tt.want {
				t.Errorf("RoundTo2(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// --- RoundTo4 ---

func TestRoundTo4(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"normal", 1.23456789, 1.2346},
		{"exact", 0.5, 0.5},
		{"negative", -2.34567, -2.3457},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RoundTo4(tt.in)
			if got != tt.want {
				t.Errorf("RoundTo4(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
