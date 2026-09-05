//go:build linux

package facts

import (
	"syscall"
	"time"
)

func platformFacts(f *Facts) (kernel string, uptimeS int64, memTotal int64) {
	var u syscall.Utsname
	if err := syscall.Uname(&u); err == nil {
		kernel = int8ToString(u.Release[:])
	}
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err == nil {
		uptimeS = int64(si.Uptime)
		memTotal = int64(si.Totalram) * int64(si.Unit)
	} else {
		uptimeS = uptimeFallback()
		memTotal = parseMeminfo()["MemTotal"]
	}
	return
}

func disks() []DiskUsage {
	var st syscall.Statfs_t
	out := []DiskUsage{}
	for _, mount := range []string{"/"} {
		if err := syscall.Statfs(mount, &st); err != nil {
			continue
		}
		total := int64(st.Blocks) * int64(st.Bsize)
		avail := int64(st.Bavail) * int64(st.Bsize)
		free := int64(st.Bfree) * int64(st.Bsize)
		out = append(out, DiskUsage{
			Filesystem: "root", Mount: mount, Total: total,
			Used: total - free, Available: avail,
		})
	}
	return out
}

func int8ToString(b []int8) string {
	var out []byte
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}

var _ = time.Now
