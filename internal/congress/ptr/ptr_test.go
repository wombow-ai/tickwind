package ptr

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fixturePelosi is the verbatim `pdftotext -layout` output (transaction table
// region) of Nancy Pelosi's PTR DocID 20026590 (filed 01/17/2025) — options +
// stock buys/sells across GOOGL, AMZN, AAPL, NVDA, PANW, TEM, VST. It exercises
// SP (spouse) owner codes, "S (partial)" sales, multi-line asset names, and the
// amount-range wrap. Captured offline so the test never hits the network/binary.
const fixturePelosi = `
          SP          Alphabet Inc. - Class A Common           P                 01/14/2025 01/14/2025           $250,001 -
                      Stock (GOOGL) [OP]                                                                         $500,000
                      F      S      : New
                      D           : Purchased 50 call options with a strike price of $150 and an expiration date of 1/16/26.


          SP          Amazon.com, Inc. - Common Stock          P                 01/14/2025 01/14/2025           $250,001 -
                      (AMZN) [OP]                                                                                $500,000
                      F      S      : New
                      D           : Purchased 50 call options with a strike price of $150 and an expiration date of 1/16/26.


          SP          Apple Inc. - Common Stock (AAPL)         S (partial)       12/31/2024 12/31/2024           $5,000,001 -
                      [ST]                                                                                       $25,000,000
                      F      S      : New
                      D           : Sold 31,600 shares.


          SP          NVIDIA Corporation - Common              S (partial)       12/31/2024 12/31/2024           $1,000,001 -
                      Stock (NVDA) [ST]                                                                          $5,000,000
                      F      S      : New
                      D           : Sold 10,000 shares.


          SP          NVIDIA Corporation - Common              P                 12/20/2024 12/20/2024           $500,001 -
                      Stock (NVDA) [ST]                                                                          $1,000,000
                      F      S       : New
                      D            : Exercised 500 call options at a strike price of $12.


          SP          Palo Alto Networks, Inc. (PANW)             P                 12/20/2024 12/20/2024            $1,000,001 -
                      [ST]                                                                                           $5,000,000
                     F      S       : New


          SP          Tempus AI, Inc. - Class A Common            P                 01/14/2025 01/14/2025            $50,001 -
                      Stock (TEM) [OP]                                                                               $100,000
                     F       S       : New


          SP          Vistra Corp. Common Stock (VST)             P                 01/14/2025 01/14/2025            $500,001 -
                      [OP]                                                                                           $1,000,000
                     F       S       : New
`

// fixtureAllen is Richard Allen's PTR DocID 20026537 — treasuries with CUSIPs
// (no exchange ticker) and a member-owned row (no SP/JT/DC owner code). The
// US-Treasury rows must yield Ticker="" (a 9-char CUSIP is not a ticker).
const fixtureAllen = `
           SP         Rollins, Inc. Common Stock (ROL)            P                 12/12/2024 01/08/2025             $15,001 -
                      [ST]                                                                                            $50,000
                      F      S       : New
                      S             O : R.W. Allen & Associates, Inc. > RWA&A - Securities


           SP         US TREASU NOTE 4.375% DUE                   P                 12/03/2024 01/08/2025             $100,001 -
                      12/15/26 (91282CJP7) [GS]                                                                       $250,000
                      F      S       : New


           US TREASURY BILL DUE 03/20/25               P                 12/03/2024 01/08/2024             $15,001 -
                      (912797KJ5) [GS]                                                                                $50,000
                      F      S       : New
`

// fixtureAderholt is Robert Aderholt's PTR DocID 20032062 — a single sale of an
// ADS with the amount range on ONE line ("$1,001 - $15,000"), exercising the
// same-line (non-wrapped) amount path.
const fixtureAderholt = `
                       GSK plc American Depositary Shares         S                 07/28/2025 08/11/2025             $1,001 - $15,000
                       (GSK) [ST]
                       F      S      : New
`

func TestParsePelosi(t *testing.T) {
	res, err := ParseText(fixturePelosi)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if len(res.Transactions) != 8 {
		t.Fatalf("got %d transactions, want 8: %+v", len(res.Transactions), res.Transactions)
	}

	want := []struct {
		ticker    string
		assetType string
		typ       TxType
		partial   bool
		low, high int64
		owner     Owner
	}{
		{"GOOGL", "OP", TxPurchase, false, 250001, 500000, OwnerSpouse},
		{"AMZN", "OP", TxPurchase, false, 250001, 500000, OwnerSpouse},
		{"AAPL", "ST", TxSale, true, 5000001, 25000000, OwnerSpouse},
		{"NVDA", "ST", TxSale, true, 1000001, 5000000, OwnerSpouse},
		{"NVDA", "ST", TxPurchase, false, 500001, 1000000, OwnerSpouse},
		{"PANW", "ST", TxPurchase, false, 1000001, 5000000, OwnerSpouse},
		{"TEM", "OP", TxPurchase, false, 50001, 100000, OwnerSpouse},
		{"VST", "OP", TxPurchase, false, 500001, 1000000, OwnerSpouse},
	}
	for i, w := range want {
		got := res.Transactions[i]
		if got.Ticker != w.ticker {
			t.Errorf("tx[%d] ticker = %q, want %q (raw=%q)", i, got.Ticker, w.ticker, got.Raw)
		}
		if got.AssetType != w.assetType {
			t.Errorf("tx[%d] assetType = %q, want %q", i, got.AssetType, w.assetType)
		}
		if got.Type != w.typ {
			t.Errorf("tx[%d] type = %q, want %q", i, got.Type, w.typ)
		}
		if got.Partial != w.partial {
			t.Errorf("tx[%d] partial = %v, want %v", i, got.Partial, w.partial)
		}
		if got.AmountLow != w.low || got.AmountHigh != w.high {
			t.Errorf("tx[%d] amount = %d-%d, want %d-%d", i, got.AmountLow, got.AmountHigh, w.low, w.high)
		}
		if got.Owner != w.owner {
			t.Errorf("tx[%d] owner = %q, want %q", i, got.Owner, w.owner)
		}
	}

	// Spot-check dates + asset name on the first row.
	first := res.Transactions[0]
	if first.TxDate.Format("2006-01-02") != "2025-01-14" {
		t.Errorf("tx[0] txDate = %s, want 2025-01-14", first.TxDate.Format("2006-01-02"))
	}
	if !strings.Contains(first.Asset, "Alphabet") {
		t.Errorf("tx[0] asset = %q, want it to contain Alphabet", first.Asset)
	}
	if first.AmountRange != "$250,001 - $500,000" {
		t.Errorf("tx[0] amountRange = %q, want %q", first.AmountRange, "$250,001 - $500,000")
	}
}

func TestParseAllenTreasuriesNoTicker(t *testing.T) {
	res, err := ParseText(fixtureAllen)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if len(res.Transactions) != 3 {
		t.Fatalf("got %d transactions, want 3: %+v", len(res.Transactions), res.Transactions)
	}

	rol := res.Transactions[0]
	if rol.Ticker != "ROL" || rol.AssetType != "ST" || rol.Owner != OwnerSpouse {
		t.Errorf("ROL row = ticker %q type %q owner %q, want ROL/ST/spouse", rol.Ticker, rol.AssetType, rol.Owner)
	}

	// Treasury rows: a 9-char CUSIP must NOT be surfaced as a ticker.
	for _, i := range []int{1, 2} {
		tx := res.Transactions[i]
		if tx.Ticker != "" {
			t.Errorf("treasury tx[%d] ticker = %q, want empty (CUSIP is not a ticker)", i, tx.Ticker)
		}
		if tx.AssetType != "GS" {
			t.Errorf("treasury tx[%d] assetType = %q, want GS", i, tx.AssetType)
		}
	}
	// The member-owned (no owner code) row defaults to self.
	if res.Transactions[2].Owner != OwnerSelf {
		t.Errorf("tx[2] owner = %q, want self", res.Transactions[2].Owner)
	}
}

func TestParseAderholtSameLineAmount(t *testing.T) {
	res, err := ParseText(fixtureAderholt)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if len(res.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(res.Transactions))
	}
	tx := res.Transactions[0]
	if tx.Ticker != "GSK" || tx.Type != TxSale || tx.Partial {
		t.Errorf("GSK row = ticker %q type %q partial %v, want GSK/sale/false", tx.Ticker, tx.Type, tx.Partial)
	}
	if tx.AmountLow != 1001 || tx.AmountHigh != 15000 {
		t.Errorf("GSK amount = %d-%d, want 1001-15000", tx.AmountLow, tx.AmountHigh)
	}
	if tx.NotifyDate.Format("2006-01-02") != "2025-08-11" {
		t.Errorf("GSK notifyDate = %s, want 2025-08-11", tx.NotifyDate.Format("2006-01-02"))
	}
}

// fixtureOpenEnded is an open-ended top band ("Over $50,000,000", no real high
// bound) whose only later $-bearing line is a "D:" narrative note carrying a
// smaller figure ($9,999). The high-bound scan must NOT adopt that narrative
// figure — the band stays open-ended ("$50,000,001+"), never an inverted
// "$50,000,000 - $9,999". (BUG 7 regression.)
const fixtureOpenEnded = `
          SP          Megacorp Inc. - Common Stock (MEGA)        P                 01/14/2025 01/14/2025           Over $50,000,000
                      [ST]
                      D           : Note re fee of $9,999 charged on the account.
                      F      S      : New
`

func TestParseOpenEndedBandKeepsOpen(t *testing.T) {
	res, err := ParseText(fixtureOpenEnded)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if len(res.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1: %+v", len(res.Transactions), res.Transactions)
	}
	tx := res.Transactions[0]
	if tx.Ticker != "MEGA" {
		t.Errorf("ticker = %q, want MEGA", tx.Ticker)
	}
	if tx.AmountLow != 50000000 {
		t.Errorf("amount_low = %d, want 50000000", tx.AmountLow)
	}
	// The narrative "$9,999" must NOT be adopted as the high bound. Open-ended ⇒ 0.
	if tx.AmountHigh != 0 {
		t.Errorf("amount_high = %d, want 0 (open-ended; must not borrow narrative $9,999)", tx.AmountHigh)
	}
	if tx.AmountRange != "$50,000,000+" {
		t.Errorf("amount_range = %q, want %q (open-ended form, not an inverted range)", tx.AmountRange, "$50,000,000+")
	}
}

// fixtureWrappedHigh is the common layout: low on the anchor, real high on the
// immediate next line. It must parse EXACTLY as before the BUG 7 guard. (No-regression.)
const fixtureWrappedHigh = `
          SP          Some Company - Common Stock                P                 01/14/2025 01/14/2025           $1,001 -
                      (SCMP) [ST]                                                                                  $15,000
                      F      S      : New
`

func TestParseWrappedHighStillParses(t *testing.T) {
	res, err := ParseText(fixtureWrappedHigh)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if len(res.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1: %+v", len(res.Transactions), res.Transactions)
	}
	tx := res.Transactions[0]
	if tx.Ticker != "SCMP" || tx.AmountLow != 1001 || tx.AmountHigh != 15000 {
		t.Errorf("got ticker %q amount %d-%d, want SCMP 1001-15000", tx.Ticker, tx.AmountLow, tx.AmountHigh)
	}
	if tx.AmountRange != "$1,001 - $15,000" {
		t.Errorf("amount_range = %q, want %q", tx.AmountRange, "$1,001 - $15,000")
	}
}

func TestParseScannedReturnsErrScanned(t *testing.T) {
	// Image-only scans extract to a few stray glyphs.
	_, err := ParseText("P\n\nT\n\nR\n  ")
	if !errors.Is(err, ErrScanned) {
		t.Fatalf("err = %v, want ErrScanned", err)
	}
}

func TestSplitAsset(t *testing.T) {
	cases := []struct {
		in                          string
		ticker, assetType, contains string
	}{
		{"Apple Inc. - Common Stock (AAPL) [ST]", "AAPL", "ST", "Apple"},
		{"Berkshire Hathaway (BRK.B) [ST]", "BRK.B", "ST", "Berkshire"},
		{"US TREASURY NOTE 4.25% DUE 12/31/25 (91282CJS1) [GS]", "", "GS", "TREASURY"},
		{"Some Fund With No Ticker", "", "", "Fund"},
	}
	for _, c := range cases {
		tk, at, name := splitAsset(c.in)
		if tk != c.ticker {
			t.Errorf("splitAsset(%q) ticker = %q, want %q", c.in, tk, c.ticker)
		}
		if at != c.assetType {
			t.Errorf("splitAsset(%q) assetType = %q, want %q", c.in, at, c.assetType)
		}
		if !strings.Contains(name, c.contains) {
			t.Errorf("splitAsset(%q) name = %q, want it to contain %q", c.in, name, c.contains)
		}
	}
}

// fakeExtractor lets Parse be exercised without poppler or the network.
type fakeExtractor struct{ text string }

func (f fakeExtractor) Extract(_ context.Context, _ []byte) (string, error) { return f.text, nil }

func TestParseViaExtractor(t *testing.T) {
	res, err := Parse(context.Background(), fakeExtractor{text: fixtureAderholt}, []byte("ignored"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Transactions) != 1 || res.Transactions[0].Ticker != "GSK" {
		t.Fatalf("got %+v, want one GSK tx", res.Transactions)
	}
}
