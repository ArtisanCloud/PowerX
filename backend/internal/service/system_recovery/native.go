package system_recovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

type Native struct {
	Profile       Profile
	Store         *snapshots.CoordinatorStore
	SnapshotStore *snapshots.Service
}
type captureContract struct {
	InstanceID    string            `json:"instance_id"`
	Protocol      int               `json:"protocol"`
	Platform      string            `json:"platform"`
	Version       string            `json:"version"`
	Environment   string            `json:"environment"`
	PostgresMajor int               `json:"postgres_major"`
	Extensions    map[string]string `json:"extensions"`
	Rows          map[string]int64  `json:"rows"`
	RedisDatabase int               `json:"redis_database"`
	WritersFrozen bool              `json:"writers_frozen"`
	LocalOnly     bool              `json:"local_only,omitempty"`
}

func privileged(ctx context.Context) context.Context {
	claims := &reqctx.CoreXClaims{IsRoot: true}
	claims.Subject = "operator.system_recovery"
	return reqctx.WithIsRoot(reqctx.WithClaims(ctx, claims), true)
}
func sourceConfig(source snapshots.Config) (*config.Config, error) {
	var cfg config.Config
	e := yaml.Unmarshal(source.EffectiveConfig, &cfg)
	return &cfg, e
}
func token() string { raw := make([]byte, 32); _, _ = rand.Read(raw); return hex.EncodeToString(raw) }
func hashFile(file string) (string, error) {
	f, e := os.Open(file)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}

// CaptureOnline 保存在线数据恢复包。运行中的 Redis 队列没有跨库一致性协议，
// 因此不伪造 writers_frozen 合同，不将该包标成可直接整站切换。
func (n *Native) CaptureOnline(ctx context.Context, j *snapshots.ManagedJob, d Deployment) error {
	source, e := n.SnapshotStore.LoadCoordinatorSource()
	if e != nil {
		return e
	}
	cfg, e := sourceConfig(source)
	if e != nil {
		return e
	}
	if cfg.Storage.DefaultDriver != "local" || cfg.Database.Driver != "postgres" || len(cfg.Database.Replicas) > 0 {
		return errors.New("在线快照当前要求 PostgreSQL 主库和本地媒体；未停止业务服务")
	}
	if e = refreshPluginSources(ctx, &source, cfg); e != nil {
		return e
	}
	if e = collectRuntimeFiles(cfg, &source, d, n.Store.Directory); e != nil {
		return e
	}
	delete(source.ConfigFiles, "recovery/contract.json")
	delete(source.ConfigFiles, "recovery/redis.rdb")
	backend := &snapshots.PostgresBackend{DSN: source.DatabaseDSN, MediaRoot: source.MediaRoot}
	service := snapshots.NewService(ctx, source, snapshots.Options{Backend: backend, Coordinator: true})
	defer service.Close()
	return service.RunQueuedCapture(ctx, j.SnapshotID)
}

func (n *Native) Capture(ctx context.Context, j *snapshots.ManagedJob, d Deployment) error {
	source, e := n.SnapshotStore.LoadCoordinatorSource()
	if e != nil {
		return e
	}
	cfg, e := sourceConfig(source)
	if e != nil {
		return e
	}
	if cfg.Cache.Driver != "redis" || cfg.Storage.DefaultDriver != "local" || cfg.Database.Driver != "postgres" || len(cfg.Database.Replicas) > 0 || cfg.Queue.Driver != "redis" || (cfg.Event.Bus.Type != "redis" && cfg.Event.Bus.Type != "local") {
		return errors.New("不支持的媒体、数据库副本或外部持久队列；完整采集已阻断")
	}
	if e = refreshPluginSources(ctx, &source, cfg); e != nil {
		return e
	}
	if e = validateDependencies(ctx, cfg, source.DatabaseDSN); e != nil {
		return e
	}
	if e = collectRuntimeFiles(cfg, &source, d, n.Store.Directory); e != nil {
		return e
	}
	contract, e := inspectDatabase(ctx, source.DatabaseDSN)
	if e != nil {
		return e
	}
	contract.Protocol = config.RecoveryProtocol
	contract.InstanceID = n.Profile.InstanceID
	contract.Version = source.Version
	contract.Platform = source.Platform
	contract.Environment = cfg.Deployment.Env
	contract.RedisDatabase = cfg.Cache.DB
	contract.WritersFrozen = true
	contract.LocalOnly = n.Profile.Local
	addr := fmt.Sprintf("%s:%d", cfg.Cache.Host, cfg.Cache.Port)
	client := redis.NewClient(&redis.Options{Addr: addr, Password: cfg.Cache.Password, DB: cfg.Cache.DB})
	defer client.Close()
	// 所有声明写入者已经停止；SAVE 完成后取得独立的 Redis 持久数据。
	rdb := n.Profile.SourceRedisRDB
	if !d.SourceManaged {
		rdb = filepath.Join(d.Root, "redis", "dump.rdb")
	}
	if d.SourceManaged {
		if e = client.Save(ctx).Err(); e != nil {
			return errors.New("独立 Redis 持久化失败，未生成完整快照")
		}
	}
	if info, e := os.Stat(rdb); e != nil || !info.Mode().IsRegular() {
		return errors.New("Redis 持久化文件不可读取")
	}
	metadata := filepath.Join(n.Store.Directory, "capture-contract.json")
	if e = snapshots.WriteCoordinatorJSON(metadata, contract); e != nil {
		return e
	}
	source.ConfigFiles["recovery/contract.json"] = metadata
	source.ConfigFiles["recovery/redis.rdb"] = rdb
	native := &snapshots.PostgresBackend{DSN: source.DatabaseDSN, MediaRoot: source.MediaRoot}
	// 即使 Core 已停止，PG 同 MVCC 与媒体引用校验仍保留。
	service := snapshots.NewService(ctx, source, snapshots.Options{Backend: native, Coordinator: true, DispatchCapture: func(context.Context, string) error { return nil }})
	defer service.Close()
	id := j.SnapshotID
	if j.Kind == "cutover" || j.Kind == "overwrite" || j.Kind == "rollback" || j.Kind == "restore" {
		row, e := service.Start(privileged(ctx), uuid.NewString())
		if e != nil {
			return e
		}
		id = row.ID
		j.RollbackSnapshotID = id
		if e = n.Store.Save(*j); e != nil {
			return e
		}
	}
	if e = service.RunQueuedCapture(ctx, id); e != nil {
		return e
	}
	if j.Kind == "cutover" || j.Kind == "overwrite" || j.Kind == "rollback" || j.Kind == "restore" {
		_, e = service.Protect(privileged(ctx), id, true)
	}
	return e
}

func inspectDatabase(ctx context.Context, dsn string) (captureContract, error) {
	c := captureContract{Extensions: map[string]string{}, Rows: map[string]int64{}}
	db, e := pgx.Connect(ctx, dsn)
	if e != nil {
		return c, errors.New("无法连接声明的来源数据库")
	}
	defer db.Close(context.WithoutCancel(ctx))
	var version int
	if e = db.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); e != nil {
		return c, e
	}
	c.PostgresMajor = version / 10000
	rows, e := db.Query(ctx, "SELECT extname,extversion FROM pg_extension ORDER BY extname")
	if e != nil {
		return c, e
	}
	for rows.Next() {
		var name, version string
		if e = rows.Scan(&name, &version); e != nil {
			rows.Close()
			return c, e
		}
		c.Extensions[name] = version
	}
	rows.Close()
	rows, e = db.Query(ctx, "SELECT schemaname,tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema') AND schemaname !~ '^pg_' ORDER BY 1,2")
	if e != nil {
		return c, e
	}
	tables := [][2]string{}
	for rows.Next() {
		var schema, table string
		if e = rows.Scan(&schema, &table); e != nil {
			rows.Close()
			return c, e
		}
		tables = append(tables, [2]string{schema, table})
	}
	rows.Close()
	for _, t := range tables {
		var count int64
		if e = db.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{t[0], t[1]}.Sanitize()).Scan(&count); e != nil {
			return c, e
		}
		c.Rows[t[0]+"."+t[1]] = count
	}
	return c, nil
}

func freePort() (int, error) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
func (n *Native) Prepare(ctx context.Context, j *snapshots.ManagedJob) (Deployment, []snapshots.RecoveryCheck, error) {
	return n.prepare(ctx, j, nil)
}
func (n *Native) prepare(ctx context.Context, j *snapshots.ManagedJob, replacement *Deployment) (Deployment, []snapshots.RecoveryCheck, error) {
	d := Deployment{ID: j.ID, Root: filepath.Join(n.Profile.CandidateDirectory, j.ID)}
	if replacement != nil {
		d = *replacement
	}
	checks := []snapshots.RecoveryCheck{}
	if len(filepath.Join(d.Root, "s", ".s.PGSQL.65535")) > 103 {
		return d, checks, errors.New("候选目录过长，无法创建 PostgreSQL Unix socket；请配置更短的独立候选目录")
	}
	if e := n.progress(j, "materializing"); e != nil {
		return d, checks, e
	}
	manifest, configuration, e := n.SnapshotStore.MaterializeForRecovery(ctx, j.SnapshotID, d.Root)
	if e != nil {
		return d, checks, e
	}
	var contract captureContract
	fullContract := json.Unmarshal(configuration["recovery/contract.json"], &contract) == nil && contract.Protocol == config.RecoveryProtocol && contract.WritersFrozen && contract.Version == manifest.Version && contract.Platform == manifest.Platform
	if fullContract && contract.LocalOnly && !n.Profile.Local {
		return d, checks, errors.New("该恢复包仅限本地演练，不能作为生产完整恢复点")
	}
	if contract.LocalOnly {
		checks = append(checks, snapshots.RecoveryCheck{Name: "local_only_contract", Passed: true, Detail: "该包来自本地开发演练，只验证已纳入的业务数据和本地组件，不作为生产完整恢复点"})
	}
	var cfg config.Config
	if e = yaml.Unmarshal(configuration["effective.yaml"], &cfg); e != nil {
		return d, checks, e
	}
	target := snapshots.RecoveryTarget{}
	for _, v := range n.Profile.Targets {
		if v.ID == j.TargetID {
			target = v
		}
	}
	if target.ID == "" || !n.Profile.InPlace && target.Mode == "formal" && (target.Environment != contract.Environment || contract.InstanceID != n.Profile.InstanceID) {
		if fullContract {
			return d, checks, errors.New("正式候选必须保留来源部署身份")
		}
	}
	if !fullContract {
		if target.ID == "" || !n.Profile.Local || !target.AllowOnlineData || target.Environment != "dev" || cfg.Deployment.Env != "dev" {
			return d, checks, errors.New("在线数据包缺少 Redis 恢复点；仅显式本地开发演练允许使用独立空 Redis，生产切换已阻断")
		}
		contract = captureContract{InstanceID: n.Profile.InstanceID, Protocol: config.RecoveryProtocol, Platform: manifest.Platform, Version: manifest.Version, Environment: "dev", RedisDatabase: cfg.Cache.DB}
		checks = append(checks, snapshots.RecoveryCheck{Name: "online_data_scope", Passed: true, Detail: "仅本地开发演练：数据库与媒体来自快照；Redis 使用独立空实例，不重放历史队列，未将在线包标成生产完整恢复点"})
	}
	for _, p := range []*int{&d.CorePort, &d.GRPCPort, &d.RedisPort, &d.WebPort, &d.PostgresPort} {
		if replacement != nil {
			continue
		}
		*p, e = freePort()
		if e != nil {
			return d, checks, e
		}
	}
	// 插件段固定在候选分配的较大端口块；任何占用由 supervisor 明确失败。
	if replacement == nil {
		d.PluginPortMin = 30000 + int(time.Now().UnixNano()%20000)
		d.PluginPortMax = d.PluginPortMin + 255
	}
	d.CoreBinary = filepath.Join(d.Root, "programs/core/powerx")
	d.WebOutput = filepath.Join(d.Root, "programs/core/web-admin")
	d.ConfigFile = filepath.Join(d.Root, "runtime.yaml")
	if n.Profile.InPlace {
		d.ConfigFile = filepath.Join(d.Root, "etc/config.yaml")
	}
	if replacement != nil {
		d.ConfigFile = replacement.ConfigFile
	}
	if e = os.MkdirAll(filepath.Dir(d.ConfigFile), 0700); e != nil {
		return d, checks, e
	}
	d.PostgresData = filepath.Join(d.Root, "postgres")
	d.SocketDirectory = filepath.Join(d.Root, "s")
	for _, directory := range []string{d.SocketDirectory, filepath.Join(d.Root, "tmp"), filepath.Join(d.Root, "redis"), filepath.Join(d.Root, "plugins")} {
		if e = os.MkdirAll(directory, 0700); e != nil {
			return d, checks, e
		}
	}
	for _, override := range n.Profile.Overrides {
		if override.Version != manifest.Version {
			return d, checks, errors.New("目标平台产物版本未获得匹配批准")
		}
		sha, e := hashFile(override.Source)
		if e != nil || sha != override.SHA256 {
			return d, checks, errors.New("目标平台产物摘要不匹配")
		}
		raw, e := os.ReadFile(override.Source)
		if e != nil {
			return d, checks, e
		}
		file := filepath.Join(d.Root, "programs", override.Path)
		if !inside(filepath.Join(d.Root, "programs"), file) {
			return d, checks, errors.New("override path escaped")
		}
		if e = os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			return d, checks, e
		}
		if e = os.WriteFile(file, raw, 0700); e != nil {
			return d, checks, e
		}
	}
	if manifest.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		found := false
		for _, override := range n.Profile.Overrides {
			if override.Path == "core/powerx" {
				found = true
			}
		}
		if !found {
			return d, checks, errors.New("来源与目标平台不同，请先在私有 profile 中登记匹配版本的本机 Core 和插件产物")
		}
	}
	if !fullContract && len(configuration["recovery/path-map.json"]) == 0 {
		if e = relocateLegacyRuntimeFiles(&cfg, d.Root); e != nil {
			return d, checks, e
		}
		configuration["recovery/path-map.json"] = []byte(`{}`)
		checks = append(checks, snapshots.RecoveryCheck{Name: "legacy_configuration", Passed: true, Detail: "旧在线包未包含辅助配置文件；引用仅重定位到隔离目录，不读取原站文件，应用启动和业务验证仍须通过"})
	}
	if n.Profile.Local && cfg.AI.Runtime.DurableSessions.Enabled {
		cfg.AI.Runtime.DurableSessions.Enabled = false
		cfg.AI.Runtime.DurableSessions.AdminChatEnabled = false
		checks = append(checks, snapshots.RecoveryCheck{Name: "local_external_agent_reports", Passed: true, Detail: "本地业务数据演练未恢复外部 S3 Agent 报告，已停用 Agent 持久任务；不访问原报告桶，不能作为生产整站验收证明"})
	}
	if e = restoreRuntimeFiles(&cfg, configuration, d.Root); e != nil {
		return d, checks, e
	}
	d.PublicBaseURL = cfg.Storage.Local.PublicBaseURL
	cfg.Deployment.Env = target.Environment
	cfg.Database.Driver = "postgres"
	cfg.Database.Host = d.SocketDirectory
	cfg.Database.Port = d.PostgresPort
	cfg.Database.UserName = "powerx_recovery"
	cfg.Database.Password = ""
	cfg.Database.Database = "powerx"
	cfg.Database.SSLMode = "disable"
	cfg.Database.Replicas = nil
	cfg.Database.ClusterMode = "single"
	cfg.Database.DSN = fmt.Sprintf("host=%s port=%d user=powerx_recovery dbname=powerx sslmode=disable", d.SocketDirectory, d.PostgresPort)
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = d.CorePort
	cfg.Server.GRPC.Host = "127.0.0.1"
	cfg.Server.GRPC.Port = d.GRPCPort
	cfg.Server.GRPC.UseTLS = false
	cfg.Server.GRPC.TLS = nil
	cfg.WebAdminPort = d.WebPort
	sourceRedisAddress := net.JoinHostPort(cfg.Cache.Host, strconv.Itoa(cfg.Cache.Port))
	cfg.Cache.Host = "127.0.0.1"
	cfg.Cache.Port = d.RedisPort
	cfg.Cache.Password = ""
	cfg.Cache.DB = contract.RedisDatabase
	cfg.Queue.Redis.Addr = fmt.Sprintf("127.0.0.1:%d", d.RedisPort)
	cfg.Queue.Redis.Password = ""
	cfg.IntegrationGateway.RedisAddr = cfg.Queue.Redis.Addr
	cfg.IntegrationGateway.RedisPassword = ""
	cfg.AgentLifecycle.RedisAddr = cfg.Queue.Redis.Addr
	cfg.AgentLifecycle.RedisPassword = ""
	cfg.Event.Bus.RedisAddr = cfg.Queue.Redis.Addr
	cfg.Event.Bus.RedisPassword = ""
	cfg.Event.Fabric.RedisAddr = cfg.Queue.Redis.Addr
	cfg.Event.Fabric.RedisPassword = ""
	cfg.Event.Fabric.Authorization.RedisAddr = cfg.Queue.Redis.Addr
	cfg.Event.Fabric.Authorization.RedisPassword = ""
	cfg.KnowledgeSpace.RedisAddr = cfg.Queue.Redis.Addr
	cfg.KnowledgeSpace.RedisPassword = ""
	cfg.Storage.Local.BasePath = filepath.Join(d.Root, "media")
	cfg.SystemSnapshots = config.SnapshotConfig{Directory: filepath.Join(d.Root, "system-snapshots")}
	cfg.Storage.Local.PublicBaseURL = fmt.Sprintf("http://127.0.0.1:%d", d.CorePort)
	d.ValidationURL = fmt.Sprintf("http://127.0.0.1:%d", d.WebPort)
	if replacement != nil {
		d.PublicBaseURL = replacement.PublicBaseURL
	}
	cfg.Storage.Local.PublicBaseURL = d.ValidationURL
	cfg.HTTPSecurity.WebAdminOrigins = []string{d.ValidationURL}
	if target.Mode == "isolated" && !n.Profile.InPlace {
		cfg.Auth.JWTSecret = token()
	}
	cfg.LogConfig.File.InfoFilePath = filepath.Join(d.Root, "logs/info.log")
	cfg.LogConfig.File.ErrorFilePath = filepath.Join(d.Root, "logs/error.log")
	cfg.LogConfig.Loki.Enable = false
	cfg.LogConfig.AgentDebug.Dir = filepath.Join(d.Root, "logs/agent")
	cfg.LogConfig.Retention.Enabled = false
	cfg.KnowledgeSpace.VectorStore.PgVector.DSN = cfg.Database.DSN
	cfg.Plugin.InstalledDir = filepath.Join(d.Root, "programs/plugins")
	cfg.Plugin.RegistryFile = filepath.Join(d.Root, "plugins/registry.json")
	cfg.Plugin.MarketCacheDir = filepath.Join(d.Root, "plugins/market-cache")
	cfg.Plugin.DevMode = false
	cfg.Plugin.DevHotload.FeatureFlags.Enabled = false
	cfg.Audit.File.Dir = filepath.Join(d.Root, "audit")
	cfg.Install.Status = "installed"
	cfg.Install.AllowWithoutDB = false
	seal := config.RecoverySeal{Protocol: config.RecoveryProtocol, JobID: j.ID, Root: d.Root, Token: token()}
	sealRaw, _ := json.Marshal(seal)
	if e = os.WriteFile(filepath.Join(d.Root, "recovery-seal.json"), sealRaw, 0600); e != nil {
		return d, checks, e
	}
	sum := sha256.Sum256(sealRaw)
	cfg.Recovery = config.RecoveryConfig{Mode: "verify", HTTPSocket: filepath.Join(d.Root, "core.sock"), Root: d.Root, JobID: j.ID, SealSHA256: hex.EncodeToString(sum[:]), PluginPortMin: d.PluginPortMin, PluginPortMax: d.PluginPortMax}
	// 插件配置只有在新目录内解密；所有旧绝对路径随后由绑定适配器重写。
	for name, raw := range configuration {
		if !strings.HasPrefix(name, "plugins/") {
			continue
		}
		file := filepath.Join(d.Root, "programs", filepath.FromSlash(name))
		if name == "plugins/registry.json" {
			file = cfg.Plugin.RegistryFile
		}
		if !inside(d.Root, file) {
			return d, checks, errors.New("configuration path escaped")
		}
		if e = os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			return d, checks, e
		}
		if e = os.WriteFile(file, raw, 0600); e != nil {
			return d, checks, e
		}
	}
	if len(configuration["plugins/registry.json"]) == 0 {
		if e = os.WriteFile(cfg.Plugin.RegistryFile, []byte(`{"plugins":{}}`), 0600); e != nil {
			return d, checks, e
		}
	}
	if fullContract {
		if e = os.WriteFile(filepath.Join(d.Root, "redis", "dump.rdb"), configuration["recovery/redis.rdb"], 0600); e != nil {
			return d, checks, e
		}
		if e = os.WriteFile(filepath.Join(d.Root, "redis-recovery-point.rdb"), configuration["recovery/redis.rdb"], 0600); e != nil {
			return d, checks, e
		}
	}
	if e = n.progress(j, "restoring_database"); e != nil {
		return d, checks, e
	}
	if e = n.initializeDatabase(ctx, d, contract); e != nil {
		return d, checks, e
	}
	// 候选 PG 是本协调器创建的独立实例，角色修改不会触碰原实例。
	if e = restoreDatabase(ctx, n.Profile.Tools.PostgresBin, cfg.Database.DSN, filepath.Join(d.Root, "database/database.dump")); e != nil {
		return d, checks, e
	}
	if e = validateDependencies(ctx, &cfg, cfg.Database.DSN); e != nil {
		return d, checks, e
	}
	if !fullContract {
		contract, e = onlineRestoredContract(ctx, cfg.Database.DSN, contract)
		if e != nil {
			return d, checks, e
		}
	}
	if e = verifyDatabase(ctx, cfg.Database.DSN, contract); e != nil {
		return d, checks, e
	}
	detail := "全部来源用户表行数、扩展版本与冻结清单一致"
	if !fullContract {
		detail = "在线 dump 已完整恢复，用户表与扩展已检查；不声称具有跨组件冻结一致性"
	}
	checks = append(checks, snapshots.RecoveryCheck{Name: "database_rows", Passed: true, Detail: detail})
	if e = n.progress(j, "mapping_configuration"); e != nil {
		return d, checks, e
	}
	roles, e := pluginmanager.RemapRecoveryBindings(ctx, &cfg, sourceRedisAddress)
	if e != nil {
		return d, checks, e
	}
	if e = snapshots.WriteCoordinatorJSON(filepath.Join(d.Root, "plugin-roles.json"), roles); e != nil {
		return d, checks, e
	}
	if e = setReadOnlyRoles(ctx, cfg.Database.DSN, roles, true); e != nil {
		return d, checks, e
	}
	if e = snapshots.WriteCoordinatorJSON(filepath.Join(d.Root, "capture-contract.json"), contract); e != nil {
		return d, checks, e
	}
	raw, e := yaml.Marshal(&cfg)
	if e != nil {
		return d, checks, e
	}
	if n.Profile.InPlace {
		e = writeInstallationConfig(d.ConfigFile, &cfg)
	} else {
		e = os.WriteFile(d.ConfigFile, raw, 0600)
	}
	if e != nil {
		return d, checks, e
	}
	checks = append(checks, snapshots.RecoveryCheck{Name: "target_mapping", Passed: true, Detail: "独立 PostgreSQL、Redis、媒体、端口和 deployment.env；插件角色及绑定已重新生成"})
	if e = n.progress(j, "verifying_application"); e != nil {
		return d, checks, e
	}
	if e = n.Start(ctx, d, true); e != nil {
		return d, checks, e
	}
	cleanupOnFailure := true
	defer func() {
		if !cleanupOnFailure {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = n.Stop(cleanup, d)
	}()
	verified, e := n.Verify(ctx, d)
	checks = append(checks, verified...)
	if e == nil {
		if e = n.Stop(ctx, d); e == nil {
			if fullContract {
				e = n.ResetCandidateVerification(ctx, d)
			} else {
				e = n.saveRedisRecoveryPoint(ctx, d)
			}
		}
		if e == nil {
			e = n.Start(ctx, d, true)
		}
		if e == nil {
			_, e = n.Verify(ctx, d)
		}
	}
	cleanupOnFailure = e != nil
	return d, checks, e
}

// ResetCandidateVerification 只在首次开放候选写入之前调用，保留快照中的队列数据。
// 验证产生的会话、缓存或旧插件临时消费不会混入正式恢复点；继续现场时绝不调用。
func (n *Native) ResetCandidateVerification(ctx context.Context, d Deployment) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	raw, e := os.ReadFile(filepath.Join(d.Root, "redis-recovery-point.rdb"))
	if e != nil {
		return e
	}
	if e = os.RemoveAll(filepath.Join(d.Root, "redis", "appendonlydir")); e != nil {
		return e
	}
	if e = os.Remove(filepath.Join(d.Root, "redis", "appendonly.aof")); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return os.WriteFile(filepath.Join(d.Root, "redis", "dump.rdb"), raw, 0600)
}
func restoreDatabase(ctx context.Context, bin, dsn, file string) error {
	cmd := exec.CommandContext(ctx, filepath.Join(bin, "pg_restore"), "--exit-on-error", "--no-owner", "--no-privileges", "--dbname", dsn, file)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	if out, e := cmd.CombinedOutput(); e != nil {
		_ = out
		return errors.New("pg_restore 失败；候选已保留，源数据库未修改")
	}
	return nil
}
func setReadOnlyRoles(ctx context.Context, dsn string, roles []string, enabled bool) error {
	db, e := pgx.Connect(ctx, dsn)
	if e != nil {
		return e
	}
	defer db.Close(context.WithoutCancel(ctx))
	value := "off"
	if enabled {
		value = "on"
	}
	for _, role := range roles {
		if _, e = db.Exec(ctx, "ALTER ROLE "+pgx.Identifier{role}.Sanitize()+" SET default_transaction_read_only = "+value); e != nil {
			return e
		}
	}
	return nil
}
func (n *Native) initializeDatabase(ctx context.Context, d Deployment, c captureContract) error {
	cmd := exec.CommandContext(ctx, filepath.Join(n.Profile.Tools.PostgresBin, "initdb"), "-D", d.PostgresData, "-U", "powerx_recovery", "--auth-local=trust", "--auth-host=reject", "--encoding=UTF8", "--no-locale")
	cmd.Env = cleanEnvironment(d.Root)
	if _, e := cmd.CombinedOutput(); e != nil {
		return errors.New("initdb 失败；需使用非 root 运维用户并安装受支持的 PostgreSQL")
	}
	if e := n.startPostgres(ctx, d); e != nil {
		return e
	}
	dsn := fmt.Sprintf("host=%s port=%d user=powerx_recovery dbname=postgres sslmode=disable", d.SocketDirectory, d.PostgresPort)
	db, e := pgx.Connect(ctx, dsn)
	if e != nil {
		return e
	}
	defer db.Close(context.WithoutCancel(ctx))
	var version int
	if e = db.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); e != nil {
		return e
	}
	if c.PostgresMajor != 0 && version/10000 != c.PostgresMajor {
		return errors.New("目标 PostgreSQL 主版本与来源不一致")
	}
	for extension, version := range c.Extensions {
		var available bool
		if e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_available_extension_versions WHERE name=$1 AND version=$2)", extension, version).Scan(&available); e != nil || !available {
			return fmt.Errorf("目标缺少兼容的 PostgreSQL 扩展 %s", extension)
		}
	}
	_, e = db.Exec(ctx, "CREATE DATABASE powerx")
	return e
}
func (n *Native) startPostgres(ctx context.Context, d Deployment) error {
	file := filepath.Join(d.Root, "processes/postgres.json")
	var record processRecord
	if snapshots.ReadCoordinatorJSON(file, &record) == nil {
		if identity, e := processIdentity(record.PID); e == nil && identity == record.Started {
			return nil
		}
		_ = os.Remove(file)
	}
	postgresArgs := []string{"-D", d.PostgresData, "-k", d.SocketDirectory, "-p", strconv.Itoa(d.PostgresPort), "-h", ""}
	exe, args, e := sandboxDatabase(RunSpec{Isolated: true, Deployment: d, Tools: n.Profile.Tools, DenyPaths: []string{n.Profile.Initial.Root, n.Profile.Directory, n.Profile.SnapshotDirectory, n.Profile.CandidateDirectory}}, filepath.Join(n.Profile.Tools.PostgresBin, "postgres"), postgresArgs)
	if e != nil {
		return e
	}
	_, e = startOwned(ctx, file, exe, d.Root, args, cleanEnvironment(d.Root), nil)
	if e != nil {
		return e
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		db, e := pgx.Connect(ctx, fmt.Sprintf("host=%s port=%d user=powerx_recovery dbname=postgres sslmode=disable", d.SocketDirectory, d.PostgresPort))
		if e == nil {
			db.Close(ctx)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errors.New("候选 PostgreSQL 未就绪")
}

func (n *Native) progress(j *snapshots.ManagedJob, phase string) error {
	if j.Status != "running" {
		return nil
	}
	j.Phase = phase
	return n.Store.Save(*j)
}
