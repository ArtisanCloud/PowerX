package system_recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

const CurrentRecoveryProtocol = "powerx.current-recovery/v1"

// currentSpec 只记录当前安装的身份和路径，不保存密码，也不创建第二套业务环境。
type currentSpec struct {
	Protocol          string   `json:"protocol"`
	ID                string   `json:"id"`
	ConfigFile        string   `json:"config_file"`
	Directory         string   `json:"directory"`
	SnapshotDirectory string   `json:"snapshot_directory"`
	Environment       string   `json:"environment"`
	Listen            string   `json:"listen"`
	Root              string   `json:"root"`
	Executable        string   `json:"executable"`
	Args              []string `json:"args"`
	ConnectionHash    string   `json:"connection_hash"`
}

// CurrentRuntime 随 Core 启动，只在恢复时 exec 为维护模式；没有常驻子进程或新端口。
type CurrentRuntime struct {
	spec        currentSpec
	Coordinator *Coordinator
	requested   atomic.Bool
	closeOnce   sync.Once
	close       func()
	lifecycle   context.Context
}

func currentDirectory(snapshotDirectory, configFile string) string {
	sum := sha256.Sum256([]byte(configFile))
	return filepath.Join(snapshotDirectory, "current-recovery", hex.EncodeToString(sum[:12]))
}

func privateDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("恢复状态目录必须是绝对路径")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("恢复状态目录必须是私有普通目录")
	}
	return nil
}

// CompleteCurrentSource 将辅助配置加入正常快照清单；敏感正文仍由快照服务加密。
func CompleteCurrentSource(cfg *config.Config, configFile string, source *snapshots.Config) error {
	if cfg.Recovery.ProfileFile != "" || cfg.Recovery.ReadOnly() || configFile == "" {
		return nil
	}
	directory := currentDirectory(source.Directory, configFile)
	if err := privateDirectory(directory); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	return collectRuntimeFiles(cfg, source, Deployment{Root: root, ConfigFile: configFile}, directory)
}

func NewCurrentRuntime(ctx context.Context, cfg *config.Config, configFile string, source snapshots.Config, service *snapshots.Service) (*CurrentRuntime, error) {
	if cfg.Recovery.ProfileFile != "" || cfg.Recovery.ReadOnly() {
		return nil, nil
	}
	if configFile == "" || service == nil {
		return nil, errors.New("当前恢复需要实际配置文件和快照目录")
	}
	file, err := filepath.Abs(configFile)
	if err != nil {
		return nil, err
	}
	// 快照服务已初始化目录；使用同一个物理路径判断与业务文件的边界。
	source.Directory, err = filepath.EvalSymlinks(source.Directory)
	if err != nil {
		return nil, err
	}
	directory := currentDirectory(source.Directory, file)
	if err = privateDirectory(directory); err != nil {
		return nil, err
	}
	var spec currentSpec
	err = snapshots.ReadCoordinatorJSON(filepath.Join(directory, "installation.json"), &spec)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if spec.ID == "" {
		spec.ID = uuid.NewString()
	}
	if spec.ConfigFile != "" && spec.ConfigFile != file {
		return nil, errors.New("当前恢复配置身份不匹配")
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(source.DatabaseDSN))
	spec.Protocol, spec.ConfigFile, spec.Directory, spec.SnapshotDirectory = CurrentRecoveryProtocol, file, directory, source.Directory
	spec.Environment, spec.Root, spec.Listen = cfg.Deployment.Env, root, fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if strings.HasPrefix(spec.Listen, ":") {
		spec.Listen = "127.0.0.1" + spec.Listen
	}
	spec.Executable, spec.Args, spec.ConnectionHash = executable, append([]string{}, os.Args...), hex.EncodeToString(hash[:])
	// 当前执行器不取得外部 Redis 服务的进程所有权。旧在线包只允许 dev 数据恢复。
	allowed := cfg.Deployment.Env == "dev" && (cfg.Database.Driver == "postgres" || cfg.Database.Driver == "postgresql") && cfg.Storage.DefaultDriver == "local"
	store, err := snapshots.NewCoordinatorStore(directory, []snapshots.RecoveryTarget{{ID: "current", Label: "当前实例", Environment: cfg.Deployment.Env, Mode: "formal", AllowOverwrite: allowed, AllowOnlineData: allowed}})
	if err != nil {
		return nil, err
	}
	r := &CurrentRuntime{spec: spec}
	store.SnapshotStore = service
	d := Deployment{ID: spec.ID, Root: root, ConfigFile: file, CoreBinary: executable, CorePort: cfg.Server.Port, SourceManaged: true}
	r.Coordinator = &Coordinator{Store: store, Initial: d, SnapshotStore: service}
	r.configureSnapshotCleanup()
	if _, err = r.Coordinator.Active(); err != nil {
		return nil, err
	}
	if err = snapshots.WriteCoordinatorJSON(filepath.Join(directory, "installation.json"), spec); err != nil {
		return nil, err
	}
	if err = snapshots.WriteCoordinatorJSON(file+".recovery.json", map[string]string{"protocol": CurrentRecoveryProtocol, "config_file": file, "directory": directory}); err != nil {
		return nil, err
	}
	if err = service.SaveCoordinatorSource(source); err != nil {
		return nil, err
	}
	if _, err = r.Coordinator.EnsureRecoveryAccess(); err != nil {
		return nil, err
	}
	store.RecoveryEntry = recoveryEntry
	if !allowed {
		store.RestoreProblem = "当前普通启动的原地恢复仅开放 dev、PostgreSQL 和本地媒体；生产队列及外部依赖的完整恢复合同尚未开放，快照仍可导出。"
	}
	store.PrepareSession = func(w http.ResponseWriter, request *http.Request, operator string) error {
		_, err := r.Coordinator.issueRecoverySession(w, request, operator, "root")
		return err
	}
	return r, nil
}

func (r *CurrentRuntime) Handler() http.Handler {
	progress := r.Coordinator.StatusHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.lifecycle != nil && strings.HasSuffix(req.URL.Path, "/events") {
			stream, cancel := context.WithCancel(req.Context())
			defer cancel()
			stop := context.AfterFunc(r.lifecycle, cancel)
			defer stop()
			req = req.WithContext(stream)
		}
		if strings.HasPrefix(req.URL.Path, recoveryEntry+"jobs/") {
			progress.ServeHTTP(w, req)
			return
		}
		r.Coordinator.portal(w, req)
	})
}

func (r *CurrentRuntime) startHeartbeat(ctx context.Context) error {
	write := func() error {
		return snapshots.WriteCoordinatorJSON(filepath.Join(r.spec.Directory, "heartbeat.json"), map[string]any{"at": time.Now().UTC()})
	}
	if err := write(); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		defer os.Remove(filepath.Join(r.spec.Directory, "heartbeat.json"))
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if write() != nil {
					return
				}
			}
		}
	}()
	return nil
}

// StartNormal 仅监听现有恢复账本的文件事件，不消费新的业务队列或轮询任务状态。
func (r *CurrentRuntime) StartNormal(ctx context.Context, stop context.CancelFunc) error {
	r.lifecycle = ctx
	unlock, err := r.Coordinator.Store.Lock()
	if err != nil {
		return err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	sub, err := opswatch.Subscribe(watchCtx, map[string][]string{"jobs": {filepath.Join(r.spec.Directory, "jobs")}}, nil)
	if err != nil {
		cancel()
		unlock()
		return err
	}
	r.close = func() { cancel(); sub.Close(); unlock() }
	if err = r.startHeartbeat(watchCtx); err != nil {
		r.Close()
		return err
	}
	go r.watchNormal(watchCtx, sub, stop)
	return nil
}

func (r *CurrentRuntime) Close() {
	r.closeOnce.Do(func() {
		if r.close != nil {
			r.close()
		}
	})
}
func (r *CurrentRuntime) TransitionRequested() bool { return r.requested.Load() }

func (r *CurrentRuntime) Pending() (bool, error) {
	jobs, err := r.Coordinator.Store.List()
	if err != nil {
		return false, err
	}
	for _, j := range jobs {
		if (j.Kind == "restore" || j.Kind == "resume") && (j.Status == "queued" || j.Status == "running" || j.Status == "attention_required") {
			return true, nil
		}
	}
	return false, nil
}

func (r *CurrentRuntime) watchNormal(ctx context.Context, sub *opswatch.Subscription, stop context.CancelFunc) {
	check := func() bool {
		if r.AwaitingStartup() {
			return false
		}
		pending, err := r.Pending()
		if err != nil || !pending {
			return false
		}
		r.requested.Store(true)
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return true
		case <-timer.C:
			stop()
			return true
		}
	}
	if check() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-sub.Changes:
			if !ok {
				return
			}
			if check() {
				return
			}
		case <-sub.Errors:
			return
		}
	}
}
