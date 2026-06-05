// Package ingest periodically pulls data from sources into the store.
// Filings (EDGAR), news (Finnhub) and social (StockTwits) refresh on the
// scheduler; prices have their own faster poller (price.go).
package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/finnhub"
	"github.com/wombow-ai/tickwind/internal/stocktwits"
	"github.com/wombow-ai/tickwind/internal/store"
)

type Scheduler struct {
	store      store.Store
	edgar      *edgar.Client
	finnhub    *finnhub.Client    // optional; nil disables news ingestion
	stocktwits *stocktwits.Client // optional; nil disables social ingestion
	watchlist  []string
	every      time.Duration
	log        *slog.Logger
}

// NewScheduler builds the filings+news+social scheduler. fh and st may be nil
// to disable news / social respectively.
func NewScheduler(st store.Store, ec *edgar.Client, fh *finnhub.Client, stw *stocktwits.Client, watchlist []string, every time.Duration, log *slog.Logger) *Scheduler {
	return &Scheduler{store: st, edgar: ec, finnhub: fh, stocktwits: stw, watchlist: watchlist, every: every, log: log}
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
		s.ingestFilings(ctx, ticker)
		s.ingestNews(ctx, ticker)
		s.ingestSocial(ctx, ticker)
		// Stay well under provider rate limits.
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (s *Scheduler) ingestFilings(ctx context.Context, ticker string) {
	sec, filings, err := s.edgar.RecentFilings(ctx, ticker, 25)
	if err != nil {
		s.log.Warn("edgar fetch failed", "ticker", ticker, "err", err)
		return
	}
	_ = s.store.UpsertSecurity(ctx, sec)
	_ = s.store.SaveFilings(ctx, ticker, filings)
	s.log.Info("ingested filings", "ticker", ticker, "name", sec.Name, "count", len(filings))
}

func (s *Scheduler) ingestNews(ctx context.Context, ticker string) {
	if s.finnhub == nil {
		return
	}
	items, err := s.finnhub.CompanyNews(ctx, ticker, 7)
	if err != nil {
		s.log.Warn("finnhub fetch failed", "ticker", ticker, "err", err)
		return
	}
	if err := s.store.SaveNews(ctx, ticker, items); err != nil {
		s.log.Warn("save news failed", "ticker", ticker, "err", err)
		return
	}
	s.log.Info("ingested news", "ticker", ticker, "count", len(items))
}

func (s *Scheduler) ingestSocial(ctx context.Context, ticker string) {
	if s.stocktwits == nil {
		return
	}
	posts, err := s.stocktwits.SymbolStream(ctx, ticker, 30)
	if err != nil {
		s.log.Warn("stocktwits fetch failed", "ticker", ticker, "err", err)
		return
	}
	if err := s.store.SaveSocial(ctx, ticker, posts); err != nil {
		s.log.Warn("save social failed", "ticker", ticker, "err", err)
		return
	}
	s.log.Info("ingested social", "ticker", ticker, "count", len(posts))
}
