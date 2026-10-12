package comparison

import "errors"

// Sentinel errors returned by the comparison service that API handlers map
// to specific HTTP status codes via errors.Is.
var (
	// ErrInvalidStressedPortfolio indicates that the stressed model
	// portfolio required by the asymmetric strategy is missing or does not
	// hold exactly the same component set as the normal portfolio. Handlers
	// map this to a 400 INVALID_STRESSED_PORTFOLIO response.
	ErrInvalidStressedPortfolio = errors.New("invalid stressed portfolio")
)
