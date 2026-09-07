package hierarchicalriskparity

// QuasiDiagonalize extracts the left-to-right leaf ordering from the
// hierarchical clustering merge records. The returned slice contains the
// original symbol indices in cluster proximity order.
//
// It works by maintaining an ordered list for each cluster. When two
// clusters merge, their lists are concatenated (left cluster first).
// The final list is the quasi-diagonal ordering.
func QuasiDiagonalize(merges []MergeRecord, nSymbols int) []int {
	if nSymbols <= 0 {
		return nil
	}
	if nSymbols == 1 {
		return []int{0}
	}

	// orders[i] holds the ordered list of original symbol indices in cluster i.
	// Original clusters are 0..nSymbols-1; merge clusters are nSymbols..nSymbols+len(merges)-1.
	maxNodes := nSymbols + len(merges)
	orders := make([][]int, maxNodes)
	for i := 0; i < nSymbols; i++ {
		orders[i] = []int{i}
	}

	for i, merge := range merges {
		newIdx := nSymbols + i
		// Concatenate left cluster's order then right cluster's order.
		orders[newIdx] = append(orders[newIdx], orders[merge.Cluster1]...)
		orders[newIdx] = append(orders[newIdx], orders[merge.Cluster2]...)
	}

	// The root cluster is the last merge.
	finalIdx := nSymbols + len(merges) - 1
	return orders[finalIdx]
}
