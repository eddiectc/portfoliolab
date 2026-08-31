package trading212import

import (
	"os"
	"path/filepath"
	"testing"
)

// loadSampleCSV reads the Trading 212 sample CSV fixture.
func loadSampleCSV(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("testdata", "trading212_sample.csv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sample CSV: %v", err)
	}
	return data
}

func TestParseCSV_ValidSample_Count(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	if len(report.Rows) != 15 {
		t.Errorf("expected 15 rows, got %d", len(report.Rows))
	}
}

func TestParseCSV_Deposit(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[0]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionDeposit},
		{"Date", row.Date, "2026-01-05"},
		{"Ticker", row.Ticker, ""},
		{"ID", row.ID, "019a0001-0001-0001-0001-000000000001"},
		{"Quantity", row.Quantity, ""},
		{"Price", row.Price, ""},
		{"Currency", row.Currency, ""},
		{"Total", row.Total, "5000.00"},
		{"TotalCurrency", row.TotalCurrency, "GBP"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_LimitBuy_AAPL(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[1]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionLimitBuy},
		{"Date", row.Date, "2026-01-06"},
		{"Ticker", row.Ticker, "AAPL"},
		{"Name", row.Name, "Apple Inc."},
		{"ID", row.ID, "EOF50000000001"},
		{"Quantity", row.Quantity, "10.0000000000"},
		{"Price", row.Price, "150.00"},    // converted from 15000 GBX
		{"Currency", row.Currency, "GBP"}, // converted from GBX
		{"Total", row.Total, "1500.00"},
		{"TotalCurrency", row.TotalCurrency, "GBP"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_MarketBuy_MSFT(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[2]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionMarketBuy},
		{"Date", row.Date, "2026-01-07"},
		{"Ticker", row.Ticker, "MSFT"},
		{"Name", row.Name, "Microsoft Corporation"},
		{"ID", row.ID, "EOF50000000002"},
		{"Quantity", row.Quantity, "5.0000000000"},
		{"Price", row.Price, "380.00"},    // converted from 38000 GBX
		{"Currency", row.Currency, "GBP"}, // converted from GBX
		{"Total", row.Total, "1900.00"},
		{"TotalCurrency", row.TotalCurrency, "GBP"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_InterestOnCash(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[3]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionInterestOnCash},
		{"Date", row.Date, "2026-01-08"},
		{"Ticker", row.Ticker, ""},
		{"ID", row.ID, "019a0002-0002-0002-0002-000000000002"},
		{"Total", row.Total, "0.68"},
		{"TotalCurrency", row.TotalCurrency, "GBP"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_LimitSell_AAPL(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[4]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionLimitSell},
		{"Date", row.Date, "2026-01-20"},
		{"Ticker", row.Ticker, "AAPL"},
		{"ID", row.ID, "EOF50000000003"},
		{"Quantity", row.Quantity, "5.0000000000"},
		{"Price", row.Price, "155.00"}, // converted from 15500 GBX
		{"Currency", row.Currency, "GBP"},
		{"Total", row.Total, "775.00"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_MarketSell_MSFT(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[5]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionMarketSell},
		{"Date", row.Date, "2026-02-01"},
		{"Ticker", row.Ticker, "MSFT"},
		{"ID", row.ID, "EOF50000000004"},
		{"Quantity", row.Quantity, "2.0000000000"},
		{"Price", row.Price, "390.00"}, // converted from 39000 GBX
		{"Currency", row.Currency, "GBP"},
		{"Total", row.Total, "780.00"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_Withdrawal(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[7]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionWithdrawal},
		{"Date", row.Date, "2026-02-10"},
		{"Ticker", row.Ticker, ""},
		{"ID", row.ID, "019a0004-0004-0004-0004-000000000004"},
		{"Total", row.Total, "2000.00"},
		{"TotalCurrency", row.TotalCurrency, "GBP"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_ETF_QGRP(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[8]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionLimitBuy},
		{"Date", row.Date, "2026-02-15"},
		{"Ticker", row.Ticker, "QGRP"},
		{"ID", row.ID, "EOF50000000005"},
		{"Quantity", row.Quantity, "100.0000000000"},
		{"Price", row.Price, "26.00"}, // converted from 2600 GBX
		{"Currency", row.Currency, "GBP"},
		{"Total", row.Total, "260.00"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_ETF_DBMG(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[10]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Action", row.Action, ActionMarketBuy},
		{"Date", row.Date, "2026-03-10"},
		{"Ticker", row.Ticker, "DBMG"},
		{"ID", row.ID, "EOF50000000006"},
		{"Quantity", row.Quantity, "50.0000000000"},
		{"Price", row.Price, "97.00"}, // converted from 9700 GBX
		{"Currency", row.Currency, "GBP"},
		{"Total", row.Total, "485.00"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseCSV_EmptyData(t *testing.T) {
	_, err := ParseCSV([]byte{})
	if err == nil {
		t.Error("expected error for empty data, got nil")
	}
}

func TestParseCSV_NoHeader(t *testing.T) {
	// Data with no header row — just raw values
	data := []byte("Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,\"Apple Inc.\",,EOF50000000001,10,15000,GBX,100,1500.00,GBP\n")
	_, err := ParseCSV(data)
	if err == nil {
		t.Error("expected error for missing header, got nil")
	}
}

func TestParseCSV_HeaderOnly(t *testing.T) {
	data := []byte("Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)\n")
	_, err := ParseCSV(data)
	if err == nil {
		t.Error("expected error for header-only CSV, got nil")
	}
}

func TestParseCSV_MissingColumns(t *testing.T) {
	data := []byte("Action,Time,Ticker\nLimit buy,2026-01-06,AAPL\n")
	_, err := ParseCSV(data)
	if err == nil {
		t.Error("expected error for missing columns, got nil")
	}
}

func TestParseCSV_UnknownAction(t *testing.T) {
	data := []byte("Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)\n" +
		"Some weird action,2026-01-06 10:15:30,US5949181045,AAPL,\"Apple Inc.\",,EOF50000000001,10,15000,GBX,100,1500.00,GBP\n")

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	if len(report.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(report.Rows))
	}

	row := report.Rows[0]
	if row.Action != ActionUnknown {
		t.Errorf("expected ActionUnknown, got %q", row.Action)
	}
}

func TestParseCSV_GBXConversion_Precision(t *testing.T) {
	// Test that GBX conversion preserves precision correctly
	data := []byte("Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)\n" +
		"Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,\"Apple Inc.\",,EOF50000000001,10,15000.5000000000,GBX,100,1500.05,GBP\n")

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[0]
	// 15000.50 GBX / 100 = 150.005 → rounded to 150.00 (banker's rounding, half-to-even)
	if row.Price != "150.00" {
		t.Errorf("expected Price 150.00, got %q", row.Price)
	}
	if row.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", row.Currency)
	}
}

func TestParseCSV_NonGBXCurrency_Unchanged(t *testing.T) {
	// If currency is not GBX, price should be unchanged
	data := []byte("Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)\n" +
		"Deposit,2026-01-05 09:00:00,,,,\"Bank Transfer\",019a0001-0001-0001-0001-000000000001,,,,,5000.00,GBP\n")

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	row := report.Rows[0]
	if row.Price != "" {
		t.Errorf("expected empty Price for deposit, got %q", row.Price)
	}
}

func TestParseCSV_AllActions_Classified(t *testing.T) {
	data := loadSampleCSV(t)

	report, err := ParseCSV(data)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	// Count actions
	expected := map[string]int{
		ActionDeposit:        2,
		ActionLimitBuy:       2,
		ActionMarketBuy:      2,
		ActionInterestOnCash: 5,
		ActionLimitSell:      2,
		ActionMarketSell:     1,
		ActionWithdrawal:     1,
	}

	got := make(map[string]int)
	for _, row := range report.Rows {
		got[row.Action]++
	}

	for action, want := range expected {
		if got[action] != want {
			t.Errorf("action %q: expected %d, got %d", action, want, got[action])
		}
	}
}
