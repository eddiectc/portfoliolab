package hierarchicalriskparity

import (
	"testing"
)

// --- Test QuasiDiagonalize ---

func TestQuasiDiagonalizeTwoSymbols(t *testing.T) {
	// Two symbols: merge (0,1) at distance 1.5.
	merges := []MergeRecord{
		{Cluster1: 0, Cluster2: 1, Distance: 1.5},
	}

	got := QuasiDiagonalize(merges, 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != 0 || got[1] != 1 {
		t.Errorf("got %v, want [0, 1]", got)
	}
}

func TestQuasiDiagonalizeThreeSymbols(t *testing.T) {
	// Three symbols: merge (0,1) at d=1.0, then merge ({0,1}=3, 2) at d=2.5.
	// Cluster 3 = {0,1}, so the ordering is [0, 1, 2].
	merges := []MergeRecord{
		{Cluster1: 0, Cluster2: 1, Distance: 1.0},
		{Cluster1: 3, Cluster2: 2, Distance: 2.5},
	}

	got := QuasiDiagonalize(merges, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Errorf("got %v, want [0, 1, 2]", got)
	}
}

func TestQuasiDiagonalizeThreeSymbolsReversedMerge(t *testing.T) {
	// Three symbols: merge (0,1) at d=1.0, then merge (2, {0,1}=3) at d=3.0.
	// Cluster 3 = {0,1}, merge order is (2, 3) → ordering is [2, 0, 1].
	merges := []MergeRecord{
		{Cluster1: 0, Cluster2: 1, Distance: 1.0},
		{Cluster1: 2, Cluster2: 3, Distance: 3.0},
	}

	got := QuasiDiagonalize(merges, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != 2 || got[1] != 0 || got[2] != 1 {
		t.Errorf("got %v, want [2, 0, 1]", got)
	}
}

func TestQuasiDiagonalizeFourSymbols(t *testing.T) {
	// Four symbols: A(0) close to B(1), C(2) close to D(3).
	// Merge 1: (0,1) → cluster 4, Merge 2: (2,3) → cluster 5
	// Merge 3: (4,5) → cluster 6 (root)
	// Ordering: [0, 1, 2, 3]
	merges := []MergeRecord{
		{Cluster1: 0, Cluster2: 1, Distance: 1.0},
		{Cluster1: 2, Cluster2: 3, Distance: 1.5},
		{Cluster1: 4, Cluster2: 5, Distance: 5.0},
	}

	got := QuasiDiagonalize(merges, 4)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	want := []int{0, 1, 2, 3}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestQuasiDiagonalizeFourSymbolsNested(t *testing.T) {
	// Four symbols: nested clustering.
	// Merge 1: (0,1) → cluster 4
	// Merge 2: (4,2) → cluster 5 (cluster 5 = {0,1,2})
	// Merge 3: (5,3) → cluster 6 (root = {0,1,2,3})
	// Ordering: [0, 1, 2, 3]
	merges := []MergeRecord{
		{Cluster1: 0, Cluster2: 1, Distance: 1.0},
		{Cluster1: 4, Cluster2: 2, Distance: 2.0},
		{Cluster1: 5, Cluster2: 3, Distance: 4.0},
	}

	got := QuasiDiagonalize(merges, 4)
	want := []int{0, 1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestQuasiDiagonalizeFourSymbolsReversed(t *testing.T) {
	// Four symbols: nested clustering with reversed merge order.
	// Merge 1: (0,1) → cluster 4
	// Merge 2: (2,4) → cluster 5 (cluster 5 = {2, 0, 1})
	// Merge 3: (3,5) → cluster 6 (root = {3, 2, 0, 1})
	// Ordering: [3, 2, 0, 1]
	merges := []MergeRecord{
		{Cluster1: 0, Cluster2: 1, Distance: 1.0},
		{Cluster1: 2, Cluster2: 4, Distance: 2.0},
		{Cluster1: 3, Cluster2: 5, Distance: 4.0},
	}

	got := QuasiDiagonalize(merges, 4)
	want := []int{3, 2, 0, 1}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestQuasiDiagonalizeSingleSymbol(t *testing.T) {
	got := QuasiDiagonalize(nil, 1)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("got %v, want [0]", got)
	}
}

func TestQuasiDiagonalizeZeroSymbols(t *testing.T) {
	got := QuasiDiagonalize(nil, 0)
	if got != nil {
		t.Errorf("got %v, want nil", got)
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
