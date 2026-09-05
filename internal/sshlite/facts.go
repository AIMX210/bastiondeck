package sshlite

import (
	"context"
	"strconv"
	"strings"
	"time"

	"bastiondeck/internal/connector"
)

// factsScript prints labelled lines that Facts parses. Using a single
// round-trip keeps collection cheap and bounded.
const factsScript = `echo "BDK_HOST=$(hostname 2>/dev/null)"
echo "BDK_UNAME=$(uname -srm 2>/dev/null)"
echo "BDK_ARCH=$(uname -m 2>/dev/null)"
echo "BDK_UP=$(cut -d. -f1 /proc/uptime 2>/dev/null || sysctl -n kern.boottime 2>/dev/null)"
echo "BDK_CORES=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null)"
echo "BDK_CPU=$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2 | sed 's/^ //')"
echo "BDK_MEM=$(awk '/MemTotal/{print $2}' /proc/meminfo 2>/dev/null)"
echo "BDK_DF_BEGIN"
df -P -k 2>/dev/null | tail -n +2
echo "BDK_DF_END"`

// Facts collects lightweight host facts in one exec round trip.
func (c *Client) Facts(ctx context.Context) (*connector.Facts, error) {
	tctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	res, err := c.Exec(tctx, connector.ExecRequest{Command: factsScript, Timeout: 12 * time.Second})
	if err != nil || res.Status != connector.StatusSuccess {
		if res != nil {
			return nil, &FactsError{res.Status, res.ErrorCode, string(res.Stderr)}
		}
		return nil, err
	}
	f := parseFacts(string(res.Stdout))
	return f, nil
}

// FactsError explains why facts collection failed.
type FactsError struct {
	Status, Code, Stderr string
}

func (e *FactsError) Error() string { return "facts: " + e.Status + " " + e.Code + " " + e.Stderr }

func parseFacts(out string) *connector.Facts {
	f := &connector.Facts{Extra: map[string]string{}}
	lines := strings.Split(out, "\n")
	inDF := false
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		switch {
		case strings.HasPrefix(ln, "BDK_HOST="):
			f.Hostname = strings.TrimPrefix(ln, "BDK_HOST=")
		case strings.HasPrefix(ln, "BDK_UNAME="):
			f.OS = strings.TrimPrefix(ln, "BDK_UNAME=")
		case strings.HasPrefix(ln, "BDK_ARCH="):
			f.Arch = strings.TrimPrefix(ln, "BDK_ARCH=")
		case strings.HasPrefix(ln, "BDK_UP="):
			f.UptimeS, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(ln, "BDK_UP=")), 10, 64)
		case strings.HasPrefix(ln, "BDK_CORES="):
			f.CPUCores, _ = strconv.Atoi(strings.TrimPrefix(ln, "BDK_CORES="))
		case strings.HasPrefix(ln, "BDK_CPU="):
			f.CPUModel = strings.TrimPrefix(ln, "BDK_CPU=")
		case strings.HasPrefix(ln, "BDK_MEM="):
			kb, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(ln, "BDK_MEM=")), 10, 64)
			f.MemTotal = kb * 1024
		case ln == "BDK_DF_BEGIN":
			inDF = true
		case ln == "BDK_DF_END":
			inDF = false
		case inDF && ln != "":
			if d, ok := parseDFLine(ln); ok {
				f.Disk = append(f.Disk, d)
			}
		}
	}
	if f.Kernel == "" {
		f.Kernel = f.OS
	}
	return f
}

func parseDFLine(ln string) (connector.DiskUsage, bool) {
	fields := strings.Fields(ln)
	// Filesystem 1024-blocks Used Available Capacity Mounted-on
	if len(fields) < 6 {
		return connector.DiskUsage{}, false
	}
	total, e1 := strconv.ParseInt(fields[1], 10, 64)
	used, e2 := strconv.ParseInt(fields[2], 10, 64)
	avail, e3 := strconv.ParseInt(fields[3], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return connector.DiskUsage{}, false
	}
	return connector.DiskUsage{
		Filesystem: fields[0],
		Mount:      fields[len(fields)-1],
		Total:      total * 1024,
		Used:       used * 1024,
		Available:  avail * 1024,
	}, true
}
