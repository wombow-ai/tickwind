// Package memory is a thread-safe in-memory store.Store implementation.
// It lets Tickwind run with zero infra (no Docker/Postgres) during early dev.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

type Store struct {
	mu        sync.RWMutex
	secs      map[string]store.Security
	filings   map[string]map[string]store.Filing // ticker -> accessionNo -> Filing
	quotes    map[string]store.Quote             // ticker -> latest quote
	news      map[string]map[string]store.News   // ticker -> id -> News
	social    map[string]map[string]store.Post   // ticker -> id -> Post
	signals   map[string]map[string]store.Signal // ticker -> source -> Signal
	hot       map[string][]store.HotStock        // board -> ranked snapshot
	insiders  map[string]store.InsiderBuy        // accession -> insider buy
	seenF4    map[string]time.Time               // form-4 accession -> filed date
	watchlist map[string][]string                // userID -> ordered tickers
	clips     map[string]map[string]store.Clip   // userID -> clipID -> Clip
}

func New() *Store {
	return &Store{
		secs:      make(map[string]store.Security),
		filings:   make(map[string]map[string]store.Filing),
		quotes:    make(map[string]store.Quote),
		news:      make(map[string]map[string]store.News),
		social:    make(map[string]map[string]store.Post),
		signals:   make(map[string]map[string]store.Signal),
		hot:       make(map[string][]store.HotStock),
		insiders:  make(map[string]store.InsiderBuy),
		seenF4:    make(map[string]time.Time),
		watchlist: make(map[string][]string),
		clips:     make(map[string]map[string]store.Clip),
	}
}

func key(ticker string) string { return strings.ToUpper(strings.TrimSpace(ticker)) }

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
	return limited(out, limit), nil
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
	return limited(out, limit), nil
}

func (s *Store) SaveSocial(_ context.Context, ticker string, posts []store.Post) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(ticker)
	m := s.social[k]
	if m == nil {
		m = make(map[string]store.Post)
		s.social[k] = m
	}
	for _, p := range posts {
		m[p.ID] = p // dedupe by id
	}
	return nil
}

func (s *Store) ListSocial(_ context.Context, ticker string, limit int) ([]store.Post, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.social[key(ticker)]
	out := make([]store.Post, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return limited(out, limit), nil
}

func (s *Store) SaveSignals(_ context.Context, signals []store.Signal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sig := range signals {
		k := key(sig.Ticker)
		if k == "" || sig.Source == "" {
			continue
		}
		m := s.signals[k]
		if m == nil {
			m = make(map[string]store.Signal)
			s.signals[k] = m
		}
		m[sig.Source] = sig // one row per (ticker, source)
	}
	return nil
}

func (s *Store) ListSignals(_ context.Context, ticker string) ([]store.Signal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.signals[key(ticker)]
	out := make([]store.Signal, 0, len(m))
	for _, sig := range m {
		out = append(out, sig)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out, nil
}

func (s *Store) SaveHotList(_ context.Context, board string, stocks []store.HotStock) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hot[board] = append([]store.HotStock(nil), stocks...) // replace this board's snapshot
	return nil
}

func (s *Store) HotList(_ context.Context, board string, limit int) ([]store.HotStock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]store.HotStock(nil), s.hot[board]...)
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return limited(out, limit), nil
}

func (s *Store) SaveInsiderBuys(_ context.Context, buys []store.InsiderBuy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range buys {
		if b.Accession == "" {
			continue
		}
		s.insiders[b.Accession] = b // upsert by accession
	}
	return nil
}

func (s *Store) RecentInsiderBuys(_ context.Context, since time.Time) ([]store.InsiderBuy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.InsiderBuy, 0)
	for _, b := range s.insiders {
		if !b.FiledDate.Before(since) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FiledDate.After(out[j].FiledDate) })
	return out, nil
}

func (s *Store) MarkForm4Seen(_ context.Context, accessions []string, filedDate time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range accessions {
		if a != "" {
			s.seenF4[a] = filedDate
		}
	}
	return nil
}

func (s *Store) SeenForm4Since(_ context.Context, since time.Time) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0)
	for a, d := range s.seenF4 {
		if !d.Before(since) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Store) Watchlist(_ context.Context, userID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.watchlist[userID]...), nil
}

func (s *Store) AllWatchlistTickers(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]struct{})
	var out []string
	for _, tickers := range s.watchlist {
		for _, t := range tickers {
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				out = append(out, t)
			}
		}
	}
	return out, nil
}

func (s *Store) AddToWatchlist(_ context.Context, userID, ticker string) error {
	t := key(ticker)
	if t == "" || userID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.watchlist[userID] {
		if x == t {
			return nil
		}
	}
	s.watchlist[userID] = append(s.watchlist[userID], t)
	return nil
}

func (s *Store) RemoveFromWatchlist(_ context.Context, userID, ticker string) error {
	t := key(ticker)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.watchlist[userID]
	next := make([]string, 0, len(cur))
	for _, x := range cur {
		if x != t {
			next = append(next, x)
		}
	}
	s.watchlist[userID] = next
	return nil
}

func (s *Store) SaveClip(_ context.Context, c store.Clip) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.clips[c.UserID]
	if m == nil {
		m = make(map[string]store.Clip)
		s.clips[c.UserID] = m
	}
	m[c.ID] = c
	return nil
}

func (s *Store) ListClips(_ context.Context, userID, ticker string, limit int) ([]store.Clip, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := key(ticker)
	out := make([]store.Clip, 0)
	for _, c := range s.clips[userID] {
		if key(c.Ticker) == k {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return limited(out, limit), nil
}

// limited returns the first limit elements (limit <= 0 means all).
func limited[T any](s []T, limit int) []T {
	if limit > 0 && len(s) > limit {
		return s[:limit]
	}
	return s
}
