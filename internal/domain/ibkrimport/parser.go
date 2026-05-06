package ibkrimport

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// ParsedReport is the result of parsing an IBKR Flex Web Report XML file.
// It contains extracted Trades, CashTransactions, and Transfers along with
// summary metadata from the FlexStatement.
type ParsedReport struct {
	// AccountID is the IBKR account identifier (e.g., "U12345678").
	AccountID string
	// AccountAlias is the user-defined account name (e.g., "MY-PORTFOLIO").
	AccountAlias string
	// Currency is the base currency of the account (e.g., "GBP").
	Currency string
	// FromDate is the start date of the report period (YYYY-MM-DD).
	FromDate string
	// ToDate is the end date of the report period (YYYY-MM-DD).
	ToDate string
	// Trades are all trade records (stocks, ETFs, FX).
	Trades []Trade
	// CashTransactions are all cash transaction records (dividends, interest, fees, etc.).
	CashTransactions []CashTransaction
	// Transfers are all transfer records (internal account transfers).
	Transfers []Transfer
}

// Trade represents a single trade record from an IBKR Flex XML file.
// All fields are stored as strings — the service layer is responsible for
// parsing numeric/date values.
type Trade struct {
	// AccountID is the IBKR account identifier.
	AccountID string
	// AcctAlias is the user-defined account name.
	AcctAlias string
	// Currency is the trade currency.
	Currency string
	// FxRateToBase is the FX rate from trade currency to account base currency.
	FxRateToBase string
	// AssetCategory is the instrument category (e.g., "STK", "CASH", "OPT").
	AssetCategory string
	// SubCategory is the instrument sub-category (e.g., "COMMON", "ETF").
	SubCategory string
	// Symbol is the broker symbol (e.g., "AAPL", "STHY", "GBP.USD").
	Symbol string
	// Description is the human-readable description.
	Description string
	// Conid is the IBKR contract ID.
	Conid string
	// Isin is the ISIN identifier.
	Isin string
	// TradeID is the IBKR trade identifier.
	TradeID string
	// Multiplier is the contract multiplier (usually "1" for stocks/ETFs).
	Multiplier string
	// ReportDate is the date the trade appears in the report (YYYY-MM-DD).
	ReportDate string
	// DateTime is the trade date and time (YYYY-MM-DD;HHMMSS).
	DateTime string
	// TradeDate is the trade execution date (YYYY-MM-DD).
	TradeDate string
	// SettleDateTarget is the expected settlement date (YYYY-MM-DD).
	SettleDateTarget string
	// TransactionType is the trade type (e.g., "ExchTrade").
	TransactionType string
	// Exchange is the exchange where the trade executed.
	Exchange string
	// Quantity is the number of shares/contracts (negative for sells).
	Quantity string
	// TradePrice is the execution price per share/contract.
	TradePrice string
	// TradeMoney is the gross trade amount (quantity * price).
	TradeMoney string
	// Proceeds is the cash proceeds from the trade (negative for buys).
	Proceeds string
	// Taxes is the tax amount.
	Taxes string
	// IbCommission is the IBKR commission (negative = charged).
	IbCommission string
	// IbCommissionCurrency is the commission currency.
	IbCommissionCurrency string
	// NetCash is the net cash flow from the trade.
	NetCash string
	// ClosePrice is the closing price on the trade date.
	ClosePrice string
	// BuySell indicates whether it's a "BUY" or "SELL".
	BuySell string
	// IbOrderID is the IBKR order identifier.
	IbOrderID string
	// TransactionID is the unique transaction identifier used for duplicate detection.
	TransactionID string
	// IbExecID is the IBKR execution identifier.
	IbExecID string
	// Cost is the total cost of the trade.
	Cost string
	// FifoPnlRealized is the realized P&L using FIFO method.
	FifoPnlRealized string
	// MtMpnL is the mark-to-market P&L.
	MtMpnL string
	// Notes contains any additional notes (e.g., "P" for partial).
	Notes string
}

// CashTransaction represents a single cash transaction record from an IBKR Flex XML file.
// Types include: Dividends, Broker Interest Received, Withholding Tax, Other Fees,
// Deposits/Withdrawals, and other cash events.
type CashTransaction struct {
	// AccountID is the IBKR account identifier.
	AccountID string
	// AcctAlias is the user-defined account name.
	AcctAlias string
	// Currency is the transaction currency.
	Currency string
	// FxRateToBase is the FX rate from transaction currency to account base currency.
	FxRateToBase string
	// AssetCategory is the instrument category (may be empty for generic cash txns).
	AssetCategory string
	// SubCategory is the instrument sub-category.
	SubCategory string
	// Symbol is the broker symbol (present for dividends, empty for generic cash txns).
	Symbol string
	// Description is the human-readable description.
	Description string
	// Conid is the IBKR contract ID.
	Conid string
	// Isin is the ISIN identifier.
	Isin string
	// DateTime is the transaction date and time (YYYY-MM-DD;HHMMSS or YYYY-MM-DD).
	DateTime string
	// SettleDate is the settlement date (YYYY-MM-DD).
	SettleDate string
	// Amount is the transaction amount (positive = credit, negative = debit).
	Amount string
	// Type is the transaction type (e.g., "Dividends", "Broker Interest Received",
	// "Withholding Tax", "Other Fees", "Deposits/Withdrawals").
	Type string
	// TradeID is the associated trade ID (if any).
	TradeID string
	// TransactionID is the unique transaction identifier used for duplicate detection.
	TransactionID string
	// ReportDate is the date the transaction appears in the report (YYYY-MM-DD).
	ReportDate string
	// ExDate is the ex-dividend date (for dividends).
	ExDate string
}

// Transfer represents a single transfer record from an IBKR Flex XML file.
// Transfers are typically internal account-to-account cash movements.
type Transfer struct {
	// AccountID is the IBKR account identifier.
	AccountID string
	// AcctAlias is the user-defined account name.
	AcctAlias string
	// Currency is the transfer currency.
	Currency string
	// FxRateToBase is the FX rate from transfer currency to account base currency.
	FxRateToBase string
	// AssetCategory is the asset category (usually "CASH").
	AssetCategory string
	// Symbol is the transfer symbol (usually "--").
	Symbol string
	// Description is the human-readable description.
	Description string
	// ReportDate is the date the transfer appears in the report (YYYY-MM-DD).
	ReportDate string
	// Date is the transfer date (YYYY-MM-DD).
	Date string
	// DateTime is the transfer date and time (YYYY-MM-DD;HHMMSS).
	DateTime string
	// SettleDate is the settlement date (YYYY-MM-DD).
	SettleDate string
	// Type is the transfer type (e.g., "INTERNAL").
	Type string
	// Direction is the transfer direction ("IN" = deposit, "OUT" = withdrawal).
	Direction string
	// Account is the counterparty account (masked, e.g., "U****4321").
	Account string
	// CashTransfer is the cash amount transferred (positive = in, negative = out).
	CashTransfer string
	// TransactionID is the unique transaction identifier used for duplicate detection.
	TransactionID string
	// ClientReference is a client reference note.
	ClientReference string
}

// ParseXML parses an IBKR Flex Web Report XML file and extracts Trades,
// CashTransactions, and Transfers. Returns a ParsedReport with all extracted
// records and summary metadata.
//
// The XML is expected to be in the "AF" (Account Activity) report format
// with self-closing tags and attributes.
func ParseXML(data []byte) (*ParsedReport, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty XML data")
	}

	// IBKR Flex XML uses attribute names like "tradePrice" and "ibOrderID"
	// which don't follow Go's standard xml tag conventions. Preprocess the
	// XML to normalize these attribute names before unmarshaling.
	preprocessed := preprocessXML(data)

	var resp flexQueryResponse
	if err := xml.Unmarshal(preprocessed, &resp); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	if len(resp.FlexStatements.Statements) == 0 {
		return nil, fmt.Errorf("no FlexStatement found in XML")
	}

	stmt := resp.FlexStatements.Statements[0]

	return &ParsedReport{
		AccountID:        stmt.AccountID,
		AccountAlias:     stmt.AcctAlias,
		Currency:         stmt.Currency,
		FromDate:         stmt.FromDate,
		ToDate:           stmt.ToDate,
		Trades:           extractTrades(stmt.Trades),
		CashTransactions: extractCashTransactions(stmt.CashTransactions),
		Transfers:        extractTransfers(stmt.Transfers),
	}, nil
}

// preprocessXML normalizes attribute names that don't follow Go's xml tag
// conventions. IBKR uses camelCase attributes like "tradePrice" and "ibOrderID"
// which we map to our struct field names.
func preprocessXML(data []byte) []byte {
	s := string(data)
	s = strings.ReplaceAll(s, `tradePrice=`, `trade_price=`)
	s = strings.ReplaceAll(s, `ibOrderID=`, `ib_order_id=`)
	s = strings.ReplaceAll(s, `ibCommission=`, `ib_commission=`)
	s = strings.ReplaceAll(s, `ibCommissionCurrency=`, `ib_commission_currency=`)
	s = strings.ReplaceAll(s, `ibExecID=`, `ib_exec_id=`)
	return []byte(s)
}

// ---- XML struct definitions for unmarshaling ----

type flexQueryResponse struct {
	FlexStatements flexStatements `xml:"FlexStatements"`
}

type flexStatements struct {
	Statements []flexStatement `xml:"FlexStatement"`
}

type flexStatement struct {
	XMLName          xml.Name `xml:"FlexStatement"`
	AccountID        string   `xml:"accountId,attr"`
	AcctAlias        string   `xml:"acctAlias,attr"`
	Currency         string   `xml:"currency,attr"`
	FromDate         string   `xml:"fromDate,attr"`
	ToDate           string   `xml:"toDate,attr"`
	Period           string   `xml:"period,attr"`
	WhenGenerated    string   `xml:"whenGenerated,attr"`
	AccountInfo      accountInfo
	Trades           trades
	CashTransactions cashTransactions
	Transfers        transfers
}

type accountInfo struct {
	XMLName     xml.Name `xml:"AccountInformation"`
	AccountID   string   `xml:"accountId,attr"`
	AcctAlias   string   `xml:"acctAlias,attr"`
	Currency    string   `xml:"currency,attr"`
}

type trades struct {
	XMLName xml.Name   `xml:"Trades"`
	Trades  []tradeXML `xml:"Trade"`
}

type tradeXML struct {
	XMLName              xml.Name `xml:"Trade"`
	AccountID            string   `xml:"accountId,attr"`
	AcctAlias            string   `xml:"acctAlias,attr"`
	Model                string   `xml:"model,attr"`
	Currency             string   `xml:"currency,attr"`
	FxRateToBase         string   `xml:"fxRateToBase,attr"`
	AssetCategory        string   `xml:"assetCategory,attr"`
	SubCategory          string   `xml:"subCategory,attr"`
	Symbol               string   `xml:"symbol,attr"`
	Description          string   `xml:"description,attr"`
	Conid                string   `xml:"conid,attr"`
	SecurityID           string   `xml:"securityID,attr"`
	SecurityIDType       string   `xml:"securityIDType,attr"`
	Cusip                string   `xml:"cusip,attr"`
	Isin                 string   `xml:"isin,attr"`
	Figi                 string   `xml:"figi,attr"`
	ListingExchange      string   `xml:"listingExchange,attr"`
	UnderlyingConid      string   `xml:"underlyingConid,attr"`
	UnderlyingSymbol     string   `xml:"underlyingSymbol,attr"`
	UnderlyingSecurityID string   `xml:"underlyingSecurityID,attr"`
	UnderlyingListExch   string   `xml:"underlyingListingExchange,attr"`
	Issuer               string   `xml:"issuer,attr"`
	IssuerCountryCode    string   `xml:"issuerCountryCode,attr"`
	TradeID              string   `xml:"tradeID,attr"`
	Multiplier           string   `xml:"multiplier,attr"`
	RelatedTradeID       string   `xml:"relatedTradeID,attr"`
	Strike               string   `xml:"strike,attr"`
	ReportDate           string   `xml:"reportDate,attr"`
	Expiry               string   `xml:"expiry,attr"`
	DateTime             string   `xml:"dateTime,attr"`
	PutCall              string   `xml:"putCall,attr"`
	TradeDate            string   `xml:"tradeDate,attr"`
	PrincipalAdjustFactor string  `xml:"principalAdjustFactor,attr"`
	SettleDateTarget     string   `xml:"settleDateTarget,attr"`
	TransactionType      string   `xml:"transactionType,attr"`
	Exchange             string   `xml:"exchange,attr"`
	Quantity             string   `xml:"quantity,attr"`
	TradePrice           string   `xml:"trade_price,attr"`
	TradeMoney           string   `xml:"tradeMoney,attr"`
	Proceeds             string   `xml:"proceeds,attr"`
	Taxes                string   `xml:"taxes,attr"`
	IbCommission         string   `xml:"ib_commission,attr"`
	IbCommissionCurrency string   `xml:"ib_commission_currency,attr"`
	NetCash              string   `xml:"netCash,attr"`
	ClosePrice           string   `xml:"closePrice,attr"`
	OpenCloseIndicator   string   `xml:"openCloseIndicator,attr"`
	Notes                string   `xml:"notes,attr"`
	Cost                 string   `xml:"cost,attr"`
	FifoPnlRealized      string   `xml:"fifoPnlRealized,attr"`
	MtMpnL               string   `xml:"mtmPnl,attr"`
	OrigTradePrice       string   `xml:"origTradePrice,attr"`
	OrigTradeDate        string   `xml:"origTradeDate,attr"`
	OrigTradeID          string   `xml:"origTradeID,attr"`
	OrigOrderID          string   `xml:"origOrderID,attr"`
	OrigTransactionID    string   `xml:"origTransactionID,attr"`
	BuySell              string   `xml:"buySell,attr"`
	ClearingFirmID       string   `xml:"clearingFirmID,attr"`
	IbOrderID            string   `xml:"ib_order_id,attr"`
	TransactionID        string   `xml:"transactionID,attr"`
	IbExecID             string   `xml:"ib_exec_id,attr"`
	RelatedTransactionID string   `xml:"relatedTransactionID,attr"`
	Rtn                  string   `xml:"rtn,attr"`
	BrokerageOrderID     string   `xml:"brokerageOrderID,attr"`
	OrderReference       string   `xml:"orderReference,attr"`
	VolatilityOrderLink  string   `xml:"volatilityOrderLink,attr"`
	ExchOrderId          string   `xml:"exchOrderId,attr"`
	ExtExecID            string   `xml:"extExecID,attr"`
	OrderTime            string   `xml:"orderTime,attr"`
	OpenDateTime         string   `xml:"openDateTime,attr"`
	HoldingPeriodDateTime string  `xml:"holdingPeriodDateTime,attr"`
	WhenRealized         string   `xml:"whenRealized,attr"`
	WhenReopened         string   `xml:"whenReopened,attr"`
	LevelOfDetail        string   `xml:"levelOfDetail,attr"`
	ChangeInPrice        string   `xml:"changeInPrice,attr"`
	ChangeInQuantity     string   `xml:"changeInQuantity,attr"`
	OrderType            string   `xml:"orderType,attr"`
	TraderID             string   `xml:"traderID,attr"`
	IsAPIOrder           string   `xml:"isAPIOrder,attr"`
	AccruedInt           string   `xml:"accruedInt,attr"`
	InitialInvestment    string   `xml:"initialInvestment,attr"`
	PositionActionID     string   `xml:"positionActionID,attr"`
	SerialNumber         string   `xml:"serialNumber,attr"`
	DeliveryType         string   `xml:"deliveryType,attr"`
	CommodityType        string   `xml:"commodityType,attr"`
	Fineness             string   `xml:"fineness,attr"`
	Weight               string   `xml:"weight,attr"`
}

type cashTransactions struct {
	XMLName      xml.Name             `xml:"CashTransactions"`
	Transactions []cashTransactionXML `xml:"CashTransaction"`
}

type cashTransactionXML struct {
	XMLName              xml.Name `xml:"CashTransaction"`
	AccountID            string   `xml:"accountId,attr"`
	AcctAlias            string   `xml:"acctAlias,attr"`
	Model                string   `xml:"model,attr"`
	Currency             string   `xml:"currency,attr"`
	FxRateToBase         string   `xml:"fxRateToBase,attr"`
	AssetCategory        string   `xml:"assetCategory,attr"`
	SubCategory          string   `xml:"subCategory,attr"`
	Symbol               string   `xml:"symbol,attr"`
	Description          string   `xml:"description,attr"`
	Conid                string   `xml:"conid,attr"`
	SecurityID           string   `xml:"securityID,attr"`
	SecurityIDType       string   `xml:"securityIDType,attr"`
	Cusip                string   `xml:"cusip,attr"`
	Isin                 string   `xml:"isin,attr"`
	Figi                 string   `xml:"figi,attr"`
	ListingExchange      string   `xml:"listingExchange,attr"`
	UnderlyingConid      string   `xml:"underlyingConid,attr"`
	UnderlyingSymbol     string   `xml:"underlyingSymbol,attr"`
	UnderlyingSecurityID string   `xml:"underlyingSecurityID,attr"`
	UnderlyingListExch   string   `xml:"underlyingListingExchange,attr"`
	Issuer               string   `xml:"issuer,attr"`
	IssuerCountryCode    string   `xml:"issuerCountryCode,attr"`
	Multiplier           string   `xml:"multiplier,attr"`
	Strike               string   `xml:"strike,attr"`
	Expiry               string   `xml:"expiry,attr"`
	PutCall              string   `xml:"putCall,attr"`
	PrincipalAdjustFactor string  `xml:"principalAdjustFactor,attr"`
	DateTime             string   `xml:"dateTime,attr"`
	SettleDate           string   `xml:"settleDate,attr"`
	AvailableForTradingDate string `xml:"availableForTradingDate,attr"`
	Amount               string   `xml:"amount,attr"`
	Type                 string   `xml:"type,attr"`
	TradeID              string   `xml:"tradeID,attr"`
	Code                 string   `xml:"code,attr"`
	TransactionID        string   `xml:"transactionID,attr"`
	ReportDate           string   `xml:"reportDate,attr"`
	ExDate               string   `xml:"exDate,attr"`
	ClientReference      string   `xml:"clientReference,attr"`
	ActionID             string   `xml:"actionID,attr"`
	LevelOfDetail        string   `xml:"levelOfDetail,attr"`
	SerialNumber         string   `xml:"serialNumber,attr"`
	DeliveryType         string   `xml:"deliveryType,attr"`
	CommodityType        string   `xml:"commodityType,attr"`
	Fineness             string   `xml:"fineness,attr"`
	Weight               string   `xml:"weight,attr"`
}

type transfers struct {
	XMLName   xml.Name      `xml:"Transfers"`
	Transfers []transferXML `xml:"Transfer"`
}

type transferXML struct {
	XMLName             xml.Name `xml:"Transfer"`
	AccountID           string   `xml:"accountId,attr"`
	AcctAlias           string   `xml:"acctAlias,attr"`
	Model               string   `xml:"model,attr"`
	Currency            string   `xml:"currency,attr"`
	FxRateToBase        string   `xml:"fxRateToBase,attr"`
	AssetCategory       string   `xml:"assetCategory,attr"`
	SubCategory         string   `xml:"subCategory,attr"`
	Symbol              string   `xml:"symbol,attr"`
	Description         string   `xml:"description,attr"`
	Conid               string   `xml:"conid,attr"`
	SecurityID          string   `xml:"securityID,attr"`
	SecurityIDType      string   `xml:"securityIDType,attr"`
	Cusip               string   `xml:"cusip,attr"`
	Isin                string   `xml:"isin,attr"`
	Figi                string   `xml:"figi,attr"`
	ListingExchange     string   `xml:"listingExchange,attr"`
	UnderlyingConid     string   `xml:"underlyingConid,attr"`
	UnderlyingSymbol    string   `xml:"underlyingSymbol,attr"`
	UnderlyingSecurityID string  `xml:"underlyingSecurityID,attr"`
	UnderlyingListExch  string   `xml:"underlyingListingExchange,attr"`
	Issuer              string   `xml:"issuer,attr"`
	IssuerCountryCode   string   `xml:"issuerCountryCode,attr"`
	Multiplier          string   `xml:"multiplier,attr"`
	Strike              string   `xml:"strike,attr"`
	Expiry              string   `xml:"expiry,attr"`
	PutCall             string   `xml:"putCall,attr"`
	PrincipalAdjustFactor string `xml:"principalAdjustFactor,attr"`
	ReportDate          string   `xml:"reportDate,attr"`
	Date                string   `xml:"date,attr"`
	DateTime            string   `xml:"dateTime,attr"`
	SettleDate          string   `xml:"settleDate,attr"`
	Type                string   `xml:"type,attr"`
	Direction           string   `xml:"direction,attr"`
	Company             string   `xml:"company,attr"`
	Account             string   `xml:"account,attr"`
	AccountName         string   `xml:"accountName,attr"`
	DeliveringBroker    string   `xml:"deliveringBroker,attr"`
	Quantity            string   `xml:"quantity,attr"`
	TransferPrice       string   `xml:"transferPrice,attr"`
	PositionAmount      string   `xml:"positionAmount,attr"`
	PositionAmountInBase string  `xml:"positionAmountInBase,attr"`
	PnlAmount           string   `xml:"pnlAmount,attr"`
	PnlAmountInBase     string   `xml:"pnlAmountInBase,attr"`
	CashTransfer        string   `xml:"cashTransfer,attr"`
	Code                string   `xml:"code,attr"`
	ClientReference     string   `xml:"clientReference,attr"`
	TransactionID       string   `xml:"transactionID,attr"`
	LevelOfDetail       string   `xml:"levelOfDetail,attr"`
	PositionInstructionID string  `xml:"positionInstructionID,attr"`
	PositionInstructionSetID string `xml:"positionInstructionSetID,attr"`
	SerialNumber        string   `xml:"serialNumber,attr"`
	DeliveryType        string   `xml:"deliveryType,attr"`
	CommodityType       string   `xml:"commodityType,attr"`
	Fineness            string   `xml:"fineness,attr"`
	Weight              string   `xml:"weight,attr"`
}

// ---- Extraction helpers ----

func extractTrades(container trades) []Trade {
	if len(container.Trades) == 0 {
		return []Trade{}
	}
	out := make([]Trade, 0, len(container.Trades))
	for _, t := range container.Trades {
		out = append(out, Trade{
			AccountID:            t.AccountID,
			AcctAlias:            t.AcctAlias,
			Currency:             t.Currency,
			FxRateToBase:         t.FxRateToBase,
			AssetCategory:        t.AssetCategory,
			SubCategory:          t.SubCategory,
			Symbol:               t.Symbol,
			Description:          t.Description,
			Conid:                t.Conid,
			Isin:                 t.Isin,
			TradeID:              t.TradeID,
			Multiplier:           t.Multiplier,
			ReportDate:           t.ReportDate,
			DateTime:             t.DateTime,
			TradeDate:            t.TradeDate,
			SettleDateTarget:     t.SettleDateTarget,
			TransactionType:      t.TransactionType,
			Exchange:             t.Exchange,
			Quantity:             t.Quantity,
			TradePrice:           t.TradePrice,
			TradeMoney:           t.TradeMoney,
			Proceeds:             t.Proceeds,
			Taxes:                t.Taxes,
			IbCommission:         t.IbCommission,
			IbCommissionCurrency: t.IbCommissionCurrency,
			NetCash:              t.NetCash,
			ClosePrice:           t.ClosePrice,
			BuySell:              t.BuySell,
			IbOrderID:            t.IbOrderID,
			TransactionID:        t.TransactionID,
			IbExecID:             t.IbExecID,
			Cost:                 t.Cost,
			FifoPnlRealized:      t.FifoPnlRealized,
			MtMpnL:               t.MtMpnL,
			Notes:                t.Notes,
		})
	}
	return out
}

func extractCashTransactions(container cashTransactions) []CashTransaction {
	if len(container.Transactions) == 0 {
		return []CashTransaction{}
	}
	out := make([]CashTransaction, 0, len(container.Transactions))
	for _, ct := range container.Transactions {
		out = append(out, CashTransaction{
			AccountID:     ct.AccountID,
			AcctAlias:     ct.AcctAlias,
			Currency:      ct.Currency,
			FxRateToBase:  ct.FxRateToBase,
			AssetCategory: ct.AssetCategory,
			SubCategory:   ct.SubCategory,
			Symbol:        ct.Symbol,
			Description:   ct.Description,
			Conid:         ct.Conid,
			Isin:          ct.Isin,
			DateTime:      ct.DateTime,
			SettleDate:    ct.SettleDate,
			Amount:        ct.Amount,
			Type:          ct.Type,
			TradeID:       ct.TradeID,
			TransactionID: ct.TransactionID,
			ReportDate:    ct.ReportDate,
			ExDate:        ct.ExDate,
		})
	}
	return out
}

func extractTransfers(container transfers) []Transfer {
	if len(container.Transfers) == 0 {
		return []Transfer{}
	}
	out := make([]Transfer, 0, len(container.Transfers))
	for _, tr := range container.Transfers {
		out = append(out, Transfer{
			AccountID:       tr.AccountID,
			AcctAlias:       tr.AcctAlias,
			Currency:        tr.Currency,
			FxRateToBase:    tr.FxRateToBase,
			AssetCategory:   tr.AssetCategory,
			Symbol:          tr.Symbol,
			Description:     tr.Description,
			ReportDate:      tr.ReportDate,
			Date:            tr.Date,
			DateTime:        tr.DateTime,
			SettleDate:      tr.SettleDate,
			Type:            tr.Type,
			Direction:       tr.Direction,
			Account:         tr.Account,
			CashTransfer:    tr.CashTransfer,
			TransactionID:   tr.TransactionID,
			ClientReference: tr.ClientReference,
		})
	}
	return out
}
