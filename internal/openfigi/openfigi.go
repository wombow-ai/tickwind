// Package openfigi maps CUSIPs to US tickers via OpenFIGI's free mapping API
// (https://www.openfigi.com/api). No API key is required — keyless access allows
// 25 requests/min with up to 10 jobs per request, which is ample for the 13F
// whale-holdings use (a few hundred CUSIPs, mapped once and cached for the
// process lifetime since CUSIP→ticker assignments don't change).
package openfigi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	mapURL = "https://api.openfigi.com/v3/mapping"
	// Keyless caps: 10 jobs/request, 25 requests/min. A key lifts these to 100 jobs/request and
	// 25 requests/6s, so a keyed client sends far fewer requests with a much smaller pacing gap.
	keylessBatch = 10
	keylessGap   = 2500 * time.Millisecond
	keyedBatch   = 100
	keyedGap     = 300 * time.Millisecond
)

// Client maps CUSIPs to tickers, caching results permanently in memory.
type Client struct {
	http      *http.Client
	url       string
	apiKey    string // optional; "" = keyless
	batchSize int    // jobs per request (key-dependent)
	batchGap  time.Duration

	mu    sync.Mutex
	cache map[string]string // CUSIP → ticker ("" = looked up, no US equity match)
}

// New builds a client. apiKey may be "" for keyless access; a key raises the batch size + rate.
func New(apiKey string) *Client {
	bs, gap := keylessBatch, keylessGap
	if apiKey != "" {
		bs, gap = keyedBatch, keyedGap
	}
	return &Client{
		http:      &http.Client{Timeout: 20 * time.Second},
		url:       mapURL,
		apiKey:    apiKey,
		batchSize: bs,
		batchGap:  gap,
		cache:     map[string]string{},
	}
}

// Map resolves CUSIPs to US tickers. Already-cached CUSIPs are served from
// memory; the rest are fetched in batches. The result holds only CUSIPs that map
// to a US equity (bonds, options, foreign and delisted issues are absent). A
// fetch error returns whatever has resolved so far plus the error.
func (c *Client) Map(ctx context.Context, cusips []string) (map[string]string, error) {
	out := map[string]string{}
	var todo []string
	seen := map[string]bool{}

	c.mu.Lock()
	for _, raw := range cusips {
		cu := strings.ToUpper(strings.TrimSpace(raw))
		if cu == "" || seen[cu] {
			continue
		}
		seen[cu] = true
		if t, ok := c.cache[cu]; ok {
			if t != "" {
				out[cu] = t
			}
			continue
		}
		todo = append(todo, cu)
	}
	c.mu.Unlock()

	for i := 0; i < len(todo); i += c.batchSize {
		if i > 0 {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(c.batchGap):
			}
		}
		end := i + c.batchSize
		if end > len(todo) {
			end = len(todo)
		}
		if err := c.mapBatch(ctx, todo[i:end], out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *Client) mapBatch(ctx context.Context, cusips []string, out map[string]string) error {
	type job struct {
		IDType   string `json:"idType"`
		IDValue  string `json:"idValue"`
		ExchCode string `json:"exchCode"`
	}
	jobs := make([]job, len(cusips))
	for i, cu := range cusips {
		jobs[i] = job{IDType: "ID_CUSIP", IDValue: cu, ExchCode: "US"}
	}
	payload, err := json.Marshal(jobs)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-OPENFIGI-APIKEY", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("openfigi: map: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openfigi: map: %s", resp.Status)
	}
	// Response is an array aligned with the request: each entry has either "data"
	// (matches) or "warning" (no match).
	var results []struct {
		Data []struct {
			Ticker string `json:"ticker"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return fmt.Errorf("openfigi: decode: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, cu := range cusips {
		ticker := ""
		if i < len(results) && len(results[i].Data) > 0 {
			ticker = strings.TrimSpace(results[i].Data[0].Ticker)
		}
		c.cache[cu] = ticker // cache the miss too, so we don't re-request it
		if ticker != "" {
			out[cu] = ticker
		}
	}
	return nil
}
