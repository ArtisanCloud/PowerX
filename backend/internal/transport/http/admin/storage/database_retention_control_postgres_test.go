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
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 实际 HTTP、JWT、PostgreSQL、私有文件及 TaskBus 阻塞消费者；Redis 传输为 miniredis。
func TestDatabaseRetentionControlNativeHTTPAndSSE(t *testing.T) {
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
	schema := "budget_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		model.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}, &capm.InvocationTrace{}, &capm.CapabilityEventPublication{}, &tenants.Tenant{}))
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	base, e := event_bus.NewRedisStreamTaskDriver(redisClient, 10*time.Second, "owned-monitoring-http")
	require.NoError(t, e)
	queue := taskqueue.NewStreamDriver(db, base)
	dir := t.TempDir()
	minimum, critical := int64(1<<20), int64(1)
	if os.Getenv("POWERX_RETENTION_BROWSER_DIR") != "" {
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
	svc, e = gov.NewService(gov.Options{Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", Environment: "test"}, FilesystemPath: dir, Collector: testCollector{}, DB: db, Budget: coordinator, Control: control, ControlWorkerEnabled: true, DatabaseRetentionAllowed: true, LifecycleWorkerEnabled: true})
	require.NoError(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	svc.StartControl(ctx, func(err error) { t.Logf("owned retention control worker: %v", err) })
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
	if os.Getenv("POWERX_RETENTION_BROWSER_DIR") != "" {
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
	path := "/custom/v2/admin/ops/storage/database-retention"
	root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	if target := os.Getenv("POWERX_RETENTION_BROWSER_DIR"); target != "" {
		tenant := &tenants.Tenant{Key: uuid.NewString(), Name: "owned browser retention"}
		require.NoError(t, db.Create(tenant).Error)
		old := time.Now().UTC().AddDate(0, 0, -120)
		require.NoError(t, db.Create(&capm.InvocationTrace{TenantUUID: tenant.UUID.String(), TraceID: uuid.NewString(), Status: "completed", RecordClass: capm.InvocationAuditClass, CompletedAt: &old}).Error)

		require.True(t, filepath.IsAbs(target))
		require.NoError(t, os.MkdirAll(target, 0700))
		credential := filepath.Join(target, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex database retention HTTP/PG fixture", "pid": os.Getpid(), "url": server.URL, "cwd": dir, "command": "go test ./internal/transport/http/admin/storage -run ^TestDatabaseRetentionControlNativeHTTPAndSSE$ -count=1 -v", "credential_file": credential})
		require.NoError(t, os.WriteFile(filepath.Join(target, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(5 * time.Minute):
			t.Fatal("owned browser timed out")
		}
		state, e := svc.DatabaseRetention(root)
		require.NoError(t, e)
		require.Equal(t, "applied", state.Configuration.State)
		require.GreaterOrEqual(t, state.Configuration.DesiredRevision, int64(3))
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
		for _, route := range [][2]string{{"GET", path}, {"PUT", path}, {"POST", path + "/adoptions"}, {"POST", path + "/changes"}, {"POST", path + "/runs"}, {"GET", path + "/runs/" + uuid.NewString()}, {"GET", path + "/requests/" + uuid.NewString()}} {
			code, _ := call(route[0], route[1], map[string]any{}, jwt)
			require.Contains(t, []int{401, 403}, code)
		}
	}
	code, raw := call("GET", path, nil, token)
	require.Equal(t, 200, code)
	var status gov.DatabaseRetentionStatus
	var config *gov.DatabaseRetentionConfiguration
	require.NoError(t, json.Unmarshal(raw, &status))
	config = status.Configuration
	require.Equal(t, "legacy", config.Authority)
	code, _ = call("PUT", path, map[string]any{"expected_revision": 1, "confirm": true, "policy": map[string]any{}}, token)
	require.Equal(t, 428, code)
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
	key := uuid.NewString()
	code, raw = call("POST", path+"/adoptions", gov.DatabaseRetentionAdoption{RequestID: key, LegacyDigest: config.LegacyDigest, Confirm: true}, token)
	require.Equal(t, 200, code)
	var mutation gov.DatabaseRetentionMutation
	require.NoError(t, json.Unmarshal(raw, &mutation))
	require.False(t, status.Policy.Enabled)
	wait := func(rev int64) {
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for config.AppliedRevision != rev {
			select {
			case <-frames:
				code, raw = call("GET", path, nil, token)
				require.Equal(t, 200, code)
				require.NoError(t, json.Unmarshal(raw, &status))
				config = status.Configuration
			case <-deadline.C:
				t.Fatal("configuration not confirmed via SSE")
			}
		}
	}
	wait(1)

	yes := true
	key = uuid.NewString()
	code, raw = call("POST", path+"/changes", gov.DatabaseRetentionChange{RequestID: key, ExpectedRevision: 1, Action: "status", Enabled: &yes, Confirm: true}, token)
	require.Equal(t, 200, code)
	wait(2)
	code, _ = call("GET", path+"/requests/"+key, nil, token)
	require.Equal(t, 200, code)
	code, _ = call("POST", path+"/changes", gov.DatabaseRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "status", Enabled: &yes, Confirm: true}, token)
	require.Equal(t, 409, code)
	tenant := &tenants.Tenant{Key: uuid.NewString(), Name: "owned http retention"}
	require.NoError(t, db.Create(tenant).Error)
	old := time.Now().UTC().AddDate(0, 0, -120)
	require.NoError(t, db.Create(&capm.InvocationTrace{TenantUUID: tenant.UUID.String(), TraceID: uuid.NewString(), Status: "completed", RecordClass: capm.InvocationAuditClass, CompletedAt: &old}).Error)
	require.NoError(t, db.Create(&capm.CapabilityEventPublication{TenantUUID: tenant.UUID.String(), TraceID: uuid.NewString(), Status: "pending", RecordClass: "legacy_unclassified", Topic: "business.order.created", Payload: datatypes.JSON(`{}`)}).Error)
	code, _ = call("POST", path+"/runs", map[string]any{"expected_revision": 2, "request_id": uuid.NewString(), "confirm": true}, token)
	require.Equal(t, 428, code)
	code, raw = call("POST", path+"/previews", map[string]any{"expected_revision": 2}, token)
	require.Equal(t, 200, code)
	var preview gov.DatabaseRetentionRun
	require.NoError(t, json.Unmarshal(raw, &preview))
	require.EqualValues(t, 1, preview.Counts.Traces)
	key = uuid.NewString()
	body := map[string]any{"expected_revision": 2, "request_id": key, "preview_id": preview.ID, "confirm": true}
	code, raw = call("POST", path+"/runs", body, token)
	require.Equal(t, 200, code)
	var completed gov.DatabaseRetentionRun
	require.NoError(t, json.Unmarshal(raw, &completed))
	require.EqualValues(t, 1, completed.Counts.Traces)
	code, raw = call("GET", path+"/runs/"+key, nil, token)
	require.Equal(t, 200, code)
	var receipt gov.DatabaseRetentionRun
	require.NoError(t, json.Unmarshal(raw, &receipt))
	require.Equal(t, completed.Counts, receipt.Counts)
	code, _ = call("GET", path+"/runs/"+uuid.NewString(), nil, token)
	require.Equal(t, 404, code)
	code, _ = call("POST", path+"/changes", map[string]any{"request_id": uuid.NewString(), "expected_revision": 2, "action": "status", "enabled": false, "confirm": true, "sql": "DELETE"}, token)
	require.Equal(t, 400, code)
	var n int64
	require.NoError(t, db.Model(&capm.InvocationTrace{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Model(&capm.CapabilityEventPublication{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
}
