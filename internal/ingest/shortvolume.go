package ingest

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/finrashvol"
)

// shortVolumeMaxFallback bounds how many prior calendar days a sweep walks back
// looking for a published FINRA daily short-volume file (weekends are skipped
// before the request, so this covers a long holiday weekend with room to spare).
const shortVolumeMaxFallback = 6

// ShortVolumeFetcher fetches and parses one trading day's FINRA short-volume
// file. Implemented by *finrashvol.Client. A missing/unpublished day returns
// finrashvol.ErrNoData so the sweep can fall back to the previous trading day.
type ShortVolumeFetcher interface {
	FetchDaily(ctx context.Context, date time.Time) ([]finrashvol.ShortVol, error)
}

// ShortVolumeIngestor pulls FINRA's daily consolidated short-volume file once per
// cycle and installs it into a *finrashvol.Cache. Files are published only on
// trading days and lag the close, so each sweep tries the target day and falls
// back to prior business days (skipping weekends) until it gets data.
//
// The cache it feeds backs the daily short-volume leaderboard (Cache.Top, a
// display-only ranking — never bulk raw rows) and the per-stock daily
// short-pressure curve (Cache.Latest/History).
type ShortVolumeIngestor struct {
	src   ShortVolumeFetcher
	cache *finrashvol.Cache
	every time.Duration
	now   func() time.Time // injectable clock for deterministic tests
	log   *slog.Logger
}

// NewShortVolumeIngestor builds the ingestor over a fetcher and cache; call Run
// to start sweeping every `every`.
func NewShortVolumeIngestor(src ShortVolumeFetcher, cache *finrashvol.Cache, every time.Duration, log *slog.Logger) *ShortVolumeIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &ShortVolumeIngestor{
		src:   src,
		cache: cache,
		every: every,
		now:   func() time.Time { return time.Now().UTC() },
		log:   log,
	}
}

// Run sweeps immediately (a startup warm) and then on every tick until ctx is
// cancelled. The daily file changes once per trading day, so the period is
// expected to be ~24h; a daily cadence is plenty.
func (i *ShortVolumeIngestor) Run(ctx context.Context) {
	i.sweep(ctx)
	t := time.NewTicker(i.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.sweep(ctx)
		}
	}
}

// sweep fetches the newest available daily file, walking back over prior business
// days on ErrNoData (weekend/holiday/not-yet-published), and installs it into the
// cache. A transient (non-ErrNoData) error aborts the sweep and keeps the
// previous snapshot — stale beats empty for once-a-day data. An ErrNoData run all
// the way back logs a warning and leaves the cache untouched.
func (i *ShortVolumeIngestor) sweep(ctx context.Context) {
	day := i.now()
	for tries := 0; tries < shortVolumeMaxFallback; tries++ {
		day = previousTradingDay(day, tries == 0)
		rows, err := i.src.FetchDaily(ctx, day)
		if errors.Is(err, finrashvol.ErrNoData) {
			continue // not published for this day — try the prior trading day
		}
		if err != nil {
			i.log.Warn("short-volume sweep failed", "date", day.Format("2006-01-02"), "err", err)
			return // transient error: keep the previous snapshot
		}
		i.cache.Set(rows)
		i.log.Info("short-volume refreshed", "as_of", i.cache.AsOf(), "symbols", i.cache.Len())
		return
	}
	i.log.Warn("short-volume sweep: no published daily file found in the lookback window")
}

// previousTradingDay returns the trading day to try. On the first attempt
// (keepToday) it returns the given day clamped off a weekend; otherwise it steps
// back one calendar day and clamps off weekends. FINRA publishes nothing on
// weekends, so those are skipped before a request rather than spending a fallback
// attempt on a guaranteed 404.
func previousTradingDay(day time.Time, keepToday bool) time.Time {
	if !keepToday {
		day = day.AddDate(0, 0, -1)
	}
	for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		day = day.AddDate(0, 0, -1)
	}
	return day
}
