package nasdaq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sampleResp mirrors a real Nasdaq IPO-calendar response: a priced section, an
// upcoming section nested under upcomingTable, and a filed section — each with
// the field names the parser reads.
const sampleResp = `{
  "data": {
    "priced": {
      "rows": [
        {
          "proposedTickerSymbol": "FRBT",
          "companyName": "Forbright Inc.",
          "proposedExchange": "NASDAQ Global",
          "proposedSharePrice": "18.00",
          "sharesOffered": "10,000,000",
          "dollarValueOfSharesOffered": "$180,000,000",
          "pricedDate": "06/12/2026",
          "dealStatus": "Priced"
        }
      ]
    },
    "upcoming": {
      "upcomingTable": {
        "rows": [
          {
            "proposedTickerSymbol": "ACME",
            "companyName": "Acme Robotics",
            "proposedExchange": "NYSE",
            "proposedSharePrice": "$15.00 - $17.00",
            "sharesOffered": "5,000,000",
            "dollarValueOfSharesOffered": "$80,000,000",
            "expectedPriceDate": "06/20/2026"
          }
        ]
      }
    },
    "filed": {
      "rows": [
        {
          "symbol": "ZEPH",
          "companyName": "Zephyr Labs",
          "proposedExchange": "NASDAQ",
          "dollarValueOfSharesOffered": "$50,000,000"
        }
      ]
    }
  }
}`

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(srv.Client())
	c.base = srv.URL + "/api/ipo/calendar"
	return c
}

func TestCalendarParse(t *testing.T) {
	month := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("date"); got != "2026-06" {
			t.Errorf("date param = %q, want 2026-06", got)
		}
		// The full browser header set must be present (Nasdaq returns empty
		// otherwise) — verify the load-bearing ones are sent.
		for _, h := range []string{"User-Agent", "Accept", "Origin", "Referer"} {
			if r.Header.Get(h) == "" {
				t.Errorf("missing required header %q", h)
			}
		}
		if !strings.Contains(r.Header.Get("Origin"), "nasdaq.com") {
			t.Errorf("Origin = %q, want a nasdaq.com origin", r.Header.Get("Origin"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleResp))
	})

	cal, err := c.Calendar(context.Background(), month)
	if err != nil {
		t.Fatalf("Calendar: %v", err)
	}
	if len(cal.Priced) != 1 || len(cal.Upcoming) != 1 || len(cal.Filed) != 1 {
		t.Fatalf("sections = priced %d / upcoming %d / filed %d, want 1/1/1",
			len(cal.Priced), len(cal.Upcoming), len(cal.Filed))
	}

	priced := cal.Priced[0]
	if priced.Ticker != "FRBT" || priced.Company != "Forbright Inc." {
		t.Errorf("priced row = %+v", priced)
	}
	if priced.Price != "18.00" || priced.Shares != "10,000,000" || priced.Amount != "$180,000,000" {
		t.Errorf("priced figures = %+v", priced)
	}
	if priced.Date != "06/12/2026" {
		t.Errorf("priced date = %q, want pricedDate 06/12/2026", priced.Date)
	}
	if priced.Kind != KindPriced {
		t.Errorf("priced kind = %q, want %q", priced.Kind, KindPriced)
	}

	up := cal.Upcoming[0]
	if up.Ticker != "ACME" || up.Date != "06/20/2026" { // falls back to expectedPriceDate
		t.Errorf("upcoming row = %+v", up)
	}
	if up.Kind != KindUpcoming {
		t.Errorf("upcoming kind = %q, want %q", up.Kind, KindUpcoming)
	}

	filed := cal.Filed[0]
	if filed.Ticker != "ZEPH" { // falls back to the `symbol` field
		t.Errorf("filed ticker = %q, want ZEPH (symbol fallback)", filed.Ticker)
	}
	if filed.Kind != KindFiled {
		t.Errorf("filed kind = %q, want %q", filed.Kind, KindFiled)
	}
}

func TestCalendarFaultTolerance(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "empty body (datacenter-IP block symptom)",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK) // 200 but no body
			},
		},
		{
			name: "non-200 status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"data":null}`))
			},
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{not json`))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, tc.handler)
			if _, err := c.Calendar(context.Background(), time.Now()); err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}

func TestCalendarMissingSections(t *testing.T) {
	// A quiet month: Nasdaq omits whole sections. The decode must yield empty
	// (non-nil) slices, never panic.
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"priced":{"rows":null}}}`))
	})
	cal, err := c.Calendar(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Calendar: %v", err)
	}
	if cal.Priced == nil || cal.Upcoming == nil || cal.Filed == nil {
		t.Fatalf("sections must be non-nil empty slices, got %+v", cal)
	}
	if len(cal.Priced)+len(cal.Upcoming)+len(cal.Filed) != 0 {
		t.Fatalf("expected all empty, got %+v", cal)
	}
}
