package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// Compile-time guard: *memory.Store satisfies the optional Pruner capability.
var _ store.Pruner = (*Store)(nil)

// hotTickerSet returns the tickers currently on any hot board. The caller must
// hold m.mu (read or write).
func (m *Store) hotTickerSet() map[string]bool {
	set := make(map[string]bool)
	for _, board := range m.hot {
		for _, h := range board {
			set[h.Ticker] = true
		}
	}
	return set
}

// PruneNews drops news older than the per-ticker cutoff: `before` normally, or
// the (earlier) `hotBefore` for tickers currently on a hot board.
func (m *Store) PruneNews(ctx context.Context, before, hotBefore time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hot := m.hotTickerSet()
	var n int64
	for ticker, byID := range m.news {
		cutoff := before
		if hot[ticker] {
			cutoff = hotBefore
		}
		for id, item := range byID {
			if item.Published.Before(cutoff) {
				delete(byID, id)
				n++
			}
		}
		if len(byID) == 0 {
			delete(m.news, ticker)
		}
	}
	return n, nil
}

// PruneSocial drops posts older than the per-ticker cutoff, but never touches a
// post whose source is in `protect` (the 大V / KOL rail).
func (m *Store) PruneSocial(ctx context.Context, before, hotBefore time.Time, protect []string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prot := make(map[string]bool, len(protect))
	for _, s := range protect {
		prot[s] = true
	}
	hot := m.hotTickerSet()
	var n int64
	for ticker, byID := range m.social {
		cutoff := before
		if hot[ticker] {
			cutoff = hotBefore
		}
		for id, p := range byID {
			if prot[p.Source] {
				continue // protected source — kept regardless of age
			}
			if p.CreatedAt.Before(cutoff) {
				delete(byID, id)
				n++
			}
		}
		if len(byID) == 0 {
			delete(m.social, ticker)
		}
	}
	return n, nil
}

// PruneFilings drops filings filed before `before`.
func (m *Store) PruneFilings(ctx context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for ticker, byAcc := range m.filings {
		for acc, f := range byAcc {
			if f.FiledAt.Before(before) {
				delete(byAcc, acc)
				n++
			}
		}
		if len(byAcc) == 0 {
			delete(m.filings, ticker)
		}
	}
	return n, nil
}

// PruneInsiderBuys drops insider buys filed before `before`.
func (m *Store) PruneInsiderBuys(ctx context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for acc, b := range m.insiders {
		if b.FiledDate.Before(before) {
			delete(m.insiders, acc)
			n++
		}
	}
	return n, nil
}

// PruneSeenForm4 drops seen-Form-4 markers filed before `before`.
func (m *Store) PruneSeenForm4(ctx context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for acc, filed := range m.seenF4 {
		if filed.Before(before) {
			delete(m.seenF4, acc)
			n++
		}
	}
	return n, nil
}

// CapPerTicker keeps only the newest n rows per ticker in "news" or "social",
// never counting or evicting rows whose source is in protect (the 大V rail).
func (m *Store) CapPerTicker(ctx context.Context, table string, n int, protect []string) (int64, error) {
	if n <= 0 {
		return 0, nil
	}
	prot := make(map[string]bool, len(protect))
	for _, s := range protect {
		prot[s] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch table {
	case "news":
		return capNewest(m.news, n, prot, func(v store.News) (time.Time, string) {
			return v.Published, v.Source
		}), nil
	case "social":
		return capNewest(m.social, n, prot, func(v store.Post) (time.Time, string) {
			return v.CreatedAt, v.Source
		}), nil
	default:
		return 0, fmt.Errorf("cap per ticker: unsupported table %q", table)
	}
}

// capNewest deletes all but the newest n non-protected rows per ticker from a
// ticker→id→record map. Protected-source rows are kept and don't count toward n.
func capNewest[T any](byTicker map[string]map[string]T, n int, prot map[string]bool, key func(T) (time.Time, string)) int64 {
	var removed int64
	for _, byID := range byTicker {
		type rec struct {
			id string
			t  time.Time
		}
		recs := make([]rec, 0, len(byID))
		for id, v := range byID {
			t, src := key(v)
			if prot[src] {
				continue // protected → never capped
			}
			recs = append(recs, rec{id, t})
		}
		if len(recs) <= n {
			continue
		}
		sort.Slice(recs, func(i, j int) bool { return recs[i].t.After(recs[j].t) })
		for _, r := range recs[n:] {
			delete(byID, r.id)
			removed++
		}
	}
	return removed
}
