package indicators

// This file registers the Increment-2 FUNDAMENTAL ratio closures (design §1.2
// Groups 1, 2, 4 + the faithfully-extractable subset of Group 3). It is a sibling of
// fundamentalRegistryMore() (Group 0); compute.go merges it into the Computer
// registry so each id participates automatically (and is ABSENT — never faked — when
// not registered).
//
// Every closure follows the established Group-0 pattern EXACTLY: guard in.hasFund,
// read price from in.price, call the matching pure ratio, and either setOK with the
// correct Unit or setInsufficient with a concrete reason (no Value). Per the
// no-fabrication contract a value is emitted ONLY when every input is present and the
// denominator is valid; an absent/zero/negative input becomes insufficient.
//
// Unit choices (the shared contract + design §1.2): margins / growth rates / ROCE /
// ROIC / LT-debt ratio / net gearing / goodwill-to-equity / R&D intensity / buyback
// yield → "%"; coverage, turnover, liquidity, EV and price multiples → "x"; the
// DIO/DSO/DPO/CCC day-counts and the EV / basic-EPS dollar/per-share amounts → ""
// (a bare amount; the FCF-style "dollars but Unit=''" rule).

// fundamentalRegistryInc2 returns the Increment-2 fundamental compute closures keyed
// by their exact catalog id (Groups 1, 2, 4 + faithful Group 3). It is merged into
// the Computer registry at construction; TestRegistryNoDuplicateIDs guards disjointness
// against the other sub-registries.
func fundamentalRegistryInc2() map[string]computeFn {
	return map[string]computeFn{
		// --- Group 1: income-statement concepts ---
		"fundamental.opm": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := opm(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive revenue or no operating income reported")
			}
		},
		"fundamental.ebit-margin": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ebitMargin(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive revenue or insufficient EBIT inputs")
			}
		},
		"fundamental.pre-tax-margin": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := preTaxMargin(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive revenue or no pre-tax income reported")
			}
		},
		"fundamental.ebitda-margin": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ebitdaMargin(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive revenue or no operating income / D&A reported")
			}
		},
		"fundamental.icr-tie": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := icrTIE(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no interest expense reported or insufficient EBIT inputs")
			}
		},
		"fundamental.roce": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := roce(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "missing assets / current liabilities or insufficient EBIT inputs")
			}
		},
		"fundamental.roic": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := roic(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "missing EBIT, debt, equity, or cash for invested capital")
			}
		},
		"fundamental.op-growth": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := opGrowth(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no prior-year operating income")
			}
		},

		// --- Group 2: current-balance-sheet concepts ---
		"fundamental.current-ratio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := currentRatio(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no current assets or current liabilities reported")
			}
		},
		"fundamental.quick-ratio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := quickRatio(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing current assets / liabilities or no inventory reported")
			}
		},
		"fundamental.cash-ratio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := cashRatio(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no current liabilities or no cash reported")
			}
		},
		"fundamental.inventory-turnover": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := inventoryTurnover(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no inventory or no cost of revenue reported")
			}
		},
		"fundamental.dio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := dio(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no inventory or no cost of revenue reported")
			}
		},
		"fundamental.receivables-turnover": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := receivablesTurnover(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no accounts receivable or non-positive revenue")
			}
		},
		"fundamental.dso": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := dso(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no accounts receivable or non-positive revenue")
			}
		},
		"fundamental.payables-turnover": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := payablesTurnover(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no accounts payable or no cost of revenue reported")
			}
		},
		"fundamental.dpo": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := dpoDays(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no accounts payable or no cost of revenue reported")
			}
		},
		"fundamental.ccc": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ccc(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "missing inventory, receivables, payables, or cost of revenue")
			}
		},
		"fundamental.fixed-asset-turnover": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := fixedAssetTurnover(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no net property, plant & equipment or non-positive revenue")
			}
		},
		"fundamental.current-asset-turnover": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := currentAssetTurnover(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no current assets or non-positive revenue")
			}
		},
		"fundamental.wc-turnover": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := wcTurnover(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "non-positive working capital or non-positive revenue")
			}
		},
		"fundamental.ocf-ratio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ocfRatio(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no current liabilities or no operating cash flow")
			}
		},

		// --- Group 3 (faithful subset): growth + per-share ---
		"fundamental.eps-growth": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := epsGrowth(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no prior-year diluted EPS")
			}
		},
		"fundamental.equity-growth": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := equityGrowth(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no prior period-end equity")
			}
		},
		"fundamental.asset-growth": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := assetGrowth(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no prior period-end total assets")
			}
		},
		"fundamental.gp-growth": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := gpGrowth(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no prior-year gross profit (or gross profit is derived)")
			}
		},
		"fundamental.eps-basic": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := epsBasic(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no basic EPS reported")
			}
		},

		// --- Group 4: debt / EV / capital-structure concepts ---
		"fundamental.lt-debt-ratio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ltDebtRatio(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no long-term debt or non-positive long-term-debt-plus-equity")
			}
		},
		"fundamental.net-gearing": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := netGearing(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive equity or no interest-bearing debt reported")
			}
		},
		"fundamental.cash-st-debt": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := cashToSTDebt(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no short-term debt or no cash reported")
			}
		},
		"fundamental.dtnw": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := dtnw(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "no goodwill/intangibles or non-positive tangible net worth")
			}
		},
		"fundamental.goodwill-equity": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := goodwillToEquity(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no goodwill reported or non-positive equity")
			}
		},
		"fundamental.tbv": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := tangiblePB(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing price/shares or non-positive tangible book value")
			}
		},
		"fundamental.ev": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ev(in.price, in.fund); ok {
				setOK(si, v, unitUSD)
			} else {
				setInsufficient(si, "missing price/shares or no interest-bearing debt reported")
			}
		},
		"fundamental.ev-sales": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := evToSales(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing EV inputs or non-positive revenue")
			}
		},
		"fundamental.ev-fcf": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := evToFCF(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing EV inputs or non-positive free cash flow")
			}
		},
		"fundamental.ev-ebitda": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := evToEBITDA(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing EV inputs or non-positive EBITDA")
			}
		},
		"fundamental.r-d-intensity": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := rdIntensity(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive revenue or no R&D expense reported")
			}
		},
		"fundamental.buyback-yield": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := buybackYield(in.price, in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no buyback reported or missing price/shares")
			}
		},
	}
}
