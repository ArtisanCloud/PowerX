package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestArchiveCommandConfirmationRootHTTPAndOwnedBrowser(t *testing.T) {
	dsn := os.Getenv("POWERX_ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("owned native source only")
	}
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 1, Scope: "access"})
	admin, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	defer admin.Close(ctx)
	name := "powerx_command_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, e)
	defer func() {
		_, e := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		require.NoError(t, e)
	}()
	u, e := url.Parse(dsn)
	require.NoError(t, e)
	u.Path = "/" + name
	db, e := pgx.Connect(ctx, u.String())
	require.NoError(t, e)
	defer db.Close(ctx)
	_, e = db.Exec(ctx, `CREATE TABLE media_assets(id int PRIMARY KEY,driver text,bucket text,storage_key text,size_bytes bigint,expected_checksum text,upload_state text,deleted_at timestamptz)`)
	require.NoError(t, e)
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	source, state := filepath.Join(root, "source"), filepath.Join(root, "archive")
	require.NoError(t, os.Mkdir(source, 0700))
	content := []byte("owned command confirmation media")
	sum := sha256.Sum256(content)
	require.NoError(t, os.WriteFile(filepath.Join(source, "photo.jpg"), content, 0600))
	_, e = db.Exec(ctx, "INSERT INTO media_assets VALUES(1,'local','','photo.jpg',$1,$2,'ready',NULL)", len(content), hex.EncodeToString(sum[:]))
	require.NoError(t, e)
	cfg := archive.Config{Directory: state, Instance: "owned-command-confirmation", Sources: []archive.Source{{ID: "owned-media", Name: "自有命令核对来源", Kind: "media", Directory: source, DatabaseDSN: u.String()}}}
	seed := archive.NewService(context.Background(), cfg, archive.Options{})
	p, e := seed.SavePolicy(ctx, archive.Policy{Name: "自有暂停策略", SourceID: "owned-media", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 90, RemoteRetentionCount: 90, IncludeDatabase: true})
	require.NoError(t, e)
	job, e := seed.Start(ctx, p.ID, uuid.NewString(), "owned", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		v, e := seed.Job(ctx, job.ID)
		return e == nil && v.Status == "success" && v.ExecutionState != "active"
	}, 10*time.Second, 20*time.Millisecond)
	seed.Close()
	// 真实 pg_dump 完成并退出后，受控回放丢失 final ack 的原记录；与 OS SIGKILL 测试分开。
	commandPath := filepath.Join(state, "command-executions", job.ID+".json")
	raw, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	var command map[string]any
	require.NoError(t, json.Unmarshal(raw, &command))
	require.Equal(t, "pg_dump", command["kind"])
	require.NotNil(t, command["identity"])
	command["state"] = "running"
	raw, e = json.Marshal(command)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(commandPath, raw, 0600))
	jobPath := filepath.Join(state, "jobs", job.ID+".json")
	b, e := os.ReadFile(jobPath)
	require.NoError(t, e)
	var record map[string]any
	require.NoError(t, json.Unmarshal(b, &record))
	record["status"] = "running"
	record["files"] = nil
	b, e = json.Marshal(record)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(jobPath, b, 0600))
	svc := archive.NewService(context.Background(), cfg, archive.Options{})
	defer svc.Close()
	maintenance := filepath.Join(root, "maintenance")
	require.NoError(t, os.Mkdir(maintenance, 0700))
	anchor, e := recovery.Initialize(maintenance, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, svc.BindRecoveryAnchor(anchor))
	secret := []byte(uuid.NewString() + uuid.NewString())
	claims := reqctx.CoreXClaims{IsRoot: true, UserID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString(), Scope: "access"}
	token, e := auth.GenerateAccessJWT(claims, "owned-command-confirmation", []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "owned-command-confirmation", []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	h := &handler{archives: svc}
	finished := make(chan struct{}, 1)
	register := func(prefix string) {
		g := router.Group(prefix)
		g.GET("/archives", h.ArchiveOverview)
		g.GET("/events", h.BackupEvents)
		g.GET("/archives/catalog", h.ArchiveCatalog)
		g.GET("/archives/jobs/:job_id", h.GetArchive)
		g.GET("/archives/commands/:record_id/review", h.ReviewArchiveCommand)
		g.POST("/archives/commands/:record_id/confirmations", h.ConfirmArchiveCommand)
		g.GET("/archives/command-confirmation-requests/:request_id", h.ArchiveCommandConfirmationRequest)
		g.POST("/__owned-fixture/finish", func(c *gin.Context) {
			select {
			case finished <- struct{}{}:
			default:
			}
			c.JSON(200, gin.H{"ok": true})
		})
	}
	register("/api/v1/admin/ops/backup")
	register("/custom/v2/admin/ops/backup")
	server := httptest.NewServer(router)
	defer server.Close()
	t.Logf("owned command confirmation fixture pid=%d cwd=%s command=%q listener=%s", os.Getpid(), mustCwd(), os.Args, server.URL)
	if output := os.Getenv("POWERX_ARCHIVE_COMMAND_CONFIRMATION_UI_FIXTURE"); output != "" {
		meta := map[string]any{"owner": "codex archive command confirmation fixture", "pid": os.Getpid(), "cwd": mustCwd(), "url": server.URL, "token": token, "directory": state, "source": source, "job_id": job.ID, "policy_id": p.ID}
		raw, e := json.Marshal(meta)
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(output, raw, 0600))
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
		raw, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		return res.StatusCode, raw
	}
	for _, bad := range []reqctx.CoreXClaims{{IsRoot: false, UserID: 1}, {IsRoot: true, UserID: 0, PluginID: "com.test.plugin"}, {IsRoot: true, UserID: 1, Platforms: []string{"api_key"}}} {
		tok, e := auth.GenerateAccessJWT(bad, "owned-command-confirmation", []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		for _, r := range []struct{ method, path string }{{"GET", "/archives/commands/" + job.ID + "/review"}, {"POST", "/archives/commands/" + job.ID + "/confirmations"}, {"GET", "/archives/command-confirmation-requests/" + uuid.NewString()}} {
			status, _ := call(r.method, r.path, nil, tok)
			require.Contains(t, []int{400, 401, 403}, status)
		}
	}
	commandBefore, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	jobBefore, e := os.ReadFile(jobPath)
	require.NoError(t, e)
	status, raw := call("GET", "/archives/commands/"+job.ID+"/review", nil, token)
	require.Equal(t, 200, status, string(raw))
	var review struct {
		Data archive.CommandDrainReview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &review))
	require.True(t, review.Data.Eligible)
	input := archive.ConfirmCommandDrainInput{RequestID: uuid.NewString(), ExpectedVersion: review.Data.ReviewVersion, ExecutionID: review.Data.ExecutionID, CommandID: review.Data.CommandID, Action: "confirm_local_command_drain", Reason: "核对原执行后明确记录本机排空证明", Confirm: true}
	status, raw = call("POST", "/archives/commands/"+job.ID+"/confirmations", input, token)
	require.Equal(t, 200, status, string(raw))
	require.Contains(t, string(raw), `"current_record_matches":true`)
	status, raw = call("GET", "/archives/command-confirmation-requests/"+input.RequestID, nil, token)
	require.Equal(t, 200, status, string(raw))
	require.Contains(t, string(raw), `"state":"confirmed"`)
	status, raw = call("POST", "/archives/commands/"+job.ID+"/confirmations", input, token)
	require.Equal(t, 200, status, string(raw))
	after, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	require.Equal(t, commandBefore, after)
	after, e = os.ReadFile(jobPath)
	require.NoError(t, e)
	require.Equal(t, jobBefore, after)
	input.Reason += " changed"
	status, _ = call("POST", "/archives/commands/"+job.ID+"/confirmations", input, token)
	require.Equal(t, 409, status)
}
