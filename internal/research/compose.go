package research

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ResearchEnricher is the narrow LLM slice the composer needs (the
// briefing.BriefEnricher pattern). It is satisfied by enrich.Enricher once
// ComposeReport is added, and is trivially fakeable in tests.
type ResearchEnricher interface {
	// Enabled reports whether a real LLM backend is configured.
	Enabled() bool
	// ComposeReport writes per-section research prose from a pre-built material
	// string, returning a section-key→prose map (qualitative only, no numbers).
	// Returns an error when disabled or on failure.
	ComposeReport(ctx context.Context, material, lang string) (map[string]string, error)
	// ComposeDeepReport is the richer, Fable-5-harnessed sibling of ComposeReport:
	// longer per-section prose plus an executive overview, over the SAME material
	// (only formatted strings — no numbers), using a possibly stronger model. Same
	// prose-only, section-key→prose contract; returns an error when disabled or on
	// failure.
	ComposeDeepReport(ctx context.Context, material, lang string) (map[string]string, error)
}

// Compose returns the FactSheet with per-section prose filled in. It builds ONE
// material string from fs.Sections, makes ONE ComposeReport call, and fills each
// SectionFacts.Prose by matching section key. The LLM is off the critical path:
// when enr is nil/disabled or the call errors, the data-only FactSheet is
// returned UNCHANGED (every prose stays "") and NO error is returned. The LLM can
// only ever touch Prose — it never sees or sets a Fact's Value or Raw.
func Compose(ctx context.Context, fs FactSheet, enr ResearchEnricher, lang string) FactSheet {
	return compose(ctx, fs, enr, lang, false)
}

// ComposeDeep is the deep-research counterpart of Compose: it fills per-section
// prose via the enricher's richer ComposeDeepReport (stronger model + Fable-5
// harness) over the SAME material. The anti-hallucination contract is IDENTICAL —
// the LLM sees only formatted strings and can only ever touch Prose; it never sees
// or sets a Fact's Value or Raw, and a stray numeric key in its reply is ignored.
// A stronger model writes richer prose over the same Go-owned facts; it never
// computes or asserts a number. Same off-the-critical-path degradation.
func ComposeDeep(ctx context.Context, fs FactSheet, enr ResearchEnricher, lang string) FactSheet {
	return compose(ctx, fs, enr, lang, true)
}

// compose is the shared implementation behind Compose / ComposeDeep. The ONLY
// difference between the two paths is which enricher method writes the prose
// (ComposeReport vs the richer ComposeDeepReport) — the material build, the
// prose-only fill, the overview/bull/bear synthesis, and the never-touch-a-number
// invariant are identical, so both paths share the same anti-hallucination
// guarantees.
func compose(ctx context.Context, fs FactSheet, enr ResearchEnricher, lang string, deep bool) FactSheet {
	if enr == nil || !enr.Enabled() {
		return fs
	}
	material := buildMaterial(fs, lang)
	if strings.TrimSpace(material) == "" {
		return fs
	}
	composeFn := enr.ComposeReport
	if deep {
		composeFn = enr.ComposeDeepReport
	}
	prose, err := composeFn(ctx, material, lang)
	if err != nil || len(prose) == 0 {
		return fs // degrade to data-only; never propagate the error
	}
	for i := range fs.Sections {
		if p, ok := prose[fs.Sections[i].Key]; ok {
			fs.Sections[i].Prose = ScrubAdvice(p)
		}
	}
	// The overview is a synthesis the LLM writes over all the other sections'
	// facts (it is NOT in the material as an input section). It carries the balanced
	// prose plus the two-sided 看多/看空 (bull/bear) reading — prose-only, no facts of
	// its own — so it exists only when the LLM produced it; the data-only report (LLM
	// off) has no overview. Rendered FIRST (prepended).
	ov := ScrubAdvice(prose[overviewKey])
	bull := splitPoints(prose[bullKey])
	bear := splitPoints(prose[bearKey])
	if ov != "" || len(bull) > 0 || len(bear) > 0 {
		fs.Sections = append([]SectionFacts{{
			Key:     overviewKey,
			TitleZH: "概览",
			TitleEN: "Overview",
			// Non-nil empty slices so the JSON carries "facts":[] / "citations":[]
			// (a nil slice marshals as null, which would break a client doing
			// `section.facts.length`). The overview is prose-only.
			Facts:     []Fact{},
			Citations: []Citation{},
			Prose:     ov,
			Bull:      bull,
			Bear:      bear,
		}}, fs.Sections...)
	}
	return fs
}

// overviewKey is the synthesis section the composer adds over all other sections;
// bullKey/bearKey carry its two-sided reading (parsed out, not rendered as sections).
const (
	overviewKey = "overview"
	bullKey     = "bull"
	bearKey     = "bear"
)

// maxBullBearPoints caps each side of the 看多/看空 reading.
const maxBullBearPoints = 5

// bulletRe strips a leading list marker ("- ", "• ", "1.", "1)", "1、") from a line
// — but NOT a bare leading number that is part of the content (e.g. "10x P/E …").
var bulletRe = regexp.MustCompile(`^\s*(?:[-–—•*·‣◦]+|\d{1,2}[.)、])\s*`)

// advicePhrases are unambiguous investment-advice / price-target / valuation-judgment
// markers. A bull/bear point (deep report) or a chat prose line that contains one is
// dropped — a deterministic backstop to the prompt guardrail (more reliable than asking
// the model to self-censor, zero latency). Bare "买入/卖出/加仓/建仓/减仓/buy/sell" is
// intentionally ABSENT: those describe insider-buy, 13F position-change, or congressional-
// sale FACTS, not advice. Expanded 2026-06-20 after an adversarial red-team found hedged
// synonyms (fair value, entry point, accumulate-at-a-price, 低估/抄底) leaking past.
var advicePhrases = []string{
	// ZH advice / price-target / valuation-judgment (NOT 买入/卖出/加仓/建仓/减仓 — facts).
	"目标价", "目标股价", "目标位", "强烈推荐", "强烈建议", "建议买", "建议卖", "应该买", "应该卖",
	"值得买入", "值得入手", "可以买入", "立即买", "逢低买入", "逢低吸纳", "抄底", "上车", "入场点",
	"合理估值", "合理价值", "内在价值", "低估", "高估",
	// EN advice / price-target / valuation-judgment (NOT bare buy/sell — facts).
	"price target", "target price", "strong buy", "strong sell", "should buy", "should sell",
	"recommend buy", "recommend sell", "must buy", "fair value", "intrinsic value",
	"fairly valued", "undervalued", "overvalued", "entry point", "good entry", "buy the dip",
	"compelling buy", "worth buying", "a buy here", "buy here", "deserves a position",
}

// adviceRe catches a buy-side action tied to a PRICE LEVEL ("buy at $150", "accumulate
// below 100", "entry around $200") — always a recommendation even when no listed phrase
// appears. Past-tense facts ("insider bought at $50") use bought/sold (not matched), and
// a bare buy/sell with no price level is not matched either.
var adviceRe = regexp.MustCompile(`(?i)\b(buy|buying|accumulate|accumulating|enter|entry|short)\b[\w\s]{0,12}?\b(at|above|below|around|near|under)\b\s*\$?\d`)

// splitPoints turns the model's newline-joined bull/bear string into trimmed points:
// it strips list markers, drops blanks, drops any point that trips the advice-guard,
// and caps the count. Returns nil for empty input (so the JSON omits the field).
func splitPoints(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		p := strings.TrimSpace(bulletRe.ReplaceAllString(line, ""))
		if p == "" || hasAdvice(p) {
			continue
		}
		out = append(out, p)
		if len(out) >= maxBullBearPoints {
			break
		}
	}
	return out
}

// ScrubAdvice is the deterministic advice backstop for FREE PROSE (report section
// bodies + the overview synthesis) — the same guard the bull/bear points and the chat
// path already run. It drops any line that trips hasAdvice, then re-checks the joined
// survivors as one string (advice phrased ACROSS lines); if that still trips, it returns
// "" so the section degrades to its Go-owned data-only facts rather than shipping advice.
// Without this, a price target / "undervalued" / "should buy" in the model's prose body
// reaches the user stopped only by the system prompt.
func ScrubAdvice(prose string) string {
	if strings.TrimSpace(prose) == "" {
		return ""
	}
	lines := strings.Split(prose, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if hasAdvice(ln) {
			continue
		}
		kept = append(kept, ln)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	if out != "" && hasAdvice(strings.ReplaceAll(out, "\n", " ")) {
		return ""
	}
	return out
}

// hasAdvice reports whether a point contains an investment-advice / price-target
// marker (case-insensitive for the ASCII phrases).
func hasAdvice(p string) bool {
	low := strings.ToLower(p)
	for _, w := range advicePhrases {
		if strings.Contains(low, strings.ToLower(w)) {
			return true
		}
	}
	return adviceRe.MatchString(p)
}

// buildMaterial assembles the single pre-formatted material string the LLM sees,
// in the briefing.buildMaterial style: a header, then one block per section keyed
// by its stable Key, listing each ok fact as "Label: Value" and noting thin
// (insufficient) facts. A section may also carry attributed CONTEXT lines (news /
// social backdrop for the sentiment section) — these are quotable, ATTRIBUTED
// material ("据新闻/据社区讨论"), explicitly marked as non-numeric so the model
// reports them with attribution and never derives a sentiment number from them.
// The LLM is instructed to key its JSON reply by these section keys. Only
// formatted values appear — never raw structs — so the model cannot recompute a
// number.
func buildMaterial(fs FactSheet, lang string) string {
	var sb strings.Builder
	name := fs.Name
	if name == "" {
		name = fs.Ticker
	}
	fmt.Fprintf(&sb, "Ticker: %s (%s)\n", fs.Ticker, name)
	if fs.AsOf != "" {
		fmt.Fprintf(&sb, "As of: %s\n", fs.AsOf)
	}
	if fs.PriceLabel != "" {
		fmt.Fprintf(&sb, "Price: %s\n", fs.PriceLabel)
	}

	for _, sec := range fs.Sections {
		writeSection(&sb, sec, lang)
	}
	return sb.String()
}

// writeSection appends ONE section's pre-formatted block (header, "Label: Value [source]"
// facts, an insufficient roll-up, and attributed non-numeric context) to sb. Extracted
// from buildMaterial so the single-section get_facts tool (FactsForSection) produces a
// byte-identical block. Output format is load-bearing for the anti-hallucination tests.
func writeSection(sb *strings.Builder, sec SectionFacts, lang string) {
	title := sec.TitleEN
	if lang != "en" && sec.TitleZH != "" {
		title = sec.TitleZH
	}
	fmt.Fprintf(sb, "\n[%s] (key=%s)\n", title, sec.Key)
	var thin []string
	for _, f := range sec.Facts {
		label := f.LabelEN
		if lang != "en" && f.LabelZH != "" {
			label = f.LabelZH
		}
		if f.Status == StatusOK {
			// Citation bracket carries the source AND the per-fact freshness stamp. The
			// single-section tool path (FactsForSection → this) has no sheet-level "As of:"
			// line and no <facts> block for cross-ticker get_stock_facts, so without this the
			// model would quote a ~45-day-stale 13F / FINRA settlement figure with no
			// traceable vintage. Facts within one section have different vintages (a live
			// price vs a 13F quarter), so the stamp is per-fact, not sheet-level.
			var cite []string
			if f.Source != "" {
				cite = append(cite, f.Source)
			}
			if f.AsOf != "" {
				cite = append(cite, pickLang(lang, "as of ", "截至 ")+f.AsOf)
			}
			src := ""
			if len(cite) > 0 {
				src = " [" + strings.Join(cite, ", ") + "]"
			}
			fmt.Fprintf(sb, "- %s: %s%s\n", label, f.Value, src)
		} else {
			thin = append(thin, label)
		}
	}
	if len(thin) > 0 {
		fmt.Fprintf(sb, "- (数据不足 / insufficient: %s)\n", strings.Join(thin, ", "))
	}
	// Attributed, NON-NUMERIC context (news/social) — quote with attribution,
	// never restate as fact, never derive a number from it.
	if len(sec.Context) > 0 {
		fmt.Fprintf(sb, "- (背景材料 / attributed context — quote with source, do NOT treat as fact or derive a number:)\n")
		for _, c := range sec.Context {
			fmt.Fprintf(sb, "  · %s\n", c)
		}
	}
}
