package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wombow-ai/tickwind/internal/nasdaq"
)

// fakeIPOSource returns a fixed calendar, for the /v1/ipo handler test.
type fakeIPOSource struct {
	cal nasdaq.Calendar
	at  time.Time
}

func (f fakeIPOSource) Calendar() (nasdaq.Calendar, time.Time) { return f.cal, f.at }

func TestGetIPONilSafeEmpty(t *testing.T) {
	// No IPO source set → 200 with well-formed empty sections (never null).
	srv := httptest.NewServer(newBareServer())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/v1/ipo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Priced    []nasdaq.IPO `json:"priced"`
		Upcoming  []nasdaq.IPO `json:"upcoming"`
		Filed     []nasdaq.IPO `json:"filed"`
		UpdatedAt string       `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Priced == nil || got.Upcoming == nil || got.Filed == nil {
		t.Fatalf("sections must be [] not null, got %+v", got)
	}
}

func TestGetIPOServesCalendar(t *testing.T) {
	s := newBareServer()
	s.SetIPO(fakeIPOSource{
		cal: nasdaq.Calendar{
			Priced:   []nasdaq.IPO{{Ticker: "FRBT", Company: "Forbright", Price: "$18.00", Kind: nasdaq.KindPriced}},
			Upcoming: []nasdaq.IPO{{Ticker: "ACME", Kind: nasdaq.KindUpcoming}},
			Filed:    []nasdaq.IPO{},
		},
		at: time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	})
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/v1/ipo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Priced    []nasdaq.IPO `json:"priced"`
		Upcoming  []nasdaq.IPO `json:"upcoming"`
		Filed     []nasdaq.IPO `json:"filed"`
		UpdatedAt string       `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Priced) != 1 || got.Priced[0].Ticker != "FRBT" || got.Priced[0].Price != "$18.00" {
		t.Fatalf("priced = %+v", got.Priced)
	}
	if len(got.Upcoming) != 1 || got.Upcoming[0].Ticker != "ACME" {
		t.Fatalf("upcoming = %+v", got.Upcoming)
	}
	if got.UpdatedAt != "2026-06-13T12:00:00Z" {
		t.Fatalf("updated_at = %q", got.UpdatedAt)
	}
}
