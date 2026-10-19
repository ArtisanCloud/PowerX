package system_recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// 使用原生 fixture 已拥有的 PG/Redis。这里只启动普通 Core，不启动协调器、网关或第二实例。
func exerciseCurrentApplication(t *testing.T, ctx context.Context, root string, cfg *config.Config, source snapshots.Config, binary, webOutput string, actor reqctx.CoreXClaims) {
	t.Helper()
	app := filepath.Join(root, "source")
	core := filepath.Join(app, "powerx")
	require.NoError(t, snapshots.CopyLocalProgram(ctx, binary, core))
	web := filepath.Join(app, "web-output")
	require.NoError(t, snapshots.CopyLocalProgram(ctx, webOutput, web))
	caps := filepath.Join(app, "platform_capabilities")
	require.NoError(t, snapshots.CopyLocalProgram(ctx, source.Artifacts["core/platform_capabilities"], caps))
	// 辅助配置也复制到本次自建实例，恢复测试不得写回工作区中的原始模板。
	flowBase, flowBusiness := filepath.Join(app, "flow-base"), filepath.Join(app, "flow-business")
	for source, target := range map[string]string{cfg.Agent.FlowSpec.BaseDir: flowBase, cfg.Agent.FlowSpec.BusinessDir: flowBusiness} {
		if _, e := os.Stat(source); os.IsNotExist(e) {
			require.NoError(t, os.MkdirAll(target, 0700))
		} else {
			require.NoError(t, snapshots.CopyLocalProgram(ctx, source, target))
		}
	}
	alias := func(path string) string { return strings.TrimPrefix(path, "/private") }
	cfg.Agent.FlowSpec.BaseDir, cfg.Agent.FlowSpec.BusinessDir = alias(flowBase), alias(flowBusiness)
	cfg.Storage.Local.BasePath = alias(cfg.Storage.Local.BasePath)
	cfg.Plugin.InstalledDir, cfg.Plugin.RegistryFile = alias(cfg.Plugin.InstalledDir), alias(cfg.Plugin.RegistryFile)
	cfg.Deployment.Env = "dev"
	cfg.Recovery = config.RecoveryConfig{}
	cfg.Install.Status = "installed"
	cfg.Install.AllowWithoutDB = false
	cfg.Server.Host = "127.0.0.1"
	var err error
	cfg.Server.Port, err = freePort()
	require.NoError(t, err)
	cfg.Server.GRPC.Port, err = freePort()
	require.NoError(t, err)
	cfg.SystemSnapshots.Directory = source.Directory
	// 持久事件和任务同属专用 DB 5；DB 0 只作多个应用共享的可重建缓存。
	cfg.Event.Bus.RedisDB = cfg.Queue.Redis.DB
	cfg.AI.Runtime.DurableSessions.Enabled = false
	cfg.AI.Runtime.DurableSessions.AdminChatEnabled = false
	cfg.Auth.Issuer = "current-state-A"
	file := filepath.Join(app, "config.yaml")
	require.NoError(t, writeInstallationConfig(file, cfg))
	source.Instance = "dev"
	source.Artifacts = map[string]string{"core/powerx": core, "core/web-admin": web, "core/platform_capabilities": caps}
	legacyCore := os.Getenv("POWERX_CURRENT_LEGACY_CORE")
	if legacyCore != "" {
		// 使用真实旧 Core 产物复现早期快照格式，业务记录仍全部来自自建夹具。
		source.Artifacts["core/powerx"] = legacyCore
		delete(source.Artifacts, "core/platform_capabilities")
	}
	source.EffectiveConfig, err = yaml.Marshal(cfg)
	require.NoError(t, err)
	service := snapshots.NewService(ctx, source, snapshots.Options{Coordinator: true, Backend: &snapshots.PostgresBackend{DSN: source.DatabaseDSN, MediaRoot: source.MediaRoot}, DispatchCapture: func(context.Context, string) error { return nil }})
	defer service.Close()
	point, err := service.Start(privileged(ctx), uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, service.RunQueuedCapture(ctx, point.ID))
	db, err := pgx.Connect(ctx, source.DatabaseDSN)
	require.NoError(t, err)
	_, err = db.Exec(ctx, "INSERT INTO public.recovery_fixture_orders(label) VALUES ('test5')")
	require.NoError(t, err)
	data := []byte("current-media-B")
	sum := sha256.Sum256(data)
	require.NoError(t, os.WriteFile(filepath.Join(source.MediaRoot, "fixture/sample.txt"), data, 0600))
	_, err = db.Exec(ctx, "UPDATE public.media_assets SET expected_checksum=$1,size_bytes=$2", hex.EncodeToString(sum[:]), len(data))
	require.NoError(t, err)
	require.NoError(t, db.Close(ctx))
	cfg.Auth.Issuer = "current-state-B"
	require.NoError(t, writeInstallationConfig(file, cfg))
	// 模拟真实开发机：缓存 DB 0 被其他项目持续使用，Core 队列独占 DB 5。
	foreignCache := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%d", cfg.Cache.Host, cfg.Cache.Port), DB: cfg.Cache.DB})
	t.Cleanup(func() { _ = foreignCache.Close() })
	require.NoError(t, foreignCache.Set(ctx, "foreign-project:keep", "untouched", 0).Err())
	cacheBefore, err := CurrentCacheNamespace(cfg, file)
	require.NoError(t, err)
	require.NoError(t, foreignCache.Set(ctx, cacheBefore+"fixture:stale-B", "B", 0).Err())
	queue := redis.NewClient(&redis.Options{Addr: cfg.Queue.Redis.Addr, DB: cfg.Queue.Redis.DB})
	require.NoError(t, queue.Set(ctx, "fixture:current-B", "B", 0).Err())
	require.NoError(t, queue.Close())
	record := filepath.Join(app, "processes/current-core.json")
	start := func() processRecord {
		t.Helper()
		_, err := startOwned(ctx, record, core, app, nil, cleanEnvironment(app, "PWD="+alias(app), "POWERX_CONFIG="+file, "POWERX_WEB_OUTPUT="+alias(web), "PLATFORM_CAPABILITIES_DIR="+alias(caps)), []int{cfg.Server.Port, cfg.Server.GRPC.Port})
		require.NoError(t, err)
		var owned processRecord
		require.NoError(t, snapshots.ReadCoordinatorJSON(record, &owned))
		require.Eventually(t, func() bool {
			response, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", cfg.Server.Port))
			if e != nil {
				return false
			}
			defer response.Body.Close()
			return response.StatusCode == 200
		}, 90*time.Second, 200*time.Millisecond)
		return owned
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, stopOwned(cleanup, record))
	})
	owned := start()
	directory := currentDirectory(source.Directory, file)
	r, err := openCurrent(directory)
	require.NoError(t, err)
	defer r.Coordinator.SnapshotStore.Close()
	require.Equal(t, alias(app), r.spec.Root, "真实启动采用 /tmp 别名 PWD，配置采用 /private/tmp")
	registered, err := r.Coordinator.SnapshotStore.LoadCoordinatorSource()
	require.NoError(t, err)
	for logical, path := range registered.ConfigFiles {
		if !strings.HasPrefix(logical, "core/files/") {
			continue
		}
		physical, e := currentRestoreTarget(path)
		require.NoError(t, e)
		require.True(t, inside(app, physical), "辅助恢复目标必须属于自建夹具：%s", path)
	}
	key, err := r.Coordinator.readRecoveryKey()
	require.NoError(t, err)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
	request := func(method, path, csrf string, body any) *http.Response {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, e := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(raw))
		require.NoError(t, e)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", base)
		if csrf != "" {
			req.Header.Set("X-PowerX-Recovery-CSRF", csrf)
		}
		req.Header.Set("Idempotency-Key", uuid.NewString())
		response, e := client.Do(req)
		require.NoError(t, e)
		return response
	}
	response := request("POST", recoveryEntry+"session", "", map[string]string{"key": key})
	require.Equal(t, 200, response.StatusCode)
	var session struct {
		CSRF string `json:"csrf"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&session))
	response.Body.Close()
	restore := func(id string) snapshots.ManagedJob {
		t.Helper()
		active, e := r.Coordinator.Active()
		require.NoError(t, e)
		response := request("POST", recoveryEntry+"restore", session.CSRF, map[string]any{"snapshot_id": id, "confirmation": id, "generation": active.Generation})
		defer response.Body.Close()
		require.Equal(t, 202, response.StatusCode)
		var receipt struct {
			Recovery snapshots.ManagedJob `json:"recovery"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
		var result snapshots.ManagedJob
		require.Eventually(t, func() bool {
			result, e = r.Coordinator.Store.Get(receipt.Recovery.ID)
			return e == nil && (result.Status == "complete" || result.Status == "failed" || result.Status == "attention_required")
		}, 120*time.Second, 100*time.Millisecond)
		identity, e := processIdentity(owned.PID)
		require.NoError(t, e)
		require.Equal(t, owned.Started, identity, "恢复期间使用同一 Core PID")
		require.Eventually(t, func() bool {
			response, e := http.Get(base + "/healthz")
			if e != nil {
				return false
			}
			defer response.Body.Close()
			return response.StatusCode == 200
		}, 15*time.Second, 100*time.Millisecond)
		return result
	}
	a := restore(point.ID)
	require.Equal(t, "complete", a.Status, a.Error)
	if legacyCore != "" {
		require.Contains(t, checkNames(a.Checks), "local_core_adapter")
		require.Contains(t, checkNames(a.Checks), "local_capability_adapter")
	}
	require.NotEmpty(t, a.RollbackSnapshotID)
	assertState := func(count int, issuer, media string) {
		t.Helper()
		db, e := pgx.Connect(ctx, source.DatabaseDSN)
		require.NoError(t, e)
		defer db.Close(ctx)
		var got int
		require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders WHERE label='test5'").Scan(&got))
		require.Equal(t, count, got)
		var config config.Config
		raw, e := os.ReadFile(file)
		require.NoError(t, e)
		require.NoError(t, yaml.Unmarshal(raw, &config))
		require.Equal(t, issuer, config.Auth.Issuer)
		raw, e = os.ReadFile(filepath.Join(source.MediaRoot, "fixture/sample.txt"))
		require.NoError(t, e)
		require.Equal(t, media, string(raw))
		require.Equal(t, source.DatabaseDSN, config.Database.DSN)
	}
	assertState(0, "current-state-A", "media-at-A")
	cacheAfter, err := CurrentCacheNamespace(cfg, file)
	require.NoError(t, err)
	require.NotEqual(t, cacheBefore, cacheAfter, "恢复后不再读取 B 的旧缓存")
	require.ErrorIs(t, foreignCache.Get(ctx, cacheAfter+"fixture:stale-B").Err(), redis.Nil)
	foreignValue, err := foreignCache.Get(ctx, "foreign-project:keep").Result()
	require.NoError(t, err)
	require.Equal(t, "untouched", foreignValue, "共享缓存库没有被清空")
	require.NoError(t, stopOwned(ctx, record))
	owned = start()
	assertState(0, "current-state-A", "media-at-A")
	afterRestart, err := CurrentCacheNamespace(cfg, file)
	require.NoError(t, err)
	require.Equal(t, cacheAfter, afterRestart, "普通重启保持当前缓存代次")
	b := restore(a.RollbackSnapshotID)
	require.Equal(t, "complete", b.Status, b.Error)
	assertState(1, "current-state-B", "current-media-B")
	queue = redis.NewClient(&redis.Options{Addr: cfg.Queue.Redis.Addr, DB: cfg.Queue.Redis.DB})
	value, err := queue.Get(ctx, "fixture:current-B").Result()
	require.NoError(t, err)
	require.Equal(t, "B", value)
	require.NoError(t, queue.Close())
	// 外部连接不会被强制踢掉，也不会进入覆盖；失败后仍启动原状态。
	blocker, err := pgx.Connect(ctx, source.DatabaseDSN)
	require.NoError(t, err)
	failed := restore(point.ID)
	require.Equal(t, "failed", failed.Status)
	require.Contains(t, failed.Error, "其他程序连接")
	require.NoError(t, blocker.Close(ctx))
	assertState(1, "current-state-B", "current-media-B")
	queueBlocker := redis.NewClient(&redis.Options{Addr: cfg.Queue.Redis.Addr, DB: cfg.Queue.Redis.DB})
	require.NoError(t, queueBlocker.Ping(ctx).Err())
	failed = restore(point.ID)
	require.Equal(t, "failed", failed.Status)
	require.Contains(t, failed.Error, "queue")
	require.Contains(t, failed.Error, "DB 5")
	require.NoError(t, queueBlocker.Close())
	assertState(1, "current-state-B", "current-media-B")
	foreignValue, err = foreignCache.Get(ctx, "foreign-project:keep").Result()
	require.NoError(t, err)
	require.Equal(t, "untouched", foreignValue)
	report := map[string]any{"passed": true, "same_core_pid_across_restore": true, "same_config_dsn_and_ports": true, "no_coordinator_or_gateway_process_started": true, "A_then_protected_B": true, "full_restart_retains_A": true, "redis_protection_restored": true, "external_connection_blocks_before_overwrite": true, "shared_cache_connection_does_not_block": true, "shared_cache_data_retained": true, "cache_namespace_rotates_on_recovery": true, "external_task_connection_blocks_before_overwrite": true, "darwin_aliased_pwd_and_mixed_paths": true, "auxiliary_restore_targets_owned_by_fixture": true, "core_pid": owned.PID, "config_file": file, "backend_port": cfg.Server.Port}
	report["legacy_snapshot_missing_catalog"] = legacyCore != ""
	workspace, _ := filepath.Abs("../../../..")
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(workspace, "test-results/current-recovery-native-evidence.json"), report))
	if ready := os.Getenv("POWERX_CURRENT_BROWSER_READY_FILE"); ready != "" {
		webPort := 3489
		record := filepath.Join(app, "processes/current-web.json")
		_, err := startOwned(ctx, record, "/opt/homebrew/bin/node", app, []string{filepath.Join(web, "server/index.mjs")}, cleanEnvironment(app, fmt.Sprintf("PORT=%d", webPort), "HOST=127.0.0.1", "POWERX_BACKEND="+base), []int{webPort})
		require.NoError(t, err)
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			require.NoError(t, stopOwned(cleanup, record))
		})
		sub, err := opswatch.Subscribe(ctx, map[string][]string{"done": {filepath.Dir(ready)}}, func(path string) bool { return filepath.Base(path) == filepath.Base(ready)+".done.json" })
		require.NoError(t, err)
		defer sub.Close()
		require.NoError(t, snapshots.WriteCoordinatorJSON(ready, map[string]any{"url": fmt.Sprintf("http://127.0.0.1:%d", webPort), "core_url": base, "core_pid": owned.PID, "snapshot_id": point.ID, "key_file": r.Coordinator.recoveryKeyFile(), "done_file": ready + ".done.json", "web_process_record": record}))
		select {
		case <-ctx.Done():
			t.Fatal("浏览器验收未完成")
		case <-sub.Changes:
		}
		var browser struct {
			Passed   bool `json:"passed"`
			Finished bool `json:"finished"`
		}
		require.NoError(t, snapshots.ReadCoordinatorJSON(ready+".done.json", &browser))
		require.True(t, browser.Passed, "真实浏览器验收失败，保留夹具与日志")
		assertState(0, "current-state-A", "media-at-A")
	}
	// 普通 Core 的真实 Root JWT 路由执行删除；仅使用本测试创建的包、业务库和私有展开目录。
	var current config.Config
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(raw, &current))
	access, err := auth.GenerateAccessJWT(actor, current.Auth.Issuer, []string{current.Auth.AudienceUser}, time.Hour, []byte(current.Auth.JWTSecret))
	require.NoError(t, err)
	api := func(method, id string, deleteProtected bool) *http.Response {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, base+strings.TrimRight(config.ResolveAPIPrefix(&current), "/")+"/admin/ops/backup/system-snapshots/"+id, nil)
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set("X-Snapshot-Confirmation", id)
		req.Header.Set("X-Snapshot-Delete-Protected", fmt.Sprint(deleteProtected))
		response, e := client.Do(req)
		require.NoError(t, e)
		return response
	}
	remove := func(id string, protected bool) {
		t.Helper()
		response := api("DELETE", id, protected)
		defer response.Body.Close()
		var result struct {
			Data struct {
				Deletion snapshots.SnapshotDeletion `json:"deletion"`
			} `json:"data"`
		}
		require.Equal(t, 200, response.StatusCode)
		require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
		require.Equal(t, snapshots.SnapshotDeletion{ID: id, RecordDeleted: true, FilesDeleted: true}, result.Data.Deletion)
		require.NoDirExists(t, filepath.Join(source.Directory, "contents", id))
		require.NoFileExists(t, filepath.Join(source.Directory, "snapshots", id+".json"))
		require.NoFileExists(t, filepath.Join(source.Directory, "requests", id+".json"))
	}
	remove(point.ID, false)
	response = api("GET", point.ID, false)
	require.Equal(t, 404, response.StatusCode)
	response.Body.Close()
	jobs, err := r.Coordinator.Store.List()
	require.NoError(t, err)
	for _, job := range jobs {
		if job.SnapshotID == point.ID || job.RollbackSnapshotID == point.ID {
			require.NoDirExists(t, r.work(job.ID), "删除已结束恢复的展开副本")
			_, err = r.Coordinator.Store.Get(job.ID)
			require.NoError(t, err, "恢复审计历史保留")
		}
	}
	response = api("DELETE", a.RollbackSnapshotID, false)
	require.Equal(t, 409, response.StatusCode, "保护快照需要单独明确确认")
	response.Body.Close()
	require.DirExists(t, filepath.Join(source.Directory, "contents", a.RollbackSnapshotID))
	remove(a.RollbackSnapshotID, true)
	if os.Getenv("POWERX_CURRENT_BROWSER_READY_FILE") != "" {
		assertState(0, "current-state-A", "media-at-A")
	} else {
		assertState(1, "current-state-B", "current-media-B")
	}
	identity, err := processIdentity(owned.PID)
	require.NoError(t, err)
	require.Equal(t, owned.Started, identity, "删除不重启业务进程")
	report["authenticated_snapshot_delete"] = true
	report["protected_delete_confirmation"] = true
	report["failed_restore_staging_cleaned"] = true
	report["business_state_unchanged_by_deletion"] = true
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(workspace, "test-results/current-recovery-native-evidence.json"), report))
}
