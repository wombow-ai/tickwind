package memory

import (
	"context"
	"fmt"
	"sort"
	"time"
)

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

// CapPerTicker keeps only the newest n rows per ticker in "news" or "social".
func (m *Store) CapPerTicker(ctx context.Context, table string, n int) (int64, error) {
	if n <= 0 {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch table {
	case "news":
		var removed int64
		for _, byID := range m.news {
			if len(byID) <= n {
				continue
			}
			ids := make([]string, 0, len(byID))
			for id := range byID {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool {
				return byID[ids[i]].Published.After(byID[ids[j]].Published)
			})
			for _, id := range ids[n:] {
				delete(byID, id)
				removed++
			}
		}
		return removed, nil
	case "social":
		var removed int64
		for _, byID := range m.social {
			if len(byID) <= n {
				continue
			}
			ids := make([]string, 0, len(byID))
			for id := range byID {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool {
				return byID[ids[i]].CreatedAt.After(byID[ids[j]].CreatedAt)
			})
			for _, id := range ids[n:] {
				delete(byID, id)
				removed++
			}
		}
		return removed, nil
	default:
		return 0, fmt.Errorf("cap per ticker: unsupported table %q", table)
	}
}
