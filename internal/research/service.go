package research

import "context"

// Service is the research façade the API handler holds. It owns the data Sources
// and the (optional) enricher, exposing a cheap data-only Report and an
// LLM-prose Compose. It is the type api.SetResearch wires in.
type Service struct {
	src       Sources
	enr       ResearchEnricher
	model     string
	deepModel string
}

// NewService builds a research Service from the data sources and the enricher.
// The enricher may be a disabled Noop (LLM_API_KEY empty); the service serves the
// data-only report regardless — the LLM is never on the critical path. model is
// the configured LLM model name surfaced for transparency ("" when disabled);
// deepModel is the model the deep (depth=deep) compose reports — pass "" to fall
// back to model (the same default the enricher itself applies).
func NewService(src Sources, enr ResearchEnricher, model, deepModel string) *Service {
	if deepModel == "" {
		deepModel = model
	}
	return &Service{src: src, enr: enr, model: model, deepModel: deepModel}
}

// Report assembles the data-only fact sheet (no LLM, cheap, never errors). lang
// ("en"/"zh") selects the language of every Go-built label embedded in a fact
// Value (flows trade/13F/short-trend labels, the Fear & Greed band, the loss
// placeholder) so the data-only sheet is correct for the request language even
// before any LLM prose.
func (s *Service) Report(ctx context.Context, ticker, lang string) FactSheet {
	return Assemble(ctx, ticker, lang, s.src)
}

// Compose fills per-section prose on an already-assembled fact sheet via the held
// enricher, degrading to the unchanged data-only sheet when the LLM is
// disabled/errors. It never errors.
func (s *Service) Compose(ctx context.Context, fs FactSheet, lang string) FactSheet {
	return Compose(ctx, fs, s.enr, lang)
}

// ComposeDeep is the deep-research counterpart of Compose: it fills richer
// per-section prose via the enricher's ComposeDeepReport (stronger model + Fable-5
// harness) over the SAME Go-owned facts, degrading identically to the data-only
// sheet when the LLM is disabled/errors. The anti-hallucination contract is
// unchanged — the LLM only ever touches Prose. It never errors.
func (s *Service) ComposeDeep(ctx context.Context, fs FactSheet, lang string) FactSheet {
	return ComposeDeep(ctx, fs, s.enr, lang)
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

// DeepModel returns the configured deep-compose model name (the stronger model
// when LLM_DEEP_MODEL is set, otherwise the normal model), or "" when the LLM is
// disabled.
func (s *Service) DeepModel() string {
	if !s.Enabled() {
		return ""
	}
	return s.deepModel
}
