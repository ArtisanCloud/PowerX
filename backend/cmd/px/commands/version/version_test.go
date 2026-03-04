package version

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/utils/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRunVersionScan(t *testing.T) {
	testutil.SkipIfNoLocalListener(t)
	orig := scanOpts
	t.Cleanup(func() { scanOpts = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/version/governance/scan", r.URL.Path)
		require.Equal(t, "acme", r.URL.Query().Get("as_tenant_uuid"))
		body, _ := io.ReadAll(r.Body)
		require.NotContains(t, string(body), "tenant_uuid")
		resp := map[string]any{
			"code": 201,
			"data": map[string]any{
				"tenant_uuid":         "acme",
				"plugin_id":           "plugin.alpha",
				"current_version":     "1.0.0",
				"recommended_version": "1.1.0",
				"risk_level":          "warning",
				"status":              "generated",
				"generated_at":        "2025-11-08T00:00:00Z",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)

	scanOpts.api = server.URL
	scanOpts.tenantUUID = "acme"
	scanOpts.pluginID = "plugin.alpha"
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	require.NoError(t, runVersionScan(cmd, nil))
	require.Contains(t, buf.String(), "Tenant acme plugin plugin.alpha")
}

func TestRunVersionBoard(t *testing.T) {
	testutil.SkipIfNoLocalListener(t)
	orig := boardOpts
	t.Cleanup(func() { boardOpts = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/version/governance/board", r.URL.Path)
		resp := map[string]any{
			"code": 200,
			"data": map[string]any{
				"total": 1,
				"riskCounts": map[string]int{
					"pass":    1,
					"warning": 0,
				},
				"items": []map[string]any{
					{
						"tenant_uuid":         "acme",
						"plugin_id":           "plugin.alpha",
						"current_version":     "1.0.0",
						"recommended_version": "1.1.0",
						"risk_level":          "pass",
						"status":              "generated",
						"generated_at":        "2025-11-08T00:00:00Z",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)

	boardOpts.api = server.URL
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	require.NoError(t, runVersionBoard(cmd, nil))
	require.Contains(t, buf.String(), "Risk Counts")
	require.Contains(t, buf.String(), "plugin.alpha")
}

func TestCompatCommands(t *testing.T) {
	testutil.SkipIfNoLocalListener(t)
	origCheck := compatCheckOpts
	origException := compatExceptionOpts
	origApprove := compatApproveOpts
	t.Cleanup(func() {
		compatCheckOpts = origCheck
		compatExceptionOpts = origException
		compatApproveOpts = origApprove
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/version/compat/check":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{
					"compatible":       false,
					"reason":           "plugin major version 2 exceeds host 1",
					"suggestedVersion": "1.5.0",
				},
			})
		case "/internal/version/compat/exception":
			require.Equal(t, "acme", r.URL.Query().Get("as_tenant_uuid"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 201,
				"data": map[string]any{
					"uuid":            "exc-1",
					"tenant_uuid":     "acme",
					"plugin_id":       "plugin.alpha",
					"current_version": "1.0.0",
					"target_version":  "2.0.0",
					"status":          "pending",
				},
			})
		case "/internal/version/compat/approve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{
					"uuid":           "exc-1",
					"status":         "approved",
					"reviewer":       "admin",
					"decision_notes": "ok",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	// check
	compatCheckOpts.api = server.URL
	compatCheckOpts.hostVersion = "1.0.0"
	compatCheckOpts.pluginVersion = "2.0.0"
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	require.NoError(t, runCompatCheck(cmd, nil))
	require.Contains(t, buf.String(), "Not compatible")

	// exception
	compatExceptionOpts.api = server.URL
	compatExceptionOpts.tenantUUID = "acme"
	compatExceptionOpts.pluginID = "plugin.alpha"
	compatExceptionOpts.currentVersion = "1.0.0"
	compatExceptionOpts.targetVersion = "2.0.0"
	compatExceptionOpts.reason = "need feature"
	buf.Reset()
	require.NoError(t, runCompatException(cmd, nil))
	require.Contains(t, buf.String(), "Exception exc-1 created")

	// approve
	compatApproveOpts.api = server.URL
	compatApproveOpts.id = "exc-1"
	compatApproveOpts.status = "approved"
	compatApproveOpts.reviewer = "admin"
	compatApproveOpts.notes = "ok"
	buf.Reset()
	require.NoError(t, runCompatApprove(cmd, nil))
	require.Contains(t, buf.String(), "Exception exc-1 updated to approved")
}
