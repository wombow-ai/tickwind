// Package api exposes the HTTP/JSON surface (stdlib net/http only).
//
// Public endpoints (market data) are open so the public stock pages can be
// crawled/shared; per-user endpoints (watchlist, clips) require a valid
// Supabase JWT and are scoped to the caller's user id.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/clip"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/topics"
)

// QuoteStream is the subset of the live hub the API needs to stream prices.
type QuoteStream interface {
	Subscribe() (<-chan store.Quote, func())
}

// BarSource provides recent daily closing prices for a ticker's sparkline. It
// may return a nil slice when no data is available; nil itself disables the
// bars endpoint (returns an empty series).
type BarSource interface {
	DailyBars(ctx context.Context, ticker string) ([]float64, error)
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

type Server struct {
	store    store.Store
	hub      QuoteStream
	clip     *clip.Fetcher
	enrich   enrich.Enricher
	auth     *auth.Verifier
	bars     BarSource
	topics   TopicSource
	opps     OpportunitySource
	gurus    GuruSource
	ingestor TickerIngestor
	log      *slog.Logger
}

func New(st store.Store, hub QuoteStream, enricher enrich.Enricher, verifier *auth.Verifier, bars BarSource, topicSrc TopicSource, oppSrc OpportunitySource, guruSrc GuruSource, ingestor TickerIngestor, log *slog.Logger) http.Handler {
	s := &Server{store: st, hub: hub, clip: clip.NewFetcher(), enrich: enricher, auth: verifier, bars: bars, topics: topicSrc, opps: oppSrc, gurus: guruSrc, ingestor: ingestor, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)

	// Per-user (auth required)
	mux.HandleFunc("GET /v1/watchlist", s.getWatchlist)
	mux.HandleFunc("POST /v1/watchlist", s.postWatchlist)
	mux.HandleFunc("DELETE /v1/watchlist/{ticker}", s.deleteWatchlist)
	mux.HandleFunc("POST /v1/stocks/{ticker}/clip", s.postClip)
	mux.HandleFunc("GET /v1/stocks/{ticker}/clips", s.getClips)

	// Public (market data — open for SEO / shareable stock pages)
	mux.HandleFunc("GET /v1/stocks/{ticker}", s.getStock)
	mux.HandleFunc("GET /v1/stocks/{ticker}/filings", s.getFilings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/quote", s.getQuote)
	mux.HandleFunc("GET /v1/stocks/{ticker}/bars", s.getBars)
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
	mux.HandleFunc("GET /v1/gurus", s.getGurus)
	mux.HandleFunc("GET /v1/stream", s.getStream)

	// auth.Middleware attaches the user when a valid bearer token is present;
	// the outer middleware adds CORS + logging.
	return s.middleware(verifier.Middleware(mux))
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
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

// ── Public: market data ──────────────────────────────────────────────────

func (s *Server) getStock(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	sec, ok, err := s.store.GetSecurity(r.Context(), ticker)
	switch {
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
	case !ok:
		writeJSON(w, http.StatusNotFound, errBody("not tracked yet: "+ticker))
	default:
		writeJSON(w, http.StatusOK, sec)
	}
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

func (s *Server) getQuote(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")
	q, ok, err := s.store.GetQuote(r.Context(), ticker)
	switch {
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
	case !ok:
		writeJSON(w, http.StatusNotFound, errBody("no quote yet: "+ticker))
	default:
		writeJSON(w, http.StatusOK, q)
	}
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
