package system_recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	media "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 本测试创建自己的 PostgreSQL 集群与 Redis，不连接或重启开发者服务。
func TestRecoveryNativeApplicationFromPortablePackage(t *testing.T) {
	if os.Getenv("POWERX_RECOVERY_NATIVE_TEST") != "1" {
		t.Skip("explicit opt-in full native application recovery")
	}
	require.Equal(t, "darwin", runtime.GOOS)
	workspace, e := filepath.Abs("../../../..")
	require.NoError(t, e)
	binary := filepath.Join(workspace, "test-results/recovery-bin/powerx")
	tool := filepath.Join(workspace, "test-results/recovery-bin/snapshot-tool")
	if file := os.Getenv("POWERX_RECOVERY_CORE_BINARY"); file != "" {
		binary = file
	}
	if file := os.Getenv("POWERX_RECOVERY_TOOL_BINARY"); file != "" {
		tool = file
	}
	binary, e = filepath.EvalSymlinks(binary)
	require.NoError(t, e)
	tool, e = filepath.EvalSymlinks(tool)
	require.NoError(t, e)
	webOutput := os.Getenv("POWERX_RECOVERY_WEB_OUTPUT")
	if webOutput == "" {
		webOutput = filepath.Join(workspace, "web-admin/.output")
	}
	for _, file := range []string{binary, tool, filepath.Join(webOutput, "server/index.mjs")} {
		_, e := os.Stat(file)
		require.NoError(t, e)
	}
	root, e := os.MkdirTemp("/private/tmp", "pxr-")
	require.NoError(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	sourceRoot := filepath.Join(root, "source")
	require.NoError(t, os.MkdirAll(filepath.Join(sourceRoot, "sockets"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceRoot, "tmp"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceRoot, "redis"), 0700))
	profile := Profile{Protocol: 1, InstanceID: uuid.NewString(), Directory: filepath.Join(root, "ops"), SnapshotDirectory: filepath.Join(root, "snapshots"), CandidateDirectory: filepath.Join(root, "candidates"), Targets: []snapshots.RecoveryTarget{{ID: "local", Label: "Local isolated", Mode: "isolated", Environment: "dev"}}, Tools: Tools{PostgresBin: "/opt/homebrew/bin", RedisServer: "/opt/homebrew/bin/redis-server", Node: "/opt/homebrew/bin/node", Coordinator: tool, Sandbox: "/usr/bin/sandbox-exec"}}
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	require.NoError(t, e)
	source := Deployment{ID: "source", Root: sourceRoot, SocketDirectory: filepath.Join(sourceRoot, "sockets"), PostgresData: filepath.Join(sourceRoot, "postgres")}
	source.PostgresPort, e = freePort()
	require.NoError(t, e)
	profile.Initial = source
	source.RedisPort, e = freePort()
	require.NoError(t, e)
	native := &Native{Profile: profile, Store: store}
	candidateID := uuid.NewString()
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		processes := []processRecord{}
		_ = filepath.WalkDir(root, func(file string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() || filepath.Base(filepath.Dir(file)) != "processes" || filepath.Ext(file) != ".json" {
				return nil
			}
			var record processRecord
			if snapshots.ReadCoordinatorJSON(file, &record) == nil {
				processes = append(processes, record)
			}
			return nil
		})
		assert.NoError(t, StopCandidateProcesses(cleanup, filepath.Join(profile.CandidateDirectory, candidateID)))
		assert.NoError(t, stopOwned(cleanup, filepath.Join(sourceRoot, "processes/redis.json")))
		assert.NoError(t, StopCandidateProcesses(cleanup, sourceRoot))
		released := true
		for _, record := range processes {
			if identity, e := processIdentity(record.PID); e == nil && identity == record.Started {
				released = false
			}
		}
		report := map[string]any{"platform": runtime.GOOS + "/" + runtime.GOARCH, "passed": !t.Failed(), "actual_core_web_auth_context_media": !t.Failed(), "native_switch_and_compensation": !t.Failed() && os.Getenv("POWERX_RECOVERY_LOCAL_ONLY") != "1" && os.Getenv("POWERX_RECOVERY_INSTANCE_ONLY") != "1", "instance_restore_restart_protection_and_clean_target": !t.Failed() && os.Getenv("POWERX_RECOVERY_INSTANCE_ONLY") == "1", "instance_sigkill_compensation": !t.Failed() && os.Getenv("POWERX_RECOVERY_INSTANCE_CRASH") == "1", "source_B_retained": !t.Failed(), "plugin_binding_role_mapping": !t.Failed(), "password_login_verified": false, "active_production_plugins_tested": false, "fixture_foreign_key_auto_migration_disabled": true, "linux_namespace_executed": false, "production_deployed": false, "owned_processes": processes, "owned_processes_released": released, "finished_at": time.Now().UTC()}
		report["offline_recovery_portal"] = !t.Failed() && os.Getenv("POWERX_RECOVERY_PORTAL_TEST") == "1"
		if os.Getenv("POWERX_CURRENT_RECOVERY_NATIVE_TEST") == "1" {
			report["native_switch_and_compensation"] = false
			report["source_B_retained"] = false
			report["current_instance_same_pid"] = !t.Failed()
		}
		assert.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(workspace, func() string {
			if os.Getenv("POWERX_CURRENT_RECOVERY_NATIVE_TEST") == "1" {
				return "test-results/current-recovery-fixture-evidence.json"
			}
			if os.Getenv("POWERX_RECOVERY_INSTANCE_ONLY") == "1" {
				return "test-results/recovery-instance-evidence.json"
			}
			if os.Getenv("POWERX_RECOVERY_LOCAL_ONLY") == "1" {
				return "test-results/recovery-local-entry-evidence.json"
			}
			return "test-results/recovery-native-evidence.json"
		}()), report))
		assert.True(t, released, "owned fixture processes must be released")
		if !t.Failed() {
			require.NoError(t, os.RemoveAll(root))
		} else {
			t.Logf("owned fixture stopped; failure evidence preserved at %s", root)
		}
	})
	require.NoError(t, native.initializeDatabase(ctx, source, captureContract{PostgresMajor: 14, Extensions: map[string]string{}}))
	dsn := fmt.Sprintf("host=%s port=%d user=powerx_recovery dbname=powerx sslmode=disable", source.SocketDirectory, source.PostgresPort)
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, e)
	sqlDB, e := db.DB()
	require.NoError(t, e)
	defer sqlDB.Close()
	require.NoError(t, database.MigrateCoreModels(db))
	require.NoError(t, persistence.MigrateAgentModels(db))
	ten := tenant.Tenant{Key: "system", Name: "System", Status: 1, Type: "system", Plan: "free"}
	require.NoError(t, db.Create(&ten).Error)
	user := iam.User{DisplayName: "Recovery fixture", Status: 1, IsRoot: true}
	require.NoError(t, db.Create(&user).Error)
	member := iam.Member{TenantUUID: ten.UUID.String(), UserID: user.ID, UserUUID: user.UUID.String(), Username: "recovery-fixture", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Exec("CREATE TABLE public.recovery_fixture_orders(id bigserial PRIMARY KEY,label text NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO public.recovery_fixture_orders(label) VALUES ('A')").Error)
	cfg := config.GetDefaults()
	cfg.Agent.FlowSpec.BaseDir = filepath.Join(workspace, "backend/pkg/corex/flow/blueprints")
	cfg.Agent.FlowSpec.BusinessDir = filepath.Join(workspace, "backend/internal/server/agent/blueprints")
	cfg.Deployment.Env = "test"
	cfg.Version = "recovery-native-test"
	cfg.Database.DSN = dsn
	cfg.Database.Driver = "postgres"
	cfg.Database.Host = source.SocketDirectory
	cfg.Database.Port = source.PostgresPort
	cfg.Database.UserName = "powerx_recovery"
	cfg.Database.Database = "powerx"
	cfg.Database.Password = ""
	cfg.Database.SSLMode = "disable"
	cfg.Server.SecretKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{17}, 32))
	cfg.Auth.JWTSecret = token()
	cfg.Auth.Issuer = "recovery.fixture"
	cfg.Auth.AudienceUser = "user"
	cfg.Plugin.Enabled = false
	cfg.Plugin.InstalledDir = filepath.Join(sourceRoot, "plugins")
	cfg.Plugin.RegistryFile = filepath.Join(sourceRoot, "registry.json")
	cfg.FeatureGate.EnableKnowledgeSpace = false
	cfg.AI.Catalog.FailIfEmpty = false
	cfg.AI.Catalog.Dirs = []string{filepath.Join(sourceRoot, "catalog")}
	require.NoError(t, os.MkdirAll(cfg.AI.Catalog.Dirs[0], 0700))
	cfg.Cache.Driver = "redis"
	cfg.Cache.Host = "127.0.0.1"
	cfg.Cache.Port = source.RedisPort
	cfg.Cache.Password = ""
	cfg.Cache.DB = 0
	cfg.Queue.Driver = "redis"
	cfg.Event.Bus.Type = "redis"
	cfg.Event.Bus.RedisAddr = fmt.Sprintf("127.0.0.1:%d", source.RedisPort)
	cfg.Queue.Redis.Addr = cfg.Event.Bus.RedisAddr
	cfg.Event.Fabric.RedisAddr = cfg.Event.Bus.RedisAddr
	cfg.Event.Fabric.Authorization.RedisAddr = cfg.Event.Bus.RedisAddr
	cfg.KnowledgeSpace.RedisAddr = cfg.Event.Bus.RedisAddr
	cfg.IntegrationGateway.RedisAddr = cfg.Event.Bus.RedisAddr
	cfg.AgentLifecycle.RedisAddr = cfg.Event.Bus.RedisAddr
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = filepath.Join(sourceRoot, "media")
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.Storage.Local.BasePath, "fixture"), 0700))
	mediaBytes := []byte("media-at-A")
	sum := sha256.Sum256(mediaBytes)
	file := filepath.Join(cfg.Storage.Local.BasePath, "fixture", "sample.txt")
	require.NoError(t, os.WriteFile(file, mediaBytes, 0600))
	asset := media.MediaAsset{TenantUUID: ten.UUID.String(), Name: "Recovery A", Driver: "local", Bucket: "fixture", StorageKey: "sample.txt", SizeBytes: int64(len(mediaBytes)), ExpectedChecksum: hex.EncodeToString(sum[:]), UploadState: "ready", MimeType: "text/plain"}
	require.NoError(t, db.Create(&asset).Error)
	_, e = startOwned(ctx, filepath.Join(sourceRoot, "processes/redis.json"), profile.Tools.RedisServer, sourceRoot, []string{"--bind", "127.0.0.1", "--port", fmt.Sprint(source.RedisPort), "--dir", filepath.Join(sourceRoot, "redis"), "--save", "", "--appendonly", "no"}, cleanEnvironment(sourceRoot), []int{source.RedisPort})
	require.NoError(t, e)
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.Event.Bus.RedisAddr})
	defer redisClient.Close()
	require.Eventually(t, func() bool { return redisClient.Ping(ctx).Err() == nil }, 5*time.Second, 50*time.Millisecond)
	require.NoError(t, redisClient.Set(ctx, "fixture:queued", "A", 0).Err())
	require.NoError(t, redisClient.Save(ctx).Err())
	binding := seedNativeBinding(t, ctx, cfg)
	raw, e := yaml.Marshal(cfg)
	require.NoError(t, e)
	snapshotConfig := snapshots.Config{Directory: profile.SnapshotDirectory, Instance: "test", Version: cfg.Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, DatabaseDSN: dsn, MediaRoot: cfg.Storage.Local.BasePath, EffectiveConfig: raw, ConfigFiles: map[string]string{}, Artifacts: map[string]string{"core/powerx": binary, "core/web-admin": webOutput, "core/platform_capabilities": filepath.Join(workspace, "backend/config/platform_capabilities")}}
	service := snapshots.NewService(ctx, snapshotConfig, snapshots.Options{Coordinator: true, Backend: &snapshots.PostgresBackend{DSN: dsn, MediaRoot: snapshotConfig.MediaRoot}, DispatchCapture: func(context.Context, string) error { return nil }})
	defer service.Close()
	native.SnapshotStore = service
	require.NoError(t, service.SaveCoordinatorSource(snapshotConfig))
	if os.Getenv("POWERX_CURRENT_RECOVERY_NATIVE_TEST") == "1" {
		require.NoError(t, sqlDB.Close())
		require.NoError(t, redisClient.Close())
		exerciseCurrentApplication(t, ctx, root, cfg, snapshotConfig, binary, webOutput, reqctx.CoreXClaims{TenantUUID: ten.UUID.String(), TenantID: ten.ID, UserUUID: user.UUID.String(), UserID: user.ID, MemberUUID: member.UUID.String(), MemberID: member.ID, IsRoot: true, Platforms: []string{"admin"}})
		return
	}
	snapshot, e := service.Start(privileged(ctx), uuid.NewString())
	require.NoError(t, e)
	captureJob := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "capture", SnapshotID: snapshot.ID}
	require.NoError(t, native.Capture(ctx, &captureJob, source))
	exported, cleanup, e := service.ExportFile(privileged(ctx), snapshot.ID, "native-portable-recovery-passphrase")
	require.NoError(t, e)
	defer cleanup()
	packageBytes, e := os.ReadFile(exported)
	require.NoError(t, e)
	require.NoError(t, db.Exec("INSERT INTO public.recovery_fixture_orders(label) VALUES ('B')").Error)
	require.NoError(t, os.WriteFile(file, []byte("media-at-B"), 0600))
	sumB := sha256.Sum256([]byte("media-at-B"))
	require.NoError(t, db.Model(&asset).Updates(map[string]any{"expected_checksum": hex.EncodeToString(sumB[:]), "size_bytes": len("media-at-B")}).Error)
	require.NoError(t, redisClient.Set(ctx, "fixture:queued", "B", 0).Err())
	targetStore := snapshots.NewService(ctx, snapshots.Config{Directory: filepath.Join(root, "imported")}, snapshots.Options{Coordinator: true})
	defer targetStore.Close()
	imported, e := targetStore.Import(privileged(ctx), bytes.NewReader(packageBytes), "native-portable-recovery-passphrase")
	require.NoError(t, e)
	native.SnapshotStore = targetStore
	// 源媒体和快照密钥离线，目标仅持有完整包、目标工具和独立 PostgreSQL。
	require.NoError(t, os.Rename(cfg.Storage.Local.BasePath, cfg.Storage.Local.BasePath+".offline"))
	require.NoError(t, os.Rename(filepath.Join(profile.SnapshotDirectory, "master.key"), filepath.Join(profile.SnapshotDirectory, "master.key.offline")))
	restoreJob := snapshots.ManagedJob{ID: candidateID, Kind: "prepare", SnapshotID: imported.ID, TargetID: "local"}
	deployment, checks, e := native.Prepare(ctx, &restoreJob)
	require.NoError(t, e, "candidate root: %s", deployment.Root)
	require.NotEmpty(t, checks)
	verifyNativeBinding(t, ctx, dsn, deployment, binding)
	for _, check := range checks {
		require.True(t, check.Passed, check.Name)
	}
	targetDB, e := pgx.Connect(ctx, fmt.Sprintf("host=%s port=%d user=powerx_recovery dbname=powerx sslmode=disable", deployment.SocketDirectory, deployment.PostgresPort))
	require.NoError(t, e)
	defer targetDB.Close(ctx)
	var count int
	require.NoError(t, targetDB.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	require.Equal(t, 1, count)
	restored, e := os.ReadFile(filepath.Join(deployment.Root, "media/fixture/sample.txt"))
	require.NoError(t, e)
	require.Equal(t, "media-at-A", string(restored))
	require.NoError(t, db.Raw("SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count).Error)
	require.Equal(t, 2, count)
	sourceBytes, e := os.ReadFile(file + ".offline")
	_ = sourceBytes
	_ = e // 源目录离线后仍通过其新位置核对 B。
	sourceBytes, e = os.ReadFile(filepath.Join(cfg.Storage.Local.BasePath+".offline", "fixture/sample.txt"))
	require.NoError(t, e)
	require.Equal(t, "media-at-B", string(sourceBytes))
	actual, e := exec.Command(binary, "--recovery-protocol").Output()
	require.NoError(t, e)
	require.True(t, CheckProtocol(actual))
	if os.Getenv("POWERX_RECOVERY_INSTANCE_ONLY") == "1" {
		exerciseNativeInstance(t, ctx, root, profile, deployment, filepath.Join(root, "imported"), imported.ID)
		return
	}
	if os.Getenv("POWERX_RECOVERY_LOCAL_ONLY") == "1" {
		_, e = targetDB.Exec(ctx, "INSERT INTO public.recovery_fixture_orders(label) VALUES ('local-B')")
		require.NoError(t, e)
		effective, e := os.ReadFile(deployment.ConfigFile)
		require.NoError(t, e)
		exerciseLocalEntry(t, ctx, root, profile, snapshots.Config{EffectiveConfig: effective, Artifacts: map[string]string{"core/powerx": deployment.CoreBinary, "core/web-admin": deployment.WebOutput, "core/platform_capabilities": filepath.Join(deployment.Root, "programs/core/platform_capabilities")}})
		return
	}
	exerciseNativeSwitch(t, ctx, root, profile, deployment, filepath.Join(root, "imported"))
	t.Logf("actual Core/Web/API/media restored A; source DB/media retain B; checks=%d snapshot=%s recovery=%s", len(checks), snapshot.ID, restoreJob.ID)
}
