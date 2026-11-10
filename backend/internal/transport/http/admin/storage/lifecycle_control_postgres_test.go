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
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
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
func TestLifecycleControlNativeHTTPAndSSE(t *testing.T) {
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
	schema := "monitor_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		model.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}))
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	base, e := event_bus.NewRedisStreamTaskDriver(redisClient, 10*time.Second, "owned-monitoring-http")
	require.NoError(t, e)
	queue := taskqueue.NewStreamDriver(db, base)
	dir, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	coordinator, e := budget.New(budget.Config{Directory: filepath.Join(dir, "runtime"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)

	for _, name := range []string{"app.log", "app.log.1.gz", "app.log.2.gz", "app.log.3.gz", "invoice.txt"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("owned lifecycle http file"), 0600))
		age := 21 * 24 * time.Hour
		if name == "app.log" {
			age = 0
		}
		if name == "app.log.1.gz" {
			age = 24 * time.Hour
		}
		at := time.Now().Add(-age)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	engine, e := lifecycle.New(lifecycle.Options{Coordinator: coordinator, InstanceID: "owned-http", AllowEnforce: true, Sources: []lifecycle.Source{{ID: "owned-logs", Name: "隔离验收日志", Kind: "application_logs", Scope: "dev", InstanceID: "owned-http", Directory: dir, Format: "numeric", ActiveFiles: []string{"app.log"}, OwnerUID: uint32(os.Geteuid())}}})
	require.NoError(t, e)
	_, e = engine.Save(context.Background(), 1, "owned-logs", 0, lifecycle.Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20, CleanupMode: "expire_delete"})
	require.NoError(t, e)

	var svc *gov.Service
	control, e := gov.NewControlService(db, gov.ControlOptions{Scope: gov.ControlScope{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http"}, Queue: queue, Changed: func() {
		if svc != nil {
			svc.PublishChange("configuration_changed")
		}
	}})
	require.NoError(t, e)
	svc, e = gov.NewService(gov.Options{Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", Environment: "test"}, FilesystemPath: dir, Collector: testCollector{}, DB: db, Budget: coordinator, Control: control, ControlWorkerEnabled: true, Lifecycle: engine})
	require.NoError(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	svc.StartControl(ctx, func(error) {})
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
	if os.Getenv("POWERX_LIFECYCLE_BROWSER_CONTROL_DIR") != "" {
		protected.POST("/__owned-test/finish", requireRootUser, func(c *gin.Context) {
			select {
			case finish <- struct{}{}:
			default:
			}
			c.Status(204)
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
	path := "/custom/v2/admin/ops/storage/runtime/policies/owned-logs"
	// 可选浏览器联调仅开放自有 fixture；凭据只写临时私有文件，退出即删除。
	if target := os.Getenv("POWERX_LIFECYCLE_BROWSER_CONTROL_DIR"); target != "" {
		require.True(t, filepath.IsAbs(target))
		require.NoError(t, os.MkdirAll(target, 0700))
		credential := filepath.Join(target, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex owned monitoring browser fixture", "pid": os.Getpid(), "url": server.URL, "cwd": dir, "command": "go test TestLifecycleControlNativeHTTPAndSSE with owned browser fixture", "credential_file": credential})
		require.NoError(t, os.WriteFile(filepath.Join(target, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(4 * time.Minute):
			t.Fatal("owned browser fixture timed out")
		}
		root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
		state, e := svc.LifecycleConfiguration(root, "owned-logs")
		require.NoError(t, e)
		require.EqualValues(t, 3, state.DesiredRevision)
		require.EqualValues(t, 3, state.AppliedRevision)
		require.False(t, state.DesiredPolicy.Enabled)
		for _, name := range []string{"app.log", "app.log.1.gz", "invoice.txt"} {
			require.FileExists(t, filepath.Join(dir, name))
		}
		for _, name := range []string{"app.log.2.gz", "app.log.3.gz"} {
			require.NoFileExists(t, filepath.Join(dir, name))
		}
		return
	}
	call := func(method, endpoint string, input any, code int) json.RawMessage {
		var raw []byte
		if input != nil {
			raw, e = json.Marshal(input)
			require.NoError(t, e)
		}
		req, e := http.NewRequest(method, server.URL+endpoint, bytes.NewReader(raw))
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response, e := client.Do(req)
		require.NoError(t, e)
		defer response.Body.Close()
		body, e := io.ReadAll(response.Body)
		require.NoError(t, e)
		require.Equal(t, code, response.StatusCode, string(body))
		var out struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &out))
		return out.Data
	}
	var status gov.LifecycleConfiguration
	require.NoError(t, json.Unmarshal(call("GET", path, nil, 200), &status))
	require.Equal(t, "legacy", status.Authority)
	call("PUT", path, map[string]any{"expected_revision": 0, "confirm": true}, 400)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	request, e := http.NewRequestWithContext(streamCtx, "GET", server.URL+"/custom/v2/admin/ops/storage/runtime/events", nil)
	require.NoError(t, e)
	request.Header.Set("Authorization", "Bearer "+token)
	response, e := client.Do(request)
	require.NoError(t, e)
	require.Equal(t, 200, response.StatusCode)
	frames := make(chan string, 32)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
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
	t.Cleanup(func() { streamCancel(); response.Body.Close(); <-readerDone })
	select {
	case name := <-frames:
		require.Equal(t, "snapshot", name)
	case <-time.After(3 * time.Second):
		t.Fatal("no initial SSE snapshot")
	}

	waitApplied := func(revision int64) {
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for {
			require.NoError(t, json.Unmarshal(call("GET", path, nil, 200), &status))
			if status.State == "applied" && status.AppliedRevision == revision {
				return
			}
			select {
			case <-frames:
			case <-deadline.C:
				t.Fatal("lifecycle publication not confirmed through SSE")
			}
		}
	}
	key := uuid.NewString()
	var mutation gov.LifecycleMutation
	require.NoError(t, json.Unmarshal(call("POST", path+"/adoptions", map[string]any{"request_id": key, "legacy_revision": status.LegacyRevision, "legacy_digest": status.LegacyDigest, "confirm": true}, 200), &mutation))
	require.False(t, mutation.OperationPolicy.Enabled)
	waitApplied(1)
	call("GET", path+"/requests/"+key, nil, 200)
	key = uuid.NewString()
	body := map[string]any{"expected_revision": 1, "request_id": key, "confirm": true, "enabled": true, "cleanup_mode": "expire_delete", "require_archive": false, "retention_days": 14, "max_bytes": 1 << 20}
	require.NoError(t, json.Unmarshal(call("PUT", path, body, 200), &mutation))
	require.EqualValues(t, 2, mutation.OperationRevision)
	waitApplied(2)
	call("PUT", path, body, 200)
	call("GET", path+"/requests/"+key, nil, 200)
	body["request_id"] = uuid.NewString()
	call("PUT", path, body, 409)
	call("GET", path+"/requests/"+uuid.NewString(), nil, 404)
	var run lifecycle.Run
	runPath := "/custom/v2/admin/ops/storage/runtime/runs"
	runBody := map[string]any{"source_id": "owned-logs", "expected_revision": 2, "request_id": uuid.NewString(), "confirm": true}
	require.NoError(t, json.Unmarshal(call("POST", runPath, runBody, 200), &run))
	require.Equal(t, 2, run.DeletedFiles)
	firstID := run.ID
	require.NoError(t, json.Unmarshal(call("POST", runPath, runBody, 200), &run))
	require.Equal(t, firstID, run.ID)
	body["expected_revision"] = 2
	body["request_id"] = uuid.NewString()
	body["enabled"] = false
	call("PUT", path, body, 200)
	waitApplied(3)
	require.False(t, status.DesiredPolicy.Enabled)
	for _, name := range []string{"app.log", "app.log.1.gz", "invoice.txt"} {
		require.FileExists(t, filepath.Join(dir, name))
	}
	for _, name := range []string{"app.log.2.gz", "app.log.3.gz"} {
		require.NoFileExists(t, filepath.Join(dir, name))
	}
}
