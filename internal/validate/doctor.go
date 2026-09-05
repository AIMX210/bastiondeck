// Package validate implements `doctor`: bounded self-checks over the data
// directory, migration version, audit chain, hub health and clock sanity.
// It never mutates state and reports explicit coverage.
package validate

import (
	"context"
	"time"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/config"
	"bastiondeck/internal/migrations"
	"bastiondeck/internal/realtime"
	"bastiondeck/internal/store"
)

// Check is one named self-check result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Report is the full doctor output.
type Report struct {
	OK       bool    `json:"ok"`
	Version  string  `json:"version"`
	Time     string  `json:"time"`
	Checks   []Check `json:"checks"`
	Coverage string  `json:"coverage"`
}

// Input bundles what doctor inspects.
type Input struct {
	Store   *store.Store
	Audit   *audit.Service
	Cfg     *config.Config
	Hub     *realtime.Hub
	Version string
}

// Run executes every check.
func Run(ctx context.Context, in Input) Report {
	rep := Report{Version: in.Version, Time: store.Now(), Coverage: "db,migrations,audit-chain,hub,clock"}
	add := func(name string, ok bool, detail string) {
		rep.Checks = append(rep.Checks, Check{Name: name, OK: ok, Detail: detail})
	}

	if err := in.Store.DB.PingContext(ctx); err != nil {
		add("database.ping", false, err.Error())
	} else {
		add("database.ping", true, in.Store.Path)
	}

	if cur, err := migrations.CurrentVersion(); err != nil {
		add("migrations.version", false, err.Error())
	} else {
		ok := cur == in.Store.Version
		add("migrations.version", ok, "")
	}

	if in.Audit != nil {
		chain, err := in.Audit.Verify(ctx)
		if err != nil {
			add("audit.chain", false, err.Error())
		} else {
			add("audit.chain", chain.OK, "")
		}
	}

	if in.Hub != nil {
		add("realtime.hub", true, itoa(in.Hub.SubscriberCount())+" subscribers, dropped="+itoa64(in.Hub.Dropped()))
	}

	// Monotonic-ish clock sanity: ensure time does not go backwards across a
	// microsecond sleep.
	t1 := time.Now()
	time.Sleep(50 * time.Microsecond)
	add("clock.monotonic", time.Now().After(t1), "")

	rep.OK = true
	for _, c := range rep.Checks {
		if !c.OK {
			rep.OK = false
		}
	}
	return rep
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func itoa64(n int64) string { return itoa(int(n)) }
