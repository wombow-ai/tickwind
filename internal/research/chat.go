package research

import "strings"

// This file exports the research package's fact-formatting + advice-guard helpers for the
// Product B personalized chat (internal/chat). Chat grounds on the SAME Go-owned FactSheet
// as the deep report, so it reuses the exact same pre-formatting (so the model never sees
// a raw number it could recompute) and the exact same deterministic advice post-filter.

// HasAdvice reports whether a prose line contains an investment-advice / price-target
// marker (e.g. "target price", "强烈推荐"). Exported so the chat layer can run the same
// deterministic post-filter the deep report runs over bull/bear points — a backstop to
// the system-prompt guardrail. Bare buy/sell is intentionally NOT flagged (it describes
// insider-buy / congressional-sale facts).
func HasAdvice(p string) bool { return hasAdvice(p) }

// Material returns the full per-ticker, pre-formatted "Label: Value [source]" context the
// chat model grounds on — the same substrate the deep report composes from. Every number
// is already Go-formatted, so the model can quote but never recompute one.
func Material(fs FactSheet, lang string) string { return buildMaterial(fs, lang) }

// FactSectionKeys is the CLOSED set of sections that carry Go-owned facts a chat user can
// pull via the get_facts tool (overview is LLM-only prose and is excluded). Report order.
func FactSectionKeys() []string {
	return []string{"valuation", "fundamentals", "technical", "flows", "sentiment"}
}

// FactsForSection returns the pre-formatted facts block for ONE section (by Key), in the
// SAME shape as Material — the get_facts(section) tool result. Returns "" when the key is
// unknown or the sheet lacks that section.
func FactsForSection(fs FactSheet, key, lang string) string {
	for _, sec := range fs.Sections {
		if sec.Key == key {
			var sb strings.Builder
			writeSection(&sb, sec, lang)
			return strings.TrimSpace(sb.String())
		}
	}
	return ""
}

// NewsContext returns the attributed, NON-NUMERIC news/social context lines across the
// whole sheet (the get_news_context tool result) — already source-attributed; the model
// must quote with attribution and never derive a number from them.
func NewsContext(fs FactSheet) []string {
	var out []string
	for _, sec := range fs.Sections {
		out = append(out, sec.Context...)
	}
	return out
}
