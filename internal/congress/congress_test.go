package congress

import (
	"testing"
)

// sampleFD mirrors the real Clerk FD index XML (incl. a leading UTF-8 BOM, a
// self-closing <Prefix/>, two PTRs, a non-PTR "C" filing, and a junk row missing
// both Last and DocID that must be dropped).
const sampleFD = "\xef\xbb\xbf" + `<?xml version="1.0" encoding="utf-8"?>
<FinancialDisclosure>
  <Member>
    <Prefix />
    <Last>Allen</Last>
    <First>Richard W.</First>
    <Suffix />
    <FilingType>P</FilingType>
    <StateDst>GA12</StateDst>
    <Year>2025</Year>
    <FilingDate>1/16/2025</FilingDate>
    <DocID>20026537</DocID>
  </Member>
  <Member>
    <Prefix>Mr.</Prefix>
    <Last>Aderholt</Last>
    <First>Robert B.</First>
    <Suffix />
    <FilingType>P</FilingType>
    <StateDst>AL04</StateDst>
    <Year>2025</Year>
    <FilingDate>9/10/2025</FilingDate>
    <DocID>20032062</DocID>
  </Member>
  <Member>
    <Prefix />
    <Last>Abel</Last>
    <First>William P.</First>
    <Suffix />
    <FilingType>C</FilingType>
    <StateDst>TX31</StateDst>
    <Year>2025</Year>
    <FilingDate>10/12/2025</FilingDate>
    <DocID>10072640</DocID>
  </Member>
  <Member>
    <Prefix />
    <Last></Last>
    <First></First>
    <Suffix />
    <FilingType>P</FilingType>
    <StateDst>CA11</StateDst>
    <Year>2025</Year>
    <FilingDate>5/1/2025</FilingDate>
    <DocID></DocID>
  </Member>
</FinancialDisclosure>`

func TestParseHouseFD(t *testing.T) {
	filings, err := parseHouseFD([]byte(sampleFD))
	if err != nil {
		t.Fatalf("parseHouseFD: %v", err)
	}
	// 4 members, but the junk row (no Last / no DocID) is dropped → 3 kept.
	if len(filings) != 3 {
		t.Fatalf("got %d filings, want 3", len(filings))
	}

	got := filings[0]
	if got.Name != "Richard W. Allen" {
		t.Errorf("name = %q, want %q", got.Name, "Richard W. Allen")
	}
	if got.State != "GA" || got.District != "12" {
		t.Errorf("state/district = %q/%q, want GA/12", got.State, got.District)
	}
	if got.Year != 2025 {
		t.Errorf("year = %d, want 2025", got.Year)
	}
	if got.FiledDate.Format("2006-01-02") != "2025-01-16" {
		t.Errorf("filed date = %s, want 2025-01-16", got.FiledDate.Format("2006-01-02"))
	}
}

func TestFilterPTRs(t *testing.T) {
	filings, err := parseHouseFD([]byte(sampleFD))
	if err != nil {
		t.Fatalf("parseHouseFD: %v", err)
	}
	ptrs := filterPTRs(filings, defaultBaseURL)

	// Only the two FilingType "P" rows survive (the "C" filing is dropped).
	if len(ptrs) != 2 {
		t.Fatalf("got %d PTRs, want 2", len(ptrs))
	}
	for _, p := range ptrs {
		if p.FilingType != "P" {
			t.Errorf("non-PTR leaked: %+v", p)
		}
	}
	// PDF URL is built from year + DocID.
	const wantURL = defaultBaseURL + "/public_disc/ptr-pdfs/2025/20026537.pdf"
	var allen *Filing
	for i := range ptrs {
		if ptrs[i].DocID == "20026537" {
			allen = &ptrs[i]
		}
	}
	if allen == nil {
		t.Fatal("Allen PTR not found")
	}
	if allen.PDFURL != wantURL {
		t.Errorf("pdf url = %q, want %q", allen.PDFURL, wantURL)
	}
}

func TestSplitStateDst(t *testing.T) {
	cases := []struct{ in, st, dst string }{
		{"GA12", "GA", "12"},
		{"AK00", "AK", "00"},
		{"CA", "CA", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		st, dst := splitStateDst(c.in)
		if st != c.st || dst != c.dst {
			t.Errorf("splitStateDst(%q) = %q/%q, want %q/%q", c.in, st, dst, c.st, c.dst)
		}
	}
}
