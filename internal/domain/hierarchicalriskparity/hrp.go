package hierarchicalriskparity

import (
	"fmt"
	"math"
	"time"
)

// ComputeHrp runs the full Hierarchical Risk Parity pipeline: validate inputs,
// compute returns, align, correlation, distance, cluster (four linkage methods),
// quasi-diagonalize, recursive bisection, and produce four allocations with
// dendrograms.
func ComputeHrp(request HrpRequest) (*HrpResult, error) {
	symbols := request.Symbols
	n := len(symbols)

	// Validate symbol count.
	if n < 2 {
		return nil, ErrInsufficientSymbols
	}
	if n > 20 {
		return nil, ErrTooManySymbols
	}

	// Compute aligned returns matrix.
	aligned, tradingDays, err := AlignReturns(request.Prices, symbols)
	if err != nil {
		return nil, err
	}

	// Compute correlation and distance matrices.
	corrMatrix := ComputeCorrelationMatrix(aligned)
	distMatrix := CorrelationToDistance(corrMatrix)

	// Compute covariance matrix for recursive bisection.
	covMatrix := computeCovarianceMatrix(aligned)

	// Run HRP for each linkage method.
	allocations := make([]HrpAllocation, 0, 4)

	for _, method := range AllLinkageMethods() {
		// Cluster.
		merges, dendrogram := clusterByMethod(distMatrix, method)

		// Replace leaf index names with actual symbol names.
		replaceLeafNames(dendrogram, symbols)

		// Quasi-diagonalize to get sorted indices.
		sortedIndices := QuasiDiagonalize(merges, n)

		// Recursive bisection to get weights per index.
		weightsByIndex := recursiveBisect(sortedIndices, covMatrix)

		// Convert index-based weights to symbol-based weights.
		weights := make(map[string]float64, n)
		for idx, w := range weightsByIndex {
			weights[symbols[idx]] = w
		}

		allocations = append(allocations, HrpAllocation{
			Method:     string(method),
			Weights:    weights,
			Dendrogram: dendrogram,
		})
	}

	return &HrpResult{
		Allocations: allocations,
		Symbols:     symbols,
		TradingDays: tradingDays,
		ComputedAt:  time.Now(),
	}, nil
}

// clusterByMethod dispatches to the appropriate linkage clustering function.
func clusterByMethod(distanceMatrix [][]float64, method LinkageMethod) ([]MergeRecord, *DendrogramNode) {
	switch method {
	case LinkageSingle:
		return clusterSingleLinkage(distanceMatrix)
	case LinkageComplete:
		return clusterCompleteLinkage(distanceMatrix)
	case LinkageAverage:
		return clusterAverageLinkage(distanceMatrix)
	case LinkageWard:
		return clusterWardLinkage(distanceMatrix)
	default:
		return clusterSingleLinkage(distanceMatrix)
	}
}

// replaceLeafNames walks the dendrogram tree and replaces leaf names
// (index strings like "0", "1") with actual symbol names.
func replaceLeafNames(node *DendrogramNode, symbols []string) {
	if node == nil {
		return
	}
	if len(node.Children) == 0 {
		// Leaf node — replace index string with symbol name.
		for i := 0; i < len(symbols); i++ {
			if node.Name == toIndexString(i) {
				node.Name = symbols[i]
				break
			}
		}
		return
	}
	for _, child := range node.Children {
		replaceLeafNames(child, symbols)
	}
}

// toIndexString produces the same index string used by buildDendrogramTree
// (fmt.Sprintf("%d", i)).
func toIndexString(i int) string {
	return fmt.Sprintf("%d", i)
}

// computeCovarianceMatrix computes the N×N covariance matrix from an aligned
// returns matrix. Each row is one trading date; each column is one symbol.
//
// Uses the standard sample covariance formula:
//
//	cov(i,j) = Σ(r_i - mean_i)(r_j - mean_j) / (n-1)
func computeCovarianceMatrix(aligned [][]float64) [][]float64 {
	nSymbols := len(aligned[0])
	nObs := len(aligned)

	// Compute means.
	means := make([]float64, nSymbols)
	for _, row := range aligned {
		for j := 0; j < nSymbols; j++ {
			means[j] += row[j]
		}
	}
	for j := range means {
		means[j] /= float64(nObs)
	}

	// Compute covariance matrix.
	cov := make([][]float64, nSymbols)
	for i := range cov {
		cov[i] = make([]float64, nSymbols)
	}

	for i := 0; i < nSymbols; i++ {
		for j := i; j < nSymbols; j++ {
			sum := 0.0
			for _, row := range aligned {
				sum += (row[i] - means[i]) * (row[j] - means[j])
			}
			val := sum / float64(nObs-1)
			// Clamp small negative values from floating-point artifacts.
			if math.Abs(val) < 1e-15 {
				val = 0
			}
			cov[i][j] = val
			cov[j][i] = val
		}
	}

	return cov
}
