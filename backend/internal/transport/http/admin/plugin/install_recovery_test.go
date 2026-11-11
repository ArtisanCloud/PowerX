package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recoveryManager struct {
	plugin_mgr.Manager
	current plugin_mgr.Plugin
	calls   int
	result  error
}

func (m *recoveryManager) Get(context.Context, string) (plugin_mgr.Plugin, error) {
	return m.current, nil
}
func (m *recoveryManager) Enable(context.Context, string) error { m.calls++; return m.result }

func TestPluginEnableResumesTenantFinalizationWhenRuntimeAlreadyEnabled(t *testing.T) {
	for _, body := range []string{"", `{"expected_version":"1.0.0","expected_revision":"reviewed"}`} {
		m := &recoveryManager{current: plugin_mgr.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", Revision: "reviewed"}, result: plugin_mgr.NewError(plugin_mgr.CodeAlreadyEnabled)}
		mgrimpl.ResetGlobalForTest(m)
		t.Cleanup(func() { mgrimpl.ResetGlobalForTest(nil) })
		r := gin.New()
		r.POST("/:id/enable", PluginEnableHandler(&shared.Deps{}))
		req := httptest.NewRequest(http.MethodPost, "/com.powerx.plugins.fixture/enable", strings.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, 1, m.calls)
	}
}

func TestPluginEnableRecoveryRejectsChangedRevisionAndRealRuntimeFailure(t *testing.T) {
	for _, test := range []struct {
		name, body    string
		result        error
		status, calls int
	}{
		{"changed", `{"expected_version":"1.0.0","expected_revision":"old"}`, nil, 409, 0},
		{"different_version", `{"expected_version":"2.0.0","expected_revision":"reviewed"}`, nil, 409, 0},
		{"runtime_failed", `{"expected_version":"1.0.0","expected_revision":"reviewed"}`, plugin_mgr.NewError(plugin_mgr.CodeHealthcheckFailed), 400, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := &recoveryManager{current: plugin_mgr.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", Revision: "reviewed"}, result: test.result}
			mgrimpl.ResetGlobalForTest(m)
			t.Cleanup(func() { mgrimpl.ResetGlobalForTest(nil) })
			r := gin.New()
			r.POST("/:id/enable", PluginEnableHandler(&shared.Deps{}))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/com.powerx.plugins.fixture/enable", strings.NewReader(test.body)))
			require.Equal(t, test.status, w.Code, w.Body.String())
			require.Equal(t, test.calls, m.calls)
		})
	}
}

func TestPendingInstallResponseExposesIdentityWithoutCredentials(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/install/local", nil)
	p := plugin_mgr.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", Revision: "revision", State: plugin_mgr.StateEnabled, HostConfig: &plugin_mgr.HostConfig{Values: map[string]string{"SECRET": "private-fixture-value"}}}
	respondPluginInstallError(c, "安装失败", plugin_mgr.PendingInstallation(p, "tenant_activation", errors.New("private-fixture-value")))
	require.Equal(t, 500, w.Code)
	require.Contains(t, w.Body.String(), "PLUGIN_INSTALL_FINALIZATION_REQUIRED")
	require.Contains(t, w.Body.String(), `"tenant_ready":false`)
	require.NotContains(t, w.Body.String(), "private-fixture-value")
	require.NotContains(t, w.Body.String(), "host_config")
}
