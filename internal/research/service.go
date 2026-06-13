package research

import "context"

// Service is the research façade the API handler holds. It owns the data Sources
// and the (optional) enricher, exposing a cheap data-only Report and an
// LLM-prose Compose. It is the type api.SetResearch wires in.
type Service struct {
	src   Sources
	enr   ResearchEnricher
	model string
}

// NewService builds a research Service from the data sources and the enricher.
// The enricher may be a disabled Noop (LLM_API_KEY empty); the service serves the
// data-only report regardless — the LLM is never on the critical path. model is
// the configured LLM model name surfaced for transparency ("" when disabled).
func NewService(src Sources, enr ResearchEnricher, model string) *Service {
	return &Service{src: src, enr: enr, model: model}
}

// Report assembles the data-only fact sheet (no LLM, cheap, never errors).
func (s *Service) Report(ctx context.Context, ticker string) FactSheet {
	return Assemble(ctx, ticker, s.src)
}

// Compose fills per-section prose on an already-assembled fact sheet via the held
// enricher, degrading to the unchanged data-only sheet when the LLM is
// disabled/errors. It never errors.
func (s *Service) Compose(ctx context.Context, fs FactSheet, lang string) FactSheet {
	return Compose(ctx, fs, s.enr, lang)
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
