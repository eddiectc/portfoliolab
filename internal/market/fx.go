package market

import (
	"context"
	"fmt"
	"time"

	"github.com/govalues/decimal"
)

// FxRate represents a currency exchange rate.
type FxRate struct {
	BaseCurrency  string          // e.g. "GBP"
	QuoteCurrency string          // e.g. "USD"
	Rate          decimal.Decimal // 1 unit of base = X units of quote
	FetchedAt     time.Time
}

// FxRateFetcher fetches currency exchange rates.
type FxRateFetcher interface {
	// FetchRate fetches the exchange rate for baseCurrency → quoteCurrency.
	// E.g., FetchRate(ctx, "GBP", "USD") returns how many USD per 1 GBP.
	FetchRate(ctx context.Context, baseCurrency, quoteCurrency string) (*FxRate, error)
}

// FormatFxPair returns a standard "BASE/QUOTE" pair string for display and storage.
func FormatFxPair(baseCurrency, quoteCurrency string) string {
	return fmt.Sprintf("%s/%s", baseCurrency, quoteCurrency)
}

// FxError is a typed error for FX operations.
type FxError struct {
	code    string
	message string
}

func (e *FxError) Error() string {
	return e.message
}
