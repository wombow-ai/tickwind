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
	"github.com/wombow-ai/tickwind/internal/clip"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/events"
	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
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
// ticker), nil-safe — powers the /v1/universe status (and later a cold-price fast
// path + the screener).
type UniverseSource interface {
	Get(ticker string) (store.Quote, bool)
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
}

// EventSource provides the latest major-events timeline. nil → empty list.
type EventSource interface {
	Get() []events.Event
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
	admins       map[string]bool // user UUIDs and/or emails (lowercased) allowed to delete any comment
	commentRL    *rateLimiter    // per-user comment-post throttle
	log          *slog.Logger
}

func New(st store.Store, hub QuoteStream, enricher enrich.Enricher, verifier *auth.Verifier, bars BarSource, topicSrc TopicSource, oppSrc OpportunitySource, universeSrc UniverseSource, guruSrc GuruSource, ingestor TickerIngestor, symbolSrc SymbolSearcher, eventSrc EventSource, fundSrc FundamentalsSource, adminIDs []string, log *slog.Logger) http.Handler {
	admins := make(map[string]bool, len(adminIDs))
	for _, id := range adminIDs {
		if id = strings.ToLower(strings.TrimSpace(id)); id != "" {
			admins[id] = true
		}
	}
	s := &Server{store: st, hub: hub, clip: clip.NewFetcher(), enrich: enricher, auth: verifier, bars: bars, topics: topicSrc, opps: oppSrc, universe: universeSrc, gurus: guruSrc, ingestor: ingestor, symbols: symbolSrc, events: eventSrc, fundamentals: fundSrc, admins: admins, commentRL: newRateLimiter(10, 10*time.Minute), log: log}
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
	mux.HandleFunc("GET /v1/holdings", s.getHoldings)
	mux.HandleFunc("POST /v1/holdings", s.postHolding)
	mux.HandleFunc("DELETE /v1/holdings/{id}", s.deleteHolding)
	mux.HandleFunc("GET /v1/comments", s.getComments) // public read
	mux.HandleFunc("POST /v1/comments", s.postComment)
	mux.HandleFunc("DELETE /v1/comments/{id}", s.deleteComment)
	mux.HandleFunc("POST /v1/comments/{id}/report", s.reportComment)

	// Public (market data — open for SEO / shareable stock pages)
	mux.HandleFunc("GET /v1/stocks/{ticker}", s.getStock)
	mux.HandleFunc("GET /v1/stocks/{ticker}/filings", s.getFilings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/quote", s.getQuote)
	mux.HandleFunc("GET /v1/stocks/{ticker}/bars", s.getBars)
	mux.HandleFunc("GET /v1/stocks/{ticker}/candles", s.getCandles)
	mux.HandleFunc("GET /v1/stocks/{ticker}/fundamentals", s.getFundamentals)
	mux.HandleFunc("GET /v1/stocks/{ticker}/news", s.getNews)
	mux.HandleFunc("GET /v1/stocks/{ticker}/social", s.getSocial)
	mux.HandleFunc("GET /v1/stocks/{ticker}/signals", s.getSignals)
	mux.HandleFunc("GET /v1/stocks/{ticker}/summary", s.getSummary)
	mux.HandleFunc("GET /v1/bars", s.getBarsBatch)
	mux.HandleFunc("GET /v1/news", s.getNewsBatch)
	mux.HandleFunc("GET /v1/social", s.getSocialBatch)
	mux.HandleFunc("GET /v1/hot", s.getHot)
	mux.HandleFunc("GET /v1/topics", s.getTopics)
	mux.HandleFunc("GET /v1/opportunities", s.getOpportunities)
	mux.HandleFunc("GET /v1/universe", s.getUniverse)
	mux.HandleFunc("GET /v1/gurus", s.getGurus)
	mux.HandleFunc("GET /v1/search", s.getSearch)
	mux.HandleFunc("GET /v1/events", s.getEvents)
	mux.HandleFunc("GET /v1/stream", s.getStream)

	// auth.Middleware attaches the user when a valid bearer token is present;
	// the outer middleware adds CORS + logging.
	return s.middleware(verifier.Middleware(mux))
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

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "tickwind"})
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
	comments, err := s.store.ListComments(r.Context(), ticker, queryLimit(r, 100))
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
	if !ok && s.bars != nil {
		// Not polled (a stock the user just navigated to): fetch a quote on demand
		// so the price shows alongside the on-demand candles (fixes K-line present
		// but price blank). Errors degrade to the 404 below.
		if oq, found, qerr := s.bars.LatestQuote(r.Context(), ticker); qerr == nil && found {
			q, ok = oq, true
		}
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("no quote yet: "+ticker))
		return
	}
	writeJSON(w, http.StatusOK, q)
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

// maxBarsBatch caps how many tickers one batched request (bars/news/social)
// will resolve.
const maxBarsBatch = 30

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
func (s *Server) getSummary(w http.ResponseWriter, r *http.Request) {
	if !s.enrich.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, errBody("llm enrichment is not enabled"))
		return
	}
	ticker := r.PathValue("ticker")
	news, _ := s.store.ListNews(r.Context(), ticker, 10)
	posts, _ := s.store.ListSocial(r.Context(), ticker, 10)
	input := summaryInput(ticker, news, posts)
	if strings.TrimSpace(input) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "summary": ""})
		return
	}
	summary, err := s.enrich.Summarize(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticker": ticker, "summary": summary})
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
