package ingest

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/congress"
	"github.com/wombow-ai/tickwind/internal/congress/ptr"
)

// fakeCongressFetcher returns canned filings + per-URL PDF bytes, counting fetch
// calls so the test can assert incremental (parse-once) behaviour. No network.
type fakeCongressFetcher struct {
	filings   []congress.Filing
	pdfByURL  map[string][]byte
	fetchHits map[string]int
	curYear   int // the year whose call serves the fixture; others (prior-year merge) get []
}

func (f *fakeCongressFetcher) FetchHousePTRs(_ context.Context, year int) ([]congress.Filing, error) {
	// The ingestor pulls the current year, then (since the fixture is thin) the
	// prior year too. Serve the fixture only for the current year so the prior-year
	// merge adds nothing — mirroring a real year boundary with no overlap.
	if f.curYear == 0 {
		f.curYear = time.Now().UTC().Year()
	}
	if year != f.curYear {
		return nil, nil
	}
	return f.filings, nil
}

func (f *fakeCongressFetcher) FetchPDF(_ context.Context, url string) ([]byte, error) {
	if f.fetchHits == nil {
		f.fetchHits = map[string]int{}
	}
	f.fetchHits[url]++
	if b, ok := f.pdfByURL[url]; ok {
		return b, nil
	}
	return nil, io.EOF // simulate a fetch failure for unknown URLs
}

// fakeExtractor maps raw "PDF" bytes (here, just the layout text) straight
// through — it returns the bytes as text, so we feed pdftotext-layout fixtures
// directly as the PDF payload and skip the binary entirely.
type fakeExtractor struct{}

func (fakeExtractor) Extract(_ context.Context, pdf []byte) (string, error) {
	return string(pdf), nil
}

// A digital PTR layout fixture (one purchase of AAPL) for member "Nancy Pelosi".
const pelosiAAPL = `
          SP          Apple Inc. - Common Stock (AAPL)         P                 01/14/2025 01/14/2025           $250,001 -
                      [ST]                                                                                       $500,000
                      F      S      : New
`

// A second digital PTR for "Robert Aderholt" (a single GSK sale, same-line range).
const aderholtGSK = `
                       GSK plc American Depositary Shares         S                 07/28/2025 08/11/2025             $1,001 - $15,000
                       (GSK) [ST]
                       F      S      : New
`

// An image-only scan: a few stray glyphs, no dated rows → ErrScanned.
const scannedText = "Periodic Transaction Report\n(scanned)\n"

func newTestIngestor(t *testing.T, fetcher CongressFetcher, ex ptr.Extractor) (*CongressIngestor, *congress.Cache) {
	t.Helper()
	cache := congress.NewCache()
	ci := NewCongressIngestor(fetcher, cache, time.Hour, ex, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ci.throttle = 0 // no inter-PDF sleep in tests
	return ci, cache
}

func TestCongressIngestorParsesAndIndexes(t *testing.T) {
	base := "https://x/ptr"
	fetcher := &fakeCongressFetcher{
		filings: []congress.Filing{
			{Name: "Nancy Pelosi", State: "CA", FilingType: "P", DocID: "20026590", PDFURL: base + "/pelosi.pdf", FiledDate: time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC)},
			{Name: "Robert B. Aderholt", State: "AL", FilingType: "P", DocID: "20032062", PDFURL: base + "/aderholt.pdf", FiledDate: time.Date(2025, 8, 11, 0, 0, 0, 0, time.UTC)},
			// Scanned (short DocID) → skipped without a fetch.
			{Name: "Old Scanner", State: "TX", FilingType: "P", DocID: "8220731", PDFURL: base + "/scan.pdf", FiledDate: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)},
			// Non-PTR ("C") → ignored entirely.
			{Name: "Not A PTR", State: "NY", FilingType: "C", DocID: "10072640", PDFURL: base + "/c.pdf", FiledDate: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)},
		},
		pdfByURL: map[string][]byte{
			base + "/pelosi.pdf":   []byte(pelosiAAPL),
			base + "/aderholt.pdf": []byte(aderholtGSK),
		},
	}
	ci, cache := newTestIngestor(t, fetcher, fakeExtractor{})
	ci.refresh(context.Background())

	// Two digital PTRs fetched once each; the scanned (short DocID) one is skipped
	// before any fetch; the non-PTR is never considered.
	if got := fetcher.fetchHits[base+"/pelosi.pdf"]; got != 1 {
		t.Errorf("pelosi fetched %d times, want 1", got)
	}
	if got := fetcher.fetchHits[base+"/scan.pdf"]; got != 0 {
		t.Errorf("scanned (short DocID) fetched %d times, want 0", got)
	}
	if got := fetcher.fetchHits[base+"/c.pdf"]; got != 0 {
		t.Errorf("non-PTR fetched %d times, want 0", got)
	}

	// By-ticker index: AAPL → Pelosi purchase; GSK → Aderholt sale.
	aapl := cache.ByTicker("aapl") // case-insensitive
	if len(aapl) != 1 || aapl[0].MemberName != "Nancy Pelosi" || aapl[0].Type != string(ptr.TxPurchase) {
		t.Fatalf("ByTicker(AAPL) = %+v, want one Pelosi purchase", aapl)
	}
	gsk := cache.ByTicker("GSK")
	if len(gsk) != 1 || gsk[0].Slug != "robert-b-aderholt" || gsk[0].Type != string(ptr.TxSale) {
		t.Fatalf("ByTicker(GSK) = %+v, want one Aderholt sale", gsk)
	}

	// By-member: slug lookup returns the member + transactions.
	m, ok := cache.ByMember("nancy-pelosi")
	if !ok || m.Name != "Nancy Pelosi" || m.State != "CA" || len(m.Transactions) != 1 {
		t.Fatalf("ByMember(nancy-pelosi) = %+v ok=%v, want Pelosi with 1 tx", m, ok)
	}
	if _, ok := cache.ByMember("nobody"); ok {
		t.Error("ByMember(nobody) should be ok=false")
	}

	// Filing index is still stored (all four filings).
	if got := cache.Len(); got != 4 {
		t.Errorf("cache.Len() = %d, want 4", got)
	}
}

func TestCongressIngestorIncrementalNoRefetch(t *testing.T) {
	base := "https://x/ptr"
	fetcher := &fakeCongressFetcher{
		filings: []congress.Filing{
			{Name: "Nancy Pelosi", State: "CA", FilingType: "P", DocID: "20026590", PDFURL: base + "/pelosi.pdf", FiledDate: time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC)},
		},
		pdfByURL: map[string][]byte{base + "/pelosi.pdf": []byte(pelosiAAPL)},
	}
	ci, cache := newTestIngestor(t, fetcher, fakeExtractor{})

	ci.refresh(context.Background())
	ci.refresh(context.Background()) // second sweep: same DocID must NOT refetch

	if got := fetcher.fetchHits[base+"/pelosi.pdf"]; got != 1 {
		t.Errorf("pelosi fetched %d times across two sweeps, want 1 (incremental)", got)
	}
	if got := cache.ByTicker("AAPL"); len(got) != 1 {
		t.Errorf("ByTicker(AAPL) lost data across sweeps: %+v", got)
	}
}

// scannedExtractor returns the scanned text regardless of input, so ptr.Parse
// returns ErrScanned for a long-DocID filing too (an 8-digit filing that is in
// fact an image-only scan).
type scannedExtractor struct{}

func (scannedExtractor) Extract(_ context.Context, _ []byte) (string, error) {
	return scannedText, nil
}

func TestCongressIngestorScannedDigitalDocIDSkipped(t *testing.T) {
	base := "https://x/ptr"
	fetcher := &fakeCongressFetcher{
		filings: []congress.Filing{
			{Name: "Scanned Member", State: "FL", FilingType: "P", DocID: "20099999", PDFURL: base + "/s.pdf", FiledDate: time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)},
		},
		pdfByURL: map[string][]byte{base + "/s.pdf": []byte("ignored")},
	}
	ci, cache := newTestIngestor(t, fetcher, scannedExtractor{})
	ci.refresh(context.Background())

	// It was fetched (8-digit DocID) but parsed as scanned → no transactions, and
	// marked seen so it's not refetched.
	if got := fetcher.fetchHits[base+"/s.pdf"]; got != 1 {
		t.Errorf("scanned-digital fetched %d times, want 1", got)
	}
	ci.refresh(context.Background())
	if got := fetcher.fetchHits[base+"/s.pdf"]; got != 1 {
		t.Errorf("scanned-digital refetched: %d, want 1 (seen)", got)
	}
	if len(cache.Members()) != 0 {
		t.Errorf("scanned filing produced members: %+v", cache.Members())
	}
}

func TestCongressIngestorGracefulDegradeNoExtractor(t *testing.T) {
	base := "https://x/ptr"
	fetcher := &fakeCongressFetcher{
		filings: []congress.Filing{
			{Name: "Nancy Pelosi", State: "CA", FilingType: "P", DocID: "20026590", PDFURL: base + "/pelosi.pdf", FiledDate: time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC)},
		},
		pdfByURL: map[string][]byte{base + "/pelosi.pdf": []byte(pelosiAAPL)},
	}
	ci, cache := newTestIngestor(t, fetcher, nil) // nil extractor → index only
	ci.refresh(context.Background())

	if got := fetcher.fetchHits[base+"/pelosi.pdf"]; got != 0 {
		t.Errorf("no-extractor mode fetched a PDF %d times, want 0", got)
	}
	if cache.Len() != 1 {
		t.Errorf("filing index not stored: Len=%d, want 1", cache.Len())
	}
	if got := cache.ByTicker("AAPL"); len(got) != 0 {
		t.Errorf("no-extractor mode produced ticker data: %+v", got)
	}
}
