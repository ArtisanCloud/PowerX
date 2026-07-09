package migration

import (
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

const (
	oldPluginDebugHostPermissionPath = "POST:/api/v1/internal/plugins/host/mock"
	newPluginDebugHostPermissionPath = "POST:/api/v1/internal/plugins/debug-hosts"
)

// EnsurePluginDebugHostPermissionPathMigration rewrites the local plugin debug
// host registration permission from the removed /host/mock path to /debug-hosts.
func EnsurePluginDebugHostPermissionPathMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		schema = "public"
	}

	iamPermission := coremodel.TableIAMPermission
	if db.Migrator().HasTable(iamPermission) {
		tableRef := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent(iamPermission))
		stmt := fmt.Sprintf(
			`UPDATE %s
			 SET meta = replace(meta::text, ?, ?)::jsonb
			 WHERE resource = 'api_key.plugin.debug_host'
			   AND action = 'register'
			   AND meta::text LIKE ?`,
			tableRef,
		)
		if err := db.Exec(stmt, oldPluginDebugHostPermissionPath, newPluginDebugHostPermissionPath, "%"+oldPluginDebugHostPermissionPath+"%").Error; err != nil {
			return fmt.Errorf("rewrite iam debug host permission path failed: %w", err)
		}
	}

	apiKeyPermission := coremodel.TableIntegrationGatewayAPIKeyPermission
	if db.Migrator().HasTable(apiKeyPermission) {
		tableRef := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent(apiKeyPermission))
		stmt := fmt.Sprintf(
			`UPDATE %s
			 SET resource_pattern = ?
			 WHERE scope = '_scope.plugin.debug_host.register'
			   AND action = 'sync'
			   AND resource_type = 'api'
			   AND resource_pattern = ?`,
			tableRef,
		)
		if err := db.Exec(stmt, newPluginDebugHostPermissionPath, oldPluginDebugHostPermissionPath).Error; err != nil {
			return fmt.Errorf("rewrite api key debug host permission snapshot failed: %w", err)
		}
	}

	return nil
}
