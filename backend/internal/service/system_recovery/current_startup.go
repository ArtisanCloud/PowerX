package system_recovery

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

func openCurrent(directory string) (*CurrentRuntime, error) {
	if err := privateDirectory(directory); err != nil {
		return nil, err
	}
	var spec currentSpec
	if err := snapshots.ReadCoordinatorJSON(filepath.Join(directory, "installation.json"), &spec); err != nil {
		return nil, err
	}
	if spec.Protocol != CurrentRecoveryProtocol || spec.Directory != directory || !filepath.IsAbs(spec.ConfigFile) || !filepath.IsAbs(spec.Executable) || spec.SnapshotDirectory == "" {
		return nil, errors.New("当前恢复启动记录无效")
	}
	service := snapshots.NewService(context.Background(), snapshots.Config{Directory: spec.SnapshotDirectory}, snapshots.Options{Coordinator: true})
	_, portText, err := net.SplitHostPort(spec.Listen)
	port, parseErr := strconv.Atoi(portText)
	if err != nil || parseErr != nil || port < 1 || port > 65535 {
		service.Close()
		return nil, errors.New("维护入口端口记录无效")
	}
	store, err := snapshots.NewCoordinatorStore(directory, []snapshots.RecoveryTarget{{ID: "current", Label: "当前实例", Environment: spec.Environment, Mode: "formal", AllowOverwrite: spec.Environment == "dev", AllowOnlineData: spec.Environment == "dev"}})
	if err != nil {
		service.Close()
		return nil, err
	}
	r := &CurrentRuntime{spec: spec}
	r.Coordinator = &Coordinator{Store: store, SnapshotStore: service, Initial: Deployment{ID: spec.ID, Root: spec.Root, ConfigFile: spec.ConfigFile, CorePort: port, SourceManaged: true, CoreBinary: spec.Executable}}
	return r, nil
}

// CurrentStartup 在数据库及插件初始化之前检查本机恢复账本，断电后仍先进入维护模式。
func CurrentBeforeConfig() (*CurrentRuntime, error) {
	for _, candidate := range config.StartupConfigPaths() {
		file, err := filepath.Abs(candidate)
		if err != nil {
			return nil, err
		}
		var pointer map[string]string
		if info, e := os.Lstat(file + ".recovery.json"); e == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
			return nil, errors.New("恢复启动指针必须是私有普通文件")
		}
		err = snapshots.ReadCoordinatorJSON(file+".recovery.json", &pointer)
		if errors.Is(err, os.ErrNotExist) {
			if _, found := os.Stat(file); found == nil {
				return nil, nil
			}
			continue
		}
		if err != nil || pointer["protocol"] != CurrentRecoveryProtocol || pointer["config_file"] != file {
			return nil, errors.New("当前恢复启动指针无效")
		}
		r, err := openCurrent(pointer["directory"])
		if err != nil {
			return nil, err
		}
		if r.spec.ConfigFile != file {
			r.Coordinator.SnapshotStore.Close()
			return nil, errors.New("恢复指针与当前配置路径不一致")
		}
		pending, err := r.Pending()
		if err != nil {
			r.Coordinator.SnapshotStore.Close()
			return nil, err
		}
		if !pending {
			r.Coordinator.SnapshotStore.Close()
			if _, e := os.Stat(file); e != nil {
				return nil, errors.New("当前安装的正式配置缺失")
			}
			return nil, nil
		}
		return r, nil
	}
	return nil, nil
}

func CurrentStartup(cfg *config.Config, configFile string) (*CurrentRuntime, error) {
	if cfg.Recovery.ProfileFile != "" || cfg.Recovery.ReadOnly() {
		return nil, nil
	}
	directory, _ := snapshots.DefaultStorage()
	if cfg.SystemSnapshots.Directory != "" {
		directory = cfg.SystemSnapshots.Directory
	}
	var dirErr error
	directory, dirErr = filepath.EvalSymlinks(directory)
	if errors.Is(dirErr, os.ErrNotExist) {
		return nil, nil
	}
	if dirErr != nil {
		return nil, dirErr
	}
	file, err := filepath.Abs(configFile)
	if err != nil {
		return nil, err
	}
	path := currentDirectory(directory, file)
	if _, err = os.Lstat(filepath.Join(path, "installation.json")); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	r, err := openCurrent(path)
	if err != nil {
		return nil, err
	}
	pending, err := r.Pending()
	if err != nil {
		r.Coordinator.SnapshotStore.Close()
		return nil, err
	}
	if !pending {
		r.Coordinator.SnapshotStore.Close()
		return nil, nil
	}
	return r, nil
}

// ExecMaintenance 使用原 PID 和原后端端口运行维护模式，现有 Web 代理地址保持不变。
func (r *CurrentRuntime) ExecMaintenance() error {
	r.Close()
	return syscall.Exec(r.spec.Executable, []string{r.spec.Executable, "--current-maintenance", r.spec.Directory}, currentExecEnvironment(r.spec.ConfigFile))
}
func currentExecEnvironment(configFile string) []string {
	env := []string{}
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "POWERX_CONFIG=") || strings.HasPrefix(item, "POWERX_RECOVERY_BOOT_JOB=") {
			continue
		}
		env = append(env, item)
	}
	return append(env, "POWERX_CONFIG="+configFile)
}
func (r *CurrentRuntime) execNormal() error {
	// 缓存可从恢复后的数据库重建；不清空可能被其他应用共用的缓存库。
	if err := r.rotateCurrentCache(); err != nil {
		return err
	}
	args := append([]string{}, r.spec.Args...)
	if len(args) == 0 {
		args = []string{r.spec.Executable}
	}
	args[0] = r.spec.Executable
	if err := os.Chdir(r.spec.Root); err != nil {
		return err
	}
	env := currentExecEnvironment(r.spec.ConfigFile)
	jobs, err := r.Coordinator.Store.List()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Status == "running" && j.Phase == "verifying_startup" {
			env = append(env, "POWERX_RECOVERY_BOOT_JOB="+j.ID)
			break
		}
	}
	return syscall.Exec(r.spec.Executable, args, env)
}

// RunCurrentMaintenance 不装配业务 DB、插件和后台任务。维护执行和页面使用同一 Core 进程。
func RunCurrentMaintenance(ctx context.Context, directory string) error {
	r, err := openCurrent(directory)
	if err != nil {
		return err
	}
	defer r.Coordinator.SnapshotStore.Close()
	unlock, err := r.Coordinator.Store.Lock()
	if err != nil {
		return err
	}
	defer func() { unlock() }()
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	if err = r.startHeartbeat(run); err != nil {
		return err
	}
	sub, err := opswatch.Subscribe(run, map[string][]string{"jobs": {filepath.Join(directory, "jobs")}}, nil)
	if err != nil {
		return err
	}
	defer sub.Close()
	listener, err := net.Listen("tcp", r.spec.Listen)
	if err != nil {
		return err
	}
	handler := r.Handler()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, recoveryEntry) {
			handler.ServeHTTP(w, req)
			return
		}
		if req.Method == "GET" && strings.Contains(req.Header.Get("Accept"), "text/html") {
			r.Coordinator.serveRecoveryPage(w, req)
			return
		}
		w.Header().Set("Retry-After", "3")
		http.Error(w, "系统正在恢复，请在原页面查看进度", http.StatusServiceUnavailable)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	for {
		ready, err := r.executeCurrentPending(run)
		if err != nil {
			r.Coordinator.executorFailure.Store(err.Error())
		}
		if ready {
			// 先结束维护连接，再在同一 PID exec 正常 Core。没有新后台进程。
			_ = server.Close()
			cancel()
			unlock()
			unlock = func() {}
			return r.execNormal()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-sub.Changes:
			if !ok {
				return errors.New("恢复账本通知已关闭")
			}
		case err := <-sub.Errors:
			return err
		}
	}
}
