package hostmonitor

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCPUAccounting(t *testing.T) {
	a, ai, e := cpuTicks("cpu 100 0 100 700 100 0 0 0 50 0\n")
	if e != nil || a != 1000 || ai != 800 {
		t.Fatalf("%d %d %v", a, ai, e)
	}
	v := cpuUsage(a, ai, 1200, 900)
	if v == nil || *v != 50 {
		t.Fatal(v)
	}
	if cpuUsage(a, ai, 900, 700) != nil {
		t.Fatal("counter reset")
	}
	if _, _, e := cpuTicks("cpu broken 0 0 0"); e == nil {
		t.Fatal("malformed accepted")
	}
}
func TestMemoryFixture(t *testing.T) {
	m, s := parseMemory("MemTotal: 1000 kB\nMemAvailable: 250 kB\nSwapTotal: 500 kB\nSwapFree: 400 kB\n")
	if *m.Used != 750*1024 || *s.Used != 100*1024 {
		t.Fatal(m, s)
	}
	m, _ = parseMemory("MemTotal: 1 kB\nMemAvailable: 2 kB\n")
	if m.Status != "error" || m.Total != nil {
		t.Fatal(m)
	}
}
func TestNetworkFixture(t *testing.T) {
	v := parseNetwork("Inter-| Receive\n eth0: 100 1 0 0 0 0 0 0 200 1 0 0 0 0 0 0\n bad: x\n")
	if len(v) != 1 || v["eth0"] != [2]uint64{100, 200} {
		t.Fatal(v)
	}
	d := parseDarwinNetwork("Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll\nen0 1500 <Link#1> aa:bb 1 0 100 2 0 200 0\nen0 1500 10.0.0 10.0.0.1 1 - 999 2 - 999 -\n")
	if d["en0"] != [2]uint64{100, 200} {
		t.Fatal(d)
	}
}
func TestGPUFixture(t *testing.T) {
	g := parseGPU("0, NVIDIA Test, 25, 100, 40, 50\n1, NVIDIA Other, [N/A], 200, 30, [N/A]\n")
	if g.Status != "partial" || len(g.Devices) != 2 || *g.Devices[0].Total != 100*1048576 || g.Devices[1].Utilization != nil {
		t.Fatal(g)
	}
	if parseGPU("").Status != "unsupported" {
		t.Fatal("empty")
	}
	if metric("NaN") != nil {
		t.Fatal("nonfinite")
	}
}
func TestFilesystemPolicy(t *testing.T) {
	for _, fs := range []string{"nfs", "nfs4", "cifs", "smbfs", "webdav", "fuse", "fuse.sshfs", "unknown"} {
		if localFilesystem(fs) {
			t.Fatal(fs)
		}
	}
	for _, fs := range []string{"apfs", "ext4", "overlay"} {
		if !localFilesystem(fs) {
			t.Fatal(fs)
		}
	}
}
func TestContainerDetection(t *testing.T) {
	if !detectContainer("0::/kubepods/pod", false) || !detectContainer("", true) || detectContainer("0::/", false) {
		t.Fatal("scope")
	}
}
func TestCanceledCollectionReturnsDTO(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	s := NewService().Collect(ctx)
	if time.Since(start) > 2*time.Second {
		t.Fatal("cancellation ignored")
	}
	if s.Warnings == nil || s.Disks == nil || s.Network.Interfaces == nil || s.GPU.Devices == nil || s.CollectedAt.IsZero() {
		t.Fatal("missing metadata")
	}
	if _, e := json.Marshal(s); e != nil {
		t.Fatal(e)
	}
}

func TestDarwinCountersWithoutAddressAndDisabledInterface(t *testing.T) {
	s := "Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll\n" +
		"lo0 16384 <Link#1> 443413685 0 565079412112 443413685 0 565079412112 0\n" +
		"gif0* 1280 <Link#2> 0 0 0 0 0 0 0\n" +
		"en0 1500 <Link#3> aa:bb:cc:dd:ee:ff 12 0 123456 23 0 234567 0\n"
	got := parseDarwinNetwork(s)
	if got["lo0"] != [2]uint64{565079412112, 565079412112} {
		t.Fatal(got)
	}
	if v, ok := got["gif0"]; !ok || v != [2]uint64{0, 0} {
		t.Fatal(got)
	}
	if got["en0"] != [2]uint64{123456, 234567} {
		t.Fatal(got)
	}
}
