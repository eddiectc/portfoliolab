package hierarchicalriskparity

import (
	"math"
	"testing"
)

// --- Helper ---

func almostEqual(t *testing.T, got, want, tol float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.10f, want %.10f (tol %.10f)", msg, got, want, tol)
	}
}

func countLeaves(node *DendrogramNode) int {
	if node == nil {
		return 0
	}
	if len(node.Children) == 0 {
		return 1
	}
	count := 0
	for _, child := range node.Children {
		count += countLeaves(child)
	}
	return count
}

// --- Test 2-symbol case (single merge) ---

func TestClusterTwoSymbols(t *testing.T) {
	dist := [][]float64{
		{0, 1.5},
		{1.5, 0},
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

			// Should produce exactly one merge.
			if len(merges) != 1 {
				t.Fatalf("merges = %d, want 1", len(merges))
			}

			merge := merges[0]
			if merge.Cluster1 != 0 || merge.Cluster2 != 1 {
				t.Errorf("merge = (%d,%d), want (0,1)", merge.Cluster1, merge.Cluster2)
			}
			almostEqual(t, merge.Distance, 1.5, 1e-9, "merge distance")

			// Tree should have 2 leaves and 1 internal node.
			if countLeaves(tree) != 2 {
				t.Errorf("leaves = %d, want 2", countLeaves(tree))
			}
			if tree.Distance != 1.5 {
				t.Errorf("root distance = %.6f, want 1.5", tree.Distance)
			}
			if len(tree.Children) != 2 {
				t.Errorf("root children = %d, want 2", len(tree.Children))
			}
		})
	}
}

// --- Test 3-symbol case — different linkage methods produce different merge orders ---

func TestClusterThreeSymbolsLinkageDifferences(t *testing.T) {
	// Three symbols: A close to B, B close to C, A far from C.
	// dist(A,B) = 1.0, dist(B,C) = 2.0, dist(A,C) = 3.0
	dist := [][]float64{
		{0, 1.0, 3.0},
		{1.0, 0, 2.0},
		{3.0, 2.0, 0},
	}

	tests := []struct {
		name     string
		fn       func([][]float64) ([]MergeRecord, *DendrogramNode)
		wantDist []float64 // expected merge distances in order
	}{
		{
			name:     "single-linkage",
			fn:       clusterSingleLinkage,
			wantDist: []float64{1.0, 2.0}, // merge(A,B)=1.0, merge({A,B},C)=min(3.0,2.0)=2.0
		},
		{
			name:     "complete-linkage",
			fn:       clusterCompleteLinkage,
			wantDist: []float64{1.0, 3.0}, // merge(A,B)=1.0, merge({A,B},C)=max(3.0,2.0)=3.0
		},
		{
			name:     "average-linkage",
			fn:       clusterAverageLinkage,
			wantDist: []float64{1.0, 2.5}, // merge(A,B)=1.0, merge({A,B},C)=(3.0+2.0)/2=2.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merges, tree := tt.fn(dist)

			if len(merges) != 2 {
				t.Fatalf("merges = %d, want 2", len(merges))
			}

			// First merge should always be (0,1) at distance 1.0.
			if merges[0].Cluster1 != 0 || merges[0].Cluster2 != 1 {
				t.Errorf("first merge = (%d,%d), want (0,1)", merges[0].Cluster1, merges[0].Cluster2)
			}
			almostEqual(t, merges[0].Distance, 1.0, 1e-9, "first merge distance")

			// Second merge distance should match expected.
			almostEqual(t, merges[1].Distance, tt.wantDist[1], 1e-9, "second merge distance")

			// Tree should have 3 leaves.
			if countLeaves(tree) != 3 {
				t.Errorf("leaves = %d, want 3", countLeaves(tree))
			}

			// Merge distances should be monotonically non-decreasing.
			for i := 1; i < len(merges); i++ {
				if merges[i].Distance < merges[i-1].Distance-1e-9 {
					t.Errorf("merge distances not monotonic: %.6f > %.6f",
						merges[i-1].Distance, merges[i].Distance)
				}
			}
		})
	}

	// Ward's method on the same data.
	t.Run("ward-linkage", func(t *testing.T) {
		merges, tree := clusterWardLinkage(dist)

		if len(merges) != 2 {
			t.Fatalf("merges = %d, want 2", len(merges))
		}

		// First merge: (0,1) at distance 1.0 (closest pair).
		almostEqual(t, merges[0].Distance, 1.0, 1e-9, "first merge distance")

		// Second merge distance: Ward update for cluster {0,1} and 2.
		// d(2,{0,1}) = ((1+1)*d(2,0) + (1+1)*d(2,1) - 1*d(0,1)) / (1+1+1)
		//            = (2*3.0 + 2*2.0 - 1*1.0) / 3
		//            = (6.0 + 4.0 - 1.0) / 3 = 9.0 / 3 = 3.0
		almostEqual(t, merges[1].Distance, 3.0, 1e-9, "second merge distance (Ward)")

		if countLeaves(tree) != 3 {
			t.Errorf("leaves = %d, want 3", countLeaves(tree))
		}
	})
}

// --- Test 4-symbol case — verify merge order and tree structure ---

func TestClusterFourSymbols(t *testing.T) {
	// Four symbols: A close to B, C close to D, the two pairs far apart.
	// dist(A,B)=1.0, dist(C,D)=1.5, dist(A,C)=5.0, dist(A,D)=5.5, dist(B,C)=5.5, dist(B,D)=6.0
	dist := [][]float64{
		{0, 1.0, 5.0, 5.5},
		{1.0, 0, 5.5, 6.0},
		{5.0, 5.5, 0, 1.5},
		{5.5, 6.0, 1.5, 0},
	}

	tests := []struct {
		name string
		fn   func([][]float64) ([]MergeRecord, *DendrogramNode)
	}{
		{"single", clusterSingleLinkage},
		{"complete", clusterCompleteLinkage},
		{"average", clusterAverageLinkage},
		{"ward", clusterWardLinkage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merges, tree := tt.fn(dist)

			if len(merges) != 3 {
				t.Fatalf("merges = %d, want 3", len(merges))
			}

			// First merge should be (0,1) at distance 1.0 (closest pair).
			if merges[0].Distance != 1.0 {
				t.Errorf("first merge distance = %.6f, want 1.0", merges[0].Distance)
			}

			// Tree should have 4 leaves.
			if countLeaves(tree) != 4 {
				t.Errorf("leaves = %d, want 4", countLeaves(tree))
			}

			// Root should have exactly 2 children.
			if len(tree.Children) != 2 {
				t.Errorf("root children = %d, want 2", len(tree.Children))
			}

			// Merge distances should be monotonically non-decreasing.
			for i := 1; i < len(merges); i++ {
				if merges[i].Distance < merges[i-1].Distance-1e-9 {
					t.Errorf("merge distances not monotonic at step %d: %.6f > %.6f",
						i, merges[i-1].Distance, merges[i].Distance)
				}
			}
		})
	}
}

// --- Test 5-symbol case ---

func TestClusterFiveSymbols(t *testing.T) {
	// 5 symbols with varying distances.
	dist := [][]float64{
		{0, 1.0, 4.0, 5.0, 7.0},
		{1.0, 0, 3.5, 4.5, 6.5},
		{4.0, 3.5, 0, 2.0, 3.0},
		{5.0, 4.5, 2.0, 0, 2.5},
		{7.0, 6.5, 3.0, 2.5, 0},
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

			if len(merges) != 4 {
				t.Fatalf("merges = %d, want 4", len(merges))
			}

			if countLeaves(tree) != 5 {
				t.Errorf("leaves = %d, want 5", countLeaves(tree))
			}

			// Monotonicity check.
			for i := 1; i < len(merges); i++ {
				if merges[i].Distance < merges[i-1].Distance-1e-9 {
					t.Errorf("merge distances not monotonic at step %d", i)
				}
			}
		})
	}
}

// --- Edge cases ---

func TestClusterSingleSymbol(t *testing.T) {
	dist := [][]float64{{0}}

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

			if len(merges) != 0 {
				t.Errorf("merges = %d, want 0", len(merges))
			}

			if tree == nil {
				t.Fatal("tree is nil, want single leaf")
			}

			if len(tree.Children) != 0 {
				t.Errorf("leaf has %d children, want 0", len(tree.Children))
			}

			if tree.Name != "0" {
				t.Errorf("leaf name = %q, want %q", tree.Name, "0")
			}
		})
	}
}

func TestClusterZeroDistances(t *testing.T) {
	// All distances zero (perfectly correlated assets).
	dist := [][]float64{
		{0, 0, 0},
		{0, 0, 0},
		{0, 0, 0},
	}

	merges, tree := clusterAverageLinkage(dist)

	if len(merges) != 2 {
		t.Fatalf("merges = %d, want 2", len(merges))
	}

	// All merges should be at distance 0.
	for i, m := range merges {
		if m.Distance != 0 {
			t.Errorf("merge %d distance = %.6f, want 0", i, m.Distance)
		}
	}

	if countLeaves(tree) != 3 {
		t.Errorf("leaves = %d, want 3", countLeaves(tree))
	}
}

// --- Test buildDendrogramTree directly ---

func TestBuildDendrogramTreeEmpty(t *testing.T) {
	tree := buildDendrogramTree(nil, 0)
	if tree != nil {
		t.Errorf("expected nil tree for 0 symbols, got non-nil")
	}
}

func TestBuildDendrogramTreeSingleLeaf(t *testing.T) {
	tree := buildDendrogramTree(nil, 1)
	if tree == nil {
		t.Fatal("expected non-nil tree for 1 symbol")
	}
	if len(tree.Children) != 0 {
		t.Errorf("leaf has %d children, want 0", len(tree.Children))
	}
	if tree.Name != "0" {
		t.Errorf("leaf name = %q, want %q", tree.Name, "0")
	}
	if tree.Distance != 0 {
		t.Errorf("leaf distance = %.6f, want 0", tree.Distance)
	}
}

func TestBuildDendrogramTreeStructure(t *testing.T) {
	// Two merges: (0,1) at d=1.0, then ({0,1},2) at d=2.5.
	merges := []MergeRecord{
		{Cluster1: 0, Cluster2: 1, Distance: 1.0},
		{Cluster1: 3, Cluster2: 2, Distance: 2.5}, // cluster 3 = {0,1}
	}

	tree := buildDendrogramTree(merges, 3)

	// Root should be at distance 2.5 with 2 children.
	if tree.Distance != 2.5 {
		t.Errorf("root distance = %.6f, want 2.5", tree.Distance)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(tree.Children))
	}

	// Left child should be internal node at distance 1.0.
	internal := tree.Children[0]
	if internal.Distance != 1.0 {
		t.Errorf("internal distance = %.6f, want 1.0", internal.Distance)
	}
	if len(internal.Children) != 2 {
		t.Errorf("internal children = %d, want 2", len(internal.Children))
	}

	// Leaves should be "0" and "1".
	if internal.Children[0].Name != "0" || internal.Children[1].Name != "1" {
		t.Errorf("leaf names = %q, %q, want %q, %q",
			internal.Children[0].Name, internal.Children[1].Name, "0", "1")
	}

	// Right child should be leaf "2".
	leaf := tree.Children[1]
	if leaf.Name != "2" {
		t.Errorf("leaf name = %q, want %q", leaf.Name, "2")
	}
	if len(leaf.Children) != 0 {
		t.Errorf("leaf has %d children, want 0", len(leaf.Children))
	}
}

// --- Test linkage method produces correct second-merge distance for known data ---

func TestLinkageSecondMergeDistance(t *testing.T) {
	// 3 symbols: A(0), B(1), C(2)
	// dist(A,B)=2, dist(A,C)=5, dist(B,C)=3
	// First merge: (A,B) at d=2
	// Second merge distances depend on linkage method:
	//   single:   min(5, 3) = 3
	//   complete: max(5, 3) = 5
	//   average:  (5+3)/2  = 4
	//   ward:     ((1+1)*5 + (1+1)*3 - 1*2) / 3 = (10+6-2)/3 = 14/3 ≈ 4.667
	dist := [][]float64{
		{0, 2, 5},
		{2, 0, 3},
		{5, 3, 0},
	}

	tests := []struct {
		name string
		fn   func([][]float64) ([]MergeRecord, *DendrogramNode)
		want float64
	}{
		{"single", clusterSingleLinkage, 3.0},
		{"complete", clusterCompleteLinkage, 5.0},
		{"average", clusterAverageLinkage, 4.0},
		{"ward", clusterWardLinkage, 14.0 / 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merges, _ := tt.fn(dist)
			if len(merges) != 2 {
				t.Fatalf("merges = %d, want 2", len(merges))
			}
			almostEqual(t, merges[1].Distance, tt.want, 1e-9, "second merge distance")
		})
	}
}
