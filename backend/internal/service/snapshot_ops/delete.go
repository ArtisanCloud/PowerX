package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// Delete 永久删除单份快照的内容、请求索引和展示记录；操作审计与恢复历史保留。
// 先写入不可恢复使用的删除状态，物理清理失败时保留可重试记录。
func (s *Service) Delete(ctx context.Context, id, confirmation string, deleteProtected bool) (SnapshotDeletion, error) {
	out := SnapshotDeletion{ID: id}
	if err := s.check(ctx); err != nil {
		return out, err
	}
	if !validID(id) || confirmation != id {
		return out, failure(400, "SNAPSHOT_DELETE_CONFIRMATION_REQUIRED", "请确认要永久删除的快照")
	}
	var coordinator *CoordinatorStore
	if s.opts.RecoveryStore != nil {
		coordinator = s.opts.RecoveryStore()
	}
	if coordinator != nil {
		unlock, err := lockFile(filepath.Join(coordinator.Directory, "locks", "queue.lock"))
		if err != nil {
			return out, err
		}
		defer unlock()
		jobs, err := coordinator.List()
		if err != nil {
			return out, err
		}
		for _, job := range jobs {
			pending := job.Status == "queued" || job.Status == "running" || job.Status == "attention_required" || job.Status == "verified"
			if pending && (job.SnapshotID == id || job.RollbackSnapshotID == id || job.Kind == "restore" || job.Kind == "resume") {
				return out, failure(409, "SNAPSHOT_IN_USE", "恢复任务仍在执行或需要处理，请完成后再删除快照")
			}
		}
	}
	unlock, err := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if err != nil {
		return out, err
	}
	defer unlock()
	point, err := s.snapshot(id)
	if err != nil {
		return out, err
	}
	switch point.Status {
	case "ready", "failed", "interrupted", "pruned", "deleting", "delete_failed":
	default:
		return out, failure(409, "SNAPSHOT_IN_USE", "快照仍在采集或校验，请完成后再删除")
	}
	if point.Protected && !deleteProtected {
		return out, failure(409, "SNAPSHOT_PROTECTED", "这份快照已受保护，请明确确认同时删除保护快照")
	}
	ids, err := s.ids("restores")
	if err != nil {
		return out, err
	}
	for _, restoreID := range ids {
		var restore Restore
		if err = readJSON(s.record("restores", restoreID), &restore); err != nil {
			return out, err
		}
		if restore.SnapshotID == id && (restore.Status == "queued" || restore.Status == "running") {
			return out, failure(409, "SNAPSHOT_IN_USE", "该快照正在恢复，请完成后再删除")
		}
	}
	if err = s.validateDeletePaths(id); err != nil {
		return out, err
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if err = s.audit(ctx, "delete", id, "started"); err != nil {
		return out, err
	}
	point.Status, point.Phase, point.ErrorCode, point.Error = "deleting", "deleting", "", ""
	point.UpdatedAt = s.opts.Now()
	if err = atomicJSON(s.record("snapshots", id), point); err != nil {
		return out, err
	}
	failed := func(cause error) (SnapshotDeletion, error) {
		point.Status, point.Phase = "delete_failed", "delete_failed"
		point.ErrorCode, point.Error = "SNAPSHOT_DELETE_FAILED", "快照文件清理未完成，请重试删除；该包暂不可用于恢复"
		point.UpdatedAt = s.opts.Now()
		_ = atomicJSON(s.record("snapshots", id), point)
		s.finishAudit(ctx, "delete", id, "failed")
		return out, dto.NewErrorWithCode(500, point.ErrorCode, point.Error, cause)
	}
	if err = os.RemoveAll(s.data(id)); err != nil {
		return failed(err)
	}
	if coordinator != nil && coordinator.CleanupSnapshotWork != nil {
		if err = coordinator.CleanupSnapshotWork(ctx, id); err != nil {
			return failed(err)
		}
	}
	if err = removeSnapshotFile(s.record("requests", id)); err != nil {
		return failed(err)
	}
	// 各目录先落盘，再移除展示记录，避免把未完成的物理清理报告成成功。
	for _, kind := range []string{"contents", "requests"} {
		if err = syncSnapshotDirectory(filepath.Join(s.cfg.Directory, kind)); err != nil {
			return failed(err)
		}
	}
	if err = os.Remove(s.record("snapshots", id)); err != nil {
		return failed(err)
	}
	if err = syncSnapshotDirectory(filepath.Join(s.cfg.Directory, "snapshots")); err != nil {
		return out, dto.NewErrorWithCode(500, "SNAPSHOT_DELETE_SYNC_FAILED", "快照已移除，但目录同步失败，请刷新后核对", err)
	}
	out.RecordDeleted, out.FilesDeleted = true, true
	s.finishAudit(ctx, "delete", id, "success")
	return out, nil
}

// 路径完全来自宿主 UUID，不接受调用方目录，也不跟随存储父目录或根软链接。
func (s *Service) validateDeletePaths(id string) error {
	for _, kind := range []string{"snapshots", "contents", "requests"} {
		dir := filepath.Join(s.cfg.Directory, kind)
		physical, err := filepath.EvalSymlinks(dir)
		if err != nil || physical != dir {
			return failure(409, "SNAPSHOT_DELETE_PATH_UNSAFE", "快照存储目录包含软链接或不可读取，拒绝清理")
		}
		path := s.record(kind, id)
		if kind == "contents" {
			path = s.data(id)
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (kind == "contents" && !info.IsDir()) || (kind != "contents" && !info.Mode().IsRegular()) {
			return failure(409, "SNAPSHOT_DELETE_PATH_UNSAFE", "快照文件路径不符合存储边界，拒绝清理")
		}
	}
	return nil
}

func removeSnapshotFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func syncSnapshotDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func missingSnapshotRecord(err error) bool {
	var app *dto.AppError
	return errors.As(err, &app) && app.HTTPCode == 404
}
