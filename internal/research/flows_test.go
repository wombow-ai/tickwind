package research

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/finra"
	"github.com/wombow-ai/tickwind/internal/finrashvol"
	"github.com/wombow-ai/tickwind/internal/indicators"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/sentiment"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/thirteenf"
)

// --- fakes for the flows + sentiment providers ---

// fakeCongress returns fixed per-ticker congressional trades.
type fakeCongress struct{ trades []congress.TickerTrade }

func (f fakeCongress) ByTicker(string) []congress.TickerTrade { return f.trades }

// fakeWhales returns fixed 13F holders.
type fakeWhales struct{ holders []thirteenf.Holder }

func (f fakeWhales) Holders(string) []thirteenf.Holder { return f.holders }

// fakeOptions returns a fixed options view (ok controls whether the symbol is
// treated as having listed options).
type fakeOptions struct {
	view ingest.OptionsView
	ok   bool
}

func (f fakeOptions) Options(context.Context, string) (ingest.OptionsView, bool) {
	return f.view, f.ok
}

// fakeShortVol returns a fixed latest short-volume row + history.
type fakeShortVol struct {
	latest  finrashvol.ShortVol
	ok      bool
	history []finrashvol.ShortVol
}

func (f fakeShortVol) Latest(string) (finrashvol.ShortVol, bool) { return f.latest, f.ok }
func (f fakeShortVol) History(string) []finrashvol.ShortVol      { return f.history }

// fakeShortInt returns a fixed settlement short-interest row.
type fakeShortInt struct {
	si finra.ShortInterest
	ok bool
}

func (f fakeShortInt) ShortInterest(string) (finra.ShortInterest, bool) { return f.si, f.ok }

// fakeMarket returns a fixed Fear & Greed result.
type fakeMarket struct {
	res sentiment.Result
	ok  bool
}

func (f fakeMarket) Latest() (sentiment.Result, bool) { return f.res, f.ok }

// fakeStore is a controllable StoreReader.
type fakeStore struct {
	signals    []store.Signal
	hot        map[string][]store.HotStock
	news       []store.News
	social     []store.Post
	insider    []store.InsiderBuy
	signalsErr error
}

func (f fakeStore) ListSignals(context.Context, string) ([]store.Signal, error) {
	return f.signals, f.signalsErr
}
func (f fakeStore) HotList(_ context.Context, board string, _ int) ([]store.HotStock, error) {
	return f.hot[board], nil
}
func (f fakeStore) ListNews(context.Context, string, int) ([]store.News, error) {
	return f.news, nil
}
func (f fakeStore) ListSocial(context.Context, string, int) ([]store.Post, error) {
	return f.social, nil
}
func (f fakeStore) RecentInsiderBuys(context.Context, time.Time) ([]store.InsiderBuy, error) {
	return f.insider, nil
}

// --- flows section tests ---

// TestAssembleFlowsFacts asserts the flows section is built from the fakes with
// the expected derived facts: congress member count + verbatim amount range, a 13F
// holder with its Period shown, options put/call ratios, and a short %.
func TestAssembleFlowsFacts(t *testing.T) {
	txDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	src := Sources{
		Congress: fakeCongress{trades: []congress.TickerTrade{
			{MemberName: "Nancy Pelosi", Slug: "nancy-pelosi", Type: "purchase", AmountRange: "$250,001 - $500,000", TxDate: txDate},
			{MemberName: "Ro Khanna", Slug: "ro-khanna", Type: "sale", AmountRange: "$1,001 - $15,000", TxDate: txDate.Add(-48 * time.Hour)},
		}},
		ThirteenF: fakeWhales{holders: []thirteenf.Holder{
			{FundSlug: "berkshire", FundName: "Berkshire Hathaway", Manager: "Warren Buffett", Value: 1e11, Weight: 42.3, Change: "add", Period: "2026-03-31"},
		}},
		Options: fakeOptions{ok: true, view: ingest.OptionsView{
			Ticker: "AAPL", PCVolume: 0.85, PCOI: 1.12, MaxPain: 190, Expiry: "2026-06-19",
			At: time.Date(2026, 6, 13, 20, 0, 0, 0, time.UTC),
		}},
		ShortVol: fakeShortVol{ok: true, latest: finrashvol.ShortVol{Symbol: "AAPL", ShortPct: 38.5, Date: "2026-06-12"}, history: []finrashvol.ShortVol{
			{ShortPct: 30.0}, {ShortPct: 38.5},
		}},
		ShortInt: fakeShortInt{ok: true, si: finra.ShortInterest{Symbol: "AAPL", DaysToCover: 2.4, ChangePct: 5.1, SettlementDate: "2026-05-30"}},
	}
	fs := Assemble(context.Background(), "AAPL", src)

	flows, ok := section(fs, "flows")
	if !ok {
		t.Fatal("flows section missing")
	}

	// Congress: distinct member count = 2.
	cm, ok := findFact(fs, "congress_members")
	if !ok {
		t.Fatal("congress_members fact missing")
	}
	if cm.Raw == nil || *cm.Raw != 2 {
		t.Errorf("congress_members raw = %v, want 2 distinct members", cm.Raw)
	}

	// Latest disclosed trade: AmountRange VERBATIM.
	cl, ok := findFact(fs, "congress_latest")
	if !ok {
		t.Fatal("congress_latest fact missing")
	}
	if !contains(cl.Value, "$250,001 - $500,000") {
		t.Errorf("congress_latest value = %q, want the verbatim amount range", cl.Value)
	}
	if !contains(cl.Value, "Nancy Pelosi") {
		t.Errorf("congress_latest value = %q, want the latest member name", cl.Value)
	}
	if cl.SourceURL != "/congress/member/nancy-pelosi" {
		t.Errorf("congress_latest source url = %q, want the member deep-link", cl.SourceURL)
	}

	// 13F: holder count + a holder line whose Period (stale) is shown.
	wc, ok := findFact(fs, "whales_count")
	if !ok {
		t.Fatal("whales_count fact missing")
	}
	if wc.AsOf != "2026-03-31" {
		t.Errorf("whales_count as_of = %q, want the filing period (13F must show its quarter)", wc.AsOf)
	}
	w1, ok := findFact(fs, "whale_1")
	if !ok {
		t.Fatal("whale_1 fact missing")
	}
	if w1.AsOf != "2026-03-31" {
		t.Errorf("whale_1 as_of = %q, want its filing Period shown (13F ~45d stale)", w1.AsOf)
	}
	if !contains(w1.Value, "42.3%") {
		t.Errorf("whale_1 value = %q, want the position weight", w1.Value)
	}
	if !contains(w1.Value, "Warren Buffett") {
		t.Errorf("whale_1 value = %q, want the manager", w1.Value)
	}

	// Options: put/call ratios present, AS-COMPUTED (not recomputed).
	pcv, ok := findFact(fs, "options_pc_volume")
	if !ok {
		t.Fatal("options_pc_volume fact missing")
	}
	if pcv.Raw == nil || *pcv.Raw != 0.85 {
		t.Errorf("options_pc_volume raw = %v, want 0.85 as-computed", pcv.Raw)
	}
	if _, ok := findFact(fs, "options_pc_oi"); !ok {
		t.Error("options_pc_oi fact missing")
	}

	// Short: derived % present.
	sp, ok := findFact(fs, "short_pct")
	if !ok {
		t.Fatal("short_pct fact missing")
	}
	if sp.Value != "38.5%" {
		t.Errorf("short_pct value = %q, want 38.5%%", sp.Value)
	}
	st, ok := findFact(fs, "short_trend")
	if !ok {
		t.Fatal("short_trend fact missing")
	}
	if !contains(st.Value, "rising") {
		t.Errorf("short_trend value = %q, want rising (30%%→38.5%%)", st.Value)
	}

	// Days-to-cover from settlement short interest.
	if dtc, ok := findFact(fs, "days_to_cover"); !ok || dtc.Raw == nil || *dtc.Raw != 2.4 {
		t.Errorf("days_to_cover fact missing/wrong: %+v", dtc)
	}

	// The flows section carries citations set in Go (never the LLM).
	if len(flows.Citations) == 0 {
		t.Error("flows section has no citations; want Go-set citations")
	}
}

// TestAssembleFlowsCongressEmptyNoFalseClaim asserts an empty/nil congress result
// produces NO congress fact (so the report never asserts "no member traded this"),
// and that an otherwise-empty flows section is OMITTED entirely.
func TestAssembleFlowsCongressEmptyNoFalseClaim(t *testing.T) {
	src := Sources{
		Congress: fakeCongress{trades: nil}, // nil = none OR PTR parsing disabled
	}
	fs := Assemble(context.Background(), "AAPL", src)

	if _, ok := findFact(fs, "congress_members"); ok {
		t.Error("congress_members fact present for an empty congress result; must be omitted (no false 'no trades' claim)")
	}
	if _, ok := findFact(fs, "congress_latest"); ok {
		t.Error("congress_latest fact present for an empty congress result")
	}
	// No other flows provider → the section has zero ok facts → omitted.
	if _, ok := section(fs, "flows"); ok {
		t.Error("flows section present despite zero usable facts; want omitted")
	}
}

// TestAssembleFlowsOptionsNoListedOptionsOmitted asserts ok=false from the options
// provider yields no options facts.
func TestAssembleFlowsOptionsNoListedOptionsOmitted(t *testing.T) {
	src := Sources{
		Options: fakeOptions{ok: false}, // symbol has no listed options
		// A short % keeps the flows section alive so we can assert the options
		// facts are specifically absent (not that the whole section vanished).
		ShortVol: fakeShortVol{ok: true, latest: finrashvol.ShortVol{ShortPct: 10, Date: "2026-06-12"}},
	}
	fs := Assemble(context.Background(), "NOOPT", src)

	if _, ok := findFact(fs, "options_pc_volume"); ok {
		t.Error("options fact present for a no-options symbol; want omitted")
	}
	if _, ok := section(fs, "flows"); !ok {
		t.Error("flows section omitted despite the short fact keeping it alive")
	}
}

// TestAssembleFlowsWhalesCountOldestPeriod is the BUG 5 check: when tracked funds
// filed for DIFFERENT quarters near a 13F deadline, the aggregate whales_count fact
// is stamped with the OLDEST (stalest) Period — the conservative as-of for the whole
// count — while each per-fund line keeps its own filing Period.
func TestAssembleFlowsWhalesCountOldestPeriod(t *testing.T) {
	src := Sources{
		ThirteenF: fakeWhales{holders: []thirteenf.Holder{
			// Sorted largest-first (as Holders returns); Periods deliberately mixed,
			// and the largest holder is NOT the oldest.
			{FundSlug: "berkshire", FundName: "Berkshire", Manager: "Buffett", Value: 1e11, Weight: 40, Change: "add", Period: "2026-03-31"},
			{FundSlug: "scion", FundName: "Scion", Manager: "Burry", Value: 5e8, Weight: 10, Change: "hold", Period: "2025-12-31"},
			{FundSlug: "pershing-square", FundName: "Pershing", Manager: "Ackman", Value: 2e8, Weight: 5, Change: "new", Period: "2026-03-31"},
		}},
	}
	fs := Assemble(context.Background(), "AAPL", src)

	wc, ok := findFact(fs, "whales_count")
	if !ok {
		t.Fatal("whales_count fact missing")
	}
	if wc.AsOf != "2025-12-31" {
		t.Errorf("whales_count as_of = %q, want the OLDEST holder Period 2025-12-31 (not the largest holder's quarter)", wc.AsOf)
	}
	// The per-fund line keeps its own (newer) Period — unchanged by this fix.
	w1, ok := findFact(fs, "whale_1")
	if !ok {
		t.Fatal("whale_1 fact missing")
	}
	if w1.AsOf != "2026-03-31" {
		t.Errorf("whale_1 as_of = %q, want its OWN filing Period 2026-03-31", w1.AsOf)
	}
}

// TestOldestPeriod unit-tests the oldest-Period helper, including empty-Period
// skipping and the all-empty case.
func TestOldestPeriod(t *testing.T) {
	tests := []struct {
		name    string
		holders []thirteenf.Holder
		want    string
	}{
		{"single", []thirteenf.Holder{{Period: "2026-03-31"}}, "2026-03-31"},
		{"mixed → oldest", []thirteenf.Holder{{Period: "2026-03-31"}, {Period: "2025-12-31"}, {Period: "2026-03-31"}}, "2025-12-31"},
		{"skips empty", []thirteenf.Holder{{Period: ""}, {Period: "2026-03-31"}}, "2026-03-31"},
		{"all empty → empty", []thirteenf.Holder{{Period: ""}, {Period: ""}}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := oldestPeriod(tc.holders); got != tc.want {
				t.Errorf("oldestPeriod = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- sentiment section tests ---

// TestAssembleSentimentMarketGuard asserts the market Fear & Greed fact appears
// ONLY when Available>0 (the neutral-50 fallback is never presented as real).
func TestAssembleSentimentMarketGuard(t *testing.T) {
	tests := []struct {
		name      string
		res       sentiment.Result
		ok        bool
		wantField bool
	}{
		{"real reading (available>0)", sentiment.Result{Score: 72, Label: "Greed", LabelZh: "贪婪", Available: 4}, true, true},
		{"neutral-50 fallback (available==0)", sentiment.Result{Score: 50, Label: "Neutral", LabelZh: "中性", Available: 0}, true, false},
		{"no result", sentiment.Result{}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// A buzz signal keeps the sentiment section alive so we can assert the
			// market fact specifically.
			src := Sources{
				Market: fakeMarket{res: tc.res, ok: tc.ok},
				Store: fakeStore{signals: []store.Signal{
					{Source: "apewisdom", Kind: "buzz", Mentions: 120, MentionsPrev: 80, Rank: 5, UpdatedAt: time.Now()},
				}},
			}
			fs := Assemble(context.Background(), "AAPL", src)
			fg, got := findFact(fs, "market_fear_greed")
			if got != tc.wantField {
				t.Fatalf("market_fear_greed present = %v, want %v", got, tc.wantField)
			}
			if tc.wantField {
				if fg.Raw == nil || *fg.Raw != 72 {
					t.Errorf("market_fear_greed raw = %v, want 72", fg.Raw)
				}
				if !contains(fg.Value, "贪婪") {
					t.Errorf("market_fear_greed value = %q, want the zh label", fg.Value)
				}
			}
		})
	}
}

// TestAssembleSentimentSignals asserts the buzz + news-sentiment facets become ok
// facts with derived numbers.
func TestAssembleSentimentSignals(t *testing.T) {
	src := Sources{
		Store: fakeStore{
			signals: []store.Signal{
				{Source: "apewisdom", Kind: "buzz", Mentions: 340, MentionsPrev: 210, Rank: 3, UpdatedAt: time.Now()},
				{Source: "alphavantage", Kind: "sentiment", Score: 0.32, Label: "Somewhat-Bullish", SampleSize: 18, UpdatedAt: time.Now()},
			},
			hot: map[string][]store.HotStock{
				"hot": {{Board: "hot", Ticker: "AAPL", Rank: 7, Mentions: 340, UpdatedAt: time.Now()}},
			},
		},
	}
	fs := Assemble(context.Background(), "AAPL", src)

	bm, ok := findFact(fs, "buzz_mentions")
	if !ok {
		t.Fatal("buzz_mentions fact missing")
	}
	if bm.Raw == nil || *bm.Raw != 340 {
		t.Errorf("buzz_mentions raw = %v, want 340", bm.Raw)
	}
	if !contains(bm.Value, "210") {
		t.Errorf("buzz_mentions value = %q, want the prior-window value noted", bm.Value)
	}
	ns, ok := findFact(fs, "news_sentiment")
	if !ok {
		t.Fatal("news_sentiment fact missing")
	}
	if ns.Raw == nil || *ns.Raw != 0.32 {
		t.Errorf("news_sentiment raw = %v, want 0.32", ns.Raw)
	}
	if !contains(ns.Value, "+0.32") {
		t.Errorf("news_sentiment value = %q, want a signed score", ns.Value)
	}
	hl, ok := findFact(fs, "hotlist_hot")
	if !ok {
		t.Fatal("hotlist_hot fact missing")
	}
	if hl.Raw == nil || *hl.Raw != 7 {
		t.Errorf("hotlist_hot raw = %v, want rank 7", hl.Raw)
	}
}

// TestAssembleSentimentNewsSocialNotNumericFacts asserts news + social UGC become
// ATTRIBUTED context (Section.Context) — never a numeric Fact, and never a
// fabricated sentiment number.
func TestAssembleSentimentNewsSocialNotNumericFacts(t *testing.T) {
	src := Sources{
		Store: fakeStore{
			// A buzz signal keeps the section alive; the corpus rides as context.
			signals: []store.Signal{{Source: "apewisdom", Kind: "buzz", Mentions: 50, Rank: 12, UpdatedAt: time.Now()}},
			news: []store.News{
				{Ticker: "AAPL", Headline: "Apple unveils new chip", Source: "Reuters", Published: time.Now()},
				{Ticker: "AAPL", Headline: "Analysts mixed on guidance", HeadlineZH: "分析师对指引看法不一", Source: "Bloomberg", Published: time.Now()},
			},
			social: []store.Post{
				{Ticker: "AAPL", Body: "loading up on calls\nthis is the way", Source: "stocktwits", Author: "bull42"},
			},
		},
	}
	fs := Assemble(context.Background(), "AAPL", src)

	sec, ok := section(fs, "sentiment")
	if !ok {
		t.Fatal("sentiment section missing")
	}

	// The corpus is attributed context, NOT facts.
	if len(sec.Context) == 0 {
		t.Fatal("sentiment section has no attributed context; want news/social lines")
	}
	for _, c := range sec.Context {
		if !contains(c, "据新闻") && !contains(c, "据社区讨论") {
			t.Errorf("context line %q is not attributed (据新闻/据社区讨论)", c)
		}
	}
	// The zh headline is preferred when present.
	foundZH := false
	for _, c := range sec.Context {
		if contains(c, "分析师对指引看法不一") {
			foundZH = true
		}
	}
	if !foundZH {
		t.Error("attributed context did not prefer the zh-translated headline")
	}

	// NO numeric fact was fabricated from the corpus. The only ok facts are the
	// structured signals — assert no headline/post text leaked into a Fact.Value
	// and that there is no synthesized "news/social sentiment" number beyond the
	// AlphaVantage facet (which is absent here — there is no sentiment signal).
	for _, f := range sec.Facts {
		if contains(f.Value, "Apple unveils") || contains(f.Value, "loading up on calls") {
			t.Errorf("UGC/news text leaked into a numeric Fact: %q", f.Value)
		}
	}
	if _, ok := findFact(fs, "news_sentiment"); ok {
		t.Error("news_sentiment fact present without an AlphaVantage sentiment signal; a number was fabricated from the corpus")
	}
}

// TestComposeNeverMutatesNumbersFlowsSentiment extends the anti-hallucination
// invariant to the new sections: even when the model returns prose for flows +
// sentiment (and stray bogus keys), every Fact's Value/Raw is unchanged and the
// attributed Context is left intact.
func TestComposeNeverMutatesNumbersFlowsSentiment(t *testing.T) {
	src := Sources{
		Indicators: &fakeIndicators{res: indicators.StockIndicatorsResult{
			Indicators: []indicators.StockIndicator{okIndicator("technical.rsi", 56.3, "")},
		}},
		Congress: fakeCongress{trades: []congress.TickerTrade{
			{MemberName: "Nancy Pelosi", Slug: "nancy-pelosi", Type: "purchase", AmountRange: "$250,001 - $500,000", TxDate: time.Now()},
		}},
		ThirteenF: fakeWhales{holders: []thirteenf.Holder{
			{FundSlug: "scion", FundName: "Scion", Manager: "Michael Burry", Value: 5e8, Weight: 12.5, Change: "new", Period: "2026-03-31"},
		}},
		ShortVol: fakeShortVol{ok: true, latest: finrashvol.ShortVol{ShortPct: 22.1, Date: "2026-06-12"}},
		Market:   fakeMarket{ok: true, res: sentiment.Result{Score: 60, Label: "Greed", LabelZh: "贪婪", Available: 3}},
		Store: fakeStore{
			signals: []store.Signal{{Source: "apewisdom", Kind: "buzz", Mentions: 99, Rank: 8, UpdatedAt: time.Now()}},
			news:    []store.News{{Ticker: "X", Headline: "headline", Source: "Reuters", Published: time.Now()}},
		},
	}
	data := Assemble(context.Background(), "X", src)

	enr := &fakeEnricher{enabled: true, prose: map[string]string{
		"flows":             "信号方向不一,机构与做空对立。",
		"sentiment":         "关注度上升,据社区讨论偏多。",
		"congress_members":  "5",          // not a section key → ignored
		"short_pct":         "0% (bogus)", // not a section key → ignored
		"market_fear_greed": "100",        // not a section key → ignored
	}}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", src), enr, "zh")

	// Prose filled for the new section keys.
	if sec, ok := section(composed, "flows"); !ok || sec.Prose == "" {
		t.Errorf("flows prose not filled: %+v", sec)
	}
	if sec, ok := section(composed, "sentiment"); !ok || sec.Prose == "" {
		t.Errorf("sentiment prose not filled: %+v", sec)
	}
	// Numbers untouched across ALL sections (incl. flows + sentiment).
	assertSameFacts(t, data, composed)

	// Attributed context survives Compose unchanged.
	before, _ := section(data, "sentiment")
	after, _ := section(composed, "sentiment")
	if len(before.Context) != len(after.Context) {
		t.Errorf("sentiment context len changed: %d → %d", len(before.Context), len(after.Context))
	}
}

// TestComposeMaterialMarksContextAttributed asserts the material the LLM receives
// flags the news/social corpus as attributed, non-numeric context.
func TestComposeMaterialMarksContextAttributed(t *testing.T) {
	src := Sources{
		Store: fakeStore{
			signals: []store.Signal{{Source: "apewisdom", Kind: "buzz", Mentions: 12, Rank: 30, UpdatedAt: time.Now()}},
			news:    []store.News{{Ticker: "X", Headline: "Big move expected", Source: "Reuters", Published: time.Now()}},
		},
	}
	enr := &fakeEnricher{enabled: true, prose: map[string]string{"sentiment": "ok"}}
	_ = Compose(context.Background(), Assemble(context.Background(), "X", src), enr, "zh")

	if !contains(enr.material, "据新闻") {
		t.Errorf("material missing the attributed news context; got:\n%s", enr.material)
	}
	if !contains(enr.material, "attributed context") {
		t.Errorf("material did not flag the corpus as attributed/non-numeric; got:\n%s", enr.material)
	}
}

// TestComposeErrorDataOnlyWithFlowsSentiment asserts a ComposeReport error leaves
// all prose "" even with the new sections present.
func TestComposeErrorDataOnlyWithFlowsSentiment(t *testing.T) {
	src := Sources{
		ShortVol: fakeShortVol{ok: true, latest: finrashvol.ShortVol{ShortPct: 15, Date: "2026-06-12"}},
		Store:    fakeStore{signals: []store.Signal{{Source: "apewisdom", Kind: "buzz", Mentions: 5, Rank: 40, UpdatedAt: time.Now()}}},
	}
	enr := &fakeEnricher{enabled: true, err: errors.New("down")}
	composed := Compose(context.Background(), Assemble(context.Background(), "X", src), enr, "zh")
	for _, sec := range composed.Sections {
		if sec.Prose != "" {
			t.Errorf("section %q prose = %q after error, want \"\"", sec.Key, sec.Prose)
		}
	}
}

// contains is a tiny substring helper for readable assertions.
func contains(s, sub string) bool { return strings.Contains(s, sub) }
