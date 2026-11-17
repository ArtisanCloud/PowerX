package hostmonitor

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
	"strings"
	"time"
)

func collectPlatform(ctx context.Context, o *Snapshot) {
	o.Scope = "host"
	cg, _ := os.ReadFile("/proc/1/cgroup")
	_, a := os.Stat("/.dockerenv")
	_, b := os.Stat("/run/.containerenv")
	o.ContainerDetected = detectContainer(string(cg), a == nil || b == nil)
	if o.ContainerDetected {
		o.Scope = "container_visible"
	}
	if u, e := os.ReadFile("/proc/uptime"); e == nil {
		f := strings.Fields(string(u))
		if len(f) > 0 {
			o.UptimeSeconds = metric(f[0])
		}
	}
	mem, e := os.ReadFile("/proc/meminfo")
	if e == nil {
		o.Memory, o.Swap = parseMemory(string(mem))
	}
	if o.Memory.Status != "ok" || o.Swap.Status != "ok" {
		o.Warnings = append(o.Warnings, "memory or swap metrics unavailable")
	}
	first, e := os.ReadFile("/proc/stat")
	start := time.Now()
	if e == nil {
		a, ai, e := cpuTicks(string(first))
		if e == nil {
			timer := time.NewTimer(200 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
				second, e := os.ReadFile("/proc/stat")
				if e == nil {
					b, bi, e := cpuTicks(string(second))
					if e == nil {
						o.CPU.Usage = cpuUsage(a, ai, b, bi)
					}
				}
			}
			o.CPU.SampleMS = time.Since(start).Milliseconds()
		}
	}
	if o.CPU.Usage != nil {
		o.CPU.Status = "ok"
	} else {
		o.Warnings = append(o.Warnings, "CPU sample unavailable or canceled")
	}
	data, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		o.Warnings = append(o.Warnings, "mount information unavailable")
		return
	}
	for _, m := range parseMounts(string(data)) {
		if ctx.Err() != nil {
			break
		}
		var st unix.Statfs_t
		d := Disk{Capacity: Capacity{Status: "error"}, MountPoint: m[0], Filesystem: m[1]}
		if !localFilesystem(m[1]) {
			d.Status = "unsupported"
			o.Disks = append(o.Disks, d)
			o.Warnings = append(o.Warnings, "remote or userspace mount space unsupported")
			continue
		}
		if unix.Statfs(m[0], &st) == nil && st.Bsize > 0 {
			total := st.Blocks * uint64(st.Bsize)
			d.Capacity = capacity(total, st.Bavail*uint64(st.Bsize))
			d.ReadOnly = ptr(st.Flags&unix.ST_RDONLY != 0)
			d.Used = ptr(total - st.Bfree*uint64(st.Bsize))
		} else {
			o.Warnings = append(o.Warnings, "mount space metrics unavailable")
		}
		o.Disks = append(o.Disks, d)
	}
}
func unescapeMount(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}
func parseMounts(s string) [][2]string {
	result := [][2]string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(s, "\n") {
		left, right, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		f, r := strings.Fields(left), strings.Fields(right)
		if len(f) < 6 || len(r) < 2 {
			continue
		}
		fs := r[0]
		switch fs {
		case "proc", "sysfs", "devtmpfs", "devpts", "cgroup", "cgroup2", "securityfs", "debugfs", "tracefs", "mqueue", "hugetlbfs", "configfs", "fusectl", "pstore", "autofs", "binfmt_misc":
			continue
		}
		path := unescapeMount(f[4])
		if !strings.HasPrefix(path, "/") || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, [2]string{path, fs})
	}
	return result
}
