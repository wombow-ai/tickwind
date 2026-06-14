package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/treasury"
)

// TreasuryFetcher fetches the latest daily par yield curve. Implemented by
// *treasury.Client. Keyless.
type TreasuryFetcher interface {
	Latest(ctx context.Context) (treasury.Curve, error)
}

// MacroIngestor refreshes the U.S. Treasury daily par yield curve into a
// *treasury.Cache on its own goroutine. The curve updates once per business day
// (around 18:00 ET), so a ~12h cadence is ample; the boot refresh warms the
// cache immediately. A failed fetch keeps the last good curve (the cache is only
// Set on success) and logs a WARN, so a transient Treasury outage never blanks
// the macro strip. Keyless — no API key, no secret.
//
// This backs GET /v1/macro (the 2Y/10Y + 2s10s recession-watch strip). All data
// is server-driven from this cache; the request path never fetches Treasury.gov.
type MacroIngestor struct {
	src   TreasuryFetcher
	cache *treasury.Cache
	every time.Duration
	log   *slog.Logger
}

// NewMacroIngestor builds the ingestor over a fetcher and cache; call Run to
// start refreshing every `every`.
func NewMacroIngestor(src TreasuryFetcher, cache *treasury.Cache, every time.Duration, log *slog.Logger) *MacroIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &MacroIngestor{src: src, cache: cache, every: every, log: log}
}

// Cache exposes the underlying snapshot store for the API handler.
func (i *MacroIngestor) Cache() *treasury.Cache { return i.cache }

// Run refreshes immediately (a startup warm) and then on every tick until ctx is
// cancelled.
func (i *MacroIngestor) Run(ctx context.Context) {
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

// refresh fetches the latest yield curve and installs it on success. On error
// the previous snapshot is kept (stale beats empty for once-a-day data) and a
// WARN is logged.
func (i *MacroIngestor) refresh(ctx context.Context) {
	curve, err := i.src.Latest(ctx)
	if err != nil {
		i.log.Warn("treasury yield-curve refresh failed", "err", err)
		return
	}
	i.cache.Set(curve)
	i.log.Info("treasury yield curve refreshed", "as_of", curve.Date, "tenors", len(curve.Yields), "spread_2s10s", curve.Spread2s10s, "inverted", curve.Inverted)
}
