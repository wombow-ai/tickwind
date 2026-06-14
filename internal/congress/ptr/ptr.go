// Package ptr parses the per-filing PDF of a U.S. House Periodic Transaction
// Report (PTR) into individual securities transactions (ticker, buy/sell, amount
// range, dates). It complements internal/congress, which only surfaces the
// filing index (member · date · PDF link); this package extracts the trade
// detail that lives inside the linked PDF.
//
// # Two PDF flavors
//
// The House Clerk serves PTRs as one of two kinds of PDF:
//
//   - Digital (e-filed): a text PDF with a real, machine-readable transaction
//     table. These dominate recent years (~87% of 2025 filings) and parse
//     cleanly. DocIDs are 8 digits (e.g. 20026590).
//   - Scanned (paper): an image-only PDF of a hand-filled form, with no
//     extractable text. These need OCR, which this package does NOT do. DocIDs
//     are typically 7 digits (e.g. 8220731). ~13% of 2025; higher in older years.
//
// Parse detects a scanned PDF (near-empty extracted text) and returns
// ErrScanned so the caller can fall back to linking the official PDF rather
// than surfacing partial data.
//
// # Extraction
//
// Digital PTRs are laid out as a fixed-column table. The most reliable way to
// recover that table as parseable text is poppler's `pdftotext -layout`, which
// preserves column alignment. This package shells out to it through the
// Extractor interface (default: PdftotextExtractor), so the system dependency is
// isolated and swappable — tests inject a fixture extractor and never touch the
// binary or the network.
//
// # System dependency (production)
//
// PdftotextExtractor requires the `pdftotext` binary (Debian/Ubuntu:
// `apt-get install -y poppler-utils`; ~20 MB). Add it to the API image's
// Dockerfile before wiring this in. If the binary is absent, NewPdftotext
// returns an error so the caller can disable PTR detail parsing gracefully.
package ptr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ErrScanned is returned by Parse / ParseText when the PDF carries no
// machine-readable text (an image-only scanned/paper filing). The caller should
// fall back to the official PDF link rather than treat the report as empty.
var ErrScanned = errors.New("ptr: scanned/image-only PDF (no extractable text); needs OCR")

// minTextBytes is the extracted-text length below which a PDF that ALSO shows no
// transaction-date pattern is treated as scanned. A digital PTR carries dated
// rows; an image-only scan yields only a handful of stray glyphs (and no dates).
const minTextBytes = 200

// anyDate matches a single MM/DD/YYYY token; its presence is strong evidence the
// text is a real (digital) report rather than an image-only scan.
var anyDate = regexp.MustCompile(`\d{2}/\d{2}/\d{4}`)

// TxType is a PTR transaction direction code.
type TxType string

const (
	// TxPurchase is a purchase (PTR code "P").
	TxPurchase TxType = "purchase"
	// TxSale is a sale, full or partial (PTR codes "S" / "S (partial)").
	TxSale TxType = "sale"
	// TxExchange is an exchange (PTR code "E").
	TxExchange TxType = "exchange"
	// TxUnknown is any direction code we do not recognize.
	TxUnknown TxType = "unknown"
)

// Owner is the PTR owner code: who holds the asset within the filer's household.
type Owner string

const (
	// OwnerSelf is the filing member (no owner code printed).
	OwnerSelf Owner = "self"
	// OwnerSpouse is the member's spouse (PTR code "SP").
	OwnerSpouse Owner = "spouse"
	// OwnerJoint is jointly held (PTR code "JT").
	OwnerJoint Owner = "joint"
	// OwnerDependent is a dependent child (PTR code "DC").
	OwnerDependent Owner = "dependent_child"
)

// Transaction is one securities transaction extracted from a PTR PDF.
type Transaction struct {
	Owner       Owner     `json:"owner"`        // who holds the asset (self/spouse/joint/dependent)
	Asset       string    `json:"asset"`        // full asset description, e.g. "Apple Inc. - Common Stock"
	Ticker      string    `json:"ticker"`       // exchange ticker if present, e.g. "AAPL"; "" for assets without one
	AssetType   string    `json:"asset_type"`   // bracket code: ST=stock, OP=options, GS=govt security, etc.; "" if absent
	Type        TxType    `json:"type"`         // purchase / sale / exchange / unknown
	Partial     bool      `json:"partial"`      // true for "S (partial)"
	AmountLow   int64     `json:"amount_low"`   // low bound of the disclosed amount range, USD (e.g. 250001)
	AmountHigh  int64     `json:"amount_high"`  // high bound, USD (e.g. 500000); 0 if open-ended/unparsed
	AmountRange string    `json:"amount_range"` // raw range text, e.g. "$250,001 - $500,000"
	TxDate      time.Time `json:"tx_date"`      // transaction date
	NotifyDate  time.Time `json:"notify_date"`  // notification date
	Raw         string    `json:"raw"`          // the raw header line this row was parsed from (debugging/audit)
}

// Result is the outcome of parsing one PTR PDF.
type Result struct {
	Transactions []Transaction `json:"transactions"`
	// Skipped counts transaction-like rows that could not be fully parsed
	// (e.g. malformed amount); they are dropped rather than guessed.
	Skipped int `json:"skipped"`
}

// Extractor turns raw PDF bytes into plain text. The default implementation
// shells out to `pdftotext -layout`; tests inject a fixture.
type Extractor interface {
	Extract(ctx context.Context, pdf []byte) (string, error)
}

// PdftotextExtractor extracts text via the poppler `pdftotext -layout` binary.
type PdftotextExtractor struct {
	// Path is the pdftotext executable path; resolved by NewPdftotext.
	Path string
}

// NewPdftotext locates the pdftotext binary on PATH and returns an Extractor.
// It errors if poppler-utils is not installed, so callers can disable PTR detail
// parsing instead of failing every filing at runtime.
func NewPdftotext() (*PdftotextExtractor, error) {
	p, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, fmt.Errorf("ptr: pdftotext not found (install poppler-utils): %w", err)
	}
	return &PdftotextExtractor{Path: p}, nil
}

// Extract runs `pdftotext -layout -nopgbrk - -` on the PDF bytes via stdin/stdout.
func (e *PdftotextExtractor) Extract(ctx context.Context, pdf []byte) (string, error) {
	cmd := exec.CommandContext(ctx, e.Path, "-layout", "-nopgbrk", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ptr: pdftotext: %w (%s)", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Parse extracts text from the PDF bytes and parses the transaction table.
// It returns ErrScanned for image-only (scanned) PDFs.
func Parse(ctx context.Context, ex Extractor, pdf []byte) (Result, error) {
	txt, err := ex.Extract(ctx, pdf)
	if err != nil {
		return Result{}, err
	}
	return ParseText(txt)
}

// ParseText parses the plain text of a digital PTR (as produced by
// `pdftotext -layout`) into transactions. It is the pure, network/binary-free
// entry point used by tests. Returns ErrScanned when the text is too short to be
// a real e-filed report.
func ParseText(text string) (Result, error) {
	// Image-only scans extract to a few stray glyphs with no dated rows. Treat a
	// PDF as scanned only when it is BOTH short AND date-free, so a legitimately
	// small one-transaction report is not mistaken for a scan.
	clean := stripNulls(text)
	if len(strings.TrimSpace(clean)) < minTextBytes && !anyDate.MatchString(clean) {
		return Result{}, ErrScanned
	}
	lines := strings.Split(clean, "\n")

	var res Result
	for i := 0; i < len(lines); i++ {
		m := txAnchor.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		tx, ok := parseRow(lines, i, m)
		if !ok {
			res.Skipped++
			continue
		}
		res.Transactions = append(res.Transactions, tx)
	}
	return res, nil
}

// txAnchor matches the spine of a transaction row: the transaction-type token,
// the transaction date, and the notification date, in their fixed order. The
// leading group captures everything before the type token (owner + asset text);
// the trailing group captures the start of the amount range. Anchoring on the
// two adjacent MM/DD/YYYY dates is far more robust than fixed column offsets,
// which drift between pages and filings.
var txAnchor = regexp.MustCompile(
	`^(.*?)\s+(S \(partial\)|[PSE])\s+(\d{2}/\d{2}/\d{4})\s+(\d{2}/\d{2}/\d{4})\s+(.*)$`,
)

var (
	tickerRe  = regexp.MustCompile(`\(([A-Z][A-Z0-9.\-]{0,9})\)`) // (AAPL), (BRK.B)
	bracketRe = regexp.MustCompile(`\[([A-Z]{2,3})\]`)            // [ST] [OP] [GS]
	cusipRe   = regexp.MustCompile(`^[0-9A-Z]{9}$`)               // 91282CJP7 — a CUSIP, not a ticker
	moneyRe   = regexp.MustCompile(`\$[\d,]+`)
	ownerRe   = regexp.MustCompile(`^(SP|JT|DC)\b`)
)

// parseRow assembles one Transaction from the anchor line at index i, pulling
// the ticker / asset-type continuation from the following line and the
// amount-high bound from the line below the anchor when the range wraps.
func parseRow(lines []string, i int, m []string) (Transaction, bool) {
	pre := strings.TrimSpace(m[1]) // owner + asset (first line)
	typeTok := m[2]                // P / S / E / S (partial)
	txDate := parseDate(m[3])      //
	notifyDate := parseDate(m[4])  //
	amtStart := strings.TrimSpace(m[5])

	if txDate.IsZero() {
		return Transaction{}, false
	}

	tx := Transaction{
		Type:       txType(typeTok),
		Partial:    strings.Contains(typeTok, "partial"),
		TxDate:     txDate,
		NotifyDate: notifyDate,
		Raw:        strings.TrimSpace(lines[i]),
	}

	// Owner code is the leading token of the pre-type text (absent ⇒ self).
	owner := OwnerSelf
	assetLine := pre
	if om := ownerRe.FindString(pre); om != "" {
		owner = ownerCode(om)
		assetLine = strings.TrimSpace(pre[len(om):])
	}
	tx.Owner = owner

	// The asset name may wrap onto the next line, which also carries the
	// "(TICKER) [TYPE]" tail. Append the continuation line (up to the "F S:"
	// sub-row) so ticker/asset-type detection sees it.
	asset := assetLine
	if cont, ok := assetContinuation(lines, i); ok {
		asset = strings.TrimSpace(asset + " " + cont)
	}

	// Amount: low bound on the anchor line, high bound on the wrapped next line.
	low := firstMoney(amtStart)
	high := wrappedAmountHigh(lines, i, low)
	tx.AmountLow = low
	tx.AmountHigh = high
	tx.AmountRange = formatRange(amtStart, low, high)

	tx.Ticker, tx.AssetType, tx.Asset = splitAsset(asset)
	return tx, true
}

// assetContinuation returns the text of the line after the anchor when it is a
// continuation of the asset name (i.e. not itself a new transaction row and not
// the "F S:"/"D:" sub-rows). It carries the wrapped asset name + (TICKER)[TYPE].
func assetContinuation(lines []string, i int) (string, bool) {
	if i+1 >= len(lines) {
		return "", false
	}
	next := strings.TrimSpace(lines[i+1])
	if next == "" || txAnchor.MatchString(lines[i+1]) {
		return "", false
	}
	// Sub-rows / boilerplate we never want folded into the asset name.
	if strings.HasPrefix(next, "F ") || strings.HasPrefix(next, "D ") ||
		strings.HasPrefix(next, "D:") || strings.HasPrefix(next, "S O") ||
		strings.HasPrefix(next, "$") {
		return "", false
	}
	// Strip a trailing wrapped amount-high (e.g. "Stock (NVDA) [ST]   $500,000").
	next = strings.TrimSpace(moneyRe.ReplaceAllString(next, ""))
	return next, next != ""
}

// wrappedAmountHigh returns the high bound of the amount range, which the layout
// places on the line below the anchor (the range wraps after the " -"). low is
// the anchor's low bound, used to reject an implausible high (a value below the
// low, which would yield an inverted "$50,000,000 - $9,999" range surfaced
// verbatim). An open-ended top band ("Over $50,000,000", no real high) must stay
// open-ended (high 0 → formatRange renders "$low+") rather than borrow a later
// narrative figure such as a "$9,999" note from a sub-row.
func wrappedAmountHigh(lines []string, i int, low int64) int64 {
	// Same-line range: "$1,001 - $15,000" — both bounds already on the anchor.
	if i < len(lines) {
		if ms := moneyRe.FindAllString(lines[i], -1); len(ms) >= 2 {
			return parseMoney(ms[len(ms)-1])
		}
	}
	for j := i + 1; j < len(lines) && j <= i+2; j++ {
		// Skip narrative / sub-row continuation lines (the "F S:"/"D:"/"S O" rows
		// and any line carrying a ":" note), mirroring assetContinuation's guard.
		// Their dollar figures (e.g. a "$9,999" in a "D:" note) are not the
		// amount-column high bound and must never be adopted as one.
		if isSubRow(lines[j]) {
			continue
		}
		if ms := moneyRe.FindAllString(lines[j], -1); len(ms) >= 1 {
			high := parseMoney(ms[len(ms)-1])
			// Only accept a high that forms a plausible range with the low. A high
			// below the low is an open-ended band that wrapped onto an unrelated
			// figure — leave it open-ended (0) rather than invert the range.
			if high >= low {
				return high
			}
			return 0
		}
	}
	return 0
}

// isSubRow reports whether a line is a PTR sub-row / narrative continuation
// rather than the amount-column high-bound wrap. These carry the filing-status
// ("F S: New"), description ("D: …"), or owner-org ("S O: …") notes, and any
// figures in them (share counts, strike prices, narrative dollar amounts) are
// not the disclosed amount range. Mirrors the guard in assetContinuation.
func isSubRow(line string) bool {
	s := strings.TrimSpace(line)
	if strings.HasPrefix(s, "F ") || strings.HasPrefix(s, "D ") ||
		strings.HasPrefix(s, "D:") || strings.HasPrefix(s, "S O") {
		return true
	}
	return strings.Contains(s, ":")
}

// splitAsset pulls the ticker and bracket asset-type out of the asset text and
// returns the cleaned asset name. A 9-char alphanumeric parenthetical is a CUSIP
// (bonds/treasuries), not a ticker, so it is not returned as Ticker.
func splitAsset(asset string) (ticker, assetType, name string) {
	asset = strings.Join(strings.Fields(asset), " ") // collapse whitespace
	if b := bracketRe.FindStringSubmatch(asset); b != nil {
		assetType = b[1]
	}
	if t := tickerRe.FindStringSubmatch(asset); t != nil {
		cand := t[1]
		if !cusipRe.MatchString(cand) {
			ticker = cand
		}
	}
	// Trim the "(TICKER) [TYPE]" tail from the displayed name.
	name = strings.TrimSpace(bracketRe.ReplaceAllString(asset, ""))
	name = strings.TrimSpace(tickerRe.ReplaceAllString(name, ""))
	name = strings.Trim(name, " -")
	return ticker, assetType, name
}

func txType(tok string) TxType {
	switch {
	case strings.HasPrefix(tok, "P"):
		return TxPurchase
	case strings.HasPrefix(tok, "S"):
		return TxSale
	case strings.HasPrefix(tok, "E"):
		return TxExchange
	default:
		return TxUnknown
	}
}

func ownerCode(s string) Owner {
	switch s {
	case "SP":
		return OwnerSpouse
	case "JT":
		return OwnerJoint
	case "DC":
		return OwnerDependent
	default:
		return OwnerSelf
	}
}

func firstMoney(s string) int64 {
	if m := moneyRe.FindString(s); m != "" {
		return parseMoney(m)
	}
	return 0
}

func parseMoney(s string) int64 {
	s = strings.ReplaceAll(strings.TrimPrefix(s, "$"), ",", "")
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

func formatRange(amtStart string, low, high int64) string {
	if low == 0 && high == 0 {
		return strings.TrimSpace(amtStart)
	}
	if high == 0 {
		return fmt.Sprintf("$%s+", group(low))
	}
	return fmt.Sprintf("$%s - $%s", group(low), group(high))
}

// group inserts thousands separators into a positive integer.
func group(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

func parseDate(s string) time.Time {
	t, err := time.Parse("01/02/2006", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func stripNulls(s string) string {
	if !strings.ContainsRune(s, 0) {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "")
}
