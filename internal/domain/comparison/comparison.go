package comparison

// Predefined maps benchmark ticker symbols to their display names.
// These are the five pre-defined benchmarks available for portfolio comparison.
var Predefined = map[string]string{
	"^GSPC":  "S&P 500",
	"^IXIC":  "NASDAQ Composite",
	"VWRP.L": "Vanguard FTSE All-World UCITS",
	"VUSA.L": "Vanguard S&P 500 UCITS",
	"XNAQ.L": "iShares NASDAQ 100 UCITS",
}

// IsValidPredefined checks if the given ticker is one of the predefined benchmarks.
func IsValidPredefined(ticker string) bool {
	_, ok := Predefined[ticker]
	return ok
}

// GetPredefined returns a copy of the predefined benchmark map for safe iteration.
func GetPredefined() map[string]string {
	result := make(map[string]string, len(Predefined))
	for k, v := range Predefined {
		result[k] = v
	}
	return result
}
