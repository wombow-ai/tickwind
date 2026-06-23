package research

import (
	"context"
	"fmt"
	"strings"
)

// sentimentBoards are the hot-list boards the assembler checks the ticker against,
// in display order. "hot" = most-discussed overall, "wsb" = WSB rank-climbers.
var sentimentBoards = []struct{ board, labelZH, labelEN string }{
	{"hot", "热门榜排名", "Trending Rank"},
	{"wsb", "WSB 榜排名", "WSB Rank"},
}

// hotListScan caps how deep into each board the assembler looks for the ticker.
const hotListScan = 50

// sentimentNewsN / sentimentSocialN cap the attributed news/social corpus fed to
// the LLM as quotable context (never as facts).
const (
	sentimentNewsN   = 5
	sentimentSocialN = 5
)

// assembleSentiment builds the §1.5 情绪面 (Sentiment) section. The market-wide
// Fear & Greed reading is injected ONLY when it has participating components
// (Available>0) — the neutral-50 fallback is never presented as real. Per-ticker
// buzz (mentions vs prior, rank) and news-sentiment ([-1,1] score + label + sample
// size) come from store.Signal facets. Hot-list presence is a rank fact. The
// news/social corpus is ATTRIBUTED context (Section.Context) for the LLM — it is
// never turned into a numeric Fact and never fabricates a sentiment number. The
// section is omitted by the caller when it has zero ok facts. lang ("en"/"zh")
// selects the language of the Go-built labels embedded in a fact Value (the Fear
// & Greed band, the buzz prior-window note) — each value carries ONE language.
func assembleSentiment(ctx context.Context, ticker string, src Sources, lang string) SectionFacts {
	sec := SectionFacts{Key: "sentiment", TitleZH: "情绪面", TitleEN: "Sentiment"}
	var citations []Citation

	// --- Market-wide Fear & Greed (context only; guard Available>0) ---
	if src.Market != nil {
		if res, ok := src.Market.Latest(); ok && res.Available > 0 {
			score := float64(res.Score)
			// Select the classification band by the request lang (EN → English band
			// only, zh → Chinese band only — never bilingual). Fall back to the other
			// language only if the preferred one is empty.
			label := pickLang(lang, res.Label, res.LabelZh)
			if label == "" {
				label = pickLang(lang, res.LabelZh, res.Label)
			}
			val := fmt.Sprintf("%s (%s)", formatPlain(score), label)
			sec.Facts = append(sec.Facts, Fact{
				Key: "market_fear_greed", LabelZH: "市场恐惧贪婪指数", LabelEN: "Market Fear & Greed",
				Value: val, Raw: copyFloat(&score), Unit: unitNone,
				Status: StatusOK, Source: srcFearGreed,
			})
			citations = append(citations, Citation{Label: srcFearGreed, Anchor: "#sentiment"})
		}
		// Available==0 → the neutral-50 fallback is NOT a real reading → omit.
	}

	// --- Per-ticker buzz + news-sentiment signals + the UGC/news corpus ---
	if src.Store != nil {
		sigFacts, sigCites := signalFacts(ctx, ticker, src.Store, lang)
		sec.Facts = append(sec.Facts, sigFacts...)
		citations = append(citations, sigCites...)

		if hf, hc := hotListFacts(ctx, ticker, src.Store); len(hf) > 0 {
			sec.Facts = append(sec.Facts, hf...)
			citations = append(citations, hc...)
		}

		sec.Context = corpusContext(ctx, ticker, src.Store, lang)
	}

	sec.Citations = citations
	return sec
}

// signalFacts reads the per-ticker store.Signal rows and emits the buzz facet
// (mentions vs prior-window, rank) and the news-sentiment facet (score [-1,1] +
// label + sample size) as ok facts. Each facet contributes independently; a source
// that fills neither is ignored. Returns the facts and any citations. lang selects
// the language of the buzz prior-window note embedded in the mentions value.
func signalFacts(ctx context.Context, ticker string, sr StoreReader, lang string) ([]Fact, []Citation) {
	sigs, err := sr.ListSignals(ctx, ticker)
	if err != nil || len(sigs) == 0 {
		return nil, nil
	}
	var facts []Fact
	var citations []Citation
	for _, s := range sigs {
		switch strings.ToLower(s.Kind) {
		case "buzz":
			if s.Mentions > 0 {
				m := float64(s.Mentions)
				val := formatPlain(m)
				if s.MentionsPrev > 0 {
					// "前值" / "prior" — single-language per lang, never bilingual.
					prior := pickLang(lang, "prior", "前值")
					val = fmt.Sprintf("%s (%s %d)", formatPlain(m), prior, s.MentionsPrev)
				}
				facts = append(facts, Fact{
					Key: "buzz_mentions", LabelZH: "社区提及量", LabelEN: "Social Mentions",
					Value: val, Raw: copyFloat(&m), Unit: unitNone,
					Status: StatusOK, Source: srcBuzz, AsOf: formatTime(s.UpdatedAt),
				})
			}
			if s.Rank > 0 {
				r := float64(s.Rank)
				facts = append(facts, Fact{
					Key: "buzz_rank", LabelZH: "社区热度排名", LabelEN: "Buzz Rank",
					Value: "#" + formatPlain(r), Raw: copyFloat(&r), Unit: unitNone,
					Status: StatusOK, Source: srcBuzz, AsOf: formatTime(s.UpdatedAt),
				})
			}
			if s.Mentions > 0 || s.Rank > 0 {
				citations = append(citations, Citation{Label: srcBuzz, Anchor: "#signals"})
			}
		case "sentiment":
			if s.SampleSize > 0 {
				sc := s.Score
				label := s.Label
				val := formatSignedScore(sc)
				if label != "" {
					val = fmt.Sprintf("%s (%s, n=%d)", formatSignedScore(sc), label, s.SampleSize)
				} else {
					val = fmt.Sprintf("%s (n=%d)", formatSignedScore(sc), s.SampleSize)
				}
				facts = append(facts, Fact{
					Key: "news_sentiment", LabelZH: "新闻情绪评分", LabelEN: "News Sentiment",
					Value: val, Raw: copyFloat(&sc), Unit: unitNone,
					Status: StatusOK, Source: srcNewsSent, AsOf: formatTime(s.UpdatedAt),
				})
				citations = append(citations, Citation{Label: srcNewsSent, Anchor: "#signals"})
			}
		}
	}
	return facts, citations
}

// formatSignedScore renders a news-sentiment score in [-1,1] with a sign and two
// decimals (e.g. "+0.32", "-0.10", "0.00").
func formatSignedScore(v float64) string {
	s := formatPlain(v)
	if v > 0 && !strings.HasPrefix(s, "+") {
		s = "+" + s
	}
	return s
}

// hotListFacts reports whether the ticker appears on any tracked hot-list board
// and at what rank. A present ticker yields a rank fact per board it is on; an
// absent ticker yields no fact (presence is the signal — absence is not asserted).
func hotListFacts(ctx context.Context, ticker string, sr StoreReader) ([]Fact, []Citation) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	var facts []Fact
	var citations []Citation
	for _, b := range sentimentBoards {
		rows, err := sr.HotList(ctx, b.board, hotListScan)
		if err != nil || len(rows) == 0 {
			continue
		}
		for _, row := range rows {
			if strings.ToUpper(strings.TrimSpace(row.Ticker)) != ticker || row.Rank <= 0 {
				continue
			}
			r := float64(row.Rank)
			facts = append(facts, Fact{
				Key: "hotlist_" + b.board, LabelZH: b.labelZH, LabelEN: b.labelEN,
				Value: "#" + formatPlain(r), Raw: copyFloat(&r), Unit: unitNone,
				Status: StatusOK, Source: srcHotList, AsOf: formatTime(row.UpdatedAt),
			})
			citations = append(citations, Citation{Label: srcHotList, Anchor: "#hot"})
			break
		}
	}
	return facts, citations
}

// corpusContext pulls the top-N recent news headlines and social post snippets and formats them as
// ATTRIBUTED strings for the LLM — per the report language: "per news …" / "per community discussion …"
// for en, "据新闻 …" / "据社区讨论 …" for zh. These are quotable backdrop ONLY — never facts, never a
// synthesized sentiment number. The headline AND the attribution label are language-matched to the
// report so an EN report is never fed a Chinese-preferring snippet or a Chinese "新闻" source label (the
// preferred headline falls back to the other language only when the preferred one is empty). Returns
// nil when there is nothing to attribute.
func corpusContext(ctx context.Context, ticker string, sr StoreReader, lang string) []string {
	var out []string

	if news, err := sr.ListNews(ctx, ticker, sentimentNewsN); err == nil {
		for _, n := range news {
			// en → the original headline (usually English for US news); zh → the Chinese translation.
			h := strings.TrimSpace(pickLang(lang, n.Headline, n.HeadlineZH))
			if h == "" {
				h = strings.TrimSpace(pickLang(lang, n.HeadlineZH, n.Headline)) // fall back to the other language
			}
			if h == "" {
				continue
			}
			src := n.Source
			if src == "" {
				src = pickLang(lang, "news", "新闻")
			}
			out = append(out, pickLang(lang,
				fmt.Sprintf("per news (%s): %s", src, h),
				fmt.Sprintf("据新闻(%s):%s", src, h)))
		}
	}

	if posts, err := sr.ListSocial(ctx, ticker, sentimentSocialN); err == nil {
		for _, p := range posts {
			body := strings.TrimSpace(collapseSpace(p.Body))
			if body == "" {
				continue
			}
			// Truncate by RUNE, not byte: a social body is often Chinese, so a
			// byte slice (body[:200]) can cut mid-rune and feed a garbled character
			// to the LLM as attributed context. Mirrors the rune-aware truncation
			// used in internal/edgar/material_events.go.
			if r := []rune(body); len(r) > 200 {
				body = string(r[:200]) + "…"
			}
			src := p.Source
			if src == "" {
				src = pickLang(lang, "community", "社区")
			}
			out = append(out, pickLang(lang,
				fmt.Sprintf("per community discussion (%s): %s", src, body),
				fmt.Sprintf("据社区讨论(%s):%s", src, body)))
		}
	}
	return out
}

// collapseSpace collapses runs of whitespace (incl. newlines) in a UGC body to
// single spaces so a multi-line post reads as one attributed line.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
