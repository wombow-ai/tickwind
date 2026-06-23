package research

import (
	"context"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/finra"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/thirteenf"
)

// IndicatorCalc computes the full P0 stock-applicable indicator set for a ticker.
// It is satisfied by *indicators.Computer and never errors (missing data degrades
// to insufficient indicators).
type IndicatorCalc interface {
	// StockIndicators returns the computed indicator set; missing inputs yield
	// insufficient indicators rather than an error.
	StockIndicators(ctx context.Context, ticker string) indicators.StockIndicatorsResult
}

// ScorecardProvider yields the factor-metric POPULATION (the percentile-ranking distribution over the
// tracked universe) + when it was built, so the report can place this stock's factors as PERCENTILES
// vs its peers — the de-isolation that makes the report read relatively, not in a vacuum. Satisfied by
// *ingest.ScorecardCache. nil → no "relative" section. Every percentile is Go-computed
// (indicators.ComputeScorecard), so it is anti-hallucination-safe.
type ScorecardProvider interface {
	Population() ([]indicators.FactorMetrics, time.Time)
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

// CongressProvider returns the recent congressional (House/Senate PTR) trades in
// a ticker, newest first. nil-safe; an empty slice means no DISCLOSED trades were
// found (which also covers the PTR-parsing-disabled case) — the assembler never
// asserts "no member traded this". Satisfied by *congress.Cache (ByTicker).
type CongressProvider interface {
	// ByTicker returns the recent congressional trades in a ticker (newest first),
	// or nil when none / PTR parsing is unavailable.
	ByTicker(ticker string) []congress.TickerTrade
}

// WhalesProvider returns the tracked 13F funds that hold a ticker (the reverse
// "which whales own this" index), largest position first. 13F is quarterly and
// filed ~45 days after quarter-end, so each Holder carries its as-of Period (the
// assembler always shows it — the data is intentionally stale). nil-safe.
// Satisfied by *thirteenf.Cache (Holders).
type WhalesProvider interface {
	// Holders returns the tracked funds holding a ticker, largest first, or nil
	// when none of the tracked funds hold it / the board is unbuilt.
	Holders(ticker string) []thirteenf.Holder
}

// OptionsProvider returns a ticker's delayed (Cboe ~15-min) options overview.
// ok=false when the symbol has no listed options (the assembler omits the options
// facts entirely). Satisfied by *ingest.OptionsCache (Options).
type OptionsProvider interface {
	// Options returns the per-stock options summary and whether the symbol has
	// listed options.
	Options(ctx context.Context, ticker string) (ingest.OptionsView, bool)
}

// ShortVolProvider returns FINRA daily short-volume data for a symbol: the latest
// day's derived ShortPct + date, and the retained history (oldest first) for a
// qualitative trend read. Only the DERIVED percentage is exposed (FINRA's terms
// are display-only — no bulk raw rows). nil-safe. Satisfied by *finrashvol.Cache.
type ShortVolProvider interface {
	// Latest returns one symbol's latest day's short volume (ok=false if absent).
	Latest(sym string) (finrashvol.ShortVol, bool)
	// History returns one symbol's retained short-volume history (oldest first).
	History(sym string) []finrashvol.ShortVol
}

// ShortIntProvider returns a symbol's latest-settlement FINRA short-interest row
// (the bi-monthly settlement data: days-to-cover, change vs prior settlement).
// ok=false when none. Derived values only. nil-safe. Satisfied by
// *ingest.ShortCache (ShortInterest).
type ShortIntProvider interface {
	// ShortInterest returns the latest-settlement short-interest row for a symbol
	// (ok=false when absent).
	ShortInterest(ticker string) (finra.ShortInterest, bool)
}

// MarketSentiment returns the latest market-wide Fear & Greed Result (NOT
// per-ticker — context only). The assembler injects it only when the Result has
// participating components (Available>0), so the neutral-50 fallback is never
// presented as a real reading. nil-safe. Satisfied by *sentiment.Cache (Latest).
type MarketSentiment interface {
	// Latest returns the latest Fear & Greed Result and whether one has been
	// computed.
	Latest() (sentiment.Result, bool)
}

// StoreReader is the narrow slice of store.Store the assembler reads for the
// per-ticker buzz/news-sentiment signals, hot-list presence, and the news +
// social corpus (the corpus feeds the LLM as ATTRIBUTED context — it is never
// turned into a numeric Fact). Every method is read-only. nil-safe. Satisfied by
// the in-process store.Store.
type StoreReader interface {
	// ListSignals returns every source's latest signal for one ticker (buzz +
	// news-sentiment facets).
	ListSignals(ctx context.Context, ticker string) ([]store.Signal, error)
	// HotList returns a board's (e.g. "hot" / "wsb") top rows by rank.
	HotList(ctx context.Context, board string, limit int) ([]store.HotStock, error)
	// ListNews returns a ticker's recent news, newest first.
	ListNews(ctx context.Context, ticker string, limit int) ([]store.News, error)
	// ListSocial returns a ticker's recent social posts, newest first.
	ListSocial(ctx context.Context, ticker string, limit int) ([]store.Post, error)
	// RecentInsiderBuys returns insider open-market buys filed on/after since
	// (across all tickers; the assembler filters to this ticker).
	RecentInsiderBuys(ctx context.Context, since time.Time) ([]store.InsiderBuy, error)
}

// Sources is the narrow slice of in-process providers the assembler needs. Each
// is an interface so the assembler is unit-testable with fakes, and each is
// nil-safe (a nil provider omits its dependent facts/section).
//
// The first three (Indicators/Fundamentals/Quote) drive the P0 valuation /
// fundamentals / technical sections. The remainder drive the 资金面 (flows) and
// 情绪面 (sentiment) sections: Congress/ThirteenF/Insider/Options/ShortVol/ShortInt
// for flows, Market/Store for sentiment. A nil provider simply omits its facts;
// a section that ends up with zero usable facts is omitted entirely.
type Sources struct {
	Indicators   IndicatorCalc
	Fundamentals FundProvider
	Quote        QuoteProvider
	Scorecard    ScorecardProvider // factor-percentile population → the "relative to market" section

	// 资金面 / flows providers.
	Congress  CongressProvider
	ThirteenF WhalesProvider
	Options   OptionsProvider
	ShortVol  ShortVolProvider
	ShortInt  ShortIntProvider

	// 情绪面 / sentiment providers. Market is the market-wide Fear & Greed context;
	// Store reads the per-ticker buzz/news signals, hot-list presence and the
	// news/social corpus (the corpus is ATTRIBUTED LLM context, never a Fact).
	Market MarketSentiment
	Store  StoreReader
}

// Source labels and URLs set in Go from the known provenance. The LLM never
// writes a URL — provenance is authoritative here.
const (
	srcSECXBRL      = "SEC XBRL"
	srcSECQuote     = "SEC XBRL × delayed quote"
	srcDelayedQuote = "delayed quote"
	srcIndicators   = "computed from daily bars"

	// 资金面 / flows source labels.
	srcCongress    = "House/Senate PTR"
	srcThirteenF   = "SEC 13F"
	srcInsiderSEC  = "SEC Form 4"
	srcOptions     = "Cboe · delayed ~15min"
	srcShortVol    = "FINRA daily short volume (display-only)"
	srcShortInt    = "FINRA settlement short interest (display-only)"
	srcFearGreed   = "Fear & Greed (market-wide)"
	srcBuzz        = "ApeWisdom (Reddit/WSB buzz)"
	srcNewsSent    = "AlphaVantage news sentiment"
	srcHotList     = "ApeWisdom trending"
	srcSECInsiders = "SEC EDGAR · Form 4"
)

const secEdgarLabel = "SEC EDGAR · companyfacts"

// insiderLookback bounds the insider-buy window the assembler reads (the
// Opportunity corpus retains ~90d; a 90-day window keeps the "recent buying"
// claim honest).
const insiderLookback = 90 * 24 * time.Hour

// congressDeepLink builds the per-member page deep-link from a member slug.
func congressDeepLink(slug string) string {
	if slug == "" {
		return ""
	}
	return "/congress/member/" + slug
}

// fundDeepLink builds the per-fund pSEO page deep-link from a fund slug.
func fundDeepLink(slug string) string {
	if slug == "" {
		return ""
	}
	return "/fund/" + slug
}

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
//
// lang ("en"/"zh", Chinese-first default) selects the language of every label the
// assembler builds in Go and embeds in a fact Value (the loss placeholder, the
// insufficient placeholder, the flows trade/13F/short-trend labels, the sentiment
// Fear & Greed band). Per-fact LabelZH/LabelEN + section TitleZH/TitleEN carry both
// languages and are selected by the frontend, so they are unaffected; only Value
// strings — which the frontend renders verbatim — need the language threaded in.
func Assemble(ctx context.Context, ticker, lang string, src Sources) FactSheet {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	fs := FactSheet{Ticker: ticker, Disclaimer: pickLang(lang, DisclaimerEN, DisclaimerZH)}

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
	valuation.Facts = append(valuation.Facts, factsFromSpecs(valuationIDOrder, valuationSpecs, byID, fundAsOf, secURL, lang)...)
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
	fundamentals.Facts = append(fundamentals.Facts, factsFromSpecs(fundamentalIDOrder, fundamentalSpecs, byID, fundAsOf, secURL, lang)...)
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
		f, emit := factFromIndicator(spec, si, "", "", lang)
		if !emit {
			continue
		}
		technical.Facts = append(technical.Facts, f)
		technical.Facts = append(technical.Facts, extraFacts(spec.key, si)...)
	}
	addSection(&fs, technical)

	// --- §1.x 相对全市场 / Relative to Market (peer percentiles — de-isolates the report) ---
	addSection(&fs, assembleRelative(indResult, src, lang))

	// --- §1.4 资金面 / Smart Money & Flows ---
	addSection(&fs, assembleFlows(ctx, ticker, src, lang))

	// --- §1.5 情绪面 / Sentiment ---
	addSection(&fs, assembleSentiment(ctx, ticker, src, lang))

	return fs
}

// assembleRelative places this stock's factor metrics as PERCENTILES vs the tracked universe — the
// de-isolation that lets the report (and its LLM prose) read corroboration vs divergence instead of
// five single-ticker snapshots. Every percentile is Go-computed (indicators.ComputeScorecard over the
// background-cached scorecard population); the section self-omits (addSection drops a zero-fact
// section) when the source is nil, the population is too thin (ComputeScorecard withholds below its
// floor), or this stock has no computable factor — never fabricated. Descriptive percentiles only,
// labels stay neutral ("value percentile", not "cheap") so it carries no advice.
func assembleRelative(indResult indicators.StockIndicatorsResult, src Sources, lang string) SectionFacts {
	if src.Scorecard == nil {
		return SectionFacts{Key: "relative", TitleZH: "相对全市场", TitleEN: "Relative to Market"}
	}
	population, popAt := src.Scorecard.Population()
	sc := indicators.ComputeScorecard(indicators.ExtractFactorMetrics(indResult), population)
	return relativeSection(sc, popAt, lang)
}

// relativeSection turns a Go-computed Scorecard into the report's "relative" SectionFacts (4 factor
// percentiles). Split from assembleRelative so it is unit-testable from a constructed Scorecard. Empty
// (→ addSection drops it) when nothing is computable.
func relativeSection(sc indicators.Scorecard, popAt time.Time, lang string) SectionFacts {
	sec := SectionFacts{Key: "relative", TitleZH: "相对全市场", TitleEN: "Relative to Market"}
	if !sc.HasAny() {
		return sec
	}
	asOf := ""
	if !popAt.IsZero() {
		asOf = popAt.Format("2006-01-02")
	}
	source := pickLang(lang, "Tickwind universe", "Tickwind 全市场")
	if sc.Population > 0 {
		source = pickLang(lang,
			"vs "+strconv.Itoa(sc.Population)+" tracked US stocks",
			"对比 "+strconv.Itoa(sc.Population)+" 只追踪美股")
	}
	add := func(key, labelZH, labelEN string, f *indicators.FactorScore) {
		if f == nil || f.Inputs <= 0 {
			return
		}
		p := f.Percentile
		pr := p
		sec.Facts = append(sec.Facts, Fact{
			Key: key, LabelZH: labelZH, LabelEN: labelEN,
			Value:  pickLang(lang, fmtPercentileEN(p), fmtPercentileZH(p)),
			Raw:    &pr,
			Status: StatusOK, Source: source, AsOf: asOf,
		})
	}
	add("value_percentile", "估值百分位", "Value percentile", sc.Value)
	add("growth_percentile", "成长百分位", "Growth percentile", sc.Growth)
	add("quality_percentile", "质量百分位", "Quality percentile", sc.Quality)
	add("momentum_percentile", "动量百分位", "Momentum percentile", sc.Momentum)
	return sec
}

// fmtPercentileEN/ZH render a 0–100 factor percentile for the report ("82nd percentile" / "第 82 百分位").
func fmtPercentileEN(p float64) string {
	n := clampPct(p)
	return strconv.Itoa(n) + ordinalSuffix(n) + " percentile"
}

func fmtPercentileZH(p float64) string {
	return "第 " + strconv.Itoa(clampPct(p)) + " 百分位"
}

func clampPct(p float64) int {
	n := int(math.Round(p))
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

// ordinalSuffix returns the English ordinal suffix for n (1→"st", 2→"nd", 3→"rd", 11–13→"th", …).
func ordinalSuffix(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
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
// are not in the spec maps), so the only states are ok and insufficient. lang
// selects the language of any Go-built placeholder (the P/E loss label).
func factsFromSpecs(order []string, specs map[string]indicatorFactSpec, byID map[string]indicators.StockIndicator, fundAsOf, secURL, lang string) []Fact {
	out := make([]Fact, 0, len(order))
	for _, id := range order {
		spec := specs[id]
		si, present := byID[id]
		if !present {
			continue
		}
		if f, emit := factFromIndicator(spec, si, fundAsOf, secURL, lang); emit {
			out = append(out, f)
		}
	}
	return out
}

// factFromIndicator converts one computed indicator into a Fact per its spec,
// applying the strict status gate. Returns (Fact, true) for an ok or insufficient
// indicator and (zero, false) for any other status (e.g. an unsupported crypto id
// that somehow appeared) so the caller skips it. A unit override on the spec wins
// (e.g. FCF's dollars); otherwise the indicator's reported unit is used. lang
// selects the language of the loss-maker P/E placeholder (rendered verbatim for an
// ok-status loss, so it must follow the request language).
func factFromIndicator(spec indicatorFactSpec, si indicators.StockIndicator, fundAsOf, secURL, lang string) (Fact, bool) {
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
			f.Value = lossLabel(lang)
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
			f.Value = lossLabel(lang)
		} else {
			f.Value = insufficientLabel(lang)
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
