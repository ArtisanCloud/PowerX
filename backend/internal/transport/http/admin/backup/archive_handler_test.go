package backup

import (
	"context"
	"encoding/json"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveHTTPAndBrowserFixture(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "photo.jpg"), []byte("test-media"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "thumbnail.jpg"), []byte("test-media"), 0600))
	cfg := archiveops.Config{Directory: t.TempDir(), Instance: "http-fixture", Sources: []archiveops.Source{{ID: "media", Name: "媒体文件", Kind: "media", Directory: source}}}
	svc := archiveops.NewService(ctx, cfg, archiveops.Options{})
	defer svc.Close()
	h := &handler{archives: svc}
	secret := []byte(uuid.NewString() + uuid.NewString())
	token, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: true, UserID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, "archive-test", []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "archive-test", []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	g := router.Group("/api/v1/admin/ops/backup/archives")
	g.GET("", h.ArchiveOverview)
	g.POST("/policies", h.SaveArchivePolicy)
	g.POST("/targets", h.SaveArchiveTarget)
	g.POST("/policies/:policy_id/run", h.RunArchive)
	g.GET("/jobs/:job_id", h.GetArchive)
	g.GET("/jobs/:job_id/download", h.DownloadArchive)
	g.POST("/jobs/:job_id/retry", h.RetryArchive)
	g.PATCH("/jobs/:job_id/protection", h.ProtectArchive)
	g.POST("/jobs/:job_id/restore", h.RestoreArchive)
	g.POST("/cleanup", h.CleanupArchives)
	g.GET("/restores/:restore_id", h.GetArchiveRestore)
	g.DELETE("/restores/:restore_id", h.DeleteArchiveRestore)
	server := httptest.NewServer(router)
	defer server.Close()
	fixture := os.Getenv("POWERX_ARCHIVE_BROWSER_FIXTURE")
	if fixture != "" {
		raw, _ := json.Marshal(map[string]any{"url": server.URL, "token": token, "pid": os.Getpid(), "owner": "Codex archive HTTP fixture", "source": source, "directory": cfg.Directory, "auth_boundary": "isolated signed Root test JWT; production login callback not exercised"})
		require.NoError(t, os.WriteFile(fixture, raw, 0600))
		t.Logf("owned archive fixture pid=%d listener=%s metadata=%s", os.Getpid(), server.URL, fixture)
		require.Eventually(t, func() bool { _, e := os.Stat(fixture + ".done"); return e == nil }, 15*time.Minute, 200*time.Millisecond)
		return
	}
	call := func(method, path, body string, headers map[string]string) (int, []byte) {
		req, e := http.NewRequest(method, server.URL+g.BasePath()+path, strings.NewReader(body))
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+token)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, e := server.Client().Do(req)
		require.NoError(t, e)
		defer resp.Body.Close()
		raw, e := io.ReadAll(resp.Body)
		require.NoError(t, e)
		return resp.StatusCode, raw
	}
	status, raw := call("POST", "/policies", `{"name":"media","source_id":"media","interval_minutes":60,"retention_days":7,"retention_count":168,"remote_retention_days":90,"remote_retention_count":2160,"include_database":false,"enabled":false}`, nil)
	require.Equal(t, 200, status, string(raw))
	var p struct {
		Data struct {
			Policy archiveops.Policy `json:"policy"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &p))
	status, raw = call("POST", "/policies/"+p.Data.Policy.ID+"/run", `{}`, map[string]string{"Idempotency-Key": uuid.NewString()})
	require.Equal(t, 202, status, string(raw))
	var started struct {
		Data struct {
			Job archiveops.Job `json:"job"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &started))
	root := reqctx.WithIsRoot(ctx, true)
	require.Eventually(t, func() bool { j, e := svc.Job(root, started.Data.Job.ID); return e == nil && j.Status != "running" }, 5*time.Second, 20*time.Millisecond)
	status, raw = call("GET", "/jobs/"+started.Data.Job.ID, "", nil)
	require.Equal(t, 200, status, string(raw))
	require.Contains(t, string(raw), "photo.jpg")
	require.NotContains(t, string(raw), "database_dsn")
	status, raw = call("PATCH", "/jobs/"+started.Data.Job.ID+"/protection", `{"protected":false}`, nil)
	require.Equal(t, 200, status, string(raw))
	status, raw = call("PATCH", "/jobs/"+started.Data.Job.ID+"/protection", `{}`, nil)
	require.Equal(t, 400, status, string(raw))
	status, raw = call("POST", "/targets", `{"name":"bad","host":"localhost;touch /tmp/injected","port":22,"user":"root","directory":"/data/archives","credential_ref":"x"}`, nil)
	require.Equal(t, 400, status, string(raw))
}
