package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gorm.io/gorm"
)

func syncPluginManifestPermissions(ctx context.Context, db *gorm.DB, manifest plugin_mgr.Manifest) error {
	if db == nil {
		return nil
	}
	rows := buildPluginPermissionRows(manifest)
	if len(rows) == 0 {
		return nil
	}
	service := iamsvc.NewPermissionService(db)
	source := "plugin:" + strings.TrimSpace(manifest.ID)
	_, err := service.SyncPermissions(ctx, source, strings.TrimSpace(manifest.Version), rows, false)
	if err != nil {
		return fmt.Errorf("sync plugin permissions failed: %w", err)
	}
	return nil
}

func syncPluginPermissionsRemoval(ctx context.Context, db *gorm.DB, pluginID string) error {
	if db == nil {
		return nil
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return nil
	}
	service := iamsvc.NewPermissionService(db)
	source := "plugin:" + pluginID
	_, err := service.SyncPermissions(ctx, source, "", nil, false)
	if err != nil {
		return fmt.Errorf("revoke plugin permissions failed: %w", err)
	}
	return nil
}

func buildPluginPermissionRows(manifest plugin_mgr.Manifest) []modelsiam.Permission {
	pluginID := strings.TrimSpace(manifest.ID)
	if pluginID == "" {
		return nil
	}
	specs := manifest.Permissions
	if len(specs) == 0 && len(manifest.RBAC.Resources) > 0 {
		specs = make([]plugin_mgr.PermissionSpec, 0, len(manifest.RBAC.Resources))
		for _, item := range manifest.RBAC.Resources {
			specs = append(specs, plugin_mgr.PermissionSpec{
				Resource: item.Resource,
				Actions:  item.Actions,
				Module:   pluginID,
				Type:     "action",
			})
		}
	}
	seen := make(map[string]struct{})
	out := make([]modelsiam.Permission, 0, len(specs)*2+len(manifest.Frontend.Admin.Menus))
	for _, spec := range specs {
		resource := strings.TrimSpace(spec.Resource)
		if resource == "" {
			continue
		}
		actions := dedupeNonEmpty(spec.Actions)
		for _, action := range actions {
			key := pluginID + "|" + resource + "|" + action
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			meta := map[string]any{
				"label":  firstNonEmpty(strings.TrimSpace(spec.Label), fmt.Sprintf("%s.%s.%s", pluginID, resource, action)),
				"module": firstNonEmpty(strings.TrimSpace(spec.Module), pluginID),
				"type":   firstNonEmpty(strings.TrimSpace(spec.Type), "action"),
			}
			allowAPIKey := true
			if spec.APIKey != nil {
				meta["api_key"] = map[string]any{
					"scope":            strings.TrimSpace(spec.APIKey.Scope),
					"action":           firstNonEmpty(strings.TrimSpace(spec.APIKey.Action), action),
					"resource_type":    strings.TrimSpace(spec.APIKey.ResourceType),
					"resource_pattern": strings.TrimSpace(spec.APIKey.ResourcePattern),
					"plugin_id":        strings.TrimSpace(spec.APIKey.PluginID),
					"effect":           firstNonEmpty(strings.TrimSpace(spec.APIKey.Effect), "allow"),
				}
				allowAPIKey = true
			}
			metaBytes, _ := json.Marshal(meta)
			out = append(out, modelsiam.Permission{
				Module:      pluginID,
				Resource:    resource,
				Action:      action,
				Effect:      "allow",
				Description: strings.TrimSpace(spec.Description),
				AllowAPIKey: allowAPIKey,
				Meta:        metaBytes,
				Status:      modelsiam.PermissionStatusActive,
				Source:      "plugin:" + pluginID,
				Introduced:  strings.TrimSpace(manifest.Version),
			})
		}
	}
	out = appendPluginMenuPermissionRows(out, seen, manifest)
	return out
}

func appendPluginMenuPermissionRows(out []modelsiam.Permission, seen map[string]struct{}, manifest plugin_mgr.Manifest) []modelsiam.Permission {
	pluginID := strings.TrimSpace(manifest.ID)
	if pluginID == "" || len(manifest.Frontend.Admin.Menus) == 0 {
		return out
	}
	var walk func(items []plugin_mgr.MenuItem)
	walk = func(items []plugin_mgr.MenuItem) {
		for _, item := range items {
			resource := plugin_mgr.PluginMenuPermissionResource(pluginID, item)
			if resource != "" {
				action := plugin_mgr.MenuPermissionAction
				key := plugin_mgr.MenuPermissionModule + "|" + resource + "|" + action
				if _, exists := seen[key]; !exists {
					seen[key] = struct{}{}
					label := strings.TrimSpace(item.Title)
					if label == "" && item.TitleI18n != nil {
						label = strings.TrimSpace(item.TitleI18n.Default)
					}
					if label == "" {
						label = resource
					}
					meta := map[string]any{
						"type":        "menu",
						"module":      plugin_mgr.MenuPermissionModule,
						"label":       label,
						"plugin_id":   pluginID,
						"plugin_name": strings.TrimSpace(manifest.Name),
						"menu_id":     plugin_mgr.PluginMenuPermissionSegment(item),
						"origin":      "plugin",
					}
					metaBytes, _ := json.Marshal(meta)
					out = append(out, modelsiam.Permission{
						Module:      plugin_mgr.MenuPermissionModule,
						Resource:    resource,
						Action:      action,
						Effect:      "allow",
						Description: fmt.Sprintf("Allow viewing plugin menu %s", label),
						AllowAPIKey: false,
						Meta:        metaBytes,
						Status:      modelsiam.PermissionStatusActive,
						Source:      "plugin:" + pluginID,
						Introduced:  strings.TrimSpace(manifest.Version),
					})
				}
			}
			if len(item.Children) > 0 {
				walk(item.Children)
			}
		}
	}
	walk(manifest.Frontend.Admin.Menus)
	return out
}

func dedupeNonEmpty(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return item
		}
	}
	return ""
}
