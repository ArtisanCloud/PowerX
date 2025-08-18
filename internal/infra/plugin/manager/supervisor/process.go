package supervisor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type Handle struct {
	ID      string
	Cmd     *exec.Cmd
	Port    int
	BaseURL string // e.g. "http://127.0.0.1:31001"
}

type Supervisor struct {
	mu    sync.Mutex
	procs map[string]*Handle // pluginID -> handle
}

func New() *Supervisor {
	return &Supervisor{procs: map[string]*Handle{}}
}

func (s *Supervisor) Start(ctx context.Context, id, entry string, args, env []string, preferredPort int) (*Handle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.procs[id]; ok {
		return nil, fmt.Errorf("process already started: %s", id)
	}

	port := preferredPort
	if port == 0 {
		p, err := getFreePort()
		if err != nil {
			return nil, err
		}
		port = p
	}

	cmd := exec.CommandContext(ctx, entry, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "PORT="+strconv.Itoa(port))
	// 把工作目录设为 entry 所在目录（便于相对路径资源）
	cmd.Dir = filepath.Dir(entry)
	// 日志直打到宿主 stdout/stderr（MVP）
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// 让子进程随宿主退出
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &Handle{ID: id, Cmd: cmd, Port: port, BaseURL: "http://127.0.0.1:" + strconv.Itoa(port)}
	s.procs[id] = h
	return h, nil
}

func (s *Supervisor) Stop(ctx context.Context, id string) error {
	s.mu.Lock()
	h, ok := s.procs[id]
	if ok {
		delete(s.procs, id)
	}
	s.mu.Unlock()
	if !ok || h.Cmd == nil || h.Cmd.Process == nil {
		return nil
	}
	// 优雅停止
	_ = h.Cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = h.Cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = h.Cmd.Process.Kill()
	}
	return nil
}

func getFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
