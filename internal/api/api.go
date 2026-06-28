// Package api exposes the HTTP/JSON surface (stdlib net/http only).
//
// Public endpoints (market data) are open so the public stock pages can be
// crawled/shared; per-user endpoints (watchlist, clips) require a valid
// Supabase JWT and are scoped to the caller's user id.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/billing"
	"github.com/wombow-ai/tickwind/internal/cashtag"
	"github.com/wombow-ai/tickwind/internal/chat"
	"github.com/wombow-ai/tickwind/internal/clip"
	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/congress/ptr"
	"github.com/wombow-ai/tickwind/internal/cryptofg"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/events"
	"github.com/wombow-ai/tickwind/internal/finra"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/insideractivity"
	"github.com/wombow-ai/tickwind/internal/materialevents"
	"github.com/wombow-ai/tickwind/internal/movement"
	"github.com/wombow-ai/tickwind/internal/nasdaq"
	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/ratecut"
	"github.com/wombow-ai/tickwind/internal/research"
	"github.com/wombow-ai/tickwind/internal/sec"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
	"github.com/wombow-ai/tickwind/internal/thirteenf"
	"github.com/wombow-ai/tickwind/internal/topics"
	"github.com/wombow-ai/tickwind/internal/treasury"
)

// QuoteStream is the subset of the live hub the API needs to stream prices.
type QuoteStream interface {
	Subscribe() (<-chan store.Quote, func())
}

// BarSource provides recent daily closing prices for a ticker's sparkline and
// full OHLC candles for the K-line chart. It may return a nil slice when no data
// is available; a nil BarSource disables both endpoints (empty series).
type BarSource interface {
	DailyBars(ctx context.Context, ticker string) ([]float64, error)
	DailyCandles(ctx context.Context, ticker string) ([]store.Candle, error)
	IntradayCandles(ctx context.Context, ticker, resolution string) ([]store.Candle, error)
	// LatestQuote fetches an on-demand quote for a ticker the price poller doesn't
	// cover (so a just-viewed stock shows a price, like its candles do).
	LatestQuote(ctx context.Context, ticker string) (store.Quote, bool, error)
}

// bulkQuoter is an OPTIONAL extension a BarSource may implement to resolve many
// on-demand quotes in ONE upstream call. getQuotesBatch type-asserts for it so a
// 40-ticker list/zone refresh is a single bulk snapshot instead of N serialized
// per-ticker LatestQuote fallbacks (the ~5-11s cold-zone latency). Sources without
// it fall back to per-ticker. Satisfied by *ingest.BarCache (one Alpaca
// SnapshotQuotesLive over the uncached misses).
type bulkQuoter interface {
	LatestQuotes(ctx context.Context, tickers []string) (map[string]store.Quote, error)
}

// TopicSource provides the latest trending-topics snapshot. nil disables the
// topics endpoint (returns an empty list).
type TopicSource interface {
	Get() topics.Snapshot
}

// OpportunitySource provides the latest Opportunity board. nil → empty list.
type OpportunitySource interface {
	Get() []opportunity.Stock
}

// UniverseSource is the whole-US-market quote cache (price + change reference per
// ticker), nil-safe — powers the /v1/universe status, a cold-price fast path, and
// the /v1/screen screener (which iterates Snapshot()).
type UniverseSource interface {
	Get(ticker string) (store.Quote, bool)
	Snapshot() map[string]store.Quote
	// Tickers returns the sorted quote-bearing symbols (every ticker with a
	// usable price), powering /v1/universe/symbols — the full ~6,695 pSEO
	// /stock universe, a subset of the SEC+Nasdaq listing /v1/symbols exposes.
	Tickers() []string
	Len() int
	UpdatedAt() time.Time
}

// GuruSource provides the latest Guru-watch rail (curated-KOL posts) plus the
// time it was last refreshed (rail freshness). nil → empty list.
type GuruSource interface {
	Get() []guru.Item
	UpdatedAt() time.Time
}

// TickerIngestor triggers a one-shot data pull (filings/news/social) for a
// single ticker, so a newly watch-listed stock is populated immediately instead
// of waiting for the next scheduler cycle. nil disables on-add ingestion.
type TickerIngestor interface {
	IngestOne(ctx context.Context, ticker string)
}

// SymbolSearcher searches the symbol directory for autocomplete. nil → empty.
type SymbolSearcher interface {
	Search(q string, limit int) []symbols.Symbol
	// ByCIK resolves a SEC Central Index Key to its symbol, so CIK-keyed filings
	// (e.g. 13D/13G ownership refs) can link to the stock page. ok=false when
	// unknown / the directory is unloaded.
	ByCIK(cik int) (symbols.Symbol, bool)
	// AllUSTickers enumerates every US-listed ticker (~6,700), so the pSEO
	// sitemap can seed a /stock/[ticker] page per symbol. nil/empty while the
	// directory is unloaded.
	AllUSTickers() []string
}

// EventSource provides the latest major-events timeline. nil → empty list.
type EventSource interface {
	Get() []events.Event
}

// EarningsSource provides the company earnings calendar — date-windowed and
// per-ticker. nil-safe (a nil source yields empty lists). store.Store satisfies
// it directly (ListEarnings / ListEarningsForTicker), so main.go passes the store.
type EarningsSource interface {
	ListEarnings(ctx context.Context, from, to time.Time) ([]store.Earning, error)
	ListEarningsForTicker(ctx context.Context, ticker string, limit int) ([]store.Earning, error)
}

// CongressSource provides the latest snapshot of congressional Periodic
// Transaction Reports (House Clerk public-domain filings). nil → empty list.
type CongressSource interface {
	Get() []congress.Filing
}

// CongressTxSource provides the ticker- and member-level transactions parsed out
// of the PTR PDFs, powering the per-stock "members trading this" chip and the
// per-member page. nil-safe (injected post-New via SetCongressTx). Satisfied by
// *congress.Cache.
type CongressTxSource interface {
	ByTicker(ticker string) []congress.TickerTrade
	ByMember(slug string) (congress.MemberTx, bool)
}

// InstitutionalSource provides the latest snapshot of SEC Schedule 13D/13G
// beneficial-ownership filings (institutional / activist stakes). nil → empty.
type InstitutionalSource interface {
	Get() []sec.OwnershipRef
}

// LiveSubscriber adds a just-viewed ticker to the real-time price stream so its
// price updates live (nil-safe; nil = no-op). Satisfied by *alpacaws.Streamer.
type LiveSubscriber interface {
	Subscribe(ticker string)
}

// IndicesSource serves the latest major-market-index levels for the homepage
// strip (nil-safe; nil = empty). Currently always nil — the backend index-level
// source (gray Yahoo) was removed, so /v1/indices returns empty and the frontend
// strip self-falls-back to keyless Alpaca ETF proxies. The seam is kept for a
// future licensed index feed.
type IndicesSource interface {
	Indices() []store.IndexQuote
}

// ShortSource serves the latest-settlement FINRA short-interest row for a
// symbol (nil-safe; nil = none). Satisfied by *ingest.ShortCache.
type ShortSource interface {
	ShortInterest(ticker string) (finra.ShortInterest, bool)
}

// BriefingSource serves the day's AI pre-market briefing (nil-safe; nil or
// ok=false = 404). Satisfied by *ingest.BriefingCache.
type BriefingSource interface {
	Get(lang string) (date, text string, at time.Time, ok bool)
}

// OptionsSource serves the per-stock delayed options overview + the whole-market
// unusual-activity board (nil-safe). Satisfied by *ingest.OptionsCache.
type OptionsSource interface {
	Options(ctx context.Context, ticker string) (ingest.OptionsView, bool)
	Unusual() ([]ingest.UnusualContract, time.Time)
}

// ThirteenFSource serves the 13F whale-holdings board — famous funds' quarterly
// holdings + quarter-over-quarter changes (nil-safe). Board powers the
// smart-money board; Holders powers the per-stock "which whales own this" chip
// (reverse index); Fund powers a single fund's pSEO page. Satisfied by
// *thirteenf.Cache.
type ThirteenFSource interface {
	Board() (thirteenf.Board, bool)
	Holders(ticker string) []thirteenf.Holder
	Fund(slug string) (thirteenf.FundHoldings, bool)
}

// ShortVolumeSource serves FINRA daily short-volume data: a ranked "most-shorted
// today" leaderboard (Top) and one symbol's latest row + short rolling history
// for the per-stock daily short-pressure curve. FINRA's terms are display-only
// (no bulk raw-row redistribution), so only the ranked Top is exposed in bulk.
// nil-safe — a nil source yields empty endpoints. Satisfied by *finrashvol.Cache.
type ShortVolumeSource interface {
	// Top returns the latest day's symbols ranked by short percentage, capped at
	// n, considering only rows with TotalVolume >= minTotalVolume.
	Top(n int, minTotalVolume int64) []finrashvol.ShortVol
	// Latest returns one symbol's latest day's short volume (ok=false if absent).
	Latest(sym string) (finrashvol.ShortVol, bool)
	// History returns one symbol's retained short-volume history (oldest first).
	History(sym string) []finrashvol.ShortVol
	// AsOf is the report date of the latest day held (YYYY-MM-DD), "" if never set.
	AsOf() string
}

// SentimentSource serves the latest Fear & Greed Result plus a daily history of
// scores for the chart. nil-safe — a nil source yields an empty index.
// Satisfied by *sentiment.Cache.
type SentimentSource interface {
	Latest() (sentiment.Result, bool)
	History() []sentiment.Point
	UpdatedAt() time.Time
}

// RateCutSource serves the aggregated Fed rate-cut prediction markets (Kalshi +
// Polymarket). nil-safe — a nil source yields an empty market list. Satisfied by
// *ratecut.Cache.
type RateCutSource interface {
	Get() []ratecut.Market
	UpdatedAt() time.Time
}

// MacroSource serves the latest U.S. Treasury daily par yield curve (the 2Y/10Y
// tenors + the 2s10s recession-watch spread) for the macro-context strip.
// nil-safe — a nil source (or one before its first refresh) yields an
// "unavailable" empty shape, never fabricated rates. Satisfied by
// *treasury.Cache.
type MacroSource interface {
	Latest() (treasury.Curve, bool)
	UpdatedAt() time.Time
}

// CryptoSource serves the latest crypto market-mood snapshot (the crypto Fear &
// Greed index + best-effort BTC/ETH prices) for the crypto-context strip —
// relevant to the crypto-linked equities COIN/MSTR/RIOT/MARA. nil-safe — a nil
// source (or one before its first refresh) yields an "unavailable" empty shape,
// never fabricated values. Satisfied by *cryptofg.Cache.
type CryptoSource interface {
	Latest() (cryptofg.Index, bool)
	UpdatedAt() time.Time
}

// IPOSource serves the latest US IPO calendar (recently priced / upcoming /
// newly filed offerings, via Nasdaq through the residential proxy). nil-safe —
// a nil source (or one before its first refresh) yields empty sections.
// Satisfied by *ingest.IPOIngestor.
type IPOSource interface {
	Calendar() (nasdaq.Calendar, time.Time)
}

// IndicatorSource serves the stock-applicable indicator catalog (static,
// embedded metadata) for the browsable indicator library. nil-safe — a nil
// source yields an empty catalog. Satisfied by *indicators.Catalog.
type IndicatorSource interface {
	Filter(q indicators.Query) []indicators.Indicator
	Facets() indicators.Facets
	Len() int
}

// IndicatorComputeSource computes the live P0 stock-applicable indicator set for
// a single ticker (latest values), wiring the catalog metadata to the ticker's
// fetched candles / fundamentals / price / market context. nil-safe — a nil
// source makes the per-stock indicators endpoint 404. Satisfied by
// *indicators.Computer.
type IndicatorComputeSource interface {
	StockIndicators(ctx context.Context, ticker string) indicators.StockIndicatorsResult
}

// ResearchSource produces the per-ticker deep-research report. Report assembles
// the data-only fact sheet (no LLM, cheap, never errors); Compose fills per-section
// qualitative prose via the optional LLM (degrades to the unchanged data-only sheet);
// Enabled reports whether the LLM backend is configured; Model is its name ("" when
// disabled). nil-safe — a nil source makes /v1/stocks/{ticker}/research 404.
// Satisfied by *research.Service.
type ResearchSource interface {
	Report(ctx context.Context, ticker, lang string) research.FactSheet
	Compose(ctx context.Context, fs research.FactSheet, lang string) research.FactSheet
	// ComposeDeep fills RICHER per-section prose (the AI Deep Research report,
	// depth=deep) via a possibly stronger model + a Fable-5 harness, over the SAME
	// Go-owned facts. Same off-the-critical-path degradation and same
	// never-touch-a-number contract as Compose. DeepModel is its model name.
	ComposeDeep(ctx context.Context, fs research.FactSheet, lang string) research.FactSheet
	Enabled() bool
	Model() string
	DeepModel() string
}

// MovementSource produces the move-triggered "why did this stock move today?"
// explainer. Report assembles the data-only explanation (Go-owned change % +
// direction + attributed evidence + canned line; never errors, never an LLM
// call); Explain optionally overlays one hedged LLM sentence (degrading to the
// data-only explanation when the LLM is off/over-cap/errors — never the LLM's
// number). Enabled reports whether the LLM backend is configured; Model is its
// name ("" when disabled). nil-safe — a nil source makes
// /v1/stocks/{ticker}/movement 404. Satisfied by *movement.Service.
type MovementSource interface {
	Report(ctx context.Context, ticker, lang string) movement.Explanation
	Explain(ctx context.Context, ticker, lang string) movement.Explanation
	Enabled() bool
	Model() string
}

// MaterialEventsSource produces a company's recent 8-K (current report) filings
// with an optional AI plain-language summary. Report assembles the facts-only
// report (Go-owned form/dates/accession URL + parsed item codes & canonical
// labels; no LLM); Summarize optionally overlays a short LLM summary per filing
// (degrading to facts-only when the LLM is off/over-cap/errors — never the LLM's
// facts). Both error only when the ticker/CIK can't be resolved or the SEC feed
// fetch fails (the handler 404s on that); an existing company with zero recent
// 8-Ks returns an empty (non-nil) Filings slice. Enabled reports whether the LLM
// backend is configured; Model is its name ("" when disabled). nil-safe — a nil
// source makes /v1/stocks/{ticker}/material-events 404. Satisfied by
// *materialevents.Service.
type MaterialEventsSource interface {
	Report(ctx context.Context, ticker string) (materialevents.Report, error)
	Summarize(ctx context.Context, ticker, lang string) (materialevents.Report, error)
	Enabled() bool
	Model() string
}

// MaterialFeedSource serves a market-wide feed of recent NOTABLE 8-K material events (leadership
// change, M&A, bankruptcy, restatement, …) across the tracked universe — facts only, no LLM; an
// optional item code filters it. See ingest.MaterialFeedCache. nil-safe — a nil source makes
// /v1/material-feed 404. Satisfied by *ingest.MaterialFeedCache.
type MaterialFeedSource interface {
	Feed(item string) ([]materialevents.FeedEvent, time.Time)
}

// EarningsDatesSource provides a company's past earnings-announcement dates (the filing dates of
// its 8-K item 2.02 reports), newest-first — the dated anchors for the deterministic
// earnings-reaction statistic. Every date is a real SEC filing date (anti-hallucination-safe).
// nil-safe — a nil source makes /v1/stocks/{ticker}/earnings-reaction 404. Satisfied by
// *edgar.Client (its EarningsDates method).
type EarningsDatesSource interface {
	EarningsDates(ctx context.Context, ticker string) ([]time.Time, error)
}

// EarningsReactionSource provides cached earnings-reaction AGGREGATES for the tracked set, so the
// earnings calendar can badge how a stock has historically moved around its reports without a
// per-row compute. nil-safe — when unset, calendar rows simply carry no reaction. Satisfied by
// *ingest.EarningsReactionCache.
type EarningsReactionSource interface {
	Reaction(ticker string) (indicators.ReactionSummary, bool)
	// PopulationRanked ranks the tracked universe by the chosen earnings-reaction view (the
	// market-wide leaderboard); see indicators.RankEarningsReaction. Reused by
	// /v1/screen/earnings-reaction.
	PopulationRanked(view string) ([]indicators.ReactionRank, time.Time)
}

// ScorecardSource provides the factor-metric POPULATION (the percentile-ranking distribution over
// the tracked universe) for the multi-factor scorecard. The /scorecard handler computes the target
// ticker's own factor metrics on-demand and ranks them against this population. nil-safe — a nil
// source makes /v1/stocks/{ticker}/scorecard 404. Satisfied by *ingest.ScorecardCache.
type ScorecardSource interface {
	Population() ([]indicators.FactorMetrics, time.Time)
	// PopulationRanked ranks the tracked universe on one factor (the market-wide factor
	// leaderboard); see indicators.RankFactor. Reused by /v1/screen/factors.
	PopulationRanked(factor string) ([]indicators.FactorRank, time.Time)
}

// RSScanSource ranks the tracked universe by relative strength vs SPY over a trailing window (the
// market-wide RS leaderboard); see indicators.RankRelativeStrength. nil-safe — a nil source makes
// /v1/screen/relative-strength 404. Satisfied by *ingest.RelativeStrengthCache.
type RSScanSource interface {
	RankByWindow(window string) ([]indicators.RSRank, time.Time)
}

// DividendScanSource ranks the tracked universe by a dividend VIEW (highest-yield | fastest-growing |
// best-covered | lowest-payout) — the market-wide dividend leaderboard; see indicators.RankDividend.
// nil-safe — a nil source makes /v1/screen/dividends 404. Satisfied by *ingest.DividendCache.
type DividendScanSource interface {
	PopulationRanked(view string) ([]indicators.DividendRank, time.Time)
}

// InsiderActivitySource produces a company's recent insider-activity timeline —
// open-market Form 4 buys AND sells, newest first, each with the Go-owned facts
// (shares/price/value/date, insider name + role, buy/sell, the best-effort
// Rule 10b5-1 planned-sale flag, accession URL) plus cheap aggregates. There is
// NO LLM in this feature: it is pure structured data. Report errors only when the
// ticker/CIK can't be resolved or the SEC feed fetch fails (the handler 404s on
// that); an existing company with zero recent Form 4s returns an empty (non-nil)
// Transactions slice. nil-safe — a nil source makes
// /v1/stocks/{ticker}/insider-activity 404. Satisfied by *insideractivity.Service.
type InsiderActivitySource interface {
	Report(ctx context.Context, ticker string) (insideractivity.Report, error)
}

type Server struct {
	store        store.Store
	hub          QuoteStream
	clip         *clip.Fetcher
	enrich       enrich.Enricher
	auth         *auth.Verifier
	bars         BarSource
	topics       TopicSource
	opps         OpportunitySource
	universe     UniverseSource
	gurus        GuruSource
	ingestor     TickerIngestor
	symbols      SymbolSearcher
	events       EventSource
	fundamentals FundamentalsSource
	etf          ETFHoldingsSource // ETF/fund N-PORT holdings (SEC), on-demand; set via SetETFHoldings
	earnings     EarningsSource
	// earningsReactions badges calendar rows with how the stock has historically moved around
	// earnings; injected post-New via SetEarningsReactions. nil → rows carry no reaction.
	earningsReactions EarningsReactionSource
	congress          CongressSource
	institutional     InstitutionalSource
	live              LiveSubscriber
	indices           IndicesSource
	short             ShortSource
	briefing          BriefingSource
	options           OptionsSource
	thirteenf         ThirteenFSource
	shortVolume       ShortVolumeSource      // injected post-New via SetShortVolume (avoids growing the New signature)
	sentiment         SentimentSource        // injected post-New via SetSentiment
	rateCut           RateCutSource          // injected post-New via SetRateCut
	macro             MacroSource            // injected post-New via SetMacro (Treasury yield curve)
	crypto            CryptoSource           // injected post-New via SetCrypto (crypto Fear & Greed)
	congressTx        CongressTxSource       // injected post-New via SetCongressTx
	ipo               IPOSource              // injected post-New via SetIPO
	indicators        IndicatorSource        // injected post-New via SetIndicators (static catalog)
	indicatorCalc     IndicatorComputeSource // injected post-New via SetIndicatorCompute (per-stock compute)
	researchCalc      ResearchSource         // injected post-New via SetResearch (deep-research report)
	movementCalc      MovementSource         // injected post-New via SetMovement (move-explainer)
	materialCalc      MaterialEventsSource   // injected post-New via SetMaterialEvents (8-K material events + AI summary)
	materialFeed      MaterialFeedSource     // injected post-New via SetMaterialFeed (market-wide notable-events feed)
	earningsDates     EarningsDatesSource    // injected post-New via SetEarningsDates (8-K 2.02 dates for earnings-reaction)
	scorecard         ScorecardSource        // injected post-New via SetScorecard (factor-percentile population)
	rsScan            RSScanSource           // injected post-New via SetRSScan (relative-strength leaderboard)
	dividendScan      DividendScanSource     // injected post-New via SetDividendScan (dividend leaderboard)
	billing           *billing.Service       // injected post-New via SetBilling (Stripe; nil/disabled until keys are set)
	insiderCalc       InsiderActivitySource  // injected post-New via SetInsiderActivity (Form 4 buy/sell timeline; no LLM)
	admins            map[string]bool        // user UUIDs and/or emails (lowercased) allowed to delete any comment
	commentRL         *rateLimiter           // per-user comment-post throttle
	// AI digest cache: one LLM generation per (ticker, ET day), then served from
	// memory — token spend stays bounded no matter the traffic. Guarded by sumMu;
	// sumInflight dedupes concurrent first requests; sumDay* enforce a global
	// per-day generation cap.
	sumMu       sync.Mutex
	sumCache    map[string]summaryEntry
	sumInflight map[string]chan struct{}
	sumDayDate  string
	sumDayCount int
	// Deep-research report cache: the data-only fact sheet is cheap, but the LLM
	// prose is one bigger generation per (ticker, ET day, lang), then served from
	// memory — mirrors the AI digest cache. Guarded by researchMu; researchInflight
	// dedupes concurrent first requests; researchDay* enforce a global per-day prose
	// generation cap (the cap gates PROSE only — the data-only report always serves).
	//
	// For depth=deep the prose is generated ASYNCHRONOUSLY in a detached background
	// goroutine (see getResearch): the request returns the data-only fact sheet
	// INSTANTLY with prose_status="generating", and researchInflight marks the
	// (ticker, ET-month, lang) entry as a bg gen in flight so concurrent polls return
	// "generating" without spawning a second gen or double-charging the quota. The bg
	// goroutine stores the prose'd sheet in researchCache and charges the quota EXACTLY
	// ONCE on success; a failed/empty gen caches nothing and charges nothing (retryable).
	researchMu       sync.Mutex
	researchCache    map[string]researchEntry
	researchInflight map[string]chan struct{}
	researchDayDate  string
	researchDayCount int
	// deepResearchLimit is the per-user, per-ET-MONTH GENERATION quota for the deep
	// report (depth=deep), set from config via SetDeepResearchLimit (default 1 =
	// free 1 report/user/month). Only a genuinely-new generation (cache miss + a real
	// LLM compose that produced prose) consumes a user's quota; viewing a globally
	// cached deep report, or a still-generating poll, is free.
	deepResearchLimit int
	// deepResearchLimitPro is the same per-user, per-ET-MONTH GENERATION quota for PRO
	// subscribers (set from config via SetDeepResearchLimitPro, default 100 ≈ on-demand).
	// The quota check picks this when tierOf == pro so upgrading actually lifts the cap —
	// otherwise the "upgrade to Pro" upsell on the limit-reached screen would be hollow.
	deepResearchLimitPro int
	// paywallEnabled turns ON the user-facing Pro paywall (free-tier deep-report
	// truncation). Default false (no paywall; full report for everyone) until the owner
	// flips it at go-live; injected post-New via SetPaywallEnabled.
	paywallEnabled bool
	// indicatorsPaywallEnabled turns ON the Pro paywall for the deterministic signals
	// layer (GET /v1/stocks/{ticker}/indicator-signals → teaser for free viewers). Default false
	// (full signal list for everyone) until go-live; injected via SetIndicatorsPaywallEnabled.
	indicatorsPaywallEnabled bool
	// signalScan is the whole-universe signals SCREENER source (a background cache),
	// injected post-New via SetSignalScan. nil → /v1/screen/signals 404s.
	signalScan SignalScanSource
	// chatSvc is the Product B personalized-chat engine (tool loop + anti-hallucination
	// firewall), injected post-New via SetChat. nil → /v1/stocks/{ticker}/chat 503s.
	chatSvc *chat.Service
	// chatMonthlyLimit is the per-Pro-user, per-ET-month MESSAGE soft-cap (SetChatLimit,
	// default 150). Over it the chat endpoint soft-degrades (a note, not an error).
	chatMonthlyLimit int
	// chatMonthlyTokenLimit is the per-Pro-user, per-ET-month TOKEN soft-cap — the cost-true
	// quota the hub gates + shows as a percentage (SetChatTokenLimit, default 1,000,000).
	chatMonthlyTokenLimit int
	// chatFreeWeeklyTokens is the per-FREE-user, per-ET-WEEK TOKEN soft-cap (SetChatFreeWeeklyTokens,
	// default 50,000) — a small weekly token taste of the chat for signed-in non-Pro users
	// (token-based like Pro); over it the chat returns an upgrade note and the UI hides the meter.
	chatFreeWeeklyTokens int
	// chatRL throttles chat posts per user (burst control atop the monthly meter).
	chatRL *rateLimiter
	// Global per-ET-day chat-generation backstop (catastrophic LLM-cost guard), guarded
	// by chatMu and reset on a new ET day.
	chatMu       sync.Mutex
	chatDayDate  string
	chatDayCount int
	// Move-explainer cache: the data-only explanation (Go number + evidence + canned
	// line) is cheap, but the LLM's hedged sentence is one small generation per
	// (ticker, ET day, lang), then served from memory — mirrors the AI digest cache.
	// Guarded by moveMu; moveInflight dedupes concurrent first requests; moveDay*
	// enforce a global per-day generation cap (the cap gates the LLM SENTENCE only —
	// the data-only explanation, including a sub-threshold "not significant", always
	// serves).
	moveMu       sync.Mutex
	moveCache    map[string]movementEntry
	moveInflight map[string]chan struct{}
	moveDayDate  string
	moveDayCount int
	// Material-events (8-K) cache: the facts (form/dates/items+labels) are cheap to
	// fetch, but the per-filing LLM summaries are the cost, so a full assembled
	// report (facts + optional summaries) is generated at most once per (ticker, ET
	// day, lang) and served from memory — mirrors the move-explainer cache. Guarded
	// by meMu; meInflight dedupes concurrent first requests; meDay* enforce a global
	// per-day report-generation cap (the cap gates the LLM-summary path only — a
	// facts-only report still serves over cap). Old days are swept on a new ET day.
	meMu       sync.Mutex
	meCache    map[string]materialEventsEntry
	meInflight map[string]chan struct{}
	meDayDate  string
	meDayCount int
	// Insider-activity (Form 4 buy/sell) cache: the timeline is pure structured
	// data (no LLM), but assembling it fetches each recent Form 4's XML (N throttled
	// SEC requests), so the assembled report is built at most once per (ticker, ET
	// day) and served from memory — mirrors the material-events cache, minus the
	// LLM/daily-cap machinery. Guarded by iaMu; iaInflight dedupes concurrent first
	// requests. Old days are swept lazily on the first hit of a new ET day.
	iaMu       sync.Mutex
	iaCache    map[string]insiderActivityEntry
	iaInflight map[string]chan struct{}
	iaDay      string
	// Follow-trade backtest cache: the simulation is deterministic for a given
	// (member, price-day), so compute once per slug per UTC day and serve from
	// memory (the per-ticker DailyCandles fetch is the only cost). Guarded by btMu.
	btMu    sync.Mutex
	btCache map[string]backtestEntry
	// Personalized overnight-digest cache: the AI overview + per-stock roll-up is
	// generated at most once per (userID, ET day) — the day's first visit pays the
	// data assembly + (optional) one LLM call, everyone else (and every refresh) hits
	// the cache, so per-user token spend is bounded. Guarded by digestMu; keyed
	// userID|day|lang. Old days are swept lazily on the first hit of a new ET day.
	digestMu    sync.Mutex
	digestCache map[string]digestEntry
	// digestInflight dedupes the background AI-overview compose so concurrent first
	// visits / polls spawn exactly one generation per (userID, day, lang). Guarded by
	// digestMu. The data rows are served INSTANTLY; only Pro users' AI summary is
	// composed in the background (and polled), so the My/Overview tab never blocks on
	// the LLM (the slow part) — see getMyDigest.
	digestInflight map[string]chan struct{}
	log            *slog.Logger
	handler        http.Handler // the assembled mux + middleware chain (set in New)
}

// ServeHTTP dispatches to the assembled mux + middleware chain, so *Server is an
// http.Handler. (New returns *Server so callers can inject the setter-based
// sources before serving; the handler chain itself is built once in New.)
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// New builds the API server. It returns a *Server (an http.Handler) so callers
// can inject additional sources via the Set* methods before serving — this keeps
// the already-long positional signature stable as new optional sources are added.
func New(st store.Store, hub QuoteStream, enricher enrich.Enricher, verifier *auth.Verifier, bars BarSource, topicSrc TopicSource, oppSrc OpportunitySource, universeSrc UniverseSource, guruSrc GuruSource, ingestor TickerIngestor, symbolSrc SymbolSearcher, eventSrc EventSource, fundSrc FundamentalsSource, earningsSrc EarningsSource, congressSrc CongressSource, institutionalSrc InstitutionalSource, liveSub LiveSubscriber, indicesSrc IndicesSource, shortSrc ShortSource, briefingSrc BriefingSource, optionsSrc OptionsSource, thirteenfSrc ThirteenFSource, adminIDs []string, log *slog.Logger) *Server {
	admins := make(map[string]bool, len(adminIDs))
	for _, id := range adminIDs {
		if id = strings.ToLower(strings.TrimSpace(id)); id != "" {
			admins[id] = true
		}
	}
	s := &Server{store: st, hub: hub, clip: clip.NewFetcher(), enrich: enricher, auth: verifier, bars: bars, topics: topicSrc, opps: oppSrc, universe: universeSrc, gurus: guruSrc, ingestor: ingestor, symbols: symbolSrc, events: eventSrc, fundamentals: fundSrc, earnings: earningsSrc, congress: congressSrc, institutional: institutionalSrc, live: liveSub, indices: indicesSrc, short: shortSrc, briefing: briefingSrc, options: optionsSrc, thirteenf: thirteenfSrc, admins: admins, commentRL: newRateLimiter(10, 10*time.Minute), chatMonthlyLimit: 150, chatFreeWeeklyTokens: 50_000, chatRL: newRateLimiter(20, 10*time.Minute), sumCache: map[string]summaryEntry{}, sumInflight: map[string]chan struct{}{}, researchCache: map[string]researchEntry{}, researchInflight: map[string]chan struct{}{}, deepResearchLimit: 1, deepResearchLimitPro: 100, moveCache: map[string]movementEntry{}, moveInflight: map[string]chan struct{}{}, meCache: map[string]materialEventsEntry{}, meInflight: map[string]chan struct{}{}, iaCache: map[string]insiderActivityEntry{}, iaInflight: map[string]chan struct{}{}, btCache: map[string]backtestEntry{}, digestCache: map[string]digestEntry{}, digestInflight: map[string]chan struct{}{}, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)

	// Per-user (auth required)
	mux.HandleFunc("GET /v1/watchlist", s.getWatchlist)
	mux.HandleFunc("POST /v1/watchlist", s.postWatchlist)
	mux.HandleFunc("DELETE /v1/watchlist/{ticker}", s.deleteWatchlist)
	mux.HandleFunc("POST /v1/stocks/{ticker}/clip", s.postClip)
	mux.HandleFunc("GET /v1/stocks/{ticker}/clips", s.getClips)
	mux.HandleFunc("POST /v1/notes", s.postNote)
	mux.HandleFunc("GET /v1/notes", s.getNotes)
	mux.HandleFunc("PATCH /v1/notes/{id}", s.patchNote)
	mux.HandleFunc("DELETE /v1/notes/{id}", s.deleteNote)
	mux.HandleFunc("GET /v1/alerts", s.getAlerts)
	mux.HandleFunc("POST /v1/alerts", s.postAlert)
	mux.HandleFunc("DELETE /v1/alerts/{id}", s.deleteAlert)
	mux.HandleFunc("PATCH /v1/alerts/{id}", s.reactivateAlert)
	mux.HandleFunc("GET /v1/holdings", s.getHoldings)
	mux.HandleFunc("POST /v1/holdings", s.postHolding)
	mux.HandleFunc("DELETE /v1/holdings/{id}", s.deleteHolding)
	mux.HandleFunc("GET /v1/me/digest", s.getMyDigest)
	mux.HandleFunc("GET /v1/me/prefs", s.getMyPrefs)
	mux.HandleFunc("PUT /v1/me/prefs", s.putMyPrefs)
	mux.HandleFunc("GET /v1/comments", s.getComments) // public read
	mux.HandleFunc("POST /v1/comments", s.postComment)
	mux.HandleFunc("PATCH /v1/comments/{id}", s.patchComment)
	mux.HandleFunc("DELETE /v1/comments/{id}", s.deleteComment)
	mux.HandleFunc("POST /v1/comments/{id}/report", s.reportComment)
	mux.HandleFunc("POST /v1/comments/{id}/like", s.likeComment)

	// Stripe billing (Pro entitlement). Registered unconditionally; each handler is
	// a no-op 404 until SetBilling injects a configured (key-bearing) service, so a
	// keyless deployment exposes nothing. The webhook is intentionally outside the
	// auth gate (server-to-server, signature-verified) and not rate-limited.
	mux.HandleFunc("POST /v1/stripe/webhook", s.stripeWebhook)
	mux.HandleFunc("POST /v1/billing/checkout", s.billingCheckout)
	mux.HandleFunc("POST /v1/billing/portal", s.billingPortal)
	mux.HandleFunc("GET /v1/billing/me", s.billingMe)

	// Public (market data — open for SEO / shareable stock pages)
	mux.HandleFunc("GET /v1/stocks/{ticker}", s.getStock)
	mux.HandleFunc("GET /v1/stocks/{ticker}/filings", s.getFilings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/quote", s.getQuote)
	mux.HandleFunc("POST /v1/stocks/{ticker}/subscribe", s.subscribeLive)
	mux.HandleFunc("POST /v1/live/subscribe", s.subscribeLiveBatch)
	mux.HandleFunc("GET /v1/stocks/{ticker}/bars", s.getBars)
	mux.HandleFunc("GET /v1/stocks/{ticker}/candles", s.getCandles)
	mux.HandleFunc("GET /v1/stocks/{ticker}/fundamentals", s.getFundamentals)
	mux.HandleFunc("GET /v1/etf/{ticker}/holdings", s.getETFHoldings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/news", s.getNews)
	mux.HandleFunc("GET /v1/stocks/{ticker}/social", s.getSocial)
	mux.HandleFunc("GET /v1/stocks/{ticker}/signals", s.getSignals)
	mux.HandleFunc("GET /v1/stocks/{ticker}/earnings", s.getStockEarnings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/summary", s.getSummary)
	mux.HandleFunc("GET /v1/bars", s.getBarsBatch)
	mux.HandleFunc("GET /v1/quotes", s.getQuotesBatch)
	mux.HandleFunc("GET /v1/news", s.getNewsBatch)
	mux.HandleFunc("GET /v1/social", s.getSocialBatch)
	mux.HandleFunc("GET /v1/hot", s.getHot)
	mux.HandleFunc("GET /v1/topics", s.getTopics)
	mux.HandleFunc("GET /v1/opportunities", s.getOpportunities)
	mux.HandleFunc("GET /v1/universe", s.getUniverse)
	mux.HandleFunc("GET /v1/universe/symbols", s.getUniverseSymbols)
	mux.HandleFunc("GET /v1/screen", s.getScreen)
	mux.HandleFunc("GET /v1/screen/signals", s.getScreenSignals)
	mux.HandleFunc("GET /v1/screen/factors", s.getFactorScreen)
	mux.HandleFunc("GET /v1/screen/relative-strength", s.getRSScreen)
	mux.HandleFunc("GET /v1/screen/earnings-reaction", s.getEarningsReactionScreen)
	mux.HandleFunc("GET /v1/screen/dividends", s.getDividendScreen)
	mux.HandleFunc("GET /v1/gurus", s.getGurus)
	mux.HandleFunc("GET /v1/search", s.getSearch)
	mux.HandleFunc("GET /v1/symbols", s.getSymbols)
	mux.HandleFunc("GET /v1/events", s.getEvents)
	mux.HandleFunc("GET /v1/earnings", s.getEarnings)
	mux.HandleFunc("GET /v1/congress", s.getCongress)
	mux.HandleFunc("GET /v1/congress/member/{slug}", s.getCongressMember)
	mux.HandleFunc("GET /v1/congress/member/{slug}/backtest", s.getCongressBacktest)
	mux.HandleFunc("GET /v1/stocks/{ticker}/congress", s.getStockCongress)
	mux.HandleFunc("GET /v1/institutional", s.getInstitutional)
	mux.HandleFunc("GET /v1/indices", s.getIndices)
	mux.HandleFunc("GET /v1/briefing", s.getBriefing)
	mux.HandleFunc("GET /v1/stocks/{ticker}/short", s.getShort)
	mux.HandleFunc("GET /v1/short-volume", s.getShortVolume)
	mux.HandleFunc("GET /v1/sentiment", s.getSentiment)
	mux.HandleFunc("GET /v1/ratecut", s.getRateCut)
	mux.HandleFunc("GET /v1/macro", s.getMacro)
	mux.HandleFunc("GET /v1/crypto", s.getCrypto)
	mux.HandleFunc("GET /v1/ipo", s.getIPO)
	mux.HandleFunc("GET /v1/stocks/{ticker}/options", s.getOptions)
	mux.HandleFunc("GET /v1/options/unusual", s.getUnusualOptions)
	mux.HandleFunc("GET /v1/13f", s.getThirteenF)
	mux.HandleFunc("GET /v1/13f/{slug}", s.getThirteenFFund)
	mux.HandleFunc("GET /v1/stocks/{ticker}/whales", s.getWhales)
	mux.HandleFunc("GET /v1/stocks/{ticker}/indicators", s.getStockIndicators)
	mux.HandleFunc("GET /v1/stocks/{ticker}/indicator-signals", s.getStockSignals)
	mux.HandleFunc("GET /v1/stocks/{ticker}/indicator-history", s.getIndicatorHistory)
	mux.HandleFunc("GET /v1/stocks/{ticker}/seasonality", s.getSeasonality)
	mux.HandleFunc("GET /v1/stocks/{ticker}/relative-strength", s.getRelativeStrength)
	mux.HandleFunc("GET /v1/stocks/{ticker}/earnings-reaction", s.getEarningsReaction)
	mux.HandleFunc("GET /v1/stocks/{ticker}/scorecard", s.getScorecard)
	mux.HandleFunc("GET /v1/stocks/{ticker}/dividend", s.getDividend)
	mux.HandleFunc("GET /v1/stocks/{ticker}/backtest", s.getBacktest)
	mux.HandleFunc("GET /v1/stocks/{ticker}/research", s.getResearch)
	mux.HandleFunc("POST /v1/stocks/{ticker}/chat", s.postChat)
	mux.HandleFunc("GET /v1/stocks/{ticker}/chat", s.getChatHistory)
	mux.HandleFunc("DELETE /v1/stocks/{ticker}/chat", s.deleteChat)
	mux.HandleFunc("GET /v1/conversations", s.getConversations)
	mux.HandleFunc("POST /v1/conversations", s.postConversation)
	mux.HandleFunc("PATCH /v1/conversations/{id}", s.patchConversation)
	mux.HandleFunc("DELETE /v1/conversations/{id}", s.deleteConversation)
	mux.HandleFunc("POST /v1/conversations/{id}/chat", s.postConvChat)
	mux.HandleFunc("POST /v1/conversations/{id}/chat/stream", s.postConvChatStream)
	mux.HandleFunc("GET /v1/chat/usage", s.getChatUsage)
	mux.HandleFunc("GET /v1/conversations/{id}/chat", s.getConvHistory)
	mux.HandleFunc("POST /v1/event", s.postEvent)       // first-party funnel events (public, fire-and-forget)
	mux.HandleFunc("GET /v1/admin/funnel", s.getFunnel) // funnel aggregate (admin-gated)
	mux.HandleFunc("GET /v1/stocks/{ticker}/movement", s.getMovement)
	mux.HandleFunc("GET /v1/stocks/{ticker}/material-events", s.getMaterialEvents)
	mux.HandleFunc("GET /v1/material-feed", s.getMaterialFeed)
	mux.HandleFunc("GET /v1/stocks/{ticker}/insider-activity", s.getInsiderActivity)
	mux.HandleFunc("GET /v1/indicators", s.getIndicators)
	mux.HandleFunc("GET /v1/stream", s.getStream)

	// auth.Middleware attaches the user when a valid bearer token is present;
	// the outer middleware adds CORS + logging.
	s.handler = s.middleware(verifier.Middleware(mux))
	return s
}

// CORSMiddleware sets permissive CORS headers and answers OPTIONS preflights with
// 204. It MUST be wired as the OUTERMOST middleware — ahead of the rate limiter,
// auth, and the mux — so that EVERY response (incl. a rate-limiter 429, a 401, or a
// panic) carries Access-Control-Allow-Origin. A 429 WITHOUT it surfaces in the
// browser as a misleading "No 'Access-Control-Allow-Origin' header" CORS error that
// blanks the page (the root cause of the intermittent blank-page reports: a bursty
// page load trips the limiter and the ACAO-less 429 reads as a CORS failure).
// Short-circuiting preflights here also keeps OPTIONS from being counted/throttled by
// the limiter, so a preflight is never the rejected request. ACAO is "*", which (by
// spec) cannot be combined with credentials — fine here since auth uses a Bearer
// header, not cookies.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// middleware logs each request. CORS now lives in the outermost CORSMiddleware (so
// rate-limited / 4xx / panic responses still carry CORS headers); this remains for
// request logging only.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).String())
	})
}

// requireUser returns the authenticated user, or writes 401 and returns false.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errBody("login required"))
		return auth.User{}, false
	}
	return u, true
}

// The two entitlement tiers the gates branch on.
const (
	tierPro  = "pro"
	tierFree = "free"
)

// tierOf returns the user's entitlement tier ("pro" | "free"), derived live from the
// Stripe-synced subscription row (active/trialing → pro; past_due keeps Pro through a
// short renewal grace). It SWALLOWS a store error and an unknown user to "free" — so a
// DB hiccup or an unconfigured-Stripe deployment never wrongly grants Pro and never
// hard-locks anyone: the viewing/quota gates then show the free experience (fail-open)
// while a Pro-only feature denies (fail-closed), both correct from this single
// "free on error" default. With Stripe unconfigured no subscription rows exist, so
// every user resolves to "free" — exactly the pre-paywall status quo.
func (s *Server) tierOf(ctx context.Context, userID string) string {
	if userID == "" {
		return tierFree
	}
	sub, ok, err := s.store.GetSubscription(ctx, userID)
	if err != nil {
		s.log.Debug("tierOf: subscription read failed (defaulting free)", "user", userID, "err", err)
		return tierFree
	}
	if !ok {
		return tierFree
	}
	switch sub.Status {
	case "active", "trialing":
		return tierPro
	case "past_due": // renewal grace: still Pro until shortly past the period end
		if time.Now().Before(sub.CurrentPeriodEnd.Add(72 * time.Hour)) {
			return tierPro
		}
		return tierFree
	default:
		return tierFree
	}
}

// health is a readiness probe: it pings the store and reports subsystem status,
// returning 503 when a dependency (the DB) is unreachable so uptime monitors
// actually catch outages instead of seeing a flat "ok".
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	db, status, code := "ok", "ok", http.StatusOK
	if err := s.store.Ping(ctx); err != nil {
		db, status, code = "down", "degraded", http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":  status,
		"service": "tickwind",
		"db":      db,
		"llm":     s.enrich != nil && s.enrich.Enabled(),
		"prices":  s.bars != nil,
		"options": s.options != nil,
		"13f":     s.thirteenf != nil,
	})
}

// ── Per-user: watchlist ──────────────────────────────────────────────────

func (s *Server) getWatchlist(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	s.writeWatchlist(w, r, u.ID, http.StatusOK)
}

func (s *Server) postWatchlist(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Ticker string `json:"ticker"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" {
		writeJSON(w, http.StatusBadRequest, errBody("a ticker is required"))
		return
	}
	if err := s.store.AddToWatchlist(r.Context(), u.ID, ticker); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	// Populate the new ticker right away (filings/news/social) instead of waiting
	// for the next scheduler cycle. Detached context — the request's is cancelled
	// once we respond — and fire-and-forget so the response isn't blocked.
	if s.ingestor != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			s.ingestor.IngestOne(ctx, ticker)
		}()
	}
	s.writeWatchlist(w, r, u.ID, http.StatusCreated)
}

func (s *Server) deleteWatchlist(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if err := s.store.RemoveFromWatchlist(r.Context(), u.ID, ticker); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	s.writeWatchlist(w, r, u.ID, http.StatusOK)
}

func (s *Server) writeWatchlist(w http.ResponseWriter, r *http.Request, userID string, code int) {
	tickers, err := s.store.Watchlist(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if tickers == nil {
		tickers = []string{}
	}
	writeJSON(w, code, map[string]any{"tickers": tickers})
}

// ── Per-user: clips (saved links) ────────────────────────────────────────

func (s *Server) postClip(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	link := strings.TrimSpace(req.URL)
	if link == "" {
		writeJSON(w, http.StatusBadRequest, errBody("a url is required"))
		return
	}

	title, err := s.clip.Title(r.Context(), link)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}

	// Dedupe per (user, url); distinct across users.
	h := fnv.New64a()
	_, _ = h.Write([]byte(u.ID + "\x00" + link))
	c := store.Clip{
		ID:        fmt.Sprintf("clip:%x", h.Sum64()),
		UserID:    u.ID,
		Ticker:    ticker,
		Title:     title,
		URL:       link,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveClip(r.Context(), c); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getClips(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	ticker := r.PathValue("ticker")
	clips, err := s.store.ListClips(r.Context(), u.ID, ticker, queryLimit(r, 50))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if clips == nil {
		clips = []store.Clip{} // marshal as [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticker": ticker,
		"count":  len(clips),
		"clips":  clips,
	})
}

// ── Per-user: notes ──────────────────────────────────────────────────────

// randNoteID returns a random "note:<hex>" id (notes aren't deduped like clips —
// a user may legitimately write two identical lines, so no content hash).
func randNoteID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "note:" + hex.EncodeToString(b[:])
}

func (s *Server) postNote(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Ticker string `json:"ticker"`
		Date   string `json:"note_date"`
		Body   string `json:"body"`
		Pinned bool   `json:"pinned"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeJSON(w, http.StatusBadRequest, errBody("a note body is required"))
		return
	}
	date := strings.TrimSpace(req.Date)
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody("note_date must be YYYY-MM-DD"))
			return
		}
	}
	now := time.Now().UTC()
	n := store.Note{
		ID:        randNoteID(),
		UserID:    u.ID,
		Ticker:    strings.ToUpper(strings.TrimSpace(req.Ticker)),
		Date:      date,
		Body:      body,
		Pinned:    req.Pinned,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.SaveNote(r.Context(), n); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) getNotes(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	notes, err := s.store.ListNotes(r.Context(), store.NoteFilter{
		UserID: u.ID,
		Ticker: strings.ToUpper(strings.TrimSpace(q.Get("ticker"))),
		From:   strings.TrimSpace(q.Get("from")),
		To:     strings.TrimSpace(q.Get("to")),
		Limit:  queryLimit(r, 200),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if notes == nil {
		notes = []store.Note{} // marshal as [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(notes), "notes": notes})
}

func (s *Server) patchNote(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Body   *string `json:"body"`
		Pinned *bool   `json:"pinned"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	if req.Body != nil {
		b := strings.TrimSpace(*req.Body)
		if b == "" {
			writeJSON(w, http.StatusBadRequest, errBody("note body cannot be empty"))
			return
		}
		req.Body = &b
	}
	n, ok2, err := s.store.UpdateNote(r.Context(), u.ID, r.PathValue("id"), req.Body, req.Pinned)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok2 {
		writeJSON(w, http.StatusNotFound, errBody("note not found"))
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	deleted, err := s.store.DeleteNote(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, errBody("note not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// ── Per-user: alerts ─────────────────────────────────────────────────────

// validAlertKinds gates the alert types the evaluator understands. The price/filing
// kinds use Threshold; the signal-condition kinds (golden_cross … signal_bearish) are
// self-describing and ignore Threshold (gated via ingest.IsSignalAlertKind below).
var validAlertKinds = map[string]bool{
	"price_above": true, "price_below": true, "pct_move": true, "new_filing": true,
	"earnings_soon": true,
	"golden_cross":  true, "death_cross": true, "rsi_oversold": true,
	"rsi_overbought": true, "signal_bullish": true, "signal_bearish": true,
}

func (s *Server) postAlert(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Ticker    string  `json:"ticker"`
		Kind      string  `json:"kind"`
		Threshold float64 `json:"threshold"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" {
		writeJSON(w, http.StatusBadRequest, errBody("a ticker is required"))
		return
	}
	if !validAlertKinds[req.Kind] {
		writeJSON(w, http.StatusBadRequest, errBody("invalid alert kind"))
		return
	}
	// Threshold is required only for the price kinds; new_filing, earnings_soon, and the
	// signal-condition kinds are self-describing (earnings_soon defaults its lead window) and
	// treat an absent threshold as 0.
	if req.Kind != "new_filing" && req.Kind != "earnings_soon" && !ingest.IsSignalAlertKind(req.Kind) && req.Threshold <= 0 {
		writeJSON(w, http.StatusBadRequest, errBody("threshold must be positive"))
		return
	}
	a := store.Alert{
		ID:        randHex(),
		UserID:    u.ID,
		Ticker:    ticker,
		Kind:      req.Kind,
		Threshold: req.Threshold,
		Active:    true,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveAlert(r.Context(), a); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) getAlerts(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	alerts, err := s.store.ListAlerts(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if alerts == nil {
		alerts = []store.Alert{} // marshal as [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(alerts), "alerts": alerts})
}

func (s *Server) deleteAlert(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	deleted, err := s.store.DeleteAlert(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, errBody("alert not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// reactivateAlert re-arms a triggered alert (active again, trigger cleared) so a
// one-shot alert can be reused without recreating it. 404 if not the user's.
func (s *Server) reactivateAlert(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	ok2, err := s.store.ReactivateAlert(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok2 {
		writeJSON(w, http.StatusNotFound, errBody("alert not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reactivated": true})
}

// ── Per-user: holdings ───────────────────────────────────────────────────

func (s *Server) postHolding(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Ticker  string  `json:"ticker"`
		Shares  float64 `json:"shares"`
		AvgCost float64 `json:"avg_cost"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" {
		writeJSON(w, http.StatusBadRequest, errBody("a ticker is required"))
		return
	}
	if req.Shares <= 0 {
		writeJSON(w, http.StatusBadRequest, errBody("shares must be positive"))
		return
	}
	if req.AvgCost < 0 {
		writeJSON(w, http.StatusBadRequest, errBody("avg_cost cannot be negative"))
		return
	}
	now := time.Now().UTC()
	h := store.Holding{
		ID:        randHex(),
		UserID:    u.ID,
		Ticker:    ticker,
		Shares:    req.Shares,
		AvgCost:   req.AvgCost,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.SaveHolding(r.Context(), h); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) getHoldings(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	holdings, err := s.store.ListHoldings(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if holdings == nil {
		holdings = []store.Holding{} // marshal as [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(holdings), "holdings": holdings})
}

func (s *Server) deleteHolding(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	deleted, err := s.store.DeleteHolding(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, errBody("holding not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// ── Per-user: prefs (generic JSON UI-state blob) ─────────────────────────
//
// A small, generic per-user JSON-prefs surface (selected indicators today,
// future view prefs under sibling keys). The blob is opaque to the store; the
// API owns the shape ({"indicators":{"ids":[...]}}) and caps the size so it
// can't be abused as arbitrary storage. Routed to the cheap-to-rebuild User
// store via Split — same class as watchlist/notes/alerts.

// maxPrefsBytes caps an uploaded prefs blob (a tiny id list, well under 8 KB).
const maxPrefsBytes = 8 << 10

// getMyPrefs returns the caller's stored prefs blob, or 200 {} when they have
// none (the client then falls back to localStorage / the default, so nothing
// regresses). 401 without a token.
func (s *Server) getMyPrefs(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	blob, found, err := s.store.GetPrefs(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if !found || len(blob) == 0 {
		_, _ = w.Write([]byte("{}")) // empty object → client uses its default
		return
	}
	_, _ = w.Write(blob) // the stored blob is already valid JSON
}

// putMyPrefs shallow-merges the posted top-level keys into the caller's stored
// blob, then persists it (204). Merging in the handler keeps the client trivial
// and ensures a PUT that only sets `indicators` never clobbers a future sibling
// pref key. The body must be a JSON object and is capped at maxPrefsBytes.
func (s *Server) putMyPrefs(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPrefsBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	if len(body) > maxPrefsBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errBody("prefs too large"))
		return
	}
	// Reject anything that is not a JSON object (arrays, strings, numbers, null).
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(body, &incoming); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("prefs must be a JSON object"))
		return
	}
	// Shallow-merge: load the existing blob, overlay the posted top-level keys,
	// re-marshal. A missing/empty stored blob starts from {}.
	merged := map[string]json.RawMessage{}
	if existing, found, err := s.store.GetPrefs(r.Context(), u.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	} else if found && len(existing) > 0 {
		// A previously stored blob is always a JSON object; ignore a decode error
		// defensively (treat a corrupt blob as empty rather than 500).
		_ = json.Unmarshal(existing, &merged)
	}
	for k, v := range incoming {
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if len(out) > maxPrefsBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errBody("prefs too large"))
		return
	}
	if err := s.store.PutPrefs(r.Context(), u.ID, out); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Comments (PUBLIC read; authenticated write) ──────────────────────────
//
// Comments are a §230-style neutral-host feature: users post opinions, we host
// them. Safeguards here: auth-gated posting, per-user rate-limiting (anti-spam),
// author/IP/timestamp captured for moderation, soft-delete (author or admin) and
// a report endpoint. The "not investment advice" disclaimer + ToS live in the UI.

func (s *Server) getComments(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("ticker")))
	viewer := "" // public endpoint: include per-user liked state when a token is present
	if u, ok := auth.UserFrom(r.Context()); ok {
		viewer = u.ID
	}
	comments, err := s.store.ListComments(r.Context(), ticker, queryLimit(r, 100), viewer)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if comments == nil {
		comments = []store.Comment{} // marshal as [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticker":   ticker,
		"count":    len(comments),
		"comments": comments,
	})
}

func (s *Server) postComment(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if !s.commentRL.allow(u.ID) {
		writeJSON(w, http.StatusTooManyRequests, errBody("you're posting too fast — please wait a moment"))
		return
	}
	var req struct {
		Ticker string `json:"ticker"`
		Body   string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeJSON(w, http.StatusBadRequest, errBody("a comment body is required"))
		return
	}
	if len([]rune(body)) > 2000 {
		writeJSON(w, http.StatusBadRequest, errBody("comment too long (2000 chars max)"))
		return
	}
	c := store.Comment{
		ID:        "cmt:" + randHex(),
		UserID:    u.ID,
		Author:    authorName(u.Email),
		Ticker:    strings.ToUpper(strings.TrimSpace(req.Ticker)),
		Body:      body,
		CreatedAt: time.Now().UTC(),
		Mentions:  cashtag.Extract(body), // $TICKER fan-out to mentioned stocks
		IP:        clientIP(r),
	}
	if err := s.store.SaveComment(r.Context(), c); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// isAdmin reports whether u is on the admin allowlist (ADMIN_USER_IDS), matched
// by Supabase UUID or by email (case-insensitive) — so an operator can list
// either form (e.g. just their login email).
func (s *Server) isAdmin(u auth.User) bool {
	if len(s.admins) == 0 {
		return false
	}
	if u.ID != "" && s.admins[strings.ToLower(u.ID)] {
		return true
	}
	return u.Email != "" && s.admins[strings.ToLower(u.Email)]
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	deleted, err := s.store.DeleteComment(r.Context(), r.PathValue("id"), u.ID, s.isAdmin(u))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, errBody("comment not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// patchComment edits the caller's own comment (body only). Same validation as
// posting; the store enforces author-only editing → 404 if not found or not the
// author's.
func (s *Server) patchComment(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeJSON(w, http.StatusBadRequest, errBody("a comment body is required"))
		return
	}
	if len([]rune(body)) > 2000 {
		writeJSON(w, http.StatusBadRequest, errBody("comment too long (2000 chars max)"))
		return
	}
	c, ok2, err := s.store.UpdateComment(r.Context(), r.PathValue("id"), u.ID, body, cashtag.Extract(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok2 {
		writeJSON(w, http.StatusNotFound, errBody("comment not found"))
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// likeComment toggles the caller's like on a comment, returning the new state +
// total count. 404 if the comment doesn't exist (or is deleted).
func (s *Server) likeComment(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	liked, likes, ok2, err := s.store.LikeComment(r.Context(), r.PathValue("id"), u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok2 {
		writeJSON(w, http.StatusNotFound, errBody("comment not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"liked": liked, "likes": likes})
}

func (s *Server) reportComment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}
	reported, err := s.store.ReportComment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !reported {
		writeJSON(w, http.StatusNotFound, errBody("comment not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reported": true})
}

// randHex returns 16 random bytes hex-encoded, for entity ids.
func randHex() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// authorName derives a public display handle from an email (local-part), with a
// neutral fallback. (We never expose the full email or the user id publicly.)
func authorName(email string) string {
	email = strings.TrimSpace(email)
	if i := strings.IndexByte(email, '@'); i > 0 {
		return email[:i]
	}
	if email != "" {
		return email
	}
	return "anon"
}

// clientIP is the best-effort client IP for moderation (Cloudflare / X-Forwarded-For aware).
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rateLimiter is a simple per-key sliding-window limiter (anti-spam).
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: make(map[string][]time.Time), max: max, window: window}
}

// allow records a hit for key and reports whether it's within the limit.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-rl.window)
	kept := rl.hits[key][:0]
	for _, t := range rl.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.hits[key] = kept
		return false
	}
	rl.hits[key] = append(kept, time.Now())
	return true
}

// ── Public: market data ──────────────────────────────────────────────────

func (s *Server) getStock(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	sec, ok, err := s.store.GetSecurity(r.Context(), ticker)
	switch {
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
	case !ok:
		s.maybeCollect(ticker) // first-time visit of a real symbol → kick off collection
		writeJSON(w, http.StatusNotFound, errBody("not tracked yet: "+ticker))
	default:
		writeJSON(w, http.StatusOK, sec)
	}
}

// maybeCollect fires a one-shot on-demand collection for an untracked but REAL
// symbol, so a first-time visit populates itself instead of showing an empty page
// forever (the bug where $MU stayed blank: nothing ever triggered its collection).
// Safe to call on every 404: it no-ops unless the ticker is in the symbol
// directory (so scraped/garbage tickers do no work), and the ingestor
// single-flights per ticker (repeated polls while collecting don't duplicate it).
func (s *Server) maybeCollect(ticker string) {
	if s.ingestor == nil || s.symbols == nil {
		return
	}
	tk := strings.ToUpper(strings.TrimSpace(ticker))
	if tk == "" {
		return
	}
	if hits := s.symbols.Search(tk, 1); len(hits) == 0 || strings.ToUpper(hits[0].Ticker) != tk {
		return // not a known symbol — don't trigger collection
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		s.ingestor.IngestOne(ctx, tk)
	}()
}

func (s *Server) getFilings(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	filings, err := s.store.ListFilings(r.Context(), ticker, queryLimit(r, 25))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticker":  ticker,
		"count":   len(filings),
		"filings": filings,
	})
}

// FundamentalsSource returns XBRL-derived fundamentals for a US ticker (cached).
type FundamentalsSource interface {
	Fundamentals(ctx context.Context, ticker string) (edgar.Fundamentals, error)
}

// ETFHoldingsSource returns a fund/ETF's largest positions from its latest SEC Form N-PORT-P,
// on-demand (the edgar client self-throttles). nil → /v1/etf/{ticker}/holdings 404s. Satisfied by
// *edgar.Client.
type ETFHoldingsSource interface {
	ETFHoldings(ctx context.Context, ticker string, max int) ([]edgar.ETFHolding, time.Time, error)
}

// SetETFHoldings wires the ETF/fund holdings source (post-New; nil-safe → endpoint 404s).
func (s *Server) SetETFHoldings(src ETFHoldingsSource) { s.etf = src }

// etfHoldingsResp is the /v1/etf/{ticker}/holdings payload: the fund's top positions by weight,
// each Go-parsed verbatim from the N-PORT-P filing, plus the filing date.
type etfHoldingsResp struct {
	Ticker   string             `json:"ticker"`
	AsOf     time.Time          `json:"as_of"`
	Count    int                `json:"count"`
	Holdings []edgar.ETFHolding `json:"holdings"`
	// NoFiling is true for a KNOWN ETF/fund that has no SEC N-PORT holdings filing yet (e.g. a
	// brand-new fund SEC hasn't mapped). The frontend shows a brief "holdings appear after the
	// fund's first SEC filing" note instead of hiding the panel (a clean state, not a 502).
	NoFiling bool `json:"no_filing,omitempty"`
}

// getETFHoldings serves a fund/ETF's largest disclosed positions (SEC Form N-PORT-P). Facts only —
// every figure is parsed verbatim from the filing; no LLM, no advice. 404 for a non-ETF ticker or a
// fund with no holdings filing; 502 on an upstream SEC failure.
func (s *Server) getETFHoldings(w http.ResponseWriter, r *http.Request) {
	if s.etf == nil {
		writeJSON(w, http.StatusNotFound, errBody("etf holdings unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	// Cheap pre-check: only funds/ETFs file N-PORT. If the symbol directory KNOWS this exact ticker
	// and it is not an ETF, skip the SEC round-trip. A ticker the directory KNOWS is an ETF sets
	// knownETF, so a "no filing yet" outcome returns a clean note (not a 404/502).
	knownETF := false
	if s.symbols != nil {
		for _, h := range s.symbols.Search(ticker, 5) {
			if strings.ToUpper(h.Ticker) == ticker {
				if !h.ETF {
					writeJSON(w, http.StatusNotFound, errBody(ticker+" is not an ETF"))
					return
				}
				knownETF = true
				break
			}
		}
	}
	limit := queryLimit(r, 25)
	if limit > 200 {
		limit = 200
	}
	holdings, asOf, err := s.etf.ETFHoldings(r.Context(), ticker, limit)
	if err != nil {
		// No N-PORT on file, or the ticker isn't in SEC's directory yet (a brand-new ETF like
		// DRAM): nothing to show. For a KNOWN ETF, return a clean 200 no_filing note so the panel
		// can say "holdings appear after the first SEC filing" instead of erroring (a 502).
		if errors.Is(err, edgar.ErrNoNPORT) || errors.Is(err, edgar.ErrTickerNotFound) {
			if knownETF {
				writeJSON(w, http.StatusOK, etfHoldingsResp{Ticker: ticker, Holdings: []edgar.ETFHolding{}, NoFiling: true})
				return
			}
			writeJSON(w, http.StatusNotFound, errBody("no holdings filing for "+ticker))
			return
		}
		writeJSON(w, http.StatusBadGateway, errBody("holdings fetch failed for "+ticker))
		return
	}
	writeJSON(w, http.StatusOK, etfHoldingsResp{Ticker: ticker, AsOf: asOf, Count: len(holdings), Holdings: holdings})
}

// fundamentalsResp embeds the reported XBRL figures and adds the price-derived
// metrics, which are null when not computable (e.g. P/E for a loss-maker).
type fundamentalsResp struct {
	edgar.Fundamentals
	Price     float64  `json:"price"`
	MarketCap *float64 `json:"market_cap"`
	PE        *float64 `json:"pe"`         // static P/E: price ÷ latest-FY diluted EPS
	PETTM     *float64 `json:"pe_ttm"`     // trailing P/E: price ÷ trailing-12-month diluted EPS
	PEForward *float64 `json:"pe_forward"` // run-rate forward P/E: price ÷ (latest standalone quarter EPS × 4)
	PB        *float64 `json:"pb"`
	// DividendYield is the trailing cash-dividend yield (latest-FY common dividends ÷
	// market cap); null for non-payers. A real-data trailing figure, NOT a forward estimate.
	DividendYield *float64 `json:"dividend_yield"`
}

// maxPlausiblePE caps the P/E ratios served: a near-zero EPS yields a 4-digit "P/E" that is
// numerical noise, not a valuation — above this ceiling the figure is suppressed (null), so a
// trough-earnings artifact never reads as an authoritative multiple.
const maxPlausiblePE = 600

// plausiblePE returns price ÷ eps as a P/E pointer, or nil when eps is non-positive (a loss,
// where P/E is meaningless) or the resulting multiple exceeds maxPlausiblePE (near-zero EPS).
func plausiblePE(price, eps float64) *float64 {
	if eps <= 0 {
		return nil
	}
	if pe := price / eps; pe <= maxPlausiblePE {
		return &pe
	}
	return nil
}

// getFundamentals serves market cap + P/E + P/B (price-derived) alongside the
// reported revenue / net income / EPS / shares from SEC XBRL. 404s for
// non-US/unknown tickers or when no XBRL data exists, so the frontend hides the
// card. Market data is free public-domain SEC data.
func (s *Server) getFundamentals(w http.ResponseWriter, r *http.Request) {
	if s.fundamentals == nil {
		writeJSON(w, http.StatusNotFound, errBody("fundamentals unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	f, err := s.fundamentals.Fundamentals(r.Context(), ticker)
	if err != nil || !f.HasData() {
		writeJSON(w, http.StatusNotFound, errBody("no fundamentals for "+ticker))
		return
	}

	resp := fundamentalsResp{Fundamentals: f}
	// Price: the polled quote first, else an on-demand fetch (mirrors getQuote).
	if q, ok, _ := s.store.GetQuote(r.Context(), ticker); ok && q.Price > 0 {
		resp.Price = q.Price
	} else if s.bars != nil {
		if oq, found, qerr := s.bars.LatestQuote(r.Context(), ticker); qerr == nil && found {
			resp.Price = oq.Price
		}
	}
	if resp.Price > 0 {
		if f.Shares > 0 {
			mc := resp.Price * float64(f.Shares)
			resp.MarketCap = &mc
		}
		resp.PE = plausiblePE(resp.Price, f.EPSDiluted)                   // static P/E (latest FY)
		resp.PETTM = plausiblePE(resp.Price, f.EPSDilutedTTM)             // trailing-12-month P/E
		resp.PEForward = plausiblePE(resp.Price, f.EPSDilutedQuarterly*4) // run-rate (latest quarter annualized)
		if f.Equity > 0 && f.Shares > 0 {
			if bvps := f.Equity / float64(f.Shares); bvps > 0 {
				pb := resp.Price / bvps
				resp.PB = &pb
			}
		}
		// Trailing dividend yield — uses COMMON-only dividends (the general concept includes
		// preferred for some issuers, which would overstate the common yield); non-payers and
		// filers that report only the general concept → null (insufficient-not-wrong).
		if resp.MarketCap != nil && f.CommonDividendsPaid > 0 {
			dy := f.CommonDividendsPaid / *resp.MarketCap
			resp.DividendYield = &dy
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getQuote(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	q, ok, err := s.store.GetQuote(r.Context(), ticker)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	// Yahoo was removed (commercial-use risk): never serve a lingering Yahoo-sourced
	// quote from the store. Treat it as absent so the on-demand path below re-resolves
	// a US ticker to a fresh Alpaca quote; an HK name (no Alpaca data) falls to "—".
	if ok && q.Source == "yahoo" {
		ok = false
	}
	// Refresh on demand when the store has nothing (a stock the user just
	// navigated to) OR when the stored quote's last trade is stale — thin names
	// can sit unrefreshed; BarCache also overlays a consolidated-tape fallback
	// for symbols with no recent IEX print. Errors degrade to what we have.
	if s.bars != nil && (!ok || time.Since(q.At) > quoteStaleAfter) {
		if oq, found, qerr := s.bars.LatestQuote(r.Context(), ticker); qerr == nil && found {
			if !ok || oq.At.After(q.At) {
				q, ok = oq, true
			}
		}
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("no quote yet: "+ticker))
		return
	}
	writeJSON(w, http.StatusOK, q)
}

// subscribeLive nudges the real-time WS streamer to subscribe a ticker the user
// just opened, so its price updates live (within the free-tier cap, LRU-evicted).
// Fire-and-forget; always 200 (no-op when streaming is disabled). Public — it only
// influences the live-stream subscription set.
func (s *Server) subscribeLive(w http.ResponseWriter, r *http.Request) {
	if s.live != nil {
		s.live.Subscribe(strings.ToUpper(strings.TrimSpace(r.PathValue("ticker"))))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// subscribeLiveBatch nudges the WS streamer to subscribe a BATCH of tickers a list
// view is showing (home Markets / watchlist / overview), so the visible set updates
// live (within the free-tier cap, LRU-evicted) instead of only on the REST poller.
// Body: {"tickers":[...]}. Fire-and-forget; always 200; capped to bound churn;
// no-op when streaming is disabled. Public — it only influences the live-stream
// subscription set, never any data.
func (s *Server) subscribeLiveBatch(w http.ResponseWriter, r *http.Request) {
	if s.live != nil {
		var body struct {
			Tickers []string `json:"tickers"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err == nil {
			const maxBatch = 30
			for i, t := range body.Tickers {
				if i >= maxBatch {
					break
				}
				s.live.Subscribe(strings.ToUpper(strings.TrimSpace(t)))
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// getBars returns recent daily closing prices for a sparkline. It degrades
// gracefully to an empty series (HTTP 200) when bars are unavailable, so the
// frontend simply renders nothing rather than erroring.
func (s *Server) getBars(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	closes := []float64{}
	resp := map[string]any{"ticker": ticker}
	if s.bars != nil {
		if got, err := s.bars.DailyBars(r.Context(), ticker); err != nil {
			s.log.Debug("bars fetch failed", "ticker", ticker, "err", err)
		} else if got != nil {
			closes = got
		}
		// 52-week high/low from the daily candle cache (same cache the K-line uses;
		// a hit is cheap). Omitted when unavailable so the frontend hides the range.
		if cs, err := s.bars.DailyCandles(r.Context(), ticker); err == nil {
			if hi, lo := yearHighLow(cs); hi > 0 && lo > 0 {
				resp["year_high"] = hi
				resp["year_low"] = lo
			}
		}
	}
	resp["closes"] = closes
	writeJSON(w, http.StatusOK, resp)
}

// yearHighLow returns the highest High and lowest Low over the last ~252 trading
// days (≈52 weeks) of daily candles. Returns 0,0 for an empty series or all-zero
// data (so the caller can omit the range rather than show a fake 0).
func yearHighLow(candles []store.Candle) (high, low float64) {
	start := 0
	if len(candles) > 252 {
		start = len(candles) - 252
	}
	for i := start; i < len(candles); i++ {
		if h := candles[i].High; h > high {
			high = h
		}
		if l := candles[i].Low; l > 0 && (low == 0 || l < low) {
			low = l
		}
	}
	return high, low
}

// getCandles returns daily OHLC candles for the K-line chart. Degrades to an
// empty series (HTTP 200) when bars are unavailable.
func (s *Server) getCandles(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	resolution := r.URL.Query().Get("resolution")
	candles := []store.Candle{}
	if s.bars != nil {
		var got []store.Candle
		var err error
		switch resolution {
		case "5Min", "15Min", "1Hour":
			got, err = s.bars.IntradayCandles(r.Context(), ticker, resolution)
		default: // "", "1Day", or unknown → daily (backward-compatible)
			got, err = s.bars.DailyCandles(r.Context(), ticker)
		}
		if err != nil {
			s.log.Debug("candles fetch failed", "ticker", ticker, "resolution", resolution, "err", err)
		} else if got != nil {
			candles = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "candles": candles})
}

// getUniverse reports the universe price-cache status (count of pre-cached
// tickers + last refresh); its per-stock data powers the screener. nil → count 0.
func (s *Server) getUniverse(w http.ResponseWriter, r *http.Request) {
	if s.universe == nil {
		writeJSON(w, http.StatusOK, map[string]any{"count": 0})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":      s.universe.Len(),
		"updated_at": s.universe.UpdatedAt(),
	})
}

// getUniverseSymbols enumerates every quote-bearing US ticker (~6,695) — the
// symbols the universe sweep has a usable price for (matching /v1/universe's
// count + the /v1/screen source). This is the pSEO /stock universe: each name
// has real content (live price + indicators + 52w range). Distinct from
// /v1/symbols, which lists the full SEC+Nasdaq directory (~16,118, ~9,400 of
// them quote-less/thin and excluded here). Always 200 with a non-nil list —
// empty when the cache is unswept/nil. ?limit= caps the slice (the sitemap
// requests the full list with no limit). Tickers are sorted; dotted names like
// BRK.B pass through verbatim. The set changes ~daily, so it's cacheable.
func (s *Server) getUniverseSymbols(w http.ResponseWriter, r *http.Request) {
	var tickers []string
	if s.universe != nil {
		tickers = s.universe.Tickers()
	}
	if tickers == nil {
		tickers = []string{}
	}
	if lim := queryLimit(r, 0); lim > 0 && len(tickers) > lim {
		tickers = tickers[:lim]
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{"symbols": tickers, "count": len(tickers)})
}

// screenCriteria captures the /v1/screen filters. Price bounds of 0 mean
// unbounded; change bounds use explicit has* flags (0% is a valid bound, and a
// change filter excludes rows whose change can't be computed).
type screenCriteria struct {
	minPrice, maxPrice   float64
	minChange, maxChange float64
	hasMinChange         bool
	hasMaxChange         bool
	session              string
	sort                 string
	limit                int
}

// screenResult is one matched stock in a screener response.
type screenResult struct {
	Ticker    string   `json:"ticker"`
	Price     float64  `json:"price"`
	PrevClose float64  `json:"prev_close,omitempty"`
	ChangePct *float64 `json:"change_pct"` // null when prev close is unknown
	Session   string   `json:"session"`
}

const (
	// A computed daily %-change outside this band is treated as a data artifact
	// (typically a reverse-split prev_close mismatch in delayed IEX data — e.g. a
	// $43 stock showing a $1 prev close → +4000%) and the change is marked unknown
	// rather than served as a bogus top gainer. A stock can't fall more than 100%,
	// and a genuine one-day listed-equity gain above ~300% is vanishingly rare.
	maxSaneChangePct = 300.0
	minSaneChangePct = -95.0
)

// screenQuotes filters a universe snapshot by the criteria, then sorts + caps it.
// Pure (no I/O) so it is directly unit-tested.
// guardedChangePct is the day-change % (price vs prev close), or nil when prev
// close is unknown or the move is implausibly large — a delayed-data reverse-
// split artifact. Shared by the screener and the hot boards.
func guardedChangePct(price, prevClose float64) *float64 {
	if prevClose <= 0 {
		return nil
	}
	v := (price - prevClose) / prevClose * 100
	if v > maxSaneChangePct || v < minSaneChangePct {
		return nil
	}
	return &v
}

func screenQuotes(quotes map[string]store.Quote, c screenCriteria) []screenResult {
	out := make([]screenResult, 0)
	for tk, q := range quotes {
		if q.Price <= 0 {
			continue // no usable price
		}
		if c.minPrice > 0 && q.Price < c.minPrice {
			continue
		}
		if c.maxPrice > 0 && q.Price > c.maxPrice {
			continue
		}
		chg := guardedChangePct(q.Price, q.PrevClose)
		if c.hasMinChange && (chg == nil || *chg < c.minChange) {
			continue
		}
		if c.hasMaxChange && (chg == nil || *chg > c.maxChange) {
			continue
		}
		if c.session != "" && !strings.EqualFold(q.Session, c.session) {
			continue
		}
		out = append(out, screenResult{Ticker: tk, Price: q.Price, PrevClose: q.PrevClose, ChangePct: chg, Session: q.Session})
	}
	sortScreen(out, c.sort)
	if c.limit > 0 && len(out) > c.limit {
		out = out[:c.limit]
	}
	return out
}

// cmpChange orders by change%, with rows lacking a change (nil) always sorted
// last and ties broken by ticker for stable output.
func cmpChange(a, b screenResult, desc bool) bool {
	an, bn := a.ChangePct == nil, b.ChangePct == nil
	if an || bn {
		if an != bn {
			return bn // a non-nil sorts before a nil
		}
		return a.Ticker < b.Ticker
	}
	if desc {
		return *a.ChangePct > *b.ChangePct
	}
	return *a.ChangePct < *b.ChangePct
}

func sortScreen(rows []screenResult, mode string) {
	switch mode {
	case "price_desc":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Price > rows[j].Price })
	case "price_asc":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Price < rows[j].Price })
	case "change_asc":
		sort.SliceStable(rows, func(i, j int) bool { return cmpChange(rows[i], rows[j], false) })
	default: // change_desc (also the empty-string default)
		sort.SliceStable(rows, func(i, j int) bool { return cmpChange(rows[i], rows[j], true) })
	}
}

// parseFloat parses a query float, reporting ok=false for blank/invalid input.
func parseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// getScreen filters the whole-US universe quote cache by price / daily %-change /
// session and returns the (sorted, capped) matches. Always 200 with a (possibly
// empty) list — never null. nil universe → empty. Market-cap/volume filters are a
// later enhancement (need a shares cache). Quotes are delayed (Alpaca IEX).
func (s *Server) getScreen(w http.ResponseWriter, r *http.Request) {
	results := []screenResult{}
	if s.universe != nil {
		q := r.URL.Query()
		c := screenCriteria{
			session: strings.ToLower(strings.TrimSpace(q.Get("session"))),
			sort:    strings.TrimSpace(q.Get("sort")),
			limit:   queryLimit(r, 50),
		}
		if v, ok := parseFloat(q.Get("min_price")); ok {
			c.minPrice = v
		}
		if v, ok := parseFloat(q.Get("max_price")); ok {
			c.maxPrice = v
		}
		if v, ok := parseFloat(q.Get("min_change")); ok {
			c.minChange, c.hasMinChange = v, true
		}
		if v, ok := parseFloat(q.Get("max_change")); ok {
			c.maxChange, c.hasMaxChange = v, true
		}
		if c.limit > 200 {
			c.limit = 200
		}
		results = screenQuotes(s.universe.Snapshot(), c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(results), "results": results})
}

// maxBarsBatch caps how many tickers one batched request (bars/news/social)
// will resolve.
const maxBarsBatch = 30

// maxQuotesBatch caps how many tickers one batched /v1/quotes request resolves.
// Higher than maxBarsBatch because a list/zone view can show ~40 stocks and these
// are cheap store reads (the on-demand fallback only fires for the few misses); the
// client chunks anything larger into multiple batches.
const maxQuotesBatch = 50

// quoteStaleAfter: a stored quote whose last trade is older than this gets an
// on-demand refresh (which can also engage the consolidated-tape fallback).
const quoteStaleAfter = 5 * time.Minute

// queryTickers reads the comma-separated `tickers` query param, uppercased,
// deduped, and capped at max.
func queryTickers(r *http.Request, max int) []string {
	raw := strings.TrimSpace(r.URL.Query().Get("tickers"))
	if raw == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= max {
			break
		}
	}
	return out
}

// getBarsBatch returns daily-close series for multiple tickers in one request
// (board sparklines), fetched concurrently via the cache. Missing/empty series
// are omitted, so the response is always 200 with a (possibly partial) map.
func (s *Server) getBarsBatch(w http.ResponseWriter, r *http.Request) {
	result := map[string][]float64{}
	list := queryTickers(r, maxBarsBatch)
	if s.bars != nil && len(list) > 0 {
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, ticker := range list {
			wg.Add(1)
			go func(ticker string) {
				defer wg.Done()
				closes, err := s.bars.DailyBars(r.Context(), ticker)
				if err != nil || len(closes) == 0 {
					return
				}
				mu.Lock()
				result[ticker] = closes
				mu.Unlock()
			}(ticker)
		}
		wg.Wait()
	}
	writeJSON(w, http.StatusOK, map[string]any{"bars": result})
}

// getQuotesBatch returns the latest quote for multiple tickers in ONE request —
// the bulk counterpart to GET /v1/stocks/{ticker}/quote that list/zone views use so
// a 40-stock page makes ONE call instead of 40 (which saturated the browser's
// per-host connection cap and the per-IP rate limiter, the ~20s Theme-Zones load).
// Two phases: (1) concurrent in-process STORE reads (fast, no upstream calls),
// keeping a stale non-Yahoo quote as a baseline; (2) the missing/stale tickers are
// refreshed with a live price — ONE bulk snapshot via the optional bulkQuoter
// (prod BarCache → a single Alpaca SnapshotQuotesLive) instead of N serialized
// per-ticker fallbacks, overwriting a baseline only when fresher. Missing tickers
// are omitted (the client renders "—" and the SSE stream fills them in), so the
// response is always 200 with a possibly-partial map.
func (s *Server) getQuotesBatch(w http.ResponseWriter, r *http.Request) {
	result := map[string]store.Quote{}
	list := queryTickers(r, maxQuotesBatch)
	if len(list) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"quotes": result})
		return
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	// Phase 1 — concurrent STORE reads (in-process, fast, no upstream calls). Keep
	// even a stale non-Yahoo quote as a baseline; a missing/stale ticker is collected
	// for the live refresh below. (Yahoo was removed for commercial-use risk: never
	// serve a lingering Yahoo-sourced quote — treat it as absent so it re-resolves.)
	needRefresh := map[string]struct{}{}
	for _, ticker := range list {
		wg.Add(1)
		go func(ticker string) {
			defer wg.Done()
			q, ok, err := s.store.GetQuote(r.Context(), ticker)
			if err != nil || (ok && q.Source == "yahoo") {
				ok = false
			}
			mu.Lock()
			if ok {
				result[ticker] = q
			}
			if !ok || time.Since(q.At) > quoteStaleAfter {
				needRefresh[ticker] = struct{}{}
			}
			mu.Unlock()
		}(ticker)
	}
	wg.Wait()

	// Phase 2 — refresh the missing/stale tickers with a LIVE price. Prefer ONE bulk
	// snapshot (prod *ingest.BarCache via bulkQuoter) over N serialized per-ticker
	// fallbacks — the whole point of the batch endpoint; overwrite a baseline only
	// when the refreshed quote is fresher. Sources without the bulk extension fall
	// back to per-ticker LatestQuote, concurrently.
	if len(needRefresh) > 0 && s.bars != nil {
		misses := make([]string, 0, len(needRefresh))
		for t := range needRefresh {
			misses = append(misses, t)
		}
		if bq, ok := s.bars.(bulkQuoter); ok {
			if quotes, err := bq.LatestQuotes(r.Context(), misses); err == nil {
				for t, q := range quotes {
					if cur, had := result[t]; !had || q.At.After(cur.At) {
						result[t] = q
					}
				}
			}
		} else {
			for _, ticker := range misses {
				wg.Add(1)
				go func(ticker string) {
					defer wg.Done()
					if oq, found, qerr := s.bars.LatestQuote(r.Context(), ticker); qerr == nil && found {
						mu.Lock()
						if cur, had := result[ticker]; !had || oq.At.After(cur.At) {
							result[ticker] = oq
						}
						mu.Unlock()
					}
				}(ticker)
			}
			wg.Wait()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"quotes": result})
}

// getNewsBatch returns recent news merged across several tickers (the home
// feed), newest first. Each item keeps its `ticker` so the UI can tag it.
func (s *Server) getNewsBatch(w http.ResponseWriter, r *http.Request) {
	perTicker := queryLimit(r, 6)
	seen := make(map[string]struct{}) // an article may be tagged to several tickers
	var all []store.News
	for _, t := range queryTickers(r, maxBarsBatch) {
		items, err := s.store.ListNews(r.Context(), t, perTicker)
		if err != nil {
			continue
		}
		for _, n := range items {
			if _, ok := seen[n.ID]; ok {
				continue
			}
			seen[n.ID] = struct{}{}
			all = append(all, n)
		}
	}
	// Optional ?topic= filter: keep only articles matching a hot-topic's keywords.
	if topic := strings.TrimSpace(r.URL.Query().Get("topic")); topic != "" {
		kept := all[:0]
		for _, n := range all {
			if topics.Match(topic, n.Headline+" "+n.Summary) {
				kept = append(kept, n)
			}
		}
		all = kept
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Published.After(all[j].Published) })
	if len(all) > maxFeed {
		all = all[:maxFeed]
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(all), "news": all})
}

// getOpportunities returns the small-cap insider-buy Opportunity board, top
// first. Always 200 with a (possibly empty) list; ?limit= caps the rows.
func (s *Server) getOpportunities(w http.ResponseWriter, r *http.Request) {
	var board []opportunity.Stock
	if s.opps != nil {
		board = s.opps.Get()
	}
	if board == nil {
		board = []opportunity.Stock{}
	}
	if lim := queryLimit(r, 0); lim > 0 && len(board) > lim {
		board = board[:lim]
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(board), "stocks": board})
}

// getGurus returns the Guru-watch rail (recent curated-KOL posts with the
// tickers they mention), newest first. Always 200 with a (possibly empty) list;
// ?limit= caps the rows.
func (s *Server) getGurus(w http.ResponseWriter, r *http.Request) {
	var rail []guru.Item
	var updatedAt time.Time
	if s.gurus != nil {
		rail = s.gurus.Get()
		updatedAt = s.gurus.UpdatedAt()
	}
	if rail == nil {
		rail = []guru.Item{}
	}
	if lim := queryLimit(r, 0); lim > 0 && len(rail) > lim {
		rail = rail[:lim]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":      len(rail),
		"updated_at": updatedAt.UTC().Format(time.RFC3339),
		"items":      rail,
	})
}

// getSearch returns symbol-directory autocomplete matches for ?q= (best first).
// Always 200 with a (possibly empty) list; ?limit= caps results (default 10).
func (s *Server) getSearch(w http.ResponseWriter, r *http.Request) {
	var results []symbols.Symbol
	if s.symbols != nil {
		results = s.symbols.Search(r.URL.Query().Get("q"), queryLimit(r, 10))
	}
	if results == nil {
		results = []symbols.Symbol{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(results), "results": results})
}

// getSymbols enumerates the full US-listed ticker universe (~6,700) so the pSEO
// sitemap can seed a /stock/[ticker] page per symbol. Tickers pass through as the
// directory holds them (dotted names like BRK.B intact; no re-sort/normalize).
// Always 200 with a non-nil list — empty when the directory is unloaded; ?limit=
// caps the slice (the sitemap requests the full list with no limit). The list
// changes ~daily, so it's cacheable.
func (s *Server) getSymbols(w http.ResponseWriter, r *http.Request) {
	var tickers []string
	if s.symbols != nil {
		tickers = s.symbols.AllUSTickers()
	}
	if tickers == nil {
		tickers = []string{}
	}
	if lim := queryLimit(r, 0); lim > 0 && len(tickers) > lim {
		tickers = tickers[:lim]
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{"symbols": tickers, "count": len(tickers)})
}

// getEvents returns the major-events timeline windowed to what's relevant now:
// events from ~2 days ago onward (so a just-passed release stays briefly
// visible), ascending. Always 200 with a (possibly empty) list; ?limit= caps it.
func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	var all []events.Event
	if s.events != nil {
		all = s.events.Get()
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -2)
	out := make([]events.Event, 0, len(all))
	for _, e := range all {
		if e.StartUTC.Before(cutoff) {
			continue
		}
		out = append(out, e)
	}
	if lim := queryLimit(r, 40); lim > 0 && len(out) > lim {
		out = out[:lim]
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "events": out})
}

// getEarnings returns the company earnings calendar within a [from, to] window
// (YYYY-MM-DD; defaults to today .. +30d). Always 200 with a (possibly empty)
// list — never null. nil source → empty.
func (s *Server) getEarnings(w http.ResponseWriter, r *http.Request) {
	earnings := []store.Earning{}
	if s.earnings != nil {
		q := r.URL.Query()
		from := time.Now().UTC().Truncate(24 * time.Hour)
		to := from.AddDate(0, 0, 30)
		if v := strings.TrimSpace(q.Get("from")); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				from = t
			}
		}
		if v := strings.TrimSpace(q.Get("to")); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				to = t
			}
		}
		if got, err := s.earnings.ListEarnings(r.Context(), from, to); err != nil {
			s.log.Debug("earnings list failed", "err", err)
		} else if got != nil {
			earnings = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(earnings), "earnings": s.withReactions(earnings)})
}

// earningRow is one earnings-calendar row enriched with the stock's cached earnings-reaction
// aggregate (present only for tracked tickers with enough history). store.Earning is embedded, so
// all its JSON fields are promoted; Reaction is omitted when absent.
type earningRow struct {
	store.Earning
	Reaction *indicators.ReactionSummary `json:"reaction,omitempty"`
}

// withReactions attaches the cached earnings-reaction summary to each calendar row whose ticker is
// in the tracked-reaction cache (no per-row compute). A no-op shape when the source is unset.
func (s *Server) withReactions(earnings []store.Earning) []earningRow {
	rows := make([]earningRow, len(earnings))
	for i, e := range earnings {
		rows[i] = earningRow{Earning: e}
		if s.earningsReactions != nil {
			if rs, ok := s.earningsReactions.Reaction(strings.ToUpper(e.Ticker)); ok && rs.Samples > 0 {
				summary := rs
				rows[i].Reaction = &summary
			}
		}
	}
	return rows
}

// getStockEarnings returns the recent/upcoming earnings rows for one ticker
// (ascending by date), capped by ?limit= (default 8). Always 200 with a
// (possibly empty) list — never null. nil source → empty.
func (s *Server) getStockEarnings(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	earnings := []store.Earning{}
	if s.earnings != nil {
		if got, err := s.earnings.ListEarningsForTicker(r.Context(), ticker, queryLimit(r, 8)); err != nil {
			s.log.Debug("ticker earnings list failed", "ticker", ticker, "err", err)
		} else if got != nil {
			earnings = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "count": len(earnings), "earnings": earnings})
}

// getCongress returns the latest congressional Periodic Transaction Report
// filings (House Clerk public-domain disclosures), newest first. Always 200 with
// a (possibly empty) list — never null. nil source → empty. ?limit= caps rows.
func (s *Server) getCongress(w http.ResponseWriter, r *http.Request) {
	var filings []congress.Filing
	if s.congress != nil {
		filings = s.congress.Get()
	}
	if filings == nil {
		filings = []congress.Filing{}
	}
	if lim := queryLimit(r, 60); lim > 0 && len(filings) > lim {
		filings = filings[:lim]
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(filings), "filings": filings})
}

// getStockCongress returns the recent congressional trades in a ticker (the
// per-stock "members trading this" chip), newest first. Always 200 with a
// (possibly empty) list — never null; nil source / unparsed ticker → empty.
func (s *Server) getStockCongress(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	trades := []congress.TickerTrade{}
	if s.congressTx != nil {
		if got := s.congressTx.ByTicker(ticker); got != nil {
			trades = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "trades": trades})
}

// getCongressMember returns one member's parsed PTR transactions by slug (the
// member page). 404 for an unknown slug or when no transactions have been parsed
// for that member yet (e.g. only scanned filings).
func (s *Server) getCongressMember(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))
	if s.congressTx == nil {
		writeJSON(w, http.StatusNotFound, errBody("member not found: "+slug))
		return
	}
	m, ok := s.congressTx.ByMember(slug)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("member not found: "+slug))
		return
	}
	txs := m.Transactions
	if txs == nil {
		txs = []ptr.Transaction{} // marshal as [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"slug":         m.Slug,
		"name":         m.Name,
		"state":        m.State,
		"transactions": txs,
	})
}

// getInstitutional returns recent SEC Schedule 13D/13G beneficial-ownership
// filings (13D = active/activist stake, higher signal; 13G = passive, e.g. the
// index giants), newest first. ?type=13d|13g filters by activist flag; ?limit=
// caps (default 60). Always 200 with a (possibly empty) list. nil source → empty.
// dailyShortPoint is one dated point on the per-stock daily short-pressure curve.
type dailyShortPoint struct {
	Date     string  `json:"date"`
	ShortPct float64 `json:"short_pct"`
}

// dailyShort is the FINRA daily short-volume view for one symbol: the latest
// day's short percentage + a rolling history for the curve. It is additive to
// the existing bi-monthly short-interest object (see getShort).
type dailyShort struct {
	ShortPct float64           `json:"short_pct"`
	AsOf     string            `json:"as_of"`
	History  []dailyShortPoint `json:"history"`
}

// dailyShortFor builds the daily-short view for a symbol from the short-volume
// source, or nil when the source is absent or has no row for the symbol.
func (s *Server) dailyShortFor(ticker string) *dailyShort {
	if s.shortVolume == nil {
		return nil
	}
	latest, ok := s.shortVolume.Latest(ticker)
	if !ok {
		return nil
	}
	hist := s.shortVolume.History(ticker)
	points := make([]dailyShortPoint, 0, len(hist))
	for _, h := range hist {
		points = append(points, dailyShortPoint{Date: h.Date, ShortPct: h.ShortPct})
	}
	return &dailyShort{ShortPct: latest.ShortPct, AsOf: latest.Date, History: points}
}

// getShort returns the symbol's short data: the existing bi-monthly FINRA
// short-interest object (or null) plus an additive `daily` object carrying the
// FINRA daily short-volume percentage + rolling history (or null). It always
// returns 200 as long as either source has a row; 404 only when neither does, so
// the existing bi-monthly shape is preserved and the daily view is purely
// additive.
func (s *Server) getShort(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))

	var si *finra.ShortInterest
	if s.short != nil {
		if v, ok := s.short.ShortInterest(ticker); ok {
			si = &v
		}
	}
	daily := s.dailyShortFor(ticker)

	if si == nil && daily == nil {
		writeJSON(w, http.StatusNotFound, errBody("no short-interest data"))
		return
	}
	// `short` and `daily` are pointers so they marshal as JSON null when absent,
	// matching the A/B contract (existing bi-monthly shape unchanged when present).
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "short": si, "daily": daily})
}

// minShortVolume is the floor on a symbol's total reported volume to appear on
// the daily short-volume leaderboard, filtering out thin names whose short
// percentage is noisy (a single odd lot can read as 100% short).
const minShortVolume = 1_000_000

// getShortVolume returns the FINRA daily short-volume leaderboard — the latest
// trading day's symbols ranked by short percentage, capped at ?limit (default
// 50). Only the ranked Top is exposed (FINRA display-only terms forbid bulk
// raw-row redistribution). Always 200 with a (possibly empty) list — never null;
// an unready source yields an empty board.
func (s *Server) getShortVolume(w http.ResponseWriter, r *http.Request) {
	stocks := []finrashvol.ShortVol{}
	asOf := ""
	if s.shortVolume != nil {
		if top := s.shortVolume.Top(queryLimit(r, 50), minShortVolume); top != nil {
			stocks = top
		}
		asOf = s.shortVolume.AsOf()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"as_of":  asOf,
		"count":  len(stocks),
		"stocks": stocks,
	})
}

// sentimentComponent is one scored Fear & Greed component in the response.
type sentimentComponent struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Note  string `json:"note"`
}

// sentimentPoint is one dated headline-score sample for the history chart.
type sentimentPoint struct {
	Date  string `json:"date"`
	Score int    `json:"score"`
}

// getSentiment returns the latest Fear & Greed index — headline score + band
// label (English and Chinese), the scored components, the last-updated time and
// a daily history for charting. Always 200; an unready source yields a neutral
// 50 with empty components/history so the frontend always has a well-formed shape.
func (s *Server) getSentiment(w http.ResponseWriter, _ *http.Request) {
	res := sentiment.Result{Score: 50, Label: "Neutral", LabelZh: "中性", Components: []sentiment.Component{}}
	var updatedAt time.Time
	points := []sentimentPoint{}
	if s.sentiment != nil {
		if r, ok := s.sentiment.Latest(); ok {
			res = r
		}
		updatedAt = s.sentiment.UpdatedAt()
		for _, p := range s.sentiment.History() {
			points = append(points, sentimentPoint{Date: p.Date, Score: p.Score})
		}
	}
	comps := make([]sentimentComponent, 0, len(res.Components))
	for _, c := range res.Components {
		comps = append(comps, sentimentComponent{Name: c.Name, Score: c.Score, Note: c.Note})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"score":      res.Score,
		"label":      res.Label,
		"label_zh":   res.LabelZh,
		"components": comps,
		"updated_at": updatedAt.UTC().Format(time.RFC3339),
		"history":    points,
	})
}

// getRateCut returns the aggregated Fed rate-cut prediction markets (Kalshi +
// Polymarket), each with its mutually-exclusive outcomes and implied
// probabilities. Macro interest-rate markets only (never political). Always 200
// with a (possibly empty) market list — never null.
func (s *Server) getRateCut(w http.ResponseWriter, _ *http.Request) {
	markets := []ratecut.Market{}
	var updatedAt time.Time
	if s.rateCut != nil {
		if got := s.rateCut.Get(); got != nil {
			markets = got
		}
		updatedAt = s.rateCut.UpdatedAt()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"markets":    markets,
		"updated_at": updatedAt.UTC().Format(time.RFC3339),
	})
}

// getMacro returns the latest U.S. Treasury daily par yield curve as a compact
// macro-context strip: the present tenors (e.g. 2Y/10Y) with their par yields,
// the derived 2s10s spread (10Y − 2Y, percentage points) and whether the curve
// is inverted (the classic recession-watch signal). Always 200; an unready or
// nil source yields available=false with an empty yields list — the frontend
// hides the strip. Only tenors the Treasury actually published appear; a missing
// tenor (and the spread when either leg is absent) is omitted, never fabricated.
func (s *Server) getMacro(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"available":    false,
		"as_of":        "",
		"yields":       []treasury.Yield{},
		"inverted":     false,
		"source":       "U.S. Treasury",
		"source_zh":    "美国财政部",
		"source_url":   "https://home.treasury.gov/resource-center/data-chart-center/interest-rates/TextView?type=daily_treasury_yield_curve",
		"updated_at":   time.Time{}.UTC().Format(time.RFC3339),
		"spread_2s10s": nil, // null (not 0) until a real curve with both legs is loaded
	}
	if s.macro != nil {
		if curve, ok := s.macro.Latest(); ok && len(curve.Yields) > 0 {
			resp["available"] = true
			resp["as_of"] = curve.Date
			resp["yields"] = curve.Yields
			resp["inverted"] = curve.HasSpread && curve.Inverted
			if curve.HasSpread {
				resp["spread_2s10s"] = curve.Spread2s10s
			}
		}
		resp["updated_at"] = s.macro.UpdatedAt().UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// SetMacro injects the Treasury yield-curve source after New (keeping New's
// signature stable). nil-safe: /v1/macro reports available=false until set.
func (s *Server) SetMacro(src MacroSource) { s.macro = src }

// cryptoFGLabelZh maps an alternative.me Fear & Greed classification to its
// Chinese label. Keyed on the lower-cased English label so casing/spacing
// variations match. An unknown label falls through to the English string.
var cryptoFGLabelZh = map[string]string{
	"extreme fear":  "极度恐惧",
	"fear":          "恐惧",
	"neutral":       "中性",
	"greed":         "贪婪",
	"extreme greed": "极度贪婪",
}

// cryptoPrice renders a best-effort coin price for the JSON envelope: a JSON
// object {price, change_24h} when the source gave a real price, or nil (→ JSON
// null) when absent. Anti-fabrication: a missing price is omitted, never a 0.
func cryptoPrice(p cryptofg.Price) any {
	if !p.Present {
		return nil
	}
	return map[string]any{
		"price":      p.USD,
		"change_24h": p.Change24h,
	}
}

// getCrypto returns the latest crypto market-mood snapshot as a compact strip:
// the crypto Fear & Greed score (0–100) + its classification (English + Chinese)
// + the index day, plus best-effort BTC/ETH spot price + 24h change. This is
// crypto context for the crypto-linked equities COIN/MSTR/RIOT/MARA. Always 200;
// an unready or nil source yields available=false (the frontend hides the strip).
// BTC/ETH are null when the price source was unavailable — never fabricated; the
// F&G score alone is the feature.
func (s *Server) getCrypto(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"available":  false,
		"score":      0,
		"label":      "",
		"label_zh":   "",
		"as_of":      "",
		"btc":        nil,
		"eth":        nil,
		"source":     "alternative.me",
		"updated_at": time.Time{}.UTC().Format(time.RFC3339),
	}
	if s.crypto != nil {
		if idx, ok := s.crypto.Latest(); ok {
			labelZh := cryptoFGLabelZh[strings.ToLower(strings.TrimSpace(idx.Label))]
			if labelZh == "" {
				labelZh = idx.Label // unknown classification → fall back to the source label
			}
			resp["available"] = true
			resp["score"] = idx.Score
			resp["label"] = idx.Label
			resp["label_zh"] = labelZh
			resp["as_of"] = idx.AsOf
			resp["btc"] = cryptoPrice(idx.BTC)
			resp["eth"] = cryptoPrice(idx.ETH)
		}
		resp["updated_at"] = s.crypto.UpdatedAt().UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// SetCrypto injects the crypto Fear & Greed source after New (keeping New's
// signature stable). nil-safe: /v1/crypto reports available=false until set.
func (s *Server) SetCrypto(src CryptoSource) { s.crypto = src }

// SetShortVolume injects the FINRA daily short-volume source after New (keeping
// New's signature stable). nil-safe: the short-volume endpoints stay empty until set.
func (s *Server) SetShortVolume(src ShortVolumeSource) { s.shortVolume = src }

// SetSentiment injects the Fear & Greed sentiment source after New. nil-safe.
func (s *Server) SetSentiment(src SentimentSource) { s.sentiment = src }

// SetRateCut injects the Fed rate-cut markets source after New. nil-safe.
func (s *Server) SetRateCut(src RateCutSource) { s.rateCut = src }

// SetCongressTx injects the parsed-PTR transaction source (ticker/member detail)
// after New. nil-safe: the per-stock chip + member page stay empty/404 until set.
func (s *Server) SetCongressTx(src CongressTxSource) { s.congressTx = src }

// SetIPO injects the US IPO-calendar source after New. nil-safe: /v1/ipo stays
// empty (200 with empty sections) until set / first refreshed.
func (s *Server) SetIPO(src IPOSource) { s.ipo = src }

// SetIndicators injects the static stock-applicable indicator catalog after New.
// nil-safe: /v1/indicators returns an empty catalog until set.
func (s *Server) SetIndicators(src IndicatorSource) { s.indicators = src }

// SetIndicatorCompute injects the per-stock indicator compute source after New.
// nil-safe: /v1/stocks/{ticker}/indicators 404s until set.
func (s *Server) SetIndicatorCompute(src IndicatorComputeSource) { s.indicatorCalc = src }

// getIndicators returns the stock-applicable indicator catalog, optionally
// filtered by `domain`, `priority`, `subcategory`, and a free-text `q` (matched
// against the English/Chinese names, abbreviation, and definition). The response
// carries the filtered indicator array, the filtered `count`, the catalog `total`
// (unfiltered, stock-applicable), and `facets` (domain/priority/subcategory
// counts over the whole catalog) so the client can build filter chips. Always
// 200 with a well-formed (possibly empty) shape; nil-safe when unset.
func (s *Server) getIndicators(w http.ResponseWriter, r *http.Request) {
	if s.indicators == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"count":      0,
			"total":      0,
			"indicators": []indicators.Indicator{},
			"facets":     indicators.Facets{},
		})
		return
	}
	q := r.URL.Query()
	list := s.indicators.Filter(indicators.Query{
		Domain:      strings.TrimSpace(q.Get("domain")),
		Priority:    strings.TrimSpace(q.Get("priority")),
		Subcategory: strings.TrimSpace(q.Get("subcategory")),
		Text:        strings.TrimSpace(q.Get("q")),
	})
	if list == nil {
		list = []indicators.Indicator{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":      len(list),
		"total":      s.indicators.Len(),
		"indicators": list,
		"facets":     s.indicators.Facets(),
	})
}

// marketContextResp is the optional market-wide context block of the per-stock
// indicators response. Each field is omitted when its reading is unavailable;
// the whole block is omitted by the caller when neither is present.
type marketContextResp struct {
	VIX       *float64              `json:"vix,omitempty"`
	FearGreed *indicators.FearGreed `json:"fear_greed,omitempty"`
}

// stockIndicatorsResp is the wire shape of GET /v1/stocks/{ticker}/indicators
// (see the shared contract): the ticker, the newest underlying data date, an
// optional market-context block, and the computed indicator set (ok →
// insufficient → unsupported, as sorted by the compute layer).
type stockIndicatorsResp struct {
	Ticker        string                      `json:"ticker"`
	AsOf          string                      `json:"as_of"`
	MarketContext *marketContextResp          `json:"market_context,omitempty"`
	Indicators    []indicators.StockIndicator `json:"indicators"`
}

// getStockIndicators serves the live P0 stock-applicable indicator set for a
// single ticker (latest values). It is graceful: it returns 200 with whatever
// computed — a name with bars but no XBRL still gets its technical indicators —
// and 404 only when the compute source is unset or there is nothing at all to
// show (an unknown/non-US ticker with no candles, no fundamentals, and no market
// context). Market data is free public-domain / display-only sources.
func (s *Server) getStockIndicators(w http.ResponseWriter, r *http.Request) {
	if s.indicatorCalc == nil {
		writeJSON(w, http.StatusNotFound, errBody("indicators unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	res := s.indicatorCalc.StockIndicators(r.Context(), ticker)

	// Detect "nothing at all": no real reading anywhere (no ok indicator, no
	// underlying data date, no market context). That signals an unknown/non-US
	// ticker with no data, which 404s so the frontend hides the panel entirely.
	hasOK := false
	for _, si := range res.Indicators {
		if si.Status == indicators.StatusOK {
			hasOK = true
			break
		}
	}
	if !hasOK && res.AsOf == "" && res.VIX == nil && res.FearGreed == nil {
		writeJSON(w, http.StatusNotFound, errBody("no indicators for "+ticker))
		return
	}

	out := stockIndicatorsResp{Ticker: res.Ticker, AsOf: res.AsOf, Indicators: res.Indicators}
	if out.Indicators == nil {
		out.Indicators = []indicators.StockIndicator{}
	}
	// Market-context block: include only the readings that are present; omit the
	// whole block when neither VIX nor Fear & Greed is available.
	if res.VIX != nil || res.FearGreed != nil {
		out.MarketContext = &marketContextResp{VIX: res.VIX, FearGreed: res.FearGreed}
	}
	writeJSON(w, http.StatusOK, out)
}

// SetResearch injects the deep-research report source after New (keeps api.New's
// positional signature stable). nil-safe: /v1/stocks/{ticker}/research 404s until set.
func (s *Server) SetResearch(src ResearchSource) { s.researchCalc = src }

// SetDeepResearchLimit sets the per-user, per-ET-MONTH GENERATION quota for the
// deep report (depth=deep) from config (DEEP_RESEARCH_MONTHLY_LIMIT, default 1 =
// free 1 report/user/month). A value <= 0 is ignored so the deep path always keeps
// a sane (>=1) default rather than silently disabling generation for everyone.
func (s *Server) SetDeepResearchLimit(n int) {
	if n > 0 {
		s.deepResearchLimit = n
	}
}

// SetDeepResearchLimitPro sets the per-user, per-ET-MONTH GENERATION quota for PRO
// subscribers (DEEP_RESEARCH_MONTHLY_LIMIT_PRO, default 100). A value <= 0 is ignored so
// Pro always keeps a sane (high) default. This is what the limit-reached upsell promises,
// so it must stay above the free limit.
func (s *Server) SetDeepResearchLimitPro(n int) {
	if n > 0 {
		s.deepResearchLimitPro = n
	}
}

// SetPaywallEnabled turns the user-facing Pro paywall on/off (free-tier deep-report
// truncation). Off by default → full report for everyone (current behavior).
func (s *Server) SetPaywallEnabled(on bool) { s.paywallEnabled = on }

// demoReportTickers are the evergreen DEMO deep reports: a small set of mega-caps whose deep
// report is ALWAYS served in full to anyone (anon/free/Pro), regardless of PAYWALL_ENABLED, and
// generated quota-free — so a prospect can see one complete report before paying (the highest-
// converting honest sales asset + a top SEO page). Exactly one by design (the scan's call).
var demoReportTickers = map[string]bool{"AAPL": true}

func isDemoReport(ticker string) bool {
	return demoReportTickers[strings.ToUpper(strings.TrimSpace(ticker))]
}

// serveDeepReady writes a READY deep report, applying the free-tier paywall truncation
// when PAYWALL_ENABLED and the requester is not Pro. The shared cache entry is never
// mutated — truncation works on a serve-time copy, so Pro viewers (and a paywall-off
// deployment) always get the full report. A demo ticker is NEVER truncated.
func (s *Server) serveDeepReady(w http.ResponseWriter, r *http.Request, e researchEntry, ticker string) {
	if s.paywallEnabled && e.llm && !isDemoReport(ticker) {
		u, _ := auth.UserFrom(r.Context())
		if s.tierOf(r.Context(), u.ID) != tierPro {
			e = truncateDeepForFree(e)
		}
	}
	s.writeResearchStatus(w, e, proseStatusReady)
}

// truncateDeepForFree returns a COPY of a ready deep entry holding the free teaser:
// the overview section's prose is kept (its bull/bear verdict is stripped — Pro-only)
// and the FIRST body section with prose is kept in full; every other section is locked
// (prose/facts/citations/bull/bear cleared, Locked=true). paywallLocked is set so the
// response carries paywall_locked=true. The cache entry is untouched (the Sections
// slice is rebuilt fresh; kept sections are shared read-only, locked ones are new).
func truncateDeepForFree(e researchEntry) researchEntry {
	src := e.fs.Sections
	out := make([]research.SectionFacts, len(src))
	keptBody := false
	for i, sec := range src {
		switch {
		case sec.Key == "overview":
			sec.Bull = nil // the two-sided verdict is a Pro unlock
			sec.Bear = nil
			out[i] = sec
		case !keptBody && strings.TrimSpace(sec.Prose) != "":
			out[i] = sec // first body section with prose — the teaser
			keptBody = true
		default:
			out[i] = research.SectionFacts{
				Key:       sec.Key,
				TitleZH:   sec.TitleZH,
				TitleEN:   sec.TitleEN,
				Facts:     []research.Fact{},
				Citations: []research.Citation{},
				Locked:    true,
			}
		}
	}
	e.fs.Sections = out
	e.paywallLocked = true
	return e
}

// llmComposeTimeout bounds a single LLM compose/enrich call so an uncached AI
// endpoint degrades FAST to its data-only fallback instead of blocking up to the
// enricher's generous ~90s HTTP ceiling when the free-tier model is rate-limited
// or slow. It is applied at each handler call boundary via context.WithTimeout;
// because every enrich method builds its request with http.NewRequestWithContext,
// the deadline cancels the real in-flight HTTP call (not just the goroutine). On
// the deadline the enrich method returns context.DeadlineExceeded, which every
// AI handler already treats as "LLM unavailable → serve the existing data-only
// fallback" (refunding any reserved cap exactly like the other error paths).
//
//   - llmComposeTimeout covers the normal/short compositions (news+social digest,
//     movement explainer, per-filing material-event summaries, normal research).
//   - llmDeepComposeTimeout is the longer bound for the deep-research compose
//     (depth=deep, composeDeepMaxTokens=6000), which legitimately needs more room.
//     Measured: a premium Claude model (Sonnet 4.6) takes ~65s for a typical report
//     and up to ~110s at the full 6000-token ceiling, so the bound is 120s — the
//     deep path is ASYNC (background goroutine; the client polls), so a generous
//     budget costs no request latency. It MUST stay below the enricher's HTTP client
//     ceiling (see enrich.New) so the context deadline (not the socket) is the bound.
//
// These are vars (not consts) only so a test can shorten them to fire the deadline
// in milliseconds; production never reassigns them.
var (
	llmComposeTimeout     = 25 * time.Second
	llmDeepComposeTimeout = 120 * time.Second
)

// researchDailyCap bounds research-prose LLM generations per day across ALL
// tickers — a hard token-budget backstop, smaller than summaryDailyCap since R2
// is a bigger call. The cap gates PROSE only: the data-only fact sheet (assemble
// is cheap, no LLM) always serves, so over-cap requests still return a 200 report.
const researchDailyCap = 80

// deepReportTTL is how long a persisted deep report (the durable store cache) is served
// without regenerating. It decouples cache freshness from the per-user MONTHLY generation
// quota: a report stays "ready" for any viewer for a rolling week (surviving restarts),
// while a user still only spends a quota slot on a genuinely-new generation.
const deepReportTTL = 7 * 24 * time.Hour

// prose_status values reported by GET /v1/stocks/{ticker}/research in the
// "prose_status" response field. They tell a polling client exactly what the
// report's qualitative prose is doing; the Go-owned numbers/facts/citations are
// ALWAYS present regardless of the status (the LLM is off the critical path).
//
//   - proseStatusReady       prose is present (this is a finished report).
//   - proseStatusGenerating  data-only NOW; a background generation is in flight
//     for this (ticker, ET-month, lang) — the client should
//     POLL the same URL until it flips to "ready" (or to a
//     terminal status). No new gen is started by a poll.
//   - proseStatusQuotaExhausted the caller is over their monthly deep-research quota
//     (or the global daily cap is momentarily exhausted) and
//     no prose'd report is cached → data-only is returned
//     gracefully (200, NOT 429); prose won't be generated for
//     this request.
//   - proseStatusLLMDisabled  the LLM backend is off → data-only is the final report
//     (no prose will ever come; the client should stop polling).
const (
	proseStatusReady          = "ready"
	proseStatusGenerating     = "generating"
	proseStatusQuotaExhausted = "quota_exhausted"
	proseStatusLLMDisabled    = "llm_disabled"
)

// researchEntry is one cached research report. For the normal path it is keyed per
// (ticker, ET day, lang); for the deep path per (ticker, ET MONTH, lang). It holds
// the (possibly prose-filled) fact sheet, whether prose is present, the LLM model
// name, and the generation timestamp.
type researchEntry struct {
	fs    research.FactSheet
	llm   bool
	model string
	at    time.Time
	// paywallLocked is set ONLY on a serve-time COPY (never a cache entry) when the
	// free-tier deep-report truncation applied, so the response carries paywall_locked.
	paywallLocked bool
}

// researchRelativeRSWindow is the trailing window the report's relative-strength percentile uses
// (mirrors research.rsRelWindow = "3M"); the cold-cache guard checks the RS leaderboard for it.
const researchRelativeRSWindow = "3M"

// relativeSourcesCold reports whether any cross-sectional "relative to market" source feeding the
// report's relative section — the factor scorecard OR the relative-strength leaderboard — is still
// COLD (empty population): the brief post-restart window before that source's first scan lands. A
// report assembled now omits that source's relative fact(s) for EVERY ticker, so the cold-cache
// guards (sync: skip the durable cache; deep: defer the gen) treat it as cold until BOTH are warm,
// so a relative-INCOMPLETE report can't freeze for the ET-day/-month. nil source = that lens is off
// → not cold (steady state, cache normally).
func (s *Server) relativeSourcesCold() bool {
	if s.scorecard != nil {
		if pop, _ := s.scorecard.Population(); len(pop) == 0 {
			return true
		}
	}
	if s.rsScan != nil {
		if ranks, _ := s.rsScan.RankByWindow(researchRelativeRSWindow); len(ranks) == 0 {
			return true
		}
	}
	if s.earningsReactions != nil {
		if ranks, _ := s.earningsReactions.PopulationRanked(indicators.ReactionViewMostVolatile); len(ranks) == 0 {
			return true
		}
	}
	return false
}

// getResearch serves the per-ticker deep-research report: a Go-assembled, source-
// attributed fact sheet (every number set in Go) plus optional per-section LLM
// prose. The LLM is OFF THE CRITICAL PATH — the data-only fact sheet always serves
// 200, even when the LLM is disabled, over the cap, or the call errors. 404 only for
// an unknown/invalid ticker (the assembled fact sheet has nothing at all to show).
//
// The NORMAL path (no/unknown depth) is SYNCHRONOUS, public, and unchanged: prose is
// generated inline at most once per (ticker, ET day, lang) and served from memory.
//
// The DEEP path (depth=deep) is ASYNCHRONOUS (owner: async + polling): it requires a
// login, returns the data-only fact sheet INSTANTLY with a prose_status, and — when a
// generation is warranted — composes the prose in a DETACHED background goroutine; the
// client polls the same URL until prose_status flips to "ready". See getResearchDeep.
func (s *Server) getResearch(w http.ResponseWriter, r *http.Request) {
	if s.researchCalc == nil {
		writeJSON(w, http.StatusNotFound, errBody("research unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	lang := "zh" // Chinese-first default; English UI requests ?lang=en
	if r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	if r.URL.Query().Get("depth") == "deep" {
		s.getResearchDeep(w, r, ticker, lang)
		return
	}
	s.getResearchSync(w, r, ticker, lang)
}

// getResearchSync is the unchanged SYNCHRONOUS normal research path: it blocks on the
// (bounded) LLM compose inline and returns prose_status="ready" when the LLM wrote
// prose, else "llm_disabled" (the data-only fact sheet). Keyed per (ticker, ET day,
// lang); prose is generated at most once per key, deduped + served from memory.
func (s *Server) getResearchSync(w http.ResponseWriter, r *http.Request, ticker, lang string) {
	day := summaryDay() // ET trading day, shared with the AI digest cache
	key := ticker + "|" + day + "|" + lang

	for {
		s.researchMu.Lock()
		if e, ok := s.researchCache[key]; ok {
			s.researchMu.Unlock()
			s.writeResearch(w, e) // cache hit → free for everyone
			return
		}
		ch, busy := s.researchInflight[key]
		if !busy {
			break // we'll generate
		}
		s.researchMu.Unlock()
		select { // someone else is generating: wait, then re-check the cache
		case <-ch:
		case <-r.Context().Done():
			return
		}
	}
	// We hold researchMu and are the generator for this key.
	s.sweepResearchDay(day)
	// Reserve a prose-generation slot only if the LLM is enabled and under cap; the
	// data-only report is always assembled regardless (off the critical path).
	wantProse := s.researchCalc.Enabled() && s.researchDayCount < researchDailyCap
	if wantProse {
		s.researchDayCount++
	}
	ch := make(chan struct{})
	s.researchInflight[key] = ch
	s.researchMu.Unlock()

	// coldRelative is set right before the data-only assemble below: true when any relative-section
	// source (the factor scorecard OR the relative-strength leaderboard) is still COLD (empty
	// population) — the brief post-restart window before its first scan lands — which makes the
	// relative section incomplete for EVERY ticker. We then SKIP the durable per-(ticker, ET-day,
	// lang) cache so a relative-INCOMPLETE sheet can't freeze for the day; the next visit
	// re-assembles once warm. Cheap: the data-only sheet always returns, and only the few reports
	// born in the brief cold window re-assemble.
	coldRelative := false

	finish := func(e *researchEntry) {
		s.researchMu.Lock()
		if e != nil && !coldRelative {
			s.researchCache[key] = *e
		}
		delete(s.researchInflight, key)
		close(ch)
		s.researchMu.Unlock()
	}
	refundGlobalCap := func() {
		if wantProse {
			s.researchMu.Lock()
			if s.researchDayDate == day && s.researchDayCount > 0 {
				s.researchDayCount--
			}
			s.researchMu.Unlock()
			wantProse = false
		}
	}

	// Capture whether any relative source is cold BEFORE assembling (see relativeSourcesCold), so
	// coldRelative reflects the state at-or-before the assemble's own reads. Reading BEFORE the
	// assemble makes the only possible race a HARMLESS false-positive (skip caching a complete
	// report once, re-gen next request) — never the harmful inverse of caching a cold report.
	coldRelative = s.relativeSourcesCold()
	// Always assemble the data-only fact sheet first (cheap, no LLM, never errors).
	fs := s.researchCalc.Report(r.Context(), ticker, lang)
	// "Nothing at all": no sections and no underlying date → unknown/invalid ticker.
	if len(fs.Sections) == 0 && fs.AsOf == "" {
		refundGlobalCap() // didn't generate prose — don't burn the global budget
		finish(nil)       // don't cache an empty miss; let a later visit reassemble
		s.maybeCollect(ticker)
		writeJSON(w, http.StatusNotFound, errBody("no research for "+ticker))
		return
	}

	hasProse := false
	if wantProse {
		// Bound the LLM compose so an uncached report degrades FAST to the data-only
		// fact sheet rather than blocking on a slow/rate-limited model. The deadline
		// cancels the real outbound HTTP call (enrich uses NewRequestWithContext); a
		// context.DeadlineExceeded surfaces as empty prose below, which refunds the
		// reserved cap exactly like a disabled/errored compose.
		ctx, cancel := context.WithTimeout(r.Context(), llmComposeTimeout)
		fs = s.researchCalc.Compose(ctx, fs, lang)
		cancel()
		hasProse = factSheetHasProse(fs)
		if !hasProse {
			refundGlobalCap() // empty prose (disabled mid-flight / error) refunds the global budget
		}
	}

	model := ""
	if hasProse {
		model = s.researchCalc.Model()
	}
	e := researchEntry{fs: fs, llm: hasProse, model: model, at: time.Now().UTC()}
	finish(&e)
	s.writeResearch(w, e)
}

// getResearchDeep is the ASYNCHRONOUS deep-research path (depth=deep). It resolves the
// user + ET-month, then:
//
//  1. deep cache hit WITH prose → return it "ready" (free: no quota, no LLM, no gen).
//  2. else assemble the cheap data-only fact sheet (404 on an unknown ticker), then:
//     - a bg gen already in flight for (ticker, ET-month, lang) → data-only "generating"
//     (no second gen, no quota touched);
//     - else within the monthly quota AND LLM enabled AND under the global cap → start a
//     DETACHED background goroutine (context.Background() + the deep compose timeout,
//     NOT the request ctx) that composes the prose, and return data-only "generating"
//     NOW. On success the goroutine caches the prose'd sheet AND increments the monthly
//     quota EXACTLY ONCE; a failed/empty gen caches nothing and charges nothing (so a
//     later poll retries) and refunds the reserved global-cap slot;
//     - else over the monthly quota / global cap → data-only "quota_exhausted";
//     - else LLM off → data-only "llm_disabled".
//
// INVARIANTS: exactly ONE bg gen per (ticker, ET-month, lang) (researchInflight is the
// single-flight gate); concurrent polls return "generating" without a duplicate gen or
// double-charge; the quota is charged exactly once per successful gen and never on a
// poll, a cache hit, or a failed gen; the bg goroutine uses a detached context so the
// instant response returning (which cancels r.Context()) can't kill the generation; the
// inflight entry is always cleared (no goroutine / map leak).
func (s *Server) getResearchDeep(w http.ResponseWriter, r *http.Request, ticker, lang string) {
	demo := isDemoReport(ticker) // an evergreen demo report: anon-viewable + generated quota-free
	u, ok := auth.UserFrom(r.Context())
	if !ok && !demo {
		writeJSON(w, http.StatusUnauthorized, errBody("登录后才能生成深度研报 / login required for deep research"))
		return
	}
	// A demo gen is quota-FREE (it's a marketing asset, not the viewer's own report): use an
	// empty genUserID so the per-user quota check + charge are skipped (the global daily cap
	// still bounds cost). A non-demo report uses the real user id.
	userID := u.ID
	if demo {
		userID = ""
	}
	period := researchMonth() // per-user QUOTA period: ET calendar month (free 1/user/month)
	day := summaryDay()       // global-cap day, shared with the normal path / digest cache
	// The deep report caches AND single-flights per (ticker, ET-MONTH, lang) so a
	// report generated this month is served "ready" all month, and there is exactly
	// one bg gen per month per (ticker, lang). The "|deep" suffix keeps it from ever
	// colliding with the normal per-day key space.
	key := ticker + "|" + period + "|" + lang + "|deep"

	deepModel := s.researchCalc.DeepModel()

	// (1) Cache hit with prose → ready (no quota, no LLM, no gen). serveDeepReady applies
	// the free-tier paywall truncation at serve time; the cached entry stays full.
	s.researchMu.Lock()
	if e, ok := s.researchCache[key]; ok && e.llm {
		s.researchMu.Unlock()
		s.serveDeepReady(w, r, e, ticker)
		return
	}
	s.researchMu.Unlock()

	// (1b) Persisted cache: a recently-generated report in the DURABLE store survives a
	// restart (the in-memory cache is wiped on every redeploy — otherwise the next viewer
	// re-pays an LLM generation). Repopulate the in-memory cache + serve when fresh
	// (deepReportTTL). Best-effort: any store error / stale / prose-less row falls through
	// to the normal (re)generate path.
	if payload, genAt, ok, err := s.store.GetDeepReport(r.Context(), ticker, lang); err == nil && ok && time.Since(genAt) < deepReportTTL {
		var pfs research.FactSheet
		if json.Unmarshal(payload, &pfs) == nil && factSheetHasProse(pfs) {
			e := researchEntry{fs: pfs, llm: true, model: deepModel, at: genAt}
			s.researchMu.Lock()
			s.researchCache[key] = e
			s.researchMu.Unlock()
			s.serveDeepReady(w, r, e, ticker)
			return
		}
	}

	// (2) Assemble the cheap data-only fact sheet (no LLM, never errors). This is the
	// instant body we return in every non-cache-hit branch below.
	fs := s.researchCalc.Report(r.Context(), ticker, lang)
	if len(fs.Sections) == 0 && fs.AsOf == "" { // unknown/invalid ticker
		s.maybeCollect(ticker)
		writeJSON(w, http.StatusNotFound, errBody("no research for "+ticker))
		return
	}
	dataOnly := researchEntry{fs: fs, llm: false, model: "", at: time.Now().UTC()}

	// coldRelative: a relative-section source (the factor scorecard OR the relative-strength
	// leaderboard — see relativeSourcesCold) was still cold (empty) when we assembled fs, so fs
	// OMITS that relative fact for every ticker. The deep path caches per ET-MONTH AND persists to
	// the store (served for deepReportTTL days), so caching a relative-INCOMPLETE deep report would
	// freeze it far longer than the sync path's one ET-day. We therefore DEFER the gen while cold
	// (below): the gen — which charges the scarce monthly quota and caches — starts only once the
	// sources are warm. nil sources = relative lens off → never cold → generate normally.
	coldRelative := s.relativeSourcesCold()

	// Decide what to do under the lock so the single-flight gate + the cap reservation
	// are atomic with the inflight check (no two requests can both become the generator).
	s.researchMu.Lock()
	// A racing request may have finished a gen between our cache check and here.
	if e, ok := s.researchCache[key]; ok && e.llm {
		s.researchMu.Unlock()
		s.serveDeepReady(w, r, e, ticker)
		return
	}
	if _, busy := s.researchInflight[key]; busy {
		// A bg gen is already in flight → poll-friendly: data-only "generating", no
		// second gen, no quota touched.
		s.researchMu.Unlock()
		s.writeResearchStatus(w, dataOnly, proseStatusGenerating)
		return
	}
	if !s.researchCalc.Enabled() { // LLM off → data-only is final, no gen.
		s.researchMu.Unlock()
		s.writeResearchStatus(w, dataOnly, proseStatusLLMDisabled)
		return
	}
	if coldRelative {
		// A relative source still cold → fs omits a relative fact. Defer the gen (which
		// caches for the ET-month + persists to the store + charges the monthly quota) until
		// the population is warm, so we never persist a relative-less deep report. The client
		// keeps polling (~4s) and a later poll — once warm — starts the real gen. Reuses the
		// "generating" terminal state: no gen is started and no quota is touched here.
		s.researchMu.Unlock()
		s.writeResearchStatus(w, dataOnly, proseStatusGenerating)
		return
	}
	s.sweepResearchDay(day)
	if s.researchDayCount >= researchDailyCap { // global daily backstop momentarily full.
		s.researchMu.Unlock()
		s.writeResearchStatus(w, dataOnly, proseStatusQuotaExhausted)
		return
	}
	// Per-user MONTHLY generation quota, TIER-AWARE: Pro gets the high (≈on-demand)
	// limit so upgrading actually lifts the cap (the limit-reached upsell's promise);
	// free gets the base limit. Read fails OPEN (a backend hiccup never locks a user
	// out). Over quota with nothing cached → graceful data-only "quota_exhausted"
	// (replaces the old hard 429). A DEMO report skips the per-user quota entirely
	// (quota-free marketing asset; the global daily cap above still bounds cost).
	if !demo {
		monthlyLimit := s.deepResearchLimit
		if s.tierOf(r.Context(), userID) == tierPro {
			monthlyLimit = s.deepResearchLimitPro
		}
		if used, err := s.store.GetDeepQuotaUsed(r.Context(), userID, period); err != nil {
			s.log.Debug("deep-research quota read failed — failing open (allow)", "user", userID, "period", period, "err", err)
		} else if used >= monthlyLimit {
			s.researchMu.Unlock()
			s.writeResearchStatus(w, dataOnly, proseStatusQuotaExhausted)
			return
		}
	}
	// We are the SOLE generator for this key: reserve the global-cap slot + mark the
	// inflight gate, then spawn the detached bg goroutine and return data-only NOW.
	s.researchDayCount++
	ch := make(chan struct{})
	s.researchInflight[key] = ch
	s.researchMu.Unlock()

	// Hand the bg goroutine its OWN copy of the fact sheet: ComposeDeep writes prose
	// into the section structs in place, which would otherwise race the instant
	// response below marshaling the same backing array. cloneFactSheet copies the
	// section element array (the read-only Facts/Citations slices stay shared).
	go s.composeDeepBackground(ticker, lang, key, day, period, userID, deepModel, cloneFactSheet(fs), ch)

	s.writeResearchStatus(w, dataOnly, proseStatusGenerating)
}

// cloneFactSheet returns a copy whose Sections live in a fresh backing array, so a
// background ComposeDeep mutating section prose can't race a concurrent marshal of the
// original. The per-section read-only Facts/Citations/Context slices are shared (the
// composer only reads them); only the Prose/Bull/Bear fields it writes are made
// independent by the element-array copy.
func cloneFactSheet(fs research.FactSheet) research.FactSheet {
	if fs.Sections == nil {
		return fs
	}
	secs := make([]research.SectionFacts, len(fs.Sections))
	copy(secs, fs.Sections)
	fs.Sections = secs
	return fs
}

// composeDeepBackground runs ONE deep-prose generation off the request path. It uses a
// DETACHED context (context.Background() + the deep compose timeout) so the already-
// returned instant response cancelling the request context can't kill it. On success it
// caches the prose'd entry under key AND charges the user's monthly quota EXACTLY ONCE;
// on an empty/failed gen it caches nothing, charges nothing (so a later poll retries),
// and refunds the reserved global-cap slot. The inflight gate (ch / researchInflight[key])
// is always released, so there is no goroutine or map leak.
func (s *Server) composeDeepBackground(ticker, lang, key, day, period, userID, deepModel string, fs research.FactSheet, ch chan struct{}) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("deep-research bg compose panicked", "ticker", ticker, "rec", rec)
			// Release the inflight gate + refund the global slot so a panic can't leak.
			s.researchMu.Lock()
			delete(s.researchInflight, key)
			close(ch)
			if s.researchDayDate == day && s.researchDayCount > 0 {
				s.researchDayCount--
			}
			s.researchMu.Unlock()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), llmDeepComposeTimeout)
	composed := s.researchCalc.ComposeDeep(ctx, fs, lang)
	cancel()
	hasProse := factSheetHasProse(composed)

	if hasProse && userID != "" {
		// Charge the monthly quota EXACTLY ONCE for this successful generation, then
		// cache the prose'd report so every later view (any user) is a free "ready".
		// The increment is best-effort: an error is logged, not fatal (the user got
		// their report; worst case they keep an extra slot this month — fail open).
		// A demo gen passes userID="" (quota-free) → no charge.
		qctx, qcancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.store.IncrDeepQuotaUsed(qctx, userID, period); err != nil {
			s.log.Debug("deep-research quota increment failed (non-fatal)", "user", userID, "period", period, "err", err)
		}
		qcancel()
	}

	s.researchMu.Lock()
	if hasProse {
		s.researchCache[key] = researchEntry{fs: composed, llm: true, model: deepModel, at: time.Now().UTC()}
	} else {
		// Empty/failed gen: cache nothing (a later poll re-attempts) and refund the
		// reserved global-cap slot so a failure never burns the daily budget.
		if s.researchDayDate == day && s.researchDayCount > 0 {
			s.researchDayCount--
		}
	}
	delete(s.researchInflight, key)
	close(ch)
	s.researchMu.Unlock()

	// Persist the prose'd report to the durable store (off the lock, best-effort) so it
	// survives a restart — keyed by (ticker, lang), TTL-served for deepReportTTL.
	if hasProse {
		if payload, err := json.Marshal(composed); err == nil {
			pctx, pcancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := s.store.SaveDeepReport(pctx, ticker, lang, payload); err != nil {
				s.log.Debug("deep-report persist failed (non-fatal)", "ticker", ticker, "err", err)
			}
			pcancel()
		}
	}
}

// sweepResearchDay rolls the global per-day prose-generation counter to `day` and
// evicts stale cache entries on a new ET day. Caller MUST hold researchMu. Entries are
// pruned by their stored generation timestamp (not by string-matching the key), so the
// deep path's month-keyed entries are not wrongly evicted by the day roll: anything
// generated before today is dropped.
func (s *Server) sweepResearchDay(day string) {
	if s.researchDayDate == day {
		return
	}
	s.researchDayDate, s.researchDayCount = day, 0
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	startOfDay := time.Now().In(loc).Truncate(24 * time.Hour)
	for k, e := range s.researchCache {
		if e.at.In(loc).Before(startOfDay) {
			delete(s.researchCache, k)
		}
	}
}

// factSheetHasProse reports whether any section carries non-empty LLM prose.
func factSheetHasProse(fs research.FactSheet) bool {
	for _, sec := range fs.Sections {
		if strings.TrimSpace(sec.Prose) != "" {
			return true
		}
	}
	return false
}

// writeResearch marshals a cached research entry into the design §3.4 wire shape,
// inferring prose_status from the entry (ready when prose is present, else llm_disabled).
func (s *Server) writeResearch(w http.ResponseWriter, e researchEntry) {
	status := proseStatusReady
	if !e.llm {
		status = proseStatusLLMDisabled
	}
	s.writeResearchStatus(w, e, status)
}

// writeResearchStatus marshals a research entry into the wire shape plus the explicit
// prose_status the client polls on. Always 200. The legacy "llm" boolean is kept for
// back-compat (llm == (prose_status == "ready")).
func (s *Server) writeResearchStatus(w http.ResponseWriter, e researchEntry, status string) {
	sections := e.fs.Sections
	if sections == nil {
		sections = []research.SectionFacts{}
	}
	resp := map[string]any{
		"ticker":       e.fs.Ticker,
		"name":         e.fs.Name,
		"as_of":        e.fs.AsOf,
		"price_label":  e.fs.PriceLabel,
		"generated_at": e.at,
		"model":        e.model,
		"llm":          e.llm,
		"prose_status": status,
		"disclaimer":   e.fs.Disclaimer,
		"sections":     sections,
	}
	if e.paywallLocked { // free-tier teaser: the rest is Pro-gated
		resp["paywall_locked"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

// SetMovement injects the move-explainer source after New (keeps api.New's
// positional signature stable). nil-safe: /v1/stocks/{ticker}/movement 404s until set.
func (s *Server) SetMovement(src MovementSource) { s.movementCalc = src }

// movementDailyCap bounds move-explainer LLM sentences per day across ALL tickers
// — a hard token-budget backstop. The cap gates the LLM SENTENCE only: the
// data-only explanation (Go number + evidence + canned line) is cheap and always
// serves, so over-cap requests still return a 200 with the canned line.
const movementDailyCap = 120

// movementEntry is one cached move explanation (per ticker per ET day per
// language). It holds the assembled Explanation and the generation timestamp.
type movementEntry struct {
	exp movement.Explanation
	at  time.Time
}

// getMovement serves the per-ticker "why did this stock move today?" explainer.
// The change % and direction are computed IN GO from the quote (never the LLM's);
// the explainer is meaningful only on a NOTABLE move (|change| >= 5%). On a
// sub-threshold or quote-less move the response is a 200 with "significant":false
// and no explanation (the frontend hides the card).
//
// ASYNC (owner: async + polling — the LLM must never block the page). The endpoint
// returns the cheap data-only explanation (the Go number + attributed evidence + a
// canned Go-built line) INSTANTLY with a prose_status, and — on a notable move when
// an LLM sentence is warranted — composes that ONE hedged sentence in a DETACHED
// background goroutine, caching the upgrade. The client polls the same URL while
// prose_status=="generating" (showing the canned line meanwhile) until it flips to a
// terminal status. prose_status:
//   - "ready"            terminal: the explanation shown is final (the LLM sentence
//     when llm:true, else the canned line, or a sub-threshold blank).
//   - "generating"       the canned line NOW; a bg LLM gen is in flight → POLL.
//   - "quota_exhausted"  over the daily LLM-sentence cap → canned line, terminal.
//
// The LLM is OFF THE CRITICAL PATH — the endpoint always serves the number + evidence
// instantly, never 500/503/blocking. The LLM sentence is generated at most once per
// (ticker, ET day, lang), single-flighted + served from memory. 404 only for an
// unknown/invalid ticker (no quote at all and no evidence).
//
// INVARIANTS (mirror getResearchDeep): exactly ONE bg gen per (ticker, ET-day, lang)
// via moveInflight; concurrent polls return "generating" without a duplicate gen or
// double cap charge; the cap is charged when the gen is reserved and REFUNDED on a
// failed/empty gen; the bg goroutine uses a DETACHED context so the instant response
// returning (which cancels r.Context()) can't kill it; the inflight gate is always
// cleared (no goroutine / map leak).
func (s *Server) getMovement(w http.ResponseWriter, r *http.Request) {
	if s.movementCalc == nil {
		writeJSON(w, http.StatusNotFound, errBody("movement unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	lang := "zh" // Chinese-first default; English UI requests ?lang=en
	if r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	day := summaryDay() // ET trading day, shared with the AI digest cache
	key := ticker + "|" + day + "|" + lang

	// (1) Cache hit → terminal "ready" (the cached explanation — LLM sentence or a
	// final canned/sub-threshold line — is the finished answer for the day).
	s.moveMu.Lock()
	if e, ok := s.moveCache[key]; ok {
		s.moveMu.Unlock()
		s.writeMovement(w, e, proseStatusReady)
		return
	}
	s.moveMu.Unlock()

	// (2) Assemble the cheap data-only explanation (Go number + evidence + canned
	// line; no LLM, never errors). lang is threaded so the canned line + Go-built
	// evidence titles come back in the requested language. This is the instant body
	// returned in every non-cache-hit branch below.
	exp := s.movementCalc.Report(r.Context(), ticker, lang)
	// "Nothing at all": no usable quote AND no evidence → unknown/invalid ticker
	// (a sub-threshold move with a real quote DOES have a number, so it is served).
	if exp.AsOf.IsZero() && exp.ChangePct == 0 && len(exp.Evidence) == 0 {
		s.maybeCollect(ticker)
		writeJSON(w, http.StatusNotFound, errBody("no movement data for "+ticker))
		return
	}
	dataOnly := movementEntry{exp: exp, at: time.Now().UTC()}

	// Decide under the lock so the single-flight gate + the cap reservation are atomic
	// with the inflight check (no two requests can both become the generator).
	s.moveMu.Lock()
	if e, ok := s.moveCache[key]; ok { // a racing request finished a gen
		s.moveMu.Unlock()
		s.writeMovement(w, e, proseStatusReady)
		return
	}
	if s.moveDayDate != day { // day roll: reset the cap + evict yesterday's entries
		s.moveDayDate, s.moveDayCount = day, 0
		for k := range s.moveCache { // key = ticker|day|lang
			if !strings.Contains(k, "|"+day+"|") {
				delete(s.moveCache, k)
			}
		}
	}
	if !exp.Significant { // sub-threshold → no LLM ever; cache the blank as terminal.
		s.moveCache[key] = dataOnly
		s.moveMu.Unlock()
		s.writeMovement(w, dataOnly, proseStatusReady)
		return
	}
	if _, busy := s.moveInflight[key]; busy { // a bg gen is in flight → poll-friendly
		s.moveMu.Unlock()
		s.writeMovement(w, dataOnly, proseStatusGenerating)
		return
	}
	if !s.movementCalc.Enabled() { // LLM off → the canned line is final; cache it.
		s.moveCache[key] = dataOnly
		s.moveMu.Unlock()
		s.writeMovement(w, dataOnly, proseStatusReady)
		return
	}
	if s.moveDayCount >= movementDailyCap { // daily LLM-sentence cap full → canned line.
		s.moveMu.Unlock()
		s.writeMovement(w, dataOnly, proseStatusQuotaExhausted) // don't cache → retry when cap frees
		return
	}
	// We are the SOLE generator for this key: reserve the cap slot + the inflight gate,
	// spawn the detached bg goroutine (which calls the LLM), and return the canned line
	// "generating" NOW.
	s.moveDayCount++
	ch := make(chan struct{})
	s.moveInflight[key] = ch
	s.moveMu.Unlock()

	go s.explainMovementBackground(ticker, lang, day, key, ch)

	s.writeMovement(w, dataOnly, proseStatusGenerating)
}

// explainMovementBackground composes ONE move-explainer LLM sentence off the request
// path, using a DETACHED context (so the already-returned instant response cancelling
// r.Context() can't kill it). On success (the LLM wrote a sentence) it caches the LLM'd
// explanation as a terminal "ready" entry — every later view is then a free upgrade —
// and keeps the reserved cap charged; on a failed/empty/disabled gen it caches NOTHING
// (so a later visit retries) and REFUNDS the cap (the caller already has the canned
// line, so a failure just leaves the card showing it). The inflight gate is always
// released, so there is no goroutine or map leak.
func (s *Server) explainMovementBackground(ticker, lang, day, key string, ch chan struct{}) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("movement bg explain panicked", "ticker", ticker, "rec", rec)
			s.moveMu.Lock()
			delete(s.moveInflight, key)
			close(ch)
			if s.moveDayDate == day && s.moveDayCount > 0 {
				s.moveDayCount--
			}
			s.moveMu.Unlock()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), llmComposeTimeout)
	exp := s.movementCalc.Explain(ctx, ticker, lang)
	cancel()

	s.moveMu.Lock()
	if exp.LLM { // the LLM wrote a sentence → cache the upgrade; the cap stays charged.
		s.moveCache[key] = movementEntry{exp: exp, at: time.Now().UTC()}
	} else {
		// Disabled mid-flight / errored / timed out / no longer significant → cache
		// nothing (a later visit retries) and refund the reserved cap slot.
		if s.moveDayDate == day && s.moveDayCount > 0 {
			s.moveDayCount--
		}
	}
	delete(s.moveInflight, key)
	close(ch)
	s.moveMu.Unlock()
}

// writeMovement marshals a movement entry into the wire shape plus the explicit
// prose_status the client polls on (the legacy "llm" boolean is kept too). Always
// 200. A sub-threshold move carries significant:false and no explanation/evidence
// (the frontend hides the card); a notable move carries the Go-owned number, the
// attributed evidence, the explanation (LLM sentence or canned line), and the
// llm/model/as_of chrome. status is "generating" while a bg LLM gen is in flight
// (the canned line is shown meanwhile), else a terminal status.
func (s *Server) writeMovement(w http.ResponseWriter, e movementEntry, status string) {
	exp := e.exp
	ev := exp.Evidence
	if ev == nil {
		ev = []movement.Evidence{}
	}
	out := map[string]any{
		"ticker":       exp.Ticker,
		"significant":  exp.Significant,
		"change_pct":   exp.ChangePct,
		"direction":    exp.Direction,
		"session":      exp.Session,
		"llm":          exp.LLM,
		"model":        exp.Model,
		"as_of":        exp.AsOf,
		"generated_at": e.at,
		"prose_status": status,
	}
	if exp.Significant {
		out["explanation"] = exp.Text
		out["evidence"] = ev
		out["disclaimer"] = exp.Disclaimer
	}
	writeJSON(w, http.StatusOK, out)
}

// SetMaterialEvents injects the 8-K material-events source after New (keeps
// api.New's positional signature stable). nil-safe: the endpoint 404s until set.
func (s *Server) SetMaterialEvents(src MaterialEventsSource) { s.materialCalc = src }

// SetMaterialFeed injects the market-wide notable-events feed source after New. nil-safe: the
// /v1/material-feed endpoint 404s until set.
func (s *Server) SetMaterialFeed(src MaterialFeedSource) { s.materialFeed = src }

// SetEarningsDates injects the earnings-dates source (8-K item 2.02 filing dates) after New —
// the dated anchors for the earnings-reaction statistic. Keeps it out of the New() signature.
func (s *Server) SetEarningsDates(src EarningsDatesSource) { s.earningsDates = src }

// SetScorecard injects the factor-percentile population source after New (the multi-factor
// scorecard ranks a ticker against it). Keeps it out of the New() signature.
func (s *Server) SetScorecard(src ScorecardSource) { s.scorecard = src }

// SetDividendScan injects the dividend leaderboard source after New (the dividend screen ranks the
// tracked universe against it). Keeps it out of the New() signature.
func (s *Server) SetDividendScan(src DividendScanSource) { s.dividendScan = src }

// SetRSScan injects the relative-strength leaderboard source after New (the RS screen ranks the
// tracked universe by trailing relative strength vs SPY). Keeps it out of the New() signature.
func (s *Server) SetRSScan(src RSScanSource) { s.rsScan = src }

// SetEarningsReactions injects the cached earnings-reaction aggregates after New (the earnings
// calendar badges each tracked row with them). Keeps it out of the New() signature.
func (s *Server) SetEarningsReactions(src EarningsReactionSource) { s.earningsReactions = src }

// materialEventsDailyCap bounds material-events LLM-summary REPORTS per day across
// ALL tickers — a hard token-budget backstop. The cap gates the LLM-summary path
// only: a facts-only report (the parsed 8-K items + canonical labels + source
// links) is cheap and always serves, so over-cap requests still return 200 with
// the filings and no summaries.
const materialEventsDailyCap = 80

// materialFeedResp is the wire shape of GET /v1/material-feed: the market-wide notable-events feed.
type materialFeedResp struct {
	Count  int                        `json:"count"`
	Events []materialevents.FeedEvent `json:"events"`
	AsOf   string                     `json:"as_of,omitempty"`
}

// defaultMaterialFeedLimit / max bound the feed length.
const (
	defaultMaterialFeedLimit = 60
	maxMaterialFeedLimit     = 120
)

// getMaterialFeed serves the market-wide NOTABLE MATERIAL-EVENTS feed: recent high-signal 8-K filings
// (leadership change, M&A, bankruptcy, restatement, delisting, …) across the tracked universe, newest
// first — the market-wide roll-up of the per-stock material-events view. `?item=5.02` filters to one
// 8-K item code; `?limit=` caps (default 60). Every field is a Go-owned SEC fact (form, dates, item
// codes + canonical labels, filing link) — FACTS ONLY, no LLM, no advice — so it is
// anti-hallucination-safe. Free + crawlable. Always 200 with a (possibly empty) list; 404 only when
// the feed source is unset; a cold cache yields an empty list (the scan refills it).
func (s *Server) getMaterialFeed(w http.ResponseWriter, r *http.Request) {
	if s.materialFeed == nil {
		writeJSON(w, http.StatusNotFound, errBody("material-events feed unavailable"))
		return
	}
	item := strings.TrimSpace(r.URL.Query().Get("item"))
	events, at := s.materialFeed.Feed(item)
	lim := queryLimit(r, defaultMaterialFeedLimit)
	if lim > maxMaterialFeedLimit {
		lim = maxMaterialFeedLimit
	}
	if lim > 0 && len(events) > lim {
		events = events[:lim]
	}
	if events == nil {
		events = []materialevents.FeedEvent{}
	}
	out := materialFeedResp{Count: len(events), Events: events}
	if !at.IsZero() {
		out.AsOf = at.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// materialEventsEntry is one cached material-events report (per ticker per ET day
// per language). It holds the assembled Report and the generation timestamp.
type materialEventsEntry struct {
	rep materialevents.Report
	at  time.Time
}

// getMaterialEvents serves a company's recent 8-K (current report) filings with an
// optional AI plain-language summary. Go owns every FACT — the form type, filing /
// report dates, accession URL, and the parsed item codes AND their canonical
// labels (the item-code → meaning map lives in internal/edgar, NEVER the LLM). The
// LLM, when on and under the daily cap, writes ONLY a short factual summary of what
// each filing's source text says happened; it never invents numbers/dates/names and
// never gives advice. The LLM is OFF THE CRITICAL PATH — the endpoint always serves
// the filings + item labels + source links, never 500/503. The full report (facts +
// optional summaries) is generated at most once per (ticker, ET day, lang) and
// served from memory. 404 only when the ticker/CIK can't be resolved at all; an
// existing company with zero recent 8-Ks returns {"filings":[]} with 200.
func (s *Server) getMaterialEvents(w http.ResponseWriter, r *http.Request) {
	if s.materialCalc == nil {
		writeJSON(w, http.StatusNotFound, errBody("material events unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	lang := "zh" // Chinese-first default; English UI requests ?lang=en
	if r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	day := summaryDay() // ET trading day, shared with the other per-ticker caches
	key := ticker + "|" + day + "|" + lang

	for {
		s.meMu.Lock()
		if e, ok := s.meCache[key]; ok {
			s.meMu.Unlock()
			s.writeMaterialEvents(w, e)
			return
		}
		ch, busy := s.meInflight[key]
		if !busy {
			break // we'll generate
		}
		s.meMu.Unlock()
		select { // someone else is generating: wait, then re-check the cache
		case <-ch:
		case <-r.Context().Done():
			return
		}
	}
	// We hold meMu and are the generator for this key.
	if s.meDayDate != day {
		s.meDayDate, s.meDayCount = day, 0
		for k := range s.meCache { // yesterday's reports are dead weight
			if !strings.Contains(k, "|"+day+"|") { // key = ticker|day|lang
				delete(s.meCache, k)
			}
		}
	}
	// Reserve an LLM-summary slot only if the LLM is enabled and under cap; the
	// facts-only report is always assembled regardless (off the critical path).
	wantLLM := s.materialCalc.Enabled() && s.meDayCount < materialEventsDailyCap
	if wantLLM {
		s.meDayCount++
	}
	ch := make(chan struct{})
	s.meInflight[key] = ch
	s.meMu.Unlock()

	finish := func(e *materialEventsEntry) {
		s.meMu.Lock()
		if e != nil {
			s.meCache[key] = *e
		}
		delete(s.meInflight, key)
		close(ch)
		s.meMu.Unlock()
	}

	var (
		rep materialevents.Report
		err error
	)
	if wantLLM {
		// Bound the LLM-summary pass so a slow/rate-limited model degrades FAST to
		// the facts-only report. The deadline cancels the real outbound HTTP call
		// (enrich uses NewRequestWithContext): a per-filing summary that times out
		// is dropped to "" inside the service (never an error), so the report still
		// serves its filings + item labels. rep.LLM=false then refunds the cap below.
		ctx, cancel := context.WithTimeout(r.Context(), llmComposeTimeout)
		rep, err = s.materialCalc.Summarize(ctx, ticker, lang)
		cancel()
		// Defensive: if the deadline fired during the EDGAR facts fetch (before any
		// summary), Summarize errors — but the company may be perfectly valid. While
		// the parent context is still alive, fall back to the facts-only report so a
		// compose timeout can never turn a real ticker into a 404.
		if err != nil && r.Context().Err() == nil {
			rep, err = s.materialCalc.Report(r.Context(), ticker)
		}
	} else {
		rep, err = s.materialCalc.Report(r.Context(), ticker)
	}
	if err != nil {
		// The ticker/CIK couldn't be resolved or the SEC feed fetch failed → no
		// cache entry (let a later visit retry), refund any reserved LLM budget,
		// and 404 (mirrors getMovement's "nothing at all" path).
		if wantLLM {
			s.meMu.Lock()
			s.meDayCount--
			s.meMu.Unlock()
		}
		finish(nil)
		s.maybeCollect(ticker)
		writeJSON(w, http.StatusNotFound, errBody("no material events for "+ticker))
		return
	}
	// If we reserved an LLM slot but no summary was actually written (LLM off
	// mid-flight, all sources too thin, or every summary errored), refund the budget.
	if wantLLM && !rep.LLM {
		s.meMu.Lock()
		s.meDayCount--
		s.meMu.Unlock()
	}

	e := materialEventsEntry{rep: rep, at: time.Now().UTC()}
	finish(&e)
	s.writeMaterialEvents(w, e)
}

// writeMaterialEvents marshals a cached material-events report into the wire shape.
// Always 200. The filings array is ALWAYS present and non-null (an existing company
// with no recent 8-Ks serializes "filings":[]); each filing carries the Go-owned
// facts + item labels, and an AI summary only when one was written.
func (s *Server) writeMaterialEvents(w http.ResponseWriter, e materialEventsEntry) {
	rep := e.rep
	filings := rep.Filings
	if filings == nil {
		filings = []materialevents.Filing{}
	}
	out := map[string]any{
		"ticker":       rep.Ticker,
		"filings":      filings,
		"count":        len(filings),
		"llm":          rep.LLM,
		"model":        rep.Model,
		"source":       "SEC EDGAR",
		"generated_at": e.at,
	}
	if rep.LLM {
		out["disclaimer"] = rep.Disclaimer
	}
	writeJSON(w, http.StatusOK, out)
}

// SetInsiderActivity injects the Form 4 insider-activity source after New (keeps
// api.New's positional signature stable). nil-safe: the endpoint 404s until set.
func (s *Server) SetInsiderActivity(src InsiderActivitySource) { s.insiderCalc = src }

// insiderActivityEntry is one cached insider-activity report (per ticker per ET
// day). It holds the assembled Report and the generation timestamp.
type insiderActivityEntry struct {
	rep insideractivity.Report
	at  time.Time
}

// getInsiderActivity serves a company's recent insider-activity timeline — Form 4
// open-market buys AND sells, newest first. Go owns EVERY fact: shares, price,
// value (= shares×price), transaction date, the insider's name + role, buy/sell,
// and the best-effort Rule 10b5-1 planned-sale flag — all parsed straight from
// the Form 4 XML. There is NO LLM in this feature. The report is assembled at
// most once per (ticker, ET day) — the day's first visit pays the per-filing XML
// fetch (capped), everyone else (and every refresh) hits the cache — then served
// from memory; server-driven, never an external operator. 404 only when the
// ticker/CIK can't be resolved; an existing company with zero recent Form 4s
// returns {"transactions":[]} with 200.
func (s *Server) getInsiderActivity(w http.ResponseWriter, r *http.Request) {
	if s.insiderCalc == nil {
		writeJSON(w, http.StatusNotFound, errBody("insider activity unavailable"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	if ticker == "" {
		writeJSON(w, http.StatusNotFound, errBody("no ticker"))
		return
	}
	day := summaryDay() // ET trading day, shared with the other per-ticker caches
	key := ticker + "|" + day

	for {
		s.iaMu.Lock()
		if s.iaDay != day { // a new ET day: yesterday's reports are dead weight
			s.iaDay = day
			for k := range s.iaCache {
				if !strings.HasSuffix(k, "|"+day) {
					delete(s.iaCache, k)
				}
			}
		}
		if e, ok := s.iaCache[key]; ok {
			s.iaMu.Unlock()
			s.writeInsiderActivity(w, e)
			return
		}
		ch, busy := s.iaInflight[key]
		if !busy {
			break // we'll generate
		}
		s.iaMu.Unlock()
		select { // someone else is generating: wait, then re-check the cache
		case <-ch:
		case <-r.Context().Done():
			return
		}
	}
	// We hold iaMu and are the generator for this key.
	ch := make(chan struct{})
	s.iaInflight[key] = ch
	s.iaMu.Unlock()

	finish := func(e *insiderActivityEntry) {
		s.iaMu.Lock()
		if e != nil {
			s.iaCache[key] = *e
		}
		delete(s.iaInflight, key)
		close(ch)
		s.iaMu.Unlock()
	}

	rep, err := s.insiderCalc.Report(r.Context(), ticker)
	if err != nil {
		// The ticker/CIK couldn't be resolved or the SEC feed fetch failed → no
		// cache entry (let a later visit retry), kick off on-demand collection, 404.
		finish(nil)
		s.maybeCollect(ticker)
		writeJSON(w, http.StatusNotFound, errBody("no insider activity for "+ticker))
		return
	}
	e := insiderActivityEntry{rep: rep, at: time.Now().UTC()}
	finish(&e)
	s.writeInsiderActivity(w, e)
}

// writeInsiderActivity marshals a cached insider-activity report into the wire
// shape. Always 200. The transactions array is ALWAYS present and non-null (an
// existing company with no recent Form 4s serializes "transactions":[]); each
// transaction carries the Go-owned facts + the 10b5-1 flag.
func (s *Server) writeInsiderActivity(w http.ResponseWriter, e insiderActivityEntry) {
	rep := e.rep
	txns := rep.Transactions
	if txns == nil {
		txns = []edgar.InsiderTransaction{}
	}
	out := map[string]any{
		"ticker":       rep.Ticker,
		"transactions": txns,
		"count":        len(txns),
		"buy_count":    rep.BuyCount,
		"sell_count":   rep.SellCount,
		"net_value":    rep.NetValue,
		"source":       "SEC EDGAR Form 4",
		"generated_at": e.at,
	}
	writeJSON(w, http.StatusOK, out)
}

// getIPO returns the US IPO calendar — recently priced, upcoming, and newly
// filed offerings (Nasdaq, delayed/display-only). Always 200 with well-formed
// (possibly empty) sections — never null — and nil-safe when the source is unset
// or hasn't refreshed yet.
func (s *Server) getIPO(w http.ResponseWriter, _ *http.Request) {
	priced, upcoming, filed := []nasdaq.IPO{}, []nasdaq.IPO{}, []nasdaq.IPO{}
	var updatedAt time.Time
	if s.ipo != nil {
		cal, at := s.ipo.Calendar()
		if cal.Priced != nil {
			priced = cal.Priced
		}
		if cal.Upcoming != nil {
			upcoming = cal.Upcoming
		}
		if cal.Filed != nil {
			filed = cal.Filed
		}
		updatedAt = at
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"priced":     priced,
		"upcoming":   upcoming,
		"filed":      filed,
		"updated_at": updatedAt.UTC().Format(time.RFC3339),
	})
}

// getIndices returns the latest major-market-index levels (homepage strip).
// getOptions returns the ticker's delayed options overview (P/C, max pain, OI
// leaders). 404 when the symbol has no listed options or the source is off.
func (s *Server) getOptions(w http.ResponseWriter, r *http.Request) {
	if s.options == nil {
		writeJSON(w, http.StatusNotFound, errBody("no options data"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	view, ok := s.options.Options(r.Context(), ticker)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("no options data"))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// getUnusualOptions returns the whole-market unusual options-activity board
// (top contracts by single-contract volume). Empty list until the first scan.
func (s *Server) getUnusualOptions(w http.ResponseWriter, _ *http.Request) {
	contracts := []ingest.UnusualContract{}
	var at time.Time
	if s.options != nil {
		if got, t := s.options.Unusual(); got != nil {
			contracts, at = got, t
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(contracts), "updated_at": at, "contracts": contracts})
}

// getThirteenF serves the 13F whale-holdings board (famous funds' latest
// quarterly holdings + QoQ changes). Empty board until the first scan completes.
func (s *Server) getThirteenF(w http.ResponseWriter, _ *http.Request) {
	board := thirteenf.Board{Funds: []thirteenf.FundHoldings{}}
	if s.thirteenf != nil {
		if b, ok := s.thirteenf.Board(); ok {
			board = b
		}
	}
	writeJSON(w, http.StatusOK, board)
}

// getWhales serves the per-stock reverse 13F lookup: which tracked funds hold
// this ticker, with each fund's position value, portfolio weight, and QoQ
// change. Always 200 — an empty holders list when nothing matches (so the
// frontend chip can self-hide) — and nil-safe when the source is unset.
func (s *Server) getWhales(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	holders := []thirteenf.Holder{}
	if s.thirteenf != nil {
		if got := s.thirteenf.Holders(ticker); got != nil {
			holders = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "holders": holders})
}

// getThirteenFFund serves one fund's latest 13F holdings by slug, for the fund
// pSEO page. 404 when the slug is unknown or the board has not been built yet.
func (s *Server) getThirteenFFund(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))
	if s.thirteenf == nil {
		writeJSON(w, http.StatusNotFound, errBody("no 13F data"))
		return
	}
	fh, ok := s.thirteenf.Fund(slug)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("unknown fund"))
		return
	}
	writeJSON(w, http.StatusOK, fh)
}

// getBriefing returns today's AI pre-market briefing; 404 until generated.
func (s *Server) getBriefing(w http.ResponseWriter, r *http.Request) {
	if s.briefing == nil {
		writeJSON(w, http.StatusNotFound, errBody("briefing not available"))
		return
	}
	lang := "zh" // Chinese-first default; English UI requests ?lang=en
	if r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	date, text, at, ok := s.briefing.Get(lang)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("briefing not generated yet"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "text": text, "generated_at": at})
}

func (s *Server) getIndices(w http.ResponseWriter, _ *http.Request) {
	indices := []store.IndexQuote{}
	if s.indices != nil {
		if got := s.indices.Indices(); got != nil {
			indices = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(indices), "indices": indices})
}

func (s *Server) getInstitutional(w http.ResponseWriter, r *http.Request) {
	var filings []sec.OwnershipRef
	if s.institutional != nil {
		filings = s.institutional.Get()
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type"))) {
	case "13d":
		filings = filterOwnership(filings, true)
	case "13g":
		filings = filterOwnership(filings, false)
	}
	if filings == nil {
		filings = []sec.OwnershipRef{}
	}
	if lim := queryLimit(r, 60); lim > 0 && len(filings) > lim {
		filings = filings[:lim]
	}
	// Resolve each filing's subject-company CIK to a ticker so the frontend can
	// link the company name to its stock page. Copy into a fresh slice (don't
	// mutate the shared cache snapshot).
	if s.symbols != nil && len(filings) > 0 {
		enriched := make([]sec.OwnershipRef, len(filings))
		for i, f := range filings {
			if f.Ticker == "" && f.CIK != 0 {
				if sym, ok := s.symbols.ByCIK(f.CIK); ok {
					f.Ticker = sym.Ticker
				}
			}
			enriched[i] = f
		}
		filings = enriched
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(filings), "filings": filings})
}

// filterOwnership keeps only filings matching the activist flag (13D vs 13G).
func filterOwnership(in []sec.OwnershipRef, activist bool) []sec.OwnershipRef {
	out := make([]sec.OwnershipRef, 0, len(in))
	for _, f := range in {
		if f.Activist == activist {
			out = append(out, f)
		}
	}
	return out
}

// getTopics returns the trending-topics snapshot (empty when disabled).
func (s *Server) getTopics(w http.ResponseWriter, _ *http.Request) {
	if s.topics == nil {
		writeJSON(w, http.StatusOK, topics.Snapshot{Window: "24h", Topics: []topics.HotTopic{}})
		return
	}
	writeJSON(w, http.StatusOK, s.topics.Get())
}

// getSocialBatch returns recent social posts merged across several tickers (the
// home "discussion" feed), newest first. Each post keeps its `ticker`.
func (s *Server) getSocialBatch(w http.ResponseWriter, r *http.Request) {
	perTicker := queryLimit(r, 6)
	seen := make(map[string]struct{}) // one post may mention several tickers
	var all []store.Post
	for _, t := range queryTickers(r, maxBarsBatch) {
		posts, err := s.store.ListSocial(r.Context(), t, perTicker)
		if err != nil {
			continue
		}
		for _, p := range posts {
			if _, ok := seen[p.ID]; ok {
				continue
			}
			seen[p.ID] = struct{}{}
			all = append(all, p)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	if len(all) > maxFeed {
		all = all[:maxFeed]
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(all), "posts": all})
}

// getHot returns one trending board, top first. ?board=hot (default) | surging.
// Always 200 with a (possibly empty) list — never null.
func (s *Server) getHot(w http.ResponseWriter, r *http.Request) {
	board := strings.TrimSpace(r.URL.Query().Get("board"))
	if board == "" {
		board = "hot"
	}
	stocks, err := s.store.HotList(r.Context(), board, queryLimit(r, 40))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if stocks == nil {
		stocks = []store.HotStock{} // marshal as [] not null
	}
	// Join a price + day change onto each row — a buzz leaderboard is far more
	// useful when you can see if the hype is riding a rip or a dump without
	// clicking through. Prefer the live universe cache (one in-memory map); fall
	// back to the per-ticker store quote for names the universe sweep hasn't
	// covered yet (it's large and rolls through symbols over time).
	var snap map[string]store.Quote
	if s.universe != nil {
		snap = s.universe.Snapshot()
	}
	for i := range stocks {
		tk := strings.ToUpper(stocks[i].Ticker)
		q, ok := snap[tk]
		if !ok || q.Price <= 0 {
			if sq, found, err := s.store.GetQuote(r.Context(), tk); err == nil && found {
				q, ok = sq, true
			}
		}
		if ok && q.Price > 0 {
			stocks[i].Price = q.Price
			stocks[i].ChangePct = guardedChangePct(q.Price, q.PrevClose)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"board":  board,
		"count":  len(stocks),
		"stocks": stocks,
	})
}

// maxFeed caps how many merged items a home feed returns.
const maxFeed = 40

func (s *Server) getNews(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	items, err := s.store.ListNews(r.Context(), ticker, queryLimit(r, 25))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticker": ticker,
		"count":  len(items),
		"news":   items,
	})
}

func (s *Server) getSocial(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	posts, err := s.store.ListSocial(r.Context(), ticker, queryLimit(r, 30))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticker": ticker,
		"count":  len(posts),
		"posts":  posts,
	})
}

// getSignals returns the per-ticker numeric pulse (buzz / sentiment) from every
// signal source. Always 200 with a (possibly empty) list — never null.
func (s *Server) getSignals(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	sigs, err := s.store.ListSignals(r.Context(), ticker)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if sigs == nil {
		sigs = []store.Signal{} // marshal as [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticker":  ticker,
		"count":   len(sigs),
		"signals": sigs,
	})
}

// getSummary returns an LLM summary of the ticker's recent news + social posts.
// It is an optional feature: when no LLM is configured it responds 503.
// summaryEntry is one cached AI digest (per ticker per ET day).
type summaryEntry struct {
	Summary string    `json:"summary"`
	At      time.Time `json:"generated_at"`
}

// summaryDailyCap bounds LLM digest generations per day across ALL tickers —
// a hard token-budget backstop (cache hits don't count).
const summaryDailyCap = 150

// summaryDay is the cache day key (ET, so it rolls with the trading day).
func summaryDay() string {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01-02")
}

// researchMonth is the per-user deep-research QUOTA period key — the ET calendar
// month ("2006-01" → e.g. "2026-06"), so the free "1 report/user/month" allowance
// rolls over on the 1st of each month in America/New_York. (It deliberately uses
// the same timezone as summaryDay; only the granularity differs — month vs day.)
func researchMonth() string {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01")
}

// researchWeek is the ET ISO-week key ("YYYY-Www") for the per-week free-chat token bucket.
func researchWeek() string {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	y, w := time.Now().In(loc).ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// etLocation returns America/New_York (UTC on load failure) — the timezone all the quota
// windows are keyed to (researchMonth / researchWeek / the reset helpers below).
func etLocation() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}

// nextMonthResetET is the first instant of next month in ET — when the per-month token quota
// (researchMonth) rolls over. The UI shows it as "resets {date}".
func nextMonthResetET() time.Time {
	now := time.Now().In(etLocation())
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return first.AddDate(0, 1, 0)
}

// nextWeekResetET is next Monday 00:00 ET — when the per-week token quota (researchWeek, an ISO
// week starting Monday) rolls over. Always the NEXT Monday, never today.
func nextWeekResetET() time.Time {
	now := time.Now().In(etLocation())
	days := (int(time.Monday) - int(now.Weekday()) + 7) % 7
	if days == 0 {
		days = 7
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return midnight.AddDate(0, 0, days)
}

// getSummary returns the ticker's AI digest in the requested language. The digest is
// generated at most once per (ticker, ET day, lang) and served from memory + a durable
// store (restart-survival); concurrent first requests are deduped.
//
// ASYNC (owner: async + polling — the LLM must never block the page). On a cache/store
// MISS the endpoint returns INSTANTLY with an empty summary + prose_status "generating"
// and composes the digest in a DETACHED background goroutine; the client polls the same
// URL until prose_status flips to a terminal status. 503 only when the LLM is disabled
// (the card hides). prose_status:
//   - "ready"            the summary is final (a cache/store hit, or an empty digest
//     when there's no material — then the card hides).
//   - "generating"       summary "" NOW; a bg gen is in flight → POLL.
//   - "quota_exhausted"  over the daily generation cap → empty, terminal (replaces the
//     old hard 429; the digest is best-effort).
//
// INVARIANTS (mirror getResearchDeep): exactly ONE bg gen per (ticker, ET-day, lang)
// via sumInflight; concurrent polls return "generating" without a duplicate gen or a
// double cap charge; the cap is charged when the gen is reserved and REFUNDED on a
// failed gen; the bg goroutine uses a DETACHED context so the instant response
// returning can't kill it; on success it caches AND persists the digest; the inflight
// gate is always cleared (no goroutine / map leak).
func (s *Server) getSummary(w http.ResponseWriter, r *http.Request) {
	if !s.enrich.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, errBody("llm enrichment is not enabled"))
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	lang := "zh" // Chinese-first default; English UI requests ?lang=en
	if r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	day := summaryDay()
	key := ticker + "|" + day + "|" + lang

	// (1) Memory cache hit → ready, free for everyone.
	s.sumMu.Lock()
	if e, ok := s.sumCache[key]; ok {
		s.sumMu.Unlock()
		s.writeSummary(w, ticker, e, proseStatusReady)
		return
	}
	s.sumMu.Unlock()

	// (2) Cold/restart-survival: a previous process may have persisted today's digest
	// for this (ticker, day, lang). A store hit is a free cache hit — load it into
	// memory + serve "ready", NO LLM. Best-effort: a store error is treated as a miss.
	if raw, ok, err := s.store.GetAISummary(r.Context(), ticker, day, lang); err != nil {
		s.log.Debug("ai_summary store read failed", "ticker", ticker, "day", day, "lang", lang, "err", err)
	} else if ok {
		var e summaryEntry
		if jerr := json.Unmarshal(raw, &e); jerr != nil {
			s.log.Debug("ai_summary store payload decode failed", "ticker", ticker, "day", day, "lang", lang, "err", jerr)
		} else {
			s.sumMu.Lock()
			s.sumCache[key] = e
			s.sumMu.Unlock()
			s.writeSummary(w, ticker, e, proseStatusReady)
			return
		}
	}

	// (3) The digest material is cheap to read; only the LLM call (~12s) must move off
	// the request path. Read news+social now to short-circuit empty-material tickers to
	// a terminal "ready" and to hand the built input to the bg goroutine.
	news, _ := s.store.ListNews(r.Context(), ticker, 10)
	posts, _ := s.store.ListSocial(r.Context(), ticker, 10)

	// Decide under the lock so the single-flight gate + the cap reservation are atomic
	// with the inflight check (no two requests can both become the generator).
	s.sumMu.Lock()
	if s.sumDayDate != day { // day roll: reset the cap + evict yesterday's digests
		s.sumDayDate, s.sumDayCount = day, 0
		for k := range s.sumCache { // key = ticker|day|lang
			if !strings.Contains(k, "|"+day+"|") {
				delete(s.sumCache, k)
			}
		}
	}
	if e, ok := s.sumCache[key]; ok { // a racing request finished a gen
		s.sumMu.Unlock()
		s.writeSummary(w, ticker, e, proseStatusReady)
		return
	}
	if _, busy := s.sumInflight[key]; busy { // a bg gen is in flight → poll-friendly
		s.sumMu.Unlock()
		s.writeSummary(w, ticker, summaryEntry{At: time.Now().UTC()}, proseStatusGenerating)
		return
	}
	if len(news) == 0 && len(posts) == 0 {
		// No material → terminal empty "ready"; cache + persist the emptiness so we
		// skip the LLM next time (and survive restarts). No cap charge.
		e := summaryEntry{Summary: "", At: time.Now().UTC()}
		s.sumCache[key] = e
		s.sumMu.Unlock()
		s.persistSummary(ticker, day, lang, e)
		s.writeSummary(w, ticker, e, proseStatusReady)
		return
	}
	if s.sumDayCount >= summaryDailyCap {
		// Over the daily cap with nothing cached → graceful empty "quota_exhausted"
		// (200, replaces the old hard 429); don't cache so a later day / refund retries.
		s.sumMu.Unlock()
		s.writeSummary(w, ticker, summaryEntry{At: time.Now().UTC()}, proseStatusQuotaExhausted)
		return
	}
	// We are the SOLE generator: reserve the cap slot + the inflight gate, spawn the
	// detached bg goroutine, and return "generating" NOW (off the critical path).
	s.sumDayCount++
	ch := make(chan struct{})
	s.sumInflight[key] = ch
	s.sumMu.Unlock()

	input := summaryInput(ticker, news, posts)
	go s.composeSummaryBackground(ticker, lang, day, key, input, ch)

	s.writeSummary(w, ticker, summaryEntry{At: time.Now().UTC()}, proseStatusGenerating)
}

// composeSummaryBackground composes ONE AI digest off the request path, using a
// DETACHED context (so the already-returned instant response can't cancel it). On a
// successful generation it caches + persists the digest (every later view is a free
// "ready", and it survives restarts) and keeps the reserved cap charged; on a failed
// generation (a real LLM error or timeout) it caches nothing — a later poll/visit
// retries — and REFUNDS the cap. The inflight gate is always released (no goroutine /
// map leak). An empty-but-successful digest is cached as-is (the card then hides).
func (s *Server) composeSummaryBackground(ticker, lang, day, key, input string, ch chan struct{}) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("ai-summary bg compose panicked", "ticker", ticker, "rec", rec)
			s.sumMu.Lock()
			delete(s.sumInflight, key)
			close(ch)
			if s.sumDayDate == day && s.sumDayCount > 0 {
				s.sumDayCount--
			}
			s.sumMu.Unlock()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), llmComposeTimeout)
	summary, err := s.enrich.Summarize(ctx, input, lang)
	cancel()

	var e summaryEntry
	ok := err == nil
	if ok {
		e = summaryEntry{Summary: strings.TrimSpace(summary), At: time.Now().UTC()}
		s.persistSummary(ticker, day, lang, e)
	}
	s.sumMu.Lock()
	if ok {
		s.sumCache[key] = e
	} else if s.sumDayDate == day && s.sumDayCount > 0 {
		s.sumDayCount-- // failed gen → refund the reserved cap slot
	}
	delete(s.sumInflight, key)
	close(ch)
	s.sumMu.Unlock()
}

// writeSummary marshals an AI-digest response plus the explicit prose_status the client
// polls on. Always 200. Back-compat: older clients ignore prose_status and read
// summary/generated_at exactly as before (an empty summary = hide the card).
func (s *Server) writeSummary(w http.ResponseWriter, ticker string, e summaryEntry, status string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ticker":       ticker,
		"summary":      e.Summary,
		"generated_at": e.At,
		"prose_status": status,
	})
}

// persistSummary best-effort writes a completed digest to the durable store (keyed by
// ticker, ET day, lang) so it survives a restart — a later process serves it as a free
// cache hit. Uses a bounded detached context; errors are logged, never fatal.
func (s *Server) persistSummary(ticker, day, lang string, e summaryEntry) {
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.SaveAISummary(ctx, ticker, day, lang, raw); err != nil {
		s.log.Debug("ai_summary store write failed", "ticker", ticker, "day", day, "lang", lang, "err", err)
	}
}

func summaryInput(ticker string, news []store.News, posts []store.Post) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ticker: %s\n\nRecent news headlines:\n", ticker)
	for _, n := range news {
		fmt.Fprintf(&b, "- %s\n", n.Headline)
	}
	b.WriteString("\nRecent social posts:\n")
	for _, p := range posts {
		fmt.Fprintf(&b, "- %s\n", p.Body)
	}
	return b.String()
}

// ── Per-user: overnight digest ("我的隔夜报告") ─────────────────────────────
//
// A personalized morning report over the signed-in user's watchlist: each
// tracked stock's overnight change %, freshest news headline (zh-preferred), and
// next earnings/event, plus an optional AI overview (2-3 zh sentences) distilled
// from that material. Read-only, login-only, never in the sitemap. One assembly +
// at most one LLM call per (user, ET day) — the rest is served from memory.

// digestStock is one watchlist row in the overnight digest: the overnight
// change %, the freshest news headline (with link), and the next earnings/event.
type digestStock struct {
	Ticker    string   `json:"ticker"`
	Name      string   `json:"name"`
	ChangePct *float64 `json:"change_pct"` // null when no prev-close reference
	Headline  string   `json:"headline,omitempty"`
	HeadURL   string   `json:"headline_url,omitempty"`
	NextEvent string   `json:"next_event,omitempty"` // e.g. "财报 11-02 盘后"
}

// digestPayload is the GET /v1/me/digest response body.
type digestPayload struct {
	Date    string        `json:"date"` // ET day, YYYY-MM-DD
	Summary string        `json:"summary"`
	Stocks  []digestStock `json:"stocks"`
	// SummaryStatus tells the client how to treat the AI overview ("Tonight's overview"),
	// which is composed off the request path so the data rows never block on the LLM:
	//   ready        — Summary is final (present, or empty when there was nothing to summarize)
	//   generating   — Pro: the bg compose is running; poll until ready
	//   pro_required — non-Pro: the overview is a Pro feature (show an upgrade card)
	//   unavailable  — the LLM is disabled (hide the overview)
	SummaryStatus string `json:"summary_status"`
}

const (
	digestSummaryReady       = "ready"
	digestSummaryGenerating  = "generating"
	digestSummaryProRequired = "pro_required"
	digestSummaryUnavailable = "unavailable"
)

// digestEntry is one cached digest (per user per ET day per language).
type digestEntry struct {
	payload digestPayload
	at      time.Time
}

// digestMaxTickers caps how many watchlist names the digest assembles (the
// per-ticker quote/news/earnings reads are bounded — a huge watchlist can't fan
// out without limit).
const digestMaxTickers = 25

// getMyDigest returns the signed-in user's personalized overnight report over their
// watchlist. The DATA ROWS are assembled + served INSTANTLY (never blocked on the LLM —
// the old slow path composed the AI overview synchronously, stalling the whole My/Overview
// tab ~12s). The AI overview ("Tonight's overview") is a PRO feature composed off the request
// path: non-Pro → summary_status="pro_required" (no LLM call); Pro → "generating" + a single
// background compose the client polls for, then "ready" from the per-(user,day,lang) cache.
func (s *Server) getMyDigest(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	lang := "zh" // Chinese-first default; English UI requests ?lang=en
	if r.URL.Query().Get("lang") == "en" {
		lang = "en"
	}
	day := summaryDay() // ET trading day, shared with the per-stock digest
	key := u.ID + "|" + day + "|" + lang

	// Fast path: a ready (LLM-composed) payload for today is cached → serve it.
	s.digestMu.Lock()
	if e, ok := s.digestCache[key]; ok && e.payload.SummaryStatus == digestSummaryReady {
		s.digestMu.Unlock()
		writeJSON(w, http.StatusOK, e.payload)
		return
	}
	// Sweep stale days lazily so the cache doesn't grow unbounded over time.
	for k := range s.digestCache {
		if !strings.Contains(k, "|"+day+"|") {
			delete(s.digestCache, k)
		}
	}
	s.digestMu.Unlock()

	tickers, err := s.store.Watchlist(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if len(tickers) > digestMaxTickers {
		tickers = tickers[:digestMaxTickers]
	}
	stocks := s.buildDigestStocks(r.Context(), tickers, lang)

	// Decide the AI-overview status WITHOUT calling the LLM on the request path.
	llmOff := s.enrich == nil || !s.enrich.Enabled()
	material := ""
	if len(stocks) > 0 && !llmOff {
		material = digestMaterial(stocks, lang)
	}
	switch {
	case llmOff || material == "":
		// Nothing to summarize (or LLM off) → terminal, no Pro gate needed.
		writeJSON(w, http.StatusOK, digestPayload{Date: day, Summary: "", Stocks: stocks, SummaryStatus: digestSummaryUnavailable})
		return
	case s.tierOf(r.Context(), u.ID) != tierPro:
		// The AI overview is a Pro feature — non-Pro gets the rows + an upgrade nudge.
		writeJSON(w, http.StatusOK, digestPayload{Date: day, Summary: "", Stocks: stocks, SummaryStatus: digestSummaryProRequired})
		return
	}

	// Pro + material: serve the rows now, compose the overview in the background (single-flight).
	s.digestMu.Lock()
	if e, ok := s.digestCache[key]; ok && e.payload.SummaryStatus == digestSummaryReady {
		s.digestMu.Unlock()
		writeJSON(w, http.StatusOK, e.payload)
		return
	}
	if _, gen := s.digestInflight[key]; !gen {
		ch := make(chan struct{})
		s.digestInflight[key] = ch
		go s.composeDigestBackground(u.ID, lang, day, key, material, stocks, ch)
	}
	s.digestMu.Unlock()
	writeJSON(w, http.StatusOK, digestPayload{Date: day, Summary: "", Stocks: stocks, SummaryStatus: digestSummaryGenerating})
}

// composeDigestBackground composes the AI overview off the request path and caches the
// ready payload (rows + summary) for the day. A failure leaves NO cache entry, so the next
// visit/poll simply retries; single-flight via digestInflight (cleared here). The stocks
// snapshot is captured at request time (the frontend overlays live quotes anyway).
func (s *Server) composeDigestBackground(userID, lang, day, key, material string, stocks []digestStock, ch chan struct{}) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("digest overview bg compose panicked", "user", userID, "rec", rec)
			s.digestMu.Lock()
			delete(s.digestInflight, key)
			close(ch)
			s.digestMu.Unlock()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	text, err := s.enrich.Summarize(ctx, material, lang)
	cancel()

	s.digestMu.Lock()
	defer s.digestMu.Unlock()
	if err == nil {
		payload := digestPayload{Date: day, Summary: strings.TrimSpace(text), Stocks: stocks, SummaryStatus: digestSummaryReady}
		s.digestCache[key] = digestEntry{payload: payload, at: time.Now().UTC()}
	} else {
		s.log.Debug("digest summary failed", "user", userID, "err", err) // no cache → next poll retries
	}
	delete(s.digestInflight, key)
	close(ch)
}

// buildDigestStocks assembles one row per watchlist ticker: overnight change %
// (quote PrevClose reference, with the same plausibility band as the briefing),
// the freshest news headline (zh-preferred), and the next earnings/event. Always
// returns a non-nil slice; per-ticker read failures degrade to partial rows.
func (s *Server) buildDigestStocks(ctx context.Context, tickers []string, lang string) []digestStock {
	stocks := make([]digestStock, 0, len(tickers))
	for _, tk := range tickers {
		tk = strings.ToUpper(strings.TrimSpace(tk))
		if tk == "" {
			continue
		}
		row := digestStock{Ticker: tk}

		// Name (best-effort; falls back to the bare ticker).
		if sec, ok, err := s.store.GetSecurity(ctx, tk); err == nil && ok && sec.Name != "" {
			row.Name = sec.Name
		}

		// Overnight change %: the latest all-session quote vs its prev close.
		if q, ok, err := s.store.GetQuote(ctx, tk); err == nil && ok {
			if q.PrevClose > 0 && q.Price > 0 {
				chg := (q.Price - q.PrevClose) / q.PrevClose * 100
				if chg <= 300 && chg >= -95 { // reject delayed-data split artifacts
					c := chg
					row.ChangePct = &c
				}
			}
		}

		// Freshest news headline (zh-preferred), with a link to the original.
		if news, err := s.store.ListNews(ctx, tk, 1); err == nil && len(news) > 0 {
			n := news[0]
			h := n.Headline
			if lang != "en" && n.HeadlineZH != "" {
				h = n.HeadlineZH
			}
			row.Headline = h
			row.HeadURL = n.URL
		}

		// Next earnings/event (nearest upcoming, else most recent).
		if s.earnings != nil {
			if es, err := s.earnings.ListEarningsForTicker(ctx, tk, 8); err == nil {
				row.NextEvent = nextEarningsLabel(es, lang)
			}
		}

		stocks = append(stocks, row)
	}
	return stocks
}

// nextEarningsLabel picks the nearest upcoming earnings date (else the most
// recent past one) and renders a short bilingual label, e.g. "财报 11-02 盘后" /
// "Earnings 11-02 AMC". Empty when there's no dated row.
func nextEarningsLabel(es []store.Earning, lang string) string {
	if len(es) == 0 {
		return ""
	}
	now := time.Now().UTC()
	var best store.Earning
	var bestSet, upcoming bool
	for _, e := range es {
		if e.Date.IsZero() {
			continue
		}
		isUp := !e.Date.Before(now.Truncate(24 * time.Hour))
		switch {
		case !bestSet:
			best, bestSet, upcoming = e, true, isUp
		case isUp && !upcoming: // prefer the first upcoming over any past
			best, upcoming = e, true
		case isUp && upcoming && e.Date.Before(best.Date): // nearest upcoming
			best = e
		case !isUp && !upcoming && e.Date.After(best.Date): // most recent past
			best = e
		}
	}
	if !bestSet {
		return ""
	}
	hourEN := map[string]string{"bmo": "BMO", "amc": "AMC", "dmh": "DMH"}[best.Hour]
	hourZH := map[string]string{"bmo": "盘前", "amc": "盘后", "dmh": "盘中"}[best.Hour]
	date := best.Date.Format("01-02")
	if lang == "en" {
		if hourEN != "" {
			return "Earnings " + date + " " + hourEN
		}
		return "Earnings " + date
	}
	if hourZH != "" {
		return "财报 " + date + " " + hourZH
	}
	return "财报 " + date
}

// digestMaterial formats the assembled watchlist rows into compact LLM input for
// the overnight overview, in the requested language. Returns "" when no row has
// anything worth summarizing (so the LLM call is skipped).
func digestMaterial(stocks []digestStock, lang string) string {
	var b strings.Builder
	any := false
	if lang == "en" {
		b.WriteString("My watchlist — overnight snapshot:\n")
	} else {
		b.WriteString("我的自选股隔夜快照:\n")
	}
	for _, st := range stocks {
		name := st.Ticker
		if st.Name != "" {
			name = st.Ticker + " (" + st.Name + ")"
		}
		fmt.Fprintf(&b, "- %s", name)
		if st.ChangePct != nil {
			fmt.Fprintf(&b, " %+.2f%%", *st.ChangePct)
			any = true
		}
		if st.Headline != "" {
			if lang == "en" {
				fmt.Fprintf(&b, " | news: %s", st.Headline)
			} else {
				fmt.Fprintf(&b, " | 新闻:%s", st.Headline)
			}
			any = true
		}
		if st.NextEvent != "" {
			fmt.Fprintf(&b, " | %s", st.NextEvent)
			any = true
		}
		b.WriteByte('\n')
	}
	if !any {
		return ""
	}
	return b.String()
}

// getStream serves live quote updates as Server-Sent Events.
func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ch, unsubscribe := s.hub.Subscribe()
	defer unsubscribe()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case q, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(q)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: quote\ndata: %s\n\n", b)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func queryLimit(r *http.Request, def int) int {
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }
