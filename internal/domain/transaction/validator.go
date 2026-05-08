package transaction

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/govalues/decimal"
)

var (
	// allowedTypes is the set of valid transaction types.
	allowedTypes = map[string]bool{
		"buy":       true,
		"sell":      true,
		"deposit":   true,
		"withdrawal": true,
		"dividend":  true,
		"interest":  true,
		"fee":       true,
		"tax":       true,
	}

	// cashSymbolPrefix is the prefix for cash account symbols.
	cashSymbolPrefix = "$CASH-"

	// iso4217Regex matches 3-letter uppercase currency codes.
	iso4217Regex = regexp.MustCompile(`^[A-Z]{3}$`)

	// dateLayout is the expected date format for transaction dates.
	dateLayout = "2006-01-02"
)

// ValidateCreateRequest validates all fields of a create request.
func ValidateCreateRequest(req CreateRequest) error {
	if err := validateDate(req.Date); err != nil {
		return ErrInvalidDate
	}
	if err := validateType(req.Type); err != nil {
		return ErrInvalidType
	}
	symbol := strings.TrimSpace(req.Symbol)
	if err := validateSymbol(symbol); err != nil {
		return ErrInvalidSymbol
	}
	if err := validateQuantity(req.Quantity); err != nil {
		return ErrInvalidQuantity
	}
	if err := validatePrice(req.Price); err != nil {
		return ErrInvalidPrice
	}
	if err := validateCurrency(req.Currency); err != nil {
		return ErrInvalidCurrency
	}
	if err := validateCashSymbolMatch(symbol, req.Currency); err != nil {
		return ErrInvalidCurrency
	}
	if err := validateNetCash(req.NetCash); err != nil {
		return ErrInvalidNetCash
	}
	if err := validateExternalFields(req.ExternalSystem, req.ExternalReference); err != nil {
		return fmt.Errorf("invalid external fields")
	}
	if err := validateLotID(req.LotID); err != nil {
		return fmt.Errorf("invalid lot_id")
	}
	return nil
}

// ValidateUpdateRequest validates only the non-nil fields of an update request.
func ValidateUpdateRequest(req UpdateRequest) error {
	if req.Date != nil {
		if err := validateDate(*req.Date); err != nil {
			return ErrInvalidDate
		}
	}
	if req.Type != nil {
		if err := validateType(*req.Type); err != nil {
			return ErrInvalidType
		}
	}
	if req.Symbol != nil {
		symbol := strings.TrimSpace(*req.Symbol)
		if err := validateSymbol(symbol); err != nil {
			return ErrInvalidSymbol
		}
	}
	if req.Quantity != nil {
		if err := validateQuantity(*req.Quantity); err != nil {
			return ErrInvalidQuantity
		}
	}
	if req.Price != nil {
		if err := validatePrice(*req.Price); err != nil {
			return ErrInvalidPrice
		}
	}
	if req.Currency != nil {
		if err := validateCurrency(*req.Currency); err != nil {
			return ErrInvalidCurrency
		}
		// Also check cash symbol match if symbol is also being updated.
		if req.Symbol != nil {
			symbol := strings.TrimSpace(*req.Symbol)
			if err := validateCashSymbolMatch(symbol, *req.Currency); err != nil {
				return ErrInvalidCurrency
			}
		}
	}
	if req.NetCash.IsSet {
		if err := validateOptionalNetCash(req.NetCash); err != nil {
			return ErrInvalidNetCash
		}
	}
	if err := validateExternalFields(req.ExternalSystem, req.ExternalReference); err != nil {
		return fmt.Errorf("invalid external fields")
	}
	return nil
}

// validateNetCash checks that net cash is non-zero (required on create).
func validateNetCash(d decimal.Decimal) error {
	if d.IsZero() {
		return fmt.Errorf("net_cash is required and must be non-zero")
	}
	return nil
}

// validateOptionalNetCash checks that an explicitly-set net cash is not null/zero.
func validateOptionalNetCash(o OptionalDecimal) error {
	if !o.IsSet {
		return nil
	}
	if o.Dec.IsZero() {
		return fmt.Errorf("net_cash cannot be null or zero")
	}
	return nil
}

// validateType checks that the type is one of the allowed transaction types.
func validateType(s string) error {
	if !allowedTypes[s] {
		return fmt.Errorf("invalid transaction type %q", s)
	}
	return nil
}

// validateQuantity checks that the quantity is non-zero.
// Negative quantities are allowed (short selling).
func validateQuantity(q decimal.Decimal) error {
	if q.IsZero() {
		return fmt.Errorf("quantity must be non-zero")
	}
	return nil
}

// validatePrice checks that the price is strictly positive.
func validatePrice(p decimal.Decimal) error {
	if !p.IsPos() {
		return fmt.Errorf("price must be greater than zero")
	}
	return nil
}

// validateCurrency checks that the currency is a valid ISO 4217 3-letter uppercase code.
func validateCurrency(s string) error {
	if !iso4217Regex.MatchString(s) {
		return fmt.Errorf("invalid currency code %q", s)
	}
	return nil
}

// validateDate checks that the date string parses as YYYY-MM-DD and is a valid calendar date.
func validateDate(s string) error {
	_, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("invalid date format %q (expected YYYY-MM-DD)", s)
	}
	return nil
}

// validateSymbol checks that the symbol is non-empty after trimming whitespace.
func validateSymbol(s string) error {
	if s == "" {
		return fmt.Errorf("symbol is required")
	}
	return nil
}

// validateCashSymbolMatch checks that when using a $CASH-{currency} symbol,
// the currency portion of the symbol matches the transaction's currency field.
func validateCashSymbolMatch(symbol, currency string) error {
	if strings.HasPrefix(symbol, cashSymbolPrefix) {
		symbolCurrency := strings.TrimPrefix(symbol, cashSymbolPrefix)
		if symbolCurrency != currency {
			return fmt.Errorf("cash symbol %q does not match currency %q", symbol, currency)
		}
	}
	return nil
}

// validateExternalFields checks that external_system and external_reference are at most 100 characters.
func validateExternalFields(externalSystem, externalReference *string) error {
	if externalSystem != nil && len(*externalSystem) > 100 {
		return fmt.Errorf("external_system must be at most 100 characters")
	}
	if externalReference != nil && len(*externalReference) > 100 {
		return fmt.Errorf("external_reference must be at most 100 characters")
	}
	return nil
}

// validateLotID checks that lot_id is at most 100 characters.
// nil or empty string is allowed (means auto-generate).
func validateLotID(lotID *string) error {
	if lotID != nil && len(*lotID) > 100 {
		return fmt.Errorf("lot_id must be at most 100 characters")
	}
	return nil
}
