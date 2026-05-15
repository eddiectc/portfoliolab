# FX Conventions

This document describes how foreign exchange rates are applied across multi-currency portfolios. All conventions are designed for the case where **foreign currency cash is retained in the account** after trades (e.g., USD cash stays in an IBKR USD account rather than being auto-converted to GBP).

## Summary

| Context | FX Rate Used | Rationale |
|---|---|---|
| **Equity curve — portfolio value** | Valuation-date FX rate | Answers: "what was my portfolio worth in base currency on this date?" |
| **Equity curve — net deposit** | Valuation-date FX rate | Consistent with portfolio value on the same date; enables reconciliation with position summaries |
| **Open positions** (unrealized P&L) | Current spot FX rate | Answers: "what is this position worth in base currency today?" |
| **Closed positions** (realized P&L) | Current spot FX rate | Answers: "what is the realized cash (still in foreign currency) worth in base currency today?" |

## Detailed Rationale

### Equity Curve — Valuation-Date FX for Both Portfolio Value and Net Deposit

The equity curve replays portfolio value on each historical date. On any given date:

- **Portfolio value**: all positions (cash and securities) are valued at that date's market prices and FX rates
- **Net deposit**: all deposits and withdrawals are converted to base currency at that date's FX rates

Both use the **same FX rate** (the valuation date's rate), so `profit = portfolio_value − net_deposit` is internally consistent on every date.

**Why not transaction-date FX for net deposit?** If you deposit USD, it stays as USD in your account — converting it to GBP at the transaction date's rate doesn't represent anything real. Using valuation-date FX answers: "if I converted everything to GBP on this date, what would my net investment be?" This is consistent with how portfolio value is computed and enables the equity curve's last point to reconcile with position summaries (both use spot FX at the current point).

**Trade-off:** Historical profit numbers shift as FX rates change (moving target). But the curve is always self-consistent, and FX impact is still captured — it just appears in both portfolio value and net deposit rather than being isolated in one.

**Implementation note:** For dates between transaction days (interpolated), both portfolio value and net deposit are revalued at that date's FX rate using forward-fill from the nearest cached rate. This ensures the identity holds at every point on the curve, not just at transaction dates.

**No separate FX P&L tracking is needed** — FX impact is naturally embedded in the day-by-day valuation. If GBP weakened against USD, your USD positions are worth more in GBP on that day, and the curve reflects that.

### Open Positions — Spot FX

Open positions use the current spot FX rate because:

1. The foreign currency was already in the account (deposited directly or from prior sells)
2. Any base→foreign conversion to fund buys is already captured as a withdrawal+deposit pair, which the equity curve prices at that date's FX rate
3. Spot FX answers: "what is this position worth in base currency if I liquidate and convert today?"

### Closed Positions — Spot FX

Closed positions also use the current spot FX rate because:

1. The realized P&L remains as **foreign currency cash** in the account until explicitly converted
2. The GBP value of that cash is inherently a moving target — it changes with FX rates
3. Using the close-date FX rate would imply the GBP value is locked in, which it isn't
4. Spot FX keeps the number consistent with open positions and the equity curve's last point

When you eventually convert USD→GBP, record it as a **withdrawal in USD + deposit in GBP**. The equity curve naturally captures the FX gain/loss from that conversion because it sees the USD outflow and GBP inflow on that date at that day's rate.

## Reconciliation Identity

The `profit_loss` metric is computed from the equity curve's last point:

```
profit_loss = portfolio_value(spot) − net_deposit(spot)
```

This represents **total portfolio return** — it includes:
- **Capital gains/losses**: price movements on open positions (unrealized) + locked-in gains from closed positions (realized)
- **Investment income**: dividends, interest
- **Costs**: fees, taxes

The equity curve's `portfolio_value` and `net_deposit` both use **valuation-date FX** (spot rate at the current point), so they're internally consistent.

### Why profit_loss ≠ unrealized + realized?

The position summary's `unrealized + realized` only captures **capital gains** (price movements). It excludes dividends, interest, fees, and taxes — which are tracked as separate transaction types and contribute to the equity curve's profit but don't appear in position P&L.

To reconcile:
```
profit_loss (equity curve) = unrealized + realized + dividends + interest − fees − taxes
```

### Historical dates

On historical dates, the equity curve uses cached FX rates for that date. Net deposit is converted at the valuation-date FX (not the transaction-date FX), so the curve is internally consistent at every point. However, past profit numbers may shift as FX rates are updated in the cache.

## What This Means in Practice

- **Realized P&L in base currency changes daily** as FX rates move. This is correct — the USD cash is still exposed to FX risk until you convert it.
- **The equity curve is the single source of truth for historical performance**. It tells you exactly how your portfolio performed on each past date, including FX effects.
- **Position summaries tell you what's driving current profit**. Unrealized (price movement on open positions) and realized (locked-in gains from closed positions), both valued at today's FX rates.

## Example

You buy 100 shares of AAPL at $150 (cost: $15,000) when GBP/USD = 1.27 (USD/GBP ≈ 0.787):

| Event | What happens |
|---|---|
| Buy at $150, FX ≈ 0.787 | Cost basis = $15,000 (≈ £11,805 at buy-date FX) |
| Price rises to $180 | Market value = $18,000, unrealized = $3,000 |
| Sell at $180 | Realized = $3,000 in USD cash |
| FX moves to 0.790 | Realized in GBP = $3,000 × 0.790 = £2,370 (not £2,361 at close-date FX) |
| FX moves to 0.795 | Realized in GBP = $3,000 × 0.795 = £2,385 (updated automatically) |

The GBP value of your realized gain changes with FX — because the cash is still in USD.

## Conversion Transactions

When you convert foreign currency cash to base currency:

1. Record a **withdrawal transaction** in the foreign currency (e.g., -$5,000 USD)
2. Record a **deposit transaction** in the base currency (e.g., +£3,950 GBP)

The equity curve will see both transactions on that date and naturally capture the FX rate implied by the pair. No special "currency conversion" transaction type is needed.
