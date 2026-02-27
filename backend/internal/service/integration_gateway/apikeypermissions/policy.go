package apikeypermissions

import (
	"encoding/json"
	"fmt"
	"strings"

	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

type ResolvedPermission struct {
	Scope           string
	Action          string
	ResourceType    string
	ResourcePattern string
	PluginID        string
	Effect          string
}

func IsCoreSensitivePermission(permission modelsiam.Permission) bool {
	module := strings.ToLower(strings.TrimSpace(permission.Module))
	resource := strings.ToLower(strings.TrimSpace(permission.Resource))
	action := strings.ToLower(strings.TrimSpace(permission.Action))

	if module == "system" && resource == "root" {
		return true
	}
	if module == "iam" && resource == "credential" {
		return true
	}
	if module == "iam" && resource == "permission" && action != "read" && action != "list" {
		return true
	}

	meta := parseMeta(permission.Meta)
	endpoint := strings.TrimSpace(anyToString(meta["api_endpoint"]))
	if endpoint == "" {
		return false
	}
	for _, prefix := range sensitiveEndpointPrefixes {
		if strings.HasPrefix(endpoint, prefix) {
			return true
		}
	}
	return false
}

func DefaultAllowAPIKey(permission modelsiam.Permission) bool {
	if strings.TrimSpace(permission.Module) == "" {
		return false
	}
	if strings.TrimSpace(permission.Resource) == "" || strings.TrimSpace(permission.Action) == "" {
		return false
	}
	return !IsCoreSensitivePermission(permission)
}

func ResolvePermission(permission modelsiam.Permission) (ResolvedPermission, bool) {
	meta := parseMeta(permission.Meta)
	if resolved, ok := resolveFromExplicitMeta(meta); ok {
		return resolved, true
	}
	if !permission.AllowAPIKey {
		return ResolvedPermission{}, false
	}
	if resolved, ok := resolveFromPermission(permission, meta); ok {
		return resolved, true
	}
	return ResolvedPermission{}, false
}

func BuildAPIKeyMeta(permission modelsiam.Permission) map[string]any {
	resolved, ok := ResolvePermission(permission)
	if !ok {
		return nil
	}
	return map[string]any{
		"scope":            resolved.Scope,
		"action":           resolved.Action,
		"resource_type":    resolved.ResourceType,
		"resource_pattern": resolved.ResourcePattern,
		"plugin_id":        resolved.PluginID,
		"effect":           resolved.Effect,
	}
}

var sensitiveEndpointPrefixes = []string{
	"/api/v1/admin/user/auth/",
	"/api/v1/admin/integration/api-keys",
	"/api/v1/admin/integration/api-key-profiles",
	"/api/v1/admin/iam/permissions",
	"/api/v1/admin/iam/roles",
}

func parseMeta(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func resolveFromExplicitMeta(meta map[string]any) (ResolvedPermission, bool) {
	raw, ok := meta["api_key"]
	if !ok {
		return ResolvedPermission{}, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return ResolvedPermission{}, false
	}
	resolved := ResolvedPermission{
		Scope:           strings.TrimSpace(anyToString(m["scope"])),
		Action:          strings.TrimSpace(anyToString(m["action"])),
		ResourceType:    strings.TrimSpace(anyToString(m["resource_type"])),
		ResourcePattern: strings.TrimSpace(anyToString(m["resource_pattern"])),
		PluginID:        strings.TrimSpace(anyToString(m["plugin_id"])),
		Effect:          strings.TrimSpace(anyToString(m["effect"])),
	}
	if resolved.Effect == "" {
		resolved.Effect = "allow"
	}
	if resolved.Scope == "" || resolved.Action == "" || resolved.ResourceType == "" || resolved.ResourcePattern == "" {
		return ResolvedPermission{}, false
	}
	return resolved, true
}

func resolveFromPermission(permission modelsiam.Permission, meta map[string]any) (ResolvedPermission, bool) {
	module := strings.TrimSpace(permission.Module)
	resource := strings.TrimSpace(permission.Resource)
	action := strings.TrimSpace(permission.Action)
	if module == "" || resource == "" || action == "" {
		return ResolvedPermission{}, false
	}
	httpMethod := strings.ToUpper(strings.TrimSpace(anyToString(meta["http_method"])))
	apiEndpoint := strings.TrimSpace(anyToString(meta["api_endpoint"]))
	resourcePattern := ""
	if httpMethod != "" && apiEndpoint != "" {
		resourcePattern = fmt.Sprintf("%s:%s", httpMethod, apiEndpoint)
	} else {
		resourcePattern = fmt.Sprintf("%s:%s.%s", strings.ToUpper(action), module, resource)
	}
	return ResolvedPermission{
		Scope:           fmt.Sprintf("_scope.%s.%s.%s", module, resource, action),
		Action:          strings.ToLower(action),
		ResourceType:    "api",
		ResourcePattern: resourcePattern,
		Effect:          "allow",
	}, true
}

func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
