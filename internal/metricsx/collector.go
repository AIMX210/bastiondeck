// Package metricsx collects lightweight Linux /proc metrics over the
// connector layer, stores 7 days of points and serves bucketed queries. It is
// intentionally not a monitoring system: no alerting, no long-term storage.
package metricsx

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"time"

	"bastiondeck/internal/connector"
	"bastiondeck/internal/store"
)

func storeNow() string { return store.Now() }

// Retention is the rolling window kept on disk.
const Retention = 7 * 24 * time.Hour

const procScript = `echo "STAT $(head -1 /proc/stat)"
grep -E 'MemTotal|MemAvailable' /proc/meminfo | tr '\n' ' '; echo
echo "LOAD $(cat /proc/loadavg)"
df -P -k | tail -n +2`

// Collector gathers and queries metrics.
type Collector struct {
	db       *sql.DB
	resolver connector.Resolver

	mu      sync.Mutex
	lastCPU map[string]cpuSample
}

type cpuSample struct {
	total, idle uint64
	at          time.Time
}

// New constructs a collector.
func New(db *sql.DB, resolver connector.Resolver) *Collector {
	return &Collector{db: db, resolver: resolver, lastCPU: map[string]cpuSample{}}
}

// Point is a stored metric value.
type Point struct {
	At    string  `json:"at"`
	Value float64 `json:"value"`
	Extra string  `json:"extra,omitempty"`
}

// CollectHost runs one collection round for a host.
func (c *Collector) CollectHost(ctx context.Context, hostID string) error {
	cli, err := c.resolver.Connect(ctx, hostID)
	if err != nil {
		return err
	}
	res, err := cli.Exec(ctx, connector.ExecRequest{Command: procScript, Timeout: 10 * time.Second})
	if err != nil {
		return err
	}
	if res.Status != connector.StatusSuccess {
		return nil
	}
	points := c.parse(string(res.Stdout), hostID)
	for _, p := range points {
		_, err := c.db.ExecContext(ctx,
			`INSERT INTO metric_points(host_id,at,kind,value,extra_json) VALUES(?,?,?,?,?)`,
			hostID, storeNow(), p.kind, p.value, p.extra)
		if err != nil {
			return err
		}
	}
	return nil
}

type rawPoint struct {
	kind, extra string
	value       float64
}

func (c *Collector) parse(out, hostID string) []rawPoint {
	var pts []rawPoint
	now := time.Now()
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimRight(ln, "\r")
		switch {
		case strings.HasPrefix(ln, "STAT "):
			if pct, ok := c.cpuPercent(hostID, strings.TrimPrefix(ln, "STAT "), now); ok {
				pts = append(pts, rawPoint{kind: "cpu", value: pct})
			}
		case strings.HasPrefix(ln, "MemTotal"):
			if used, total, ok := parseMem(ln); ok {
				pct := 0.0
				if total > 0 {
					pct = float64(used) / float64(total) * 100
				}
				pts = append(pts, rawPoint{kind: "mem", value: pct,
					extra: `{"usedBytes":` + strconv.FormatInt(used, 10) + `,"totalBytes":` + strconv.FormatInt(total, 10) + `}`})
			}
		case strings.HasPrefix(ln, "LOAD "):
			fields := strings.Fields(ln)
			if len(fields) >= 2 {
				if v, err := strconv.ParseFloat(fields[1], 64); err == nil {
					pts = append(pts, rawPoint{kind: "load", value: v})
				}
			}
		case strings.Contains(ln, "/"):
			fields := strings.Fields(ln)
			if len(fields) >= 6 {
				total, _ := strconv.ParseInt(fields[1], 10, 64)
				used, _ := strconv.ParseInt(fields[2], 10, 64)
				avail, _ := strconv.ParseInt(fields[3], 10, 64)
				mount := fields[len(fields)-1]
				pct := 0.0
				if total > 0 {
					pct = float64(used) / float64(total) * 100
				}
				pts = append(pts, rawPoint{kind: "disk", value: pct, extra: `{"mount":"` + mount + `","totalBytes":` + strconv.FormatInt(total*1024, 10) +
					`,"usedBytes":` + strconv.FormatInt(used*1024, 10) +
					`,"availBytes":` + strconv.FormatInt(avail*1024, 10) + `}`})
			}
		}
	}
	return pts
}

// cpuPercent computes utilisation between two /proc/stat samples.
func (c *Collector) cpuPercent(hostID, statLine string, now time.Time) (float64, bool) {
	fields := strings.Fields(statLine)
	if len(fields) < 5 {
		return 0, false
	}
	var total, idle uint64
	for i, f := range fields[1:] {
		v, _ := strconv.ParseUint(f, 10, 64)
		total += v
		if i == 3 { // idle is the 4th numeric column
			idle = v
		}
	}
	c.mu.Lock()
	prev, ok := c.lastCPU[hostID]
	c.lastCPU[hostID] = cpuSample{total: total, idle: idle, at: now}
	c.mu.Unlock()
	if !ok || total <= prev.total {
		return 0, false
	}
	dt := total - prev.total
	di := idle - prev.idle
	if dt == 0 {
		return 0, false
	}
	return float64(dt-di) / float64(dt) * 100, true
}

func parseMem(line string) (used, total int64, ok bool) {
	var t, a int64
	// /proc/meminfo looks like "MemTotal:       1000 kB" and the collection
	// script joins MemTotal/MemAvailable onto one line, so after Fields() the
	// colon-bearing key and its numeric value are adjacent tokens. The compact
	// "MemTotal:1000" form is accepted as well.
	fields := strings.Fields(line)
	for i := 0; i < len(fields); i++ {
		key := strings.TrimSuffix(fields[i], ":")
		valTok := ""
		if parts := strings.SplitN(fields[i], ":", 2); len(parts) == 2 && parts[1] != "" {
			key, valTok = parts[0], parts[1]
		} else if i+1 < len(fields) {
			valTok = fields[i+1]
		}
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(valTok), 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			t = v * 1024
		case "MemAvailable":
			a = v * 1024
		}
	}
	if t == 0 {
		return 0, 0, false
	}
	return t - a, t, true
}

// Query returns points in a window, optionally downsampled by averaging into
// roughly maxBuckets buckets.
func (c *Collector) Query(ctx context.Context, hostID, kind, from, to string, maxBuckets int) ([]Point, error) {
	if maxBuckets <= 0 {
		maxBuckets = 120
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT at,value,extra_json FROM metric_points WHERE host_id=? AND kind=? AND at>=? AND at<=? ORDER BY at`,
		hostID, kind, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []Point
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.At, &p.Value, &p.Extra); err != nil {
			return nil, err
		}
		all = append(all, p)
	}
	return downsample(all, maxBuckets), rows.Err()
}

func downsample(in []Point, maxBuckets int) []Point {
	if len(in) <= maxBuckets {
		return in
	}
	bucket := len(in) / maxBuckets
	if bucket < 1 {
		bucket = 1
	}
	out := make([]Point, 0, maxBuckets+1)
	for i := 0; i < len(in); i += bucket {
		end := i + bucket
		if end > len(in) {
			end = len(in)
		}
		var sum float64
		for _, p := range in[i:end] {
			sum += p.Value
		}
		n := end - i
		out = append(out, Point{At: in[i].At, Value: sum / float64(n), Extra: in[end-1].Extra})
	}
	return out
}

// Prune removes points older than the retention window.
func (c *Collector) Prune(ctx context.Context) (int64, error) {
	cut := time.Now().UTC().Add(-Retention).Format(time.RFC3339)
	res, err := c.db.ExecContext(ctx, `DELETE FROM metric_points WHERE at<?`, cut)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
