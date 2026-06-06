package events

import (
	"strings"
	"testing"
	"time"
)

func ids(es []Event) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}

func TestMerge(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC) }
	got := Merge(
		[]Event{{ID: "a", StartUTC: d(3)}, {ID: "b", StartUTC: d(1)}},
		[]Event{{ID: "a", StartUTC: d(9)}, {ID: "z", StartUTC: time.Time{}}, {ID: "", StartUTC: d(2)}},
	)
	if len(got) != 2 { // dup "a" collapsed, zero-time + empty-id dropped
		t.Fatalf("len=%d want 2 (%v)", len(got), ids(got))
	}
	if got[0].ID != "b" || got[1].ID != "a" { // ascending by start
		t.Errorf("order=%v want [b a]", ids(got))
	}
	if !got[1].StartUTC.Equal(d(3)) { // first "a" (d3) wins over the later dup (d9)
		t.Errorf("dedupe should keep first 'a' (d3), got %v", got[1].StartUTC)
	}
}

const sampleICS = "BEGIN:VCALENDAR\r\n" +
	"BEGIN:VEVENT\r\nUID:1\r\nDTSTART;TZID=US-Eastern:20260206T083000\r\nSUMMARY:Employment Situation\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:2\r\nDTSTART;VALUE=DATE:20260715\r\nSUMMARY:Consumer Price Index\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:3\r\nDTSTART;TZID=US-Eastern:20260210T100000\r\nSUMMARY:Metropolitan Area Employment (Monthly)\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseICS(t *testing.T) {
	got := parseICS([]byte(sampleICS))
	if len(got) != 2 { // the unmapped "Metropolitan Area" release is skipped
		t.Fatalf("len=%d want 2 (%v)", len(got), ids(got))
	}
	nfp := got[0]
	if nfp.Subtype != "nfp" || nfp.Importance != "high" || nfp.AllDay {
		t.Errorf("nfp = %+v", nfp)
	}
	if h := nfp.StartUTC.Hour(); h != 13 { // 08:30 EST (Feb) → 13:30 UTC
		t.Errorf("nfp UTC hour=%d want 13", h)
	}
	cpi := got[1]
	if cpi.Subtype != "cpi" || !cpi.AllDay || cpi.ID != "bls-cpi-2026-07-15" {
		t.Errorf("cpi = %+v", cpi)
	}
}

func TestCurated(t *testing.T) {
	got := Curated()
	if len(got) == 0 {
		t.Fatal("no curated events")
	}
	hasFOMC := false
	for _, e := range got {
		if e.ID == "" || e.StartUTC.IsZero() || e.Title == "" {
			t.Errorf("malformed curated event: %+v", e)
		}
		if e.Subtype == "fomc" {
			hasFOMC = true
		}
	}
	if !hasFOMC {
		t.Error("expected at least one FOMC event")
	}
	if !strings.HasPrefix(Curated()[0].ID, "fomc-") {
		t.Errorf("first curated id = %q, want fomc-*", Curated()[0].ID)
	}
}
