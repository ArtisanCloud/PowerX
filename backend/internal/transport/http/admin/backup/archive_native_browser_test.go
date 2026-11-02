package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

// 组合真实归档 HTTP、鉴权、SSE、生命周期删除与 MinIO，供实际 Nuxt 浏览器验收。
// Root JWT 是测试凭据；不加载日常配置、数据库或历史对象。
func TestArchiveNativeBrowserFixture(t *testing.T) {
	input, output := os.Getenv("POWERX_ARCHIVE_UI_NATIVE_CONFIG"), os.Getenv("POWERX_ARCHIVE_UI_NATIVE_FIXTURE")
	if input == "" || output == "" {
		t.Skip("opt-in owned native archive browser acceptance")
	}
	var cfg struct{ Endpoint, Bucket, Region, AccessKey, SecretKey string }
	raw, err := os.ReadFile(input)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &cfg))
	endpoint, err := url.Parse(cfg.Endpoint)
	require.NoError(t, err)
	require.Equal(t, "http", endpoint.Scheme)
	require.Equal(t, "127.0.0.1:19400", endpoint.Host, "only the owned isolated MinIO listener is accepted")
	require.Contains(t, cfg.Bucket, "powerx-archive-ui-")
	client, err := minio.New(cfg.Endpoint, &minio.Options{AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey, Region: cfg.Region, ForcePathStyle: true})
	require.NoError(t, err)
	versioned, err := client.BucketVersioningEnabled(context.Background(), cfg.Bucket)
	require.NoError(t, err)
	require.True(t, versioned, "runner must provision an owned private versioned bucket")
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	source := filepath.Join(root, "logs")
	require.NoError(t, os.Mkdir(source, 0700))
	content := []byte("owned native archive UI canary: source may be reclaimed, history must remain readable\n")
	for i, name := range []string{"app.log", "app.log.1", "app.log.2"} {
		p := filepath.Join(source, name)
		require.NoError(t, os.WriteFile(p, content, 0600))
		when := time.Now().AddDate(0, 0, -i*10)
		require.NoError(t, os.Chtimes(p, when, when))
	}
	state := filepath.Join(root, "archives")
	svc := archiveops.NewService(context.Background(), archiveops.Config{Directory: state, Instance: "owned-native-ui", ExecutionNodeID: "owned-native-ui", Sources: []archiveops.Source{{ID: "owned-logs", Name: "自有测试日志", Kind: "application_logs", Directory: source, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}}, archiveops.Options{})
	t.Cleanup(svc.Close)
	svc.StartScheduler()
	coordinator, err := budget.New(budget.Config{Directory: filepath.Join(root, "budget"), FilesystemPath: root, HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-native-ui", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	engine, err := lifecycle.New(lifecycle.Options{Coordinator: coordinator, InstanceID: "owned-native-ui", AllowEnforce: true, Sources: []lifecycle.Source{{ID: "owned-logs", Name: "自有测试日志", Kind: "application_logs", Scope: "dev", InstanceID: "owned-native-ui", Directory: source, Format: "numeric", ActiveFiles: []string{"app.log"}, OwnerUID: uint32(os.Geteuid()), RequireArchive: true}}, VerifyArchive: func(ctx context.Context, src lifecycle.Source, c lifecycle.Candidate) (bool, error) {
		return svc.VerifyLifecycleFile(ctx, src.Directory, c.Name, c.Bytes, time.Unix(0, c.ModifiedNS))
	}, ArchiveReady: func(ctx context.Context, src lifecycle.Source) (bool, error) {
		return svc.LifecycleArchiveReady(ctx, src.Directory)
	}})
	require.NoError(t, err)
	secret := []byte(uuid.NewString() + uuid.NewString())
	token, err := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: true, UserID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, "owned-archive-ui", []string{"admin"}, time.Hour, secret)
	require.NoError(t, err)
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "owned-archive-ui", []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	h := &handler{archives: svc}
	g := router.Group("/api/v1/admin/ops/backup")
	g.GET("/events", h.BackupEvents)
	a := g.Group("/archives")
	a.GET("", h.ArchiveOverview)
	a.GET("/catalog", h.ArchiveCatalog)
	a.POST("/s3-credentials", h.SaveArchiveS3Credential)
	a.POST("/targets", h.SaveArchiveTarget)
	a.POST("/targets/:target_id/test", h.TestArchiveTarget)
	a.POST("/policies", h.SaveArchivePolicy)
	a.POST("/policies/:policy_id/run", h.RunArchive)
	a.GET("/jobs/:job_id", h.GetArchive)
	a.GET("/jobs/:job_id/preview", h.PreviewArchiveLog)
	a.GET("/jobs/:job_id/download", h.DownloadArchive)
	a.POST("/__owned-fixture/reclaim", func(c *gin.Context) {
		_, e := engine.Save(c.Request.Context(), 1, "owned-logs", 0, lifecycle.Policy{Enabled: true, CleanupMode: "archive_then_delete", RetentionDays: 7, MaxBytes: 1 << 20, RequireArchive: true})
		if e != nil {
			c.JSON(500, gin.H{"problem": e.Error()})
			return
		}
		run, e := engine.RunWithKey(c.Request.Context(), "owned-logs", "native-browser-reclaim")
		if e != nil || run.DeletedFiles != 1 {
			c.JSON(500, gin.H{"problem": "owned source was not safely reclaimed", "run": run})
			return
		}
		view, e := svc.Overview(c.Request.Context())
		if e != nil {
			c.JSON(500, gin.H{"problem": "catalog unavailable"})
			return
		}
		removed := 0
		for _, j := range view.Jobs {
			detail, e := svc.Job(c.Request.Context(), j.ID)
			if e != nil {
				c.JSON(500, gin.H{"problem": "owned job detail unavailable"})
				return
			}
			for _, f := range detail.Files {
				p := filepath.Join(state, "objects", f.SHA256[:2], f.SHA256)
				if e := os.Remove(p); e == nil {
					removed++
				}
			}
		}
		_, e = os.Stat(filepath.Join(source, "app.log.2"))
		c.JSON(200, gin.H{"source_deleted": os.IsNotExist(e), "cas_removed": removed, "deleted_files": run.DeletedFiles})
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	sum := sha256.Sum256(content)
	metadata := map[string]interface{}{"url": server.URL, "token": token, "pid": os.Getpid(), "owner": "owned-native-archive-ui", "source": source, "directory": state, "expected_sha256": hex.EncodeToString(sum[:]), "expected_text": string(content), "auth_boundary": "isolated signed Root fixture JWT; daily login is not exercised"}
	raw, err = json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(output, raw, 0600))
	t.Logf("owned native archive fixture pid=%d listener=%s", os.Getpid(), server.URL)
	require.Eventually(t, func() bool { _, e := os.Stat(output + ".done"); return e == nil }, 15*time.Minute, 200*time.Millisecond)
}
