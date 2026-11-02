package storagebudget

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type operationKey struct{}
type operation struct {
	mu              sync.Mutex
	lease           *Lease
	peak, written   int64
	cancel          context.CancelFunc
	deadline        time.Time
	external        func() (int64, error)
	externalWritten int64
}

// Enabled 表示部署已明确开启共享写入准入。
func Enabled() bool { defaultMu.RLock(); defer defaultMu.RUnlock(); return defaultCoordinator != nil }

// BeginPath 将嵌套采集加入同一租约，避免快照调用归档时与自己竞争。
// 退出会等待续租器停止并释放租约；过期或续租失败会取消整个操作。
func BeginPath(ctx context.Context, path, name string, peak int64) (context.Context, func(), error) {
	if !Enabled() {
		return ctx, func() {}, nil
	}
	if old, _ := ctx.Value(operationKey{}).(*operation); old != nil {
		defaultMu.RLock()
		c := defaultCoordinator
		defaultMu.RUnlock()
		if c == nil || old.lease.c != c || !c.sameDevice(path) {
			return ctx, nil, ErrUnavailable
		}
		return ctx, func() {}, nil
	}
	lease, err := ReservePathDefault(ctx, path, name, peak)
	if err != nil {
		return ctx, nil, err
	}
	run, cancel := context.WithCancel(ctx)
	op := &operation{lease: lease, peak: peak, cancel: cancel, deadline: time.Now().Add(2 * time.Minute)}
	run = context.WithValue(run, operationKey{}, op)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-tick.C:
				op.mu.Lock()
				var err error
				if op.external != nil {
					var n int64
					n, err = op.external()
					if err == nil {
						op.written += n - op.externalWritten
						op.externalWritten = n
						if op.written > op.peak {
							err = ErrInsufficient
						}
					}
				}
				if err == nil {
					err = lease.Progress(run, op.written, op.peak, 2*time.Minute)
				}
				if err == nil {
					op.deadline = time.Now().Add(2 * time.Minute)
				}
				op.mu.Unlock()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	var once sync.Once
	finish := func() {
		once.Do(func() {
			cancel()
			<-done
			releaseCtx, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			for i := 0; i < 3; i++ {
				if err := lease.Release(releaseCtx); err == nil {
					return
				} else if !errors.Is(err, ErrBusy) {
					return
				}
				select {
				case <-releaseCtx.Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		})
	}
	return run, finish, nil
}
func (c *Coordinator) sameDevice(path string) bool {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		_, err := os.Stat(p)
		if err == nil {
			return sameDevice(p, c.device)
		}
		if !os.IsNotExist(err) || p == filepath.Dir(p) {
			return false
		}
	}
}

// Writer 在新增本地字节之前扩张租约；不能以任务开始时一次检查代替写入准入。
func Writer(ctx context.Context, w io.Writer) io.Writer {
	if op, _ := ctx.Value(operationKey{}).(*operation); op != nil {
		return operationWriter{ctx, op, w}
	}
	return contextWriter{ctx, w}
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

type operationWriter struct {
	ctx context.Context
	op  *operation
	w   io.Writer
}

func (w operationWriter) Write(p []byte) (int, error) {
	w.op.mu.Lock()
	defer w.op.mu.Unlock()
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if time.Now().After(w.op.deadline) {
		w.op.cancel()
		return 0, ErrUnavailable
	}
	needed := w.op.written + int64(len(p))
	if needed > w.op.peak {
		peak := needed + (8 << 20)
		if err := w.op.lease.Progress(w.ctx, w.op.written, peak, 2*time.Minute); err != nil {
			w.op.cancel()
			return 0, err
		}
		w.op.peak = peak
		w.op.deadline = time.Now().Add(2 * time.Minute)
	}
	n, err := w.w.Write(p)
	w.op.written += int64(n)
	return n, err
}

// TreeBytes 是有界峰值估算；未知来源拒绝准入，不把未完成扫描算作零。
func TreeBytes(ctx context.Context, root string) (int64, error) {
	var total int64
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return ErrUnavailable
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		count++
		if count > 200000 {
			return ErrUnavailable
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrUnavailable
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil || !info.Mode().IsRegular() || info.Size() > 1<<50-total {
			return ErrUnavailable
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// TrackOutput 计量固定脚本的最终文件及其受信任临时前缀；配合脚本文件大小硬限制。
// 不扫描上传内容或任意调用方指定的目录。
func TrackOutput(ctx context.Context, path string) error {
	op, _ := ctx.Value(operationKey{}).(*operation)
	if op == nil {
		return nil
	}
	op.mu.Lock()
	defer op.mu.Unlock()
	op.external = func() (int64, error) {
		dir, err := os.Open(filepath.Dir(path))
		if err != nil {
			return 0, ErrUnavailable
		}
		defer dir.Close()
		items, err := dir.Readdir(1025)
		if err != nil && err != io.EOF {
			return 0, ErrUnavailable
		}
		if len(items) > 1024 {
			return 0, ErrUnavailable
		}
		var size int64
		for _, info := range items {
			if info.Name() != filepath.Base(path) && !strings.HasPrefix(info.Name(), filepath.Base(path)+".tmp.") {
				continue
			}
			if !info.Mode().IsRegular() || info.Size() < 0 {
				return 0, ErrUnavailable
			}
			size += info.Size()
		}
		return size, nil
	}
	return nil
}
