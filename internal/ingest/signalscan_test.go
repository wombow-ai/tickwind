package ingest

import (
	"context"
	"testing"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

type fakeSignalCompute struct {
	m map[string]indicators.StockIndicatorsResult
}

func (f fakeSignalCompute) StockIndicators(_ context.Context, tk string) indicators.StockIndicatorsResult {
	return f.m[tk]
}

type fakeTickerSource struct{ t []string }

func (f fakeTickerSource) Tickers() []string { return f.t }

func rsiResult(v float64) indicators.StockIndicatorsResult {
	return indicators.StockIndicatorsResult{
		Indicators: []indicators.StockIndicator{
			{Indicator: indicators.Indicator{ID: "technical.rsi"}, Status: indicators.StatusOK, Value: &v},
		},
	}
}

func TestSignalScanCache(t *testing.T) {
	compute := fakeSignalCompute{m: map[string]indicators.StockIndicatorsResult{
		"AAPL": rsiResult(25), // oversold → bullish
		"MSFT": rsiResult(75), // overbought → bearish
		"TSLA": rsiResult(50), // neutral → no signal
	}}
	univ := fakeTickerSource{t: []string{"AAPL", "MSFT", "TSLA"}}

	c := NewSignalScanCache(compute, univ, nil)
	c.scan(context.Background())

	bull, at := c.Screen(indicators.SignalScreen{Direction: indicators.DirBullish})
	if at.IsZero() {
		t.Fatal("scan timestamp should be set after a scan")
	}
	if len(bull) != 1 || bull[0].Ticker != "AAPL" {
		t.Fatalf("bullish screen = %+v, want [AAPL]", bull)
	}

	bear, _ := c.Screen(indicators.SignalScreen{Direction: indicators.DirBearish})
	if len(bear) != 1 || bear[0].Ticker != "MSFT" {
		t.Fatalf("bearish screen = %+v, want [MSFT]", bear)
	}

	all, _ := c.Screen(indicators.SignalScreen{})
	if len(all) != 2 {
		t.Fatalf("unfiltered screen = %+v, want 2 (AAPL,MSFT; TSLA's neutral RSI emits no signal)", all)
	}

	// An empty-universe scan must KEEP the previous board, not blank it.
	c.universe = fakeTickerSource{t: nil}
	c.scan(context.Background())
	again, _ := c.Screen(indicators.SignalScreen{})
	if len(again) != 2 {
		t.Fatalf("empty-universe scan blanked the board: got %+v, want the previous 2", again)
	}
}
