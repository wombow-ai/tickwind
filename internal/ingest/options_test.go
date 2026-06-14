package ingest

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/wombow-ai/tickwind/internal/cboe"
)

// fakeOptionsSource is a fake OptionsSource that returns a canned chain for any
// ticker it is configured to know, counting how many times each ticker is
// fetched so the test can assert scanUnusual pulls each chain exactly once.
type fakeOptionsSource struct {
	mu     sync.Mutex
	chains map[string]cboe.Chain
	calls  map[string]int
}

func (f *fakeOptionsSource) Options(_ context.Context, ticker string) (cboe.Chain, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[ticker]++
	ch, ok := f.chains[ticker]
	return ch, ok, nil
}

// chainWithMagnet returns a small but well-formed chain with three distinct
// OI-bearing strikes at one expiry (enough to satisfy MaxPain's minimum) plus
// volume so the unusual board has something to rank.
func chainWithMagnet(expiry string) cboe.Chain {
	return cboe.Chain{
		Contracts: []cboe.Contract{
			{Type: "C", Strike: 100, Expiry: expiry, OI: 500, Volume: 300},
			{Type: "C", Strike: 110, Expiry: expiry, OI: 400, Volume: 200},
			{Type: "C", Strike: 120, Expiry: expiry, OI: 300, Volume: 100},
			{Type: "P", Strike: 100, Expiry: expiry, OI: 450, Volume: 250},
			{Type: "P", Strike: 110, Expiry: expiry, OI: 350, Volume: 150},
			{Type: "P", Strike: 120, Expiry: expiry, OI: 250, Volume: 50},
		},
	}
}

// TestScanUnusualWarmsCache proves that scanUnusual now writes the per-ticker
// OptionsView into c.cache for each scanned ticker (so research reports get the
// options block without an on-demand /options hit) while fetching each chain
// exactly once.
func TestScanUnusualWarmsCache(t *testing.T) {
	// Drive a small scan set so the test doesn't pay the real ~1s inter-ticker
	// polite gap × 40 names (the gap itself is exercised — two tickers means it
	// fires once — proving it stays intact). Restore the production list after.
	orig := unusualScan
	unusualScan = []string{"AAPL", "NVDA"}
	defer func() { unusualScan = orig }()

	// Use a far-future expiry so NearestExpiry/MaxPain see it as a valid magnet
	// regardless of the day the test runs.
	const expiry = "2099-12-18"
	src := &fakeOptionsSource{
		chains: map[string]cboe.Chain{},
		calls:  map[string]int{},
	}
	for _, tk := range unusualScan {
		src.chains[tk] = chainWithMagnet(expiry)
	}
	c := NewOptionsCache(src)

	c.scanUnusual(context.Background())

	// Every scan ticker must now be in the cache via Cached (cache-only, the
	// exact path the research report uses), with the view viewFromChain builds.
	want, ok := viewFromChain(unusualScan[0], chainWithMagnet(expiry))
	if !ok {
		t.Fatalf("viewFromChain returned ok=false for a well-formed chain")
	}
	for _, tk := range unusualScan {
		got, ok := c.Cached(tk)
		if !ok {
			t.Fatalf("Cached(%q) returned ok=false; scanUnusual did not warm the cache", tk)
		}
		if got.PCVolume != want.PCVolume || got.PCOI != want.PCOI {
			t.Errorf("Cached(%q) p/c ratios = (%v,%v), want (%v,%v)", tk, got.PCVolume, got.PCOI, want.PCVolume, want.PCOI)
		}
		if got.MaxPain != want.MaxPain {
			t.Errorf("Cached(%q) MaxPain = %v, want %v", tk, got.MaxPain, want.MaxPain)
		}
		if got.Expiry != want.Expiry {
			t.Errorf("Cached(%q) Expiry = %q, want %q", tk, got.Expiry, want.Expiry)
		}
		if got.Ticker != tk {
			t.Errorf("Cached(%q) Ticker = %q, want %q", tk, got.Ticker, tk)
		}
	}

	// Chain fetched exactly once per ticker — no double Cboe pull from the new
	// cache-warming (board + cache share the one fetch).
	src.mu.Lock()
	defer src.mu.Unlock()
	for _, tk := range unusualScan {
		if src.calls[tk] != 1 {
			t.Errorf("ticker %q fetched %d times, want exactly 1", tk, src.calls[tk])
		}
	}

	// The board is still built from the same scan.
	board, _ := c.Unusual()
	if len(board) == 0 {
		t.Errorf("unusual board is empty after scanUnusual; board building regressed")
	}
}

// TestComputeMatchesViewFromChain confirms the extraction is behavior-preserving:
// compute (which fetches then builds) yields the same view as viewFromChain on
// the same chain.
func TestComputeMatchesViewFromChain(t *testing.T) {
	const expiry = "2099-12-18"
	chain := chainWithMagnet(expiry)
	src := &fakeOptionsSource{
		chains: map[string]cboe.Chain{"AAPL": chain},
		calls:  map[string]int{},
	}
	c := NewOptionsCache(src)

	got, ok := c.compute(context.Background(), "AAPL")
	if !ok {
		t.Fatalf("compute returned ok=false for a well-formed chain")
	}
	want, _ := viewFromChain("AAPL", chain)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compute view = %+v, viewFromChain view = %+v; extraction changed behavior", got, want)
	}
}
