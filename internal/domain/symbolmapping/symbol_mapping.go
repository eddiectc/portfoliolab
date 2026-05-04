package symbolmapping

import "time"

// BrokerSymbol represents a broker-specific symbol mapped to an internal symbol.
type BrokerSymbol struct {
	ID           int64     `json:"id"`
	SymbolID     int64     `json:"symbol_mapping_id"`
	BrokerName   string    `json:"broker_name"`
	BrokerSymbol string    `json:"broker_symbol"`
	CreatedAt    time.Time `json:"created_at"`
}

// SymbolMapping maps an internal symbol to a market data provider symbol,
// with optional broker-specific symbol associations.
type SymbolMapping struct {
	ID               int64          `json:"id"`
	InternalSymbol   string         `json:"internal_symbol"`
	MarketDataSymbol string         `json:"market_data_symbol"`
	BrokerSymbols    []BrokerSymbol `json:"broker_symbols"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// CreateRequest is the DTO for creating a symbol mapping.
type CreateRequest struct {
	InternalSymbol   string                `json:"internal_symbol"`
	MarketDataSymbol string                `json:"market_data_symbol"`
	BrokerSymbols    []BrokerSymbolRequest `json:"broker_symbols,omitempty"`
}

// BrokerSymbolRequest is the DTO for adding a broker symbol.
type BrokerSymbolRequest struct {
	BrokerName   string `json:"broker_name"`
	BrokerSymbol string `json:"broker_symbol"`
}

// UpdateRequest is the DTO for updating a symbol mapping.
type UpdateRequest struct {
	InternalSymbol   *string `json:"internal_symbol,omitempty"`
	MarketDataSymbol *string `json:"market_data_symbol,omitempty"`
}
