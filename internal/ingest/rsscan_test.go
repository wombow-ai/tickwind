package ingest

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

func mkRS(ticker, window string, rel float64) indicators.TickerRelStrength {
	return indicators.TickerRelStrength{Ticker: ticker, RS: indicators.RelativeStrength{
		Benchmark: "SPY",
		Windows:   []indicators.RelStrengthWindow{{Label: window, Relative: rel}},
	}}
}

// emptyCandles is an ERCandleSource that returns no candles, so the on-demand RS compute path fails
// gracefully (insufficient-not-wrong) rather than panicking on a nil source.
type emptyCandles struct{}

func (emptyCandles) DailyCandles(context.Context, string) ([]store.Candle, error) { return nil, nil }

func TestRSPercentile(t *testing.T) {
	// Population of 10 peers with 3M excess returns 0,1,…,9 pp.
	c := &RelativeStrengthCache{candles: emptyCandles{}, at: time.Now().UTC()}
	for i := 0; i < 10; i++ {
		c.pop = append(c.pop, mkRS("T"+strconv.Itoa(i), "3M", float64(i)))
	}

	// A target IN the population (T7, rel=7): 7 of 10 peers rank below → 70th percentile.
	pct, n, _, ok := c.RSPercentile(context.Background(), "T7", "3M")
	if !ok || n != 10 || pct != 70 {
		t.Fatalf("RSPercentile(T7) = %v/%d/%v, want 70/10/true", pct, n, ok)
	}

	// Case-insensitive ticker match.
	if p, _, _, ok := c.RSPercentile(context.Background(), "t0", "3M"); !ok || p != 0 {
		t.Fatalf("RSPercentile(t0) = %v/%v, want 0/true (lowest, nothing below)", p, ok)
	}

	// A target NOT in the population, with no candles → on-demand compute fails → withheld.
	if _, _, _, ok := c.RSPercentile(context.Background(), "ZZZZ", "3M"); ok {
		t.Fatal("RSPercentile(ZZZZ) ok=true; want false (not in universe + no candles to compute)")
	}

	// Population below the floor (minRSPopulation) → withheld, never a misleading percentile.
	small := &RelativeStrengthCache{candles: emptyCandles{}, at: time.Now().UTC()}
	for i := 0; i < minRSPopulation-1; i++ {
		small.pop = append(small.pop, mkRS("S"+strconv.Itoa(i), "3M", float64(i)))
	}
	if _, _, _, ok := small.RSPercentile(context.Background(), "S0", "3M"); ok {
		t.Fatalf("RSPercentile with %d peers ok=true; want false (below minRSPopulation %d)", minRSPopulation-1, minRSPopulation)
	}
}
