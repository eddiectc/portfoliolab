package marketdata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/govalues/decimal"
)

// mockStore simulates the data-layer store with internal state and a call log
// so the service under test is exercised against realistic guard behavior.
type mockStore struct {
	upserted    *Override
	upsertErr   error
	upsertCalls int

	deleted      bool
	deleteErr    error
	deleteCalls  int
	deleteSymbol string
	deleteDate   string
	deleteType   DataType

	listed       []Override
	listErr      error
	listCalls    int
	listSymbol   *string
	listSymbolOK bool
}

func (m *mockStore) Upsert(_ context.Context, o *Override) error {
	m.upsertCalls++
	m.upserted = o
	return m.upsertErr
}

func (m *mockStore) Delete(_ context.Context, symbol, date string, dt DataType) (bool, error) {
	m.deleteCalls++
	m.deleteSymbol = symbol
	m.deleteDate = date
	m.deleteType = dt
	if m.deleteErr != nil {
		return false, m.deleteErr
	}
	return m.deleted, nil
}

func (m *mockStore) List(_ context.Context, symbol *string) ([]Override, error) {
	m.listCalls++
	m.listSymbol = symbol
	m.listSymbolOK = symbol != nil
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listed, nil
}

func mustDec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	return decimal.MustParse(s)
}

func TestSet(t *testing.T) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")

	price := mustDec(t, "44.02")

	tests := []struct {
		name     string
		req      SetRequest
		wantErr  error
		wantCall bool
	}{
		{
			name: "valid stock override",
			req:  SetRequest{Symbol: "XNAQ.L", Date: "2025-10-24", DataType: DataTypeStock, Price: price, Currency: "GBP"},
		},
		{
			name: "valid fx override on today",
			req:  SetRequest{Symbol: "GBPUSD", Date: today, DataType: DataTypeFx, Price: mustDec(t, "1.305"), Currency: "USD"},
		},
		{
			name: "valid nav override on yesterday",
			req:  SetRequest{Symbol: "VOOE.L", Date: yesterday, DataType: DataTypeNav, Price: mustDec(t, "1.1005"), Currency: "USD"},
		},
		{
			name:     "empty symbol",
			req:      SetRequest{Symbol: "", Date: yesterday, DataType: DataTypeStock, Price: price, Currency: "GBP"},
			wantErr:  ErrInvalidSymbol,
			wantCall: false,
		},
		{
			name:     "zero price",
			req:      SetRequest{Symbol: "XNAQ.L", Date: yesterday, DataType: DataTypeStock, Price: decimal.Zero, Currency: "GBP"},
			wantErr:  ErrInvalidPrice,
			wantCall: false,
		},
		{
			name:     "negative price",
			req:      SetRequest{Symbol: "XNAQ.L", Date: yesterday, DataType: DataTypeStock, Price: mustDec(t, "-0.01"), Currency: "GBP"},
			wantErr:  ErrInvalidPrice,
			wantCall: false,
		},
		{
			name:     "malformed date 2025-13-40",
			req:      SetRequest{Symbol: "XNAQ.L", Date: "2025-13-40", DataType: DataTypeStock, Price: price, Currency: "GBP"},
			wantErr:  ErrInvalidDate,
			wantCall: false,
		},
		{
			name:     "malformed date 01/01/2025",
			req:      SetRequest{Symbol: "XNAQ.L", Date: "01/01/2025", DataType: DataTypeStock, Price: price, Currency: "GBP"},
			wantErr:  ErrInvalidDate,
			wantCall: false,
		},
		{
			name:     "future date",
			req:      SetRequest{Symbol: "XNAQ.L", Date: tomorrow, DataType: DataTypeStock, Price: price, Currency: "GBP"},
			wantErr:  ErrInvalidDate,
			wantCall: false,
		},
		{
			name:     "unknown data type",
			req:      SetRequest{Symbol: "XNAQ.L", Date: yesterday, DataType: "dividend", Price: price, Currency: "GBP"},
			wantErr:  ErrInvalidDataType,
			wantCall: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{}
			svc := NewService(store)

			got, err := svc.Set(context.Background(), tc.req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Set() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if store.upsertCalls != 0 {
					t.Errorf("store.Upsert called %d times, want 0 on validation failure", store.upsertCalls)
				}
				if got != nil {
					t.Errorf("Set() = %v, want nil on error", got)
				}
				return
			}

			if store.upsertCalls != 1 {
				t.Fatalf("store.Upsert called %d times, want 1", store.upsertCalls)
			}
			o := store.upserted
			if o.Symbol != tc.req.Symbol {
				t.Errorf("Symbol = %q, want %q", o.Symbol, tc.req.Symbol)
			}
			if o.Date != tc.req.Date {
				t.Errorf("Date = %q, want %q", o.Date, tc.req.Date)
			}
			if o.DataType != tc.req.DataType {
				t.Errorf("DataType = %q, want %q", o.DataType, tc.req.DataType)
			}
			if !o.Price.Equal(tc.req.Price) {
				t.Errorf("Price = %v, want %v", o.Price, tc.req.Price)
			}
			if o.Currency != tc.req.Currency {
				t.Errorf("Currency = %q, want %q", o.Currency, tc.req.Currency)
			}
			if o.Source != "override" {
				t.Errorf("Source = %q, want %q", o.Source, "override")
			}
		})
	}
}

func TestSetPropagatesStoreError(t *testing.T) {
	store := &mockStore{upsertErr: errors.New("db down")}
	svc := NewService(store)

	_, err := svc.Set(context.Background(), SetRequest{
		Symbol: "XNAQ.L", Date: "2025-10-24", DataType: DataTypeStock,
		Price: mustDec(t, "44.02"), Currency: "GBP",
	})
	if !errors.Is(err, store.upsertErr) {
		t.Fatalf("Set() error = %v, want %v", err, store.upsertErr)
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name      string
		deleted   bool
		deleteErr error
		dataType  DataType
		wantErr   error
	}{
		{name: "existing override", deleted: true, dataType: DataTypeStock},
		{name: "no matching override", dataType: DataTypeStock, wantErr: ErrNotFound},
		{name: "store error", deleted: true, deleteErr: errors.New("db down"), dataType: DataTypeNav},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{deleted: tc.deleted, deleteErr: tc.deleteErr}
			svc := NewService(store)

			err := svc.Remove(context.Background(), "XNAQ.L", "2025-10-24", tc.dataType)
			if tc.deleteErr != nil {
				if !errors.Is(err, tc.deleteErr) {
					t.Fatalf("Remove() error = %v, want %v", err, tc.deleteErr)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Remove() error = %v, want %v", err, tc.wantErr)
			}
			if store.deleteCalls != 1 {
				t.Fatalf("store.Delete called %d times, want 1", store.deleteCalls)
			}
			if store.deleteSymbol != "XNAQ.L" || store.deleteDate != "2025-10-24" || store.deleteType != tc.dataType {
				t.Errorf("store.Delete(%q, %q, %q), want (XNAQ.L, 2025-10-24, %q)",
					store.deleteSymbol, store.deleteDate, store.deleteType, tc.dataType)
			}
		})
	}
}

func TestRemoveValidatesDataTypeBeforeStore(t *testing.T) {
	store := &mockStore{}
	svc := NewService(store)

	err := svc.Remove(context.Background(), "XNAQ.L", "2025-10-24", "dividend")
	if !errors.Is(err, ErrInvalidDataType) {
		t.Fatalf("Remove() error = %v, want %v", err, ErrInvalidDataType)
	}
	if store.deleteCalls != 0 {
		t.Errorf("store.Delete called %d times, want 0 on invalid data type", store.deleteCalls)
	}
}

func TestList(t *testing.T) {
	all := []Override{{Symbol: "A", Date: "2025-01-01", DataType: DataTypeStock}}
	sym := "XNAQ.L"

	tests := []struct {
		name        string
		symbol      *string
		wantNilPass bool
	}{
		{name: "nil symbol lists all", symbol: nil, wantNilPass: true},
		{name: "symbol filters", symbol: &sym, wantNilPass: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{listed: all}
			svc := NewService(store)

			got, err := svc.List(context.Background(), tc.symbol)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(got) != len(all) || got[0].Symbol != "A" {
				t.Fatalf("List() = %+v, want pass-through of %+v", got, all)
			}
			if store.listCalls != 1 {
				t.Fatalf("store.List called %d times, want 1", store.listCalls)
			}
			if store.listSymbolOK != !tc.wantNilPass {
				t.Errorf("store.List received symbol %v, want nil=%v", store.listSymbol, tc.wantNilPass)
			}
		})
	}
}
