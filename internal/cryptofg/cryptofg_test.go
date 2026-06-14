package cryptofg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseFNG(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantErr   bool
		wantScore int
		wantLabel string
		wantAsOf  string
	}{
		{
			name:      "greed score parses, timestamp → date",
			body:      `{"name":"Fear and Greed Index","data":[{"value":"63","value_classification":"Greed","timestamp":"1781395200"}],"metadata":{"error":null}}`,
			wantScore: 63,
			wantLabel: "Greed",
			wantAsOf:  "2026-06-14",
		},
		{
			name:      "extreme fear",
			body:      `{"data":[{"value":"18","value_classification":"Extreme Fear","timestamp":"1781395200"}]}`,
			wantScore: 18,
			wantLabel: "Extreme Fear",
			wantAsOf:  "2026-06-14",
		},
		{
			name:      "value clamped to 0..100",
			body:      `{"data":[{"value":"140","value_classification":"Extreme Greed","timestamp":""}]}`,
			wantScore: 100,
			wantLabel: "Extreme Greed",
			wantAsOf:  "", // no timestamp → no date, never fabricated
		},
		{
			name:    "empty data array → ErrNoData",
			body:    `{"data":[],"metadata":{"error":null}}`,
			wantErr: true,
		},
		{
			name:    "non-numeric value → ErrNoData (never a stand-in score)",
			body:    `{"data":[{"value":"n/a","value_classification":"Unknown","timestamp":"1781395200"}]}`,
			wantErr: true,
		},
		{
			name:    "garbage body → error",
			body:    `not json`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFNG([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseFNG() = %+v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFNG() error = %v", err)
			}
			if got.Score != tc.wantScore {
				t.Errorf("Score = %d, want %d", got.Score, tc.wantScore)
			}
			if got.Label != tc.wantLabel {
				t.Errorf("Label = %q, want %q", got.Label, tc.wantLabel)
			}
			if got.AsOf != tc.wantAsOf {
				t.Errorf("AsOf = %q, want %q", got.AsOf, tc.wantAsOf)
			}
		})
	}
}

func TestParsePrices(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantErr    bool
		btcPresent bool
		btcUSD     float64
		btcChg     float64
		ethPresent bool
		ethUSD     float64
	}{
		{
			name:       "both prices present with 24h change",
			body:       `{"bitcoin":{"usd":64413,"usd_24h_change":1.012},"ethereum":{"usd":1675.75,"usd_24h_change":0.067}}`,
			btcPresent: true, btcUSD: 64413, btcChg: 1.012,
			ethPresent: true, ethUSD: 1675.75,
		},
		{
			name:       "missing ethereum object → ETH absent, not zero-filled",
			body:       `{"bitcoin":{"usd":64413,"usd_24h_change":1.012}}`,
			btcPresent: true, btcUSD: 64413,
			ethPresent: false,
		},
		{
			name:       "zero / non-positive USD → absent (never a real 0)",
			body:       `{"bitcoin":{"usd":0,"usd_24h_change":1.0},"ethereum":{"usd":-5,"usd_24h_change":0}}`,
			btcPresent: false,
			ethPresent: false,
		},
		{
			name:       "empty object → both absent",
			body:       `{}`,
			btcPresent: false,
			ethPresent: false,
		},
		{
			name:    "garbage body → error",
			body:    `nope`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			btc, eth, err := ParsePrices([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParsePrices() = %+v/%+v, want error", btc, eth)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePrices() error = %v", err)
			}
			if btc.Present != tc.btcPresent {
				t.Errorf("BTC.Present = %v, want %v", btc.Present, tc.btcPresent)
			}
			if tc.btcPresent {
				if btc.USD != tc.btcUSD {
					t.Errorf("BTC.USD = %v, want %v", btc.USD, tc.btcUSD)
				}
				if tc.btcChg != 0 && btc.Change24h != tc.btcChg {
					t.Errorf("BTC.Change24h = %v, want %v", btc.Change24h, tc.btcChg)
				}
			} else if btc.USD != 0 {
				t.Errorf("BTC.USD = %v with Present=false, want 0 (never fabricated)", btc.USD)
			}
			if eth.Present != tc.ethPresent {
				t.Errorf("ETH.Present = %v, want %v", eth.Present, tc.ethPresent)
			}
			if tc.ethPresent && eth.USD != tc.ethUSD {
				t.Errorf("ETH.USD = %v, want %v", eth.USD, tc.ethUSD)
			}
		})
	}
}

// TestLatestOverHTTP exercises the full client: F&G is required, prices are
// best-effort and the score still serves when the price host fails.
func TestLatestOverHTTP(t *testing.T) {
	fng := `{"data":[{"value":"63","value_classification":"Greed","timestamp":"1781395200"}]}`
	price := `{"bitcoin":{"usd":64413,"usd_24h_change":1.012},"ethereum":{"usd":1675.75,"usd_24h_change":0.067}}`

	t.Run("both ok", func(t *testing.T) {
		fngSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(fng))
		}))
		defer fngSrv.Close()
		priceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(price))
		}))
		defer priceSrv.Close()

		c := New()
		c.fngURL, c.priceURL = fngSrv.URL, priceSrv.URL
		got, err := c.Latest(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got.Score != 63 || got.Label != "Greed" {
			t.Errorf("got score=%d label=%q, want 63/Greed", got.Score, got.Label)
		}
		if !got.BTC.Present || got.BTC.USD != 64413 {
			t.Errorf("BTC = %+v, want present 64413", got.BTC)
		}
		if !got.ETH.Present || got.ETH.USD != 1675.75 {
			t.Errorf("ETH = %+v, want present 1675.75", got.ETH)
		}
	})

	t.Run("price host down → score still serves, prices absent", func(t *testing.T) {
		fngSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(fng))
		}))
		defer fngSrv.Close()
		priceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
		}))
		defer priceSrv.Close()

		c := New()
		c.fngURL, c.priceURL = fngSrv.URL, priceSrv.URL
		got, err := c.Latest(context.Background())
		if err != nil {
			t.Fatalf("Latest should not error when only prices fail: %v", err)
		}
		if got.Score != 63 {
			t.Errorf("Score = %d, want 63 (F&G still serves)", got.Score)
		}
		if got.BTC.Present || got.ETH.Present {
			t.Errorf("prices should be absent when the price host fails, got BTC=%+v ETH=%+v", got.BTC, got.ETH)
		}
	})

	t.Run("F&G host down → error", func(t *testing.T) {
		fngSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "down", http.StatusServiceUnavailable)
		}))
		defer fngSrv.Close()

		c := New()
		c.fngURL = fngSrv.URL
		if _, err := c.Latest(context.Background()); err == nil {
			t.Fatal("Latest should error when the core F&G signal is unavailable")
		}
	})
}

func TestCache(t *testing.T) {
	c := NewCache()
	if _, ok := c.Latest(); ok {
		t.Fatal("fresh cache Latest() ok=true, want false")
	}
	if !c.UpdatedAt().IsZero() {
		t.Fatal("fresh cache UpdatedAt not zero")
	}
	idx := Index{Score: 63, Label: "Greed", AsOf: "2026-06-14", BTC: Price{USD: 64413, Present: true}}
	c.Set(idx)
	got, ok := c.Latest()
	if !ok || got.Score != 63 || !got.BTC.Present {
		t.Fatalf("Latest() = %+v ok=%v, want the set index", got, ok)
	}
	if c.UpdatedAt().IsZero() {
		t.Fatal("UpdatedAt zero after Set")
	}
}
