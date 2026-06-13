package ingest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/cboe"
)

// optionsTTL is how long a fetched chain stays fresh. Cboe's feed is ~15-min
// delayed, so refetching more often than that buys nothing.
const optionsTTL = 15 * time.Minute

// unusualScan is the set of heavily-optioned US names the whole-market unusual-
// activity board scans. Kept to liquid mega-caps, meme stocks and major ETFs —
// where outsized single-contract volume actually signals something.
var unusualScan = []string{
	"AAPL", "NVDA", "TSLA", "AMD", "MSFT", "META", "AMZN", "GOOGL", "NFLX", "AVGO",
	"MU", "INTC", "SMCI", "PLTR", "COIN", "HOOD", "MSTR", "SOFI", "BABA", "NIO",
	"F", "BAC", "AAL", "CCL", "GME", "AMC", "UBER", "DIS", "PYPL", "CRM",
	"SPY", "QQQ", "IWM", "TQQQ", "SQQQ", "ARKK", "GLD", "TLT", "SLV", "XLF",
}

// unusualTopN caps the board size; unusualRefresh is the scan cadence.
const (
	unusualTopN    = 30
	unusualRefresh = 30 * time.Minute
)

// OptionsView is the per-stock options summary served to the API: put/call
// ratios, max pain (nearest expiry), and the open-interest leaders.
type OptionsView struct {
	Ticker   string          `json:"ticker"`
	PCVolume float64         `json:"pc_volume"`
	PCOI     float64         `json:"pc_oi"`
	MaxPain  float64         `json:"max_pain,omitempty"`
	Expiry   string          `json:"expiry,omitempty"` // the expiry max_pain/used for
	TopOI    []cboe.Contract `json:"top_oi"`
	At       time.Time       `json:"updated_at"`
}

// OptionsSource is the slice of *cboe.Client the cache needs.
type OptionsSource interface {
	Options(ctx context.Context, ticker string) (cboe.Chain, bool, error)
}

// OptionsCache serves per-stock options overviews, computed on demand from the
// Cboe delayed chain and cached per ticker (15-min TTL). Concurrent first
// requests for a ticker are deduped so one chain fetch serves them all.
type OptionsCache struct {
	src OptionsSource

	mu       sync.Mutex
	cache    map[string]optionsEntry
	inflight map[string]chan struct{}

	// Whole-market unusual-activity board, refreshed by Run.
	unusualMu sync.RWMutex
	unusual   []UnusualContract
	unusualAt time.Time
}

// UnusualContract is one contract on the unusual-activity board: the per-stock
// chain contract plus its parent ticker and volume/OI ratio.
type UnusualContract struct {
	Ticker string  `json:"ticker"`
	Type   string  `json:"type"`
	Strike float64 `json:"strike"`
	Expiry string  `json:"expiry"`
	Volume int64   `json:"volume"`
	OI     int64   `json:"oi"`
	VolOI  float64 `json:"vol_oi"` // volume ÷ open interest (0 when OI is 0)
	IV     float64 `json:"iv"`
}

type optionsEntry struct {
	view OptionsView
	ok   bool // false = symbol has no options (negative-cached to avoid refetch storms)
	at   time.Time
}

// NewOptionsCache builds the cache.
func NewOptionsCache(src OptionsSource) *OptionsCache {
	return &OptionsCache{src: src, cache: map[string]optionsEntry{}, inflight: map[string]chan struct{}{}}
}

// Options returns the cached overview for a ticker (ok=false when the symbol
// has no listed options). Computes + caches on a miss; dedupes concurrent
// first requests.
func (c *OptionsCache) Options(ctx context.Context, ticker string) (OptionsView, bool) {
	for {
		c.mu.Lock()
		if e, fresh := c.cache[ticker]; fresh && time.Since(e.at) < optionsTTL {
			c.mu.Unlock()
			return e.view, e.ok
		}
		ch, busy := c.inflight[ticker]
		if !busy {
			break
		}
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return OptionsView{}, false
		}
	}
	ch := make(chan struct{})
	c.inflight[ticker] = ch
	c.mu.Unlock()

	view, ok := c.compute(ctx, ticker)
	c.mu.Lock()
	c.cache[ticker] = optionsEntry{view: view, ok: ok, at: time.Now()}
	delete(c.inflight, ticker)
	close(ch)
	c.mu.Unlock()
	return view, ok
}

// Cached returns a ticker's options view from the in-memory cache ONLY, without
// triggering a live Cboe chain fetch on a miss. It is for latency-sensitive
// readers on the request path (e.g. the deep-research report's data-only
// assembler, which must stay cheap) that must not block on a multi-MB fetch: a
// cold or stale entry returns ok=false and the caller simply omits the options
// data. The background Run scan and the on-demand Options path keep the cache
// warm for liquid names.
func (c *OptionsCache) Cached(ticker string) (OptionsView, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, fresh := c.cache[ticker]; fresh && time.Since(e.at) < optionsTTL {
		return e.view, e.ok
	}
	return OptionsView{}, false
}

// Unusual returns the latest whole-market unusual-activity board (top contracts
// by single-contract volume) and when it was built. nil before the first scan.
func (c *OptionsCache) Unusual() ([]UnusualContract, time.Time) {
	c.unusualMu.RLock()
	defer c.unusualMu.RUnlock()
	return c.unusual, c.unusualAt
}

// Run scans the unusual list immediately and then every unusualRefresh until
// ctx is cancelled. Chains are fetched serially with a polite gap (each is
// ~1-2 MB) on a background goroutine, off the request path.
func (c *OptionsCache) Run(ctx context.Context) {
	c.scanUnusual(ctx)
	t := time.NewTicker(unusualRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.scanUnusual(ctx)
		}
	}
}

func (c *OptionsCache) scanUnusual(ctx context.Context) {
	var all []UnusualContract
	for i, tk := range unusualScan {
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second): // polite gap between large chain pulls
			}
		}
		chain, ok, err := c.src.Options(ctx, tk)
		if err != nil || !ok {
			continue
		}
		for _, ct := range chain.Contracts {
			if ct.Volume <= 0 {
				continue
			}
			var volOI float64
			if ct.OI > 0 {
				volOI = float64(ct.Volume) / float64(ct.OI)
			}
			all = append(all, UnusualContract{
				Ticker: tk, Type: ct.Type, Strike: ct.Strike, Expiry: ct.Expiry,
				Volume: ct.Volume, OI: ct.OI, VolOI: volOI, IV: ct.IV,
			})
		}
	}
	if len(all) == 0 {
		return // total miss (e.g. CDN down) — keep the previous board
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Volume > all[j].Volume })
	if len(all) > unusualTopN {
		all = all[:unusualTopN]
	}
	c.unusualMu.Lock()
	c.unusual, c.unusualAt = all, time.Now().UTC()
	c.unusualMu.Unlock()
}

func (c *OptionsCache) compute(ctx context.Context, ticker string) (OptionsView, bool) {
	chain, ok, err := c.src.Options(ctx, ticker)
	if err != nil || !ok {
		return OptionsView{}, false
	}
	pv, po := cboe.PutCallRatio(chain.Contracts)
	today := time.Now().UTC().Format("2006-01-02")
	exp := cboe.NearestExpiry(chain.Contracts, today)
	return OptionsView{
		Ticker:   ticker,
		PCVolume: pv,
		PCOI:     po,
		MaxPain:  cboe.MaxPain(chain.Contracts, exp),
		Expiry:   exp,
		TopOI:    cboe.OITop(chain.Contracts, 10),
		At:       chain.At,
	}, true
}
