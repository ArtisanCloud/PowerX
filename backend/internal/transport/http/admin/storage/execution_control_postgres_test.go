package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	observer "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	anchor "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 实际 HTTP、JWT、PostgreSQL、私有文件及 TaskBus 阻塞消费者；Redis 传输为 miniredis。
func TestExecutionControlNativeHTTPAndSSE(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("owned temporary PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	previous := model.PowerXSchema
	schema := "execution_control_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		model.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}, &capm.HealthProbeResult{}, &capm.DiscoveryCacheEntry{}, &tenants.Tenant{}))
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	base, e := event_bus.NewRedisStreamTaskDriver(redisClient, 10*time.Second, "owned-monitoring-http")
	require.NoError(t, e)
	queue := taskqueue.NewStreamDriver(db, base)
	dir := t.TempDir()
	minimum, critical := int64(1<<20), int64(1)
	if os.Getenv("POWERX_EXECUTION_BROWSER_DIR") != "" || os.Getenv("POWERX_RECOVERY_BROWSER_DIR") != "" {
		minimum = 20 << 30
		critical = 10 << 30
	}
	coordinator, e := budget.New(budget.Config{Directory: filepath.Join(dir, "runtime"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", MinFreeBytes: minimum, CriticalFreeBytes: critical})
	require.NoError(t, e)
	var svc *gov.Service
	control, e := gov.NewControlService(db, gov.ControlOptions{Scope: gov.ControlScope{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http"}, Queue: queue, Changed: func() {
		if svc != nil {
			svc.PublishChange("configuration_changed")
		}
	}})
	require.NoError(t, e)
	svc, e = gov.NewService(gov.Options{Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", Environment: "test"}, FilesystemPath: dir, Collector: testCollector{}, DB: db, Budget: coordinator, Control: control, ControlWorkerEnabled: true, DatabaseRetentionAllowed: true, LifecycleWorkerEnabled: true, DatabaseCollector: observer.PostgresCollector{DB: db, ClusterID: "owned-system-browser"}})
	require.NoError(t, e)
	anchorDir := filepath.Join(dir, "maintenance")
	require.NoError(t, os.Mkdir(anchorDir, 0700))
	recoveryAnchor, e := anchor.Initialize(anchorDir, "owned-http")
	require.NoError(t, e)
	require.NoError(t, svc.BindRecoveryAnchor(recoveryAnchor))
	ctx, cancel := context.WithCancel(context.Background())
	svc.StartControl(ctx, func(err error) { t.Logf("owned system retention control worker: %v", err) })
	t.Cleanup(func() {
		cancel()
		select {
		case <-control.Done():
		case <-time.After(10 * time.Second):
			t.Error("owned publication worker did not stop")
		}
	})
	secret := []byte("owned-monitor-http-no-deployment-key")
	const issuer = "owned-monitoring"
	token, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, issuer, []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	router := gin.New()
	protected := router.Group("/custom/v2", authmw.JwtMiddleware(secret, issuer, []string{"admin"}, []string{"access"}, nil))
	RegisterAPIRoutes(nil, protected, &shared.Deps{DB: db, StorageGovernance: svc})
	finish := make(chan struct{}, 1)
	if os.Getenv("POWERX_EXECUTION_BROWSER_DIR") != "" {
		protected.POST("/__owned-test/finish", requireRootUser, func(c *gin.Context) {
			select {
			case finish <- struct{}{}:
			default:
			}
			c.Status(204)
		})
	}

	if os.Getenv("POWERX_RECOVERY_BROWSER_DIR") != "" {
		protected.POST("/__owned-test/next-recovery", requireRootUser, func(c *gin.Context) {
			out, e := recoveryAnchor.Begin(c.Request.Context(), uuid.NewString(), uuid.NewString())
			if e != nil {
				c.Status(503)
				return
			}
			svc.PublishChange("configuration_changed")
			c.JSON(200, map[string]any{"data": out})
		})
	}
	server := httptest.NewServer(router)
	address := strings.TrimPrefix(server.URL, "http://")
	t.Logf("owned HTTP server: pid=%d cwd=%s command=go-test listener=%s", os.Getpid(), dir, address)
	t.Cleanup(func() {
		server.Close()
		c, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			c.Close()
			t.Error("owned HTTP listener still open")
		}
	})
	client := server.Client()
	path := "/custom/v2/admin/ops/storage/execution-control"
	root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})

	browserDirectory := os.Getenv("POWERX_EXECUTION_BROWSER_DIR")
	if value := os.Getenv("POWERX_RECOVERY_BROWSER_DIR"); value != "" {
		browserDirectory = value
		_, e = recoveryAnchor.Begin(root, uuid.NewString(), uuid.NewString())
		require.NoError(t, e)
	}
	if target := browserDirectory; target != "" {
		old := time.Now().UTC().AddDate(0, 0, -120)
		now := time.Now().UTC()
		require.NoError(t, db.Create(&[]capm.HealthProbeResult{{AdapterID: "owned-browser", ProbeWindowStart: old, Status: "healthy", NextRetryAt: old, CreatedAt: old, UpdatedAt: old}, {AdapterID: "owned-browser", ProbeWindowStart: now, Status: "healthy", NextRetryAt: now, CreatedAt: now, UpdatedAt: now}}).Error)
		require.True(t, filepath.IsAbs(target))
		require.NoError(t, os.MkdirAll(target, 0700))
		credential := filepath.Join(target, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex execution protection HTTP/PG fixture", "pid": os.Getpid(), "url": server.URL, "cwd": dir, "command": "go test ./internal/transport/http/admin/storage -run ^TestExecutionControlNativeHTTPAndSSE$ -count=1 -v", "credential_file": credential})
		require.NoError(t, os.WriteFile(filepath.Join(target, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(8 * time.Minute):
			t.Fatal("owned browser timed out")
		}
		state, e := svc.ExecutionConfiguration(root)
		require.NoError(t, e)
		require.True(t, state.Confirmed)
		if os.Getenv("POWERX_RECOVERY_BROWSER_DIR") != "" {
			require.GreaterOrEqual(t, state.DesiredRevision, int64(4))
		} else {
			require.GreaterOrEqual(t, state.DesiredRevision, int64(5))
		}
		return
	}
	call := func(method, endpoint string, input any, jwt string) (int, json.RawMessage) {
		var raw []byte
		if input != nil {
			raw, e = json.Marshal(input)
			require.NoError(t, e)
		}
		request, e := http.NewRequest(method, server.URL+endpoint, bytes.NewReader(raw))
		require.NoError(t, e)
		if jwt != "" {
			request.Header.Set("Authorization", "Bearer "+jwt)
		} else {
			request.Header.Set("X-API-Key", "owned-never-authorized")
		}
		request.Header.Set("Content-Type", "application/json")
		response, e := client.Do(request)
		require.NoError(t, e)
		defer response.Body.Close()
		body, e := io.ReadAll(response.Body)
		require.NoError(t, e)
		var out struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &out))
		return response.StatusCode, out.Data
	}
	sign := func(claims reqctx.CoreXClaims) string {
		claims.TenantUUID = uuid.NewString()
		claims.MemberUUID = uuid.NewString()
		jwt, e := auth.GenerateAccessJWT(claims, issuer, []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		return jwt
	}

	for _, jwt := range []string{"", sign(reqctx.CoreXClaims{UserID: 2}), sign(reqctx.CoreXClaims{UserID: 1, IsRoot: true, PluginID: "com.powerx.plugins.owned"})} {
		for _, route := range [][2]string{{"GET", path}, {"POST", path + "/changes"}, {"GET", path + "/requests/" + uuid.NewString()}} {
			code, _ := call(route[0], route[1], map[string]any{}, jwt)
			require.Contains(t, []int{401, 403}, code)
		}
	}
	code, raw := call("GET", path, nil, token)
	require.Equal(t, 200, code)
	var config gov.ExecutionConfiguration
	require.NoError(t, json.Unmarshal(raw, &config))
	require.Equal(t, "unmanaged", config.Mode)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	request, e := http.NewRequestWithContext(streamCtx, "GET", server.URL+"/custom/v2/admin/ops/storage/runtime/events", nil)
	require.NoError(t, e)
	request.Header.Set("Authorization", "Bearer "+token)
	response, e := client.Do(request)
	require.NoError(t, e)
	require.Equal(t, 200, response.StatusCode)
	frames := make(chan string, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") {
				select {
				case frames <- strings.TrimSpace(strings.TrimPrefix(line, "event:")):
				default:
				}
			}
		}
	}()
	t.Cleanup(func() { streamCancel(); response.Body.Close(); <-done })
	select {
	case name := <-frames:
		require.Equal(t, "snapshot", name)
	case <-time.After(3 * time.Second):
		t.Fatal("no snapshot")
	}

	wait := func(rev int64) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for config.AppliedRevision != rev || !config.Confirmed && !(config.Mode == "recovery" && config.Recovery.Prepared) {
			select {
			case <-frames:
				code, raw = call("GET", path, nil, token)
				require.Equal(t, 200, code)
				require.NoError(t, json.Unmarshal(raw, &config))
			case <-deadline.C:
				t.Fatalf("execution publication not confirmed via SSE: wanted %d, got %+v", rev, config)
			}
		}
	}
	change := func(action string, rev int64) gov.ExecutionChange {
		return gov.ExecutionChange{RequestID: uuid.NewString(), ExpectedRevision: rev, Action: action, Reason: "独立 HTTP 执行保护验收", Confirm: true}
	}
	pause := change("pause", 0)
	code, raw = call("POST", path+"/changes", pause, token)
	require.Equal(t, 200, code)
	var mutation gov.ExecutionMutation
	require.NoError(t, json.Unmarshal(raw, &mutation))
	require.Equal(t, "paused", mutation.Configuration.Mode)
	wait(1)
	code, _ = call("POST", path+"/changes", change("resume", 0), token)
	require.Equal(t, 409, code)
	code, _ = call("POST", path+"/changes", map[string]any{"request_id": uuid.NewString(), "expected_revision": 1, "action": "resume", "reason": "bad raw input", "confirm": true, "sql": "DELETE"}, token)
	require.Equal(t, 400, code)
	code, _ = call("POST", path+"/changes", change("resume", 1), token)
	require.Equal(t, 200, code)
	wait(2)
	code, raw = call("GET", path+"/requests/"+pause.RequestID, nil, token)
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(raw, &mutation))
	require.Equal(t, int64(1), mutation.OperationRevision)
	require.Equal(t, "pause", mutation.Operation.Action)
	require.Equal(t, "open", mutation.Configuration.Mode)
	code, _ = call("GET", path+"/requests/"+uuid.NewString(), nil, token)
	require.Equal(t, 404, code)
	code, _ = call("POST", path+"/changes", change("begin_handoff", 2), token)
	require.Equal(t, 200, code)
	wait(3)
	complete := change("complete_handoff", 3)
	code, _ = call("POST", path+"/changes", complete, token)
	require.Equal(t, 400, code)
	complete.ExternalStopped = true
	complete.AdmittedTasksChecked = true
	code, _ = call("POST", path+"/changes", complete, token)
	require.Equal(t, 200, code)
	wait(4)
	require.Equal(t, "paused", config.Mode)
	code, _ = call("POST", path+"/changes", change("resume", 4), token)
	require.Equal(t, 200, code)

	wait(5)
	epoch, e := recoveryAnchor.Begin(root, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	svc.PublishChange("configuration_changed")
	code, raw = call("GET", path, nil, token)
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(raw, &config))
	require.Equal(t, "recovery", config.Mode)
	ack := change("acknowledge_restore", 5)
	ack.ExpectedRecoveryEpoch = epoch.Epoch
	ack.ExternalStopped = true
	ack.AdmittedTasksChecked = true
	code, _ = call("POST", path+"/changes", ack, token)
	require.Equal(t, 400, code)
	ack.EnvironmentChecked = true
	code, _ = call("POST", path+"/changes", ack, token)
	require.Equal(t, 409, code)
	prepared := ack
	prepared.RequestID = uuid.NewString()
	prepared.Action = "prepare_restore"
	code, _ = call("POST", path+"/changes", prepared, token)
	require.Equal(t, 200, code)
	wait(6)
	ack.ExpectedRevision = 6
	code, _ = call("POST", path+"/changes", ack, token)
	require.Equal(t, 200, code)
	wait(7)
	require.Equal(t, "paused", config.Mode)
	require.Equal(t, "acknowledged", config.Recovery.State)
}
