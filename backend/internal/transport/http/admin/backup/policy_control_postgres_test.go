package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
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
func TestBackupPolicyNativeHTTPAndSSE(t *testing.T) {
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
	schema := "backup_policy_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		model.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, database.MigrateBackupPolicyControlModels(db))
	require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}))
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	base, e := event_bus.NewRedisStreamTaskDriver(redisClient, 10*time.Second, "owned-monitoring-http")
	require.NoError(t, e)
	queue := taskqueue.NewStreamDriver(db, base)
	dir := t.TempDir()
	minimum, critical := int64(1<<20), int64(1)
	if os.Getenv("POWERX_DB_BACKUP_BROWSER_DIR") != "" {
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
	svc, e = gov.NewService(gov.Options{Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", Environment: "test"}, FilesystemPath: dir, Collector: targetCollector{}, DB: db, Budget: coordinator, Control: control, ControlWorkerEnabled: true})
	require.NoError(t, e)
	policies, jobs := backupops.NewPolicyService(db), backupops.NewJobService(db)
	require.NoError(t, svc.BindDatabaseBackupControl(policies, jobs, "test"))
	ctx, cancel := context.WithCancel(context.Background())
	svc.StartControl(ctx, func(err error) { t.Logf("owned budget control worker: %v", err) })
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
	registerProtectedRoutes(protected, &shared.Deps{DB: db, StorageGovernance: svc, DatabaseBackupPolicies: policies, DatabaseBackupJobs: jobs})
	finish := make(chan struct{}, 1)
	if os.Getenv("POWERX_DB_BACKUP_BROWSER_DIR") != "" {
		protected.POST("/__owned-test/finish", RequireBackupRoot(), func(c *gin.Context) {
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

	root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	if target := os.Getenv("POWERX_DB_BACKUP_BROWSER_DIR"); target != "" {
		require.True(t, filepath.IsAbs(target))
		credential := filepath.Join(target, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex database backup HTTP fixture", "pid": os.Getpid(), "cwd": dir, "command": "go test ./internal/transport/http/admin/backup -run ^TestBackupPolicyNativeHTTPAndSSE$ -count=1 -v", "url": server.URL, "credential_file": credential})
		require.NoError(t, os.WriteFile(filepath.Join(target, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(5 * time.Minute):
			t.Fatal("owned browser timed out")
		}
		conf, err := policies.ControlConfiguration(root)
		require.NoError(t, err)
		require.Equal(t, "applied", conf.State)
		require.GreaterOrEqual(t, conf.DesiredRevision, int64(3))
		return
	}
	call := func(method, path string, input any, jwt string) (int, json.RawMessage) {
		var raw []byte
		if input != nil {
			raw, _ = json.Marshal(input)
		}
		req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if jwt != "" {
			req.Header.Set("Authorization", "Bearer "+jwt)
		} else {
			req.Header.Set("X-API-Key", "fixture-key")
		}
		res, err := server.Client().Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(body, &env)
		return res.StatusCode, env.Data
	}
	path := "/custom/v2/admin/ops/backup/policy-configuration"
	sign := func(claims reqctx.CoreXClaims) string {
		claims.TenantUUID = uuid.NewString()
		claims.MemberUUID = uuid.NewString()
		claims.MemberID = 1
		claims.TenantID = 1
		jwt, err := auth.GenerateAccessJWT(claims, issuer, []string{"admin"}, time.Hour, secret)
		require.NoError(t, err)
		return jwt
	}
	for _, jwt := range []string{"", sign(reqctx.CoreXClaims{UserID: 2}), sign(reqctx.CoreXClaims{UserID: 1, IsRoot: true, Platforms: []string{"service_actor"}})} {
		for _, ep := range []struct{ method, path string }{{"GET", path}, {"POST", path + "/changes"}, {"POST", path + "/adoptions"}, {"GET", path + "/requests/" + uuid.NewString()}} {
			status, _ := call(ep.method, ep.path, map[string]any{}, jwt)
			require.Contains(t, []int{401, 403}, status, ep.path)
		}
	}
	status, raw := call("GET", path, nil, token)
	require.Equal(t, 200, status)
	var conf backupops.PolicyConfiguration
	require.NoError(t, json.Unmarshal(raw, &conf))
	require.Equal(t, "legacy", conf.Authority)
	for _, input := range []map[string]any{{"confirm": true, "unexpected": true}, {"confirm": false}} {
		status, _ = call("POST", path+"/adoptions", input, token)
		require.Equal(t, 400, status)
	}
	// 真实 SSE 连接先收到初始失效通知，随后接收配置提交及发布事件。
	run, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	req, _ := http.NewRequestWithContext(run, "GET", server.URL+"/custom/v2/admin/ops/backup/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := server.Client().Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	lines := make(chan string, 32)
	go func() {
		scan := bufio.NewScanner(res.Body)
		for scan.Scan() {
			select {
			case lines <- scan.Text():
			case <-run.Done():
				return
			}
		}
	}()
	await := func() {
		for {
			select {
			case line := <-lines:
				if strings.HasPrefix(line, "data:") && strings.Contains(line, "backup_policies") {
					return
				}
			case <-run.Done():
				t.Fatal("missing policy SSE")
			}
		}
	}
	await()
	adoption := backupops.PolicyAdoption{RequestID: uuid.NewString(), LegacyDigest: conf.LegacyDigest, Confirm: true}
	status, raw = call("POST", path+"/adoptions", adoption, token)
	require.Equal(t, 200, status)
	await()
	require.Eventually(t, func() bool { x, e := policies.ControlConfiguration(root); return e == nil && x.State == "applied" }, 3*time.Second, 10*time.Millisecond)
	settings := backupops.PolicySettings{Name: "Owned HTTP policy", Schedule: "1h", RetentionCount: 3, RetentionDays: 7, RetentionMode: "count", Timezone: "Asia/Shanghai", DrillIntervalDays: 7, TargetRef: "local_dump"}
	in := backupops.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "save", Settings: &settings, Confirm: true}
	status, raw = call("POST", path+"/changes", in, token)
	require.Equal(t, 200, status)
	var mutation backupops.PolicyMutation
	require.NoError(t, json.Unmarshal(raw, &mutation))
	require.Equal(t, in.RequestID, mutation.PolicyUUID)
	status, _ = call("POST", path+"/changes", in, token)
	require.Equal(t, 200, status)
	status, _ = call("GET", path+"/requests/"+in.RequestID, nil, token)
	require.Equal(t, 200, status)
	in.RequestID = uuid.NewString()
	status, _ = call("POST", path+"/changes", in, token)
	require.Equal(t, 409, status)
	for _, ep := range []string{"/custom/v2/admin/ops/backup/policies", "/custom/v2/admin/backup/policies"} {
		status, _ = call("POST", ep, map[string]any{"name": "legacy", "interval_hours": 1, "retention_count": 3}, token)
		require.Equal(t, 428, status, fmt.Sprint(ep))
	}
}
