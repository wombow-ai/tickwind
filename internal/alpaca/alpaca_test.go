package alpaca

import (
	"testing"
	"time"
)

func TestSessionAt(t *testing.T) {
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no tz data: %v", err)
	}
	c := New("key", "secret", "", "iex")

	// 2026-06-08 is a Monday; 2026-06-06 is a Saturday.
	tests := []struct {
		name string
		ts   time.Time
		want string
	}{
		{"pre-market", time.Date(2026, 6, 8, 5, 0, 0, 0, et), "pre"},
		{"regular", time.Date(2026, 6, 8, 10, 0, 0, 0, et), "regular"},
		{"after-hours", time.Date(2026, 6, 8, 17, 0, 0, 0, et), "post"},
		{"overnight", time.Date(2026, 6, 8, 2, 0, 0, 0, et), "overnight"},
		{"weekend", time.Date(2026, 6, 6, 12, 0, 0, 0, et), "closed"},
		{"zero time", time.Time{}, "closed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.sessionAt(tc.ts); got != tc.want {
				t.Fatalf("sessionAt(%v) = %q; want %q", tc.ts, got, tc.want)
			}
		})
	}
}
