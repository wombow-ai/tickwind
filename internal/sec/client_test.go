package sec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent (SEC requires one)")
		}
		if !strings.Contains(r.URL.Path, "EntityCommonStockSharesOutstanding") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"cik":320193,"val":15000000000},{"cik":1750,"val":35427056}]}`))
	}))
	defer srv.Close()

	c := New("Tickwind (test@tickwind.com)")
	c.dataBase = srv.URL
	c.minGap = 0

	m, err := c.Shares(context.Background(), 2024, 1)
	if err != nil {
		t.Fatalf("Shares: %v", err)
	}
	if m[320193] != 15000000000 || m[1750] != 35427056 {
		t.Errorf("shares = %v", m)
	}
}

// TestSharesFrameGuards verifies the anti-hallucination guards on the shares
// frame fetch: a fresh, plausible row is kept; a frozen-ancient row (its "end"
// instant far older than the frame's quarter) is rejected; and 0/1-share garbage
// is dropped — so a candidate is left off the board rather than gated by a wrong
// market cap. It also confirms Shares hits dei and SharesFallback hits us-gaap.
func TestSharesFrameGuards(t *testing.T) {
	// Frame named CY2025Q1I → quarter end 2025-03-31. A row dated 2011-12-31 is
	// ~13 years stale (> sharesFrameStaleAfterDays) and must be dropped.
	const frame = `{"data":[
		{"cik":111,"end":"2025-02-28","val":50000000},
		{"cik":222,"end":"2011-12-31","val":941481},
		{"cik":333,"end":"2025-03-15","val":1},
		{"cik":444,"end":"2025-03-15","val":0}
	]}`

	tests := []struct {
		name    string
		call    func(c *Client) (map[int]int64, error)
		wantSub string // substring the frame URL must contain
	}{
		{"dei", func(c *Client) (map[int]int64, error) { return c.Shares(context.Background(), 2025, 1) },
			"/dei/EntityCommonStockSharesOutstanding/"},
		{"us-gaap-fallback", func(c *Client) (map[int]int64, error) { return c.SharesFallback(context.Background(), 2025, 1) },
			"/us-gaap/CommonStockSharesOutstanding/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, tc.wantSub) {
					t.Errorf("path = %q, want substring %q", r.URL.Path, tc.wantSub)
				}
				_, _ = w.Write([]byte(frame))
			}))
			defer srv.Close()

			c := New("Tickwind (test@tickwind.com)")
			c.dataBase = srv.URL
			c.minGap = 0

			m, err := tc.call(c)
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if m[111] != 50000000 {
				t.Errorf("fresh row: got %d, want 50000000", m[111])
			}
			if _, ok := m[222]; ok {
				t.Errorf("frozen 2011 row should be dropped, got %d", m[222])
			}
			if _, ok := m[333]; ok {
				t.Error("1-share garbage row should be dropped")
			}
			if _, ok := m[444]; ok {
				t.Error("0-share garbage row should be dropped")
			}
		})
	}
}

func TestParseFormIndex(t *testing.T) {
	idx := strings.Join([]string{
		"Description:  Master Index of EDGAR Dissemination Feed by Form Type",
		"Form Type   Company Name                 CIK       Date Filed  File Name",
		"-----------------------------------------------------------------------",
		"4           ABERCROMBIE & FITCH CO /DE/  1018840   20250401    edgar/data/1018840/0001225208-25-003785.txt",
		"4/A         SOME AMENDED FILER INC       222222    20250401    edgar/data/222222/0000000000-25-000001.txt",
		"8-K         BIG CO                       333333    20250401    edgar/data/333333/0000000000-25-000002.txt",
		"4           TINY BIOTECH INC             444444    20250401    edgar/data/444444/0000000000-25-000003.txt",
	}, "\n")

	refs := parseFormIndex([]byte(idx))
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2 (exact form 4 only; 4/A and 8-K excluded)", len(refs))
	}
	if refs[0].CIK != 1018840 || refs[0].Accession != "0001225208-25-003785" {
		t.Errorf("ref0 = %+v", refs[0])
	}
	if refs[0].Company != "ABERCROMBIE & FITCH CO /DE/" {
		t.Errorf("company = %q", refs[0].Company)
	}
	if refs[1].CIK != 444444 {
		t.Errorf("ref1 = %+v", refs[1])
	}
}

func TestFetchForm4(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/index.json"):
			_, _ = w.Write([]byte(`{"directory":{"item":[{"name":"form4.htm"},{"name":"primary_doc.xml"}]}}`))
		case strings.HasSuffix(r.URL.Path, "/primary_doc.xml"):
			_, _ = w.Write([]byte(sampleForm4)) // defined in form4_test.go
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("Tickwind (test@tickwind.com)")
	c.archiveBase = srv.URL
	c.minGap = 0

	f, err := c.FetchForm4(context.Background(), Form4Ref{CIK: 1018840, Accession: "0001225208-25-003785"})
	if err != nil {
		t.Fatalf("FetchForm4: %v", err)
	}
	if f.Ticker != "ACME" {
		t.Errorf("ticker = %q want ACME", f.Ticker)
	}
	if len(f.Buys) != 1 || f.BuyValue() != 125000 {
		t.Errorf("buys = %+v", f.Buys)
	}
}
