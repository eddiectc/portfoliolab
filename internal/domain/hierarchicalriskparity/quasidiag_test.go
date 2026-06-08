package hierarchicalriskparity

import (
	"testing"
)

func TestQuasiDiagonalize(t *testing.T) {
	tests := []struct {
		name     string
		merges   []MergeRecord
		nSymbols int
		want     []int
	}{
		{
			name: "two symbols",
			merges: []MergeRecord{
				{Cluster1: 0, Cluster2: 1, Distance: 1.5},
			},
			nSymbols: 2,
			want:     []int{0, 1},
		},
		{
			name: "three symbols — {0,1} then with 2",
			merges: []MergeRecord{
				{Cluster1: 0, Cluster2: 1, Distance: 1.0},
				{Cluster1: 3, Cluster2: 2, Distance: 2.5},
			},
			nSymbols: 3,
			want:     []int{0, 1, 2},
		},
		{
			name: "three symbols — reversed merge order",
			// merge (0,1) → cluster 3, then (2, 3) → ordering [2, 0, 1]
			merges: []MergeRecord{
				{Cluster1: 0, Cluster2: 1, Distance: 1.0},
				{Cluster1: 2, Cluster2: 3, Distance: 3.0},
			},
			nSymbols: 3,
			want:     []int{2, 0, 1},
		},
		{
			name: "four symbols — two independent pairs",
			merges: []MergeRecord{
				{Cluster1: 0, Cluster2: 1, Distance: 1.0},
				{Cluster1: 2, Cluster2: 3, Distance: 1.5},
				{Cluster1: 4, Cluster2: 5, Distance: 5.0},
			},
			nSymbols: 4,
			want:     []int{0, 1, 2, 3},
		},
		{
			name: "four symbols — nested clustering",
			// (0,1)→4, (4,2)→5, (5,3)→6 → [0, 1, 2, 3]
			merges: []MergeRecord{
				{Cluster1: 0, Cluster2: 1, Distance: 1.0},
				{Cluster1: 4, Cluster2: 2, Distance: 2.0},
				{Cluster1: 5, Cluster2: 3, Distance: 4.0},
			},
			nSymbols: 4,
			want:     []int{0, 1, 2, 3},
		},
		{
			name: "four symbols — nested reversed",
			// (0,1)→4, (2,4)→5, (3,5)→6 → [3, 2, 0, 1]
			merges: []MergeRecord{
				{Cluster1: 0, Cluster2: 1, Distance: 1.0},
				{Cluster1: 2, Cluster2: 4, Distance: 2.0},
				{Cluster1: 3, Cluster2: 5, Distance: 4.0},
			},
			nSymbols: 4,
			want:     []int{3, 2, 0, 1},
		},
		{
			name:     "single symbol",
			merges:   nil,
			nSymbols: 1,
			want:     []int{0},
		},
		{
			name:     "zero symbols",
			merges:   nil,
			nSymbols: 0,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuasiDiagonalize(tt.merges, tt.nSymbols)
			if !intsEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Verify QuasiDiagonalize matches dendrogram tree traversal ---

func TestQuasiDiagonalizeMatchesDendrogram(t *testing.T) {
	// Use the same distance matrix as TestClusterFourSymbols.
	dist := [][]float64{
		{0, 1.0, 5.0, 5.5},
		{1.0, 0, 5.5, 6.0},
		{5.0, 5.5, 0, 1.5},
		{5.5, 6.0, 1.5, 0},
	}

	methods := []struct {
		name string
		fn   func([][]float64) ([]MergeRecord, *DendrogramNode)
	}{
		{"single", clusterSingleLinkage},
		{"complete", clusterCompleteLinkage},
		{"average", clusterAverageLinkage},
		{"ward", clusterWardLinkage},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			merges, tree := m.fn(dist)

			// QuasiDiagonalize from merge records.
			qd := QuasiDiagonalize(merges, 4)

			// Traverse the dendrogram tree to collect leaf names.
			var leaves []string
			collectLeaves(tree, &leaves)

			// Convert leaf names to indices and compare.
			treeOrder := make([]int, len(leaves))
			for i, name := range leaves {
				treeOrder[i] = parseLeafIndex(name)
			}

			for i := range qd {
				if qd[i] != treeOrder[i] {
					t.Errorf("quasi-diag %v != dendrogram %v", qd, treeOrder)
					break
				}
			}
		})
	}
}

func collectLeaves(node *DendrogramNode, leaves *[]string) {
	if len(node.Children) == 0 {
		*leaves = append(*leaves, node.Name)
		return
	}
	for _, child := range node.Children {
		collectLeaves(child, leaves)
	}
}

func parseLeafIndex(name string) int {
	var result int
	for _, c := range name {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
