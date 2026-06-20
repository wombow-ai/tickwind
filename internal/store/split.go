package store

import (
	"context"
	"encoding/json"
	"time"
)

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

// Ping checks both backends — a readiness probe should fail if either the
// durable Market store or the per-user store is unreachable.
func (s Split) Ping(ctx context.Context) error {
	if err := s.Market.Ping(ctx); err != nil {
		return err
	}
	return s.User.Ping(ctx)
}

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

func (s Split) ListUntranslatedNews(ctx context.Context, limit int) ([]News, error) {
	return s.Market.ListUntranslatedNews(ctx, limit)
}

func (s Split) SetNewsTranslation(ctx context.Context, ticker, id, headlineZH string) error {
	return s.Market.SetNewsTranslation(ctx, ticker, id, headlineZH)
}

func (s Split) SaveSocial(ctx context.Context, ticker string, posts []Post) error {
	return s.Market.SaveSocial(ctx, ticker, posts)
}

func (s Split) ListSocial(ctx context.Context, ticker string, limit int) ([]Post, error) {
	return s.Market.ListSocial(ctx, ticker, limit)
}

func (s Split) SaveSignals(ctx context.Context, signals []Signal) error {
	return s.Market.SaveSignals(ctx, signals)
}

func (s Split) ListSignals(ctx context.Context, ticker string) ([]Signal, error) {
	return s.Market.ListSignals(ctx, ticker)
}

func (s Split) SaveHotList(ctx context.Context, board string, stocks []HotStock) error {
	return s.Market.SaveHotList(ctx, board, stocks)
}

func (s Split) HotList(ctx context.Context, board string, limit int) ([]HotStock, error) {
	return s.Market.HotList(ctx, board, limit)
}

func (s Split) SaveInsiderBuys(ctx context.Context, buys []InsiderBuy) error {
	return s.Market.SaveInsiderBuys(ctx, buys)
}

func (s Split) RecentInsiderBuys(ctx context.Context, since time.Time) ([]InsiderBuy, error) {
	return s.Market.RecentInsiderBuys(ctx, since)
}

func (s Split) SaveEarnings(ctx context.Context, es []Earning) error {
	return s.Market.SaveEarnings(ctx, es)
}
func (s Split) ListEarnings(ctx context.Context, from, to time.Time) ([]Earning, error) {
	return s.Market.ListEarnings(ctx, from, to)
}
func (s Split) ListEarningsForTicker(ctx context.Context, ticker string, limit int) ([]Earning, error) {
	return s.Market.ListEarningsForTicker(ctx, ticker, limit)
}

func (s Split) MarkForm4Seen(ctx context.Context, accessions []string, filedDate time.Time) error {
	return s.Market.MarkForm4Seen(ctx, accessions, filedDate)
}

func (s Split) SeenForm4Since(ctx context.Context, since time.Time) ([]string, error) {
	return s.Market.SeenForm4Since(ctx, since)
}

// Fear & Greed history is public market data → the durable Market store.

func (s Split) SaveFearGreed(ctx context.Context, date string, score int) error {
	return s.Market.SaveFearGreed(ctx, date, score)
}

func (s Split) FearGreedHistory(ctx context.Context, limit int) ([]FearGreedPoint, error) {
	return s.Market.FearGreedHistory(ctx, limit)
}

// AI digests are public market data + costly to regenerate → durable Market store.

func (s Split) SaveAISummary(ctx context.Context, ticker, day, lang string, payload []byte) error {
	return s.Market.SaveAISummary(ctx, ticker, day, lang, payload)
}

func (s Split) SaveDeepReport(ctx context.Context, ticker, lang string, payload []byte) error {
	return s.Market.SaveDeepReport(ctx, ticker, lang, payload)
}
func (s Split) GetDeepReport(ctx context.Context, ticker, lang string) ([]byte, time.Time, bool, error) {
	return s.Market.GetDeepReport(ctx, ticker, lang)
}
func (s Split) GetAISummary(ctx context.Context, ticker, day, lang string) ([]byte, bool, error) {
	return s.Market.GetAISummary(ctx, ticker, day, lang)
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

func (s Split) SaveNote(ctx context.Context, n Note) error { return s.User.SaveNote(ctx, n) }

func (s Split) ListNotes(ctx context.Context, f NoteFilter) ([]Note, error) {
	return s.User.ListNotes(ctx, f)
}

func (s Split) UpdateNote(ctx context.Context, userID, id string, body *string, pinned *bool) (Note, bool, error) {
	return s.User.UpdateNote(ctx, userID, id, body, pinned)
}

func (s Split) DeleteNote(ctx context.Context, userID, id string) (bool, error) {
	return s.User.DeleteNote(ctx, userID, id)
}

func (s Split) SaveAlert(ctx context.Context, a Alert) error { return s.User.SaveAlert(ctx, a) }
func (s Split) ListAlerts(ctx context.Context, userID string) ([]Alert, error) {
	return s.User.ListAlerts(ctx, userID)
}
func (s Split) DeleteAlert(ctx context.Context, userID, id string) (bool, error) {
	return s.User.DeleteAlert(ctx, userID, id)
}
func (s Split) ReactivateAlert(ctx context.Context, userID, id string) (bool, error) {
	return s.User.ReactivateAlert(ctx, userID, id)
}
func (s Split) ListActiveAlerts(ctx context.Context) ([]Alert, error) {
	return s.User.ListActiveAlerts(ctx)
}
func (s Split) MarkAlertTriggered(ctx context.Context, id string, at time.Time) error {
	return s.User.MarkAlertTriggered(ctx, id, at)
}

func (s Split) SaveHolding(ctx context.Context, h Holding) error { return s.User.SaveHolding(ctx, h) }
func (s Split) ListHoldings(ctx context.Context, userID string) ([]Holding, error) {
	return s.User.ListHoldings(ctx, userID)
}
func (s Split) DeleteHolding(ctx context.Context, userID, id string) (bool, error) {
	return s.User.DeleteHolding(ctx, userID, id)
}

// Prefs are per-user UI state → the cheap-to-rebuild User store.

func (s Split) GetPrefs(ctx context.Context, userID string) (json.RawMessage, bool, error) {
	return s.User.GetPrefs(ctx, userID)
}
func (s Split) PutPrefs(ctx context.Context, userID string, blob json.RawMessage) error {
	return s.User.PutPrefs(ctx, userID, blob)
}

// Deep-research per-user monthly generation quota → the cheap-to-rebuild User store
// (per-user state, same class as prefs/holdings; losing it just resets the quota).

func (s Split) GetDeepQuotaUsed(ctx context.Context, userID, period string) (int, error) {
	return s.User.GetDeepQuotaUsed(ctx, userID, period)
}
func (s Split) IncrDeepQuotaUsed(ctx context.Context, userID, period string) error {
	return s.User.IncrDeepQuotaUsed(ctx, userID, period)
}

// Product B chat history + the per-user monthly message meter → the cheap-to-rebuild
// User store (same class as the deep quota; losing it only drops chat history).

func (s Split) AppendChatMessage(ctx context.Context, m ChatMessage) error {
	return s.User.AppendChatMessage(ctx, m)
}
func (s Split) ListChatMessages(ctx context.Context, conversationID string, limit int) ([]ChatMessage, error) {
	return s.User.ListChatMessages(ctx, conversationID, limit)
}
func (s Split) ClearChatMessages(ctx context.Context, conversationID string) error {
	return s.User.ClearChatMessages(ctx, conversationID)
}
func (s Split) GetOrCreateStockConversation(ctx context.Context, userID, ticker string) (string, error) {
	return s.User.GetOrCreateStockConversation(ctx, userID, ticker)
}
func (s Split) CreateConversation(ctx context.Context, userID, title, anchorTicker string) (string, error) {
	return s.User.CreateConversation(ctx, userID, title, anchorTicker)
}
func (s Split) ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	return s.User.ListConversations(ctx, userID)
}
func (s Split) GetConversation(ctx context.Context, userID, id string) (Conversation, bool, error) {
	return s.User.GetConversation(ctx, userID, id)
}
func (s Split) RenameConversation(ctx context.Context, userID, id, title string) error {
	return s.User.RenameConversation(ctx, userID, id, title)
}
func (s Split) DeleteConversation(ctx context.Context, userID, id string) error {
	return s.User.DeleteConversation(ctx, userID, id)
}
func (s Split) GetChatMsgUsed(ctx context.Context, userID, period string) (int, error) {
	return s.User.GetChatMsgUsed(ctx, userID, period)
}
func (s Split) IncrChatMsgUsed(ctx context.Context, userID, period string) error {
	return s.User.IncrChatMsgUsed(ctx, userID, period)
}

// Subscriptions / Stripe billing → the DURABLE Market store (billing is not cheap
// to rebuild, unlike the per-user quota above).

func (s Split) GetSubscription(ctx context.Context, userID string) (Subscription, bool, error) {
	return s.Market.GetSubscription(ctx, userID)
}
func (s Split) GetSubscriptionByCustomer(ctx context.Context, customerID string) (Subscription, bool, error) {
	return s.Market.GetSubscriptionByCustomer(ctx, customerID)
}
func (s Split) UpsertSubscription(ctx context.Context, sub Subscription) error {
	return s.Market.UpsertSubscription(ctx, sub)
}
func (s Split) MarkStripeEventSeen(ctx context.Context, eventID, eventType string) (bool, error) {
	return s.Market.MarkStripeEventSeen(ctx, eventID, eventType)
}
func (s Split) StripeEventSeen(ctx context.Context, eventID string) (bool, error) {
	return s.Market.StripeEventSeen(ctx, eventID)
}

// Comments are public, valuable community content → the durable Market store.

func (s Split) SaveComment(ctx context.Context, c Comment) error {
	return s.Market.SaveComment(ctx, c)
}

func (s Split) ListComments(ctx context.Context, ticker string, limit int, viewerID string) ([]Comment, error) {
	return s.Market.ListComments(ctx, ticker, limit, viewerID)
}

func (s Split) DeleteComment(ctx context.Context, id, userID string, admin bool) (bool, error) {
	return s.Market.DeleteComment(ctx, id, userID, admin)
}

func (s Split) ReportComment(ctx context.Context, id string) (bool, error) {
	return s.Market.ReportComment(ctx, id)
}

func (s Split) UpdateComment(ctx context.Context, id, userID, body string, mentions []string) (Comment, bool, error) {
	return s.Market.UpdateComment(ctx, id, userID, body, mentions)
}

func (s Split) LikeComment(ctx context.Context, id, userID string) (bool, int, bool, error) {
	return s.Market.LikeComment(ctx, id, userID)
}
