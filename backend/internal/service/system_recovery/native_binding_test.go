package system_recovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type bindingProof struct{ PluginID, User, Schema, BindingUUID, PasswordHash string }

func seedNativeBinding(t *testing.T, ctx context.Context, cfg *config.Config) bindingProof {
	t.Helper()
	id := "com.powerx.plugins.recovery-fixture"
	root := filepath.Join(cfg.Plugin.InstalledDir, id, "1.0.0")
	require.NoError(t, os.MkdirAll(root, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte("id: "+id+"\nversion: 1.0.0\nname: Recovery binding fixture\nruntime:\n  kind: static\n"), 0600))
	descriptor, e := manager.NewFSLoader().LoadDescriptor(ctx, root)
	require.NoError(t, e)
	registry := manager.NewJSONRegistry(cfg.Plugin.RegistryFile)
	require.NoError(t, registry.Load(ctx))
	require.NoError(t, registry.Put(ctx, descriptor, plugin_mgr.StateInstalled))
	require.NoError(t, registry.Save(ctx))
	_, e = manager.RemapRecoveryBindings(ctx, cfg)
	require.NoError(t, e)
	require.NoError(t, registry.Load(ctx))
	plugin, ok := registry.Get(ctx, id)
	require.True(t, ok)
	database := plugin.HostConfig.Spec["database"].(map[string]any)
	proof := bindingProof{PluginID: id, User: database["user"].(string), Schema: database["schema"].(string), BindingUUID: database["binding_uuid"].(string)}
	connection, e := pgx.Connect(ctx, cfg.Database.DSN)
	require.NoError(t, e)
	defer connection.Close(ctx)
	require.NoError(t, connection.QueryRow(ctx, "SELECT rolpassword FROM pg_authid WHERE rolname=$1", proof.User).Scan(&proof.PasswordHash))
	_, e = connection.Exec(ctx, "CREATE TABLE "+pgx.Identifier{proof.Schema, "fixture_orders"}.Sanitize()+"(id integer PRIMARY KEY,label text NOT NULL)")
	require.NoError(t, e)
	_, e = connection.Exec(ctx, "INSERT INTO "+pgx.Identifier{proof.Schema, "fixture_orders"}.Sanitize()+" VALUES(1,'A-plugin')")
	require.NoError(t, e)
	return proof
}
func verifyNativeBinding(t *testing.T, ctx context.Context, sourceDSN string, target Deployment, proof bindingProof) {
	t.Helper()
	registry := manager.NewJSONRegistry(filepath.Join(target.Root, "plugins/registry.json"))
	require.NoError(t, registry.Load(ctx))
	plugin, ok := registry.Get(ctx, proof.PluginID)
	require.True(t, ok)
	database := plugin.HostConfig.Spec["database"].(map[string]any)
	require.Equal(t, "dev", database["deployment_env"])
	require.NotEqual(t, proof.User, database["user"])
	require.NotEqual(t, proof.BindingUUID, database["binding_uuid"])
	require.Equal(t, proof.Schema, database["schema"])
	targetConnection, e := pgx.Connect(ctx, database["dsn"].(string))
	require.NoError(t, e)
	defer targetConnection.Close(ctx)
	var label string
	require.NoError(t, targetConnection.QueryRow(ctx, "SELECT label FROM fixture_orders WHERE id=1").Scan(&label))
	require.Equal(t, "A-plugin", label)
	var readOnly string
	require.NoError(t, targetConnection.QueryRow(ctx, "SHOW default_transaction_read_only").Scan(&readOnly))
	require.Equal(t, "on", readOnly)
	_, e = targetConnection.Exec(ctx, "UPDATE fixture_orders SET label='forbidden' WHERE id=1")
	require.Error(t, e)
	sourceConnection, e := pgx.Connect(ctx, sourceDSN)
	require.NoError(t, e)
	defer sourceConnection.Close(ctx)
	var hash string
	require.NoError(t, sourceConnection.QueryRow(ctx, "SELECT rolpassword FROM pg_authid WHERE rolname=$1", proof.User).Scan(&hash))
	require.Equal(t, proof.PasswordHash, hash, "target binding must not rotate the original cluster role")
}
