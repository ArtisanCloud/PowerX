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
	if len(specs) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]modelsiam.Permission, 0, len(specs)*2)
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
