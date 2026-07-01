package research

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/store"
)

type fakeEarnings struct{ rows []store.Earning }

// ListEarnings mimics the store's forward-window read: only rows in [from, to).
func (f fakeEarnings) ListEarnings(_ context.Context, from, to time.Time) ([]store.Earning, error) {
	var out []store.Earning
	for _, e := range f.rows {
		if !e.Date.Before(from) && e.Date.Before(to) {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeMatEvents struct{ evs []edgar.MaterialEvent }

func (f fakeMatEvents) MaterialEventsN(_ context.Context, _ string, _ int) ([]edgar.MaterialEvent, error) {
	return f.evs, nil
}

func f64(v float64) *float64 { return &v }

func TestAssembleCatalysts(t *testing.T) {
	future := time.Now().UTC().AddDate(0, 0, 20)
	farther := time.Now().UTC().AddDate(0, 0, 40)
	past := time.Now().UTC().AddDate(0, 0, -30)

	src := Sources{
		Earnings: fakeEarnings{rows: []store.Earning{
			{Ticker: "AAPL", Date: past, Hour: "amc"},                           // ignored (past)
			{Ticker: "AAPL", Date: farther, Hour: "bmo"},                        // later future
			{Ticker: "AAPL", Date: future, Hour: "amc", EPSEstimate: f64(1.93)}, // the next one
		}},
		MaterialEvents: fakeMatEvents{evs: []edgar.MaterialEvent{
			{Form: "8-K", FiledDate: "2026-06-20", Items: []edgar.EventItem{{Code: "5.02", LabelEN: "Officer change", LabelZH: "高管变动"}}, AccessionURL: "https://sec.gov/x"},
			{Form: "8-K", FiledDate: "2026-06-10", Items: []edgar.EventItem{{Code: "2.02", LabelEN: "Results", LabelZH: "业绩"}}},
			{Form: "8-K", FiledDate: "2026-06-01", Items: []edgar.EventItem{{Code: "1.01", LabelEN: "Material agreement", LabelZH: "重大协议"}}},
			{Form: "8-K", FiledDate: "2026-05-20", Items: []edgar.EventItem{{Code: "8.01", LabelEN: "Other", LabelZH: "其他"}}}, // 4th → dropped by cap
		}},
	}

	sec := assembleCatalysts(context.Background(), "AAPL", src, "en")
	if sec.Key != "catalysts" {
		t.Fatalf("key = %q, want catalysts", sec.Key)
	}

	byKey := map[string]Fact{}
	for _, f := range sec.Facts {
		byKey[f.Key] = f
	}

	ne, ok := byKey["next_earnings"]
	if !ok {
		t.Fatalf("no next_earnings fact; facts = %+v", sec.Facts)
	}
	if !strings.Contains(ne.Value, "est. EPS $1.93") {
		t.Errorf("next_earnings value missing est EPS: %q", ne.Value)
	}
	if !strings.Contains(ne.Value, "After close") {
		t.Errorf("next_earnings value missing session label: %q", ne.Value)
	}
	// It must be the SOONEST future date (future, not farther) → its AsOf is `future`.
	if ne.AsOf != future.Format("2006-01-02") {
		t.Errorf("next_earnings picked the wrong date: as-of %q, want %q", ne.AsOf, future.Format("2006-01-02"))
	}

	// At most catalystMaxFilings (3) recent-8k facts, newest label present.
	var filings int
	for k := range byKey {
		if strings.HasPrefix(k, "recent_8k_") {
			filings++
		}
	}
	if filings != catalystMaxFilings {
		t.Errorf("recent 8-K facts = %d, want %d (cap)", filings, catalystMaxFilings)
	}
	if got := byKey["recent_8k_1"].Value; !strings.Contains(got, "Officer change") || !strings.Contains(got, "2026-06-20") {
		t.Errorf("recent_8k_1 value = %q, want the Go label + date", got)
	}

	// Only-past earnings → no next_earnings fact.
	pastOnly := Sources{Earnings: fakeEarnings{rows: []store.Earning{{Ticker: "AAPL", Date: past}}}}
	if s := assembleCatalysts(context.Background(), "AAPL", pastOnly, "en"); len(s.Facts) != 0 {
		t.Errorf("past-only earnings should yield no facts, got %+v", s.Facts)
	}

	// Nil providers → empty section (addSection drops it upstream).
	if s := assembleCatalysts(context.Background(), "AAPL", Sources{}, "en"); len(s.Facts) != 0 {
		t.Errorf("nil providers should yield an empty section, got %+v", s.Facts)
	}
}
