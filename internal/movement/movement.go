// Package movement assembles a move-triggered, evidence-grounded explanation of
// a stock's notable daily price move. It mirrors the research package's anti-
// hallucination split: Go owns every number (the change % and direction are
// computed in Go from the quote and are NEVER the LLM's), assembles a small set
// of ATTRIBUTED evidence (recent news / filings / insider buys), and the LLM —
// when enabled — writes ONLY one short, hedged Chinese sentence over that
// evidence. When the LLM is off, over the daily cap, or errors, a canned Go-built
// explanation is served instead. The explainer is meaningful only on a notable
// move, so a sub-threshold (|change| < notableThreshold) move returns
// Significant=false with no explanation — the data (number + evidence) is always
// served, never a 500/503.
//
// See internal/research for the proven pattern this clones.
package movement

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// notableThreshold is the absolute daily change (in percent) at or above which a
// move is "notable" enough to explain. Below it, an explanation would be noise,
// so the endpoint returns Significant=false and the frontend hides the card.
const notableThreshold = 5.0

// Disclaimer is the mandatory second-layer label shown when the LLM wrote the
// prose. It backs the prompt guardrails: AI-generated, hedged, not advice.
const Disclaimer = "AI 生成 · 仅供参考 · 非投资建议"

// Evidence is one attributed item that MIGHT relate to the move: a recent news
// headline, a filing, or an insider buy. Title/URL/Time are set in Go from the
// typed source — the LLM may reference these headlines but never invents one and
// never writes a URL. Type is one of "news" | "filing" | "insider".
type Evidence struct {
	Type  string    `json:"type"`
	Title string    `json:"title"`
	URL   string    `json:"url,omitempty"`
	Time  time.Time `json:"time"`
}

// Explanation is the assembled movement explainer for one ticker. ChangePct and
// Direction are Go-owned (computed from the quote); Significant gates whether an
// Explanation is meaningful (a sub-threshold move carries no Text/Evidence). Text
// is either the LLM's hedged sentence (LLM=true) or the canned Go line (LLM=false);
// it is ALWAYS non-empty for a significant move. AsOf is the quote's timestamp.
type Explanation struct {
	Ticker      string     `json:"ticker"`
	Significant bool       `json:"significant"`
	ChangePct   float64    `json:"change_pct"`
	Direction   string     `json:"direction"` // "up" | "down"
	Session     string     `json:"session"`
	Text        string     `json:"explanation"`
	Evidence    []Evidence `json:"evidence"`
	LLM         bool       `json:"llm"`
	Model       string     `json:"model"`
	AsOf        time.Time  `json:"as_of"`
	Disclaimer  string     `json:"disclaimer,omitempty"`
}

// changePct computes the day's percent change from a quote: (Price − PrevClose) /
// PrevClose × 100. It returns ok=false when there is no usable reference (no
// positive price or prev close) so the caller never reports a fabricated 0%.
func changePct(q store.Quote) (float64, bool) {
	if q.Price <= 0 || q.PrevClose <= 0 {
		return 0, false
	}
	return (q.Price - q.PrevClose) / q.PrevClose * 100, true
}

// direction maps a signed change to "up"/"down" (a flat 0 reads "up", but a flat
// move is never significant so this is moot in practice).
func direction(pct float64) string {
	if pct < 0 {
		return "down"
	}
	return "up"
}

// maxEvidence caps how many evidence items the explainer carries (and shows the
// LLM). A small set keeps the prompt tight and the card scannable.
const maxEvidence = 5

// newsLookback / filingLookback / insiderLookback bound how recent an evidence
// item must be to be considered a plausible catalyst for TODAY's move. News is
// the freshest signal (~2 days); filings/insider buys are checked for today only.
const (
	newsLookback    = 48 * time.Hour
	filingLookback  = 24 * time.Hour
	insiderLookback = 24 * time.Hour
)

// Inputs is the typed evidence corpus the assembler reads, gathered in Go from
// the store/caches and passed in. Each slice is newest-first and may be empty;
// the assembler filters each to its lookback window and the move's as-of time.
type Inputs struct {
	Quote   store.Quote
	News    []store.News
	Filings []store.Filing
	Insider []store.InsiderBuy // already filtered to this ticker
}

// Assemble builds the data-only Explanation for a ticker with NO LLM. It computes
// the Go-owned change % + direction from the quote, gathers a small attributed
// evidence set, and fills Text with a canned Chinese line (the data-only
// fallback). It is pure and never errors. When the move is below threshold (or
// there is no usable quote) it returns Significant=false with no Text/Evidence —
// the caller serves the number but the frontend hides the card.
//
// The returned Explanation always has LLM=false; a caller with an enabled LLM
// calls Material + the enricher and overwrites Text (see Service.Explain).
func Assemble(ticker string, in Inputs) Explanation {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	exp := Explanation{
		Ticker:   ticker,
		Session:  in.Quote.Session,
		AsOf:     in.Quote.At,
		Evidence: []Evidence{},
	}

	pct, ok := changePct(in.Quote)
	if !ok || math.Abs(pct) < notableThreshold {
		// Not a notable move (or no usable quote): serve the number when we have
		// one, but no explanation — the frontend hides the card.
		exp.ChangePct = pct
		exp.Direction = direction(pct)
		exp.Significant = false
		return exp
	}

	exp.ChangePct = pct
	exp.Direction = direction(pct)
	exp.Significant = true
	exp.Evidence = gatherEvidence(ticker, in)
	exp.Text = cannedText(pct, exp.Evidence)
	return exp
}

// gatherEvidence builds the attributed evidence set, newest-first across all
// types, capped at maxEvidence. News inside newsLookback, filings inside
// filingLookback, and insider buys inside insiderLookback (relative to the
// quote's as-of, falling back to now) are eligible. Everything is set in Go from
// the typed source — every item is attributed.
func gatherEvidence(ticker string, in Inputs) []Evidence {
	asOf := in.Quote.At
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	out := make([]Evidence, 0, maxEvidence)

	for _, n := range in.News {
		if n.Published.IsZero() || asOf.Sub(n.Published) > newsLookback || n.Published.After(asOf.Add(time.Hour)) {
			continue
		}
		title := strings.TrimSpace(n.HeadlineZH)
		if title == "" {
			title = strings.TrimSpace(n.Headline)
		}
		if title == "" {
			continue
		}
		out = append(out, Evidence{Type: "news", Title: title, URL: n.URL, Time: n.Published})
	}
	for _, f := range in.Filings {
		if f.FiledAt.IsZero() || asOf.Sub(f.FiledAt) > filingLookback || f.FiledAt.After(asOf.Add(time.Hour)) {
			continue
		}
		title := strings.TrimSpace(f.Title)
		if title == "" {
			title = strings.TrimSpace(f.Form)
		}
		if title == "" {
			continue
		}
		out = append(out, Evidence{Type: "filing", Title: title, URL: f.URL, Time: f.FiledAt})
	}
	for _, b := range in.Insider {
		if !strings.EqualFold(b.Ticker, ticker) {
			continue
		}
		if b.FiledDate.IsZero() || asOf.Sub(b.FiledDate) > insiderLookback || b.FiledDate.After(asOf.Add(time.Hour)) {
			continue
		}
		out = append(out, Evidence{Type: "insider", Title: insiderTitle(b), URL: b.FilingURL, Time: b.FiledDate})
	}

	// Newest-first across all types, then cap.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Time.After(out[j-1].Time); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > maxEvidence {
		out = out[:maxEvidence]
	}
	return out
}

// insiderTitle renders an insider-buy evidence line in Chinese from the typed
// fields — never the LLM's words.
func insiderTitle(b store.InsiderBuy) string {
	who := strings.TrimSpace(b.OwnerName)
	if who == "" {
		who = "内部人"
	}
	return fmt.Sprintf("%s 内部人买入 %s", who, formatUSD(b.Value))
}

// cannedText builds the data-only Chinese explanation (LLM off / over cap /
// error). It states the Go-owned move and, when present, the top evidence
// headline as attributed context — never a definitive cause. With no evidence it
// admits there is no clear catalyst.
func cannedText(pct float64, ev []Evidence) string {
	dir := "涨"
	if pct < 0 {
		dir = "跌"
	}
	mag := formatPct(pct)
	if len(ev) == 0 {
		return fmt.Sprintf("今日%s%s,暂无明确催化消息。", dir, mag)
	}
	return fmt.Sprintf("今日%s%s;近期消息:%s", dir, mag, ev[0].Title)
}

// formatPct renders the absolute change to one decimal with a trailing %.
func formatPct(pct float64) string {
	return fmt.Sprintf("%.1f%%", math.Abs(pct))
}

// formatUSD renders a dollar value compactly ($1.2M / $25K / $900) for insider
// evidence — the only number movement sets itself, and it is a public Form-4 sum.
func formatUSD(v float64) string {
	a := math.Abs(v)
	switch {
	case a >= 1e9:
		return fmt.Sprintf("$%.1fB", v/1e9)
	case a >= 1e6:
		return fmt.Sprintf("$%.1fM", v/1e6)
	case a >= 1e3:
		return fmt.Sprintf("$%.0fK", v/1e3)
	default:
		return fmt.Sprintf("$%.0f", v)
	}
}

// Material assembles the single pre-formatted material string the LLM sees, in
// the research.buildMaterial style. It states the Go-owned move (the LLM must
// NOT recompute or alter it) and lists each attributed evidence headline. The
// LLM is instructed (in the system prompt) to reference ONLY these headlines and
// to HEDGE. Only formatted values appear so the model cannot recompute a number.
// Returns "" when the move is not significant (no LLM call is warranted).
func Material(exp Explanation) string {
	if !exp.Significant {
		return ""
	}
	dir := "上涨"
	if exp.Direction == "down" {
		dir = "下跌"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "股票: %s\n", exp.Ticker)
	fmt.Fprintf(&sb, "今日%s %s(已由系统计算,不得改动或重新计算)\n", dir, formatPct(exp.ChangePct))
	if exp.Session != "" {
		fmt.Fprintf(&sb, "交易时段: %s\n", exp.Session)
	}
	if len(exp.Evidence) == 0 {
		sb.WriteString("近期证据材料: 无(没有可归因的新闻/公告/内部人交易)\n")
		return sb.String()
	}
	sb.WriteString("近期证据材料(只能引用以下条目,逐条带来源类型,不得编造其它催化因素):\n")
	for _, e := range exp.Evidence {
		fmt.Fprintf(&sb, "- [%s] %s\n", evidenceSourceLabel(e.Type), e.Title)
	}
	return sb.String()
}

// evidenceSourceLabel maps an evidence type to its Chinese source-type label for
// the material (so the model attributes "据新闻/据公告/据内部人交易").
func evidenceSourceLabel(t string) string {
	switch t {
	case "news":
		return "新闻"
	case "filing":
		return "公告/SEC文件"
	case "insider":
		return "内部人交易"
	default:
		return "材料"
	}
}
