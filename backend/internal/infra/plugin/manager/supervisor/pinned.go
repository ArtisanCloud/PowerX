package supervisor

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

var ErrUnpinnedProcess = errors.New("plugin.runtime_not_pinned")

// freezeExecutable 固定实际启动字节，避免安装目录被覆盖或 symlink 变更时调用新实现。
// 初期只支持独立 Go 原生可执行文件；脚本、解释器和远程 dev 挂载明确拒绝。
func freezeExecutable(entry string) (path, directory, digest string, err error) {
	if info, readErr := buildinfo.ReadFile(entry); readErr != nil || info.Path == "cmd/go" {
		return "", "", "", ErrUnpinnedProcess
	}
	directory, err = os.MkdirTemp("", "powerx-plugin-image-")
	if err != nil {
		return "", "", "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(directory)
		}
	}()
	source, err := os.Open(entry)
	if err != nil {
		return "", "", "", err
	}
	defer source.Close()
	path = filepath.Join(directory, "plugin")
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0500)
	if err != nil {
		return "", "", "", err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(target, hash), io.LimitReader(source, (1<<30)+1))
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil || size > 1<<30 {
		return "", "", "", ErrUnpinnedProcess
	}
	if _, err = buildinfo.ReadFile(path); err != nil {
		return "", "", "", ErrUnpinnedProcess
	}
	ok = true
	return path, directory, "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

type PinnedProcess struct {
	Generation, ImplementationSHA256 string
	InstalledEntry                   string
	ImagePath                        string // 仅宿主诊断使用，不进入用户报告或回执。
	Port                             int
}

func (s *Supervisor) Pin(id string) (PinnedProcess, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pinLocked(id)
}
func (s *Supervisor) pinLocked(id string) (PinnedProcess, error) {
	p, ok := s.proc[id]
	if !ok || p.exited.Load() || p.imageDir == "" || p.info.State != ProcRunning || !p.info.Healthy {
		return PinnedProcess{}, ErrUnpinnedProcess
	}
	return PinnedProcess{Generation: strconv.Itoa(p.info.PID) + ":" + p.info.StartedAt.UTC().Format("20060102T150405.000000000Z"), ImplementationSHA256: p.imageSHA256, Port: p.port, InstalledEntry: p.installedEntry, ImagePath: p.cmd.Path}, nil
}

// WithPinnedProcess 阻止 stop/restart 与实际 HTTP 调用穿插；不允许端口复用改投新实现。
func (s *Supervisor) WithPinnedProcess(ctx context.Context, id string, expected PinnedProcess, call func(int, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	actual, err := s.pinLocked(id)
	if err != nil || actual != expected {
		return ErrUnpinnedProcess
	}
	if err := call(actual.Port, envToMap(s.proc[id].cmd.Env)["POWERX_INTERNAL_TOKEN"]); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// 只接受仍是同一个宿主进程；provider 的 200 不提供身份信任。
	if now, err := s.pinLocked(id); err != nil || now != expected {
		return fmt.Errorf("%w", ErrUnpinnedProcess)
	}
	return nil
}
