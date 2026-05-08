package web

import "testing"

func TestFormatDecimal_Positive(t *testing.T) {
	tests := []struct {
		input    string
		decimals int
		want     string
	}{
		{"1234567", 2, "1,234,567.00"},
		{"1234567.8912", 2, "1,234,567.89"},
		{"1.2345", 4, "1.2345"},
		{"1.23", 4, "1.2300"},
		{"100", 2, "100.00"},
		{"0", 2, "0.00"},
		{"123", 0, "123"},
		{"0.5", 2, "0.50"},
	}
	for _, tt := range tests {
		got := formatDecimal(tt.input, tt.decimals)
		if got != tt.want {
			t.Errorf("formatDecimal(%q, %d) = %q, want %q", tt.input, tt.decimals, got, tt.want)
		}
	}
}

func TestFormatDecimal_Negative(t *testing.T) {
	tests := []struct {
		input    string
		decimals int
		want     string
	}{
		{"-1234567.89", 2, "-1,234,567.89"},
		{"-100", 2, "-100.00"},
		{"-0.5", 2, "-0.50"},
	}
	for _, tt := range tests {
		got := formatDecimal(tt.input, tt.decimals)
		if got != tt.want {
			t.Errorf("formatDecimal(%q, %d) = %q, want %q", tt.input, tt.decimals, got, tt.want)
		}
	}
}

func TestFormatDecimal_Empty(t *testing.T) {
	got := formatDecimal("", 2)
	if got != "" {
		t.Errorf("formatDecimal(%q, 2) = %q, want %q", "", got, "")
	}
}

func TestAddThousandsSeparator(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"123", "123"},
		{"1234", "1,234"},
		{"12345", "12,345"},
		{"123456", "123,456"},
		{"1234567", "1,234,567"},
		{"12345678", "12,345,678"},
		{"123456789", "123,456,789"},
	}
	for _, tt := range tests {
		got := addThousandsSeparator(tt.input)
		if got != tt.want {
			t.Errorf("addThousandsSeparator(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
