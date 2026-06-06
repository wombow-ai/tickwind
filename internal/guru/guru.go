// Package guru builds the "Guru-watch" rail: recent posts from curated finance
// writers (KOLs, e.g. Serenity) with the tickers each one mentions, so an
// opinionated newsletter view sits alongside the data. Every item is attributed
// and linked to its source — opinions for context, never advice — and sourcing
// is read-only public RSS, never the full (possibly paywalled) body.
package guru

import (
	"sort"
	"time"
)

// Item is one curated KOL post in the rail.
type Item struct {
	Author    string    `json:"author"`    // the writer/publication (curated name)
	Title     string    `json:"title"`     //
	URL       string    `json:"url"`       // link to the source post
	Teaser    string    `json:"teaser"`    // short fair-use snippet, never the full body
	Published time.Time `json:"published"` //
	Tickers   []string  `json:"tickers"`   // tickers the post mentions (cashtags)
}

// Rank prepares the rail: it keeps only items that mention at least one ticker
// (the rail is stock-anchored — every row must link somewhere), drops duplicates
// by URL, sorts newest-first and caps to max (max<=0 means no cap).
func Rank(items []Item, max int) []Item {
	seen := make(map[string]struct{}, len(items))
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if len(it.Tickers) == 0 || it.URL == "" {
			continue
		}
		if _, ok := seen[it.URL]; ok {
			continue
		}
		seen[it.URL] = struct{}{}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}
