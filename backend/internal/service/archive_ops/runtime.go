package archive_ops

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// RuntimeConfig 显式配置优先；无部署配置时发布私有运行快照供独立 worker 读取。
func RuntimeConfig(defaults Config) (Config, error) {
	if path := os.Getenv("POWERX_OPS_ARCHIVE_CONFIG"); path != "" {
		return LoadConfig(path)
	}
	if defaults.Directory == "" {
		defaults.Directory = DefaultDirectory()
	}
	if e := validateConfig(&defaults); e != nil {
		return defaults, e
	}
	return defaults, nil
}
func (s *Service) PublishRuntimeConfig() error {
	if s.initErr != nil {
		return s.initErr
	}
	if os.Getenv("POWERX_OPS_ARCHIVE_CONFIG") != "" {
		return nil
	}
	raw, e := marshalConfig(s.cfg)
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(s.cfg.Directory, "runtime-config.yaml"), raw)
}
func (s *Service) StartScheduler() {
	if s.initErr != nil {
		return
	}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.workers.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.workers.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(s.ctx, 35*time.Minute)
				_ = s.Tick(ctx)
				cancel()
			}
		}
	}()
}
