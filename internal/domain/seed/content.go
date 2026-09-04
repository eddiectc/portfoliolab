// Package seed contains the fixed sample dataset created automatically on
// startup of a deployment with no portfolio (f030). The dataset is expressed
// in existing domain types (portfolio, account, symbol mapping, transaction,
// model portfolio, allocation target) so no parallel type system is needed.
//
// No symbol details are embedded here (D9): mappings carry data_source_url
// for the two Vanguard ETFs, and the startup background refresh fills in
// details and live prices (Vanguard extractor for URL'd symbols, Yahoo
// otherwise).
package seed

import (
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/allocation"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"

	"github.com/govalues/decimal"
)

// AccountSeed pairs an account with its transactions (deposits first).
type AccountSeed struct {
	Account      account.Account
	Transactions []transaction.Transaction
}

// Dataset is the complete fixed sample dataset.
type Dataset struct {
	Portfolio        portfolio.Portfolio
	Accounts         []AccountSeed
	Symbols          []symbolmapping.SymbolMapping
	Model            modelportfolio.ModelPortfolio
	ModelTargets     []allocation.TargetEntry
	PortfolioTargets []allocation.TargetEntry
}

// vanguardBaseURL is the base URL for the sample Vanguard ETF source pages.
const vanguardBaseURL = "https://www.vanguardinvestor.co.uk/investments"

// seedDate is the fixed date for a single calendar day in the dataset.
func seedDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// Dataset returns the exact sample dataset described in the f030 spec
// (values user-confirmed 2026-07-21, D8/D10).
//
// Dataset shape:
//   - Portfolio "Sample" (GBP) with two accounts: "Main Investment" (GBP)
//     and "US Brokerage" (USD).
//   - 10 transactions: 2 initial deposits (2024-07-01) and 8 buy/sell
//     trades (2024-07-01, 2025-07-15, 2026-03-10).
//   - Symbol mappings for VWRP.L, VUTA.L (benchmark, with data_source_url)
//     and GOOG, BRK-B.
//   - "Sample Model" model portfolio and 70/30 target allocations on both
//     the model and the sample portfolio.
func NewDataset() *Dataset {
	d20240701 := seedDate(2024, time.July, 1)
	d20250715 := seedDate(2025, time.July, 15)
	d20260310 := seedDate(2026, time.March, 10)

	vwrpURL := vanguardBaseURL + "/vanguard-ftse-all-world-ucits-etf-usd-accumulating"
	vutaURL := vanguardBaseURL + "/vanguard-usd-treasury-bond-ucits-etf-usd-accumulating"

	cashGBP := "$CASH-GBP"
	cashUSD := "$CASH-USD"

	return &Dataset{
		Portfolio: portfolio.Portfolio{Name: "Sample", Currency: "GBP"},
		Accounts: []AccountSeed{
			{
				Account: account.Account{Name: "Main Investment"},
				Transactions: []transaction.Transaction{
					{
						// D1: initial cash as a deposit (positive net cash).
						Date: d20240701, Type: "deposit", Symbol: cashGBP,
						Quantity: decimal.MustParse("100000.00"),
						Price:    decimal.MustParse("1.00"),
						Currency: "GBP",
						NetCash:  decimal.MustParse("100000.00"),
					},
					{
						Date: d20240701, Type: "buy", Symbol: "VWRP.L",
						Quantity: decimal.MustParse("666"),
						Price:    decimal.MustParse("105.00"),
						Currency: "GBP",
						NetCash:  decimal.MustParse("-69930.00"),
					},
					{
						Date: d20240701, Type: "buy", Symbol: "VUTA.L",
						Quantity: decimal.MustParse("1522"),
						Price:    decimal.MustParse("19.70"),
						Currency: "GBP",
						NetCash:  decimal.MustParse("-29983.40"),
					},
					{
						// 2025 rebalance toward the 70/30 target:
						// 648 VWRP.L / 1630 VUTA.L.
						Date: d20250715, Type: "sell", Symbol: "VWRP.L",
						Quantity: decimal.MustParse("-18"),
						Price:    decimal.MustParse("144.60"),
						Currency: "GBP",
						NetCash:  decimal.MustParse("2602.80"),
					},
					{
						Date: d20250715, Type: "buy", Symbol: "VUTA.L",
						Quantity: decimal.MustParse("108"),
						Price:    decimal.MustParse("19.65"),
						Currency: "GBP",
						NetCash:  decimal.MustParse("-2122.20"),
					},
				},
			},
			{
				Account: account.Account{Name: "US Brokerage"},
				Transactions: []transaction.Transaction{
					{
						// D1: initial cash as a deposit (positive net cash).
						Date: d20240701, Type: "deposit", Symbol: cashUSD,
						Quantity: decimal.MustParse("30000.00"),
						Price:    decimal.MustParse("1.00"),
						Currency: "USD",
						NetCash:  decimal.MustParse("30000.00"),
					},
					{
						Date: d20240701, Type: "buy", Symbol: "GOOG",
						Quantity: decimal.MustParse("40"),
						Price:    decimal.MustParse("148.50"),
						Currency: "USD",
						NetCash:  decimal.MustParse("-5940.00"),
					},
					{
						Date: d20240701, Type: "buy", Symbol: "BRK-B",
						Quantity: decimal.MustParse("12"),
						Price:    decimal.MustParse("412.30"),
						Currency: "USD",
						NetCash:  decimal.MustParse("-4947.60"),
					},
					{
						Date: d20260310, Type: "buy", Symbol: "GOOG",
						Quantity: decimal.MustParse("4"),
						Price:    decimal.MustParse("152.30"),
						Currency: "USD",
						NetCash:  decimal.MustParse("-609.20"),
					},
					{
						Date: d20260310, Type: "buy", Symbol: "BRK-B",
						Quantity: decimal.MustParse("1"),
						Price:    decimal.MustParse("418.90"),
						Currency: "USD",
						NetCash:  decimal.MustParse("-418.90"),
					},
				},
			},
		},
		Symbols: []symbolmapping.SymbolMapping{
			{
				InternalSymbol:   "VWRP.L",
				MarketDataSymbol: "VWRP.L",
				IsBenchmark:      true,
				DataSourceURL:    vwrpURL,
			},
			{
				InternalSymbol:   "VUTA.L",
				MarketDataSymbol: "VUTA.L",
				IsBenchmark:      true,
				DataSourceURL:    vutaURL,
			},
			{
				InternalSymbol:   "GOOG",
				MarketDataSymbol: "GOOG",
			},
			{
				InternalSymbol:   "BRK-B",
				MarketDataSymbol: "BRK-B",
			},
		},
		Model: modelportfolio.ModelPortfolio{
			Name: "Sample Model",
			Entries: []modelportfolio.ModelPortfolioEntry{
				{Symbol: "VWRP.L", WeightPct: decimal.MustParse("70")},
				{Symbol: "VUTA.L", WeightPct: decimal.MustParse("30")},
			},
		},
		// Model targets mirror the model entries (70/30).
		ModelTargets: []allocation.TargetEntry{
			{Symbol: "VWRP.L", TargetPct: decimal.MustParse("70")},
			{Symbol: "VUTA.L", TargetPct: decimal.MustParse("30")},
		},
		// The sample portfolio carries the same 70/30 target so the
		// allocation page (actual vs target) renders out of the box.
		PortfolioTargets: []allocation.TargetEntry{
			{Symbol: "VWRP.L", TargetPct: decimal.MustParse("70")},
			{Symbol: "VUTA.L", TargetPct: decimal.MustParse("30")},
		},
	}
}
