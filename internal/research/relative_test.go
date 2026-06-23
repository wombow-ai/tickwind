package research

import (
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/indicators"
)

func fscore(p float64, inputs int) *indicators.FactorScore {
	return &indicators.FactorScore{Percentile: p, Inputs: inputs}
}

func TestRelativeSection(t *testing.T) {
	at := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	// Full scorecard → 4 percentile facts, EN "Nth percentile" values, population cited in source.
	sc := indicators.Scorecard{
		Value: fscore(18, 3), Growth: fscore(72.4, 4), Quality: fscore(55, 2), Momentum: fscore(88, 3),
		Population: 50,
	}
	sec := relativeSection(sc, at, "en")
	if sec.Key != "relative" || len(sec.Facts) != 4 {
		t.Fatalf("want 4 facts on the relative section, got %d: %+v", len(sec.Facts), sec)
	}
	byKey := map[string]Fact{}
	for _, f := range sec.Facts {
		byKey[f.Key] = f
	}
	if v := byKey["value_percentile"]; v.LabelEN != "Value percentile" || v.Value != "18th percentile" || v.Raw == nil || *v.Raw != 18 {
		t.Fatalf("value fact = %+v, want 'Value percentile' / '18th percentile' / raw 18", v)
	}
	if byKey["momentum_percentile"].Value != "88th percentile" {
		t.Fatalf("momentum value = %q, want '88th percentile'", byKey["momentum_percentile"].Value)
	}
	if byKey["growth_percentile"].Value != "72nd percentile" { // 72.4 rounds to 72
		t.Fatalf("growth value = %q, want '72nd percentile'", byKey["growth_percentile"].Value)
	}
	if !strings.Contains(byKey["value_percentile"].Source, "50") {
		t.Fatalf("source should cite the population (50), got %q", byKey["value_percentile"].Source)
	}
	if byKey["value_percentile"].AsOf != "2026-06-23" {
		t.Fatalf("as-of = %q, want 2026-06-23", byKey["value_percentile"].AsOf)
	}

	// A factor with Inputs<=0 is skipped (insufficient — never a fabricated percentile).
	partial := indicators.Scorecard{Value: fscore(40, 0), Momentum: fscore(60, 2), Population: 30}
	ps := relativeSection(partial, at, "en")
	if len(ps.Facts) != 1 || ps.Facts[0].Key != "momentum_percentile" {
		t.Fatalf("Inputs<=0 should be skipped; want only momentum, got %+v", ps.Facts)
	}

	// Empty scorecard → no facts (addSection then drops the whole section).
	if empty := relativeSection(indicators.Scorecard{}, at, "en"); len(empty.Facts) != 0 {
		t.Fatalf("empty scorecard should yield no facts, got %+v", empty.Facts)
	}

	// zh formatting.
	zh := relativeSection(sc, at, "zh")
	for _, f := range zh.Facts {
		if f.Key == "value_percentile" && f.Value != "第 18 百分位" {
			t.Fatalf("zh value = %q, want 第 18 百分位", f.Value)
		}
	}
}

func TestAssembleRelativeNilSource(t *testing.T) {
	sec := assembleRelative(indicators.StockIndicatorsResult{}, Sources{}, "en")
	if len(sec.Facts) != 0 {
		t.Fatalf("nil scorecard source → no facts, got %+v", sec.Facts)
	}
}

func TestOrdinalSuffix(t *testing.T) {
	cases := map[int]string{1: "st", 2: "nd", 3: "rd", 4: "th", 11: "th", 12: "th", 13: "th", 21: "st", 22: "nd", 23: "rd", 100: "th", 111: "th"}
	for n, want := range cases {
		if got := ordinalSuffix(n); got != want {
			t.Errorf("ordinalSuffix(%d) = %q, want %q", n, got, want)
		}
	}
	if fmtPercentileEN(82) != "82nd percentile" {
		t.Errorf("fmtPercentileEN(82) = %q", fmtPercentileEN(82))
	}
	if fmtPercentileEN(105) != "100th percentile" { // clamp to 100
		t.Errorf("clamp: fmtPercentileEN(105) = %q, want 100th percentile", fmtPercentileEN(105))
	}
}
