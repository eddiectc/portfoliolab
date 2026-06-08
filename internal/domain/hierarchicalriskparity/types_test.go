package hierarchicalriskparity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- Test HrpError ---

func TestHrpError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *HrpError
		wantSub string
	}{
		{
			name:    "insufficient symbols",
			err:     ErrInsufficientSymbols,
			wantSub: "insufficient_symbols",
		},
		{
			name:    "too many symbols",
			err:     ErrTooManySymbols,
			wantSub: "too_many_symbols",
		},
		{
			name:    "insufficient data",
			err:     ErrInsufficientData,
			wantSub: "insufficient_data",
		},
		{
			name:    "numerical failure",
			err:     ErrNumericalFailure,
			wantSub: "numerical_failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("Error() = %q, want to contain %q", got, tt.wantSub)
			}
			if !strings.Contains(got, "hierarchical risk parity") {
				t.Errorf("Error() = %q, want to contain 'hierarchical risk parity'", got)
			}
		})
	}
}

// --- Test HrpError implements error ---

func TestHrpErrorImplementsError(t *testing.T) {
	var err error = ErrInsufficientSymbols
	if err == nil {
		t.Error("HrpError does not implement error interface")
	}
}

// --- Test AllLinkageMethods ---

func TestAllLinkageMethods(t *testing.T) {
	methods := AllLinkageMethods()
	if len(methods) != 4 {
		t.Errorf("AllLinkageMethods() returned %d methods, want 4", len(methods))
	}
	want := []LinkageMethod{LinkageSingle, LinkageComplete, LinkageAverage, LinkageWard}
	for i, m := range methods {
		if m != want[i] {
			t.Errorf("methods[%d] = %q, want %q", i, m, want[i])
		}
	}
}

// --- Test DendrogramNode JSON round-trip ---

func TestDendrogramNodeJSONRoundTrip(t *testing.T) {
	original := &DendrogramNode{
		Distance: 1.23,
		Children: []*DendrogramNode{
			{Name: "AAPL"},
			{
				Name:     "",
				Distance: 0.87,
				Children: []*DendrogramNode{
					{Name: "GOOGL"},
					{Name: "MSFT"},
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded DendrogramNode
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Distance != 1.23 {
		t.Errorf("Distance = %f, want 1.23", decoded.Distance)
	}
	if len(decoded.Children) != 2 {
		t.Fatalf("Children len = %d, want 2", len(decoded.Children))
	}
	if decoded.Children[0].Name != "AAPL" {
		t.Errorf("Children[0].Name = %q, want %q", decoded.Children[0].Name, "AAPL")
	}
	if len(decoded.Children[1].Children) != 2 {
		t.Fatalf("Children[1].Children len = %d, want 2", len(decoded.Children[1].Children))
	}
	if decoded.Children[1].Children[0].Name != "GOOGL" {
		t.Errorf("Children[1].Children[0].Name = %q, want %q",
			decoded.Children[1].Children[0].Name, "GOOGL")
	}
}

// --- Test HrpAllocation JSON round-trip ---

func TestHrpAllocationJSONRoundTrip(t *testing.T) {
	original := HrpAllocation{
		Method: "single",
		Weights: map[string]float64{
			"AAPL":  0.35,
			"GOOGL": 0.25,
			"MSFT":  0.40,
		},
		Dendrogram: &DendrogramNode{
			Distance: 1.5,
			Children: []*DendrogramNode{
				{Name: "AAPL"},
				{Name: "GOOGL"},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded HrpAllocation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Method != "single" {
		t.Errorf("Method = %q, want %q", decoded.Method, "single")
	}
	if len(decoded.Weights) != 3 {
		t.Fatalf("Weights len = %d, want 3", len(decoded.Weights))
	}
	if decoded.Weights["AAPL"] != 0.35 {
		t.Errorf("Weights[AAPL] = %f, want 0.35", decoded.Weights["AAPL"])
	}
	if decoded.Dendrogram == nil {
		t.Error("Dendrogram is nil, want non-nil")
	}
}

// --- Test HrpAllocation zero-value omitempty ---

func TestHrpAllocationZeroValueOmitEmpty(t *testing.T) {
	original := HrpAllocation{
		Method:     "ward",
		Weights:    map[string]float64{},
		Dendrogram: nil,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded HrpAllocation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Method != "ward" {
		t.Errorf("Method = %q, want %q", decoded.Method, "ward")
	}
	if decoded.Weights != nil && len(decoded.Weights) != 0 {
		t.Errorf("Weights = %v, want nil or empty", decoded.Weights)
	}
	if decoded.Dendrogram != nil {
		t.Errorf("Dendrogram = %v, want nil", decoded.Dendrogram)
	}

	// Verify omitempty: the raw JSON should not contain "dendrogram" key
	if strings.Contains(string(data), "dendrogram") {
		t.Error("JSON contains \"dendrogram\" key, want it omitted for nil value")
	}
}

// --- Test HrpResult JSON round-trip ---

func TestHrpResultJSONRoundTrip(t *testing.T) {
	original := HrpResult{
		Symbols:     []string{"AAPL", "GOOGL", "MSFT"},
		TradingDays: 252,
		ComputedAt:  time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		Allocations: []HrpAllocation{
			{
				Method: "single",
				Weights: map[string]float64{
					"AAPL": 0.33, "GOOGL": 0.33, "MSFT": 0.34,
				},
				Dendrogram: &DendrogramNode{Distance: 1.0},
			},
			{
				Method: "complete",
				Weights: map[string]float64{
					"AAPL": 0.34, "GOOGL": 0.33, "MSFT": 0.33,
				},
				Dendrogram: &DendrogramNode{Distance: 1.2},
			},
		},
		Warnings: []string{"FX rate unavailable for JPY"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded HrpResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded.Symbols) != 3 {
		t.Errorf("Symbols len = %d, want 3", len(decoded.Symbols))
	}
	if decoded.TradingDays != 252 {
		t.Errorf("TradingDays = %d, want 252", decoded.TradingDays)
	}
	if len(decoded.Allocations) != 2 {
		t.Errorf("Allocations len = %d, want 2", len(decoded.Allocations))
	}
	if decoded.Allocations[0].Method != "single" {
		t.Errorf("Allocations[0].Method = %q, want %q",
			decoded.Allocations[0].Method, "single")
	}
	if len(decoded.Warnings) != 1 {
		t.Errorf("Warnings len = %d, want 1", len(decoded.Warnings))
	}
}

// --- Test HrpResult empty state ---

func TestHrpResultEmptyState(t *testing.T) {
	result := HrpResult{
		Message: "not enough data for computation",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded HrpResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Message != "not enough data for computation" {
		t.Errorf("Message = %q, want %q", decoded.Message, "not enough data for computation")
	}
	if decoded.Allocations != nil {
		t.Errorf("Allocations = %v, want nil", decoded.Allocations)
	}
}
