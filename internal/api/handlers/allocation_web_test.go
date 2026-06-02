package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/allocation"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

func TestRegisterRoutes_AllocationWeb(t *testing.T) {
	r := chi.NewRouter()
	handler := &AllocationWebHandler{}
	handler.RegisterRoutes(r)
	// Verify it doesn't panic
}

// Test that the allocation page template renders without panic (empty state).
func TestAllocationTemplate_EmptyState(t *testing.T) {
	renderer := newTestRenderer(t)

	data := allocationPageData{
		PageData:   web.PageData{Title: "Allocation"},
		Portfolios: []portfolio.Portfolio{},
		Filter:     AllocationFilter{},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(body, "Allocation") {
		t.Error("missing title")
	}
	if !strings.Contains(body, "No allocation data available") {
		t.Error("expected empty state message")
	}
}

// Test that the allocation page renders with data.
func TestAllocationTemplate_WithData(t *testing.T) {
	renderer := newTestRenderer(t)

	mvBase := decimal.MustParse("50000.00")
	totalValue := decimal.MustParse("150000.00")

	alloc := &allocation.AllocationResult{
		Rows: []allocation.AllocationRow{
			{
				Symbol:          "AAPL",
				MarketValue:     decimal.MustParse("50000.00"),
				MarketValueBase: &mvBase,
				AllocationPct:   decimal.MustParse("33.3"),
				Currency:        "USD",
				HasMarketData:   true,
				AccountBreakdown: []allocation.AccountBreakdown{
					{
						AccountID:       1,
						AccountName:     "Main",
						Quantity:        decimal.MustParse("100.00"),
						MarketValue:     decimal.MustParse("50000.00"),
						MarketValueBase: &mvBase,
						PctOfSymbol:     decimal.MustParse("100.0"),
					},
				},
			},
			{
				Symbol:          "MSFT",
				MarketValue:     decimal.MustParse("45000.00"),
				MarketValueBase: func() *decimal.Decimal { v := decimal.MustParse("45000.00"); return &v }(),
				AllocationPct:   decimal.MustParse("30.0"),
				Currency:        "USD",
				HasMarketData:   true,
			},
		},
		TotalValueBase: totalValue,
		BaseCurrency:   "USD",
		CashRow: &allocation.AllocationRow{
			Symbol:          "$CASH",
			MarketValue:     decimal.MustParse("55000.00"),
			MarketValueBase: func() *decimal.Decimal { v := decimal.MustParse("55000.00"); return &v }(),
			AllocationPct:   decimal.MustParse("36.7"),
			Currency:        "USD",
			HasMarketData:   true,
		},
		MarketDataAvailable: true,
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Allocation:        alloc,
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
		LastUpdatedText:   "2026-05-19 10:30:00 UTC",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	checkContains("total value", "150,000.00")
	checkContains("AAPL row", "AAPL")
	checkContains("AAPL value", "50,000.00")
	checkContains("AAPL pct", "33.30%")
	checkContains("MSFT row", "MSFT")
	checkContains("MSFT pct", "30.00%")
	checkContains("cash row", "$CASH")
	checkContains("cash pct", "36.70%")
	checkContains("last updated", "2026-05-19 10:30:00 UTC")
	checkContains("expand button", "expand-btn")
}

// Test that the allocation page renders with drift data.
func TestAllocationTemplate_WithDrift(t *testing.T) {
	renderer := newTestRenderer(t)

	mvBase := decimal.MustParse("50000.00")
	totalValue := decimal.MustParse("100000.00")

	alloc := &allocation.AllocationResult{
		Rows: []allocation.AllocationRow{
			{
				Symbol:          "AAPL",
				MarketValue:     decimal.MustParse("50000.00"),
				MarketValueBase: &mvBase,
				AllocationPct:   decimal.MustParse("50.0"),
				Currency:        "USD",
				HasMarketData:   true,
			},
		},
		TotalValueBase:      totalValue,
		BaseCurrency:        "USD",
		MarketDataAvailable: true,
	}

	drift := &allocation.DriftResult{
		Rows: []allocation.DriftRow{
			{
				Symbol:     "AAPL",
				ActualPct:  decimal.MustParse("50.0"),
				TargetPct:  decimal.MustParse("40.0"),
				DriftPct:   decimal.MustParse("10.0"),
				IsBalanced: false,
			},
			{
				Symbol:     "MSFT",
				ActualPct:  decimal.MustParse("0.0"),
				TargetPct:  decimal.MustParse("30.0"),
				DriftPct:   decimal.MustParse("-30.0"),
				IsBalanced: false,
			},
			{
				Symbol:     "$CASH",
				ActualPct:  decimal.MustParse("50.0"),
				TargetPct:  decimal.MustParse("30.0"),
				DriftPct:   decimal.MustParse("20.0"),
				IsBalanced: false,
			},
		},
		BaseCurrency: "USD",
		HasTarget:    true,
	}

	rebalance := &allocation.RebalanceResult{
		Suggestions: []allocation.RebalanceSuggestion{
			{
				Symbol:         "AAPL",
				Direction:      "sell",
				Shares:         decimal.MustParse("5.00"),
				DollarValue:    decimal.MustParse("10000.00"),
				DriftReduction: decimal.MustParse("10.0"),
			},
			{
				Symbol:         "MSFT",
				Direction:      "buy",
				Shares:         decimal.MustParse("10.00"),
				DollarValue:    decimal.MustParse("30000.00"),
				DriftReduction: decimal.MustParse("30.0"),
			},
		},
		BaseCurrency:     "USD",
		TotalDollarValue: decimal.MustParse("40000.00"),
		IsBalanced:       false,
	}

	targets := []allocation.TargetAllocation{
		{PortfolioID: 1, Symbol: "AAPL", TargetPct: decimal.MustParse("40.0")},
		{PortfolioID: 1, Symbol: "MSFT", TargetPct: decimal.MustParse("30.0")},
		{PortfolioID: 1, Symbol: "$CASH", TargetPct: decimal.MustParse("30.0")},
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Allocation:        alloc,
		Drift:             drift,
		Rebalance:         rebalance,
		Targets:           targets,
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	checkContains := func(label, text string) {
		t.Helper()
		if !strings.Contains(body, text) {
			t.Errorf("page missing %s: %q", label, text)
		}
	}

	// Drift table
	checkContains("drift section", "Target Allocation")
	checkContains("drift AAPL actual", "50.00%")
	checkContains("drift AAPL target", "40.00%")
	checkContains("drift AAPL drift", "10.0%")
	checkContains("off-target", "Off-target")

	// Rebalance section
	checkContains("rebalance section", "Rebalancing Suggestions")
	checkContains("sell direction", "sell")
	checkContains("buy direction", "buy")
	checkContains("total trade value", "40,000.00")

	// Target form
	checkContains("target form", "Target Allocation")
	checkContains("target AAPL", "AAPL")
	checkContains("save targets", "Save Targets")
	checkContains("delete targets", "Delete All Targets")
	checkContains("add symbol button", "Add Symbol")
}

// Test that the allocation page renders with balanced portfolio.
func TestAllocationTemplate_Balanced(t *testing.T) {
	renderer := newTestRenderer(t)

	drift := &allocation.DriftResult{
		Rows: []allocation.DriftRow{
			{
				Symbol:     "AAPL",
				ActualPct:  decimal.MustParse("40.0"),
				TargetPct:  decimal.MustParse("40.0"),
				DriftPct:   decimal.Zero,
				IsBalanced: true,
			},
		},
		BaseCurrency: "USD",
		HasTarget:    true,
	}

	rebalance := &allocation.RebalanceResult{
		Suggestions:      []allocation.RebalanceSuggestion{},
		BaseCurrency:     "USD",
		TotalDollarValue: decimal.Zero,
		IsBalanced:       true,
		Message:          "Portfolio is balanced",
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Drift:             drift,
		Rebalance:         rebalance,
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Balanced") {
		t.Error("expected balanced status")
	}
	if !strings.Contains(body, "Portfolio is balanced") {
		t.Error("expected balanced message in rebalance section")
	}
}

// Test that the allocation page renders with no target (empty target state).
func TestAllocationTemplate_NoTarget(t *testing.T) {
	renderer := newTestRenderer(t)

	drift := &allocation.DriftResult{
		Rows:         []allocation.DriftRow{},
		BaseCurrency: "USD",
		HasTarget:    false,
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Drift:             drift,
		Targets:           []allocation.TargetAllocation{},
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	if !strings.Contains(body, "No target allocation saved") {
		t.Error("expected no-target message")
	}
	if !strings.Contains(body, "Set Target Allocation") {
		t.Error("expected target form header")
	}
	// Should NOT show "Delete All Targets" when no target exists
	if strings.Contains(body, "Delete All Targets") {
		t.Error("should not show delete button when no target exists")
	}
}

// Test that the allocation page renders with error state.
func TestAllocationTemplate_ErrorState(t *testing.T) {
	renderer := newTestRenderer(t)

	data := allocationPageData{
		PageData:   web.PageData{Title: "Allocation"},
		Portfolios: []portfolio.Portfolio{},
		SaveError:  "An error occurred while computing allocation data.",
		Filter:     AllocationFilter{},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "An error occurred") {
		t.Error("expected error message in page")
	}
}

// Test that warnings are displayed.
func TestAllocationTemplate_WithWarnings(t *testing.T) {
	renderer := newTestRenderer(t)

	mvBase := decimal.MustParse("50000.00")
	alloc := &allocation.AllocationResult{
		Rows: []allocation.AllocationRow{
			{
				Symbol:          "AAPL",
				MarketValueBase: &mvBase,
				AllocationPct:   decimal.MustParse("100.0"),
				Currency:        "USD",
				HasMarketData:   true,
			},
		},
		TotalValueBase:      mvBase,
		BaseCurrency:        "USD",
		MarketDataAvailable: true,
		Warnings:            []string{"market data unavailable for 1 position(s): TEST"},
	}

	data := allocationPageData{
		PageData:     web.PageData{Title: "Allocation"},
		Allocation:   alloc,
		Portfolios:   []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		Filter:       AllocationFilter{},
		BaseCurrency: "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "market data unavailable") {
		t.Error("expected warning in page")
	}
}

// Test that the allocation page renders with drift warning.
func TestAllocationTemplate_WithDriftWarning(t *testing.T) {
	renderer := newTestRenderer(t)

	drift := &allocation.DriftResult{
		Rows:         []allocation.DriftRow{},
		BaseCurrency: "USD",
		HasTarget:    true,
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Drift:             drift,
		DriftWarning:      "Could not compute drift: zero total portfolio value",
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Could not compute drift") {
		t.Error("expected drift warning in page")
	}
	if !strings.Contains(body, "zero total portfolio value") {
		t.Error("expected error detail in drift warning")
	}
}

// Test that the allocation page renders with rebalance warning.
func TestAllocationTemplate_WithRebalanceWarning(t *testing.T) {
	renderer := newTestRenderer(t)

	drift := &allocation.DriftResult{
		Rows:         []allocation.DriftRow{},
		BaseCurrency: "USD",
		HasTarget:    true,
	}

	rebalance := &allocation.RebalanceResult{
		Suggestions:  []allocation.RebalanceSuggestion{},
		BaseCurrency: "USD",
		IsBalanced:   false,
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Drift:             drift,
		Rebalance:         rebalance,
		RebalanceWarning:  "Could not compute rebalancing suggestions: market data unavailable",
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Could not compute rebalancing suggestions") {
		t.Error("expected rebalance warning in page")
	}
}

// Test that the allocation page renders with target warning.
func TestAllocationTemplate_WithTargetWarning(t *testing.T) {
	renderer := newTestRenderer(t)

	drift := &allocation.DriftResult{
		Rows:         []allocation.DriftRow{},
		BaseCurrency: "USD",
		HasTarget:    false,
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Drift:             drift,
		TargetWarning:     "Could not load target allocation: database connection failed",
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Could not load target allocation") {
		t.Error("expected target warning in page")
	}
	if !strings.Contains(body, "database connection failed") {
		t.Error("expected error detail in target warning")
	}
}

// Test AllocationFilter.QueryParams.
func TestAllocationFilter_QueryParams(t *testing.T) {
	tests := []struct {
		name   string
		filter AllocationFilter
		want   string
	}{
		{
			name:   "empty filter",
			filter: AllocationFilter{},
			want:   "",
		},
		{
			name:   "single portfolio",
			filter: AllocationFilter{PortfolioIDs: []int64{1}},
			want:   "&portfolio_ids=1",
		},
		{
			name:   "multiple portfolios",
			filter: AllocationFilter{PortfolioIDs: []int64{1, 2, 3}},
			want:   "&portfolio_ids=1,2,3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.QueryParams()
			if got != tt.want {
				t.Errorf("QueryParams() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test parseWebAllocationFilter.
func TestParseWebAllocationFilter(t *testing.T) {
	tests := []struct {
		name  string
		query map[string][]string
		want  AllocationFilter
	}{
		{
			name:  "empty query",
			query: map[string][]string{},
			want:  AllocationFilter{},
		},
		{
			name:  "single portfolio",
			query: map[string][]string{"portfolio_ids": {"1"}},
			want:  AllocationFilter{PortfolioIDs: []int64{1}},
		},
		{
			name:  "multiple portfolios",
			query: map[string][]string{"portfolio_ids": {"1,2,3"}},
			want:  AllocationFilter{PortfolioIDs: []int64{1, 2, 3}},
		},
		{
			name:  "with spaces",
			query: map[string][]string{"portfolio_ids": {"1, 2, 3"}},
			want:  AllocationFilter{PortfolioIDs: []int64{1, 2, 3}},
		},
		{
			name:  "invalid IDs ignored",
			query: map[string][]string{"portfolio_ids": {"1,abc,3"}},
			want:  AllocationFilter{PortfolioIDs: []int64{1, 3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWebAllocationFilter(tt.query)
			if len(got.PortfolioIDs) != len(tt.want.PortfolioIDs) {
				t.Errorf("got %d IDs, want %d", len(got.PortfolioIDs), len(tt.want.PortfolioIDs))
				return
			}
			for i := range got.PortfolioIDs {
				if got.PortfolioIDs[i] != tt.want.PortfolioIDs[i] {
					t.Errorf("ID[%d] = %d, want %d", i, got.PortfolioIDs[i], tt.want.PortfolioIDs[i])
				}
			}
		})
	}
}

// Test selectedSinglePortfolio.
func TestSelectedSinglePortfolio(t *testing.T) {
	h := &AllocationWebHandler{}

	// No portfolios.
	got := h.selectedSinglePortfolio(AllocationFilter{})
	if got != "" {
		t.Errorf("empty filter = %q, want \"\"", got)
	}

	// Single portfolio.
	got = h.selectedSinglePortfolio(AllocationFilter{PortfolioIDs: []int64{5}})
	if got != "5" {
		t.Errorf("single = %q, want \"5\"", got)
	}

	// Multiple portfolios.
	got = h.selectedSinglePortfolio(AllocationFilter{PortfolioIDs: []int64{1, 2}})
	if got != "" {
		t.Errorf("multiple = %q, want \"\"", got)
	}
}

// Test serializeDriftData.
func TestSerializeDriftData(t *testing.T) {
	// Nil.
	got := serializeDriftData(nil)
	if got != "{}" {
		t.Errorf("nil = %q, want {}", got)
	}

	// Empty rows.
	got = serializeDriftData(&allocation.DriftResult{})
	if got != "{}" {
		t.Errorf("empty = %q, want {}", got)
	}

	// With data.
	drift := &allocation.DriftResult{
		Rows: []allocation.DriftRow{
			{Symbol: "AAPL", ActualPct: decimal.MustParse("50.0"), TargetPct: decimal.MustParse("40.0"), DriftPct: decimal.MustParse("10.0"), IsBalanced: false},
		},
		BaseCurrency: "USD",
		HasTarget:    true,
	}
	got = serializeDriftData(drift)
	if got == "{}" {
		t.Error("expected non-empty JSON")
	}
	if !strings.Contains(got, "AAPL") {
		t.Error("expected AAPL in JSON")
	}
}

// Test serializeRebalanceData.
func TestSerializeRebalanceData(t *testing.T) {
	// Nil.
	got := serializeRebalanceData(nil)
	if got != "{}" {
		t.Errorf("nil = %q, want {}", got)
	}

	// Empty suggestions.
	got = serializeRebalanceData(&allocation.RebalanceResult{IsBalanced: true})
	if got != "{}" {
		t.Errorf("empty = %q, want {}", got)
	}

	// With data.
	rebalance := &allocation.RebalanceResult{
		Suggestions: []allocation.RebalanceSuggestion{
			{Symbol: "AAPL", Direction: "sell", Shares: decimal.MustParse("5.00")},
		},
		BaseCurrency: "USD",
	}
	got = serializeRebalanceData(rebalance)
	if got == "{}" {
		t.Error("expected non-empty JSON")
	}
	if !strings.Contains(got, "AAPL") {
		t.Error("expected AAPL in JSON")
	}
}

// Test that parsing 5 target entries of 20% each sums to 100% and saves successfully.
// Regression test: form parsing + decimal sum validation for 5 × 20% = 100%.
func TestSaveTargetAllocation_FormParsing_FiveEntries20Pct(t *testing.T) {
	// Simulate form values: 5 entries, each 20%
	formData := url.Values{}
	formData.Set("portfolio_id", "1")
	for i := 0; i < 5; i++ {
		formData.Set("symbol_"+strconv.Itoa(i), fmt.Sprintf("SYM%d", i))
		formData.Set("target_pct_"+strconv.Itoa(i), "20")
	}

	// Parse entries the same way the handler does
	var entries []allocation.TargetEntry
	i := 0
	for {
		symbol := formData.Get("symbol_" + strconv.Itoa(i))
		if symbol == "" {
			break
		}
		pctStr := formData.Get("target_pct_" + strconv.Itoa(i))
		pct, err := decimal.Parse(pctStr)
		if err != nil {
			t.Fatalf("parse error for %s: %v", symbol, err)
		}
		entries = append(entries, allocation.TargetEntry{
			Symbol:    symbol,
			TargetPct: pct,
		})
		i++
	}

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Validate sum == 100 (same logic as SaveTargetAllocation)
	var sum decimal.Decimal
	for _, e := range entries {
		sum, _ = sum.Add(e.TargetPct)
	}
	hundred := decimal.MustNew(10000, 2)
	if !sum.Equal(hundred) {
		t.Errorf("sum %q does not equal 100%% (hundred=%q)", sum.String(), hundred.String())
	}
}

// Test the full HandleSaveTarget flow with 5 entries of 20% each.
// Regression test: end-to-end form submission → service validation.
func TestAllocationHandler_SaveTarget_FiveEntries20Pct(t *testing.T) {
	mock := newMockAllocationService()
	handler := &AllocationWebHandler{
		allocSvc: mock,
		renderer: newTestRenderer(t),
	}

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Build form body: 5 entries × 20%
	body := "portfolio_id=1"
	for i := 0; i < 5; i++ {
		body += fmt.Sprintf("&symbol_%d=SYM%d", i, i)
		body += fmt.Sprintf("&target_pct_%d=20", i)
	}

	req := httptest.NewRequest(http.MethodPost, "/allocation/target", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Should redirect (303) with flash "Target allocations saved"
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if len(mock.lastEntries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(mock.lastEntries))
	}
}

// Test that the allocation page renders with model portfolios in the dropdown data.
func TestAllocationTemplate_WithModelPortfolios(t *testing.T) {
	renderer := newTestRenderer(t)

	drift := &allocation.DriftResult{
		Rows:         []allocation.DriftRow{},
		BaseCurrency: "USD",
		HasTarget:    false,
	}

	modelPortfolios := []modelportfolio.ModelPortfolioSummary{
		{ID: 1, Name: "60/40 Balanced", EntryCount: 2},
		{ID: 2, Name: "Growth Portfolio", EntryCount: 5},
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Drift:             drift,
		ModelPortfolios:   modelPortfolios,
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Load from model:") {
		t.Error("expected model portfolio selector label")
	}
	if !strings.Contains(body, "60/40 Balanced") {
		t.Error("expected model portfolio name in dropdown")
	}
	if !strings.Contains(body, "2 entries") {
		t.Error("expected entry count in dropdown")
	}
	if !strings.Contains(body, "Growth Portfolio") {
		t.Error("expected second model portfolio name in dropdown")
	}
	if !strings.Contains(body, `onclick="loadModelPortfolio()"`) {
		t.Error("expected loadModelPortfolio JS handler")
	}
}

// Test that the allocation page hides the model portfolio section when none exist.
func TestAllocationTemplate_NoModelPortfolios(t *testing.T) {
	renderer := newTestRenderer(t)

	drift := &allocation.DriftResult{
		Rows:         []allocation.DriftRow{},
		BaseCurrency: "USD",
		HasTarget:    false,
	}

	data := allocationPageData{
		PageData:          web.PageData{Title: "Allocation"},
		Drift:             drift,
		ModelPortfolios:   []modelportfolio.ModelPortfolioSummary{},
		Portfolios:        []portfolio.Portfolio{{ID: 1, Name: "Main", Currency: "USD"}},
		SelectedPortfolio: "1",
		Filter:            AllocationFilter{PortfolioIDs: []int64{1}},
		BaseCurrency:      "USD",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "allocation/list", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()

	if strings.Contains(body, "Load from model:") {
		t.Error("should not show model portfolio selector when none exist")
	}
}
