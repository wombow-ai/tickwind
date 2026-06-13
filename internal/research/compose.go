package research

import (
	"context"
	"fmt"
	"strings"
)

// ResearchEnricher is the narrow LLM slice the composer needs (the
// briefing.BriefEnricher pattern). It is satisfied by enrich.Enricher once
// ComposeReport is added, and is trivially fakeable in tests.
type ResearchEnricher interface {
	// Enabled reports whether a real LLM backend is configured.
	Enabled() bool
	// ComposeReport writes per-section research prose from a pre-built material
	// string, returning a section-key→prose map (qualitative only, no numbers).
	// Returns an error when disabled or on failure.
	ComposeReport(ctx context.Context, material, lang string) (map[string]string, error)
}

// Compose returns the FactSheet with per-section prose filled in. It builds ONE
// material string from fs.Sections, makes ONE ComposeReport call, and fills each
// SectionFacts.Prose by matching section key. The LLM is off the critical path:
// when enr is nil/disabled or the call errors, the data-only FactSheet is
// returned UNCHANGED (every prose stays "") and NO error is returned. The LLM can
// only ever touch Prose — it never sees or sets a Fact's Value or Raw.
func Compose(ctx context.Context, fs FactSheet, enr ResearchEnricher, lang string) FactSheet {
	if enr == nil || !enr.Enabled() {
		return fs
	}
	material := buildMaterial(fs, lang)
	if strings.TrimSpace(material) == "" {
		return fs
	}
	prose, err := enr.ComposeReport(ctx, material, lang)
	if err != nil || len(prose) == 0 {
		return fs // degrade to data-only; never propagate the error
	}
	for i := range fs.Sections {
		if p, ok := prose[fs.Sections[i].Key]; ok {
			fs.Sections[i].Prose = strings.TrimSpace(p)
		}
	}
	// The overview is a synthesis the LLM writes over all the other sections'
	// facts (it is NOT in the material as an input section). It is prose-only —
	// no facts of its own — so it exists only when the LLM produced it; the
	// data-only report (LLM off) has no overview. Rendered FIRST (prepended).
	if ov := strings.TrimSpace(prose[overviewKey]); ov != "" {
		fs.Sections = append([]SectionFacts{{
			Key:     overviewKey,
			TitleZH: "概览",
			TitleEN: "Overview",
			Prose:   ov,
		}}, fs.Sections...)
	}
	return fs
}

// overviewKey is the synthesis section the composer adds over all other sections.
const overviewKey = "overview"

// buildMaterial assembles the single pre-formatted material string the LLM sees,
// in the briefing.buildMaterial style: a header, then one block per section keyed
// by its stable Key, listing each ok fact as "Label: Value" and noting thin
// (insufficient) facts. A section may also carry attributed CONTEXT lines (news /
// social backdrop for the sentiment section) — these are quotable, ATTRIBUTED
// material ("据新闻/据社区讨论"), explicitly marked as non-numeric so the model
// reports them with attribution and never derives a sentiment number from them.
// The LLM is instructed to key its JSON reply by these section keys. Only
// formatted values appear — never raw structs — so the model cannot recompute a
// number.
func buildMaterial(fs FactSheet, lang string) string {
	var sb strings.Builder
	name := fs.Name
	if name == "" {
		name = fs.Ticker
	}
	fmt.Fprintf(&sb, "Ticker: %s (%s)\n", fs.Ticker, name)
	if fs.AsOf != "" {
		fmt.Fprintf(&sb, "As of: %s\n", fs.AsOf)
	}
	if fs.PriceLabel != "" {
		fmt.Fprintf(&sb, "Price: %s\n", fs.PriceLabel)
	}

	for _, sec := range fs.Sections {
		title := sec.TitleEN
		if lang != "en" && sec.TitleZH != "" {
			title = sec.TitleZH
		}
		fmt.Fprintf(&sb, "\n[%s] (key=%s)\n", title, sec.Key)
		var thin []string
		for _, f := range sec.Facts {
			label := f.LabelEN
			if lang != "en" && f.LabelZH != "" {
				label = f.LabelZH
			}
			if f.Status == StatusOK {
				src := ""
				if f.Source != "" {
					src = " [" + f.Source + "]"
				}
				fmt.Fprintf(&sb, "- %s: %s%s\n", label, f.Value, src)
			} else {
				thin = append(thin, label)
			}
		}
		if len(thin) > 0 {
			fmt.Fprintf(&sb, "- (数据不足 / insufficient: %s)\n", strings.Join(thin, ", "))
		}
		// Attributed, NON-NUMERIC context (news/social) — quote with attribution,
		// never restate as fact, never derive a number from it.
		if len(sec.Context) > 0 {
			fmt.Fprintf(&sb, "- (背景材料 / attributed context — quote with source, do NOT treat as fact or derive a number:)\n")
			for _, c := range sec.Context {
				fmt.Fprintf(&sb, "  · %s\n", c)
			}
		}
	}
	return sb.String()
}
