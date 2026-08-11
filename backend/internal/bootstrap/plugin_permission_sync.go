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
		if row, ok := pluginPermissionRowFromPermissionCodeSpec(pluginID, manifest.Version, spec); ok {
			key := strings.TrimSpace(row.Module) + "|" + strings.TrimSpace(row.Resource) + "|" + strings.TrimSpace(row.Action)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, row)
			continue
		}
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

func pluginPermissionRowFromPermissionCodeSpec(pluginID, version string, spec plugin_mgr.PermissionSpec) (modelsiam.Permission, bool) {
	permissionCode := strings.TrimSpace(spec.PermissionCode)
	if permissionCode == "" {
		return modelsiam.Permission{}, false
	}
	module, resource, action, ok := splitPluginPermissionCode(permissionCode)
	if !ok {
		return modelsiam.Permission{}, false
	}
	meta := map[string]any{
		"type":        firstNonEmpty(strings.TrimSpace(spec.Type), "action"),
		"module":      firstNonEmpty(strings.TrimSpace(spec.Module), module),
		"plugin_id":   pluginID,
		"permission":  permissionCode,
		"risk_level":  strings.TrimSpace(spec.RiskLevel),
		"data_scope":  strings.TrimSpace(spec.DataScope),
		"independent": spec.Independent,
	}
	if len(spec.TitleI18n) > 0 {
		meta["title_i18n"] = spec.TitleI18n
	}
	if len(spec.DescriptionI18n) > 0 {
		meta["description_i18n"] = spec.DescriptionI18n
	}
	if businessPermissionCode := strings.TrimSpace(spec.BusinessPermissionCode); businessPermissionCode != "" {
		meta["business_permission_code"] = businessPermissionCode
	}
	if grants := dedupeNonEmpty(spec.DefaultRoleGrants); len(grants) > 0 {
		meta["default_role_grants"] = grants
	}
	if bindings := permissionProtocolBindingsMeta(spec.ProtocolBindings); len(bindings) > 0 {
		meta["protocol_bindings"] = bindings
	}
	if label := strings.TrimSpace(spec.Label); label != "" {
		meta["label"] = label
	}
	if spec.APIKey != nil {
		meta["api_key"] = map[string]any{
			"scope":            strings.TrimSpace(spec.APIKey.Scope),
			"action":           firstNonEmpty(strings.TrimSpace(spec.APIKey.Action), action),
			"resource_type":    strings.TrimSpace(spec.APIKey.ResourceType),
			"resource_pattern": strings.TrimSpace(spec.APIKey.ResourcePattern),
			"plugin_id":        strings.TrimSpace(spec.APIKey.PluginID),
			"effect":           firstNonEmpty(strings.TrimSpace(spec.APIKey.Effect), "allow"),
		}
	}
	metaBytes, _ := json.Marshal(meta)
	return modelsiam.Permission{
		Module:      module,
		Resource:    resource,
		Action:      action,
		Effect:      "allow",
		Description: strings.TrimSpace(spec.Description),
		AllowAPIKey: spec.AllowAPIKey || spec.APIKey != nil,
		Meta:        metaBytes,
		Status:      modelsiam.PermissionStatusActive,
		Source:      "plugin:" + pluginID,
		Introduced:  strings.TrimSpace(version),
	}, true
}

func splitPluginPermissionCode(code string) (module string, resource string, action string, ok bool) {
	code = strings.TrimSpace(code)
	parts := strings.Split(code, ":")
	if len(parts) != 2 {
		return "", "", "", false
	}
	left := strings.TrimSpace(parts[0])
	action = strings.TrimSpace(parts[1])
	dot := strings.LastIndex(left, ".")
	if dot <= 0 || dot >= len(left)-1 || action == "" {
		return "", "", "", false
	}
	module = strings.TrimSpace(left[:dot])
	resource = strings.TrimSpace(left[dot+1:])
	return module, resource, action, module != "" && resource != ""
}

func permissionProtocolBindingsMeta(bindings []plugin_mgr.PermissionProtocolBinding) []map[string]any {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		item := map[string]any{}
		if value := strings.TrimSpace(binding.Channel); value != "" {
			item["channel"] = value
		}
		if value := strings.TrimSpace(binding.Method); value != "" {
			item["method"] = value
		}
		if value := strings.TrimSpace(binding.Path); value != "" {
			item["path"] = value
		}
		if value := strings.TrimSpace(binding.RPC); value != "" {
			item["rpc"] = value
		}
		if value := strings.TrimSpace(binding.Tool); value != "" {
			item["tool"] = value
		}
		if value := strings.TrimSpace(binding.ActorContext); value != "" {
			item["actor_context"] = value
		}
		if value := strings.TrimSpace(binding.ResourceScope); value != "" {
			item["resource_scope"] = value
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
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
					if route := strings.TrimSpace(item.Route); route != "" {
						meta["route"] = route
					}
					if p := strings.TrimSpace(item.Path); p != "" {
						meta["path"] = p
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
