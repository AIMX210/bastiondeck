//go:build !linux

package facts

import "runtime"

func platformFacts(f *Facts) (string, int64, int64) {
	return runtime.GOOS, uptimeFallback(), 0
}

func disks() []DiskUsage { return nil }
