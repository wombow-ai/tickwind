package sec

import "testing"

func TestParseInfoTable(t *testing.T) {
	// Namespaced root (as EDGAR emits), Apple split across two lots, and a bond
	// reported as principal (PRN) — its shares must NOT count toward a share total.
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<informationTable xmlns="http://www.sec.gov/edgar/document/thirteenf/informationtable">
  <infoTable><nameOfIssuer>APPLE INC</nameOfIssuer><titleOfClass>COM</titleOfClass><cusip>037833100</cusip><value>100</value><shrsOrPrnAmt><sshPrnamt>10</sshPrnamt><sshPrnamtType>SH</sshPrnamtType></shrsOrPrnAmt></infoTable>
  <infoTable><nameOfIssuer>APPLE INC</nameOfIssuer><titleOfClass>COM</titleOfClass><cusip>037833100</cusip><value>50</value><shrsOrPrnAmt><sshPrnamt>5</sshPrnamt><sshPrnamtType>SH</sshPrnamtType></shrsOrPrnAmt></infoTable>
  <infoTable><nameOfIssuer>ACME 5% NOTES</nameOfIssuer><titleOfClass>NOTE</titleOfClass><cusip>11111AAA1</cusip><value>200</value><shrsOrPrnAmt><sshPrnamt>999</sshPrnamt><sshPrnamtType>PRN</sshPrnamtType></shrsOrPrnAmt></infoTable>
</informationTable>`
	hs, err := parseInfoTable([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 2 {
		t.Fatalf("want 2 aggregated holdings, got %d", len(hs))
	}
	// Apple: two lots aggregated → value 150, shares 15.
	if hs[0].CUSIP != "037833100" || hs[0].Issuer != "APPLE INC" || hs[0].Value != 150 || hs[0].Shares != 15 {
		t.Errorf("apple aggregate = %+v", hs[0])
	}
	// Bond: value summed, but PRN principal is not counted as shares.
	if hs[1].CUSIP != "11111AAA1" || hs[1].Value != 200 || hs[1].Shares != 0 {
		t.Errorf("bond = %+v (PRN shares must be 0)", hs[1])
	}
}
