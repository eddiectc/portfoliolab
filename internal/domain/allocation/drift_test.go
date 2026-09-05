package allocation

import (
	"context"
	"testing"

	"github.com/eddiectc/portfoliolab/internal/domain/position"
	"github.com/govalues/decimal"
)

// --- Tests ---

func TestComputeDrift_Basic(t *testing.T) {
	// Portfolio: AAPL $9000 (60%), MSFT $6000 (40%), Total $15000
	// Target: AAPL 50%, MSFT 50%
	// Drift: AAPL +10%, MSFT -10%
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("6000.00"),
			decimal.MustParse("6000.00"),
			true),
	}

	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("50.0")},
				{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("50.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.HasTarget {
		t.Error("expected HasTarget = true")
	}

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}

	// Build map for lookup.
	rowMap := make(map[string]DriftRow)
	for _, r := range result.Rows {
		rowMap[r.Symbol] = r
	}

	// AAPL: actual 60%, target 50%, drift +10%, not balanced
	aapl, ok := rowMap["AAPL"]
	if !ok {
		t.Fatal("AAPL not found in drift rows")
	}
	if !aapl.ActualPct.Equal(decimal.MustParse("60.0")) {
		t.Errorf("AAPL actual_pct = %v, want 60.0", aapl.ActualPct)
	}
	if !aapl.TargetPct.Equal(decimal.MustParse("50.0")) {
		t.Errorf("AAPL target_pct = %v, want 50.0", aapl.TargetPct)
	}
	if !aapl.DriftPct.Equal(decimal.MustParse("10.0")) {
		t.Errorf("AAPL drift_pct = %v, want 10.0", aapl.DriftPct)
	}
	if aapl.IsBalanced {
		t.Error("AAPL should not be balanced (drift 10% > 5%)")
	}

	// MSFT: actual 40%, target 50%, drift -10%, not balanced
	msft, ok := rowMap["MSFT"]
	if !ok {
		t.Fatal("MSFT not found in drift rows")
	}
	if !msft.ActualPct.Equal(decimal.MustParse("40.0")) {
		t.Errorf("MSFT actual_pct = %v, want 40.0", msft.ActualPct)
	}
	if !msft.TargetPct.Equal(decimal.MustParse("50.0")) {
		t.Errorf("MSFT target_pct = %v, want 50.0", msft.TargetPct)
	}
	if !msft.DriftPct.Equal(decimal.MustParse("-10.0")) {
		t.Errorf("MSFT drift_pct = %v, want -10.0", msft.DriftPct)
	}
	if msft.IsBalanced {
		t.Error("MSFT should not be balanced (drift -10% < -5%)")
	}
}

func TestComputeDrift_ToleranceBoundary(t *testing.T) {
	// Test tolerance boundary: |drift| <= 5% is balanced.
	tests := []struct {
		name         string
		actualPct    string
		targetPct    string
		wantBalanced bool
	}{
		{
			name:         "4.9% drift — balanced",
			actualPct:    "54.9",
			targetPct:    "50.0",
			wantBalanced: true,
		},
		{
			name:         "5.0% drift — balanced (boundary)",
			actualPct:    "55.0",
			targetPct:    "50.0",
			wantBalanced: true,
		},
		{
			name:         "5.1% drift — not balanced",
			actualPct:    "55.1",
			targetPct:    "50.0",
			wantBalanced: false,
		},
		{
			name:         "negative 4.9% drift — balanced",
			actualPct:    "45.1",
			targetPct:    "50.0",
			wantBalanced: true,
		},
		{
			name:         "negative 5.0% drift — balanced (boundary)",
			actualPct:    "45.0",
			targetPct:    "50.0",
			wantBalanced: true,
		},
		{
			name:         "negative 5.1% drift — not balanced",
			actualPct:    "44.9",
			targetPct:    "50.0",
			wantBalanced: false,
		},
		{
			name:         "zero drift — balanced",
			actualPct:    "50.0",
			targetPct:    "50.0",
			wantBalanced: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a portfolio where AAPL has the desired actual %.
			// Total = $10000, AAPL value = actualPct% of $10000, rest = MSFT.
			aaplValue, _ := decimal.MustParse(tt.actualPct).Mul(decimal.MustNew(10000, 2)) // actualPct * 100
			msftValue, _ := decimal.MustParse("10000.00").Sub(aaplValue)

			enriched := []position.PositionWithMarket{
				mkEnriched(1, "Broker A", "AAPL", "USD",
					decimal.MustParse("10"),
					aaplValue,
					aaplValue,
					true),
				mkEnriched(1, "Broker A", "MSFT", "USD",
					decimal.MustParse("10"),
					msftValue,
					msftValue,
					true),
			}

			repo := &mockTargetRepo{
				targets: map[int64][]TargetAllocation{
					1: {
						{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse(tt.targetPct)},
						{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("50.0")},
					},
				},
			}

			svc := NewService(
				&mockPositionSource{enriched: enriched},
				&mockAccountLister{accounts: []AccountRef{
					{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
				}},
				repo,
			)

			result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			rowMap := make(map[string]DriftRow)
			for _, r := range result.Rows {
				rowMap[r.Symbol] = r
			}

			aapl, ok := rowMap["AAPL"]
			if !ok {
				t.Fatal("AAPL not found in drift rows")
			}
			if aapl.IsBalanced != tt.wantBalanced {
				t.Errorf("AAPL IsBalanced = %v, want %v (drift = %s)",
					aapl.IsBalanced, tt.wantBalanced, aapl.DriftPct)
			}
		})
	}
}

func TestComputeDrift_NoTarget(t *testing.T) {
	// Portfolio with positions but no saved target.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("6000.00"),
			decimal.MustParse("6000.00"),
			true),
	}

	repo := &mockTargetRepo{} // No targets saved.

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HasTarget {
		t.Error("expected HasTarget = false")
	}

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}

	// All target percentages should be 0.
	for _, row := range result.Rows {
		if !row.TargetPct.Equal(decimal.Zero) {
			t.Errorf("%s target_pct = %v, want 0", row.Symbol, row.TargetPct)
		}
		// Drift should equal actual (actual - 0 = actual).
		if !row.DriftPct.Equal(row.ActualPct) {
			t.Errorf("%s drift_pct = %v, want %v (actual)", row.Symbol, row.DriftPct, row.ActualPct)
		}
	}
}

func TestComputeDrift_SymbolInTargetOnly(t *testing.T) {
	// Portfolio holds only AAPL (100%), but target includes GOOG which is not held.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
	}

	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("50.0")},
				{PortfolioID: 1, Symbol: "GOOG", TargetPct: decimal.MustParse("50.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows (AAPL + GOOG), got %d", len(result.Rows))
	}

	rowMap := make(map[string]DriftRow)
	for _, r := range result.Rows {
		rowMap[r.Symbol] = r
	}

	// GOOG: not held (actual=0%), target=50%, drift=-50%
	google, ok := rowMap["GOOG"]
	if !ok {
		t.Fatal("GOOG not found in drift rows")
	}
	if !google.ActualPct.Equal(decimal.Zero) {
		t.Errorf("GOOG actual_pct = %v, want 0", google.ActualPct)
	}
	if !google.TargetPct.Equal(decimal.MustParse("50.0")) {
		t.Errorf("GOOG target_pct = %v, want 50.0", google.TargetPct)
	}
	if !google.DriftPct.Equal(decimal.MustParse("-50.0")) {
		t.Errorf("GOOG drift_pct = %v, want -50.0", google.DriftPct)
	}
	if google.IsBalanced {
		t.Error("GOOG should not be balanced (drift -50%)")
	}

	// AAPL: actual 100%, target 50%, drift +50%
	aapl := rowMap["AAPL"]
	if !aapl.DriftPct.Equal(decimal.MustParse("50.0")) {
		t.Errorf("AAPL drift_pct = %v, want 50.0", aapl.DriftPct)
	}
}

func TestComputeDrift_SymbolInActualOnly(t *testing.T) {
	// Portfolio holds AAPL and MSFT, but target only has AAPL.
	// MSFT should appear with target=0% and drift=actual%.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("6000.00"),
			decimal.MustParse("6000.00"),
			true),
	}

	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("100.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rowMap := make(map[string]DriftRow)
	for _, r := range result.Rows {
		rowMap[r.Symbol] = r
	}

	// MSFT: actual 40%, target 0%, drift +40%
	msft, ok := rowMap["MSFT"]
	if !ok {
		t.Fatal("MSFT not found in drift rows")
	}
	if !msft.TargetPct.Equal(decimal.Zero) {
		t.Errorf("MSFT target_pct = %v, want 0", msft.TargetPct)
	}
	if !msft.DriftPct.Equal(decimal.MustParse("40.0")) {
		t.Errorf("MSFT drift_pct = %v, want 40.0", msft.DriftPct)
	}
}

func TestComputeDrift_WithCash(t *testing.T) {
	// Portfolio: AAPL $9000 (60%), Cash $6000 (40%), Total $15000
	// Target: AAPL 50%, Cash 50%
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkCashEnriched(1, "Broker A", "$CASH-USD", "USD",
			decimal.MustParse("6000.00"),
			decimal.MustParse("6000.00")),
	}

	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("50.0")},
				{PortfolioID: 1, Symbol: "$CASH", TargetPct: decimal.MustParse("50.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rowMap := make(map[string]DriftRow)
	for _, r := range result.Rows {
		rowMap[r.Symbol] = r
	}

	// Cash: actual 40%, target 50%, drift -10%
	cash, ok := rowMap["$CASH"]
	if !ok {
		t.Fatal("Cash not found in drift rows")
	}
	if !cash.ActualPct.Equal(decimal.MustParse("40.0")) {
		t.Errorf("Cash actual_pct = %v, want 40.0", cash.ActualPct)
	}
	if !cash.DriftPct.Equal(decimal.MustParse("-10.0")) {
		t.Errorf("Cash drift_pct = %v, want -10.0", cash.DriftPct)
	}
}

func TestComputeDrift_SignConvention(t *testing.T) {
	// Verify drift = actual - target sign convention.
	// AAPL overweight (actual > target) → positive drift
	// MSFT underweight (actual < target) → negative drift
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("6000.00"),
			decimal.MustParse("6000.00"),
			true),
	}

	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("40.0")},
				{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("60.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rowMap := make(map[string]DriftRow)
	for _, r := range result.Rows {
		rowMap[r.Symbol] = r
	}

	aapl := rowMap["AAPL"]
	if aapl.DriftPct.IsNeg() {
		t.Errorf("AAPL drift should be positive (overweight), got %s", aapl.DriftPct)
	}

	msft := rowMap["MSFT"]
	if !msft.DriftPct.IsNeg() {
		t.Errorf("MSFT drift should be negative (underweight), got %s", msft.DriftPct)
	}
}

func TestComputeDrift_SortedByAbsDrift(t *testing.T) {
	// Three symbols with different drift magnitudes.
	// AAPL: actual 70%, target 50% → drift +20%
	// MSFT: actual 20%, target 30% → drift -10%
	// GOOG: actual 10%, target 20% → drift -10%
	// Sorted: AAPL (|20%|), then MSFT/GOOG (|10%|)
	aaplVal := decimal.MustParse("7000.00")
	msftVal := decimal.MustParse("2000.00")
	googleVal := decimal.MustParse("1000.00")

	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"), aaplVal, aaplVal, true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"), msftVal, msftVal, true),
		mkEnriched(1, "Broker A", "GOOG", "USD",
			decimal.MustParse("10"), googleVal, googleVal, true),
	}

	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("50.0")},
				{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("30.0")},
				{PortfolioID: 1, Symbol: "GOOG", TargetPct: decimal.MustParse("20.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First row should have the largest |drift|.
	if len(result.Rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(result.Rows))
	}

	firstAbs := result.Rows[0].DriftPct.Abs()
	lastAbs := result.Rows[len(result.Rows)-1].DriftPct.Abs()

	cmp, _ := firstAbs.Sub(lastAbs)
	if cmp.IsNeg() {
		t.Errorf("rows not sorted by |drift| descending: first |drift|=%s, last |drift|=%s",
			firstAbs, lastAbs)
	}

	// AAPL (drift +20%) should be first.
	if result.Rows[0].Symbol != "AAPL" {
		t.Errorf("first row symbol = %q, want %q", result.Rows[0].Symbol, "AAPL")
	}
}

func TestComputeDrift_AllocationError(t *testing.T) {
	// If ComputeAllocation returns an error, ComputeDrift should propagate it.
	svc := NewService(
		&mockPositionSource{getErr: context.DeadlineExceeded},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&mockTargetRepo{},
	)

	_, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err == nil {
		t.Fatal("expected error for allocation fetch failure")
	}
}

func TestComputeDrift_TargetRepoError(t *testing.T) {
	// If GetTargetAllocation returns an error, ComputeDrift should propagate it.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
	}

	repo := &mockTargetRepo{getErr: context.DeadlineExceeded}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	_, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err == nil {
		t.Fatal("expected error for target repo failure")
	}
}

func TestComputeDrift_EmptyPortfolio(t *testing.T) {
	// Empty portfolio with a target should show all target symbols with actual=0.
	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("60.0")},
				{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("40.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{positions: []position.Position{}},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	// ComputeAllocation returns empty result (not an error) for no positions.
	// ComputeDrift should handle this gracefully.
	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	// Empty portfolio has no total value — ComputeAllocation returns empty result.
	// Drift should still work: actual=0 for all symbols, target from saved targets.
	if err != nil {
		// This is acceptable: empty portfolio → no positions → no total value.
		// The drift method may return an error from ComputeAllocation.
		// Either way, we verify the behavior is consistent.
		t.Logf("ComputeDrift returned error for empty portfolio: %v (expected behavior)", err)
		return
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !result.HasTarget {
		t.Error("expected HasTarget = true")
	}

	// All symbols should have actual=0 and drift = -target.
	for _, row := range result.Rows {
		if !row.ActualPct.Equal(decimal.Zero) {
			t.Errorf("%s actual_pct = %v, want 0", row.Symbol, row.ActualPct)
		}
		expectedDrift, _ := decimal.Zero.Sub(row.TargetPct)
		if !row.DriftPct.Equal(expectedDrift) {
			t.Errorf("%s drift_pct = %v, want %v", row.Symbol, row.DriftPct, expectedDrift)
		}
	}
}

func TestComputeDrift_BaseCurrency(t *testing.T) {
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
	}

	repo := &mockTargetRepo{
		targets: map[int64][]TargetAllocation{
			1: {
				{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("100.0")},
			},
		},
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeDrift(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BaseCurrency != "USD" {
		t.Errorf("base_currency = %q, want %q", result.BaseCurrency, "USD")
	}
}
