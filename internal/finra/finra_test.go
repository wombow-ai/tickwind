package finra

import (
	"testing"
	"time"
)

// A captured consolidatedShortInterest response (trimmed, live 2026-06-10).
const sampleRows = `[
  {"stockSplitFlag":null,"previousShortPositionQuantity":4824122,"averageDailyVolumeQuantity":2091764,"issueName":"Agilent Technologies Inc.","currentShortPositionQuantity":5502353,"changePreviousNumber":678231,"accountingYearMonthNumber":20260515,"settlementDate":"2026-05-15","marketClassCode":"NYSE","symbolCode":"A","daysToCoverQuantity":2.63,"issuerServicesGroupExchangeCode":"A","revisionFlag":null,"changePercent":14.06},
  {"previousShortPositionQuantity":6160856,"averageDailyVolumeQuantity":4271198,"issueName":"Alcoa Corporation","currentShortPositionQuantity":5926587,"changePreviousNumber":-234269,"settlementDate":"2026-05-15","marketClassCode":"NYSE","symbolCode":"AA","daysToCoverQuantity":1.39,"changePercent":-3.8},
  {"issueName":"missing symbol — skipped","settlementDate":"2026-05-15"}
]`

func TestParseRows(t *testing.T) {
	rows, err := parseRows([]byte(sampleRows))
	if err != nil {
		t.Fatalf("parseRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (symbol-less row skipped)", len(rows))
	}
	a := rows[0]
	if a.Symbol != "A" || a.Name != "Agilent Technologies Inc." || a.Market != "NYSE" {
		t.Fatalf("row 0 identity = %+v", a)
	}
	if a.ShortQty != 5502353 || a.PrevShortQty != 4824122 || a.AvgDailyVolume != 2091764 {
		t.Fatalf("row 0 quantities = %+v", a)
	}
	if a.DaysToCover != 2.63 || a.ChangePct != 14.06 || a.SettlementDate != "2026-05-15" {
		t.Fatalf("row 0 metrics = %+v", a)
	}
	if rows[1].ChangePct != -3.8 {
		t.Fatalf("row 1 change = %v, want -3.8", rows[1].ChangePct)
	}
}

func TestParseRowsRejectsNonArray(t *testing.T) {
	if _, err := parseRows([]byte(`{"statusCode":400,"message":"nope"}`)); err == nil {
		t.Fatal("expected an error for a non-array body")
	}
}

func TestLatestSettlementCandidates(t *testing.T) {
	// 2026-06-10 (Wed): candidates must lead with May EOM (Fri 05-29, since
	// 05-31 is a Sunday → rolled back), then May mid (Fri 05-15), never a
	// weekend, never a future date, newest first.
	today := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got := LatestSettlementCandidates(today, 2)
	if len(got) == 0 {
		t.Fatal("no candidates")
	}
	if got[0] != "2026-05-29" {
		t.Fatalf("first candidate = %s, want 2026-05-29 (May EOM rolled back from Sunday)", got[0])
	}
	prev := ""
	for i, s := range got {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("candidate %d %q: %v", i, s, err)
		}
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("candidate %s is a weekend", s)
		}
		if d.After(today) {
			t.Fatalf("candidate %s is in the future", s)
		}
		if prev != "" && s >= prev {
			t.Fatalf("candidates not strictly newest-first: %s after %s", s, prev)
		}
		prev = s
	}
	// Mid-May must be among them.
	found := false
	for _, s := range got {
		if s == "2026-05-15" {
			found = true
		}
	}
	if !found {
		t.Fatalf("2026-05-15 missing from %v", got)
	}
}
