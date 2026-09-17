// Package marketdata defines user-set market data overrides: historical price
// corrections that win over provider rows on read while the provider rows
// themselves stay untouched.
package marketdata

import (
	"errors"
	"time"

	"github.com/govalues/decimal"
)

// DataType identifies the kind of market data an override corrects.
type DataType string

const (
	DataTypeStock DataType = "stock"
	DataTypeFx    DataType = "fx"
	DataTypeNav   DataType = "nav"
)

// Valid reports whether dt is one of the supported data types.
func (dt DataType) Valid() bool {
	switch dt {
	case DataTypeStock, DataTypeFx, DataTypeNav:
		return true
	default:
		return false
	}
}

// Source is the constant source identifier for override rows.
const Source = "override"

// Override is a user-set correction for one historical market data point,
// keyed by (Symbol, Date, DataType). It takes precedence over provider rows
// inside historical reads; provider rows are never deleted or modified.
type Override struct {
	Symbol    string          `json:"symbol"`
	Date      string          `json:"date"` // YYYY-MM-DD
	DataType  DataType        `json:"data_type"`
	Price     decimal.Decimal `json:"price"`
	Currency  string          `json:"currency"`
	Source    string          `json:"source"` // always "override"
	CreatedAt time.Time       `json:"-"`
	UpdatedAt time.Time       `json:"-"`
}

// SetRequest is the DTO for setting (creating or replacing) an override.
// Date is accepted as a string (YYYY-MM-DD) and validated in the service layer.
type SetRequest struct {
	Symbol   string          `json:"symbol"`
	Date     string          `json:"date"`
	DataType DataType        `json:"data_type"`
	Price    decimal.Decimal `json:"price"`
	Currency string          `json:"currency"`
}

// ErrNotFound indicates that no override exists for the requested
// (symbol, date, data_type) key.
var ErrNotFound = errors.New("override not found")
