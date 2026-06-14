// Package substack reads public Substack/blog RSS feeds of finance "big-V"/KOL
// writers and extracts the tickers each post mentions (cashtags). It is the data
// behind the "Guru-watch" rail — newsletter-cadence opinions, attributed and
// linked to the source, never republished in full and never investment advice.
package substack

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Feed is one curated KOL publication (public RSS).
type Feed struct {
	Name string
	URL  string
}

// Feeds is the curated set of finance writers reachable via public RSS (verified
// live). Serenity (@aleabitoreddit) is the headline small-cap/semis voice; the
// rest skew small-cap / special-situations / value, on-theme for opportunities.
var Feeds = []Feed{
	{"Serenity", "https://aleabitoreddit.substack.com/feed"},
	{"The Value Road", "https://thevalueroad.substack.com/feed"},
	{"Planet MicroCap", "https://microcapnewsletter.substack.com/feed"},
	{"Emerging Value", "https://emergingvalue.substack.com/feed"},
	{"TripleS Special Situations", "https://triplesinvesting.substack.com/feed"},
	{"Capital Employed", "https://www.capitalemployed.com/feed"},
	{"Stock Market Nerd", "https://www.stockmarketnerd.com/feed"},
}

// Post is one newsletter post with the tickers it mentions.
type Post struct {
	Title     string
	URL       string
	Author    string
	Teaser    string // short, fair-use snippet — never the full (possibly paywalled) body
	Published time.Time
	Tickers   []string
}

// Client fetches and parses Substack/blog RSS feeds. The http.Client is
// injectable so production can route through a RESIDENTIAL proxy — Substack's
// feeds sit behind Cloudflare, which blocks datacenter IPs (the fetch fails and
// the rail goes stale), the same constraint the Nasdaq IPO client handles.
type Client struct {
	http *http.Client
}

// New returns a Client with a default direct http.Client (no proxy). Kept for
// back-compat and tests; production uses NewWithClient with a proxied client.
func New() *Client {
	return NewWithClient(nil)
}

// NewWithClient returns a Client over the given http.Client (a residential-proxy
// client in production, a test client in tests). A nil client falls back to a
// default with a sane timeout.
func NewWithClient(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{http: hc}
}

type rssFeed struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
			Creator string `xml:"creator"` // dc:creator
			Content string `xml:"encoded"` // content:encoded (full HTML on free posts)
			Desc    string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

var (
	cashtagRe = regexp.MustCompile(`\$([A-Z]{1,5})(?:\.[A-Z])?\b`)
	tagRe     = regexp.MustCompile(`<[^>]*>`)
	// stop drops common all-caps words that look like cashtags but aren't tickers.
	stop = map[string]bool{
		"A": true, "I": true, "AI": true, "CEO": true, "CFO": true, "CTO": true,
		"USD": true, "EPS": true, "IPO": true, "ETF": true, "GDP": true, "USA": true,
		"UK": true, "EU": true, "PM": true, "AM": true, "ET": true, "FED": true,
		"SEC": true, "Q1": true, "Q2": true, "Q3": true, "Q4": true, "YOY": true,
		"TAM": true, "ROE": true, "ROIC": true, "FCF": true, "DCF": true, "P": true,
	}
)

// Posts fetches one feed and returns its recent posts with extracted tickers.
func (c *Client) Posts(ctx context.Context, feedURL string) ([]Post, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("substack: build request: %w", err)
	}
	// Send a browser-like header set: Substack feeds sit behind Cloudflare,
	// which 403s a bare Go/bot User-Agent (mirrors the proxied Nasdaq client).
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("substack: get %s: %w", feedURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("substack: get %s: %s", feedURL, resp.Status)
	}

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("substack: decode %s: %w", feedURL, err)
	}

	out := make([]Post, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		body := stripHTML(it.Content)
		if body == "" {
			body = stripHTML(it.Desc)
		}
		out = append(out, Post{
			Title:     strings.TrimSpace(it.Title),
			URL:       strings.TrimSpace(it.Link),
			Author:    strings.TrimSpace(it.Creator),
			Teaser:    snippet(body, 200),
			Published: parseRSSDate(it.PubDate),
			Tickers:   extractTickers(it.Title + " " + body),
		})
	}
	return out, nil
}

// extractTickers pulls cashtag tickers from text, deduped, minus the stoplist.
func extractTickers(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range cashtagRe.FindAllStringSubmatch(text, -1) {
		t := m[1]
		if stop[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func stripHTML(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, " ")))
}

func snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return strings.TrimSpace(s[:n]) + "…"
	}
	return s
}

func parseRSSDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		time.RFC1123Z, time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
