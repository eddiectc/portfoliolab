package ibkrimport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
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

func (m *mockSymbolResolver) ResolveBrokerSymbol(brokerName, brokerSymbol string) string {
	return m.brokerSymbols[brokerName+"|"+brokerSymbol]
}

func (m *mockSymbolResolver) SymbolExists(symbol string) bool {
	return m.symbols[symbol]
}

type mockDuplicateChecker struct {
	references map[string]bool // "externalSystem|externalReference" -> bool
}

func newMockDuplicateChecker(refs ...string) *mockDuplicateChecker {
	m := &mockDuplicateChecker{references: make(map[string]bool)}
	for _, r := range refs {
		m.references["IBKR|"+r] = true
	}
	return m
}

func (m *mockDuplicateChecker) ExternalReferenceExists(_ context.Context, externalSystem, externalReference string) bool {
	return m.references[externalSystem+"|"+externalReference]
}

type mockTransactionCreator struct {
	created   []*transaction.Transaction
	fail      bool
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

// setupService creates a Service with configured mocks.
func setupService(t *testing.T, accountIDs []int64, symbols []string, dupRefs []string) (*Service, *mockSymbolResolver, *mockDuplicateChecker, *mockTransactionCreator, *mockAccountChecker, *mockSymbolCreator, *mockBrokerSymbolAdder) {
	t.Helper()
	resolver := newMockSymbolResolver(symbols...)
	dupCheck := newMockDuplicateChecker(dupRefs...)
	creator := newMockTransactionCreator()
	accounts := newMockAccountChecker(accountIDs...)
	symCreate := newMockSymbolCreator()
	brokerAdd := newMockBrokerSymbolAdder()
	svc := NewService(resolver, dupCheck, creator, accounts, symCreate, brokerAdd)
	return svc, resolver, dupCheck, creator, accounts, symCreate, brokerAdd
}

// ==================== PREVIEW ====================

func TestService_Preview_SampleXML(t *testing.T) {
	svc, resolver, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "STHY", "$CASH-USD", "$CASH-GBP", "$CASH-XYZ"}, nil)
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")
	resolver.addBrokerSymbol("IBKR", "STHY", "STHY")

	data := loadSampleXML(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	// 5 STK trades + 2 FX entries = 7 importable trades
	// 7 cash transactions = 7 importable
	// 2 transfers = 2 importable
	// Total: 16 importable, 0 skipped, 0 errored
	if got.ImportableCount != 16 {
		t.Errorf("expected 16 importable, got %d", got.ImportableCount)
	}
	if got.SkippedCount != 0 {
		t.Errorf("expected 0 skipped, got %d", got.SkippedCount)
	}
	if got.ErroredCount != 0 {
		t.Errorf("expected 0 errored, got %d", got.ErroredCount)
	}

	// Verify first trade (AAPL buy)
	if len(got.Importable) > 0 {
		first := got.Importable[0]
		if first.Type != "buy" {
			t.Errorf("expected first trade type 'buy', got %q", first.Type)
		}
		if first.Symbol != "AAPL" {
			t.Errorf("expected first trade symbol 'AAPL', got %q", first.Symbol)
		}
		if first.Quantity != "100" {
			t.Errorf("expected first trade quantity '100', got %q", first.Quantity)
		}
		if first.Currency != "USD" {
			t.Errorf("expected first trade currency 'USD', got %q", first.Currency)
		}
	}
}

func TestService_Preview_AllDuplicates(t *testing.T) {
	svc, resolver, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "STHY", "$CASH-USD", "$CASH-GBP", "$CASH-XYZ"},
		[]string{"30000000001", "30000000002", "30000000003", "30000000004", "30000000005",
			"30000000006", "30000000010", "30000000011", "30000000012", "30000000013",
			"30000000014", "30000000015", "30000000016", "30000000020", "30000000021"})
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")
	resolver.addBrokerSymbol("IBKR", "STHY", "STHY")

	data := loadSampleXML(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if got.ImportableCount != 0 {
		t.Errorf("expected 0 importable, got %d", got.ImportableCount)
	}
	// All 15 records skipped (FX trade is 1 skip, not 2 — duplicate check before entry generation)
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
	// Only resolve AAPL, not STHY
	svc, resolver, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "$CASH-USD", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")

	data := loadSampleXML(t)
	got, err := svc.Preview(ctx, data, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	skippedUnmapped := 0
	for _, s := range got.Skipped {
		if strings.Contains(s.Reason, "unmapped") {
			skippedUnmapped++
		}
	}
	if skippedUnmapped == 0 {
		t.Error("expected some skipped transactions with unmapped symbol reason")
	}
}

func TestService_Preview_FXTrades(t *testing.T) {
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{"$CASH-GBP", "$CASH-USD"}, nil)

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="GBP" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades>
						<Trade accountId="U111" acctAlias="TEST" currency="GBP" fxRateToBase="1" assetCategory="CASH" subCategory="" symbol="GBP.USD" description="GBP.USD" conid="" isin="" tradeID="" multiplier="1" reportDate="2025-06-01" dateTime="2025-06-01;100000" tradeDate="2025-06-01" settleDateTarget="2025-06-03" transactionType="ExchTrade" exchange="IDEALFX" quantity="10000" tradePrice="1.27" tradeMoney="12700" proceeds="-12700" taxes="0" ibCommission="-1.27" ibCommissionCurrency="GBP" netCash="0" closePrice="0" buySell="BUY" ibOrderID="5000000005" transactionID="30000000006" ibExecID="" />
					</Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.Preview(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if got.ImportableCount != 2 {
		t.Fatalf("expected 2 importable (FX generates 2), got %d", got.ImportableCount)
	}

	withdrawal := got.Importable[0]
	if withdrawal.Type != "withdrawal" {
		t.Errorf("expected withdrawal type, got %q", withdrawal.Type)
	}
	if withdrawal.Symbol != "$CASH-GBP" {
		t.Errorf("expected symbol $CASH-GBP, got %q", withdrawal.Symbol)
	}
	if withdrawal.Currency != "GBP" {
		t.Errorf("expected currency GBP, got %q", withdrawal.Currency)
	}

	deposit := got.Importable[1]
	if deposit.Type != "deposit" {
		t.Errorf("expected deposit type, got %q", deposit.Type)
	}
	if deposit.Symbol != "$CASH-USD" {
		t.Errorf("expected symbol $CASH-USD, got %q", deposit.Symbol)
	}
	if deposit.Currency != "USD" {
		t.Errorf("expected currency USD, got %q", deposit.Currency)
	}
	if deposit.Quantity != "10000" {
		t.Errorf("expected quantity 10000, got %q", deposit.Quantity)
	}
}

func TestService_Preview_Transfers(t *testing.T) {
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{"$CASH-GBP"}, nil)

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="GBP" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades></Trades>
					<CashTransactions></CashTransactions>
					<Transfers>
						<Transfer accountId="U111" acctAlias="TEST" currency="GBP" fxRateToBase="1" assetCategory="CASH" symbol="--" description="TRANSFER IN" reportDate="2025-07-10" date="2025-07-10" dateTime="2025-07-10;080000" settleDate="2025-07-11" type="INTERNAL" direction="IN" cashTransfer="250.00" transactionID="30000000020" />
						<Transfer accountId="U111" acctAlias="TEST" currency="GBP" fxRateToBase="1" assetCategory="CASH" symbol="--" description="TRANSFER OUT" reportDate="2025-09-01" date="2025-09-01" dateTime="2025-09-01;140000" settleDate="2025-09-02" type="INTERNAL" direction="OUT" cashTransfer="-100.00" transactionID="30000000021" />
					</Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.Preview(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if got.ImportableCount != 2 {
		t.Fatalf("expected 2 importable, got %d", got.ImportableCount)
	}

	if got.Importable[0].Type != "deposit" {
		t.Errorf("expected deposit type, got %q", got.Importable[0].Type)
	}
	if got.Importable[0].Symbol != "$CASH-GBP" {
		t.Errorf("expected symbol $CASH-GBP, got %q", got.Importable[0].Symbol)
	}

	if got.Importable[1].Type != "withdrawal" {
		t.Errorf("expected withdrawal type, got %q", got.Importable[1].Type)
	}
}

func TestService_Preview_CashTransactions(t *testing.T) {
	svc, resolver, _, _, _, _, _ := setupService(t, []int64{1}, []string{"STHY", "$CASH-USD", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("IBKR", "STHY", "STHY")

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="GBP" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades></Trades>
					<CashTransactions>
						<CashTransaction accountId="U111" currency="USD" symbol="STHY" description="DIV" amount="250.00" type="Dividends" transactionID="10" reportDate="2025-05-31" />
						<CashTransaction accountId="U111" currency="GBP" description="INT" amount="12.50" type="Broker Interest Received" transactionID="11" reportDate="2025-06-04" />
						<CashTransaction accountId="U111" currency="USD" symbol="STHY" description="TAX" amount="-25.00" type="Withholding Tax" transactionID="12" reportDate="2025-05-31" />
						<CashTransaction accountId="U111" currency="USD" description="FEE" amount="-0.15" type="Other Fees" transactionID="13" reportDate="2025-08-15" />
						<CashTransaction accountId="U111" currency="GBP" description="DEPOSIT" amount="5000" type="Deposits/Withdrawals" transactionID="14" reportDate="2025-04-10" />
						<CashTransaction accountId="U111" currency="GBP" description="WITHDRAWAL" amount="-3000" type="Deposits/Withdrawals" transactionID="15" reportDate="2025-09-20" />
					</CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.Preview(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if got.ImportableCount != 6 {
		t.Fatalf("expected 6 importable, got %d", got.ImportableCount)
	}

	wantTypes := []string{"dividend", "interest", "tax", "fee", "deposit", "withdrawal"}
	for i, wantType := range wantTypes {
		if got.Importable[i].Type != wantType {
			t.Errorf("entry %d: expected type %q, got %q", i, wantType, got.Importable[i].Type)
		}
	}

	if got.Importable[0].Symbol != "STHY" {
		t.Errorf("dividend: expected symbol STHY, got %q", got.Importable[0].Symbol)
	}
	if got.Importable[1].Symbol != "$CASH-GBP" {
		t.Errorf("interest: expected symbol $CASH-GBP, got %q", got.Importable[1].Symbol)
	}
}

func TestService_Preview_NonExistentAccount(t *testing.T) {
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.Preview(ctx, []byte("dummy"), 999)
	if err == nil {
		t.Fatal("expected error for non-existent account, got nil")
	}
	if err.Error() != "account not found" {
		t.Errorf("expected 'account not found', got %q", err.Error())
	}
}

func TestService_Preview_InvalidXML(t *testing.T) {
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.Preview(ctx, []byte("not xml"), 1)
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
	if !strings.Contains(err.Error(), "invalid XML data") {
		t.Errorf("expected 'invalid XML data', got %q", err.Error())
	}
}

func TestService_Preview_EmptySections(t *testing.T) {
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades></Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.Preview(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if got.ImportableCount != 0 {
		t.Errorf("expected 0 importable, got %d", got.ImportableCount)
	}
	if got.Importable == nil {
		t.Error("expected non-nil Importable slice")
	}
	if got.Skipped == nil {
		t.Error("expected non-nil Skipped slice")
	}
	if got.Errored == nil {
		t.Error("expected non-nil Errored slice")
	}
}

// ==================== CONFIRM IMPORT ====================

func TestService_ConfirmImport_Basic(t *testing.T) {
	svc, resolver, _, creator, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "STHY", "$CASH-USD", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")
	resolver.addBrokerSymbol("IBKR", "STHY", "STHY")

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades>
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="100" tradePrice="190" tradeMoney="19000" proceeds="-19000" netCash="-19003.80" buySell="BUY" tradeDate="2025-04-15" transactionID="30000000001" ibOrderID="5000000001" />
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="-50" tradePrice="220" tradeMoney="-11000" proceeds="11000" netCash="10997.80" buySell="SELL" tradeDate="2025-07-20" transactionID="30000000002" ibOrderID="5000000002" />
					</Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.ConfirmImport(ctx, xmlData, 1)
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
		if txn.ExternalSystem == nil || *txn.ExternalSystem != "IBKR" {
			t.Errorf("expected ExternalSystem 'IBKR', got %v", txn.ExternalSystem)
		}
		if txn.ExternalReference == nil {
			t.Error("expected non-nil ExternalReference")
		}
	}
}

func TestService_ConfirmImport_RollbackOnFailure(t *testing.T) {
	svc, resolver, _, creator, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "$CASH-USD"}, nil)
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")
	creator.WithFailure()

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades>
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="100" tradePrice="190" tradeMoney="19000" proceeds="-19000" netCash="-19003.80" buySell="BUY" tradeDate="2025-04-15" transactionID="30000000001" ibOrderID="5000000001" />
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="-50" tradePrice="220" tradeMoney="-11000" proceeds="11000" netCash="10997.80" buySell="SELL" tradeDate="2025-07-20" transactionID="30000000002" ibOrderID="5000000002" />
					</Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	_, err := svc.ConfirmImport(ctx, xmlData, 1)
	if err == nil {
		t.Fatal("expected error from batch create, got nil")
	}
}

func TestService_ConfirmImport_DetectsNewDuplicates(t *testing.T) {
	resolver := newMockSymbolResolver("AAPL")
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")
	dupCheck := newMockDuplicateChecker("30000000001")
	creator := newMockTransactionCreator()
	accounts := newMockAccountChecker(1)
	symCreate := newMockSymbolCreator()
	brokerAdd := newMockBrokerSymbolAdder()
	svc := NewService(resolver, dupCheck, creator, accounts, symCreate, brokerAdd)
	_ = creator // used in service, verified via got.CreatedCount

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades>
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="100" tradePrice="190" tradeMoney="19000" proceeds="-19000" netCash="-19003.80" buySell="BUY" tradeDate="2025-04-15" transactionID="30000000001" ibOrderID="5000000001" />
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="-50" tradePrice="220" tradeMoney="-11000" proceeds="11000" netCash="10997.80" buySell="SELL" tradeDate="2025-07-20" transactionID="30000000002" ibOrderID="5000000002" />
					</Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.ConfirmImport(ctx, xmlData, 1)
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
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.ConfirmImport(ctx, []byte("dummy"), 999)
	if err == nil {
		t.Fatal("expected error for non-existent account, got nil")
	}
}

func TestService_ConfirmImport_InvalidXML(t *testing.T) {
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	_, err := svc.ConfirmImport(ctx, []byte("not xml"), 1)
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestService_ConfirmImport_FXTrades(t *testing.T) {
	svc, _, _, creator, _, _, _ := setupService(t, []int64{1}, []string{"$CASH-GBP", "$CASH-USD"}, nil)

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="GBP" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades>
						<Trade accountId="U111" currency="GBP" assetCategory="CASH" subCategory="" symbol="GBP.USD" description="GBP.USD" quantity="10000" tradePrice="1.27" tradeMoney="12700" proceeds="-12700" netCash="0" buySell="BUY" tradeDate="2025-06-01" transactionID="30000000006" ibOrderID="5000000005" />
					</Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.ConfirmImport(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	if got.CreatedCount != 2 {
		t.Errorf("expected 2 created (FX generates 2), got %d", got.CreatedCount)
	}
	if creator.CreatedCount() != 2 {
		t.Errorf("expected 2 created in creator, got %d", creator.CreatedCount())
	}

	txns := creator.Created()
	if txns[0].Type != "withdrawal" {
		t.Errorf("expected first FX txn to be withdrawal, got %q", txns[0].Type)
	}
	if txns[0].Symbol != "$CASH-GBP" {
		t.Errorf("expected first FX txn symbol $CASH-GBP, got %q", txns[0].Symbol)
	}
	if txns[1].Type != "deposit" {
		t.Errorf("expected second FX txn to be deposit, got %q", txns[1].Type)
	}
	if txns[1].Symbol != "$CASH-USD" {
		t.Errorf("expected second FX txn symbol $CASH-USD, got %q", txns[1].Symbol)
	}
}

func TestService_ConfirmImport_MixedRecords(t *testing.T) {
	svc, resolver, _, _, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "STHY", "$CASH-USD", "$CASH-GBP"}, nil)
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")
	resolver.addBrokerSymbol("IBKR", "STHY", "STHY")

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades>
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="100" tradePrice="190" tradeMoney="19000" proceeds="-19000" netCash="-19003.80" buySell="BUY" tradeDate="2025-04-15" transactionID="30000000001" ibOrderID="5000000001" />
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="UNKNOWN" description="UNKNOWN" quantity="10" tradePrice="50" tradeMoney="500" proceeds="-500" netCash="-500" buySell="BUY" tradeDate="2025-04-15" transactionID="30000000099" ibOrderID="5000000099" />
					</Trades>
					<CashTransactions>
						<CashTransaction accountId="U111" currency="USD" symbol="STHY" description="DIV" amount="250.00" type="Dividends" transactionID="30000000010" reportDate="2025-05-31" />
						<CashTransaction accountId="U111" currency="GBP" description="INT" amount="12.50" type="Broker Interest Received" transactionID="30000000011" reportDate="2025-06-04" />
					</CashTransactions>
					<Transfers>
						<Transfer accountId="U111" currency="GBP" description="TRANSFER IN" date="2025-07-10" type="INTERNAL" direction="IN" cashTransfer="250.00" transactionID="30000000020" />
					</Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.ConfirmImport(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	// 1 AAPL buy + 1 STHY dividend + 1 GBP interest + 1 GBP deposit = 4 created
	// 1 UNKNOWN symbol = 1 skipped
	if got.CreatedCount != 4 {
		t.Errorf("expected 4 created, got %d", got.CreatedCount)
	}
	if got.SkippedCount != 1 {
		t.Errorf("expected 1 skipped, got %d", got.SkippedCount)
	}
}

func TestService_ConfirmImport_TradeFieldValues(t *testing.T) {
	svc, resolver, _, creator, _, _, _ := setupService(t, []int64{1}, []string{"AAPL"}, nil)
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades>
						<Trade accountId="U111" currency="USD" assetCategory="STK" subCategory="COMMON" symbol="AAPL" description="APPLE" quantity="100" tradePrice="190" tradeMoney="19000" proceeds="-19000" netCash="-19003.80" buySell="BUY" tradeDate="2025-04-15" transactionID="30000000001" ibOrderID="5000000001" />
					</Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	_, err := svc.ConfirmImport(ctx, xmlData, 1)
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
	if !txn.Quantity.Equal(decimal.MustNew(100, 0)) {
		t.Errorf("expected Quantity 100, got %s", txn.Quantity.String())
	}
	if !txn.Price.Equal(decimal.MustNew(190, 0)) {
		t.Errorf("expected Price 190, got %s", txn.Price.String())
	}
	if !txn.NetCash.Equal(decimal.MustNew(-1900380, 2)) {
		t.Errorf("expected NetCash -19003.80, got %s", txn.NetCash.String())
	}
	if txn.Currency != "USD" {
		t.Errorf("expected Currency 'USD', got %q", txn.Currency)
	}
	wantDate := time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC)
	if !txn.Date.Equal(wantDate) {
		t.Errorf("expected Date 2025-04-15, got %s", txn.Date.Format(time.RFC3339))
	}
	if txn.ExternalSystem == nil || *txn.ExternalSystem != "IBKR" {
		t.Errorf("expected ExternalSystem 'IBKR', got %v", txn.ExternalSystem)
	}
	if txn.ExternalReference == nil || *txn.ExternalReference != "30000000001" {
		t.Errorf("expected ExternalReference '30000000001', got %v", txn.ExternalReference)
	}
}

func TestService_ConfirmImport_CashTransactionFieldValues(t *testing.T) {
	svc, resolver, _, creator, _, _, _ := setupService(t, []int64{1}, []string{"STHY", "$CASH-USD"}, nil)
	resolver.addBrokerSymbol("IBKR", "STHY", "STHY")

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades></Trades>
					<CashTransactions>
						<CashTransaction accountId="U111" currency="USD" symbol="STHY" description="DIV" amount="250.00" type="Dividends" transactionID="30000000010" reportDate="2025-05-31" />
					</CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	_, err := svc.ConfirmImport(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	txns := creator.Created()
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	txn := txns[0]

	if txn.Type != "dividend" {
		t.Errorf("expected Type 'dividend', got %q", txn.Type)
	}
	if txn.Symbol != "STHY" {
		t.Errorf("expected Symbol 'STHY', got %q", txn.Symbol)
	}
	if !txn.Quantity.Equal(decimal.MustNew(25000, 2)) {
		t.Errorf("expected Quantity 250.00, got %s", txn.Quantity.String())
	}
	if !txn.Price.Equal(decimal.One) {
		t.Errorf("expected Price 1, got %s", txn.Price.String())
	}
	wantDate := time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC)
	if !txn.Date.Equal(wantDate) {
		t.Errorf("expected Date 2025-05-31, got %s", txn.Date.Format(time.RFC3339))
	}
}

func TestService_ConfirmImport_EmptyReport(t *testing.T) {
	svc, _, _, _, _, _, _ := setupService(t, []int64{1}, []string{}, nil)

	xmlData := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="2025-01-01" toDate="2025-12-31" period="test" whenGenerated="2025-12-31;120000">
					<Trades></Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)

	got, err := svc.ConfirmImport(ctx, xmlData, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	if got.CreatedCount != 0 {
		t.Errorf("expected 0 created, got %d", got.CreatedCount)
	}
}

// ==================== HELPER FUNCTIONS ====================

func TestClassifyTrade(t *testing.T) {
	tests := []struct {
		name     string
		buySell  string
		expected string
	}{
		{"buy", "BUY", "buy"},
		{"sell", "SELL", "sell"},
		{"unknown fallback", "TRANSFER", "buy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trade := Trade{BuySell: tt.buySell}
			got := classifyTrade(trade)
			if got != tt.expected {
				t.Errorf("classifyTrade(%q) = %q, want %q", tt.buySell, got, tt.expected)
			}
		})
	}
}

func TestClassifyCashTransaction(t *testing.T) {
	tests := []struct {
		name       string
		amount     string
		ctype      string
		wantType   string
		wantSymbol bool
	}{
		{"dividend", "250.00", "Dividends", "dividend", true},
		{"withholding tax", "-25.00", "Withholding Tax", "tax", false},
		{"broker interest", "12.50", "Broker Interest Received", "interest", false},
		{"other fees", "-0.15", "Other Fees", "fee", false},
		{"deposit", "5000", "Deposits/Withdrawals", "deposit", false},
		{"withdrawal", "-3000", "Deposits/Withdrawals", "withdrawal", false},
		{"zero amount", "0", "Deposits/Withdrawals", "deposit", false},
		{"empty amount", "", "Deposits/Withdrawals", "deposit", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := CashTransaction{Amount: tt.amount, Type: tt.ctype}
			gotType, gotSymbol := classifyCashTransaction(ct)
			if gotType != tt.wantType {
				t.Errorf("classifyCashTransaction(%q, %q) type = %q, want %q", tt.amount, tt.ctype, gotType, tt.wantType)
			}
			if gotSymbol != tt.wantSymbol {
				t.Errorf("classifyCashTransaction(%q, %q) needsSymbol = %v, want %v", tt.amount, tt.ctype, gotSymbol, tt.wantSymbol)
			}
		})
	}
}

func TestClassifyTransfer(t *testing.T) {
	tests := []struct {
		name       string
		direction  string
		cashTransfer string
		expected   string
	}{
		{"deposit IN", "IN", "250.00", "deposit"},
		{"withdrawal OUT", "OUT", "-100.00", "withdrawal"},
		{"fallback positive", "", "50", "deposit"},
		{"fallback negative", "", "-50", "withdrawal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := Transfer{Direction: tt.direction, CashTransfer: tt.cashTransfer}
			got := classifyTransfer(tr)
			if got != tt.expected {
				t.Errorf("classifyTransfer(%q, %q) = %q, want %q", tt.direction, tt.cashTransfer, got, tt.expected)
			}
		})
	}
}

func TestIsSupportedTrade(t *testing.T) {
	tests := []struct {
		name          string
		assetCategory string
		subCategory   string
		expected      bool
	}{
		{"common stock", "STK", "COMMON", true},
		{"ETF", "STK", "ETF", true},
		{"cash FX", "CASH", "", false},
		{"bond", "BND", "", false},
		{"opt", "OPT", "", false},
		{"futures", "FUT", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trade := Trade{AssetCategory: tt.assetCategory, SubCategory: tt.subCategory}
			got := isSupportedTrade(trade)
			if got != tt.expected {
				t.Errorf("isSupportedTrade(%q, %q) = %v, want %v", tt.assetCategory, tt.subCategory, got, tt.expected)
			}
		})
	}
}

func TestAbsStr(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{"-123", "123"},
		{"123", "123"},
		{"-0", "0"},
		{"", ""},
		{"-19003.80", "19003.80"},
	}

	for _, tt := range tests {
		got := absStr(tt.input)
		if got != tt.output {
			t.Errorf("absStr(%q) = %q, want %q", tt.input, got, tt.output)
		}
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
		{"JPY", "$CASH-JPY"},
	}

	for _, tt := range tests {
		got := cashSymbol(tt.currency)
		if got != tt.expected {
			t.Errorf("cashSymbol(%q) = %q, want %q", tt.currency, got, tt.expected)
		}
	}
}

// ==================== CREATE SYMBOL / ADD BROKER SYMBOL ====================

func TestService_CreateSymbol(t *testing.T) {
	svc, _, _, _, _, symCreate, _ := setupService(t, []int64{1}, []string{}, nil)

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
	svc, _, _, _, _, _, brokerAdd := setupService(t, []int64{1}, []string{}, nil)

	err := svc.AddBrokerSymbolMapping(ctx, "IBKR", "AAPL", "AAPL")
	if err != nil {
		t.Fatalf("AddBrokerSymbolMapping: %v", err)
	}
	if len(brokerAdd.added) != 1 {
		t.Errorf("expected 1 mapping added, got %d", len(brokerAdd.added))
	}
	if brokerAdd.added[0].brokerName != "IBKR" {
		t.Errorf("expected brokerName 'IBKR', got %q", brokerAdd.added[0].brokerName)
	}
	if brokerAdd.added[0].brokerSymbol != "AAPL" {
		t.Errorf("expected brokerSymbol 'AAPL', got %q", brokerAdd.added[0].brokerSymbol)
	}
	if brokerAdd.added[0].internal != "AAPL" {
		t.Errorf("expected internal 'AAPL', got %q", brokerAdd.added[0].internal)
	}
}

// ==================== SAMPLE XML INTEGRATION ====================

func TestService_ConfirmImport_SampleXML(t *testing.T) {
	svc, resolver, _, creator, _, _, _ := setupService(t, []int64{1}, []string{"AAPL", "STHY", "$CASH-USD", "$CASH-GBP", "$CASH-XYZ"}, nil)
	resolver.addBrokerSymbol("IBKR", "AAPL", "AAPL")
	resolver.addBrokerSymbol("IBKR", "STHY", "STHY")

	data := loadSampleXML(t)
	got, err := svc.ConfirmImport(ctx, data, 1)
	if err != nil {
		t.Fatalf("ConfirmImport: %v", err)
	}

	// 5 STK trades + 2 FX txns + 7 cash txns + 2 transfers = 16
	if got.CreatedCount != 16 {
		t.Errorf("expected 16 created, got %d", got.CreatedCount)
	}
	if creator.CreatedCount() != 16 {
		t.Errorf("expected 16 in creator, got %d", creator.CreatedCount())
	}

	// Verify external fields on all transactions
	for i, txn := range creator.Created() {
		if txn.ExternalSystem == nil || *txn.ExternalSystem != "IBKR" {
			t.Errorf("txn %d: expected ExternalSystem 'IBKR', got %v", i, txn.ExternalSystem)
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
