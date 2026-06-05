// Package api exposes the HTTP/JSON surface (stdlib net/http only).
package api

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/clip"
	"github.com/wombow-ai/tickwind/internal/store"
)

// QuoteStream is the subset of the live hub the API needs to stream prices.
type QuoteStream interface {
	Subscribe() (<-chan store.Quote, func())
}

type Server struct {
	store store.Store
	hub   QuoteStream
	clip  *clip.Fetcher
	log   *slog.Logger
}

func New(st store.Store, hub QuoteStream, log *slog.Logger) http.Handler {
	s := &Server{store: st, hub: hub, clip: clip.NewFetcher(), log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/stocks/{ticker}", s.getStock)
	mux.HandleFunc("GET /v1/stocks/{ticker}/filings", s.getFilings)
	mux.HandleFunc("GET /v1/stocks/{ticker}/quote", s.getQuote)
	mux.HandleFunc("GET /v1/stocks/{ticker}/news", s.getNews)
	mux.HandleFunc("GET /v1/stocks/{ticker}/social", s.getSocial)
	mux.HandleFunc("POST /v1/stocks/{ticker}/clip", s.postClip)
	mux.HandleFunc("GET /v1/stream", s.getStream)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).String())
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "tickwind"})
}

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

// postClip saves a pasted link to the ticker's feed as a clip Post. It fetches
// the page title (best-effort) and stores it under source "clip".
func (s *Server) postClip(w http.ResponseWriter, r *http.Request) {
	ticker := r.PathValue("ticker")

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

	h := fnv.New64a()
	_, _ = h.Write([]byte(link))
	post := store.Post{
		Ticker:    ticker,
		ID:        fmt.Sprintf("clip:%x", h.Sum64()),
		Source:    "clip",
		Author:    "you",
		Body:      title,
		URL:       link,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.SaveSocial(r.Context(), ticker, []store.Post{post}); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, post)
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

	// Flush headers immediately so the client (and any proxy) sees the stream
	// open right away, rather than waiting for the first event.
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

// queryLimit reads a positive ?limit= value, falling back to def.
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
