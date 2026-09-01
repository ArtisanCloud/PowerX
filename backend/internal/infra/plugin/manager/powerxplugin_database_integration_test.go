package manager

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	coreconfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// TestPowerXPluginDatabaseIsolationIntegration 使用真实 PowerXPlugin dist 和 Core 数据库验证安装绑定。
// 默认跳过；显式提供 POWERX_TEST_PLUGIN_DIST 与 POWERX_TEST_CONFIG 后执行，并在结束时清理本次环境隔离对象。
func TestPowerXPluginDatabaseIsolationIntegration(t *testing.T) {
	dist := os.Getenv("POWERX_TEST_PLUGIN_DIST")
	configPath := os.Getenv("POWERX_TEST_CONFIG")
	if dist == "" || configPath == "" {
		t.Skip("set POWERX_TEST_PLUGIN_DIST and POWERX_TEST_CONFIG to run integration test")
	}

	if err := coreconfig.InitGlobalConfig(configPath); err != nil {
		t.Fatalf("InitGlobalConfig() error = %v", err)
	}
	cfg := coreconfig.GetGlobalConfig()
	if cfg == nil {
		t.Fatal("global config is nil")
	}
	if cfg.Deployment.Env != coreconfig.DeploymentEnvDev {
		t.Fatalf("integration test requires deployment.env=dev, got %q", cfg.Deployment.Env)
	}

	ctx := context.Background()
	tmp := t.TempDir()
	registry := NewJSONRegistry(filepath.Join(tmp, "registry.json"))
	if err := registry.Load(ctx); err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}
	m := &managerImpl{opts: Options{
		InstalledRoot: filepath.Join(tmp, "installed"),
		RegistryFile:  filepath.Join(tmp, "registry.json"),
		Registry:      registry,
		Loader:        NewFSLoader(),
		CoreConfig:    cfg,
	}}
	pluginID := "com.powerx.plugins.base"
	expectedSchema := makePluginSchema(pluginID)
	expectedUser := makePluginUser(cfg.Deployment.Env, pluginID)
	cleanupHostConfig := &plugin_mgr.HostConfig{Spec: map[string]any{
		"database": map[string]any{
			"managed":        true,
			"deployment_env": cfg.Deployment.Env,
			"plugin_key":     pluginID,
			"plugin_uuid":    pluginDatabasePluginUUID(pluginID),
			"binding_uuid":   pluginDatabaseBindingUUID(cfg.Deployment.Env, pluginID),
			"schema":         expectedSchema,
			"user":           expectedUser,
		},
	}}
	cfg.Plugin.AllowDestructiveDBCleanup = true
	t.Cleanup(func() {
		if err := m.cleanupPluginDatabaseResources(pluginID, cleanupHostConfig); err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("cleanupPluginDatabaseResources() error = %v", err)
		}
	})

	installed, err := m.InstallFromFile(ctx, dist, plugin_mgr.InstallOptions{RunMigrations: true})
	if err != nil {
		t.Fatalf("InstallFromFile() error = %v", err)
	}
	if installed.ID != pluginID {
		t.Fatalf("installed plugin id = %q, want %q", installed.ID, pluginID)
	}

	if err := m.validatePluginDatabaseBinding(pluginID, installed.HostConfig); err != nil {
		t.Fatalf("validatePluginDatabaseBinding() error = %v", err)
	}
	dbSpec, ok := installed.HostConfig.Spec["database"].(map[string]any)
	if !ok {
		t.Fatalf("database binding missing from host config: %#v", installed.HostConfig.Spec)
	}
	if got := getStringFromMap(dbSpec, "schema"); got != expectedSchema {
		t.Fatalf("database.schema = %q, want %q", got, expectedSchema)
	}
	if got := getStringFromMap(dbSpec, "user"); got != expectedUser {
		t.Fatalf("database.user = %q, want %q", got, expectedUser)
	}
	if got := getStringFromMap(dbSpec, "deployment_env"); got != cfg.Deployment.Env {
		t.Fatalf("database.deployment_env = %q, want %q", got, cfg.Deployment.Env)
	}

	db, closeDB, err := connectAdminDB(cfg.Database)
	if err != nil {
		t.Fatalf("connectAdminDB() error = %v", err)
	}
	defer closeDB()
	var schemaExists int
	if err := db.Raw("SELECT 1 FROM information_schema.schemata WHERE schema_name = ?", expectedSchema).Row().Scan(&schemaExists); err != nil {
		t.Fatalf("schema %q not found: %v", expectedSchema, err)
	}
	var roleExists int
	if err := db.Raw("SELECT 1 FROM pg_roles WHERE rolname = ?", expectedUser).Row().Scan(&roleExists); err != nil {
		t.Fatalf("role %q not found: %v", expectedUser, err)
	}
	var schemaOwner string
	if err := db.Raw(`
SELECT r.rolname
FROM pg_namespace n
JOIN pg_roles r ON r.oid = n.nspowner
WHERE n.nspname = ?`, expectedSchema).Row().Scan(&schemaOwner); err != nil {
		t.Fatalf("read schema owner for %q: %v", expectedSchema, err)
	}
	if schemaOwner != expectedUser {
		t.Fatalf("schema owner = %q, want %q", schemaOwner, expectedUser)
	}
	var tableOwner string
	if err := db.Raw(`
SELECT r.rolname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_roles r ON r.oid = c.relowner
WHERE n.nspname = ? AND c.relname = ?`, expectedSchema, "plugin_tenant_ext").Row().Scan(&tableOwner); err != nil {
		t.Fatalf("read plugin_tenant_ext owner: %v", err)
	}
	if tableOwner != expectedUser {
		t.Fatalf("plugin_tenant_ext owner = %q, want %q", tableOwner, expectedUser)
	}

}
