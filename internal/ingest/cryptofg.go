package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/cryptofg"
)

// CryptoFGFetcher fetches the latest crypto Fear & Greed snapshot (the F&G score
// plus best-effort BTC/ETH prices). Implemented by *cryptofg.Client. Keyless.
type CryptoFGFetcher interface {
	Latest(ctx context.Context) (cryptofg.Index, error)
}

// CryptoFGIngestor refreshes the crypto Fear & Greed index (alternative.me) plus
// best-effort BTC/ETH prices (CoinGecko) into a *cryptofg.Cache on its own
// goroutine. The F&G index updates ~once a day but the fetch is cheap and BTC/ETH
// prices move intraday, so an ~hourly cadence is used; the boot refresh warms the
// cache immediately. A failed fetch keeps the last good snapshot (the cache is
// only Set on success) and logs a WARN, so a transient outage never blanks the
// crypto strip. Keyless — no API key, no secret.
//
// This backs GET /v1/crypto (the crypto market-mood strip). All data is
// server-driven from this cache; the request path never fetches alternative.me
// or CoinGecko.
type CryptoFGIngestor struct {
	src   CryptoFGFetcher
	cache *cryptofg.Cache
	every time.Duration
	log   *slog.Logger
}

// NewCryptoFGIngestor builds the ingestor over a fetcher and cache; call Run to
// start refreshing every `every`.
func NewCryptoFGIngestor(src CryptoFGFetcher, cache *cryptofg.Cache, every time.Duration, log *slog.Logger) *CryptoFGIngestor {
	if log == nil {
		log = slog.Default()
	}
	return &CryptoFGIngestor{src: src, cache: cache, every: every, log: log}
}

// Cache exposes the underlying snapshot store for the API handler.
func (i *CryptoFGIngestor) Cache() *cryptofg.Cache { return i.cache }

// Run refreshes immediately (a startup warm) and then on every tick until ctx is
// cancelled.
func (i *CryptoFGIngestor) Run(ctx context.Context) {
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

// refresh fetches the latest crypto F&G snapshot and installs it on success. On
// error the previous snapshot is kept (stale beats empty for once-a-day data) and
// a WARN is logged. Note: a missing-prices result is NOT an error — the client
// returns the F&G score with prices absent, and that snapshot is installed.
func (i *CryptoFGIngestor) refresh(ctx context.Context) {
	idx, err := i.src.Latest(ctx)
	if err != nil {
		i.log.Warn("crypto fear & greed refresh failed", "err", err)
		return
	}
	i.cache.Set(idx)
	i.log.Info("crypto fear & greed refreshed",
		"score", idx.Score, "label", idx.Label, "as_of", idx.AsOf,
		"btc", idx.BTC.Present, "eth", idx.ETH.Present)
}
