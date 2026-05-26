package wisdomtree

import (
	"context"
	"errors"
	"testing"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	if e.Name() != Name {
		t.Errorf("expected name %q, got %q", Name, e.Name())
	}
}

func TestExtractor_Match(t *testing.T) {
	e := NewExtractor()

	if !e.Match("https://www.wisdomtree.eu/en-gb/etfs/wmgt") {
		t.Error("expected wisdomtree.eu URL to match")
	}
	if e.Match("https://www.vanguard.com/etfs/vo") {
		t.Error("expected vanguard.com URL to not match")
	}
}

func TestExtractor_Extract_NotImplemented(t *testing.T) {
	e := NewExtractor()

	_, err := e.Extract(context.Background(), "https://www.wisdomtree.eu/en-gb/etfs/wmgt")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}
