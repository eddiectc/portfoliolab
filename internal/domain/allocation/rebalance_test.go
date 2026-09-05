package allocation

import (
	"context"
	"strings"
	"testing"

	"github.com/eddiectc/portfoliolab/internal/domain/position"
	"github.com/govalues/decimal"
)

// --- Tests ---

func TestComputeRebalancingSuggestions_Basic(t *testing.T) {
	// Portfolio: AAPL $9000 (60%), MSFT $6000 (40%), Total $15000
	// Target: AAPL 50%, MSFT 50%
	// Drift: AAPL +10% (SELL), MSFT -10% (BUY)
	// AAPL: sell 10% of $15000 = $1500, at $150/share = 10 shares
	// MSFT: buy 10% of $15000 = $1500, at $600/share = 2.5 shares
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),      // 60 shares
			decimal.MustParse("9000.00"), // 60 * $150
			decimal.MustParse("9000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"),      // 10 shares
			decimal.MustParse("6000.00"), // 10 * $600
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsBalanced {
		t.Error("expected IsBalanced = false")
	}

	if len(result.Suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(result.Suggestions))
	}

	// Build map for lookup.
	sugMap := make(map[string]RebalanceSuggestion)
	for _, s := range result.Suggestions {
		sugMap[s.Symbol] = s
	}

	// AAPL: SELL 10 shares, $1500
	aapl, ok := sugMap["AAPL"]
	if !ok {
		t.Fatal("AAPL not found in suggestions")
	}
	if aapl.Direction != "sell" {
		t.Errorf("AAPL direction = %q, want %q", aapl.Direction, "sell")
	}
	if !aapl.Shares.Equal(decimal.MustParse("10.00")) {
		t.Errorf("AAPL shares = %v, want 10.00", aapl.Shares)
	}
	if !aapl.DollarValue.Equal(decimal.MustParse("1500.00")) {
		t.Errorf("AAPL dollar_value = %v, want 1500.00", aapl.DollarValue)
	}
	if !aapl.DriftReduction.Equal(decimal.MustParse("10.0")) {
		t.Errorf("AAPL drift_reduction = %v, want 10.0", aapl.DriftReduction)
	}

	// MSFT: BUY 2.5 shares, $1500
	msft, ok := sugMap["MSFT"]
	if !ok {
		t.Fatal("MSFT not found in suggestions")
	}
	if msft.Direction != "buy" {
		t.Errorf("MSFT direction = %q, want %q", msft.Direction, "buy")
	}
	if !msft.Shares.Equal(decimal.MustParse("2.50")) {
		t.Errorf("MSFT shares = %v, want 2.50", msft.Shares)
	}
	if !msft.DollarValue.Equal(decimal.MustParse("1500.00")) {
		t.Errorf("MSFT dollar_value = %v, want 1500.00", msft.DollarValue)
	}

	// Total dollar value should be $3000.
	if !result.TotalDollarValue.Equal(decimal.MustParse("3000.00")) {
		t.Errorf("total_dollar_value = %v, want 3000.00", result.TotalDollarValue)
	}

	// Sorted by |drift| descending — both have same drift, order is stable.
	// Verify base currency.
	if result.BaseCurrency != "USD" {
		t.Errorf("base_currency = %q, want %q", result.BaseCurrency, "USD")
	}
}

func TestComputeRebalancingSuggestions_ToleranceBoundary(t *testing.T) {
	tests := []struct {
		name           string
		actualPct      string
		targetPct      string
		wantSuggestion bool
	}{
		{
			name:           "4.9% drift — no suggestion",
			actualPct:      "54.9",
			targetPct:      "50.0",
			wantSuggestion: false,
		},
		{
			name:           "5.0% drift — no suggestion (boundary)",
			actualPct:      "55.0",
			targetPct:      "50.0",
			wantSuggestion: false,
		},
		{
			name:           "5.1% drift — suggestion",
			actualPct:      "55.1",
			targetPct:      "50.0",
			wantSuggestion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aaplValue, _ := decimal.MustParse(tt.actualPct).Mul(decimal.MustNew(10000, 2)) // actualPct * 100
			msftValue, _ := decimal.MustParse("10000.00").Sub(aaplValue)

			enriched := []position.PositionWithMarket{
				mkEnriched(1, "Broker A", "AAPL", "USD",
					decimal.MustParse("10"), aaplValue, aaplValue, true),
				mkEnriched(1, "Broker A", "MSFT", "USD",
					decimal.MustParse("10"), msftValue, msftValue, true),
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

			result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			hasAAPL := false
			for _, s := range result.Suggestions {
				if s.Symbol == "AAPL" {
					hasAAPL = true
					break
				}
			}

			if hasAAPL != tt.wantSuggestion {
				t.Errorf("AAPL suggestion = %v, want %v", hasAAPL, tt.wantSuggestion)
			}
		})
	}
}

func TestComputeRebalancingSuggestions_NoDrift(t *testing.T) {
	// All symbols within tolerance → balanced.
	// AAPL 50.5%, MSFT 49.5%, Target: AAPL 50%, MSFT 50%
	// Drift: AAPL +0.5%, MSFT -0.5% — both within 5% tolerance.
	aaplVal := decimal.MustParse("5050.00")
	msftVal := decimal.MustParse("4950.00")

	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"), aaplVal, aaplVal, true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("10"), msftVal, msftVal, true),
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsBalanced {
		t.Error("expected IsBalanced = true")
	}

	if len(result.Suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(result.Suggestions))
	}
}

func TestComputeRebalancingSuggestions_SymbolNotYetHeld(t *testing.T) {
	// Portfolio holds only AAPL (100%), target includes GOOG at 50%.
	// GOOG is not held → buy full target amount.
	// Target: 50% of $1500 = $750. GOOG price = $150 → 5 shares.
	googPrice := decimal.MustParse("150.00")

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
		&mockPositionSource{
			enriched: enriched,
			prices:   map[string]*decimal.Decimal{"GOOG": &googPrice},
		},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sugMap := make(map[string]RebalanceSuggestion)
	for _, s := range result.Suggestions {
		sugMap[s.Symbol] = s
	}

	// GOOG: BUY 5 shares, $750
	google, ok := sugMap["GOOG"]
	if !ok {
		t.Fatal("GOOG not found in suggestions")
	}
	if google.Direction != "buy" {
		t.Errorf("GOOG direction = %q, want %q", google.Direction, "buy")
	}
	if !google.Shares.Equal(decimal.MustParse("5.00")) {
		t.Errorf("GOOG shares = %v, want 5.00", google.Shares)
	}
	if !google.DollarValue.Equal(decimal.MustParse("750.00")) {
		t.Errorf("GOOG dollar_value = %v, want 750.00", google.DollarValue)
	}

	// AAPL: SELL (actual 100%, target 50%, drift +50%)
	aapl, ok := sugMap["AAPL"]
	if !ok {
		t.Fatal("AAPL not found in suggestions")
	}
	if aapl.Direction != "sell" {
		t.Errorf("AAPL direction = %q, want %q", aapl.Direction, "sell")
	}
}

func TestComputeRebalancingSuggestions_ZeroTarget(t *testing.T) {
	// Portfolio holds AAPL (60%), MSFT (40%).
	// Target: AAPL 100%, MSFT 0%.
	// MSFT: actual 40%, target 0%, drift +40% → SELL full holding.
	// MSFT: sell 40% of $15000 = $6000, at $600/share = 10 shares.
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sugMap := make(map[string]RebalanceSuggestion)
	for _, s := range result.Suggestions {
		sugMap[s.Symbol] = s
	}

	// MSFT: SELL 10 shares, $6000
	msft, ok := sugMap["MSFT"]
	if !ok {
		t.Fatal("MSFT not found in suggestions")
	}
	if msft.Direction != "sell" {
		t.Errorf("MSFT direction = %q, want %q", msft.Direction, "sell")
	}
	if !msft.Shares.Equal(decimal.MustParse("10.00")) {
		t.Errorf("MSFT shares = %v, want 10.00", msft.Shares)
	}
	if !msft.DollarValue.Equal(decimal.MustParse("6000.00")) {
		t.Errorf("MSFT dollar_value = %v, want 6000.00", msft.DollarValue)
	}

	// AAPL: BUY (actual 60%, target 100%, drift -40%)
	aapl, ok := sugMap["AAPL"]
	if !ok {
		t.Fatal("AAPL not found in suggestions")
	}
	if aapl.Direction != "buy" {
		t.Errorf("AAPL direction = %q, want %q", aapl.Direction, "buy")
	}
}

func TestComputeRebalancingSuggestions_MissingMarketData(t *testing.T) {
	// Portfolio holds AAPL (with market data) and GOOG (no market data).
	// Target: AAPL 50%, GOOG 50%.
	// GOOG has no market data → warning, excluded.
	// AAPL: actual 100% (only symbol with data), target 50%, drift +50% → SELL.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
		mkEnriched(1, "Broker A", "GOOG", "USD",
			decimal.MustParse("5"),
			decimal.Zero,
			decimal.Zero,
			false), // No market data
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// GOOG should not be in suggestions.
	for _, s := range result.Suggestions {
		if s.Symbol == "GOOG" {
			t.Error("GOOG should not be in suggestions (no market data)")
		}
	}

	// Should have a warning about GOOG.
	foundWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "GOOG") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning about GOOG missing market data")
	}
}

func TestComputeRebalancingSuggestions_CashDrift(t *testing.T) {
	// Portfolio: AAPL $9000 (60%), Cash $6000 (40%), Total $15000
	// Target: AAPL 50%, Cash 50%
	// Cash drift: actual 40%, target 50%, drift -10% → excluded with warning
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cash should not be in suggestions.
	for _, s := range result.Suggestions {
		if s.Symbol == "$CASH" {
			t.Error("Cash should not be in suggestions")
		}
	}

	// Should have a warning about cash drift.
	foundWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "cash") && strings.Contains(w, "drift") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning about cash drift")
	}

	// AAPL should be in suggestions (actual 60%, target 50%, drift +10%)
	foundAAPL := false
	for _, s := range result.Suggestions {
		if s.Symbol == "AAPL" && s.Direction == "sell" {
			foundAAPL = true
			break
		}
	}
	if !foundAAPL {
		t.Error("expected AAPL sell suggestion")
	}
}

func TestComputeRebalancingSuggestions_SortedByDrift(t *testing.T) {
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All three have |drift| > 5%, so all should be in suggestions.
	if len(result.Suggestions) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(result.Suggestions))
	}

	// First should have the largest drift reduction.
	if result.Suggestions[0].Symbol != "AAPL" {
		t.Errorf("first suggestion symbol = %q, want %q", result.Suggestions[0].Symbol, "AAPL")
	}

	// Verify descending order.
	for i := 1; i < len(result.Suggestions); i++ {
		cmp, _ := result.Suggestions[i-1].DriftReduction.Sub(result.Suggestions[i].DriftReduction)
		if cmp.IsNeg() {
			t.Errorf("suggestions not sorted by drift_reduction descending at index %d", i)
		}
	}
}

func TestComputeRebalancingSuggestions_NoTarget(t *testing.T) {
	// No saved target → drift shows actual% only with target=0%.
	// All symbols will have drift = actual% (since target=0%).
	// If actual% > 5%, suggestion will be generated (sell everything to hit 0% target).
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1500.00"),
			decimal.MustParse("1500.00"),
			true),
	}

	svc := NewService(
		&mockPositionSource{enriched: enriched},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&mockTargetRepo{}, // No targets.
	)

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AAPL at 100% with target 0% → drift +100% → SELL all
	if len(result.Suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(result.Suggestions))
	}

	if result.Suggestions[0].Symbol != "AAPL" {
		t.Errorf("symbol = %q, want %q", result.Suggestions[0].Symbol, "AAPL")
	}
	if result.Suggestions[0].Direction != "sell" {
		t.Errorf("direction = %q, want %q", result.Suggestions[0].Direction, "sell")
	}
}

func TestComputeRebalancingSuggestions_AllocationError(t *testing.T) {
	// If ComputeAllocation returns an error, rebalancing should propagate it.
	svc := NewService(
		&mockPositionSource{getErr: context.DeadlineExceeded},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		&mockTargetRepo{},
	)

	_, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err == nil {
		t.Fatal("expected error for allocation fetch failure")
	}
}

func TestComputeRebalancingSuggestions_ShareRounding(t *testing.T) {
	// AAPL $9000 (60%), MSFT $6000 (40%), Total $15000
	// Target: AAPL 50%, MSFT 50%
	// AAPL price = $150, drift = +10% → $1500 / $150 = 10.00 shares (exact)
	// MSFT price = $240, drift = -10% → $1500 / $240 = 6.25 shares (exact)
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("60"),
			decimal.MustParse("9000.00"),
			decimal.MustParse("9000.00"),
			true),
		mkEnriched(1, "Broker A", "MSFT", "USD",
			decimal.MustParse("25"),      // 25 shares
			decimal.MustParse("6000.00"), // 25 * $240
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sugMap := make(map[string]RebalanceSuggestion)
	for _, s := range result.Suggestions {
		sugMap[s.Symbol] = s
	}

	// AAPL: price derived from allocation = $9000/60 = $150
	// $1500 / $150 = 10.00
	aapl := sugMap["AAPL"]
	if !aapl.Shares.Equal(decimal.MustParse("10.00")) {
		t.Errorf("AAPL shares = %v, want 10.00", aapl.Shares)
	}

	// MSFT: price derived from allocation = $6000/25 = $240
	// $1500 / $240 = 6.25
	msft := sugMap["MSFT"]
	if !msft.Shares.Equal(decimal.MustParse("6.25")) {
		t.Errorf("MSFT shares = %v, want 6.25", msft.Shares)
	}

}

func TestComputeRebalancingSuggestions_MarketPriceLookup(t *testing.T) {
	// Symbol in target but not yet held — requires GetMarketPrice.
	// AAPL held at 100%, GOOG not held (target 50%).
	// GOOG price from GetMarketPrice = $120.
	// GOOG: buy 50% of $1000 = $500 / $120 = 4.17 shares.
	googPrice := decimal.MustParse("120.00")

	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1000.00"),
			decimal.MustParse("1000.00"),
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
		&mockPositionSource{
			enriched: enriched,
			prices:   map[string]*decimal.Decimal{"GOOG": &googPrice},
		},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sugMap := make(map[string]RebalanceSuggestion)
	for _, s := range result.Suggestions {
		sugMap[s.Symbol] = s
	}

	google, ok := sugMap["GOOG"]
	if !ok {
		t.Fatal("GOOG not found in suggestions")
	}
	if google.Direction != "buy" {
		t.Errorf("GOOG direction = %q, want %q", google.Direction, "buy")
	}
	// $500 / $120 = 4.1666... → 4.17
	if !google.Shares.Equal(decimal.MustParse("4.17")) {
		t.Errorf("GOOG shares = %v, want 4.17", google.Shares)
	}
	if !google.DollarValue.Equal(decimal.MustParse("500.00")) {
		t.Errorf("GOOG dollar_value = %v, want 500.00", google.DollarValue)
	}
}

func TestComputeRebalancingSuggestions_MarketPriceUnavailable(t *testing.T) {
	// Symbol in target but not held, and GetMarketPrice returns nil.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1000.00"),
			decimal.MustParse("1000.00"),
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
		&mockPositionSource{enriched: enriched}, // No price for GOOG.
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// GOOG should not be in suggestions (no price available).
	for _, s := range result.Suggestions {
		if s.Symbol == "GOOG" {
			t.Error("GOOG should not be in suggestions (no market price)")
		}
	}

	// Should have a warning.
	if len(result.Warnings) == 0 {
		t.Error("expected warning about GOOG missing market data")
	}
}

func TestComputeRebalancingSuggestions_TotalDollarValue(t *testing.T) {
	// AAPL $9000 (60%), MSFT $6000 (40%), Total $15000
	// Target: AAPL 40%, MSFT 60%
	// AAPL: drift +20% → $3000 SELL
	// MSFT: drift -20% → $3000 BUY
	// Total dollar value: $6000
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

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.TotalDollarValue.Equal(decimal.MustParse("6000.00")) {
		t.Errorf("total_dollar_value = %v, want 6000.00", result.TotalDollarValue)
	}
}

func TestComputeRebalancingSuggestions_MarketPriceError(t *testing.T) {
	// Symbol in target but not held, and GetMarketPrice returns an error.
	// AAPL held at 100%, GOOG not held (target 50%).
	// GOOG price lookup fails → warning, excluded.
	enriched := []position.PositionWithMarket{
		mkEnriched(1, "Broker A", "AAPL", "USD",
			decimal.MustParse("10"),
			decimal.MustParse("1000.00"),
			decimal.MustParse("1000.00"),
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
		&mockPositionSource{
			enriched: enriched,
			priceErr: map[string]error{"GOOG": context.DeadlineExceeded},
		},
		&mockAccountLister{accounts: []AccountRef{
			{ID: 1, Name: "Broker A", PortfolioID: 1, PortfolioCurrency: "USD"},
		}},
		repo,
	)

	result, err := svc.ComputeRebalancingSuggestions(ctx, AllocationFilter{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// GOOG should not be in suggestions (price lookup error).
	for _, s := range result.Suggestions {
		if s.Symbol == "GOOG" {
			t.Error("GOOG should not be in suggestions (market price error)")
		}
	}

	// Should have a warning about GOOG.
	foundWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "GOOG") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning about GOOG market price error")
	}

	// AAPL should still be in suggestions (actual 100%, target 50% → SELL).
	foundAAPL := false
	for _, s := range result.Suggestions {
		if s.Symbol == "AAPL" && s.Direction == "sell" {
			foundAAPL = true
			break
		}
	}
	if !foundAAPL {
		t.Error("expected AAPL sell suggestion")
	}
}
