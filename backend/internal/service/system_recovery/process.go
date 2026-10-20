package system_recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

type processRecord struct {
	PID        int      `json:"pid"`
	Started    string   `json:"started"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	Directory  string   `json:"directory"`
	Ports      []int    `json:"ports"`
}

func startOwned(ctx context.Context, recordFile, exe, dir string, args, env []string, ports []int) (*exec.Cmd, error) {
	if _, e := os.Stat(recordFile); e == nil {
		return nil, errors.New("owned process record already exists; recover it before starting")
	}
	if e := os.MkdirAll(filepath.Dir(recordFile), 0700); e != nil {
		return nil, e
	}
	log, e := os.OpenFile(recordFile+".log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = cmd.Start(); e != nil {
		log.Close()
		return nil, e
	}
	identity, e := processIdentity(cmd.Process.Pid)
	if e == nil {
		record := processRecord{cmd.Process.Pid, identity, exe, args, dir, ports}
		e = snapshots.WriteCoordinatorJSON(recordFile, record)
		if e == nil {
			e = snapshots.WriteCoordinatorJSON(fmt.Sprintf("%s.owned-%d.json", recordFile, cmd.Process.Pid), record)
		}
	}
	if e != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		log.Close()
		return nil, e
	}
	go func() { _ = cmd.Wait(); _ = log.Close() }()
	return cmd, nil
}
func stopOwned(ctx context.Context, file string) error {
	return stopOwnedSignal(ctx, file, syscall.SIGTERM)
}
func stopOwnedSignal(ctx context.Context, file string, signal syscall.Signal) error {
	var record processRecord
	e := snapshots.ReadCoordinatorJSON(file, &record)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	current, e := processIdentity(record.PID)
	if e != nil {
		_ = os.Remove(file)
		return nil
	}
	if record.PID <= 1 {
		return errors.New("invalid owned process record")
	}
	if current != record.Started {
		// 重启后 PID 可能复用；只丢弃过期记录，绝不终止占用该 PID 的其他进程。
		return os.Remove(file)
	}
	// 先让 runner 关闭其拥有的 Core/插件，再清理同一进程组的残留子进程。
	_ = syscall.Kill(record.PID, signal)
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, e = processIdentity(record.PID); e != nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			_ = syscall.Kill(-record.PID, syscall.SIGKILL)
			return fmt.Errorf("owned process %d required forced termination; inspect %s", record.PID, file)
		case <-ticker.C:
		}
	}
	_ = syscall.Kill(-record.PID, syscall.SIGKILL)
	return os.Remove(file)
}
func cleanEnvironment(root string, extra ...string) []string {
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin", "HOME=" + root, "TMPDIR=" + filepath.Join(root, "tmp"), "LANG=C.UTF-8"}
	return append(env, extra...)
}
