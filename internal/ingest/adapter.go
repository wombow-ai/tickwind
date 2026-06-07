package ingest

import (
	"context"
	"sync"
	"time"

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
