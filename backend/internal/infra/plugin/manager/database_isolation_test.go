package manager

import (
	"strings"
	"testing"

	coreconfig "github.com/ArtisanCloud/PowerX/config"
	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestPluginDatabaseRoleIncludesDeploymentWhileSchemaRemainsStable(t *testing.T) {
	pluginID := "com.powerx.plugin.ai-craft"
	devSchema := makePluginSchema(pluginID)
	prodSchema := makePluginSchema(pluginID)
	devUser := makePluginUser(coreconfig.DeploymentEnvDev, pluginID)
	prodUser := makePluginUser(coreconfig.DeploymentEnvProd, pluginID)

	if devSchema != "px_com_powerx_plugin_ai_craft" {
		t.Fatalf("dev schema = %q", devSchema)
	}
	if prodSchema != devSchema {
		t.Fatalf("prod schema = %q", prodSchema)
	}
	if !strings.HasPrefix(devUser, "pxu_dev_com_powerx_plugin_ai_craft_") {
		t.Fatalf("dev user = %q", devUser)
	}
	if !strings.HasPrefix(prodUser, "pxu_prod_com_powerx_plugin_ai_craft_") {
		t.Fatalf("prod user = %q", prodUser)
	}
	if devUser == prodUser {
		t.Fatal("deployment environments must produce different role names")
	}
	if len(devSchema) > 63 || len(devUser) > 63 {
		t.Fatalf("database identifier exceeds PostgreSQL limit: schema=%d user=%d", len(devSchema), len(devUser))
	}
	if got := makePluginSchema(pluginID); got != devSchema {
		t.Fatalf("schema naming is not deterministic: %q != %q", got, devSchema)
	}
}

func TestValidatePluginDatabaseBindingRejectsMissingAndMismatchedEnvironment(t *testing.T) {
	pluginID := "com.powerx.plugin.demo"
	m := &managerImpl{opts: Options{CoreConfig: &coreconfig.Config{
		Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev},
	}}}

	if err := m.validatePluginDatabaseBinding(pluginID, nil); err == nil || !strings.Contains(err.Error(), "reinstall") {
		t.Fatalf("missing binding error = %v", err)
	}

	hostConfig := &plugin_mgr.HostConfig{Spec: map[string]any{
		"database": map[string]any{
			"deployment_env": coreconfig.DeploymentEnvProd,
			"plugin_key":     pluginID,
			"plugin_uuid":    pluginDatabasePluginUUID(pluginID),
			"binding_uuid":   pluginDatabaseBindingUUID(coreconfig.DeploymentEnvProd, pluginID),
			"schema":         makePluginSchema(pluginID),
			"user":           makePluginUser(coreconfig.DeploymentEnvProd, pluginID),
		},
	}}
	if err := m.validatePluginDatabaseBinding(pluginID, hostConfig); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("mismatched binding error = %v", err)
	}
}

func TestValidatePluginDatabaseBindingAcceptsExactBinding(t *testing.T) {
	pluginID := "com.powerx.plugin.demo"
	env := coreconfig.DeploymentEnvStaging
	m := &managerImpl{opts: Options{CoreConfig: &coreconfig.Config{
		Deployment: coreconfig.DeploymentConfig{Env: env},
	}}}
	hostConfig := &plugin_mgr.HostConfig{Spec: map[string]any{
		"database": map[string]any{
			"deployment_env": env,
			"plugin_key":     pluginID,
			"plugin_uuid":    pluginDatabasePluginUUID(pluginID),
			"binding_uuid":   pluginDatabaseBindingUUID(env, pluginID),
			"schema":         makePluginSchema(pluginID),
			"user":           makePluginUser(env, pluginID),
		},
	}}
	if err := m.validatePluginDatabaseBinding(pluginID, hostConfig); err != nil {
		t.Fatalf("validatePluginDatabaseBinding() error = %v", err)
	}
}

func TestValidatePostgresPluginOwnershipSourcesAllowsOnlyCoreAndExactLegacyRole(t *testing.T) {
	pluginID := "com.powerx.plugins.base"
	section := &databaseSection{
		Schema:    makePluginSchema(pluginID),
		User:      makePluginUser(coreconfig.DeploymentEnvDev, pluginID),
		PluginKey: pluginID,
	}
	cfg := corexdb.DatabaseConfig{UserName: "powerx"}

	if err := validatePostgresPluginOwnershipSources([]string{"powerx", makeLegacyPluginRoleName(pluginID)}, cfg, section); err != nil {
		t.Fatalf("expected Core and legacy plugin owners to be accepted: %v", err)
	}
	if err := validatePostgresPluginOwnershipSources([]string{"another_plugin_role"}, cfg, section); err == nil || !strings.Contains(err.Error(), "unexpected existing owner") {
		t.Fatalf("unexpected owner error = %v", err)
	}
}

func testDatabaseSection(pluginID string) (*databaseSection, error) {
	env := coreconfig.DeploymentEnvDev
	return &databaseSection{
		Driver:        "postgres",
		DSN:           "postgres://plugin:secret@127.0.0.1/powerx",
		Schema:        makePluginSchema(pluginID),
		User:          makePluginUser(env, pluginID),
		Password:      "secret",
		SearchPath:    makePluginSchema(pluginID),
		Managed:       true,
		DeploymentEnv: env,
		PluginKey:     pluginID,
		PluginUUID:    pluginDatabasePluginUUID(pluginID),
		BindingUUID:   pluginDatabaseBindingUUID(env, pluginID),
	}, nil
}
