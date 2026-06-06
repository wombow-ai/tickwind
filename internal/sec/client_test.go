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
