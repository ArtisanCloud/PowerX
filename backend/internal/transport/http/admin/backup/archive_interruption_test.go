package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestArchiveInterruptionRootHTTPAndOwnedBrowser(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	source, state := filepath.Join(root, "source"), filepath.Join(root, "archive")
	require.NoError(t, os.Mkdir(source, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "photo.jpg"), []byte("owned partial content"), 0600))
	cfg := archive.Config{Directory: state, Instance: "owned-disposition", Sources: []archive.Source{{ID: "owned-media", Name: "自有中断采集", Kind: "media", Directory: source}}}
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 1, Scope: "access"})
	seed := archive.NewService(context.Background(), cfg, archive.Options{})
	p, e := seed.SavePolicy(ctx, archive.Policy{Name: "自有暂停策略", SourceID: "owned-media", IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 90, RemoteRetentionCount: 90})
	require.NoError(t, e)
	key := uuid.NewString()
	j, e := seed.Start(ctx, p.ID, key, "owned.root", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool { out, e := seed.Job(ctx, j.ID); return e == nil && out.Status == "success" }, 5*time.Second, 20*time.Millisecond)
	seed.Close()
	path := filepath.Join(state, "jobs", j.ID+".json")
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	var record map[string]any
	require.NoError(t, json.Unmarshal(raw, &record))
	record["status"] = "running"
	record["phase"] = "capturing"
	record["files"] = nil
	record["size_bytes"] = 0
	raw, e = json.Marshal(record)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	svc := archive.NewService(context.Background(), cfg, archive.Options{})
	t.Cleanup(svc.Close)
	maintenance := filepath.Join(root, "maintenance")
	require.NoError(t, os.Mkdir(maintenance, 0700))
	anchor, e := recovery.Initialize(maintenance, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, svc.BindRecoveryAnchor(anchor))
	// 自有未知请求：只改测试台账；不模拟已完成动作，不读日常库。
	unknown := archive.RetainInterruptedInput{RequestID: uuid.NewString(), ExpectedVersion: strings.Repeat("a", 64), Action: "retain_and_close", Reason: "现场无法确认原请求是否已执行", Confirm: true}
	hash := func(v any) string {
		b, e := json.Marshal(v)
		require.NoError(t, e)
		h := sha256.Sum256(b)
		return hex.EncodeToString(h[:])
	}
	epoch, e := anchor.Read(ctx)
	require.NoError(t, e)
	unknownDigest := hash([]any{j.ID, unknown})
	ledger := map[string]any{"request_id": unknown.RequestID, "job_id": j.ID, "state": "registered", "input_digest": unknownDigest, "expected_version": unknown.ExpectedVersion, "action": unknown.Action, "reason": unknown.Reason, "operator": 1, "created_at": time.Now().UTC(), "epoch": hash(epoch)}
	ledgerRaw, e := json.Marshal(ledger)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(state, "interruption-requests", unknown.RequestID+".json"), ledgerRaw, 0600))
	commandPath := filepath.Join(state, "command-executions", j.ID+".json")
	if os.Getenv("POWERX_ARCHIVE_COMMAND_DRAIN_UI") == "1" {
		b, e := os.ReadFile(commandPath)
		require.NoError(t, e)
		var command map[string]any
		require.NoError(t, json.Unmarshal(b, &command))
		command["state"] = "prepared"
		command["kind"] = "pg_dump"
		command["command_id"] = uuid.NewString()
		b, e = json.Marshal(command)
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(commandPath, b, 0600))
	}
	secret := []byte(uuid.NewString() + uuid.NewString())
	claims := reqctx.CoreXClaims{IsRoot: true, UserID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString(), Scope: "access"}
	token, e := auth.GenerateAccessJWT(claims, "owned-disposition", []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "owned-disposition", []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	h := &handler{archives: svc}
	finished := make(chan struct{}, 1)
	register := func(prefix string) {
		g := router.Group(prefix)
		g.GET("/archives", h.ArchiveOverview)
		g.GET("/events", h.BackupEvents)
		g.GET("/archives/catalog", h.ArchiveCatalog)
		g.GET("/archives/jobs/:job_id", h.GetArchive)
		g.GET("/archives/jobs/:job_id/interruption-review", h.ReviewArchiveInterruption)
		g.POST("/archives/jobs/:job_id/interruption-retentions", h.RetainArchiveInterruption)
		g.GET("/archives/interruption-requests/:request_id", h.ArchiveInterruptionRequest)
		g.GET("/archives/interruption-requests/:request_id/review", h.ReviewArchiveInterruptionRequest)
		g.POST("/archives/interruption-requests/:request_id/closures", h.CloseArchiveInterruptionRequest)
		if os.Getenv("POWERX_ARCHIVE_COMMAND_DRAIN_UI") == "1" {
			// 只属于本测试、尚未 fork 的 controlled prepared 夹具；不是产品解锁接口。
			g.POST("/__owned-fixture/drain", func(c *gin.Context) {
				b, e := os.ReadFile(commandPath)
				require.NoError(t, e)
				var command map[string]any
				require.NoError(t, json.Unmarshal(b, &command))
				require.Equal(t, "prepared", command["state"])
				command["state"] = "drained"
				b, e = json.Marshal(command)
				require.NoError(t, e)
				require.NoError(t, os.WriteFile(commandPath, b, 0600))
				c.JSON(200, gin.H{"ok": true})
			})
		}
		g.POST("/__owned-fixture/sync", func(c *gin.Context) {
			b, e := os.ReadFile(path)
			if e == nil {
				e = os.WriteFile(path, b, 0600)
			}
			require.NoError(t, e)
			c.JSON(200, gin.H{"ok": true})
		})
		g.POST("/__owned-fixture/finish", func(c *gin.Context) {
			select {
			case finished <- struct{}{}:
			default:
			}
			c.JSON(200, gin.H{"ok": true})
		})
	}
	register("/custom/v2/admin/ops/backup")
	register("/api/v1/admin/ops/backup")
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	t.Logf("owned disposition fixture pid=%d cwd=%s command=%q listener=%s", os.Getpid(), mustCwd(), os.Args, server.URL)
	if output := os.Getenv("POWERX_ARCHIVE_DISPOSITION_UI_FIXTURE"); output != "" {
		metadata := map[string]any{"owner": "codex archive disposition fixture", "url": server.URL, "token": token, "pid": os.Getpid(), "cwd": mustCwd(), "job_id": j.ID, "policy_id": p.ID, "directory": state, "source": source, "unknown_request_id": unknown.RequestID, "unknown_input_digest": unknownDigest, "unknown_expected_version": unknown.ExpectedVersion}
		b, e := json.Marshal(metadata)
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(output, b, 0600))
		select {
		case <-finished:
		case <-time.After(7 * time.Minute):
			t.Fatal("owned browser did not finish")
		}
		return
	}
	call := func(method, path string, body any, access string) (int, []byte) {
		b, _ := json.Marshal(body)
		req, e := http.NewRequest(method, server.URL+"/custom/v2/admin/ops/backup"+path, strings.NewReader(string(b)))
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set("Content-Type", "application/json")
		res, e := server.Client().Do(req)
		require.NoError(t, e)
		defer res.Body.Close()
		out, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		return res.StatusCode, out
	}
	for _, bad := range []reqctx.CoreXClaims{{IsRoot: false, UserID: 1}, {IsRoot: true, UserID: 0, PluginID: "com.test.plugin"}, {IsRoot: true, UserID: 1, Platforms: []string{"api_key"}}} {
		tok, e := auth.GenerateAccessJWT(bad, "owned-disposition", []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		status, _ := call("GET", "/archives/jobs/"+j.ID+"/interruption-review", nil, tok)
		require.NotEqual(t, 200, status)
	}
	for _, bad := range []reqctx.CoreXClaims{{IsRoot: false, UserID: 1}, {IsRoot: true, UserID: 0, PluginID: "com.test.plugin"}} {
		tok, e := auth.GenerateAccessJWT(bad, "owned-disposition", []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		for _, route := range []struct{ method, path string }{{"GET", "/review"}, {"POST", "/closures"}} {
			status, _ := call(route.method, "/archives/interruption-requests/"+unknown.RequestID+route.path, nil, tok)
			require.Contains(t, []int{400, 401, 403}, status)
		}
	}
	before, e := os.ReadFile(path)
	require.NoError(t, e)
	status, body := call("GET", "/archives/interruption-requests/"+unknown.RequestID+"/review", nil, token)
	require.Equal(t, 200, status, string(body))
	var rv struct {
		Data archive.InterruptionRequestReview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &rv))
	closeInput := archive.CloseInterruptionRequestInput{RequestID: uuid.NewString(), ExpectedVersion: rv.Data.ReviewVersion, Action: "close_unknown_keep_protection", Reason: "核对原请求后保留未知历史与任务保护", Confirm: true}
	status, body = call("POST", "/archives/interruption-requests/"+unknown.RequestID+"/closures", closeInput, token)
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), `"state":"closed_unknown"`)
	require.Contains(t, string(body), `"current_protection_matches":true`)
	status, body = call("GET", "/archives/interruption-requests/"+unknown.RequestID, nil, token)
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), closeInput.RequestID)
	status, body = call("POST", "/archives/interruption-requests/"+unknown.RequestID+"/closures", closeInput, token)
	require.Equal(t, 200, status, string(body))
	after, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, before, after)
	status, body = call("GET", "/archives/jobs/"+j.ID+"/interruption-review", nil, token)
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), "event: complete")
	var review archive.InterruptionReview
	for _, f := range strings.Split(string(body), "\n\n") {
		if strings.HasPrefix(f, "event: complete\n") {
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(f, "event: complete\ndata: ")), &review))
		}
	}
	in := archive.RetainInterruptedInput{RequestID: uuid.NewString(), ExpectedVersion: review.ReviewVersion, Action: "retain_and_close", Reason: "保留原内容 <只结束这一采集>", Confirm: true}
	status, body = call("POST", "/archives/jobs/"+j.ID+"/interruption-retentions", in, token)
	require.Equal(t, 200, status, string(body))
	status, body = call("GET", "/archives/interruption-requests/"+in.RequestID, nil, token)
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), `"state":"retained"`)
	require.Contains(t, string(body), `"current_record_matches":true`)
	status, body = call("POST", "/archives/jobs/"+j.ID+"/interruption-retentions", in, token)
	require.Equal(t, 200, status, string(body))
	require.NotContains(t, string(body), "retained_objects")
}
