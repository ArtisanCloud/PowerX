package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// 回退绑定已完成的切换及调用方观察到的部署代次，不能接受任意库名或目录。
func (c *CoordinatorStore) EnqueueRollback(ctx context.Context, switchID, key string, generation uint64) (ManagedJob, error) {
	if e := root(ctx); e != nil {
		return ManagedJob{}, e
	}
	prior, e := c.Get(switchID)
	if e != nil {
		return ManagedJob{}, e
	}
	if (prior.Kind != "cutover" && prior.Kind != "overwrite") || prior.Status != "complete" || prior.PreviousID == "" {
		return ManagedJob{}, failure(409, "RECOVERY_ROLLBACK_UNAVAILABLE", "该任务没有可回退的原环境")
	}
	return c.enqueue(ctx, "rollback", prior.SnapshotID, prior.TargetID, key, generation, prior.ID)
}
func (c *CoordinatorStore) EnqueueCleanup(ctx context.Context, candidate, key string, generation uint64) (ManagedJob, error) {
	if e := root(ctx); e != nil {
		return ManagedJob{}, e
	}
	prior, e := c.Get(candidate)
	if e != nil {
		return ManagedJob{}, e
	}
	if prior.Kind != "prepare" || prior.Status == "queued" || prior.Status == "running" || prior.Status == "deleted" {
		return ManagedJob{}, failure(409, "RECOVERY_CLEANUP_UNAVAILABLE", "只能清理已停止准备的恢复环境")
	}
	return c.enqueue(ctx, "cleanup", prior.SnapshotID, prior.TargetID, key, generation, prior.ID)
}

// EnvironmentState returns only UI-safe deployment identity; private paths and
// connection settings never cross the Admin boundary.
func (c *CoordinatorStore) EnvironmentState() (map[string]any, error) {
	var state struct {
		Generation   uint64         `json:"generation"`
		Maintenance  bool           `json:"maintenance"`
		LastSwitchID string         `json:"last_switch_id"`
		JobID        string         `json:"job_id,omitempty"`
		Deployment   map[string]any `json:"deployment"`
	}
	if e := ReadCoordinatorJSON(filepath.Join(c.Directory, "active.json"), &state); e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	return map[string]any{"generation": state.Generation, "maintenance": state.Maintenance, "active_environment_id": state.Deployment["id"], "last_switch_id": state.LastSwitchID}, nil
}
