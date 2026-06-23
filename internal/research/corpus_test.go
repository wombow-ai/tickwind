package research

import (
	"context"
	"strings"
	"testing"

	"github.com/wombow-ai/tickwind/internal/store"
)

func TestCorpusContextLang(t *testing.T) {
	st := fakeStore{
		news:   []store.News{{Headline: "Apple beats earnings", HeadlineZH: "苹果财报超预期", Source: "Reuters"}},
		social: []store.Post{{Body: "great quarter", Source: "StockTwits"}},
	}

	// EN report → the ORIGINAL (English) headline + English attribution; no Chinese leak (the bug).
	en := corpusContext(context.Background(), "AAPL", st, "en")
	if len(en) != 2 {
		t.Fatalf("want 2 attributed lines, got %d: %v", len(en), en)
	}
	if !strings.Contains(en[0], "per news (Reuters): Apple beats earnings") {
		t.Errorf("en news line = %q, want the English headline + 'per news'", en[0])
	}
	if !strings.Contains(en[1], "per community discussion (StockTwits): great quarter") {
		t.Errorf("en social line = %q, want 'per community discussion'", en[1])
	}
	joined := strings.Join(en, " | ")
	for _, bad := range []string{"据新闻", "据社区讨论", "苹果财报超预期", "新闻", "社区"} {
		if strings.Contains(joined, bad) {
			t.Errorf("EN corpus leaked Chinese %q: %v", bad, en)
		}
	}

	// ZH report → the Chinese-translated headline + Chinese attribution (behavior preserved).
	zh := corpusContext(context.Background(), "AAPL", st, "zh")
	if !strings.Contains(zh[0], "据新闻(Reuters):苹果财报超预期") {
		t.Errorf("zh news line = %q, want the Chinese headline + 据新闻", zh[0])
	}
	if !strings.Contains(zh[1], "据社区讨论(StockTwits):great quarter") {
		t.Errorf("zh social line = %q, want 据社区讨论", zh[1])
	}

	// EN report with ONLY a Chinese headline available → fall back to it (better than dropping), but
	// keep the ENGLISH attribution prefix.
	only := fakeStore{news: []store.News{{HeadlineZH: "仅中文标题", Source: "X"}}}
	enFb := corpusContext(context.Background(), "AAPL", only, "en")
	if len(enFb) != 1 || !strings.Contains(enFb[0], "仅中文标题") || !strings.HasPrefix(enFb[0], "per news") {
		t.Errorf("en fallback should use the zh headline under an English 'per news' prefix, got %v", enFb)
	}

	// Empty Source → the language-matched fallback label ("news" / "新闻").
	noSrc := fakeStore{news: []store.News{{Headline: "h", Source: ""}}}
	if got := corpusContext(context.Background(), "AAPL", noSrc, "en"); !strings.Contains(got[0], "per news (news):") {
		t.Errorf("en empty-source label = %q, want 'news'", got[0])
	}
	if got := corpusContext(context.Background(), "AAPL", noSrc, "zh"); !strings.Contains(got[0], "据新闻(新闻):") {
		t.Errorf("zh empty-source label = %q, want 新闻", got[0])
	}
}
