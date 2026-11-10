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
func TestMonitoringControlNativeHTTPAndSSE(t *testing.T) {
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
	dir := t.TempDir()
	coordinator, e := budget.New(budget.Config{Directory: filepath.Join(dir, "runtime"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	var svc *gov.Service
	control, e := gov.NewControlService(db, gov.ControlOptions{Scope: gov.ControlScope{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http"}, Queue: queue, Changed: func() {
		if svc != nil {
			svc.PublishChange("configuration_changed")
		}
	}})
	require.NoError(t, e)
	svc, e = gov.NewService(gov.Options{Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", Environment: "test"}, FilesystemPath: dir, Collector: testCollector{}, DB: db, Budget: coordinator, Control: control, ControlWorkerEnabled: true})
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
	if os.Getenv("POWERX_MONITOR_BROWSER_CONTROL_DIR") != "" {
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
	path := "/custom/v2/admin/ops/storage/database-tables/monitoring"
	// 可选浏览器联调仅开放自有 fixture；凭据只写临时私有文件，退出即删除。
	if target := os.Getenv("POWERX_MONITOR_BROWSER_CONTROL_DIR"); target != "" {
		require.True(t, filepath.IsAbs(target))
		require.NoError(t, os.MkdirAll(target, 0700))
		credential := filepath.Join(target, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex owned monitoring browser fixture", "pid": os.Getpid(), "url": server.URL, "cwd": dir, "command": "go test TestMonitoringControlNativeHTTPAndSSE with owned browser fixture", "credential_file": credential})
		require.NoError(t, os.WriteFile(filepath.Join(target, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(4 * time.Minute):
			t.Fatal("owned browser fixture timed out")
		}
		root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
		state, e := svc.DatabaseTableMonitoring(root)
		require.NoError(t, e)
		require.EqualValues(t, 2, state.Configuration.DesiredRevision)
		require.EqualValues(t, 2, state.Configuration.AppliedRevision)
		require.EqualValues(t, 128<<20, state.Policy.GrowthBytes)
		require.False(t, state.Policy.Enabled)
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
	var status gov.TableMonitoringStatus
	require.NoError(t, json.Unmarshal(call("GET", path, nil, 200), &status))
	require.Equal(t, "legacy", status.Configuration.Authority)
	call("PUT", path, map[string]any{"expected_revision": 0, "confirm": true, "policy": status.Policy}, 400)
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
	key := uuid.NewString()
	var mutation gov.TableMonitoringMutation
	require.NoError(t, json.Unmarshal(call("POST", path+"/adoptions", map[string]any{"request_id": key, "legacy_revision": status.Configuration.LegacyRevision, "legacy_digest": status.Configuration.LegacyDigest, "confirm": true}, 200), &mutation))
	require.False(t, mutation.OperationPolicy.Enabled)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for status.Configuration.State != "applied" {
		select {
		case <-frames:
			require.NoError(t, json.Unmarshal(call("GET", path, nil, 200), &status))
		case <-deadline.C:
			t.Fatal("publication never confirmed through SSE")
		}
	}
	require.EqualValues(t, 1, status.Configuration.DesiredRevision)
	require.EqualValues(t, 1, *status.Configuration.SnapshotRevision)
	call("GET", path+"/requests/"+key, nil, 200)
	policy := mutation.OperationPolicy
	policy.GrowthBytes = 128 << 20
	key = uuid.NewString()
	require.NoError(t, json.Unmarshal(call("PUT", path, map[string]any{"expected_revision": 1, "request_id": key, "confirm": true, "policy": policy}, 200), &mutation))
	for status.Configuration.AppliedRevision != 2 {
		select {
		case <-frames:
			require.NoError(t, json.Unmarshal(call("GET", path, nil, 200), &status))
		case <-time.After(5 * time.Second):
			t.Fatal("updated thresholds never confirmed through push")
		}
	}
	require.EqualValues(t, 128<<20, status.Policy.GrowthBytes)
	call("GET", path+"/requests/"+key, nil, 200)

	call("PUT", path, map[string]any{"expected_revision": 0, "request_id": uuid.NewString(), "confirm": true, "policy": mutation.OperationPolicy}, 409)
	call("GET", path+"/requests/"+uuid.NewString(), nil, 404)
	// 清空已提交文件事件后，单次应用通知只触发推送，不形成 SSE 自回声。
	time.Sleep(100 * time.Millisecond)
drain:
	for {
		select {
		case <-frames:
		default:
			break drain
		}
	}
	svc.PublishChange("owned_single_event")
	select {
	case name := <-frames:
		require.Equal(t, "change", name)
	case <-time.After(time.Second):
		t.Fatal("in-memory publication event not bridged to SSE")
	}
	select {
	case <-frames:
		t.Fatal("SSE notification echoed without a state change")
	case <-time.After(100 * time.Millisecond):
	}
}
