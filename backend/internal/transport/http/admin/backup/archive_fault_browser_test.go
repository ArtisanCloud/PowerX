package backup

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/internal/testutil/archivefault"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 实际归档服务、版本协议和 SSE 的自有浏览器夹具；控制库故障另由原生测试验证。
// 不加载日常配置，不提供生产写路由，不将协议通过当成 OSS 验收。
func TestArchiveAcceptedFaultBrowserFixture(t *testing.T) {
	output := os.Getenv("POWERX_ARCHIVE_FAULT_UI_FIXTURE")
	restartUI := false
	if path := os.Getenv("POWERX_ARCHIVE_RESTART_UI_FIXTURE"); path != "" {
		output = path
		restartUI = true
	}
	if output == "" {
		t.Skip("opt-in owned archive fault browser acceptance")
	}
	peer := archivefault.New(t)
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	source, state := filepath.Join(root, "logs"), filepath.Join(root, "archives")
	require.NoError(t, os.Mkdir(source, 0700))
	for i, name := range []string{"app.log", "app.log.1", "app.log.2"} {
		path := filepath.Join(source, name)
		require.NoError(t, os.WriteFile(path, []byte("owned UI failure canary "+name+"\n"), 0600))
		at := time.Now().AddDate(0, 0, -10-i)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	cfg := archive.Config{Directory: state, Instance: "owned-fault-ui", ExecutionNodeID: "owned-fault-ui", Sources: []archive.Source{{ID: "owned-logs", Name: "自有故障测试日志", Kind: "application_logs", Directory: source, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}, S3Credentials: map[string]archive.S3Credential{"owned": {AccessKey: "fixture", SecretKey: "fixture-secret"}}}
	svc := archive.NewService(context.Background(), cfg, archive.Options{})
	t.Cleanup(func() { svc.Close() })
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 1})
	target, e := svc.SaveTarget(ctx, archive.Target{Name: "Owned protocol", Driver: "s3", Endpoint: peer.URL, Bucket: "archives", Prefix: "owned/ui", Region: "us-east-1", CredentialRef: "owned", ForcePathStyle: true})
	require.NoError(t, e)
	require.NoError(t, svc.TestTarget(ctx, target.ID))
	policy, e := svc.SavePolicy(ctx, archive.Policy{Name: "自有故障策略", SourceID: "owned-logs", TargetID: target.ID, Enabled: true, IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 10, RemoteRetentionDays: 30, RemoteRetentionCount: 100, RemoteMaxBytes: 16 << 20})
	require.NoError(t, e)
	peer.Arm("object-ack", nil)
	accepted, e := svc.Start(ctx, policy.ID, uuid.NewString(), "owned-test", "owned-ui-fault")
	require.NoError(t, e)
	require.Eventually(t, func() bool { j, e := svc.Job(ctx, accepted.ID); return e == nil && j.RemoteState == "failed" }, 10*time.Second, 10*time.Millisecond)
	_, _, fired, hookError := peer.Snapshot()
	require.True(t, fired, "the browser must observe a real accepted-write fault")
	require.NoError(t, hookError)
	if restartUI {
		// UI 夹具单独验证 Service 重建后的原生 HTTP/SSE；真实 SIGKILL 由 process_restart_test 验收。
		svc.Close()
		path := filepath.Join(state, "jobs", accepted.ID+".json")
		raw, e := os.ReadFile(path)
		require.NoError(t, e)
		var record map[string]any
		require.NoError(t, json.Unmarshal(raw, &record))
		record["remote_state"] = "pending"
		record["phase"] = "uploading"
		record["remote_error"] = ""
		raw, e = json.Marshal(record)
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(path, raw, 0600))
		svc = archive.NewService(context.Background(), cfg, archive.Options{})
		svc.StartScheduler()
		j, e := svc.Job(ctx, accepted.ID)
		require.NoError(t, e)
		require.True(t, j.InterruptionPending)
		require.Equal(t, "interrupted", j.ExecutionState)
	}
	secret := []byte(uuid.NewString() + uuid.NewString())
	token, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: true, UserID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, "owned-archive-fault-ui", []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "owned-archive-fault-ui", []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	h := &handler{archives: svc}
	g := router.Group("/api/v1/admin/ops/backup")
	g.GET("/events", h.BackupEvents)
	g.GET("/archives", h.ArchiveOverview)
	g.GET("/archives/catalog", h.ArchiveCatalog)
	g.GET("/archives/jobs/:job_id", h.GetArchive)
	if restartUI {
		g.POST("/archives/jobs/:job_id/retry", h.RetryArchive)
	}
	finished := make(chan struct{}, 1)
	g.POST("/__owned-fixture/recover", func(c *gin.Context) {
		peer.Arm("", nil)
		job, e := svc.Retry(c.Request.Context(), accepted.ID)
		if e != nil {
			c.JSON(500, gin.H{"problem": "owned retry failed"})
			return
		}
		c.JSON(200, gin.H{"job_id": job.ID})
	})
	g.POST("/__owned-fixture/sync", func(c *gin.Context) {
		// A real fsnotify change drives the production SSE metadata invalidation.
		path := filepath.Join(state, "jobs", accepted.ID+".json")
		raw, e := os.ReadFile(path)
		if e == nil {
			e = os.WriteFile(path, raw, 0600)
		}
		if e != nil {
			c.JSON(500, gin.H{"problem": "owned sync failed"})
			return
		}
		c.JSON(200, gin.H{"ok": true})
	})
	g.POST("/__owned-fixture/finish", func(c *gin.Context) {
		select {
		case finished <- struct{}{}:
		default:
		}
		c.JSON(200, gin.H{"ok": true})
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	metadata := map[string]any{"owner": "codex archive fault UI fixture", "url": server.URL, "s3_url": peer.URL, "pid": os.Getpid(), "cwd": mustCwd(), "token": token, "job_id": accepted.ID, "directory": state, "source": source, "auth_boundary": "isolated signed Root JWT; daily identity and control database are not exercised"}
	raw, e := json.Marshal(metadata)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(output, raw, 0600))
	t.Logf("owned archive fault UI pid=%d http=%s s3=%s", os.Getpid(), server.URL, peer.URL)
	select {
	case <-finished:
	case <-time.After(8 * time.Minute):
		t.Fatal("owned browser did not finish")
	}
}
