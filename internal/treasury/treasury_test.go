package treasury

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseLatest(t *testing.T) {
	tests := []struct {
		name        string
		csv         string
		wantErr     bool
		wantDate    string
		wantYields  map[string]float64 // tenor → expected rate (exact match)
		absent      []string           // tenors that must NOT appear
		wantSpread  float64
		hasSpread   bool
		wantInverte bool
	}{
		{
			name: "newest row wins, full curve, positive spread",
			csv: `Date,"1 Mo","1.5 Month","2 Mo","3 Mo","4 Mo","6 Mo","1 Yr","2 Yr","3 Yr","5 Yr","7 Yr","10 Yr","20 Yr","30 Yr"
06/12/2026,3.69,3.70,3.70,3.78,3.79,3.82,3.86,4.09,4.12,4.21,4.34,4.48,4.98,4.97
06/11/2026,3.69,3.69,3.70,3.78,3.79,3.81,3.85,4.05,4.09,4.18,4.31,4.45,4.96,4.95
`,
			wantDate:   "2026-06-12",
			wantYields: map[string]float64{"3M": 3.78, "2Y": 4.09, "10Y": 4.48, "30Y": 4.97, "1.5M": 3.70},
			wantSpread: 0.39, // 4.48 - 4.09
			hasSpread:  true,
		},
		{
			name: "inverted curve → negative spread, inverted=true",
			csv: `Date,"2 Yr","10 Yr"
07/03/2023,4.94,3.86
`,
			wantDate:    "2023-07-03",
			wantYields:  map[string]float64{"2Y": 4.94, "10Y": 3.86},
			wantSpread:  -1.08,
			hasSpread:   true,
			wantInverte: true,
		},
		{
			name: "blank tenor cell → that tenor absent, never zero-filled",
			csv: `Date,"1 Mo","2 Mo","3 Mo","4 Mo","6 Mo","1 Yr","2 Yr","3 Yr","5 Yr","7 Yr","10 Yr","20 Yr","30 Yr"
01/04/2022,0.06,0.05,0.08,,0.22,0.38,0.77,1.02,1.37,1.57,1.66,2.10,2.07
`,
			wantDate:   "2022-01-04",
			wantYields: map[string]float64{"3M": 0.08, "2Y": 0.77, "10Y": 1.66},
			absent:     []string{"4M", "1.5M"}, // 4 Mo blank, 1.5 Month not even a column
			wantSpread: 0.89,                   // 1.66 - 0.77
			hasSpread:  true,
		},
		{
			name: "varying column set (older year, no 1.5 Month) parses by name",
			csv: `Date,"1 Mo","2 Mo","3 Mo","6 Mo","1 Yr","2 Yr","3 Yr","5 Yr","7 Yr","10 Yr","20 Yr","30 Yr"
06/12/2019,2.35,2.36,2.31,2.16,1.95,1.84,1.81,1.85,1.97,2.12,2.43,2.59
`,
			wantDate:   "2019-06-12",
			wantYields: map[string]float64{"2Y": 1.84, "10Y": 2.12},
			absent:     []string{"4M", "1.5M"},
			wantSpread: 0.28,
			hasSpread:  true,
		},
		{
			name: "10Y present but 2Y missing → no spread (not faked from 0)",
			csv: `Date,"3 Mo","10 Yr","30 Yr"
06/12/2026,3.78,4.48,4.97
`,
			wantDate:   "2026-06-12",
			wantYields: map[string]float64{"3M": 3.78, "10Y": 4.48, "30Y": 4.97},
			absent:     []string{"2Y"},
			hasSpread:  false,
		},
		{
			name:    "header only → ErrNoData",
			csv:     "Date,\"2 Yr\",\"10 Yr\"\n",
			wantErr: true,
		},
		{
			name: "skips a leading garbage/blank-rate row to the first real one",
			csv: `Date,"2 Yr","10 Yr"
N/A,,
06/10/2026,4.05,4.45
`,
			wantDate:   "2026-06-10",
			wantYields: map[string]float64{"2Y": 4.05, "10Y": 4.45},
			wantSpread: 0.40,
			hasSpread:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLatest([]byte(tc.csv))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseLatest() = %+v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLatest() error = %v", err)
			}
			if got.Date != tc.wantDate {
				t.Errorf("Date = %q, want %q", got.Date, tc.wantDate)
			}
			for tenor, want := range tc.wantYields {
				r, ok := got.Rate(tenor)
				if !ok {
					t.Errorf("tenor %s missing, want %v", tenor, want)
					continue
				}
				if r != want {
					t.Errorf("tenor %s = %v, want %v", tenor, r, want)
				}
			}
			for _, tenor := range tc.absent {
				if r, ok := got.Rate(tenor); ok {
					t.Errorf("tenor %s present (=%v) but must be absent (never fabricated)", tenor, r)
				}
			}
			if got.HasSpread != tc.hasSpread {
				t.Errorf("HasSpread = %v, want %v", got.HasSpread, tc.hasSpread)
			}
			if tc.hasSpread {
				if got.Spread2s10s != tc.wantSpread {
					t.Errorf("Spread2s10s = %v, want %v", got.Spread2s10s, tc.wantSpread)
				}
				if got.Inverted != tc.wantInverte {
					t.Errorf("Inverted = %v, want %v", got.Inverted, tc.wantInverte)
				}
			} else if got.Spread2s10s != 0 {
				t.Errorf("Spread2s10s = %v with no spread, want 0", got.Spread2s10s)
			}
		})
	}
}

func TestYieldsAreSortedShortToLong(t *testing.T) {
	csv := `Date,"30 Yr","2 Yr","3 Mo","10 Yr"
06/12/2026,4.97,4.09,3.78,4.48
`
	got, err := ParseLatest([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"3M", "2Y", "10Y", "30Y"}
	if len(got.Yields) != len(want) {
		t.Fatalf("got %d yields, want %d (%+v)", len(got.Yields), len(want), got.Yields)
	}
	for i, w := range want {
		if got.Yields[i].Tenor != w {
			t.Errorf("Yields[%d].Tenor = %q, want %q (canonical short→long order)", i, got.Yields[i].Tenor, w)
		}
	}
}

func TestClientLatestOverHTTP(t *testing.T) {
	body := `Date,"1 Mo","2 Mo","3 Mo","6 Mo","1 Yr","2 Yr","3 Yr","5 Yr","7 Yr","10 Yr","20 Yr","30 Yr"
06/12/2026,3.69,3.70,3.78,3.82,3.86,4.09,4.12,4.21,4.34,4.48,4.98,4.97
06/11/2026,3.69,3.70,3.78,3.81,3.85,4.05,4.09,4.18,4.31,4.45,4.96,4.95
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A bare client must send a non-empty UA (we set a browser one).
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "no UA", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New()
	c.base = srv.URL
	got, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Date != "2026-06-12" {
		t.Errorf("Date = %q, want 2026-06-12 (newest row)", got.Date)
	}
	if !got.HasSpread || got.Spread2s10s != 0.39 {
		t.Errorf("spread = %v (has=%v), want 0.39", got.Spread2s10s, got.HasSpread)
	}
	if got.Inverted {
		t.Errorf("Inverted = true, want false for a +0.39 spread")
	}
}

func TestCache(t *testing.T) {
	c := NewCache()
	if _, ok := c.Latest(); ok {
		t.Fatal("fresh cache Latest() ok=true, want false")
	}
	if !c.UpdatedAt().IsZero() {
		t.Fatal("fresh cache UpdatedAt not zero")
	}
	curve := Curve{Date: "2026-06-12", Yields: []Yield{{"2Y", 4.09}, {"10Y", 4.48}}, Spread2s10s: 0.39, HasSpread: true}
	c.Set(curve)
	got, ok := c.Latest()
	if !ok || got.Date != "2026-06-12" || got.Spread2s10s != 0.39 {
		t.Fatalf("Latest() = %+v ok=%v, want the set curve", got, ok)
	}
	if c.UpdatedAt().IsZero() {
		t.Fatal("UpdatedAt zero after Set")
	}
}
