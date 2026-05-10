package handlers

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// --- PositionFilter Tests ---

func TestPositionFilter_QueryParams_Empty(t *testing.T) {
	f := PositionFilter{}
	got := f.QueryParams()
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestPositionFilter_QueryParams_AccountOnly(t *testing.T) {
	f := PositionFilter{AccountID: "5"}
	got := f.QueryParams()
	want := "&account_id=5"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestPositionFilter_QueryParams_PortfolioOnly(t *testing.T) {
	f := PositionFilter{PortfolioID: "3"}
	got := f.QueryParams()
	want := "&portfolio_id=3"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestPositionFilter_QueryParams_Both(t *testing.T) {
	f := PositionFilter{AccountID: "5", PortfolioID: "3"}
	got := f.QueryParams()
	want := "&account_id=5&portfolio_id=3"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestPositionFilter_PaginationQuery_NoFilters(t *testing.T) {
	f := PositionFilter{}
	got := f.PaginationQuery(2)
	// Should at least have page=2
	if got == "" {
		t.Error("expected non-empty query string")
	}
	if got[0] != '?' {
		t.Errorf("expected query to start with ?, got %q", got)
	}
}

func TestPositionFilter_PaginationQuery_WithFilters(t *testing.T) {
	f := PositionFilter{AccountID: "7"}
	got := f.PaginationQuery(3)
	values, err := url.ParseQuery(got[1:]) // strip leading ?
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}
	if values.Get("page") != "3" {
		t.Errorf("expected page=3, got %q", values.Get("page"))
	}
	if values.Get("account_id") != "7" {
		t.Errorf("expected account_id=7, got %q", values.Get("account_id"))
	}
}

// --- parsePositionFilter Tests ---

func TestParsePositionFilter_Empty(t *testing.T) {
	query := url.Values{}
	f := parsePositionFilter(query)
	if f.AccountID != "" || f.PortfolioID != "" {
		t.Errorf("expected empty filter, got %+v", f)
	}
}

func TestParsePositionFilter_AccountOnly(t *testing.T) {
	query := url.Values{"account_id": []string{"42"}}
	f := parsePositionFilter(query)
	if f.AccountID != "42" {
		t.Errorf("expected account_id=42, got %q", f.AccountID)
	}
	if f.PortfolioID != "" {
		t.Errorf("expected empty portfolio_id, got %q", f.PortfolioID)
	}
}

func TestParsePositionFilter_PortfolioOnly(t *testing.T) {
	query := url.Values{"portfolio_id": []string{"99"}}
	f := parsePositionFilter(query)
	if f.PortfolioID != "99" {
		t.Errorf("expected portfolio_id=99, got %q", f.PortfolioID)
	}
	if f.AccountID != "" {
		t.Errorf("expected empty account_id, got %q", f.AccountID)
	}
}

func TestParsePositionFilter_Both(t *testing.T) {
	query := url.Values{"account_id": []string{"5"}, "portfolio_id": []string{"10"}}
	f := parsePositionFilter(query)
	if f.AccountID != "5" {
		t.Errorf("expected account_id=5, got %q", f.AccountID)
	}
	if f.PortfolioID != "10" {
		t.Errorf("expected portfolio_id=10, got %q", f.PortfolioID)
	}
}

// --- Template tests with cache status ---

func TestPositionsTemplate_CacheStatusCurrent(t *testing.T) {
	renderer := newTestRenderer(t)

	data := openPositionListPageData{
		PageData:        web.PageData{Title: "Open Positions"},
		Positions:       []position.PositionWithMarket{},
		Accounts:        []account.Account{},
		HasCacheStatus:  true,
		LastRefreshText: "Updated 2m ago",
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "position/open", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "cache-status-current") {
		t.Error("expected cache-status-current class")
	}
	if !strings.Contains(body, "Updated 2m ago") {
		t.Error("expected 'Updated 2m ago' in page")
	}
}

func TestPositionsTemplate_CacheStatusRefreshing(t *testing.T) {
	renderer := newTestRenderer(t)

	data := openPositionListPageData{
		PageData:     web.PageData{Title: "Open Positions"},
		Positions:    []position.PositionWithMarket{},
		Accounts:     []account.Account{},
		HasCacheStatus: true,
		CacheStatus:  marketcache.CacheStatus{Refreshing: true},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "position/open", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "cache-status-refreshing") {
		t.Error("expected cache-status-refreshing class")
	}
	if !strings.Contains(body, "Refreshing...") {
		t.Error("expected 'Refreshing...' in page")
	}
}

func TestPositionsTemplate_NoCacheStatus(t *testing.T) {
	renderer := newTestRenderer(t)

	data := openPositionListPageData{
		PageData:       web.PageData{Title: "Open Positions"},
		Positions:      []position.PositionWithMarket{},
		Accounts:       []account.Account{},
		HasCacheStatus: false,
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "position/open", data); err != nil {
		t.Fatalf("template render failed: %v", err)
	}

	body := w.Body.String()
	if strings.Contains(body, "cache-status") {
		t.Error("expected no cache status indicator when HasCacheStatus is false")
	}
}


