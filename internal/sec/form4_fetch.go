package sec

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Form4Ref identifies a Form 4 filing listed in the SEC daily index.
type Form4Ref struct {
	CIK       int
	Company   string
	Accession string // e.g. "0001225208-25-003785"
}

// DailyForm4 returns the Form 4 filings in the SEC daily index for date.
// Ownership forms filed after ~10pm ET disseminate the next business day, so
// callers should scan yesterday + today and dedupe by accession.
func (c *Client) DailyForm4(ctx context.Context, date time.Time) ([]Form4Ref, error) {
	q := (int(date.Month())-1)/3 + 1
	url := fmt.Sprintf("%s/Archives/edgar/daily-index/%d/QTR%d/form.%s.idx",
		c.archiveBase, date.Year(), q, date.Format("20060102"))
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseFormIndex(body), nil
}

// parseFormIndex extracts Form 4 rows from a form.idx (whitespace-aligned:
// Form Type | Company | CIK | Date Filed | File Name). Header/separator lines
// are skipped naturally because their last field is not an edgar/data path.
func parseFormIndex(body []byte) []Form4Ref {
	var out []Form4Ref
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 || fields[0] != "4" { // exact "4" excludes "4/A" amendments
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
		out = append(out, Form4Ref{
			CIK:       cik,
			Company:   strings.Join(fields[1:len(fields)-3], " "),
			Accession: strings.TrimSuffix(base, ".txt"),
		})
	}
	return out
}

// FetchForm4 resolves a filing's primary ownership XML (whose document name
// varies) by reading the filing folder index, then parses the first XML that
// yields an issuer ticker.
func (c *Client) FetchForm4(ctx context.Context, ref Form4Ref) (Form4, error) {
	accNo := strings.ReplaceAll(ref.Accession, "-", "")
	folder := fmt.Sprintf("%s/Archives/edgar/data/%d/%s", c.archiveBase, ref.CIK, accNo)
	idx, err := c.get(ctx, folder+"/index.json")
	if err != nil {
		return Form4{}, err
	}
	var dir struct {
		Directory struct {
			Item []struct {
				Name string `json:"name"`
			} `json:"item"`
		} `json:"directory"`
	}
	if err := json.Unmarshal(idx, &dir); err != nil {
		return Form4{}, fmt.Errorf("sec: folder index %s: %w", ref.Accession, err)
	}

	var lastErr error
	for _, it := range dir.Directory.Item {
		if !strings.HasSuffix(strings.ToLower(it.Name), ".xml") {
			continue
		}
		body, err := c.get(ctx, folder+"/"+it.Name)
		if err != nil {
			lastErr = err
			continue
		}
		f, err := ParseForm4(body)
		if err != nil {
			lastErr = err
			continue
		}
		if f.Ticker != "" {
			return f, nil
		}
	}
	if lastErr != nil {
		return Form4{}, lastErr
	}
	return Form4{}, fmt.Errorf("sec: no ownership xml in %s", ref.Accession)
}
