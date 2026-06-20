package ingest

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/store"
)

func TestPriceAlertHit(t *testing.T) {
	q := func(price, prev float64) store.Quote { return store.Quote{Price: price, PrevClose: prev} }
	cases := []struct {
		name string
		a    store.Alert
		q    store.Quote
		want bool
	}{
		{"above hit", store.Alert{Kind: "price_above", Threshold: 100}, q(105, 0), true},
		{"above miss", store.Alert{Kind: "price_above", Threshold: 100}, q(95, 0), false},
		{"below hit", store.Alert{Kind: "price_below", Threshold: 100}, q(95, 0), true},
		{"below miss", store.Alert{Kind: "price_below", Threshold: 100}, q(105, 0), false},
		{"pct up hit", store.Alert{Kind: "pct_move", Threshold: 5}, q(110, 100), true},  // +10%
		{"pct down hit", store.Alert{Kind: "pct_move", Threshold: 5}, q(90, 100), true}, // -10%
		{"pct miss", store.Alert{Kind: "pct_move", Threshold: 5}, q(102, 100), false},   // +2%
		{"pct no prevclose", store.Alert{Kind: "pct_move", Threshold: 5}, q(110, 0), false},
		{"new_filing not price", store.Alert{Kind: "new_filing"}, q(110, 100), false},
	}
	for _, c := range cases {
		if got := priceAlertHit(c.a, c.q); got != c.want {
			t.Errorf("%s: priceAlertHit = %v, want %v", c.name, got, c.want)
		}
	}
}

type fakeAlertStore struct {
	active    []store.Alert
	filings   map[string][]store.Filing
	triggered map[string]time.Time
}

func (f *fakeAlertStore) ListActiveAlerts(context.Context) ([]store.Alert, error) {
	return f.active, nil
}
func (f *fakeAlertStore) MarkAlertTriggered(_ context.Context, id string, at time.Time) error {
	if f.triggered == nil {
		f.triggered = map[string]time.Time{}
	}
	f.triggered[id] = at
	return nil
}
func (f *fakeAlertStore) ListFilings(_ context.Context, ticker string, _ int) ([]store.Filing, error) {
	return f.filings[ticker], nil
}

type fakePrices map[string]store.Quote

func (f fakePrices) LatestQuote(_ context.Context, ticker string) (store.Quote, bool, error) {
	q, ok := f[ticker]
	return q, ok, nil
}

type fakeSignals map[string][]indicators.Signal

func (f fakeSignals) SignalsFor(ticker string) []indicators.Signal { return f[ticker] }

func TestSignalAlertHit(t *testing.T) {
	golden := []indicators.Signal{{ID: "technical.ma-cross", Direction: indicators.DirBullish, Label: "Golden cross"}}
	rsiOver := []indicators.Signal{{ID: "technical.rsi", Direction: indicators.DirBearish, Label: "RSI overbought"}}
	cases := []struct {
		kind string
		sigs []indicators.Signal
		want bool
	}{
		{"golden_cross", golden, true},
		{"death_cross", golden, false}, // golden present, not death
		{"signal_bullish", golden, true},
		{"signal_bearish", golden, false},
		{"rsi_overbought", rsiOver, true},
		{"rsi_oversold", rsiOver, false},
		{"signal_bearish", rsiOver, true},
		{"golden_cross", nil, false}, // no signals → no hit (never fabricated)
		{"not_a_signal_kind", golden, false},
	}
	for _, c := range cases {
		if got := signalAlertHit(c.kind, c.sigs); got != c.want {
			t.Errorf("signalAlertHit(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
	if !IsSignalAlertKind("golden_cross") || IsSignalAlertKind("price_above") {
		t.Error("IsSignalAlertKind misclassified a kind")
	}
}

func TestEvaluateTriggers(t *testing.T) {
	created := time.Now().Add(-time.Hour)
	st := &fakeAlertStore{
		active: []store.Alert{
			{ID: "a", Ticker: "AAPL", Kind: "price_above", Threshold: 100, Active: true, CreatedAt: created},  // 105 → hit
			{ID: "b", Ticker: "AAPL", Kind: "price_below", Threshold: 100, Active: true, CreatedAt: created},  // 105 → no
			{ID: "c", Ticker: "MSTR", Kind: "new_filing", Active: true, CreatedAt: created},                   // newer filing → hit
			{ID: "d", Ticker: "NVDA", Kind: "new_filing", Active: true, CreatedAt: time.Now().Add(time.Hour)}, // filing older than created → no
			{ID: "e", Ticker: "GOOG", Kind: "golden_cross", Active: true, CreatedAt: created},                 // GOOG has a golden cross → hit
			{ID: "f", Ticker: "GOOG", Kind: "death_cross", Active: true, CreatedAt: created},                  // GOOG has golden, not death → no
			{ID: "g", Ticker: "TSLA", Kind: "signal_bearish", Active: true, CreatedAt: created},               // TSLA has a bearish signal → hit
		},
		filings: map[string][]store.Filing{
			"MSTR": {{FiledAt: time.Now()}},
			"NVDA": {{FiledAt: time.Now().Add(-2 * time.Hour)}},
		},
	}
	sigs := fakeSignals{
		"GOOG": {{ID: "technical.ma-cross", Direction: indicators.DirBullish, Label: "Golden cross"}},
		"TSLA": {{ID: "technical.rsi", Direction: indicators.DirBearish, Label: "RSI overbought"}},
	}
	ev := NewAlertEvaluator(st, fakePrices{"AAPL": {Price: 105, PrevClose: 100}}, sigs, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	ev.evaluate(context.Background())

	for _, id := range []string{"a", "c", "e", "g"} {
		if _, ok := st.triggered[id]; !ok {
			t.Errorf("alert %q should have triggered", id)
		}
	}
	for _, id := range []string{"b", "d", "f"} {
		if _, ok := st.triggered[id]; ok {
			t.Errorf("alert %q should NOT have triggered", id)
		}
	}
}
