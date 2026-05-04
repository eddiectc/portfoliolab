package market

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/govalues/decimal"
	yf "github.com/wnjoon/go-yfinance/pkg/ticker"
)

// Quote represents a simplified market data quote for display purposes.
type Quote struct {
	Symbol      string          `json:"symbol"`
	Name        string          `json:"name"`
	Exchange    string          `json:"exchange"`
	Currency    string          `json:"currency"`
	LatestPrice decimal.Decimal `json:"latest_price"`
}

// QuoteFetcher fetches market data quotes for symbols.
type QuoteFetcher interface {
	FetchQuote(ctx context.Context, symbol string) (*Quote, error)
}

// YahooFinanceFetcher implements QuoteFetcher using go-yfinance.
type YahooFinanceFetcher struct {
	logger *slog.Logger
}

// NewYahooFinanceFetcher creates a new YahooFinanceFetcher.
func NewYahooFinanceFetcher(logger *slog.Logger) *YahooFinanceFetcher {
	return &YahooFinanceFetcher{logger: logger}
}

// FetchQuote fetches the current quote for a symbol from Yahoo Finance.
// Returns an error if the fetch fails; the caller should handle gracefully.
func (f *YahooFinanceFetcher) FetchQuote(_ context.Context, symbol string) (*Quote, error) {
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

	return &Quote{
		Symbol:      quote.Symbol,
		Name:        quote.ShortName,
		Exchange:    quote.Exchange,
		Currency:    quote.Currency,
		LatestPrice: price,
	}, nil
}
