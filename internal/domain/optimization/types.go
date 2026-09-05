// Package optimization provides shared types and infrastructure for portfolio
// optimization methods (Efficient Frontier, Hierarchical Risk Parity).
//
// Both optimization methods share the same data-fetching pipeline:
//   - Symbol resolution (internal symbol → market data symbol)
//   - Historical price fetching
//   - FX conversion to a common base currency
//   - Data span computation
//
// Domain-specific computation (mean-variance optimization, clustering, etc.)
// lives in the efficientfrontier and hierarchicalriskparity packages respectively.
package optimization

import (
	"context"
	"time"

	"github.com/eddiectc/portfoliolab/internal/market"
)

// --- Shared Interfaces ---

// MarketDataHistorySource fetches historical prices for a symbol.
type MarketDataHistorySource interface {
	GetHistoricalPrices(ctx context.Context, symbol string, start, end time.Time) ([]market.HistoricalPrice, error)
}

// MarketDataSymbolResolver maps an internal symbol to its market data provider symbol.
type MarketDataSymbolResolver interface {
	GetMarketDataSymbol(ctx context.Context, internalSymbol string) (string, error)
}

// SymbolLister returns the list of all known internal symbols.
type SymbolLister interface {
	ListAllSymbols(ctx context.Context) ([]string, error)
}

// PortfolioSymbolSource returns the distinct symbols held in a real portfolio.
type PortfolioSymbolSource interface {
	GetSymbolsByPortfolio(ctx context.Context, portfolioID int64) ([]string, error)
}

// ModelPortfolioSource retrieves a model portfolio by ID.
type ModelPortfolioSource interface {
	Get(ctx context.Context, id int64) (ModelPortfolioRef, error)
}

// ModelPortfolioRef is a lightweight reference to a model portfolio's entries.
type ModelPortfolioRef struct {
	Symbols []string
}

// FxRateSource returns the current FX rate between two currencies.
type FxRateSource interface {
	GetCurrentFxRate(ctx context.Context, baseCurrency, quoteCurrency string) (*FxRate, error)
}

// FxRate holds a currency exchange rate.
type FxRate struct {
	BaseCurrency  string
	QuoteCurrency string
	Rate          float64
}

// --- Shared Service Result Types ---

// DataSpan describes the actual date range and trading day count for a symbol.
type DataSpan struct {
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	TradingDays     int    `json:"trading_days"`
	RequestedPeriod string `json:"requested_period"`
	ActualPeriod    string `json:"actual_period"`
}

// ServiceResult wraps an optimization engine result with service-level metadata.
// Generic T is the engine-specific result type (e.g. FrontierResult, HrpResult).
type ServiceResult[T any] struct {
	// Result is the computation output (may have Message for empty states).
	Result *T
	// Warnings are non-fatal issues collected during data fetching.
	Warnings []string
	// ExcludedSymbols are symbols that could not be resolved or had no data.
	ExcludedSymbols []string
	// SymbolDataSpan is the actual data coverage per symbol.
	SymbolDataSpan map[string]DataSpan
}

// --- Shared Request Types ---

// ComputeRequest holds the common input for optimization computations.
type ComputeRequest struct {
	// Symbols is the list of candidate internal symbol names.
	Symbols []string
	// Period is the lookback period (e.g. "1Y", "3Y", "5Y").
	Period string
	// BaseCurrency is the target currency for price conversion.
	// If empty, the first symbol's currency is used as base.
	BaseCurrency string
}
