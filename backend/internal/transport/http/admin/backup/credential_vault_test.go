package backup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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

	vault "github.com/ArtisanCloud/PowerX/internal/infra/archive_credentials"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestArchiveCredentialNativeHTTPAndSSE(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	dir := filepath.Join(root, "independent-vault")
	require.NoError(t, os.Mkdir(dir, 0700))
	key := make([]byte, 32)
	_, e = rand.Read(key)
	require.NoError(t, e)
	cfg := vault.Config{Directory: dir, KeyringFile: filepath.Join(root, "external-keyring.json"), StoreID: "owned-credential-vault"}
	ring := vault.Keyring{Version: 1, StoreID: cfg.StoreID, ActiveKeyID: "initial-key", Keys: map[string]string{"initial-key": base64.StdEncoding.EncodeToString(key)}}
	raw, e := json.Marshal(ring)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(cfg.KeyringFile, raw, 0600))
	svc := archiveops.NewService(context.Background(), archiveops.Config{Directory: filepath.Join(root, "metadata"), Instance: "owned", ExecutionNodeID: "owned-credentials", CredentialVault: &cfg, S3Credentials: map[string]archiveops.S3Credential{"legacy-test": {Revision: 7, AccessKey: "owned-old-access", SecretKey: "owned-old-secret"}}}, archiveops.Options{})
	t.Cleanup(svc.Close)
	secret := []byte("isolated-credential-http-test-no-deployment-key")
	issuer := "owned-credential-http"
	claims := reqctx.CoreXClaims{UserID: 11, IsRoot: true, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}
	token, e := auth.GenerateAccessJWT(claims, issuer, []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	router := gin.New()
	g := router.Group("/custom/v2/admin/ops/backup", authmw.JwtMiddleware(secret, issuer, []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	h := &handler{archives: svc}
	a := g.Group("/archives")
	a.GET("", h.ArchiveOverview)
	a.GET("/catalog", h.ArchiveCatalog)
	g.GET("/events", h.BackupEvents)
	a.GET("/s3-credentials", h.ArchiveCredentialOverview)
	a.POST("/s3-credentials", h.SaveArchiveS3Credential)
	a.GET("/s3-credentials/requests/:request_id", h.ArchiveCredentialRequest)
	a.POST("/s3-credentials/:credential_ref/adoptions", h.AdoptArchiveCredential)
	a.POST("/s3-credentials/:credential_ref/revocations", h.RevokeArchiveCredential)
	a.POST("/credential-store/rotations", h.RotateArchiveCredentialKey)
	finish := make(chan struct{}, 1)
	a.POST("/__owned-test/advance-keyring", func(c *gin.Context) {
		next := make([]byte, 32)
		if _, err := rand.Read(next); err != nil {
			c.Status(500)
			return
		}
		ring.Keys["rotated-key"] = base64.StdEncoding.EncodeToString(next)
		ring.ActiveKeyID = "rotated-key"
		b, err := json.Marshal(ring)
		if err != nil || os.WriteFile(cfg.KeyringFile, b, 0600) != nil {
			c.Status(500)
			return
		}
		c.Status(204)
	})
	a.POST("/__owned-test/finish", func(c *gin.Context) {
		select {
		case finish <- struct{}{}:
		default:
		}
		c.Status(204)
	})
	server := httptest.NewServer(router)
	address := strings.TrimPrefix(server.URL, "http://")
	t.Logf("owned credential HTTP: pid=%d cwd=%s command=go-test listener=%s", os.Getpid(), root, address)
	t.Cleanup(func() {
		server.Close()
		c, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			c.Close()
			t.Error("owned HTTP listener remains open")
		}
	})
	if target := os.Getenv("POWERX_CREDENTIAL_BROWSER_DIR"); target != "" {
		require.True(t, filepath.IsAbs(target))
		credential := filepath.Join(target, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex credential browser fixture", "pid": os.Getpid(), "url": server.URL, "cwd": root, "command": "go test TestArchiveCredentialNativeHTTPAndSSE", "credential_file": credential})
		require.NoError(t, os.WriteFile(filepath.Join(target, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(5 * time.Minute):
			t.Fatal("owned browser timed out")
		}
		ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 11, IsRoot: true, Scope: "access"})
		out, e := svc.CredentialOverview(ctx)
		require.NoError(t, e)
		require.True(t, out.Store.Ready)
		require.Len(t, out.Items, 2)
		require.Equal(t, "rotated-key", out.Store.KeyID)
		delete(ring.Keys, "initial-key")
		b, err := json.Marshal(ring)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cfg.KeyringFile, b, 0600))
		after, err := svc.CredentialOverview(ctx)
		require.NoError(t, err)
		require.True(t, after.Store.Ready)
		require.NoFileExists(t, filepath.Join(root, "metadata", "secrets", "browser-new.json"))
		for _, r := range out.Items {
			require.Equal(t, "sealed", r.Origin)
			if r.Ref == "browser-new" {
				require.Equal(t, "revoked", r.State)
			}
		}
		return
	}
	base := "/custom/v2/admin/ops/backup/archives/s3-credentials"
	call := func(method, path string, input any, want int, credential string) json.RawMessage {
		var b []byte
		if input != nil {
			b, e = json.Marshal(input)
			require.NoError(t, e)
		}
		req, e := http.NewRequest(method, server.URL+path, bytes.NewReader(b))
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+credential)
		req.Header.Set("Content-Type", "application/json")
		response, e := server.Client().Do(req)
		require.NoError(t, e)
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		require.NoError(t, e)
		require.Equal(t, want, response.StatusCode, string(raw))
		require.NotContains(t, string(raw), "owned-old-secret")
		var out struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &out))
		return out.Data
	}
	for _, c := range []reqctx.CoreXClaims{{UserID: 11}, {IsRoot: true}, {UserID: 11, IsRoot: true, Platforms: []string{"api_key"}}} {
		c.TenantUUID = uuid.NewString()
		c.MemberUUID = uuid.NewString()
		bad, e := auth.GenerateAccessJWT(c, issuer, []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		want := 403
		if c.UserID == 0 {
			want = 401
		}
		call("GET", base, nil, want, bad)
		call("POST", base, map[string]any{}, want, bad)
	}
	var view archiveops.CredentialOverview
	require.NoError(t, json.Unmarshal(call("GET", base, nil, 200, token), &view))
	require.True(t, view.Store.Ready)
	require.Len(t, view.Items, 1)
	old := view.Items[0]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", server.URL+"/custom/v2/admin/ops/backup/events", nil)
	require.NoError(t, e)
	req.Header.Set("Authorization", "Bearer "+token)
	response, e := server.Client().Do(req)
	require.NoError(t, e)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	frames := make(chan string, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "event:") {
				select {
				case frames <- scanner.Text():
				default:
				}
			}
		}
	}()
	defer func() { cancel(); response.Body.Close(); <-done }()
	select {
	case <-frames:
	case <-time.After(3 * time.Second):
		t.Fatal("no SSE snapshot")
	}
	request := uuid.NewString()
	body := map[string]any{"request_id": request, "expected_revision": old.Revision, "legacy_digest": old.LegacyDigest, "confirm": true}
	var saved archiveops.CredentialMutation
	require.NoError(t, json.Unmarshal(call("POST", base+"/legacy-test/adoptions", body, 200, token), &saved))
	require.EqualValues(t, 1, saved.Receipt.Revision)
	call("GET", base+"/requests/"+request, nil, 200, token)
	select {
	case <-frames:
	case <-time.After(3 * time.Second):
		t.Fatal("no credential SSE invalidation")
	}
	call("POST", base+"/legacy-test/adoptions", body, 200, token)
	call("POST", base, map[string]any{"request_id": uuid.NewString(), "ref": "new-private", "expected_revision": 0, "access_key": "owned-new-access", "secret_key": "owned-new-secret"}, 200, token)
	call("POST", base+"/new-private/revocations", map[string]any{"request_id": uuid.NewString(), "expected_revision": 1, "confirm": true}, 200, token)
	call("POST", base, map[string]any{"ref": "bad", "access_key": "bad", "secret_key": "bad"}, 400, token)
	call("POST", base+"/new-private/revocations", map[string]any{"request_id": uuid.NewString(), "expected_revision": 1, "confirm": true}, 409, token)
	call("GET", base+"/requests/"+uuid.NewString(), nil, 404, token)
}
