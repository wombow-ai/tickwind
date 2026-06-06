package store

import "context"

// Split presents a single Store while routing each operation to one of two
// backends:
//
//   - Market: the collected/scraped corpus — securities, filings, quotes, news,
//     social. This is expensive (or impossible) to re-collect, so it belongs on
//     durable, backed-up storage.
//   - User: per-user state — watchlist and clips. Cheap to reconstruct (users
//     just re-add tickers), so it can live on local/ephemeral storage.
//
// The rest of the app depends only on Store and is unaware of the split.
type Split struct {
	Market Store
	User   Store
}

// Compile-time assurance that Split satisfies Store.
var _ Store = Split{}

// ── Market: collected data (durable) ─────────────────────────────────────

func (s Split) UpsertSecurity(ctx context.Context, sec Security) error {
	return s.Market.UpsertSecurity(ctx, sec)
}

func (s Split) GetSecurity(ctx context.Context, ticker string) (Security, bool, error) {
	return s.Market.GetSecurity(ctx, ticker)
}

func (s Split) SaveFilings(ctx context.Context, ticker string, filings []Filing) error {
	return s.Market.SaveFilings(ctx, ticker, filings)
}

func (s Split) ListFilings(ctx context.Context, ticker string, limit int) ([]Filing, error) {
	return s.Market.ListFilings(ctx, ticker, limit)
}

func (s Split) UpsertQuote(ctx context.Context, q Quote) error {
	return s.Market.UpsertQuote(ctx, q)
}

func (s Split) GetQuote(ctx context.Context, ticker string) (Quote, bool, error) {
	return s.Market.GetQuote(ctx, ticker)
}

func (s Split) SaveNews(ctx context.Context, ticker string, items []News) error {
	return s.Market.SaveNews(ctx, ticker, items)
}

func (s Split) ListNews(ctx context.Context, ticker string, limit int) ([]News, error) {
	return s.Market.ListNews(ctx, ticker, limit)
}

func (s Split) SaveSocial(ctx context.Context, ticker string, posts []Post) error {
	return s.Market.SaveSocial(ctx, ticker, posts)
}

func (s Split) ListSocial(ctx context.Context, ticker string, limit int) ([]Post, error) {
	return s.Market.ListSocial(ctx, ticker, limit)
}

// ── User: per-user state (local/ephemeral) ───────────────────────────────

func (s Split) Watchlist(ctx context.Context, userID string) ([]string, error) {
	return s.User.Watchlist(ctx, userID)
}

func (s Split) AddToWatchlist(ctx context.Context, userID, ticker string) error {
	return s.User.AddToWatchlist(ctx, userID, ticker)
}

func (s Split) RemoveFromWatchlist(ctx context.Context, userID, ticker string) error {
	return s.User.RemoveFromWatchlist(ctx, userID, ticker)
}

func (s Split) AllWatchlistTickers(ctx context.Context) ([]string, error) {
	return s.User.AllWatchlistTickers(ctx)
}

func (s Split) SaveClip(ctx context.Context, c Clip) error {
	return s.User.SaveClip(ctx, c)
}

func (s Split) ListClips(ctx context.Context, userID, ticker string, limit int) ([]Clip, error) {
	return s.User.ListClips(ctx, userID, ticker, limit)
}
