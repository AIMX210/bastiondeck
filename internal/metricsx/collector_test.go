package metricsx

import (
	"strings"
	"testing"
	"time"
)

func newCollector() *Collector { return &Collector{lastCPU: map[string]cpuSample{}} }

func TestCPUPercDelta(t *testing.T) {
	c := newCollector()
	now := time.Now()
	// /proc/stat cpu line: cpu user nice system idle iowait irq softirq ...
	line1 := "cpu 100 0 50 800 0 0 0"
	line2 := "cpu 200 0 100 900 0 0 0"
	if _, ok := c.cpuPercent("h", line1, now); ok {
		t.Fatal("first sample cannot yield a delta")
	}
	pct, ok := c.cpuPercent("h", line2, now.Add(time.Second))
	if !ok {
		t.Fatal("second sample must yield a delta")
	}
	// dt = (200+100+900) - (100+50+800) = 250; idle delta = 100; busy = 150
	want := 150.0 / 250.0 * 100
	if d := pct - want; d > 0.01 || d < -0.01 {
		t.Fatalf("cpu pct = %.2f want %.2f", pct, want)
	}
}

func TestCPUBadLine(t *testing.T) {
	c := newCollector()
	if _, ok := c.cpuPercent("h", "cpu 1", time.Now()); ok {
		t.Fatal("short line must not be accepted")
	}
}

func TestCPUNonMonotonic(t *testing.T) {
	c := newCollector()
	now := time.Now()
	c.cpuPercent("h", "cpu 100 0 50 800 0 0 0", now)
	// Counters went backwards: no valid delta.
	if _, ok := c.cpuPercent("h", "cpu 10 0 5 80 0 0 0", now.Add(time.Second)); ok {
		t.Fatal("non-monotonic counters must be rejected")
	}
}

func TestParseMemProcFormat(t *testing.T) {
	// Regression: real /proc/meminfo has "Key:   value kB" and the collector
	// joins MemTotal/MemAvailable onto one line; this used to never parse.
	line := "MemTotal:        1000 kB MemFree: 200 kB MemAvailable:   300 kB"
	used, total, ok := parseMem(line)
	if !ok {
		t.Fatal("expected ok for proc format")
	}
	if total != 1000*1024 {
		t.Fatalf("total = %d", total)
	}
	if used != 700*1024 {
		t.Fatalf("used = %d", used)
	}
}

func TestParseMemCompactFormat(t *testing.T) {
	line := "MemTotal:1000 MemFree:200 MemAvailable:300"
	used, total, ok := parseMem(line)
	if !ok {
		t.Fatal("expected ok")
	}
	if total != 1000*1024 || used != 700*1024 {
		t.Fatalf("used=%d total=%d", used, total)
	}
}

func TestParseFullRound(t *testing.T) {
	c := newCollector()
	out := "STAT cpu 100 0 50 800 0 0 0\n" +
		"MemTotal:        1000 kB MemAvailable: 400 kB\n" +
		"LOAD 0.5 0.4 0.3\n" +
		"/dev/sda1 1000 600 400 60% /\n"
	// First STAT seeds baseline; second round produces cpu point.
	pts := c.parse(out, "h")
	pts2 := c.parse(strings.Replace(out, "cpu 100 0 50 800 0 0 0", "cpu 200 0 100 900 0 0 0", 1), "h")
	all := append(pts, pts2...)
	kinds := map[string]int{}
	for _, p := range all {
		kinds[p.kind]++
	}
	for _, k := range []string{"mem", "load", "disk"} {
		if kinds[k] == 0 {
			t.Fatalf("missing point kind %q in %+v", k, all)
		}
	}
}

func TestParseMemMissingTotal(t *testing.T) {
	if _, _, ok := parseMem("MemAvailable: 300 kB"); ok {
		t.Fatal("without MemTotal it must fail")
	}
}

func TestDownsample(t *testing.T) {
	pts := make([]Point, 10)
	for i := range pts {
		pts[i] = Point{At: "t", Value: float64(i + 1)}
	}
	out := downsample(pts, 5)
	if len(out) != 5 {
		t.Fatalf("buckets = %d, want 5", len(out))
	}
	// First bucket averages points 1,2 → 1.5
	if out[0].Value != 1.5 {
		t.Fatalf("first avg = %v", out[0].Value)
	}
	// Under the limit: identity.
	if len(downsample(pts, 20)) != 10 {
		t.Fatal("small input must be returned as-is")
	}
	if len(downsample(nil, 5)) != 0 {
		t.Fatal("nil input → empty")
	}
}
