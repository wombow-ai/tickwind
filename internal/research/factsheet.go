// Package research assembles a structured, source-attributed per-ticker fact
// sheet (every number set in Go from a typed source) and, optionally, fills
// per-section qualitative prose via a feature-flagged LLM enricher. The split is
// the core anti-hallucination guarantee: Go owns every number, the LLM owns only
// words and can never alter a Fact's value. The LLM is off the critical path —
// when it is disabled or errors, the data-only fact sheet is returned unchanged.
//
// See docs/research/2026-06-13-r2-ai-research-design.md (§1-§3) for the full
// design. P0 emits exactly three sections: valuation, fundamentals, technical.
package research

// Status values for a Fact. They mirror indicators.Status so the frontend can
// render a muted "数据不足" chip (with the Reason) instead of a blank, and skip
// unsupported metrics entirely.
const (
	// StatusOK means the Fact carries a real, formatted Value.
	StatusOK = "ok"
	// StatusInsufficient means the underlying datum was missing/invalid; Value is
	// the literal "数据不足" placeholder and Reason explains why (never a number).
	StatusInsufficient = "insufficient"
)

// Disclaimer is the mandatory second-layer label shown on every report. It backs
// the prompt guardrails: AI-generated prose, numbers from public data, not advice.
const Disclaimer = "AI 生成 · 数字来自公开数据 · 非投资建议"

// Fact is one labeled, source-attributed datum. Value carries the already-
// formatted string the report shows (e.g. "41.2x", "亏损", "$4.5T", "—"); Raw is
// the underlying number when present (for the frontend / future PDF). Source and
// SourceURL are the citation. Status mirrors indicators.Status so the frontend
// can render "数据不足" with the Reason instead of a blank. A Fact is the SOLE
// source of a number in the report — the LLM never sets Value or Raw.
type Fact struct {
	Key       string   `json:"key"`      // stable id, e.g. "pe", "roe", "rsi"
	LabelZH   string   `json:"label_zh"` // "市盈率(P/E)"
	LabelEN   string   `json:"label_en"` // "P/E (TTM)"
	Value     string   `json:"value"`    // formatted display string
	Raw       *float64 `json:"raw,omitempty"`
	Unit      string   `json:"unit,omitempty"`   // "%" | "x" | "price" | "USD" | ""
	Status    string   `json:"status"`           // "ok" | "insufficient"
	Reason    string   `json:"reason,omitempty"` // verbatim from indicators when not ok
	Source    string   `json:"source"`           // citation label, e.g. "SEC XBRL FY2024"
	SourceURL string   `json:"source_url,omitempty"`
	AsOf      string   `json:"as_of,omitempty"` // freshness stamp
}

// SectionFacts is one report section's pre-LLM data: its facts plus the citations
// they collapse to. Title carries both languages; Prose is filled by the composer
// (empty when the LLM is off — the data-only report).
type SectionFacts struct {
	Key       string     `json:"key"` // "valuation" | "fundamentals" | "technical"
	TitleZH   string     `json:"title_zh"`
	TitleEN   string     `json:"title_en"`
	Facts     []Fact     `json:"facts"` // only Status==ok facts carry a Value
	Citations []Citation `json:"citations"`
	Prose     string     `json:"prose"` // qualitative LLM prose; "" when LLM off
}

// Citation maps a section's claim space to a source. The frontend turns Anchor
// into a deep-link (in-page sub-section) or uses URL directly (filing / source
// page). The LLM is told the sources but writes no URLs — citations are set in Go.
type Citation struct {
	Label  string `json:"label"`            // "SEC EDGAR · companyfacts"
	Anchor string `json:"anchor,omitempty"` // in-page section id, e.g. "#fundamentals"
	URL    string `json:"url,omitempty"`    // external (SEC filing, source page)
}

// FactSheet is the entire numeric backbone for one ticker, assembled with NO LLM.
// It is the data-only report when the LLM is disabled, and the material source
// when it is enabled. Pure and unit-testable.
type FactSheet struct {
	Ticker     string         `json:"ticker"`
	Name       string         `json:"name,omitempty"`
	AsOf       string         `json:"as_of"`       // newest underlying date across sources
	PriceLabel string         `json:"price_label"` // "$190.12 · alpaca · delayed · regular"
	Sections   []SectionFacts `json:"sections"`    // valuation/fundamentals/technical
	Disclaimer string         `json:"disclaimer"`  // the mandatory label
}
