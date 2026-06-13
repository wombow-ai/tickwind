package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/ratecut"
)

// RateCutAggregator is the fault-tolerant multi-source driver the ingestor runs.
// Implemented by *ratecut.Aggregator: Refresh fetches each source once (caching
// successes, never clearing a failing source's last-good snapshot) and returns
// per-source errors; Cache exposes the snapshot store for the API.
type RateCutAggregator interface {
	Refresh(ctx context.Context) map[string]error
	Cache() *ratecut.Cache
}

// RateCutIngestor periodically refreshes the Fed rate-cut prediction markets
// (Kalshi + Polymarket) into the aggregator's cache. Prediction-market odds move
// slowly between FOMC meetings, so a 15–30 min cadence is ample; a per-source
// failure leaves that source's previous snapshot intact (handled by the
// aggregator), so one provider being down never blanks the board.
type RateCutIngestor struct {
	agg   RateCutAggregator
	every time.Duration
	log   *slog.Logger
}

// NewRateCutIngestor builds the ingestor; call Run to start refreshing.
func NewRateCutIngestor(agg RateCutAggregator, every time.Duration, log *slog.Logger) *RateCutIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &RateCutIngestor{agg: agg, every: every, log: log}
}

// Cache exposes the underlying snapshot store for the API handler.
func (i *RateCutIngestor) Cache() *ratecut.Cache { return i.agg.Cache() }

// Run refreshes immediately (a startup warm) and then on every tick until ctx is
// cancelled.
func (i *RateCutIngestor) Run(ctx context.Context) {
	i.refresh(ctx)
	t := time.NewTicker(i.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.refresh(ctx)
		}
	}
}

// refresh fetches every source once, logging any per-source failures. Successful
// sources are cached regardless of whether siblings failed.
func (i *RateCutIngestor) refresh(ctx context.Context) {
	errs := i.agg.Refresh(ctx)
	for source, err := range errs {
		i.log.Warn("ratecut source refresh failed", "source", source, "err", err)
	}
	if c := i.agg.Cache(); c != nil {
		i.log.Info("ratecut refreshed", "sources", c.Len())
	}
}
