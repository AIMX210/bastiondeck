package schedule

import (
	"testing"
	"time"
)

func TestParseRejects(t *testing.T) {
	bad := []string{
		"", "* * * *", // too few fields
		"* * * * * *",  // too many
		"60 * * * *",   // minute out of range
		"* 24 * * *",   // hour out of range
		"* * 32 * *",   // dom out of range
		"* * * 13 *",   // month out of range
		"* * * * 8",    // dow out of range
		"a * * * *",    // not a number/range
		"1-99 * * * *", // range beyond bound
	}
	for _, expr := range bad {
		if _, err := Parse(expr); err == nil {
			t.Fatalf("expected parse error for %q", expr)
		}
	}
}

func TestParseAccepts(t *testing.T) {
	ok := []string{
		"* * * * *",
		"0 0 1 1 *",
		"*/15 9-17 * * 1-5",
		"5,35 * * * 0,6",
		"0 0 * * 0",
	}
	for _, expr := range ok {
		e, err := Parse(expr)
		if err != nil {
			t.Fatalf("expected %q to parse: %v", expr, err)
		}
		if e.String() != expr {
			t.Fatalf("roundtrip = %q want %q", e.String(), expr)
		}
	}
}

func TestNextAfterSteps(t *testing.T) {
	// 2026-09-05 is a Saturday.
	base := time.Date(2026, 9, 5, 13, 30, 0, 0, time.UTC)
	cases := []struct {
		expr string
		want time.Time
	}{
		{"0 * * * *", time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)},
		{"*/15 * * * *", time.Date(2026, 9, 5, 13, 45, 0, 0, time.UTC)},
		{"0 9 * * 1", time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)},  // next Monday
		{"0 0 1 * *", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}, // next 1st
	}
	for _, tc := range cases {
		e, err := Parse(tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		got := e.NextAfter(base)
		if !got.Equal(tc.want) {
			t.Fatalf("%s: got %v want %v", tc.expr, got, tc.want)
		}
	}
}

func TestNextAfterAlwaysAdvances(t *testing.T) {
	e, _ := Parse("* * * * *")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := now
	for i := 0; i < 1000; i++ {
		nxt := e.NextAfter(cur)
		if !nxt.After(cur) {
			t.Fatalf("did not advance at %v → %v", cur, nxt)
		}
		cur = nxt
	}
}
