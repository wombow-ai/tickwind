package sentiment

import (
	"sync"
	"testing"
)

// fp / ip are small helpers for taking the address of a literal optional input.
func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func TestCompute(t *testing.T) {
	tests := []struct {
		name          string
		in            Inputs
		wantScore     int
		wantLabel     string
		wantLabelZh   string
		wantAvailable int
	}{
		{
			name: "extreme greed",
			// Low VIX, low put/call, broad advance, mostly new highs, hot buzz,
			// low short interest — every component pegs toward greed.
			in: Inputs{
				VIX:          fp(12),
				PutCallRatio: fp(0.7),
				Advancers:    ip(95),
				Decliners:    ip(5),
				NewHighs:     ip(90),
				NewLows:      ip(10),
				Heat:         fp(95),
				ShortPct:     fp(32), // light short activity (below the ~48% norm) -> 80
			},
			wantScore:     89, // mean of 90,85,95,90,95,80 = 89.17 -> 89
			wantLabel:     "Extreme Greed",
			wantLabelZh:   "极度贪婪",
			wantAvailable: 6,
		},
		{
			name: "extreme fear",
			// High VIX, high put/call, broad decline, mostly new lows, cold buzz,
			// heavy short interest — every component pegs toward fear.
			in: Inputs{
				VIX:          fp(40),
				PutCallRatio: fp(1.2),
				Advancers:    ip(5),
				Decliners:    ip(95),
				NewHighs:     ip(10),
				NewLows:      ip(90),
				Heat:         fp(5),
				ShortPct:     fp(70), // heavily elevated short volume (well above ~48%) -> 9
			},
			wantScore:     9, // mean of 10,15,5,10,5,9 = 9.0 -> 9
			wantLabel:     "Extreme Fear",
			wantLabelZh:   "极度恐惧",
			wantAvailable: 6,
		},
		{
			name: "neutral midpoints",
			// Each component sits at its range midpoint -> ~50.
			in: Inputs{
				VIX:          fp(26),   // mid of [12,40] -> 50
				PutCallRatio: fp(0.95), // mid of [0.7,1.2] -> 50
				Advancers:    ip(50),
				Decliners:    ip(50), // 50
				NewHighs:     ip(50),
				NewLows:      ip(50), // 50
				Heat:         fp(50), // 50
				ShortPct:     fp(48), // the structural baseline ~48% -> 50 (neutral)
			},
			wantScore:     50, // mean 50,50,50,50,50,50 = 50 -> 50
			wantLabel:     "Neutral",
			wantLabelZh:   "中性",
			wantAvailable: 6,
		},
		{
			name: "partial components reweighted",
			// Only VIX (90) and Heat (40) supplied -> mean 65 -> Greed, and the
			// missing components do not drag the average toward 50.
			in: Inputs{
				VIX:  fp(12),
				Heat: fp(40),
			},
			wantScore:     65,
			wantLabel:     "Greed",
			wantLabelZh:   "贪婪",
			wantAvailable: 2,
		},
		{
			name:          "empty inputs",
			in:            Inputs{},
			wantScore:     50,
			wantLabel:     "Neutral",
			wantLabelZh:   "中性",
			wantAvailable: 0,
		},
		{
			name: "breadth skipped on zero denominator",
			// Advancers+Decliners == 0 -> breadth skipped; only Heat remains.
			in: Inputs{
				Advancers: ip(0),
				Decliners: ip(0),
				Heat:      fp(60),
			},
			wantScore:     60,
			wantLabel:     "Greed",
			wantLabelZh:   "贪婪",
			wantAvailable: 1,
		},
		{
			name: "vix clamps below floor",
			// VIX 5 maps above 90 but clamps to 100; sole component -> 100.
			in:            Inputs{VIX: fp(5)},
			wantScore:     100,
			wantLabel:     "Extreme Greed",
			wantLabelZh:   "极度贪婪",
			wantAvailable: 1,
		},
		{
			name: "vix clamps above ceiling",
			// VIX 80 maps below 10 (negative) but clamps to 0; sole component -> 0.
			in:            Inputs{VIX: fp(80)},
			wantScore:     0,
			wantLabel:     "Extreme Fear",
			wantLabelZh:   "极度恐惧",
			wantAvailable: 1,
		},
		{
			name: "put/call clamps low",
			// Ratio 0.3 maps above 85 but clamps to 100.
			in:            Inputs{PutCallRatio: fp(0.3)},
			wantScore:     100,
			wantLabel:     "Extreme Greed",
			wantLabelZh:   "极度贪婪",
			wantAvailable: 1,
		},
		{
			name: "heat clamps above 100",
			// Caller over-normalised Heat to 150 -> clamps to 100.
			in:            Inputs{Heat: fp(150)},
			wantScore:     100,
			wantLabel:     "Extreme Greed",
			wantLabelZh:   "极度贪婪",
			wantAvailable: 1,
		},
		{
			name: "short pct clamps low",
			// An extreme 80% short volume maps below 0 and clamps to 0.
			in:            Inputs{ShortPct: fp(80)},
			wantScore:     0,
			wantLabel:     "Extreme Fear",
			wantLabelZh:   "极度恐惧",
			wantAvailable: 1,
		},
		{
			name: "fear band boundary",
			// Sole Heat 30 -> score 30 -> Fear (>=25, <45).
			in:            Inputs{Heat: fp(30)},
			wantScore:     30,
			wantLabel:     "Fear",
			wantLabelZh:   "恐惧",
			wantAvailable: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compute(tt.in)
			if got.Score != tt.wantScore {
				t.Errorf("Score = %d, want %d", got.Score, tt.wantScore)
			}
			if got.Label != tt.wantLabel {
				t.Errorf("Label = %q, want %q", got.Label, tt.wantLabel)
			}
			if got.LabelZh != tt.wantLabelZh {
				t.Errorf("LabelZh = %q, want %q", got.LabelZh, tt.wantLabelZh)
			}
			if got.Available != tt.wantAvailable {
				t.Errorf("Available = %d, want %d", got.Available, tt.wantAvailable)
			}
			if len(got.Components) != got.Available {
				t.Errorf("len(Components) = %d, want Available %d", len(got.Components), got.Available)
			}
			for _, c := range got.Components {
				if c.Score < 0 || c.Score > 100 {
					t.Errorf("component %q score %d out of [0,100]", c.Name, c.Score)
				}
			}
		})
	}
}

func TestClassifyBoundaries(t *testing.T) {
	tests := []struct {
		score       int
		wantLabel   string
		wantLabelZh string
	}{
		{0, "Extreme Fear", "极度恐惧"},
		{24, "Extreme Fear", "极度恐惧"},
		{25, "Fear", "恐惧"},
		{44, "Fear", "恐惧"},
		{45, "Neutral", "中性"},
		{54, "Neutral", "中性"},
		{55, "Greed", "贪婪"},
		{74, "Greed", "贪婪"},
		{75, "Extreme Greed", "极度贪婪"},
		{100, "Extreme Greed", "极度贪婪"},
	}
	for _, tt := range tests {
		label, labelZh := classify(tt.score)
		if label != tt.wantLabel || labelZh != tt.wantLabelZh {
			t.Errorf("classify(%d) = %q/%q, want %q/%q", tt.score, label, labelZh, tt.wantLabel, tt.wantLabelZh)
		}
	}
}

func TestCache(t *testing.T) {
	c := NewCache()

	if _, ok := c.Latest(); ok {
		t.Fatal("Latest() ok = true before any Set")
	}
	if h := c.History(); h != nil {
		t.Fatalf("History() = %v before any Set, want nil", h)
	}
	if !c.UpdatedAt().IsZero() {
		t.Fatal("UpdatedAt() non-zero before any Set")
	}

	r1 := Compute(Inputs{Heat: fp(80)})
	c.Set(r1, "2026-06-11")
	got, ok := c.Latest()
	if !ok || got.Score != r1.Score {
		t.Fatalf("Latest() = %+v,%v, want %+v,true", got, ok, r1)
	}
	if c.UpdatedAt().IsZero() {
		t.Fatal("UpdatedAt() zero after Set")
	}

	r2 := Compute(Inputs{Heat: fp(20)})
	c.Set(r2, "2026-06-12")
	if h := c.History(); len(h) != 2 || h[0].Date != "2026-06-11" || h[1].Date != "2026-06-12" {
		t.Fatalf("History() = %+v, want two chronological points", h)
	}

	// Same-day re-Set collapses to one point and updates its score.
	r3 := Compute(Inputs{Heat: fp(90)})
	c.Set(r3, "2026-06-12")
	h := c.History()
	if len(h) != 2 {
		t.Fatalf("len(History) = %d after same-day re-Set, want 2", len(h))
	}
	if h[1].Score != r3.Score {
		t.Errorf("History[1].Score = %d after re-Set, want %d", h[1].Score, r3.Score)
	}

	// Empty date updates latest without touching history.
	r4 := Compute(Inputs{Heat: fp(10)})
	c.Set(r4, "")
	if got, _ := c.Latest(); got.Score != r4.Score {
		t.Errorf("Latest().Score = %d after empty-date Set, want %d", got.Score, r4.Score)
	}
	if len(c.History()) != 2 {
		t.Errorf("len(History) = %d after empty-date Set, want 2", len(c.History()))
	}

	// Returned history is a copy: mutating it must not affect the cache.
	h2 := c.History()
	h2[0].Score = -999
	if c.History()[0].Score == -999 {
		t.Error("History() returned an aliased slice; mutation leaked into cache")
	}
}

func TestCacheSeed(t *testing.T) {
	c := NewCache()

	pts := []Point{
		{Date: "2026-06-10", Score: 20},
		{Date: "2026-06-11", Score: 30},
		{Date: "2026-06-12", Score: 40},
	}
	c.Seed(pts)

	got := c.History()
	if len(got) != 3 {
		t.Fatalf("History() len = %d after Seed, want 3", len(got))
	}
	for i := range pts {
		if got[i] != pts[i] {
			t.Fatalf("History()[%d] = %+v, want %+v", i, got[i], pts[i])
		}
	}

	// Seed copies its input: mutating the source slice must not affect the cache.
	pts[0].Score = -999
	if c.History()[0].Score == -999 {
		t.Error("Seed aliased its input; mutation leaked into the cache")
	}

	// A Set after Seed appends a new day to the seeded history.
	c.Set(Compute(Inputs{Heat: fp(50)}), "2026-06-13")
	if h := c.History(); len(h) != 4 || h[3].Date != "2026-06-13" {
		t.Fatalf("History() = %+v after post-Seed Set, want a 4th point dated 2026-06-13", h)
	}

	// Seeding again replaces the history; an empty seed clears it.
	c.Seed(nil)
	if h := c.History(); h != nil {
		t.Fatalf("History() = %+v after Seed(nil), want nil", h)
	}
}

func TestCacheConcurrent(t *testing.T) {
	c := NewCache()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Set(Compute(Inputs{Heat: fp(float64(i % 100))}), "2026-06-13")
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Latest()
			c.History()
			c.UpdatedAt()
		}()
	}
	wg.Wait()
	if _, ok := c.Latest(); !ok {
		t.Fatal("Latest() ok = false after concurrent writes")
	}
}
