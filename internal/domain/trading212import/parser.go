package trading212import

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/govalues/decimal"
)

// Action constants for Trading 212 transaction types.
const (
	ActionLimitBuy       = "limit_buy"
	ActionMarketBuy      = "market_buy"
	ActionLimitSell      = "limit_sell"
	ActionMarketSell     = "market_sell"
	ActionDeposit        = "deposit"
	ActionWithdrawal     = "withdrawal"
	ActionInterestOnCash = "interest_on_cash"
	ActionUnknown        = "unknown"
)

// ParsedRow represents a single record extracted from a Trading 212 CSV.
// Price and Currency are already converted from GBX to GBP if applicable.
type ParsedRow struct {
	// Action is the classified transaction type (e.g. "limit_buy", "deposit").
	Action string
	// Date is the transaction date extracted from the Time column (YYYY-MM-DD).
	Date string
	// Ticker is the broker ticker symbol (empty for cash transactions).
	Ticker string
	// Name is the human-readable security name.
	Name string
	// ID is the unique transaction identifier used for duplicate detection.
	ID string
	// Quantity is the number of shares (as a string, parsed by service layer).
	Quantity string
	// Price is the price per share in GBP (GBX already converted, as string).
	Price string
	// Currency is the price currency (GBP after GBX conversion, as string).
	Currency string
	// Total is the total amount (as a string, parsed by service layer).
	Total string
	// TotalCurrency is the total amount currency (as string).
	TotalCurrency string
}

// ParsedReport is the result of parsing a Trading 212 CSV file.
// It contains all extracted rows.
type ParsedReport struct {
	Rows []ParsedRow
}

// expectedHeaders are the column headers expected in a Trading 212 CSV export.
var expectedHeaders = []string{
	"Action",
	"Time",
	"ISIN",
	"Ticker",
	"Name",
	"Notes",
	"ID",
	"No. of shares",
	"Price / share",
	"Currency (Price / share)",
	"Exchange rate",
	"Total",
	"Currency (Total)",
}

// ParseCSV parses a Trading 212 CSV export and extracts structured records.
// It validates the header row, classifies actions, converts GBX prices to GBP,
// and returns a ParsedReport with all rows.
func ParseCSV(data []byte) (*ParsedReport, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty CSV data")
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1 // allow variable field count (cash rows have empty fields)
	reader.LazyQuotes = true

	// Read header row.
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	if len(header) == 0 {
		return nil, fmt.Errorf("empty header row")
	}

	// Validate header columns.
	headerMap, err := validateHeader(header)
	if err != nil {
		return nil, err
	}

	var rows []ParsedRow

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row: %w", err)
		}

		rows = append(rows, parseRow(record, headerMap))
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("no data rows found in CSV")
	}

	return &ParsedReport{
		Rows: rows,
	}, nil
}

// validateHeader checks that the CSV header contains all expected columns and
// returns a map of column name to index.
func validateHeader(header []string) (map[string]int, error) {
	headerMap := make(map[string]int, len(header))
	for i, h := range header {
		headerMap[strings.TrimSpace(h)] = i
	}

	missing := []string{}
	for _, expected := range expectedHeaders {
		if _, ok := headerMap[expected]; !ok {
			missing = append(missing, expected)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing columns: %s", strings.Join(missing, ", "))
	}

	return headerMap, nil
}

// parseRow extracts fields from a CSV record using the header index map.
func parseRow(record []string, headerMap map[string]int) ParsedRow {
	get := func(col string) string {
		if idx, ok := headerMap[col]; ok && idx < len(record) {
			return strings.TrimSpace(record[idx])
		}
		return ""
	}

	action := classifyAction(get("Action"))

	// Parse date from Time column (format: "2006-01-02 15:04:05").
	date := ""
	if timeStr := get("Time"); timeStr != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
			date = t.Format("2006-01-02")
		} else if t, err := time.Parse("2006-01-02", timeStr); err == nil {
			date = t.Format("2006-01-02")
		}
	}

	// Parse price and handle GBX→GBP conversion.
	priceStr := get("Price / share")
	currencyStr := get("Currency (Price / share)")
	price, currency := convertPrice(priceStr, currencyStr)

	return ParsedRow{
		Action:        action,
		Date:          date,
		Ticker:        get("Ticker"),
		Name:          get("Name"),
		ID:            get("ID"),
		Quantity:      get("No. of shares"),
		Price:         price,
		Currency:      currency,
		Total:         get("Total"),
		TotalCurrency: get("Currency (Total)"),
	}
}

// convertPrice converts GBX prices to GBP by dividing by 100.
// If the currency is not GBX, the price and currency are returned unchanged.
// Returns the price as a string and the (possibly converted) currency.
func convertPrice(priceStr, currencyStr string) (string, string) {
	if priceStr == "" {
		return priceStr, currencyStr
	}

	if strings.EqualFold(currencyStr, "GBX") {
		price, err := decimal.Parse(priceStr)
		if err != nil {
			// If parsing fails, return original values.
			return priceStr, currencyStr
		}
		divisor := decimal.MustNew(100, 0)
		converted, err := price.Quo(divisor)
		if err != nil {
			return priceStr, currencyStr
		}
		// Round to 2 decimal places for GBP.
		converted = converted.Round(2)
		return converted.String(), "GBP"
	}

	return priceStr, currencyStr
}

// classifyAction maps a raw Trading 212 action string to a standardized action constant.
func classifyAction(raw string) string {
	switch strings.TrimSpace(raw) {
	case "Limit buy":
		return ActionLimitBuy
	case "Market buy":
		return ActionMarketBuy
	case "Limit sell":
		return ActionLimitSell
	case "Market sell":
		return ActionMarketSell
	case "Deposit":
		return ActionDeposit
	case "Withdrawal":
		return ActionWithdrawal
	case "Interest on cash":
		return ActionInterestOnCash
	default:
		return ActionUnknown
	}
}
