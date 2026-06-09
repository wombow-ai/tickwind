package sec

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OwnershipRef identifies a beneficial-ownership filing (Schedule 13D / 13G) in
// the SEC daily index. 13D = an active/activist >5% stake (higher signal); 13G =
// a passive holding (e.g. the BlackRock/Vanguard/State Street index giants).
// FormType is one of "SC 13D", "SC 13D/A", "SC 13G", "SC 13G/A". Company is the
// subject issuer as listed in the index; the filer (the institution) lives in the
// filing document and is resolved separately.
type OwnershipRef struct {
	FormType  string `json:"form_type"`
	CIK       int    `json:"cik"`
	Company   string `json:"company"`
	Accession string `json:"accession"`
	FiledDate string `json:"filed_date"`      // as listed in the index (YYYYMMDD)
	Activist  bool   `json:"activist"`        // true for 13D (and 13D/A); false for 13G
	Filer     string `json:"filer,omitempty"` // the reporting institution (from the filing header)
}

// DailyBeneficialOwnership returns the Schedule 13D/13G filings in the SEC daily
// index for date, using the same throttled, gzip-aware, UA-bearing client as the
// Form-4 path. Ownership forms disseminate the next business day, so callers
// should scan recent days and dedupe by accession.
func (c *Client) DailyBeneficialOwnership(ctx context.Context, date time.Time) ([]OwnershipRef, error) {
	q := (int(date.Month())-1)/3 + 1
	url := fmt.Sprintf("%s/Archives/edgar/daily-index/%d/QTR%d/form.%s.idx",
		c.archiveBase, date.Year(), q, date.Format("20060102"))
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseOwnershipIndex(body), nil
}

// FetchFiler resolves the reporting institution (the "FILED BY" party) for a
// 13D/13G ref by reading just the SGML header of the full submission. Returns ""
// (no error) when the filer can't be determined.
func (c *Client) FetchFiler(ctx context.Context, ref OwnershipRef) (string, error) {
	url := fmt.Sprintf("%s/Archives/edgar/data/%d/%s.txt", c.archiveBase, ref.CIK, ref.Accession)
	body, err := c.getLimited(ctx, url, 64<<10) // header lives at the top
	if err != nil {
		return "", err
	}
	return parseFiler(body), nil
}

// parseFiler extracts the reporting institution from an EDGAR full-submission
// SGML header: the first "COMPANY CONFORMED NAME:" appearing after a "FILED BY:"
// marker. Pure — unit-tested.
func parseFiler(header []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(header))
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	filedBy := false
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "FILED BY:") {
			filedBy = true
			continue
		}
		if filedBy {
			if i := strings.Index(line, "COMPANY CONFORMED NAME:"); i >= 0 {
				name := strings.TrimSpace(line[i+len("COMPANY CONFORMED NAME:"):])
				return name
			}
		}
	}
	return ""
}

// ownershipForms maps the daily-index form token (the field after "SC") to
// whether it's an activist 13D.
var ownershipForms = map[string]bool{
	"13D": true, "13D/A": true, // activist
	"13G": false, "13G/A": false, // passive
}

// parseOwnershipIndex extracts Schedule 13D/13G rows from a form.idx
// (whitespace-aligned: Form Type | Company | CIK | Date Filed | File Name). The
// form type is two tokens ("SC 13D"), so the company spans fields[2:len-3].
func parseOwnershipIndex(body []byte) []OwnershipRef {
	var out []OwnershipRef
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 6 || fields[0] != "SC" {
			continue
		}
		activist, ok := ownershipForms[fields[1]]
		if !ok {
			continue
		}
		file := fields[len(fields)-1]
		if !strings.HasPrefix(file, "edgar/data/") || !strings.HasSuffix(file, ".txt") {
			continue
		}
		cik, err := strconv.Atoi(fields[len(fields)-3])
		if err != nil {
			continue
		}
		base := file[strings.LastIndex(file, "/")+1:]
		out = append(out, OwnershipRef{
			FormType:  "SC " + fields[1],
			CIK:       cik,
			Company:   strings.Join(fields[2:len(fields)-3], " "),
			Accession: strings.TrimSuffix(base, ".txt"),
			FiledDate: fields[len(fields)-2],
			Activist:  activist,
		})
	}
	return out
}
