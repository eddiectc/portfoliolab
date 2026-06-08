// Package util provides shared utility functions used across the application.
package util

import "time"

// PeriodCutoff returns the start date for the given lookback period string
// and a warning if the period was unrecognized (defaults to 1Y).
func PeriodCutoff(period string) (time.Time, string) {
	now := time.Now()
	switch period {
	case "3M":
		return now.AddDate(0, -3, 0), ""
	case "6M":
		return now.AddDate(0, -6, 0), ""
	case "1Y":
		return now.AddDate(-1, 0, 0), ""
	case "3Y":
		return now.AddDate(-3, 0, 0), ""
	case "5Y":
		return now.AddDate(-5, 0, 0), ""
	case "10Y":
		return now.AddDate(-10, 0, 0), ""
	default:
		return now.AddDate(-1, 0, 0),
			"unrecognized period " + period + " — defaulting to 1Y"
	}
}
