package substack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel>
  <title>Serenity</title>
  <item>
    <title>Sivers: The Undiscovered CPO Laser Chokepoint</title>
    <link>https://aleabitoreddit.substack.com/p/sivers</link>
    <pubDate>Mon, 19 May 2026 12:00:00 +0000</pubDate>
    <dc:creator>Serenity</dc:creator>
    <content:encoded><![CDATA[<p>My top pick is $SIVE. Also watching $POET, $MTSI and $AMD. This is not $AI hype, and the $CEO agrees.</p>]]></content:encoded>
    <description>A deep dive into the laser chokepoint.</description>
  </item>
</channel>
</rss>`

func TestPosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		_, _ = w.Write([]byte(sampleRSS))
	}))
	defer srv.Close()

	posts, err := New().Posts(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("posts=%d want 1", len(posts))
	}
	p := posts[0]
	if p.Author != "Serenity" {
		t.Errorf("author=%q", p.Author)
	}
	if !strings.Contains(p.Title, "Sivers") {
		t.Errorf("title=%q", p.Title)
	}
	if p.URL != "https://aleabitoreddit.substack.com/p/sivers" {
		t.Errorf("url=%q", p.URL)
	}
	if p.Published.Year() != 2026 {
		t.Errorf("published=%v", p.Published)
	}
	if p.Teaser == "" {
		t.Error("teaser should not be empty")
	}
	// SIVE, POET, MTSI, AMD; AI + CEO dropped by the stoplist.
	want := map[string]bool{"SIVE": true, "POET": true, "MTSI": true, "AMD": true}
	if len(p.Tickers) != 4 {
		t.Fatalf("tickers=%v want 4 (AI/CEO stopped)", p.Tickers)
	}
	for _, tk := range p.Tickers {
		if !want[tk] {
			t.Errorf("unexpected ticker %q", tk)
		}
	}
}

func TestExtractTickers(t *testing.T) {
	got := extractTickers("Buy $AAPL and $NVDA, not $AI or $CEO. $AAPL again.", nil)
	if len(got) != 2 { // AAPL (deduped) + NVDA; AI + CEO stopped
		t.Fatalf("got %v, want [AAPL NVDA]", got)
	}

	// US-exchange-prefixed mentions are extracted; with no universe set we trust the
	// author's explicit notation. Mixed-case exchange names + tight/padded colons.
	ex := extractTickers("We like Carvana (NYSE: CVNA) and Oracle (NASDAQ:ORCL); also Nasdaq: ZS.", nil)
	exset := map[string]bool{}
	for _, tk := range ex {
		exset[tk] = true
	}
	if !exset["CVNA"] || !exset["ORCL"] || !exset["ZS"] {
		t.Fatalf("exchange-prefixed extract = %v, want CVNA/ORCL/ZS", ex)
	}

	// Bare parentheticals are NOT extracted at all — "Reddit (RDDT)" yields nothing,
	// because prose acronyms collide with real tickers (a universe hit can't tell them
	// apart). A missing chip beats a wrong one.
	if b := extractTickers("Reddit (RDDT) had a strong quarter.", map[string]bool{"RDDT": true}); len(b) != 0 {
		t.Fatalf("bare parentheticals must not extract, got %v", b)
	}

	// With the universe known, only REAL US tickers survive: "$THE" (emphasis), an
	// all-caps word after "NASDAQ:" ("NASDAQ: GREAT"), and a foreign "LSE:" venue are
	// dropped; "$NVDA" and "NASDAQ: AAPL" are kept.
	valid := map[string]bool{"NVDA": true, "AAPL": true}
	v := extractTickers("$THE $NVDA story; listed NASDAQ: AAPL but NASDAQ: GREAT hype; Barclays (LSE: BARC).", valid)
	vset := map[string]bool{}
	for _, tk := range v {
		vset[tk] = true
	}
	if !vset["NVDA"] || !vset["AAPL"] {
		t.Fatalf("validated keep = %v, want NVDA+AAPL", v)
	}
	if vset["THE"] || vset["GREAT"] || vset["BARC"] {
		t.Fatalf("validated should drop THE/GREAT/BARC, got %v", v)
	}

	// A bare "NASDAQ" with no colon+ticker (e.g. "NASDAQ Price 10.82") must NOT match,
	// and a post that names no ticker returns a non-nil EMPTY slice (JSON []).
	none := extractTickers("An exciting IPO. NASDAQ Price 10.82. A company I've been watching.", nil)
	if none == nil || len(none) != 0 {
		t.Fatalf("no-ticker extract = %v, want non-nil empty", none)
	}
}
