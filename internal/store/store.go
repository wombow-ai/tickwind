// Package store defines the domain types and the storage interface.
// v1 ships an in-memory implementation; a Postgres (TimescaleDB + pgvector)
// implementation is added when we deploy to the server.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// NewID returns a random 128-bit hex id (for conversations etc.). Falls back to a
// time-based id if the system RNG is unavailable (never in practice).
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "c" + hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000")))
	}
	return hex.EncodeToString(b[:])
}

// Security is a tracked instrument.
type Security struct {
	Ticker string `json:"ticker"`
	CIK    string `json:"cik,omitempty"`
	Name   string `json:"name"`
	Market string `json:"market"` // US | HK | KR
}

// Filing is a regulatory disclosure (e.g. 8-K, 10-Q, Form 4).
type Filing struct {
	Ticker      string    `json:"ticker"`
	Form        string    `json:"form"`
	Title       string    `json:"title"`
	FiledAt     time.Time `json:"filed_at"`
	AccessionNo string    `json:"accession_no"`
	URL         string    `json:"url"`
}

// Quote is the latest traded price for a security. It covers all trading
// sessions — pre-market, regular, after-hours and overnight.
type Quote struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"` // latest trade, all-session (incl. pre/post/overnight)
	// PrevClose is the previous trading day's closing price, used to compute
	// the day's change. Zero when unknown (omitted from JSON).
	PrevClose float64 `json:"prev_close,omitempty"`
	// RegularClose is the current/most-recent REGULAR-session close (Alpaca
	// dailyBar close). It equals the live regular price during market hours and
	// the day's close after; in pre/post sessions the frontend shows Price (the
	// extended-hours price) against this as the extended change. Zero → unknown.
	RegularClose float64   `json:"regular_close,omitempty"`
	Session      string    `json:"session"` // pre | regular | post | overnight | closed
	Source       string    `json:"source"`
	At           time.Time `json:"at"`
}

// IndexQuote is a major-market-index level (e.g. the S&P 500 itself, not an
// ETF proxy) for the homepage indices strip. PrevClose is the prior session's
// close, for the day-change %. Zero → unknown. (The backend index-level source
// was removed; /v1/indices returns empty and the frontend strip falls back to
// keyless Alpaca ETF proxies, so this type currently only shapes the empty
// response envelope — kept for when a licensed index feed is added.)
type IndexQuote struct {
	Symbol    string    `json:"symbol"` // index symbol, e.g. ^GSPC
	Name      string    `json:"name,omitempty"`
	Price     float64   `json:"price"`
	PrevClose float64   `json:"prev_close,omitempty"`
	Source    string    `json:"source"`
	At        time.Time `json:"at"`
}

// Candle is one daily OHLC bar (+ volume) for the candlestick chart. Time is the
// bar's date (UTC midnight).
type Candle struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

// News is a company-news article for a security.
type News struct {
	Ticker   string `json:"ticker"`
	ID       string `json:"id"` // source-assigned id, used for dedupe
	Headline string `json:"headline"`
	// HeadlineZH is the AI-translated Simplified-Chinese headline, filled in
	// asynchronously by the translate ingestor (LLM); empty until translated.
	// Headlines are immutable, so a translation is written once and kept.
	HeadlineZH string    `json:"headline_zh,omitempty"`
	Summary    string    `json:"summary"`
	Source     string    `json:"source"`
	URL        string    `json:"url"`
	Published  time.Time `json:"published"`
}

// Post is a social-media message about a security (e.g. from StockTwits).
type Post struct {
	Ticker    string    `json:"ticker"`
	ID        string    `json:"id"` // "<source>:<rawid>", used for dedupe
	Source    string    `json:"source"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// Signal is a per-ticker numeric "pulse" aggregated from a non-post source:
// mention-momentum (e.g. ApeWisdom) or news sentiment (e.g. Alpha Vantage).
// Unlike News/Post it is not a feed of items but a single rolled-up snapshot,
// stored one row per (ticker, source). Each source fills only the facet it
// provides (Kind says which), so zero-valued fields just mean "not applicable".
type Signal struct {
	Ticker string `json:"ticker"`
	Source string `json:"source"` // e.g. "apewisdom" | "alphavantage"
	Kind   string `json:"kind"`   // "buzz" | "sentiment"

	// Buzz facet — mention momentum (e.g. Reddit/WSB via ApeWisdom).
	Mentions     int `json:"mentions,omitempty"`
	MentionsPrev int `json:"mentions_prev,omitempty"` // same window, 24h earlier
	Rank         int `json:"rank,omitempty"`          // 1 = most mentioned (0 = N/A)
	RankPrev     int `json:"rank_prev,omitempty"`
	Upvotes      int `json:"upvotes,omitempty"`

	// Sentiment facet — news sentiment, normalized to [-1, 1].
	Score      float64 `json:"score,omitempty"`
	Label      string  `json:"label,omitempty"`       // e.g. "Somewhat-Bullish"
	SampleSize int     `json:"sample_size,omitempty"` // articles aggregated

	UpdatedAt time.Time `json:"updated_at"`
}

// HotStock is one row of a trending leaderboard — a market-wide ranking of US
// stocks by social attention (mention volume/momentum, via ApeWisdom). Several
// boards share this shape, distinguished by Board ("hot" = most discussed,
// "surging" = biggest attention risers). Unlike Signal it is not tied to a
// watched ticker: each board is a global snapshot, replaced wholesale on refresh.
type HotStock struct {
	Board        string    `json:"board"` // "hot" | "surging" | …
	Ticker       string    `json:"ticker"`
	Name         string    `json:"name"`
	Rank         int       `json:"rank"`          // 1 = top of this board
	RankPrev     int       `json:"rank_prev"`     // source-board rank 24h earlier (0 = new/unknown); transient input for rank-climb
	Mentions     int       `json:"mentions"`      // discussion volume in the window
	MentionsPrev int       `json:"mentions_prev"` // same window, 24h earlier
	Change       float64   `json:"change"`        // momentum vs 24h ago: mention growth, or board rank-climb (WSB)
	Upvotes      int       `json:"upvotes"`
	Score        float64   `json:"score"` // this board's ranking score
	UpdatedAt    time.Time `json:"updated_at"`
	// Price + ChangePct are joined in by the API from the live universe cache
	// (not stored in the hotlist table); ChangePct is nil when unknown.
	Price     float64  `json:"price,omitempty"`
	ChangePct *float64 `json:"change_pct,omitempty"`
}

// InsiderBuy is one Form 4 filing's open-market insider PURCHASE, summarized
// (the filing's P-transactions aggregated). It is the persistent backbone of the
// Opportunity board — the board itself is derived from recent buys + live market
// caps. Public-domain SEC data; dedupe key is the accession number.
type InsiderBuy struct {
	Accession  string    `json:"accession"` // PK / dedupe key
	Ticker     string    `json:"ticker"`
	CIK        int       `json:"cik"`
	Company    string    `json:"company"`
	OwnerName  string    `json:"owner_name"`
	Title      string    `json:"title"` // officer title, or "Director"
	IsOfficer  bool      `json:"is_officer"`
	IsDirector bool      `json:"is_director"`
	FiledDate  time.Time `json:"filed_date"`
	Shares     float64   `json:"shares"` // total P-shares in the filing
	Price      float64   `json:"price"`  // average buy price
	Value      float64   `json:"value"`  // total $ value of the buys
	FilingURL  string    `json:"filing_url"`
}

// Clip is a link a user saved to a ticker (private, per-user).
type Clip struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Ticker    string    `json:"ticker"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// Note is a user's private note/opinion, attached to a stock (Ticker) and/or a
// calendar date (Date), or neither (a free-floating note). Per-user data (User
// store); editable (unlike clips), so it carries UpdatedAt.
type Note struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Ticker    string    `json:"ticker,omitempty"`    // "" = not stock-scoped
	Date      string    `json:"note_date,omitempty"` // "YYYY-MM-DD"; "" = undated
	Body      string    `json:"body"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NoteFilter selects a user's notes: by Ticker, by [From,To] date range, or all
// of them (all filters empty). Always scoped to UserID.
type NoteFilter struct {
	UserID string
	Ticker string // "" = any
	From   string // "YYYY-MM-DD" inclusive; "" = open
	To     string // "YYYY-MM-DD" inclusive; "" = open
	Limit  int
}

// Alert is a per-user price/event alert on a ticker. Kind ∈ {price_above,
// price_below, pct_move, new_filing}; Threshold is the price level or percent
// (ignored for new_filing). Evaluated off the request path (see internal/ingest).
type Alert struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Ticker      string    `json:"ticker"`
	Kind        string    `json:"kind"`
	Threshold   float64   `json:"threshold"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	TriggeredAt time.Time `json:"triggered_at,omitempty"` // zero = not yet triggered
}

// Holding is a user's position in a ticker: shares held + average cost per share.
// Current value and gain/loss are derived from the live quote (shares × price) at
// read time, never stored, so they track price moves. One row per (user, ticker) —
// re-saving a held ticker overwrites it. Per-user → routed to the User store.
type Holding struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Ticker    string    `json:"ticker"`
	Shares    float64   `json:"shares"`
	AvgCost   float64   `json:"avg_cost"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Earning is a scheduled/reported company earnings event (from the Finnhub
// calendar). Hour ∈ {bmo (before open), amc (after close), dmh (during), ""}.
// Estimate/actual fields are nil when not yet reported/forecast.
type Earning struct {
	Ticker          string    `json:"ticker"`
	Date            time.Time `json:"date"`
	Hour            string    `json:"hour,omitempty"`
	EPSEstimate     *float64  `json:"eps_estimate,omitempty"`
	EPSActual       *float64  `json:"eps_actual,omitempty"`
	RevenueEstimate *float64  `json:"revenue_estimate,omitempty"`
	RevenueActual   *float64  `json:"revenue_actual,omitempty"`
}

// Comment is a PUBLIC user comment on a stock (Ticker) or the global community
// board (Ticker == ""). Unlike notes/clips it's visible to everyone, so it
// carries a public Author display name. IP is captured for moderation but is
// never serialized to clients (json:"-").
type Comment struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Author    string     `json:"author"`
	Ticker    string     `json:"ticker,omitempty"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at,omitempty"` // set when the author edits; nil if never edited
	Likes     int        `json:"likes"`               // total like count (per-user deduped)
	Liked     bool       `json:"liked"`               // whether the requesting viewer liked it (false when anon)
	// Mentions are the $TICKER cashtags extracted from Body at write time
	// (uppercased, deduped). A comment also appears in each mentioned stock's
	// comment list — see ListComments.
	Mentions []string `json:"mentions,omitempty"`
	IP       string   `json:"-"`
}

// FearGreedPoint is one calendar day's headline Fear & Greed score, persisted so
// the sentiment-history curve survives redeploys. Date is the calendar day in
// "2006-01-02" form. It mirrors sentiment.Point but lives in store so the store
// package never imports sentiment (avoiding an import cycle).
type FearGreedPoint struct {
	Date  string `json:"date"`
	Score int    `json:"score"`
}

// Store is the persistence boundary. Every backend (memory, postgres)
// implements this so the rest of the app never depends on a driver.
type Store interface {
	// Ping verifies the backend is reachable (used by the /healthz readiness
	// probe). Returns nil when healthy.
	Ping(ctx context.Context) error

	UpsertSecurity(ctx context.Context, s Security) error
	GetSecurity(ctx context.Context, ticker string) (Security, bool, error)

	SaveFilings(ctx context.Context, ticker string, filings []Filing) error
	ListFilings(ctx context.Context, ticker string, limit int) ([]Filing, error)

	UpsertQuote(ctx context.Context, q Quote) error
	GetQuote(ctx context.Context, ticker string) (Quote, bool, error)

	SaveNews(ctx context.Context, ticker string, items []News) error
	ListNews(ctx context.Context, ticker string, limit int) ([]News, error)
	// ListUntranslatedNews returns up to limit recent news rows with no Chinese
	// headline yet (for the translate ingestor), newest first.
	ListUntranslatedNews(ctx context.Context, limit int) ([]News, error)
	// SetNewsTranslation stores the translated headline for one news row.
	SetNewsTranslation(ctx context.Context, ticker, id, headlineZH string) error

	SaveSocial(ctx context.Context, ticker string, posts []Post) error
	ListSocial(ctx context.Context, ticker string, limit int) ([]Post, error)

	// Signals are per-ticker numeric buzz/sentiment, one row per (ticker,
	// source). SaveSignals upserts a bulk batch (each may be a different ticker);
	// ListSignals returns every source's latest signal for one ticker.
	SaveSignals(ctx context.Context, signals []Signal) error
	ListSignals(ctx context.Context, ticker string) ([]Signal, error)

	// HotList boards (hot / surging / …) are global leaderboards. SaveHotList
	// replaces one board's snapshot; HotList returns that board's top by rank.
	SaveHotList(ctx context.Context, board string, stocks []HotStock) error
	HotList(ctx context.Context, board string, limit int) ([]HotStock, error)

	// InsiderBuys are the persistent corpus behind the Opportunity board.
	// SaveInsiderBuys upserts by accession; RecentInsiderBuys returns buys filed
	// on/after `since`.
	SaveInsiderBuys(ctx context.Context, buys []InsiderBuy) error
	RecentInsiderBuys(ctx context.Context, since time.Time) ([]InsiderBuy, error)

	// Earnings is the upcoming/just-reported company earnings calendar (Finnhub).
	// SaveEarnings upserts by (ticker, date); routed to the durable Market store.
	SaveEarnings(ctx context.Context, es []Earning) error
	ListEarnings(ctx context.Context, from, to time.Time) ([]Earning, error)
	ListEarningsForTicker(ctx context.Context, ticker string, limit int) ([]Earning, error)

	// SeenForm4 records which Form-4 accessions have already been fetched (a
	// buy or not), so a restart skips re-fetching them instead of re-sweeping
	// the whole SEC index. MarkForm4Seen upserts; SeenForm4Since returns the
	// accessions seen on/after `since` (the only window the ingestor rescans).
	MarkForm4Seen(ctx context.Context, accessions []string, filedDate time.Time) error
	SeenForm4Since(ctx context.Context, since time.Time) ([]string, error)

	// FearGreed is the durable daily history of the headline Fear & Greed score
	// (public market data → Market store). SaveFearGreed upserts one day's score,
	// idempotent on the date ("2006-01-02"); a same-day re-save replaces the
	// score. FearGreedHistory returns the most recent `limit` days in
	// CHRONOLOGICAL order (oldest→newest), or all days when limit<=0; it always
	// returns a non-nil (possibly empty) slice.
	SaveFearGreed(ctx context.Context, date string, score int) error
	FearGreedHistory(ctx context.Context, limit int) ([]FearGreedPoint, error)

	// AISummary persists one per-stock AI digest (the LLM summary served at
	// GET /v1/stocks/{ticker}/summary) so it survives process restarts — a true
	// ~1-day TTL keyed by (ticker, ET trading day, lang). Unlike the free-to-
	// rebuild universe/opportunity caches, the digest costs LLM tokens to
	// regenerate, so it earns durable storage (public market data → Market store).
	// SaveAISummary upserts the serialized digest payload (opaque to the store);
	// GetAISummary returns ok=false when there's no entry for that key (the caller
	// then generates), so it's safe to call on every cache miss. The day key is the
	// TTL boundary: yesterday's rows are stale (the caller ignores them by never
	// asking for a past day; old rows are pruned by the retention pruner / harmless
	// if left). Both are best-effort from the caller's view — a store error must not
	// break serving (the caller logs and falls through to generate).
	SaveAISummary(ctx context.Context, ticker, day, lang string, payload []byte) error
	GetAISummary(ctx context.Context, ticker, day, lang string) ([]byte, bool, error)

	// DeepReport persists the prose'd AI Deep Research FactSheet (Product A) so a
	// generated report SURVIVES a server restart — the in-memory cache is wiped on every
	// redeploy, after which the next viewer would otherwise pay a fresh (costly) LLM
	// generation. Keyed by (ticker, lang) only — the report is user-agnostic + shared;
	// the caller enforces a freshness TTL using the returned generatedAt. Durable (Market
	// store), one row per key. SaveDeepReport upserts (payload + generated_at=now());
	// GetDeepReport returns ok=false when there's no row.
	SaveDeepReport(ctx context.Context, ticker, lang string, payload []byte) error
	GetDeepReport(ctx context.Context, ticker, lang string) (payload []byte, generatedAt time.Time, ok bool, err error)

	// Watchlist is one user's tracked tickers, in insertion order.
	Watchlist(ctx context.Context, userID string) ([]string, error)
	AddToWatchlist(ctx context.Context, userID, ticker string) error
	RemoveFromWatchlist(ctx context.Context, userID, ticker string) error
	// AllWatchlistTickers is the de-duplicated union across all users (drives
	// ingestion — we fetch market data for every ticker anyone tracks).
	AllWatchlistTickers(ctx context.Context) ([]string, error)

	// Clips are a user's private saved links.
	SaveClip(ctx context.Context, c Clip) error
	ListClips(ctx context.Context, userID, ticker string, limit int) ([]Clip, error)

	// Notes are a user's private notes/opinions (stock- and/or date-scoped).
	// Update/Delete take userID so ownership is enforced in the query (not-yours
	// → found=false → 404), and return found=false when the note isn't the user's.
	SaveNote(ctx context.Context, n Note) error
	ListNotes(ctx context.Context, f NoteFilter) ([]Note, error)
	UpdateNote(ctx context.Context, userID, id string, body *string, pinned *bool) (Note, bool, error)
	DeleteNote(ctx context.Context, userID, id string) (bool, error)

	// Alerts are per-user ticker price/event alerts (routed to the User store).
	SaveAlert(ctx context.Context, a Alert) error
	ListAlerts(ctx context.Context, userID string) ([]Alert, error)
	DeleteAlert(ctx context.Context, userID, id string) (bool, error)
	// ReactivateAlert re-arms a triggered alert (active=true, triggered_at
	// cleared). Only the owner (userID) may; found=false if unknown/not theirs.
	ReactivateAlert(ctx context.Context, userID, id string) (bool, error)
	// ListActiveAlerts returns ALL users' active, not-yet-triggered alerts (for
	// the evaluator goroutine); MarkAlertTriggered stamps one as fired.
	ListActiveAlerts(ctx context.Context) ([]Alert, error)
	MarkAlertTriggered(ctx context.Context, id string, at time.Time) error

	// Holdings are a user's portfolio positions (routed to the User store).
	// SaveHolding upserts by (user, ticker); Delete takes userID so ownership is
	// enforced in the query (returns found=false when the id isn't the user's).
	SaveHolding(ctx context.Context, h Holding) error
	ListHoldings(ctx context.Context, userID string) ([]Holding, error)
	DeleteHolding(ctx context.Context, userID, id string) (bool, error)

	// Prefs is a per-user JSON preferences blob (small UI state: selected
	// indicators, future view prefs). Opaque to the store — the API owns the
	// shape and caps the size. Routed to the User store via Split (cheap to
	// rebuild, same class as watchlist/notes/alerts). GetPrefs returns ok=false
	// when the user has none (the caller then falls back to defaults, so nothing
	// regresses); PutPrefs overwrites the whole blob.
	GetPrefs(ctx context.Context, userID string) (json.RawMessage, bool, error)
	PutPrefs(ctx context.Context, userID string, blob json.RawMessage) error

	// DeepResearchQuota is the per-user, per-MONTH generation counter that gates the
	// AI Deep Research report (depth=deep): each user gets a small number of NEW
	// deep-report generations per ET CALENDAR MONTH, site-wide (not per stock; free =
	// 1 report/user/month). Viewing an already-generated (globally cached) report
	// does NOT touch this — only a genuinely-new LLM generation increments it. The
	// `period` argument is the ET-month key ("2026-06" style, America/New_York); the
	// underlying column is reused as-is (old per-day rows like "2026-06-15" simply
	// never match a month key, so they become harmless dead weight). Routed to the
	// cheap-to-rebuild User store via Split (same class as prefs/holdings — losing it
	// just resets the period's quota, never market data). GetDeepQuotaUsed returns
	// the count used in the period (0 when no row); IncrDeepQuotaUsed upserts +1.
	// Both are best-effort from the caller's view: a read error fails OPEN (the
	// handler logs + allows, never locking a user out), and an increment error is
	// logged, not fatal.
	GetDeepQuotaUsed(ctx context.Context, userID, period string) (int, error)
	IncrDeepQuotaUsed(ctx context.Context, userID, period string) error

	// Chat persistence (Product B/C). Messages belong to a CONVERSATION (m.ConversationID);
	// ordered oldest→newest. Routed to the cheap-to-rebuild User store via Split.
	// AppendChatMessage adds one turn (and bumps the conversation's updated_at);
	// ListChatMessages returns the most recent `limit` messages for a conversation in
	// chronological order (display + windowed LLM context); ClearChatMessages deletes a
	// conversation's messages (keeping the conversation row). GetOrCreateStockConversation
	// returns the per-(user,ticker) stock conversation, creating it (anchored to the
	// ticker) on first use and LAZILY migrating any legacy (user,ticker) messages that
	// predate conversation_id — so the per-stock chat keeps working with no big-bang backfill.
	AppendChatMessage(ctx context.Context, m ChatMessage) error
	ListChatMessages(ctx context.Context, conversationID string, limit int) ([]ChatMessage, error)
	ClearChatMessages(ctx context.Context, conversationID string) error
	GetOrCreateStockConversation(ctx context.Context, userID, ticker string) (string, error)

	// Conversations (Product C — the unified chat hub). A Conversation is a named chat
	// thread owned by a user, optionally anchored to a stock (AnchorTicker) or general /
	// cross-stock (empty). Messages will reference a conversation; the legacy per-(user,
	// ticker) chat is migrated lazily (C2). All routed to the cheap-to-rebuild User store.
	// CreateConversation returns the new id; List is newest-updated first; Rename/Delete
	// are no-ops on an id the user doesn't own (ownership enforced in the query).
	CreateConversation(ctx context.Context, userID, title, anchorTicker string) (string, error)
	ListConversations(ctx context.Context, userID string) ([]Conversation, error)
	GetConversation(ctx context.Context, userID, id string) (Conversation, bool, error)
	RenameConversation(ctx context.Context, userID, id, title string) error
	DeleteConversation(ctx context.Context, userID, id string) error

	// ChatMsgQuota is the per-user, per-ET-MONTH MESSAGE meter for Product B (Pro is
	// soft-capped at ~150 msgs/mo). Same shape + best-effort semantics as
	// DeepResearchQuota (read fails OPEN, increment is logged-not-fatal), routed to the
	// User store. GetChatMsgUsed returns the count used in the period (0 when none);
	// IncrChatMsgUsed upserts +1.
	GetChatMsgUsed(ctx context.Context, userID, period string) (int, error)
	IncrChatMsgUsed(ctx context.Context, userID, period string) error

	// Comments are PUBLIC user posts on a stock (Ticker) or the global board
	// (Ticker == ""). Durable (Market store). List excludes soft-deleted rows;
	// Delete is author-or-admin (admin=true skips the author check); Report flags
	// a comment for moderation. All return found=false when the id is unknown.
	SaveComment(ctx context.Context, c Comment) error
	ListComments(ctx context.Context, ticker string, limit int, viewerID string) ([]Comment, error)
	DeleteComment(ctx context.Context, id, userID string, admin bool) (bool, error)
	ReportComment(ctx context.Context, id string) (bool, error)
	// UpdateComment edits a comment's body, replacing its cashtag mentions with
	// the given set (extracted from the new body). Only the author (userID
	// match) may edit; returns ok=false if the comment doesn't exist or isn't theirs. Sets
	// EditedAt to now.
	UpdateComment(ctx context.Context, id, userID, body string, mentions []string) (Comment, bool, error)
	// LikeComment toggles a user's like on a comment (one per user). Returns the
	// new liked state for this user and the total like count; ok=false when the
	// comment doesn't exist or is deleted.
	LikeComment(ctx context.Context, id, userID string) (liked bool, likes int, ok bool, err error)

	// Subscriptions hold the Stripe-synced per-user Pro entitlement — the single
	// source of truth for a user's tier, written by the Stripe webhook and read
	// O(1) on the gate hot path. DURABLE (Market store): billing is not cheap to
	// rebuild. The whole surface is INERT until Stripe is configured (no writes
	// without a webhook), so a keyless deployment behaves exactly as today.
	// GetSubscription / GetSubscriptionByCustomer return found=false when the user/
	// customer has no row; UpsertSubscription writes the full row keyed by user_id.
	GetSubscription(ctx context.Context, userID string) (Subscription, bool, error)
	GetSubscriptionByCustomer(ctx context.Context, customerID string) (Subscription, bool, error)
	UpsertSubscription(ctx context.Context, sub Subscription) error
	// MarkStripeEventSeen records a webhook event id for idempotency; it returns
	// fresh=true the FIRST time an id is seen (the caller then processes it) and
	// false if it was already recorded (skip — Stripe delivers at-least-once).
	MarkStripeEventSeen(ctx context.Context, eventID, eventType string) (fresh bool, err error)
	// StripeEventSeen reports whether a webhook event id was already recorded — a
	// READ-ONLY pre-check so the webhook can record an event as seen only AFTER it has
	// been processed successfully (a transient failure then leaves it unrecorded → the
	// Stripe retry genuinely reprocesses it instead of being short-circuited as a dup).
	StripeEventSeen(ctx context.Context, eventID string) (seen bool, err error)
}

// ChatMessage is one persisted turn of a Product B conversation. The thread is implicit
// per (UserID, Ticker). Role is "user" | "assistant". Content holds the user's question
// (plain text) or the assistant's rendered answer (a JSON-encoded ordered block list:
// prose + surfaced-widget refs) — the chat service owns that encoding. CreatedAt is set
// by the store on append (used only for ordering/display).
type ChatMessage struct {
	ConversationID string
	UserID         string
	Ticker         string // anchor/context ticker ("" for general conversations)
	Role           string
	Content        string
	CreatedAt      time.Time
}

// Conversation is a named chat thread in the unified hub (Product C). AnchorTicker is the
// stock it's anchored to ("" = general / cross-stock). Title is auto-derived from the
// first message or user-set. UpdatedAt bumps on each new message (drives the sidebar order).
type Conversation struct {
	ID           string
	UserID       string
	Title        string
	AnchorTicker string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Subscription is a user's billing/entitlement state, synced from Stripe webhooks.
// Tier is the DERIVED entitlement the gates read ("pro" | "free"); Status is the
// raw Stripe subscription status. CurrentPeriodEnd powers a small renewal grace
// window. A user with no row is implicitly free.
type Subscription struct {
	UserID               string
	StripeCustomerID     string
	StripeSubscriptionID string
	Status               string // raw Stripe status: active/trialing/past_due/canceled/…
	Tier                 string // derived: "pro" | "free"
	PriceID              string
	Interval             string // "month" | "year"
	CurrentPeriodEnd     time.Time
	CancelAtPeriodEnd    bool
	UpdatedAt            time.Time
}
