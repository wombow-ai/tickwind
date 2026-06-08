package symbols

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Nasdaq Trader's SymbolDirectory files are free, keyless, pipe-delimited, and
// refreshed daily. They cover Nasdaq-listed issues plus NYSE/Arca/American/Cboe/
// IEX listings — the latter is where most ETFs live (e.g. DRAM), which are absent
// from SEC's company_tickers file. https://www.nasdaqtrader.com/trader.aspx?id=symboldirdefs
const (
	nasdaqListedURL = "https://www.nasdaqtrader.com/dynamic/SymDir/nasdaqlisted.txt"
	otherListedURL  = "https://www.nasdaqtrader.com/dynamic/SymDir/otherlisted.txt"
)

// FetchNasdaqTrader downloads both SymbolDirectory files and returns their US
// symbols. Best-effort: if one file fails it still returns the other's symbols;
// it errors only if BOTH fail (so a partial outage never drops the whole feed).
func FetchNasdaqTrader(ctx context.Context, hc *http.Client, userAgent string) ([]Symbol, error) {
	var out []Symbol
	var errs []error
	if data, err := fetchSymDir(ctx, hc, userAgent, nasdaqListedURL); err != nil {
		errs = append(errs, err)
	} else {
		out = append(out, parseNasdaqListed(data)...)
	}
	if data, err := fetchSymDir(ctx, hc, userAgent, otherListedURL); err != nil {
		errs = append(errs, err)
	} else {
		out = append(out, parseOtherListed(data)...)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("symbols: nasdaq trader: %w", errors.Join(errs...))
	}
	return out, nil
}

func fetchSymDir(ctx context.Context, hc *http.Client, userAgent, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("symbols: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("symbols: %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("symbols: read %s: %w", url, err)
	}
	return data, nil
}

// parseNasdaqListed parses nasdaqlisted.txt:
//
//	Symbol|Security Name|Market Category|Test Issue|Financial Status|Round Lot Size|ETF|NextShares
//
// Every row is Nasdaq-listed; test issues are dropped.
func parseNasdaqListed(data []byte) []Symbol {
	var out []Symbol
	forEachDataRow(data, func(c []string) {
		if len(c) < 7 || strings.EqualFold(strings.TrimSpace(c[3]), "Y") { // test issue
			return
		}
		t, n := strings.TrimSpace(c[0]), strings.TrimSpace(c[1])
		if t == "" || n == "" {
			return
		}
		out = append(out, Symbol{Ticker: t, Name: n, Exchange: "Nasdaq", Country: "US"})
	})
	return out
}

// parseOtherListed parses otherlisted.txt (NYSE / Arca / American / Cboe / IEX):
//
//	ACT Symbol|Security Name|Exchange|CQS Symbol|ETF|Round Lot Size|Test Issue|NASDAQ Symbol
//
// Test issues are dropped; the single-letter Exchange code is mapped to a name.
func parseOtherListed(data []byte) []Symbol {
	var out []Symbol
	forEachDataRow(data, func(c []string) {
		if len(c) < 7 || strings.EqualFold(strings.TrimSpace(c[6]), "Y") { // test issue
			return
		}
		t, n := strings.TrimSpace(c[0]), strings.TrimSpace(c[1])
		if t == "" || n == "" {
			return
		}
		out = append(out, Symbol{Ticker: t, Name: n, Exchange: otherExch(strings.TrimSpace(c[2])), Country: "US"})
	})
	return out
}

// forEachDataRow scans a Nasdaq Trader pipe file, calling fn with the split
// columns of each data row — skipping the header line and the trailing
// "File Creation Time: …" line.
func forEachDataRow(data []byte, fn func(cols []string)) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first { // header row
			first = false
			continue
		}
		if line == "" || strings.HasPrefix(line, "File Creation Time") {
			continue
		}
		fn(strings.Split(line, "|"))
	}
}

// otherExch maps an otherlisted.txt single-letter Exchange code to a readable
// name (unknown codes pass through unchanged).
func otherExch(code string) string {
	switch code {
	case "N":
		return "NYSE"
	case "P":
		return "NYSE Arca"
	case "A":
		return "NYSE American"
	case "Z":
		return "Cboe BZX"
	case "V":
		return "IEX"
	case "M":
		return "NYSE Chicago"
	default:
		return code
	}
}
