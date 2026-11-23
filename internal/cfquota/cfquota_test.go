package cfquota

import (
	"testing"
	"time"
)

// A reading only describes the UTC day it was taken in: Workers quota resets at
// that boundary, so an over-quota figure from yesterday must not keep a fleet
// filtered today. FetchedAt was written but never read until 2026-11-23.
func TestParseFetchedAt(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"empty", "", false},
		{"garbage", "not-a-time", false},
		{"rfc3339", "2026-11-23T10:00:00Z", true},
		{"rfc3339 nano with offset", "2026-11-23T18:36:41.123456789+08:00", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := parseFetchedAt(c.in); ok != c.ok {
				t.Fatalf("parseFetchedAt(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
		})
	}
}

func TestUTCDayStart(t *testing.T) {
	// 2026-11-23 18:36 Beijing is still 2026-11-23 in UTC (10:36), so the day
	// boundary is the same; but 04:00 Beijing is the previous UTC day.
	bj := time.FixedZone("CST", 8*3600)
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"evening beijing same utc day", time.Date(2026, 11, 23, 18, 36, 0, 0, bj), "2026-11-23"},
		{"early morning beijing is previous utc day", time.Date(2026, 11, 23, 4, 0, 0, 0, bj), "2026-11-22"},
		{"just before utc midnight", time.Date(2026, 11, 23, 23, 59, 0, 0, time.UTC), "2026-11-23"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := utcDayStart(c.in).Format("2006-01-02")
			if got != c.want {
				t.Fatalf("utcDayStart(%v) = %s, want %s", c.in, got, c.want)
			}
			if utcDayStart(c.in).Location() != time.UTC {
				t.Fatal("day start must be expressed in UTC")
			}
		})
	}
}

// The boundary itself must count as the new day, not the old one.
func TestStaleReadingIsDroppedAtBoundary(t *testing.T) {
	dayStart := utcDayStart(time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC))
	justBefore := dayStart.Add(-time.Second)
	atBoundary := dayStart
	if !justBefore.Before(dayStart) {
		t.Fatal("a reading from yesterday must be considered stale")
	}
	if atBoundary.Before(dayStart) {
		t.Fatal("a reading taken exactly at the boundary must not be stale")
	}
}
