package cron

import (
	"testing"
	"time"
)

// TestMonthAdvanceResetsDay verifies that when the month field does not match,
// the Next algorithm resets the day to 1 before adding one month. Without this
// reset, a high day value (e.g., 30 or 31) causes date normalization to skip
// an entire month when the next month has fewer days.
func TestMonthAdvanceResetsDay(t *testing.T) {
	// "0 0 1 3 *" fires on March 1 at midnight.
	// from = 2026-01-30T12:00:00Z => t = Jan 30 12:01.
	// Jan (month=1) not in {3} => advance month.
	// Correct: Date(2026,1,1,...).AddDate(0,1,0) = Feb 1, then Feb not in {3},
	//          Date(2026,2,1,...).AddDate(0,1,0) = Mar 1, day=1 matches => 2026-03-01.
	// Bug: Date(2026,1,30,...).AddDate(0,1,0) = "Feb 30" normalized to Mar 2 => skip Mar 1.
	s, _ := Parse("0 0 1 3 *")
	from := time.Date(2026, 1, 30, 12, 0, 0, 0, time.UTC)
	got, err := s.Next(from)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

// TestMonthAdvanceFromDay31 uses a day-31 starting point crossing a short month.
func TestMonthAdvanceFromDay31(t *testing.T) {
	// "0 0 15 6 *" fires on June 15.
	// from = 2026-03-31T00:00:00Z => t = Mar 31 00:01.
	// Mar not in {6} => advance month.
	// Correct: reset to Mar 1, +1 month = Apr 1. Then Apr not in {6}, May, Jun matches.
	// Bug: Mar 31 +1 month = "Apr 31" = May 1 => skips April.
	s, _ := Parse("0 0 15 6 *")
	from := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	got, err := s.Next(from)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	want := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}
