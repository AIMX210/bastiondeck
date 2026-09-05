// Package facts gathers local host facts without shelling out where possible.
package facts

import (
	"encoding/json"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Facts is the agent-side view of the local machine.
type Facts struct {
	Hostname string            `json:"hostname"`
	OS       string            `json:"os"`
	Kernel   string            `json:"kernel"`
	Arch     string            `json:"arch"`
	UptimeS  int64             `json:"uptimeSec"`
	CPUCores int               `json:"cpuCores"`
	MemTotal int64             `json:"memTotal"`
	Disk     []DiskUsage       `json:"disk"`
	Extra    map[string]string `json:"extra,omitempty"`
}

// DiskUsage mirrors the server DTO.
type DiskUsage struct {
	Filesystem string `json:"filesystem"`
	Mount      string `json:"mount"`
	Total      int64  `json:"total"`
	Used       int64  `json:"used"`
	Available  int64  `json:"available"`
}

// Gather collects facts for the current host.
func Gather() Facts {
	f := Facts{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
		Extra:    map[string]string{},
	}
	if hn, err := os.Hostname(); err == nil {
		f.Hostname = hn
	}
	f.Kernel, f.UptimeS, f.MemTotal = platformFacts(&f)
	f.Disk = disks()
	return f
}

// JSON marshals facts.
func (f Facts) JSON() json.RawMessage {
	b, _ := json.Marshal(f)
	return b
}

// bootedAt tracks process-relative uptime fallback.
var startedAt = time.Now()

func uptimeFallback() int64 { return int64(time.Since(startedAt).Seconds()) }

func parseMeminfo() map[string]int64 {
	out := map[string]int64{}
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		out[strings.TrimSpace(parts[0])] = n * 1024 // kB -> bytes
	}
	return out
}

var _ = syscall.Sysinfo_t{}
