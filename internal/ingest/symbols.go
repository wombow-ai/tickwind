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
	// Merge the curated foreign (TW/HK) seed names so the markets we price are
	// searchable alongside the US SEC directory.
	idx := symbols.Build(append(syms, symbols.ForeignSeeds()...))
	s.cache.Set(idx)
	s.log.Info("symbols: loaded directory", "count", idx.Len())
}
