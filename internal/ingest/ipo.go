package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/nasdaq"
)

// IPOCalendarSource fetches one month's Nasdaq IPO calendar. Implemented by
// *nasdaq.Client. ok-by-error: a datacenter-IP block / empty body returns an
// error, so the ingestor keeps its previous good snapshot.
type IPOCalendarSource interface {
	Calendar(ctx context.Context, month time.Time) (nasdaq.Calendar, error)
}

// ipoSnapshot is the immutable value swapped atomically into the cache.
type ipoSnapshot struct {
	cal       nasdaq.Calendar
	updatedAt time.Time
}

// IPOIngestor periodically pulls the current month's US IPO calendar (Nasdaq,
// via the residential proxy) into an atomically-swapped cache. The calendar
// moves slowly (a few priced/upcoming deals a day), so a multi-hour cadence is
// ample; a failed fetch (proxy unconfigured, block, or transient error) keeps
// the previous good snapshot rather than blanking the board.
type IPOIngestor struct {
	src   IPOCalendarSource
	every time.Duration
	now   func() time.Time // injectable clock for tests
	log   *slog.Logger

	mu   sync.RWMutex
	snap ipoSnapshot
}

// NewIPOIngestor builds the ingestor over a calendar source; call Run to start.
func NewIPOIngestor(src IPOCalendarSource, every time.Duration, log *slog.Logger) *IPOIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &IPOIngestor{
		src:   src,
		every: every,
		now:   func() time.Time { return time.Now().UTC() },
		log:   log,
	}
}

// Run refreshes immediately (a startup warm) and then on every tick until ctx is
// cancelled.
func (i *IPOIngestor) Run(ctx context.Context) {
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

// refresh fetches the current month's calendar and swaps it in on success;
// on failure it logs and keeps the previous snapshot.
func (i *IPOIngestor) refresh(ctx context.Context) {
	cal, err := i.src.Calendar(ctx, i.now())
	if err != nil {
		i.log.Warn("ipo calendar refresh failed (keeping previous)", "err", err)
		return
	}
	i.mu.Lock()
	i.snap = ipoSnapshot{cal: cal, updatedAt: i.now()}
	i.mu.Unlock()
	i.log.Info("ipo calendar refreshed", "priced", len(cal.Priced), "upcoming", len(cal.Upcoming), "filed", len(cal.Filed))
}

// Calendar returns the latest snapshot + when it was fetched. Before the first
// successful refresh the calendar's slices are nil; the API handler coerces them
// to [] so the response is always a well-formed shape.
func (i *IPOIngestor) Calendar() (nasdaq.Calendar, time.Time) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.snap.cal, i.snap.updatedAt
}
