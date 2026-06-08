package hierarchicalriskparity

import (
	"fmt"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
)

// LinkageMethod identifies a hierarchical clustering linkage criterion.
type LinkageMethod string

const (
	// LinkageSingle uses the minimum distance between any two members of
	// two clusters (nearest-neighbor).
	LinkageSingle LinkageMethod = "single"
	// LinkageComplete uses the maximum distance between any two members
	// of two clusters (farthest-neighbor).
	LinkageComplete LinkageMethod = "complete"
	// LinkageAverage uses the mean distance between all pairs of members
	// across two clusters.
	LinkageAverage LinkageMethod = "average"
	// LinkageWard minimizes the total within-cluster variance.
	LinkageWard LinkageMethod = "ward"
)

// AllLinkageMethods returns the four supported linkage methods in a
// deterministic order.
func AllLinkageMethods() []LinkageMethod {
	return []LinkageMethod{
		LinkageSingle,
		LinkageComplete,
		LinkageAverage,
		LinkageWard,
	}
}

// HrpRequest holds the input for an HRP computation.
type HrpRequest struct {
	// Symbols is the list of candidate symbol names (internal symbols).
	Symbols []string
	// Prices maps each symbol to its historical price series.
	// Prices must be sorted by date ascending.
	Prices map[string][]market.HistoricalPrice
	// Period is the lookback period used (e.g. "1Y", "3Y", "5Y").
	Period string
}

// HrpResult holds the complete output of an HRP computation.
type HrpResult struct {
	// Allocations are the four HRP allocations, one per linkage method.
	Allocations []HrpAllocation `json:"allocations"`
	// Symbols is the ordered list of symbols used in the computation.
	Symbols []string `json:"symbols"`
	// TradingDays is the number of trading days the data covers.
	TradingDays int `json:"trading_days"`
	// ComputedAt is the time the HRP was computed.
	ComputedAt time.Time `json:"computed_at"`
	// Warnings are non-fatal issues (e.g. data gaps, FX unavailable).
	Warnings []string `json:"warnings,omitempty"`
	// Message is an empty-state message when HRP could not be computed.
	Message string `json:"message,omitempty"`
}

// HrpAllocation is a single HRP allocation for one linkage method.
type HrpAllocation struct {
	// Method is the linkage method name (e.g. "single", "complete").
	Method string `json:"method"`
	// Weights maps each symbol to its allocation weight as a fraction (0.0-1.0).
	Weights map[string]float64 `json:"weights"`
	// Dendrogram is the tree structure for the clustering dendrogram.
	Dendrogram *DendrogramNode `json:"dendrogram,omitempty"`
}

// DendrogramNode represents a node in the hierarchical clustering tree,
// serialized for ECharts tree chart rendering.
type DendrogramNode struct {
	// Name is the label for this node (symbol name for leaves, empty for internal nodes).
	Name string `json:"name"`
	// Children are the sub-nodes (nil for leaf nodes).
	Children []*DendrogramNode `json:"children,omitempty"`
	// Distance is the linkage distance at which this merge occurred.
	// Zero for leaf nodes.
	Distance float64 `json:"distance,omitempty"`
}

// HrpError is a typed error for HRP computation failures.
type HrpError struct {
	// Code is a machine-readable error identifier.
	Code string
	// Message is a human-readable description.
	Message string
}

func (e *HrpError) Error() string {
	return fmt.Sprintf("hierarchical risk parity: %s — %s", e.Code, e.Message)
}

// ErrInsufficientSymbols is returned when fewer than 2 symbols are provided.
var ErrInsufficientSymbols = &HrpError{
	Code:    "insufficient_symbols",
	Message: "at least 2 symbols are required",
}

// ErrTooManySymbols is returned when more than 20 symbols are provided.
var ErrTooManySymbols = &HrpError{
	Code:    "too_many_symbols",
	Message: "maximum 20 symbols supported",
}

// ErrInsufficientData is returned when price data is too short for computation.
var ErrInsufficientData = &HrpError{
	Code:    "insufficient_data",
	Message: "insufficient price data for computation",
}

// ErrNumericalFailure is returned when the computation fails due to
// a numerical error (e.g. singular matrix in bisection).
var ErrNumericalFailure = &HrpError{
	Code:    "numerical_failure",
	Message: "computation failed due to numerical error",
}
