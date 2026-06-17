package research

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/thirteenf"
)

// flowsTopHolders caps how many 13F holders the report lists.
const flowsTopHolders = 4

// shortRisingThreshold is the relative change in ShortPct (latest vs earliest of
// the retained history) above which the daily short-volume trend reads "rising"
// (below the negative of it, "falling"); within the band it is "flat". Kept
// qualitative — the report shows the derived percentage, never a synthesized delta.
const shortRisingThreshold = 0.15

// assembleFlows builds the §1.4 资金面 (Smart Money & Flows) section. Each
// provider is nil-safe and contributes independently; the section is omitted by
// the caller (addSection) when it has zero ok facts. Every number is derived in Go
// from a structured source — congress amount ranges are shown VERBATIM, 13F
// holder Period is always shown (intentionally stale), FINRA exposes only the
// derived ShortPct/DaysToCover. List data (members, holders) is formatted into Go
// strings; no number is ever synthesized. lang ("en"/"zh") selects the language of
// every Go-built label embedded in a fact Value (trade direction, 13F change tag,
// short trend) — the value carries ONE language, never a bilingual "x / y" string.
func assembleFlows(ctx context.Context, ticker string, src Sources, lang string) SectionFacts {
	sec := SectionFacts{Key: "flows", TitleZH: "资金面", TitleEN: "Smart Money & Flows"}
	var citations []Citation

	// --- Congress (House/Senate PTR) ---
	if src.Congress != nil {
		if trades := src.Congress.ByTicker(ticker); len(trades) > 0 {
			sec.Facts = append(sec.Facts, congressFacts(trades, lang)...)
			citations = append(citations, Citation{
				Label:  srcCongress,
				Anchor: "#congress",
				URL:    congressDeepLink(trades[0].Slug),
			})
		}
		// An empty/nil result is NOT a fact: nil means either no disclosed trades OR
		// PTR parsing is disabled, so the report never asserts "no member traded
		// this". The section simply omits congress facts.
	}

	// --- 13F whales (reverse "which whales own this") ---
	if src.ThirteenF != nil {
		if holders := src.ThirteenF.Holders(ticker); len(holders) > 0 {
			sec.Facts = append(sec.Facts, thirteenFFacts(holders, lang)...)
			citations = append(citations, Citation{
				Label:  srcThirteenF,
				Anchor: "#whales",
				URL:    fundDeepLink(holders[0].FundSlug),
			})
		}
	}

	// --- Insider buys (SEC Form 4, open-market purchases) ---
	if src.Store != nil {
		if f, ok := insiderFact(ctx, ticker, src.Store); ok {
			sec.Facts = append(sec.Facts, f...)
			citations = append(citations, Citation{Label: srcSECInsiders, Anchor: "#insiders"})
		}
	}

	// --- Options (delayed Cboe chain) ---
	if src.Options != nil {
		if view, ok := src.Options.Options(ctx, ticker); ok {
			if of := optionsFacts(view); len(of) > 0 {
				sec.Facts = append(sec.Facts, of...)
				citations = append(citations, Citation{Label: srcOptions, Anchor: "#options"})
			}
		}
		// ok=false → no listed options → omit (no facts).
	}

	// --- Short (daily short-volume % + settlement short interest) ---
	if sf, cited := shortFacts(ticker, src, lang); len(sf) > 0 {
		sec.Facts = append(sec.Facts, sf...)
		if cited {
			citations = append(citations, Citation{Label: srcShortVol, Anchor: "#short"})
		}
	}

	sec.Citations = citations
	return sec
}

// congressFacts builds the congress facts: a distinct-member count plus a verbatim
// summary of the latest member's trade. AmountRange is shown EXACTLY as disclosed
// (never converted to a point dollar amount). TxDate is formatted as a date. lang
// selects the buy/sell direction label embedded in the latest-trade value.
func congressFacts(trades []congress.TickerTrade, lang string) []Fact {
	members := map[string]struct{}{}
	for _, t := range trades {
		members[t.Slug] = struct{}{}
	}
	count := float64(len(members))

	facts := []Fact{{
		Key: "congress_members", LabelZH: "国会披露交易议员数", LabelEN: "Members Disclosing Trades",
		Value: formatPlain(count), Raw: copyFloat(&count), Unit: unitNone,
		Status: StatusOK, Source: srcCongress,
	}}

	// Latest member trade — AmountRange verbatim, never a synthesized $ amount.
	latest := trades[0] // ByTicker is newest-first
	dir := tradeTypeLabel(latest.Type, lang)
	amount := latest.AmountRange
	if amount == "" {
		amount = dash
	}
	val := fmt.Sprintf("%s · %s · %s", latest.MemberName, dir, amount)
	facts = append(facts, Fact{
		Key: "congress_latest", LabelZH: "最近披露交易", LabelEN: "Latest Disclosed Trade",
		Value:  val,
		Status: StatusOK, Source: srcCongress,
		SourceURL: congressDeepLink(latest.Slug), AsOf: formatDate(latest.TxDate),
	})
	return facts
}

// tradeTypeLabel maps a PTR trade type to a buy/sell label in the request lang
// (EN → English only, zh → Chinese only — never the bilingual "x / y" form). An
// unrecognised non-empty type is passed through verbatim.
func tradeTypeLabel(t, lang string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "purchase", "buy":
		return pickLang(lang, "buy", "买入")
	case "sale", "sell", "sale (full)", "sale (partial)":
		return pickLang(lang, "sell", "卖出")
	case "exchange":
		return pickLang(lang, "exchange", "兑换")
	default:
		if t == "" {
			return dash
		}
		return t
	}
}

// thirteenFFacts builds the 13F holder facts: a holder-count fact plus a per-holder
// line for the top funds. Each line shows the manager/fund, the position weight (%
// of the FUND's book) and the change tag; the holder's filing Period (intentionally
// stale, ~45d) is carried as the AsOf and is ALWAYS shown. lang selects the change-
// tag label (new/add/trim/hold) embedded in each holder value.
func thirteenFFacts(holders []thirteenf.Holder, lang string) []Fact {
	count := float64(len(holders))
	facts := []Fact{{
		Key: "whales_count", LabelZH: "持仓机构数(13F)", LabelEN: "Tracked Funds Holding (13F)",
		Value: formatPlain(count), Raw: copyFloat(&count), Unit: unitNone,
		// Stamp the aggregate count with the OLDEST holder Period: holders can span
		// funds that filed for different quarters near a 13F deadline, so the most
		// conservative (stalest) quarter is the honest as-of for the whole count.
		Status: StatusOK, Source: srcThirteenF, AsOf: oldestPeriod(holders),
	}}

	top := holders
	if len(top) > flowsTopHolders {
		top = top[:flowsTopHolders]
	}
	for i, h := range top {
		who := h.Manager
		if who == "" {
			who = h.FundName
		} else if h.FundName != "" {
			who = h.Manager + " · " + h.FundName
		}
		weight := formatValue(&h.Weight, unitPercent)
		change := changeTagLabel(h.Change, lang)
		val := fmt.Sprintf("%s · %s of book · %s", who, weight, change)
		facts = append(facts, Fact{
			Key: fmt.Sprintf("whale_%d", i+1), LabelZH: "机构持仓", LabelEN: "Fund Holding",
			Value:  val,
			Status: StatusOK, Source: srcThirteenF,
			SourceURL: fundDeepLink(h.FundSlug),
			AsOf:      h.Period, // MUST show — 13F is ~45d stale.
		})
	}
	return facts
}

// oldestPeriod returns the earliest (stalest) non-empty Period across holders.
// Periods are YYYY-MM-DD, so lexical order matches chronological order. Empty
// Periods are skipped; "" is returned only when every holder lacks one.
func oldestPeriod(holders []thirteenf.Holder) string {
	oldest := ""
	for _, h := range holders {
		if h.Period == "" {
			continue
		}
		if oldest == "" || h.Period < oldest {
			oldest = h.Period
		}
	}
	return oldest
}

// changeTagLabel maps a 13F quarter-over-quarter change tag to a label in the
// request lang (EN → English only, zh → Chinese only — never bilingual). An
// unrecognised non-empty tag is passed through verbatim.
func changeTagLabel(c, lang string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "new":
		return pickLang(lang, "new", "新建仓")
	case "add":
		return pickLang(lang, "add", "加仓")
	case "trim":
		return pickLang(lang, "trim", "减仓")
	case "hold":
		return pickLang(lang, "hold", "维持")
	default:
		if c == "" {
			return dash
		}
		return c
	}
}

// insiderFact aggregates the ticker's recent (≤ insiderLookback) SEC Form-4
// open-market PURCHASES into distinct buyers, total $ value, average price and the
// latest filing date. ok=false (no facts) when there were no recent buys for this
// ticker. All values are derived in Go from the structured InsiderBuy rows.
func insiderFact(ctx context.Context, ticker string, sr StoreReader) ([]Fact, bool) {
	since := time.Now().Add(-insiderLookback)
	buys, err := sr.RecentInsiderBuys(ctx, since)
	if err != nil || len(buys) == 0 {
		return nil, false
	}
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	buyers := map[string]struct{}{}
	var totalValue, weightedShares, weightedPriceShares float64
	var latest time.Time
	matched := 0
	for _, b := range buys {
		if strings.ToUpper(strings.TrimSpace(b.Ticker)) != ticker {
			continue
		}
		matched++
		buyers[strings.ToLower(strings.TrimSpace(b.OwnerName))] = struct{}{}
		totalValue += b.Value
		if b.Shares > 0 {
			weightedShares += b.Shares
			weightedPriceShares += b.Price * b.Shares
		}
		if b.FiledDate.After(latest) {
			latest = b.FiledDate
		}
	}
	if matched == 0 {
		return nil, false
	}

	distinct := float64(len(buyers))
	facts := []Fact{{
		Key: "insider_buyers", LabelZH: "内部人买入人数(90天)", LabelEN: "Insider Buyers (90d)",
		Value: formatPlain(distinct), Raw: copyFloat(&distinct), Unit: unitNone,
		Status: StatusOK, Source: srcInsiderSEC, AsOf: formatDate(latest),
	}}
	if totalValue > 0 {
		tv := totalValue
		facts = append(facts, Fact{
			Key: "insider_value", LabelZH: "内部人买入总额", LabelEN: "Insider Buy Value",
			Value: formatValue(&tv, unitUSD), Raw: copyFloat(&tv), Unit: unitUSD,
			Status: StatusOK, Source: srcInsiderSEC, AsOf: formatDate(latest),
		})
	}
	if weightedShares > 0 {
		avg := weightedPriceShares / weightedShares
		facts = append(facts, Fact{
			Key: "insider_avg_price", LabelZH: "内部人买入均价", LabelEN: "Avg Buy Price",
			Value: formatPrice(avg), Raw: copyFloat(&avg), Unit: unitPrice,
			Status: StatusOK, Source: srcInsiderSEC, AsOf: formatDate(latest),
		})
	}
	return facts, true
}

// optionsFacts builds the delayed-options facts: put/call volume + OI ratios, max
// pain (with its expiry), and the leading open-interest contract. Ratios are taken
// AS COMPUTED by the OptionsCache (the report never recomputes them). Returns nil
// when the view carries no usable ratio (a malformed/empty chain). Every fact is
// labeled delayed · Cboe ~15min via srcOptions.
func optionsFacts(view ingest.OptionsView) []Fact {
	var facts []Fact
	if view.PCVolume > 0 {
		pcv := view.PCVolume
		facts = append(facts, Fact{
			Key: "options_pc_volume", LabelZH: "认沽/认购量比(P/C Vol)", LabelEN: "Put/Call (Volume)",
			Value: formatPlain(pcv), Raw: copyFloat(&pcv), Unit: unitNone,
			Status: StatusOK, Source: srcOptions, AsOf: formatTime(view.At),
		})
	}
	if view.PCOI > 0 {
		pco := view.PCOI
		facts = append(facts, Fact{
			Key: "options_pc_oi", LabelZH: "认沽/认购持仓比(P/C OI)", LabelEN: "Put/Call (OI)",
			Value: formatPlain(pco), Raw: copyFloat(&pco), Unit: unitNone,
			Status: StatusOK, Source: srcOptions, AsOf: formatTime(view.At),
		})
	}
	if view.MaxPain > 0 {
		mp := view.MaxPain
		val := formatPrice(mp)
		if view.Expiry != "" {
			val = formatPrice(mp) + " @ " + view.Expiry
		}
		facts = append(facts, Fact{
			Key: "options_max_pain", LabelZH: "最大痛点(Max Pain)", LabelEN: "Max Pain",
			Value: val, Raw: copyFloat(&mp), Unit: unitPrice,
			Status: StatusOK, Source: srcOptions, AsOf: formatTime(view.At),
		})
	}
	// Top open-interest contract — descriptive only (the leader of the chain).
	if len(view.TopOI) > 0 {
		c := view.TopOI[0]
		val := fmt.Sprintf("%s %s @ %s · OI %d", optionTypeLabel(c.Type), formatPrice(c.Strike), c.Expiry, c.OI)
		facts = append(facts, Fact{
			Key: "options_top_oi", LabelZH: "最大未平仓合约", LabelEN: "Top OI Contract",
			Value:  val,
			Status: StatusOK, Source: srcOptions, AsOf: formatTime(view.At),
		})
	}
	return facts
}

// optionTypeLabel maps a Cboe contract type ("C"/"P") to a readable label.
func optionTypeLabel(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "C":
		return "Call"
	case "P":
		return "Put"
	default:
		return t
	}
}

// shortFacts builds the short facts from the daily short-volume provider (the
// derived ShortPct + a qualitative rising/falling/flat trend from the retained
// history) and, when present, the bi-monthly settlement short-interest row
// (days-to-cover + change). Only DERIVED values are exposed — never bulk raw
// FINRA rows. The bool reports whether any short-volume fact (the cited source)
// was emitted. lang selects the rising/falling/flat trend label language.
func shortFacts(ticker string, src Sources, lang string) ([]Fact, bool) {
	var facts []Fact
	citedShortVol := false

	if src.ShortVol != nil {
		if sv, ok := src.ShortVol.Latest(ticker); ok && sv.ShortPct > 0 {
			pct := sv.ShortPct
			f := Fact{
				Key: "short_pct", LabelZH: "当日做空占比", LabelEN: "Daily Short Volume %",
				Value: formatValue(&pct, unitPercent), Raw: copyFloat(&pct), Unit: unitPercent,
				Status: StatusOK, Source: srcShortVol, AsOf: sv.Date,
			}
			facts = append(facts, f)
			citedShortVol = true

			// Qualitative trend from the retained history (latest vs earliest);
			// shown as a labeled string, NOT a synthesized delta number.
			if trend := shortTrend(src.ShortVol.History(ticker), lang); trend != "" {
				facts = append(facts, Fact{
					Key: "short_trend", LabelZH: "做空趋势", LabelEN: "Short Trend",
					Value:  trend,
					Status: StatusOK, Source: srcShortVol, AsOf: sv.Date,
				})
			}
		}
	}

	if src.ShortInt != nil {
		if si, ok := src.ShortInt.ShortInterest(ticker); ok && si.DaysToCover > 0 {
			dtc := si.DaysToCover
			facts = append(facts, Fact{
				Key: "days_to_cover", LabelZH: "回补天数(Days to Cover)", LabelEN: "Days to Cover",
				Value: formatPlain(dtc), Raw: copyFloat(&dtc), Unit: unitNone,
				Status: StatusOK, Source: srcShortInt, AsOf: si.SettlementDate,
			})
			if si.ChangePct != 0 {
				chg := si.ChangePct
				facts = append(facts, Fact{
					Key: "short_interest_change", LabelZH: "空头持仓变化", LabelEN: "Short Interest Change",
					Value: formatValue(&chg, unitPercent), Raw: copyFloat(&chg), Unit: unitPercent,
					Status: StatusOK, Source: srcShortInt, AsOf: si.SettlementDate,
				})
			}
		}
	}
	return facts, citedShortVol
}

// shortTrend reads a qualitative rising/falling/flat trend off the retained daily
// short-volume history (oldest first), comparing the latest ShortPct against the
// earliest. It returns "" when there is too little history to judge. The output is
// a single-language label in the request lang (EN → English only, zh → Chinese
// only — never a bilingual "x / y" string) — and never a synthesized number.
func shortTrend(hist []finrashvol.ShortVol, lang string) string {
	if len(hist) < 2 {
		return ""
	}
	first := hist[0].ShortPct
	last := hist[len(hist)-1].ShortPct
	if first <= 0 {
		return ""
	}
	rel := (last - first) / first
	switch {
	case rel > shortRisingThreshold:
		return pickLang(lang, "rising", "上升")
	case rel < -shortRisingThreshold:
		return pickLang(lang, "falling", "下降")
	default:
		return pickLang(lang, "flat", "平稳")
	}
}

// formatDate renders a time as YYYY-MM-DD (UTC), or "" for the zero time.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// formatTime renders a time as YYYY-MM-DD HH:MM UTC, or "" for the zero time —
// used for the ~15-min-delayed options as-of stamp.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
