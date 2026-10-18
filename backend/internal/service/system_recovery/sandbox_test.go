package system_recovery

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRecoverySandboxPolicyStartsBeforeApplication(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox integration")
	}
	root, e := os.MkdirTemp("/private/tmp", "pxsandbox-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(root)
	spec := RunSpec{Isolated: true, Deployment: Deployment{Root: root, CorePort: 25340, GRPCPort: 25341, WebPort: 25342, RedisPort: 25343, PluginPortMin: 25344, PluginPortMax: 25345}, Tools: Tools{Sandbox: "/usr/bin/sandbox-exec", Coordinator: "/usr/bin/true"}}
	exe, args, e := sandboxCommand(spec, filepath.Join(root, "run.json"))
	if e != nil {
		t.Fatal(e)
	}
	// 只验证 OS 策略可加载，后续原生测试再验证真实应用和网络拒绝。
	args = args[:2]
	args = append(args, "/usr/bin/true")
	cmd := exec.CommandContext(context.Background(), exe, args...)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("sandbox failed before application launch: %v: %s", e, out)
	}
}

func TestRecoverySandboxHelper(t *testing.T) {
	if os.Getenv("PX_RECOVERY_SANDBOX_CHILD") != "1" {
		return
	}
	if _, e := os.ReadFile(os.Getenv("PX_FORBIDDEN_FILE")); e == nil {
		os.Exit(31)
	}
	if conn, e := net.DialTimeout("tcp", os.Getenv("PX_FORBIDDEN_ADDR"), time.Second); e == nil {
		conn.Close()
		os.Exit(32)
	}
	if e := os.WriteFile(filepath.Join(os.Getenv("PX_CANDIDATE_ROOT"), "writable"), []byte("candidate only"), 0600); e != nil {
		os.Exit(33)
	}
	if conn, e := net.DialTimeout("unix", filepath.Join(os.Getenv("PX_CANDIDATE_ROOT"), "probe.sock"), time.Second); e != nil {
		os.Exit(34)
	} else {
		conn.Close()
	}
}

func TestRecoverySandboxRejectsSourceFilesAndConnections(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox integration")
	}
	root, e := os.MkdirTemp("/private/tmp", "pxsandbox-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(root)
	source, e := os.CreateTemp("/private/tmp", "pxsource-")
	if e != nil {
		t.Fatal(e)
	}
	source.WriteString("source secret")
	source.Close()
	defer os.Remove(source.Name())
	forbidden, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer forbidden.Close()
	socket, e := net.Listen("unix", filepath.Join(root, "probe.sock"))
	if e != nil {
		t.Fatal(e)
	}
	defer socket.Close()
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	spec := RunSpec{Isolated: true, Deployment: Deployment{Root: root, CorePort: 25340, GRPCPort: 25341, WebPort: 25342, RedisPort: 25343, PluginPortMin: 25344, PluginPortMax: 25345}, Tools: Tools{Sandbox: "/usr/bin/sandbox-exec", Coordinator: executable}}
	exe, args, e := sandboxCommand(spec, filepath.Join(root, "run.json"))
	if e != nil {
		t.Fatal(e)
	}
	args = append(args[:2], executable, "-test.run=^TestRecoverySandboxHelper$", "-test.v")
	command := exec.Command(exe, args...)
	command.Env = cleanEnvironment(root, "PX_RECOVERY_SANDBOX_CHILD=1", "PX_FORBIDDEN_FILE="+source.Name(), "PX_FORBIDDEN_ADDR="+forbidden.Addr().String(), "PX_CANDIDATE_ROOT="+root)
	if out, e := command.CombinedOutput(); e != nil {
		t.Fatalf("isolation boundary failed: %v: %s", e, out)
	}
	if _, e = os.Stat(filepath.Join(root, "writable")); e != nil {
		t.Fatal("helper did not run")
	}
}
