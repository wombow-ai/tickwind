package indicators

import "github.com/wombow-ai/tickwind/internal/edgar"

// This file holds Increment-3 COMPOSITE fundamental scores (design §1.2 Group 5).
// Same anti-fabrication contract as the rest: a score is emitted (ok=true) only when
// EVERY input it needs is present; any missing concept → ok=false → the compute layer
// renders "insufficient" with a reason and NEVER fabricates a number. Composite scores
// are deliberately conservative: a partial score would mislead, so we withhold it.
//
// Currently: Altman Z-score and Piotroski F-score (the latter needs a full prior-FY
// income/balance set, now extracted). Beneish-M (current+prior, some A-share-flavored
// terms) stays ABSENT from the registry until its inputs land — never faked.

// altmanZ computes the original Altman Z-score for public manufacturers:
//
//	Z = 1.2·X1 + 1.4·X2 + 3.3·X3 + 0.6·X4 + 1.0·X5
//	X1 = working capital / total assets   X2 = retained earnings / total assets
//	X3 = EBIT / total assets              X4 = market cap / total liabilities
//	X5 = revenue / total assets
//
// A dimensionless score (Unit ""): >2.99 safe · 1.81–2.99 grey · <1.81 distress.
// ok=false unless every denominator/input is present — total & current assets/
// liabilities, total liabilities, revenue, a market cap (price × shares) and a
// derivable EBIT — so a non-classified or thin balance sheet yields "insufficient",
// never a partial Z. Retained earnings is used as reported (legitimately negative for
// heavy-buyback issuers; an absent concept reads 0 — a minor conservative bias on X2).
func altmanZ(price float64, f edgar.Fundamentals) (float64, bool) {
	ta := f.TotalAssets
	if ta <= 0 || f.TotalLiabilities <= 0 || f.Revenue <= 0 || price <= 0 || f.Shares <= 0 ||
		f.AssetsCurrent <= 0 || f.LiabilitiesCurrent <= 0 {
		return 0, false
	}
	eb, ok := ebit(f)
	if !ok {
		return 0, false
	}
	marketCap := price * float64(f.Shares)
	x1 := (f.AssetsCurrent - f.LiabilitiesCurrent) / ta
	x2 := f.RetainedEarnings / ta
	x3 := eb / ta
	x4 := marketCap / f.TotalLiabilities
	x5 := f.Revenue / ta
	return 1.2*x1 + 1.4*x2 + 3.3*x3 + 0.6*x4 + 1.0*x5, true
}

// piotroskiF computes the Piotroski F-score: the integer sum (0–9) of 9 binary
// fundamental signals — 4 profitability, 3 leverage/liquidity, 2 efficiency — that
// jointly grade financial health and YoY improvement (8–9 strong, 0–2 weak). It is
// a dimensionless score (Unit "").
//
// ALL-OR-NOTHING anti-fabrication contract: the 9 points span the current AND prior
// fiscal year, so the score is only faithful when a full prior-FY income/balance set
// exists. We require, up front:
//   - denominators > 0: TotalAssets, TotalAssetsPrior, LiabilitiesCurrent,
//     LiabilitiesCurrentPrior, Revenue, RevenuePrior (every ratio below divides by
//     one of these; a 0 denominator is an absent concept, not a real value);
//   - prior-FY data present (non-zero): NetIncomePrior, GrossProfitPrior,
//     AssetsCurrentPrior, LongTermDebtPrior, SharesPrior — their presence is the
//     proxy for "a prior balance sheet was actually extracted". If any is 0 we cannot
//     get a complete prior year, so we return ok=false (insufficient) and emit NO
//     value — NEVER a partial score (e.g. a 6/9 with 3 components silently dropped
//     would read as a misleadingly low score).
//
// We do NOT gate on the sign-bearing current flows NetIncome / OperatingCashFlow /
// GrossProfit / LongTermDebt / AssetsCurrent: those legitimately read 0 or negative
// and are exactly what points 1–4/5/6/8 test. They are required to be *extractable*,
// which the denominator/prior gates above already guarantee in practice (a filer with
// assets, revenue and a prior balance sheet reports net income, OCF, gross profit and
// a current balance). LongTermDebtPrior > 0 is required (proves the prior balance
// exists); the CURRENT LongTermDebt may be 0 — a firm that paid all long-term debt
// down to 0 has 0 <= prior, so leverage did not rise → point 5 is awarded. A fully
// debt-free firm (both 0) cannot reach this path because LongTermDebtPrior == 0 fails
// the prior gate → insufficient; that is the conservative choice (we cannot prove a
// prior balance sheet from an absent debt concept).
func piotroskiF(f edgar.Fundamentals) (int, bool) {
	// Denominators must be strictly positive (a 0 here is an absent concept).
	if f.TotalAssets <= 0 || f.TotalAssetsPrior <= 0 ||
		f.LiabilitiesCurrent <= 0 || f.LiabilitiesCurrentPrior <= 0 ||
		f.Revenue <= 0 || f.RevenuePrior <= 0 {
		return 0, false
	}
	// Prior-FY components must be present (non-zero) — their absence means we could
	// not assemble a complete prior year, so the score is insufficient, never partial.
	if f.NetIncomePrior == 0 || f.GrossProfitPrior == 0 ||
		f.AssetsCurrentPrior == 0 || f.LongTermDebtPrior == 0 || f.SharesPrior == 0 {
		return 0, false
	}

	score := 0
	b := func(pass bool) {
		if pass {
			score++
		}
	}

	roa := f.NetIncome / f.TotalAssets
	roaPrior := f.NetIncomePrior / f.TotalAssetsPrior

	// PROFITABILITY (4).
	b(roa > 0)                           // (1) ROA > 0
	b(f.OperatingCashFlow > 0)           // (2) OCF > 0
	b(roa > roaPrior)                    // (3) ΔROA > 0
	b(f.OperatingCashFlow > f.NetIncome) // (4) accrual: OCF > net income (cash-backed earnings)

	// LEVERAGE / LIQUIDITY (3).
	b(f.LongTermDebt <= f.LongTermDebtPrior) // (5) leverage did not rise (≤; a debt-free-vs-prior firm passes)
	curr := f.AssetsCurrent / f.LiabilitiesCurrent
	currPrior := f.AssetsCurrentPrior / f.LiabilitiesCurrentPrior
	b(curr > currPrior)          // (6) current ratio improved
	b(f.Shares <= f.SharesPrior) // (7) no dilution (shares did not increase)

	// EFFICIENCY (2).
	gm := f.GrossProfit / f.Revenue
	gmPrior := f.GrossProfitPrior / f.RevenuePrior
	b(gm > gmPrior) // (8) gross margin improved
	turn := f.Revenue / f.TotalAssets
	turnPrior := f.RevenuePrior / f.TotalAssetsPrior
	b(turn > turnPrior) // (9) asset turnover improved

	return score, true
}

// --- RISK / RETURN: market beta + ~1-year total shareholder return ---
//
// These two are the §Group-5 RISK/RETURN pair. Unlike the composite scores above they
// are price-series driven (TSR uses the stock's own daily closes + the latest-FY
// dividend; beta uses the DATE-ALIGNED stock-vs-SPY daily returns the Computer prepares
// in computeInput.marketReturns). Same anti-fabrication contract: a number is emitted
// only when the window is long enough to be faithful — never invented for a thin/new
// name.

// tsrTradingDays is the ~1-year lookback (≈252 US trading days). The start price is
// taken this many bars back from the latest close.
const tsrTradingDays = 252

// tsrMinCloses is the minimum daily-close count for a faithful ~1-year TSR. Below this
// the window is materially shorter than a year, so the result is not a 1-year return
// and is reported insufficient rather than mislabeled. (Allowing a small shortfall
// below the full 252 tolerates holiday/half-day gaps in a real annual series.)
const tsrMinCloses = 240

// betaMinPairs is the minimum number of ALIGNED daily-return pairs required for a
// faithful beta. Fewer than ~60 (≈3 trading months) overfits noise, so beta is reported
// insufficient — never fabricated for a thin or newly-listed name.
const betaMinPairs = 60

// tsr computes the ~1-year total shareholder return as a PERCENT: price appreciation
// plus the latest-FY dividend per share, over the starting price ~252 trading days ago.
//
//	start = closes[max(0, len-252)] ; end = price (else the last close)
//	divPerShare = (Shares>0 && DividendsPaid>0) ? DividendsPaid/Shares : 0   (non-payers → 0)
//	tsr = ((end - start) + divPerShare) / start * 100
//
// APPROXIMATION (documented): the dividend term is the latest-FY total dividends paid
// divided by shares — a per-share dividend aligned to the ~1-year price window, not a
// sum of the actual ex-dates inside the window (which Fundamentals does not carry). A
// non-payer legitimately contributes 0, so TSR collapses to the pure price return.
//
// GATING: ok=false unless there are >= tsrMinCloses closes (≈1 trading year — else it
// is not a 1-year return) AND start>0 AND end>0. price<=0 falls back to the last close.
func tsr(closes []float64, price float64, f edgar.Fundamentals) (float64, bool) {
	if len(closes) < tsrMinCloses {
		return 0, false
	}
	// start = closes[max(0, len-252)]: ~252 trading days back, clamped to the first
	// close when the series is shorter than a full year (but still >= tsrMinCloses).
	startIdx := len(closes) - tsrTradingDays
	if startIdx < 0 {
		startIdx = 0
	}
	start := closes[startIdx]
	end := price
	if end <= 0 {
		end = closes[len(closes)-1]
	}
	if start <= 0 || end <= 0 {
		return 0, false
	}
	divPerShare := 0.0
	if f.Shares > 0 && f.DividendsPaid > 0 {
		divPerShare = f.DividendsPaid / float64(f.Shares)
	}
	return ((end - start) + divPerShare) / start * 100, true
}

// beta computes the market beta from two ALREADY-DATE-ALIGNED, equal-length daily
// return series (the stock's and the market's):
//
//	β = Σ((r_s − mean_s)(r_m − mean_m)) / Σ((r_m − mean_m)^2)
//	  = cov(stock, market) / var(market)
//
// The alignment (matching the two series by date, dropping unpaired days) lives in the
// Computer; this helper is the pure, unit-testable core over the prepared pairs.
//
// ok=false unless len == len (equal, both >= betaMinPairs) AND var(market) > 0. A
// zero-variance market window (or too few pairs) cannot yield a faithful beta, so it is
// reported insufficient rather than fabricated.
func beta(stockReturns, marketReturns []float64) (float64, bool) {
	n := len(marketReturns)
	if n != len(stockReturns) || n < betaMinPairs {
		return 0, false
	}
	var sumS, sumM float64
	for i := 0; i < n; i++ {
		sumS += stockReturns[i]
		sumM += marketReturns[i]
	}
	meanS := sumS / float64(n)
	meanM := sumM / float64(n)
	var cov, varM float64
	for i := 0; i < n; i++ {
		ds := stockReturns[i] - meanS
		dm := marketReturns[i] - meanM
		cov += ds * dm
		varM += dm * dm
	}
	if varM <= 0 {
		return 0, false
	}
	return cov / varM, true
}

// fundamentalRegistryInc3 registers the Increment-3 composite-score closures. Each id
// is a real catalog id; the remaining composite (beneish-m) is simply not registered,
// so it stays absent rather than faked.
func fundamentalRegistryInc3() map[string]computeFn {
	return map[string]computeFn{
		"fundamental.altman-z-score": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := altmanZ(in.price, in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "missing balance-sheet inputs (assets/liabilities/revenue/EBIT/market cap)")
			}
		},
		"fundamental.piotroski-f-score": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := piotroskiF(in.fund); ok {
				setOK(si, float64(v), unitNone)
			} else {
				setInsufficient(si, "missing current+prior-FY inputs (need two years of assets/liabilities/revenue/income, prior gross profit, long-term debt and share count)")
			}
		},
		// TSR — ~1-year total shareholder return (price appreciation + latest-FY
		// dividend-per-share). hasFund-gated like the rest (the dividend term needs
		// Fundamentals) AND needs ≈1 trading year of the stock's own closes.
		"fundamental.tsr": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := tsr(in.closes, in.price, in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "need ≈1 year (240+) of daily closes with a positive start/end price for a 1-year TSR")
			}
		},
		// BETA — market beta vs SPY over the DATE-ALIGNED ~1-year daily-return series.
		// Gated on the Computer-prepared aligned pairs (in.stockReturns / in.marketReturns):
		// empty when SPY candles are unavailable or the ticker IS SPY → insufficient,
		// never a fabricated beta for a thin/new name.
		"fundamental.beta": func(in computeInput, si *StockIndicator) {
			if v, ok := beta(in.stockReturns, in.marketReturns); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "need 60+ date-aligned stock-vs-SPY daily-return pairs with non-zero market variance (SPY series unavailable or too short)")
			}
		},
	}
}
