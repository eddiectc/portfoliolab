package brokerimport

// PreviewResponse is returned by the Preview method and categorizes
// parsed records into importable, skipped, and errored groups.
type PreviewResponse struct {
	Importable      []PreviewTransaction `json:"importable"`
	Skipped         []SkippedTransaction `json:"skipped"`
	Errored         []ErroredTransaction `json:"errored"`
	ImportableCount int                  `json:"importable_count"`
	SkippedCount    int                  `json:"skipped_count"`
	ErroredCount    int                  `json:"errored_count"`
}

// PreviewTransaction holds the display fields for a transaction
// that can be imported.
type PreviewTransaction struct {
	Date              string `json:"date"`
	Type              string `json:"type"`
	Symbol            string `json:"symbol"`
	Quantity          string `json:"quantity"`
	Price             string `json:"price"`
	Currency          string `json:"currency"`
	NetCash           string `json:"net_cash"`
	ExternalReference string `json:"external_reference"`
	Description       string `json:"description"`
}

// SkippedTransaction holds the reference and reason for a skipped record.
// BrokerSymbol is populated when the reason is an unmapped symbol,
// indicating which broker symbol could not be resolved.
// Display fields (Date, Type, Symbol, etc.) mirror PreviewTransaction
// so skipped rows can be shown in the same table as importable ones.
type SkippedTransaction struct {
	ExternalReference string `json:"external_reference"`
	Reason            string `json:"reason"`
	BrokerSymbol      string `json:"broker_symbol,omitempty"`
	// Display fields (populated for consistent table rendering)
	Date        string `json:"date"`
	Type        string `json:"type"`
	Symbol      string `json:"symbol"`
	Quantity    string `json:"quantity"`
	Price       string `json:"price"`
	Currency    string `json:"currency"`
	NetCash     string `json:"net_cash"`
	Description string `json:"description"`
}

// ErroredTransaction holds the reference and error message for a failed record.
type ErroredTransaction struct {
	ExternalReference string `json:"external_reference"`
	ErrorMessage      string `json:"error_message"`
}

// ImportResult summarizes the outcome of a confirmed import.
type ImportResult struct {
	CreatedCount int      `json:"created_count"`
	SkippedCount int      `json:"skipped_count"`
	Errors       []string `json:"errors,omitempty"`
}
