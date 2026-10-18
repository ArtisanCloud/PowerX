package system_recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	pluginmanager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

type RunSpec struct {
	VerificationServer bool       `json:"verification_server,omitempty"`
	LocalRehearsal     bool       `json:"local_rehearsal,omitempty"`
	StatusURL          string     `json:"status_url,omitempty"`
	Deployment         Deployment `json:"deployment"`
	Tools              Tools      `json:"tools"`
	Hold               bool       `json:"hold"`
	Isolated           bool       `json:"isolated"`
	DenyPaths          []string   `json:"deny_paths,omitempty"`
	Token              string     `json:"token,omitempty"`
}

// PreflightSource 拒绝把不支持维护协议的旧单元误当作已受控的写入边界。
func (n *Native) PreflightSource(ctx context.Context, d Deployment) error {
	if !d.SourceManaged {
		return nil
	}
	if len(n.Profile.WriterUnits) != 1 {
		return errors.New("当前受控部署必须由一个托管实例单元拥有 Core、Web 和插件；其他写入者需先完成适配")
	}
	for _, unit := range n.Profile.WriterUnits {
		raw, e := exec.CommandContext(ctx, "systemctl", "show", "--property=ExecStart", "--value", unit).Output()
		if e != nil {
			return errors.New("无法核验托管实例单元")
		}
		command := string(raw)
		if !strings.Contains(command, n.Profile.Tools.Coordinator) || !strings.Contains(command, "run-active") || !strings.Contains(command, n.Profile.File) {
			return errors.New("写入服务未使用受控 run-active 启动入口，已阻断维护操作")
		}
	}
	return nil
}

func (n *Native) Stop(ctx context.Context, d Deployment) error {
	if d.SourceManaged {
		if len(n.Profile.WriterUnits) == 0 {
			return errors.New("source has no managed writer unit")
		}
		for _, unit := range n.Profile.WriterUnits {
			if e := exec.CommandContext(ctx, "systemctl", "stop", "--", unit).Run(); e != nil {
				return fmt.Errorf("无法停止登记的写入服务 %s", unit)
			}
			if exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", unit).Run() == nil {
				return errors.New("writer unit is still active")
			}
		}
		return nil
	}
	return stopOwned(ctx, filepath.Join(d.Root, "processes/instance.json"))
}
func (n *Native) Start(ctx context.Context, d Deployment, hold bool) error {
	if n.Profile.InPlace && n.Profile.Local && n.Profile.GatewayWebListen != "" {
		d.PublicBaseURL = "http://" + n.Profile.GatewayWebListen
	}
	if n.Profile.InPlace && (d.PostgresData != filepath.Join(d.Root, "postgres") || d.SocketDirectory != filepath.Join(d.Root, "s") || d.ConfigFile != filepath.Join(d.Root, "etc/config.yaml")) {
		return errors.New("固定实例组件路径不属于登记目标")
	}
	if d.SourceManaged {
		if e := snapshots.WriteCoordinatorJSON(filepath.Join(n.Store.Directory, "source-mode.json"), struct {
			Hold bool `json:"hold"`
		}{hold}); e != nil {
			return e
		}
		for _, unit := range n.Profile.WriterUnits {
			if e := exec.CommandContext(ctx, "systemctl", "restart", "--", unit).Run(); e != nil {
				return fmt.Errorf("无法恢复登记的写入服务 %s", unit)
			}
		}
		return nil
	}
	if e := stopOwned(ctx, filepath.Join(d.Root, "processes/instance.json")); e != nil {
		return e
	}
	for _, socket := range []string{"core.sock", "web.sock", "redis.sock"} {
		_ = os.Remove(filepath.Join(d.Root, socket))
	}
	if e := n.startPostgres(ctx, d); e != nil {
		return e
	}
	raw, e := os.ReadFile(d.ConfigFile)
	if e != nil {
		return e
	}
	var cfg config.Config
	if e = yaml.Unmarshal(raw, &cfg); e != nil {
		return e
	}
	if n.Profile.InPlace {
		if err := validateVerificationDatabase(d, cfg.Database.DSN); err != nil {
			return err
		}
		for _, path := range []string{cfg.Storage.Local.BasePath, cfg.Plugin.InstalledDir, cfg.Plugin.RegistryFile} {
			if !filepath.IsAbs(path) || !inside(d.Root, path) {
				return errors.New("固定实例配置仍指向目标以外的媒体或插件")
			}
		}
	}
	var roles []string
	if e = snapshots.ReadCoordinatorJSON(filepath.Join(d.Root, "plugin-roles.json"), &roles); e != nil {
		return e
	}
	if e = setReadOnlyRoles(ctx, cfg.Database.DSN, roles, hold); e != nil {
		return e
	}
	if e = pluginmanager.SetRecoveryRuntimeMode(ctx, &cfg, hold); e != nil {
		return e
	}
	if hold {
		cfg.Recovery.Mode = "hold"
	} else {
		cfg.Recovery.Mode = "active"
		cfg.Recovery.ProfileFile = n.Profile.File
	}
	var active Active
	if snapshots.ReadCoordinatorJSON(filepath.Join(n.Store.Directory, "active.json"), &active) == nil && active.Deployment.ID == d.ID {
		cfg.Server.Port = n.Profile.Initial.CorePort
		cfg.Server.GRPC.Port = n.Profile.Initial.GRPCPort
		cfg.WebAdminPort = n.Profile.Initial.WebPort
		cfg.Storage.Local.PublicBaseURL = d.PublicBaseURL
		if n.Profile.GatewayWebListen != "" {
			cfg.HTTPSecurity.WebAdminOrigins = []string{"http://" + n.Profile.GatewayWebListen}
		}
	}
	cfg.Event.Bus.RedisAddr = fmt.Sprintf("127.0.0.1:%d", d.RedisPort)
	cfg.Event.Bus.RedisPassword = ""
	runtimeFile := filepath.Join(d.Root, "launch.yaml")
	if n.Profile.InPlace && !hold {
		runtimeFile = d.ConfigFile
	}
	raw, e = yaml.Marshal(&cfg)
	if e != nil {
		return e
	}
	if n.Profile.InPlace && !hold {
		e = writeInstallationConfig(runtimeFile, &cfg)
	} else {
		e = os.WriteFile(runtimeFile, raw, 0600)
	}
	if e != nil {
		return e
	}
	launch := d
	launch.ConfigFile = runtimeFile
	launch.CorePort = cfg.Server.Port
	launch.GRPCPort = cfg.Server.GRPC.Port
	launch.WebPort = cfg.WebAdminPort
	var seal config.RecoverySeal
	if e = snapshots.ReadCoordinatorJSON(filepath.Join(d.Root, "recovery-seal.json"), &seal); e != nil {
		return e
	}
	spec := RunSpec{LocalRehearsal: n.Profile.Local, Deployment: launch, Tools: n.Profile.Tools, Hold: hold, Isolated: hold, Token: seal.Token, StatusURL: "http://" + n.Profile.StatusListen, DenyPaths: []string{n.Profile.Initial.Root, n.Profile.Directory, n.Profile.SnapshotDirectory, n.Profile.CandidateDirectory}}
	for _, target := range n.Profile.Targets {
		if n.Profile.InPlace && target.Mode == "isolated" {
			spec.VerificationServer = true
			spec.Isolated = true
		}
	}
	file := filepath.Join(d.Root, "run.json")
	if e = snapshots.WriteCoordinatorJSON(file, spec); e != nil {
		return e
	}
	if n.Profile.InPlace && !hold {
		for _, name := range []string{"launch.yaml", "runtime.yaml"} {
			if err := os.Remove(filepath.Join(d.Root, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	exe, args, e := sandboxCommand(spec, file)
	if e != nil {
		return e
	}
	_, e = startOwned(ctx, filepath.Join(d.Root, "processes/instance.json"), exe, d.Root, args, cleanEnvironment(d.Root), []int{launch.CorePort, launch.GRPCPort, launch.WebPort, launch.RedisPort})
	return e
}

// sandboxCommand 在候选 Core 和插件执行之前应用 OS 隔离；缺工具时明确阻断。
func sandboxCommand(spec RunSpec, file string) (string, []string, error) {
	if !spec.Isolated {
		return spec.Tools.Coordinator, []string{"-action", "run-instance", "-run-file", file}, nil
	}
	root := spec.Deployment.Root
	switch runtime.GOOS {
	case "darwin":
		// 禁止读取源目录和其他用户目录；文件写入仅限候选目录，网络仅开放候选端口与 socket。
		q := strconv.Quote
		rules := `(version 1)(allow default)(deny file-write*)(allow file-write* (subpath ` + q(root) + `) (literal "/dev/null"))(deny network-outbound)(allow network-outbound (remote unix-socket (subpath ` + q(root) + `)))`
		for _, denied := range append([]string{"/Users", "/private/var/www", "/private/tmp", "/private/var/folders", "/home", "/root", "/srv", "/mnt"}, spec.DenyPaths...) {
			if denied == "" {
				continue
			}
			rules += `(deny file-read-data file-read-xattr file-map-executable (require-all (subpath ` + q(denied) + `) (require-not (subpath ` + q(root) + `)) (require-not (literal ` + q(spec.Tools.Coordinator) + `))))`
		}

		for _, port := range []int{spec.Deployment.CorePort, spec.Deployment.GRPCPort, spec.Deployment.WebPort, spec.Deployment.RedisPort} {
			if port <= 0 {
				continue
			}
			rules += `(allow network-outbound (remote tcp "localhost:` + strconv.Itoa(port) + `"))`
		}
		for port := spec.Deployment.PluginPortMin; port >= 1024 && port <= spec.Deployment.PluginPortMax; port++ {
			rules += `(allow network-outbound (remote tcp "localhost:` + strconv.Itoa(port) + `"))`
		}
		sandbox := filepath.Join(root, "isolation.sb")
		if e := os.WriteFile(sandbox, []byte(rules), 0600); e != nil {
			return "", nil, e
		}
		return spec.Tools.Sandbox, []string{"-f", sandbox, spec.Tools.Coordinator, "-action", "run-instance", "-run-file", file}, nil
	case "linux":
		// 独立网络命名空间中仅有候选 Core、Redis 和插件；PostgreSQL 通过候选私有 Unix socket 连接。
		args := []string{"--die-with-parent", "--unshare-pid", "--unshare-net", "--unshare-ipc", "--unshare-uts", "--new-session", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"}
		for _, dir := range []string{"/usr", "/bin", "/lib", "/lib64", "/etc/ssl", "/etc/ld.so.cache"} {
			if _, e := os.Stat(dir); e == nil {
				args = append(args, "--ro-bind", dir, dir)
			}
		}
		args = append(args, "--bind", root, root, "--ro-bind", spec.Tools.Coordinator, spec.Tools.Coordinator, "--ro-bind", spec.Tools.RedisServer, spec.Tools.RedisServer, "--ro-bind", spec.Tools.Node, spec.Tools.Node, "--chdir", root, "--", spec.Tools.Coordinator, "-action", "run-instance", "-run-file", file)
		return spec.Tools.Sandbox, args, nil
	default:
		return "", nil, errors.New("当前平台不支持恢复隔离")
	}
}

// sandboxDatabase 让 PostgreSQL 本身也继承文件与网络边界，SQL 不能借服务器进程绕过隔离。
func sandboxDatabase(spec RunSpec, exe string, args []string) (string, []string, error) {
	command, arguments, e := sandboxCommand(spec, filepath.Join(spec.Deployment.Root, "unused-run.json"))
	if e != nil {
		return "", nil, e
	}
	if runtime.GOOS == "darwin" {
		policy := arguments[1]
		raw, e := os.ReadFile(policy)
		if e != nil {
			return "", nil, e
		}
		databasePolicy := filepath.Join(spec.Deployment.Root, "postgres-isolation.sb")
		if e = os.WriteFile(databasePolicy, raw, 0600); e != nil {
			return "", nil, e
		}
		return command, append([]string{"-f", databasePolicy, exe}, args...), nil
	}
	// 最后五个参数是固定的协调器命令；替换为受信任的 PostgreSQL 命令。
	return command, append(arguments[:len(arguments)-5], append([]string{exe}, args...)...), nil
}

// RunInstance 只能读取私有受控文件，Core 和 Node 子进程退出时按归属回收。
func RunInstance(ctx context.Context, file string) error {
	var spec RunSpec
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("private run spec required")
	}
	if e = snapshots.ReadCoordinatorJSON(file, &spec); e != nil {
		return e
	}
	d := spec.Deployment
	// sandbox 中的 helper 自身已受隔离，所有子进程继承相同 OS 边界。
	env := cleanEnvironment(d.Root, "POWERX_CONFIG="+d.ConfigFile, "POWERX_WEB_OUTPUT="+d.WebOutput, "POWERX_LINKS_ROOT="+filepath.Join(d.Root, "programs/core"))
	if !d.SourceManaged {
		catalog := filepath.Join(d.Root, "programs/core/platform_capabilities")
		info, err := os.Lstat(catalog)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("RECOVERY_PROGRAM_CATALOG_MISSING: 快照缺少 Core 正式能力目录，请重新备份或配置匹配的程序产物")
		}
		env = append(env, "PLATFORM_CAPABILITIES_DIR="+catalog)
	}
	if spec.Hold && spec.Token == "" {
		env = append(env, "POWERX_COORDINATOR_MAINTENANCE=1")
	}
	if spec.Token != "" {
		env = append(env, "POWERX_RECOVERY_TOKEN="+spec.Token)
	}
	if spec.VerificationServer {
		env = append(env, "POWERX_RECOVERY_VERIFY_SERVER=1")
	}
	children := []*exec.Cmd{}
	defer func() {
		for i := len(children) - 1; i >= 0; i-- {
			cmd := children[i]
			if cmd.Process != nil {
				_ = cmd.Process.Signal(os.Interrupt)
			}
		}
		time.Sleep(300 * time.Millisecond)
		for i := len(children) - 1; i >= 0; i-- {
			cmd := children[i]
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}()
	start := func(exe string, args []string, extra ...string) (*exec.Cmd, error) {
		cmd := exec.Command(exe, args...)
		cmd.Dir = d.Root
		cmd.Env = append(append([]string{}, env...), extra...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e := cmd.Start(); e != nil {
			return nil, e
		}
		children = append(children, cmd)
		return cmd, nil
	}
	var runtimeConfig config.Config
	runtimeRaw, e := os.ReadFile(d.ConfigFile)
	if e != nil {
		return e
	}
	if e = yaml.Unmarshal(runtimeRaw, &runtimeConfig); e != nil {
		return e
	}
	if spec.LocalRehearsal && runtimeConfig.Deployment.Env == "dev" {
		env = append(env, "POWERX_LOCAL_REHEARSAL=1")
	}
	var redisProcess *exec.Cmd
	if !d.SourceManaged {
		// 首次恢复必须先加载 RDB。直接以 appendonly=yes 启动且没有 AOF 时，
		// Redis 会创建空 AOF 并忽略已有 dump.rdb，导致队列／持久数据丢失。
		appendOnly := "no"
		for _, name := range []string{"appendonlydir/appendonly.aof.manifest", "appendonly.aof"} {
			if _, err := os.Stat(filepath.Join(d.Root, "redis", name)); err == nil {
				appendOnly = "yes"
			}
		}
		redisProcess, e = start(spec.Tools.RedisServer, []string{"--bind", "127.0.0.1", "--port", strconv.Itoa(d.RedisPort), "--dir", filepath.Join(d.Root, "redis"), "--dbfilename", "dump.rdb", "--appendonly", appendOnly, "--appendfsync", "always", "--save", "60", "1", "--protected-mode", "yes", "--unixsocket", filepath.Join(d.Root, "redis.sock"), "--unixsocketperm", "700"})
		if e != nil {
			return e
		}
		if e = readyRecoveryRedis(ctx, filepath.Join(d.Root, "redis.sock")); e != nil {
			return e
		}
	}
	core, e := start(d.CoreBinary, nil)
	if e != nil {
		return e
	}
	publicCoreBase := runtimeConfig.Storage.Local.PublicBaseURL
	if publicCoreBase == "" {
		publicCoreBase = fmt.Sprintf("http://127.0.0.1:%d", d.CorePort)
	}
	_, e = start(spec.Tools.Node, []string{filepath.Join(d.WebOutput, "server/index.mjs")}, "HOST=127.0.0.1", "PORT="+strconv.Itoa(d.WebPort), "NITRO_UNIX_SOCKET="+func() string {
		if d.SourceManaged || !spec.Isolated {
			return ""
		}
		return filepath.Join(d.Root, "web.sock")
	}(), "POWERX_BACKEND="+fmt.Sprintf("http://127.0.0.1:%d", d.CorePort), "NUXT_UPSTREAM="+fmt.Sprintf("http://127.0.0.1:%d", d.CorePort), "NUXT_PUBLIC_POWERX_CORE_BASE="+publicCoreBase, "NUXT_PUBLIC_API_BASE="+runtimeConfig.Server.APIPrefix, "POWERX_RECOVERY_STATUS_URL="+spec.StatusURL)
	if e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- core.Wait() }()
	select {
	case <-ctx.Done():
		// Core 正常退出会先关闭 supervisor 中的插件，再让 Redis 保存一致状态。
		_ = core.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = core.Process.Kill()
		}
		if redisProcess != nil {
			_ = redisProcess.Process.Signal(os.Interrupt)
			_ = redisProcess.Wait()
		}
		return ctx.Err()
	case e := <-done:
		return fmt.Errorf("core exited: %w", e)
	}
}

// readyRecoveryRedis 在 Core／插件启动前，等待 RDB 加载和初始 AOF 持久化完成。
func readyRecoveryRedis(ctx context.Context, socket string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	defer client.Close()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if client.Ping(ctx).Err() == nil {
			break
		}
		select {
		case <-ctx.Done():
			return errors.New("候选 Redis 未就绪，未启动 Core")
		case <-ticker.C:
		}
	}
	if e := client.ConfigSet(ctx, "appendonly", "yes").Err(); e != nil {
		return errors.New("候选 Redis 无法启用持久化，未启动 Core")
	}
	for {
		info, e := client.Info(ctx, "persistence").Result()
		if e == nil && strings.Contains(info, "aof_enabled:1\r\n") && strings.Contains(info, "aof_rewrite_in_progress:0\r\n") && strings.Contains(info, "aof_rewrite_scheduled:0\r\n") && strings.Contains(info, "aof_last_bgrewrite_status:ok\r\n") {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("候选 Redis 初始持久化未完成，未启动 Core")
		case <-ticker.C:
		}
	}
}

func httpClient(socket string) *http.Client {
	transport := &http.Transport{Proxy: nil}
	if socket != "" {
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

// CheckProtocol 仅用于受隔离进程的启动结果，不能在主机直接执行未经校验的包内程序。
func CheckProtocol(raw []byte) bool {
	var result struct {
		Protocol int    `json:"protocol"`
		Platform string `json:"platform"`
	}
	return json.Unmarshal(raw, &result) == nil && result.Protocol == config.RecoveryProtocol && strings.TrimSpace(result.Platform) != ""
}
