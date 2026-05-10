package market

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/govalues/decimal"
	"github.com/wnjoon/go-yfinance/pkg/multi"
	"github.com/wnjoon/go-yfinance/pkg/models"
	yf "github.com/wnjoon/go-yfinance/pkg/ticker"
)

// HistoricalPrice is a single daily historical price point fetched from
// a market data provider. Close is the unadjusted closing price.
type HistoricalPrice struct {
	Date     time.Time       // trading day
	Close    decimal.Decimal // unadjusted close price
	Currency string          // e.g. "USD", "GBP"
}

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
	// FetchHistoricalPricesBatch fetches daily historical prices for multiple
	// symbols over a date range using a shared HTTP client. Returns a map of
	// symbol → prices (sorted by date ASC) and a slice of failed symbol names.
	FetchHistoricalPricesBatch(ctx context.Context, symbols []string, start, end time.Time) (map[string][]HistoricalPrice, []string)
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

// FetchHistoricalPricesBatch fetches daily historical prices for multiple
// symbols over [start, end] using go-yfinance's multi package with a shared
// HTTP client (single auth session). Uses AutoAdjust: false (unadjusted prices)
// for accurate portfolio valuation and Interval: "1d".
// Returns a map of symbol → prices (sorted by date ASC) and a slice of failed
// symbol names. Bars outside [start, end] are filtered out.
func (f *YahooFinanceFetcher) FetchHistoricalPricesBatch(_ context.Context, symbols []string, start, end time.Time) (map[string][]HistoricalPrice, []string) {
	result := make(map[string][]HistoricalPrice)
	if len(symbols) == 0 {
		return result, nil
	}

	// multi.NewTickers creates tickers sharing one HTTP client,
	// so cookie/crumb auth is done once and reused for all symbols.
	tickers, err := multi.NewTickers(symbols)
	if err != nil {
		f.logger.Warn("failed to create tickers for historical batch", "error", err)
		return result, symbols
	}
	defer tickers.Close()

	var failedSymbols []string
	for _, sym := range tickers.Symbols() {
		tkr := tickers.Get(sym)
		if tkr == nil {
			failedSymbols = append(failedSymbols, sym)
			continue
		}

		bars, err := tkr.History(models.HistoryParams{
			Start:      &start,
			End:        &end,
			Interval:   "1d",
			AutoAdjust: false,
		})
		if err != nil {
			f.logger.Debug("failed to fetch historical prices", "symbol", sym, "error", err)
			failedSymbols = append(failedSymbols, sym)
			continue
		}

		// Get currency from the cached chart metadata (set by History()).
		meta := tkr.GetHistoryMetadata()
		currency := ""
		if meta != nil {
			currency = meta.Currency
		}

		var prices []HistoricalPrice
		for _, bar := range bars {
			if bar.Date.Before(start) || bar.Date.After(end) {
				continue
			}
			close, convErr := decimal.NewFromFloat64(bar.Close)
			if convErr != nil {
				f.logger.Debug("failed to convert close price", "symbol", sym, "close", bar.Close, "error", convErr)
				continue
			}
			prices = append(prices, HistoricalPrice{
				Date:     bar.Date,
				Close:    close,
				Currency: currency,
			})
		}
		if len(prices) > 0 {
			result[sym] = prices
		}
	}

	return result, failedSymbols
}

// FxPairToYahooSymbol converts base/quote currencies to Yahoo's
// ticker format, e.g. ("GBP", "USD") → "GBPUSD=X".
func FxPairToYahooSymbol(baseCurrency, quoteCurrency string) string {
	return baseCurrency + quoteCurrency + "=X"
}
