package ingest

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/congress/ptr"
)

// CongressFetcher fetches recent House Periodic Transaction Reports for a year
// and downloads an individual filing's PDF (satisfied by *congress.Client).
type CongressFetcher interface {
	FetchHousePTRs(ctx context.Context, year int) ([]congress.Filing, error)
	FetchPDF(ctx context.Context, url string) ([]byte, error)
}

const (
	// minDigitalDocID is the DocID length (after trimming) at/above which a PTR is
	// treated as a digital (machine-readable) filing worth parsing. Digital DocIDs
	// are 8 digits; shorter ones are typically scanned/paper filings with no
	// extractable text, so we skip the PDF fetch entirely for them.
	minDigitalDocID = 8
	// maxParsePerSweep caps how many new PDFs are parsed in a single sweep, so a
	// first run (with a full backlog of unseen DocIDs) can't hammer the Clerk
	// server or peg the CPU on pdftotext. The newest filings are parsed first; the
	// rest are picked up over subsequent sweeps.
	maxParsePerSweep = 60
	// defaultPDFThrottle paces the per-PDF fetch+parse, easing load on the Clerk
	// host and the local pdftotext process. Tunable via the ingestor's throttle
	// field (tests zero it).
	defaultPDFThrottle = 400 * time.Millisecond
)

// CongressIngestor refreshes the in-memory cache of recent congressional
// Periodic Transaction Reports (PTRs) from the public-domain House Clerk dataset
// on a slow cadence. Runs in its own goroutine, off the request path; memory-only
// + rebuildable, like the Universe / Opportunity caches (no per-refresh DB write).
//
// When an Extractor is injected, each sweep also downloads and parses the digital
// PTR PDFs it hasn't seen before — incrementally, throttled, and capped per sweep
// — accumulating the per-transaction detail (ticker, amount, buy/sell) so the
// board can drill down to ticker- and member-level trades. Without an Extractor
// (pdftotext unavailable), it degrades to storing the filing index alone.
type CongressIngestor struct {
	client    CongressFetcher
	cache     *congress.Cache
	every     time.Duration
	max       int
	extractor ptr.Extractor
	throttle  time.Duration
	log       *slog.Logger

	// seen accumulates parse results across sweeps so each digital PTR is fetched
	// + parsed exactly once. Accessed only from the single Run goroutine.
	seen   map[string]bool              // DocID → already attempted
	parsed map[string][]ptr.Transaction // DocID → parsed transactions
}

// NewCongressIngestor builds the ingestor; every is the refresh cadence. The
// extractor turns PTR PDFs into transactions; pass nil (or a failed
// ptr.NewPdftotext) to skip PDF parsing and store only the filing index.
func NewCongressIngestor(client CongressFetcher, cache *congress.Cache, every time.Duration, extractor ptr.Extractor, log *slog.Logger) *CongressIngestor {
	return &CongressIngestor{
		client:    client,
		cache:     cache,
		every:     every,
		max:       150,
		extractor: extractor,
		throttle:  defaultPDFThrottle,
		log:       log,
		seen:      make(map[string]bool),
		parsed:    make(map[string][]ptr.Transaction),
	}
}

// Run refreshes once on startup, then on the cadence, until ctx is cancelled.
func (c *CongressIngestor) Run(ctx context.Context) {
	if c.extractor == nil {
		c.log.Warn("congress: no PTR extractor (pdftotext) — storing filing index only, no ticker-level detail")
	}
	c.refresh(ctx)
	t := time.NewTicker(c.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.refresh(ctx)
		}
	}
}

func (c *CongressIngestor) refresh(ctx context.Context) {
	year := time.Now().UTC().Year()
	all, err := c.client.FetchHousePTRs(ctx, year)
	if err != nil {
		c.log.Warn("congress: fetch failed", "year", year, "err", err)
	}
	// The annual index is sparse early in the year (and a year-boundary view wants
	// the prior year's tail). Pull the prior year too when the current one is thin
	// or errored, then merge.
	if len(all) < 50 || err != nil {
		if prev, perr := c.client.FetchHousePTRs(ctx, year-1); perr != nil {
			c.log.Warn("congress: prior-year fetch failed", "year", year-1, "err", perr)
		} else {
			all = append(all, prev...)
		}
	}
	if len(all) == 0 {
		return
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].FiledDate.After(all[j].FiledDate) })
	if len(all) > c.max {
		all = all[:c.max]
	}

	// Without an extractor, keep the original behaviour: store the filing index
	// only (graceful degradation when pdftotext is missing).
	if c.extractor == nil {
		c.cache.Set(all)
		c.log.Info("congress: refreshed", "filings", len(all))
		return
	}

	c.parseNew(ctx, all)
	byMember := c.aggregate(all)
	c.cache.SetWithTransactions(all, byMember)
	c.log.Info("congress: refreshed", "filings", len(all), "parsed_docs", len(c.parsed), "members", len(byMember))
}

// parseNew fetches + parses the digital PTR PDFs in filings that we haven't seen
// before, up to maxParsePerSweep (newest first), throttled. Results accumulate in
// c.parsed; scanned/error filings are marked seen so they aren't retried forever.
func (c *CongressIngestor) parseNew(ctx context.Context, filings []congress.Filing) {
	var scanned, failed, ok int
	budget := maxParsePerSweep
	for _, f := range filings {
		if budget <= 0 {
			break
		}
		if !strings.EqualFold(f.FilingType, "P") {
			continue // only PTRs carry trade detail
		}
		docID := strings.TrimSpace(f.DocID)
		if docID == "" || c.seen[docID] {
			continue
		}
		if len(docID) < minDigitalDocID {
			// Short DocID ⇒ almost certainly a scanned/paper filing with no
			// extractable text. Skip the fetch and remember it so we don't recheck.
			c.seen[docID] = true
			scanned++
			continue
		}
		if f.PDFURL == "" {
			c.seen[docID] = true
			continue
		}
		if ctx.Err() != nil {
			return // shutdown / sweep timeout
		}

		budget--
		c.seen[docID] = true
		pdf, err := c.client.FetchPDF(ctx, f.PDFURL)
		if err != nil {
			c.log.Debug("congress: pdf fetch failed", "doc_id", docID, "err", err)
			failed++
			continue
		}
		res, err := ptr.Parse(ctx, c.extractor, pdf)
		switch {
		case errors.Is(err, ptr.ErrScanned):
			scanned++
		case err != nil:
			c.log.Debug("congress: ptr parse failed", "doc_id", docID, "err", err)
			failed++
		default:
			c.parsed[docID] = res.Transactions
			ok++
		}

		// Throttle between PDFs (skip when disabled). Respect ctx cancellation.
		if c.throttle <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.throttle):
		}
	}
	if ok > 0 || scanned > 0 || failed > 0 {
		c.log.Info("congress: ptr parse pass", "parsed", ok, "scanned", scanned, "failed", failed)
	}
}

// aggregate groups the accumulated transactions by member (owner), keyed by slug,
// over the filings currently in the snapshot. A filing's owner name (and state)
// comes from the index; its transactions come from c.parsed. Only filings still
// in the snapshot contribute, so members drop off as their filings age out.
func (c *CongressIngestor) aggregate(filings []congress.Filing) map[string]congress.MemberTx {
	byMember := make(map[string]congress.MemberTx)
	usedDoc := make(map[string]bool) // dedupe: a DocID may appear twice across the year merges
	for _, f := range filings {
		docID := strings.TrimSpace(f.DocID)
		if docID == "" || usedDoc[docID] {
			continue
		}
		usedDoc[docID] = true
		txs := c.parsed[docID]
		if len(txs) == 0 {
			continue
		}
		slug := congress.Slugify(f.Name)
		if slug == "" {
			continue
		}
		m := byMember[slug]
		if m.Slug == "" {
			m = congress.MemberTx{Slug: slug, Name: f.Name, State: f.State}
		}
		m.Transactions = append(m.Transactions, txs...)
		byMember[slug] = m
	}
	// Newest transaction first within each member, so the member page leads with
	// recent trades.
	for slug, m := range byMember {
		txs := m.Transactions
		sort.SliceStable(txs, func(i, j int) bool { return txs[i].TxDate.After(txs[j].TxDate) })
		m.Transactions = txs
		byMember[slug] = m
	}
	return byMember
}
