package congress

import (
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress/ptr"
)

// MemberTx is one member's parsed PTR transactions, keyed by a URL-safe slug
// derived from the member's display name (see Slugify). It powers the per-member
// page (/v1/congress/member/{slug}).
type MemberTx struct {
	Slug         string            `json:"slug"`         // "nancy-pelosi"
	Name         string            `json:"name"`         // "Nancy Pelosi"
	State        string            `json:"state"`        // "CA"
	Transactions []ptr.Transaction `json:"transactions"` // newest filing's trades, accumulated
}

// TickerTrade is one member's trade in a given ticker, the flattened shape served
// by the per-stock "members trading this" chip (/v1/stocks/{ticker}/congress).
type TickerTrade struct {
	MemberName  string    `json:"member"`       // "Nancy Pelosi"
	Slug        string    `json:"slug"`         // "nancy-pelosi"
	Type        string    `json:"type"`         // "purchase" / "sale" / "exchange"
	AmountRange string    `json:"amount_range"` // "$250,001 - $500,000"
	TxDate      time.Time `json:"tx_date"`      // transaction date
}

// Cache holds the latest snapshot of recent Periodic Transaction Reports,
// swapped atomically by the ingestor. Memory-only + rebuildable (the House Clerk
// index is cheap to re-fetch), mirroring internal/universe's atomic cache —
// lock-free reads, no per-refresh DB writes.
//
// Beyond the filing index, the snapshot also carries the transactions parsed out
// of the digital PTR PDFs, indexed two ways: by member slug (for the member page)
// and by ticker (for the per-stock chip).
type Cache struct {
	v atomic.Value // *snapshot

	// validTicker, when set, gates which extracted PTR tickers are ticker-indexed
	// into byTicker (and thus served by ByTicker / the 资金面 research facts). The
	// ptr parser extracts a ticker from ANY uppercase parenthetical, so a non-ticker
	// acronym or an out-of-universe symbol (e.g. a crypto "(BTC)") could otherwise
	// be minted as a false ticker on an unrelated real stock. A nil validator (tests,
	// or before the symbol universe loads) keeps every parsed ticker — degrade
	// safely, never drop everything. Set once before the first SetWithTransactions.
	validTicker func(string) bool
}

// SetTickerValidator installs an optional ticker-universe gate applied when the
// by-ticker index is (re)built. It mirrors the guru rail's SetValidTickers: only
// tickers the validator accepts (real US symbols) are surfaced on a stock's
// congress chip / 资金面 facts, dropping non-ticker parentheticals the same way
// CUSIPs are already dropped. Nil-safe: pass nil (or never call it) to keep
// today's behavior. Call before Run starts the refresh goroutine so the
// assignment happens-before the reader; it is then re-applied on every snapshot.
func (c *Cache) SetTickerValidator(fn func(string) bool) { c.validTicker = fn }

type snapshot struct {
	filings   []Filing
	byMember  map[string]MemberTx      // slug → member + transactions
	byTicker  map[string][]TickerTrade // upper-case ticker → trades
	members   []MemberTx               // members sorted by name (for Members())
	updatedAt time.Time
}

// NewCache returns an empty cache (Len 0 until the first refresh).
func NewCache() *Cache { return &Cache{} }

// Set replaces the snapshot with a fresh filings slice (expected newest first),
// keeping any previously parsed transactions. Use SetWithTransactions to swap
// both at once; Set is retained for callers (and degraded paths) that only have
// the filing index.
func (c *Cache) Set(filings []Filing) {
	prev := c.snap()
	var byMember map[string]MemberTx
	if prev != nil {
		byMember = prev.byMember
	}
	c.SetWithTransactions(filings, byMember)
}

// SetWithTransactions replaces the snapshot with a fresh filings slice plus the
// parsed transactions keyed by member slug. It rebuilds the by-ticker index and
// the sorted members list from byMember. A nil byMember stores an empty index
// (the filing index alone, e.g. when PTR parsing is unavailable).
func (c *Cache) SetWithTransactions(filings []Filing, byMember map[string]MemberTx) {
	if byMember == nil {
		byMember = map[string]MemberTx{}
	}
	byTicker := make(map[string][]TickerTrade)
	members := make([]MemberTx, 0, len(byMember))
	for _, m := range byMember {
		members = append(members, m)
		for _, tx := range m.Transactions {
			tk := strings.ToUpper(strings.TrimSpace(tx.Ticker))
			if tk == "" {
				continue // assets without a ticker (bonds, funds) aren't ticker-indexed
			}
			// Drop a parenthetical that isn't a real US symbol (a description
			// acronym, or an out-of-universe crypto symbol), so it can't assert a
			// congressional trade on an unrelated real stock. Nil validator ⇒ keep
			// all (test / pre-load fallback). The member page still carries the raw
			// transaction (asset name intact); only ticker-indexing is gated.
			if c.validTicker != nil && !c.validTicker(tk) {
				continue
			}
			byTicker[tk] = append(byTicker[tk], TickerTrade{
				MemberName:  m.Name,
				Slug:        m.Slug,
				Type:        string(tx.Type),
				AmountRange: tx.AmountRange,
				TxDate:      tx.TxDate,
			})
		}
	}
	// Newest trade first within each ticker, so the per-stock chip leads with
	// recent activity.
	for tk := range byTicker {
		trades := byTicker[tk]
		sort.SliceStable(trades, func(i, j int) bool { return trades[i].TxDate.After(trades[j].TxDate) })
		byTicker[tk] = trades
	}
	sort.SliceStable(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	c.v.Store(&snapshot{
		filings:   filings,
		byMember:  byMember,
		byTicker:  byTicker,
		members:   members,
		updatedAt: time.Now().UTC(),
	})
}

func (c *Cache) snap() *snapshot {
	s, _ := c.v.Load().(*snapshot)
	return s
}

// Get returns the cached filings (newest first), or nil when never refreshed.
func (c *Cache) Get() []Filing {
	if s := c.snap(); s != nil {
		return s.filings
	}
	return nil
}

// ByTicker returns the recent congressional trades in a ticker (newest first),
// or nil when none. The ticker match is case-insensitive.
func (c *Cache) ByTicker(ticker string) []TickerTrade {
	s := c.snap()
	if s == nil {
		return nil
	}
	return s.byTicker[strings.ToUpper(strings.TrimSpace(ticker))]
}

// ByMember returns one member's parsed transactions by slug (ok=false when the
// slug is unknown or no transactions have been parsed for them yet).
func (c *Cache) ByMember(slug string) (MemberTx, bool) {
	s := c.snap()
	if s == nil {
		return MemberTx{}, false
	}
	m, ok := s.byMember[strings.TrimSpace(strings.ToLower(slug))]
	return m, ok
}

// Members returns every member with parsed transactions, sorted by name.
func (c *Cache) Members() []MemberTx {
	if s := c.snap(); s != nil {
		return s.members
	}
	return nil
}

// Len is the number of cached filings (0 for an empty cache).
func (c *Cache) Len() int {
	if s := c.snap(); s != nil {
		return len(s.filings)
	}
	return 0
}

// UpdatedAt is when the snapshot was last refreshed (zero if never).
func (c *Cache) UpdatedAt() time.Time {
	if s := c.snap(); s != nil {
		return s.updatedAt
	}
	return time.Time{}
}

// Slugify turns a member display name into a URL-safe slug: lower-cased, with
// runs of non-alphanumeric characters (spaces, dots, etc.) collapsed to a single
// hyphen and the ends trimmed. "Nancy Pelosi" → "nancy-pelosi"; "Richard W.
// Allen" → "richard-w-allen".
func Slugify(name string) string {
	var b strings.Builder
	lastHyphen := true // leading-hyphen guard
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
