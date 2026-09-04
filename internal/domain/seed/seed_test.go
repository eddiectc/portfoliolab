package seed

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// mockStore simulates the data-layer store with internal state so the service
// under test is exercised against realistic guard behavior.
type mockStore struct {
	portfolioExists bool
	wantDataset     *Dataset // the dataset passed to Seed, for assertions
	result          *Result
	seedErr         error
	seedCalls       int
}

func (m *mockStore) Seed(_ context.Context, d *Dataset) (*Result, error) {
	m.seedCalls++
	m.wantDataset = d
	if m.portfolioExists {
		return nil, ErrPortfolioExists
	}
	if m.seedErr != nil {
		return nil, m.seedErr
	}
	return m.result, nil
}

// mockRecalc records RecalculatePortfolio calls.
type mockRecalc struct {
	err   error
	calls []int64 // portfolio IDs
}

func (m *mockRecalc) RecalculatePortfolio(_ context.Context, portfolioID int64) error {
	m.calls = append(m.calls, portfolioID)
	return m.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestSeedDemoSkipsWhenPortfolioExists(t *testing.T) {
	store := &mockStore{portfolioExists: true}
	recalc := &mockRecalc{}
	svc := NewService(store, recalc, testLogger())

	if err := svc.SeedDemo(context.Background()); err != nil {
		t.Fatalf("SeedDemo() error = %v, want nil (skip is not a failure)", err)
	}
	if len(recalc.calls) != 0 {
		t.Errorf("recalculate called %d times, want 0 on skip", len(recalc.calls))
	}
	if store.seedCalls != 1 {
		t.Errorf("store.Seed called %d times, want 1", store.seedCalls)
	}
}

func TestSeedDemoAssignsUniqueLotIDsToTradesOnly(t *testing.T) {
	store := &mockStore{result: &Result{PortfolioID: 1}}
	recalc := &mockRecalc{}
	svc := NewService(store, recalc, testLogger())

	if err := svc.SeedDemo(context.Background()); err != nil {
		t.Fatalf("SeedDemo() error = %v", err)
	}
	if store.wantDataset == nil {
		t.Fatal("store did not receive a dataset")
	}

	seen := map[string]bool{}
	trades, deposits := 0, 0
	for _, acc := range store.wantDataset.Accounts {
		for _, tx := range acc.Transactions {
			if tx.Type == "buy" || tx.Type == "sell" {
				trades++
				if tx.LotID == nil || *tx.LotID == "" {
					t.Errorf("%s %s: lot ID not assigned", acc.Account.Name, tx.Symbol)
					continue
				}
				if !strings.HasPrefix(*tx.LotID, "LOT-") {
					t.Errorf("%s %s: lot ID %q does not have LOT- prefix", acc.Account.Name, tx.Symbol, *tx.LotID)
				}
				if seen[*tx.LotID] {
					t.Errorf("duplicate lot ID %q", *tx.LotID)
				}
				seen[*tx.LotID] = true
			} else {
				deposits++
				if tx.LotID != nil {
					t.Errorf("%s %s: deposit has lot ID %v, want nil", acc.Account.Name, tx.Symbol, *tx.LotID)
				}
			}
		}
	}
	if trades != 8 {
		t.Errorf("trade transactions = %d, want 8", trades)
	}
	if deposits != 2 {
		t.Errorf("deposit transactions = %d, want 2", deposits)
	}
	if len(seen) != 8 {
		t.Errorf("unique lot IDs = %d, want 8", len(seen))
	}
}

func TestSeedDemoCallsRecalcWithPortfolioID(t *testing.T) {
	store := &mockStore{result: &Result{PortfolioID: 42}}
	recalc := &mockRecalc{}
	svc := NewService(store, recalc, testLogger())

	if err := svc.SeedDemo(context.Background()); err != nil {
		t.Fatalf("SeedDemo() error = %v", err)
	}
	if len(recalc.calls) != 1 || recalc.calls[0] != 42 {
		t.Errorf("recalculate calls = %v, want [42]", recalc.calls)
	}
}

func TestSeedDemoSwallowsRecalcFailure(t *testing.T) {
	store := &mockStore{result: &Result{PortfolioID: 1}}
	recalc := &mockRecalc{err: errors.New("recalc boom")}
	svc := NewService(store, recalc, testLogger())

	if err := svc.SeedDemo(context.Background()); err != nil {
		t.Fatalf("SeedDemo() error = %v, want nil (recalc failure is non-fatal)", err)
	}
}

func TestSeedDemoReturnsStoreError(t *testing.T) {
	store := &mockStore{seedErr: errors.New("db down")}
	recalc := &mockRecalc{}
	svc := NewService(store, recalc, testLogger())

	err := svc.SeedDemo(context.Background())
	if !errors.Is(err, store.seedErr) {
		t.Fatalf("SeedDemo() error = %v, want store error", err)
	}
	if len(recalc.calls) != 0 {
		t.Errorf("recalculate called %d times, want 0 after store failure", len(recalc.calls))
	}
}
