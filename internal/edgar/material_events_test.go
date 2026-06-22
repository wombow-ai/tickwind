package edgar

import (
	"strings"
	"testing"
	"time"
)

// TestParseItems covers the item-code parser: the SEC feed packs item codes into
// one string separated by commas and/or newlines, optionally with an "Item "
// prefix; each kept code maps to its Go-owned canonical label pair, unknown codes
// fall back to a generic label, and duplicates are dropped (first-seen order).
func TestParseItems(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   []string // expected codes in order
		wantEN map[string]string
		wantZH map[string]string
	}{
		{
			name:   "comma separated",
			raw:    "5.02,9.01",
			want:   []string{"5.02", "9.01"},
			wantEN: map[string]string{"5.02": "Departure of Directors or Certain Officers; Election of Directors; Appointment of Certain Officers; Compensatory Arrangements of Certain Officers", "9.01": "Financial Statements and Exhibits"},
			wantZH: map[string]string{"5.02": "董事或高管离任/任命及薪酬安排", "9.01": "财务报表与附件"},
		},
		{
			name: "newline separated",
			raw:  "2.02\n9.01",
			want: []string{"2.02", "9.01"},
			wantEN: map[string]string{
				"2.02": "Results of Operations and Financial Condition",
				"9.01": "Financial Statements and Exhibits",
			},
		},
		{
			name: "carriage-return + Item prefix + whitespace",
			raw:  " Item 7.01 \r\n Item 8.01 ",
			want: []string{"7.01", "8.01"},
			wantEN: map[string]string{
				"7.01": "Regulation FD Disclosure",
				"8.01": "Other Events",
			},
		},
		{
			name: "duplicate codes deduped, first-seen order kept",
			raw:  "5.07,9.01,5.07",
			want: []string{"5.07", "9.01"},
		},
		{
			name: "unknown/future code falls back to generic label, never crashes",
			raw:  "9.99",
			want: []string{"9.99"},
			wantEN: map[string]string{
				"9.99": "Item 9.99",
			},
			wantZH: map[string]string{
				"9.99": "事项 9.99",
			},
		},
		{
			name: "empty string yields no items",
			raw:  "",
			want: nil,
		},
		{
			name: "whitespace-only yields no items",
			raw:  "  \n  ",
			want: nil,
		},
		{
			name: "semicolon + comma mix",
			raw:  "1.01; 2.01, 8.01",
			want: []string{"1.01", "2.01", "8.01"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseItems(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("parseItems(%q) returned %d items, want %d: %+v", tc.raw, len(got), len(tc.want), got)
			}
			for i, code := range tc.want {
				if got[i].Code != code {
					t.Errorf("item[%d].Code = %q, want %q", i, got[i].Code, code)
				}
				if got[i].LabelEN == "" || got[i].LabelZH == "" {
					t.Errorf("item[%d] (%s) has an empty label: en=%q zh=%q", i, code, got[i].LabelEN, got[i].LabelZH)
				}
				if want, ok := tc.wantEN[code]; ok && got[i].LabelEN != want {
					t.Errorf("item[%d] (%s) LabelEN = %q, want %q", i, code, got[i].LabelEN, want)
				}
				if want, ok := tc.wantZH[code]; ok && got[i].LabelZH != want {
					t.Errorf("item[%d] (%s) LabelZH = %q, want %q", i, code, got[i].LabelZH, want)
				}
			}
		})
	}
}

// TestLabelCoverage asserts the canonical item-code → label map covers the
// standard SEC 8-K item codes (Sections 1–9) with non-empty bilingual labels, and
// that an unknown code degrades to a generic label in both languages (never a
// fabricated meaning).
func TestLabelCoverage(t *testing.T) {
	// The standard SEC Form 8-K item codes that MUST be mapped.
	standard := []string{
		"1.01", "1.02", "1.03", "1.04", "1.05",
		"2.01", "2.02", "2.03", "2.04", "2.05", "2.06",
		"3.01", "3.02", "3.03",
		"4.01", "4.02",
		"5.01", "5.02", "5.03", "5.04", "5.05", "5.06", "5.07", "5.08",
		"6.01", "6.02", "6.03", "6.04", "6.05", "6.06",
		"7.01",
		"8.01",
		"9.01",
	}
	for _, code := range standard {
		en, zh := label(code)
		if en == "" || zh == "" {
			t.Errorf("code %s has an empty canonical label: en=%q zh=%q", code, en, zh)
		}
		// A standard code must NOT fall back to the generic "Item X.XX" form.
		if en == "Item "+code {
			t.Errorf("standard code %s fell back to a generic English label — missing from itemLabels", code)
		}
		if zh == "事项 "+code {
			t.Errorf("standard code %s fell back to a generic Chinese label — missing from itemLabels", code)
		}
	}

	// Unknown / future code → generic bilingual label, never a fabricated meaning.
	en, zh := label("12.34")
	if en != "Item 12.34" {
		t.Errorf("unknown code English fallback = %q, want %q", en, "Item 12.34")
	}
	if zh != "事项 12.34" {
		t.Errorf("unknown code Chinese fallback = %q, want %q", zh, "事项 12.34")
	}
}

// TestExtractMaterialEvents covers the parallel-array filtering: only 8-K / 8-K/A
// forms within the lookback window are kept, the amendment flag and dates are set
// correctly, the accession + primary-doc URLs are built from the CIK, and the
// result is capped + newest-first.
func TestExtractMaterialEvents(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	old := time.Now().UTC().AddDate(0, 0, -400).Format("2006-01-02") // outside lookback

	var sub submissions8KResp
	r := &sub.Filings.Recent
	// Index-aligned parallel arrays. Mix forms so the 8-K filter is exercised.
	r.Form = []string{"8-K", "10-Q", "8-K/A", "4", "8-K", "8-K"}
	r.FilingDate = []string{today, today, yesterday, today, old, yesterday}
	r.ReportDate = []string{today, today, yesterday, today, old, yesterday}
	r.AccessionNumber = []string{
		"0000320193-26-000001",
		"0000320193-26-000002",
		"0000320193-26-000003",
		"0000320193-26-000004",
		"0000320193-26-000005",
		"0000320193-26-000006",
	}
	r.PrimaryDocument = []string{"a.htm", "b.htm", "c.htm", "d.htm", "e.htm", "f.htm"}
	r.Items = []string{"2.02,9.01", "", "5.02", "", "8.01", "7.01"}

	cik := "0000320193"
	got := extractMaterialEvents(sub, cik)

	// Expect the 3 in-window 8-K/8-K/A filings (indices 0, 2, 5); the 10-Q, Form 4,
	// and the out-of-window 8-K (index 4) are dropped.
	if len(got) != 3 {
		t.Fatalf("extractMaterialEvents kept %d filings, want 3: %+v", len(got), got)
	}

	// Every kept filing is an 8-K or 8-K/A.
	for _, ev := range got {
		if ev.Form != "8-K" && ev.Form != "8-K/A" {
			t.Errorf("kept a non-8-K form: %q", ev.Form)
		}
		if ev.Items == nil {
			t.Errorf("filing %s has nil Items (must be coerced non-nil)", ev.FiledDate)
		}
	}

	// Newest-first: today before yesterday.
	if got[0].FiledDate != today {
		t.Errorf("first filing FiledDate = %q, want newest (%q)", got[0].FiledDate, today)
	}

	// The amendment flag + label parsing for the 8-K/A (index 2, items "5.02").
	var amend *MaterialEvent
	for i := range got {
		if got[i].Form == "8-K/A" {
			amend = &got[i]
		}
	}
	if amend == nil {
		t.Fatal("expected the 8-K/A amendment to be kept")
	}
	if !amend.Amendment {
		t.Errorf("8-K/A Amendment flag = false, want true")
	}
	if len(amend.Items) != 1 || amend.Items[0].Code != "5.02" {
		t.Errorf("8-K/A items = %+v, want a single 5.02", amend.Items)
	}

	// Accession URL is the folder index (no-dashes accession), primary-doc URL adds
	// the document. CIK is trimmed of leading zeros.
	for _, ev := range got {
		if !strings.HasPrefix(ev.AccessionURL, "https://www.sec.gov/Archives/edgar/data/320193/") {
			t.Errorf("AccessionURL %q lacks the trimmed-CIK folder prefix", ev.AccessionURL)
		}
		if !strings.HasSuffix(ev.AccessionURL, "/") {
			t.Errorf("AccessionURL %q should be the folder index (trailing slash)", ev.AccessionURL)
		}
		if !strings.Contains(ev.PrimaryDocURL, "/320193/") || !strings.HasSuffix(ev.PrimaryDocURL, ".htm") {
			t.Errorf("PrimaryDocURL %q malformed", ev.PrimaryDocURL)
		}
	}
}

// TestExtractMaterialEventsCap asserts the result is capped at maxMaterialEvents
// even when many in-window 8-Ks exist.
func TestExtractMaterialEventsCap(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	n := maxMaterialEvents + 5
	var sub submissions8KResp
	r := &sub.Filings.Recent
	for i := 0; i < n; i++ {
		r.Form = append(r.Form, "8-K")
		r.FilingDate = append(r.FilingDate, today)
		r.ReportDate = append(r.ReportDate, today)
		r.AccessionNumber = append(r.AccessionNumber, "0000320193-26-00000"+string(rune('0'+i%10)))
		r.PrimaryDocument = append(r.PrimaryDocument, "doc.htm")
		r.Items = append(r.Items, "8.01")
	}
	got := extractMaterialEvents(sub, "0000320193")
	if len(got) != maxMaterialEvents {
		t.Fatalf("extractMaterialEvents kept %d, want cap %d", len(got), maxMaterialEvents)
	}
}

// TestHTMLToText covers the best-effort HTML→plain-text reducer feeding the LLM:
// script/style/comments are dropped, tags stripped, entities unescaped, and
// whitespace collapsed — no markup must leak into the prompt.
func TestHTMLToText(t *testing.T) {
	in := `<html><head><style>.x{color:red}</style><script>var a=1;</script></head>` +
		`<body><!-- comment --><p>Apple Inc. announced&nbsp;a&nbsp;new&#160;CEO &amp; CFO.</p>` +
		`<div>Item&nbsp;5.02</div></body></html>`
	got := htmlToText(in)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("markup leaked into output: %q", got)
	}
	if strings.Contains(got, "color:red") || strings.Contains(got, "var a=1") {
		t.Errorf("script/style leaked into output: %q", got)
	}
	if strings.Contains(got, "comment") {
		t.Errorf("HTML comment leaked into output: %q", got)
	}
	if !strings.Contains(got, "Apple Inc. announced a new CEO & CFO.") {
		t.Errorf("expected unescaped body text, got %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Errorf("whitespace not collapsed: %q", got)
	}
}

// TestTruncateRunes asserts truncation is rune-safe (never splits a multibyte
// char) and leaves a short string unchanged.
func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 100); got != "hello" {
		t.Errorf("short string changed: %q", got)
	}
	// 5 Chinese runes; truncate to 3 → 3 runes, valid UTF-8.
	got := truncateRunes("你好世界啊", 3)
	if r := []rune(got); len(r) != 3 {
		t.Errorf("truncateRunes returned %d runes, want 3: %q", len(r), got)
	}
}

func TestExtractEarningsDates(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	d10 := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02") // within earningsMinSpacing of today
	d90 := time.Now().UTC().AddDate(0, 0, -90).Format("2006-01-02") // a separate quarter
	d180 := time.Now().UTC().AddDate(0, 0, -180).Format("2006-01-02")
	tooOld := time.Now().UTC().AddDate(-4, 0, 0).Format("2006-01-02") // outside earningsDatesLookback

	var sub submissions8KResp
	r := &sub.Filings.Recent
	// Index-aligned parallel arrays mixing forms + item codes.
	r.Form = []string{"8-K", "8-K", "10-Q", "8-K/A", "8-K", "8-K", "8-K", "8-K"}
	r.FilingDate = []string{today, d90, today, d180, d180, tooOld, today, d10}
	r.Items = []string{
		"2.02,9.01", // earnings — keep
		"2.02",      // earnings (separate quarter) — keep
		"2.02",      // a 10-Q (not 8-K) — drop
		"2.02",      // 8-K/A amendment — drop
		"5.02",      // not earnings — drop
		"2.02",      // earnings but too old — drop
		"2.02",      // earnings, SAME date as index 0 (today) — dedupe
		"2.02",      // earnings 10 days ago — within 45d of today → COLLAPSED (intra-quarter)
	}
	// Pad the other arrays so at() never indexes out of range.
	r.AccessionNumber = make([]string, len(r.Form))
	r.ReportDate = make([]string, len(r.Form))
	r.PrimaryDocument = make([]string, len(r.Form))

	got := extractEarningsDates(sub)
	// Expect 2 distinct quarterly dates: today (the SAME-date filing deduped + the d10 intra-quarter
	// filing collapsed into it) + d90. d180/index4 is 5.02; the 10-Q + 8-K/A drop; tooOld is out of window.
	if len(got) != 2 {
		t.Fatalf("extractEarningsDates kept %d dates, want 2: %v", len(got), got)
	}
	if !got[0].After(got[1]) {
		t.Fatalf("not newest-first: %v", got)
	}
	if got[0].Format("2006-01-02") != today || got[1].Format("2006-01-02") != d90 {
		t.Fatalf("wrong dates: %v (want %s, %s)", got, today, d90)
	}
}

func TestHasItem(t *testing.T) {
	if !hasItem("2.02,9.01", "2.02") {
		t.Fatal("hasItem missed 2.02 in '2.02,9.01'")
	}
	if hasItem("5.02,9.01", "2.02") {
		t.Fatal("hasItem false-positive on '5.02,9.01'")
	}
	if hasItem("", "2.02") {
		t.Fatal("hasItem false-positive on empty")
	}
}
