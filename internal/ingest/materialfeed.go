package ingest

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/materialevents"
)

// materialFeedScanEvery / Pace / Max: an 8-K only appears when a company files one, so an hourly
// refresh is ample; the pace spaces the per-ticker SEC fetches so a scan never bursts (the shared
// edgar client also self-throttles); Max caps the feed at the newest notable events.
const (
	materialFeedScanEvery = 60 * time.Minute
	materialFeedScanPace  = 80 * time.Millisecond
	materialFeedMax       = 120
	// materialFeedPerTicker: how many recent 8-Ks to pull PER TICKER before filtering to notable
	// items — higher than the per-stock view's 10 so routine filings (earnings/exhibits) don't crowd a
	// ticker's NOTABLE events out of the window (which left the feed thin at the default cap).
	materialFeedPerTicker = 40
)

// MaterialEventSource yields a ticker's recent 8-K material events, capped at `max` (FACTS ONLY — no
// LLM; satisfied by *edgar.Client). Declared here so this package needn't import api.
type MaterialEventSource interface {
	MaterialEventsN(ctx context.Context, ticker string, max int) ([]edgar.MaterialEvent, error)
}

// MaterialFeedCache precomputes a market-wide feed of NOTABLE recent 8-K events (leadership change,
// M&A, bankruptcy, restatement, …) across the bounded tracked universe (analyticTickers), off the
// request path — so a "/events" feed surfaces the high-signal corporate filings of recognizable names
// without a per-request fan-out. Anti-hallucination-safe: Go owns every item code + label and there is
// NO LLM on the feed. On a total miss it keeps the previous feed rather than blanking it.
type MaterialFeedCache struct {
	src     MaterialEventSource
	tickers TickerSource
	every   time.Duration
	log     *slog.Logger

	mu   sync.RWMutex
	feed []materialevents.FeedEvent
	at   time.Time
}

// NewMaterialFeedCache builds the cache over a bounded TickerSource (pass analyticTickers). A nil
// logger is tolerated (discarded).
func NewMaterialFeedCache(src MaterialEventSource, tickers TickerSource, log *slog.Logger) *MaterialFeedCache {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &MaterialFeedCache{src: src, tickers: tickers, every: materialFeedScanEvery, log: log}
}

// Run scans immediately, then every `every` until ctx is cancelled, on a background goroutine.
func (c *MaterialFeedCache) Run(ctx context.Context) {
	c.scan(ctx)
	t := time.NewTicker(c.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.scan(ctx)
		}
	}
}

// scan fetches each tracked ticker's recent 8-Ks, keeps only events carrying a NOTABLE item code,
// aggregates them newest-first, caps the feed, and atomically swaps it in. A ticker whose fetch fails
// or has no notable event simply contributes nothing (never fabricated). On a total miss it keeps the
// previous feed.
func (c *MaterialFeedCache) scan(ctx context.Context) {
	tickers := c.tickers(ctx)
	var all []materialevents.FeedEvent
	for i, tk := range tickers {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(materialFeedScanPace):
			}
		}
		events, err := c.src.MaterialEventsN(ctx, tk, materialFeedPerTicker)
		if err != nil {
			continue
		}
		for _, ev := range events {
			notable := materialevents.NotableItems(ev.Items)
			if len(notable) == 0 {
				continue // only routine items (earnings/exhibits/Reg FD) → not on the feed
			}
			all = append(all, materialevents.FeedEvent{
				Ticker:       tk,
				Form:         ev.Form,
				FiledDate:    ev.FiledDate,
				ReportDate:   ev.ReportDate,
				AccessionURL: ev.AccessionURL,
				Items:        notable,
			})
		}
	}
	if len(all) == 0 {
		return // empty scan — keep the previous feed rather than blanking it
	}
	// Newest filing first; ticker tie-break keeps same-day ordering deterministic.
	sort.SliceStable(all, func(x, y int) bool {
		if all[x].FiledDate != all[y].FiledDate {
			return all[x].FiledDate > all[y].FiledDate
		}
		return all[x].Ticker < all[y].Ticker
	})
	if len(all) > materialFeedMax {
		all = all[:materialFeedMax]
	}
	c.mu.Lock()
	c.feed, c.at = all, time.Now().UTC()
	c.mu.Unlock()
	c.log.Debug("material-events feed refreshed", "events", len(all))
}

// Feed returns the cached notable-event feed (newest first) + when it was built. An optional item
// code (e.g. "5.02") filters to events carrying that code. Reads the cache; the scan swaps in a fresh
// slice (never mutates), so grabbing the slice reference under the lock then filtering outside it is
// race-safe. The returned slice must not be mutated by the caller.
func (c *MaterialFeedCache) Feed(item string) ([]materialevents.FeedEvent, time.Time) {
	c.mu.RLock()
	feed, at := c.feed, c.at
	c.mu.RUnlock()
	if item == "" {
		return feed, at
	}
	out := make([]materialevents.FeedEvent, 0, len(feed))
	for _, e := range feed {
		for _, it := range e.Items {
			if it.Code == item {
				out = append(out, e)
				break
			}
		}
	}
	return out, at
}
