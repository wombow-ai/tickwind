package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

// AlertStore is the slice of the store the evaluator needs.
type AlertStore interface {
	ListActiveAlerts(ctx context.Context) ([]store.Alert, error)
	MarkAlertTriggered(ctx context.Context, id string, at time.Time) error
	ListFilings(ctx context.Context, ticker string, limit int) ([]store.Filing, error)
}

// PriceLatest fetches the latest quote for a ticker (ingest.BarCache satisfies it).
type PriceLatest interface {
	LatestQuote(ctx context.Context, ticker string) (store.Quote, bool, error)
}

// AlertEvaluator periodically checks active user alerts against the latest price
// and stamps matches as triggered. Runs in its own goroutine off the request
// path (like the pruner / opportunity ingestor). Currently handles price_above /
// price_below; pct_move and new_filing are added next.
type AlertEvaluator struct {
	store  AlertStore
	prices PriceLatest
	every  time.Duration
	log    *slog.Logger
}

// NewAlertEvaluator builds the evaluator; every is the check cadence.
func NewAlertEvaluator(st AlertStore, prices PriceLatest, every time.Duration, log *slog.Logger) *AlertEvaluator {
	return &AlertEvaluator{store: st, prices: prices, every: every, log: log}
}

// Run blocks until ctx is cancelled.
func (e *AlertEvaluator) Run(ctx context.Context) {
	t := time.NewTicker(e.every)
	defer t.Stop()
	e.evaluate(ctx) // once on startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.evaluate(ctx)
		}
	}
}

// tickerData caches a ticker's latest quote + newest filing time for one
// evaluate cycle (fetched lazily, once per ticker).
type tickerData struct {
	q          store.Quote
	haveQuote  bool
	lastFiling time.Time
	haveFiling bool
}

func (e *AlertEvaluator) evaluate(ctx context.Context) {
	alerts, err := e.store.ListActiveAlerts(ctx)
	if err != nil {
		e.log.Warn("alerts: list active", "err", err)
		return
	}
	if len(alerts) == 0 {
		return
	}
	cache := make(map[string]*tickerData)
	fired := 0
	now := time.Now().UTC()
	for _, a := range alerts {
		d := cache[a.Ticker]
		if d == nil {
			d = &tickerData{}
			cache[a.Ticker] = d
		}
		var hit bool
		if a.Kind == "new_filing" {
			if !d.haveFiling {
				if fs, ferr := e.store.ListFilings(ctx, a.Ticker, 5); ferr == nil {
					for _, f := range fs {
						if f.FiledAt.After(d.lastFiling) {
							d.lastFiling = f.FiledAt
						}
					}
				}
				d.haveFiling = true
			}
			hit = !d.lastFiling.IsZero() && d.lastFiling.After(a.CreatedAt)
		} else {
			if !d.haveQuote {
				if q, found, qerr := e.prices.LatestQuote(ctx, a.Ticker); qerr == nil && found {
					d.q = q
				}
				d.haveQuote = true
			}
			hit = d.q.Price > 0 && priceAlertHit(a, d.q)
		}
		if !hit {
			continue
		}
		if err := e.store.MarkAlertTriggered(ctx, a.ID, now); err != nil {
			e.log.Warn("alerts: mark triggered", "id", a.ID, "err", err)
			continue
		}
		fired++
	}
	if fired > 0 {
		e.log.Info("alerts: triggered", "count", fired, "checked", len(alerts))
	}
}

// priceAlertHit reports whether a price-based alert (price_above / price_below /
// pct_move) is met by the latest quote. new_filing is handled in evaluate.
func priceAlertHit(a store.Alert, q store.Quote) bool {
	switch a.Kind {
	case "price_above":
		return q.Price >= a.Threshold
	case "price_below":
		return q.Price <= a.Threshold
	case "pct_move":
		if q.PrevClose <= 0 {
			return false
		}
		pct := (q.Price - q.PrevClose) / q.PrevClose * 100
		if pct < 0 {
			pct = -pct
		}
		return pct >= a.Threshold
	default:
		return false
	}
}
