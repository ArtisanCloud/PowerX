package storage

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	storageobserver "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

type testCollector struct{}

func (testCollector) Collect(context.Context, string, []storagegov.Resource) storagegov.Snapshot {
	return storagegov.Snapshot{Filesystem: storagegov.Filesystem{State: "unavailable", View: "process_filesystem"}}
}

func request(t *testing.T, claims reqctx.CoreXClaims, method, path, body string, services ...*storagegov.Service) *httptest.ResponseRecorder {
	t.Helper()
	claims.TenantUUID = "00000000-0000-4000-8000-000000000001"
	claims.MemberUUID = "00000000-0000-4000-8000-000000000002"
	gin.SetMode(gin.TestMode)
	svc, err := storagegov.NewService(storagegov.Options{FilesystemPath: "/", Collector: testCollector{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(services) > 0 {
		svc = services[0]
	}
	secret := []byte("storage-observer-test-secret-not-a-deployment-key")
	issuer := claims.Issuer
	if issuer == "" {
		issuer = "storage-test"
	}
	token, err := auth.GenerateAccessJWT(claims, issuer, []string{"admin"}, time.Hour, secret)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	group := r.Group("/api/v1", authmw.JwtMiddleware(secret, issuer, []string{"admin"}, []string{"access"}, nil))
	RegisterAPIRoutes(r.Group("/public"), group, &shared.Deps{StorageGovernance: svc})
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestStorageRouteCollectsRegisteredTemporaryDirectory(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "private-payload-name")
	if err = os.WriteFile(file, make([]byte, 65536), 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := storagegov.NewService(storagegov.Options{FilesystemPath: root, Collector: storageobserver.Collector{}, Resources: []storagegov.Resource{{ID: "owned-source", Name: "Owned fixture", Kind: "backup_files", Scope: "shared", Path: root}}})
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, reqctx.CoreXClaims{UserID: 1, IsRoot: true}, "GET", "/api/v1/admin/ops/storage/overview", "", svc)
	var out struct {
		Data storagegov.Overview `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || out.Data.Filesystem.State != "observed" || len(out.Data.Resources) != 1 || out.Data.Resources[0].PhysicalBytes == nil || *out.Data.Resources[0].LogicalBytes != 65536 {
		t.Fatalf("real route/collector failed: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private-payload-name") || strings.Contains(w.Body.String(), root) {
		t.Fatal("filesystem paths leaked")
	}
}

func TestStorageRoutesRequireHumanRootJWT(t *testing.T) {
	sts := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	sts.Issuer = "powerx-sts"
	for _, claims := range []reqctx.CoreXClaims{{UserID: 1, IsRoot: false}, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}, sts, {UserID: 0, IsRoot: true}} {
		w := request(t, claims, "GET", "/api/v1/admin/ops/storage/overview", "")
		if w.Code != 403 {
			t.Fatalf("accepted non-root user plane: status %d: %s", w.Code, w.Body.String())
		}
	}
	w := request(t, reqctx.CoreXClaims{UserID: 1, IsRoot: true}, "GET", "/api/v1/admin/ops/storage/overview", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"cleanup_enabled":false`) || !strings.Contains(w.Body.String(), `"total_bytes":null`) {
		t.Fatalf("invalid observation: %d %s", w.Code, w.Body.String())
	}
}

func TestStorageRoutesRejectProxyInputsAndUnsafeOperations(t *testing.T) {
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	for _, body := range []string{`{"target_days":7,"max_bytes":3000,"path":"/tmp"}`, `{"target_days":7,"max_bytes":3000,"daily_increment_bytes":1.5}`, `{"target_days":7,"max_bytes":3000} {}`, `null`, `{"target_days":0,"max_bytes":3000}`} {
		w := request(t, root, "POST", "/api/v1/admin/ops/storage/estimates", body)
		if w.Code != 400 {
			t.Fatalf("accepted %s: %d", body, w.Code)
		}
	}
	if w := request(t, root, "GET", "/api/v1/admin/ops/storage/overview?path=/tmp", ""); w.Code != 400 {
		t.Fatal("query override accepted")
	}
	if w := request(t, root, "POST", "/api/v1/admin/ops/storage/cleanup-plans", `{}`); w.Code != 404 {
		t.Fatal("cleanup unexpectedly available")
	}
	w := request(t, root, "POST", "/api/v1/admin/ops/storage/estimates", `{"target_days":14,"max_bytes":3000,"daily_increment_bytes":300}`)
	var envelope struct {
		Data storagegov.Estimate `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &envelope)
	if w.Code != 200 || envelope.Data.State != "budget_insufficient" || envelope.Data.CanEnable || *envelope.Data.RequiredBytes != 5460 {
		t.Fatalf("invalid estimate: %d %s", w.Code, w.Body.String())
	}
}

func TestStorageDraftBodyRequiresExplicitRevisionAndProtection(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":1,"state":"draft","budget":{},"resources":[]}`,
		`{"schema_version":1,"expected_revision":null,"state":"draft","budget":{},"resources":[]}`,
		`{"schema_version":1,"expected_revision":0,"state":"draft","budget":{},"resources":null}`,
		`{"schema_version":1,"expected_revision":0,"state":"draft","budget":{},"resources":[{"resource_id":"a"}]}`,
	} {
		if policyFieldsPresent([]byte(raw)) {
			t.Fatalf("accepted missing/null fields: %s", raw)
		}
	}
	if !policyFieldsPresent([]byte(`{"schema_version":1,"expected_revision":0,"state":"draft","budget":{},"resources":[{"resource_id":"a","target_days":1,"max_bytes":1,"retention_count":0,"min_verified_points":0,"protect_latest":false,"require_verified_archive":false}]}`)) {
		t.Fatal("explicit false or zero rejected")
	}
}

func TestStorageScopedEstimatesResolveEnvironmentServerSide(t *testing.T) {
	svc, err := storagegov.NewService(storagegov.Options{FilesystemPath: "/", Collector: testCollector{}, Resources: []storagegov.Resource{
		{ID: "prod-logs", Name: "Logs", Kind: "application_logs", Scope: "prod", InstanceID: "production", Path: "/private/logs"},
		{ID: "dev-backups", Name: "Backups", Kind: "backup_files", Scope: "dev", InstanceID: "development", Path: "/private/backups"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	w := request(t, root, "POST", "/api/v1/admin/ops/storage/estimates", `{"resource_id":"prod-logs","retention_mode":"days","target_days":14,"max_bytes":3000,"daily_increment_bytes":300}`, svc)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"scope":"prod"`) || !strings.Contains(w.Body.String(), `"instance_id":"production"`) || strings.Contains(w.Body.String(), "/private/") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{
		`{"resource_id":"unknown","retention_mode":"days","target_days":7,"max_bytes":3000}`,
		`{"resource_id":"prod-logs","scope":"dev","retention_mode":"days","target_days":7,"max_bytes":3000}`,
		`{"resource_id":"dev-backups","retention_mode":"days","target_days":7,"max_bytes":3000}`,
	} {
		if got := request(t, root, "POST", "/api/v1/admin/ops/storage/estimates", body, svc); got.Code != 400 {
			t.Fatal("scope/method override accepted", got.Body.String())
		}
	}
	w = request(t, root, "POST", "/api/v1/admin/ops/storage/estimates", `{"resource_id":"dev-backups","retention_mode":"count","target_days":0,"retention_count":24,"max_bytes":3000}`, svc)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"artifact_inventory_required"`) || !strings.Contains(w.Body.String(), `"required_bytes":null`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
