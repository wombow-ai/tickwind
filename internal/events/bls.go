package events

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// blsICS is the U.S. Bureau of Labor Statistics release schedule as iCalendar
// (public-domain US-government data). It carries NFP, CPI, PPI, JOLTS, etc.
const blsICS = "https://www.bls.gov/schedule/news_release/bls.ics"

// blsScheduleURL is the human-readable schedule page used for attribution.
const blsScheduleURL = "https://www.bls.gov/schedule/"

// FetchBLS downloads the BLS schedule and returns the mapped high-signal macro
// events. A descriptive User-Agent is sent (good citizenship for gov sites).
func FetchBLS(ctx context.Context, hc *http.Client) ([]Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blsICS, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Tickwind/1.0 (+https://tickwind.com)")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("events: fetch bls: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("events: bls: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("events: read bls: %w", err)
	}
	return parseICS(data), nil
}

// parseICS walks the VEVENTs in an iCalendar body and returns the high-signal
// macro events (unmapped/low-value releases are dropped to keep the timeline
// dense). It never panics on format variations — unknown lines are ignored.
func parseICS(data []byte) []Event {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.UTC
	}
	var out []Event
	var summary, dtstart string
	inEvent := false

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "BEGIN:VEVENT":
			inEvent, summary, dtstart = true, "", ""
		case line == "END:VEVENT":
			if inEvent {
				if ev, ok := eventFromVEVENT(summary, dtstart, loc); ok {
					out = append(out, ev)
				}
			}
			inEvent = false
		case inEvent && strings.HasPrefix(line, "SUMMARY:"):
			summary = strings.TrimSpace(line[len("SUMMARY:"):])
		case inEvent && strings.HasPrefix(line, "DTSTART"):
			dtstart = line
		}
	}
	return out
}

// eventFromVEVENT maps one VEVENT's SUMMARY + DTSTART to an Event, or ok=false
// if the release isn't one we surface or the date can't be parsed.
func eventFromVEVENT(summary, dtstartLine string, loc *time.Location) (Event, bool) {
	subtype, importance, title, ok := classifyBLS(summary)
	if !ok {
		return Event{}, false
	}
	when, allDay, ok := parseDTSTART(dtstartLine, loc)
	if !ok {
		return Event{}, false
	}
	return Event{
		ID:         "bls-" + subtype + "-" + when.Format("2006-01-02"),
		Title:      title,
		Category:   "macro",
		Subtype:    subtype,
		StartUTC:   when,
		AllDay:     allDay,
		Importance: importance,
		Region:     "US",
		SourceName: "BLS",
		SourceURL:  blsScheduleURL,
	}, true
}

// parseDTSTART parses an iCalendar DTSTART line into a UTC time, returning
// (when, allDay, ok). It handles the BLS form "DTSTART;TZID=US-Eastern:2025...T..."
// (Eastern local), the UTC form "...Z", and date-only "DTSTART;VALUE=DATE:2025...".
func parseDTSTART(line string, loc *time.Location) (time.Time, bool, bool) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return time.Time{}, false, false
	}
	params, val := line[:colon], strings.TrimSpace(line[colon+1:])

	if strings.Contains(params, "VALUE=DATE") || len(val) == 8 { // date-only
		if t, err := time.Parse("20060102", val); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true, true
		}
		return time.Time{}, false, false
	}
	if strings.HasSuffix(val, "Z") { // explicit UTC
		if t, err := time.Parse("20060102T150405Z", val); err == nil {
			return t.UTC(), false, true
		}
	}
	if t, err := time.ParseInLocation("20060102T150405", val, loc); err == nil { // local (TZID)
		return t.UTC(), false, true
	}
	return time.Time{}, false, false
}

// classifyBLS maps a BLS release SUMMARY to (subtype, importance, title), or
// ok=false to drop low-signal releases.
func classifyBLS(summary string) (subtype, importance, title string, ok bool) {
	s := strings.ToLower(summary)
	switch {
	case strings.Contains(s, "employment situation"):
		return "nfp", "high", "US Jobs report (NFP)", true
	case strings.Contains(s, "consumer price index"):
		return "cpi", "high", "US CPI", true
	case strings.Contains(s, "producer price index"):
		return "ppi", "med", "US PPI", true
	case strings.Contains(s, "gross domestic product"):
		return "gdp", "high", "US GDP", true
	case strings.Contains(s, "job openings"):
		return "jobs", "med", "US JOLTS", true
	case strings.Contains(s, "employment cost"):
		return "eci", "med", "US Employment Cost Index", true
	case strings.Contains(s, "real earnings"):
		return "earnings", "med", "US Real Earnings", true
	default:
		return "", "", "", false
	}
}
