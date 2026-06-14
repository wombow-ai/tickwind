package indicators

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/store"
)

// This file is the per-stock COMPUTE LAYER. It wires the pure technical /
// fundamental math (technical.go, fundamental.go) to a ticker's fetched data via
// narrow, injected source interfaces, and evaluates EVERY P0 stock-applicable
// catalog id into a StockIndicator (embedding the catalog metadata record). The
// Computer does the I/O once per Compute call (candles, fundamentals, price);
// the per-indicator closures are pure over that fetched data.

// Point is a single dated value. Phase 1 emits latest values only; Point is part
// of the shared contract for the later historical-series phase.
type Point struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// Status values for a computed indicator.
const (
	// StatusOK means the indicator computed a real value.
	StatusOK = "ok"
	// StatusInsufficient means inputs were missing/zero/invalid (no fabrication).
	StatusInsufficient = "insufficient"
	// StatusUnsupported means the indicator is not applicable to US equities
	// (e.g. a crypto-market data feed).
	StatusUnsupported = "unsupported"
)

// Unit strings for a computed value (see the shared contract).
const (
	unitPercent = "%"
	unitRatio   = "ratio"
	unitPrice   = "price"
	unitMult    = "x"
	unitNone    = ""
	// unitUSD is a large dollar amount (free cash flow, enterprise value) rendered
	// compact ("$4.5T") by the frontend — distinct from unitNone so it is not shown
	// as a raw 13-digit number.
	unitUSD = "usd"
)

// StockIndicator is one indicator computed (or attempted) for a single stock. It
// embeds the catalog metadata record so the API response carries the id, names,
// formula, interpretation, etc. alongside the computed value.
type StockIndicator struct {
	Indicator                    // embedded catalog metadata
	Status    string             `json:"status"`           // ok | insufficient | unsupported
	Reason    string             `json:"reason,omitempty"` // why not ok
	Value     *float64           `json:"value,omitempty"`  // headline scalar (nil unless ok)
	Unit      string             `json:"unit,omitempty"`   // % | ratio | price | x | ""
	Extra     map[string]float64 `json:"extra,omitempty"`  // extra lines (MACD signal/hist, BOLL bands, KDJ k/d/j)
}

// computeInput is the per-ticker data bundle passed to each indicator closure.
// Candles are oldest→newest (the K-line ordering); HasFund reports whether XBRL
// fundamentals were available; Price is the latest price (0 when unavailable).
type computeInput struct {
	opens   []float64
	highs   []float64
	lows    []float64
	closes  []float64
	volumes []float64
	fund    edgar.Fundamentals
	hasFund bool
	price   float64
}

// computeFn evaluates one indicator over the fetched per-ticker data, mutating
// the StockIndicator (already seeded with the catalog record + a default
// insufficient status) to ok with a value, or to insufficient with a reason.
type computeFn func(in computeInput, si *StockIndicator)

// cryptoOnlyIDs are the P0 sentiment indicators sourced from crypto-derivatives /
// crypto-flow feeds; they are not applicable to US equities and are always
// reported as unsupported.
var cryptoOnlyIDs = map[string]struct{}{
	"sentiment.crypto-fear-greed":      {},
	"sentiment.fr":                     {}, // funding rate
	"sentiment.liquidations":           {},
	"sentiment.lsr":                    {}, // long/short ratio
	"sentiment.oi":                     {}, // open interest
	"sentiment.spot-btc-etf-net-flows": {},
	"sentiment.spot-eth-etf-net-flows": {},
}

const cryptoUnsupportedReason = "crypto-market data source; not applicable to US equities"

// marketContextIDs are the P0 sentiment indicators served from the market-wide
// context (VIX, CNN Fear & Greed) rather than per-stock data.
const (
	idVIX       = "sentiment.cboe-volatility-index"
	idFearGreed = "sentiment.cnn-fear-greed"
)

// Default technical parameters (used when the catalog default_params is absent;
// where present, default_params is honored — see paramPeriod / paramsMACD).
const (
	defaultSMAPeriod = 20
	// EMA headlines the primary/fast period from the catalog's {periods:[12,26]}
	// hint (12 ∈ the list, unlike 20), so the served value is dataset-faithful.
	defaultEMAPeriod  = 12
	defaultRSIPeriod  = 14
	defaultBollPeriod = 20
	defaultBollMult   = 2.0
	defaultATRPeriod  = 14
	defaultMACDFast   = 12
	defaultMACDSlow   = 26
	defaultMACDSignal = 9
	defaultStochN     = 9
	defaultStochSlowK = 3
	defaultStochSlowD = 3
)

// technicalRegistry maps each P0 technical indicator id to its compute closure.
// The closures read parameters from the indicator's catalog default_params where
// present, falling back to the documented defaults above.
func technicalRegistry() map[string]computeFn {
	return map[string]computeFn{
		"technical.sma-ma": func(in computeInput, si *StockIndicator) {
			period := paramFirstPeriod(si.DefaultParams, defaultSMAPeriod)
			if v, ok := sma(in.closes, period); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for the moving-average window")
			}
		},
		"technical.ema": func(in computeInput, si *StockIndicator) {
			period := paramFirstPeriod(si.DefaultParams, defaultEMAPeriod)
			if v, ok := ema(in.closes, period); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "not enough daily closes for the moving-average window")
			}
		},
		"technical.rsi": func(in computeInput, si *StockIndicator) {
			period := paramPeriod(si.DefaultParams, defaultRSIPeriod)
			if v, ok := rsiWilder(in.closes, period); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "need more daily closes for RSI")
			}
		},
		"technical.macd": func(in computeInput, si *StockIndicator) {
			fast, slow, signal := paramsMACD(si.DefaultParams)
			if v, ok := macd(in.closes, fast, slow, signal); ok {
				setOK(si, v.Line, unitNone)
				si.Extra = map[string]float64{"signal": v.Signal, "hist": v.Histogram}
			} else {
				setInsufficient(si, "need more daily closes for MACD")
			}
		},
		"technical.boll": func(in computeInput, si *StockIndicator) {
			period, mult := paramsBoll(si.DefaultParams)
			if v, ok := bollinger(in.closes, period, mult); ok {
				setOK(si, v.Middle, unitPrice)
				si.Extra = map[string]float64{"upper": v.Upper, "mid": v.Middle, "lower": v.Lower}
			} else {
				setInsufficient(si, "need more daily closes for Bollinger Bands")
			}
		},
		"technical.atr": func(in computeInput, si *StockIndicator) {
			if v, ok := atrWilder(in.highs, in.lows, in.closes, defaultATRPeriod); ok {
				setOK(si, v, unitPrice)
			} else {
				setInsufficient(si, "need more daily bars for ATR")
			}
		},
		"technical.stochastic-kdj": func(in computeInput, si *StockIndicator) {
			n, slowK, slowD := paramsStoch(si.DefaultParams)
			if v, ok := stochasticKDJ(in.highs, in.lows, in.closes, n, slowK, slowD); ok {
				setOK(si, v.K, unitNone)
				si.Extra = map[string]float64{"k": v.K, "d": v.D, "j": v.J}
			} else {
				setInsufficient(si, "need more daily bars for KDJ")
			}
		},
		"technical.vwap": func(in computeInput, si *StockIndicator) {
			// VWAP resets each session (catalog: "resets daily"; its interpretation
			// is intraday support/resistance). With only daily bars a faithful
			// intraday VWAP can't be computed — a multi-day volume-weighted mean
			// would diverge materially from the indicator's meaning — so report
			// insufficient rather than ship a mislabeled number. The pure vwap()
			// helper stays for when intraday bars become available.
			_ = in
			setInsufficient(si, "intraday VWAP needs intraday bars; only daily data is available")
		},
		"technical.vol": func(in computeInput, si *StockIndicator) {
			if v, ok := latestVolume(in.volumes); ok {
				setOK(si, v, unitNone)
			} else {
				setInsufficient(si, "no volume bars")
			}
		},
	}
}

// fundamentalRegistry maps each P0 fundamental indicator id to its compute
// closure. Each requires XBRL fundamentals (and some a live price).
func fundamentalRegistry() map[string]computeFn {
	return map[string]computeFn{
		"fundamental.pe-ttm": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := peTTM(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "non-positive EPS (loss) or no price")
			}
		},
		"fundamental.pb": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := pb(in.price, in.fund); ok {
				setOK(si, v, unitMult)
			} else {
				setInsufficient(si, "missing price, shares, or equity")
			}
		},
		"fundamental.roe": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := roe(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive equity")
			}
		},
		"fundamental.npm": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := npm(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-positive revenue")
			}
		},
		"fundamental.gpm": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := gpm(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no gross profit reported or non-positive revenue")
			}
		},
		"fundamental.revenue-growth-yoy": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := revenueGrowthYoY(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no prior-year revenue")
			}
		},
		"fundamental.earnings-growth-yoy": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := earningsGrowthYoY(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no prior-year net income")
			}
		},
		"fundamental.fcf": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := fcf(in.fund); ok {
				setOK(si, v, unitUSD)
			} else {
				setInsufficient(si, "no operating cash flow reported")
			}
		},
		"fundamental.dy": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := dividendYield(in.price, in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "non-dividend payer or no price")
			}
		},
		"fundamental.debt-to-asset": func(in computeInput, si *StockIndicator) {
			if !in.hasFund {
				setInsufficient(si, "no SEC fundamentals available")
				return
			}
			if v, ok := debtToAsset(in.fund); ok {
				setOK(si, v, unitPercent)
			} else {
				setInsufficient(si, "no total assets reported")
			}
		},
	}
}

// setOK marks the indicator ok with a value + unit.
func setOK(si *StockIndicator, v float64, unit string) {
	val := v
	si.Status = StatusOK
	si.Value = &val
	si.Unit = unit
	si.Reason = ""
}

// setInsufficient marks the indicator insufficient with a verbatim reason and
// clears any value/unit (no fabrication on a missing input).
func setInsufficient(si *StockIndicator, reason string) {
	si.Status = StatusInsufficient
	si.Reason = reason
	si.Value = nil
	si.Unit = ""
}

// OHLCVSource provides a ticker's daily OHLCV candles (oldest→newest).
type OHLCVSource interface {
	DailyCandles(ctx context.Context, ticker string) ([]store.Candle, error)
}

// FundamentalsProvider provides a ticker's XBRL-derived fundamentals.
type FundamentalsProvider interface {
	Fundamentals(ctx context.Context, ticker string) (edgar.Fundamentals, error)
}

// PriceProvider provides a ticker's latest price (ok=false when unavailable).
type PriceProvider interface {
	Price(ctx context.Context, ticker string) (float64, bool)
}

// MarketContextProvider provides market-wide sentiment context. Any method may
// report ok=false; the whole provider may be nil.
type MarketContextProvider interface {
	VIX() (float64, bool)
	FearGreed() (score int, label string, ok bool)
}

// Computer evaluates the P0 stock-applicable catalog for a ticker. Its sources
// are injected so the package stays decoupled and testable; any source may be
// nil, in which case the dependent indicators degrade to insufficient.
type Computer struct {
	catalog *Catalog
	ohlcv   OHLCVSource
	fund    FundamentalsProvider
	price   PriceProvider
	market  MarketContextProvider

	registry map[string]computeFn // implemented per-stock indicators
}

// NewComputer builds a Computer from the catalog and the (possibly nil) data
// sources. The P0 technical + fundamental registries and the expanded
// technicalRegistryMore / fundamentalRegistryMore / fundamentalRegistryInc2 sets
// (design §1.1/§1.2) are merged once at construction. The sub-registries must have
// disjoint ids — a double-registered id would silently shadow one closure;
// TestRegistryNoDuplicateIDs guards against that.
func NewComputer(catalog *Catalog, ohlcv OHLCVSource, fund FundamentalsProvider, price PriceProvider, market MarketContextProvider) *Computer {
	reg := technicalRegistry()
	for id, fn := range fundamentalRegistry() {
		reg[id] = fn
	}
	for id, fn := range technicalRegistryMore() {
		reg[id] = fn
	}
	for id, fn := range fundamentalRegistryMore() {
		reg[id] = fn
	}
	for id, fn := range fundamentalRegistryInc2() {
		reg[id] = fn
	}
	for id, fn := range fundamentalRegistryInc3() {
		reg[id] = fn
	}
	return &Computer{
		catalog:  catalog,
		ohlcv:    ohlcv,
		fund:     fund,
		price:    price,
		market:   market,
		registry: reg,
	}
}

// computedIDs returns the catalog records this Computer evaluates: every id with a
// registered closure, plus the market-context and crypto-only ids (whose dispatch
// lives in evaluate). Iterating the registry — not a priority filter — means adding
// a closure is the ONLY step needed to surface a new indicator, and unimplemented
// ids are simply absent (never shown as a broken "not computed" row). Catalog order
// is preserved so the response keeps the dataset's grouping.
func (c *Computer) computedIDs() []Indicator {
	out := make([]Indicator, 0, len(c.registry)+len(cryptoOnlyIDs)+2)
	for _, rec := range c.catalog.All() { // catalog order preserved
		_, registered := c.registry[rec.ID]
		_, crypto := cryptoOnlyIDs[rec.ID]
		isCtx := rec.ID == idVIX || rec.ID == idFearGreed
		if registered || crypto || isCtx {
			out = append(out, rec)
		}
	}
	return out
}

// StockIndicatorsResult is the value the api handler marshals to the wire
// contract. AsOf is the newest underlying data date (may be ""); VIX and
// FearGreed are nil/empty when unavailable. Indicators are sorted ok →
// insufficient → unsupported.
type StockIndicatorsResult struct {
	Ticker     string
	AsOf       string
	VIX        *float64
	FearGreed  *FearGreed
	Indicators []StockIndicator
}

// FearGreed is the CNN Fear & Greed reading for the market-context block.
type FearGreed struct {
	Score int    `json:"score"`
	Label string `json:"label"`
}

// StockIndicators computes every implemented stock-applicable indicator for a
// ticker (the registered closures plus the market-context and crypto-only ids —
// see computedIDs), fetching candles, fundamentals, and price once. Unimplemented
// catalog ids are absent from the result, not surfaced as broken rows. It never
// errors on missing data: an unavailable source degrades the dependent indicators
// to insufficient (graceful — a name with bars but no XBRL still returns the
// technicals). The returned Indicators are sorted ok → insufficient →
// unsupported.
func (c *Computer) StockIndicators(ctx context.Context, ticker string) StockIndicatorsResult {
	in, asOf := c.gather(ctx, ticker)

	records := c.computedIDs()
	out := make([]StockIndicator, 0, len(records))
	for _, rec := range records {
		si := StockIndicator{Indicator: rec, Status: StatusInsufficient, Reason: "not computed"}
		c.evaluate(rec, in, &si)
		out = append(out, si)
	}
	sortByStatus(out)

	res := StockIndicatorsResult{Ticker: ticker, AsOf: asOf, Indicators: out}
	if c.market != nil {
		if v, ok := c.market.VIX(); ok {
			vv := v
			res.VIX = &vv
		}
		if score, label, ok := c.market.FearGreed(); ok {
			res.FearGreed = &FearGreed{Score: score, Label: label}
		}
	}
	return res
}

// gather fetches the per-ticker data once and assembles the computeInput plus the
// newest underlying data date (the latest candle's date, else the fundamentals
// AsOf). Missing sources leave the corresponding fields zero/empty.
func (c *Computer) gather(ctx context.Context, ticker string) (computeInput, string) {
	var in computeInput
	var asOf string

	if c.ohlcv != nil {
		if candles, err := c.ohlcv.DailyCandles(ctx, ticker); err == nil && len(candles) > 0 {
			in.opens = make([]float64, len(candles))
			in.highs = make([]float64, len(candles))
			in.lows = make([]float64, len(candles))
			in.closes = make([]float64, len(candles))
			in.volumes = make([]float64, len(candles))
			for i, cd := range candles {
				in.opens[i] = cd.Open
				in.highs[i] = cd.High
				in.lows[i] = cd.Low
				in.closes[i] = cd.Close
				in.volumes[i] = cd.Volume
			}
			asOf = candles[len(candles)-1].Time.Format("2006-01-02")
		}
	}
	if c.fund != nil {
		if f, err := c.fund.Fundamentals(ctx, ticker); err == nil && f.HasData() {
			in.fund = f
			in.hasFund = true
			if asOf == "" {
				asOf = f.AsOf
			}
		}
	}
	if c.price != nil {
		if p, ok := c.price.Price(ctx, ticker); ok {
			in.price = p
		}
	}
	return in, asOf
}

// evaluate fills si for one catalog record: a registered per-stock indicator runs
// its closure; the market-context ids read VIX / Fear & Greed; the crypto-only
// ids are unsupported. Any unrecognized P0 id stays insufficient ("not computed")
// — the registry-coverage test guarantees this never happens in practice.
func (c *Computer) evaluate(rec Indicator, in computeInput, si *StockIndicator) {
	if fn, ok := c.registry[rec.ID]; ok {
		fn(in, si)
		return
	}
	switch rec.ID {
	case idVIX:
		if c.market != nil {
			if v, ok := c.market.VIX(); ok {
				setOK(si, v, unitNone)
				return
			}
		}
		setInsufficient(si, "no market VIX available")
	case idFearGreed:
		if c.market != nil {
			if score, _, ok := c.market.FearGreed(); ok {
				setOK(si, float64(score), unitNone)
				return
			}
		}
		setInsufficient(si, "no market Fear & Greed available")
	default:
		if _, isCrypto := cryptoOnlyIDs[rec.ID]; isCrypto {
			si.Status = StatusUnsupported
			si.Reason = cryptoUnsupportedReason
			si.Value = nil
			si.Unit = ""
		}
	}
}

// statusRank orders ok < insufficient < unsupported for the response sort.
var statusRank = map[string]int{StatusOK: 0, StatusInsufficient: 1, StatusUnsupported: 2}

// sortByStatus orders indicators ok → insufficient → unsupported, stable within a
// status group (preserving catalog order).
func sortByStatus(sis []StockIndicator) {
	sort.SliceStable(sis, func(i, j int) bool {
		return statusRank[sis[i].Status] < statusRank[sis[j].Status]
	})
}

// --- catalog default_params parsing helpers ---

// paramPeriod reads {"period": n} from raw default_params, returning def when
// absent or malformed.
func paramPeriod(raw json.RawMessage, def int) int {
	if len(raw) == 0 {
		return def
	}
	var p struct {
		Period int `json:"period"`
	}
	if err := json.Unmarshal(raw, &p); err == nil && p.Period > 0 {
		return p.Period
	}
	return def
}

// paramFirstPeriod reads the first usable period from either {"period": n} or
// {"periods": [...]} (the SMA/EMA catalog uses a periods list), returning def when
// none is present. A periods list is a render hint (multiple MA lines); the
// headline scalar stays that indicator's documented default window (def) so the
// latest scalar is well-defined rather than picking an arbitrary list entry.
func paramFirstPeriod(raw json.RawMessage, def int) int {
	if len(raw) == 0 {
		return def
	}
	var single struct {
		Period int `json:"period"`
	}
	if err := json.Unmarshal(raw, &single); err == nil && single.Period > 0 {
		return single.Period
	}
	// A periods list is a render hint (multiple MA lines); the headline scalar
	// stays the documented default window rather than picking an arbitrary one.
	return def
}

// paramsMACD reads {"fast","slow","signal"} from raw, falling back per field.
func paramsMACD(raw json.RawMessage) (fast, slow, signal int) {
	fast, slow, signal = defaultMACDFast, defaultMACDSlow, defaultMACDSignal
	if len(raw) == 0 {
		return
	}
	var p struct {
		Fast, Slow, Signal int
	}
	if err := json.Unmarshal(raw, &p); err == nil {
		if p.Fast > 0 {
			fast = p.Fast
		}
		if p.Slow > 0 {
			slow = p.Slow
		}
		if p.Signal > 0 {
			signal = p.Signal
		}
	}
	return
}

// paramsBoll reads {"period","stddev"} from raw, falling back per field.
func paramsBoll(raw json.RawMessage) (period int, mult float64) {
	period, mult = defaultBollPeriod, defaultBollMult
	if len(raw) == 0 {
		return
	}
	var p struct {
		Period int     `json:"period"`
		StdDev float64 `json:"stddev"`
	}
	if err := json.Unmarshal(raw, &p); err == nil {
		if p.Period > 0 {
			period = p.Period
		}
		if p.StdDev > 0 {
			mult = p.StdDev
		}
	}
	return
}

// paramsStoch reads {"k","d"} from raw (the KDJ catalog uses k=window, d=smoothing,
// j=bool), falling back per field. The slow-K and slow-D smoothing both default to
// the documented 3.
func paramsStoch(raw json.RawMessage) (n, slowK, slowD int) {
	n, slowK, slowD = defaultStochN, defaultStochSlowK, defaultStochSlowD
	if len(raw) == 0 {
		return
	}
	var p struct {
		K int `json:"k"`
		D int `json:"d"`
	}
	if err := json.Unmarshal(raw, &p); err == nil {
		if p.K > 0 {
			n = p.K
		}
		if p.D > 0 {
			slowK, slowD = p.D, p.D
		}
	}
	return
}
