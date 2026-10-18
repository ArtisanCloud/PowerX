package notifications

import (
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNavigationActionsRejectUnsafeDestinations(t *testing.T) {
	for _, path := range []string{"https://external.test/x", "//external.test/x", "/%2fexternal.test/x", "/a/../b", "/a/%2e%2e/b", "/a/%252e%252e/b", "/a\\b", "/a%5cb", "/a#b", "/a%00b", "/a b", "/a%20b", "/a\nb", " /a"} {
		t.Run(path, func(t *testing.T) {
			_, err := NavigationActions(&Navigation{Label: "fixture_action", Path: path})
			var app *dto.AppError
			require.ErrorAs(t, err, &app)
			require.Equal(t, 400, app.HTTPCode)
			require.Equal(t, "NOTIFICATION_NAVIGATION_INVALID", app.Code)
		})
	}
	raw, err := NavigationActions(nil)
	require.NoError(t, err)
	require.Nil(t, raw)
}
func TestNavigationActionsPreserveOpaqueEntry(t *testing.T) {
	path := "/_p/plugin.test/admin/orders?target=opaque%3A123"
	raw, err := NavigationActions(&Navigation{Label: "fixture_action", Path: path})
	require.NoError(t, err)
	var actions []Action
	require.NoError(t, json.Unmarshal(raw, &actions))
	require.Len(t, actions, 1)
	require.Equal(t, "navigate", actions[0].Action)
	require.Equal(t, path, actions[0].Params["path"])
}
