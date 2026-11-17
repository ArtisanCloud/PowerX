package hostmonitor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Service struct{}

func NewService() *Service { return &Service{} }
func (s *Service) Collect(ctx context.Context) Snapshot {
	start := time.Now()
	out := Snapshot{OS: runtime.GOOS, Arch: runtime.GOARCH, Scope: "unknown", ScopeNote: "OS-visible resources; not container limits", CPU: CPU{Status: "error", LogicalCores: runtime.NumCPU()}, Memory: Capacity{Status: "error"}, Swap: Capacity{Status: "error"}, Disks: []Disk{}, Network: Network{Status: "error", Interfaces: []Interface{}}, GPU: GPU{Status: "unsupported", Devices: []GPUDevice{}}, Warnings: []string{}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	collectPlatform(ctx, &out)
	out.Hostname, _ = os.Hostname()
	collectNetwork(ctx, &out)
	collectGPU(ctx, &out)
	out.CollectedAt = time.Now().UTC()
	out.DurationMS = time.Since(start).Milliseconds()
	return out
}
func collectNetwork(ctx context.Context, out *Snapshot) {
	counters := map[string][2]uint64{}
	if runtime.GOOS == "linux" {
		b, e := os.ReadFile("/proc/net/dev")
		if e == nil {
			counters = parseNetwork(string(b))
		}
	}
	if runtime.GOOS == "darwin" {
		if b, e := command(ctx, "netstat", "-ibn"); e == nil {
			counters = parseDarwinNetwork(string(b))
		}
	}
	interfaces, e := net.Interfaces()
	if e != nil {
		out.Warnings = append(out.Warnings, "network interfaces unavailable")
		return
	}
	out.Network.Status = "ok"
	for _, it := range interfaces {
		item := Interface{Name: it.Name, Addresses: []string{}, Up: it.Flags&net.FlagUp != 0, Loopback: it.Flags&net.FlagLoopback != 0, PointToPoint: it.Flags&net.FlagPointToPoint != 0}
		addresses, e := it.Addrs()
		if e != nil {
			out.Network.Status = "partial"
		} else {
			for _, a := range addresses {
				item.Addresses = append(item.Addresses, a.String())
			}
		}
		if c, ok := counters[it.Name]; ok {
			item.RX = ptr(c[0])
			item.TX = ptr(c[1])
		} else {
			out.Network.Status = "partial"
		}
		out.Network.Interfaces = append(out.Network.Interfaces, item)
	}
	if out.Network.Status != "ok" {
		out.Warnings = append(out.Warnings, "some network addresses or byte counters unavailable")
	}
}

// Commands have fixed arguments, bounded time and bounded output; stderr is never returned.
type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1024*1024 {
		return 0, errors.New("output limit")
	}
	return b.Buffer.Write(p)
}
func command(ctx context.Context, name string, args ...string) ([]byte, error) {
	timeout := 1200 * time.Millisecond
	if name == "iostat" {
		timeout = 2200 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	var b limitedBuffer
	cmd.Stdout = &b
	cmd.Stderr = io.Discard
	e := cmd.Run()
	return b.Bytes(), e
}
func collectGPU(ctx context.Context, out *Snapshot) {
	_, e := exec.LookPath("nvidia-smi")
	if e != nil {
		out.Warnings = append(out.Warnings, "NVIDIA monitoring unsupported: nvidia-smi unavailable")
		return
	}
	b, e := command(ctx, "nvidia-smi", "--query-gpu=index,name,utilization.gpu,memory.total,memory.used,temperature.gpu", "--format=csv,noheader,nounits")
	if e != nil {
		out.GPU.Status = "error"
		out.Warnings = append(out.Warnings, "NVIDIA query failed or timed out")
		return
	}
	out.GPU = parseGPU(string(b))
	if out.GPU.Status != "ok" {
		out.Warnings = append(out.Warnings, "NVIDIA devices absent or metrics incomplete")
	}
}
func detectContainer(cgroup string, marker bool) bool {
	return marker || strings.Contains(cgroup, "docker") || strings.Contains(cgroup, "kubepods") || strings.Contains(cgroup, "containerd") || strings.Contains(cgroup, "lxc")
}
