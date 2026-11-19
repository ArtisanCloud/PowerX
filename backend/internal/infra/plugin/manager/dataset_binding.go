package manager

import (
	"errors"
	"path/filepath"
	"strings"

	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ManagedDatasetBinding proves that the installed plugin is using the primary
// PostgreSQL database already opened by Core. The observation never uses a DSN
// received from the caller or opens a connection from plugin input.
func ManagedDatasetBinding(plugin plugin_mgr.Plugin, deploymentEnv string, cfg corexdb.DatabaseConfig) (uuid.UUID, string, string, error) {
	unavailable := errors.New("managed dataset database binding unavailable")
	if plugin.HostConfig == nil || plugin.ID == "" || deploymentEnv == "" {
		return uuid.Nil, "", "", unavailable
	}
	spec, ok := plugin.HostConfig.Spec["database"].(map[string]any)
	if !ok {
		return uuid.Nil, "", "", unavailable
	}
	managed, ok := boolFromMap(spec, "managed")
	if !ok || !managed || normalizeDriver(getStringFromMap(spec, "driver")) != "postgres" {
		return uuid.Nil, "", "", unavailable
	}
	schema, role := makePluginSchema(plugin.ID), makePluginUser(deploymentEnv, plugin.ID)
	binding := pluginDatabaseBindingUUID(deploymentEnv, plugin.ID)
	if getStringFromMap(spec, "deployment_env") != deploymentEnv || getStringFromMap(spec, "plugin_key") != plugin.ID ||
		getStringFromMap(spec, "plugin_uuid") != pluginDatabasePluginUUID(plugin.ID) || getStringFromMap(spec, "binding_uuid") != binding ||
		getStringFromMap(spec, "schema") != schema || getStringFromMap(spec, "user") != role {
		return uuid.Nil, "", "", unavailable
	}
	configured, e1 := pgx.ParseConfig(makeDatabaseDSN(cfg))
	dsn := getStringFromMap(spec, "dsn")
	if strings.TrimSpace(dsn) == "" {
		return uuid.Nil, "", "", unavailable
	}
	bound, e2 := pgx.ParseConfig(dsn)
	if e1 != nil || e2 != nil || bound.User != role || configured.Database == "" ||
		configured.Database != bound.Database || configured.Port != bound.Port ||
		!strings.EqualFold(cleanDatasetEndpoint(configured.Host), cleanDatasetEndpoint(bound.Host)) ||
		hasDifferentDatasetEndpoint(configured) || hasDifferentDatasetEndpoint(bound) {
		return uuid.Nil, "", "", unavailable
	}
	return uuid.MustParse(binding), schema, role, nil
}

func hasDifferentDatasetEndpoint(cfg *pgx.ConnConfig) bool {
	for _, fallback := range cfg.Fallbacks {
		if fallback.Host != cfg.Host || fallback.Port != cfg.Port {
			return true
		}
	}
	return false
}

func cleanDatasetEndpoint(host string) string {
	if filepath.IsAbs(host) {
		if path, err := filepath.EvalSymlinks(host); err == nil {
			return path
		}
		return filepath.Clean(host)
	}
	return host
}
