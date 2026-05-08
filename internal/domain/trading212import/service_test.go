package trading212import

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"github.com/govalues/decimal"
)

var ctx = context.Background()

// ---- Mocks ----

type mockSymbolResolver struct {
	brokerSymbols map[string]string // "brokerName|brokerSymbol" -> internalSymbol
	symbols       map[string]bool
}

func newMockSymbolResolver(symbols ...string) *mockSymbolResolver {
	m := &mockSymbolResolver{
		brokerSymbols: make(map[string]string),
		symbols:       make(map[string]bool),
	}
	for _, s := range symbols {
		m.symbols[s] = true
	}
	return m
}

func (m *mockSymbolResolver) addBrokerSymbol(brokerName, brokerSymbol, internalSymbol string) {
	m.brokerSymbols[brokerName+"|"+brokerSymbol] = internalSymbol
}

func (m *mockSymbolResolver) ResolveBrokerSymbol(_ context.Context, brokerName, brokerSymbol string) string {
	return m.brokerSymbols[brokerName+"|"+brokerSymbol]
}

func (m *mockSymbolResolver) SymbolExists(_ context.Context, symbol string) bool {
	return m.symbols[symbol]
}

type mockDuplicateChecker struct {
	references map[string]bool // "externalSystem|externalReference" -> bool
}

func newMockDuplicateChecker(refs ...string) *mockDuplicateChecker {
	m := &mockDuplicateChecker{references: make(map[string]bool)}
	for _, r := range refs {
		m.references["Trading212|"+r] = true
	}
	return m
}

func (m *mockDuplicateChecker) ExternalReferenceExists(_ context.Context, externalSystem, externalReference string) bool {
	return m.references[externalSystem+"|"+externalReference]
}

type mockTransactionCreator struct {
	created []*transaction.Transaction
	fail    bool
}

func newMockTransactionCreator() *mockTransactionCreator {
	return &mockTransactionCreator{}
}

func (m *mockTransactionCreator) WithFailure() *mockTransactionCreator {
	m.fail = true
	return m
}

func (m *mockTransactionCreator) BatchCreate(_ context.Context, txns []*transaction.Transaction) error {
	if m.fail {
		return &mockBatchError{}
	}
	m.created = append(m.created, txns...)
	return nil
}

func (m *mockTransactionCreator) CreatedCount() int {
	return len(m.created)
}

func (m *mockTransactionCreator) Created() []*transaction.Transaction {
	return m.created
}

type mockBatchError struct{}

func (e *mockBatchError) Error() string {
	return "batch create failed"
}

type mockAccountChecker struct {
	ids map[int64]bool
}

func newMockAccountChecker(ids ...int64) *mockAccountChecker {
	m := &mockAccountChecker{ids: make(map[int64]bool)}
	for _, id := range ids {
		m.ids[id] = true
	}
	return m
}

func (m *mockAccountChecker) AccountExists(_ context.Context, id int64) bool {
	return m.ids[id]
}

type mockSymbolCreator struct {
	created []string
}

func newMockSymbolCreator() *mockSymbolCreator {
	return &mockSymbolCreator{}
}

func (m *mockSymbolCreator) CreateSymbol(_ context.Context, internalSymbol, _ string) error {
	m.created = append(m.created, internalSymbol)
	return nil
}

type mockBrokerSymbolAdder struct {
	added []struct {
		brokerName   string
		brokerSymbol string
		internal     string
	}
}

func newMockBrokerSymbolAdder() *mockBrokerSymbolAdder {
	return &mockBrokerSymbolAdder{}
}

func (m *mockBrokerSymbolAdder) AddBrokerSymbolMapping(_ context.Context, brokerName, brokerSymbol, internal string) error {
	m.added = append(m.added, struct {
		brokerName   string
		brokerSymbol string
		internal     string
	}{brokerName, brokerSymbol, internal})
	return nil
}

type mockPositionRecalculator struct {
	recalculated []int64 // account IDs that were recalculated
	fail         bool
}

func newMockPositionRecalculator() *mockPositionRecalculator {
	return &mockPositionRecalculator{}
}

func (m *mockPositionRecalculator) WithFailure() *mockPositionRecalculator {
	m.fail = true
	return m
}

func (m *mockPositionRecalculator) RecalculateAccount(_ context.Context, accountID int64) error {
	m.recalculated = append(m.recalculated, accountID)
	if m.fail {
		return fmt.Errorf("recalculation failed")
	}
	return nil
}

// setupService creates a Service with configured mocks.
func setupService(t *testing.T, accountIDs []int64, symbols []string, dupRefs []string) (*Service, *mockSymbolResolver, *mockDuplicateChecker, *mockTransactionCreator, *mockAccountChecker, *mockSymbolCreator, *mockBrokerSymbolAdder, *mockPositionRecalculator) {
	t.Helper()
	resolver := newMockSymbolResolver(symbols...)
	dupCheck := newMockDuplicateChecker(dupRefs...)
	creator := newMockTransactionCreator()
	accounts := newMockAccountChecker(accountIDs...)
	symCreate := newMockSymbolCreator()
	brokerAdd := newMockBrokerSymbolAdder()
	recalc := newMockPositionRecalculator()
	svc := NewService(resolver, dupCheck, creator, accounts, symCreate, brokerAdd, WithPositionRecalculator(recalc))
	return svc, resolver, dupCheck, creator, accounts, symCreate, brokerAdd, recalc
}

// ==================== PREVIEW ====================

func TestService_Preview_SampleCSV(t *testing.T) {
	svc, resolver, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "MSFT", "QGRP", "DBMG", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")
	resolver.addBrokerSymbol("Trading212", "MSFT", "MSFT")
	resolver.addBrokerSymbol("Trading212", "QGRP", "QGRP")
	resolver.addBrokerSymbol("Trading212", "DBMG", "DBMG")

	data := loadSampleCSV(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	// Sample CSV: 15 rows
	// 6 trades (3 buys: AAPL, MSFT, QGRP; 3 sells: AAPL, MSFT, DBMG... wait let me recount)
	// Actually: Limit buy AAPL, Market buy MSFT, Limit sell AAPL, Market sell MSFT,
	//   Limit buy QGRP, Market buy DBMG, Limit sell QGRP = 7 trades
	// 2 deposits, 1 withdrawal, 5 interest = 8 cash
	// Total: 15 rows, all importable (no duplicates, all symbols resolved)
	if got.ImportableCount != 15 {
		t.Errorf("expected 15 importable, got %d", got.ImportableCount)
	}
	if got.SkippedCount != 0 {
		t.Errorf("expected 0 skipped, got %d", got.SkippedCount)
	}
	if got.ErroredCount != 0 {
		t.Errorf("expected 0 errored, got %d", got.ErroredCount)
	}

	// Verify first row (Deposit)
	if len(got.Importable) > 0 {
		first := got.Importable[0]
		if first.Type != "deposit" {
			t.Errorf("expected first type 'deposit', got %q", first.Type)
		}
		if first.Symbol != "$CASH-GBP" {
			t.Errorf("expected first symbol '$CASH-GBP', got %q", first.Symbol)
		}
		if first.Quantity != "5000.00" {
			t.Errorf("expected first quantity '5000.00', got %q", first.Quantity)
		}
	}

	// Verify second row (Limit buy AAPL)
	if len(got.Importable) > 1 {
		second := got.Importable[1]
		if second.Type != "buy" {
			t.Errorf("expected second type 'buy', got %q", second.Type)
		}
		if second.Symbol != "AAPL" {
			t.Errorf("expected second symbol 'AAPL', got %q", second.Symbol)
		}
		if second.Quantity != "10.0000000000" {
			t.Errorf("expected second quantity '10.0000000000', got %q", second.Quantity)
		}
		if second.Price != "150.00" {
			t.Errorf("expected second price '150.00', got %q", second.Price)
		}
		if second.Currency != "GBP" {
			t.Errorf("expected second currency 'GBP', got %q", second.Currency)
		}
	}
}

func TestService_Preview_AllDuplicates(t *testing.T) {
	// Mark all 15 IDs from sample CSV as duplicates
	allRefs := []string{
		"019a0001-0001-0001-0001-000000000001", // Deposit
		"EOF50000000001",                        // Limit buy AAPL
		"EOF50000000002",                        // Market buy MSFT
		"019a0002-0002-0002-0002-000000000002", // Interest
		"EOF50000000003",                        // Limit sell AAPL
		"EOF50000000004",                        // Market sell MSFT
		"019a0003-0003-0003-0003-000000000003", // Interest
		"019a0004-0004-0004-0004-000000000004", // Withdrawal
		"EOF50000000005",                        // Limit buy QGRP
		"019a0005-0005-0005-0005-000000000005", // Interest
		"EOF50000000006",                        // Market buy DBMG
		"019a0006-0006-0006-0006-000000000006", // Deposit
		"019a0007-0007-0007-0007-000000000007", // Interest
		"EOF50000000007",                        // Limit sell QGRP
		"019a0008-0008-0008-0008-000000000008", // Interest
	}
	svc, resolver, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "MSFT", "QGRP", "DBMG", "$CASH-GBP"}, allRefs)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")
	resolver.addBrokerSymbol("Trading212", "MSFT", "MSFT")
	resolver.addBrokerSymbol("Trading212", "QGRP", "QGRP")
	resolver.addBrokerSymbol("Trading212", "DBMG", "DBMG")

	data := loadSampleCSV(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if got.ImportableCount != 0 {
		t.Errorf("expected 0 importable, got %d", got.ImportableCount)
	}
	if got.SkippedCount != 15 {
		t.Errorf("expected 15 skipped, got %d", got.SkippedCount)
	}

	for _, s := range got.Skipped {
		if !strings.Contains(s.Reason, "duplicate") {
			t.Errorf("expected duplicate reason, got %q for %s", s.Reason, s.ExternalReference)
		}
	}
}

func TestService_Preview_UnmappedSymbols(t *testing.T) {
	// Only resolve AAPL, not MSFT/QGRP/DBMG
	svc, resolver, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	data := loadSampleCSV(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	skippedUnmapped := 0
	for _, s := range got.Skipped {
		if strings.Contains(s.Reason, "unmapped") {
			skippedUnmapped++
			if s.BrokerSymbol == "" {
				t.Errorf("skipped transaction with reason %q has empty BrokerSymbol", s.Reason)
			}
		}
	}
	if skippedUnmapped == 0 {
		t.Error("expected some skipped transactions with unmapped symbol reason")
	}
}

func TestService_Preview_NetCashCalculation(t *testing.T) {
	svc, resolver, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	data := loadSampleCSV(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	// Check net cash for each importable row
	for _, entry := range got.Importable {
		switch entry.Type {
		case "buy":
			// Buys should have negative net cash
			if !strings.HasPrefix(entry.NetCash, "-") {
				t.Errorf("buy %s: expected negative net cash, got %q", entry.Symbol, entry.NetCash)
			}
		case "sell":
			// Sells should have positive net cash
			if strings.HasPrefix(entry.NetCash, "-") {
				t.Errorf("sell %s: expected positive net cash, got %q", entry.Symbol, entry.NetCash)
			}
		case "deposit":
			// Deposits should have positive net cash
			if strings.HasPrefix(entry.NetCash, "-") {
				t.Errorf("deposit: expected positive net cash, got %q", entry.NetCash)
			}
		case "withdrawal":
			// Withdrawals should have negative net cash (money leaving the account)
			if !strings.HasPrefix(entry.NetCash, "-") {
				t.Errorf("withdrawal: expected negative net cash, got %q", entry.NetCash)
			}
		case "interest":
			// Interest should have positive net cash
			if strings.HasPrefix(entry.NetCash, "-") {
				t.Errorf("interest: expected positive net cash, got %q", entry.NetCash)
			}
		}
	}
}

func TestService_Preview_GBXPathConversion(t *testing.T) {
	// Parser already converts GBX → GBP; service receives clean data
	svc, resolver, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	data := loadSampleCSV(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	// Find the AAPL buy (second row, index 1)
	for _, entry := range got.Importable {
		if entry.Symbol == "AAPL" && entry.Type == "buy" {
			if entry.Price != "150.00" {
				t.Errorf("expected GBP price '150.00' (converted from 15000 GBX), got %q", entry.Price)
			}
			if entry.Currency != "GBP" {
				t.Errorf("expected currency 'GBP', got %q", entry.Currency)
			}
			return
		}
	}
	t.Error("AAPL buy entry not found in importable")
}

func TestService_Preview_NonExistentAccount(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.Preview(ctx, []byte("dummy"), 999)
	if err == nil {
		t.Fatal("expected error for non-existent account, got nil")
	}
	if err.Error() != "account not found" {
		t.Errorf("expected 'account not found', got %q", err.Error())
	}
}

func TestService_Preview_InvalidCSV(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.Preview(ctx, []byte("not csv"), 1)
	if err == nil {
		t.Fatal("expected error for invalid CSV, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CSV data") {
		t.Errorf("expected 'invalid CSV data', got %q", err.Error())
	}
}

func TestService_Preview_EmptyCSV(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.Preview(ctx, []byte(""), 1)
	if err == nil {
		t.Fatal("expected error for empty CSV, got nil")
	}
}

func TestService_Preview_HeaderOnly(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	headerOnly := []byte("Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)")
	_, err := svc.Preview(ctx, headerOnly, 1)
	if err == nil {
		t.Fatal("expected error for header-only CSV, got nil")
	}
}

func TestService_Preview_EmptySections(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	// Valid CSV header with no data rows — parser returns error
	_, err := svc.Preview(ctx, []byte("Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)\n"), 1)
	if err == nil {
		t.Fatal("expected error for CSV with no data rows, got nil")
	}
}

func TestService_Preview_UnsupportedActionTypes(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	// CSV with unsupported action types (Dividend, Fee, Tax)
	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Dividend,2026-01-10 09:00:00,US5949181045,AAPL,"Apple Inc.",,DIV001,,,"1.00","GBP",,25.00,"GBP"
Fee,2026-01-11 10:00:00,,,,"Account Fee",FEE001,,,,,5.00,"GBP"
Tax,2026-01-12 11:00:00,,,,"Tax Adjustment",TAX001,,,,,3.50,"GBP"
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"`)

	got, err := svc.Preview(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	// 3 unsupported actions should be skipped, 1 valid buy should be skipped (unmapped symbol)
	if got.ImportableCount != 0 {
		t.Errorf("expected 0 importable, got %d", got.ImportableCount)
	}
	if got.SkippedCount != 4 {
		t.Errorf("expected 4 skipped, got %d", got.SkippedCount)
	}

	unsupportedCount := 0
	for _, s := range got.Skipped {
		if s.Reason == "unsupported action type" {
			unsupportedCount++
		}
	}
	if unsupportedCount != 3 {
		t.Errorf("expected 3 skipped with 'unsupported action type', got %d", unsupportedCount)
	}

	// Verify the unsupported rows have their IDs and display fields
	for _, s := range got.Skipped {
		if s.Reason == "unsupported action type" {
			if s.ExternalReference == "" {
				t.Error("unsupported action skip should have ExternalReference")
			}
			if s.Type != "unknown" {
				t.Errorf("unsupported action skip should have type 'unknown', got %q", s.Type)
			}
		}
	}
}

// ==================== CONFIRM IMPORT ====================

func TestService_ConfirmImport_Basic(t *testing.T) {
	svc, resolver, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	// Minimal CSV: 1 buy + 1 deposit
	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"
Deposit,2026-01-05 09:00:00,,,,"Bank Transfer",019a0001-0001-0001-0001-000000000001,,,,,5000.00,"GBP"`)

	got, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	if got.CreatedCount != 2 {
		t.Errorf("expected 2 created, got %d", got.CreatedCount)
	}
	if creator.CreatedCount() != 2 {
		t.Errorf("expected creator to have 2 transactions, got %d", creator.CreatedCount())
	}

	for _, txn := range creator.Created() {
		if txn.ExternalSystem == nil || *txn.ExternalSystem != "Trading212" {
			t.Errorf("expected ExternalSystem 'Trading212', got %v", txn.ExternalSystem)
		}
		if txn.ExternalReference == nil {
			t.Error("expected non-nil ExternalReference")
		}
	}
}

func TestService_ConfirmImport_RollbackOnFailure(t *testing.T) {
	svc, resolver, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")
	creator.WithFailure()

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"
Limit sell,2026-01-20 11:30:00,US5949181045,AAPL,"Apple Inc.",,EOF50000000002,5.0000000000,15500.0000000000,GBX,100.00000000,775.00,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err == nil {
		t.Fatal("expected error from batch create, got nil")
	}
}

func TestService_ConfirmImport_DetectsNewDuplicates(t *testing.T) {
	resolver := newMockSymbolResolver("AAPL", "$CASH-GBP")
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")
	dupCheck := newMockDuplicateChecker("EOF50000000001")
	creator := newMockTransactionCreator()
	accounts := newMockAccountChecker(1)
	symCreate := newMockSymbolCreator()
	brokerAdd := newMockBrokerSymbolAdder()
	svc := NewService(resolver, dupCheck, creator, accounts, symCreate, brokerAdd)

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"
Limit sell,2026-01-20 11:30:00,US5949181045,AAPL,"Apple Inc.",,EOF50000000002,5.0000000000,15500.0000000000,GBX,100.00000000,775.00,"GBP"`)

	got, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	if got.CreatedCount != 1 {
		t.Errorf("expected 1 created (first is duplicate), got %d", got.CreatedCount)
	}
	if got.SkippedCount != 1 {
		t.Errorf("expected 1 skipped, got %d", got.SkippedCount)
	}
}

func TestService_ConfirmImport_NonExistentAccount(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.ConfirmImport(ctx, []byte("dummy"), 999)
	if err == nil {
		t.Fatal("expected error for non-existent account, got nil")
	}
}

func TestService_ConfirmImport_InvalidCSV(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.ConfirmImport(ctx, []byte("not csv"), 1)
	if err == nil {
		t.Fatal("expected error for invalid CSV, got nil")
	}
}

func TestService_ConfirmImport_SampleCSV(t *testing.T) {
	svc, resolver, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "MSFT", "QGRP", "DBMG", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")
	resolver.addBrokerSymbol("Trading212", "MSFT", "MSFT")
	resolver.addBrokerSymbol("Trading212", "QGRP", "QGRP")
	resolver.addBrokerSymbol("Trading212", "DBMG", "DBMG")

	data := loadSampleCSV(t)
	got, err := svc.ConfirmImport(ctx, data, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	// All 15 rows should be created
	if got.CreatedCount != 15 {
		t.Errorf("expected 15 created, got %d", got.CreatedCount)
	}
	if creator.CreatedCount() != 15 {
		t.Errorf("expected 15 in creator, got %d", creator.CreatedCount())
	}

	// Verify external fields on all transactions
	for i, txn := range creator.Created() {
		if txn.ExternalSystem == nil || *txn.ExternalSystem != "Trading212" {
			t.Errorf("txn %d: expected ExternalSystem 'Trading212', got %v", i, txn.ExternalSystem)
		}
		if txn.ExternalReference == nil {
			t.Errorf("txn %d: expected non-nil ExternalReference", i)
		}
		if txn.CreatedAt.IsZero() {
			t.Errorf("txn %d: expected non-zero CreatedAt", i)
		}
		if txn.UpdatedAt.IsZero() {
			t.Errorf("txn %d: expected non-zero UpdatedAt", i)
		}
	}
}

func TestService_ConfirmImport_TradeFieldValues(t *testing.T) {
	svc, resolver, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	txn := txns[0]

	if txn.AccountID != 1 {
		t.Errorf("expected AccountID 1, got %d", txn.AccountID)
	}
	if txn.Type != "buy" {
		t.Errorf("expected Type 'buy', got %q", txn.Type)
	}
	if txn.Symbol != "AAPL" {
		t.Errorf("expected Symbol 'AAPL', got %q", txn.Symbol)
	}
	// Quantity from CSV: "10.0000000000"
	qty, _ := decimal.Parse("10.0000000000")
	if !txn.Quantity.Equal(qty) {
		t.Errorf("expected Quantity %s, got %s", qty.String(), txn.Quantity.String())
	}
	// Price converted from GBX: 15000/100 = 150
	if !txn.Price.Equal(decimal.MustNew(150, 0)) {
		t.Errorf("expected Price 150, got %s", txn.Price.String())
	}
	// NetCash = -Total (buy) = -1500.00
	if !txn.NetCash.Equal(decimal.MustNew(-150000, 2)) {
		t.Errorf("expected NetCash -1500.00, got %s", txn.NetCash.String())
	}
	if txn.Currency != "GBP" {
		t.Errorf("expected Currency 'GBP', got %q", txn.Currency)
	}
	wantDate := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	if !txn.Date.Equal(wantDate) {
		t.Errorf("expected Date 2026-01-06, got %s", txn.Date.Format(time.RFC3339))
	}
	if txn.ExternalSystem == nil || *txn.ExternalSystem != "Trading212" {
		t.Errorf("expected ExternalSystem 'Trading212', got %v", txn.ExternalSystem)
	}
	if txn.ExternalReference == nil || *txn.ExternalReference != "EOF50000000001" {
		t.Errorf("expected ExternalReference 'EOF50000000001', got %v", txn.ExternalReference)
	}
}

func TestService_ConfirmImport_SellFieldValues(t *testing.T) {
	svc, resolver, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit sell,2026-01-20 11:30:00,US5949181045,AAPL,"Apple Inc.",,EOF50000000003,5.0000000000,15500.0000000000,GBX,100.00000000,775.00,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	txn := txns[0]

	if txn.Type != "sell" {
		t.Errorf("expected Type 'sell', got %q", txn.Type)
	}
	// NetCash = +Total (sell) = +775.00
	if !txn.NetCash.Equal(decimal.MustNew(77500, 2)) {
		t.Errorf("expected NetCash 775.00, got %s", txn.NetCash.String())
	}
}

func TestService_ConfirmImport_DepositFieldValues(t *testing.T) {
	svc, _, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"$CASH-GBP"}, nil)

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Deposit,2026-01-05 09:00:00,,,,"Bank Transfer",019a0001-0001-0001-0001-000000000001,,,,,5000.00,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	txn := txns[0]

	if txn.Type != "deposit" {
		t.Errorf("expected Type 'deposit', got %q", txn.Type)
	}
	if txn.Symbol != "$CASH-GBP" {
		t.Errorf("expected Symbol '$CASH-GBP', got %q", txn.Symbol)
	}
	if !txn.Quantity.Equal(decimal.MustNew(500000, 2)) {
		t.Errorf("expected Quantity 5000.00, got %s", txn.Quantity.String())
	}
	if !txn.Price.Equal(decimal.One) {
		t.Errorf("expected Price 1, got %s", txn.Price.String())
	}
	if !txn.NetCash.Equal(decimal.MustNew(500000, 2)) {
		t.Errorf("expected NetCash 5000.00, got %s", txn.NetCash.String())
	}
}

func TestService_ConfirmImport_WithdrawalFieldValues(t *testing.T) {
	svc, _, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"$CASH-GBP"}, nil)

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Withdrawal,2026-02-10 16:00:00,,,,"Bank Transfer",019a0004-0004-0004-0004-000000000004,,,,,2000.00,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	txn := txns[0]

	if txn.Type != "withdrawal" {
		t.Errorf("expected Type 'withdrawal', got %q", txn.Type)
	}
	// Withdrawal: NetCash = -Total
	if !txn.NetCash.Equal(decimal.MustNew(-200000, 2)) {
		t.Errorf("expected NetCash -2000.00, got %s", txn.NetCash.String())
	}
}

func TestService_ConfirmImport_InterestFieldValues(t *testing.T) {
	svc, _, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"$CASH-GBP"}, nil)

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Interest on cash,2026-01-08 02:00:00,,,,"Interest on cash",019a0002-0002-0002-0002-000000000002,,,,,0.68,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	txn := txns[0]

	if txn.Type != "interest" {
		t.Errorf("expected Type 'interest', got %q", txn.Type)
	}
	if !txn.Quantity.Equal(decimal.MustNew(68, 2)) {
		t.Errorf("expected Quantity 0.68, got %s", txn.Quantity.String())
	}
	if !txn.NetCash.Equal(decimal.MustNew(68, 2)) {
		t.Errorf("expected NetCash 0.68, got %s", txn.NetCash.String())
	}
}

func TestService_ConfirmImport_EmptyReport(t *testing.T) {
	svc, _, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	// Header only — parser returns error
	_, err := svc.ConfirmImport(ctx, []byte("Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)\n"), 1)
	if err == nil {
		t.Fatal("expected error for header-only CSV, got nil")
	}
}

func TestService_ConfirmImport_MixedRecords(t *testing.T) {
	svc, resolver, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"
Deposit,2026-01-05 09:00:00,,,,"Bank Transfer",019a0001-0001-0001-0001-000000000001,,,,,5000.00,"GBP"
Limit sell,2026-01-20 11:30:00,US5949181045,AAPL,"Apple Inc.",,EOF50000000003,5.0000000000,15500.0000000000,GBX,100.00000000,775.00,"GBP"
Interest on cash,2026-01-08 02:00:00,,,,"Interest on cash",019a0002-0002-0002-0002-000000000002,,,,,0.68,"GBP"`)

	got, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	// 4 rows, all importable
	if got.CreatedCount != 4 {
		t.Errorf("expected 4 created, got %d", got.CreatedCount)
	}
	if creator.CreatedCount() != 4 {
		t.Errorf("expected 4 in creator, got %d", creator.CreatedCount())
	}
}

func TestService_ConfirmImport_LotIDAutoGenerated(t *testing.T) {
	svc, resolver, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"
Limit sell,2026-01-20 11:30:00,US5949181045,AAPL,"Apple Inc.",,EOF50000000003,5.0000000000,15500.0000000000,GBX,100.00000000,775.00,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}

	// Both trades should have auto-generated lot_ids starting with "LOT-"
	for i, txn := range txns {
		if txn.LotID == nil {
			t.Errorf("txn %d (%s): expected non-nil LotID", i, txn.Type)
		} else if !strings.HasPrefix(*txn.LotID, "LOT-") {
			t.Errorf("txn %d (%s): expected LotID starting with 'LOT-', got %q", i, txn.Type, *txn.LotID)
		}
	}

	// Each trade gets a unique lot_id
	if txns[0].LotID != nil && txns[1].LotID != nil && *txns[0].LotID == *txns[1].LotID {
		t.Error("expected different lot_ids for different trades")
	}
}

func TestService_ConfirmImport_PositionRecalculation(t *testing.T) {
	svc, resolver, _, _, _, _, _, recalc := setupService(t, []int64{1}, []string{"AAPL", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("Trading212", "AAPL", "AAPL")

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Limit buy,2026-01-06 10:15:30,US5949181045,AAPL,"Apple Inc.",,EOF50000000001,10.0000000000,15000.0000000000,GBX,100.00000000,1500.00,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	if len(recalc.recalculated) != 1 {
		t.Errorf("expected 1 recalculation, got %d", len(recalc.recalculated))
	}
	if len(recalc.recalculated) > 0 && recalc.recalculated[0] != 1 {
		t.Errorf("expected recalc for account 1, got %d", recalc.recalculated[0])
	}
}

func TestService_ConfirmImport_NoLotIDForNonTrades(t *testing.T) {
	svc, _, _, creator, _, _, _, _ := setupService(t, []int64{1}, []string{"$CASH-GBP"}, nil)

	csvData := []byte(`Action,Time,ISIN,Ticker,Name,Notes,ID,No. of shares,Price / share,Currency (Price / share),Exchange rate,Total,Currency (Total)
Deposit,2026-01-05 09:00:00,,,,"Bank Transfer",019a0001-0001-0001-0001-000000000001,,,,,5000.00,"GBP"
Interest on cash,2026-01-08 02:00:00,,,,"Interest on cash",019a0002-0002-0002-0002-000000000002,,,,,0.68,"GBP"`)

	_, err := svc.ConfirmImport(ctx, csvData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}

	// Deposit and interest should NOT have lot_id
	for i, txn := range txns {
		if txn.LotID != nil {
			t.Errorf("txn %d (%s): expected nil LotID, got %q", i, txn.Type, *txn.LotID)
		}
	}
}

// ==================== CREATE SYMBOL / ADD BROKER SYMBOL ====================

func TestService_CreateSymbol(t *testing.T) {
	svc, _, _, _, _, symCreate, _, _ := setupService(t, []int64{1}, []string{}, nil)

	err := svc.CreateSymbol(ctx, "NEWSYM", "NEWSYM")
	if err != nil {
		t.Fatalf("CreateSymbol: %v", err)
	}
	if len(symCreate.created) != 1 {
		t.Errorf("expected 1 symbol created, got %d", len(symCreate.created))
	}
	if symCreate.created[0] != "NEWSYM" {
		t.Errorf("expected 'NEWSYM', got %q", symCreate.created[0])
	}
}

func TestService_AddBrokerSymbolMapping(t *testing.T) {
	svc, _, _, _, _, _, brokerAdd, _ := setupService(t, []int64{1}, []string{}, nil)

	err := svc.AddBrokerSymbolMapping(ctx, "Trading212", "AAPL", "AAPL")
	if err != nil {
		t.Fatalf("AddBrokerSymbolMapping: %v", err)
	}
	if len(brokerAdd.added) != 1 {
		t.Errorf("expected 1 mapping added, got %d", len(brokerAdd.added))
	}
	if brokerAdd.added[0].brokerName != "Trading212" {
		t.Errorf("expected brokerName 'Trading212', got %q", brokerAdd.added[0].brokerName)
	}
	if brokerAdd.added[0].brokerSymbol != "AAPL" {
		t.Errorf("expected brokerSymbol 'AAPL', got %q", brokerAdd.added[0].brokerSymbol)
	}
	if brokerAdd.added[0].internal != "AAPL" {
		t.Errorf("expected internal 'AAPL', got %q", brokerAdd.added[0].internal)
	}
}

// ==================== HELPER FUNCTIONS ====================

func TestIsTradeAction(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		expected bool
	}{
		{"limit buy", ActionLimitBuy, true},
		{"market buy", ActionMarketBuy, true},
		{"limit sell", ActionLimitSell, true},
		{"market sell", ActionMarketSell, true},
		{"deposit", ActionDeposit, false},
		{"withdrawal", ActionWithdrawal, false},
		{"interest", ActionInterestOnCash, false},
		{"unknown", ActionUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTradeAction(tt.action)
			if got != tt.expected {
				t.Errorf("isTradeAction(%q) = %v, want %v", tt.action, got, tt.expected)
			}
		})
	}
}

func TestActionToTxnType(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		expected string
	}{
		{"limit buy", ActionLimitBuy, "buy"},
		{"market buy", ActionMarketBuy, "buy"},
		{"limit sell", ActionLimitSell, "sell"},
		{"market sell", ActionMarketSell, "sell"},
		{"deposit", ActionDeposit, "deposit"},
		{"withdrawal", ActionWithdrawal, "withdrawal"},
		{"interest", ActionInterestOnCash, "interest"},
		{"unknown", ActionUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := actionToTxnType(tt.action)
			if got != tt.expected {
				t.Errorf("actionToTxnType(%q) = %q, want %q", tt.action, got, tt.expected)
			}
		})
	}
}

func TestComputeNetCash(t *testing.T) {
	total := decimal.MustNew(150000, 2) // 1500.00

	tests := []struct {
		name     string
		action   string
		expected decimal.Decimal
	}{
		{"limit buy", ActionLimitBuy, total.Neg()},
		{"market buy", ActionMarketBuy, total.Neg()},
		{"limit sell", ActionLimitSell, total},
		{"market sell", ActionMarketSell, total},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeNetCash(total, tt.action)
			if !got.Equal(tt.expected) {
				t.Errorf("computeNetCash(%s, %q) = %s, want %s", total.String(), tt.action, got.String(), tt.expected.String())
			}
		})
	}
}

func TestCashSymbol(t *testing.T) {
	tests := []struct {
		currency string
		expected string
	}{
		{"USD", "$CASH-USD"},
		{"GBP", "$CASH-GBP"},
		{"EUR", "$CASH-EUR"},
	}

	for _, tt := range tests {
		got := cashSymbol(tt.currency)
		if got != tt.expected {
			t.Errorf("cashSymbol(%q) = %q, want %q", tt.currency, got, tt.expected)
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input   string
		wantDay int
	}{
		{"2026-01-06", 6},
		{"2026-12-31", 31},
		{"2025-01-01", 1},
		{"", 1},        // zero time = Jan 1 year 1
		{"invalid", 1}, // zero time = Jan 1 year 1
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseDate(tt.input)
			if tt.input != "" && tt.input != "invalid" && got.Day() != tt.wantDay {
				t.Errorf("parseDate(%q) day = %d, want %d", tt.input, got.Day(), tt.wantDay)
			}
		})
	}
}
