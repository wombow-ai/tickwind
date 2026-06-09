package sec

import "testing"

func TestParseFiler(t *testing.T) {
	// EDGAR full-submission SGML header: SUBJECT COMPANY block first (the issuer),
	// then FILED BY (the reporting institution). parseFiler must return the latter.
	header := []byte("<SEC-HEADER>\n" +
		"ACCESSION NUMBER:\t\t0001104659-26-071337\n" +
		"SUBJECT COMPANY:\t\n" +
		"\tCOMPANY DATA:\t\n" +
		"\t\tCOMPANY CONFORMED NAME:\t\t\tGENCO SHIPPING & TRADING LTD\n" +
		"\t\tCENTRAL INDEX KEY:\t\t\t0001326200\n" +
		"FILED BY:\t\n" +
		"\tCOMPANY DATA:\t\n" +
		"\t\tCOMPANY CONFORMED NAME:\t\t\tCENTERBRIDGE PARTNERS LP\n" +
		"\t\tCENTRAL INDEX KEY:\t\t\t0001234567\n")
	if got := parseFiler(header); got != "CENTERBRIDGE PARTNERS LP" {
		t.Errorf("parseFiler = %q, want CENTERBRIDGE PARTNERS LP", got)
	}
	// No FILED BY block → empty.
	if got := parseFiler([]byte("SUBJECT COMPANY:\n\tCOMPANY CONFORMED NAME:\tACME\n")); got != "" {
		t.Errorf("parseFiler(no filed-by) = %q, want empty", got)
	}
}

func TestParseOwnershipIndex(t *testing.T) {
	// A daily form.idx slice (whitespace-aligned), mixing 13D/13G/amendments, a
	// Form 4 (must be ignored), and header/separator lines.
	idx := []byte(`Description:           Daily Index of EDGAR Dissemination Feed
Form Type   Company Name                                     CIK         Date Filed  File Name
---------------------------------------------------------------------------------------------
4           SOME INSIDER                                     0001234567  2026-06-09  edgar/data/1234567/0001234567-26-000001.txt
SC 13D      ACME ROBOTICS INC                                0000111111  2026-06-09  edgar/data/111111/0000111111-26-000010.txt
SC 13G      WIDGET CORP                                      0000222222  2026-06-09  edgar/data/222222/0000222222-26-000020.txt
SC 13D/A    ACME ROBOTICS INC                                0000111111  2026-06-09  edgar/data/111111/0000111111-26-000011.txt
SC 13G/A    BIG INDEX FUND TARGET CO                         0000333333  2026-06-09  edgar/data/333333/0000333333-26-000030.txt
`)
	refs := parseOwnershipIndex(idx)
	if len(refs) != 4 {
		t.Fatalf("got %d ownership refs, want 4: %+v", len(refs), refs)
	}

	first := refs[0]
	if first.FormType != "SC 13D" || first.CIK != 111111 || first.Company != "ACME ROBOTICS INC" {
		t.Errorf("first = %+v, want SC 13D / 111111 / ACME ROBOTICS INC", first)
	}
	if first.Accession != "0000111111-26-000010" {
		t.Errorf("accession = %q, want 0000111111-26-000010", first.Accession)
	}
	if !first.Activist {
		t.Error("SC 13D should be marked activist")
	}

	// 13G is passive.
	var g *OwnershipRef
	for i := range refs {
		if refs[i].FormType == "SC 13G" {
			g = &refs[i]
		}
	}
	if g == nil {
		t.Fatal("SC 13G not parsed")
	}
	if g.Activist {
		t.Error("SC 13G should be passive (not activist)")
	}
	if g.Company != "WIDGET CORP" {
		t.Errorf("13G company = %q, want WIDGET CORP", g.Company)
	}

	// Amendments counted, Form 4 excluded.
	var nD, nG int
	for _, r := range refs {
		switch {
		case r.FormType == "SC 13D" || r.FormType == "SC 13D/A":
			nD++
		case r.FormType == "SC 13G" || r.FormType == "SC 13G/A":
			nG++
		}
	}
	if nD != 2 || nG != 2 {
		t.Errorf("counts D=%d G=%d, want 2/2", nD, nG)
	}
}
