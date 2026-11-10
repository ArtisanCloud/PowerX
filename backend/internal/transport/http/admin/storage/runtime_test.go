package storage

import (
	"context"
	"encoding/json"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runtimeFixture(t *testing.T) *gov.Service {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	c, err := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "state"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "dev", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	e, err := lifecycle.New(lifecycle.Options{Coordinator: c, InstanceID: "dev", AllowEnforce: true, Sources: []lifecycle.Source{{ID: "logs", Name: "Logs", Kind: "application_logs", Scope: "dev", InstanceID: "dev", Directory: dir, ActiveFiles: []string{"app.log"}, Format: "numeric", OwnerUID: uint32(os.Geteuid())}}})
	require.NoError(t, err)
	_, err = e.Save(context.Background(), 1, "logs", 0, lifecycle.Policy{Enabled: true, RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	s, err := gov.NewService(gov.Options{FilesystemPath: dir, Collector: testCollector{}, Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "dev"}, Budget: c, Lifecycle: e, LifecycleWorkerEnabled: true})
	require.NoError(t, err)
	return s
}
func TestRuntimeRoutesRequireHumanRootAndStrictTypedInput(t *testing.T) {
	s := runtimeFixture(t)
	prefix := "/api/v1/admin/ops/storage/runtime"
	for _, in := range []struct{ method, path, body string }{{"GET", prefix, ""}, {"GET", prefix + "/policies/logs", ""}, {"GET", prefix + "/policies/logs/requests/request", ""}, {"POST", prefix + "/policies/logs/adoptions", `{}`}, {"PUT", prefix + "/budget", `{"expected_revision":1,"confirm":true}`}, {"PUT", prefix + "/policies/logs", `{}`}, {"POST", prefix + "/runs", `{}`}, {"GET", prefix + "/events", ""}, {"POST", prefix + "/runs/id/acknowledge", `{"confirm":true}`}} {
		for _, claims := range []reqctx.CoreXClaims{{UserID: 1}, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}, {UserID: 0, IsRoot: true}} {
			w := request(t, claims, in.method, in.path, in.body, s)
			require.Equal(t, 403, w.Code, w.Body.String())
		}
	}
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	for _, body := range []string{`{"source_id":"logs","path":"/tmp"}`, `{"source_id":"logs","retention_days":7,"max_bytes":100,"confirm":null}`, `{"source_id":"logs","retention_days":7,"max_bytes":100} {}`} {
		w := request(t, root, "POST", prefix+"/previews", body, s)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	body := `{"expected_revision":0,"enabled":true,"retention_days":7,"max_bytes":1048576,"require_archive":false,"confirm":true}`
	w := request(t, root, "PUT", prefix+"/policies/logs", body, s)
	require.Equal(t, 400, w.Code, w.Body.String())
	body = `{"request_id":"10000000-0000-4000-8000-000000000002","expected_revision":1,"enabled":true,"cleanup_mode":"expire_delete","retention_days":7,"max_bytes":1048576,"require_archive":false,"confirm":true}`
	w = request(t, root, "PUT", prefix+"/policies/logs", body, s)
	require.Equal(t, 503, w.Code, w.Body.String(), "no database authority must never fall back to legacy JSON writes")
	w = request(t, root, "GET", prefix, "", s)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), s.RuntimeChangeDirectories()[0])
	require.Contains(t, w.Body.String(), `"database_cleanup_enabled":false`)
	var out struct {
		Data gov.RuntimeStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.True(t, out.Data.Lifecycle.Policies[0].Enabled)
	run := `{"source_id":"logs","expected_revision":1,"request_id":"10000000-0000-4000-8000-000000000001","confirm":true}`
	w = request(t, root, "POST", prefix+"/runs", run, s)
	require.Equal(t, 200, w.Code, w.Body.String())
	first := w.Body.String()
	w = request(t, root, "POST", prefix+"/runs", run, s)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, first, w.Body.String())
	_, err := s.Runtime(context.Background())
	require.ErrorIs(t, err, gov.ErrRootRequired)
	w = request(t, root, "GET", prefix+"/events?token=forbidden", "", s)
	require.Equal(t, 400, w.Code)
	require.False(t, strings.Contains(w.Body.String(), "forbidden"))
}

func TestDatabaseRetentionReadinessIsReadableInObservationModeAndStillRootOnly(t *testing.T) {
	path := "/api/v1/admin/ops/storage/database-retention"
	for _, claims := range []reqctx.CoreXClaims{{UserID: 1}, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}, {UserID: 0, IsRoot: true}} {
		w := request(t, claims, "GET", path, "")
		require.Equal(t, 403, w.Code, w.Body.String())
	}
	w := request(t, reqctx.CoreXClaims{UserID: 1, IsRoot: true}, "GET", path, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var out struct {
		Data gov.DatabaseRetentionStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.False(t, out.Data.Ready)
	require.False(t, out.Data.StateConfigured)
	require.False(t, out.Data.Policy.Enabled)
	require.Equal(t, "database_retention_runtime_not_configured", out.Data.Problem)
	require.Equal(t, "database_unavailable", out.Data.MigrationProblem)
	w = request(t, reqctx.CoreXClaims{UserID: 1, IsRoot: true}, "PUT", path, `{"expected_revision":0,"confirm":true,"policy":{"enabled":true}}`)
	require.Equal(t, 428, w.Code, w.Body.String())
}
