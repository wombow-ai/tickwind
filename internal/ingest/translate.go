package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/store"
)

// translateBatch is the headlines-per-LLM-request size: large enough to
// amortize the prompt, small enough that one malformed reply wastes little.
const translateBatch = 40

// translateStore is the slice of the Store the translator needs.
type translateStore interface {
	ListUntranslatedNews(ctx context.Context, limit int) ([]store.News, error)
	SetNewsTranslation(ctx context.Context, ticker, id, headlineZH string) error
}

// TranslateIngestor fills in Chinese headlines for ingested English news via
// the LLM enricher, newest first, one batch per sweep. Headlines are immutable
// so each row is translated exactly once (≈$0.00001/title via DeepSeek); a
// failed sweep just retries next tick.
type TranslateIngestor struct {
	store translateStore
	enr   enrich.Enricher
	every time.Duration
	log   *slog.Logger
}

// NewTranslateIngestor builds the ingestor; call Run to start.
func NewTranslateIngestor(st translateStore, enr enrich.Enricher, every time.Duration, log *slog.Logger) *TranslateIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &TranslateIngestor{store: st, enr: enr, every: every, log: log}
}

// Run sweeps immediately and then on every tick until ctx is cancelled.
func (t *TranslateIngestor) Run(ctx context.Context) {
	t.sweep(ctx)
	tick := time.NewTicker(t.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.sweep(ctx)
		}
	}
}

func (t *TranslateIngestor) sweep(ctx context.Context) {
	if !t.enr.Enabled() {
		return
	}
	items, err := t.store.ListUntranslatedNews(ctx, translateBatch)
	if err != nil {
		t.log.Warn("translate: list failed", "err", err)
		return
	}
	if len(items) == 0 {
		return
	}
	titles := make([]string, len(items))
	for i, n := range items {
		titles[i] = n.Headline
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	zh, err := t.enr.TranslateTitles(cctx, titles)
	if err != nil {
		t.log.Warn("translate: llm failed", "count", len(titles), "err", err)
		return
	}
	saved := 0
	for i, n := range items {
		if zh[i] == "" {
			continue
		}
		if err := t.store.SetNewsTranslation(ctx, n.Ticker, n.ID, zh[i]); err != nil {
			t.log.Warn("translate: save failed", "ticker", n.Ticker, "err", err)
			continue
		}
		saved++
	}
	t.log.Info("translated news headlines", "batch", len(items), "saved", saved)
}
