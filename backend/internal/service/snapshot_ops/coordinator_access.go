package snapshot_ops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// SaveCoordinatorSource 只供本机宿主装配：配置和来源连接仅以主密钥密文保存。
func (s *Service) SaveCoordinatorSource(c Config) error {
	if s.initErr != nil {
		return s.initErr
	}
	raw, e := json.Marshal(c)
	if e != nil {
		return e
	}
	enc, e := encryptConfig(raw, s.key)
	if e != nil {
		return e
	}
	return atomicFile(filepath.Join(s.cfg.Directory, "source.enc"), enc)
}

// LoadCoordinatorSource 不通过 HTTP 暴露，独立运维进程必须具有快照目录权限。
func (s *Service) LoadCoordinatorSource() (Config, error) {
	var c Config
	enc, e := os.ReadFile(filepath.Join(s.cfg.Directory, "source.enc"))
	if e != nil {
		return c, e
	}
	raw, e := decryptConfig(enc, s.key)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(raw, &c)
	return c, e
}

// RunQueuedCapture 同步执行已登记的在线采集任务，不暂停来源服务。
func (s *Service) RunQueuedCapture(ctx context.Context, id string) error {
	if !validID(id) {
		return errors.New("invalid snapshot id")
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	j, e := s.snapshot(id)
	if e != nil {
		return e
	}
	if j.Status == "ready" {
		return nil
	}
	if j.Status != "queued" {
		return errors.New("capture is not queued")
	}
	e = s.captureWithRetry(ctx, &j)
	if e != nil {
		j.Status = "failed"
		j.Phase = "failed"
		j.ErrorCode = "SNAPSHOT_CAPTURE_FAILED"
		j.Error = e.Error()
		_ = os.RemoveAll(s.data(id))
	} else {
		j.Status = "ready"
		j.Phase = "complete"
	}
	j.UpdatedAt = s.opts.Now()
	if write := atomicJSON(s.record("snapshots", id), j); write != nil {
		return write
	}
	s.finishAudit(ctx, "capture", id, j.Status)
	if e == nil {
		_ = s.cleanup(ctx)
	}
	return e
}

// FailCoordinatorCapture 关闭未执行或被中断的协调器采集，避免 queued 永久占住入口。
func (s *Service) FailCoordinatorCapture(id, reason string) error {
	if !validID(id) {
		return errors.New("invalid snapshot id")
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	j, e := s.snapshot(id)
	if e != nil {
		return e
	}
	if j.Status != "queued" && j.Status != "capturing" && j.Status != "verifying" {
		return nil
	}
	j.Status = "failed"
	j.Phase = "failed"
	j.ErrorCode = "SNAPSHOT_COORDINATOR_INTERRUPTED"
	j.Error = reason
	j.UpdatedAt = s.opts.Now()
	if e = atomicJSON(s.record("snapshots", id), j); e != nil {
		return e
	}
	return os.RemoveAll(s.data(id))
}

// MaterializeForRecovery 校验并复制完整内容到新私有目录；不会调用包内可执行程序。
func (s *Service) MaterializeForRecovery(ctx context.Context, id, target string) (Manifest, map[string][]byte, error) {
	var m Manifest
	if !validID(id) || !filepath.IsAbs(target) {
		return m, nil, errors.New("invalid recovery identity")
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return m, nil, e
	}
	defer unlock()
	j, e := s.snapshot(id)
	if e != nil || j.Status != "ready" {
		return m, nil, errors.New("snapshot is not ready")
	}
	if e = readJSON(filepath.Join(s.data(id), "manifest.json"), &m); e != nil {
		return m, nil, e
	}
	if e = validateManifest(m, id); e != nil {
		return m, nil, e
	}
	if e = verifyFiles(ctx, s.data(id), m); e != nil {
		return m, nil, e
	}
	wrapped, e := os.ReadFile(filepath.Join(s.data(id), "key.enc"))
	if e != nil {
		return m, nil, e
	}
	key, e := decryptConfig(wrapped, s.key)
	if e != nil {
		return m, nil, e
	}
	configuration, e := readConfiguration(s.data(id), key)
	if e != nil {
		return m, nil, e
	}
	if _, e = os.Lstat(target); !errors.Is(e, os.ErrNotExist) {
		return m, nil, errors.New("target must be a new directory")
	}
	if e = os.MkdirAll(target, 0700); e != nil {
		return m, nil, e
	}
	for _, entry := range m.Files {
		if e = copyTree(ctx, filepath.Join(s.data(id), filepath.FromSlash(entry.Path)), filepath.Join(target, filepath.FromSlash(entry.Path))); e != nil {
			return m, nil, e
		}
	}
	return m, configuration, nil
}
