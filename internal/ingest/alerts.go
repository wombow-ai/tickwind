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

func (e *AlertEvaluator) evaluate(ctx context.Context) {
	alerts, err := e.store.ListActiveAlerts(ctx)
	if err != nil {
		e.log.Warn("alerts: list active", "err", err)
		return
	}
	if len(alerts) == 0 {
		return
	}
	prices := make(map[string]float64) // ticker -> latest price (0 = unavailable); dedupes fetches this cycle
	fired := 0
	now := time.Now().UTC()
	for _, a := range alerts {
		px, seen := prices[a.Ticker]
		if !seen {
			if q, found, qerr := e.prices.LatestQuote(ctx, a.Ticker); qerr == nil && found {
				px = q.Price
			}
			prices[a.Ticker] = px
		}
		if px <= 0 || !alertHit(a, px) {
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

// alertHit reports whether a price-based alert's condition is met. pct_move and
// new_filing need prev-close / filings and are evaluated elsewhere (not here).
func alertHit(a store.Alert, price float64) bool {
	switch a.Kind {
	case "price_above":
		return price >= a.Threshold
	case "price_below":
		return price <= a.Threshold
	default:
		return false
	}
}
