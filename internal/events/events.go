// Package events builds the "Major events timeline / 大事件时间线": upcoming and
// recent market-moving events — macro releases (FOMC, CPI, NFP, …) from US-gov
// public-domain schedules, plus a curated set of notable world events. The data
// is informational (what to watch), never advice. Served from an atomically-
// swapped snapshot for lock-free reads.
package events

import (
	"sort"
	"sync/atomic"
	"time"
)

// Event is one dated, market-relevant event on the timeline.
type Event struct {
	ID         string    `json:"id"`          // stable, e.g. "fomc-2026-03-18", "bls-cpi-2026-07-15"
	Title      string    `json:"title"`       // "FOMC rate decision", "US CPI"
	Category   string    `json:"category"`    // "macro" | "world"
	Subtype    string    `json:"subtype"`     // "fomc"|"nfp"|"cpi"|"ppi"|"gdp"|"jobs"|"worldcup"|"election"|...
	StartUTC   time.Time `json:"start"`       // normalized to UTC
	AllDay     bool      `json:"all_day"`     // true when only a date is known
	Importance string    `json:"importance"`  // "high" | "med" | "low" (editorial)
	Region     string    `json:"region"`      // "US" | "Global" | country
	SourceName string    `json:"source_name"` // "BLS" | "Federal Reserve" | "curated"
	SourceURL  string    `json:"source_url"`  //
}

// Cache holds the latest event timeline for lock-free reads (atomic swap),
// shared between the ingestor (writer) and the API (reader).
type Cache struct {
	v atomic.Value
}

// NewCache returns a Cache seeded with an empty timeline.
func NewCache() *Cache {
	c := &Cache{}
	c.v.Store([]Event{})
	return c
}

// Set replaces the current timeline.
func (c *Cache) Set(events []Event) { c.v.Store(events) }

// Get returns the current timeline (ascending by start).
func (c *Cache) Get() []Event { return c.v.Load().([]Event) }

// Merge concatenates event lists, drops entries with no ID or zero time,
// dedupes by ID (first occurrence wins, so curated overrides feeds), and sorts
// ascending by start time.
func Merge(lists ...[]Event) []Event {
	seen := make(map[string]struct{})
	out := make([]Event, 0)
	for _, l := range lists {
		for _, e := range l {
			if e.ID == "" || e.StartUTC.IsZero() {
				continue
			}
			if _, dup := seen[e.ID]; dup {
				continue
			}
			seen[e.ID] = struct{}{}
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartUTC.Before(out[j].StartUTC) })
	return out
}
