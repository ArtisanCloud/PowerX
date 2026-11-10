package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

func (s *Service) validateCaptureAdmission(j *Snapshot) error {
	if !s.controlRequired() && j.PolicyRevision == 0 || j.Trigger == "maintenance" && j.PolicyID == "" {
		return nil
	}
	if j.PolicyRevision < 1 || j.PolicyFingerprint != s.PolicyFingerprint() || j.AdmissionDeadline == nil || s.opts.Now().After(*j.AdmissionDeadline) || j.AdmissionDeadline.After(j.CreatedAt.Add(46*time.Minute)) {
		return failure(409, "SNAPSHOT_ADMISSION_UNCONFIRMED", "采集准入已过期或来源变化")
	}
	return nil
}
func (s *Service) finishCaptureCleanup(ctx context.Context, j *Snapshot) {
	var e error
	if s.controlRequired() || j.PolicyRevision > 0 {
		if j.PolicyID == "" {
			return
		}
		e = s.cleanupControlled(ctx, *j)
	} else {
		e = s.cleanup(ctx)
	}
	if e != nil {
		j.CleanupError = e.Error()
		_ = atomicJSON(s.record("snapshots", j.ID), j)
		s.finishAudit(ctx, "retention", j.ID, "stopped")
	}
}

// 只按本次已批准计划处理同计划历史。递归删除前先在短 SQL 守卫内原子隔离；未知意图不重做。
func (s *Service) cleanupControlled(ctx context.Context, capture Snapshot) error {
	if s.policyAuthority == nil {
		return failure(503, "SNAPSHOT_POLICY_CONTROL_UNAVAILABLE", "策略控制面不可用")
	}
	pendingReferences := map[string]bool{}
	var coordinator *CoordinatorStore
	if s.opts.RecoveryStore != nil {
		coordinator = s.opts.RecoveryStore()
	}
	if coordinator != nil {
		unlock, e := lockFile(filepath.Join(coordinator.Directory, "locks", "queue.lock"))
		if e != nil {
			return e
		}
		defer unlock()
		jobs, e := coordinator.List()
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if j.Status == "queued" || j.Status == "running" || j.Status == "attention_required" || j.Status == "verified" {
				pendingReferences[j.SnapshotID] = true
				pendingReferences[j.RollbackSnapshotID] = true
				if j.Kind != "capture" {
					return failure(409, "SNAPSHOT_IN_USE", "恢复任务未结束，暂停自动清理")
				}
			}
		}
	}
	ids, e := s.ids("snapshots")
	if e != nil {
		return e
	}
	rows := []Snapshot{}
	for _, id := range ids {
		j, err := s.snapshot(id)
		if err != nil {
			return err
		}
		if j.PolicyID == capture.PolicyID && !j.Imported {
			if j.RetentionToken != "" && j.Status != "pruned" {
				return failure(409, "SNAPSHOT_RETENTION_UNCONFIRMED", "旧删除意图需人工核对")
			}
			if j.Status == "ready" {
				rows = append(rows, j)
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	restores, e := s.ids("restores")
	if e != nil {
		return e
	}
	references := pendingReferences
	for _, id := range restores {
		var r Restore
		if e = readJSON(s.record("restores", id), &r); e != nil {
			return e
		}
		if r.Status == "queued" || r.Status == "running" {
			references[r.SnapshotID] = true
		}
	}
	for index, j := range rows {
		if index == 0 || j.Protected || references[j.ID] {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e = s.validateDeletePaths(j.ID); e != nil {
			return e
		}
		token := uuid.NewString()
		quarantine := filepath.Join(s.cfg.Directory, "retention", token)
		entered := false
		expired := false
		e = s.policyAuthority.Guard(ctx, capture.PolicyID, capture.PolicyRevision, "cleanup", func(p PolicyEntry) error {
			if index < p.Settings.RetentionCount && s.opts.Now().Sub(j.CreatedAt) < time.Duration(p.Settings.RetentionDays)*24*time.Hour {
				return nil
			}
			expired = true
			current, err := s.snapshot(j.ID)
			if err != nil {
				return err
			}
			if current.Status != "ready" || current.Protected || current.RetentionToken != "" {
				return failure(409, "SNAPSHOT_RETENTION_CHANGED", "快照状态变化")
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			entered = true
			j.RetentionToken = token
			j.Status = "pruning"
			j.Phase = "pruning"
			j.UpdatedAt = s.opts.Now()
			if err = atomicJSON(s.record("snapshots", j.ID), j); err != nil {
				return err
			}
			if _, err = s.retentionDirectory(true); err != nil {
				return err
			}
			if err = os.Rename(s.data(j.ID), quarantine); err != nil {
				return err
			}
			if err = syncSnapshotDirectory(filepath.Join(s.cfg.Directory, "contents")); err != nil {
				return err
			}
			return syncSnapshotDirectory(filepath.Dir(quarantine))
		})
		if e != nil {
			if entered {
				j.Status = "prune_unconfirmed"
				j.Phase = j.Status
				j.CleanupError = e.Error()
				_ = atomicJSON(s.record("snapshots", j.ID), j)
			}
			return e
		}
		if !expired {
			continue
		}
		if _, e = s.retentionDirectory(false); e == nil {
			e = os.RemoveAll(quarantine)
		}
		if e == nil {
			e = syncSnapshotDirectory(filepath.Dir(quarantine))
		}
		if e != nil {
			j.Status = "prune_unconfirmed"
			j.Phase = j.Status
			j.CleanupError = e.Error()
			_ = atomicJSON(s.record("snapshots", j.ID), j)
			return e
		}
		j.Status = "pruned"
		j.Phase = "pruned"
		j.UpdatedAt = s.opts.Now()
		if e = atomicJSON(s.record("snapshots", j.ID), j); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) removeRetentionQuarantine(j Snapshot) error {
	if j.RetentionToken == "" {
		return nil
	}
	if !validID(j.RetentionToken) {
		return errors.New("invalid retention intent")
	}
	root, e := s.retentionDirectory(false)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	path := filepath.Join(root, j.RetentionToken)
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe retention quarantine")
	}
	return os.RemoveAll(path)
}

// 与普通快照路径保持相同边界：隔离父目录必须是真实私有目录，不能借链接指向另一个安装。
func (s *Service) retentionDirectory(create bool) (string, error) {
	path := filepath.Join(s.cfg.Directory, "retention")
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) && create {
		if e = os.Mkdir(path, 0700); e != nil && !os.IsExist(e) {
			return "", e
		}
		info, e = os.Lstat(path)
	}
	if e != nil {
		return "", e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return "", failure(409, "SNAPSHOT_RETENTION_PATH_UNSAFE", "隔离目录必须为本机私有普通目录")
	}
	real, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", e
	}
	if real != path {
		return "", failure(409, "SNAPSHOT_RETENTION_PATH_UNSAFE", "隔离目录越过存储边界")
	}
	return path, nil
}
