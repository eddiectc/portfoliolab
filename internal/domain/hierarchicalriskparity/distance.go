package hierarchicalriskparity

import (
	"math"
)

// CorrelationToDistance converts a correlation matrix to a distance matrix
// using the standard HRP transformation: d(i,j) = sqrt(2 * (1 - corr(i,j))).
//
// The result is symmetric with 0 on the diagonal.
// Distances are in the range [0, 2]: 0 for perfectly correlated assets,
// ~1.41 for uncorrelated, and 2 for perfectly negatively correlated.
func CorrelationToDistance(corrMatrix [][]float64) [][]float64 {
	n := len(corrMatrix)
	dist := make([][]float64, n)
	for i := range dist {
		dist[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		dist[i][i] = 0
		for j := i + 1; j < n; j++ {
			// Clamp to [0, 2] to handle floating-point artifacts
			// (e.g. correlation slightly outside [-1, 1]).
			val := 2.0 * (1.0 - corrMatrix[i][j])
			if val < 0 {
				val = 0
			}
			d := math.Sqrt(val)
			dist[i][j] = d
			dist[j][i] = d
		}
	}

	return dist
}
