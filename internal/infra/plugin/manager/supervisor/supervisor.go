package supervisor

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const healthDebug = true

type Supervisor struct {
	mu   sync.RWMutex
	proc map[string]*process
}

type process struct {
	id   string
	cmd  *exec.Cmd
	port int

	opts Options
	info ProcInfo

	cancel context.CancelFunc
	wg     sync.WaitGroup

	logs *ringBuffer // 收集 stdout/stderr

}

func New() *Supervisor {
	return &Supervisor{proc: map[string]*process{}}
}

func (s *Supervisor) Start(ctx context.Context, id string, entry string, args []string, extraEnv map[string]string, opts Options) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.proc[id]; ok {
		return 0, fmt.Errorf("proc %s already running", id)
	}

	// 1) 选可用端口
	port, err := pickFreePort()
	if err != nil {
		return 0, err
	}

	// 2) 组装命令
	cmd := exec.CommandContext(ctx, entry, args...)
	env := os.Environ()
	env = append(env, fmt.Sprintf("PORT=%d", port))
	if extraEnv == nil {
		extraEnv = map[string]string{}
	}
	if v, ok := extraEnv["POWERX_HTTP_ADDR"]; !ok || strings.TrimSpace(v) == "" || v == DynamicBindPlaceholder {
		extraEnv["POWERX_HTTP_ADDR"] = fmt.Sprintf("127.0.0.1:%d", port)
	}
	for k, v := range extraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// 3) 准备 process
	pctx, cancel := context.WithCancel(context.Background())
	pr := &process{
		id:     id,
		cmd:    cmd,
		port:   port,
		opts:   defaultOpts(opts),
		cancel: cancel,
		info: ProcInfo{
			ID:        id,
			PID:       0,
			Port:      port,
			State:     ProcStarting,
			Healthy:   false,
			Restarts:  0,
			StartedAt: time.Now().UTC(),
		},
		logs: newRingBuffer(256 * 1024), // 256KB 环形日志
	}
	s.proc[id] = pr

	// 4) 将 stdout/stderr 输出到控制台 + 缓冲
	cmd.Stdout = io.MultiWriter(os.Stdout, pr.logs)
	cmd.Stderr = io.MultiWriter(os.Stderr, pr.logs)

	// 5) 启动
	if err := cmd.Start(); err != nil {
		delete(s.proc, id)
		return 0, err
	}
	pr.info.PID = cmd.Process.Pid
	pr.info.State = ProcStarting

	// 6) 后台监控
	pr.wg.Add(2)
	go pr.waitExit(&s.mu)
	go pr.healthLoop(pctx, &s.mu)

	return port, nil
}

func (s *Supervisor) Stop(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pr, ok := s.proc[id]
	if !ok {
		return nil
	}
	// 停健康循环
	if pr.cancel != nil {
		pr.cancel()
	}
	// 发 SIGTERM，等一会再 SIGKILL
	if pr.cmd != nil && pr.cmd.Process != nil {
		_ = pr.cmd.Process.Signal(syscall.SIGTERM)
		doneCh := make(chan struct{})
		go func() { pr.wg.Wait(); close(doneCh) }()
		select {
		case <-doneCh:
		case <-time.After(5 * time.Second):
			_ = pr.cmd.Process.Kill()
		}
	}
	pr.info.State = ProcStopped
	pr.info.StoppedAt = time.Now().UTC()
	delete(s.proc, id)
	return nil
}

func (s *Supervisor) Restart(ctx context.Context, id string) error {
	s.mu.RLock()
	pr, ok := s.proc[id]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("proc %s not running", id)
	}
	entry := pr.cmd.Path
	args := pr.cmd.Args[1:]
	env := envToMap(pr.cmd.Env)
	opts := pr.opts

	_ = s.Stop(id)
	_, err := s.Start(ctx, id, entry, args, env, opts)
	return err
}

func (s *Supervisor) Status(id string) (ProcInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pr, ok := s.proc[id]
	if !ok {
		return ProcInfo{ID: id, State: ProcStopped}, false
	}
	return pr.info, true
}

func (p *process) waitExit(mu *sync.RWMutex) {
	defer p.wg.Done()
	err := p.cmd.Wait()

	// 进程真正退出
	mu.Lock()
	defer mu.Unlock()

	if err != nil {
		p.info.LastExitErr = err.Error()
	}
	// 如果是 Stop 显式终止，外层会置 Stopped；这里分情况
	if p.info.State != ProcStopped {
		p.info.State = ProcExited
	}

	// 自愈：自动重启
	if p.opts.AutoRestart && p.info.State != ProcStopped {
		backoff := p.opts.BackoffBase
		if backoff <= 0 {
			backoff = time.Second
		}
		for {
			time.Sleep(backoff)
			// 重新拉起
			cmd := exec.Command(p.cmd.Path, p.cmd.Args[1:]...)
			cmd.Env = p.cmd.Env
			cmd.Stdout = io.MultiWriter(os.Stdout, p.logs)
			cmd.Stderr = io.MultiWriter(os.Stderr, p.logs)
			if err := cmd.Start(); err != nil {
				p.info.LastExitErr = err.Error()
			} else {
				p.cmd = cmd
				p.info.PID = cmd.Process.Pid
				p.info.State = ProcStarting
				p.info.Restarts++
				// 健康循环会继续存在（用 context 控制），无需再加一次
				return
			}
			// 退避上限
			backoff *= 2
			if backoff > p.opts.BackoffMax && p.opts.BackoffMax > 0 {
				backoff = p.opts.BackoffMax
			}
		}
	}
}

func (p *process) healthLoop(ctx context.Context, mu *sync.RWMutex) {
	defer p.wg.Done()

	if p.opts.HealthPath == "" {
		mu.Lock()
		p.info.State = ProcRunning
		p.info.Healthy = true
		mu.Unlock()
		return
	}

	cli := &http.Client{Timeout: p.opts.HealthTimeout}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", p.port, p.opts.HealthPath)
	interval := p.opts.HealthInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}

	if healthDebug {
		log.Printf("[plugin:health] start id=%s port=%d url=%s interval=%s", p.id, p.port, url, interval)
	}

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			if healthDebug {
				log.Printf("[plugin:health] stop  id=%s", p.id)
			}
			return
		case <-t.C:
			ok := probe(cli, url)

			mu.Lock()
			prevState, prevHealthy := p.info.State, p.info.Healthy
			if ok {
				if p.info.State == ProcStarting || p.info.State == ProcUnhealthy {
					p.info.State = ProcRunning
				}
				p.info.Healthy = true
				p.info.HealthOKCount++
				p.info.HealthFails = 0
			} else {
				p.info.HealthFails++
				p.info.Healthy = false
				p.info.State = ProcUnhealthy
				if p.info.HealthFails >= 5 && p.cmd != nil && p.cmd.Process != nil {
					if healthDebug {
						log.Printf("[plugin:health] term id=%s port=%d url=%s reason=consecutive_fails count=%d",
							p.id, p.port, url, p.info.HealthFails)
					}
					_ = p.cmd.Process.Signal(syscall.SIGTERM)
				}
			}
			curState, curHealthy := p.info.State, p.info.Healthy
			mu.Unlock()

			if healthDebug {
				log.Printf("[plugin:health] tick  id=%s port=%d url=%s ok=%v state:%s->%s healthy:%v->%v fail=%d ok=%d",
					p.id, p.port, url, ok, prevState, curState, prevHealthy, curHealthy, p.info.HealthFails, p.info.HealthOKCount)
			}
		}
	}
}

// ———— 工具 ————

func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func defaultOpts(o Options) Options {
	if o.HealthInterval <= 0 {
		o.HealthInterval = 2 * time.Second
	}
	if o.HealthTimeout <= 0 {
		o.HealthTimeout = 1 * time.Second
	}
	if o.BackoffBase <= 0 {
		o.BackoffBase = 1 * time.Second
	}
	if o.BackoffMax <= 0 {
		o.BackoffMax = 10 * time.Second
	}
	return o
}

func envToMap(env []string) map[string]string {
	out := map[string]string{}
	for _, e := range env {
		if kv, ok := splitOnce(e, '='); ok {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

func splitOnce(s string, sep byte) ([2]string, bool) {
	i := -1
	for idx := range s {
		if s[idx] == sep {
			i = idx
			break
		}
	}
	if i < 0 {
		return [2]string{}, false
	}
	return [2]string{s[:i], s[i+1:]}, true
}

func probe(cli *http.Client, url string) bool {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := cli.Do(req)
	if err != nil {
		// 吞掉连接类错误
		return false
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// Logs 返回插件最近 tailBytes 的日志（UTF-8 字符串；不是按行截断）
// 返回 (content, true) 表示拿到；( "", false ) 表示没有该进程或没有日志
func (s *Supervisor) Logs(id string, tailBytes int) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pr, ok := s.proc[id]
	if !ok || pr.logs == nil {
		return "", false
	}
	b := pr.logs.Snapshot(tailBytes)
	if len(b) == 0 {
		return "", true
	}
	return string(b), true
}
