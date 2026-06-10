package finnhub

import (
	"testing"
	"time"
)

func TestParseEarningsCalendar(t *testing.T) {
	data := []byte(`{"earningsCalendar":[
		{"date":"2026-06-12","symbol":"AAPL","hour":"amc","epsEstimate":1.5,"epsActual":null,"revenueEstimate":120000000000,"revenueActual":null},
		{"date":"2026-06-13","symbol":"MSFT","hour":"bmo","epsEstimate":2.9},
		{"date":"","symbol":"NODATE"},
		{"date":"2026-06-14","symbol":""}
	]}`)
	got, err := parseEarningsCalendar(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 { // the two malformed rows are dropped
		t.Fatalf("got %d earnings, want 2: %+v", len(got), got)
	}
	a := got[0]
	if a.Ticker != "AAPL" || a.Hour != "amc" {
		t.Errorf("AAPL row wrong: %+v", a)
	}
	if !a.Date.Equal(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AAPL date = %v, want 2026-06-12", a.Date)
	}
	if a.EPSEstimate == nil || *a.EPSEstimate != 1.5 {
		t.Errorf("AAPL epsEstimate = %v, want 1.5", a.EPSEstimate)
	}
	if a.EPSActual != nil {
		t.Errorf("AAPL epsActual = %v, want nil (not yet reported)", *a.EPSActual)
	}
	if got[1].Ticker != "MSFT" || got[1].Hour != "bmo" {
		t.Errorf("MSFT row wrong: %+v", got[1])
	}
}

func TestParseEarningsCalendarEmpty(t *testing.T) {
	got, err := parseEarningsCalendar([]byte(`{"earningsCalendar":[]}`))
	if err != nil || len(got) != 0 {
		t.Fatalf("empty calendar = %+v (err %v), want 0 rows", got, err)
	}
}

func TestParseQuote(t *testing.T) {
	// Real /quote shape: c=last consolidated price, pc=prev close, t=unix seconds.
	p, pc, at, ok, err := parseQuote([]byte(`{"c":261.74,"h":263.31,"l":260.68,"o":261.07,"pc":259.45,"t":1582641000}`))
	if err != nil || !ok {
		t.Fatalf("parseQuote: ok=%v err=%v", ok, err)
	}
	if p != 261.74 || pc != 259.45 {
		t.Errorf("price/pc = %v/%v, want 261.74/259.45", p, pc)
	}
	if at.Unix() != 1582641000 {
		t.Errorf("at = %v, want unix 1582641000", at)
	}
	// Unknown symbol → zeroes → ok=false, no error.
	if _, _, _, ok, err := parseQuote([]byte(`{"c":0,"pc":0,"t":0}`)); ok || err != nil {
		t.Errorf("zero quote: ok=%v err=%v, want false/nil", ok, err)
	}
}
