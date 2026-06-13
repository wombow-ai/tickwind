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
	"github.com/wombow-ai/tickwind/internal/cashtag"
	"github.com/wombow-ai/tickwind/internal/clip"
	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/congress/ptr"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/events"
	"github.com/wombow-ai/tickwind/internal/finra"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/nasdaq"
	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/ratecut"
	"github.com/wombow-ai/tickwind/internal/sec"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
	"github.com/wombow-ai/tickwind/internal/thirteenf"
	"github.com/wombow-ai/tickwind/internal/topics"
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
	Len() int
	UpdatedAt() time.Time
}

// GuruSource provides the latest Guru-watch rail (curated-KOL posts). nil →
// empty list.
type GuruSource interface {
	Get() []guru.Item
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
// strip (nil-safe; nil = empty). Satisfied by *ingest.IndicesCache.
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

type Server struct {
	store         store.Store
	hub           QuoteStream
	clip          *clip.Fetcher
	enrich        enrich.Enricher
	auth          *auth.Verifier
	bars          BarSource
	topics        TopicSource
	opps          OpportunitySource
	universe      UniverseSource
	gurus         GuruSource
	ingestor      TickerIngestor
	symbols       SymbolSearcher
	events        EventSource
	fundamentals  FundamentalsSource
	earnings      EarningsSource
	congress      CongressSource
	institutional InstitutionalSource
	live          LiveSubscriber
	indices       IndicesSource
	short         ShortSource
	briefing      BriefingSource
	options       OptionsSource
	thirteenf     ThirteenFSource
	shortVolume   ShortVolumeSource      // injected post-New via SetShortVolume (avoids growing the New signature)
	sentiment     SentimentSource        // injected post-New via SetSentiment
	rateCut       RateCutSource          // injected post-New via SetRateCut
	congressTx    CongressTxSource       // injected post-New via SetCongressTx
	ipo           IPOSource              // injected post-New via SetIPO
	indicators    IndicatorSource        // injected post-New via SetIndicators (static catalog)
	indicatorCalc IndicatorComputeSource // injected post-New via SetIndicatorCompute (per-stock compute)
	admins        map[string]bool        // user UUIDs and/or emails (lowercased) allowed to delete any comment
	commentRL     *rateLimiter           // per-user comment-post throttle
	// AI digest cache: one LLM generation per (ticker, ET day), then served from
	// memory — token spend stays bounded no matter the traffic. Guarded by sumMu;
	// sumInflight dedupes concurrent first requests; sumDay* enforce a global
	// per-day generation cap.
	sumMu       sync.Mutex
	sumCache    map[string]summaryEntry
	sumInflight map[string]chan struct{}
	sumDayDate  string
	sumDayCount int
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
	log         *slog.Logger
	handler     http.Handler // the assembled mux + middleware chain (set in New)
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
	s := &Server{store: st, hub: hub, clip: clip.NewFetcher(), enrich: enricher, auth: verifier, bars: bars, topics: topicSrc, opps: oppSrc, universe: universeSrc, gurus: guruSrc, ingestor: ingestor, symbols: symbolSrc, events: eventSrc, fundamentals: fundSrc, earnings: earningsSrc, congress: congressSrc, institutional: institutionalSrc, live: liveSub, indices: indicesSrc, short: shortSrc, briefing: briefingSrc, options: optionsSrc, thirteenf: thirteenfSrc, admins: admins, commentRL: newRateLimiter(10, 10*time.Minute), sumCache: map[string]summaryEntry{}, sumInflight: map[string]chan struct{}{}, btCache: map[string]backtestEntry{}, digestCache: map[string]digestEntry{}, log: log}
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
	mux.HandleFunc("GET /v1/comments", s.getComments) // public read
	mux.HandleFunc("POST /v1/comments", s.postComment)
	mux.HandleFunc("PATCH /v1/comments/{id}", s.patchComment)
	mux.HandleFunc("DELETE /v1/comments/{id}", s.deleteComment)
	mux.HandleFunc("POST /v1/comments/{id}/report", s.reportComment)
	mux.HandleFunc("POST /v1/comments/{id}/like", s.likeComment)

	// Public (market data — open for SEO / shareable stock pages)
	mux.HandleFunc("GET /v1/stocks/{ticker}", s.getStock)
	mux.HandleFunc("GET /v1/stocks/{ticker}/filings", s.getFilings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/quote", s.getQuote)
	mux.HandleFunc("POST /v1/stocks/{ticker}/subscribe", s.subscribeLive)
	mux.HandleFunc("GET /v1/stocks/{ticker}/bars", s.getBars)
	mux.HandleFunc("GET /v1/stocks/{ticker}/candles", s.getCandles)
	mux.HandleFunc("GET /v1/stocks/{ticker}/fundamentals", s.getFundamentals)
	mux.HandleFunc("GET /v1/stocks/{ticker}/news", s.getNews)
	mux.HandleFunc("GET /v1/stocks/{ticker}/social", s.getSocial)
	mux.HandleFunc("GET /v1/stocks/{ticker}/signals", s.getSignals)
	mux.HandleFunc("GET /v1/stocks/{ticker}/earnings", s.getStockEarnings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/summary", s.getSummary)
	mux.HandleFunc("GET /v1/bars", s.getBarsBatch)
	mux.HandleFunc("GET /v1/news", s.getNewsBatch)
	mux.HandleFunc("GET /v1/social", s.getSocialBatch)
	mux.HandleFunc("GET /v1/hot", s.getHot)
	mux.HandleFunc("GET /v1/topics", s.getTopics)
	mux.HandleFunc("GET /v1/opportunities", s.getOpportunities)
	mux.HandleFunc("GET /v1/universe", s.getUniverse)
	mux.HandleFunc("GET /v1/screen", s.getScreen)
	mux.HandleFunc("GET /v1/gurus", s.getGurus)
	mux.HandleFunc("GET /v1/search", s.getSearch)
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
	mux.HandleFunc("GET /v1/ipo", s.getIPO)
	mux.HandleFunc("GET /v1/stocks/{ticker}/options", s.getOptions)
	mux.HandleFunc("GET /v1/options/unusual", s.getUnusualOptions)
	mux.HandleFunc("GET /v1/13f", s.getThirteenF)
	mux.HandleFunc("GET /v1/13f/{slug}", s.getThirteenFFund)
	mux.HandleFunc("GET /v1/stocks/{ticker}/whales", s.getWhales)
	mux.HandleFunc("GET /v1/stocks/{ticker}/indicators", s.getStockIndicators)
	mux.HandleFunc("GET /v1/indicators", s.getIndicators)
	mux.HandleFunc("GET /v1/stream", s.getStream)

	// auth.Middleware attaches the user when a valid bearer token is present;
	// the outer middleware adds CORS + logging.
	s.handler = s.middleware(verifier.Middleware(mux))
	return s
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
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

// validAlertKinds gates the alert types the evaluator (added next) understands.
var validAlertKinds = map[string]bool{
	"price_above": true, "price_below": true, "pct_move": true, "new_filing": true,
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
	if req.Kind != "new_filing" && req.Threshold <= 0 {
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

// fundamentalsResp embeds the reported XBRL figures and adds the price-derived
// metrics, which are null when not computable (e.g. P/E for a loss-maker).
type fundamentalsResp struct {
	edgar.Fundamentals
	Price     float64  `json:"price"`
	MarketCap *float64 `json:"market_cap"`
	PE        *float64 `json:"pe"`
	PB        *float64 `json:"pb"`
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
		if f.EPSDiluted > 0 { // P/E only meaningful for positive earnings
			pe := resp.Price / f.EPSDiluted
			resp.PE = &pe
		}
		if f.Equity > 0 && f.Shares > 0 {
			if bvps := f.Equity / float64(f.Shares); bvps > 0 {
				pb := resp.Price / bvps
				resp.PB = &pb
			}
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

// getBars returns recent daily closing prices for a sparkline. It degrades
// gracefully to an empty series (HTTP 200) when bars are unavailable, so the
// frontend simply renders nothing rather than erroring.
func (s *Server) getBars(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.PathValue("ticker")))
	closes := []float64{}
	if s.bars != nil {
		if got, err := s.bars.DailyBars(r.Context(), ticker); err != nil {
			s.log.Debug("bars fetch failed", "ticker", ticker, "err", err)
		} else if got != nil {
			closes = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "closes": closes})
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
	if s.gurus != nil {
		rail = s.gurus.Get()
	}
	if rail == nil {
		rail = []guru.Item{}
	}
	if lim := queryLimit(r, 0); lim > 0 && len(rail) > lim {
		rail = rail[:lim]
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(rail), "items": rail})
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
	writeJSON(w, http.StatusOK, map[string]any{"count": len(earnings), "earnings": earnings})
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

// getSummary returns the ticker's AI digest, generated at most once per ET day
// (first visitor pays the LLM call, everyone else hits the cache; concurrent
// first requests are deduped). 503 when no LLM is configured, 429 past the
// daily generation cap.
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

	for {
		s.sumMu.Lock()
		if e, ok := s.sumCache[key]; ok {
			s.sumMu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"ticker": ticker, "summary": e.Summary, "generated_at": e.At,
			})
			return
		}
		ch, busy := s.sumInflight[key]
		if !busy {
			break // we'll generate
		}
		s.sumMu.Unlock()
		select { // someone else is generating: wait, then re-check the cache
		case <-ch:
		case <-r.Context().Done():
			return
		}
	}
	// We hold sumMu and are the generator for this key.
	if s.sumDayDate != day {
		s.sumDayDate, s.sumDayCount = day, 0
		for k := range s.sumCache { // yesterday's digests are dead weight
			if !strings.Contains(k, "|"+day+"|") { // key = ticker|day|lang
				delete(s.sumCache, k)
			}
		}
	}
	if s.sumDayCount >= summaryDailyCap {
		s.sumMu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, errBody("daily AI digest budget reached — try again tomorrow"))
		return
	}
	s.sumDayCount++
	ch := make(chan struct{})
	s.sumInflight[key] = ch
	s.sumMu.Unlock()

	finish := func(e *summaryEntry) {
		s.sumMu.Lock()
		if e != nil {
			s.sumCache[key] = *e
		}
		delete(s.sumInflight, key)
		close(ch)
		s.sumMu.Unlock()
	}

	news, _ := s.store.ListNews(r.Context(), ticker, 10)
	posts, _ := s.store.ListSocial(r.Context(), ticker, 10)
	input := summaryInput(ticker, news, posts)
	if len(news) == 0 && len(posts) == 0 {
		e := summaryEntry{Summary: "", At: time.Now().UTC()} // cache the emptiness too
		finish(&e)
		writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "summary": "", "generated_at": e.At})
		return
	}
	summary, err := s.enrich.Summarize(r.Context(), input, lang)
	if err != nil {
		s.sumMu.Lock()
		s.sumDayCount-- // failed generation shouldn't burn budget
		s.sumMu.Unlock()
		finish(nil)
		writeJSON(w, http.StatusBadGateway, errBody(err.Error()))
		return
	}
	e := summaryEntry{Summary: summary, At: time.Now().UTC()}
	finish(&e)
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "summary": e.Summary, "generated_at": e.At})
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
}

// digestEntry is one cached digest (per user per ET day per language).
type digestEntry struct {
	payload digestPayload
	at      time.Time
}

// digestMaxTickers caps how many watchlist names the digest assembles (the
// per-ticker quote/news/earnings reads are bounded — a huge watchlist can't fan
// out without limit).
const digestMaxTickers = 25

// getMyDigest returns the signed-in user's personalized overnight report over
// their watchlist. Generated at most once per (user, ET day, language) and served
// from memory after; an empty watchlist yields {stocks:[]} with 200; the LLM
// overview is best-effort (empty summary when the LLM is off / fails) and the
// data rows always populate regardless.
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

	s.digestMu.Lock()
	if e, ok := s.digestCache[key]; ok {
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
	payload := digestPayload{Date: day, Summary: "", Stocks: stocks}

	// AI overview is best-effort: when the LLM is enabled and there's material,
	// distill a short zh/en综述 from the assembled rows. A failure (or disabled
	// LLM) leaves Summary empty — the data rows still serve.
	if len(stocks) > 0 && s.enrich != nil && s.enrich.Enabled() {
		if material := digestMaterial(stocks, lang); material != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
			if text, err := s.enrich.Summarize(ctx, material, lang); err != nil {
				s.log.Debug("digest summary failed", "user", u.ID, "err", err)
			} else {
				payload.Summary = strings.TrimSpace(text)
			}
			cancel()
		}
	}

	e := digestEntry{payload: payload, at: time.Now().UTC()}
	s.digestMu.Lock()
	s.digestCache[key] = e
	s.digestMu.Unlock()
	writeJSON(w, http.StatusOK, payload)
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
