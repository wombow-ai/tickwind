package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/sec"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

// fakeSnapshotter returns a fixed price map (or an error) for Snapshots, so the
// test can simulate a healthy fetch, a rate-limit error, and an empty result.
type fakeSnapshotter struct {
	prices map[string]float64
	err    error
}

func (f fakeSnapshotter) Snapshots(_ context.Context, _ []string) (map[string]float64, error) {
	return f.prices, f.err
}

// TestRecomputeKeepsLastGoodBoardOnPriceFailure is the regression for the empty
// Opportunity board: a transient price-fetch failure (an Alpaca 429) once
// overwrote a healthy board with an empty one, because recompute proceeded with
// an empty price map and every row was gated out on the price<=0 check. The fix
// keeps the last-good board when the price fetch fails (or prices nothing).
func TestRecomputeKeepsLastGoodBoardOnPriceFailure(t *testing.T) {
	now := time.Now().UTC()
	st := memory.New()
	// One qualifying small-cap buy: $300k, with dei shares → market cap in band
	// once a price is supplied.
	buy := store.InsiderBuy{
		Accession: "acc-1", Ticker: "AAT", CIK: 1500217, Company: "American Assets Trust",
		OwnerName: "Jane Insider", FiledDate: now.AddDate(0, 0, -1), Shares: 10000, Value: 300_000,
		FilingURL: "https://example.com/filing",
	}
	if err := st.SaveInsiderBuys(context.Background(), []store.InsiderBuy{buy}); err != nil {
		t.Fatalf("seed buys: %v", err)
	}
	cache := opportunity.NewCache()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	o := NewOpportunityIngestor(st, nil, fakeSnapshotter{prices: map[string]float64{"AAT": 21.0}}, cache, time.Hour, 0, log)
	o.shares = map[int]int64{1500217: 61_390_936} // ~$1.3B @ $21 → small-cap band

	// 1) Healthy fetch → board populates.
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after healthy recompute: got %d rows, want 1", got)
	}

	// 2) Rate-limit error → board MUST be preserved (the bug).
	o.prices = fakeSnapshotter{err: errors.New("alpaca: snapshots 429 Too Many Requests")}
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after 429: got %d rows, want last-good 1 (board was clobbered)", got)
	}

	// 3) Empty price map, no error (priced nothing) → also preserved.
	o.prices = fakeSnapshotter{prices: map[string]float64{}}
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after empty prices: got %d rows, want last-good 1", got)
	}

	// 4) Healthy fetch again → board still recomputes normally.
	o.prices = fakeSnapshotter{prices: map[string]float64{"AAT": 21.0}}
	o.recompute(context.Background())
	if got := len(cache.Get()); got != 1 {
		t.Fatalf("after recovery: got %d rows, want 1", got)
	}
}

// TestRefreshSharesFallback verifies the shares-coverage widening: the canonical
// dei cover-page frame is used when present, the us-gaap frame fills CIKs the dei
// frames left unresolved, a stale (frozen-ancient) us-gaap row is rejected rather
// than used (so that candidate is DROPPED, not given a wrong cap), and an issuer
// absent from BOTH frames stays off the board. It drives the real *sec.Client
// against an httptest server serving the dei + us-gaap shares frames.
func TestRefreshSharesFallback(t *testing.T) {
	// Four issuers among the trailing-30-day buys:
	//   100 — has dei shares → use dei
	//   200 — NO dei, has us-gaap shares → use fallback (the coverage win)
	//   300 — NO dei, only a frozen-ancient us-gaap row → rejected → dropped
	//   400 — absent from BOTH frames → dropped
	const (
		ckDei      = 100
		ckFallback = 200
		ckStale    = 300
		ckMissing  = 400
	)
	// The ingestor sweeps the 3 most-recent quarters; serve the same frame body for
	// every quarter (its CYyyyyQqI is echoed in the path). The dei frame holds only
	// CIK 100; the us-gaap frame holds 200 (fresh) and 300 (frozen 2011 instant).
	deiFrame := func(qe string) string {
		return fmt.Sprintf(`{"data":[{"cik":%d,"end":%q,"val":40000000}]}`, ckDei, qe)
	}
	gaapFrame := func(qe string) string {
		return fmt.Sprintf(`{"data":[{"cik":%d,"end":%q,"val":50000000},{"cik":%d,"end":"2011-12-31","val":941481}]}`,
			ckFallback, qe, ckStale)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path: /api/xbrl/frames/<tax>/<concept>/shares/CY2025Q1I.json — derive a
		// recent quarter-end so no row trips the staleness guard for the fresh rows.
		qe := "2025-03-31"
		switch {
		case strings.Contains(r.URL.Path, "/dei/EntityCommonStockSharesOutstanding/"):
			_, _ = w.Write([]byte(deiFrame(qe)))
		case strings.Contains(r.URL.Path, "/us-gaap/CommonStockSharesOutstanding/"):
			_, _ = w.Write([]byte(gaapFrame(qe)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	sc := sec.NewForTest("Tickwind (test@tickwind.com)", srv.URL)

	now := time.Now().UTC()
	st := memory.New()
	buys := []store.InsiderBuy{
		{Accession: "a1", Ticker: "DEII", CIK: ckDei, OwnerName: "A", FiledDate: now.AddDate(0, 0, -1), Shares: 1000, Value: 300_000, FilingURL: "u"},
		{Accession: "a2", Ticker: "FBCK", CIK: ckFallback, OwnerName: "B", FiledDate: now.AddDate(0, 0, -1), Shares: 1000, Value: 300_000, FilingURL: "u"},
		{Accession: "a3", Ticker: "STAL", CIK: ckStale, OwnerName: "C", FiledDate: now.AddDate(0, 0, -1), Shares: 1000, Value: 300_000, FilingURL: "u"},
		{Accession: "a4", Ticker: "MISS", CIK: ckMissing, OwnerName: "D", FiledDate: now.AddDate(0, 0, -1), Shares: 1000, Value: 300_000, FilingURL: "u"},
	}
	if err := st.SaveInsiderBuys(context.Background(), buys); err != nil {
		t.Fatalf("seed buys: %v", err)
	}
	cache := opportunity.NewCache()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Price each at $20 → cap = 20 × 40-50M ≈ $0.8B-$1B, inside the small-cap band.
	prices := map[string]float64{"DEII": 20, "FBCK": 20, "STAL": 20, "MISS": 20}
	o := NewOpportunityIngestor(st, sc, fakeSnapshotter{prices: prices}, cache, time.Hour, 0, log)

	o.refreshShares(context.Background())
	if o.shares[ckDei] != 40_000_000 {
		t.Errorf("dei CIK: shares = %d, want 40000000 (dei used)", o.shares[ckDei])
	}
	if o.shares[ckFallback] != 50_000_000 {
		t.Errorf("fallback CIK: shares = %d, want 50000000 (us-gaap fallback used)", o.shares[ckFallback])
	}
	if v, ok := o.shares[ckStale]; ok {
		t.Errorf("stale us-gaap CIK should be dropped, got %d", v)
	}
	if v, ok := o.shares[ckMissing]; ok {
		t.Errorf("CIK absent from both frames should be dropped, got %d", v)
	}

	o.recompute(context.Background())
	board := cache.Get()
	onBoard := map[string]bool{}
	for _, s := range board {
		onBoard[s.Ticker] = true
	}
	if !onBoard["DEII"] {
		t.Error("DEII (dei shares) should be on the board")
	}
	if !onBoard["FBCK"] {
		t.Error("FBCK (us-gaap fallback) should be on the board — the coverage win")
	}
	if onBoard["STAL"] {
		t.Error("STAL (stale shares) must NOT be on the board (insufficient, not wrong)")
	}
	if onBoard["MISS"] {
		t.Error("MISS (no shares at all) must NOT be on the board")
	}
}

// TestRecomputeEmptyWhenNoBuys confirms the genuine-no-data path still empties
// the board (so a real quiet period is honestly reflected, not frozen stale).
func TestRecomputeEmptyWhenNoBuys(t *testing.T) {
	st := memory.New()
	cache := opportunity.NewCache()
	cache.Set([]opportunity.Stock{{Ticker: "STALE"}}) // pretend a prior board exists
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	o := NewOpportunityIngestor(st, nil, fakeSnapshotter{prices: map[string]float64{}}, cache, time.Hour, 0, log)

	o.recompute(context.Background())
	if got := len(cache.Get()); got != 0 {
		t.Fatalf("no buys should empty the board: got %d rows, want 0", got)
	}
}
