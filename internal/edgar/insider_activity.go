package edgar

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/sec"
)

// InsiderTransaction is one open-market insider transaction (a buy or a sell)
// from a Form 4 filing — the building block of the per-ticker insider-activity
// timeline. Every field is a FACT owned by Go, parsed straight from the Form 4
// primary XML (internal/sec.ParseForm4): no LLM, no derived guess. Value is
// Shares×Price (Go-computed from the source figures). Planned10b5_1 is the
// best-effort Rule 10b5-1 planned-sale flag (false for buys, and false for a
// sale lacking a reliable indicator — never fabricated).
type InsiderTransaction struct {
	// Type is "buy" (Form 4 code P) or "sell" (code S).
	Type string `json:"type"`
	// Owner is the reporting insider's name as filed (e.g. "Cook Timothy D").
	Owner string `json:"owner"`
	// Role is the insider's role/title: the filed officer title when present,
	// else "Director"/"Officer" from the relationship flags, else "".
	Role string `json:"role,omitempty"`
	// Shares / Price / Value are the transaction figures; Value = Shares×Price.
	Shares float64 `json:"shares"`
	Price  float64 `json:"price"`
	Value  float64 `json:"value"`
	// Date is the transaction date (YYYY-MM-DD), or the filing date as a fallback
	// when the line omits a transaction date.
	Date string `json:"date"`
	// Planned10b5_1 reports an affirmed Rule 10b5-1 planned sale (sells only).
	Planned10b5_1 bool `json:"planned_10b5_1"`
	// AccessionURL is the human-readable filing index page on sec.gov.
	AccessionURL string `json:"accession_url"`
}

// insiderActivityLookback bounds how far back a Form 4 is considered "recent":
// we keep filings filed within this window. Form 4s are filed within 2 business
// days of the trade, so ~90 days captures a meaningful recent timeline.
const insiderActivityLookback = 90 * 24 * time.Hour

// maxInsiderFilings caps how many recent Form 4 filings are FETCHED per refresh.
// Large caps file many Form 4s and each is one throttled SEC request, so this
// bounds the per-refresh request fan-out; the cache absorbs the cost thereafter.
const maxInsiderFilings = 25

// maxInsiderTransactions caps how many parsed transactions are returned (newest
// first), so a chatty filer can't produce an unbounded timeline.
const maxInsiderTransactions = 25

// InsiderActivity returns a US ticker's recent open-market insider transactions
// (Form 4 buys AND sells), newest first. It REUSES the client's CIK lookup +
// SEC-compliant fetch: it lists the company's recent Form 4 / 4/A filings via the
// submissions feed, fetches each one's primary XML (capped at maxInsiderFilings),
// parses it with sec.ParseForm4 (Go owns every figure), and flattens the buys/
// sells into a newest-first timeline (capped at maxInsiderTransactions). Returns
// an error only when the ticker/CIK can't be resolved or the feed fetch fails (the
// handler 404s on that); an existing company with zero recent Form 4s returns an
// empty (non-nil) slice and nil error. Per-filing XML fetch/parse failures are
// skipped (best-effort) — one bad filing never fails the whole timeline.
func (c *Client) InsiderActivity(ctx context.Context, ticker string) ([]InsiderTransaction, error) {
	info, err := c.lookup(ctx, ticker)
	if err != nil {
		return nil, err
	}
	var sub submissions8KResp // reuses the parallel-arrays decode (form/date/accession/primaryDoc)
	if err := c.get(ctx, fmt.Sprintf(submissionsURL, info.CIK), &sub); err != nil {
		return nil, err
	}
	refs := recentForm4Refs(sub, info.CIK)

	out := make([]InsiderTransaction, 0, maxInsiderTransactions)
	for _, ref := range refs {
		if ref.xmlURL == "" {
			continue
		}
		body, err := c.getText(ctx, ref.xmlURL)
		if err != nil {
			continue // best-effort: a single bad filing never fails the timeline
		}
		f, err := sec.ParseForm4([]byte(body))
		if err != nil {
			continue
		}
		for _, b := range f.Buys {
			out = append(out, InsiderTransaction{
				Type:         "buy",
				Owner:        f.OwnerName,
				Role:         insiderRole(f),
				Shares:       b.Shares,
				Price:        b.Price,
				Value:        b.Value,
				Date:         coalesceDate(b.Date, ref.filedDate),
				AccessionURL: ref.accessionURL,
			})
		}
		for _, s := range f.Sells {
			out = append(out, InsiderTransaction{
				Type:          "sell",
				Owner:         f.OwnerName,
				Role:          insiderRole(f),
				Shares:        s.Shares,
				Price:         s.Price,
				Value:         s.Value,
				Date:          coalesceDate(s.Date, ref.filedDate),
				Planned10b5_1: s.Planned10b5_1,
				AccessionURL:  ref.accessionURL,
			})
		}
	}

	// Newest first by transaction date (YYYY-MM-DD compares lexically). Stable so
	// same-date lines keep their fetch (newest-filing-first) order.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	if len(out) > maxInsiderTransactions {
		out = out[:maxInsiderTransactions]
	}
	return out, nil
}

// form4Ref is one recent Form 4 filing resolved to the URLs we need.
type form4Ref struct {
	filedDate    string
	accessionURL string // human-readable filing index page
	xmlURL       string // direct URL to the primary ownership XML
}

// recentForm4Refs filters the submissions feed's parallel arrays down to recent
// Form 4 / 4/A filings (within insiderActivityLookback, newest first, capped at
// maxInsiderFilings) and resolves each to its accession + primary-XML URLs. Pure
// (no I/O) so it is unit-testable. The submissions feed's primaryDocument for a
// Form 4 is the XSL-RENDERED path ("xslF345X0N/form4.xml" → HTML); stripping the
// leading "xsl.../" segment yields the raw XML the parser needs.
func recentForm4Refs(sub submissions8KResp, cik string) []form4Ref {
	r := sub.Filings.Recent
	cikTrimmed := strings.TrimLeft(cik, "0")
	cutoff := time.Now().UTC().Add(-insiderActivityLookback)

	out := make([]form4Ref, 0, maxInsiderFilings)
	for i := 0; i < len(r.Form); i++ {
		form := strings.TrimSpace(r.Form[i])
		if form != "4" && form != "4/A" {
			continue
		}
		filed := at(r.FilingDate, i)
		if t, err := time.Parse("2006-01-02", filed); err == nil && t.Before(cutoff) {
			continue
		}
		acc := at(r.AccessionNumber, i)
		accNoDashes := strings.ReplaceAll(acc, "-", "")
		ref := form4Ref{
			filedDate: filed,
			accessionURL: fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/",
				cikTrimmed, accNoDashes),
		}
		if doc := rawOwnershipDoc(at(r.PrimaryDocument, i)); doc != "" {
			ref.xmlURL = fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/%s",
				cikTrimmed, accNoDashes, doc)
		}
		out = append(out, ref)
		if len(out) >= maxInsiderFilings {
			break
		}
	}
	// The submissions feed is already newest-first, but sort defensively by filed
	// date (descending) so ordering is guaranteed regardless of feed quirks.
	sort.SliceStable(out, func(i, j int) bool { return out[i].filedDate > out[j].filedDate })
	return out
}

// rawOwnershipDoc maps a submissions primaryDocument to the RAW ownership XML
// document name. The feed reports the XSL-rendered path for Form 4s
// ("xslF345X05/form4.xml", "xslF345X06/form4.xml", …) which serves HTML; the raw
// XML lives at the same folder without that leading "xsl.../" segment. A document
// without an xsl prefix is returned unchanged (already the raw doc). Returns ""
// for an empty/non-XML document so the caller skips it.
func rawOwnershipDoc(primaryDoc string) string {
	d := strings.TrimSpace(primaryDoc)
	if d == "" {
		return ""
	}
	if i := strings.LastIndex(d, "/"); i >= 0 {
		seg := d[:i]
		if strings.HasPrefix(strings.ToLower(seg), "xsl") {
			d = d[i+1:] // strip the leading "xsl.../" render-prefix → raw XML name
		}
	}
	if !strings.HasSuffix(strings.ToLower(d), ".xml") {
		return "" // Form 4 primary docs are XML; anything else isn't parseable here
	}
	return d
}

// insiderRole derives a display role for an insider from the parsed Form 4: the
// filed officer title when present, else a generic "Director"/"Officer" from the
// relationship flags, else "". Never fabricated.
func insiderRole(f sec.Form4) string {
	if t := strings.TrimSpace(f.OfficerTitle); t != "" {
		return t
	}
	switch {
	case f.IsOfficer:
		return "Officer"
	case f.IsDirector:
		return "Director"
	default:
		return ""
	}
}

// coalesceDate returns the transaction date when present, else the filing date
// as a fallback (a Form 4 is filed within 2 business days of the trade, so the
// filed date is a faithful stand-in when a line omits its transaction date).
func coalesceDate(txnDate, filedDate string) string {
	if d := strings.TrimSpace(txnDate); d != "" {
		return d
	}
	return strings.TrimSpace(filedDate)
}
