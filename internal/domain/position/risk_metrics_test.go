package position

import (
	"testing"

	"github.com/govalues/decimal"
)

func TestComputeRiskMetrics_ZeroVolatility(t *testing.T) {
	// All returns are identical → zero volatility → nil Sharpe.
	// Sortino may still be computed if there is downside deviation
	// (returns below risk-free rate), even with zero total volatility.
	returns := []DailyReturn{
		{Date: mustTime("2024-01-01"), ReturnPct: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-02"), ReturnPct: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-03"), ReturnPct: decimal.MustParse("1.00")},
	}

	riskFree := decimal.MustParse("4.50")
	result := ComputeRiskMetrics(returns, &riskFree)

	// Volatility should be 0 (or very close).
	if result.AnnualizedVolatilityPct == nil {
		t.Fatal("expected non-nil annualized volatility")
	}
	if result.AnnualizedVolatilityPct.Sign() != 0 {
		t.Errorf("volatility: got %s, want 0 (all returns identical)", result.AnnualizedVolatilityPct.String())
	}

	// Sharpe should be nil (division by zero volatility).
	if result.SharpeRatio != nil {
		t.Errorf("sharpe: got %s, want nil (zero volatility)", result.SharpeRatio.String())
	}
	// Sortino: returns (1% daily) are well above daily risk-free (4.5%/252 ≈ 0.018%),
	// so there is no downside deviation → Sortino is nil.
	if result.SortinoRatio != nil {
		t.Errorf("sortino: got %s, want nil (no downside returns)", result.SortinoRatio.String())
	}
}

func TestComputeRiskMetrics_PositiveReturns(t *testing.T) {
	// Mixed positive/negative returns with a risk-free rate.
	returns := []DailyReturn{
		{Date: mustTime("2024-01-01"), ReturnPct: decimal.MustParse("0.50")},
		{Date: mustTime("2024-01-02"), ReturnPct: decimal.MustParse("1.20")},
		{Date: mustTime("2024-01-03"), ReturnPct: decimal.MustParse("-0.30")},
		{Date: mustTime("2024-01-04"), ReturnPct: decimal.MustParse("0.80")},
		{Date: mustTime("2024-01-05"), ReturnPct: decimal.MustParse("-0.10")},
	}

	riskFree := decimal.MustParse("4.50")
	result := ComputeRiskMetrics(returns, &riskFree)

	if result.AnnualizedVolatilityPct == nil {
		t.Fatal("expected non-nil annualized volatility")
	}
	// Vol should be positive.
	if !result.AnnualizedVolatilityPct.IsPos() {
		t.Errorf("volatility: got %s, want positive value", result.AnnualizedVolatilityPct.String())
	}

	if result.SharpeRatio == nil {
		t.Fatal("expected non-nil Sharpe ratio")
	}
	if result.SortinoRatio == nil {
		t.Fatal("expected non-nil Sortino ratio")
	}
}

func TestComputeRiskMetrics_NegativeReturns(t *testing.T) {
	// Mostly negative returns.
	returns := []DailyReturn{
		{Date: mustTime("2024-01-01"), ReturnPct: decimal.MustParse("-1.00")},
		{Date: mustTime("2024-01-02"), ReturnPct: decimal.MustParse("-0.50")},
		{Date: mustTime("2024-01-03"), ReturnPct: decimal.MustParse("-2.00")},
		{Date: mustTime("2024-01-04"), ReturnPct: decimal.MustParse("-0.30")},
	}

	riskFree := decimal.MustParse("4.50")
	result := ComputeRiskMetrics(returns, &riskFree)

	if result.AnnualizedVolatilityPct == nil {
		t.Fatal("expected non-nil annualized volatility")
	}

	// Sharpe should be negative (returns well below risk-free rate).
	if result.SharpeRatio == nil {
		t.Fatal("expected non-nil Sharpe ratio")
	}
	if result.SharpeRatio.Sign() >= 0 {
		t.Errorf("sharpe: got %s, want negative (returns below risk-free)", result.SharpeRatio.String())
	}

	// Sortino should also be negative.
	if result.SortinoRatio == nil {
		t.Fatal("expected non-nil Sortino ratio")
	}
	if result.SortinoRatio.Sign() >= 0 {
		t.Errorf("sortino: got %s, want negative (all returns below risk-free)", result.SortinoRatio.String())
	}
}

func TestComputeRiskMetrics_NilRiskFreeRate(t *testing.T) {
	// When risk-free rate is nil, Sharpe and Sortino should be nil.
	returns := []DailyReturn{
		{Date: mustTime("2024-01-01"), ReturnPct: decimal.MustParse("0.50")},
		{Date: mustTime("2024-01-02"), ReturnPct: decimal.MustParse("1.20")},
		{Date: mustTime("2024-01-03"), ReturnPct: decimal.MustParse("-0.30")},
	}

	result := ComputeRiskMetrics(returns, nil)

	if result.AnnualizedVolatilityPct == nil {
		t.Fatal("expected non-nil annualized volatility even without risk-free rate")
	}

	if result.SharpeRatio != nil {
		t.Errorf("sharpe: got %s, want nil (no risk-free rate)", result.SharpeRatio.String())
	}
	if result.SortinoRatio != nil {
		t.Errorf("sortino: got %s, want nil (no risk-free rate)", result.SortinoRatio.String())
	}
}

func TestComputeRiskMetrics_ValidRiskFreeRate(t *testing.T) {
	// Mixed returns (some positive, some negative), risk-free rate provided.
	// Sharpe and Sortino should both be computed.
	returns := []DailyReturn{
		{Date: mustTime("2024-01-01"), ReturnPct: decimal.MustParse("0.50")},
		{Date: mustTime("2024-01-02"), ReturnPct: decimal.MustParse("1.20")},
		{Date: mustTime("2024-01-03"), ReturnPct: decimal.MustParse("-0.80")},
		{Date: mustTime("2024-01-04"), ReturnPct: decimal.MustParse("0.30")},
		{Date: mustTime("2024-01-05"), ReturnPct: decimal.MustParse("-0.50")},
	}

	riskFree := decimal.MustParse("4.50")
	result := ComputeRiskMetrics(returns, &riskFree)

	if result.AnnualizedVolatilityPct == nil {
		t.Fatal("expected non-nil annualized volatility")
	}
	if result.SharpeRatio == nil {
		t.Fatal("expected non-nil Sharpe ratio with valid risk-free rate")
	}
	if result.SortinoRatio == nil {
		t.Fatal("expected non-nil Sortino ratio with valid risk-free rate")
	}
}

func TestComputeRiskMetrics_Empty(t *testing.T) {
	result := ComputeRiskMetrics([]DailyReturn{}, nil)

	if result.AnnualizedVolatilityPct != nil {
		t.Errorf("volatility: got %s, want nil (empty input)", result.AnnualizedVolatilityPct.String())
	}
	if result.SharpeRatio != nil {
		t.Errorf("sharpe: got %s, want nil (empty input)", result.SharpeRatio.String())
	}
	if result.SortinoRatio != nil {
		t.Errorf("sortino: got %s, want nil (empty input)", result.SortinoRatio.String())
	}
}

func TestComputeRiskMetrics_SinglePoint(t *testing.T) {
	returns := []DailyReturn{
		{Date: mustTime("2024-01-01"), ReturnPct: decimal.MustParse("1.00")},
	}

	result := ComputeRiskMetrics(returns, nil)

	if result.AnnualizedVolatilityPct != nil {
		t.Errorf("volatility: got %s, want nil (single point)", result.AnnualizedVolatilityPct.String())
	}
}

func TestComputeRiskMetrics_AllPositiveReturns_NoDownside(t *testing.T) {
	// All returns above risk-free rate → no downside → Sortino is nil.
	riskFree := decimal.MustParse("0.10") // 0.10% daily
	returns := []DailyReturn{
		{Date: mustTime("2024-01-01"), ReturnPct: decimal.MustParse("1.00")},
		{Date: mustTime("2024-01-02"), ReturnPct: decimal.MustParse("0.50")},
		{Date: mustTime("2024-01-03"), ReturnPct: decimal.MustParse("0.80")},
	}

	result := ComputeRiskMetrics(returns, &riskFree)

	if result.AnnualizedVolatilityPct == nil {
		t.Fatal("expected non-nil annualized volatility")
	}
	if result.SharpeRatio == nil {
		t.Fatal("expected non-nil Sharpe ratio")
	}
	// All returns above risk-free → no downside deviation → Sortino nil.
	if result.SortinoRatio != nil {
		t.Errorf("sortino: got %s, want nil (no downside returns)", result.SortinoRatio.String())
	}
}
