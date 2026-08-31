package optimization

import (
	"strings"
	"testing"
	"time"
)

func TestApproximatePeriodLabel(t *testing.T) {
	now := time.Now()
	tests := []struct {
		startDaysAgo int
		wantContains string
	}{
		{10, "D"},
		{45, "M"},
		{180, "M"},
		{365, "Y"},
		{730, "Y"},
	}

	for _, tt := range tests {
		start := now.AddDate(0, 0, -tt.startDaysAgo)
		label := approximatePeriodLabel(start, now)
		if !strings.Contains(label, tt.wantContains) {
			t.Errorf("start %d days ago: label %q should contain %q", tt.startDaysAgo, label, tt.wantContains)
		}
	}
}

func TestExpectedTradingDays(t *testing.T) {
	tests := []struct {
		period string
		want   int
	}{
		{"3M", 63},
		{"6M", 126},
		{"1Y", 252},
		{"3Y", 756},
		{"5Y", 1260},
		{"10Y", 2520},
		{"7Y", 0}, // unrecognized
		{"", 0},   // empty
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			got := expectedTradingDays(tt.period)
			if got != tt.want {
				t.Errorf("expectedTradingDays(%q) = %d, want %d", tt.period, got, tt.want)
			}
		})
	}
}
