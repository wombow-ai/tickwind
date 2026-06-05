// Package memory is a thread-safe in-memory store.Store implementation.
// It lets Tickwind run with zero infra (no Docker/Postgres) during early dev.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/wombow-ai/tickwind/internal/store"
)

type Store struct {
	mu      sync.RWMutex
	secs    map[string]store.Security
	filings map[string]map[string]store.Filing // ticker -> accessionNo -> Filing
	quotes  map[string]store.Quote             // ticker -> latest quote
	news    map[string]map[string]store.News   // ticker -> id -> News
}

func New() *Store {
	return &Store{
		secs:    make(map[string]store.Security),
		filings: make(map[string]map[string]store.Filing),
		quotes:  make(map[string]store.Quote),
		news:    make(map[string]map[string]store.News),
	}
}

func key(ticker string) string { return strings.ToUpper(ticker) }

func (s *Store) UpsertSecurity(_ context.Context, sec store.Security) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secs[key(sec.Ticker)] = sec
	return nil
}

func (s *Store) GetSecurity(_ context.Context, ticker string) (store.Security, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sec, ok := s.secs[key(ticker)]
	return sec, ok, nil
}

func (s *Store) SaveFilings(_ context.Context, ticker string, filings []store.Filing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(ticker)
	m := s.filings[k]
	if m == nil {
		m = make(map[string]store.Filing)
		s.filings[k] = m
	}
	for _, f := range filings {
		m[f.AccessionNo] = f // dedupe by accession number
	}
	return nil
}

func (s *Store) ListFilings(_ context.Context, ticker string, limit int) ([]store.Filing, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.filings[key(ticker)]
	out := make([]store.Filing, 0, len(m))
	for _, f := range m {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FiledAt.After(out[j].FiledAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) UpsertQuote(_ context.Context, q store.Quote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotes[key(q.Ticker)] = q
	return nil
}

func (s *Store) GetQuote(_ context.Context, ticker string) (store.Quote, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.quotes[key(ticker)]
	return q, ok, nil
}

func (s *Store) SaveNews(_ context.Context, ticker string, items []store.News) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(ticker)
	m := s.news[k]
	if m == nil {
		m = make(map[string]store.News)
		s.news[k] = m
	}
	for _, n := range items {
		m[n.ID] = n // dedupe by id
	}
	return nil
}

func (s *Store) ListNews(_ context.Context, ticker string, limit int) ([]store.News, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.news[key(ticker)]
	out := make([]store.News, 0, len(m))
	for _, n := range m {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
