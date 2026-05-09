package market

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/govalues/decimal"
	"github.com/wnjoon/go-yfinance/pkg/multi"
	yf "github.com/wnjoon/go-yfinance/pkg/ticker"
)

// MarketData represents a stored market data entry for either stock quotes
// or FX rates. Date uses empty string ("") as sentinel for "latest/current"
// and YYYY-MM-DD for historical snapshots.
type MarketData struct {
	Symbol    string          `json:"symbol"`
	Price     decimal.Decimal `json:"price"`
	Currency  string          `json:"currency"`
	DataType  string          `json:"data_type"` // "stock" or "fx"
	Source    string          `json:"source"`     // provider identifier, e.g. "yahoo"
	Date      string          `json:"date"`       // "" = latest, "YYYY-MM-DD" = historical
	FetchedAt time.Time       `json:"fetched_at"`
}

// MarketDataFetcher fetches market data for both stock quotes and FX rates.
type MarketDataFetcher interface {
	FetchQuote(ctx context.Context, symbol string) (*MarketData, error)
	// FetchFxRate fetches the FX rate for baseCurrency → quoteCurrency.
	FetchFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*MarketData, error)
	// FetchQuotesBatch fetches quotes for multiple symbols using a shared
	// HTTP client (single auth session). Returns a map of symbol → MarketData
	// for successfully fetched quotes. Symbols that fail are omitted.
	FetchQuotesBatch(ctx context.Context, symbols []string) map[string]*MarketData
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

// FetchFxRate fetches the current FX rate from Yahoo Finance.
// baseCurrency is the source currency, quoteCurrency is the target.
// E.g., FetchFxRate(ctx, "GBP", "USD") returns how many USD per 1 GBP.
// Returns a MarketData entry with data_type="fx" and date="" (latest).
func (f *YahooFinanceFetcher) FetchFxRate(_ context.Context, baseCurrency, quoteCurrency string) (*MarketData, error) {
	yahooSymbol := FxPairToYahooSymbol(baseCurrency, quoteCurrency)

	t, err := yf.New(yahooSymbol)
	if err != nil {
		f.logger.Warn("failed to create FX ticker", "base", baseCurrency, "quote", quoteCurrency, "yahooSymbol", yahooSymbol, "error", err)
		return nil, fmt.Errorf("failed to create ticker for %s/%s: %w", baseCurrency, quoteCurrency, err)
	}
	defer t.Close()

	quote, err := t.Quote()
	if err != nil {
		f.logger.Warn("failed to fetch FX quote", "base", baseCurrency, "quote", quoteCurrency, "error", err)
		return nil, fmt.Errorf("failed to fetch FX rate for %s/%s: %w", baseCurrency, quoteCurrency, err)
	}

	price, err := decimal.NewFromFloat64(quote.RegularMarketPrice)
	if err != nil {
		f.logger.Warn("failed to convert FX price to decimal", "base", baseCurrency, "quote", quoteCurrency, "price", quote.RegularMarketPrice, "error", err)
		return nil, fmt.Errorf("failed to convert FX price for %s/%s: %w", baseCurrency, quoteCurrency, err)
	}

	return &MarketData{
		Symbol:    FormatFxPair(baseCurrency, quoteCurrency),
		Price:     price,
		Currency:  quoteCurrency,
		DataType:  "fx",
		Source:    "yahoo",
		Date:      "",
		FetchedAt: time.Now(),
	}, nil
}

// FetchRate fetches the current FX rate and returns an FxRate.
// This satisfies the FxRateFetcher interface.
func (f *YahooFinanceFetcher) FetchRate(ctx context.Context, baseCurrency, quoteCurrency string) (*FxRate, error) {
	md, err := f.FetchFxRate(ctx, baseCurrency, quoteCurrency)
	if err != nil {
		return nil, err
	}

	return &FxRate{
		BaseCurrency:  baseCurrency,
		QuoteCurrency: quoteCurrency,
		Rate:          md.Price,
		FetchedAt:     md.FetchedAt,
	}, nil
}

// FetchQuotesBatch fetches quotes for multiple symbols using a shared HTTP
// client (single auth session via go-yfinance's multi package). Returns a map
// of symbol → MarketData for successfully fetched quotes. Symbols that fail
// to fetch are omitted from the result.
func (f *YahooFinanceFetcher) FetchQuotesBatch(_ context.Context, symbols []string) map[string]*MarketData {
	result := make(map[string]*MarketData)
	if len(symbols) == 0 {
		return result
	}

	// multi.NewTickers creates tickers sharing one HTTP client,
	// so cookie/crumb auth is done once and reused for all symbols.
	tickers, err := multi.NewTickers(symbols)
	if err != nil {
		f.logger.Warn("failed to create tickers for batch fetch", "error", err)
		return result
	}
	defer tickers.Close()

	now := time.Now()
	for _, sym := range tickers.Symbols() {
		tkr := tickers.Get(sym)
		if tkr == nil {
			continue
		}

		quote, err := tkr.Quote()
		if err != nil {
			f.logger.Debug("failed to fetch quote in batch", "symbol", sym, "error", err)
			continue
		}

		price, err := decimal.NewFromFloat64(quote.RegularMarketPrice)
		if err != nil {
			f.logger.Warn("failed to convert batch price", "symbol", sym, "error", err)
			continue
		}

		result[sym] = &MarketData{
			Symbol:   quote.Symbol,
			Price:    price,
			Currency: quote.Currency,
			DataType: "stock",
			Source:   "yahoo",
			Date:     "",
			FetchedAt: now,
		}
	}

	return result
}

// FxPairToYahooSymbol converts base/quote currencies to Yahoo's
// ticker format, e.g. ("GBP", "USD") → "GBPUSD=X".
func FxPairToYahooSymbol(baseCurrency, quoteCurrency string) string {
	return baseCurrency + quoteCurrency + "=X"
}
