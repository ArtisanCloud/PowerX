package system_recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// cleanupSnapshotWork 由快照删除在入队锁内调用；只删除已结束任务的私有展开目录。
// 不读取 journal 中的安装路径，不删除当前数据库、媒体或相邻回退文件。
func (r *CurrentRuntime) cleanupSnapshotWork(ctx context.Context, id string) error {
	jobs, err := r.Coordinator.Store.List()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.SnapshotID != id && job.RollbackSnapshotID != id {
			continue
		}
		switch job.Status {
		case "failed", "complete", "rolled_back", "interrupted", "deleted":
		default:
			return errors.New("恢复任务尚未结束，不能清理其暂存文件")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		work := r.work(job.ID)
		physical, err := currentRestoreTarget(work)
		if err != nil || !inside(filepath.Join(r.spec.Directory, "work"), physical) {
			return errors.New("恢复暂存目录不符合当前维护边界，拒绝清理")
		}
		if err = os.RemoveAll(physical); err != nil {
			return err
		}
	}
	return nil
}

func (r *CurrentRuntime) configureSnapshotCleanup() {
	r.Coordinator.Store.CleanupSnapshotWork = r.cleanupSnapshotWork
}
