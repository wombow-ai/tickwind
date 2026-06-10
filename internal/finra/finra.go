// Package finra is a minimal anonymous client for FINRA's public Query API,
// used for the consolidated equity short-interest dataset (the squeeze radar).
// No key or registration: the dataset is public and anonymous access is
// rate-limited but ample for our twice-monthly data refreshed daily.
//
// API shape (verified live 2026-06-10): POST
// /data/group/otcMarket/name/consolidatedShortInterest with a JSON body of
// {limit, offset, compareFilters}. settlementDate is a PARTITION key — sorting
// is rejected unless it is pinned with an EQUAL filter, so the ingestor first
// probes candidate settlement dates (newest first) and then pages the matching
// partition.
package finra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const datasetURL = "https://api.finra.org/data/group/otcMarket/name/consolidatedShortInterest"

// Client queries FINRA anonymously.
type Client struct {
	hc   *http.Client
	base string // overridable in tests
}

// New returns a ready Client.
func New() *Client {
	return &Client{hc: &http.Client{Timeout: 25 * time.Second}, base: datasetURL}
}

// ShortInterest is one symbol's row for one settlement period.
type ShortInterest struct {
	Symbol         string  `json:"symbol"`
	Name           string  `json:"name,omitempty"`
	Market         string  `json:"market,omitempty"` // NYSE | NNM | ...
	SettlementDate string  `json:"settlement_date"`  // YYYY-MM-DD
	ShortQty       int64   `json:"short_qty"`
	PrevShortQty   int64   `json:"prev_short_qty,omitempty"`
	AvgDailyVolume int64   `json:"avg_daily_volume,omitempty"`
	DaysToCover    float64 `json:"days_to_cover"`
	ChangePct      float64 `json:"change_pct"`
}

// row mirrors the dataset's field names.
type row struct {
	SymbolCode                    string  `json:"symbolCode"`
	IssueName                     string  `json:"issueName"`
	MarketClassCode               string  `json:"marketClassCode"`
	SettlementDate                string  `json:"settlementDate"`
	CurrentShortPositionQuantity  float64 `json:"currentShortPositionQuantity"`
	PreviousShortPositionQuantity float64 `json:"previousShortPositionQuantity"`
	AverageDailyVolumeQuantity    float64 `json:"averageDailyVolumeQuantity"`
	DaysToCoverQuantity           float64 `json:"daysToCoverQuantity"`
	ChangePercent                 float64 `json:"changePercent"`
}

type apiError struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

type compareFilter struct {
	FieldName   string `json:"fieldName"`
	FieldValue  string `json:"fieldValue"`
	CompareType string `json:"compareType"`
}

type dataRequest struct {
	Limit          int             `json:"limit"`
	Offset         int             `json:"offset"`
	CompareFilters []compareFilter `json:"compareFilters,omitempty"`
}

// Rows fetches one page of the short-interest partition for an exact
// settlement date (YYYY-MM-DD). An empty page means the period either isn't
// published yet or the offset ran past the end.
func (c *Client) Rows(ctx context.Context, settlementDate string, limit, offset int) ([]ShortInterest, error) {
	body, err := json.Marshal(dataRequest{
		Limit:  limit,
		Offset: offset,
		CompareFilters: []compareFilter{
			{FieldName: "settlementDate", FieldValue: settlementDate, CompareType: "EQUAL"},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Tickwind/0.1 (contact@tickwind.com)")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var ae apiError
		if json.Unmarshal(raw, &ae) == nil && ae.Message != "" {
			return nil, fmt.Errorf("finra: %s: %s", resp.Status, ae.Message)
		}
		return nil, fmt.Errorf("finra: %s", resp.Status)
	}
	return parseRows(raw)
}

// parseRows decodes a dataset response (a bare JSON array of rows).
func parseRows(raw []byte) ([]ShortInterest, error) {
	var rows []row
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("finra: parse rows: %w", err)
	}
	out := make([]ShortInterest, 0, len(rows))
	for _, r := range rows {
		if r.SymbolCode == "" || r.SettlementDate == "" {
			continue
		}
		out = append(out, ShortInterest{
			Symbol:         r.SymbolCode,
			Name:           r.IssueName,
			Market:         r.MarketClassCode,
			SettlementDate: r.SettlementDate,
			ShortQty:       int64(r.CurrentShortPositionQuantity),
			PrevShortQty:   int64(r.PreviousShortPositionQuantity),
			AvgDailyVolume: int64(r.AverageDailyVolumeQuantity),
			DaysToCover:    r.DaysToCoverQuantity,
			ChangePct:      r.ChangePercent,
		})
	}
	return out, nil
}

// LatestSettlementCandidates returns plausible settlement dates to probe,
// newest first. FINRA settles short interest twice a month — on the 15th and
// on the last day of the month, each rolled BACK to a business day when it
// lands on a weekend (we also emit a couple of extra preceding business days
// to ride out market holidays). Publication lags settlement by ~9–11 days, so
// callers probe until the first date that returns rows.
func LatestSettlementCandidates(today time.Time, months int) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(d time.Time) {
		// Roll back to a business day, then add it and 2 more business days
		// before it (holiday slack), newest first.
		for n := 0; n < 3; n++ {
			for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				d = d.AddDate(0, 0, -1)
			}
			s := d.Format("2006-01-02")
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
			d = d.AddDate(0, 0, -1)
		}
	}
	y, m, _ := today.Date()
	loc := today.Location()
	for i := 0; i < months; i++ {
		first := time.Date(y, m, 1, 0, 0, 0, 0, loc).AddDate(0, -i, 0)
		eom := first.AddDate(0, 1, -1) // last day of that month
		mid := time.Date(first.Year(), first.Month(), 15, 0, 0, 0, 0, loc)
		// Within a month the EOM anchor is newer than the mid anchor.
		for _, anchor := range []time.Time{eom, mid} {
			if anchor.After(today) {
				continue // settlement in the future can't be published
			}
			add(anchor)
		}
	}
	return out
}
