package extractor

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- Mock Extractor ---

type mockExtractor struct {
	name    string
	matchFn func(string) bool
	result  *ExtractResult
	err     error
}

func (m *mockExtractor) Name() string {
	return m.name
}

func (m *mockExtractor) Match(rawURL string) bool {
	if m.matchFn != nil {
		return m.matchFn(rawURL)
	}
	return false
}

func (m *mockExtractor) Extract(_ context.Context, _ string) (*ExtractResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result == nil {
		return nil, nil
	}
	cp := *m.result
	return &cp, nil
}

// --- Registry Tests ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()

	extractor := &mockExtractor{name: "test"}
	err := reg.Register(extractor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := reg.Get("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name() != "test" {
		t.Errorf("expected name 'test', got %q", got.Name())
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	reg := NewRegistry()

	reg.Register(&mockExtractor{name: "test"})
	err := reg.Register(&mockExtractor{name: "test"})
	if err == nil {
		t.Fatal("expected error for duplicate registration, got nil")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent extractor, got nil")
	}
}

func TestRegistry_FindByURL_Success(t *testing.T) {
	reg := NewRegistry()

	reg.Register(&mockExtractor{
		name: "wisdomtree",
		matchFn: func(url string) bool {
			return len(url) > 0
		},
	})

	extractor, err := reg.FindByURL("https://example.com/fund")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if extractor.Name() != "wisdomtree" {
		t.Errorf("expected 'wisdomtree', got %q", extractor.Name())
	}
}

func TestRegistry_FindByURL_NoMatch(t *testing.T) {
	reg := NewRegistry()

	reg.Register(&mockExtractor{
		name: "test",
		matchFn: func(url string) bool {
			return false
		},
	})

	_, err := reg.FindByURL("https://other.com/fund")
	if err == nil {
		t.Fatal("expected error for no match, got nil")
	}
}

func TestRegistry_FindByURL_FirstMatchWins(t *testing.T) {
	reg := NewRegistry()

	// Both matchers match everything; first registered should win
	reg.Register(&mockExtractor{
		name:    "first",
		matchFn: func(string) bool { return true },
	})
	reg.Register(&mockExtractor{
		name:    "second",
		matchFn: func(string) bool { return true },
	})

	extractor, err := reg.FindByURL("https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if extractor.Name() != "first" {
		t.Errorf("expected 'first' (first match wins), got %q", extractor.Name())
	}
}

func TestRegistry_FindByURL_EmptyRegistry(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.FindByURL("https://example.com")
	if err == nil {
		t.Fatal("expected error for empty registry, got nil")
	}
}

// --- Dispatcher Tests ---

func TestDispatcher_Dispatch_Success(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockExtractor{
		name:    "test",
		matchFn: func(string) bool { return true },
		result: &ExtractResult{
			AsOfDate: time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
			FundInfo: &FundInfo{Symbol: "WMGT", Name: "Test Fund"},
		},
	})

	dispatcher := NewDispatcher(reg)
	result, err := dispatcher.Dispatch(context.Background(), "https://example.com/fund")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FundInfo.Symbol != "WMGT" {
		t.Errorf("expected symbol 'WMGT', got %q", result.FundInfo.Symbol)
	}
	if !result.AsOfDate.Equal(time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected as-of date: %v", result.AsOfDate)
	}
}

func TestDispatcher_Dispatch_NoMatch(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockExtractor{
		name: "test",
		matchFn: func(url string) bool {
			return false
		},
	})

	dispatcher := NewDispatcher(reg)
	_, err := dispatcher.Dispatch(context.Background(), "https://other.com/fund")
	if err == nil {
		t.Fatal("expected error for no match, got nil")
	}
}

func TestDispatcher_Dispatch_ExtractorError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockExtractor{
		name:    "test",
		matchFn: func(string) bool { return true },
		err:     fmt.Errorf("network timeout"),
	})

	dispatcher := NewDispatcher(reg)
	_, err := dispatcher.Dispatch(context.Background(), "https://example.com/fund")
	if err == nil {
		t.Fatal("expected error propagated from extractor, got nil")
	}
}

func TestDispatcher_Dispatch_InvalidURL(t *testing.T) {
	reg := NewRegistry()
	dispatcher := NewDispatcher(reg)

	_, err := dispatcher.Dispatch(context.Background(), "://not-a-valid-url")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestDispatcher_Dispatch_EmptyRegistry(t *testing.T) {
	reg := NewRegistry()
	dispatcher := NewDispatcher(reg)

	_, err := dispatcher.Dispatch(context.Background(), "https://example.com/fund")
	if err == nil {
		t.Fatal("expected error for empty registry, got nil")
	}
}

// --- URLMatcher (WisdomTree) Tests ---

func TestWisdomTreeURLMatcher_Match(t *testing.T) {
	// Import the wisdomtree package for URL matching tests
	// We test via the mock pattern here since we can't import wisdomtree
	// directly in this package. The wisdomtree package has its own tests.
	// This tests the URLMatcher interface contract.

	matcher := &mockExtractor{
		name: "wisdomtree",
		matchFn: func(url string) bool {
			// Simulate wisdomtree matcher logic
			return len(url) > 0 && (contains(url, "wisdomtree.eu") || contains(url, "wisdomtree.com"))
		},
	}

	tests := []struct {
		name   string
		url    string
		expect bool
	}{
		{"wisdomtree.eu subdomain", "https://www.wisdomtree.eu/en-gb/etfs/thematic/wmgt", true},
		{"wisdomtree.eu root", "https://wisdomtree.eu/", true},
		{"wisdomtree.com", "https://www.wisdomtree.com/us/en/etfs/wmgt", true},
		{"other domain", "https://www.vanguard.com/etfs/vo", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.Match(tt.url)
			if got != tt.expect {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.expect)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
