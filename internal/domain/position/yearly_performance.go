package position

import (
	"sort"

	"github.com/govalues/decimal"
)

// YearlyReturn holds the calendar year and the percentage return for that year,
// computed from the first and last NAV per unit within the year.
// ReturnPct is expressed as a percentage (e.g. 12.50 = 12.50%).
type YearlyReturn struct {
	Year      int             `json:"year"`
	ReturnPct decimal.Decimal `json:"return_pct"`
}

// ComputeYearlyPerformance computes calendar-year returns from a sequence of
// NavPoints. It groups points by year and computes each year's return as
// (lastNAV - firstNAV) / firstNAV × 100, using NAV per unit for consistency
// across cash flow events.
//
// Years are returned in ascending order. Years with only one data point get
// 0% return (first = last). Years with no positive NAV are skipped entirely.
//
// Returns nil for empty input.
func ComputeYearlyPerformance(points []NavPoint) []YearlyReturn {
	if len(points) == 0 {
		return nil
	}

	// Group points by year, tracking first and last NAV per unit.
	type yearBucket struct {
		firstNAV decimal.Decimal
		lastNAV  decimal.Decimal
		count    int
	}

	buckets := make(map[int]*yearBucket)
	var years []int // preserve insertion order

	for _, p := range points {
		year := p.Date.Year()
		bucket, exists := buckets[year]
		if !exists {
			bucket = &yearBucket{}
			buckets[year] = bucket
			years = append(years, year)
		}

		if bucket.count == 0 {
			bucket.firstNAV = p.NavPerUnit
		}
		bucket.lastNAV = p.NavPerUnit
		bucket.count++
	}

	// Sort years ascending (should already be in order since NavPoints are
	// chronological, but be safe).
	sort.Ints(years)

	var result []YearlyReturn
	for _, year := range years {
		bucket := buckets[year]

		// Skip years where first NAV is not positive (undefined return).
		if !bucket.firstNAV.IsPos() {
			continue
		}

		var returnPct decimal.Decimal
		if bucket.count > 1 {
			diff, _ := bucket.lastNAV.Sub(bucket.firstNAV)
			returnPct, _ = diff.Quo(bucket.firstNAV)
			returnPct, _ = returnPct.Mul(decimal.MustNew(100, 0))
			returnPct = returnPct.Round(4)
		}

		result = append(result, YearlyReturn{
			Year:      year,
			ReturnPct: returnPct,
		})
	}

	return result
}
