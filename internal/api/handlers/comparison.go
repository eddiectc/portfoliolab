package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"github.com/eddiectc/portfoliolab/internal/domain/comparison"
)

// validComparisonPeriods lists the accepted period values for error messages.
var validComparisonPeriods = []string{"1W", "1M", "3M", "1Y", "3Y", "5Y", "YTD", "All"}

// validPeriodSet is the set of accepted period filter values.
var validPeriodSet = map[string]bool{
	"1W": true, "1M": true, "3M": true, "1Y": true,
	"3Y": true, "5Y": true, "YTD": true, "All": true,
}

// validPortfolioTypes is the set of accepted portfolio type values.
var validPortfolioTypes = map[string]bool{
	string(comparison.PortTypeModel): true,
	string(comparison.PortTypeReal):  true,
}

// validPortfolioTypeNames lists the accepted portfolio type values for error messages.
var validPortfolioTypeNames = []string{
	string(comparison.PortTypeModel),
	string(comparison.PortTypeReal),
}

// validStrategies is the set of accepted allocation strategy values.
var validStrategies = map[string]bool{
	comparison.StrategyBuyAndHold:           true,
	comparison.StrategyRebalanced:           true,
	comparison.StrategyRebalancedAsymmetric: true,
}

// validStrategyNames lists the accepted allocation strategy values for error messages.
var validStrategyNames = []string{
	comparison.StrategyBuyAndHold,
	comparison.StrategyRebalanced,
	comparison.StrategyRebalancedAsymmetric,
}

// validRebalanceIntervals is the set of accepted rebalance interval values.
var validRebalanceIntervals = map[string]bool{
	comparison.IntervalMonthly:   true,
	comparison.IntervalQuarterly: true,
	comparison.IntervalAnnual:    true,
}

// validRebalanceIntervalNames lists the accepted rebalance interval values for error messages.
var validRebalanceIntervalNames = []string{
	comparison.IntervalMonthly,
	comparison.IntervalQuarterly,
	comparison.IntervalAnnual,
}

// comparisonService is the interface the handler depends on.
type comparisonService interface {
	ComputeComparison(ctx context.Context, req comparison.ComparisonRequest) (*comparison.ComparisonResult, error)
}

// ComparisonHandler handles HTTP requests for portfolio comparisons.
type ComparisonHandler struct {
	svc comparisonService
}

// NewComparisonHandler creates a new comparison HTTP handler.
func NewComparisonHandler(svc comparisonService) *ComparisonHandler {
	return &ComparisonHandler{svc: svc}
}

// RegisterRoutes mounts comparison API routes on the given router.
func (h *ComparisonHandler) RegisterRoutes(r *chi.Mux) {
	r.Get("/api/comparison", h.HandleComparison)
}

// HandleComparison handles GET /api/comparison.
// Query params: portfolio_a_id, portfolio_a_type, portfolio_b_id, portfolio_b_type,
// period, date_from, date_to, base_currency, starting_value, risk_free_rate,
// strategy, rebalance_interval, rebalance_threshold_pct,
// stressed_a_id, stressed_b_id, drawdown_trigger_pct.
func (h *ComparisonHandler) HandleComparison(w http.ResponseWriter, r *http.Request) {
	req, parseErr := parseComparisonRequest(r.URL.Query())
	if parseErr != nil {
		writeJSONError(w, http.StatusBadRequest, parseErr.Code, parseErr.Error)
		return
	}

	result, err := h.computeResult(r.Context(), req)
	if err != nil {
		if errors.Is(err, comparison.ErrInvalidStressedPortfolio) {
			writeJSONError(w, http.StatusBadRequest, "INVALID_STRESSED_PORTFOLIO", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to compute comparison")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// computeResult computes the comparison result for the given request.
// Shared between the API handler and the web handler.
func (h *ComparisonHandler) computeResult(ctx context.Context, req comparison.ComparisonRequest) (*comparison.ComparisonResult, error) {
	return h.svc.ComputeComparison(ctx, req)
}

// parseComparisonRequest extracts comparison parameters from query params
// and validates required fields. Returns an APIError for validation failures.
func parseComparisonRequest(query url.Values) (comparison.ComparisonRequest, *APIError) {
	var req comparison.ComparisonRequest

	// --- Portfolio A ---
	if v := query.Get("portfolio_a_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			req.PortfolioAID = n
		} else {
			return req, &APIError{Code: "INVALID_PORTFOLIO_A_ID", Error: "invalid portfolio_a_id: " + v}
		}
	} else {
		return req, &APIError{Code: "MISSING_PORTFOLIO_A_ID", Error: "portfolio_a_id is required"}
	}

	if v := query.Get("portfolio_a_type"); v != "" {
		if !validPortfolioTypes[v] {
			return req, &APIError{
				Code:  "INVALID_PORTFOLIO_A_TYPE",
				Error: "invalid portfolio_a_type: " + v + ", must be one of: " + strings.Join(validPortfolioTypeNames, ", "),
			}
		}
		req.PortfolioAType = comparison.PortfolioType(v)
	} else {
		return req, &APIError{Code: "MISSING_PORTFOLIO_A_TYPE", Error: "portfolio_a_type is required"}
	}

	// --- Portfolio B ---
	if v := query.Get("portfolio_b_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			req.PortfolioBID = n
		} else {
			return req, &APIError{Code: "INVALID_PORTFOLIO_B_ID", Error: "invalid portfolio_b_id: " + v}
		}
	} else {
		return req, &APIError{Code: "MISSING_PORTFOLIO_B_ID", Error: "portfolio_b_id is required"}
	}

	if v := query.Get("portfolio_b_type"); v != "" {
		if !validPortfolioTypes[v] {
			return req, &APIError{
				Code:  "INVALID_PORTFOLIO_B_TYPE",
				Error: "invalid portfolio_b_type: " + v + ", must be one of: " + strings.Join(validPortfolioTypeNames, ", "),
			}
		}
		req.PortfolioBType = comparison.PortfolioType(v)
	} else {
		return req, &APIError{Code: "MISSING_PORTFOLIO_B_TYPE", Error: "portfolio_b_type is required"}
	}

	// --- Period ---
	if v := query.Get("period"); v != "" {
		if !validPeriodSet[v] {
			return req, &APIError{
				Code:  "INVALID_PERIOD",
				Error: "invalid period: " + v + ", must be one of: " + strings.Join(validComparisonPeriods, ", "),
			}
		}
		req.Period = v
	}

	// --- Custom date range ---
	if v := query.Get("date_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			req.DateFrom = &t
		} else {
			return req, &APIError{Code: "INVALID_DATE_FROM", Error: "invalid date_from: " + v + ", must be YYYY-MM-DD"}
		}
	}

	if v := query.Get("date_to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			req.DateTo = &t
		} else {
			return req, &APIError{Code: "INVALID_DATE_TO", Error: "invalid date_to: " + v + ", must be YYYY-MM-DD"}
		}
	}

	// --- Base currency ---
	if v := query.Get("base_currency"); v != "" {
		req.BaseCurrency = v
	}

	// --- Starting value ---
	if v := query.Get("starting_value"); v != "" {
		if d, err := decimal.Parse(v); err == nil {
			req.StartingValue = d
		} else {
			return req, &APIError{Code: "INVALID_STARTING_VALUE", Error: "invalid starting_value: " + v + ", must be a positive number"}
		}
	} else {
		// Default starting value: 10000
		req.StartingValue = decimal.MustParse("10000")
	}

	// Validate starting value > 0
	if req.StartingValue.IsNeg() || req.StartingValue.Equal(decimal.Zero) {
		return req, &APIError{Code: "INVALID_STARTING_VALUE", Error: "starting_value must be greater than 0"}
	}

	// --- Risk-free rate ---
	if v := query.Get("risk_free_rate"); v != "" {
		if d, err := decimal.Parse(v); err == nil {
			req.RiskFreeRatePct = &d
		} else {
			return req, &APIError{Code: "INVALID_RISK_FREE_RATE", Error: "invalid risk_free_rate: " + v + ", must be a number"}
		}
	}

	// --- Allocation strategy (model portfolios only) ---
	if v := query.Get("strategy"); v != "" {
		if !validStrategies[v] {
			return req, &APIError{
				Code:  "INVALID_STRATEGY",
				Error: "invalid strategy: " + v + ", must be one of: " + strings.Join(validStrategyNames, ", "),
			}
		}
		req.Strategy = v
	}

	// --- Rebalance interval ---
	if v := query.Get("rebalance_interval"); v != "" {
		if !validRebalanceIntervals[v] {
			return req, &APIError{
				Code:  "INVALID_REBALANCE_INTERVAL",
				Error: "invalid rebalance_interval: " + v + ", must be one of: " + strings.Join(validRebalanceIntervalNames, ", "),
			}
		}
		req.RebalanceInterval = v
	}

	// --- Rebalance drift threshold (percentage points) ---
	if v := query.Get("rebalance_threshold_pct"); v != "" {
		d, err := decimal.Parse(v)
		if err != nil || d.IsNeg() || d.Cmp(decimal.MustParse("100")) > 0 {
			return req, &APIError{Code: "INVALID_REBALANCE_THRESHOLD", Error: "invalid rebalance_threshold_pct: " + v + ", must be a number between 0 and 100"}
		}
		req.RebalanceThresholdPct = &d
	}

	// --- Stressed model portfolio A (asymmetric strategy only) ---
	if v := query.Get("stressed_a_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			id := n
			req.StressedAID = &id
		} else {
			return req, &APIError{Code: "INVALID_STRESSED_PORTFOLIO", Error: "invalid stressed_a_id: " + v}
		}
	}

	// --- Stressed model portfolio B (asymmetric strategy only) ---
	if v := query.Get("stressed_b_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			id := n
			req.StressedBID = &id
		} else {
			return req, &APIError{Code: "INVALID_STRESSED_PORTFOLIO", Error: "invalid stressed_b_id: " + v}
		}
	}

	// --- Drawdown trigger (asymmetric strategy only, percentage points) ---
	if v := query.Get("drawdown_trigger_pct"); v != "" {
		d, err := decimal.Parse(v)
		if err != nil || d.IsNeg() || d.Cmp(decimal.MustParse("100")) > 0 {
			return req, &APIError{Code: "INVALID_DRAWDOWN_TRIGGER", Error: "invalid drawdown_trigger_pct: " + v + ", must be a number between 0 and 100"}
		}
		req.DrawdownTriggerPct = &d
	}

	// --- Asymmetric strategy requires a stressed portfolio per model side ---
	if req.Strategy == comparison.StrategyRebalancedAsymmetric {
		if req.PortfolioAType == comparison.PortTypeModel && req.StressedAID == nil {
			return req, &APIError{Code: "INVALID_STRESSED_PORTFOLIO", Error: "stressed_a_id is required when strategy is rebalanced_asymmetric and portfolio_a_type is model"}
		}
		if req.PortfolioBType == comparison.PortTypeModel && req.StressedBID == nil {
			return req, &APIError{Code: "INVALID_STRESSED_PORTFOLIO", Error: "stressed_b_id is required when strategy is rebalanced_asymmetric and portfolio_b_type is model"}
		}
	}

	return req, nil
}
