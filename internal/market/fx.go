package market

import (
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

// FormatFxPair returns a standard "BASE/QUOTE" pair string for display and storage.
func FormatFxPair(baseCurrency, quoteCurrency string) string {
	return fmt.Sprintf("%s/%s", baseCurrency, quoteCurrency)
}
