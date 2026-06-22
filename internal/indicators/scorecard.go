package indicators

import "math"

// minScorecardPopulation is the floor below which a factor percentile is withheld — ranking a stock
// against a handful of peers is noise, not a statistic.
const minScorecardPopulation = 8

// FactorMetrics holds one stock's raw inputs for the four factor scores, pulled from its computed
// indicators. A NaN field = the metric was unavailable (insufficient) — it is skipped in BOTH the
// population distribution and the stock's own factor (never imputed as 0 or a median).
type FactorMetrics struct {
	// Value (LOWER raw = cheaper = higher value percentile → inverted when ranking).
	PE, PB, PS float64
	// Growth (higher = faster).
	RevGrowth, EarnGrowth float64
	// Quality (higher = better).
	ROE, ROIC, EBITMargin, Piotroski float64
	// Momentum (higher = stronger).
	TSR float64
}

// scorecardID maps each FactorMetrics field to the catalog indicator id it is read from.
var scorecardID = struct {
	PE, PB, PS, RevGrowth, EarnGrowth, ROE, ROIC, EBITMargin, Piotroski, TSR string
}{
	PE:         "fundamental.pe-ttm",
	PB:         "fundamental.pb",
	PS:         "fundamental.ps",
	RevGrowth:  "fundamental.revenue-growth-yoy",
	EarnGrowth: "fundamental.earnings-growth-yoy",
	ROE:        "fundamental.roe",
	ROIC:       "fundamental.roic",
	EBITMargin: "fundamental.ebit-margin",
	Piotroski:  "fundamental.piotroski-f-score",
	TSR:        "fundamental.tsr",
}

// ExtractFactorMetrics reads the factor sub-metrics out of a computed indicators result. Only
// StatusOK indicators with a non-nil value contribute; everything else stays NaN (unavailable).
func ExtractFactorMetrics(res StockIndicatorsResult) FactorMetrics {
	byID := make(map[string]float64, len(res.Indicators))
	for _, si := range res.Indicators {
		if si.Status == StatusOK && si.Value != nil {
			byID[si.ID] = *si.Value
		}
	}
	get := func(id string) float64 {
		if v, ok := byID[id]; ok {
			return v
		}
		return math.NaN()
	}
	return FactorMetrics{
		PE:         get(scorecardID.PE),
		PB:         get(scorecardID.PB),
		PS:         get(scorecardID.PS),
		RevGrowth:  get(scorecardID.RevGrowth),
		EarnGrowth: get(scorecardID.EarnGrowth),
		ROE:        get(scorecardID.ROE),
		ROIC:       get(scorecardID.ROIC),
		EBITMargin: get(scorecardID.EBITMargin),
		Piotroski:  get(scorecardID.Piotroski),
		TSR:        get(scorecardID.TSR),
	}
}

// FactorScore is one factor's standing: a 0..100 percentile (mean of its available sub-metric
// percentiles vs the population) and how many sub-metrics contributed. It is a DESCRIPTIVE
// percentile, never a rating/recommendation.
type FactorScore struct {
	Percentile float64 `json:"percentile"`
	Inputs     int     `json:"inputs"`
}

// Scorecard is a stock's four factor percentiles vs a population. Each factor is INDEPENDENT and
// descriptive — there is deliberately NO blended composite "score" (that would read as a rating,
// which violates the no-advice rule). A factor with no available sub-metric (or too small a
// population) is nil (omitted), never 0/50.
type Scorecard struct {
	Value      *FactorScore `json:"value,omitempty"`
	Growth     *FactorScore `json:"growth,omitempty"`
	Quality    *FactorScore `json:"quality,omitempty"`
	Momentum   *FactorScore `json:"momentum,omitempty"`
	Population int          `json:"population"`
}

// HasAny reports whether at least one factor was computable.
func (s Scorecard) HasAny() bool {
	return s.Value != nil || s.Growth != nil || s.Quality != nil || s.Momentum != nil
}

// percentile returns the fraction (0..100) of the population whose value is <= v, ignoring NaNs,
// and whether the population was large enough (>= minScorecardPopulation) to be meaningful.
func percentile(v float64, pop []float64) (float64, bool) {
	if math.IsNaN(v) {
		return 0, false
	}
	le, total := 0, 0
	for _, x := range pop {
		if math.IsNaN(x) {
			continue
		}
		total++
		if x <= v {
			le++
		}
	}
	if total < minScorecardPopulation {
		return 0, false
	}
	return round2(float64(le) / float64(total) * 100), true
}

// subMetric ties a target stock's raw value to its population column getter + whether LOWER is
// better (value metrics — inverted so cheaper → higher percentile).
type subMetric struct {
	val    float64
	get    func(FactorMetrics) float64
	invert bool
}

// factorScore averages the available sub-metric percentiles into one factor percentile (nil when
// none are computable).
func factorScore(pop []FactorMetrics, subs []subMetric) *FactorScore {
	sum := 0.0
	cnt := 0
	for _, s := range subs {
		col := make([]float64, len(pop))
		for i, m := range pop {
			col[i] = s.get(m)
		}
		p, ok := percentile(s.val, col)
		if !ok {
			continue
		}
		if s.invert {
			p = round2(100 - p)
		}
		sum += p
		cnt++
	}
	if cnt == 0 {
		return nil
	}
	return &FactorScore{Percentile: round2(sum / float64(cnt)), Inputs: cnt}
}

// ComputeScorecard ranks `target` against `population` on the four factors. Value sub-metrics are
// inverted (lower P/E etc. → higher value percentile). Deterministic.
func ComputeScorecard(target FactorMetrics, population []FactorMetrics) Scorecard {
	return Scorecard{
		Value: factorScore(population, []subMetric{
			{target.PE, func(m FactorMetrics) float64 { return m.PE }, true},
			{target.PB, func(m FactorMetrics) float64 { return m.PB }, true},
			{target.PS, func(m FactorMetrics) float64 { return m.PS }, true},
		}),
		Growth: factorScore(population, []subMetric{
			{target.RevGrowth, func(m FactorMetrics) float64 { return m.RevGrowth }, false},
			{target.EarnGrowth, func(m FactorMetrics) float64 { return m.EarnGrowth }, false},
		}),
		Quality: factorScore(population, []subMetric{
			{target.ROE, func(m FactorMetrics) float64 { return m.ROE }, false},
			{target.ROIC, func(m FactorMetrics) float64 { return m.ROIC }, false},
			{target.EBITMargin, func(m FactorMetrics) float64 { return m.EBITMargin }, false},
			{target.Piotroski, func(m FactorMetrics) float64 { return m.Piotroski }, false},
		}),
		Momentum: factorScore(population, []subMetric{
			{target.TSR, func(m FactorMetrics) float64 { return m.TSR }, false},
		}),
		Population: len(population),
	}
}
