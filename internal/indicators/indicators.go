// Package indicators serves the stock-applicable indicator catalog — a static,
// dataset-driven metadata library (Glassnode/LookNode style) used to render a
// browsable indicator reference and, later, to drive per-stock computation.
//
// The dataset (docs/indicators/indicators.json, 414 records) is the single
// source of truth: a package-local copy is embedded at build time so the catalog
// is immutable and dependency-free. Records are never hand-maintained in code —
// the catalog is generated from the embedded JSON and the public formulas it
// carries are shown verbatim.
//
// This phase exposes catalog metadata only (browse / filter / search). Per-stock
// indicator computation is Phase 1 — see the Compute TODO below for its hook.
package indicators

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// indicatorsJSON is a package-local copy of docs/indicators/indicators.json.
// go:embed cannot reference paths outside the package directory ("../"), so the
// dataset is duplicated here; keep it in sync with the canonical docs copy.
//
//go:embed indicators.json
var indicatorsJSON []byte

// Indicator is one catalog record. The fields mirror the dataset schema exactly
// (see docs/indicators/SPEC.md); JSON tags preserve the on-the-wire key names so
// the API response is a faithful pass-through of the dataset.
type Indicator struct {
	ID         string `json:"id"`
	Domain     string `json:"domain"`
	DomainName string `json:"domain_name"`
	// Subcategory is the sub-group within a domain. It is the only Chinese-leaning
	// label kept from the source — most subcategories are English here.
	Subcategory string `json:"subcategory"`
	Priority    string `json:"priority"`
	AppliesTo   string `json:"applies_to"`
	NameEN      string `json:"name_en"`
	NameZH      string `json:"name_zh"`
	Abbr        string `json:"abbr"`
	Definition  string `json:"definition"`
	// Formula is the calculation logic, shown verbatim (math + symbols preserved).
	Formula string `json:"formula"`
	// Inputs are the OHLCV inputs for technical indicators (may be null/empty).
	Inputs []string `json:"inputs"`
	// DefaultParams are suggested default parameters; kept as raw JSON because the
	// shape varies per indicator (e.g. {"period":14} or {"periods":[12,26]}).
	DefaultParams json.RawMessage `json:"default_params,omitempty"`
	// TALibOrLib is the TA-Lib function or library hint where one exists.
	TALibOrLib string `json:"talib_or_lib,omitempty"`
	// OutputType is a render/served shape hint (overlay/oscillator/volume/...).
	OutputType string `json:"output_type,omitempty"`
	DataSource string `json:"data_source"`
	// Interpretation is how to read it (typical thresholds & signals).
	Interpretation string `json:"interpretation"`
}

// Query filters the catalog. Zero-value fields are ignored, so an empty Query
// returns the whole stock-applicable catalog.
type Query struct {
	Domain      string // exact match on Indicator.Domain (e.g. "technical")
	Priority    string // exact match on Indicator.Priority (e.g. "P0")
	Subcategory string // exact match on Indicator.Subcategory
	// Text is a free-text query matched (case-insensitively) against the English
	// and Chinese names, abbreviation, and definition.
	Text string
}

// Facet is a value and how many catalog records carry it, for building filter
// chips on the client.
type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Facets summarizes the catalog along the dimensions the UI filters by.
type Facets struct {
	Domains       []Facet `json:"domains"`
	Priorities    []Facet `json:"priorities"`
	Subcategories []Facet `json:"subcategories"`
}

// Catalog is the immutable, in-memory indicator catalog (stock-applicable only).
// It is safe for concurrent reads — all accessors are pure over the loaded slice.
type Catalog struct {
	all    []Indicator // stock-applicable indicators, in dataset order
	facets Facets      // precomputed facets over the full catalog
}

// priorityRank orders priorities P0 < P1 < P2 so P0 (MVP core) sorts first.
var priorityRank = map[string]int{"P0": 0, "P1": 1, "P2": 2}

// appliesToStock reports whether an indicator applies to stocks (applies_to is
// "stock" or "both"); crypto-only indicators are excluded from this catalog.
func appliesToStock(appliesTo string) bool {
	return appliesTo == "stock" || appliesTo == "both"
}

// Load parses the embedded dataset and returns a Catalog containing only the
// stock-applicable indicators (applies_to ∈ {stock, both}). It returns an error
// if the embedded JSON is malformed, so a bad dataset fails the build's tests
// rather than silently serving an empty catalog.
func Load() (*Catalog, error) {
	var raw []Indicator
	if err := json.Unmarshal(indicatorsJSON, &raw); err != nil {
		return nil, fmt.Errorf("indicators: parse embedded dataset: %w", err)
	}
	all := make([]Indicator, 0, len(raw))
	for _, ind := range raw {
		if appliesToStock(ind.AppliesTo) {
			all = append(all, ind)
		}
	}
	c := &Catalog{all: all}
	c.facets = computeFacets(all)
	return c, nil
}

// MustLoad is Load but panics on error. It is intended for package-level
// initialization where a malformed embedded dataset is a programming error.
func MustLoad() *Catalog {
	c, err := Load()
	if err != nil {
		panic(err)
	}
	return c
}

// Len returns the number of stock-applicable indicators in the catalog.
func (c *Catalog) Len() int { return len(c.all) }

// All returns the whole stock-applicable catalog (a fresh copy, so callers
// cannot mutate the shared slice).
func (c *Catalog) All() []Indicator {
	out := make([]Indicator, len(c.all))
	copy(out, c.all)
	return out
}

// Facets returns the precomputed facet counts over the full catalog (domains,
// priorities, subcategories), so filter UIs can show counts even while a filter
// is applied.
func (c *Catalog) Facets() Facets { return c.facets }

// Filter returns the indicators matching q, in dataset order. An empty Query
// returns the whole catalog. The result is always non-nil (possibly empty).
func (c *Catalog) Filter(q Query) []Indicator {
	domain := strings.TrimSpace(q.Domain)
	priority := strings.TrimSpace(q.Priority)
	subcat := strings.TrimSpace(q.Subcategory)
	text := strings.ToLower(strings.TrimSpace(q.Text))

	out := make([]Indicator, 0, len(c.all))
	for _, ind := range c.all {
		if domain != "" && ind.Domain != domain {
			continue
		}
		if priority != "" && ind.Priority != priority {
			continue
		}
		if subcat != "" && ind.Subcategory != subcat {
			continue
		}
		if text != "" && !matchesText(ind, text) {
			continue
		}
		out = append(out, ind)
	}
	return out
}

// matchesText reports whether the lower-cased query appears in the indicator's
// English/Chinese name, abbreviation, or definition.
func matchesText(ind Indicator, lowerQuery string) bool {
	fields := [...]string{ind.NameEN, ind.NameZH, ind.Abbr, ind.Definition}
	for _, f := range fields {
		if f != "" && strings.Contains(strings.ToLower(f), lowerQuery) {
			return true
		}
	}
	return false
}

// computeFacets tallies the catalog along each filter dimension. Domains and
// priorities are returned in a stable, meaningful order (priorities P0→P2);
// subcategories are sorted alphabetically.
func computeFacets(all []Indicator) Facets {
	domainCount := map[string]int{}
	priorityCount := map[string]int{}
	subcatCount := map[string]int{}
	for _, ind := range all {
		domainCount[ind.Domain]++
		priorityCount[ind.Priority]++
		subcatCount[ind.Subcategory]++
	}
	return Facets{
		Domains:       sortedFacets(domainCount, nil),
		Priorities:    sortedFacets(priorityCount, priorityRank),
		Subcategories: sortedFacets(subcatCount, nil),
	}
}

// sortedFacets turns a value→count map into a sorted Facet slice. When rank is
// provided, values are ordered by their rank (lower first); otherwise they are
// ordered by descending count, then alphabetically for stability.
func sortedFacets(counts map[string]int, rank map[string]int) []Facet {
	out := make([]Facet, 0, len(counts))
	for v, n := range counts {
		out = append(out, Facet{Value: v, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if rank != nil {
			ri, oki := rank[out[i].Value]
			rj, okj := rank[out[j].Value]
			if oki && okj && ri != rj {
				return ri < rj
			}
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// TODO(phase1): per-stock indicator computation lives here. Add a Compute method
// (or a separate computation package) that, given an indicator ID + a ticker's
// OHLCV/fundamentals series, evaluates the indicator using its talib_or_lib hint
// or its formula. This Phase-0 catalog already carries the inputs, default_params
// and talib_or_lib metadata each computation needs.
