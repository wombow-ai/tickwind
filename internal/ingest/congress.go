package ingest

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress"
)

// CongressFetcher fetches recent House Periodic Transaction Reports for a year
// (satisfied by *congress.Client).
type CongressFetcher interface {
	FetchHousePTRs(ctx context.Context, year int) ([]congress.Filing, error)
}

// CongressIngestor refreshes the in-memory cache of recent congressional
// Periodic Transaction Reports (PTRs) from the public-domain House Clerk dataset
// on a slow cadence. Runs in its own goroutine, off the request path; memory-only
// + rebuildable, like the Universe / Opportunity caches (no per-refresh DB write).
type CongressIngestor struct {
	client CongressFetcher
	cache  *congress.Cache
	every  time.Duration
	max    int
	log    *slog.Logger
}

// NewCongressIngestor builds the ingestor; every is the refresh cadence.
func NewCongressIngestor(client CongressFetcher, cache *congress.Cache, every time.Duration, log *slog.Logger) *CongressIngestor {
	return &CongressIngestor{client: client, cache: cache, every: every, max: 150, log: log}
}

// Run refreshes once on startup, then on the cadence, until ctx is cancelled.
func (c *CongressIngestor) Run(ctx context.Context) {
	c.refresh(ctx)
	t := time.NewTicker(c.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.refresh(ctx)
		}
	}
}

func (c *CongressIngestor) refresh(ctx context.Context) {
	year := time.Now().UTC().Year()
	all, err := c.client.FetchHousePTRs(ctx, year)
	if err != nil {
		c.log.Warn("congress: fetch failed", "year", year, "err", err)
	}
	// The annual index is sparse early in the year (and a year-boundary view wants
	// the prior year's tail). Pull the prior year too when the current one is thin
	// or errored, then merge.
	if len(all) < 50 || err != nil {
		if prev, perr := c.client.FetchHousePTRs(ctx, year-1); perr != nil {
			c.log.Warn("congress: prior-year fetch failed", "year", year-1, "err", perr)
		} else {
			all = append(all, prev...)
		}
	}
	if len(all) == 0 {
		return
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].FiledDate.After(all[j].FiledDate) })
	if len(all) > c.max {
		all = all[:c.max]
	}
	c.cache.Set(all)
	c.log.Info("congress: refreshed", "filings", len(all))
}
