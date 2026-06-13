package research

import (
	"context"
	"net/url"
	"strings"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

// IndicatorCalc computes the full P0 stock-applicable indicator set for a ticker.
// It is satisfied by *indicators.Computer and never errors (missing data degrades
// to insufficient indicators).
type IndicatorCalc interface {
	// StockIndicators returns the computed indicator set; missing inputs yield
	// insufficient indicators rather than an error.
	StockIndicators(ctx context.Context, ticker string) indicators.StockIndicatorsResult
}

// FundProvider returns a ticker's XBRL-derived fundamentals (SEC companyfacts).
type FundProvider interface {
	// Fundamentals returns the latest reported fundamentals, or an error for an
	// unknown/non-US ticker or fetch failure.
	Fundamentals(ctx context.Context, ticker string) (edgar.Fundamentals, error)
}

// QuoteProvider returns a ticker's latest (delayed) quote. The implementation
// reads the polled quote first and falls back to an on-demand fetch, mirroring
// getFundamentals; ok=false when no price is available.
type QuoteProvider interface {
	// Quote returns the latest quote and whether one was found.
	Quote(ctx context.Context, ticker string) (store.Quote, bool)
}

// Sources is the narrow slice of in-process providers the assembler needs. Each
// is an interface so the assembler is unit-testable with fakes, and each is
// nil-safe (a nil provider omits its dependent facts/section). For P0 these are
// the three richest already-wired sources.
type Sources struct {
	Indicators   IndicatorCalc
	Fundamentals FundProvider
	Quote        QuoteProvider
}

// Source labels and URLs set in Go from the known provenance. The LLM never
// writes a URL — provenance is authoritative here.
const (
	srcSECXBRL      = "SEC XBRL"
	srcSECQuote     = "SEC XBRL × delayed quote"
	srcDelayedQuote = "delayed quote"
	srcIndicators   = "computed from daily bars"
)

const secEdgarLabel = "SEC EDGAR · companyfacts"

// secEdgarURL builds the EDGAR company-search deep-link for a ticker.
func secEdgarURL(ticker string) string {
	return "https://www.sec.gov/cgi-bin/browse-edgar?action=getcompany&ticker=" +
		url.QueryEscape(strings.ToUpper(ticker)) + "&type=10-K&dateb=&owner=include&count=40"
}

// indicatorFactSpec maps an indicator id to the Fact metadata the report uses.
// Unit overrides the indicator's reported unit where the indicator set is unit-
// agnostic but the report needs richer rendering (e.g. fundamental.fcf is "" but
// represents DOLLARS → render compact USD, sign-aware).
type indicatorFactSpec struct {
	key       string
	labelZH   string
	labelEN   string
	unit      string // "" → use the indicator's reported unit; else override
	source    string
	sourceURL bool // when true, attach the SEC EDGAR deep-link
	asOfFund  bool // when true, stamp AsOf from the fundamentals period/date
}

// valuationSpecs are the §1.1 valuation facts sourced from the indicator set.
// MarketCap and Price are injected separately (price-derived / quote). Per the
// de-dup rule PE/PB/DY come ONLY from the indicator set.
var valuationSpecs = map[string]indicatorFactSpec{
	"fundamental.pe-ttm": {key: "pe", labelZH: "市盈率(P/E)", labelEN: "P/E (TTM)", source: srcSECQuote, sourceURL: true, asOfFund: true},
	"fundamental.pb":     {key: "pb", labelZH: "市净率(P/B)", labelEN: "P/B", source: srcSECQuote, sourceURL: true, asOfFund: true},
	"fundamental.dy":     {key: "dy", labelZH: "股息率", labelEN: "Dividend Yield", source: srcSECQuote, sourceURL: true, asOfFund: true},
}

// fundamentalSpecs are the §1.2 fundamentals facts sourced from the indicator
// set. Revenue / NetIncome / EPSDiluted are injected separately (from edgar
// directly). fundamental.fcf is DOLLARS despite a "" unit → unitUSD override.
var fundamentalSpecs = map[string]indicatorFactSpec{
	"fundamental.revenue-growth-yoy":  {key: "revenue_growth_yoy", labelZH: "营收同比增长", labelEN: "Revenue Growth (YoY)", source: srcSECXBRL, sourceURL: true, asOfFund: true},
	"fundamental.earnings-growth-yoy": {key: "earnings_growth_yoy", labelZH: "盈利同比增长", labelEN: "Earnings Growth (YoY)", source: srcSECXBRL, sourceURL: true, asOfFund: true},
	"fundamental.gpm":                 {key: "gross_margin", labelZH: "毛利率", labelEN: "Gross Margin", source: srcSECXBRL, sourceURL: true, asOfFund: true},
	"fundamental.npm":                 {key: "net_margin", labelZH: "净利率", labelEN: "Net Margin", source: srcSECXBRL, sourceURL: true, asOfFund: true},
	"fundamental.roe":                 {key: "roe", labelZH: "净资产收益率(ROE)", labelEN: "ROE", source: srcSECXBRL, sourceURL: true, asOfFund: true},
	"fundamental.fcf":                 {key: "fcf", labelZH: "自由现金流", labelEN: "Free Cash Flow", unit: unitUSD, source: srcSECXBRL, sourceURL: true, asOfFund: true},
	"fundamental.debt-to-asset":       {key: "debt_to_asset", labelZH: "资产负债率", labelEN: "Debt / Assets", source: srcSECXBRL, sourceURL: true, asOfFund: true},
}

// technicalSpecs are the §1.3 technical facts sourced from the indicator set. Any
// Extra lines (MACD signal/hist, BOLL bands, KDJ k/d/j) ride along in
// extraFacts. technical.vwap is intentionally EXCLUDED (always insufficient with
// daily-only bars).
var technicalSpecs = map[string]indicatorFactSpec{
	"technical.rsi":            {key: "rsi", labelZH: "RSI(14)", labelEN: "RSI (14)", source: srcIndicators},
	"technical.macd":           {key: "macd", labelZH: "MACD", labelEN: "MACD", source: srcIndicators},
	"technical.sma-ma":         {key: "sma", labelZH: "均线(SMA20)", labelEN: "SMA (20)", source: srcIndicators},
	"technical.ema":            {key: "ema", labelZH: "指数均线(EMA12)", labelEN: "EMA (12)", source: srcIndicators},
	"technical.boll":           {key: "boll", labelZH: "布林带(中轨)", labelEN: "Bollinger (mid)", source: srcIndicators},
	"technical.atr":            {key: "atr", labelZH: "ATR(14)", labelEN: "ATR (14)", source: srcIndicators},
	"technical.stochastic-kdj": {key: "kdj", labelZH: "KDJ(%K)", labelEN: "KDJ (%K)", source: srcIndicators},
	"technical.vol":            {key: "volume", labelZH: "成交量", labelEN: "Volume", source: srcIndicators},
}

// Assemble builds the data-only fact sheet for a ticker with NO LLM. It calls
// src.Indicators.StockIndicators exactly once, walks the returned indicators with
// a strict status gate (a value is read only when Status==StatusOK), and emits
// exactly three sections (valuation / fundamentals / technical), each omitted
// when it has zero ok facts. It never errors and never fabricates: insufficient
// indicators become a "数据不足" fact with the verbatim reason and NO Raw number;
// the crypto/unsupported ids are skipped entirely. Numbers come from a single
// source per metric (PE/PB/DY/margins/growth/ROE/FCF/debt from the indicator set,
// MarketCap from price × Shares, Revenue/NetIncome/EPS from edgar).
func Assemble(ctx context.Context, ticker string, src Sources) FactSheet {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	fs := FactSheet{Ticker: ticker, Disclaimer: Disclaimer}

	// One indicator computation for the whole report.
	var indResult indicators.StockIndicatorsResult
	if src.Indicators != nil {
		indResult = src.Indicators.StockIndicators(ctx, ticker)
	}
	fs.AsOf = indResult.AsOf

	// Index the computed indicators by id for O(1) spec lookup.
	byID := make(map[string]indicators.StockIndicator, len(indResult.Indicators))
	for _, si := range indResult.Indicators {
		byID[si.ID] = si
	}

	// Fundamentals (nil-safe → no fundamentals-derived facts / section).
	var fund edgar.Fundamentals
	var haveFund bool
	if src.Fundamentals != nil {
		if f, err := src.Fundamentals.Fundamentals(ctx, ticker); err == nil && f.HasData() {
			fund, haveFund = f, true
			if fs.Name == "" {
				fs.Name = f.Name
			}
			if fs.AsOf == "" {
				fs.AsOf = f.AsOf
			}
		}
	}

	// Latest delayed quote → price fact + price label.
	var price float64
	if src.Quote != nil {
		if q, ok := src.Quote.Quote(ctx, ticker); ok && q.Price > 0 {
			price = q.Price
			fs.PriceLabel = priceLabel(q)
		}
	}

	secURL := secEdgarURL(ticker)
	fundAsOf := fund.AsOf
	if fund.Period != "" {
		fundAsOf = fund.Period // "FY2024" reads better as the freshness stamp
	}

	fundCitations := []Citation{{Label: secEdgarLabel, Anchor: "#fundamentals", URL: secURL}}

	// --- §1.1 估值 / Valuation ---
	valuation := SectionFacts{Key: "valuation", TitleZH: "估值", TitleEN: "Valuation", Citations: fundCitations}
	// MarketCap = price × Shares (the SOLE source for market cap — never the
	// indicator set). Emitted only when both are present (never 0-as-value).
	if price > 0 && haveFund && fund.Shares > 0 {
		mc := price * float64(fund.Shares)
		valuation.Facts = append(valuation.Facts, Fact{
			Key: "market_cap", LabelZH: "市值", LabelEN: "Market Cap",
			Value: formatValue(&mc, unitUSD), Raw: &mc, Unit: unitUSD,
			Status: StatusOK, Source: srcSECQuote, SourceURL: secURL, AsOf: fundAsOf,
		})
	}
	// Price (delayed, labeled) — the SOLE source is the quote.
	if price > 0 {
		p := price
		valuation.Facts = append(valuation.Facts, Fact{
			Key: "price", LabelZH: "股价", LabelEN: "Price",
			Value: formatValue(&p, unitPrice), Raw: &p, Unit: unitPrice,
			Status: StatusOK, Source: srcDelayedQuote,
		})
	}
	valuation.Facts = append(valuation.Facts, factsFromSpecs(valuationIDOrder, valuationSpecs, byID, fundAsOf, secURL)...)
	addSection(&fs, valuation)

	// --- §1.2 基本面 / Fundamentals ---
	fundamentals := SectionFacts{Key: "fundamentals", TitleZH: "基本面", TitleEN: "Fundamentals", Citations: fundCitations}
	if haveFund {
		rev := fund.Revenue
		fundamentals.Facts = append(fundamentals.Facts, Fact{
			Key: "revenue", LabelZH: "营业收入", LabelEN: "Revenue",
			Value: formatValue(&rev, unitUSD), Raw: &rev, Unit: unitUSD,
			Status: StatusOK, Source: srcSECXBRL, SourceURL: secURL, AsOf: fundAsOf,
		})
		ni := fund.NetIncome // can be negative (loss) — format the sign.
		fundamentals.Facts = append(fundamentals.Facts, Fact{
			Key: "net_income", LabelZH: "净利润", LabelEN: "Net Income",
			Value: formatValue(&ni, unitUSD), Raw: &ni, Unit: unitUSD,
			Status: StatusOK, Source: srcSECXBRL, SourceURL: secURL, AsOf: fundAsOf,
		})
		eps := fund.EPSDiluted
		fundamentals.Facts = append(fundamentals.Facts, Fact{
			Key: "eps_diluted", LabelZH: "摊薄每股收益", LabelEN: "Diluted EPS",
			Value: formatPrice(eps), Raw: &eps, Unit: unitPrice,
			Status: StatusOK, Source: srcSECXBRL, SourceURL: secURL, AsOf: fundAsOf,
		})
	}
	fundamentals.Facts = append(fundamentals.Facts, factsFromSpecs(fundamentalIDOrder, fundamentalSpecs, byID, fundAsOf, secURL)...)
	addSection(&fs, fundamentals)

	// --- §1.3 技术面 / Technical ---
	technical := SectionFacts{
		Key: "technical", TitleZH: "技术面", TitleEN: "Technical",
		Citations: []Citation{{Label: "Tickwind · daily indicators", Anchor: "#indicators"}},
	}
	for _, id := range technicalIDOrder {
		spec, ok := technicalSpecs[id]
		if !ok {
			continue
		}
		si, present := byID[id]
		if !present {
			continue
		}
		f, emit := factFromIndicator(spec, si, "", "")
		if !emit {
			continue
		}
		technical.Facts = append(technical.Facts, f)
		technical.Facts = append(technical.Facts, extraFacts(spec.key, si)...)
	}
	addSection(&fs, technical)

	return fs
}

// technicalIDOrder fixes the display order of the technical facts (the map is
// unordered). It lists every id in technicalSpecs.
var technicalIDOrder = []string{
	"technical.rsi", "technical.macd", "technical.sma-ma", "technical.ema",
	"technical.boll", "technical.atr", "technical.stochastic-kdj", "technical.vol",
}

// valuationIDOrder / fundamentalIDOrder fix the display order for their spec maps.
var valuationIDOrder = []string{"fundamental.pe-ttm", "fundamental.pb", "fundamental.dy"}

var fundamentalIDOrder = []string{
	"fundamental.revenue-growth-yoy", "fundamental.earnings-growth-yoy",
	"fundamental.gpm", "fundamental.npm", "fundamental.roe",
	"fundamental.fcf", "fundamental.debt-to-asset",
}

// factsFromSpecs builds the facts for a spec map in the given display order,
// status-gating each indicator. Unsupported (crypto) ids never reach here (they
// are not in the spec maps), so the only states are ok and insufficient.
func factsFromSpecs(order []string, specs map[string]indicatorFactSpec, byID map[string]indicators.StockIndicator, fundAsOf, secURL string) []Fact {
	out := make([]Fact, 0, len(order))
	for _, id := range order {
		spec := specs[id]
		si, present := byID[id]
		if !present {
			continue
		}
		if f, emit := factFromIndicator(spec, si, fundAsOf, secURL); emit {
			out = append(out, f)
		}
	}
	return out
}

// factFromIndicator converts one computed indicator into a Fact per its spec,
// applying the strict status gate. Returns (Fact, true) for an ok or insufficient
// indicator and (zero, false) for any other status (e.g. an unsupported crypto id
// that somehow appeared) so the caller skips it. A unit override on the spec wins
// (e.g. FCF's dollars); otherwise the indicator's reported unit is used.
func factFromIndicator(spec indicatorFactSpec, si indicators.StockIndicator, fundAsOf, secURL string) (Fact, bool) {
	f := Fact{
		Key:     spec.key,
		LabelZH: spec.labelZH,
		LabelEN: spec.labelEN,
		Source:  spec.source,
	}
	if spec.sourceURL {
		f.SourceURL = secURL
	}
	if spec.asOfFund {
		f.AsOf = fundAsOf
	}
	switch si.Status {
	case indicators.StatusOK:
		unit := spec.unit
		if unit == "" {
			unit = si.Unit
		}
		f.Status = StatusOK
		f.Unit = unit
		f.Raw = copyFloat(si.Value)
		f.Value = formatValue(si.Value, unit)
		// A P/E whose multiple comes back non-positive (defensive: the indicator
		// reports insufficient for a loss, but guard the loss case explicitly).
		if spec.key == "pe" && si.Value != nil && *si.Value <= 0 {
			f.Value = loss
		}
		return f, true
	case indicators.StatusInsufficient:
		f.Status = StatusInsufficient
		f.Reason = si.Reason
		// A *genuine* loss-maker P/E reads "亏损"; but an insufficient P/E caused by
		// MISSING fundamentals — e.g. an ETF/ADR/foreign name with price bars but no
		// SEC XBRL ("no SEC fundamentals available") — must NOT assert a loss, or the
		// valuation section (kept alive by the ok price fact) ships a fabricated
		// "loss-making" claim. Gate on the indicator's reason, which names the
		// non-positive-EPS/loss cause explicitly; otherwise show the placeholder.
		if spec.key == "pe" && (strings.Contains(si.Reason, "EPS") || strings.Contains(si.Reason, "loss")) {
			f.Value = loss
		} else {
			f.Value = "数据不足"
		}
		f.Raw = nil // NEVER a number for an absent field.
		return f, true
	default:
		// Unsupported (crypto) ids are skipped entirely.
		return Fact{}, false
	}
}

// extraFacts emits the secondary lines an indicator carries in its Extra map
// (MACD signal/hist, BOLL upper/lower, KDJ d/j) as their own ok facts so the
// report shows them. Only emitted when the parent indicator is ok with Extra.
func extraFacts(parentKey string, si indicators.StockIndicator) []Fact {
	if si.Status != indicators.StatusOK || len(si.Extra) == 0 {
		return nil
	}
	type line struct {
		key, labelZH, labelEN, extraKey, unit string
	}
	var lines []line
	switch parentKey {
	case "macd":
		lines = []line{
			{"macd_signal", "MACD 信号线", "MACD Signal", "signal", unitNone},
			{"macd_hist", "MACD 柱", "MACD Histogram", "hist", unitNone},
		}
	case "boll":
		lines = []line{
			{"boll_upper", "布林带上轨", "Bollinger Upper", "upper", unitPrice},
			{"boll_lower", "布林带下轨", "Bollinger Lower", "lower", unitPrice},
		}
	case "kdj":
		lines = []line{
			{"kdj_d", "KDJ %D", "KDJ %D", "d", unitNone},
			{"kdj_j", "KDJ %J", "KDJ %J", "j", unitNone},
		}
	default:
		return nil
	}
	out := make([]Fact, 0, len(lines))
	for _, ln := range lines {
		v, ok := si.Extra[ln.extraKey]
		if !ok {
			continue
		}
		vv := v
		out = append(out, Fact{
			Key: ln.key, LabelZH: ln.labelZH, LabelEN: ln.labelEN,
			Value: formatValue(&vv, ln.unit), Raw: &vv, Unit: ln.unit,
			Status: StatusOK, Source: srcIndicators,
		})
	}
	return out
}

// addSection appends a section to the fact sheet only when it has at least one
// ok fact (a section with zero ok facts is omitted entirely, per §2.4).
func addSection(fs *FactSheet, sec SectionFacts) {
	for _, f := range sec.Facts {
		if f.Status == StatusOK {
			fs.Sections = append(fs.Sections, sec)
			return
		}
	}
}

// priceLabel renders the delayed-quote label "$190.12 · alpaca · delayed ·
// regular" from a quote, omitting empty parts.
func priceLabel(q store.Quote) string {
	parts := []string{formatPrice(q.Price)}
	if q.Source != "" {
		parts = append(parts, q.Source)
	}
	parts = append(parts, "delayed")
	if q.Session != "" {
		parts = append(parts, q.Session)
	}
	return strings.Join(parts, " · ")
}

// copyFloat returns a fresh pointer to the value of p (nil → nil) so a Fact never
// aliases the indicator's internal pointer.
func copyFloat(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
