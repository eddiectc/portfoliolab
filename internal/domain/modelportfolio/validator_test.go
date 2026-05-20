package modelportfolio

import (
	"errors"
	"strings"
	"testing"

	"github.com/govalues/decimal"
)

// d is a shorthand for decimal.MustNew(value, scale).
// MustNew(value, scale) interprets value as an integer shifted by 10^scale,
// so d(5000, 2) == 50.00 and d(1, 2) == 0.01.
func d(value int64, scale int) decimal.Decimal {
	return decimal.MustNew(value, scale)
}

// entryPair is a shorthand for a symbol + percentage pair in test helpers.
type entryPair struct {
	symbol string
	pct    int64
}

// entries is a helper to create ModelPortfolioEntry slices from entryPair values.
func entries(pairs ...entryPair) []ModelPortfolioEntry {
	out := make([]ModelPortfolioEntry, len(pairs))
	for i, p := range pairs {
		out[i] = ModelPortfolioEntry{Symbol: p.symbol, WeightPct: d(p.pct, 2)}
	}
	return out
}

// --- ValidateCreateRequest Tests ---

func TestValidateCreateRequest_Valid(t *testing.T) {
	tests := []struct {
		name string
		req  CreateRequest
	}{
		{
			name: "two entries summing to 100",
			req: CreateRequest{
				Name:    "Balanced",
				Entries: entries(entryPair{symbol: "AAPL", pct: 5000}, entryPair{symbol: "MSFT", pct: 5000}),
			},
		},
		{
			name: "single entry at 100",
			req: CreateRequest{
				Name:    "All In",
				Entries: entries(entryPair{symbol: "VOO", pct: 10000}),
			},
		},
		{
			name: "five entries summing to 100",
			req: CreateRequest{
				Name: "Diversified",
				Entries: entries(
					entryPair{symbol: "AAPL", pct: 2000},
					entryPair{symbol: "MSFT", pct: 2000},
					entryPair{symbol: "GOOGL", pct: 2000},
					entryPair{symbol: "AMZN", pct: 2000},
					entryPair{symbol: "TSLA", pct: 2000},
				),
			},
		},
		{
			name: "fractional weights",
			req: CreateRequest{
				Name: "Fractional",
				Entries: entries(
					entryPair{symbol: "AAPL", pct: 3333},
					entryPair{symbol: "MSFT", pct: 3333},
					entryPair{symbol: "GOOGL", pct: 3334},
				),
			},
		},
		{
			name: "name at max length",
			req: CreateRequest{
				Name:    strings.Repeat("a", 100),
				Entries: entries(entryPair{symbol: "VOO", pct: 10000}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateRequest(tt.req)
			if err != nil {
				t.Errorf("expected nil, got %v", err)
			}
		})
	}
}

func TestValidateCreateRequest_EmptyName(t *testing.T) {
	req := CreateRequest{
		Name:    "",
		Entries: entries(entryPair{symbol: "AAPL", pct: 10000}),
	}
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

func TestValidateCreateRequest_NameTooLong(t *testing.T) {
	req := CreateRequest{
		Name:    strings.Repeat("a", 101),
		Entries: entries(entryPair{symbol: "AAPL", pct: 10000}),
	}
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got %v", err)
	}
}

func TestValidateCreateRequestEmptyEntries(t *testing.T) {
	req := CreateRequest{
		Name:    "Empty",
		Entries: []ModelPortfolioEntry{},
	}
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrEmptyEntries) {
		t.Errorf("expected ErrEmptyEntries, got %v", err)
	}
}

func TestValidateCreateRequest_NilEntries(t *testing.T) {
	req := CreateRequest{
		Name:    "Nil",
		Entries: nil,
	}
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrEmptyEntries) {
		t.Errorf("expected ErrEmptyEntries, got %v", err)
	}
}

func TestValidateCreateRequest_ZeroWeight(t *testing.T) {
	req := CreateRequest{
		Name: "Zero Weight",
		Entries: entries(
			entryPair{symbol: "AAPL", pct: 100000}, // 1000.00%
			entryPair{symbol: "MSFT", pct: 0},
		),
	}
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrInvalidWeight) {
		t.Errorf("expected ErrInvalidWeight, got %v", err)
	}
}

func TestValidateCreateRequest_NegativeWeight(t *testing.T) {
	req := CreateRequest{
		Name: "Negative Weight",
		Entries: entries(
			entryPair{symbol: "AAPL", pct: 12000},
			entryPair{symbol: "MSFT", pct: -2000},
		),
	}
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrInvalidWeight) {
		t.Errorf("expected ErrInvalidWeight, got %v", err)
	}
}

func TestValidateCreateRequest_DuplicateSymbol(t *testing.T) {
	req := CreateRequest{
		Name: "Duplicate",
		Entries: entries(
			entryPair{symbol: "AAPL", pct: 5000},
			entryPair{symbol: "AAPL", pct: 5000},
		),
	}
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrDuplicateSymbol) {
		t.Errorf("expected ErrDuplicateSymbol, got %v", err)
	}
}

func TestValidateCreateRequest_WeightSumNot100(t *testing.T) {
	tests := []struct {
		name string
		req  CreateRequest
	}{
		{
			name: "sum too low",
			req: CreateRequest{
				Name: "Low",
				Entries: entries(
					entryPair{symbol: "AAPL", pct: 3000},
					entryPair{symbol: "MSFT", pct: 3000},
				),
			},
		},
		{
			name: "sum too high",
			req: CreateRequest{
				Name: "High",
				Entries: entries(
					entryPair{symbol: "AAPL", pct: 6000},
					entryPair{symbol: "MSFT", pct: 6000},
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateRequest(tt.req)
			if err == nil {
				t.Error("expected error for weight sum != 100, got nil")
			}
			// Error message should include total and delta.
			if !strings.Contains(err.Error(), "current total:") {
				t.Errorf("expected error message with total/delta, got: %v", err)
			}
		})
	}
}

// --- ValidateUpdateRequest Tests ---

func TestValidateUpdateRequest_Valid(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateRequest
	}{
		{
			name: "update name only",
			req: UpdateRequest{
				Name:    strPtr("New Name"),
				Entries: entries(entryPair{symbol: "AAPL", pct: 10000}),
			},
		},
		{
			name: "update entries only",
			req: UpdateRequest{
				Entries: entries(
					entryPair{symbol: "AAPL", pct: 4000},
					entryPair{symbol: "MSFT", pct: 6000},
				),
			},
		},
		{
			name: "update both",
			req: UpdateRequest{
				Name: strPtr("Updated"),
				Entries: entries(
					entryPair{symbol: "GOOGL", pct: 5000},
					entryPair{symbol: "AMZN", pct: 5000},
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateRequest(tt.req)
			if err != nil {
				t.Errorf("expected nil, got %v", err)
			}
		})
	}
}

func TestValidateUpdateRequest_InvalidName(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateRequest
	}{
		{
			name: "empty name",
			req: UpdateRequest{
				Name:    strPtr(""),
				Entries: entries(entryPair{symbol: "AAPL", pct: 10000}),
			},
		},
		{
			name: "name too long",
			req: UpdateRequest{
				Name:    strPtr(strings.Repeat("a", 101)),
				Entries: entries(entryPair{symbol: "AAPL", pct: 10000}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateRequest(tt.req)
			if !errors.Is(err, ErrInvalidName) {
				t.Errorf("expected ErrInvalidName, got %v", err)
			}
		})
	}
}

func TestValidateUpdateRequestEmptyEntries(t *testing.T) {
	req := UpdateRequest{
		Name:    strPtr("Still Named"),
		Entries: []ModelPortfolioEntry{},
	}
	err := ValidateUpdateRequest(req)
	if !errors.Is(err, ErrEmptyEntries) {
		t.Errorf("expected ErrEmptyEntries, got %v", err)
	}
}

func TestValidateUpdateRequest_WeightSumNot100(t *testing.T) {
	req := UpdateRequest{
		Name:    strPtr("Bad"),
		Entries: entries(entryPair{symbol: "AAPL", pct: 3000}, entryPair{symbol: "MSFT", pct: 3000}),
	}
	err := ValidateUpdateRequest(req)
	if err == nil {
		t.Error("expected error for weight sum != 100, got nil")
	}
}

// --- ValidateEntries Tests ---

func TestValidateEntries_Tolerance(t *testing.T) {
	// Exactly 100% — should pass.
	req := entries(entryPair{symbol: "A", pct: 5000}, entryPair{symbol: "B", pct: 5000})
	err := ValidateEntries(req)
	if err != nil {
		t.Errorf("expected nil for exactly 100%%, got %v", err)
	}

	// 100.01% — exactly at 0.01% tolerance boundary — should pass.
	req = entries(entryPair{symbol: "A", pct: 5001}, entryPair{symbol: "B", pct: 5000})
	err = ValidateEntries(req)
	if err != nil {
		t.Errorf("expected nil for 100.01%% (at tolerance), got %v", err)
	}

	// 100.02% — exceeds 0.01% tolerance — should fail.
	req = entries(entryPair{symbol: "A", pct: 5001}, entryPair{symbol: "B", pct: 5001})
	err = ValidateEntries(req)
	if err == nil {
		t.Error("expected error for 100.02%% (exceeds tolerance), got nil")
	}

	// 99.99% — exactly at 0.01% tolerance boundary — should pass.
	req = entries(entryPair{symbol: "A", pct: 4999}, entryPair{symbol: "B", pct: 5000})
	err = ValidateEntries(req)
	if err != nil {
		t.Errorf("expected nil for 99.99%% (at tolerance), got %v", err)
	}

	// 99.98% — exceeds 0.01% tolerance — should fail.
	req = entries(entryPair{symbol: "A", pct: 4999}, entryPair{symbol: "B", pct: 4999})
	err = ValidateEntries(req)
	if err == nil {
		t.Error("expected error for 99.98%% (exceeds tolerance), got nil")
	}
}

func TestValidateEntries_SingleEntry100(t *testing.T) {
	req := entries(entryPair{symbol: "VOO", pct: 10000})
	err := ValidateEntries(req)
	if err != nil {
		t.Errorf("expected nil for single entry at 100%%, got %v", err)
	}
}

func TestValidateEntries_SingleEntryNot100(t *testing.T) {
	// 99.90% — clearly outside 0.01% tolerance — should fail.
	req := entries(entryPair{symbol: "VOO", pct: 9990})
	err := ValidateEntries(req)
	if err == nil {
		t.Error("expected error for single entry at 99.90%%, got nil")
	}
}

// --- Helpers ---

func strPtr(s string) *string {
	return &s
}
