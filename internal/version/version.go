// Package version holds build-time injected version metadata.
package version

import "fmt"

// These are overridden at link time, e.g.
//
//	go build -ldflags "-X bastiondeck/internal/version.Version=0.1.0 \
//	 -X bastiondeck/internal/version.Commit=abcdef -X bastiondeck/internal/version.BuildDate=2026-09-05"
var (
	Version   = "0.0.0-dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// String returns a single human readable build descriptor.
func String() string {
	return fmt.Sprintf("BastionDeck %s (commit %s, built %s)", Version, Commit, BuildDate)
}

// Short returns just version and short commit.
func Short() string {
	c := Commit
	if len(c) > 7 {
		c = c[:7]
	}
	return fmt.Sprintf("%s+%s", Version, c)
}
