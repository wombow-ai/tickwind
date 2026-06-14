// Package materialevents assembles a company's recent 8-K (current report) filings
// with an optional AI-written plain-language summary, mirroring the project's
// proven anti-hallucination split (see internal/movement and internal/research):
// Go owns every FACT — the form type, filing/report dates, accession URL, and the
// parsed item codes AND their canonical labels (the item-code → meaning map lives
// in internal/edgar, never the LLM). The LLM, when enabled, writes ONLY a short
// hedged summary of what each filing's source text says happened; it never invents
// numbers/dates/names and never gives advice. When the LLM is disabled, over the
// daily cap, errors, or the source text is too thin, the summary is simply absent
// and the filing still renders its item labels + source link. The LLM is NEVER on
// the critical path.
package materialevents

import (
	"context"
	"fmt"
	"strings"

	"github.com/wombow-ai/tickwind/internal/edgar"
)

// Disclaimer is the mandatory label shown when the LLM wrote a summary: AI-
// generated, factual, not advice. Mirrors movement.Disclaimer.
const Disclaimer = "AI 生成 · 仅供参考 · 非投资建议"

// Filing is one 8-K (or 8-K/A amendment) on the wire. Every field except Summary
// is a Go-owned fact (parsed from the SEC feed); Summary is the OPTIONAL LLM
// prose (empty when no LLM / too thin / errored).
type Filing struct {
	Form         string            `json:"form"`
	Amendment    bool              `json:"amendment"`
	FiledDate    string            `json:"filed_date"`
	ReportDate   string            `json:"report_date,omitempty"`
	AccessionURL string            `json:"accession_url"`
	Items        []edgar.EventItem `json:"items"`
	Summary      string            `json:"summary,omitempty"`
}

// Report is the assembled material-events response for one ticker. Filings is
// newest-first and is ALWAYS non-nil (an existing company with no recent 8-Ks
// yields an empty slice — never null). LLM/Model report whether any summary was
// AI-written and with which model (for transparency + the disclaimer).
type Report struct {
	Ticker     string   `json:"ticker"`
	Filings    []Filing `json:"filings"`
	LLM        bool     `json:"llm"`
	Model      string   `json:"model"`
	Disclaimer string   `json:"disclaimer,omitempty"`
}

// EventFetcher is the narrow EDGAR slice the service needs: list a ticker's recent
// 8-Ks (facts only) and fetch the plain-text source of one filing for summarizing.
// Satisfied by *edgar.Client. The summary source fetch is best-effort — a failing
// fetch degrades that filing to "no summary", never an error for the whole report.
type EventFetcher interface {
	// MaterialEvents returns the ticker's recent 8-K / 8-K/A filings (facts only),
	// newest first. Returns an error only when the ticker/CIK can't be resolved or
	// the feed fetch fails; an existing company with zero recent 8-Ks returns an
	// empty slice and nil error.
	MaterialEvents(ctx context.Context, ticker string) ([]edgar.MaterialEvent, error)
	// EventSummarySource fetches the bounded plain-text body of one filing's
	// primary document for LLM input, or "" (no error) when unavailable/too thin.
	EventSummarySource(ctx context.Context, ev edgar.MaterialEvent) (string, error)
}

// Enricher is the narrow LLM slice the service needs: a guarded call that writes a
// short factual summary from a filing's source material. Satisfied by
// enrich.Enricher (SummarizeFiling). Trivially fakeable in tests.
type Enricher interface {
	// Enabled reports whether a real LLM backend is configured.
	Enabled() bool
	// SummarizeFiling writes a short plain-language summary of an 8-K filing from
	// the pre-built material string. Returns an error when disabled or on failure.
	SummarizeFiling(ctx context.Context, material, lang string) (string, error)
}

// Service is the material-events façade the API handler holds. It owns the EDGAR
// fetcher and the optional enricher, exposing Report (the Go-owned facts + an
// optional AI summary per filing). The LLM is never on the critical path — Report
// degrades to facts-only when the LLM is disabled/over-cap/errors.
type Service struct {
	edgar EventFetcher
	enr   Enricher
	model string
	// maxSummaries bounds how many filings in one report get an LLM summary call —
	// the freshest N — so a chatty filer doesn't fan out into many LLM calls.
	maxSummaries int
}

// NewService builds a material-events Service. The enricher may be a disabled
// Noop; the service serves the facts-only report regardless. model is the
// configured LLM model name surfaced for transparency ("" when disabled).
func NewService(ef EventFetcher, enr Enricher, model string) *Service {
	return &Service{edgar: ef, enr: enr, model: model, maxSummaries: defaultMaxSummaries}
}

// defaultMaxSummaries caps how many of a report's (newest) filings get an LLM
// summary call, to bound per-request token spend.
const defaultMaxSummaries = 6

// Report assembles the facts-only material-events report (no LLM): the Go-owned
// 8-K facts, newest first, each with item labels but no summary. It returns an
// error only when the ticker/CIK can't be resolved or the feed fetch fails (the
// handler 404s on that); an existing company with zero recent 8-Ks yields an
// empty (non-nil) Filings slice and nil error.
func (s *Service) Report(ctx context.Context, ticker string) (Report, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	rep := Report{Ticker: ticker, Filings: []Filing{}, Model: ""}
	if s.edgar == nil {
		return rep, fmt.Errorf("materialevents: no edgar fetcher")
	}
	events, err := s.edgar.MaterialEvents(ctx, ticker)
	if err != nil {
		return Report{}, err
	}
	for _, ev := range events {
		rep.Filings = append(rep.Filings, toFiling(ev))
	}
	return rep, nil
}

// Summarize returns the report with each (newest, up to maxSummaries) filing's
// AI summary filled in when the LLM is enabled and the call succeeds, otherwise
// the unchanged facts-only report. It NEVER errors beyond the facts fetch — a
// per-filing source-fetch or LLM failure degrades that filing to no summary, the
// rest are unaffected. The Go-owned facts are untouched by the LLM.
func (s *Service) Summarize(ctx context.Context, ticker, lang string) (Report, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if s.edgar == nil {
		return Report{Ticker: ticker, Filings: []Filing{}}, fmt.Errorf("materialevents: no edgar fetcher")
	}
	events, err := s.edgar.MaterialEvents(ctx, ticker)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Ticker: ticker, Filings: make([]Filing, 0, len(events)), Model: ""}
	useLLM := s.enr != nil && s.enr.Enabled()

	for i, ev := range events {
		f := toFiling(ev)
		// Only summarize the freshest maxSummaries filings, and only when the LLM
		// is on — everything else is served facts-only (off the critical path).
		if useLLM && i < s.maxSummaries {
			if text := s.summarizeOne(ctx, ev, lang); text != "" {
				f.Summary = text
				rep.LLM = true
			}
		}
		rep.Filings = append(rep.Filings, f)
	}
	if rep.LLM {
		rep.Model = s.model
		rep.Disclaimer = Disclaimer
	}
	return rep, nil
}

// summarizeOne fetches one filing's source text and asks the LLM for a short
// summary. Returns "" (never an error) when the source is too thin, the fetch
// fails, or the LLM errors — the caller then serves that filing facts-only. Never
// fabricates: a "暂无足够信息" / "Not enough information" sentinel from the model is
// dropped to "" so the UI shows item labels rather than a hollow summary.
func (s *Service) summarizeOne(ctx context.Context, ev edgar.MaterialEvent, lang string) string {
	src, err := s.edgar.EventSummarySource(ctx, ev)
	if err != nil || strings.TrimSpace(src) == "" {
		return "" // unavailable / too thin → no summary, never fabricate
	}
	material := buildMaterial(ev, src, lang)
	text, err := s.enr.SummarizeFiling(ctx, material, lang)
	if err != nil {
		return ""
	}
	text = strings.TrimSpace(text)
	if isInsufficient(text) {
		return ""
	}
	return text
}

// isInsufficient reports whether the model's reply is the "not enough info"
// sentinel (in either language) — treated as no summary so the UI degrades to
// item labels rather than showing an empty/hollow line.
func isInsufficient(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return true
	}
	return strings.Contains(text, "暂无足够信息") ||
		strings.Contains(t, "not enough information") ||
		strings.Contains(t, "not enough info")
}

// buildMaterial assembles the single material string the LLM sees for one filing:
// the Go-owned item-category context (the canonical labels — NOT to be altered)
// plus the body excerpt. The LLM summarizes only what the excerpt states, using
// the item categories as context. Mirrors movement.Material's style.
func buildMaterial(ev edgar.MaterialEvent, src, lang string) string {
	var sb strings.Builder
	if lang == "en" {
		fmt.Fprintf(&sb, "Filing type: %s\n", ev.Form)
		if len(ev.Items) > 0 {
			sb.WriteString("Official item categories (system-provided, do not alter):\n")
			for _, it := range ev.Items {
				fmt.Fprintf(&sb, "- %s (%s)\n", it.LabelEN, it.Code)
			}
		}
		sb.WriteString("Filing body excerpt (summarize only what this states):\n")
	} else {
		fmt.Fprintf(&sb, "公告类型: %s\n", ev.Form)
		if len(ev.Items) > 0 {
			sb.WriteString("官方事项类别(由系统提供,不得改动):\n")
			for _, it := range ev.Items {
				fmt.Fprintf(&sb, "- %s(%s)\n", it.LabelZH, it.Code)
			}
		}
		sb.WriteString("公告正文节选(只能根据以下内容总结):\n")
	}
	sb.WriteString(src)
	return sb.String()
}

// toFiling maps a Go-owned edgar.MaterialEvent to the wire Filing (facts only;
// Summary stays empty until a caller fills it). Items is coerced non-nil.
func toFiling(ev edgar.MaterialEvent) Filing {
	items := ev.Items
	if items == nil {
		items = []edgar.EventItem{}
	}
	return Filing{
		Form:         ev.Form,
		Amendment:    ev.Amendment,
		FiledDate:    ev.FiledDate,
		ReportDate:   ev.ReportDate,
		AccessionURL: ev.AccessionURL,
		Items:        items,
	}
}

// Enabled reports whether the held enricher has a real LLM backend configured.
func (s *Service) Enabled() bool {
	return s.enr != nil && s.enr.Enabled()
}

// Model returns the configured LLM model name, or "" when the LLM is disabled.
func (s *Service) Model() string {
	if !s.Enabled() {
		return ""
	}
	return s.model
}
