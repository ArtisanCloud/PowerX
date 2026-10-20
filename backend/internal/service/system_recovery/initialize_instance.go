package system_recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

// InitializeInstallation restores into an empty registered server. The same
// fixed paths and service command are used for all later restores and restarts.
func InitializeInstallation(ctx context.Context, file, snapshotID, confirmation string) (result error) {
	if uuid.Validate(snapshotID) != nil || snapshotID != confirmation {
		return errors.New("请以 -confirmation 明确确认导入的快照 UUID")
	}
	p, err := ReadProfile(file)
	if err != nil {
		return err
	}
	if !p.InPlace {
		return errors.New("初始化需要 restore_in_place 固定实例合同")
	}
	store, err := snapshots.NewCoordinatorStore(p.Directory, p.Targets)
	if err != nil {
		return err
	}
	unlock, err := store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	for _, path := range []string{p.Initial.Root, filepath.Join(p.Directory, "active.json")} {
		if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return errors.New("目标实例或运行记录已经存在，初始化不会覆盖；请使用正常恢复")
		}
	}
	if p.Initial.CorePort < 1024 || p.Initial.GRPCPort < 1024 || p.Initial.WebPort < 1024 || p.Initial.RedisPort < 1024 || p.Initial.PostgresPort < 1024 || p.Initial.PluginPortMin < 1024 || p.Initial.PluginPortMax < p.Initial.PluginPortMin {
		return errors.New("初始化需要登记完整的非特权实例端口")
	}
	service := snapshots.NewService(ctx, snapshots.Config{Directory: p.SnapshotDirectory}, snapshots.Options{Coordinator: true})
	defer service.Close()
	n := &Native{Profile: p, Store: store, SnapshotStore: service}
	j := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "initialize", SnapshotID: snapshotID, TargetID: p.Targets[0].ID, RestoreMode: "replace_instance", Status: "running", CreatedAt: time.Now().UTC(), Operator: "operator.initialize_instance"}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if e := StopCandidateProcesses(cleanup, p.Initial.Root); e != nil {
			result = errors.Join(result, e)
		}
		if result != nil {
			j.Status = "failed"
			j.Error = result.Error()
		} else {
			j.Status = "complete"
			j.Phase = "complete"
			j.BusinessVerified = true
		}
		if e := store.Save(j); e != nil {
			result = errors.Join(result, e)
		}
	}()
	d, checks, err := n.prepare(ctx, &j, &p.Initial)
	j.Checks = checks
	if err != nil {
		return err
	}
	if !checksPassed(checks) {
		return errors.New("目标服务器恢复验证未通过")
	}
	if d.ID != p.Initial.ID || d.Root != p.Initial.Root || d.ConfigFile != p.Initial.ConfigFile {
		return errors.New("初始化改变了固定实例路径")
	}
	return snapshots.WriteCoordinatorJSON(filepath.Join(p.Directory, "active.json"), Active{Generation: 1, Deployment: d})
}
