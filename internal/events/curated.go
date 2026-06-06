package events

import "time"

// Curated is the hand-maintained set of high-value events that aren't in an
// automated feed: FOMC rate-decision days and notable scheduled world events.
// Verified against official calendars; refresh by editing this file (e.g. add
// 2027 FOMC dates or new elections). Dates are the decision/headline day, UTC.
func Curated() []Event {
	const fed = "Federal Reserve"
	const fedURL = "https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm"
	const fifaURL = "https://www.fifa.com/en/tournaments/mens/worldcup/canadamexicousa2026"
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}

	out := []Event{}

	// FOMC rate decisions — the 2nd (decision/statement) day of each 2026 meeting.
	for _, d := range []time.Time{
		day(2026, time.January, 28), day(2026, time.March, 18), day(2026, time.April, 29),
		day(2026, time.June, 17), day(2026, time.July, 29), day(2026, time.September, 16),
		day(2026, time.October, 28), day(2026, time.December, 9),
	} {
		out = append(out, Event{
			ID:         "fomc-" + d.Format("2006-01-02"),
			Title:      "FOMC rate decision",
			Category:   "macro",
			Subtype:    "fomc",
			StartUTC:   d,
			AllDay:     true,
			Importance: "high",
			Region:     "US",
			SourceName: fed,
			SourceURL:  fedURL,
		})
	}

	// Notable scheduled world events (market- or attention-moving).
	out = append(out,
		Event{
			ID: "wc2026-open", Title: "FIFA World Cup 2026 — opening match",
			Category: "world", Subtype: "worldcup", StartUTC: day(2026, time.June, 11), AllDay: true,
			Importance: "med", Region: "Global", SourceName: "curated", SourceURL: fifaURL,
		},
		Event{
			ID: "wc2026-final", Title: "FIFA World Cup 2026 — final",
			Category: "world", Subtype: "worldcup", StartUTC: day(2026, time.July, 19), AllDay: true,
			Importance: "med", Region: "Global", SourceName: "curated", SourceURL: fifaURL,
		},
		Event{
			ID: "us-midterms-2026", Title: "US midterm elections",
			Category: "world", Subtype: "election", StartUTC: day(2026, time.November, 3), AllDay: true,
			Importance: "high", Region: "US", SourceName: "curated", SourceURL: "https://www.usa.gov/midterm-elections",
		},
	)
	return out
}
