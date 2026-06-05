// Package clip fetches a web page and extracts a display title, for the
// "clipper" feature (saving links — X, Xiaohongshu, TikTok, … — to a stock's
// feed). Title extraction is best-effort and never fatal: an unreachable or
// bot-blocked page falls back to the URL host.
package clip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ogTitleRe = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// Fetcher extracts page titles for clipped links.
type Fetcher struct {
	http *http.Client
}

// NewFetcher returns a Fetcher with a short timeout.
func NewFetcher() *Fetcher {
	return &Fetcher{http: &http.Client{Timeout: 8 * time.Second}}
}

// Title fetches rawURL and returns a best-effort display title, falling back to
// the URL host when the page can't be read or has no title. Only http(s) URLs
// are accepted.
func (f *Fetcher) Title(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("clip: invalid url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Tickwind/0.1; +https://tickwind.com)")

	resp, err := f.http.Do(req)
	if err != nil {
		return u.Host, nil // unreachable: still save the link, titled by host
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	html := string(body)
	if m := ogTitleRe.FindStringSubmatch(html); len(m) == 2 {
		return cleanTitle(m[1]), nil
	}
	if m := titleRe.FindStringSubmatch(html); len(m) == 2 {
		return cleanTitle(m[1]), nil
	}
	return u.Host, nil
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(
		"&amp;", "&", "&quot;", `"`, "&#39;", "'", "&apos;", "'",
		"&lt;", "<", "&gt;", ">", "&nbsp;", " ",
	).Replace(s)
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace/newlines
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
