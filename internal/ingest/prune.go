package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/config"
	"github.com/wombow-ai/tickwind/internal/store"
)

// Pruner periodically bounds the durable, append-mostly market tables (news,
// social, filings, insider_buys, seen_form4) so storage reaches a steady state
// instead of growing linearly forever. It runs off the request path on its own
// goroutine, mirroring the ingest scheduler's initial-pass + ticker shape.
//
// The retention windows are tiered per the owner's intent: old non-key data is
// evicted, hot-list tickers keep a longer window, and the 大V / KOL rail
// (protected social sources, e.g. "substack") is kept indefinitely.
type Pruner struct {
	store store.Pruner
	cfg   config.RetentionConfig
	every time.Duration
	log   *slog.Logger
}

// NewPruner builds the retention pruner. A non-positive PRUNE_EVERY defaults to 6h.
func NewPruner(p store.Pruner, cfg config.RetentionConfig, log *slog.Logger) *Pruner {
	every := cfg.Every
	if every <= 0 {
		every = 6 * time.Hour
	}
	return &Pruner{store: p, cfg: cfg, every: every, log: log}
}

// Run executes an initial pass immediately, then prunes every `every` until ctx
// is cancelled.
func (p *Pruner) Run(ctx context.Context) {
	p.runOnce(ctx)
	t := time.NewTicker(p.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.runOnce(ctx)
		}
	}
}

func (p *Pruner) runOnce(ctx context.Context) {
	now := time.Now().UTC()
	ago := func(days int) time.Time { return now.AddDate(0, 0, -days) }
	c := p.cfg
	var total int64

	step := func(name string, fn func() (int64, error)) {
		n, err := fn()
		if err != nil {
			p.log.Warn("prune step failed", "table", name, "err", err)
			return
		}
		if n > 0 {
			p.log.Info("pruned rows", "table", name, "rows", n)
		}
		total += n
	}

	if c.NewsDays > 0 {
		// hot window must be at least the normal window (longer = more days = earlier).
		step("news", func() (int64, error) {
			return p.store.PruneNews(ctx, ago(c.NewsDays), ago(maxInt(c.NewsHotDays, c.NewsDays)))
		})
	}
	if c.SocialDays > 0 {
		step("social", func() (int64, error) {
			return p.store.PruneSocial(ctx, ago(c.SocialDays), ago(maxInt(c.SocialHotDays, c.SocialDays)), c.ProtectSocialSources)
		})
	}
	if c.FilingsDays > 0 {
		step("filings", func() (int64, error) { return p.store.PruneFilings(ctx, ago(c.FilingsDays)) })
	}
	if c.InsiderDays > 0 {
		step("insider_buys", func() (int64, error) { return p.store.PruneInsiderBuys(ctx, ago(c.InsiderDays)) })
	}
	if c.SeenForm4Days > 0 {
		step("seen_form4", func() (int64, error) { return p.store.PruneSeenForm4(ctx, ago(c.SeenForm4Days)) })
	}
	if c.CapNewsPerTicker > 0 {
		step("news_cap", func() (int64, error) { return p.store.CapPerTicker(ctx, "news", c.CapNewsPerTicker) })
	}
	if c.CapSocialPerTicker > 0 {
		step("social_cap", func() (int64, error) { return p.store.CapPerTicker(ctx, "social", c.CapSocialPerTicker) })
	}

	if total > 0 {
		p.log.Info("prune pass complete", "total_rows", total)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
