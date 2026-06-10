package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/finra"
)

// shortPageSize is the rows-per-request when paging a settlement partition
// (~16k symbols per period → a handful of pages, refreshed daily).
const shortPageSize = 5000

// ShortSource fetches one page of short-interest rows for an exact settlement
// date. Implemented by *finra.Client.
type ShortSource interface {
	Rows(ctx context.Context, settlementDate string, limit, offset int) ([]finra.ShortInterest, error)
}

// ShortCache holds the full latest-settlement short-interest table keyed by
// symbol. The data only changes twice a month (published with a ~10-day lag),
// so a daily sweep is plenty; a failed sweep keeps the previous table.
type ShortCache struct {
	src   ShortSource
	every time.Duration
	log   *slog.Logger

	mu         sync.RWMutex
	bySym      map[string]finra.ShortInterest
	settlement string // the period the table holds, YYYY-MM-DD
}

// NewShortCache builds the cache; call Run to start sweeping.
func NewShortCache(src ShortSource, every time.Duration, log *slog.Logger) *ShortCache {
	if log == nil {
		log = slog.Default()
	}
	return &ShortCache{src: src, every: every, log: log}
}

// ShortInterest returns the latest-period row for a symbol.
func (c *ShortCache) ShortInterest(ticker string) (finra.ShortInterest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	si, ok := c.bySym[ticker]
	return si, ok
}

// Settlement returns the period the cache currently holds ("" before the
// first successful sweep).
func (c *ShortCache) Settlement() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settlement
}

// Run sweeps immediately and then on every tick until ctx is cancelled.
func (c *ShortCache) Run(ctx context.Context) {
	c.sweep(ctx)
	t := time.NewTicker(c.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.sweep(ctx)
		}
	}
}

// sweep probes candidate settlement dates newest-first until one has rows,
// then pages that whole partition into the table. Any error aborts the sweep
// and keeps the previous table (stale beats empty for twice-monthly data).
func (c *ShortCache) sweep(ctx context.Context) {
	for _, date := range finra.LatestSettlementCandidates(time.Now().UTC(), 2) {
		if date == c.Settlement() {
			return // already holding the newest published period
		}
		rows, err := c.src.Rows(ctx, date, shortPageSize, 0)
		if err != nil {
			c.log.Warn("short-interest sweep failed", "date", date, "err", err)
			return
		}
		if len(rows) == 0 {
			continue // not published (or not a settlement date) — try older
		}
		all := rows
		for offset := shortPageSize; len(rows) == shortPageSize; offset += shortPageSize {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond): // politeness between pages
			}
			rows, err = c.src.Rows(ctx, date, shortPageSize, offset)
			if err != nil {
				c.log.Warn("short-interest page failed", "date", date, "offset", offset, "err", err)
				return
			}
			all = append(all, rows...)
		}
		bySym := make(map[string]finra.ShortInterest, len(all))
		for _, si := range all {
			bySym[si.Symbol] = si
		}
		c.mu.Lock()
		c.bySym = bySym
		c.settlement = date
		c.mu.Unlock()
		c.log.Info("short-interest table refreshed", "settlement", date, "symbols", len(bySym))
		return
	}
	c.log.Warn("short-interest sweep: no published settlement period found")
}
