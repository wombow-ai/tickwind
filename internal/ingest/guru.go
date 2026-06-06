package ingest

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/substack"
)

// guruSource is the social-post source tag for guru/newsletter posts.
const guruSource = "substack"

// GuruFeedClient fetches one KOL RSS feed's recent posts (substack.Client).
type GuruFeedClient interface {
	Posts(ctx context.Context, feedURL string) ([]substack.Post, error)
}

// GuruSocialStore persists guru posts as per-ticker social posts so they also
// surface on each mentioned stock's Discussion tab. A nil store disables that
// (rail-only); store.Store satisfies it.
type GuruSocialStore interface {
	SaveSocial(ctx context.Context, ticker string, posts []store.Post) error
}

// GuruIngestor populates the "Guru-watch" rail: it periodically fetches the
// curated KOL feeds, keeps the posts that mention tickers and publishes a
// ranked, newest-first rail into a shared Cache. It also fans each post out to
// every ticker it names as a social post, so a guru's view shows up on those
// stocks' Discussion tabs (融入信号). Newsletter cadence (slow), and it needs no
// API key (public RSS), so it runs in its own goroutine independent of the
// price-gated scheduler.
type GuruIngestor struct {
	client GuruFeedClient
	feeds  []substack.Feed
	cache  *guru.Cache
	store  GuruSocialStore // optional: also surface posts on per-stock Discussion
	max    int
	every  time.Duration
	log    *slog.Logger
}

// NewGuruIngestor builds the ingestor. every is the refresh cadence; max caps
// the rail size; st (may be nil) also persists posts to per-stock Discussion.
func NewGuruIngestor(client GuruFeedClient, feeds []substack.Feed, cache *guru.Cache, st GuruSocialStore, max int, every time.Duration, log *slog.Logger) *GuruIngestor {
	return &GuruIngestor{client: client, feeds: feeds, cache: cache, store: st, max: max, every: every, log: log}
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

// refresh fetches every curated feed, rebuilds the rail, and fans posts out to
// per-stock Discussion. A feed that fails (network/parse) is skipped, so one
// dead newsletter never empties the rail.
func (g *GuruIngestor) refresh(ctx context.Context) {
	var items []guru.Item
	byTicker := map[string][]store.Post{} // ticker -> guru posts to add to Discussion
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
			if g.store != nil {
				id := postID(p.URL)
				for _, tk := range p.Tickers {
					byTicker[tk] = append(byTicker[tk], store.Post{
						Ticker:    tk,
						ID:        id,
						Source:    guruSource,
						Author:    f.Name,
						Body:      p.Title,
						URL:       p.URL,
						CreatedAt: p.Published,
					})
				}
			}
		}
	}

	rail := guru.Rank(items, g.max)
	g.cache.Set(rail)

	saved := 0
	if g.store != nil {
		for tk, posts := range byTicker {
			if ctx.Err() != nil {
				break
			}
			if err := g.store.SaveSocial(ctx, tk, posts); err != nil {
				g.log.Debug("guru: save discussion failed", "ticker", tk, "err", err)
				continue
			}
			saved += len(posts)
		}
	}
	g.log.Info("guru: refreshed rail", "feeds_ok", ok, "feeds", len(g.feeds), "items", len(rail), "discussion_posts", saved)
}

// postID is a stable dedupe id for a guru post (same across the tickers it
// mentions; the store merges by id within each ticker, so re-ingest is a no-op).
func postID(url string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(url))
	return fmt.Sprintf("%s:%x", guruSource, h.Sum64())
}
