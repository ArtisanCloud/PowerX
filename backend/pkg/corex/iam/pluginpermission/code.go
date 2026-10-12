package pluginpermission

import (
	"encoding/json"
	"strings"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

// EffectivePluginPermissionCode is shared by the proxy and delegated identity
// snapshots; technical route permissions cannot become business grants.
func EffectivePluginPermissionCode(row models.Permission) string {
	var meta map[string]any
	if len(row.Meta) > 0 && json.Unmarshal(row.Meta, &meta) != nil {
		return ""
	}
	if meta["type"] == "api" {
		if code, ok := meta["business_permission_code"].(string); ok && strings.TrimSpace(code) != "" {
			left, action, valid := strings.Cut(strings.TrimSpace(code), ":")
			if !valid || strings.TrimSpace(left) == "" || strings.TrimSpace(action) == "" {
				return ""
			}
			return strings.TrimSpace(left) + ":" + strings.TrimSpace(action)
		}
		independent := meta["independent"] == true || meta["independent"] == "true"
		if !independent {
			return ""
		}
	}
	resource, action := strings.TrimSpace(row.Resource), strings.TrimSpace(row.Action)
	if resource == "" || action == "" {
		return ""
	}
	if row.Module != "" {
		resource = strings.TrimSpace(row.Module) + "." + resource
	}
	return resource + ":" + action
}
