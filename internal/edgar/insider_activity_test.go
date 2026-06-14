package edgar

import (
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/sec"
)

// TestRawOwnershipDoc covers mapping a submissions primaryDocument to the raw
// ownership XML name: the XSL render-prefix is stripped, a bare XML name is kept,
// and non-XML / empty docs yield "".
func TestRawOwnershipDoc(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"xslF345X06/form4.xml", "form4.xml"},       // current XSL render prefix
		{"xslF345X05/wk-form4.xml", "wk-form4.xml"}, // older render prefix
		{"form4.xml", "form4.xml"},                  // already raw
		{"doc.XML", "doc.XML"},                      // case-insensitive .xml suffix
		{"primary_doc.html", ""},                    // not XML → skip
		{"", ""},                                    // empty → skip
		{"xslF345X06/", ""},                         // prefix but no doc → skip (not .xml)
	}
	for _, tt := range tests {
		if got := rawOwnershipDoc(tt.in); got != tt.want {
			t.Errorf("rawOwnershipDoc(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRecentForm4Refs covers the filing filter: only Form 4 / 4/A within the
// lookback window are kept, newest-first, capped, with the XSL prefix stripped to
// the raw XML URL and an accession index URL built from the trimmed CIK.
func TestRecentForm4Refs(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	old := time.Now().UTC().Add(-200 * 24 * time.Hour).Format("2006-01-02")

	var sub submissions8KResp
	r := &sub.Filings.Recent
	r.Form = []string{"4", "8-K", "4/A", "4", "10-Q"}
	r.FilingDate = []string{today, today, today, old, today}
	r.AccessionNumber = []string{"0000320193-26-000001", "x", "0000320193-26-000002", "0000320193-25-000003", "y"}
	r.PrimaryDocument = []string{"xslF345X06/form4.xml", "ax.htm", "xslF345X06/form4.xml", "xslF345X06/form4.xml", "q.htm"}

	refs := recentForm4Refs(sub, "0000320193")
	// Two recent Form 4 / 4/A kept (the old one is past the lookback; 8-K/10-Q skipped).
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2 (recent 4 and 4/A only): %+v", len(refs), refs)
	}
	wantAcc := "https://www.sec.gov/Archives/edgar/data/320193/000032019326000001/"
	if refs[0].accessionURL != wantAcc {
		t.Errorf("accessionURL = %q, want %q", refs[0].accessionURL, wantAcc)
	}
	wantXML := "https://www.sec.gov/Archives/edgar/data/320193/000032019326000001/form4.xml"
	if refs[0].xmlURL != wantXML {
		t.Errorf("xmlURL = %q, want %q (XSL prefix stripped)", refs[0].xmlURL, wantXML)
	}
}

// TestRecentForm4RefsCap: the per-refresh fetch fan-out is capped.
func TestRecentForm4RefsCap(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	n := maxInsiderFilings + 5
	var sub submissions8KResp
	r := &sub.Filings.Recent
	for i := 0; i < n; i++ {
		r.Form = append(r.Form, "4")
		r.FilingDate = append(r.FilingDate, today)
		r.AccessionNumber = append(r.AccessionNumber, "0000320193-26-00000"+string(rune('0'+i%10)))
		r.PrimaryDocument = append(r.PrimaryDocument, "xslF345X06/form4.xml")
	}
	if got := len(recentForm4Refs(sub, "0000320193")); got != maxInsiderFilings {
		t.Errorf("got %d refs, want cap %d", got, maxInsiderFilings)
	}
}

// TestInsiderRole covers role derivation: filed title wins, else generic
// Officer/Director from the flags, else "".
func TestInsiderRole(t *testing.T) {
	tests := []struct {
		name string
		f    sec.Form4
		want string
	}{
		{"officer title", sec.Form4{IsOfficer: true, OfficerTitle: "CEO"}, "CEO"},
		{"officer no title", sec.Form4{IsOfficer: true}, "Officer"},
		{"director", sec.Form4{IsDirector: true}, "Director"},
		{"officer title wins over director flag", sec.Form4{IsDirector: true, IsOfficer: true, OfficerTitle: "CFO"}, "CFO"},
		{"neither", sec.Form4{}, ""},
	}
	for _, tt := range tests {
		if got := insiderRole(tt.f); got != tt.want {
			t.Errorf("%s: insiderRole = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestCoalesceDate: the transaction date wins; the filing date is the fallback.
func TestCoalesceDate(t *testing.T) {
	if got := coalesceDate("2026-05-20", "2026-05-22"); got != "2026-05-20" {
		t.Errorf("coalesceDate(txn,filed) = %q, want the txn date", got)
	}
	if got := coalesceDate("", "2026-05-22"); got != "2026-05-22" {
		t.Errorf("coalesceDate(empty,filed) = %q, want the filed date", got)
	}
	if got := coalesceDate("  ", " "); got != "" {
		t.Errorf("coalesceDate(blank,blank) = %q, want empty", got)
	}
}
