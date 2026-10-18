package system_recovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RunLocal owns a clone, never the developer's original PostgreSQL cluster or
// media. Stable web/API gateways make whole-environment switching observable.
func RunLocal(ctx context.Context, root string, source snapshots.Config, deployment Deployment, tools Tools, probes []BusinessProbe, webPort, apiPort int, prepareOnly bool, overrides ...ArtifactOverride) (result error) {
	root, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return e
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return e
	}
	lock, e := os.OpenFile(filepath.Join(root, "entry.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.New("该本地演练目录已有启动进程，请先停止该进程")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	profileFile := filepath.Join(root, "profile.json")
	webAddress := fmt.Sprintf("127.0.0.1:%d", webPort)
	apiAddress := fmt.Sprintf("127.0.0.1:%d", apiPort)
	if e = CheckLocalEntryPorts(webPort, apiPort); e != nil {
		return e
	}

	var profile Profile
	if _, e = os.Lstat(profileFile); e == nil {
		if prepareOnly {
			return errors.New("仅验证模式需要新的独立目录；已有环境不能作为本次重新验证的证明")
		}
		profile, e = ReadProfile(profileFile)
		if e != nil {
			return e
		}
		if len(overrides) > 0 && !reflect.DeepEqual(profile.Overrides, overrides) {
			return errors.New("程序覆盖合同与已有演练环境不同，请使用新的独立目录重新验证")
		}
		if !profile.Local || profile.GatewayWebListen != webAddress || profile.GatewayBackendListen != apiAddress {
			return errors.New("已存在的本地合同与请求入口不一致")
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	} else {
		cfg, e := sourceConfig(source)
		if e != nil {
			return e
		}
		if cfg.Deployment.Env != "dev" {
			return errors.New("本地恢复演练只允许 deployment.env=dev")
		}
		database, e := pgx.ParseConfig(source.DatabaseDSN)
		if e != nil {
			return errors.New("本地来源数据库配置无效")
		}
		if database.Host != "127.0.0.1" && database.Host != "localhost" && database.Host != "::1" && !filepath.IsAbs(database.Host) {
			return errors.New("本地演练拒绝连接远程来源数据库")
		}
		status, e := freePort()
		if e != nil {
			return e
		}
		profile = Profile{Overrides: overrides, Local: true, Protocol: config.RecoveryProtocol, InstanceID: uuid.NewString(), Directory: filepath.Join(root, "ops"), CandidateDirectory: filepath.Join(root, "c"), SnapshotDirectory: filepath.Join(root, "snapshots"), File: profileFile, Tools: tools, ExclusiveWriters: true, DedicatedRedis: true, SourceRedisRDB: filepath.Join(root, "source-unused.rdb"), StatusListen: fmt.Sprintf("127.0.0.1:%d", status), GatewayWebListen: webAddress, GatewayBackendListen: apiAddress, BusinessProbes: probes, Initial: deployment, Targets: []snapshots.RecoveryTarget{{ID: "local", Label: "本地开发演练", Environment: "dev", Mode: "formal", AllowOverwrite: true, AllowOnlineData: true}}}
		store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
		if e != nil {
			return e
		}
		// The original source is inventory only, never a managed deployment. Validate
		// the final profile after Initial points at the newly owned complete clone.
		source.Directory = filepath.Join(root, "bootstrap")
		source.Instance = profile.InstanceID
		if e = collectRuntimeFiles(cfg, &source, deployment, profile.Directory); e != nil {
			return e
		}
		backend := &snapshots.PostgresBackend{DSN: source.DatabaseDSN, MediaRoot: source.MediaRoot}
		service := snapshots.NewService(ctx, source, snapshots.Options{Coordinator: true, Backend: backend, DispatchCapture: func(context.Context, string) error { return nil }})
		defer service.Close()
		initial, e := service.Start(privileged(ctx), uuid.NewString())
		if e != nil {
			return e
		}
		if e = service.RunQueuedCapture(ctx, initial.ID); e != nil {
			return e
		}
		native := &Native{Profile: profile, Store: store, SnapshotStore: service}
		job := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "prepare", TargetID: "local", SnapshotID: initial.ID, RestoreMode: "new_environment"}
		d, checks, e := native.Prepare(ctx, &job)
		if e != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_ = StopCandidateProcesses(cleanup, filepath.Join(profile.CandidateDirectory, job.ID))
			return e
		}
		for _, check := range checks {
			if strings.HasPrefix(check.Name, "local_") || check.Name == "online_data_scope" {
				fmt.Println("演练范围：" + check.Detail)
			}
		}
		if e = StopCandidateProcesses(ctx, d.Root); e != nil {
			return e
		}
		profile.Initial = d
		if e = profile.Validate(); e != nil {
			return e
		}
		if e = snapshots.WriteCoordinatorJSON(profileFile, profile); e != nil {
			return e
		}
	}
	if !profile.InPlace {
		profile.Tools = tools
	}
	if e = upgradeLegacyLocalCore(&profile, deployment.CoreBinary, source.Version); e != nil {
		return e
	}
	if e = upgradeLegacyLocalWeb(ctx, &profile, deployment.WebOutput, source.Version); e != nil {
		return e
	}
	if e = isolateLocalSnapshotStorage(ctx, &profile); e != nil {
		return e
	}
	if e = adoptLocalInstallation(&profile); e != nil {
		return e
	}
	if e = profile.Validate(); e != nil {
		return e
	}
	if e = snapshots.WriteCoordinatorJSON(profile.File, profile); e != nil {
		return e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		entries, e := os.ReadDir(profile.CandidateDirectory)
		if e != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if e = StopCandidateProcesses(cleanup, filepath.Join(profile.CandidateDirectory, entry.Name())); e != nil {
				result = errors.Join(result, e)
			}
		}
	}()
	if prepareOnly {
		fmt.Println("本地完整副本已完成启动和只读验证，已停止自建进程；未切换活动入口。")
		return nil
	}
	fmt.Printf("本地恢复演练入口 http://%s/ops/backup\n原数据库和媒体保持不变；此入口使用完整开发副本，Redis 为独立本地实例。Ctrl+C 停止自建进程，环境数据保留，可用同一命令继续。\n", webAddress)
	e = RunCoordinator(ctx, profileFile)
	if errors.Is(e, context.Canceled) {
		return nil
	}
	return e
}

// Check before rebuilding Nuxt, so a mistaken invocation cannot disturb a
// developer-owned running dev server's build directory.
func CheckLocalEntryPorts(web, api int) error {
	if web < 1024 || web > 65535 || api < 1024 || api > 65535 || web == api {
		return errors.New("请选择两个不同的本地非特权端口")
	}
	listeners := []net.Listener{}
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	for _, port := range []int{web, api} {
		address := fmt.Sprintf("127.0.0.1:%d", port)
		l, e := net.Listen("tcp", address)
		if e != nil {
			return fmt.Errorf("入口 %s 被占用：请自行停止原开发 Core/Nuxt 后重试；工具不会关闭已有进程", address)
		}
		listeners = append(listeners, l)
	}
	return nil
}
