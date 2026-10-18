package snapshot_ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
)

// captureWithRetry 在同一个持久化任务内有界重试；不增加私有队列，不查询页面状态。
// 每次重新取得 MVCC 并清除上次的半成品，绝不混合两个恢复点的文件。
func (s *Service) captureWithRetry(ctx context.Context, j *Snapshot) error {
	delays := s.captureRetryDelays
	if delays == nil {
		delays = []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second}
	}
	return s.captureAttempts(ctx, j, delays)
}
func (s *Service) captureAttempts(ctx context.Context, j *Snapshot, delays []time.Duration) error {
	for attempt := 0; ; attempt++ {
		if e := ctx.Err(); e != nil {
			return e
		}
		j.Attempt = attempt + 1
		j.RetryAt = nil
		j.Error = ""
		j.ErrorCode = ""
		j.CapturedAt = nil
		j.Components = nil
		j.RecoverySupported = false
		e := s.capture(ctx, j)
		if e == nil {
			return nil
		}
		if !errors.Is(e, archiveops.ErrSourceChanged) {
			return e
		}
		if attempt >= len(delays) {
			return fmt.Errorf("备份来源持续变化，已尝试 %d 次，业务未暂停；请稍后重新创建快照: %w", attempt+1, e)
		}
		if e := s.resetCaptureData(j.ID); e != nil {
			return e
		}
		retryAt := s.opts.Now().Add(delays[attempt])
		j.RetryAt = &retryAt
		j.Status = "capturing"
		j.Phase = "retry_wait"
		j.ErrorCode = "SNAPSHOT_SOURCE_CHANGED"
		j.Error = "备份来源同时发生变化，备份将自动重试；业务可继续操作"
		j.CapturedAt = nil
		j.UpdatedAt = s.opts.Now()
		if e := atomicJSON(s.record("snapshots", j.ID), j); e != nil {
			return e
		}
		timer := time.NewTimer(delays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			j.RetryAt = nil
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (s *Service) resetCaptureData(id string) error {
	// 保留原始请求用于导出/幂等核对，仅移除本次未封存内容。
	dir := s.data(id)
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.Name() == "request.json" {
			continue
		}
		if e := os.RemoveAll(filepath.Join(dir, entry.Name())); e != nil {
			return e
		}
	}
	return nil
}
