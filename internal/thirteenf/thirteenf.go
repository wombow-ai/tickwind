// Package thirteenf builds the "whale holdings" board: the latest quarterly 13F
// holdings of a curated set of famous fund managers, with quarter-over-quarter
// changes. 13F data is public-domain SEC data, filed ~45 days after quarter-end,
// so the board is explicitly as-of a past quarter and refreshed slowly.
package thirteenf

import (
	"context"
	"sort"
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

// Filer fetches 13F filings and holdings (satisfied by *sec.Client).
type Filer interface {
	ThirteenFFilings(ctx context.Context, cik, n int) ([]sec.Filing13F, error)
	Holdings(ctx context.Context, cik int, accession string) ([]sec.Holding, error)
}

// Mapper resolves CUSIPs to tickers (satisfied by *openfigi.Client).
type Mapper interface {
	Map(ctx context.Context, cusips []string) (map[string]string, error)
}

// Cache builds and serves the whale-holdings board, refreshed by Run.
type Cache struct {
	filer  Filer
	mapper Mapper

	mu    sync.RWMutex
	board Board
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

// Recompute rebuilds the board for every fund (best-effort: a fund that fails to
// fetch is skipped). An all-fail run keeps the previous board.
func (c *Cache) Recompute(ctx context.Context) {
	var funds []FundHoldings
	for _, fund := range Funds {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if fh, ok := computeFund(ctx, c.filer, c.mapper, fund); ok {
			funds = append(funds, fh)
		}
	}
	if len(funds) == 0 {
		return
	}
	c.mu.Lock()
	c.board = Board{Funds: funds, At: time.Now().UTC()}
	c.mu.Unlock()
}

// computeFund builds one fund's holdings: latest filing's top positions, each
// tagged against the prior quarter. ok=false when the fund has no usable filing.
func computeFund(ctx context.Context, f Filer, m Mapper, fund Fund) (FundHoldings, bool) {
	filings, err := f.ThirteenFFilings(ctx, fund.CIK, 2)
	if err != nil || len(filings) == 0 {
		return FundHoldings{}, false
	}
	latest, err := f.Holdings(ctx, fund.CIK, filings[0].Accession)
	if err != nil || len(latest) == 0 {
		return FundHoldings{}, false
	}

	// Prior quarter's share counts, for the quarter-over-quarter diff.
	var prevShares map[string]int64
	if len(filings) > 1 {
		if prev, err := f.Holdings(ctx, fund.CIK, filings[1].Accession); err == nil && len(prev) > 0 {
			prevShares = make(map[string]int64, len(prev))
			for _, h := range prev {
				prevShares[h.CUSIP] = h.Shares
			}
		}
	}

	var total int64
	for _, h := range latest {
		total += h.Value
	}
	sort.Slice(latest, func(i, j int) bool { return latest[i].Value > latest[j].Value })
	top := latest
	if len(top) > topN {
		top = top[:topN]
	}

	cusips := make([]string, len(top))
	for i, h := range top {
		cusips[i] = h.CUSIP
	}
	tickers, _ := m.Map(ctx, cusips) // best-effort; unmapped CUSIPs keep an empty ticker

	positions := make([]Position, 0, len(top))
	for _, h := range top {
		p := Position{Ticker: tickers[h.CUSIP], Issuer: h.Issuer, Value: h.Value, Shares: h.Shares}
		if total > 0 {
			p.Pct = float64(h.Value) / float64(total) * 100
		}
		p.Change, p.ChgPct = classify(h.Shares, prevShares, h.CUSIP)
		positions = append(positions, p)
	}

	return FundHoldings{
		Slug: fund.Slug, Name: fund.Name, Manager: fund.Manager,
		Period: filings[0].Period, Filed: filings[0].Filed,
		Count: len(latest), Value: total, Positions: positions,
	}, true
}

// classify labels a position against the prior quarter's share count. With no
// prior filing to compare, everything reads as "hold" (we can't infer a change).
func classify(shares int64, prev map[string]int64, cusip string) (string, float64) {
	if prev == nil {
		return "hold", 0
	}
	ps, ok := prev[cusip]
	if !ok || ps == 0 {
		return "new", 0
	}
	if shares == ps {
		return "hold", 0
	}
	delta := float64(shares-ps) / float64(ps) * 100
	switch {
	case delta >= 5:
		return "add", delta
	case delta <= -5:
		return "trim", delta
	default:
		return "hold", delta
	}
}
