package indicators

// This file holds the Group-0 FUNDAMENTAL ratio closures (design §1.2 Group 0):
// indicators computable today from the EXISTING edgar.Fundamentals fields plus a
// live price — NO new XBRL extraction. Each closure guards in.hasFund, reads the
// price from in.price, calls the matching pure ratio in fundamental.go, and either
// setOK with the right Unit or setInsufficient (no Value). Per the no-fabrication
// rule a value is emitted ONLY when every input is present and the denominator is
// valid; an absent or zero/negative input becomes insufficient, never a 0.
//
// Unit choices (see the shared contract): margins / yields / ratio-as-percent →
// "%"; turnover and price multiples → "x"; USD dollar amounts (market cap, capex)
// and per-share figures (BVPS, SPS, DPS, CFPS, FCFPS, diluted EPS, stated OCF) →
// "" (a bare currency amount).
//
// The map is keyed by the EXACT Group-0 catalog id; compute.go merges it into the
// Computer registry so each id participates automatically.

// fundamentalRegistryMore returns the Group-0 fundamental compute closures keyed
// by their exact catalog id. It is a sibling of fundamentalRegistry() (the P0 set)
// and is merged into the Computer registry at construction.
func fundamentalRegistryMore() map[string]computeFn {
	return map[string]computeFn{
		"fundamental.market-cap": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := marketCap(in.price, in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "missing price or shares outstanding")
			}
		},
		"fundamental.eps-diluted": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := epsDiluted(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no diluted EPS reported")
			}
		},
		"fundamental.bvps": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := bvps(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "non-positive equity or no shares outstanding")
			}
		},
		"fundamental.sps": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := sps(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "non-positive revenue or no shares outstanding")
			}
		},
		"fundamental.ps": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ps(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing price, shares, or revenue")
			}
		},
		"fundamental.d-e": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := debtToEquity(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "non-positive equity")
			}
		},
		"fundamental.equity-multiplier": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := equityMultiplier(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "non-positive equity or no total assets")
			}
		},
		"fundamental.roa": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := roa(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no total assets reported")
			}
		},
		"fundamental.gp-a": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := gpToAssets(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no gross profit reported or no total assets")
			}
		},
		"fundamental.total-asset-turnover": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := assetTurnover(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "non-positive revenue or no total assets")
			}
		},
		"fundamental.ocf-ni": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ocfToNI(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "non-positive net income or no operating cash flow")
			}
		},
		"fundamental.ocf-cfo": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := ocfCFO(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no operating cash flow reported")
			}
		},
		"fundamental.fcf-conversion": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := fcfConversion(in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "non-positive net income or no operating cash flow")
			}
		},
		"fundamental.capex": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := capex(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no capital expenditure reported")
			}
		},
		"fundamental.capex-sales": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := capexToSales(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive revenue or no capital expenditure")
			}
		},
		"fundamental.fcf-yield": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := fcfYield(in.price, in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "missing price/shares or no operating cash flow")
			}
		},
		"fundamental.pcf": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := pcf(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing price/shares or non-positive operating cash flow")
			}
		},
		"fundamental.cfps": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := cfps(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no shares outstanding or no operating cash flow")
			}
		},
		"fundamental.fcfps": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := fcfps(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no shares outstanding or no operating cash flow")
			}
		},
		"fundamental.payout-ratio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := payoutRatio(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive net income or a non-dividend payer")
			}
		},
		"fundamental.retention-ratio": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := retentionRatio(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive net income or a non-dividend payer")
			}
		},
		"fundamental.dps": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := dps(in.fund); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no shares outstanding or a non-dividend payer")
			}
		},
		"fundamental.sgr": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := sgr(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive equity or insufficient payout inputs")
			}
		},
		"fundamental.accruals": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := accruals(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no total assets or no operating cash flow")
			}
		},
		"fundamental.tobin-s-q": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := tobinsQ(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing price/shares or no total assets")
			}
		},
		"fundamental.pe-lyr": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := peLYR(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing price/shares or non-positive prior-year net income")
			}
		},
	}
}
