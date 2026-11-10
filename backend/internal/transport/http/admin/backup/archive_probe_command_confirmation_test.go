package backup

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

func TestArchiveProbeCommandConfirmationRootHTTPAndOwnedBrowser(t *testing.T) {
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 1, Scope: "access"})
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	state := filepath.Join(root, "archive")
	source := filepath.Join(root, "source")
	require.NoError(t, os.Mkdir(source, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "photo.jpg"), []byte("owned command confirmation media"), 0600))
	key := filepath.Join(root, "key")
	require.NoError(t, exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).Run())
	known := filepath.Join(root, "known_hosts")
	require.NoError(t, os.WriteFile(known, []byte{}, 0600))
	// 仅本机空闲监听口：真实 ssh 启动后连接拒绝并退出，绝不连接生产主机。
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	cfg := archive.Config{Directory: state, Instance: "owned-probe-confirmation", Credentials: map[string]archive.Credential{"owned": {KeyFile: key, KnownHostsFile: known}}}
	seed := archive.NewService(context.Background(), cfg, archive.Options{})
	target, e := seed.SaveTarget(ctx, archive.Target{Name: "自有原探测目标", Host: "127.0.0.1", Port: port, User: "archive", Directory: filepath.Join(root, "remote"), CredentialRef: "owned"})
	require.NoError(t, e)
	require.Error(t, seed.TestTarget(ctx, target.ID))
	seed.Close()
	recordID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("archive-command-probe/"+target.ID)).String()
	commandPath := filepath.Join(state, "command-executions", recordID+".json")
	raw, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	var command map[string]any
	require.NoError(t, json.Unmarshal(raw, &command))
	require.Equal(t, "ssh", command["kind"])
	require.NotNil(t, command["identity"])
	command["state"] = "running"
	raw, e = json.Marshal(command)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(commandPath, raw, 0600))
	targetPath := filepath.Join(state, "targets", target.ID+".json")
	if os.Getenv("POWERX_STORAGE_CORE_RELEASE_UI") == "1" {
		cfg.RemoteExecutionBlocked = true
	}
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
	if output := os.Getenv("POWERX_ARCHIVE_PROBE_CONFIRMATION_UI_FIXTURE"); output != "" {
		meta := map[string]any{"owner": "codex archive probe confirmation fixture", "pid": os.Getpid(), "cwd": mustCwd(), "url": server.URL, "token": token, "directory": state, "source": source, "record_id": recordID, "target_id": target.ID}
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
		for _, r := range []struct{ method, path string }{{"GET", "/archives/commands/" + recordID + "/review"}, {"POST", "/archives/commands/" + recordID + "/confirmations"}, {"GET", "/archives/command-confirmation-requests/" + uuid.NewString()}} {
			status, _ := call(r.method, r.path, nil, tok)
			require.Contains(t, []int{400, 401, 403}, status)
		}
	}
	commandBefore, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	targetBefore, e := os.ReadFile(targetPath)
	require.NoError(t, e)
	status, raw := call("GET", "/archives/commands/"+recordID+"/review", nil, token)
	require.Equal(t, 200, status, string(raw))
	var review struct {
		Data archive.CommandDrainReview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &review))
	require.True(t, review.Data.Eligible)
	input := archive.ConfirmCommandDrainInput{RequestID: uuid.NewString(), ExpectedVersion: review.Data.ReviewVersion, ExecutionID: review.Data.ExecutionID, CommandID: review.Data.CommandID, Action: "confirm_local_command_drain", Reason: "核对原执行后明确记录本机排空证明", Confirm: true, TargetID: target.ID}
	status, raw = call("POST", "/archives/commands/"+recordID+"/confirmations", input, token)
	require.Equal(t, 200, status, string(raw))
	require.Contains(t, string(raw), `"current_record_matches":true`)
	status, raw = call("GET", "/archives/command-confirmation-requests/"+input.RequestID, nil, token)
	require.Equal(t, 200, status, string(raw))
	require.Contains(t, string(raw), `"state":"confirmed"`)
	status, raw = call("POST", "/archives/commands/"+recordID+"/confirmations", input, token)
	require.Equal(t, 200, status, string(raw))
	after, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	require.Equal(t, commandBefore, after)
	after, e = os.ReadFile(targetPath)
	require.NoError(t, e)
	require.Equal(t, targetBefore, after)
	input.Reason += " changed"
	status, _ = call("POST", "/archives/commands/"+recordID+"/confirmations", input, token)
	require.Equal(t, 409, status)
}
