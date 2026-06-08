package ingest

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/wombow-ai/tickwind/internal/symbols"
)

// SymbolIngestor loads the searchable symbol directory (SEC public-domain US
// listings) into a shared Cache and refreshes it on a slow cadence (listings
// change rarely). Needs no API key — just SEC's required User-Agent. Runs in its
// own goroutine; a failed refresh keeps the last good index.
type SymbolIngestor struct {
	cache     *symbols.Cache
	http      *http.Client
	userAgent string
	every     time.Duration
	log       *slog.Logger
}

// NewSymbolIngestor builds the ingestor. userAgent is sent to SEC (must include
// contact info); every is the refresh cadence (e.g. 24h).
func NewSymbolIngestor(cache *symbols.Cache, userAgent string, every time.Duration, log *slog.Logger) *SymbolIngestor {
	return &SymbolIngestor{
		cache:     cache,
		http:      &http.Client{Timeout: 30 * time.Second},
		userAgent: userAgent,
		every:     every,
		log:       log,
	}
}

// Run loads the directory immediately, then on every tick, until ctx is cancelled.
func (s *SymbolIngestor) Run(ctx context.Context) {
	s.refresh(ctx)
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refresh(ctx)
		}
	}
}

func (s *SymbolIngestor) refresh(ctx context.Context) {
	syms, err := symbols.FetchUS(ctx, s.http, s.userAgent)
	if err != nil {
		s.log.Warn("symbols: refresh failed", "err", err)
		return // keep serving the last good index
	}
	// Add free Nasdaq Trader listings (NYSE/Arca/Cboe/IEX) so ETFs and other
	// non-SEC-filer symbols (e.g. DRAM) are searchable too. Best-effort: on
	// failure we still build from the SEC directory alone.
	nt, err := symbols.FetchNasdaqTrader(ctx, s.http, s.userAgent)
	if err != nil {
		s.log.Warn("symbols: nasdaq trader fetch failed", "err", err)
	}
	// SEC first so its cleaner names/exchange win on ticker collisions; Nasdaq
	// Trader and the curated TW/HK seeds only add symbols SEC is missing.
	all := append(syms, nt...)
	all = append(all, symbols.ForeignSeeds()...)
	idx := symbols.Build(all)
	s.cache.Set(idx)
	s.log.Info("symbols: loaded directory", "count", idx.Len(), "sec", len(syms), "nasdaq", len(nt))
}
