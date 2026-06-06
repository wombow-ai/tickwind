package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/substack"
)

// GuruFeedClient fetches one KOL RSS feed's recent posts (substack.Client).
type GuruFeedClient interface {
	Posts(ctx context.Context, feedURL string) ([]substack.Post, error)
}

// GuruIngestor populates the "Guru-watch" rail: it periodically fetches the
// curated KOL feeds, keeps the posts that mention tickers and publishes a
// ranked, newest-first rail into a shared Cache. Newsletter cadence (slow), and
// it needs no API key (public RSS), so it runs in its own goroutine independent
// of the price-gated scheduler.
type GuruIngestor struct {
	client GuruFeedClient
	feeds  []substack.Feed
	cache  *guru.Cache
	max    int
	every  time.Duration
	log    *slog.Logger
}

// NewGuruIngestor builds the ingestor. every is the refresh cadence; max caps
// the rail size.
func NewGuruIngestor(client GuruFeedClient, feeds []substack.Feed, cache *guru.Cache, max int, every time.Duration, log *slog.Logger) *GuruIngestor {
	return &GuruIngestor{client: client, feeds: feeds, cache: cache, max: max, every: every, log: log}
}

// Run refreshes the rail immediately, then on every tick, until ctx is cancelled.
func (g *GuruIngestor) Run(ctx context.Context) {
	g.refresh(ctx)
	t := time.NewTicker(g.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.refresh(ctx)
		}
	}
}

// refresh fetches every curated feed and rebuilds the rail. A feed that fails
// (network/parse) is skipped, so one dead newsletter never empties the rail.
func (g *GuruIngestor) refresh(ctx context.Context) {
	var items []guru.Item
	ok := 0
	for _, f := range g.feeds {
		if ctx.Err() != nil {
			return
		}
		posts, err := g.client.Posts(ctx, f.URL)
		if err != nil {
			g.log.Debug("guru: feed fetch failed", "feed", f.Name, "err", err)
			continue
		}
		ok++
		for _, p := range posts {
			if len(p.Tickers) == 0 {
				continue // only stock-anchored posts belong in the rail
			}
			items = append(items, guru.Item{
				Author:    f.Name, // curated publication name is our canonical attribution
				Title:     p.Title,
				URL:       p.URL,
				Teaser:    p.Teaser,
				Published: p.Published,
				Tickers:   p.Tickers,
			})
		}
	}
	rail := guru.Rank(items, g.max)
	g.cache.Set(rail)
	g.log.Info("guru: refreshed rail", "feeds_ok", ok, "feeds", len(g.feeds), "items", len(rail))
}
