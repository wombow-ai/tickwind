package symbols

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// secURL is SEC's public-domain ticker+name+exchange directory (US listings).
const secURL = "https://www.sec.gov/files/company_tickers_exchange.json"

// secResp matches the file shape: a fields header + row-oriented data, e.g.
// {"fields":["cik","name","ticker","exchange"],"data":[[1045810,"NVIDIA CORP","NVDA","Nasdaq"],...]}.
type secResp struct {
	Fields []string `json:"fields"`
	Data   [][]any  `json:"data"`
}

// FetchUS downloads the SEC directory and returns the US symbols. userAgent must
// be a descriptive UA with contact info (SEC requires it and caps clients at
// ~10 req/s — we fetch this once a day, so we stay far under).
func FetchUS(ctx context.Context, hc *http.Client, userAgent string) ([]Symbol, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, secURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("symbols: fetch sec: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("symbols: sec: %s", resp.Status)
	}

	var body secResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("symbols: decode sec: %w", err)
	}

	// Resolve columns by field name (defensive against reordering).
	col := make(map[string]int, len(body.Fields))
	for i, f := range body.Fields {
		col[strings.ToLower(f)] = i
	}
	nameCol, tickerCol, exchCol := col["name"], col["ticker"], col["exchange"]

	out := make([]Symbol, 0, len(body.Data))
	for _, row := range body.Data {
		name := cell(row, nameCol)
		ticker := cell(row, tickerCol)
		if name == "" || ticker == "" {
			continue
		}
		out = append(out, Symbol{
			Ticker:   ticker,
			Name:     name,
			Exchange: cell(row, exchCol),
			Country:  "US",
		})
	}
	return out, nil
}

// cell returns the string at column i of a data row, or "" if absent/non-string.
func cell(row []any, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	s, _ := row[i].(string)
	return strings.TrimSpace(s)
}
