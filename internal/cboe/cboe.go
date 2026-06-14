// Package cboe is a minimal client for Cboe's public delayed-options endpoint,
// used for the per-stock options overview (put/call ratio, max pain, OI
// leaders). The data is ~15 minutes delayed and the CDN needs no key; this is
// an owner-authorized free-display source (never resold, always labeled
// "delayed · Cboe"), same policy as the other gray quote sources.
package cboe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"
)

const baseURL = "https://cdn.cboe.com/api/global/delayed_quotes/options/"

// Client fetches delayed option chains from Cboe's CDN.
type Client struct {
	hc   *http.Client
	base string // overridable in tests
}

// New returns a ready Client.
func New() *Client {
	return &Client{hc: &http.Client{Timeout: 25 * time.Second}, base: baseURL}
}

// Contract is one option contract, decoded from its OCC symbol + the row's
// open interest / volume / implied vol.
type Contract struct {
	Symbol string  `json:"contract"`
	Type   string  `json:"type"`   // "C" | "P"
	Strike float64 `json:"strike"` // dollars
	Expiry string  `json:"expiry"` // YYYY-MM-DD
	OI     int64   `json:"oi"`
	Volume int64   `json:"volume"`
	IV     float64 `json:"iv"`
}

// Chain is a decoded option chain plus the feed timestamp.
type Chain struct {
	Contracts []Contract
	At        time.Time
}

// Options fetches and decodes the delayed chain for a ticker. ok=false when the
// symbol has no listed options (the CDN 404s).
func (c *Client) Options(ctx context.Context, ticker string) (Chain, bool, error) {
	u := c.base + ticker + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Chain{}, false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Tickwind/0.1)")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return Chain{}, false, fmt.Errorf("cboe options %s: %w", ticker, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Chain{}, false, nil // no options for this symbol
	}
	if resp.StatusCode != http.StatusOK {
		return Chain{}, false, fmt.Errorf("cboe options %s: %s", ticker, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return Chain{}, false, err
	}
	return parseOptions(body)
}

type optionsResp struct {
	Timestamp string `json:"timestamp"`
	Data      struct {
		Options []struct {
			Option       string  `json:"option"`
			IV           float64 `json:"iv"`
			OpenInterest float64 `json:"open_interest"`
			Volume       float64 `json:"volume"`
		} `json:"options"`
	} `json:"data"`
}

// parseOptions decodes the CDN body into a Chain. ok=false when no contracts.
func parseOptions(body []byte) (Chain, bool, error) {
	var r optionsResp
	if err := json.Unmarshal(body, &r); err != nil {
		return Chain{}, false, fmt.Errorf("cboe: parse options: %w", err)
	}
	out := make([]Contract, 0, len(r.Data.Options))
	for _, o := range r.Data.Options {
		typ, strike, expiry, ok := decodeOCC(o.Option)
		if !ok {
			continue
		}
		out = append(out, Contract{
			Symbol: o.Option, Type: typ, Strike: strike, Expiry: expiry,
			OI: int64(o.OpenInterest), Volume: int64(o.Volume), IV: o.IV,
		})
	}
	if len(out) == 0 {
		return Chain{}, false, nil
	}
	at, _ := time.Parse("2006-01-02 15:04:05", r.Timestamp) // best-effort; zero on failure
	return Chain{Contracts: out, At: at.UTC()}, true, nil
}

// decodeOCC splits an OCC option symbol — ROOT + YYMMDD + C/P + strike×1000
// (8 digits) — into type, strike (dollars) and expiry (YYYY-MM-DD). The fixed
// 15-char suffix is parsed from the right so multi-char roots work.
func decodeOCC(sym string) (typ string, strike float64, expiry string, ok bool) {
	if len(sym) < 16 { // need at least 1-char root + 15-char suffix
		return "", 0, "", false
	}
	suf := sym[len(sym)-15:]
	yy, mm, dd := suf[0:2], suf[2:4], suf[4:6]
	typ = suf[6:7]
	if typ != "C" && typ != "P" {
		return "", 0, "", false
	}
	strikeMills, err := strconv.ParseInt(suf[7:15], 10, 64)
	if err != nil {
		return "", 0, "", false
	}
	return typ, float64(strikeMills) / 1000, "20" + yy + "-" + mm + "-" + dd, true
}

// PutCallRatio returns the put/call ratio by volume and by open interest
// (puts ÷ calls). A ratio > 1 leans bearish/hedged. Zero when no calls.
func PutCallRatio(cs []Contract) (byVolume, byOI float64) {
	var cv, pv, co, po int64
	for _, c := range cs {
		if c.Type == "P" {
			pv += c.Volume
			po += c.OI
		} else {
			cv += c.Volume
			co += c.OI
		}
	}
	if cv > 0 {
		byVolume = float64(pv) / float64(cv)
	}
	if co > 0 {
		byOI = float64(po) / float64(co)
	}
	return byVolume, byOI
}

// NearestExpiry returns the soonest expiry (>= today) that has open interest,
// or the soonest expiry overall if none are in the future. "" when empty.
func NearestExpiry(cs []Contract, today string) string {
	best := ""
	for _, c := range cs {
		if c.OI == 0 || c.Expiry < today {
			continue
		}
		if best == "" || c.Expiry < best {
			best = c.Expiry
		}
	}
	return best
}

// minMaxPainStrikes is the minimum number of DISTINCT strikes carrying open
// interest at the chosen expiry before a non-zero Max Pain is reported. Below
// this, the "pain valley" is degenerate (a single strike trivially minimizes
// total pain at 0, and two strikes give a meaningless coin-flip), so we report
// nothing rather than an authoritative-looking but vacuous magnet — the same
// insufficient-not-wrong stance as the stale-shares guard and the all-or-nothing
// Altman-Z/Piotroski scores. 3 is the smallest count at which an interior strike
// can win on genuine OI weighting (a real call-vs-put balance, not an artifact).
const minMaxPainStrikes = 3

// MaxPain returns the strike that minimizes total in-the-money option value to
// holders for the given expiry (the classic "max pain" magnet), using OI. It
// returns 0 when the expiry has fewer than minMaxPainStrikes distinct strikes
// with open interest (insufficient to compute a meaningful magnet) — callers
// gate on >0 and omit the fact entirely in that case.
//
// Strike iteration is deterministic: candidates are sorted ascending and ties on
// minimal total pain (a common flat valley on balanced chains) resolve to the
// LOWER strike, so identical input always yields the identical Max Pain. (The
// underlying spot is not available in this code path — the Cboe chain carries no
// spot — so a spot-distance tie-break isn't possible here; the lower strike is
// the deterministic, documented choice.)
func MaxPain(cs []Contract, expiry string) float64 {
	strikeSet := map[float64]bool{}
	for _, c := range cs {
		if c.Expiry == expiry && c.OI > 0 {
			strikeSet[c.Strike] = true
		}
	}
	if len(strikeSet) < minMaxPainStrikes {
		return 0
	}
	strikes := make([]float64, 0, len(strikeSet))
	for k := range strikeSet {
		strikes = append(strikes, k)
	}
	sort.Float64s(strikes) // deterministic iteration; ties resolve to the lower strike
	bestK, bestPain := 0.0, -1.0
	for _, k := range strikes {
		var pain float64
		for _, c := range cs {
			if c.Expiry != expiry || c.OI == 0 {
				continue
			}
			if c.Type == "C" && k > c.Strike {
				pain += float64(c.OI) * (k - c.Strike)
			} else if c.Type == "P" && k < c.Strike {
				pain += float64(c.OI) * (c.Strike - k)
			}
		}
		// Strict improvement only: because strikes ascend, the first (lowest)
		// strike that hits the minimum wins and later ties never displace it.
		if bestPain < 0 || pain < bestPain {
			bestPain, bestK = pain, k
		}
	}
	return bestK
}

// OITop returns the n contracts with the highest open interest, OI-descending.
func OITop(cs []Contract, n int) []Contract {
	out := append([]Contract(nil), cs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].OI != out[j].OI {
			return out[i].OI > out[j].OI
		}
		return out[i].Volume > out[j].Volume
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
