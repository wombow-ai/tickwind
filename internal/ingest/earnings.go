package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// EarningsSource fetches the earnings calendar in a date range (satisfied by
// *finnhub.Client).
type EarningsSource interface {
	EarningsCalendar(ctx context.Context, from, to time.Time) ([]store.Earning, error)
}

// EarningsIngestor periodically refreshes the upcoming/just-reported earnings
// calendar from Finnhub into the store. Own goroutine, slow cadence; only started
// when a Finnhub token is configured.
type EarningsIngestor struct {
	store store.Store
	src   EarningsSource
	every time.Duration
	log   *slog.Logger
}

// NewEarningsIngestor builds the ingestor; every is the refresh cadence (e.g. 6h).
func NewEarningsIngestor(st store.Store, src EarningsSource, every time.Duration, log *slog.Logger) *EarningsIngestor {
	return &EarningsIngestor{store: st, src: src, every: every, log: log}
}

// Run refreshes immediately, then on every tick, until ctx is cancelled.
func (e *EarningsIngestor) Run(ctx context.Context) {
	e.refresh(ctx)
	t := time.NewTicker(e.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.refresh(ctx)
		}
	}
}

func (e *EarningsIngestor) refresh(ctx context.Context) {
	now := time.Now().UTC()
	es, err := e.src.EarningsCalendar(ctx, now.AddDate(0, 0, -2), now.AddDate(0, 0, 45))
	if err != nil {
		e.log.Warn("earnings: fetch failed", "err", err)
		return
	}
	if err := e.store.SaveEarnings(ctx, es); err != nil {
		e.log.Warn("earnings: save failed", "err", err)
		return
	}
	e.log.Info("earnings: refreshed calendar", "count", len(es))
}
