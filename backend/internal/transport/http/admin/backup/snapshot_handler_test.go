package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type snapshotHTTPBackend struct{}

func (snapshotHTTPBackend) Capture(ctx context.Context, dir string) (string, error) {
	inventory := []map[string]any{}
	for name, raw := range map[string]string{"database/database.dump": "owned fixture", "media/image.jpg": "A"} {
		p := filepath.Join(dir, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return "", e
		}
		if e := os.WriteFile(p, []byte(raw), 0600); e != nil {
			return "", e
		}
		inventory = append(inventory, map[string]any{"path": name, "size": len(raw), "sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))})
	}
	if e := os.MkdirAll(filepath.Join(dir, "metadata"), 0700); e != nil {
		return "", e
	}
	raw, _ := json.Marshal(inventory)
	if e := os.WriteFile(filepath.Join(dir, "metadata", "media-inventory.json"), raw, 0600); e != nil {
		return "", e
	}
	return "fixture-mvcc", nil
}
func (snapshotHTTPBackend) RestoreDatabase(context.Context, string) (string, string, error) {
	return "copy", "owned_copy", nil
}
func snapshotHTTPFixture(t *testing.T) (*snapshotops.Service, *gin.Engine, string, string) {
	t.Helper()
	artifact := filepath.Join(t.TempDir(), "powerx")
	require.NoError(t, os.WriteFile(artifact, []byte("fixture executable"), 0700))
	svc := snapshotops.NewService(context.Background(), snapshotops.Config{Directory: t.TempDir(), Instance: "http-fixture", Version: "v1", EffectiveConfig: []byte("fixture: private-value"), Artifacts: map[string]string{"core/powerx": artifact}}, snapshotops.Options{Backend: snapshotHTTPBackend{}})
	t.Cleanup(svc.Close)
	secret := []byte(uuid.NewString() + uuid.NewString())
	root, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: true, UserID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, "snapshot-test", []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	member, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: false, UserID: 2, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, "snapshot-test", []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	r := gin.New()
	r.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "snapshot-test", []string{"admin"}, []string{"access"}, nil))
	h := &handler{snapshots: svc}
	g := r.Group("/custom/v2/admin/ops/backup/system-snapshots", RequireRestoreRoot())
	g.GET("", h.SnapshotOverview)
	g.POST("", h.CreateSnapshot)
	g.POST("/imports", h.ImportSnapshot)
	g.GET("/policies", h.ListSnapshotPolicies)
	g.POST("/policies", h.PutSnapshotPolicy)
	g.PUT("/policies/:policy_id", h.PutSnapshotPolicy)
	g.PATCH("/policies/:policy_id/status", h.SetSnapshotPolicyStatus)
	g.POST("/:snapshot_id/export", h.ExportSnapshot)
	g.GET("/:snapshot_id", h.GetSnapshot)
	g.DELETE("/:snapshot_id", h.DeleteSnapshot)
	g.POST("/:snapshot_id/restores", h.RestoreSnapshot)
	return svc, r, root, member
}

func TestSnapshotDeleteHTTPRemovesOwnedFilesAndEnforcesRootConfirmation(t *testing.T) {
	svc, router, root, member := snapshotHTTPFixture(t)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true}), true)
	point, err := svc.Start(ctx, uuid.NewString())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		point, err = svc.Get(ctx, point.ID)
		return err == nil && point.Status == "ready"
	}, 10*time.Second, 10*time.Millisecond)
	view, err := svc.Overview(ctx)
	require.NoError(t, err)
	path := "/custom/v2/admin/ops/backup/system-snapshots/" + point.ID
	call := func(token, confirmation, protected, query string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("DELETE", path+query, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		request.Header.Set("X-Snapshot-Confirmation", confirmation)
		request.Header.Set("X-Snapshot-Delete-Protected", protected)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	require.Equal(t, 401, call("", point.ID, "true", "").Code)
	require.Equal(t, 403, call(member, point.ID, "true", "").Code)
	require.Equal(t, 400, call(root, "wrong", "true", "").Code)
	require.Equal(t, 400, call(root, point.ID, "force", "").Code)
	require.Equal(t, 400, call(root, point.ID, "true", "?path=/outside").Code)
	require.Eventually(t, func() bool {
		_, err = svc.Protect(ctx, point.ID, true)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond, "采集完成后的归档收尾必须释放操作锁")
	require.Equal(t, 409, call(root, point.ID, "false", "").Code)
	require.DirExists(t, filepath.Join(view.Directory, "contents", point.ID))
	response := call(root, point.ID, "true", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"record_deleted":true`)
	require.Contains(t, response.Body.String(), `"files_deleted":true`)
	require.NoDirExists(t, filepath.Join(view.Directory, "contents", point.ID))
	require.NoFileExists(t, filepath.Join(view.Directory, "snapshots", point.ID+".json"))
	require.NoFileExists(t, filepath.Join(view.Directory, "requests", point.ID+".json"))
	require.Equal(t, 404, call(root, point.ID, "true", "").Code)
}
func TestSystemSnapshotHTTPRootBoundaryAndStrictBody(t *testing.T) {
	svc, r, root, member := snapshotHTTPFixture(t)
	base := "/custom/v2/admin/ops/backup/system-snapshots"
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Idempotency-Key", uuid.NewString())
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		return out
	}
	require.Equal(t, 401, call("GET", base, "", "").Code)
	require.Equal(t, 403, call("GET", base, member, "").Code)
	ok := call("GET", base, root, "")
	require.Equal(t, 200, ok.Code)
	require.NotContains(t, ok.Body.String(), "private-value")
	require.NotContains(t, ok.Body.String(), "master.key")
	require.Equal(t, 400, call("POST", base, root, `{"shell":"anything"}`).Code)
	require.Equal(t, 400, call("POST", base, root, `{}`).Code)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true}), true)
	p, err := svc.PutPolicy(ctx, "", "Daily", snapshotops.Policy{Enabled: true, IntervalMinutes: 1440, RetentionDays: 7, RetentionCount: 7})
	require.NoError(t, err)
	created := call("POST", base, root, fmt.Sprintf(`{"policy_id":%q}`, p.ID))
	require.Equal(t, 202, created.Code)
	var response struct {
		Data struct {
			Snapshot snapshotops.Snapshot `json:"snapshot"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &response))
	require.NotEmpty(t, response.Data.Snapshot.ID)
	wrong := call("POST", base+"/"+response.Data.Snapshot.ID+"/restores", root, `{"confirmation":"wrong"}`)
	require.Equal(t, 400, wrong.Code)
	tooLarge := httptest.NewRequest("POST", base+"/imports", nil)
	tooLarge.Header.Set("Authorization", "Bearer "+root)
	tooLarge.ContentLength = 9 * 1024 * 1024 * 1024
	limited := httptest.NewRecorder()
	r.ServeHTTP(limited, tooLarge)
	require.Equal(t, 413, limited.Code)
}
func TestSystemSnapshotHTTPCompleteEncryptedExportAndImport(t *testing.T) {
	svc, r, token, _ := snapshotHTTPFixture(t)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true}), true)
	j, e := svc.Start(ctx, uuid.NewString())
	require.NoError(t, e)
	require.Eventually(t, func() bool { j, e = svc.Get(ctx, j.ID); return e == nil && j.Status == "ready" }, 10*time.Second, 10*time.Millisecond)
	base := "/custom/v2/admin/ops/backup/system-snapshots"
	var exported *httptest.ResponseRecorder
	require.Eventually(t, func() bool {
		req := httptest.NewRequest("POST", base+"/"+j.ID+"/export", bytes.NewBufferString(`{"password":"http-independent-passphrase"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		exported = httptest.NewRecorder()
		r.ServeHTTP(exported, req)
		return exported.Code != 409
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, 200, exported.Code)
	require.Equal(t, "no-store", exported.Header().Get("Cache-Control"))
	require.NotContains(t, exported.Body.String(), "private-value")
	_, target, targetToken, _ := snapshotHTTPFixture(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, e := form.CreateFormFile("file", "snapshot.pxsnapshot")
	require.NoError(t, e)
	_, e = file.Write(exported.Body.Bytes())
	require.NoError(t, e)
	require.NoError(t, form.WriteField("password", "http-independent-passphrase"))
	require.NoError(t, form.Close())
	req := httptest.NewRequest("POST", base+"/imports", &body)
	req.Header.Set("Authorization", "Bearer "+targetToken)
	req.Header.Set("Content-Type", form.FormDataContentType())
	imported := httptest.NewRecorder()
	target.ServeHTTP(imported, req)
	require.Equal(t, 200, imported.Code, imported.Body.String())
	require.Contains(t, imported.Body.String(), j.ID)
	require.NotContains(t, imported.Body.String(), "private-value")
	require.NotContains(t, imported.Body.String(), "http-independent-passphrase")
}

func TestSnapshotOverviewExposesEffectiveDirectoryOnlyToRoot(t *testing.T) {
	_, router, root, member := snapshotHTTPFixture(t)
	for _, test := range []struct {
		token  string
		status int
	}{{root, 200}, {member, 403}} {
		request := httptest.NewRequest("GET", "/custom/v2/admin/ops/backup/system-snapshots", nil)
		request.Header.Set("Authorization", "Bearer "+test.token)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, request)
		require.Equal(t, test.status, out.Code)
		if test.status == 200 {
			var reply struct {
				Data struct {
					Overview snapshotops.Overview `json:"overview"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(out.Body.Bytes(), &reply))
			require.True(t, filepath.IsAbs(reply.Data.Overview.Directory))
			require.Equal(t, "explicit", reply.Data.Overview.DirectorySource)
		} else {
			require.NotContains(t, out.Body.String(), "directory_source")
			require.NotContains(t, out.Body.String(), "master.key")
		}
	}
}
