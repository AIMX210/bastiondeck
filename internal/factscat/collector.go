// Package factscat implements ServerCat-style host metric collection: one
// SSH round-trip reads /proc & /sys (see the Android port, workspace/
// servercat-android), parses the sections and diffs consecutive samples into
// displayable snapshots. Brought over from the mobile port (2026-09-06).
package factscat

import (
	"strconv"
	"strings"
	"sync"
)

// CollectCmd runs on the target host via any connector.Exec. Section markers
// (@@) keep parsing resilient to missing files.
const CollectCmd = "echo '@@OS'; cat /etc/os-release 2>/dev/null; " +
	"echo '@@UP'; cat /proc/uptime; " +
	"echo '@@CPU'; date +%s%3N; cat /proc/stat; " +
	"echo '@@MEM'; cat /proc/meminfo; " +
	"echo '@@LOAD'; cat /proc/loadavg; " +
	"echo '@@NET'; cat /proc/net/dev; " +
	"echo '@@DISK'; cat /proc/diskstats; " +
	"echo '@@DF'; df -kP /; " +
	"echo '@@SOCK'; cat /proc/net/sockstat; cat /proc/net/sockstat6 2>/dev/null; " +
	"echo '@@SNMP'; cat /proc/net/snmp 2>/dev/null; " +
	"echo '@@TEMP'; sh -c 'cat /sys/class/thermal/thermal_*/temp 2>/dev/null; " +
	"cat /sys/class/hwmon/hwmon*/temp1_input 2>/dev/null'; " +
	"echo '@@END'"

// Raw is one undiffed sample.
type Raw struct {
	TS            int64   `json:"ts"`
	CPUCores      int     `json:"cpuCores"`
	CPUTot        []int64 `json:"cpuTot"`
	MemTotal      int64   `json:"memTotal"`
	MemAvail      int64   `json:"memAvail"`
	MemCached     int64   `json:"memCached"`
	MemBuffer     int64   `json:"memBuffer"`
	SwapTotal     int64   `json:"swapTotal"`
	SwapFree      int64   `json:"swapFree"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	Rx            int64   `json:"rx"`
	Tx            int64   `json:"tx"`
	SectorsRead   int64   `json:"sectorsRead"`
	SectorsWrite  int64   `json:"sectorsWrite"`
	Reads         int64   `json:"reads"`
	Writes        int64   `json:"writes"`
	DFTotal       int64   `json:"dfTotal"`
	DFUsed        int64   `json:"dfUsed"`
	DFAvail       int64   `json:"dfAvail"`
	TCP4          int64   `json:"tcp4"`
	TCP6          int64   `json:"tcp6"`
	UDP4          int64   `json:"udp4"`
	UDP6          int64   `json:"udp6"`
	TCPActive     int64   `json:"tcpActive"`
	TCPPassive    int64   `json:"tcpPassive"`
	TCPFail       int64   `json:"tcpFail"`
	Retrans       int64   `json:"retrans"`
	OutSegs       int64   `json:"outSegs"`
	TempsC        []float64 `json:"tempsC"`
	Uptime        int64   `json:"uptime"`
	OS            string  `json:"os"`
}

// Snapshot is a displayable, diffed sample (rates in bytes/s, iops/s, %).
type Snapshot struct {
	TS           int64   `json:"ts"`
	CPUCores     int     `json:"cpuCores"`
	CPUUsedPct   float64 `json:"cpuUsedPct"`
	CPUUserPct   float64 `json:"cpuUserPct"`
	CPUSysPct    float64 `json:"cpuSysPct"`
	CPUIowaitPct float64 `json:"cpuIowaitPct"`
	MemTotal     int64   `json:"memTotal"`
	MemUsed      int64   `json:"memUsed"`
	MemCached    int64   `json:"memCached"`
	MemBuffer    int64   `json:"memBuffer"`
	MemUsedPct   float64 `json:"memUsedPct"`
	SwapTotal    int64   `json:"swapTotal"`
	SwapUsed     int64   `json:"swapUsed"`
	SwapUsedPct  float64 `json:"swapUsedPct"`
	Load1        float64 `json:"load1"`
	Load5        float64 `json:"load5"`
	Load15       float64 `json:"load15"`
	DFTotal      int64   `json:"dfTotal"`
	DFUsed       int64   `json:"dfUsed"`
	DFAvail      int64   `json:"dfAvail"`
	DiskUsedPct  float64 `json:"diskUsedPct"`
	ReadBps      int64   `json:"readBps"`
	WriteBps     int64   `json:"writeBps"`
	RIOPS        int64   `json:"rIops"`
	WIOPS        int64   `json:"wIops"`
	RxBytes      int64   `json:"rxBytes"`
	TxBytes      int64   `json:"txBytes"`
	RxBps        int64   `json:"rxBps"`
	TxBps        int64   `json:"txBps"`
	TCPActivePerS   int64   `json:"tcpActivePerS"`
	TCPPassivePerS  int64   `json:"tcpPassivePerS"`
	TCPFailPerS     int64   `json:"tcpFailPerS"`
	RetransPct      float64 `json:"retransPct"`
	TCP4 int64 `json:"tcp4"`
	TCP6 int64 `json:"tcp6"`
	UDP4 int64 `json:"udp4"`
	UDP6 int64 `json:"udp6"`
	TempsC []float64 `json:"tempsC"`
	Uptime int64     `json:"uptime"`
	OS     string    `json:"os"`
}

// Sections splits @@-marked output.
func Sections(out string) map[string]string {
	m := map[string]string{}
	cur := ""
	var sb strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "@@") {
			if cur != "" {
				m[cur] = sb.String()
			}
			cur = strings.TrimSpace(strings.TrimPrefix(line, "@@"))
			sb.Reset()
			continue
		}
		if cur != "" {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	if cur != "" {
		m[cur] = sb.String()
	}
	return m
}

func firstField(s, prefix string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, prefix) {
			f := strings.Fields(strings.TrimPrefix(l, prefix))
			if len(f) > 0 {
				return f[0]
			}
		}
	}
	return ""
}

func atoi(s string) int64 { v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64); return v }
func atof(s string) float64 { v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return v }

// Parse turns collector output into a Raw sample. Nil when CPU section is
// unreadable (host not Linux or command failed).
func Parse(out string) *Raw {
	s := Sections(out)
	cpu, ok := s["CPU"]
	if !ok {
		return nil
	}
	lines := nonEmpty(cpu)
	if len(lines) < 2 {
		return nil
	}
	ts := atoi(lines[0])
	if ts == 0 {
		ts = atoi(firstField(cpu+"\n0", ""))
	}
	var agg []int64
	cores := 0
	for _, l := range lines[1:] {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		if f[0] == "cpu" && agg == nil {
			for _, v := range f[1:] {
				agg = append(agg, atoi(v))
			}
			if len(agg) < 8 { // 内核裁剪过的行补零
				for len(agg) < 8 {
					agg = append(agg, 0)
				}
			}
			if len(agg) > 8 { // 丢弃 guest/guest_nice 之后的多余字段（对齐差分约定）
				agg = agg[:8]
			}
		}
		if len(f[0]) > 3 && strings.HasPrefix(f[0], "cpu") {
			cores++
		}
	}
	if agg == nil {
		return nil
	}
	if cores == 0 {
		cores = 1
	}
	mem := func(k string) int64 { return atoi(firstField(s["MEM"], k+":")) }
	swapT := mem("SwapTotal")
	swapF := mem("SwapFree")
	var l1, l5, l15 float64
	if f := strings.Fields(strings.TrimSpace(s["LOAD"])); len(f) >= 3 {
		l1, l5, l15 = atof(f[0]), atof(f[1]), atof(f[2])
	}
	var rx, tx int64
	for _, l := range strings.Split(s["NET"], "\n") {
		l = strings.TrimSpace(l)
		p := strings.Fields(strings.Replace(l, ":", " ", 1))
		if len(p) < 16 || p[0] == "lo" {
			continue
		}
		rx += atoi(p[1])
		tx += atoi(p[9])
	}
	var sr, sw, rd, wr int64
	for _, l := range strings.Split(s["DISK"], "\n") {
		f := strings.Fields(strings.TrimSpace(l))
		if len(f) < 10 {
			continue
		}
		if strings.HasPrefix(f[2], "loop") || strings.HasPrefix(f[2], "ram") {
			continue
		}
		rd += atoi(f[3])
		sr += atoi(f[5])
		wr += atoi(f[7])
		sw += atoi(f[9])
	}
	var dfT, dfU, dfA int64
	for _, l := range strings.Split(s["DF"], "\n") {
		f := strings.Fields(strings.TrimSpace(l))
		if len(f) >= 4 && !strings.HasPrefix(f[0], "Filesystem") && f[len(f)-1] == "/" {
			dfT, dfU, dfA = atoi(f[1]), atoi(f[2]), atoi(f[3])
			break
		}
	}
	// 真实格式：`TCP: inuse 15 orphan 0 tw 12 alloc 18 mem 2`（注意大写 TCP:）
	sock := func(prefix string) int64 {
		for _, l := range strings.Split(s["SOCK"], "\n") {
			t := strings.TrimSpace(l)
			if !strings.HasPrefix(t, prefix) {
				continue
			}
			f := strings.Fields(t)
			for i, w := range f {
				if w == "inuse" && i+1 < len(f) {
					return atoi(f[i+1])
				}
			}
		}
		return 0
	}
	snmp := func(section, field string) int64 {
		ls := []string{}
		for _, l := range strings.Split(s["SNMP"], "\n") {
			if strings.HasPrefix(l, section) {
				ls = append(ls, l)
			}
		}
		if len(ls) < 2 {
			return 0
		}
		names := strings.Fields(ls[0])
		vals := strings.Fields(ls[1])
		for i, n := range names {
			if n == field && i < len(vals) {
				return atoi(vals[i])
			}
		}
		return 0
	}
	var temps []float64
	for _, l := range strings.Split(s["TEMP"], "\n") {
		if v := atof(l); v > 0 {
			temps = append(temps, v/1000)
		}
	}
	os := "Linux"
	for _, l := range strings.Split(s["OS"], "\n") {
		if strings.HasPrefix(l, "PRETTY_NAME=") {
			os = strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), `"`)
		}
	}
	return &Raw{
		TS: ts, CPUCores: cores, CPUTot: agg,
		MemTotal: mem("MemTotal"), MemAvail: mem("MemAvailable"),
		MemCached: mem("Cached") + mem("SReclaimable"), MemBuffer: mem("Buffers"),
		SwapTotal: swapT, SwapFree: swapF,
		Load1: l1, Load5: l5, Load15: l15,
		Rx: rx, Tx: tx, SectorsRead: sr, SectorsWrite: sw, Reads: rd, Writes: wr,
		DFTotal: dfT, DFUsed: dfU, DFAvail: dfA,
		TCP4: sock("TCP:"), TCP6: sock("TCP6:"), UDP4: sock("UDP:"), UDP6: sock("UDP6:"),
		TCPActive: snmp("Tcp:", "ActiveOpens"), TCPPassive: snmp("Tcp:", "PassiveOpens"),
		TCPFail: snmp("Tcp:", "AttemptFails"), Retrans: snmp("Tcp:", "RetransSegs"),
		OutSegs: snmp("Tcp:", "OutSegs"),
		TempsC: temps, Uptime: int64(atof(firstField(s["UP"], ""))), OS: os,
	}
}

func nonEmpty(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func sum(a []int64) int64 {
	var t int64
	for _, v := range a {
		t += v
	}
	return t
}

// Diff computes rates between two samples (prev=nil → rates are zero).
func Diff(prev, cur *Raw) *Snapshot {
	dt := 0.001
	if prev != nil {
		dt = float64(cur.TS - prev.TS) / 1000.0
		if dt < 0.001 {
			dt = 0.001
		}
	}
	rate := func(a, b int64) int64 {
		if prev == nil {
			return 0
		}
		if d := b - a; d > 0 {
			return int64(float64(d) / dt)
		}
		return 0
	}
	pct := func(i int) float64 {
		if prev == nil {
			return 0
		}
		d := cur.CPUTot[i] - prev.CPUTot[i]
		if d < 0 {
			d = 0
		}
		tot := sum(cur.CPUTot) - sum(prev.CPUTot)
		if tot <= 0 {
			return 0
		}
		return float64(d) * 100 / float64(tot)
	}
	memUsed := cur.MemTotal - cur.MemAvail
	if memUsed < 0 {
		memUsed = 0
	}
	memPct := 0.0
	if cur.MemTotal > 0 {
		memPct = float64(memUsed) * 100 / float64(cur.MemTotal)
	}
	swU := cur.SwapTotal - cur.SwapFree
	swPct := 0.0
	if cur.SwapTotal > 0 {
		swPct = float64(swU) * 100 / float64(cur.SwapTotal)
	}
	dfPct := 0.0
	if cur.DFTotal > 0 {
		dfPct = float64(cur.DFUsed) * 100 / float64(cur.DFTotal)
	}
	retr := 0.0
	if prev != nil {
		if segs := cur.OutSegs - prev.OutSegs; segs > 0 {
			r := cur.Retrans - prev.Retrans
			if r < 0 {
				r = 0
			}
			retr = float64(r) * 100 / float64(segs)
		}
	}
	return &Snapshot{
		TS: cur.TS, CPUCores: cur.CPUCores,
		CPUUsedPct: firstOr(100-pct(3), prev == nil), CPUUserPct: pct(0), CPUSysPct: pct(2), CPUIowaitPct: pct(4),
		MemTotal: cur.MemTotal, MemUsed: memUsed, MemCached: cur.MemCached, MemBuffer: cur.MemBuffer,
		MemUsedPct: memPct,
		SwapTotal: cur.SwapTotal, SwapUsed: swU, SwapUsedPct: swPct,
		Load1: cur.Load1, Load5: cur.Load5, Load15: cur.Load15,
		DFTotal: cur.DFTotal * 1024, DFUsed: cur.DFUsed * 1024, DFAvail: cur.DFAvail * 1024,
		DiskUsedPct: dfPct,
		ReadBps: rate(prevSectorsRead(prev), cur.SectorsRead) * 512,
		WriteBps: rate(prevSectorsWrite(prev), cur.SectorsWrite) * 512,
		RIOPS: rate(prevReads(prev), cur.Reads), WIOPS: rate(prevWrites(prev), cur.Writes),
		RxBytes: cur.Rx, TxBytes: cur.Tx,
		RxBps: rate(prevRx(prev), cur.Rx), TxBps: rate(prevTx(prev), cur.Tx),
		TCPActivePerS: rate(prevActive(prev), cur.TCPActive),
		TCPPassivePerS: rate(prevPassive(prev), cur.TCPPassive),
		TCPFailPerS: rate(prevFail(prev), cur.TCPFail),
		RetransPct: retr,
		TCP4: cur.TCP4, TCP6: cur.TCP6, UDP4: cur.UDP4, UDP6: cur.UDP6,
		TempsC: cur.TempsC, Uptime: cur.Uptime, OS: cur.OS,
	}
}

// firstOr：首帧无差分，展示值取 0 而不是 100%（避免仪表盘首帧闪红）
func firstOr(v float64, first bool) float64 { if first { return 0 }; return v }

func prevRx(p *Raw) int64 { if p == nil { return 0 }; return p.Rx }
func prevTx(p *Raw) int64 { if p == nil { return 0 }; return p.Tx }
func prevSectorsRead(p *Raw) int64 { if p == nil { return 0 }; return p.SectorsRead }
func prevSectorsWrite(p *Raw) int64 { if p == nil { return 0 }; return p.SectorsWrite }
func prevReads(p *Raw) int64 { if p == nil { return 0 }; return p.Reads }
func prevWrites(p *Raw) int64 { if p == nil { return 0 }; return p.Writes }
func prevActive(p *Raw) int64 { if p == nil { return 0 }; return p.TCPActive }
func prevPassive(p *Raw) int64 { if p == nil { return 0 }; return p.TCPPassive }
func prevFail(p *Raw) int64 { if p == nil { return 0 }; return p.TCPFail }

// Cache keeps the last raw sample per host for rate diffing (in-memory,
// matching the repo's single-process assumption). Mutex-guarded: HTTP handlers
// may race on the same host.
type Cache struct {
	mu   sync.Mutex
	last map[string]*Raw
}

func NewCache() *Cache { return &Cache{last: map[string]*Raw{}} }

// Put stores the sample and returns the diffed snapshot.
func (c *Cache) Put(hostID string, cur *Raw) *Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := c.last[hostID]
	c.last[hostID] = cur
	return Diff(prev, cur)
}

