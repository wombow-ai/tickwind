// Package market classifies a stock ticker by its listing venue using the
// BASE.SUFFIX convention (e.g. "005930.KS", "2330.TW"), so ingestion can
// dispatch to the right per-market adapter. Bare (suffix-less) tickers are US,
// which keeps the existing US path the default with zero behaviour change.
package market

import "strings"

// Market is a listing venue.
type Market string

const (
	US Market = "US"
	KR Market = "KR"
	TW Market = "TW"
	HK Market = "HK"
)

// Of classifies a ticker by its suffix (case-insensitive). A ticker with no
// recognised suffix is US.
func Of(ticker string) Market {
	u := strings.ToUpper(strings.TrimSpace(ticker))
	switch {
	case strings.HasSuffix(u, ".KS"), strings.HasSuffix(u, ".KQ"):
		return KR
	case strings.HasSuffix(u, ".TW"), strings.HasSuffix(u, ".TWO"):
		return TW
	case strings.HasSuffix(u, ".HK"):
		return HK
	default:
		return US
	}
}

// Base strips the venue suffix: "005930.KS" → "005930", "AAPL" → "AAPL". It
// never trims leading zeros (KR/TW/HK codes are fixed-width strings).
func Base(ticker string) string {
	if i := strings.LastIndexByte(ticker, '.'); i > 0 {
		return ticker[:i]
	}
	return ticker
}
