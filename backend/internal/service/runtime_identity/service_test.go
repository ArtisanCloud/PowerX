package runtime_identity

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"testing"
)

type identityManager struct {
	plugin_mgr.Manager
	version   string
	getErr    error
	runtimeOK bool
	state     supervisor.ProcState
}

func (m identityManager) Get(context.Context, string) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{Version: m.version}, m.getErr
}
func (m identityManager) RuntimeStatus(string) (supervisor.ProcInfo, bool) {
	return supervisor.ProcInfo{State: m.state}, m.runtimeOK
}
func testService(m identityManager) *Service {
	return &Service{manager: func() (plugin_mgr.Manager, error) { return m, nil }, info: CoreInfo{Version: "v1.0.0", DeploymentEnv: "dev", Ready: true}}
}
func TestLookupRuntimeIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		m      identityManager
		status int
		code   string
	}{
		{"success", identityManager{version: "0.1.75", runtimeOK: true, state: supervisor.ProcRunning}, 200, ""},
		{"not found", identityManager{getErr: plugin_mgr.NewError(plugin_mgr.CodeNotFound)}, 404, "RUNTIME_IDENTITY_PLUGIN_NOT_FOUND"},
		{"registry failure", identityManager{getErr: errors.New("registry IO failed")}, 503, "RUNTIME_IDENTITY_UNAVAILABLE"},
		{"missing version", identityManager{}, 503, "RUNTIME_IDENTITY_VERSION_UNAVAILABLE"},
		{"missing supervisor", identityManager{version: "1", state: supervisor.ProcStopped}, 503, "RUNTIME_IDENTITY_STATE_UNAVAILABLE"},
		{"missing state", identityManager{version: "1", runtimeOK: true}, 503, "RUNTIME_IDENTITY_STATE_UNAVAILABLE"},
		{"stopped is known", identityManager{version: "1", runtimeOK: true, state: supervisor.ProcStopped}, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item, err := testService(tc.m).Lookup(context.Background(), "plugin.test")
			if tc.status != 200 {
				require.Equal(t, tc.status, dto.StatusCode(err))
				require.Equal(t, tc.code, dto.CodeOf(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, "registry", item.PluginVersionSource)
			require.Equal(t, "plugin.test", item.PluginID)
		})
	}
}
