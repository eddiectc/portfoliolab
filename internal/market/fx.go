package market

import (
	"context"
	"strings"
	"time"

	"github.com/govalues/decimal"
)

// FxRate represents a currency exchange rate.
type FxRate struct {
	Pair         string          // "BASE/QUOTE" (e.g. "GBP/USD")
	BaseCurrency string          // "GBP"
	QuoteCurrency string         // "USD"
	Rate         decimal.Decimal // 1 unit of base = X units of quote
	FetchedAt    time.Time
}

// FxRateFetcher fetches currency exchange rates.
type FxRateFetcher interface {
	FetchRate(ctx context.Context, pair string) (*FxRate, error)
}

// ParseFxPair splits a "BASE/QUOTE" pair into its components.
// Returns an error if the format is invalid.
func ParseFxPair(pair string) (base, quote string, err error) {
	parts := strings.Split(pair, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", &FxError{
			code:    "invalid_fx_pair",
			message: "invalid FX pair format: " + pair + " (expected BASE/QUOTE)",
		}
	}
	return parts[0], parts[1], nil
}

// FxError is a typed error for FX operations.
type FxError struct {
	code    string
	message string
}

func (e *FxError) Error() string {
	return e.message
}
