package ingest

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/wombow-ai/tickwind/internal/events"
)

// EventsIngestor populates the major-events timeline: it merges the curated
// events (FOMC, world events) with the BLS economic-calendar feed into a shared
// Cache and refreshes daily. Key-free; if BLS fails it still serves the curated
// set. Runs in its own goroutine (slow cadence, independent of the scheduler).
type EventsIngestor struct {
	cache *events.Cache
	http  *http.Client
	every time.Duration
	log   *slog.Logger
}

// NewEventsIngestor builds the ingestor. every is the refresh cadence (e.g. 12h).
func NewEventsIngestor(cache *events.Cache, every time.Duration, log *slog.Logger) *EventsIngestor {
	return &EventsIngestor{
		cache: cache,
		http:  &http.Client{Timeout: 30 * time.Second},
		every: every,
		log:   log,
	}
}

// Run refreshes the timeline immediately, then on every tick, until ctx ends.
func (e *EventsIngestor) Run(ctx context.Context) {
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

func (e *EventsIngestor) refresh(ctx context.Context) {
	bls, err := events.FetchBLS(ctx, e.http)
	if err != nil {
		e.log.Warn("events: bls fetch failed", "err", err) // fall back to curated-only
	}
	merged := events.Merge(events.Curated(), bls)
	e.cache.Set(merged)
	e.log.Info("events: refreshed timeline", "total", len(merged), "bls", len(bls))
}
