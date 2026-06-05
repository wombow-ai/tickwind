// Package ingest periodically pulls data from sources into the store.
// v1 has one source (EDGAR filings); price/news/social sources plug in here.
package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/store"
)

type Scheduler struct {
	store     store.Store
	edgar     *edgar.Client
	watchlist []string
	every     time.Duration
	log       *slog.Logger
}

func NewScheduler(st store.Store, ec *edgar.Client, watchlist []string, every time.Duration, log *slog.Logger) *Scheduler {
	return &Scheduler{store: st, edgar: ec, watchlist: watchlist, every: every, log: log}
}

// Run blocks until ctx is cancelled, refreshing every `every`.
func (s *Scheduler) Run(ctx context.Context) {
	s.runOnce(ctx)
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	for _, ticker := range s.watchlist {
		sec, filings, err := s.edgar.RecentFilings(ctx, ticker, 25)
		if err != nil {
			s.log.Warn("edgar fetch failed", "ticker", ticker, "err", err)
		} else {
			_ = s.store.UpsertSecurity(ctx, sec)
			_ = s.store.SaveFilings(ctx, ticker, filings)
			s.log.Info("ingested filings", "ticker", ticker, "name", sec.Name, "count", len(filings))
		}
		// Stay well under SEC's 10 req/s limit.
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}
