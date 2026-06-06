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
