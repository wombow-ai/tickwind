package sec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultDataBase    = "https://data.sec.gov" // XBRL frames / companyfacts
	defaultArchiveBase = "https://www.sec.gov"  // Archives, company_tickers, daily-index
	maxBody            = 64 << 20               // 64 MiB cap on any single response
)

// Client fetches public-domain SEC EDGAR data. It self-throttles to stay under
// SEC's 10 req/s fair-access limit and sends the required descriptive
// User-Agent (generic agents are 403'd). Safe for concurrent use; requests are
// serialized by the throttle.
type Client struct {
	http        *http.Client
	userAgent   string
	dataBase    string // data.sec.gov
	archiveBase string // www.sec.gov

	mu     sync.Mutex
	last   time.Time
	minGap time.Duration
}

// New returns a Client. userAgent MUST be descriptive and include a contact
// email, e.g. "Tickwind (contact@tickwind.com)".
func New(userAgent string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 30 * time.Second},
		userAgent:   userAgent,
		dataBase:    defaultDataBase,
		archiveBase: defaultArchiveBase,
		minGap:      120 * time.Millisecond, // ≈8 req/s, safely under SEC's 10/s
	}
}

// NewForTest returns a Client whose data.sec.gov AND www.sec.gov bases both
// point at base (an httptest server) with throttling disabled. It is for tests
// in other packages that need to drive the EDGAR endpoints (e.g. the shares
// frames the Opportunity ingestor sweeps) against a stub server.
func NewForTest(userAgent, base string) *Client {
	c := New(userAgent)
	c.dataBase = base
	c.archiveBase = base
	c.minGap = 0
	return c
}

// throttle blocks until at least minGap has elapsed since the previous request.
func (c *Client) throttle(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.minGap - time.Since(c.last); wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
	c.last = time.Now()
}

// get performs a rate-limited GET with the SEC User-Agent and returns the body
// (Go transparently negotiates + decompresses gzip), capped at maxBody.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	return c.getLimited(ctx, url, maxBody)
}

// getLimited is get but reads at most n bytes, then closes the body (aborting the
// rest of the transfer) — used to grab only the SGML header of a large filing.
func (c *Client) getLimited(ctx context.Context, url string, n int64) ([]byte, error) {
	c.throttle(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("sec: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sec: get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sec: get %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, n))
}

// sharesFrameStaleAfterDays bounds how far a frame row's reported instant (its
// "end" date) may lag the calendar quarter the frame is named for before the
// row is rejected as stale. A quarterly XBRL frame (CY{Y}Q{Q}I) collects each
// filer's most-recent point-in-time fact reported AS OF that quarter, so a
// current filer's end date sits within the quarter (≤~120 days of the quarter
// end). A frozen concept — e.g. an undimensioned dei/us-gaap cover-page count
// that stopped updating years ago — can still surface in a recent frame carrying
// its last (ancient) instant; this window (~15 months, matching the fundamentals
// path's sharesStaleAfterDays) drops such a row so a frozen count (a 2011
// Berkshire-style figure) can never feed a wrong market cap. Anti-hallucination:
// reject rather than use a provably-stale share count.
const sharesFrameStaleAfterDays = 450

// Shares returns common shares outstanding per CIK from the dei XBRL frame
// (dei:EntityCommonStockSharesOutstanding) for the given calendar quarter
// (instantaneous, the cover-page total — one row per CIK). Merge a few recent
// quarters at the call site and keep the freshest.
func (c *Client) Shares(ctx context.Context, year, quarter int) (map[int]int64, error) {
	return c.sharesFrame(ctx, "dei", "EntityCommonStockSharesOutstanding", year, quarter)
}

// SharesFallback returns common shares outstanding per CIK from the us-gaap
// XBRL frame (us-gaap:CommonStockSharesOutstanding) for the given calendar
// quarter. It is the fallback source for issuers that do NOT report the
// canonical dei cover-page concept (a real cohort of small/mid-caps), used at
// the call site ONLY for CIKs the dei frames left unresolved — widening the
// Opportunity board's coverage without a per-ticker companyfacts sweep, reusing
// the same throttled frames access pattern as Shares.
func (c *Client) SharesFallback(ctx context.Context, year, quarter int) (map[int]int64, error) {
	return c.sharesFrame(ctx, "us-gaap", "CommonStockSharesOutstanding", year, quarter)
}

// sharesFrame fetches a shares-outstanding XBRL frame for taxonomy/concept and
// calendar quarter, returning shares per CIK. It applies an anti-hallucination
// guard: a row whose reported instant ("end") is far older than the frame's
// quarter (sharesFrameStaleAfterDays) is REJECTED as a frozen count, and a row
// whose value is implausibly tiny (≤1 share — the 0/1 garbage carries some
// issuers like Paramount/Fox emit) is dropped, so a candidate is left off the
// board rather than gated by a wrong cap.
func (c *Client) sharesFrame(ctx context.Context, taxonomy, concept string, year, quarter int) (map[int]int64, error) {
	url := fmt.Sprintf(
		"%s/api/xbrl/frames/%s/%s/shares/CY%dQ%dI.json",
		c.dataBase, taxonomy, concept, year, quarter,
	)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			CIK int    `json:"cik"`
			End string `json:"end"` // the instant the value is reported as of (YYYY-MM-DD)
			// val is usually an integer share count, but a few filers report a
			// fractional value — decode as float64 so one odd row can't fail the
			// whole frame, then truncate to a share count.
			Val float64 `json:"val"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sec: shares decode: %w", err)
	}
	quarterEnd := quarterEndDate(year, quarter)
	out := make(map[int]int64, len(resp.Data))
	for _, d := range resp.Data {
		v := int64(d.Val)
		if v <= 1 { // reject 0/1-share garbage (e.g. Paramount/Fox carries)
			continue
		}
		if frameRowStale(d.End, quarterEnd) { // reject a frozen ancient instant
			continue
		}
		if v > out[d.CIK] { // defensive: keep the largest if dupes appear
			out[d.CIK] = v
		}
	}
	return out, nil
}

// quarterEndDate returns the last calendar day of the given quarter (the anchor
// the frame's row instants are compared against for the staleness guard).
func quarterEndDate(year, quarter int) time.Time {
	endMonth := time.Month(quarter * 3) // Q1→Mar, Q2→Jun, Q3→Sep, Q4→Dec
	firstNext := time.Date(year, endMonth, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	return firstNext.AddDate(0, 0, -1)
}

// frameRowStale reports whether a frame row's reported instant (end, YYYY-MM-DD)
// is more than sharesFrameStaleAfterDays before the frame's quarter end — i.e. a
// frozen count surfacing in a recent frame. Returns false (not stale) when end
// is empty or unparseable, so the guard only ever fires on a provable gap and
// never drops a row it cannot judge.
func frameRowStale(end string, quarterEnd time.Time) bool {
	if end == "" {
		return false
	}
	e, err := time.Parse("2006-01-02", end)
	if err != nil {
		return false
	}
	return quarterEnd.Sub(e).Hours()/24 > sharesFrameStaleAfterDays
}
