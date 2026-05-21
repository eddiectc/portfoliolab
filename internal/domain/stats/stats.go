package stats

import (
	"math"
)

// PearsonCorrelation computes the Pearson correlation coefficient of two
// equally-lengthed float64 series. Returns the coefficient and sample count.
// If either series has zero variance, returns 0.
func PearsonCorrelation(x, y []float64) (float64, int) {
	n := len(x)
	if n == 0 || n != len(y) {
		return 0, 0
	}

	// Compute means.
	sumX, sumY := 0.0, 0.0
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	// Compute numerator and denominators.
	num := 0.0
	sumDx2 := 0.0
	sumDy2 := 0.0
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		num += dx * dy
		sumDx2 += dx * dx
		sumDy2 += dy * dy
	}

	if sumDx2 == 0 || sumDy2 == 0 {
		return 0, n
	}

	return num / math.Sqrt(sumDx2*sumDy2), n
}

// AlignSeries takes two key→value maps and produces aligned float64 slices
// (matching keys in the same order) plus the overlap count.
// x always corresponds to map a, y to map b.
// Keys are compared as strings; the caller is responsible for providing
// consistent key formatting (e.g. date strings, timestamps).
func AlignSeries(a, b map[string]float64) ([]float64, []float64, int) {
	// Build a walk order from the smaller map for lookup efficiency.
	var mapFromA bool // true if the lookup map is a
	var lookup map[string]float64
	var walk map[string]float64

	if len(a) <= len(b) {
		mapFromA = true
		lookup = a
		walk = b
	} else {
		mapFromA = false
		lookup = b
		walk = a
	}

	x := make([]float64, 0, len(lookup))
	y := make([]float64, 0, len(lookup))

	for key, walkVal := range walk {
		mapped, ok := lookup[key]
		if !ok {
			continue
		}
		if mapFromA {
			// walk is b, lookup is a: mapped=a, walkVal=b
			x = append(x, mapped)
			y = append(y, walkVal)
		} else {
			// walk is a, lookup is b: walkVal=a, mapped=b
			x = append(x, walkVal)
			y = append(y, mapped)
		}
	}

	return x, y, len(x)
}

// RoundTo2 rounds a float64 to 2 decimal places.
// Normalizes -0 to 0 to avoid JSON serializing as -0.
func RoundTo2(v float64) float64 {
	result := math.Round(v*100) / 100
	if result == 0 {
		return 0 // normalize -0 to 0
	}
	return result
}

// RoundTo4 rounds a float64 to 4 decimal places.
func RoundTo4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
