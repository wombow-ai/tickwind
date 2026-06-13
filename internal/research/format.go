package research

import (
	"math"
	"strconv"
)

// Unit strings used by Facts. They mirror indicators' unit vocabulary, plus the
// "USD" unit the assembler uses for dollar figures (market cap, FCF) that the
// indicator set reports unitless.
const (
	unitPercent = "%"
	unitMult    = "x"
	unitPrice   = "price"
	unitUSD     = "USD"
	unitNone    = ""
)

// dash is the placeholder for a nil / absent value. Never present 0 as a value
// for an absent field.
const dash = "—"

// loss is the placeholder for a loss-maker's P/E (a non-positive-EPS multiple is
// intentionally "亏损/—", never 0).
const loss = "亏损"

// formatValue renders a raw number for display by its unit, mirroring the
// frontend formatters so the report and the cards agree:
//   - "%"     → "42.0%"
//   - "x"     → "41.2x"
//   - "price" → "$190.12" (price-decimals tiering: more dp under $10)
//   - "USD"   → compact "$1.2B" / "$98.80B" / "$4.51T", sign-aware
//   - ""      → the bare number (e.g. RSI 56.3, MACD line, volume)
//
// A nil pointer renders the em-dash placeholder.
func formatValue(raw *float64, unit string) string {
	if raw == nil {
		return dash
	}
	v := *raw
	switch unit {
	case unitPercent:
		return strconv.FormatFloat(v, 'f', 1, 64) + "%"
	case unitMult:
		return strconv.FormatFloat(v, 'f', 1, 64) + "x"
	case unitPrice:
		return formatPrice(v)
	case unitUSD:
		return fmtCompactUSD(v)
	default:
		return formatPlain(v)
	}
}

// formatPrice mirrors web priceDecimals + fmtPrice (USD): 4dp under $1, 3dp under
// $10, 2dp otherwise. Always carries the "$" symbol.
func formatPrice(v float64) string {
	dp := 2
	a := math.Abs(v)
	switch {
	case a > 0 && a < 1:
		dp = 4
	case a < 10:
		dp = 3
	}
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return sign + "$" + strconv.FormatFloat(v, 'f', dp, 64)
}

// fmtCompactUSD mirrors the web fmtCompactUSD: "$4.51T" / "$416.20B" / "$1.20M" /
// "-$3.85B". 2dp for T/B/M, 1dp for K, 0dp under $1K, sign-aware.
func fmtCompactUSD(v float64) string {
	a := math.Abs(v)
	sign := ""
	if v < 0 {
		sign = "-"
	}
	switch {
	case a >= 1e12:
		return sign + "$" + strconv.FormatFloat(a/1e12, 'f', 2, 64) + "T"
	case a >= 1e9:
		return sign + "$" + strconv.FormatFloat(a/1e9, 'f', 2, 64) + "B"
	case a >= 1e6:
		return sign + "$" + strconv.FormatFloat(a/1e6, 'f', 2, 64) + "M"
	case a >= 1e3:
		return sign + "$" + strconv.FormatFloat(a/1e3, 'f', 1, 64) + "K"
	default:
		return sign + "$" + strconv.FormatFloat(a, 'f', 0, 64)
	}
}

// formatPlain renders a bare number with at most two decimals, trimming trailing
// zeros (e.g. RSI "56.3", a whole-number volume "1234567"). Integers stay clean.
func formatPlain(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}
