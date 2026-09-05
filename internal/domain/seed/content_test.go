package seed

import (
	"testing"

	"github.com/eddiectc/portfoliolab/internal/domain/position"
	"github.com/eddiectc/portfoliolab/internal/domain/transaction"

	"github.com/govalues/decimal"
)

// TestContentPinnedDataset pins the entire fixed sample dataset: any change
// to a single field (date, symbol, price, quantity sign, net_cash sign,
// account name, benchmark flag, source URL, model entry) fails this test.
func TestContentPinnedDataset(t *testing.T) {
	c := NewDataset()

	// Portfolio.
	if c.Portfolio.Name != "Sample" || c.Portfolio.Currency != "GBP" {
		t.Fatalf("portfolio = %+v, want Sample/GBP", c.Portfolio)
	}

	// Accounts.
	if len(c.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(c.Accounts))
	}
	if c.Accounts[0].Account.Name != "Main Investment" {
		t.Errorf("account[0] name = %q, want Main Investment", c.Accounts[0].Account.Name)
	}
	if c.Accounts[1].Account.Name != "US Brokerage" {
		t.Errorf("account[1] name = %q, want US Brokerage", c.Accounts[1].Account.Name)
	}

	// Symbol mappings (D9: mappings only, no embedded details).
	wantSyms := map[string]struct {
		benchmark bool
		sourceURL string
	}{
		"VWRP.L": {true, "https://www.vanguardinvestor.co.uk/investments/vanguard-ftse-all-world-ucits-etf-usd-accumulating"},
		"VUTA.L": {true, "https://www.vanguardinvestor.co.uk/investments/vanguard-usd-treasury-bond-ucits-etf-usd-accumulating"},
		"GOOG":   {false, ""},
		"BRK-B":  {false, ""},
	}
	if len(c.Symbols) != len(wantSyms) {
		t.Fatalf("symbols = %d, want %d", len(c.Symbols), len(wantSyms))
	}
	for _, s := range c.Symbols {
		want, ok := wantSyms[s.InternalSymbol]
		if !ok {
			t.Errorf("unexpected symbol %q", s.InternalSymbol)
			continue
		}
		if s.MarketDataSymbol != s.InternalSymbol {
			t.Errorf("%s: market data symbol = %q, want same as internal", s.InternalSymbol, s.MarketDataSymbol)
		}
		if s.IsBenchmark != want.benchmark {
			t.Errorf("%s: is_benchmark = %v, want %v", s.InternalSymbol, s.IsBenchmark, want.benchmark)
		}
		if s.DataSourceURL != want.sourceURL {
			t.Errorf("%s: data_source_url = %q, want %q", s.InternalSymbol, s.DataSourceURL, want.sourceURL)
		}
		delete(wantSyms, s.InternalSymbol)
	}
	for name := range wantSyms {
		t.Errorf("missing symbol %q", name)
	}

	// Model portfolio.
	if c.Model.Name != "Sample Model" || len(c.Model.Entries) != 2 {
		t.Fatalf("model = %+v, want Sample Model with 2 entries", c.Model)
	}
	checkTarget(t, "model entry[0]", c.Model.Entries[0].Symbol, c.Model.Entries[0].WeightPct, "VWRP.L", "70")
	checkTarget(t, "model entry[1]", c.Model.Entries[1].Symbol, c.Model.Entries[1].WeightPct, "VUTA.L", "30")

	// Model and portfolio targets (each sums to 100).
	if len(c.ModelTargets) != 2 || len(c.PortfolioTargets) != 2 {
		t.Fatalf("targets: model = %d, portfolio = %d, want 2 each", len(c.ModelTargets), len(c.PortfolioTargets))
	}
	checkTarget(t, "model target[0]", c.ModelTargets[0].Symbol, c.ModelTargets[0].TargetPct, "VWRP.L", "70")
	checkTarget(t, "model target[1]", c.ModelTargets[1].Symbol, c.ModelTargets[1].TargetPct, "VUTA.L", "30")
	checkTarget(t, "portfolio target[0]", c.PortfolioTargets[0].Symbol, c.PortfolioTargets[0].TargetPct, "VWRP.L", "70")
	checkTarget(t, "portfolio target[1]", c.PortfolioTargets[1].Symbol, c.PortfolioTargets[1].TargetPct, "VUTA.L", "30")
	if s, err := c.ModelTargets[0].TargetPct.Add(c.ModelTargets[1].TargetPct); err != nil {
		t.Fatalf("model target sum: %v", err)
	} else if s.String() != "100" {
		t.Errorf("model target sum = %s, want 100", s.String())
	}
	if s, err := c.PortfolioTargets[0].TargetPct.Add(c.PortfolioTargets[1].TargetPct); err != nil {
		t.Fatalf("portfolio target sum: %v", err)
	} else if s.String() != "100" {
		t.Errorf("portfolio target sum = %s, want 100", s.String())
	}

	// Transactions: the complete pinned table (dates, types, symbols,
	// signed quantities, prices, currencies, signed net cash).
	wantTxs := map[string][]txWant{
		"Main Investment": {
			{"2024-07-01", "deposit", "$CASH-GBP", "100000.00", "1.00", "GBP", "100000.00"},
			{"2024-07-01", "buy", "VWRP.L", "666", "105.00", "GBP", "-69930.00"},
			{"2024-07-01", "buy", "VUTA.L", "1522", "19.70", "GBP", "-29983.40"},
			{"2025-07-15", "sell", "VWRP.L", "-18", "144.60", "GBP", "2602.80"},
			{"2025-07-15", "buy", "VUTA.L", "108", "19.65", "GBP", "-2122.20"},
		},
		"US Brokerage": {
			{"2024-07-01", "deposit", "$CASH-USD", "30000.00", "1.00", "USD", "30000.00"},
			{"2024-07-01", "buy", "GOOG", "40", "148.50", "USD", "-5940.00"},
			{"2024-07-01", "buy", "BRK-B", "12", "412.30", "USD", "-4947.60"},
			{"2026-03-10", "buy", "GOOG", "4", "152.30", "USD", "-609.20"},
			{"2026-03-10", "buy", "BRK-B", "1", "418.90", "USD", "-418.90"},
		},
	}
	total := 0
	for i, acc := range c.Accounts {
		name := c.Accounts[i].Account.Name
		want, ok := wantTxs[name]
		if !ok {
			t.Fatalf("no expected transactions for account %q", name)
		}
		if len(acc.Transactions) != len(want) {
			t.Fatalf("account %s: %d transactions, want %d", name, len(acc.Transactions), len(want))
		}
		for j, w := range want {
			tx := acc.Transactions[j]
			if tx.Date.Format("2006-01-02") != w.date || tx.Type != w.ttype || tx.Symbol != w.symbol ||
				tx.Quantity.String() != w.qty || tx.Price.String() != w.price || tx.Currency != w.cur ||
				tx.NetCash.String() != w.netCash {
				t.Errorf("account %s tx[%d] = date %s type %s symbol %s qty %s price %s ccy %s net_cash %s, want %v",
					name, j, tx.Date.Format("2006-01-02"), tx.Type, tx.Symbol, tx.Quantity.String(),
					tx.Price.String(), tx.Currency, tx.NetCash.String(), w)
			}
			total++
		}
	}
	if total != 10 {
		t.Errorf("total transactions = %d, want 10", total)
	}

	// Cash positions implied by the dataset (via the existing position math):
	// GBP: 100000 - 69930 - 29983.40 + 2602.80 - 2122.20 = 567.20
	// USD: 30000 - 5940 - 4947.60 - 609.20 - 418.90 = +18084.30
	var all []transaction.Transaction
	for _, acc := range c.Accounts {
		all = append(all, acc.Transactions...)
	}
	cash := position.ComputeCashPositions(all)
	wantCash := map[string]string{
		"$CASH-GBP": "567.20",
		"$CASH-USD": "18084.30",
	}
	if len(cash) != len(wantCash) {
		t.Fatalf("cash positions = %d, want %d", len(cash), len(wantCash))
	}
	for _, p := range cash {
		want, ok := wantCash[p.Symbol]
		if !ok {
			t.Errorf("unexpected cash position %q", p.Symbol)
			continue
		}
		if p.Quantity.String() != want {
			t.Errorf("cash %s quantity = %s, want %s", p.Symbol, p.Quantity.String(), want)
		}
	}
}

// checkTarget asserts a target/entry pair by symbol and exact percentage
// string.
func checkTarget(t *testing.T, label, symbol string, pct decimal.Decimal, wantSymbol, wantPct string) {
	t.Helper()
	if symbol != wantSymbol || pct.String() != wantPct {
		t.Errorf("%s = %s %s, want %s %s", label, symbol, pct.String(), wantSymbol, wantPct)
	}
}

type txWant struct {
	date    string
	ttype   string
	symbol  string
	qty     string
	price   string
	cur     string
	netCash string
}
