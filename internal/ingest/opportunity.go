package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/sec"
	"github.com/wombow-ai/tickwind/internal/store"
)

// PriceSnapshotter returns the latest price per symbol in bulk (alpaca.Client).
type PriceSnapshotter interface {
	Snapshots(ctx context.Context, symbols []string) (map[string]float64, error)
}

// OpportunityIngestor builds the small-cap insider-buy "Opportunity" board. It
// sweeps the SEC daily Form-4 index for open-market purchases (persisting them),
// then periodically recomputes the ranked board from the trailing-30-day buys +
// live market caps into a shared Cache. Runs in its own goroutine (its cadence
// and SEC dependency differ from the main scheduler).
type OpportunityIngestor struct {
	store    store.Store
	sec      *sec.Client
	prices   PriceSnapshotter
	cache    *opportunity.Cache
	every    time.Duration
	backfill int // days of Form-4 history to seed on startup
	log      *slog.Logger

	seen     map[string]struct{} // processed accessions (seeded from store on start)
	shares   map[int]int64       // cik -> shares outstanding (cached)
	sharesAt time.Time
}

// NewOpportunityIngestor builds the ingestor. every is the recompute/forward-
// ingest cadence; backfillDays seeds recent history on startup.
func NewOpportunityIngestor(st store.Store, sc *sec.Client, prices PriceSnapshotter, cache *opportunity.Cache, every time.Duration, backfillDays int, log *slog.Logger) *OpportunityIngestor {
	return &OpportunityIngestor{
		store: st, sec: sc, prices: prices, cache: cache, every: every,
		backfill: backfillDays, log: log, seen: map[string]struct{}{}, shares: map[int]int64{},
	}
}

// Run blocks until ctx is cancelled.
func (o *OpportunityIngestor) Run(ctx context.Context) {
	o.loadSeen(ctx) // skip re-fetching Form-4s already processed before a restart
	o.refreshShares(ctx)
	o.recompute(ctx) // surface any already-persisted buys immediately
	now := time.Now().UTC()
	for d := o.backfill; d >= 0; d-- { // oldest → newest
		if ctx.Err() != nil {
			return
		}
		o.ingestDay(ctx, now.AddDate(0, 0, -d))
		o.recompute(ctx) // progressive: fill the board as each day completes
	}

	t := time.NewTicker(o.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n := time.Now().UTC()
			o.ingestDay(ctx, n.AddDate(0, 0, -1)) // late-disseminated filings
			o.ingestDay(ctx, n)
			if time.Since(o.sharesAt) > 18*time.Hour {
				o.refreshShares(ctx)
			}
			o.recompute(ctx)
		}
	}
}

// ingestDay fetches the daily Form-4 index for date and persists the open-market
// buys from filings not already seen (this run or, via the persisted seen-set, a
// previous one). Successfully-fetched accessions are recorded so a restart skips
// re-fetching them.
func (o *OpportunityIngestor) ingestDay(ctx context.Context, date time.Time) {
	refs, err := o.sec.DailyForm4(ctx, date)
	if err != nil {
		// Weekends/holidays/not-yet-published days 404 — expected, skip quietly.
		o.log.Debug("opportunity: no form-4 index", "date", date.Format("2006-01-02"), "err", err)
		return
	}
	var buys []store.InsiderBuy
	var seenNow []string // accessions fetched successfully this pass → persist
	fetched := 0
	for _, ref := range refs {
		if ctx.Err() != nil {
			return
		}
		if _, ok := o.seen[ref.Accession]; ok {
			continue // already processed (this run or before a restart)
		}
		fetched++
		f, err := o.sec.FetchForm4(ctx, ref)
		if err != nil {
			continue // transient fetch error → not marked seen, retried on a later pass
		}
		o.seen[ref.Accession] = struct{}{}
		seenNow = append(seenNow, ref.Accession)
		if !f.HasBuys() {
			continue // most Form 4s are awards/sales, not buys
		}
		var sh float64
		for _, b := range f.Buys {
			sh += b.Shares
		}
		val := f.BuyValue()
		price := 0.0
		if sh > 0 {
			price = val / sh
		}
		title := f.OfficerTitle
		if title == "" && f.IsDirector {
			title = "Director"
		}
		accNo := strings.ReplaceAll(ref.Accession, "-", "")
		buys = append(buys, store.InsiderBuy{
			Accession: ref.Accession, Ticker: f.Ticker, CIK: ref.CIK, Company: f.IssuerName,
			OwnerName: f.OwnerName, Title: title, IsOfficer: f.IsOfficer, IsDirector: f.IsDirector,
			FiledDate: date, Shares: sh, Price: price, Value: val,
			FilingURL: fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%d/%s/", ref.CIK, accNo),
		})
	}
	if len(seenNow) > 0 {
		if err := o.store.MarkForm4Seen(ctx, seenNow, date); err != nil {
			o.log.Warn("opportunity: mark form-4 seen", "err", err)
		}
	}
	if err := o.store.SaveInsiderBuys(ctx, buys); err != nil {
		o.log.Warn("opportunity: save insider buys", "err", err)
		return
	}
	if fetched > 0 {
		o.log.Info("opportunity: ingested form-4", "date", date.Format("2006-01-02"), "fetched", fetched, "buys", len(buys))
	}
}

// loadSeen seeds the in-memory seen-set from persisted processed accessions, so
// a restart skips re-fetching Form-4s we already handled (no full re-sweep). The
// window must cover everything the ingestor rescans — the backfill span plus the
// daily forward catch-up — with margin.
func (o *OpportunityIngestor) loadSeen(ctx context.Context) {
	days := o.backfill + 7
	if days < 40 {
		days = 40
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	accs, err := o.store.SeenForm4Since(ctx, since)
	if err != nil {
		o.log.Warn("opportunity: load seen form-4", "err", err)
		return
	}
	for _, a := range accs {
		o.seen[a] = struct{}{}
	}
	o.log.Info("opportunity: loaded seen form-4", "count", len(accs))
}

// refreshShares pulls shares-outstanding from the last few dei frames and merges.
func (o *OpportunityIngestor) refreshShares(ctx context.Context) {
	merged := make(map[int]int64)
	for _, qd := range recentQuarters(time.Now().UTC(), 3) {
		m, err := o.sec.Shares(ctx, qd[0], qd[1])
		if err != nil {
			o.log.Warn("opportunity: shares fetch failed", "quarter", fmt.Sprintf("CY%dQ%d", qd[0], qd[1]), "err", err)
			continue
		}
		for cik, v := range m {
			if v > merged[cik] {
				merged[cik] = v
			}
		}
	}
	if len(merged) > 0 {
		o.shares = merged
		o.sharesAt = time.Now().UTC()
	}
	o.log.Info("opportunity: refreshed shares", "ciks", len(merged))
}

// recompute rebuilds the board from the trailing-30-day buys + live prices.
func (o *OpportunityIngestor) recompute(ctx context.Context) {
	now := time.Now().UTC()
	buys, err := o.store.RecentInsiderBuys(ctx, now.AddDate(0, 0, -30))
	if err != nil {
		o.log.Warn("opportunity: recent buys", "err", err)
		return
	}
	if len(buys) == 0 {
		o.cache.Set([]opportunity.Stock{})
		return
	}
	seenT := make(map[string]struct{})
	var tickers []string
	for _, b := range buys {
		if !opportunity.ValidTicker(b.Ticker) {
			continue
		}
		if _, ok := seenT[b.Ticker]; ok {
			continue
		}
		seenT[b.Ticker] = struct{}{}
		tickers = append(tickers, b.Ticker)
	}
	prices, err := o.prices.Snapshots(ctx, tickers)
	if err != nil {
		o.log.Warn("opportunity: prices", "err", err)
		prices = map[string]float64{}
	}
	board := opportunity.Recompute(now, buys, o.shares, prices)
	o.cache.Set(board)
	o.log.Info("opportunity: recomputed board", "candidates", len(tickers), "rows", len(board))
}

// recentQuarters returns the n calendar quarters before the current one (the
// current quarter's dei frame is often not yet populated).
func recentQuarters(now time.Time, n int) [][2]int {
	y, q := now.Year(), (int(now.Month())-1)/3+1
	out := make([][2]int, 0, n)
	for i := 0; i < n; i++ {
		q--
		if q < 1 {
			q = 4
			y--
		}
		out = append(out, [2]int{y, q})
	}
	return out
}
