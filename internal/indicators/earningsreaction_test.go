package indicators

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/store"
)

func mustDate(s string) time.Time { t, _ := time.Parse(dateOnly, s); return t }

func TestComputeEarningsReaction(t *testing.T) {
	// Daily candles 2024-01-01 .. ~2024-06-29 (180 bars), default close 100 (no gaps → every
	// event's ~2-session window is 2 days, well under earningsWindowMax).
	c := rsDaily("2024-01-01", 180, 100)
	// Five quarterly-ish events; set the day-after close to control each reaction (before=100).
	rsSet(c, "2024-02-02", 110) // 2024-02-01 → +10%
	rsSet(c, "2024-03-02", 90)  // 2024-03-01 → -10%
	rsSet(c, "2024-04-02", 105) // 2024-04-01 → +5%
	rsSet(c, "2024-05-02", 100) // 2024-05-01 →  0%
	rsSet(c, "2024-06-02", 102) // 2024-06-01 → +2%

	dates := []time.Time{ // newest-first, like the real source
		mustDate("2024-06-01"), mustDate("2024-05-01"), mustDate("2024-04-01"),
		mustDate("2024-03-01"), mustDate("2024-02-01"),
	}
	er, ok := ComputeEarningsReaction(dates, c)
	if !ok {
		t.Fatal("ok=false, want true")
	}
	if er.Samples != 5 {
		t.Fatalf("samples=%d, want 5", er.Samples)
	}
	// sum=+7 → avg 1.4; |.|=27 → absavg 5.4; positive {+10,+5,+2}=3/5 → up 0.6.
	if er.AvgMove != 1.4 || er.AvgAbsMove != 5.4 || er.UpRate != 0.6 {
		t.Fatalf("aggregates = avg %v / absavg %v / up %v, want 1.4 / 5.4 / 0.6", er.AvgMove, er.AvgAbsMove, er.UpRate)
	}
	if len(er.Events) != 5 || er.Events[0].Date != "2024-06-01" || er.Events[0].Move != 2 {
		t.Fatalf("events[0] = %+v, want 2024-06-01 / +2 (newest first)", er.Events)
	}
}

func TestComputeEarningsReaction_Insufficient(t *testing.T) {
	c := rsDaily("2024-01-01", 180, 100)
	threeDates := []time.Time{mustDate("2024-04-01"), mustDate("2024-03-01"), mustDate("2024-02-01")}
	tests := []struct {
		name    string
		dates   []time.Time
		candles []store.Candle
	}{
		{"no earnings dates", nil, c},
		{"fewer than the sample floor", threeDates, c}, // 3 bracketed < minEarningsSamples(4)
		{"all dates out of candle range", []time.Time{mustDate("2030-01-01"), mustDate("2010-01-01")}, c},
		{"too few candles", []time.Time{mustDate("2024-02-01")}, rsDaily("2024-01-01", 1, 100)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := ComputeEarningsReaction(tc.dates, tc.candles); ok {
				t.Fatal("ok=true, want false")
			}
		})
	}
}

func TestComputeEarningsReaction_SkipsGapStretchedWindow(t *testing.T) {
	// Four clean monthly events + one whose surrounding candles are missing for ~2 weeks (a halt):
	// that event's window exceeds earningsWindowMax and must be skipped, leaving 4 (still >= floor).
	c := rsDaily("2024-01-01", 180, 100)
	for _, d := range []string{"2024-02-02", "2024-03-02", "2024-04-02", "2024-05-02"} {
		rsSet(c, d, 105)
	}
	// Drop candles around 2024-06-01 so its bracket spans > 8 days.
	c = dropRange(c, "2024-05-26", "2024-06-08")
	dates := []time.Time{
		mustDate("2024-06-01"), // halted window → skipped
		mustDate("2024-05-01"), mustDate("2024-04-01"), mustDate("2024-03-01"), mustDate("2024-02-01"),
	}
	er, ok := ComputeEarningsReaction(dates, c)
	if !ok || er.Samples != 4 {
		t.Fatalf("samples=%d ok=%v, want 4/true (gap-stretched event skipped)", er.Samples, ok)
	}
}

// dropRange removes candles whose date is within [from, to] inclusive.
func dropRange(c []store.Candle, from, to string) []store.Candle {
	out := c[:0:0]
	for _, x := range c {
		d := x.Time.Format(dateOnly)
		if d >= from && d <= to {
			continue
		}
		out = append(out, x)
	}
	return out
}

func TestCandleStrictlyBracket(t *testing.T) {
	c := rsDaily("2024-01-01", 10, 100) // 2024-01-01..01-10
	rsSet(c, "2024-01-06", 120)
	d := mustDate("2024-01-05") // a date that IS a trading day — must be EXCLUDED from both sides
	before, okB := candleStrictlyBefore(c, d)
	after, okA := candleStrictlyAfter(c, d)
	if !okB || !okA || before.Close != 100 || after.Close != 120 {
		t.Fatalf("bracket: before %v(%v) after %v(%v), want 100/true 120/true", before.Close, okB, after.Close, okA)
	}
	if before.Time.Format(dateOnly) != "2024-01-04" || after.Time.Format(dateOnly) != "2024-01-06" {
		t.Fatalf("bracket dates: before %s after %s, want 2024-01-04 / 2024-01-06", before.Time.Format(dateOnly), after.Time.Format(dateOnly))
	}
}

func TestEarningsReactionSummary(t *testing.T) {
	er := EarningsReaction{
		Events:     []EarningsEvent{{}, {}},
		AvgMove:    -0.5,
		AvgAbsMove: 4.2,
		UpRate:     0.55,
		Samples:    9,
	}
	s := er.Summary()
	if s.AvgAbsMove != 4.2 || s.UpRate != 0.55 || s.Samples != 9 {
		t.Fatalf("summary = %+v, want {4.2, 0.55, 9}", s)
	}
}

func TestRankEarningsReaction(t *testing.T) {
	pop := []TickerReaction{
		{Ticker: "AAPL", ReactionSummary: ReactionSummary{AvgAbsMove: 8, UpRate: 0.5, Samples: 10}},
		{Ticker: "TSLA", ReactionSummary: ReactionSummary{AvgAbsMove: 12, UpRate: 0.7, Samples: 8}},
		{Ticker: "NVDA", ReactionSummary: ReactionSummary{AvgAbsMove: 6, UpRate: 0.9, Samples: 6}},
		{Ticker: "KO", ReactionSummary: ReactionSummary{AvgAbsMove: 3, UpRate: 0.9, Samples: 12}},  // ties NVDA on up-rate
		{Ticker: "LOWS", ReactionSummary: ReactionSummary{AvgAbsMove: 2, UpRate: 0.4, Samples: 3}}, // below floor → dropped
		{Ticker: "", ReactionSummary: ReactionSummary{AvgAbsMove: 99, UpRate: 1, Samples: 99}},     // empty ticker → dropped
	}

	order := func(rs []ReactionRank) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.Ticker
		}
		return out
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	if got := order(RankEarningsReaction(pop, ReactionViewMostVolatile)); !eq(got, []string{"TSLA", "AAPL", "NVDA", "KO"}) {
		t.Fatalf("most-volatile = %v, want [TSLA AAPL NVDA KO] (LOWS+empty dropped)", got)
	}
	// up-rate desc; KO & NVDA both 0.9 → more samples (KO 12) first.
	if got := order(RankEarningsReaction(pop, ReactionViewHighestUpRate)); !eq(got, []string{"KO", "NVDA", "TSLA", "AAPL"}) {
		t.Fatalf("highest-up-rate = %v, want [KO NVDA TSLA AAPL]", got)
	}
	if got := RankEarningsReaction(pop, "bogus"); got != nil {
		t.Fatalf("unknown view = %v, want nil", got)
	}
	if !ValidReactionView("most-volatile") || !ValidReactionView("highest-up-rate") || ValidReactionView("calmest") {
		t.Fatal("ValidReactionView mismatch")
	}
}
