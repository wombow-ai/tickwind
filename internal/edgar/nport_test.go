package edgar

import "testing"

// nportFixture is a trimmed N-PORT-P primary_doc.xml with four positions exercising the contract:
// a ticker-bearing one, an ISIN-only one (no ticker), a zero-weight one (dropped), and a nameless
// one (dropped).
const nportFixture = `<?xml version="1.0" encoding="UTF-8"?>
<edgarSubmission>
  <genInfo><repPdDate>2026-03-31</repPdDate><repPdEnd>2026-09-30</repPdEnd></genInfo>
  <formData>
    <invstOrSecs>
      <invstOrSec>
        <name>Big Co</name><title>Big Co</title><cusip>111111111</cusip>
        <identifiers><ticker value="big"/></identifiers>
        <valUSD>500.00</valUSD><pctVal>5.0</pctVal><assetCat>EC</assetCat><invCountry>US</invCountry>
      </invstOrSec>
      <invstOrSec>
        <name>Mega Co</name><title>Mega Co</title><cusip>222222222</cusip>
        <identifiers><isin value="US2222222222"/></identifiers>
        <valUSD>900.00</valUSD><pctVal>9.0</pctVal><assetCat>EC</assetCat><invCountry>US</invCountry>
      </invstOrSec>
      <invstOrSec>
        <name>Tiny Co</name><cusip>333333333</cusip>
        <valUSD>10.00</valUSD><pctVal>0.0</pctVal>
      </invstOrSec>
      <invstOrSec>
        <name></name><valUSD>1.0</valUSD><pctVal>1.0</pctVal>
      </invstOrSec>
    </invstOrSecs>
  </formData>
</edgarSubmission>`

func TestParseNPORTHoldings(t *testing.T) {
	hs, repPd, err := parseNPORTHoldings(nportFixture, 25)
	if err != nil {
		t.Fatal(err)
	}
	if repPd != "2026-03-31" {
		t.Fatalf("repPdDate = %q; want 2026-03-31 (the report period date, the holdings' as-of)", repPd)
	}
	// Zero-weight (Tiny) and nameless positions are dropped → 2 remain.
	if len(hs) != 2 {
		t.Fatalf("got %d holdings; want 2", len(hs))
	}
	// Sorted by pctVal desc → Mega (9.0) first, Big (5.0) second.
	if hs[0].Name != "Mega Co" || hs[0].PctVal != 9.0 {
		t.Fatalf("top = %+v; want Mega Co @ 9.0", hs[0])
	}
	if hs[0].Ticker != "" {
		t.Fatalf("Mega has only an ISIN; ticker must be empty (never fabricated), got %q", hs[0].Ticker)
	}
	if hs[1].Name != "Big Co" || hs[1].Ticker != "BIG" || hs[1].ValUSD != 500 {
		t.Fatalf("second = %+v; want Big Co / BIG / 500 (ticker upper-cased)", hs[1])
	}

	// Cap: top-1 is the biggest weight.
	if one, _, _ := parseNPORTHoldings(nportFixture, 1); len(one) != 1 || one[0].Name != "Mega Co" {
		t.Fatalf("cap=1: got %+v; want [Mega Co]", one)
	}
}
