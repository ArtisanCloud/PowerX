package system_recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// 在刚恢复的自建完整实例内再执行 A→B、真实切换失败回退和成功切换。
// 所有服务、数据目录、账号和监听端口均由本测试创建。
func exerciseNativeSwitch(t *testing.T, ctx context.Context, root string, profile Profile, source Deployment, importedDirectory string) {
	t.Helper()
	profile.CandidateDirectory = filepath.Join(root, "fc")
	profile.SnapshotDirectory = importedDirectory
	profile.Initial = source
	profile.Local = true
	profile.ExclusiveWriters = true
	profile.DedicatedRedis = true
	profile.SourceRedisRDB = filepath.Join(source.Root, "redis/dump.rdb")
	statusPort, e := freePort()
	require.NoError(t, e)
	profile.StatusListen = fmt.Sprintf("127.0.0.1:%d", statusPort)
	profile.Targets = []snapshots.RecoveryTarget{{ID: "formal", Label: "Owned formal candidate", Mode: "formal", Environment: "dev", AllowOverwrite: true, AllowOnlineData: true}}
	profile.File = filepath.Join(root, "managed-profile.json")
	require.NoError(t, snapshots.WriteCoordinatorJSON(profile.File, profile))
	_, e = ReadProfile(profile.File)
	require.NoError(t, e)
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	require.NoError(t, e)
	offline := snapshots.NewService(ctx, snapshots.Config{Directory: importedDirectory}, snapshots.Options{Coordinator: true})
	defer offline.Close()
	native := &Native{Profile: profile, Store: store, SnapshotStore: offline}
	raw, e := os.ReadFile(source.ConfigFile)
	require.NoError(t, e)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	cfg.Recovery.ProfileFile = profile.File
	raw, e = yaml.Marshal(&cfg)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(source.ConfigFile, raw, 0600))
	coordinator := &Coordinator{Store: store, Driver: native, Initial: source}
	_, e = coordinator.Active()
	require.NoError(t, e)
	owned := []Deployment{source}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		for _, deployment := range owned {
			assert.NoError(t, StopCandidateProcesses(cleanup, deployment.Root))
		}
	})
	require.NoError(t, native.Start(ctx, source, false))
	_, e = native.Verify(ctx, source)
	require.NoError(t, e)
	// 等实际 Core 将当前生效配置发布到独立运维目录，而非手工拼接 Source。
	var sourceConfig snapshots.Config
	require.Eventually(t, func() bool {
		var err error
		sourceConfig, err = offline.LoadCoordinatorSource()
		return err == nil && sourceConfig.DatabaseDSN != ""
	}, 10*time.Second, 100*time.Millisecond)
	require.NoError(t, native.Stop(ctx, source))
	sourceService := snapshots.NewService(ctx, sourceConfig, snapshots.Options{Coordinator: true, Backend: &snapshots.PostgresBackend{DSN: sourceConfig.DatabaseDSN, MediaRoot: sourceConfig.MediaRoot}, DispatchCapture: func(context.Context, string) error { return nil }})
	defer sourceService.Close()
	native.SnapshotStore = sourceService
	stateA, e := sourceService.Start(privileged(ctx), uuid.NewString())
	require.NoError(t, e)
	capture := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "capture", SnapshotID: stateA.ID}
	require.NoError(t, native.Capture(ctx, &capture, source))
	db, e := pgx.Connect(ctx, cfg.Database.DSN)
	require.NoError(t, e)
	defer db.Close(ctx)
	_, e = db.Exec(ctx, "INSERT INTO public.recovery_fixture_orders(label) VALUES ('B-live')")
	require.NoError(t, e)
	mediaB := []byte("live-state-B")
	sum := sha256.Sum256(mediaB)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Storage.Local.BasePath, "fixture/sample.txt"), mediaB, 0600))
	_, e = db.Exec(ctx, "UPDATE public.media_assets SET expected_checksum=$1,size_bytes=$2", hex.EncodeToString(sum[:]), len(mediaB))
	require.NoError(t, e)
	require.NoError(t, native.Start(ctx, source, false))
	_, e = native.Verify(ctx, source)
	require.NoError(t, e)
	cache := redis.NewClient(&redis.Options{Network: "unix", Addr: filepath.Join(source.Root, "redis.sock")})
	require.NoError(t, cache.Set(ctx, "fixture:queued", "B-live", 0).Err())
	cache.Close()
	prepared, e := store.EnqueueRestore(privileged(ctx), stateA.ID, "formal", uuid.NewString())
	require.NoError(t, e)
	owned = append(owned, Deployment{Root: filepath.Join(profile.CandidateDirectory, prepared.ID)})
	require.NoError(t, coordinator.RunOnce(ctx))
	prepared, e = store.Get(prepared.ID)
	require.NoError(t, e)
	require.Equal(t, "verified", prepared.Status, prepared.Error)
	require.True(t, prepared.BusinessVerified)
	gatewayCtx, stopGateway := context.WithCancel(ctx)
	defer stopGateway()
	closeGateway, e := coordinator.startGateways(gatewayCtx)
	require.NoError(t, e)
	defer closeGateway()
	response, e := http.Get(prepared.ValidationURL)
	require.NoError(t, e)
	require.Equal(t, 200, response.StatusCode)
	body, e := io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, e)
	require.Contains(t, string(body), "<html")
	request, e := http.NewRequest("POST", prepared.ValidationURL+"/api/v1/admin/customers", nil)
	require.NoError(t, e)
	response, e = http.DefaultClient.Do(request)
	require.NoError(t, e)
	response.Body.Close()
	require.Equal(t, 503, response.StatusCode)

	// 此时正式站 B 一直正常，候选 A 已完整启动和验证。
	checks, e := native.Verify(ctx, source)
	require.NoError(t, e)
	require.NotEmpty(t, checks)
	failed, e := store.EnqueueCutover(privileged(ctx), prepared.ID, uuid.NewString(), 1)
	require.NoError(t, e)
	crashOwnedCoordinator(t, ctx, root, profile)
	require.NoError(t, coordinator.Recover(ctx))
	failed, e = store.Get(failed.ID)
	require.NoError(t, e)
	require.Equal(t, "rolled_back", failed.Status, failed.Error)
	require.NotEmpty(t, failed.RollbackSnapshotID)
	active, e := coordinator.Active()
	require.NoError(t, e)
	require.Equal(t, source.ID, active.Deployment.ID)
	require.False(t, active.Maintenance)
	var count int
	require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	require.Equal(t, 2, count)
	bytes, e := os.ReadFile(filepath.Join(cfg.Storage.Local.BasePath, "fixture/sample.txt"))
	require.NoError(t, e)
	require.Equal(t, "live-state-B", string(bytes))
	// 切换前回退点必须真的是当前 B，不是候选准备前的 A。
	rollbackRoot := filepath.Join(root, "rollback-package-inspection")
	_, configuration, e := sourceService.MaterializeForRecovery(ctx, failed.RollbackSnapshotID, rollbackRoot)
	require.NoError(t, e)
	require.NotEmpty(t, configuration["recovery/redis.rdb"])
	rollbackBytes, e := os.ReadFile(filepath.Join(rollbackRoot, "media/fixture/sample.txt"))
	require.NoError(t, e)
	require.Equal(t, "live-state-B", string(rollbackBytes))
	coordinator.Checkpoint = nil
	switched, e := store.EnqueueCutover(privileged(ctx), prepared.ID, uuid.NewString(), 1)
	require.NoError(t, e)
	require.NoError(t, coordinator.RunOnce(ctx))
	switched, e = store.Get(switched.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", switched.Status, switched.Error)
	require.True(t, switched.BusinessVerified)
	active, e = coordinator.Active()
	require.NoError(t, e)
	require.Equal(t, prepared.ID, active.Deployment.ID)
	require.EqualValues(t, 2, active.Generation)
	require.False(t, active.Maintenance)
	var actual Deployment
	require.NoError(t, snapshots.ReadCoordinatorJSON(coordinator.candidateFile(prepared.ID), &actual))
	actualRaw, e := os.ReadFile(filepath.Join(actual.Root, "launch.yaml"))
	require.NoError(t, e)
	var current config.Config
	require.NoError(t, yaml.Unmarshal(actualRaw, &current))
	require.Equal(t, source.CorePort, current.Server.Port)
	require.Equal(t, source.WebPort, current.WebAdminPort)
	target, e := pgx.Connect(ctx, current.Database.DSN)
	require.NoError(t, e)
	defer target.Close(ctx)
	require.NoError(t, target.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	require.Equal(t, 1, count)
	actualMedia, e := os.ReadFile(filepath.Join(current.Storage.Local.BasePath, "fixture/sample.txt"))
	require.NoError(t, e)
	require.Equal(t, "media-at-A", string(actualMedia))
	activeCache := redis.NewClient(&redis.Options{Network: "unix", Addr: filepath.Join(actual.Root, "redis.sock")})
	defer activeCache.Close()
	queued, e := activeCache.Get(ctx, "fixture:queued").Result()
	require.NoError(t, e)
	require.Equal(t, "A", queued, "正式切换应保留恢复点 A 的 Redis 队列")
	require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, target.Close(ctx))
	back, e := store.EnqueueRollback(privileged(ctx), switched.ID, uuid.NewString(), active.Generation)
	require.NoError(t, e)
	require.NoError(t, coordinator.RunOnce(ctx))
	back, e = store.Get(back.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", back.Status, back.Error)
	require.NotEmpty(t, back.RollbackSnapshotID)
	active, e = coordinator.Active()
	require.NoError(t, e)
	require.Equal(t, source.ID, active.Deployment.ID)
	require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	require.Equal(t, 2, count)
	cleanup, e := store.EnqueueCleanup(privileged(ctx), prepared.ID, uuid.NewString(), active.Generation)
	require.NoError(t, e)
	require.NoError(t, coordinator.RunOnce(ctx))
	cleanup, e = store.Get(cleanup.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", cleanup.Status, cleanup.Error)
	_, e = os.Stat(actual.Root)
	require.True(t, os.IsNotExist(e))
	// Start a fresh A candidate, then exercise local overwrite on the owned active
	// environment. The source cluster remains outside the overwrite boundary.
	second, e := store.EnqueueRestore(privileged(ctx), stateA.ID, "formal", uuid.NewString())
	require.NoError(t, e)
	owned = append(owned, Deployment{Root: filepath.Join(profile.CandidateDirectory, second.ID)})
	require.NoError(t, coordinator.RunOnce(ctx))
	second, e = store.Get(second.ID)
	require.NoError(t, e)
	require.Equal(t, "verified", second.Status, second.Error)
	secondSwitch, e := store.EnqueueCutover(privileged(ctx), second.ID, uuid.NewString(), active.Generation)
	require.NoError(t, e)
	require.NoError(t, coordinator.RunOnce(ctx))
	secondSwitch, e = store.Get(secondSwitch.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", secondSwitch.Status, secondSwitch.Error)
	active, e = coordinator.Active()
	require.NoError(t, e)
	var secondEnv Deployment
	require.NoError(t, snapshots.ReadCoordinatorJSON(coordinator.candidateFile(second.ID), &secondEnv))
	var secondCfg config.Config
	raw, e = os.ReadFile(secondEnv.ConfigFile)
	require.NoError(t, e)
	require.NoError(t, yaml.Unmarshal(raw, &secondCfg))
	secondDB, e := pgx.Connect(ctx, secondCfg.Database.DSN)
	require.NoError(t, e)
	_, e = secondDB.Exec(ctx, "INSERT INTO public.recovery_fixture_orders(label) VALUES ('C-after-switch')")
	require.NoError(t, e)
	secondDB.Close(ctx)
	replacement, e := store.EnqueueRestoreMode(privileged(ctx), stateA.ID, "formal", uuid.NewString(), "overwrite_current")
	require.NoError(t, e)
	owned = append(owned, Deployment{Root: filepath.Join(profile.CandidateDirectory, replacement.ID)})
	require.NoError(t, coordinator.RunOnce(ctx))
	replacement, e = store.Get(replacement.ID)
	require.NoError(t, e)
	require.Equal(t, "verified", replacement.Status, replacement.Error)
	overwrite, e := store.EnqueueCutover(privileged(ctx), replacement.ID, uuid.NewString(), active.Generation)
	require.NoError(t, e)
	require.Equal(t, "overwrite", overwrite.Kind)
	require.NoError(t, coordinator.RunOnce(ctx))
	overwrite, e = store.Get(overwrite.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", overwrite.Status, overwrite.Error)
	require.NotEmpty(t, overwrite.RollbackSnapshotID)
	secondDB, e = pgx.Connect(ctx, secondCfg.Database.DSN)
	require.NoError(t, e)
	require.NoError(t, secondDB.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	require.Equal(t, 1, count)
	secondDB.Close(ctx)
	active, e = coordinator.Active()
	require.NoError(t, e)
	backOverwrite, e := store.EnqueueRollback(privileged(ctx), overwrite.ID, uuid.NewString(), active.Generation)
	require.NoError(t, e)
	require.NoError(t, coordinator.RunOnce(ctx))
	backOverwrite, e = store.Get(backOverwrite.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", backOverwrite.Status, backOverwrite.Error)
	secondDB, e = pgx.Connect(ctx, secondCfg.Database.DSN)
	require.NoError(t, e)
	require.NoError(t, secondDB.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	require.Equal(t, 2, count)
	secondDB.Close(ctx)

	require.NoError(t, native.Stop(ctx, active.Deployment))
	exerciseLocalEntry(t, ctx, root, profile, sourceConfig)

	t.Logf("native whole switch passed: failed=%s coordinator_SIGKILL_recovered_B=true completed=%s active_A=true fresh_rollback=%s", failed.ID, switched.ID, switched.RollbackSnapshotID)
}

// Exercise the actual CLI used by make dev-recovery, using only this test's
// source cluster. The caller's ordinary development services stay untouched.
func exerciseLocalEntry(t *testing.T, ctx context.Context, root string, profile Profile, source snapshots.Config) {
	t.Helper()
	cfg, e := sourceConfig(source)
	require.NoError(t, e)
	cfg.Recovery = config.RecoveryConfig{}
	cfg.SystemSnapshots.Directory = filepath.Join(root, "local-snapshots")
	raw, e := yaml.Marshal(cfg)
	require.NoError(t, e)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "local-input"), 0700))
	file := filepath.Join(root, "local-input", "config.yaml")
	require.NoError(t, os.WriteFile(file, raw, 0600))
	web, e := freePort()
	require.NoError(t, e)
	api, e := freePort()
	require.NoError(t, e)
	directory := filepath.Join(root, "local")
	record := filepath.Join(root, "processes/local-entry.json")
	env := append(os.Environ(), "POWERX_WEB_OUTPUT="+source.Artifacts["core/web-admin"], "PLATFORM_CAPABILITIES_DIR="+source.Artifacts["core/platform_capabilities"])
	args := []string{"-action", "local", "-config", file, "-core-binary", source.Artifacts["core/powerx"], "-local-directory", directory, "-web-port", fmt.Sprint(web), "-api-port", fmt.Sprint(api)}
	_, e = startOwned(ctx, record, profile.Tools.Coordinator, filepath.Dir(file), args, env, []int{web, api})
	require.NoError(t, e)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		assert.NoError(t, stopOwned(cleanup, record))
	}()
	ready := false
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		response, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", api))
		if e == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		select {
		case <-ctx.Done():
			require.NoError(t, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatalf("local CLI did not become ready; inspect owned process log %s.log", record)
	}
	localProfile, e := ReadProfile(filepath.Join(directory, "profile.json"))
	require.NoError(t, e)
	require.True(t, localProfile.Local)
	var active Active
	require.NoError(t, snapshots.ReadCoordinatorJSON(filepath.Join(localProfile.Directory, "active.json"), &active))
	require.NotEqual(t, cfg.Database.Host, active.Deployment.SocketDirectory)
	localCfgRaw, e := os.ReadFile(filepath.Join(active.Deployment.Root, "launch.yaml"))
	require.NoError(t, e)
	var localCfg config.Config
	require.NoError(t, yaml.Unmarshal(localCfgRaw, &localCfg))
	localDB, e := pgx.Connect(ctx, localCfg.Database.DSN)
	require.NoError(t, e)
	var count int
	require.NoError(t, localDB.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders").Scan(&count))
	var claims reqctx.CoreXClaims
	require.NoError(t, localDB.QueryRow(ctx, `SELECT u.id,u.uuid::text,m.id,m.uuid::text,m.tenant_uuid::text FROM public.iam_user u JOIN public.iam_member m ON m.user_id=u.id WHERE u.is_root ORDER BY m.id LIMIT 1`).Scan(&claims.UserID, &claims.UserUUID, &claims.MemberID, &claims.MemberUUID, &claims.TenantUUID))
	localDB.Close(ctx)
	require.Equal(t, 2, count)
	claims.IsRoot = true
	claims.Platforms = []string{"admin"}
	claims.Env = localCfg.Deployment.Env
	bearer, e := auth.GenerateAccessJWT(claims, localCfg.Auth.Issuer, []string{localCfg.Auth.AudienceUser}, time.Minute, []byte(localCfg.Auth.JWTSecret))
	require.NoError(t, e)
	controlURL := fmt.Sprintf("http://127.0.0.1:%d", web) + "/" + strings.Trim(localCfg.Server.APIPrefix, "/") + "/admin/ops/backup/system-snapshots/recovery"
	denied, e := http.Get(controlURL)
	require.NoError(t, e)
	denied.Body.Close()
	require.Equal(t, 401, denied.StatusCode)
	controlRequest, e := http.NewRequestWithContext(ctx, "GET", controlURL, nil)
	require.NoError(t, e)
	controlRequest.Header.Set("Authorization", "Bearer "+bearer)
	controlResponse, e := http.DefaultClient.Do(controlRequest)
	require.NoError(t, e)
	require.Equal(t, 200, controlResponse.StatusCode)
	var controlState struct {
		Data struct {
			ActiveID string `json:"active_environment_id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(controlResponse.Body).Decode(&controlState))
	controlResponse.Body.Close()
	require.Equal(t, active.Deployment.ID, controlState.Data.ActiveID)
	response, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/users/login", web))
	require.NoError(t, e)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, stopOwned(ctx, record))
	// No owned native process may outlive the CLI's graceful shutdown.
	entries, e := os.ReadDir(localProfile.CandidateDirectory)
	require.NoError(t, e)
	for _, entry := range entries {
		for _, name := range []string{"instance", "postgres"} {
			var p processRecord
			if snapshots.ReadCoordinatorJSON(filepath.Join(localProfile.CandidateDirectory, entry.Name(), "processes", name+".json"), &p) == nil {
				identity, e := processIdentity(p.PID)
				require.True(t, e != nil || identity != p.Started)
			}
		}
	}
	t.Log("actual local CLI clone, stable web/API gateways and graceful process cleanup passed")
}
