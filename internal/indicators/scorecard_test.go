package indicators

import (
	"math"
	"testing"
)

func fptr2(v float64) *float64 { return &v }

func TestExtractFactorMetrics(t *testing.T) {
	res := StockIndicatorsResult{
		Ticker: "AAPL",
		Indicators: []StockIndicator{
			{Indicator: Indicator{ID: "fundamental.pe-ttm"}, Status: StatusOK, Value: fptr2(30)},
			{Indicator: Indicator{ID: "fundamental.roe"}, Status: StatusOK, Value: fptr2(0.45)},
			{Indicator: Indicator{ID: "fundamental.ps"}, Status: StatusInsufficient}, // not OK → NaN
			{Indicator: Indicator{ID: "fundamental.tsr"}, Status: StatusOK, Value: fptr2(40)},
		},
	}
	m := ExtractFactorMetrics(res)
	if m.PE != 30 || m.ROE != 0.45 || m.TSR != 40 {
		t.Fatalf("extracted = %+v, want PE 30 / ROE 0.45 / TSR 40", m)
	}
	if !math.IsNaN(m.PS) || !math.IsNaN(m.PB) {
		t.Fatalf("unavailable metrics must be NaN: PS=%v PB=%v", m.PS, m.PB)
	}
}

func TestPercentile(t *testing.T) {
	pop := []float64{10, 20, 30, 40, 50, 60, 70, 80} // 8 values (== minScorecardPopulation)
	// 30 → values <= 30 are {10,20,30} = 3/8 = 37.5%
	if p, ok := percentile(30, pop); !ok || p != 37.5 {
		t.Fatalf("percentile(30) = %v/%v, want 37.5/true", p, ok)
	}
	// NaN target → not computable
	if _, ok := percentile(math.NaN(), pop); ok {
		t.Fatal("NaN target should be uncomputable")
	}
	// too-small population (< 8) → withheld
	if _, ok := percentile(30, []float64{10, 20, 30}); ok {
		t.Fatal("population < minScorecardPopulation should be withheld")
	}
	// NaNs in the population are ignored (8 reals + 2 NaN → still ranks vs the 8 reals)
	withNaN := append([]float64{math.NaN(), math.NaN()}, pop...)
	if p, ok := percentile(80, withNaN); !ok || p != 100 {
		t.Fatalf("percentile(80) ignoring NaN = %v/%v, want 100/true", p, ok)
	}
}

func TestComputeScorecard(t *testing.T) {
	// A population of 8 names with rising quality (ROE) + falling value (P/E).
	pop := make([]FactorMetrics, 8)
	for i := range pop {
		pop[i] = FactorMetrics{
			PE:  float64(10 + i*5),   // 10..45
			ROE: float64(i+1) * 0.05, // 0.05..0.40
			PB:  math.NaN(), PS: math.NaN(), RevGrowth: math.NaN(), EarnGrowth: math.NaN(),
			ROIC: math.NaN(), EBITMargin: math.NaN(), Piotroski: math.NaN(), TSR: math.NaN(),
		}
	}
	// Target: a CHEAP (low P/E) + HIGH-quality (high ROE) name.
	target := FactorMetrics{
		PE: 10, ROE: 0.40,
		PB: math.NaN(), PS: math.NaN(), RevGrowth: math.NaN(), EarnGrowth: math.NaN(),
		ROIC: math.NaN(), EBITMargin: math.NaN(), Piotroski: math.NaN(), TSR: math.NaN(),
	}
	sc := ComputeScorecard(target, pop)
	if sc.Population != 8 {
		t.Fatalf("population = %d, want 8", sc.Population)
	}
	// Value: P/E 10 is the lowest → percentile(10)=12.5 → inverted 87.5 (cheapest = high value pct).
	if sc.Value == nil || sc.Value.Percentile != 87.5 || sc.Value.Inputs != 1 {
		t.Fatalf("value = %+v, want pct 87.5 / inputs 1", sc.Value)
	}
	// Quality: ROE 0.40 is the highest → percentile = 100.
	if sc.Quality == nil || sc.Quality.Percentile != 100 {
		t.Fatalf("quality = %+v, want pct 100", sc.Quality)
	}
	// Growth + Momentum: no sub-metrics available → omitted (insufficient-not-wrong).
	if sc.Growth != nil || sc.Momentum != nil {
		t.Fatalf("growth/momentum should be nil (no inputs): %+v / %+v", sc.Growth, sc.Momentum)
	}
	if !sc.HasAny() {
		t.Fatal("HasAny should be true")
	}
}

func TestRankFactor(t *testing.T) {
	nan := math.NaN()
	// 10 names: P/E rises (T00 cheapest), ROE rises (T09 best quality); other metrics unavailable.
	pop := make([]TickerFactorMetrics, 10)
	for i := range pop {
		pop[i] = TickerFactorMetrics{
			Ticker: fmtTicker(i),
			Metrics: FactorMetrics{
				PE: float64(10 + i*5), ROE: float64(i+1) * 0.05,
				PB: nan, PS: nan, RevGrowth: nan, EarnGrowth: nan,
				ROIC: nan, EBITMargin: nan, Piotroski: nan, TSR: nan,
			},
		}
	}

	t.Run("unknown factor → empty", func(t *testing.T) {
		if got := RankFactor(pop, "bogus"); len(got) != 0 {
			t.Fatalf("unknown factor → %d results, want 0", len(got))
		}
	})

	t.Run("growth (no sub-metric) → empty", func(t *testing.T) {
		if got := RankFactor(pop, "growth"); len(got) != 0 {
			t.Fatalf("growth has no inputs → %d results, want 0", len(got))
		}
	})

	t.Run("too-small population → empty", func(t *testing.T) {
		if got := RankFactor(pop[:4], "value"); len(got) != 0 {
			t.Fatalf("pop < min → %d results, want 0 (percentile withheld)", len(got))
		}
	})

	t.Run("value: cheapest first, sorted desc", func(t *testing.T) {
		got := RankFactor(pop, "value")
		if len(got) != 10 {
			t.Fatalf("got %d, want 10", len(got))
		}
		if got[0].Ticker != "T00" {
			t.Fatalf("rank 1 = %s, want T00 (cheapest P/E)", got[0].Ticker)
		}
		for i := 1; i < len(got); i++ {
			if got[i].Percentile > got[i-1].Percentile {
				t.Fatalf("not sorted desc at %d: %.1f > %.1f", i, got[i].Percentile, got[i-1].Percentile)
			}
		}
	})

	t.Run("matches the per-stock scorecard exactly", func(t *testing.T) {
		// The leaderboard percentile for a member must equal what ComputeScorecard gives that member
		// against the same population — the two surfaces can never disagree.
		metrics := make([]FactorMetrics, len(pop))
		for i, m := range pop {
			metrics[i] = m.Metrics
		}
		ranked := RankFactor(pop, "quality")
		byTicker := map[string]float64{}
		for _, r := range ranked {
			byTicker[r.Ticker] = r.Percentile
		}
		for _, m := range pop {
			sc := ComputeScorecard(m.Metrics, metrics)
			if sc.Quality == nil {
				t.Fatalf("%s quality nil unexpectedly", m.Ticker)
			}
			if byTicker[m.Ticker] != sc.Quality.Percentile {
				t.Fatalf("%s: leaderboard %.2f != scorecard %.2f", m.Ticker, byTicker[m.Ticker], sc.Quality.Percentile)
			}
		}
	})
}

// fmtTicker formats a synthetic 2-digit ticker (T00..T09) for the rank-factor fixtures.
func fmtTicker(i int) string {
	const digits = "0123456789"
	return "T" + string(digits[i/10]) + string(digits[i%10])
}

func TestComputeScorecard_AllInsufficient(t *testing.T) {
	nan := math.NaN()
	empty := FactorMetrics{PE: nan, PB: nan, PS: nan, RevGrowth: nan, EarnGrowth: nan, ROE: nan, ROIC: nan, EBITMargin: nan, Piotroski: nan, TSR: nan}
	pop := make([]FactorMetrics, 10)
	for i := range pop {
		pop[i] = empty
	}
	sc := ComputeScorecard(empty, pop)
	if sc.HasAny() {
		t.Fatalf("all-insufficient → no factors, got %+v", sc)
	}
}
