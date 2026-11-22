package manager

import (
	"fmt"
	"testing"

	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestManagedDatasetBinding(t *testing.T) {
	const id = "com.powerx.plugins.storage-fixture"
	const env = "test"
	role := makePluginUser(env, id)
	dsn := func(host, database string) string {
		return fmt.Sprintf("host=%s port=5432 dbname=%s user=%s sslmode=disable", host, database, role)
	}
	metadata := map[string]any{
		"managed": true, "driver": "postgres", "deployment_env": env, "plugin_key": id,
		"plugin_uuid": pluginDatabasePluginUUID(id), "binding_uuid": pluginDatabaseBindingUUID(env, id),
		"schema": makePluginSchema(id), "user": role, "dsn": dsn("127.0.0.1", "isolated"),
	}
	plugin := plugin_mgr.Plugin{ID: id, HostConfig: &plugin_mgr.HostConfig{Spec: map[string]any{"database": metadata}}}
	cfg := corexdb.DatabaseConfig{Driver: "postgres", DSN: "host=127.0.0.1 port=5432 dbname=isolated user=fixture_admin sslmode=disable"}
	binding, schema, actualRole, err := ManagedDatasetBinding(plugin, env, cfg)
	require.NoError(t, err)
	require.Equal(t, pluginDatabaseBindingUUID(env, id), binding.String())
	require.Equal(t, makePluginSchema(id), schema)
	require.Equal(t, role, actualRole)
	for _, bad := range []string{dsn("127.0.0.1", "other"), dsn("127.0.0.2", "isolated"), ""} {
		metadata["dsn"] = bad
		_, _, _, err := ManagedDatasetBinding(plugin, env, cfg)
		require.Error(t, err)
	}
	metadata["dsn"] = dsn("127.0.0.1", "isolated")
	metadata["schema"] = "public"
	_, _, _, err = ManagedDatasetBinding(plugin, env, cfg)
	require.Error(t, err)
}
