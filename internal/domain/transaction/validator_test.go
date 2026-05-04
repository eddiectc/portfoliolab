package transaction

import (
	"errors"
	"testing"

	"github.com/govalues/decimal"
)

// d is a shorthand for decimal.MustNew(value, scale).
// MustNew(value, scale) interprets value as an integer shifted by 10^scale,
// so d(15000, 2) == 150.00 and d(75, 2) == 0.75.
func d(value int64, scale int) decimal.Decimal {
	return decimal.MustNew(value, scale)
}

// dp returns a pointer to a decimal.
func dp(value int64, scale int) *decimal.Decimal {
	val := decimal.MustNew(value, scale)
	return &val
}

// --- ValidateCreateRequest Tests ---

func TestValidateCreateRequest_Valid(t *testing.T) {
	tests := []struct {
		name string
		req  CreateRequest
	}{
		{
			name: "buy transaction",
			req: CreateRequest{
				AccountID: 1, Date: "2025-01-15", Type: "buy",
				Symbol: "AAPL", Quantity: d(10, 0), Price: d(15000, 2),
				Currency: "USD",
			},
		},
		{
			name: "sell with negative quantity",
			req: CreateRequest{
				AccountID: 1, Date: "2025-03-20", Type: "sell",
				Symbol: "AAPL", Quantity: d(-5, 0), Price: d(17500, 2),
				Currency: "USD", NetCash: dp(87000, 2),
			},
		},
		{
			name: "deposit with cash symbol",
			req: CreateRequest{
				AccountID: 1, Date: "2025-01-01", Type: "deposit",
				Symbol: "$CASH-USD", Quantity: d(1000000, 2), Price: d(1, 0),
				Currency: "USD", NetCash: dp(1000000, 2),
			},
		},
		{
			name: "withdrawal with cash symbol",
			req: CreateRequest{
				AccountID: 1, Date: "2025-06-15", Type: "withdrawal",
				Symbol: "$CASH-USD", Quantity: d(-200000, 2), Price: d(1, 0),
				Currency: "USD", NetCash: dp(-200000, 2),
			},
		},
		{
			name: "dividend",
			req: CreateRequest{
				AccountID: 1, Date: "2025-03-14", Type: "dividend",
				Symbol: "MSFT", Quantity: d(1, 0), Price: d(300, 2),
				Currency: "USD", NetCash: dp(300, 2),
			},
		},
		{
			name: "fee",
			req: CreateRequest{
				AccountID: 1, Date: "2025-01-15", Type: "fee",
				Symbol: "$CASH-USD", Quantity: d(-495, 2), Price: d(1, 0),
				Currency: "USD", NetCash: dp(-495, 2),
			},
		},
		{
			name: "interest",
			req: CreateRequest{
				AccountID: 1, Date: "2025-06-30", Type: "interest",
				Symbol: "$CASH-USD", Quantity: d(2550, 2), Price: d(1, 0),
				Currency: "USD", NetCash: dp(2550, 2),
			},
		},
		{
			name: "tax",
			req: CreateRequest{
				AccountID: 1, Date: "2025-12-31", Type: "tax",
				Symbol: "$CASH-USD", Quantity: d(-15000, 2), Price: d(1, 0),
				Currency: "USD", NetCash: dp(-15000, 2),
			},
		},
		{
			name: "short position (negative quantity)",
			req: CreateRequest{
				AccountID: 1, Date: "2025-01-15", Type: "buy",
				Symbol: "TSLA", Quantity: d(-100, 0), Price: d(20000, 2),
				Currency: "USD",
			},
		},
		{
			name: "non-USD currency",
			req: CreateRequest{
				AccountID: 1, Date: "2025-01-15", Type: "buy",
				Symbol: "VOD.L", Quantity: d(500, 0), Price: d(75, 2),
				Currency: "GBP",
			},
		},
		{
			name: "with external reference",
			req: CreateRequest{
				AccountID: 1, Date: "2025-02-10", Type: "buy",
				Symbol: "VOO", Quantity: d(2, 0), Price: d(25000, 2),
				Currency: "USD",
				ExternalSystem:    strPtr("IBKR"),
				ExternalReference: strPtr("TXN-12345"),
			},
		},
		{
			name: "cash symbol GBP",
			req: CreateRequest{
				AccountID: 1, Date: "2025-01-01", Type: "deposit",
				Symbol: "$CASH-GBP", Quantity: d(500000, 2), Price: d(1, 0),
				Currency: "GBP",
			},
		},
		{
			name: "symbol with leading/trailing whitespace (trimmed)",
			req: CreateRequest{
				AccountID: 1, Date: "2025-01-15", Type: "buy",
				Symbol: "  AAPL  ", Quantity: d(10, 0), Price: d(15000, 2),
				Currency: "USD",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateRequest(tt.req)
			if err != nil {
				t.Errorf("expected nil, got %v", err)
			}
		})
	}
}

func TestValidateCreateRequest_InvalidDate(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Type: "buy", Symbol: "AAPL",
		Quantity: d(10, 0), Price: d(15000, 2), Currency: "USD",
	}

	tests := []struct {
		name string
		date string
	}{
		{"not a date", "not-a-date"},
		{"empty", ""},
		{"wrong format", "15/01/2025"},
		{"invalid month", "2025-13-01"},
		{"invalid day", "2025-01-32"},
		{"partial", "2025-01"},
		{"with time", "2025-01-15T10:30:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := basicReq
			req.Date = tt.date
			err := ValidateCreateRequest(req)
			if !errors.Is(err, ErrInvalidDate) {
				t.Errorf("expected ErrInvalidDate, got %v", err)
			}
		})
	}
}

func TestValidateCreateRequest_InvalidType(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Date: "2025-01-15", Symbol: "AAPL",
		Quantity: d(10, 0), Price: d(15000, 2), Currency: "USD",
	}

	tests := []struct {
		name string
		typ  string
	}{
		{"exchange", "exchange"},
		{"split", "split"},
		{"empty", ""},
		{"random", "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := basicReq
			req.Type = tt.typ
			err := ValidateCreateRequest(req)
			if !errors.Is(err, ErrInvalidType) {
				t.Errorf("expected ErrInvalidType for %q, got %v", tt.typ, err)
			}
		})
	}
	// Verify all 8 allowed types pass
	t.Run("all allowed types", func(t *testing.T) {
		for _, typ := range []string{"buy", "sell", "deposit", "withdrawal", "dividend", "interest", "fee", "tax"} {
			t.Run(typ, func(t *testing.T) {
				req := basicReq
				req.Type = typ
				err := ValidateCreateRequest(req)
				if err != nil {
					t.Errorf("expected nil for allowed type %q, got %v", typ, err)
				}
			})
		}
	})
}

func TestValidateCreateRequest_InvalidSymbol(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Date: "2025-01-15", Type: "buy",
		Quantity: d(10, 0), Price: d(15000, 2), Currency: "USD",
	}

	tests := []struct {
		name   string
		symbol string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"tabs", "\t\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := basicReq
			req.Symbol = tt.symbol
			err := ValidateCreateRequest(req)
			if !errors.Is(err, ErrInvalidSymbol) {
				t.Errorf("expected ErrInvalidSymbol, got %v", err)
			}
		})
	}
}

func TestValidateCreateRequest_InvalidQuantity(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Date: "2025-01-15", Type: "buy", Symbol: "AAPL",
		Price: d(15000, 2), Currency: "USD",
	}

	req := basicReq
	req.Quantity = decimal.Zero
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrInvalidQuantity) {
		t.Errorf("expected ErrInvalidQuantity for zero quantity, got %v", err)
	}
}

func TestValidateCreateRequest_InvalidPrice(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Date: "2025-01-15", Type: "buy", Symbol: "AAPL",
		Quantity: d(10, 0), Currency: "USD",
	}

	tests := []struct {
		name  string
		price decimal.Decimal
	}{
		{"zero", decimal.Zero},
		{"negative", d(-10, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := basicReq
			req.Price = tt.price
			err := ValidateCreateRequest(req)
			if !errors.Is(err, ErrInvalidPrice) {
				t.Errorf("expected ErrInvalidPrice for %s, got %v", tt.name, err)
			}
		})
	}
}

func TestValidateCreateRequest_InvalidCurrency(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Date: "2025-01-15", Type: "buy", Symbol: "AAPL",
		Quantity: d(10, 0), Price: d(15000, 2),
	}

	tests := []struct {
		name     string
		currency string
	}{
		{"too short", "US"},
		{"lowercase", "usd"},
		{"mixed case", "Usd"},
		{"too long", "USDX"},
		{"with numbers", "USD1"},
		{"empty", ""},
		{"special chars", "U$D"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := basicReq
			req.Currency = tt.currency
			err := ValidateCreateRequest(req)
			if !errors.Is(err, ErrInvalidCurrency) {
				t.Errorf("expected ErrInvalidCurrency for %q, got %v", tt.currency, err)
			}
		})
	}
}

func TestValidateCreateRequest_CashSymbolMismatch(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Date: "2025-01-15", Type: "deposit",
		Quantity: d(1000000, 2), Price: d(1, 0),
	}

	req := basicReq
	req.Symbol = "$CASH-USD"
	req.Currency = "GBP"
	err := ValidateCreateRequest(req)
	if !errors.Is(err, ErrInvalidCurrency) {
		t.Errorf("expected ErrInvalidCurrency for cash symbol mismatch, got %v", err)
	}
}

func TestValidateCreateRequest_ExternalFieldsTooLong(t *testing.T) {
	basicReq := CreateRequest{
		AccountID: 1, Date: "2025-01-15", Type: "buy", Symbol: "AAPL",
		Quantity: d(10, 0), Price: d(15000, 2), Currency: "USD",
	}

	// External system too long
	req := basicReq
	req.ExternalSystem = strPtr(string(make([]byte, 101)))
	err := ValidateCreateRequest(req)
	if err == nil {
		t.Error("expected error for external_system > 100 chars, got nil")
	}

	// External reference too long
	req = basicReq
	req.ExternalReference = strPtr(string(make([]byte, 101)))
	err = ValidateCreateRequest(req)
	if err == nil {
		t.Error("expected error for external_reference > 100 chars, got nil")
	}

	// Exactly 100 chars — should pass
	req = basicReq
	req.ExternalSystem = strPtr(string(make([]byte, 100)))
	err = ValidateCreateRequest(req)
	if err != nil {
		t.Errorf("expected nil for 100-char external_system, got %v", err)
	}
}

// --- ValidateUpdateRequest Tests ---

func TestValidateUpdateRequest_Valid(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateRequest
	}{
		{
			name: "update date only",
			req: UpdateRequest{
				Date: strPtr("2025-01-16"),
			},
		},
		{
			name: "update quantity and price",
			req: UpdateRequest{
				Quantity: dp(12, 0),
				Price:    dp(14850, 2),
			},
		},
		{
			name: "update netCash",
			req: UpdateRequest{
				NetCash: dp(-151000, 2),
			},
		},
		{
			name: "update type",
			req: UpdateRequest{
				Type: strPtr("sell"),
			},
		},
		{
			name: "update symbol",
			req: UpdateRequest{
				Symbol: strPtr("AAPL.WS"),
			},
		},
		{
			name: "update currency",
			req: UpdateRequest{
				Currency: strPtr("EUR"),
			},
		},
		{
			name: "no changes (empty update)",
			req:  UpdateRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateRequest(tt.req)
			if err != nil {
				t.Errorf("expected nil, got %v", err)
			}
		})
	}
}

func TestValidateUpdateRequest_InvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		req     UpdateRequest
		wantErr error
	}{
		{
			name:    "invalid date",
			req:     UpdateRequest{Date: strPtr("not-a-date")},
			wantErr: ErrInvalidDate,
		},
		{
			name:    "invalid type",
			req:     UpdateRequest{Type: strPtr("split")},
			wantErr: ErrInvalidType,
		},
		{
			name:    "empty symbol",
			req:     UpdateRequest{Symbol: strPtr("")},
			wantErr: ErrInvalidSymbol,
		},
		{
			name:    "zero quantity",
			req:     UpdateRequest{Quantity: dp(0, 0)},
			wantErr: ErrInvalidQuantity,
		},
		{
			name:    "zero price",
			req:     UpdateRequest{Price: dp(0, 0)},
			wantErr: ErrInvalidPrice,
		},
		{
			name:    "negative price",
			req:     UpdateRequest{Price: dp(-10, 0)},
			wantErr: ErrInvalidPrice,
		},
		{
			name:    "invalid currency",
			req:     UpdateRequest{Currency: strPtr("XX")},
			wantErr: ErrInvalidCurrency,
		},
		{
			name:    "cash symbol mismatch on update",
			req:     UpdateRequest{Symbol: strPtr("$CASH-USD"), Currency: strPtr("GBP")},
			wantErr: ErrInvalidCurrency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateRequest(tt.req)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// --- Individual Validator Tests ---

func TestValidateType_Allowed(t *testing.T) {
	for _, typ := range []string{"buy", "sell", "deposit", "withdrawal", "dividend", "interest", "fee", "tax"} {
		t.Run(typ, func(t *testing.T) {
			if err := validateType(typ); err != nil {
				t.Errorf("expected nil for allowed type %q, got %v", typ, err)
			}
		})
	}
}

func TestValidateType_Disallowed(t *testing.T) {
	for _, typ := range []string{"exchange", "split", "conversion", "", "BUY", "Buy"} {
		t.Run(typ, func(t *testing.T) {
			if err := validateType(typ); err == nil {
				t.Errorf("expected error for disallowed type %q, got nil", typ)
			}
		})
	}
}

func TestValidateCurrency_Valid(t *testing.T) {
	for _, c := range []string{"USD", "EUR", "GBP", "JPY", "CHF", "CAD", "AUD"} {
		t.Run(c, func(t *testing.T) {
			if err := validateCurrency(c); err != nil {
				t.Errorf("expected nil for valid currency %q, got %v", c, err)
			}
		})
	}
}

func TestValidateCurrency_Invalid(t *testing.T) {
	for _, c := range []string{"US", "usd", "Usd", "USDX", "U$D", "", "123", "USD "} {
		t.Run(c, func(t *testing.T) {
			if err := validateCurrency(c); err == nil {
				t.Errorf("expected error for invalid currency %q, got nil", c)
			}
		})
	}
}

func TestValidateDate_Valid(t *testing.T) {
	for _, dateStr := range []string{"2025-01-15", "2025-12-31", "2000-02-29", "2100-06-15"} {
		t.Run(dateStr, func(t *testing.T) {
			if err := validateDate(dateStr); err != nil {
				t.Errorf("expected nil for valid date %q, got %v", dateStr, err)
			}
		})
	}
}

func TestValidateDate_Invalid(t *testing.T) {
	for _, dateStr := range []string{"not-a-date", "", "2025-13-01", "2025-01-32", "15/01/2025", "2025-01", "2025-01-15T10:30:00"} {
		t.Run(dateStr, func(t *testing.T) {
			if err := validateDate(dateStr); err == nil {
				t.Errorf("expected error for invalid date %q, got nil", dateStr)
			}
		})
	}
}

func TestValidateQuantity_NonZero(t *testing.T) {
	// Zero should fail
	if err := validateQuantity(decimal.Zero); err == nil {
		t.Error("expected error for zero quantity, got nil")
	}
	// Positive should pass
	if err := validateQuantity(d(10, 0)); err != nil {
		t.Errorf("expected nil for positive quantity, got %v", err)
	}
	// Negative should pass (short selling)
	if err := validateQuantity(d(-100, 0)); err != nil {
		t.Errorf("expected nil for negative quantity, got %v", err)
	}
	// Fractional should pass
	if err := validateQuantity(d(5, 2)); err != nil {
		t.Errorf("expected nil for fractional quantity, got %v", err)
	}
}

func TestValidatePrice_Positive(t *testing.T) {
	// Zero should fail
	if err := validatePrice(decimal.Zero); err == nil {
		t.Error("expected error for zero price, got nil")
	}
	// Negative should fail
	if err := validatePrice(d(-10, 0)); err == nil {
		t.Error("expected error for negative price, got nil")
	}
	// Positive should pass
	if err := validatePrice(d(15000, 2)); err != nil {
		t.Errorf("expected nil for positive price, got %v", err)
	}
	// Fractional should pass
	if err := validatePrice(d(75, 2)); err != nil {
		t.Errorf("expected nil for fractional price, got %v", err)
	}
}

func TestValidateCashSymbolMatch(t *testing.T) {
	tests := []struct {
		name     string
		symbol   string
		currency string
		wantErr  bool
	}{
		{"matching cash", "$CASH-USD", "USD", false},
		{"matching cash GBP", "$CASH-GBP", "GBP", false},
		{"mismatching cash", "$CASH-USD", "EUR", true},
		{"non-cash symbol", "AAPL", "USD", false},
		{"non-cash symbol any currency", "VOD.L", "GBP", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCashSymbolMatch(tt.symbol, tt.currency)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected nil, got %v", err)
			}
		})
	}
}

// --- Helpers ---

func strPtr(s string) *string {
	return &s
}
