package plugin_mgr

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPluginHTTPRouteUsesOnlyConfiguredPluginBase(t *testing.T) {
	for _, base := range []string{"/api/v1", "/api/v2", "/gateway/custom", "", "/"} {
		route, err := PluginHTTPRoute(base, "/internal/tools/parcel-volumes")
		require.NoError(t, err)
		expected := base
		if base == "/" {
			expected = ""
		}
		require.Equal(t, expected+"/internal/tools/parcel-volumes", route)
	}
	for _, value := range []string{"https://host/tool", "//host", "/a/../b", "/a%2fb", "/a?b=1", "/a\\b"} {
		_, err := PluginHTTPRoute("/api/v2", value)
		require.Error(t, err)
	}
}
