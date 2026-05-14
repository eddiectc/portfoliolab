package performance

import (
	"testing"

	"github.com/govalues/decimal"
)

func TestComputeYearlyPerformance_SingleYear(t *testing.T) {
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: dec(10000, 2)}, // NAV $100.00
		{Date: mustTime("2024-06-01"), NavPerUnit: dec(11000, 2)}, // NAV $110.00
		{Date: mustTime("2024-12-31"), NavPerUnit: dec(12000, 2)}, // NAV $120.00
	}

	result := ComputeYearlyPerformance(points)

	if len(result) != 1 {
		t.Fatalf("got %d years, want 1", len(result))
	}

	if result[0].Year != 2024 {
		t.Errorf("year: got %d, want 2024", result[0].Year)
	}

	// (120.00 - 100.00) / 100.00 × 100 = 20.00%
	want := decimal.MustParse("20.00")
	if !result[0].ReturnPct.Equal(want) {
		t.Errorf("return: got %s, want %s", result[0].ReturnPct.String(), want.String())
	}
}

func TestComputeYearlyPerformance_MultipleYears(t *testing.T) {
	points := []NavPoint{
		{Date: mustTime("2023-01-01"), NavPerUnit: dec(10000, 2)}, // $100.00
		{Date: mustTime("2023-12-31"), NavPerUnit: dec(11000, 2)}, // $110.00 (+10%)
		{Date: mustTime("2024-01-01"), NavPerUnit: dec(11000, 2)}, // $110.00
		{Date: mustTime("2024-06-30"), NavPerUnit: dec(12100, 2)}, // $121.00
		{Date: mustTime("2024-12-31"), NavPerUnit: dec(10890, 2)}, // $108.90 (-10% from start)
		{Date: mustTime("2025-01-01"), NavPerUnit: dec(10890, 2)}, // $108.90
		{Date: mustTime("2025-06-30"), NavPerUnit: dec(12000, 2)}, // $120.00
	}

	result := ComputeYearlyPerformance(points)

	if len(result) != 3 {
		t.Fatalf("got %d years, want 3", len(result))
	}

	// 2023: (110 - 100) / 100 × 100 = +10.00%
	want2023 := decimal.MustParse("10.00")
	if result[0].Year != 2023 || !result[0].ReturnPct.Equal(want2023) {
		t.Errorf("2023: got year=%d, return=%s, want year=2023, return=%s",
			result[0].Year, result[0].ReturnPct.String(), want2023.String())
	}

	// 2024: (108.90 - 110.00) / 110.00 × 100 = -1.00%
	want2024 := decimal.MustParse("-1.00")
	if result[1].Year != 2024 || !result[1].ReturnPct.Equal(want2024) {
		t.Errorf("2024: got year=%d, return=%s, want year=2024, return=%s",
			result[1].Year, result[1].ReturnPct.String(), want2024.String())
	}

	// 2025: (120.00 - 108.90) / 108.90 × 100 ≈ 10.1928%
	want2025 := decimal.MustParse("10.1928")
	if result[2].Year != 2025 || !result[2].ReturnPct.Equal(want2025) {
		t.Errorf("2025: got year=%d, return=%s, want year=2025, return=%s",
			result[2].Year, result[2].ReturnPct.String(), want2025.String())
	}
}

func TestComputeYearlyPerformance_PartialYear(t *testing.T) {
	// Only mid-year data — still computes return from first to last point in that year.
	points := []NavPoint{
		{Date: mustTime("2024-06-01"), NavPerUnit: dec(10000, 2)}, // $100.00
		{Date: mustTime("2024-09-30"), NavPerUnit: dec(10500, 2)}, // $105.00
	}

	result := ComputeYearlyPerformance(points)

	if len(result) != 1 {
		t.Fatalf("got %d years, want 1", len(result))
	}

	// (105.00 - 100.00) / 100.00 × 100 = 5.00%
	want := decimal.MustParse("5.00")
	if !result[0].ReturnPct.Equal(want) {
		t.Errorf("return: got %s, want %s", result[0].ReturnPct.String(), want.String())
	}
}

func TestComputeYearlyPerformance_SinglePoint(t *testing.T) {
	// Only one point — first = last, return is 0%.
	points := []NavPoint{
		{Date: mustTime("2024-06-01"), NavPerUnit: dec(10000, 2)},
	}

	result := ComputeYearlyPerformance(points)

	if len(result) != 1 {
		t.Fatalf("got %d years, want 1", len(result))
	}

	if result[0].Year != 2024 {
		t.Errorf("year: got %d, want 2024", result[0].Year)
	}

	if !result[0].ReturnPct.Equal(decimal.Zero) {
		t.Errorf("single point return: got %s, want 0", result[0].ReturnPct.String())
	}
}

func TestComputeYearlyPerformance_CrossYearBoundary(t *testing.T) {
	// Points straddling a year boundary — each year gets its own first/last.
	points := []NavPoint{
		{Date: mustTime("2023-12-01"), NavPerUnit: dec(10000, 2)}, // 2023 start
		{Date: mustTime("2023-12-31"), NavPerUnit: dec(10500, 2)}, // 2023 end (+5%)
		{Date: mustTime("2024-01-01"), NavPerUnit: dec(10500, 2)}, // 2024 start
		{Date: mustTime("2024-01-15"), NavPerUnit: dec(10200, 2)}, // 2024 mid
		{Date: mustTime("2024-01-31"), NavPerUnit: dec(10800, 2)}, // 2024 end (+2.86% from start)
	}

	result := ComputeYearlyPerformance(points)

	if len(result) != 2 {
		t.Fatalf("got %d years, want 2", len(result))
	}

	// 2023: (105.00 - 100.00) / 100.00 × 100 = 5.00%
	want2023 := decimal.MustParse("5.00")
	if !result[0].ReturnPct.Equal(want2023) {
		t.Errorf("2023 return: got %s, want %s", result[0].ReturnPct.String(), want2023.String())
	}

	// 2024: (108.00 - 105.00) / 105.00 × 100 ≈ 2.8571%
	want2024 := decimal.MustParse("2.8571")
	if !result[1].ReturnPct.Equal(want2024) {
		t.Errorf("2024 return: got %s, want %s", result[1].ReturnPct.String(), want2024.String())
	}
}

func TestComputeYearlyPerformance_Empty(t *testing.T) {
	result := ComputeYearlyPerformance([]NavPoint{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %d years", len(result))
	}
}

func TestComputeYearlyPerformance_ZeroNAV(t *testing.T) {
	// Year with zero NAV first point — skipped entirely.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: decimal.Zero},
		{Date: mustTime("2024-06-01"), NavPerUnit: dec(10000, 2)},
	}

	result := ComputeYearlyPerformance(points)
	if len(result) != 0 {
		t.Errorf("expected 0 years (zero NAV skipped), got %d", len(result))
	}
}

func TestComputeYearlyPerformance_NoChange(t *testing.T) {
	// Flat NAV — return is 0%.
	points := []NavPoint{
		{Date: mustTime("2024-01-01"), NavPerUnit: dec(10000, 2)},
		{Date: mustTime("2024-06-01"), NavPerUnit: dec(10000, 2)},
		{Date: mustTime("2024-12-31"), NavPerUnit: dec(10000, 2)},
	}

	result := ComputeYearlyPerformance(points)

	if len(result) != 1 {
		t.Fatalf("got %d years, want 1", len(result))
	}

	if !result[0].ReturnPct.Equal(decimal.Zero) {
		t.Errorf("flat return: got %s, want 0", result[0].ReturnPct.String())
	}
}

func TestComputeYearlyPerformance_Ordering(t *testing.T) {
	// Verify years are returned in ascending order.
	points := []NavPoint{
		{Date: mustTime("2022-01-01"), NavPerUnit: dec(10000, 2)},
		{Date: mustTime("2022-12-31"), NavPerUnit: dec(11000, 2)},
		{Date: mustTime("2023-01-01"), NavPerUnit: dec(11000, 2)},
		{Date: mustTime("2023-12-31"), NavPerUnit: dec(10000, 2)},
		{Date: mustTime("2024-01-01"), NavPerUnit: dec(10000, 2)},
		{Date: mustTime("2024-12-31"), NavPerUnit: dec(12000, 2)},
	}

	result := ComputeYearlyPerformance(points)

	if len(result) != 3 {
		t.Fatalf("got %d years, want 3", len(result))
	}

	if result[0].Year != 2022 || result[1].Year != 2023 || result[2].Year != 2024 {
		t.Errorf("years out of order: got %d, %d, %d",
			result[0].Year, result[1].Year, result[2].Year)
	}
}
