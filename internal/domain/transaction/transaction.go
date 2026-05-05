package transaction

import (
	"time"

	"github.com/govalues/decimal"
)

// Transaction represents a single investment event (buy, sell, deposit, etc.)
// belonging to an account. It is the source of truth for all portfolio analytics.
type Transaction struct {
	ID                int64            `json:"id"`
	AccountID         int64            `json:"account_id"`
	Date              time.Time        `json:"date"`
	Type              string           `json:"type"`
	Symbol            string           `json:"symbol"`
	Quantity          decimal.Decimal  `json:"quantity"`
	Price             decimal.Decimal  `json:"price"`
	Currency          string           `json:"currency"`
	NetCash           decimal.Decimal  `json:"net_cash"`
	ExternalSystem    *string          `json:"external_system,omitempty"`
	ExternalReference *string          `json:"external_reference,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

// CreateRequest is the DTO for creating a transaction.
// Date is accepted as a string (YYYY-MM-DD) and parsed to time.Time in the service layer.
type CreateRequest struct {
	AccountID         int64            `json:"account_id"`
	Date              string           `json:"date"`
	Type              string           `json:"type"`
	Symbol            string           `json:"symbol"`
	Quantity          decimal.Decimal  `json:"quantity"`
	Price             decimal.Decimal  `json:"price"`
	Currency          string           `json:"currency"`
	NetCash           decimal.Decimal  `json:"net_cash"`
	ExternalSystem    *string          `json:"external_system,omitempty"`
	ExternalReference *string          `json:"external_reference,omitempty"`
}

// OptionalDecimal distinguishes between "field not sent" (IsSet=false) and
// "field sent with a value" (IsSet=true). It is used for UpdateRequest fields
// where the caller must explicitly provide a value (null/zero is rejected).
type OptionalDecimal struct {
	Dec   decimal.Decimal
	IsSet bool
}

// UnmarshalJSON implements json.Unmarshaler for OptionalDecimal.
// It tracks whether the field was present in the JSON to distinguish
// "omitted" from "explicitly null".
func (o *OptionalDecimal) UnmarshalJSON(data []byte) error {
	// First check if the raw value is JSON null.
	trimmed := []byte(data)
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t' || trimmed[0] == '\n' || trimmed[0] == '\r') {
		trimmed = trimmed[1:]
	}
	if string(trimmed) == "null" {
		o.IsSet = true
		o.Dec = decimal.Zero
		return nil
	}
	// Field is present with a non-null value.
	o.IsSet = true
	return o.Dec.UnmarshalJSON(data)
}

// MarshalJSON implements json.Marshaler for OptionalDecimal.
func (o OptionalDecimal) MarshalJSON() ([]byte, error) {
	if !o.IsSet {
		return []byte("null"), nil
	}
	return o.Dec.MarshalJSON()
}

// Get returns the decimal pointer if the field was set, nil otherwise.
func (o *OptionalDecimal) Get() *decimal.Decimal {
	if !o.IsSet {
		return nil
	}
	return &o.Dec
}

// UpdateRequest is the DTO for updating a transaction.
// All fields are pointers — only non-nil fields are applied.
// Date is accepted as a string (YYYY-MM-DD) and parsed to time.Time in the service layer.
type UpdateRequest struct {
	Date              *string          `json:"date,omitempty"`
	Type              *string          `json:"type,omitempty"`
	Symbol            *string          `json:"symbol,omitempty"`
	Quantity          *decimal.Decimal `json:"quantity,omitempty"`
	Price             *decimal.Decimal `json:"price,omitempty"`
	Currency          *string          `json:"currency,omitempty"`
	NetCash           OptionalDecimal  `json:"net_cash"`
	ExternalSystem    *string          `json:"external_system,omitempty"`
	ExternalReference *string          `json:"external_reference,omitempty"`
}

// ListFilters holds optional filter criteria for listing transactions.
// Nil fields are treated as "no filter" for that dimension.
type ListFilters struct {
	AccountID *int64
	Symbol    *string
	Type      *string
	DateFrom  *time.Time
	DateTo    *time.Time
}

// TransactionWithAccount is a Transaction with the resolved account name,
// returned by list queries that JOIN the accounts table.
type TransactionWithAccount struct {
	Transaction
	AccountName string `json:"account_name"`
}
