package movement

import (
	"context"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// StoreReader is the narrow, read-only slice of store.Store the service needs to
// gather a ticker's quote and evidence corpus. Every method is nil-safe at the
// call site. Satisfied by the in-process store.Store.
type StoreReader interface {
	// GetQuote returns the ticker's latest quote (ok=false when none).
	GetQuote(ctx context.Context, ticker string) (store.Quote, bool, error)
	// ListNews returns a ticker's recent news, newest first.
	ListNews(ctx context.Context, ticker string, limit int) ([]store.News, error)
	// ListFilings returns a ticker's recent filings, newest first.
	ListFilings(ctx context.Context, ticker string, limit int) ([]store.Filing, error)
	// RecentInsiderBuys returns insider open-market buys filed on/after since
	// (across all tickers; the assembler filters to this ticker).
	RecentInsiderBuys(ctx context.Context, since time.Time) ([]store.InsiderBuy, error)
}

// QuoteProvider returns a ticker's latest (delayed) quote, reading the polled
// quote first and falling back to an on-demand fetch — the same fallback the
// research report and fundamentals card use, so the move % is computed from the
// same number the rest of the app shows. ok=false when no price is available.
// nil-safe in the service (a nil provider falls back to the store's GetQuote).
type QuoteProvider interface {
	Quote(ctx context.Context, ticker string) (store.Quote, bool)
}

// Enricher is the narrow LLM slice the service needs: a guarded call that writes
// ONE hedged Chinese sentence from the move material. Satisfied by enrich.Enricher
// (ExplainMove). Trivially fakeable in tests.
type Enricher interface {
	// Enabled reports whether a real LLM backend is configured.
	Enabled() bool
	// ExplainMove writes one short hedged explanation of a price move from the
	// pre-built material string (the move number + attributed evidence). Returns an
	// error when disabled or on failure.
	ExplainMove(ctx context.Context, material, lang string) (string, error)
}

// Service is the movement façade the API handler holds. It owns the read-only
// store, the quote provider, and the optional enricher, exposing the data-only
// Assemble (the Go-owned number + evidence + canned line) and an LLM-prose
// Explain. The LLM is never on the critical path — Explain degrades to the
// data-only explanation when the LLM is disabled/over-cap/errors.
type Service struct {
	store StoreReader
	quote QuoteProvider
	enr   Enricher
	model string
}

// NewService builds a movement Service. The enricher may be a disabled Noop; the
// service serves the data-only explanation regardless. quote may be nil (the
// service then reads the quote straight from the store). model is the configured
// LLM model name surfaced for transparency ("" when disabled).
func NewService(st StoreReader, quote QuoteProvider, enr Enricher, model string) *Service {
	return &Service{store: st, quote: quote, enr: enr, model: model}
}

// inputs gathers the ticker's quote and evidence corpus from the store/provider.
// Each read is best-effort: a failing read degrades to no items, never an error.
func (s *Service) inputs(ctx context.Context, ticker string) Inputs {
	var in Inputs
	if s.quote != nil {
		if q, ok := s.quote.Quote(ctx, ticker); ok {
			in.Quote = q
		}
	}
	if in.Quote.Price <= 0 && s.store != nil {
		if q, ok, _ := s.store.GetQuote(ctx, ticker); ok {
			in.Quote = q
		}
	}
	if s.store != nil {
		in.News, _ = s.store.ListNews(ctx, ticker, 10)
		in.Filings, _ = s.store.ListFilings(ctx, ticker, 10)
		if buys, err := s.store.RecentInsiderBuys(ctx, time.Now().Add(-insiderLookback)); err == nil {
			for _, b := range buys {
				if strings.EqualFold(b.Ticker, ticker) {
					in.Insider = append(in.Insider, b)
				}
			}
		}
	}
	return in
}

// Report assembles the data-only Explanation (no LLM, cheap, never errors): the
// Go-owned move % + direction, the attributed evidence, and a canned Chinese
// line. Significant=false (with no Text/Evidence) for a sub-threshold move.
func (s *Service) Report(ctx context.Context, ticker string) Explanation {
	return Assemble(ticker, s.inputs(ctx, ticker))
}

// Explain returns the Explanation with the LLM's hedged sentence when the LLM is
// enabled and the call succeeds, otherwise the unchanged data-only explanation
// (canned line, LLM=false). It NEVER errors. The Go-owned number, direction,
// significance, and evidence are untouched by the LLM — only Text may change.
func (s *Service) Explain(ctx context.Context, ticker, lang string) Explanation {
	exp := s.Report(ctx, ticker)
	if !exp.Significant {
		return exp // no explanation warranted for a sub-threshold move
	}
	if s.enr == nil || !s.enr.Enabled() {
		return exp // data-only canned line
	}
	material := Material(exp)
	if strings.TrimSpace(material) == "" {
		return exp
	}
	text, err := s.enr.ExplainMove(ctx, material, lang)
	if err != nil || strings.TrimSpace(text) == "" {
		return exp // degrade to the canned line; never propagate the error
	}
	exp.Text = strings.TrimSpace(text)
	exp.LLM = true
	exp.Model = s.model
	exp.Disclaimer = Disclaimer
	return exp
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
