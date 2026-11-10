package backup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 真正丢失接纳响应后，用签名 Root JWT 和原 UUID 只读找回，不再提交新任务。
func TestArchiveRunRequestHTTPAcceptedResponseLossAndRootBoundary(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "owned.jpg"), []byte("owned acceptance canary"), 0600))
	a := archive.NewService(context.Background(), archive.Config{Directory: t.TempDir(), Instance: "owned-request", Sources: []archive.Source{{ID: "media", Kind: "media", Directory: source}}}, archive.Options{})
	t.Cleanup(a.Close)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 11, Scope: "access"})
	policy, e := a.SavePolicy(ctx, archive.Policy{Name: "Owned request", SourceID: "media", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 30, RemoteRetentionCount: 30})
	require.NoError(t, e)
	secret := []byte(uuid.NewString() + uuid.NewString())
	issue := func(root bool, planes []string) string {
		token, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: root, UserID: 11, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString(), Platforms: planes}, "owned-run-request", []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		return token
	}
	token := issue(true, nil)
	h := &handler{archives: a}
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "owned-run-request", []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	g := router.Group("/custom/v2/admin/ops/backup/archives")
	// 在实际服务已经写入接纳任务后，关闭 HTTP socket 丢弃响应。
	g.POST("/policies/:policy_id/run", func(c *gin.Context) {
		h.RunArchive(c)
		if c.GetHeader("X-Owned-Lose-Reply") == "1" && c.Writer.Status() == 202 {
			conn, _, e := c.Writer.Hijack()
			if e == nil {
				_ = conn.Close()
			}
		}
	})
	g.GET("/policies/:policy_id/requests/:request_id", h.ArchiveRunRequest)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	cwd, e := os.Getwd()
	require.NoError(t, e)
	t.Logf("owned archive request HTTP pid=%d cwd=%s command=%q listener=%s", os.Getpid(), cwd, os.Args, server.URL)
	key := uuid.NewString()
	runPath := "/policies/" + policy.ID + "/run"
	lookup := "/policies/" + policy.ID + "/requests/" + key
	call := func(method, path, body, jwt string, lose bool) (int, []byte, error) {
		r, e := http.NewRequest(method, server.URL+g.BasePath()+path, strings.NewReader(body))
		require.NoError(t, e)
		if jwt != "" {
			r.Header.Set("Authorization", "Bearer "+jwt)
		}
		r.Header.Set("Idempotency-Key", key)
		if lose {
			r.Header.Set("X-Owned-Lose-Reply", "1")
		}
		resp, e := server.Client().Do(r)
		if e != nil {
			return 0, nil, e
		}
		defer resp.Body.Close()
		raw, e := io.ReadAll(resp.Body)
		return resp.StatusCode, raw, e
	}
	_, _, e = call("POST", runPath, `{"expected_policy_revision":`+strconv.FormatInt(policy.Revision, 10)+`}`, token, true)
	require.Error(t, e, "the browser acceptance response must really be lost")
	status, raw, e := call("GET", lookup, "", token, false)
	require.NoError(t, e)
	require.Equal(t, 200, status, string(raw))
	var out struct {
		Data archive.RunRequestReceipt `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, key, out.Data.RequestID)
	require.NotEmpty(t, out.Data.JobID)
	for _, candidate := range []struct {
		jwt    string
		status int
	}{{"", 401}, {issue(false, nil), 403}, {issue(true, []string{"api_key"}), 403}, {issue(true, []string{"service_actor"}), 403}} {
		status, _, e = call("GET", lookup, "", candidate.jwt, false)
		require.NoError(t, e)
		require.Equal(t, candidate.status, status)
	}
	for _, candidate := range []struct {
		path, body string
		status     int
	}{{lookup + "?retry=true", "", 400}, {lookup, `{}`, 400}, {"/policies/" + policy.ID + "/requests/invalid", "", 400}, {"/policies/" + policy.ID + "/requests/" + uuid.NewString(), "", 404}, {"/policies/" + uuid.NewString() + "/requests/" + key, "", 409}} {
		status, _, e = call("GET", candidate.path, candidate.body, token, false)
		require.NoError(t, e)
		require.Equal(t, candidate.status, status, candidate.path)
	}
	status, raw, e = call("POST", runPath, `{"expected_policy_revision":`+strconv.FormatInt(policy.Revision, 10)+`}`, token, false)
	require.NoError(t, e)
	require.Equal(t, 202, status, string(raw))
	var replay struct {
		Data struct {
			Job archive.Job `json:"job"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &replay))
	require.Equal(t, out.Data.JobID, replay.Data.Job.ID)
	for _, body := range []string{`{"expected_policy_revision":null}`, `{"expected_policy_revision":0}`, `{"expected_policy_revision":"1"}`, `{"expected_policy_revision":1.5}`} {
		status, _, e = call("POST", runPath, body, token, false)
		require.NoError(t, e)
		require.Equal(t, 400, status)
	}
	files, e := filepath.Glob(filepath.Join(aDirectory(t, a, ctx), "jobs", "*.json"))
	require.NoError(t, e)
	require.Len(t, files, 1)
}

func aDirectory(t *testing.T, a *archive.Service, ctx context.Context) string {
	t.Helper()
	view, e := a.Overview(ctx)
	require.NoError(t, e)
	return view.Directory
}
