package hostmonitor

import (
	"context"
	"golang.org/x/sys/unix"
	"strconv"
	"strings"
	"time"
)

func collectPlatform(ctx context.Context, o *Snapshot) {
	o.Scope = "host"
	if b, e := command(ctx, "sysctl", "-n", "kern.boottime"); e == nil {
		parts := strings.Fields(string(b))
		for i, v := range parts {
			if v == "sec" && i+2 < len(parts) {
				n, e := strconv.ParseInt(strings.Trim(parts[i+2], ","), 10, 64)
				if e == nil {
					o.UptimeSeconds = ptr(time.Since(time.Unix(n, 0)).Seconds())
				}
				break
			}
		}
	}
	if b, e := command(ctx, "iostat", "-c", "2", "-w", "1", "-n", "0"); e == nil {
		o.CPU.Usage = parseDarwinCPU(string(b))
		if o.CPU.Usage != nil {
			o.CPU.Status = "ok"
			o.CPU.SampleMS = 1000
		}
	}

	if o.CPU.Status != "ok" {
		o.Warnings = append(o.Warnings, "CPU sample unavailable")
	}
	var total uint64
	if b, e := command(ctx, "sysctl", "-n", "hw.memsize"); e == nil {
		total, _ = number(string(b))
	}
	if b, e := command(ctx, "vm_stat"); e == nil && total > 0 {
		o.Memory = parseVMStat(string(b), total)
	}
	if b, e := command(ctx, "sysctl", "-n", "vm.swapusage"); e == nil {
		o.Swap = parseSwapUsage(string(b))
	}
	if o.Memory.Status != "ok" || o.Swap.Status != "ok" {
		o.Warnings = append(o.Warnings, "memory or swap metrics unavailable")
	}
	if b, e := command(ctx, "mount"); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			_, rest, ok := strings.Cut(line, " on ")
			if !ok {
				continue
			}
			path, opts, ok := strings.Cut(rest, " (")
			if !ok {
				continue
			}
			fs := strings.Split(opts, ",")[0]
			if fs == "autofs" || fs == "devfs" {
				continue
			}
			var st unix.Statfs_t
			d := Disk{Capacity: Capacity{Status: "error"}, MountPoint: path, Filesystem: strings.TrimSuffix(fs, ")")}
			if !localFilesystem(d.Filesystem) {
				d.Status = "unsupported"
				o.Disks = append(o.Disks, d)
				o.Warnings = append(o.Warnings, "remote or userspace mount space unsupported")
				continue
			}
			if unix.Statfs(path, &st) == nil {
				total := st.Blocks * uint64(st.Bsize)
				d.Capacity = capacity(total, st.Bavail*uint64(st.Bsize))
				d.ReadOnly = ptr(st.Flags&unix.MNT_RDONLY != 0)
				d.Used = ptr(total - st.Bfree*uint64(st.Bsize))
			} else {
				o.Warnings = append(o.Warnings, "mount space metrics unavailable")
			}
			o.Disks = append(o.Disks, d)
		}
	} else {
		o.Warnings = append(o.Warnings, "mount information unavailable")
	}
}
func parseVMStat(s string, total uint64) Capacity {
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return Capacity{Status: "error"}
	}
	var page uint64
	f := strings.Fields(lines[0])
	for i, v := range f {
		if v == "of" && i+1 < len(f) {
			page, _ = number(f[i+1])
		}
	}
	var available uint64
	found := 0
	for _, l := range lines[1:] {
		key, value, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		switch key {
		case "Pages free", "Pages inactive", "Pages speculative":
			n, e := number(strings.TrimSuffix(strings.TrimSpace(value), "."))
			if e == nil {
				available += n * page
				found++
			}
		}
	}
	if page == 0 || found != 3 || available > total {
		return Capacity{Status: "error"}
	}
	return capacity(total, available)
}
func parseSwapUsage(s string) Capacity {
	f := strings.Fields(s)
	values := map[string]uint64{}
	for i := 0; i+2 < len(f); i++ {
		if f[i+1] != "=" {
			continue
		}
		v := f[i+2]
		if strings.HasSuffix(v, "M") {
			if n := metric(strings.TrimSuffix(v, "M")); n != nil {
				values[f[i]] = uint64(*n * 1048576)
			}
		}
	}
	t, ok := values["total"]
	a, ok2 := values["free"]
	if !ok || !ok2 || a > t {
		return Capacity{Status: "error"}
	}
	return capacity(t, a)
}

func parseDarwinCPU(s string) *float64 {
	var value *float64
	samples := 0
	userCol, sysCol, idleCol := -1, -1, -1
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		for i, v := range f {
			switch v {
			case "us":
				userCol = i
			case "sy":
				sysCol = i
			case "id":
				idleCol = i
			}
		}
		if userCol < 0 || sysCol < 0 || idleCol < 0 || len(f) <= userCol || len(f) <= sysCol || len(f) <= idleCol {
			continue
		}
		user, sys, idle := metric(f[userCol]), metric(f[sysCol]), metric(f[idleCol])
		if user == nil || sys == nil || idle == nil || *user > 100 || *sys > 100 || *idle > 100 {
			continue
		}
		samples++
		value = ptr(100 - *idle)
	}
	if samples < 2 {
		return nil
	}
	return value
}
