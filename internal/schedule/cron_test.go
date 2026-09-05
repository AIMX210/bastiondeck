package schedule

import (
	"testing"
	"time"
)

func TestParseValidation(t *testing.T) {
	good := []string{"* * * * *", "*/5 * * * *", "0 9 * * 1-5", "0,30 */2 1,15 * *"}
	for _, g := range good {
		if _, err := Parse(g); err != nil {
			t.Errorf("expected %q valid: %v", g, err)
		}
	}
	bad := []string{"", "60 * * * *", "* * * *", "a b c d e"}
	for _, b := range bad {
		if _, err := Parse(b); err == nil {
			t.Errorf("expected %q invalid", b)
		}
	}
}

func TestNextEveryFiveMinutes(t *testing.T) {
	e, _ := Parse("*/5 * * * *")
	from := time.Date(2026, 9, 5, 10, 3, 0, 0, time.UTC)
	next := e.NextAfter(from)
	want := time.Date(2026, 9, 5, 10, 5, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}

func TestNextWeekdayNineAM(t *testing.T) {
	e, _ := Parse("0 9 * * 1") // every Monday 09:00
	// 2026-09-05 is a Saturday.
	from := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	next := e.NextAfter(from)
	if next.Weekday() != time.Monday || next.Hour() != 9 || next.Minute() != 0 {
		t.Fatalf("expected next Monday 09:00, got %v %s", next, next.Weekday())
	}
}

func TestNextMonthRollover(t *testing.T) {
	e, _ := Parse("0 0 1 * *") // first of month midnight
	from := time.Date(2026, 1, 31, 23, 59, 0, 0, time.UTC)
	next := e.NextAfter(from)
	want := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v want %v", next, want)
	}
}
