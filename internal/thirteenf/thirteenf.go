// Package thirteenf builds the "whale holdings" board: the latest quarterly 13F
// holdings of a curated set of famous fund managers, with quarter-over-quarter
// changes. 13F data is public-domain SEC data, filed ~45 days after quarter-end,
// so the board is explicitly as-of a past quarter and refreshed slowly.
package thirteenf

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/sec"
)

// Fund is a tracked 13F filer (a well-known manager). CIKs are verified against
// EDGAR's submissions API.
type Fund struct {
	CIK     int
	Name    string // firm
	Manager string // the person it's known for
	Slug    string
}

// Funds is the curated whitelist — recognizable managers, weighted toward names
// a Chinese retail audience follows (Buffett, Burry, Ackman, and Li Lu's
// China-focused Himalaya Capital).
var Funds = []Fund{
	{1067983, "Berkshire Hathaway", "Warren Buffett", "berkshire"},
	{1649339, "Scion Asset Management", "Michael Burry", "scion"},
	{1336528, "Pershing Square", "Bill Ackman", "pershing-square"},
	{1709323, "Himalaya Capital", "Li Lu", "himalaya"},
	{1536411, "Duquesne Family Office", "Stanley Druckenmiller", "duquesne"},
	{1040273, "Third Point", "Daniel Loeb", "third-point"},
	{1061768, "Baupost Group", "Seth Klarman", "baupost"},
	{1350694, "Bridgewater Associates", "Ray Dalio", "bridgewater"},
}

// topN caps the positions shown per fund.
const topN = 15

// refreshEvery is the board's rebuild cadence. 13F is quarterly, so a slow
// refresh (twice a day) is plenty to pick up new filings near a deadline.
const refreshEvery = 12 * time.Hour

// Position is one holding on a fund's board.
type Position struct {
	Ticker string  `json:"ticker"` // "" when the CUSIP has no US-equity match
	Issuer string  `json:"issuer"`
	Value  int64   `json:"value"` // whole USD
	Shares int64   `json:"shares"`
	Pct    float64 `json:"pct"`     // % of the fund's 13F portfolio value
	Change string  `json:"change"`  // new | add | trim | hold
	ChgPct float64 `json:"chg_pct"` // signed share change vs prior quarter (%), 0 for new/hold
}

// FundHoldings is one fund's latest 13F snapshot with quarter-over-quarter tags.
type FundHoldings struct {
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	Manager   string     `json:"manager"`
	Period    string     `json:"period"` // quarter-end (as-of), YYYY-MM-DD
	Filed     string     `json:"filed"`  // filing date, YYYY-MM-DD
	Count     int        `json:"count"`  // total positions in the filing
	Value     int64      `json:"value"`  // total 13F portfolio value (USD)
	Positions []Position `json:"positions"`
}

// Board is the whole whitelist's holdings plus the build time.
type Board struct {
	Funds []FundHoldings `json:"funds"`
	At    time.Time      `json:"updated_at"`
}

// Holder is one fund that holds a given ticker, for the "which whales own this
// stock" reverse lookup. It carries enough to render a chip and deep-link to the
// fund's page without a second fetch.
type Holder struct {
	FundSlug string  `json:"fund_slug"`
	FundName string  `json:"fund_name"`
	Manager  string  `json:"manager"`
	Value    int64   `json:"value"`  // position value in this fund (whole USD)
	Weight   float64 `json:"weight"` // % of the fund's 13F portfolio
	Change   string  `json:"change"` // new | add | trim | hold
	Period   string  `json:"period"` // the fund's filing quarter-end (as-of), YYYY-MM-DD
}

// Filer fetches 13F filings and holdings (satisfied by *sec.Client).
type Filer interface {
	ThirteenFFilings(ctx context.Context, cik, n int) ([]sec.Filing13F, error)
	Holdings(ctx context.Context, cik int, accession string) ([]sec.Holding, error)
}

// Mapper resolves CUSIPs to tickers (satisfied by *openfigi.Client).
type Mapper interface {
	Map(ctx context.Context, cusips []string) (map[string]string, error)
}

// Cache builds and serves the whale-holdings board, refreshed by Run. Alongside
// the board it keeps two derived indexes rebuilt atomically with it: byTicker
// (ticker → funds holding it, for the per-stock "whales" chip) and bySlug (fund
// slug → its holdings, for the fund pSEO page).
type Cache struct {
	filer  Filer
	mapper Mapper

	mu       sync.RWMutex
	board    Board
	byTicker map[string][]Holder     // upper-cased ticker → funds holding it
	bySlug   map[string]FundHoldings // fund slug → its holdings
}

// NewCache builds the cache.
func NewCache(f Filer, m Mapper) *Cache {
	return &Cache{filer: f, mapper: m}
}

// Board returns the latest board and whether it has been built yet.
func (c *Cache) Board() (Board, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.board, !c.board.At.IsZero()
}

// Holders returns the funds on the whitelist that hold ticker, newest/largest
// first (sorted by position value). Returns nil when the ticker is held by none
// of the tracked funds or the board has not been built yet. Case-insensitive.
func (c *Cache) Holders(ticker string) []Holder {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byTicker[ticker]
}

// Fund returns one fund's holdings by slug (ok=false when the slug is unknown or
// the board has not been built yet). Case-insensitive on the slug.
func (c *Cache) Fund(slug string) (FundHoldings, bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return FundHoldings{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	fh, ok := c.bySlug[slug]
	return fh, ok
}

// Run builds the board immediately, then rebuilds every refreshEvery until ctx
// is cancelled. Runs on a background goroutine, off the request path.
func (c *Cache) Run(ctx context.Context) {
	c.Recompute(ctx)
	t := time.NewTicker(refreshEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.Recompute(ctx)
		}
	}
}

// fundData is one fund's computed holdings: the rendered FundHoldings (positions
// capped to topN for display) plus its FULL position list (every holding), kept
// only for building the reverse holder index so the holder count is complete.
type fundData struct {
	holdings FundHoldings
	allPos   []Position
}

// Recompute rebuilds the board for every fund (best-effort: a fund that fails to
// fetch is skipped). An all-fail run keeps the previous board.
func (c *Cache) Recompute(ctx context.Context) {
	var funds []fundData
	for _, fund := range Funds {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if fd, ok := computeFund(ctx, c.filer, c.mapper, fund); ok {
			funds = append(funds, fd)
		}
	}
	if len(funds) == 0 {
		return
	}
	board := make([]FundHoldings, len(funds))
	for i, fd := range funds {
		board[i] = fd.holdings
	}
	byTicker, bySlug := buildIndexes(funds)
	c.mu.Lock()
	c.board = Board{Funds: board, At: time.Now().UTC()}
	c.byTicker = byTicker
	c.bySlug = bySlug
	c.mu.Unlock()
}

// buildIndexes derives the reverse (ticker → holders) and per-slug (slug →
// holdings) lookups from the freshly built fund list. The reverse index walks the
// fund's FULL position list (not the topN-truncated render set) so the holder
// count is complete — a fund holding the ticker as its #16+ position is still
// counted. It skips positions with no resolved ticker (CUSIPs OpenFIGI couldn't
// map) and sorts each ticker's holders by position value, largest first. The
// per-slug index carries the rendered (topN-capped) holdings for the fund page.
func buildIndexes(funds []fundData) (map[string][]Holder, map[string]FundHoldings) {
	byTicker := make(map[string][]Holder)
	bySlug := make(map[string]FundHoldings, len(funds))
	for _, fd := range funds {
		fh := fd.holdings
		bySlug[strings.ToLower(fh.Slug)] = fh
		for _, p := range fd.allPos {
			if p.Ticker == "" {
				continue
			}
			t := strings.ToUpper(p.Ticker)
			byTicker[t] = append(byTicker[t], Holder{
				FundSlug: fh.Slug,
				FundName: fh.Name,
				Manager:  fh.Manager,
				Value:    p.Value,
				Weight:   p.Pct,
				Change:   p.Change,
				Period:   fh.Period,
			})
		}
	}
	for t := range byTicker {
		hs := byTicker[t]
		sort.Slice(hs, func(i, j int) bool { return hs[i].Value > hs[j].Value })
	}
	return byTicker, bySlug
}

// computeFund builds one fund's holdings: every position is computed and tagged
// against the prior quarter (so the reverse holder index is complete), but the
// rendered FundHoldings.Positions is capped to topN for display. ok=false when the
// fund has no usable filing.
func computeFund(ctx context.Context, f Filer, m Mapper, fund Fund) (fundData, bool) {
	filings, err := f.ThirteenFFilings(ctx, fund.CIK, 2)
	if err != nil || len(filings) == 0 {
		return fundData{}, false
	}
	latest, err := f.Holdings(ctx, fund.CIK, filings[0].Accession)
	if err != nil || len(latest) == 0 {
		return fundData{}, false
	}

	// Prior quarter's share counts AND values, for the quarter-over-quarter diff.
	// Both are needed: share counts drive the diff for equities (SH), but a
	// fixed-income position (PRN) carries Shares=0 with a real Value (sec's
	// parseInfoTable counts only SH amounts), so it must be diffed by value.
	var prevShares, prevValues map[string]int64
	if len(filings) > 1 {
		if prev, err := f.Holdings(ctx, fund.CIK, filings[1].Accession); err == nil && len(prev) > 0 {
			prevShares = make(map[string]int64, len(prev))
			prevValues = make(map[string]int64, len(prev))
			for _, h := range prev {
				prevShares[h.CUSIP] = h.Shares
				prevValues[h.CUSIP] = h.Value
			}
		}
	}

	// Total is summed over ALL holdings so each position's weight % is the share of
	// the full portfolio (never the truncated render set).
	var total int64
	for _, h := range latest {
		total += h.Value
	}
	sort.Slice(latest, func(i, j int) bool { return latest[i].Value > latest[j].Value })

	// Resolve tickers for EVERY holding so the reverse holder index is complete
	// (a #16+ position must still be counted), not just the rendered top-N.
	cusips := make([]string, len(latest))
	for i, h := range latest {
		cusips[i] = h.CUSIP
	}
	tickers, _ := m.Map(ctx, cusips) // best-effort; unmapped CUSIPs keep an empty ticker

	allPos := make([]Position, 0, len(latest))
	for _, h := range latest {
		p := Position{Ticker: tickers[h.CUSIP], Issuer: h.Issuer, Value: h.Value, Shares: h.Shares}
		if total > 0 {
			p.Pct = float64(h.Value) / float64(total) * 100
		}
		p.Change, p.ChgPct = classify(h.Shares, h.Value, prevShares, prevValues, h.CUSIP)
		allPos = append(allPos, p)
	}

	// The rendered list is the topN largest positions (display cap only); the full
	// allPos list is what the reverse index walks.
	rendered := allPos
	if len(rendered) > topN {
		rendered = rendered[:topN]
	}

	return fundData{
		holdings: FundHoldings{
			Slug: fund.Slug, Name: fund.Name, Manager: fund.Manager,
			Period: filings[0].Period, Filed: filings[0].Filed,
			Count: len(latest), Value: total, Positions: rendered,
		},
		allPos: allPos,
	}, true
}

// classify labels a position against the prior quarter. With no prior filing to
// compare, everything reads as "hold" (we can't infer a change). For ordinary
// equity (SH) holdings it diffs the share count; for a position whose share count
// is 0 in BOTH quarters — a fixed-income (PRN) holding, whose principal sec's
// parseInfoTable deliberately omits from Shares — it diffs the Value instead, so a
// bond held across quarters reads "hold"/"add"/"trim" rather than "new" every time.
func classify(shares, value int64, prevShares, prevValues map[string]int64, cusip string) (string, float64) {
	if prevShares == nil {
		return "hold", 0
	}
	ps, ok := prevShares[cusip]
	// A PRN-only position carries Shares=0 in both quarters; falling back to the
	// (always-populated) value delta keeps a held bond from re-reading "new".
	if shares == 0 && ps == 0 {
		return classifyByValue(value, prevValues, cusip)
	}
	if !ok || ps == 0 {
		return "new", 0
	}
	if shares == ps {
		return "hold", 0
	}
	return changeTag(float64(shares-ps) / float64(ps) * 100)
}

// classifyByValue diffs a position by its prior-quarter Value, used for PRN
// (bond/note) holdings whose share count is 0. A CUSIP absent last quarter (or
// with no prior value to compare) is genuinely "new".
func classifyByValue(value int64, prevValues map[string]int64, cusip string) (string, float64) {
	pv, ok := prevValues[cusip]
	if !ok || pv == 0 {
		return "new", 0
	}
	if value == pv {
		return "hold", 0
	}
	return changeTag(float64(value-pv) / float64(pv) * 100)
}

// changeTag maps a signed percent delta to an add/trim/hold tag (±5% band).
func changeTag(delta float64) (string, float64) {
	switch {
	case delta >= 5:
		return "add", delta
	case delta <= -5:
		return "trim", delta
	default:
		return "hold", delta
	}
}
