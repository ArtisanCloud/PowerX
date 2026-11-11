package plugin

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/pluginbinding"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

type InstallationReadiness struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

// InstallationReadiness 只核对已保存的当前租户配置和实时 ACL，不创建主题、授权或凭据。
func (s *TenantPluginInstanceService) InstallationReadiness(ctx context.Context, tenantUUID string, p plugin_mgr.Plugin) (InstallationReadiness, error) {
	tenantUUID, err := canonicalTenantUUID(tenantUUID)
	if err != nil {
		return InstallationReadiness{}, err
	}
	cfg, err := s.requireRepo().Get(ctx, tenantUUID, p.ID, reposetting.KeyClientCredentials)
	if err != nil {
		return InstallationReadiness{}, err
	}
	if cfg == nil {
		return InstallationReadiness{Reason: "TENANT_PLUGIN_NOT_FOUND"}, nil
	}
	if !cfg.Enabled || dbsetting.NormalizePluginInstanceStatus(cfg.Status, cfg.Enabled) != dbsetting.PluginInstanceStatusEnabled {
		return InstallationReadiness{Reason: "TENANT_PLUGIN_DISABLED"}, nil
	}
	if !pluginbinding.Declared(p) {
		return InstallationReadiness{Ready: true}, nil
	}
	var doc map[string]any
	if json.Unmarshal(cfg.ValueJSON, &doc) != nil {
		return InstallationReadiness{Reason: "TENANT_PLUGIN_CONFIG_INVALID"}, nil
	}
	settings, _ := doc["config"].(map[string]any)
	tenantPlugin := pluginbinding.TenantPlugin(p, settings)
	path, err := pluginbinding.ManifestPath(tenantPlugin)
	if err == nil {
		err = pluginbinding.NewService(s.requireRepo().DB()).WithNamespaceOwners(s.namespaceOwners).Prepare(ctx, tenantPlugin, tenantUUID, path, false, nil)
	}
	if err != nil {
		reason, _, _ := strings.Cut(err.Error(), ":")
		if !strings.HasPrefix(reason, "EVENT_FABRIC_") {
			return InstallationReadiness{}, err
		}
		return InstallationReadiness{Reason: reason}, nil
	}
	return InstallationReadiness{Ready: true}, nil
}
