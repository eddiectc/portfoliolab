package market

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/govalues/decimal"
	yf "github.com/wnjoon/go-yfinance/pkg/ticker"
)

// MarketData represents a stored market data entry for either stock quotes
// or FX rates. Date uses empty string ("") as sentinel for "latest/current"
// and YYYY-MM-DD for historical snapshots.
type MarketData struct {
	Symbol   string           `json:"symbol"`
	Price    decimal.Decimal  `json:"price"`
	Currency string           `json:"currency"`
	DataType string           `json:"data_type"` // "stock" or "fx"
	Source   string           `json:"source"`     // provider identifier, e.g. "yahoo"
	Date     string           `json:"date"`       // "" = latest, "YYYY-MM-DD" = historical
	FetchedAt time.Time       `json:"fetched_at"`
}

// MarketDataFetcher fetches market data for both stock quotes and FX rates.
type MarketDataFetcher interface {
	FetchQuote(ctx context.Context, symbol string) (*MarketData, error)
	FetchFxRate(ctx context.Context, pair string) (*MarketData, error)
}

// YahooFinanceFetcher implements MarketDataFetcher using go-yfinance.
type YahooFinanceFetcher struct {
	logger *slog.Logger
}

// NewYahooFinanceFetcher creates a new YahooFinanceFetcher.
func NewYahooFinanceFetcher(logger *slog.Logger) *YahooFinanceFetcher {
	return &YahooFinanceFetcher{logger: logger}
}

// FetchQuote fetches the current quote for a symbol from Yahoo Finance.
// Returns a MarketData entry with data_type="stock" and date="" (latest).
// Returns an error if the fetch fails; the caller should handle gracefully.
func (f *YahooFinanceFetcher) FetchQuote(_ context.Context, symbol string) (*MarketData, error) {
	t, err := yf.New(symbol)
	if err != nil {
		f.logger.Warn("failed to create ticker", "symbol", symbol, "error", err)
		return nil, fmt.Errorf("failed to create ticker for %s: %w", symbol, err)
	}
	defer t.Close()

	quote, err := t.Quote()
	if err != nil {
		f.logger.Warn("failed to fetch quote", "symbol", symbol, "error", err)
		return nil, fmt.Errorf("failed to fetch quote for %s: %w", symbol, err)
	}

	price, err := decimal.NewFromFloat64(quote.RegularMarketPrice)
	if err != nil {
		f.logger.Warn("failed to convert price to decimal", "symbol", symbol, "price", quote.RegularMarketPrice, "error", err)
		return nil, fmt.Errorf("failed to convert price for %s: %w", symbol, err)
	}

	return &MarketData{
		Symbol:   quote.Symbol,
		Price:    price,
		Currency: quote.Currency,
		DataType: "stock",
		Source:   "yahoo",
		Date:     "",
		FetchedAt: time.Now(),
	}, nil
}

// FetchFxRate fetches the current FX rate for a currency pair from Yahoo Finance.
// The pair is specified as "BASE/QUOTE" (e.g. "GBP/USD").
// Returns a MarketData entry with data_type="fx" and date="" (latest).
func (f *YahooFinanceFetcher) FetchFxRate(_ context.Context, pair string) (*MarketData, error) {
	yahooSymbol := FxPairToYahooSymbol(pair)

	t, err := yf.New(yahooSymbol)
	if err != nil {
		f.logger.Warn("failed to create FX ticker", "pair", pair, "yahooSymbol", yahooSymbol, "error", err)
		return nil, fmt.Errorf("failed to create ticker for FX pair %s: %w", pair, err)
	}
	defer t.Close()

	quote, err := t.Quote()
	if err != nil {
		f.logger.Warn("failed to fetch FX quote", "pair", pair, "error", err)
		return nil, fmt.Errorf("failed to fetch FX rate for %s: %w", pair, err)
	}

	price, err := decimal.NewFromFloat64(quote.RegularMarketPrice)
	if err != nil {
		f.logger.Warn("failed to convert FX price to decimal", "pair", pair, "price", quote.RegularMarketPrice, "error", err)
		return nil, fmt.Errorf("failed to convert FX price for %s: %w", pair, err)
	}

	parts := strings.Split(pair, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid FX pair format: %s (expected BASE/QUOTE)", pair)
	}

	return &MarketData{
		Symbol:   pair,
		Price:    price,
		Currency: parts[1], // quote currency
		DataType: "fx",
		Source:   "yahoo",
		Date:     "",
		FetchedAt: time.Now(),
	}, nil
}

// FxPairToYahooSymbol converts a currency pair like "GBP/USD" to Yahoo's
// ticker format "GBPUSD=X".
func FxPairToYahooSymbol(pair string) string {
	parts := strings.Split(pair, "/")
	if len(parts) != 2 {
		return pair + "=X"
	}
	return parts[0] + parts[1] + "=X"
}
