package sec

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
)

// Filing13F identifies one 13F-HR holdings filing.
type Filing13F struct {
	Accession string // e.g. "0001193125-26-226661"
	Filed     string // filing date, YYYY-MM-DD
	Period    string // period of report (quarter-end), YYYY-MM-DD
}

// Holding is one 13F position, aggregated across the filer's lots for a CUSIP.
// Value is in whole US dollars (SEC's reporting unit since 2023).
type Holding struct {
	Issuer string
	CUSIP  string
	Class  string
	Value  int64
	Shares int64
}

// ThirteenFFilings returns up to n most-recent 13F-HR filings for a CIK, newest
// first. Amendments (13F-HR/A) and notices (13F-NT) are excluded — only the full
// holdings reports carry a complete information table.
func (c *Client) ThirteenFFilings(ctx context.Context, cik, n int) ([]Filing13F, error) {
	url := fmt.Sprintf("%s/submissions/CIK%010d.json", c.dataBase, cik)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Filings struct {
			Recent struct {
				Form       []string `json:"form"`
				Accession  []string `json:"accessionNumber"`
				FilingDate []string `json:"filingDate"`
				ReportDate []string `json:"reportDate"`
			} `json:"recent"`
		} `json:"filings"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sec: 13f submissions %d decode: %w", cik, err)
	}
	r := resp.Filings.Recent
	var out []Filing13F
	for i, f := range r.Form {
		if f != "13F-HR" {
			continue
		}
		out = append(out, Filing13F{
			Accession: r.Accession[i],
			Filed:     idx(r.FilingDate, i),
			Period:    idx(r.ReportDate, i),
		})
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

func idx(s []string, i int) string {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return ""
}

// Holdings fetches and parses the information table of a 13F-HR filing, returning
// positions aggregated by CUSIP (a filer commonly splits one holding across
// managers/lots). Value is whole dollars.
func (c *Client) Holdings(ctx context.Context, cik int, accession string) ([]Holding, error) {
	accNo := strings.ReplaceAll(accession, "-", "")
	// The information table is the filing's .xml that isn't the cover page
	// (primary_doc.xml) — its name varies (e.g. "53405.xml"), so discover it
	// from the folder index.
	idxURL := fmt.Sprintf("%s/Archives/edgar/data/%d/%s/index.json", c.archiveBase, cik, accNo)
	idxBody, err := c.get(ctx, idxURL)
	if err != nil {
		return nil, err
	}
	var index struct {
		Directory struct {
			Item []struct {
				Name string `json:"name"`
			} `json:"item"`
		} `json:"directory"`
	}
	if err := json.Unmarshal(idxBody, &index); err != nil {
		return nil, fmt.Errorf("sec: 13f index %s decode: %w", accession, err)
	}
	infoFile := ""
	for _, it := range index.Directory.Item {
		low := strings.ToLower(it.Name)
		if strings.HasSuffix(low, ".xml") && low != "primary_doc.xml" {
			infoFile = it.Name
			break
		}
	}
	if infoFile == "" {
		return nil, fmt.Errorf("sec: 13f %s: no information table xml", accession)
	}
	xmlBody, err := c.get(ctx, fmt.Sprintf("%s/Archives/edgar/data/%d/%s/%s", c.archiveBase, cik, accNo, infoFile))
	if err != nil {
		return nil, err
	}
	return parseInfoTable(xmlBody)
}

// parseInfoTable decodes the 13F information-table XML and aggregates rows by
// CUSIP. It is namespace-agnostic: Go's xml decoder matches local element names,
// so the varying default namespace on <informationTable> doesn't matter.
func parseInfoTable(body []byte) ([]Holding, error) {
	var doc struct {
		Rows []struct {
			Issuer string `xml:"nameOfIssuer"`
			Class  string `xml:"titleOfClass"`
			CUSIP  string `xml:"cusip"`
			Value  int64  `xml:"value"`
			Shrs   struct {
				Amt  int64  `xml:"sshPrnamt"`
				Type string `xml:"sshPrnamtType"`
			} `xml:"shrsOrPrnAmt"`
		} `xml:"infoTable"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("sec: 13f infotable decode: %w", err)
	}
	byCUSIP := map[string]*Holding{}
	var order []string
	for _, r := range doc.Rows {
		cusip := strings.ToUpper(strings.TrimSpace(r.CUSIP))
		if cusip == "" {
			continue
		}
		h := byCUSIP[cusip]
		if h == nil {
			h = &Holding{Issuer: strings.TrimSpace(r.Issuer), CUSIP: cusip, Class: strings.TrimSpace(r.Class)}
			byCUSIP[cusip] = h
			order = append(order, cusip)
		}
		h.Value += r.Value
		// Count share amounts only (SH); skip principal amounts (PRN = bonds).
		if r.Shrs.Type == "SH" || r.Shrs.Type == "" {
			h.Shares += r.Shrs.Amt
		}
	}
	out := make([]Holding, 0, len(order))
	for _, cusip := range order {
		out = append(out, *byCUSIP[cusip])
	}
	return out, nil
}
