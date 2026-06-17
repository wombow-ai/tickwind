package ingest

import (
	"context"
	"sync"
	"time"

	"github.com/wombow-ai/tickwind/internal/brapi"
	"github.com/wombow-ai/tickwind/internal/dart"
	"github.com/wombow-ai/tickwind/internal/krx"
	"github.com/wombow-ai/tickwind/internal/market"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/symbols"
	"github.com/wombow-ai/tickwind/internal/tpex"
	"github.com/wombow-ai/tickwind/internal/twse"
)

// MarketAdapter pulls per-market data for one suffixed ticker (e.g. "2330.TW").
// Returning ok=false / nil means "this market has no such facet right now" — the
// scheduler/poller treat that as nothing-to-ingest, not an error. US has no
// adapter, so the existing EDGAR/Alpaca/Finnhub path stays the default branch.
type MarketAdapter interface {
	Market() market.Market
	Quote(ctx context.Context, ticker string) (store.Quote, bool, error)
	Filings(ctx context.Context, ticker string) (store.Security, []store.Filing, bool, error)
	News(ctx context.Context, ticker string) ([]store.News, error)
}

// twSource is the subset of the twse/tpex clients the Taiwan adapter needs
// (an interface so the adapter is unit-testable without network).
type twSource interface {
	EODQuotes(ctx context.Context) (map[string]store.Quote, error)
	Companies(ctx context.Context) ([]symbols.Symbol, error)
}

// TWAdapter serves Taiwan EOD prices + company names for the main board (.TW,
// TWSE) and OTC (.TWO, TPEx) from the free open APIs. It caches each day's
// whole-market table (one fetch prices every symbol) and refreshes when stale.
// Per-symbol filings/news aren't available for TW yet (Filings returns only the
// Security so the stock page shows the name; News returns nil).
type TWAdapter struct {
	sources []twSource
	ttl     time.Duration

	mu     sync.Mutex
	quotes map[string]store.Quote
	names  map[string]string
	at     time.Time
}

// NewTWAdapter builds the Taiwan adapter from the TWSE + TPEx clients.
func NewTWAdapter(tw *twse.Client, tp *tpex.Client) *TWAdapter {
	return &TWAdapter{sources: []twSource{tw, tp}, ttl: time.Hour}
}

// Market identifies this adapter's venue.
func (a *TWAdapter) Market() market.Market { return market.TW }

// ensureFresh refreshes the cached whole-market tables when stale. Each source
// is best-effort: one board failing (e.g. TPEx) never drops the other.
func (a *TWAdapter) ensureFresh(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.quotes) > 0 && time.Since(a.at) < a.ttl {
		return
	}
	quotes := make(map[string]store.Quote)
	names := make(map[string]string)
	got := false
	for _, src := range a.sources {
		if qs, err := src.EODQuotes(ctx); err == nil {
			for tk, q := range qs {
				quotes[tk] = q
			}
			got = true
		}
		if cos, err := src.Companies(ctx); err == nil {
			for _, c := range cos {
				names[c.Ticker] = c.Name
			}
		}
	}
	if got {
		a.quotes, a.names, a.at = quotes, names, time.Now()
	}
}

// Quote returns the cached EOD quote for ticker (ok=false if not listed).
func (a *TWAdapter) Quote(ctx context.Context, ticker string) (store.Quote, bool, error) {
	a.ensureFresh(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	q, ok := a.quotes[ticker]
	return q, ok, nil
}

// Filings returns the Security (so the stock page shows the company name +
// market); TW per-symbol filings aren't wired yet, so the filing list is nil.
func (a *TWAdapter) Filings(ctx context.Context, ticker string) (store.Security, []store.Filing, bool, error) {
	a.ensureFresh(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	name := a.names[ticker]
	if name == "" {
		return store.Security{}, nil, false, nil
	}
	return store.Security{Ticker: ticker, Name: name, Market: string(market.TW)}, nil, true, nil
}

// News is not available for TW yet.
func (a *TWAdapter) News(ctx context.Context, ticker string) ([]store.News, error) { return nil, nil }

// krxClient / dartClient are the subsets of the KR clients the adapter needs.
type krxClient interface {
	EODQuotes(ctx context.Context) (map[string]store.Quote, error)
	Companies(ctx context.Context) ([]symbols.Symbol, error)
}
type dartClient interface {
	CorpCodeMap(ctx context.Context) (map[string]string, error)
	RecentFilings(ctx context.Context, ticker, corpCode string, limit int) ([]store.Filing, error)
}

// KRAdapter serves Korea EOD prices + names (KRX) and filings (OpenDART) for
// KOSPI (.KS) + KOSDAQ (.KQ). Quotes/names are cached hourly (one KRX call per
// board prices the whole market); the corp-code map (ticker → DART id) is
// fetched once (it changes rarely). Filings come from OpenDART when keyed.
type KRAdapter struct {
	krx  krxClient
	dart dartClient
	ttl  time.Duration

	mu     sync.Mutex
	quotes map[string]store.Quote
	names  map[string]string
	corp   map[string]string // 6-digit stock code → DART corp_code
	at     time.Time
}

// NewKRAdapter builds the Korea adapter from the KRX + OpenDART clients.
func NewKRAdapter(k *krx.Client, d *dart.Client) *KRAdapter {
	return &KRAdapter{krx: k, dart: d, ttl: time.Hour}
}

// Market identifies this adapter's venue.
func (a *KRAdapter) Market() market.Market { return market.KR }

func (a *KRAdapter) ensureFresh(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.corp) == 0 { // corp codes change rarely → fetch once
		if m, err := a.dart.CorpCodeMap(ctx); err == nil && len(m) > 0 {
			a.corp = m
		}
	}
	if len(a.quotes) > 0 && time.Since(a.at) < a.ttl {
		return
	}
	quotes, err := a.krx.EODQuotes(ctx)
	if err != nil || len(quotes) == 0 {
		return // keep last good
	}
	names := make(map[string]string)
	if cos, err := a.krx.Companies(ctx); err == nil {
		for _, c := range cos {
			names[c.Ticker] = c.Name
		}
	}
	a.quotes, a.names, a.at = quotes, names, time.Now()
}

// Quote returns the cached EOD quote for ticker (ok=false if not listed).
func (a *KRAdapter) Quote(ctx context.Context, ticker string) (store.Quote, bool, error) {
	a.ensureFresh(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	q, ok := a.quotes[ticker]
	return q, ok, nil
}

// Filings returns the Security (name + market) and, when OpenDART is keyed and
// the ticker maps to a corp_code, its recent disclosures.
func (a *KRAdapter) Filings(ctx context.Context, ticker string) (store.Security, []store.Filing, bool, error) {
	a.ensureFresh(ctx)
	a.mu.Lock()
	name := a.names[ticker]
	corp := a.corp[market.Base(ticker)]
	a.mu.Unlock()
	if name == "" {
		return store.Security{}, nil, false, nil
	}
	sec := store.Security{Ticker: ticker, Name: name, Market: string(market.KR)}
	var filings []store.Filing
	if corp != "" { // network call outside the lock
		if f, err := a.dart.RecentFilings(ctx, ticker, corp, 25); err == nil {
			filings = f
		}
	}
	return sec, filings, true, nil
}

// News is not wired for KR yet.
func (a *KRAdapter) News(ctx context.Context, ticker string) ([]store.News, error) { return nil, nil }

// brapiSource is the subset of the brapi client the Brazil adapter needs.
type brapiSource interface {
	Quote(ctx context.Context, symbol string) (brapi.Quote, bool, error)
}

// BRAdapter serves Brazilian B3 (Bovespa) delayed quotes + company names for
// .SA tickers via brapi.dev. Like the HK adapter this is an owner-authorized
// convenience source (delayed, free token), not a redistribution-clean feed;
// per-ticker quotes are cached briefly so the poller doesn't hammer brapi.
// Tickwind's canonical ticker carries the ".SA" suffix (PETR4.SA) for venue
// routing; brapi itself wants the bare code, so calls strip the suffix.
type BRAdapter struct {
	src brapiSource
	ttl time.Duration

	mu    sync.Mutex
	cache map[string]brEntry
}

type brEntry struct {
	q    store.Quote
	name string
	at   time.Time
}

// NewBRAdapter builds the Brazil adapter from the brapi client.
func NewBRAdapter(c *brapi.Client) *BRAdapter {
	return &BRAdapter{src: c, ttl: time.Minute, cache: map[string]brEntry{}}
}

// Market identifies this adapter's venue.
func (a *BRAdapter) Market() market.Market { return market.BR }

// fetch returns a cached entry for ticker (canonical ".SA" form), refreshing
// from brapi when stale and falling back to the last good value on error.
func (a *BRAdapter) fetch(ctx context.Context, ticker string) (brEntry, bool) {
	a.mu.Lock()
	if e, ok := a.cache[ticker]; ok && time.Since(e.at) < a.ttl {
		a.mu.Unlock()
		return e, true
	}
	a.mu.Unlock()

	bq, ok, err := a.src.Quote(ctx, market.Base(ticker)) // bare code; outside the lock
	if err != nil || !ok {
		a.mu.Lock()
		e, had := a.cache[ticker]
		a.mu.Unlock()
		return e, had // last good (if any)
	}
	e := brEntry{
		q: store.Quote{
			Ticker:    ticker,
			Price:     bq.Price,
			PrevClose: bq.PrevClose,
			Session:   brClockSession(time.Now()),
			Source:    "brapi",
			At:        bq.At,
		},
		name: bq.Name,
		at:   time.Now(),
	}
	a.mu.Lock()
	a.cache[ticker] = e
	a.mu.Unlock()
	return e, true
}

// Quote returns the cached delayed quote for ticker (ok=false if unknown).
func (a *BRAdapter) Quote(ctx context.Context, ticker string) (store.Quote, bool, error) {
	e, ok := a.fetch(ctx, ticker)
	if !ok || e.q.Price == 0 {
		return store.Quote{}, false, nil
	}
	return e.q, true, nil
}

// Filings returns the Security (name + market) so the stock page shows the
// company; B3 per-symbol filings (CVM) aren't wired yet.
func (a *BRAdapter) Filings(ctx context.Context, ticker string) (store.Security, []store.Filing, bool, error) {
	e, ok := a.fetch(ctx, ticker)
	if !ok || e.name == "" {
		return store.Security{}, nil, false, nil
	}
	return store.Security{Ticker: ticker, Name: e.name, Market: string(market.BR)}, nil, true, nil
}

// News is not wired for BR yet.
func (a *BRAdapter) News(ctx context.Context, ticker string) ([]store.News, error) { return nil, nil }

// brClockSession approximates the B3 session from the wall clock. Brazil has no
// DST (since 2019) so BRT is a fixed UTC-3; regular hours are ~10:00–17:00 BRT.
// Informational only — price/change are exact regardless.
func brClockSession(now time.Time) string {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*60*60)
	}
	t := now.In(loc)
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return "closed"
	}
	mins := t.Hour()*60 + t.Minute()
	if mins >= 10*60 && mins <= 17*60 {
		return "regular"
	}
	return "closed"
}
