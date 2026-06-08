package hierarchicalriskparity

import (
	"fmt"
	"math"
)

// MergeRecord records a single merge step in hierarchical clustering.
type MergeRecord struct {
	// Cluster1 is the index of the first cluster merged (0-based among active clusters).
	Cluster1 int
	// Cluster2 is the index of the second cluster merged.
	Cluster2 int
	// Distance is the linkage distance at which the merge occurred.
	Distance float64
}

// clusterSingleLinkage performs agglomerative hierarchical clustering using
// single linkage (nearest-neighbor). The distance between two clusters is
// the minimum distance between any pair of their members.
func clusterSingleLinkage(distanceMatrix [][]float64) ([]MergeRecord, *DendrogramNode) {
	n := len(distanceMatrix)
	merges := hierarchicalCluster(distanceMatrix, n, func(d [][]float64, i, j, k int, _ []int) float64 {
		a := d[k][i]
		b := d[k][j]
		if b < a {
			return b
		}
		return a
	})
	return merges, buildDendrogramTree(merges, n)
}

// clusterCompleteLinkage performs agglomerative hierarchical clustering using
// complete linkage (farthest-neighbor). The distance between two clusters is
// the maximum distance between any pair of their members.
func clusterCompleteLinkage(distanceMatrix [][]float64) ([]MergeRecord, *DendrogramNode) {
	n := len(distanceMatrix)
	merges := hierarchicalCluster(distanceMatrix, n, func(d [][]float64, i, j, k int, _ []int) float64 {
		a := d[k][i]
		b := d[k][j]
		if b > a {
			return b
		}
		return a
	})
	return merges, buildDendrogramTree(merges, n)
}

// clusterAverageLinkage performs agglomerative hierarchical clustering using
// average linkage (UPGMA). The distance between two clusters is the mean
// distance between all pairs of their members.
func clusterAverageLinkage(distanceMatrix [][]float64) ([]MergeRecord, *DendrogramNode) {
	n := len(distanceMatrix)
	merges := hierarchicalCluster(distanceMatrix, n, func(d [][]float64, i, j, k int, sizes []int) float64 {
		ni := sizes[i]
		nj := sizes[j]
		return (float64(ni)*d[k][i] + float64(nj)*d[k][j]) / float64(ni+nj)
	})
	return merges, buildDendrogramTree(merges, n)
}

// clusterWardLinkage performs agglomerative hierarchical clustering using
// Ward's method. The merge that minimizes the total within-cluster variance
// is chosen at each step. The Lance-Williams update formula is applied:
//
//	d(k,{i,j}) = ((n_k+n_i)*d(k,i) + (n_k+n_j)*d(k,j) - n_k*d(i,j)) / (n_k+n_i+n_j)
func clusterWardLinkage(distanceMatrix [][]float64) ([]MergeRecord, *DendrogramNode) {
	n := len(distanceMatrix)
	merges := hierarchicalCluster(distanceMatrix, n, func(d [][]float64, i, j, k int, sizes []int) float64 {
		ni := sizes[i]
		nj := sizes[j]
		nk := sizes[k]
		total := nk + ni + nj
		return (float64(nk+ni)*d[k][i] + float64(nk+nj)*d[k][j] - float64(nk)*d[i][j]) / float64(total)
	})
	return merges, buildDendrogramTree(merges, n)
}

// distanceUpdateFunc computes the new distance between cluster k and the
// merged cluster {i,j} using a linkage-specific formula.
type distanceUpdateFunc func(d [][]float64, i, j, k int, sizes []int) float64

// hierarchicalCluster performs agglomerative hierarchical clustering using
// the provided distance update function.
//
// It starts with n individual clusters (one per data point) and repeatedly
// merges the two closest clusters until one remains. The merge sequence is
// returned as MergeRecords.
func hierarchicalCluster(distanceMatrix [][]float64, n int, update distanceUpdateFunc) []MergeRecord {
	if n < 2 {
		return nil
	}

	// Pre-allocate for at most 2n nodes (n original + n-1 merges + 1 safety).
	maxNodes := 2 * n
	dist := make([][]float64, maxNodes)
	sizes := make([]int, maxNodes)
	active := make([]bool, maxNodes)

	// Initialize with original points.
	for i := 0; i < n; i++ {
		dist[i] = make([]float64, maxNodes)
		for j := 0; j < n; j++ {
			dist[i][j] = distanceMatrix[i][j]
		}
		sizes[i] = 1
		active[i] = true
	}

	var merges []MergeRecord
	nextIdx := n

	for {
		// Find the pair of active clusters with the minimum distance.
		minDist := math.Inf(1)
		minI, minJ := -1, -1

		for ai := 0; ai < nextIdx; ai++ {
			if !active[ai] {
				continue
			}
			for aj := ai + 1; aj < nextIdx; aj++ {
				if !active[aj] {
					continue
				}
				if dist[ai][aj] < minDist {
					minDist = dist[ai][aj]
					minI = ai
					minJ = aj
				}
			}
		}

		if minI == -1 {
			break
		}

		// Record the merge.
		merges = append(merges, MergeRecord{
			Cluster1: minI,
			Cluster2: minJ,
			Distance: minDist,
		})

		// Deactivate the merged clusters.
		active[minI] = false
		active[minJ] = false

		// Create the new merged cluster.
		k := nextIdx
		nextIdx++
		sizes[k] = sizes[minI] + sizes[minJ]
		active[k] = true
		dist[k] = make([]float64, maxNodes)

		// Update distances from the new cluster to all remaining active clusters.
		for m := 0; m < nextIdx; m++ {
			if !active[m] || m == k {
				continue
			}
			newDist := update(dist, minI, minJ, m, sizes)
			dist[k][m] = newDist
			dist[m][k] = newDist
		}

		// Check if only one active cluster remains.
		activeCount := 0
		for i := 0; i < nextIdx; i++ {
			if active[i] {
				activeCount++
			}
		}
		if activeCount <= 1 {
			break
		}
	}

	return merges
}

// buildDendrogramTree constructs a DendrogramNode tree from the merge records.
// Leaf nodes are indexed 0..nSymbols-1; internal nodes are created for each
// merge. The root is the last internal node.
//
// Leaf names are set to the string representation of the index (e.g. "0", "1").
// The caller should replace these with actual symbol names.
func buildDendrogramTree(merges []MergeRecord, nSymbols int) *DendrogramNode {
	if nSymbols == 0 {
		return nil
	}

	// Total nodes: nSymbols leaves + len(merges) internal nodes.
	totalNodes := nSymbols + len(merges)
	nodes := make([]*DendrogramNode, totalNodes)

	// Create leaf nodes (indexed 0..nSymbols-1).
	for i := 0; i < nSymbols; i++ {
		nodes[i] = &DendrogramNode{
			Name:     fmt.Sprintf("%d", i),
			Distance: 0,
		}
	}

	// Create internal nodes for each merge.
	for idx, merge := range merges {
		internalIdx := nSymbols + idx
		nodes[internalIdx] = &DendrogramNode{
			Name:     "",
			Children: []*DendrogramNode{nodes[merge.Cluster1], nodes[merge.Cluster2]},
			Distance: merge.Distance,
		}
	}

	// Root is the last internal node, or the single leaf if no merges.
	if len(merges) == 0 {
		return nodes[0]
	}
	return nodes[totalNodes-1]
}
