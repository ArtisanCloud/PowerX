package hostmonitor

import (
	"encoding/csv"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func number(s string) (uint64, error) { return strconv.ParseUint(strings.TrimSpace(s), 10, 64) }
func cpuTicks(s string) (uint64, uint64, error) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[0] != "cpu" {
			continue
		}
		var total, idle uint64
		// guest and guest_nice are already included in user/nice.
		for i := 1; i < len(f) && i <= 8; i++ {
			n, e := number(f[i])
			if e != nil {
				return 0, 0, e
			}
			total += n
			if i == 4 || i == 5 {
				idle += n
			}
		}
		return total, idle, nil
	}
	return 0, 0, fmt.Errorf("missing cpu")
}
func cpuUsage(a, ai, b, bi uint64) *float64 {
	if b <= a || bi < ai || bi-ai > b-a {
		return nil
	}
	return ptr(100 * float64((b-a)-(bi-ai)) / float64(b-a))
}
func parseMemory(s string) (Capacity, Capacity) {
	values := map[string]uint64{}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[2] != "kB" {
			continue
		}
		n, e := number(f[1])
		if e == nil && n <= math.MaxUint64/1024 {
			values[strings.TrimSuffix(f[0], ":")] = n * 1024
		}
	}
	makeCap := func(t, a string) Capacity {
		total, ok := values[t]
		avail, ok2 := values[a]
		if !ok || !ok2 || avail > total {
			return Capacity{Status: "error"}
		}
		return capacity(total, avail)
	}
	return makeCap("MemTotal", "MemAvailable"), makeCap("SwapTotal", "SwapFree")
}
func parseNetwork(s string) map[string][2]uint64 {
	out := map[string][2]uint64{}
	for _, line := range strings.Split(s, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 16 {
			continue
		}
		rx, e := number(f[0])
		tx, e2 := number(f[8])
		if e == nil && e2 == nil {
			out[strings.TrimSpace(name)] = [2]uint64{rx, tx}
		}
	}
	return out
}
func parseDarwinNetwork(s string) map[string][2]uint64 {
	out := map[string][2]uint64{}
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return out
	}
	header := strings.Fields(lines[0])
	rx, tx := -1, -1
	for i, v := range header {
		if v == "Ibytes" {
			rx = i
		}
		if v == "Obytes" {
			tx = i
		}
	}
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if rx < 0 || tx < 0 || len(f) < 4 || !strings.HasPrefix(f[2], "<Link#") {
			continue
		}
		// netstat omits the Address field for interfaces without a MAC address.
		// All counters after it shift by one; never interpret packet counts as bytes.
		shift := len(f) - len(header)
		if shift != 0 && shift != -1 {
			continue
		}
		ri, ti := rx+shift, tx+shift
		if ri < 3 || ti < 3 || ri >= len(f) || ti >= len(f) {
			continue
		}
		a, e := number(f[ri])
		b, e2 := number(f[ti])
		if e == nil && e2 == nil {
			out[strings.TrimSuffix(f[0], "*")] = [2]uint64{a, b}
		}
	}
	return out
}
func metric(s string) *float64 {
	v, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	return ptr(v)
}
func mib(s string) *uint64 {
	v := metric(s)
	if v == nil || *v >= float64(math.MaxUint64)/1048576 {
		return nil
	}
	return ptr(uint64(*v * 1048576))
}
func parseGPU(s string) GPU {
	out := GPU{Status: "unsupported", Devices: []GPUDevice{}}
	rows, e := csv.NewReader(strings.NewReader(s)).ReadAll()
	if e != nil {
		out.Status = "error"
		return out
	}
	partial := false
	for _, r := range rows {
		if len(r) != 6 {
			partial = true
			continue
		}
		idx, e := strconv.Atoi(strings.TrimSpace(r[0]))
		if e != nil || idx < 0 {
			partial = true
			continue
		}
		d := GPUDevice{idx, strings.TrimSpace(r[1]), metric(r[2]), mib(r[3]), mib(r[4]), metric(r[5])}
		if d.Utilization != nil && *d.Utilization > 100 {
			d.Utilization = nil
		}
		if d.Utilization == nil || d.Total == nil || d.Used == nil || d.Temperature == nil {
			partial = true
		}
		out.Devices = append(out.Devices, d)
	}
	if len(out.Devices) > 0 {
		out.Status = "ok"
	}
	if partial {
		out.Status = "partial"
	}
	return out
}

// Only known kernel-local filesystems are eligible for synchronous statfs.
// Unknown/remote/userspace mounts remain visible but are never probed.
func localFilesystem(fs string) bool {
	switch fs {
	case "ext2", "ext3", "ext4", "xfs", "btrfs", "zfs", "tmpfs", "ramfs", "overlay", "aufs", "squashfs", "vfat", "exfat", "ntfs3", "f2fs", "jfs", "reiserfs", "apfs", "hfs", "hfsplus", "ufs":
		return true
	}
	return false
}
