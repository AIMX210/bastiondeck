package factscat

import "testing"

const sample = `@@OS
PRETTY_NAME="Ubuntu 22.04.4 LTS"
@@UP
12345.67 23456.78
@@CPU
1717000000123
cpu  100 200 300 700 50 10 5 15 0 0
cpu0 50 100 150 350 25 5 2 7 0 0
cpu1 50 100 150 350 25 5 3 8 0 0
@@MEM
MemTotal:        8000000 kB
MemFree:         2000000 kB
MemAvailable:    4000000 kB
Cached:          1500000 kB
Buffers:          300000 kB
SReclaimable:     200000 kB
SwapTotal:       1000000 kB
SwapFree:         800000 kB
@@LOAD
0.50 0.40 0.30 1/100 1234
@@NET
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000    10    0    0    0     0          0         0     1000    10    0    0    0     0       0          0
  eth0: 5000    50    0    0    0     0          0         0     3000    40    0    0    0     0       0          0
@@DISK
   8       0 sda 100 0 2000 10 50 0 1000 20 0 30 30 0 0 0 0 0 0
   7       0 loop0 1 0 8 0 0 0 0 0 0 0 0 0 0 0 0 0 0
@@DF
/dev/vda1        1000000    400000   600000      40% /
@@SOCK
sockets: used 318
TCP: inuse 15 orphan 0 tw 12 alloc 18 mem 2
UDP: inuse 5 mem 0
UDPLITE: inuse 0
TCP6: inuse 3
UDP6: inuse 1
@@SNMP
Tcp: RtoAlgorithm RtoMin RtoMax MaxConn ActiveOpens PassiveOpens AttemptFails EstabResets CurrEstab InSegs OutSegs RetransSegs InErrs OutRsts OutNoPorts
Tcp: 1 200 120000 -1 100 200 10 5 20 1000 8000 400 2 3 4
@@TEMP
45000
46000
@@END`

func TestParse(t *testing.T) {
	r := Parse(sample)
	if r == nil {
		t.Fatal("parse failed")
	}
	if r.TS != 1717000000123 {
		t.Fatalf("ts=%d", r.TS)
	}
	if r.CPUCores != 2 || len(r.CPUTot) != 8 {
		t.Fatalf("cpu=%d %v", r.CPUCores, r.CPUTot)
	}
	if r.MemTotal != 8000000 || r.MemAvail != 4000000 || r.SwapTotal != 1000000 {
		t.Fatalf("mem=%d/%d swap=%d", r.MemTotal, r.MemAvail, r.SwapTotal)
	}
	if r.Rx != 5000 || r.Tx != 3000 { // lo 排除
		t.Fatalf("net rx=%d tx=%d", r.Rx, r.Tx)
	}
	if r.SectorsRead != 2000 || r.SectorsWrite != 1000 { // loop 排除
		t.Fatalf("disk sr=%d sw=%d", r.SectorsRead, r.SectorsWrite)
	}
	if r.DFTotal != 1000000 || r.DFUsed != 400000 {
		t.Fatalf("df=%d/%d", r.DFTotal, r.DFUsed)
	}
	if r.TCP4 != 15 || r.TCP6 != 3 || r.UDP4 != 5 {
		t.Fatalf("sock=%d/%d/%d", r.TCP4, r.TCP6, r.UDP4)
	}
	if r.TCPActive != 100 || r.OutSegs != 8000 || r.Retrans != 400 {
		t.Fatalf("snmp=%d/%d/%d", r.TCPActive, r.OutSegs, r.Retrans)
	}
	if len(r.TempsC) != 2 || r.TempsC[0] != 45.0 {
		t.Fatalf("temps=%v", r.TempsC)
	}
	if r.Uptime != 12345 || r.OS != "Ubuntu 22.04.4 LTS" {
		t.Fatalf("up=%d os=%q", r.Uptime, r.OS)
	}
}

func TestDiffRates(t *testing.T) {
	r1 := Parse(sample)
	r2 := Parse(replaceAll(sample,
		"1717000000123", "1717000001123",
		"5000    50", "6000    50",
		"3000    40", "3200    40",
		"cpu  100 200 300 700 50 10 5 15 0 0", "cpu  110 210 310 760 50 10 5 15 0 0"))
	if r2 == nil {
		t.Fatal("r2 parse failed")
	}
	s := Diff(r1, r2)
	if s.RxBps != 1000 || s.TxBps != 200 {
		t.Fatalf("bps=%d/%d", s.RxBps, s.TxBps)
	}
	if s.CPUUsedPct <= 0 || s.CPUUsedPct >= 100 {
		t.Fatalf("cpu=%f", s.CPUUsedPct)
	}
	first := Diff(nil, r2)
	if first.RxBps != 0 || first.CPUUsedPct != 0 {
		t.Fatalf("first sample must be rate-free: %+v", first)
	}
}

func TestCachePut(t *testing.T) {
	c := NewCache()
	r1 := Parse(sample)
	r2 := Parse(replaceAll(sample, "1717000000123", "1717000001123", "5000    50", "6000    50"))
	s1 := c.Put("hst_x", r1)
	if s1.RxBps != 0 {
		t.Fatal("first put has rates")
	}
	s2 := c.Put("hst_x", r2)
	if s2.RxBps != 1000 {
		t.Fatalf("rx=%d", s2.RxBps)
	}
	if s2.OS != "Ubuntu 22.04.4 LTS" {
		t.Fatalf("os=%q", s2.OS)
	}
}

func replaceAll(s string, pairs ...string) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		s = replaceOnce(s, pairs[i], pairs[i+1])
	}
	return s
}

func replaceOnce(s, old, new string) string {
	idx := indexOf(s, old)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
