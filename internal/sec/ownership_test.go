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
SCHEDULE 13D   NEWFORM ACTIVIST CO                           0000444444  20260618    edgar/data/444444/0000444444-26-000040.txt
SCHEDULE 13G/A NEWFORM PASSIVE CO                            0000555555  20260618    edgar/data/555555/0000555555-26-000050.txt
`)
	refs := parseOwnershipIndex(idx)
	if len(refs) != 6 {
		t.Fatalf("got %d ownership refs, want 6: %+v", len(refs), refs)
	}

	// The post-2024 "SCHEDULE 13D" form-type token is parsed (the bug that emptied
	// the board) and normalized to the canonical "SC 13D" FormType.
	var nf *OwnershipRef
	for i := range refs {
		if refs[i].CIK == 444444 {
			nf = &refs[i]
		}
	}
	if nf == nil || nf.FormType != "SC 13D" || nf.Company != "NEWFORM ACTIVIST CO" || !nf.Activist {
		t.Errorf("SCHEDULE 13D row = %+v, want SC 13D / NEWFORM ACTIVIST CO / activist", nf)
	}
	// The compact YYYYMMDD index date is normalized to ISO YYYY-MM-DD.
	if nf != nil && nf.FiledDate != "2026-06-18" {
		t.Errorf("SCHEDULE row FiledDate = %q, want 2026-06-18 (normalized from 20260618)", nf.FiledDate)
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
	if nD != 3 || nG != 3 {
		t.Errorf("counts D=%d G=%d, want 3/3 (incl. the SCHEDULE-form rows)", nD, nG)
	}
}
