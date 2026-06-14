package sec

import "testing"

const sampleForm4 = `<?xml version="1.0"?>
<ownershipDocument>
  <issuer>
    <issuerCik>0001018840</issuerCik>
    <issuerName>ACME SMALLCAP INC</issuerName>
    <issuerTradingSymbol>ACME</issuerTradingSymbol>
  </issuer>
  <reportingOwner>
    <reportingOwnerId><rptOwnerName>Doe Jane</rptOwnerName></reportingOwnerId>
    <reportingOwnerRelationship>
      <isDirector>0</isDirector>
      <isOfficer>1</isOfficer>
      <officerTitle>Chief Financial Officer</officerTitle>
    </reportingOwnerRelationship>
  </reportingOwner>
  <nonDerivativeTable>
    <nonDerivativeTransaction>
      <transactionCoding><transactionCode>P</transactionCode></transactionCoding>
      <transactionAmounts>
        <transactionShares><value>10000.0000</value></transactionShares>
        <transactionPricePerShare><value>12.50</value></transactionPricePerShare>
        <transactionAcquiredDisposedCode><value>A</value></transactionAcquiredDisposedCode>
      </transactionAmounts>
    </nonDerivativeTransaction>
    <nonDerivativeTransaction>
      <transactionCoding><transactionCode>A</transactionCode></transactionCoding>
      <transactionAmounts>
        <transactionShares><value>5000.0000</value></transactionShares>
        <transactionPricePerShare><value>0.00</value></transactionPricePerShare>
        <transactionAcquiredDisposedCode><value>A</value></transactionAcquiredDisposedCode>
      </transactionAmounts>
    </nonDerivativeTransaction>
    <nonDerivativeTransaction>
      <transactionCoding><transactionCode>S</transactionCode></transactionCoding>
      <transactionAmounts>
        <transactionShares><value>2000.0000</value></transactionShares>
        <transactionPricePerShare><value>13.00</value></transactionPricePerShare>
        <transactionAcquiredDisposedCode><value>D</value></transactionAcquiredDisposedCode>
      </transactionAmounts>
    </nonDerivativeTransaction>
  </nonDerivativeTable>
</ownershipDocument>`

func TestParseForm4(t *testing.T) {
	f, err := ParseForm4([]byte(sampleForm4))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	if f.Ticker != "ACME" {
		t.Errorf("ticker=%q want ACME", f.Ticker)
	}
	if f.IssuerName != "ACME SMALLCAP INC" {
		t.Errorf("issuer=%q", f.IssuerName)
	}
	if f.OwnerName != "Doe Jane" {
		t.Errorf("owner=%q", f.OwnerName)
	}
	if !f.IsOfficer || f.IsDirector {
		t.Errorf("officer=%v director=%v want true/false", f.IsOfficer, f.IsDirector)
	}
	if f.OfficerTitle != "Chief Financial Officer" {
		t.Errorf("title=%q", f.OfficerTitle)
	}
	// Only the P buy survives; the A award ($0) and the S sale are dropped.
	if len(f.Buys) != 1 {
		t.Fatalf("buys=%d want 1", len(f.Buys))
	}
	b := f.Buys[0]
	if b.Shares != 10000 || b.Price != 12.5 || b.Value != 125000 {
		t.Errorf("buy=%+v want shares 10000 price 12.5 value 125000", b)
	}
	if !f.HasBuys() || f.BuyValue() != 125000 {
		t.Errorf("HasBuys=%v BuyValue=%v", f.HasBuys(), f.BuyValue())
	}
}

// TestParseForm4Sells: the S sale is parsed into Sells with the transaction
// date captured, and the existing P buy is unaffected (regression guard). The
// $0 award (code A) and the gift are dropped from both lists.
func TestParseForm4Sells(t *testing.T) {
	f, err := ParseForm4([]byte(sampleForm4))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	// The buy pipeline is unchanged.
	if len(f.Buys) != 1 || f.Buys[0].Value != 125000 {
		t.Fatalf("buys=%+v want one $125k buy", f.Buys)
	}
	// One S sale survives (the award/gift are dropped).
	if len(f.Sells) != 1 {
		t.Fatalf("sells=%d want 1", len(f.Sells))
	}
	s := f.Sells[0]
	if s.Shares != 2000 || s.Price != 13.0 || s.Value != 26000 {
		t.Errorf("sale=%+v want shares 2000 price 13 value 26000", s)
	}
	if s.Planned10b5_1 {
		t.Error("sale flagged 10b5-1 with no affirmation/footnote (must default false)")
	}
	if !f.HasSells() || f.SellValue() != 26000 {
		t.Errorf("HasSells=%v SellValue=%v", f.HasSells(), f.SellValue())
	}
}

// TestParseForm4TransactionDate: each parsed buy/sale carries its
// <transactionDate><value>, needed for timeline ordering.
func TestParseForm4TransactionDate(t *testing.T) {
	const xml = `<ownershipDocument><issuer><issuerTradingSymbol>X</issuerTradingSymbol></issuer>` +
		`<nonDerivativeTable>` +
		`<nonDerivativeTransaction>` +
		`<transactionCoding><transactionCode>P</transactionCode></transactionCoding>` +
		`<transactionDate><value>2026-05-20</value></transactionDate>` +
		`<transactionAmounts><transactionShares><value>100</value></transactionShares>` +
		`<transactionPricePerShare><value>10</value></transactionPricePerShare>` +
		`<transactionAcquiredDisposedCode><value>A</value></transactionAcquiredDisposedCode>` +
		`</transactionAmounts></nonDerivativeTransaction>` +
		`<nonDerivativeTransaction>` +
		`<transactionCoding><transactionCode>S</transactionCode></transactionCoding>` +
		`<transactionDate><value>2026-05-21</value></transactionDate>` +
		`<transactionAmounts><transactionShares><value>200</value></transactionShares>` +
		`<transactionPricePerShare><value>11</value></transactionPricePerShare>` +
		`<transactionAcquiredDisposedCode><value>D</value></transactionAcquiredDisposedCode>` +
		`</transactionAmounts></nonDerivativeTransaction>` +
		`</nonDerivativeTable></ownershipDocument>`
	f, err := ParseForm4([]byte(xml))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	if len(f.Buys) != 1 || f.Buys[0].Date != "2026-05-20" {
		t.Errorf("buy date=%q want 2026-05-20 (buys=%+v)", buyDate(f), f.Buys)
	}
	if len(f.Sells) != 1 || f.Sells[0].Date != "2026-05-21" {
		t.Errorf("sell date=%q want 2026-05-21 (sells=%+v)", sellDate(f), f.Sells)
	}
}

func buyDate(f Form4) string {
	if len(f.Buys) == 0 {
		return ""
	}
	return f.Buys[0].Date
}
func sellDate(f Form4) string {
	if len(f.Sells) == 0 {
		return ""
	}
	return f.Sells[0].Date
}

// TestParseForm4Planned10b5OneStructured: the document-level <aff10b5One>true
// affirmation (the post-2023 checkbox) flags the sale as a planned 10b5-1 sale.
func TestParseForm4Planned10b5OneStructured(t *testing.T) {
	const xml = `<ownershipDocument><issuer><issuerTradingSymbol>X</issuerTradingSymbol></issuer>` +
		`<aff10b5One>true</aff10b5One>` +
		`<nonDerivativeTable><nonDerivativeTransaction>` +
		`<transactionCoding><transactionCode>S</transactionCode></transactionCoding>` +
		`<transactionDate><value>2026-05-21</value></transactionDate>` +
		`<transactionAmounts><transactionShares><value>500</value></transactionShares>` +
		`<transactionPricePerShare><value>20</value></transactionPricePerShare>` +
		`<transactionAcquiredDisposedCode><value>D</value></transactionAcquiredDisposedCode>` +
		`</transactionAmounts></nonDerivativeTransaction></nonDerivativeTable></ownershipDocument>`
	f, err := ParseForm4([]byte(xml))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	if len(f.Sells) != 1 || !f.Sells[0].Planned10b5_1 {
		t.Errorf("sale not flagged 10b5-1 from <aff10b5One>true: %+v", f.Sells)
	}
}

// TestParseForm4Planned10b5OneFootnote: a pre-2023 filing with no <aff10b5One>
// checkbox but a footnote referencing a "Rule 10b5-1 trading plan" is detected
// via the conservative footnote backstop.
func TestParseForm4Planned10b5OneFootnote(t *testing.T) {
	const xml = `<ownershipDocument><issuer><issuerTradingSymbol>X</issuerTradingSymbol></issuer>` +
		`<nonDerivativeTable><nonDerivativeTransaction>` +
		`<transactionCoding><transactionCode>S</transactionCode></transactionCoding>` +
		`<transactionAmounts><transactionShares><value>500</value></transactionShares>` +
		`<transactionPricePerShare><value>20</value></transactionPricePerShare>` +
		`<transactionAcquiredDisposedCode><value>D</value></transactionAcquiredDisposedCode>` +
		`</transactionAmounts></nonDerivativeTransaction></nonDerivativeTable>` +
		`<footnotes><footnote id="F1">This transaction was made pursuant to a Rule 10b5-1 trading plan adopted on January 2, 2022.</footnote></footnotes>` +
		`</ownershipDocument>`
	f, err := ParseForm4([]byte(xml))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	if len(f.Sells) != 1 || !f.Sells[0].Planned10b5_1 {
		t.Errorf("sale not flagged 10b5-1 from footnote backstop: %+v", f.Sells)
	}
}

// TestParseForm4NotPlannedWhenAbsent: with neither the affirmation nor a 10b5-1
// footnote, the flag stays false — it is never guessed.
func TestParseForm4NotPlannedWhenAbsent(t *testing.T) {
	const xml = `<ownershipDocument><issuer><issuerTradingSymbol>X</issuerTradingSymbol></issuer>` +
		`<aff10b5One>false</aff10b5One>` +
		`<nonDerivativeTable><nonDerivativeTransaction>` +
		`<transactionCoding><transactionCode>S</transactionCode></transactionCoding>` +
		`<transactionAmounts><transactionShares><value>500</value></transactionShares>` +
		`<transactionPricePerShare><value>20</value></transactionPricePerShare>` +
		`<transactionAcquiredDisposedCode><value>D</value></transactionAcquiredDisposedCode>` +
		`</transactionAmounts></nonDerivativeTransaction></nonDerivativeTable>` +
		`<footnotes><footnote id="F1">Weighted average sale price across multiple trades.</footnote></footnotes>` +
		`</ownershipDocument>`
	f, err := ParseForm4([]byte(xml))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	if len(f.Sells) != 1 || f.Sells[0].Planned10b5_1 {
		t.Errorf("sale wrongly flagged 10b5-1 with no indicator: %+v", f.Sells)
	}
}

// TestParseForm4Planned10b5OneFootnoteBoundary: a footnote whose only "10b5-1"
// is part of a longer digit run ("10b5-10") must NOT flag — the substring match
// is boundary-guarded so the anti-hallucination flag is never set without a real
// Rule 10b5-1 reference.
func TestParseForm4Planned10b5OneFootnoteBoundary(t *testing.T) {
	const xml = `<ownershipDocument><issuer><issuerTradingSymbol>X</issuerTradingSymbol></issuer>` +
		`<nonDerivativeTable><nonDerivativeTransaction>` +
		`<transactionCoding><transactionCode>S</transactionCode></transactionCoding>` +
		`<transactionAmounts><transactionShares><value>500</value></transactionShares>` +
		`<transactionPricePerShare><value>20</value></transactionPricePerShare>` +
		`<transactionAcquiredDisposedCode><value>D</value></transactionAcquiredDisposedCode>` +
		`</transactionAmounts></nonDerivativeTransaction></nonDerivativeTable>` +
		`<footnotes><footnote id="F1">See item 10b5-10 and contract no. 110b5-1234 referenced herein.</footnote></footnotes>` +
		`</ownershipDocument>`
	f, err := ParseForm4([]byte(xml))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	if len(f.Sells) != 1 || f.Sells[0].Planned10b5_1 {
		t.Errorf("sale wrongly flagged 10b5-1 from a digit-run substring: %+v", f.Sells)
	}
}

func TestParseForm4AwardOnly(t *testing.T) {
	const award = `<ownershipDocument><issuer><issuerTradingSymbol>X</issuerTradingSymbol></issuer>` +
		`<nonDerivativeTable><nonDerivativeTransaction>` +
		`<transactionCoding><transactionCode>A</transactionCode></transactionCoding>` +
		`<transactionAmounts><transactionShares><value>100</value></transactionShares>` +
		`<transactionPricePerShare><value>0</value></transactionPricePerShare>` +
		`<transactionAcquiredDisposedCode><value>A</value></transactionAcquiredDisposedCode>` +
		`</transactionAmounts></nonDerivativeTransaction></nonDerivativeTable></ownershipDocument>`
	f, err := ParseForm4([]byte(award))
	if err != nil {
		t.Fatalf("ParseForm4: %v", err)
	}
	if f.HasBuys() {
		t.Error("an award-only filing should yield no open-market buys")
	}
}
