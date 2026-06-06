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
	store   store.Store
	edgar   *edgar.Client
	finnhub *finnhub.Client // optional; nil disables news ingestion
	social  []SocialSource
	signals []SignalSource
	hot     HotSource // optional; nil disables the trending hot list
	tickers TickerSource
	every   time.Duration
	log     *slog.Logger
}

// NewScheduler builds the filings+news+social+signals+hotlist scheduler. fh and
// hot may be nil to disable news / the hot list; social/signals may be empty.
func NewScheduler(st store.Store, ec *edgar.Client, fh *finnhub.Client, social []SocialSource, signals []SignalSource, hot HotSource, tickers TickerSource, every time.Duration, log *slog.Logger) *Scheduler {
	return &Scheduler{store: st, edgar: ec, finnhub: fh, social: social, signals: signals, hot: hot, tickers: tickers, every: every, log: log}
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

func (s *Scheduler) ingestNews(ctx context.Context, ticker string) {
	if s.finnhub == nil {
		return
	}
	items, err := s.finnhub.CompanyNews(ctx, ticker, 7)
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

// hotListSize caps how many stocks the trending leaderboard holds.
const hotListSize = 40

func (s *Scheduler) ingestHotList(ctx context.Context) {
	if s.hot == nil {
		return
	}
	stocks, err := s.hot.Leaderboard(ctx, hotListSize)
	if err != nil {
		s.log.Warn("hotlist fetch failed", "source", s.hot.Name(), "err", err)
		return
	}
	rankHotList(stocks)
	if err := s.store.SaveHotList(ctx, stocks); err != nil {
		s.log.Warn("save hotlist failed", "err", err)
		return
	}
	s.log.Info("ingested hotlist", "source", s.hot.Name(), "count", len(stocks))
}

// rankHotList fills each stock's Change + Heat from its mention counts, sorts the
// slice hottest-first, and assigns Rank (1..N).
//
// Heat blends discussion VOLUME with MOMENTUM — a stock is "hot" when it is both
// heavily mentioned and gaining attention (the distinction trackers like
// StockTwits draw between "most active" and "trending"). See heatScore.
func rankHotList(stocks []store.HotStock) {
	now := time.Now().UTC()
	for i := range stocks {
		m, prev := stocks[i].Mentions, stocks[i].MentionsPrev
		if prev > 0 {
			stocks[i].Change = float64(m-prev) / float64(prev)
		}
		stocks[i].Heat = heatScore(m, prev)
		stocks[i].UpdatedAt = now
	}
	sort.SliceStable(stocks, func(i, j int) bool {
		if stocks[i].Heat != stocks[j].Heat {
			return stocks[i].Heat > stocks[j].Heat
		}
		return stocks[i].Mentions > stocks[j].Mentions
	})
	for i := range stocks {
		stocks[i].Rank = i + 1
	}
}

// heatScore = mentions × (1 + rise), where rise is the 24h mention growth
// clamped to [0, 2]. So a flat or cooling name scores at its raw volume (never
// penalised below it — it is still being discussed), while an accelerating one
// is boosted up to 3× (a stock tripling its mentions tops a steadier name with
// the same volume). This rewards stocks that are both loud and getting louder.
func heatScore(mentions, mentionsPrev int) float64 {
	rise := 0.0
	if mentionsPrev > 0 {
		rise = float64(mentions-mentionsPrev) / float64(mentionsPrev)
	}
	if rise < 0 {
		rise = 0
	}
	if rise > 2 {
		rise = 2
	}
	return float64(mentions) * (1 + rise)
}
