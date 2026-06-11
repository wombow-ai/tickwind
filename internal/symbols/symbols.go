// Package symbols provides an in-memory, searchable directory of stock symbols
// (ticker + company name + exchange) for autocomplete. The US directory is built
// from SEC's public-domain company_tickers_exchange.json; international markets
// append later via the Country field. Search runs against an immutable snapshot,
// swapped atomically by the ingestor, so reads are lock-free.
package symbols

import (
	"sort"
	"strings"
)

// Symbol is one searchable security.
type Symbol struct {
	Ticker   string `json:"ticker"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"` // "Nasdaq" | "NYSE" | "OTC" | ...
	Country  string `json:"country"`  // "US" for now (per-source); intl later
	// Aliases are alternate search terms (notably Chinese names, e.g. "英伟达"
	// for NVDA) so zh-first users can find a stock by its native name.
	Aliases []string `json:"aliases,omitempty"`
}

// Index is an immutable, searchable snapshot of the directory.
type Index struct {
	all       []Symbol
	nameLower []string         // parallel to all: lower-cased name (substring scan)
	aliases   [][]string       // parallel to all: this symbol's aliases (e.g. CJK names)
	byTicker  map[string]int   // upper ticker -> index in all
	nameTok   map[string][]int // lower name token -> indices in all
}

// Build constructs a searchable Index, deduped by ticker. Each symbol's curated
// aliases (Chinese names) are merged from the alias table so a CJK query can
// resolve it.
func Build(syms []Symbol) *Index {
	idx := &Index{
		byTicker: make(map[string]int, len(syms)),
		nameTok:  make(map[string][]int),
	}
	aliasTable := Aliases()
	for _, s := range syms {
		t := strings.ToUpper(strings.TrimSpace(s.Ticker))
		if t == "" {
			continue
		}
		if _, dup := idx.byTicker[t]; dup {
			continue
		}
		s.Ticker = t
		if extra := aliasTable[t]; len(extra) > 0 {
			s.Aliases = append(append([]string{}, s.Aliases...), extra...)
		}
		i := len(idx.all)
		idx.all = append(idx.all, s)
		idx.nameLower = append(idx.nameLower, strings.ToLower(s.Name))
		idx.aliases = append(idx.aliases, s.Aliases)
		idx.byTicker[t] = i
		seen := map[string]bool{}
		for _, tok := range tokenize(s.Name) {
			if seen[tok] {
				continue
			}
			seen[tok] = true
			idx.nameTok[tok] = append(idx.nameTok[tok], i)
		}
		// ASCII aliases (e.g. "Meta", "Alphabet") also feed the token index so
		// English-keyword search finds them; CJK aliases are matched separately.
		for _, a := range s.Aliases {
			for _, tok := range tokenize(a) {
				if seen[tok] {
					continue
				}
				seen[tok] = true
				idx.nameTok[tok] = append(idx.nameTok[tok], i)
			}
		}
	}
	return idx
}

// Len returns the number of indexed symbols (0 for a nil Index).
func (idx *Index) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.all)
}

// USTickers returns every indexed US ticker (for the universe price sweep). nil-safe.
func (idx *Index) USTickers() []string {
	if idx == nil {
		return nil
	}
	out := make([]string, 0, len(idx.all))
	for _, s := range idx.all {
		if s.Country == "US" {
			out = append(out, s.Ticker)
		}
	}
	return out
}

// Search returns up to limit symbols matching q, best first:
//
//	rank 0 exact ticker · 1 ticker prefix · 2 name-token prefix · 3 name substring
//
// Ties prefer major exchanges, then the shorter ticker, then alphabetical. A nil
// Index or blank query returns nil.
func (idx *Index) Search(q string, limit int) []Symbol {
	if idx == nil {
		return nil
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	up, low := strings.ToUpper(q), strings.ToLower(q)

	best := make(map[int]int) // symbol index -> best (lowest) rank seen
	consider := func(i, rank int) {
		if r, ok := best[i]; !ok || rank < r {
			best[i] = rank
		}
	}

	if i, ok := idx.byTicker[up]; ok { // 0: exact ticker
		consider(i, 0)
	}
	for t, i := range idx.byTicker { // 1: ticker prefix
		if t != up && strings.HasPrefix(t, up) {
			consider(i, 1)
		}
	}
	for tok, idxs := range idx.nameTok { // 2: name-token prefix
		if strings.HasPrefix(tok, low) {
			for _, i := range idxs {
				consider(i, 2)
			}
		}
	}
	if len(low) >= 3 { // 3: name substring (only ≥3 chars, to limit noise)
		for i, nl := range idx.nameLower {
			if strings.Contains(nl, low) {
				consider(i, 3)
			}
		}
	}

	if hasCJK(q) { // CJK query → match curated aliases (the only index that has them)
		for i, als := range idx.aliases {
			for _, a := range als {
				if a == q { // exact alias, e.g. "英伟达"
					consider(i, 0)
					break
				}
				if strings.Contains(a, q) || strings.Contains(q, a) {
					consider(i, 2)
				}
			}
		}
	}

	type hit struct{ i, rank int }
	hits := make([]hit, 0, len(best))
	for i, r := range best {
		hits = append(hits, hit{i, r})
	}
	sort.Slice(hits, func(a, b int) bool {
		ha, hb := hits[a], hits[b]
		if ha.rank != hb.rank {
			return ha.rank < hb.rank
		}
		sa, sb := idx.all[ha.i], idx.all[hb.i]
		if ea, eb := exchRank(sa.Exchange), exchRank(sb.Exchange); ea != eb {
			return ea < eb
		}
		if len(sa.Ticker) != len(sb.Ticker) {
			return len(sa.Ticker) < len(sb.Ticker)
		}
		return sa.Ticker < sb.Ticker
	})

	out := make([]Symbol, 0, limit)
	for _, h := range hits {
		out = append(out, idx.all[h.i])
		if len(out) >= limit {
			break
		}
	}
	return out
}

// hasCJK reports whether s contains any CJK character (so search routes to the
// alias index). Covers the CJK Unified Ideographs block, enough for our names.
func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// exchRank orders exchanges so primary listings beat OTC on ties.
func exchRank(e string) int {
	switch strings.ToLower(e) {
	case "nasdaq", "nyse":
		return 0
	case "nyse arca", "nyse american", "cboe", "cboe bzx", "bats", "iex":
		return 1
	case "otc":
		return 3
	default:
		return 2
	}
}

// tokenize splits a company name into lower-cased alphanumeric tokens, dropping
// the most common corporate-suffix noise so "co"/"inc" don't match everything.
func tokenize(name string) []string {
	fields := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := fields[:0]
	for _, f := range fields {
		if f != "" && !noiseToken[f] {
			out = append(out, f)
		}
	}
	return out
}

var noiseToken = map[string]bool{
	"inc": true, "incorporated": true, "corp": true, "corporation": true,
	"co": true, "company": true, "ltd": true, "limited": true, "plc": true,
	"the": true, "sa": true, "ag": true, "nv": true,
}
