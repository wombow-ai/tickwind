package ingest

import (
	"context"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/cboe"
)

// optionsTTL is how long a fetched chain stays fresh. Cboe's feed is ~15-min
// delayed, so refetching more often than that buys nothing.
const optionsTTL = 15 * time.Minute

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
