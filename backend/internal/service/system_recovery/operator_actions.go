package system_recovery

import (
	"context"
	"errors"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

// Local operator entry: possession of the private profile is required. No
// progress ticket or arbitrary path/DSN can authorize an environment action.
func QueueEnvironmentAction(ctx context.Context, file, action, id, confirmation string, generation uint64) (snapshots.ManagedJob, error) {
	if id == "" || id != confirmation || generation == 0 {
		return snapshots.ManagedJob{}, errors.New("请先核对 recovery-status，显式提供相同 job/confirmation 和当前 generation")
	}
	profile, e := ReadProfile(file)
	if e != nil {
		return snapshots.ManagedJob{}, e
	}
	if profile.InPlace {
		return snapshots.ManagedJob{}, errors.New("固定实例不支持环境切换或删除；请选择保护快照执行恢复")
	}
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	if e != nil {
		return snapshots.ManagedJob{}, e
	}
	if !store.Online() {
		return snapshots.ManagedJob{}, errors.New("独立协调器尚未在线")
	}
	switch action {
	case "rollback-environment":
		return store.EnqueueRollback(privileged(ctx), id, uuid.NewString(), generation)
	case "cleanup-environment":
		return store.EnqueueCleanup(privileged(ctx), id, uuid.NewString(), generation)
	}
	return snapshots.ManagedJob{}, errors.New("未知环境操作")
}
func EnvironmentStatus(file string) (map[string]any, error) {
	profile, e := ReadProfile(file)
	if e != nil {
		return nil, e
	}
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	if e != nil {
		return nil, e
	}
	state, e := store.EnvironmentState()
	if e != nil {
		return nil, e
	}
	jobs, e := store.List()
	if e != nil {
		return nil, e
	}
	for i := range jobs {
		jobs[i].RequestHash = ""
	}
	state["jobs"] = jobs
	state["online"] = store.Online()
	return state, nil
}
