package ibkrimport

import (
	"os"
	"path/filepath"
	"testing"
)

// loadSampleXML reads the IBKR sample XML fixture.
func loadSampleXML(t *testing.T) []byte {
	t.Helper()
	dir := filepath.Dir(t.Name())
	_ = dir // avoid unused warning
	path := filepath.Join("testdata", "ibkr_sample.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sample XML: %v", err)
	}
	return data
}

func TestParseXML_ValidSample(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// Verify summary metadata.
	if report.AccountID != "U12345678" {
		t.Errorf("expected AccountID U12345678, got %q", report.AccountID)
	}
	if report.AccountAlias != "MY-PORTFOLIO" {
		t.Errorf("expected AccountAlias MY-PORTFOLIO, got %q", report.AccountAlias)
	}
	if report.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", report.Currency)
	}
	if report.FromDate != "20250401" {
		t.Errorf("expected FromDate 20250401, got %q", report.FromDate)
	}
	if report.ToDate != "20250930" {
		t.Errorf("expected ToDate 20250930, got %q", report.ToDate)
	}
}

func TestParseXML_Trades_Count(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	if len(report.Trades) != 6 {
		t.Errorf("expected 6 Trades, got %d", len(report.Trades))
	}
}

func TestParseXML_Trades_STK_COMMON_Buy(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// First trade: AAPL BUY
	trade := report.Trades[0]
	if trade.Symbol != "AAPL" {
		t.Errorf("expected Symbol AAPL, got %q", trade.Symbol)
	}
	if trade.AssetCategory != "STK" {
		t.Errorf("expected AssetCategory STK, got %q", trade.AssetCategory)
	}
	if trade.SubCategory != "COMMON" {
		t.Errorf("expected SubCategory COMMON, got %q", trade.SubCategory)
	}
	if trade.BuySell != "BUY" {
		t.Errorf("expected BuySell BUY, got %q", trade.BuySell)
	}
	if trade.Quantity != "100" {
		t.Errorf("expected Quantity 100, got %q", trade.Quantity)
	}
	if trade.TradePrice != "190" {
		t.Errorf("expected TradePrice 190, got %q", trade.TradePrice)
	}
	if trade.NetCash != "-19003.80" {
		t.Errorf("expected NetCash -19003.80, got %q", trade.NetCash)
	}
	if trade.Currency != "USD" {
		t.Errorf("expected Currency USD, got %q", trade.Currency)
	}
	if trade.TransactionID != "30000000001" {
		t.Errorf("expected TransactionID 30000000001, got %q", trade.TransactionID)
	}
	if trade.ReportDate != "20250415" {
		t.Errorf("expected ReportDate 20250415, got %q", trade.ReportDate)
	}
	if trade.IbCommission != "-3.80" {
		t.Errorf("expected IbCommission -3.80, got %q", trade.IbCommission)
	}
	if trade.IbOrderID != "5000000001" {
		t.Errorf("expected IbOrderID 5000000001, got %q", trade.IbOrderID)
	}
}

func TestParseXML_Trades_STK_COMMON_Sell(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// Second trade: AAPL SELL (negative quantity)
	trade := report.Trades[1]
	if trade.Symbol != "AAPL" {
		t.Errorf("expected Symbol AAPL, got %q", trade.Symbol)
	}
	if trade.BuySell != "SELL" {
		t.Errorf("expected BuySell SELL, got %q", trade.BuySell)
	}
	if trade.Quantity != "-50" {
		t.Errorf("expected Quantity -50, got %q", trade.Quantity)
	}
	if trade.TradePrice != "220" {
		t.Errorf("expected TradePrice 220, got %q", trade.TradePrice)
	}
	if trade.NetCash != "10997.80" {
		t.Errorf("expected NetCash 10997.80, got %q", trade.NetCash)
	}
	if trade.TransactionID != "30000000002" {
		t.Errorf("expected TransactionID 30000000002, got %q", trade.TransactionID)
	}
}

func TestParseXML_Trades_STK_ETF_Buy(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// Third trade: STHY BUY (ETF)
	trade := report.Trades[2]
	if trade.Symbol != "STHY" {
		t.Errorf("expected Symbol STHY, got %q", trade.Symbol)
	}
	if trade.SubCategory != "ETF" {
		t.Errorf("expected SubCategory ETF, got %q", trade.SubCategory)
	}
	if trade.BuySell != "BUY" {
		t.Errorf("expected BuySell BUY, got %q", trade.BuySell)
	}
	if trade.Quantity != "500" {
		t.Errorf("expected Quantity 500, got %q", trade.Quantity)
	}
	if trade.TradePrice != "96" {
		t.Errorf("expected TradePrice 96, got %q", trade.TradePrice)
	}
	if trade.NetCash != "-48010" {
		t.Errorf("expected NetCash -48010, got %q", trade.NetCash)
	}
	if trade.TransactionID != "30000000003" {
		t.Errorf("expected TransactionID 30000000003, got %q", trade.TransactionID)
	}
}

func TestParseXML_Trades_STK_ETF_Sell_MultipleLots(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// Fourth and fifth trades: STHY SELL (two lots from same order)
	trade4 := report.Trades[3]
	trade5 := report.Trades[4]

	if trade4.Symbol != "STHY" || trade4.BuySell != "SELL" {
		t.Errorf("expected STHY SELL, got %q %q", trade4.Symbol, trade4.BuySell)
	}
	if trade4.Quantity != "-200" {
		t.Errorf("expected Quantity -200, got %q", trade4.Quantity)
	}
	if trade4.TransactionID != "30000000004" {
		t.Errorf("expected TransactionID 30000000004, got %q", trade4.TransactionID)
	}
	if trade4.IbOrderID != "5000000004" {
		t.Errorf("expected IbOrderID 5000000004, got %q", trade4.IbOrderID)
	}

	if trade5.Symbol != "STHY" || trade5.BuySell != "SELL" {
		t.Errorf("expected STHY SELL, got %q %q", trade5.Symbol, trade5.BuySell)
	}
	if trade5.Quantity != "-300" {
		t.Errorf("expected Quantity -300, got %q", trade5.Quantity)
	}
	if trade5.TransactionID != "30000000005" {
		t.Errorf("expected TransactionID 30000000005, got %q", trade5.TransactionID)
	}
	// Same order, different transaction IDs
	if trade5.IbOrderID != "5000000004" {
		t.Errorf("expected IbOrderID 5000000004 (same order), got %q", trade5.IbOrderID)
	}
}

func TestParseXML_Trades_CASH_FX(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// Sixth trade: GBP.USD FX
	trade := report.Trades[5]
	if trade.Symbol != "GBP.USD" {
		t.Errorf("expected Symbol GBP.USD, got %q", trade.Symbol)
	}
	if trade.AssetCategory != "CASH" {
		t.Errorf("expected AssetCategory CASH, got %q", trade.AssetCategory)
	}
	if trade.SubCategory != "" {
		t.Errorf("expected empty SubCategory, got %q", trade.SubCategory)
	}
	if trade.BuySell != "BUY" {
		t.Errorf("expected BuySell BUY, got %q", trade.BuySell)
	}
	if trade.Quantity != "10000" {
		t.Errorf("expected Quantity 10000, got %q", trade.Quantity)
	}
	if trade.TradePrice != "1.27" {
		t.Errorf("expected TradePrice 1.27, got %q", trade.TradePrice)
	}
	if trade.NetCash != "0" {
		t.Errorf("expected NetCash 0, got %q", trade.NetCash)
	}
	if trade.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", trade.Currency)
	}
	if trade.TransactionID != "30000000006" {
		t.Errorf("expected TransactionID 30000000006, got %q", trade.TransactionID)
	}
}

func TestParseXML_CashTransactions_Count(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	if len(report.CashTransactions) != 7 {
		t.Errorf("expected 7 CashTransactions, got %d", len(report.CashTransactions))
	}
}

func TestParseXML_CashTransactions_Dividend(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[0]
	if ct.Type != "Dividends" {
		t.Errorf("expected Type Dividends, got %q", ct.Type)
	}
	if ct.Symbol != "STHY" {
		t.Errorf("expected Symbol STHY, got %q", ct.Symbol)
	}
	if ct.Amount != "250.00" {
		t.Errorf("expected Amount 250.00, got %q", ct.Amount)
	}
	if ct.Currency != "USD" {
		t.Errorf("expected Currency USD, got %q", ct.Currency)
	}
	if ct.TransactionID != "30000000010" {
		t.Errorf("expected TransactionID 30000000010, got %q", ct.TransactionID)
	}
	if ct.ReportDate != "20250531" {
		t.Errorf("expected ReportDate 20250531, got %q", ct.ReportDate)
	}
	if ct.ExDate != "20250515" {
		t.Errorf("expected ExDate 20250515, got %q", ct.ExDate)
	}
}

func TestParseXML_CashTransactions_BrokerInterestGBP(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[1]
	if ct.Type != "Broker Interest Received" {
		t.Errorf("expected Type 'Broker Interest Received', got %q", ct.Type)
	}
	if ct.Symbol != "" {
		t.Errorf("expected empty Symbol, got %q", ct.Symbol)
	}
	if ct.Amount != "12.50" {
		t.Errorf("expected Amount 12.50, got %q", ct.Amount)
	}
	if ct.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", ct.Currency)
	}
	if ct.TransactionID != "30000000011" {
		t.Errorf("expected TransactionID 30000000011, got %q", ct.TransactionID)
	}
}

func TestParseXML_CashTransactions_BrokerInterestUSD(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[2]
	if ct.Type != "Broker Interest Received" {
		t.Errorf("expected Type 'Broker Interest Received', got %q", ct.Type)
	}
	if ct.Amount != "85.00" {
		t.Errorf("expected Amount 85.00, got %q", ct.Amount)
	}
	if ct.Currency != "USD" {
		t.Errorf("expected Currency USD, got %q", ct.Currency)
	}
	if ct.TransactionID != "30000000012" {
		t.Errorf("expected TransactionID 30000000012, got %q", ct.TransactionID)
	}
}

func TestParseXML_CashTransactions_WithholdingTax(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[3]
	if ct.Type != "Withholding Tax" {
		t.Errorf("expected Type 'Withholding Tax', got %q", ct.Type)
	}
	if ct.Symbol != "STHY" {
		t.Errorf("expected Symbol STHY, got %q", ct.Symbol)
	}
	if ct.Amount != "-25.00" {
		t.Errorf("expected Amount -25.00, got %q", ct.Amount)
	}
	if ct.Currency != "USD" {
		t.Errorf("expected Currency USD, got %q", ct.Currency)
	}
	if ct.TransactionID != "30000000013" {
		t.Errorf("expected TransactionID 30000000013, got %q", ct.TransactionID)
	}
}

func TestParseXML_CashTransactions_OtherFees(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[4]
	if ct.Type != "Other Fees" {
		t.Errorf("expected Type 'Other Fees', got %q", ct.Type)
	}
	if ct.Amount != "-0.15" {
		t.Errorf("expected Amount -0.15, got %q", ct.Amount)
	}
	if ct.Currency != "USD" {
		t.Errorf("expected Currency USD, got %q", ct.Currency)
	}
	if ct.TransactionID != "30000000014" {
		t.Errorf("expected TransactionID 30000000014, got %q", ct.TransactionID)
	}
}

func TestParseXML_CashTransactions_Deposit(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[5]
	if ct.Type != "Deposits/Withdrawals" {
		t.Errorf("expected Type 'Deposits/Withdrawals', got %q", ct.Type)
	}
	if ct.Amount != "5000" {
		t.Errorf("expected Amount 5000, got %q", ct.Amount)
	}
	if ct.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", ct.Currency)
	}
	if ct.TransactionID != "30000000015" {
		t.Errorf("expected TransactionID 30000000015, got %q", ct.TransactionID)
	}
}

func TestParseXML_CashTransactions_Withdrawal(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[6]
	if ct.Type != "Deposits/Withdrawals" {
		t.Errorf("expected Type 'Deposits/Withdrawals', got %q", ct.Type)
	}
	if ct.Amount != "-3000" {
		t.Errorf("expected Amount -3000, got %q", ct.Amount)
	}
	if ct.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", ct.Currency)
	}
	if ct.TransactionID != "30000000016" {
		t.Errorf("expected TransactionID 30000000016, got %q", ct.TransactionID)
	}
}

func TestParseXML_Transfers_Count(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	if len(report.Transfers) != 2 {
		t.Errorf("expected 2 Transfers, got %d", len(report.Transfers))
	}
}

func TestParseXML_Transfers_Deposit(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	tr := report.Transfers[0]
	if tr.Type != "INTERNAL" {
		t.Errorf("expected Type INTERNAL, got %q", tr.Type)
	}
	if tr.Direction != "IN" {
		t.Errorf("expected Direction IN, got %q", tr.Direction)
	}
	if tr.CashTransfer != "250.00" {
		t.Errorf("expected CashTransfer 250.00, got %q", tr.CashTransfer)
	}
	if tr.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", tr.Currency)
	}
	if tr.TransactionID != "30000000020" {
		t.Errorf("expected TransactionID 30000000020, got %q", tr.TransactionID)
	}
	if tr.Date != "20250710" {
		t.Errorf("expected Date 20250710, got %q", tr.Date)
	}
}

func TestParseXML_Transfers_Withdrawal(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	tr := report.Transfers[1]
	if tr.Type != "INTERNAL" {
		t.Errorf("expected Type INTERNAL, got %q", tr.Type)
	}
	if tr.Direction != "OUT" {
		t.Errorf("expected Direction OUT, got %q", tr.Direction)
	}
	if tr.CashTransfer != "-100.00" {
		t.Errorf("expected CashTransfer -100.00, got %q", tr.CashTransfer)
	}
	if tr.Currency != "GBP" {
		t.Errorf("expected Currency GBP, got %q", tr.Currency)
	}
	if tr.TransactionID != "30000000021" {
		t.Errorf("expected TransactionID 30000000021, got %q", tr.TransactionID)
	}
	if tr.Date != "20250901" {
		t.Errorf("expected Date 20250901, got %q", tr.Date)
	}
}

func TestParseXML_EmptyData(t *testing.T) {
	_, err := ParseXML([]byte{})
	if err == nil {
		t.Error("expected error for empty data, got nil")
	}
}

func TestParseXML_InvalidXML(t *testing.T) {
	_, err := ParseXML([]byte("not xml at all"))
	if err == nil {
		t.Error("expected error for invalid XML, got nil")
	}
}

func TestParseXML_EmptyXML(t *testing.T) {
	_, err := ParseXML([]byte("<></>"))
	if err == nil {
		t.Error("expected error for empty XML, got nil")
	}
}

func TestParseXML_NoFlexStatement(t *testing.T) {
	data := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="0">
			</FlexStatements>
		</FlexQueryResponse>
	`)
	_, err := ParseXML(data)
	if err == nil {
		t.Error("expected error for no FlexStatement, got nil")
	}
}

func TestParseXML_EmptySections(t *testing.T) {
	// XML with empty Trades, CashTransactions, and Transfers sections
	data := []byte(`
		<FlexQueryResponse queryName="test" type="AF">
			<FlexStatements count="1">
				<FlexStatement accountId="U111" acctAlias="TEST" currency="USD" fromDate="20250101" toDate="20251231" period="test" whenGenerated="20251231;120000">
					<Trades></Trades>
					<CashTransactions></CashTransactions>
					<Transfers></Transfers>
				</FlexStatement>
			</FlexStatements>
		</FlexQueryResponse>
	`)
	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// Empty sections should return empty slices, not nil
	if report.Trades == nil {
		t.Error("expected non-nil Trades slice, got nil")
	}
	if len(report.Trades) != 0 {
		t.Errorf("expected 0 Trades, got %d", len(report.Trades))
	}
	if report.CashTransactions == nil {
		t.Error("expected non-nil CashTransactions slice, got nil")
	}
	if len(report.CashTransactions) != 0 {
		t.Errorf("expected 0 CashTransactions, got %d", len(report.CashTransactions))
	}
	if report.Transfers == nil {
		t.Error("expected non-nil Transfers slice, got nil")
	}
	if len(report.Transfers) != 0 {
		t.Errorf("expected 0 Transfers, got %d", len(report.Transfers))
	}
}

func TestParseXML_TradeFields_Complete(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	// Verify all fields of the first trade are populated correctly
	trade := report.Trades[0]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"AccountID", trade.AccountID, "U12345678"},
		{"AcctAlias", trade.AcctAlias, "MY-PORTFOLIO"},
		{"Currency", trade.Currency, "USD"},
		{"FxRateToBase", trade.FxRateToBase, "0.79"},
		{"AssetCategory", trade.AssetCategory, "STK"},
		{"SubCategory", trade.SubCategory, "COMMON"},
		{"Symbol", trade.Symbol, "AAPL"},
		{"Description", trade.Description, "APPLE INC"},
		{"Conid", trade.Conid, "72063691"},
		{"Isin", trade.Isin, "US0378331005"},
		{"TradeID", trade.TradeID, "1000000001"},
		{"Multiplier", trade.Multiplier, "1"},
		{"ReportDate", trade.ReportDate, "20250415"},
		{"DateTime", trade.DateTime, "20250415;143022"},
		{"TradeDate", trade.TradeDate, "20250415"},
		{"SettleDateTarget", trade.SettleDateTarget, "20250417"},
		{"TransactionType", trade.TransactionType, "ExchTrade"},
		{"Exchange", trade.Exchange, "ARCA"},
		{"Quantity", trade.Quantity, "100"},
		{"TradePrice", trade.TradePrice, "190"},
		{"TradeMoney", trade.TradeMoney, "19000"},
		{"Proceeds", trade.Proceeds, "-19000"},
		{"Taxes", trade.Taxes, "0"},
		{"IbCommission", trade.IbCommission, "-3.80"},
		{"IbCommissionCurrency", trade.IbCommissionCurrency, "USD"},
		{"NetCash", trade.NetCash, "-19003.80"},
		{"ClosePrice", trade.ClosePrice, "191.50"},
		{"BuySell", trade.BuySell, "BUY"},
		{"IbOrderID", trade.IbOrderID, "5000000001"},
		{"TransactionID", trade.TransactionID, "30000000001"},
		{"IbExecID", trade.IbExecID, "aabbccdd.00112233.01.01"},
		{"Cost", trade.Cost, "19003.80"},
		{"FifoPnlRealized", trade.FifoPnlRealized, "0"},
		{"MtMpnL", trade.MtMpnL, "150"},
		{"Notes", trade.Notes, ""},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseXML_CashTransactionFields_Dividend(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	ct := report.CashTransactions[0]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"AccountID", ct.AccountID, "U12345678"},
		{"AcctAlias", ct.AcctAlias, "MY-PORTFOLIO"},
		{"Currency", ct.Currency, "USD"},
		{"FxRateToBase", ct.FxRateToBase, "0.77"},
		{"AssetCategory", ct.AssetCategory, "STK"},
		{"SubCategory", ct.SubCategory, "ETF"},
		{"Symbol", ct.Symbol, "STHY"},
		{"Description", ct.Description, "STHY(IE00B7N3YW49) CASH DIVIDEND USD 0.50 PER SHARE"},
		{"Conid", ct.Conid, "104217732"},
		{"Isin", ct.Isin, "IE00B7N3YW49"},
		{"DateTime", ct.DateTime, "20250530;200000"},
		{"SettleDate", ct.SettleDate, "20250530"},
		{"Amount", ct.Amount, "250.00"},
		{"Type", ct.Type, "Dividends"},
		{"TransactionID", ct.TransactionID, "30000000010"},
		{"ReportDate", ct.ReportDate, "20250531"},
		{"ExDate", ct.ExDate, "20250515"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}

func TestParseXML_TransferFields_Complete(t *testing.T) {
	data := loadSampleXML(t)

	report, err := ParseXML(data)
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}

	tr := report.Transfers[0]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"AccountID", tr.AccountID, "U12345678"},
		{"AcctAlias", tr.AcctAlias, "MY-PORTFOLIO"},
		{"Currency", tr.Currency, "GBP"},
		{"FxRateToBase", tr.FxRateToBase, "1"},
		{"AssetCategory", tr.AssetCategory, "CASH"},
		{"Symbol", tr.Symbol, "--"},
		{"Description", tr.Description, "TRANSFER FROM U87654321 TO U12345678"},
		{"ReportDate", tr.ReportDate, "20250710"},
		{"Date", tr.Date, "20250710"},
		{"DateTime", tr.DateTime, "20250710;080000"},
		{"SettleDate", tr.SettleDate, "20250711"},
		{"Type", tr.Type, "INTERNAL"},
		{"Direction", tr.Direction, "IN"},
		{"Account", tr.Account, "U****4321"},
		{"CashTransfer", tr.CashTransfer, "250.00"},
		{"TransactionID", tr.TransactionID, "30000000020"},
		{"ClientReference", tr.ClientReference, "Internal transfer"},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.field, tc.want, tc.got)
		}
	}
}
