package indicators

import "github.com/wombow-ai/tickwind/internal/edgar"

// This file holds Increment-3 COMPOSITE fundamental scores (design §1.2 Group 5).
// Same anti-fabrication contract as the rest: a score is emitted (ok=true) only when
// EVERY input it needs is present; any missing concept → ok=false → the compute layer
// renders "insufficient" with a reason and NEVER fabricates a number. Composite scores
// are deliberately conservative: a partial score would mislead, so we withhold it.
//
// Currently: Altman Z-score. Piotroski-F (needs a full prior-FY income/balance set) and
// Beneish-M (current+prior, some A-share-flavored terms) follow once their prior-year
// fields are extracted — they stay ABSENT from the registry until then, never faked.

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

// fundamentalRegistryInc3 registers the Increment-3 composite-score closures. Each id
// is a real catalog id; unimplemented composites (piotroski-f, beneish-m) are simply
// not registered, so they stay absent rather than faked.
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
	}
}
