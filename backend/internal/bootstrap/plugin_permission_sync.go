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
	menuValidation := buildPluginMenuPermissionValidation(manifest)
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
	out := make([]modelsiam.Permission, 0, len(specs)*2)
	for _, spec := range specs {
		if row, ok := pluginPermissionRowFromPermissionCodeSpec(pluginID, manifest.Version, spec, menuValidation); ok {
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
	for _, diagnostic := range menuValidation.missing {
		if row, ok := pluginMenuPermissionDiagnosticRow(pluginID, manifest.Version, diagnostic); ok {
			key := strings.TrimSpace(row.Module) + "|" + strings.TrimSpace(row.Resource) + "|" + strings.TrimSpace(row.Action)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, row)
		}
	}
	return out
}

type pluginMenuPolicyDiagnostic struct {
	PermissionCode string
	MenuPath       []string
	TitleI18n      map[string]string
}

type pluginMenuPermissionValidation struct {
	pathsByPermission map[string][]string
	missing           []pluginMenuPolicyDiagnostic
}

func buildPluginMenuPermissionValidation(manifest plugin_mgr.Manifest) pluginMenuPermissionValidation {
	declared := map[string]struct{}{}
	for _, spec := range manifest.Permissions {
		if code := strings.TrimSpace(spec.PermissionCode); code != "" {
			declared[code] = struct{}{}
		}
	}
	result := pluginMenuPermissionValidation{
		pathsByPermission: map[string][]string{},
	}
	var walk func(items []plugin_mgr.MenuItem, ancestors []string)
	walk = func(items []plugin_mgr.MenuItem, ancestors []string) {
		for _, item := range items {
			id := strings.TrimSpace(item.ID)
			path := ancestors
			if id != "" {
				path = append(append([]string{}, ancestors...), id)
			}
			for _, policy := range dedupeNonEmpty(item.RequiredPolicies) {
				if _, exists := result.pathsByPermission[policy]; !exists && len(path) > 0 {
					result.pathsByPermission[policy] = append([]string{}, path...)
				}
				if _, ok := declared[policy]; !ok {
					result.missing = append(result.missing, pluginMenuPolicyDiagnostic{
						PermissionCode: policy,
						MenuPath:       append([]string{}, path...),
						TitleI18n:      menuLabelToPermissionTitleI18n(item.TitleI18n, item.Title, policy),
					})
				}
			}
			if len(item.Children) > 0 {
				walk(item.Children, path)
			}
		}
	}
	walk(manifest.Frontend.Admin.Menus, nil)
	return result
}

func menuLabelToPermissionTitleI18n(label *plugin_mgr.MenuLabel, title string, fallback string) map[string]string {
	out := map[string]string{}
	if label != nil {
		if value := strings.TrimSpace(label.Default); value != "" {
			out["zh-CN"] = value
		}
		if value := strings.TrimSpace(label.Key); value != "" && len(out) == 0 {
			out["zh-CN"] = value
		}
	}
	if value := strings.TrimSpace(title); value != "" {
		out["zh-CN"] = value
	}
	if len(out) == 0 {
		out["zh-CN"] = fallback
	}
	return out
}

func pluginPermissionRowFromPermissionCodeSpec(pluginID, version string, spec plugin_mgr.PermissionSpec, menuValidation pluginMenuPermissionValidation) (modelsiam.Permission, bool) {
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
		"resource":    firstNonEmpty(strings.TrimSpace(spec.Resource), resource),
		"action":      firstNonEmpty(strings.TrimSpace(spec.Action), action),
		"plugin_id":   pluginID,
		"permission":  permissionCode,
		"risk_level":  strings.TrimSpace(spec.RiskLevel),
		"data_scope":  strings.TrimSpace(spec.DataScope),
		"independent": spec.Independent,
	}
	if paths := dedupeNonEmpty(spec.MenuPath); len(paths) > 0 {
		meta["menu_path"] = paths
	}
	if codes := dedupeNonEmpty(spec.PagePermissionCodes); len(codes) > 0 {
		meta["page_permission_codes"] = codes
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
	if errors := pluginPermissionManifestRegistrationErrors(spec, permissionCode, menuValidation); len(errors) > 0 {
		meta["registration_errors"] = errors
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

func pluginMenuPermissionDiagnosticRow(pluginID, version string, diagnostic pluginMenuPolicyDiagnostic) (modelsiam.Permission, bool) {
	code := strings.TrimSpace(diagnostic.PermissionCode)
	if code == "" {
		return modelsiam.Permission{}, false
	}
	module, resource, action, ok := splitPluginPermissionCode(code)
	if !ok {
		module = "menu"
		resource = code
		action = "view"
	}
	meta := map[string]any{
		"type":                "menu",
		"module":              firstNonEmpty(module, "menu"),
		"resource":            resource,
		"action":              action,
		"plugin_id":           pluginID,
		"permission":          code,
		"menu_path":           dedupeNonEmpty(diagnostic.MenuPath),
		"title_i18n":          diagnostic.TitleI18n,
		"description_i18n":    map[string]string{"zh-CN": "菜单声明引用了未登记的菜单权限。"},
		"registration_errors": []string{"menu_permission_declaration_missing"},
	}
	metaBytes, _ := json.Marshal(meta)
	return modelsiam.Permission{
		Module:      module,
		Resource:    resource,
		Action:      action,
		Effect:      "allow",
		Description: "Menu references a missing permission declaration.",
		AllowAPIKey: false,
		Meta:        metaBytes,
		Status:      modelsiam.PermissionStatusActive,
		Source:      "plugin:" + pluginID,
		Introduced:  strings.TrimSpace(version),
	}, true
}

func pluginPermissionManifestRegistrationErrors(spec plugin_mgr.PermissionSpec, permissionCode string, menuValidation pluginMenuPermissionValidation) []string {
	errors := make([]string, 0)
	if strings.TrimSpace(spec.Type) == "menu" {
		expected, referenced := menuValidation.pathsByPermission[permissionCode]
		if !referenced {
			errors = append(errors, "menu_permission_orphan")
		} else if !sameStringSlice(dedupeNonEmpty(spec.MenuPath), expected) {
			errors = append(errors, "menu_path_mismatch")
			errors = append(errors, "menu_path_expected:"+strings.Join(expected, "/"))
			errors = append(errors, "menu_path_actual:"+strings.Join(dedupeNonEmpty(spec.MenuPath), "/"))
		}
		if containsInvalidMenuPathSegment(spec.MenuPath) {
			errors = append(errors, "menu_path_invalid_technical_segment")
		}
	}
	return dedupeNonEmpty(errors)
}

func sameStringSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if strings.TrimSpace(a[idx]) != strings.TrimSpace(b[idx]) {
			return false
		}
	}
	return true
}

func containsInvalidMenuPathSegment(path []string) bool {
	for _, segment := range path {
		value := strings.ToLower(strings.TrimSpace(segment))
		if value == "" {
			continue
		}
		if strings.Contains(value, "com.powerx.plugin") ||
			strings.Contains(value, "com.powerx.plugins") ||
			strings.Contains(value, "/_p/") ||
			strings.HasPrefix(value, "_p/") ||
			strings.Contains(value, "/api/v1") ||
			strings.HasPrefix(value, "api/v1") ||
			strings.HasPrefix(value, "http://") ||
			strings.HasPrefix(value, "https://") {
			return true
		}
	}
	return false
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
