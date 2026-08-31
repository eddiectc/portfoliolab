package wisdomtree

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loadFixture reads a file from testdata/.
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// newTestClient returns a client that keeps the production rate limit
// (minDelay = 1s) but does not actually sleep between requests.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	c := NewClient()
	c.throttle = func(time.Duration) {}
	return c
}

func TestExtractWtClassID(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    int
		wantErr bool
	}{
		{
			name: "QGRW page snippet (UCITS, escaped quotes)",
			body: loadFixture(t, "page_qgrw_wtclassid.txt"),
			want: 49567173,
		},
		{
			name: "EZM page snippet (US, escaped quotes)",
			body: loadFixture(t, "page_ezm_wtclassid.txt"),
			want: 1000518,
		},
		{
			name: "unescaped form",
			body: `{"foo":"bar","wtClassID":123456}`,
			want: 123456,
		},
		{
			name: "first match wins (all occurrences identical on a page)",
			body: `...\\"wtClassID\":111111,...\"wtClassID\":111111,...`,
			want: 111111,
		},
		{
			name:    "not found",
			body:    `<html>no flight payload here</html>`,
			wantErr: true,
		},
		{
			name:    "id too short",
			body:    `"wtClassID":123,`,
			wantErr: true,
		},
		{
			name:    "longer field name containing wtClassID does not match",
			body:    `{"parentwtClassID":123456}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractWtClassID(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got id %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ExtractWtClassID() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClient_FundHoldings_URLAndParse(t *testing.T) {
	client := newTestClient(t)
	var gotURL string
	client.SetFetchFunc(func(url string) (string, error) {
		gotURL = url
		return loadFixture(t, "holdings_qgrw.json"), nil
	})

	records, err := client.FundHoldings(context.Background(), 49567173)
	if err != nil {
		t.Fatalf("FundHoldings() error: %v", err)
	}
	if want := "https://www.wisdomtree.com/api/fund-holdings/49567173"; gotURL != want {
		t.Errorf("fetched %q, want %q", gotURL, want)
	}
	if len(records) != 101 {
		t.Fatalf("got %d records, want 101", len(records))
	}

	// Spot-check the top holding (Nvidia, from RESEARCH.md §5.1).
	var nvda *holdingRecord
	for i := range records {
		if records[i].SecurityName == "Nvidia Corp" {
			nvda = &records[i]
			break
		}
	}
	if nvda == nil {
		t.Fatal("Nvidia Corp not found in QGRW holdings")
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"SecurityTicker", derefPtr(nvda.SecurityTicker), "NVDA UQ"},
		{"Figi", derefPtr(nvda.Figi), "BBG000BBJQV0"},
		{"SectorName", derefPtr(nvda.SectorName), "Information Technology"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if nvda.AssetGroup != "EQ" {
		t.Errorf("AssetGroup = %q, want %q", nvda.AssetGroup, "EQ")
	}
	if nvda.MarketValueBase != 7003317.62 {
		t.Errorf("MarketValueBase = %v, want 7003317.62", nvda.MarketValueBase)
	}
	if nvda.Wgt != 0.1476209845990145 {
		t.Errorf("Wgt = %v, want 0.1476209845990145", nvda.Wgt)
	}
	if nvda.Shares != 30719 {
		t.Errorf("Shares = %v, want 30719", nvda.Shares)
	}
	if want := "2026-08-27T00:00:00.000Z"; nvda.DT != want {
		t.Errorf("DT = %q, want %q", nvda.DT, want)
	}
}

func TestClient_FundHistory_URLAndParse(t *testing.T) {
	client := newTestClient(t)
	var gotURL string
	client.SetFetchFunc(func(url string) (string, error) {
		gotURL = url
		return loadFixture(t, "fund_history_qgrw.json"), nil
	})

	points, err := client.FundHistory(context.Background(), 49567173)
	if err != nil {
		t.Fatalf("FundHistory() error: %v", err)
	}
	if want := "https://www.wisdomtree.com/api/fund-history/49567173"; gotURL != want {
		t.Errorf("fetched %q, want %q", gotURL, want)
	}
	if len(points) != 601 {
		t.Fatalf("got %d points, want 601", len(points))
	}

	first, last := points[0], points[len(points)-1]
	if want := "2024-04-16T00:00:00.000Z"; first.DT != want {
		t.Errorf("first DT = %q, want %q", first.DT, want)
	}
	if first.NavDelta != nil {
		t.Errorf("first NavDelta = %v, want nil (null in JSON)", *first.NavDelta)
	}
	// Last point must match the page NAV table (RESEARCH.md §7 cross-check).
	if want := "2026-08-28T00:00:00.000Z"; last.DT != want {
		t.Errorf("last DT = %q, want %q", last.DT, want)
	}
	if last.NAV != 42.9737 {
		t.Errorf("last NAV = %v, want 42.9737", last.NAV)
	}
	if last.AUM != 47442.9648 {
		t.Errorf("last AUM = %v, want 47442.9648", last.AUM)
	}
	if last.Ticker != "QGRW LN" {
		t.Errorf("last Ticker = %q, want %q", last.Ticker, "QGRW LN")
	}

	// Dates must be ascending.
	for i := 1; i < len(points); i++ {
		if points[i].DT < points[i-1].DT {
			t.Fatalf("dates not ascending at %d: %s < %s", i, points[i].DT, points[i-1].DT)
		}
	}
}

// TestClient_FundHoldings_AllFixtures unmarshals every captured holdings API
// response and verifies count, single as-of date, and weight sum.
func TestClient_FundHoldings_AllFixtures(t *testing.T) {
	funds := []struct {
		file       string
		id         int
		count      int
		dt         string
		fundTicker string
	}{
		{"holdings_qgrw.json", 49567173, 101, "2026-08-27T00:00:00.000Z", "QGRW LN"},
		{"holdings_wmgt.json", 46987205, 920, "2026-08-27T00:00:00.000Z", "WMGT LN"},
		{"holdings_ezm.json", 1000518, 508, "2026-08-28T00:00:00.000Z", "EZM"},
	}
	for _, f := range funds {
		t.Run(f.file, func(t *testing.T) {
			client := newTestClient(t)
			client.SetFetchFunc(func(url string) (string, error) {
				if want := fmt.Sprintf("https://www.wisdomtree.com/api/fund-holdings/%d", f.id); url != want {
					t.Errorf("fetched %q, want %q", url, want)
				}
				return loadFixture(t, f.file), nil
			})

			records, err := client.FundHoldings(context.Background(), f.id)
			if err != nil {
				t.Fatalf("FundHoldings() error: %v", err)
			}
			if len(records) != f.count {
				t.Fatalf("got %d records, want %d", len(records), f.count)
			}
			sumWgt := 0.0
			for i, r := range records {
				if r.DT != f.dt {
					t.Fatalf("record %d DT = %q, want %q (all rows share one date)", i, r.DT, f.dt)
				}
				if r.FundTicker != f.fundTicker {
					t.Fatalf("record %d FundTicker = %q, want %q", i, r.FundTicker, f.fundTicker)
				}
				if r.WtClassID != f.id {
					t.Fatalf("record %d WtClassID = %d, want %d", i, r.WtClassID, f.id)
				}
				sumWgt += r.Wgt
			}
			if math.Abs(sumWgt-1.0) > 1e-9 {
				t.Errorf("sum(wgt) = %v, want 1.0", sumWgt)
			}
		})
	}
}

// TestClient_FundHistory_AllFixtures unmarshals every captured fund-history
// API response and verifies count and first/last dates.
func TestClient_FundHistory_AllFixtures(t *testing.T) {
	funds := []struct {
		file    string
		id      int
		count   int
		firstDT string
		lastDT  string
		lastNav float64
	}{
		{"fund_history_qgrw.json", 49567173, 601, "2024-04-16T00:00:00.000Z", "2026-08-28T00:00:00.000Z", 42.9737},
		{"fund_history_wmgt.json", 46987205, 691, "2023-12-05T00:00:00.000Z", "2026-08-28T00:00:00.000Z", 42.1558},
		{"fund_history_ezm.json", 1000518, 4914, "2007-02-21T00:00:00.000Z", "2026-08-28T00:00:00.000Z", 76.0329},
	}
	for _, f := range funds {
		t.Run(f.file, func(t *testing.T) {
			client := newTestClient(t)
			client.SetFetchFunc(func(url string) (string, error) {
				if want := fmt.Sprintf("https://www.wisdomtree.com/api/fund-history/%d", f.id); url != want {
					t.Errorf("fetched %q, want %q", url, want)
				}
				return loadFixture(t, f.file), nil
			})

			points, err := client.FundHistory(context.Background(), f.id)
			if err != nil {
				t.Fatalf("FundHistory() error: %v", err)
			}
			if len(points) != f.count {
				t.Fatalf("got %d points, want %d", len(points), f.count)
			}
			if points[0].DT != f.firstDT {
				t.Errorf("first DT = %q, want %q", points[0].DT, f.firstDT)
			}
			last := points[len(points)-1]
			if last.DT != f.lastDT {
				t.Errorf("last DT = %q, want %q", last.DT, f.lastDT)
			}
			if last.NAV != f.lastNav {
				t.Errorf("last NAV = %v, want %v", last.NAV, f.lastNav)
			}
		})
	}
}

func TestClient_FundHoldings_Errors(t *testing.T) {
	t.Run("fetch error propagates", func(t *testing.T) {
		client := newTestClient(t)
		client.SetFetchFunc(func(string) (string, error) {
			return "", errors.New("network down")
		})
		_, err := client.FundHoldings(context.Background(), 123)
		if err == nil || !strings.Contains(err.Error(), "network down") {
			t.Fatalf("err = %v, want wrap of 'network down'", err)
		}
	})

	t.Run("non-JSON body is a parse error", func(t *testing.T) {
		client := newTestClient(t)
		client.SetFetchFunc(func(string) (string, error) {
			return "<html>404 page</html>", nil
		})
		_, err := client.FundHoldings(context.Background(), 123)
		if err == nil || !strings.Contains(err.Error(), "unmarshal fund-holdings") {
			t.Fatalf("err = %v, want 'unmarshal fund-holdings'", err)
		}
	})

}

func TestClient_FundHistory_Errors(t *testing.T) {
	t.Run("fetch error propagates", func(t *testing.T) {
		client := newTestClient(t)
		client.SetFetchFunc(func(string) (string, error) {
			return "", errors.New("network down")
		})
		_, err := client.FundHistory(context.Background(), 123)
		if err == nil || !strings.Contains(err.Error(), "network down") {
			t.Fatalf("err = %v, want wrap of 'network down'", err)
		}
	})

	t.Run("non-JSON body is a parse error", func(t *testing.T) {
		client := newTestClient(t)
		client.SetFetchFunc(func(string) (string, error) {
			return "not json", nil
		})
		_, err := client.FundHistory(context.Background(), 123)
		if err == nil || !strings.Contains(err.Error(), "unmarshal fund-history") {
			t.Fatalf("err = %v, want 'unmarshal fund-history'", err)
		}
	})
}

// TestClient_RateLimitingShared verifies Fetch, FundHoldings and FundHistory
// all go through the same shared rate limiter: the first request does not
// wait, and each subsequent request waits ~minDelay. The waits are asserted
// via the throttle hook, so the test is deterministic and sleep-free.
func TestClient_RateLimitingShared(t *testing.T) {
	client := NewClient()
	var waits []time.Duration
	client.SetThrottle(func(d time.Duration) { waits = append(waits, d) })
	calls := 0
	client.SetFetchFunc(func(string) (string, error) {
		calls++
		return "[]", nil
	})

	ctx := context.Background()
	if _, err := client.Fetch("https://www.wisdomtree.com/us/funds/ezm/"); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if _, err := client.FundHoldings(ctx, 1); err != nil {
		t.Fatalf("FundHoldings() error: %v", err)
	}
	if _, err := client.FundHistory(ctx, 1); err != nil {
		t.Fatalf("FundHistory() error: %v", err)
	}

	if calls != 3 {
		t.Fatalf("fetch called %d times, want 3", calls)
	}
	if len(waits) != 2 {
		t.Fatalf("throttle called %d times, want 2 (first request must not wait)", len(waits))
	}
	for i, w := range waits {
		// Mocked fetches are instantaneous, so each wait is ~minDelay
		// (1s minus the few microseconds between requests).
		if w < 900*time.Millisecond {
			t.Errorf("throttle wait[%d] = %v, want ~%v (shared limiter)", i, w, client.minDelay)
		}
	}
}

func derefPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
