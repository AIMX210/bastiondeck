// Package schedule implements a small, dependency-free 5-field cron parser
// (minute hour day-of-month month day-of-week) supporting numbers, *, */n,
// lists and ranges. It computes the next fire time without spawning timers
// of its own (the engine ticks).
package schedule

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Expr is a parsed cron expression.
type Expr struct {
	raw   string
	mins  bitset
	hours bitset
	doms  bitset
	mons  bitset
	dows  bitset
}

type bitset uint64 // enough for 0..59

func (b *bitset) set(v int)     { *b |= 1 << uint(v) }
func (b bitset) has(v int) bool { return b&(1<<uint(v)) != 0 }

// Parse validates and parses a 5-field expression.
func Parse(s string) (*Expr, error) {
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return nil, errors.New("cron expression must have 5 fields")
	}
	e := &Expr{raw: s}
	bounds := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	targets := []*bitset{&e.mins, &e.hours, &e.doms, &e.mons, &e.dows}
	for i, f := range fields {
		bs, err := parseField(f, bounds[i][0], bounds[i][1])
		if err != nil {
			return nil, fmt.Errorf("field %d %q: %w", i+1, f, err)
		}
		*targets[i] = bs
	}
	return e, nil
}

func parseField(f string, lo, hi int) (bitset, error) {
	var b bitset
	for _, part := range strings.Split(f, ",") {
		step := 1
		rangePart := part
		if i := strings.Index(part, "/"); i >= 0 {
			rangePart = part[:i]
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("bad step %q", part)
			}
			step = n
		}
		start, end := lo, hi
		switch {
		case rangePart == "*":
			start, end = lo, hi
		case strings.Contains(rangePart, "-"):
			pp := strings.SplitN(rangePart, "-", 2)
			a, e1 := strconv.Atoi(pp[0])
			z, e2 := strconv.Atoi(pp[1])
			if e1 != nil || e2 != nil {
				return 0, fmt.Errorf("bad range %q", rangePart)
			}
			start, end = a, z
		default:
			n, err := strconv.Atoi(rangePart)
			if err != nil {
				return 0, fmt.Errorf("bad value %q", rangePart)
			}
			if strings.Contains(part, "/") {
				end = hi
			} else {
				start, end = n, n
			}
		}
		if start < lo || end > hi || start > end {
			return 0, fmt.Errorf("%d..%d outside %d..%d", start, end, lo, hi)
		}
		for v := start; v <= end; v += step {
			b.set(v)
		}
	}
	return b, nil
}

// NextAfter returns the earliest matching time strictly after t.
func (e *Expr) NextAfter(t time.Time) time.Time {
	// Start at the next minute with zero seconds.
	cand := t.Truncate(time.Minute).Add(time.Minute)
	loc := t.Location()
	// Search up to 4 years ahead (covers leap-year cycles).
	limit := cand.AddDate(4, 0, 0)
	for cand.Before(limit) {
		if !e.mons.has(int(cand.Month())) {
			cand = firstOfNextMonth(cand)
			continue
		}
		dom := cand.Day()
		dow := int(cand.Weekday())
		domStar := e.doms == allBits(1, 31)
		dowStar := e.dows == allBits(0, 6)
		dayOK := true
		if !domStar && !dowStar {
			dayOK = e.doms.has(dom) || e.dows.has(dow) // OR semantics per standard cron
		} else if !domStar {
			dayOK = e.doms.has(dom)
		} else if !dowStar {
			dayOK = e.dows.has(dow)
		}
		if !dayOK {
			cand = cand.AddDate(0, 0, 1)
			cand = time.Date(cand.Year(), cand.Month(), cand.Day(), 0, 0, 0, 0, loc)
			continue
		}
		if !e.hours.has(cand.Hour()) {
			cand = cand.Add(time.Hour)
			cand = time.Date(cand.Year(), cand.Month(), cand.Day(), cand.Hour(), 0, 0, 0, loc)
			continue
		}
		if !e.mins.has(cand.Minute()) {
			cand = cand.Add(time.Minute)
			continue
		}
		return cand
	}
	return time.Time{}
}

func firstOfNextMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
}

func allBits(lo, hi int) bitset {
	var b bitset
	for v := lo; v <= hi; v++ {
		b.set(v)
	}
	return b
}

// String returns the normalised source expression.
func (e *Expr) String() string { return e.raw }
