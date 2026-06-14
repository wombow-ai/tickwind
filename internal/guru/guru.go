// Package guru builds the "Guru-watch" rail: recent posts from curated finance
// writers (KOLs, e.g. Serenity) with the tickers each one mentions, so an
// opinionated newsletter view sits alongside the data. Every item is attributed
// and linked to its source — opinions for context, never advice — and sourcing
// is read-only public RSS, never the full (possibly paywalled) body.
package guru

import (
	"sort"
	"strings"
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

// maxPerAuthor caps how many of one publication's posts lead the rail, so a bursty
// newsletter (several posts in a day) can't crowd out everyone else.
const maxPerAuthor = 2

// Rank prepares the rail: it drops items with no source URL or title, removes
// duplicates by URL, sorts newest-first, diversifies by author, and caps to max
// (max<=0 means no cap).
//
// Items need NOT mention a ticker. These newsletters overwhelmingly name companies
// in prose (or as paywall teasers that name nothing at all), not as $cashtags, so
// requiring a ticker froze the rail on the rare tagged post and dropped every fresh
// one. Freshness is the rail's value; tickers, when present, still render as
// deep-link chips.
func Rank(items []Item, max int) []Item {
	seen := make(map[string]struct{}, len(items))
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.URL == "" || strings.TrimSpace(it.Title) == "" {
			continue
		}
		if _, ok := seen[it.URL]; ok {
			continue
		}
		seen[it.URL] = struct{}{}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	out = diversifyByAuthor(out, maxPerAuthor)
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// diversifyByAuthor reorders a newest-first list so that no single author leads with
// more than perAuthor posts: it keeps each author's first perAuthor items in place,
// then appends the rest (still newest-first) to backfill. With a small max this puts a
// diverse set up top while never dropping content outright.
func diversifyByAuthor(items []Item, perAuthor int) []Item {
	if perAuthor <= 0 {
		return items
	}
	kept := make([]Item, 0, len(items))
	overflow := make([]Item, 0)
	count := make(map[string]int, len(items))
	for _, it := range items {
		if count[it.Author] < perAuthor {
			count[it.Author]++
			kept = append(kept, it)
		} else {
			overflow = append(overflow, it)
		}
	}
	return append(kept, overflow...)
}
