package hostmonitor

import (
	"context"
	"testing"
)

func TestVMStatFixture(t *testing.T) {
	m := parseVMStat("Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free: 10.\nPages inactive: 20.\nPages speculative: 5.\nPages wired down: 100.\n", 1000000)
	if m.Status != "ok" || *m.Available != 35*16384 {
		t.Fatal(m)
	}
	if parseVMStat("broken", 100).Status != "error" {
		t.Fatal("malformed")
	}
	s := parseSwapUsage("total = 1024.00M used = 256.00M free = 768.00M (encrypted)")
	if s.Status != "ok" || *s.Used != 256*1048576 {
		t.Fatal(s)
	}
}

func TestDarwinLiveCPU(t *testing.T) {
	s := NewService().Collect(context.Background())
	if s.CPU.Status != "ok" || s.CPU.Usage == nil {
		t.Fatalf("CPU: %+v warnings: %v", s.CPU, s.Warnings)
	}
	t.Logf("CPU %.2f%% sample=%dms duration=%dms", *s.CPU.Usage, s.CPU.SampleMS, s.DurationMS)
}
func TestDarwinCPUFixture(t *testing.T) {
	v := parseDarwinCPU("      cpu    load average\n us sy id   1m   5m   15m\n16 10 74 1.0 2.0 3.0\n31 11 57 1.0 2.0 3.0\n")
	if v == nil || *v != 43 {
		t.Fatal(v)
	}
	if parseDarwinCPU("16 10 74 1 2 3") != nil {
		t.Fatal("single lifetime sample accepted")
	}
}

func TestDarwinCPUWithDiskColumns(t *testing.T) {
	v := parseDarwinCPU("disk0 disk1 cpu load average\nKB/t tps MB/s KB/t tps MB/s us sy id 1m 5m 15m\n1 2 3 4 5 6 10 20 70 1 2 3\nKB/t tps MB/s KB/t tps MB/s us sy id 1m 5m 15m\n1 2 3 4 5 6 30 46 24 1 2 3\n")
	if v == nil || *v != 76 {
		t.Fatal(v)
	}
}
