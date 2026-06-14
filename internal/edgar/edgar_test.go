package edgar

import "testing"

// TestLookupCanonicalClassShare is the FIX-2 regression: SEC's ticker directory
// keys class / preferred shares with a HYPHEN ("BRK-B"), but Tickwind's canonical
// form — used by the price universe, the pSEO /stock/BRK.B URLs, aliases, and the
// symbols index — is the DOT form ("BRK.B"). Before the fix, lookup("BRK.B") missed
// the hyphen-keyed CIK, so Fundamentals / RecentFilings / MaterialEvents /
// InsiderActivity / the research path ALL returned not-found for the canonical dot
// form (live-confirmed: /v1/stocks/BRK.B/fundamentals → 404). lookup now retries
// the canonical (dot) and hyphen variants, so EITHER form resolves to the CIK.
//
// This is white-box (seeds tickerMap directly) so it needs no network: every
// data-fetching method (Fundamentals, RecentFilings, MaterialEvents,
// InsiderActivity, research) resolves the CIK through lookup first, so proving the
// round-trip BRK.B → CIK here proves they all now reach the companyfacts /
// submissions fetch instead of erroring at resolution.
func TestLookupCanonicalClassShare(t *testing.T) {
	c := New("test-agent")
	// Seed the directory the way SEC actually keys it: hyphen form for the class
	// share, plus a plain ticker and a preferred series.
	c.tickerMap = map[string]tickerInfo{
		"BRK-B":  {CIK: "0001067983", Title: "BERKSHIRE HATHAWAY INC"},
		"AAPL":   {CIK: "0000320193", Title: "Apple Inc."},
		"USB-PA": {CIK: "0000036104", Title: "U.S. BANCORP"},
	}

	tests := []struct {
		name    string
		ticker  string
		wantCIK string
		wantErr bool
	}{
		{"canonical dot resolves hyphen-keyed CIK", "BRK.B", "0001067983", false},
		{"original hyphen form still resolves", "BRK-B", "0001067983", false},
		{"lowercase canonical dot resolves", "brk.b", "0001067983", false},
		{"plain ticker unaffected", "AAPL", "0000320193", false},
		{"preferred dot form resolves hyphen CIK", "USB.PA", "0000036104", false},
		{"genuinely unknown ticker still errors", "ZZZZ", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info, err := c.lookup(t.Context(), tc.ticker)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("lookup(%q) = %+v, nil; want error", tc.ticker, info)
				}
				return
			}
			if err != nil {
				t.Fatalf("lookup(%q) error: %v (BRK.B-class would lose all EDGAR data)", tc.ticker, err)
			}
			if info.CIK != tc.wantCIK {
				t.Errorf("lookup(%q).CIK = %q, want %q", tc.ticker, info.CIK, tc.wantCIK)
			}
		})
	}
}
