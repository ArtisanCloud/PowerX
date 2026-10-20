package system_recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

// RunCoordinator 与业务 Core 分开部署，调度、进度和切换不依赖业务数据库。
func RunCoordinator(ctx context.Context, file string) error {
	profile, e := ReadProfile(file)
	if e != nil {
		return e
	}
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	if e != nil {
		return e
	}
	service := snapshots.NewService(ctx, snapshots.Config{Directory: profile.SnapshotDirectory}, snapshots.Options{Coordinator: true})
	defer service.Close()
	native := &Native{Profile: profile, Store: store, SnapshotStore: service}
	coordinator := &Coordinator{Store: store, Driver: native, Initial: profile.Initial}
	if profile.InPlace {
		active, err := coordinator.Active()
		if err != nil {
			return err
		}
		if active.Deployment.ID != profile.Initial.ID || active.Deployment.Root != profile.Initial.Root || active.Deployment.ConfigFile != profile.Initial.ConfigFile {
			return errors.New("当前账本与固定实例不一致，已阻止启动其他环境")
		}
	}
	// 每次调度读取当前 Core 发布的密文配置，切换以后来源随 active 部署更新。
	schedulerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-schedulerCtx.Done():
				return
			case <-ticker.C:
				source, e := service.LoadCoordinatorSource()
				if e != nil {
					continue
				}
				scheduler := snapshots.NewService(schedulerCtx, source, snapshots.Options{Coordinator: true, Backend: &snapshots.PostgresBackend{DSN: source.DatabaseDSN, MediaRoot: source.MediaRoot}, DispatchCapture: store.EnqueueCapture})
				_ = scheduler.Tick(schedulerCtx)
				scheduler.Close()
			}
		}
	}()
	return coordinator.runWithStatus(ctx, profile.StatusListen)
}

// RunActive 是 systemd 唯一的实例启动入口，所有组件读取同一份 active 指针。
func RunActive(ctx context.Context, file string) error {
	profile, e := ReadProfile(file)
	if e != nil {
		return e
	}
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	if e != nil {
		return e
	}
	coordinator := Coordinator{Store: store, Initial: profile.Initial}
	active, e := coordinator.Active()
	if e != nil {
		return e
	}
	if !active.Deployment.SourceManaged {
		// 新部署由协调器按 active 指针恢复；旧单元不再启动旧 Core。
		<-ctx.Done()
		return ctx.Err()
	}
	d := active.Deployment
	var mode struct {
		Hold bool `json:"hold"`
	}
	mode.Hold = active.Maintenance
	_ = snapshots.ReadCoordinatorJSON(filepath.Join(profile.Directory, "source-mode.json"), &mode)
	spec := RunSpec{Deployment: d, Tools: profile.Tools, Hold: mode.Hold, StatusURL: "http://" + profile.StatusListen}
	// 旧部署不重新创建 PG 或 Redis，维护启动在 Core 构造 worker 之前生效。
	file = filepath.Join(profile.Directory, "source-run.json")
	if e := snapshots.WriteCoordinatorJSON(file, spec); e != nil {
		return e
	}
	return RunInstance(ctx, file)
}

// EnsureActive 在协调器或服务器重启后继续运行已确认的完整部署，不回到旧组件组合。
func (n *Native) EnsureActive(ctx context.Context, active Active) error {
	if n.Profile.InPlace && (active.Deployment.ID != n.Profile.Initial.ID || active.Deployment.ConfigFile != n.Profile.Initial.ConfigFile) {
		return errors.New("启动记录偏离固定实例，拒绝启动")
	}
	if active.Maintenance || active.Deployment.SourceManaged {
		return nil
	}
	var record processRecord
	if snapshots.ReadCoordinatorJSON(filepath.Join(active.Deployment.Root, "processes/instance.json"), &record) == nil {
		if identity, e := processIdentity(record.PID); e == nil && identity == record.Started {
			return nil
		}
	}
	return n.Start(ctx, active.Deployment, false)
}

// StopCandidateProcesses 只供本机验收清理或显式运维清理，不删除候选持久数据。
func StopCandidateProcesses(ctx context.Context, root string) error {
	var failures []error
	for _, name := range []string{"instance", "postgres"} {
		signal := syscall.SIGTERM
		if name == "postgres" {
			signal = syscall.SIGINT
		} // fast shutdown closes clients; TERM is smart shutdown and can wait forever
		if e := stopOwnedSignal(ctx, filepath.Join(root, "processes", name+".json"), signal); e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}

func (n *Native) CleanupFailed(ctx context.Context, j snapshots.ManagedJob) error {
	switch j.Kind {
	case "restore":
		if j.Status != "rolled_back" {
			return nil
		}
		if uuid.Validate(j.ID) != nil {
			return errors.New("恢复清理任务身份无效")
		}
		for _, prefix := range []string{"retained-failed-", "retained-interrupted-"} {
			root := filepath.Join(n.Profile.CandidateDirectory, prefix+j.ID)
			if !canonicalNewPath(root) || root == n.Profile.Initial.Root {
				return errors.New("恢复补偿清理目录越界")
			}
			if err := StopCandidateProcesses(ctx, root); err != nil {
				return err
			}
			if err := os.RemoveAll(root); err != nil {
				return err
			}
		}
		return nil
	case "prepare":
		return StopCandidateProcesses(ctx, filepath.Join(n.Profile.CandidateDirectory, j.ID))
	case "capture":
		return n.SnapshotStore.FailCoordinatorCapture(j.SnapshotID, "协调器采集失败或中断；原站恢复状态见独立任务")
	}
	return nil
}
