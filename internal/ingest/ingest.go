// Package ingest periodically pulls data from sources into the store.
// Filings (EDGAR), news (Finnhub) and social (StockTwits, Reddit, …) refresh on
// the scheduler; prices have their own faster poller (price.go). The set of
// tickers comes from a TickerSource (default set ∪ all users' watchlists), read
// each cycle so watchlist edits take effect without a restart.
package ingest

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/finnhub"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/topics"
)

// TickerSource returns the tickers to ingest this cycle.
type TickerSource func(context.Context) []string

// SocialSource is one provider of social posts for a ticker (e.g. StockTwits,
// Reddit). New sources implement this and are passed to NewScheduler.
type SocialSource interface {
	Name() string
	Posts(ctx context.Context, ticker string, limit int) ([]store.Post, error)
}

// SignalSource produces per-ticker numeric signals (buzz / sentiment) in bulk:
// one call may cover many tickers at once, unlike the per-ticker SocialSource.
// Providers like ApeWisdom (mention momentum) and Alpha Vantage (news
// sentiment) implement this; it returns only the tickers it has data for.
type SignalSource interface {
	Name() string
	Signals(ctx context.Context, tickers []string) ([]store.Signal, error)
}

// HotSource produces a market-wide ranked leaderboard of the most-discussed
// stocks (e.g. ApeWisdom), independent of the watched-ticker set. nil disables
// the trending "hot list".
type HotSource interface {
	Name() string
	Leaderboard(ctx context.Context, limit int) ([]store.HotStock, error)
}

type Scheduler struct {
	store      store.Store
	edgar      *edgar.Client
	finnhub    *finnhub.Client // optional; nil disables news ingestion
	social     []SocialSource
	signals    []SignalSource
	hot        HotSource     // optional; nil disables the trending hot list
	topicCache *topics.Cache // optional; nil disables the topics strip
	tickers    TickerSource
	every      time.Duration
	log        *slog.Logger
}

// NewScheduler builds the filings+news+social+signals+hotlist+topics scheduler.
// fh, hot and topicCache may be nil to disable those; social/signals may be empty.
func NewScheduler(st store.Store, ec *edgar.Client, fh *finnhub.Client, social []SocialSource, signals []SignalSource, hot HotSource, topicCache *topics.Cache, tickers TickerSource, every time.Duration, log *slog.Logger) *Scheduler {
	return &Scheduler{store: st, edgar: ec, finnhub: fh, social: social, signals: signals, hot: hot, topicCache: topicCache, tickers: tickers, every: every, log: log}
}

// Run blocks until ctx is cancelled, refreshing every `every`.
func (s *Scheduler) Run(ctx context.Context) {
	s.runOnce(ctx)
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	tickers := s.tickers(ctx)
	for _, ticker := range tickers {
		s.ingestFilings(ctx, ticker)
		s.ingestNews(ctx, ticker)
		s.ingestSocial(ctx, ticker)
		// Stay well under provider rate limits.
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
	// Signal sources are bulk (one call covers many tickers), so run them once
	// per cycle after the per-ticker passes.
	s.ingestSignals(ctx, tickers)
	// The trending leaderboard is market-wide (not tied to the watchlist).
	s.ingestHotList(ctx)
	// Trending topics are derived from the news just ingested across all tickers.
	s.ingestTopics(ctx, tickers)
}

// IngestOne runs a one-shot filings+news+social pull for a single ticker, used
// to populate a freshly watch-listed stock immediately rather than waiting for
// the next scheduler cycle. Safe to call concurrently with Run — it only
// touches the concurrency-safe store, not scheduler state.
func (s *Scheduler) IngestOne(ctx context.Context, ticker string) {
	s.ingestFilings(ctx, ticker)
	s.ingestNews(ctx, ticker)
	s.ingestSocial(ctx, ticker)
	s.log.Info("ingested on-demand", "ticker", ticker)
}

// ingestTopics recomputes the trending-topics snapshot from the recent news
// across all tickers (already in the store), keyed by article id so a story
// tagged to several tickers is counted once.
func (s *Scheduler) ingestTopics(ctx context.Context, tickers []string) {
	if s.topicCache == nil {
		return
	}
	seen := make(map[string]int) // news id -> index in arts
	var arts []topics.Article
	for _, tk := range tickers {
		items, err := s.store.ListNews(ctx, tk, 40)
		if err != nil {
			continue
		}
		for _, n := range items {
			if idx, ok := seen[n.ID]; ok {
				arts[idx].Tickers = append(arts[idx].Tickers, n.Ticker)
				continue
			}
			seen[n.ID] = len(arts)
			arts = append(arts, topics.Article{
				Headline:    n.Headline,
				Summary:     n.Summary,
				Tickers:     []string{n.Ticker},
				PublishedAt: n.Published,
			})
		}
	}
	snap := topics.Recompute(time.Now().UTC(), arts)
	s.topicCache.Set(snap)
	s.log.Info("recomputed topics", "count", len(snap.Topics))
}

func (s *Scheduler) ingestFilings(ctx context.Context, ticker string) {
	sec, filings, err := s.edgar.RecentFilings(ctx, ticker, 25)
	if err != nil {
		s.log.Warn("edgar fetch failed", "ticker", ticker, "err", err)
		return
	}
	_ = s.store.UpsertSecurity(ctx, sec)
	_ = s.store.SaveFilings(ctx, ticker, filings)
	s.log.Info("ingested filings", "ticker", ticker, "name", sec.Name, "count", len(filings))
}

// newsLookbackDays is how far back company-news is fetched — 30 (vs a tight 7)
// so a freshly-added or thinly-covered ticker still surfaces recent articles.
const newsLookbackDays = 30

func (s *Scheduler) ingestNews(ctx context.Context, ticker string) {
	if s.finnhub == nil {
		return
	}
	items, err := s.finnhub.CompanyNews(ctx, ticker, newsLookbackDays)
	if err != nil {
		s.log.Warn("finnhub fetch failed", "ticker", ticker, "err", err)
		return
	}
	if err := s.store.SaveNews(ctx, ticker, items); err != nil {
		s.log.Warn("save news failed", "ticker", ticker, "err", err)
		return
	}
	s.log.Info("ingested news", "ticker", ticker, "count", len(items))
}

func (s *Scheduler) ingestSocial(ctx context.Context, ticker string) {
	for _, src := range s.social {
		posts, err := src.Posts(ctx, ticker, 30)
		if err != nil {
			s.log.Warn("social fetch failed", "source", src.Name(), "ticker", ticker, "err", err)
			continue
		}
		if err := s.store.SaveSocial(ctx, ticker, posts); err != nil {
			s.log.Warn("save social failed", "source", src.Name(), "ticker", ticker, "err", err)
			continue
		}
		s.log.Info("ingested social", "source", src.Name(), "ticker", ticker, "count", len(posts))
	}
}

func (s *Scheduler) ingestSignals(ctx context.Context, tickers []string) {
	if len(tickers) == 0 {
		return
	}
	for _, src := range s.signals {
		sigs, err := src.Signals(ctx, tickers)
		if err != nil {
			s.log.Warn("signals fetch failed", "source", src.Name(), "err", err)
			continue
		}
		if err := s.store.SaveSignals(ctx, sigs); err != nil {
			s.log.Warn("save signals failed", "source", src.Name(), "err", err)
			continue
		}
		s.log.Info("ingested signals", "source", src.Name(), "count", len(sigs))
	}
}

// hotListSize caps how many leaderboard rows we fetch + rank per board.
const hotListSize = 40

// Scoring constants. shrinkC is the Bayesian-shrinkage pseudo-count: a stock
// with shrinkC mentions has its momentum term halved, dampening low-base
// blow-ups (e.g. 2→6 mentions = +200%) while leaving high-volume names ~intact.
// surgingMinMentions floors the surging board so micro-chatter can't surge in.
const (
	shrinkC            = 50
	surgingMinMentions = 25
)

func (s *Scheduler) ingestHotList(ctx context.Context) {
	if s.hot == nil {
		return
	}
	raw, err := s.hot.Leaderboard(ctx, hotListSize)
	if err != nil {
		s.log.Warn("hotlist fetch failed", "source", s.hot.Name(), "err", err)
		return
	}
	for board, stocks := range buildBoards(raw) {
		if err := s.store.SaveHotList(ctx, board, stocks); err != nil {
			s.log.Warn("save hotlist failed", "board", board, "err", err)
			continue
		}
		s.log.Info("ingested hotlist", "board", board, "source", s.hot.Name(), "count", len(stocks))
	}
}

// buildBoards derives the leaderboards from raw ApeWisdom entries:
//   - "hot": most discussed — volume × momentum (heatScore).
//   - "surging": biggest attention risers — momentum-led (surgeScore), gated by
//     a minimum mention floor.
//
// Both share a Bayesian-shrunk momentum term so a tiny-base spike can't dominate
// (the distinction trackers like StockTwits draw between "most active" and
// "trending"). Each entry's Change is set once; rankBoard then tags Board, Score
// and Rank per board.
func buildBoards(raw []store.HotStock) map[string][]store.HotStock {
	now := time.Now().UTC()
	for i := range raw {
		m, prev := raw[i].Mentions, raw[i].MentionsPrev
		if prev > 0 {
			raw[i].Change = float64(m-prev) / float64(prev)
		}
		raw[i].UpdatedAt = now
	}
	return map[string][]store.HotStock{
		"hot": rankBoard(raw, "hot", 0, func(h store.HotStock) float64 {
			return heatScore(h.Mentions, h.MentionsPrev)
		}),
		"surging": rankBoard(raw, "surging", surgingMinMentions, func(h store.HotStock) float64 {
			return surgeScore(h.Mentions, h.MentionsPrev)
		}),
	}
}

// rankBoard scores a copy of raw with score(), drops entries below minMentions,
// sorts highest-first and assigns Board + Rank (1..N).
func rankBoard(raw []store.HotStock, board string, minMentions int, score func(store.HotStock) float64) []store.HotStock {
	out := make([]store.HotStock, 0, len(raw))
	for _, h := range raw {
		if h.Mentions < minMentions {
			continue
		}
		h.Board = board
		h.Score = score(h)
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Mentions > out[j].Mentions
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// heatScore = mentions × (1 + shrink·clamp(growth,2)) — VOLUME-led with a
// momentum boost. The boost is Bayesian-shrunk by volume so a low-base spike
// can't inflate it; a flat/cooling name scores at its raw volume (never
// penalised — it is still being discussed).
func heatScore(mentions, mentionsPrev int) float64 {
	return float64(mentions) * (1 + shrink(mentions)*clamp(growth(mentions, mentionsPrev), 3))
}

// surgeScore = shrink·clamp(growth,3) — MOMENTUM-led: ranks by 24h mention
// growth, Bayesian-shrunk by volume so thin names don't dominate (used with a
// minimum mention floor). Independent of absolute volume, so mid-caps catching
// fire surface above perennially-loud mega-caps.
func surgeScore(mentions, mentionsPrev int) float64 {
	return shrink(mentions) * clamp(growth(mentions, mentionsPrev), 3)
}

// growth is the 24h mention growth as a fraction, floored at 0 (cooling names
// contribute no momentum rather than going negative).
func growth(mentions, mentionsPrev int) float64 {
	if mentionsPrev <= 0 {
		return 0
	}
	g := float64(mentions-mentionsPrev) / float64(mentionsPrev)
	if g < 0 {
		return 0
	}
	return g
}

// shrink is the Bayesian shrinkage weight mentions/(mentions+shrinkC) ∈ [0,1).
func shrink(mentions int) float64 {
	return float64(mentions) / float64(mentions+shrinkC)
}

// clamp caps v at hi.
func clamp(v, hi float64) float64 {
	if v > hi {
		return hi
	}
	return v
}
