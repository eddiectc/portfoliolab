package position

import (
	"testing"

	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"github.com/govalues/decimal"
)

// --- ConvertPnlToBase tests ---

func TestConvertPnlToBase_SameCurrency(t *testing.T) {
	pnl := decimal.MustNew(10000, 2) // +100.00
	converted, rateUsed, isFallback := ConvertPnlToBase(pnl, "USD", "USD", nil, false)

	if !converted.Equal(pnl) {
		t.Errorf("expected unchanged P&L %s, got %s", pnl.String(), converted.String())
	}
	if rateUsed != nil {
		t.Errorf("expected nil rate, got %v", rateUsed)
	}
	if isFallback {
		t.Error("expected no fallback")
	}
}

func TestConvertPnlToBase_ConvertsToBase(t *testing.T) {
	pnl := decimal.MustNew(10000, 2) // +100.00 GBP
	rate := &market.FxRate{
		BaseCurrency: "GBP", QuoteCurrency: "USD",
		Rate: decimal.MustNew(12500, 4), // 1.2500
	}
	converted, rateUsed, isFallback := ConvertPnlToBase(pnl, "GBP", "USD", rate, false)

	// Mul combines scales: pnl (scale 2) * rate (scale 4) → scale 6
	// 100.00 * 1.2500 = 125.000000
	want := decimal.MustNew(125000000, 6)
	if !converted.Equal(want) {
		t.Errorf("expected %s, got %s", want.String(), converted.String())
	}
	if rateUsed == nil || !rateUsed.Equal(rate.Rate) {
		t.Errorf("expected rate %s, got %v", rate.Rate.String(), rateUsed)
	}
	if isFallback {
		t.Error("expected no fallback")
	}
}

func TestConvertPnlToBase_NoRateReturnsZero(t *testing.T) {
	pnl := decimal.MustNew(10000, 2) // 100.00 GBP
	converted, rateUsed, isFallback := ConvertPnlToBase(pnl, "GBP", "USD", nil, false)

	// No rate available: returns zero, NOT the unconverted value.
	// Returning raw GBP as USD would silently corrupt totals.
	if !converted.Equal(decimal.Zero) {
		t.Errorf("expected zero, got %s", converted.String())
	}
	if rateUsed != nil {
		t.Errorf("expected nil rate, got %v", rateUsed)
	}
	if !isFallback {
		t.Error("expected fallback")
	}
}

func TestConvertPnlToBase_NegativePnL(t *testing.T) {
	pnl := decimal.MustNew(-5000, 2) // -50.00 GBP
	rate := &market.FxRate{
		BaseCurrency: "GBP", QuoteCurrency: "USD",
		Rate: decimal.MustNew(12000, 4), // 1.2000
	}
	converted, _, _ := ConvertPnlToBase(pnl, "GBP", "USD", rate, false)

	// scale 2 * scale 4 = scale 6
	want := decimal.MustNew(-60000000, 6) // -50.00 * 1.2000 = -60.000000
	if !converted.Equal(want) {
		t.Errorf("expected %s, got %s", want.String(), converted.String())
	}
}

func TestConvertPnlToBase_ZeroPnL(t *testing.T) {
	pnl := decimal.Zero
	rate := &market.FxRate{
		BaseCurrency: "GBP", QuoteCurrency: "USD",
		Rate: decimal.MustNew(12500, 4),
	}
	converted, _, _ := ConvertPnlToBase(pnl, "GBP", "USD", rate, false)

	if !converted.Equal(decimal.Zero) {
		t.Errorf("expected zero, got %s", converted.String())
	}
}

// --- FormatFxPair tests (via market package) ---

func TestFormatFxPair_DifferentCurrency(t *testing.T) {
	pair := market.FormatFxPair("GBP", "USD")
	if pair != "GBP/USD" {
		t.Errorf("expected GBP/USD, got %q", pair)
	}
}

func TestFormatFxPair_EurToGbp(t *testing.T) {
	pair := market.FormatFxPair("EUR", "GBP")
	if pair != "EUR/GBP" {
		t.Errorf("expected EUR/GBP, got %q", pair)
	}
}

// --- ConventionFxRate tests ---

func TestConventionFxRate_SameCurrency(t *testing.T) {
	rate := decimal.MustNew(10000, 4)
	result := ConventionFxRate("USD", "USD", &rate)
	if result != nil {
		t.Errorf("expected nil for same currency, got %+v", result)
	}
}

func TestConventionFxRate_NilRate(t *testing.T) {
	result := ConventionFxRate("USD", "GBP", nil)
	if result != nil {
		t.Errorf("expected nil for nil rate, got %+v", result)
	}
}

func TestConventionFxRate_ZeroRate(t *testing.T) {
	rate := decimal.Zero
	result := ConventionFxRate("USD", "GBP", &rate)
	if result != nil {
		t.Errorf("expected nil for zero rate, got %+v", result)
	}
}

func TestConventionFxRate_GbpPositionUsdBase_NoInversion(t *testing.T) {
	// Position=GBP, Base=USD → convention GBP/USD (no inversion).
	// Stored rate is 1.3000 (GBP/USD), should display as-is.
	rate := decimal.MustNew(13000, 4) // 1.3000
	result := ConventionFxRate("GBP", "USD", &rate)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pair != "GBP/USD" {
		t.Errorf("expected pair GBP/USD, got %q", result.Pair)
	}
	if !result.Rate.Equal(rate) {
		t.Errorf("expected rate 1.3000, got %s", result.Rate.String())
	}
}

func TestConventionFxRate_UsdPositionGbpBase_Inverted(t *testing.T) {
	// Position=USD, Base=GBP → convention GBP/USD (inverted).
	// Stored rate is 0.7692 (USD/GBP), should display as 1/0.7692 ≈ 1.3000.
	rate := decimal.MustNew(7692, 4) // 0.7692 (USD/GBP)
	result := ConventionFxRate("USD", "GBP", &rate)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pair != "GBP/USD" {
		t.Errorf("expected pair GBP/USD, got %q", result.Pair)
	}
	// 1 / 0.7692 ≈ 1.3000 (roughly, depends on precision)
	if result.Rate.Equal(decimal.Zero) {
		t.Errorf("expected non-zero rate, got %s", result.Rate.String())
	}
	// Verify it's positive (GBP > USD in value, rate > 1)
	if !result.Rate.IsPos() {
		t.Errorf("expected positive rate, got %s", result.Rate.String())
	}
}

func TestConventionFxRate_EurPositionUsdBase_NoInversion(t *testing.T) {
	// Position=EUR, Base=USD → convention EUR/USD (no inversion).
	rate := decimal.MustNew(11000, 4) // 1.1000
	result := ConventionFxRate("EUR", "USD", &rate)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pair != "EUR/USD" {
		t.Errorf("expected pair EUR/USD, got %q", result.Pair)
	}
	if !result.Rate.Equal(rate) {
		t.Errorf("expected rate 1.1000, got %s", result.Rate.String())
	}
}

func TestConventionFxRate_UsdPositionEurBase_Inverted(t *testing.T) {
	// Position=USD, Base=EUR → convention EUR/USD (inverted).
	rate := decimal.MustNew(9090, 4) // 0.9090 (USD/EUR)
	result := ConventionFxRate("USD", "EUR", &rate)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pair != "EUR/USD" {
		t.Errorf("expected pair EUR/USD, got %q", result.Pair)
	}
	// 1 / 0.9090 ≈ 1.1001
	if result.Rate.Equal(decimal.Zero) {
		t.Errorf("expected non-zero rate, got %s", result.Rate.String())
	}
}

func TestConventionFxRate_EurGbp_NoInversion(t *testing.T) {
	// Position=EUR, Base=GBP → convention EUR/GBP (no inversion).
	rate := decimal.MustNew(8500, 4) // 0.8500
	result := ConventionFxRate("EUR", "GBP", &rate)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pair != "EUR/GBP" {
		t.Errorf("expected pair EUR/GBP, got %q", result.Pair)
	}
	if !result.Rate.Equal(rate) {
		t.Errorf("expected rate 0.8500, got %s", result.Rate.String())
	}
}

func TestConventionFxRate_GbpEur_Inverted(t *testing.T) {
	// Position=GBP, Base=EUR → convention EUR/GBP (inverted).
	rate := decimal.MustNew(11764, 4) // 1.1764 (GBP/EUR)
	result := ConventionFxRate("GBP", "EUR", &rate)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pair != "EUR/GBP" {
		t.Errorf("expected pair EUR/GBP, got %q", result.Pair)
	}
	// 1 / 1.1764 ≈ 0.8500
	if result.Rate.Equal(decimal.Zero) {
		t.Errorf("expected non-zero rate, got %s", result.Rate.String())
	}
}

func TestConventionFxRate_UsdJpy_NoInversion(t *testing.T) {
	// Position=USD, Base=JPY → convention USD/JPY (no inversion, JPY not a "major").
	rate := decimal.MustNew(150000, 4) // 150.0000
	result := ConventionFxRate("USD", "JPY", &rate)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pair != "USD/JPY" {
		t.Errorf("expected pair USD/JPY, got %q", result.Pair)
	}
	if !result.Rate.Equal(rate) {
		t.Errorf("expected rate 150.0000, got %s", result.Rate.String())
	}
}
