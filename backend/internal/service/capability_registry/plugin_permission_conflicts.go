package capability_registry

import (
	"fmt"
	"strings"

	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func pluginPermissionRowKey(row iammodel.Permission) string {
	return strings.Join([]string{row.Module, row.Resource, row.Action}, "\x00")
}

// 先检查整个 catalog，再发布任何 capability。独立 API Key 权限只能由一条
// capability 声明拥有，其他能力可以引用其业务权限，但不能再定义冲突的授权。
func validateCatalogAPIKeyPermissionOwnership(plugin pluginMetadata, capabilities []catalogCapability) error {
	type owner struct {
		capabilityID string
		spec         plugin_mgr.APIKeyPermissionSpec
	}
	owners := map[string]owner{}
	for _, capability := range capabilities {
		for _, raw := range capability.Permissions {
			if raw.APIKey == nil {
				continue
			}
			declaration := normalizePluginPermissionDeclaration(raw)
			row, err := permissionRowFromPluginDeclaration(plugin, capability, declaration)
			if err != nil {
				return fmt.Errorf("capability %s: %w", capability.capabilityID(), err)
			}
			key := pluginPermissionRowKey(row)
			current := owner{capability.capabilityID(), *declaration.APIKey}
			if previous, exists := owners[key]; exists && previous != current {
				return fmt.Errorf("%w: permission=%s capabilities=%s,%s", iamrepo.ErrPluginAPIKeyPermissionConflict, declaration.PermissionCode, previous.capabilityID, current.capabilityID)
			}
			owners[key] = current
		}
	}
	for _, capability := range capabilities {
		for _, declaration := range capability.Permissions {
			module, resource, action, ok := parsePluginPermissionCode(declaration.PermissionCode, plugin.ID)
			if !ok {
				continue // 完整的声明校验由各 capability 的发布校验执行。
			}
			key := strings.Join([]string{module, resource, action}, "\x00")
			if previous, exists := owners[key]; exists && (previous.capabilityID != capability.capabilityID() || declaration.APIKey == nil) {
				return fmt.Errorf("%w: permission=%s capabilities=%s,%s", iamrepo.ErrPluginAPIKeyPermissionConflict, declaration.PermissionCode, previous.capabilityID, capability.capabilityID())
			}
		}
	}
	return nil
}
