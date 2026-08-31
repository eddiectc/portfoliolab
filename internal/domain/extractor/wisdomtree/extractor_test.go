package wisdomtree

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	if e.Name() != Name {
		t.Errorf("expected name %q, got %q", Name, e.Name())
	}
}

// qgrwPage is the full-page fixture: the wtClassID capture plus the tables
// and sector-section captures (each fetch streamed part of the payload).
func qgrwPage(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, p := range []string{"page_qgrw_wtclassid.txt", "flight_qgrw_tables.html", "flight_qgrw_sector.html"} {
		b.WriteString(loadFixture(t, p))
	}
	return b.String()
}

// newTestExtractor wires the extractor to the QGRW fixtures; the returned
// fetch func may be overridden per test to inject failures.
func newTestExtractor(t *testing.T) *Extractor {
	t.Helper()
	e := NewExtractor()
	e.client.minDelay = 0
	e.client.SetFetchFunc(func(url string) (string, error) {
		switch {
		case strings.Contains(url, "/fund-holdings/"):
			return loadFixture(t, "holdings_qgrw.json"), nil
		case strings.Contains(url, "/fund-history/"):
			return loadFixture(t, "fund_history_qgrw.json"), nil
		default:
			return qgrwPage(t), nil
		}
	})
	return e
}

func TestExtractor_Extract_Errors(t *testing.T) {
	t.Run("page fetch failure", func(t *testing.T) {
		e := newTestExtractor(t)
		e.client.SetFetchFunc(func(string) (string, error) {
			return "", errors.New("network down")
		})
		_, err := e.Extract(context.Background(), "https://www.wisdomtree.com/gb/products/equities/qgrw")
		if err == nil || !strings.Contains(err.Error(), "fetch page") {
			t.Fatalf("err = %v, want 'fetch page' wrap", err)
		}
	})

	t.Run("missing wtClassID", func(t *testing.T) {
		e := newTestExtractor(t)
		e.client.SetFetchFunc(func(url string) (string, error) {
			if strings.Contains(url, "/fund-holdings/") {
				return loadFixture(t, "holdings_qgrw.json"), nil
			}
			if strings.Contains(url, "/fund-history/") {
				return loadFixture(t, "fund_history_qgrw.json"), nil
			}
			return "<html>no flight payload here</html>", nil
		})
		_, err := e.Extract(context.Background(), "https://www.wisdomtree.com/gb/products/equities/qgrw")
		if err == nil || !strings.Contains(err.Error(), "wtClassID") {
			t.Fatalf("err = %v, want 'wtClassID' wrap", err)
		}
	})

	t.Run("holdings API failure", func(t *testing.T) {
		e := newTestExtractor(t)
		e.client.SetFetchFunc(func(url string) (string, error) {
			if strings.Contains(url, "/fund-holdings/") {
				return "<html>404 page</html>", nil
			}
			if strings.Contains(url, "/fund-history/") {
				return loadFixture(t, "fund_history_qgrw.json"), nil
			}
			return loadFixture(t, "page_qgrw_wtclassid.txt"), nil
		})
		_, err := e.Extract(context.Background(), "https://www.wisdomtree.com/gb/products/equities/qgrw")
		if err == nil || !strings.Contains(err.Error(), "fund-holdings API") {
			t.Fatalf("err = %v, want 'fund-holdings API' wrap", err)
		}
	})

	t.Run("no tradeable holdings rows", func(t *testing.T) {
		e := newTestExtractor(t)
		e.client.SetFetchFunc(func(url string) (string, error) {
			if strings.Contains(url, "/fund-holdings/") {
				return `[{"securityName":"CASH W-O","securityTicker":null,"wgt":1}]`, nil
			}
			if strings.Contains(url, "/fund-history/") {
				return loadFixture(t, "fund_history_qgrw.json"), nil
			}
			return qgrwPage(t), nil
		})
		_, err := e.Extract(context.Background(), "https://www.wisdomtree.com/gb/products/equities/qgrw")
		if err == nil || !strings.Contains(err.Error(), "no tradeable rows") {
			t.Fatalf("err = %v, want 'no tradeable rows' wrap", err)
		}
	})

	t.Run("page without Net Asset Value table", func(t *testing.T) {
		// The wtClassID-only capture carries no flight tables at all: the
		// as-of date is unavailable and the extraction must fail atomically.
		e := newTestExtractor(t)
		e.client.SetFetchFunc(func(url string) (string, error) {
			switch {
			case strings.Contains(url, "/fund-holdings/"):
				return loadFixture(t, "holdings_qgrw.json"), nil
			case strings.Contains(url, "/fund-history/"):
				return loadFixture(t, "fund_history_qgrw.json"), nil
			default:
				return loadFixture(t, "page_qgrw_wtclassid.txt"), nil
			}
		})
		_, err := e.Extract(context.Background(), "https://www.wisdomtree.com/gb/products/equities/qgrw")
		if err == nil || !strings.Contains(err.Error(), "Net Asset Value table") {
			t.Fatalf("err = %v, want 'Net Asset Value table' wrap", err)
		}
	})

	t.Run("canceled context before fetch", func(t *testing.T) {
		e := NewExtractor()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := e.Extract(ctx, "https://www.wisdomtree.com/us/products/equity/ezm")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}
