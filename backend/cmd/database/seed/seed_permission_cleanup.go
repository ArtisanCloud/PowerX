package seed

import (
	"context"
	"fmt"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

func CleanupInvalidPermissionRows(db *gorm.DB) error {
	ctx := seedCtx()
	var ids []uint64
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where(`
			source IN ?
			OR meta->>'type' = ?
			OR source LIKE ?
			OR source LIKE ?
			OR (meta->>'type' = ? AND COALESCE(meta->'title_i18n'->>'zh-CN', meta->'title_i18n'->>'zh', meta->'title_i18n'->>'en', meta->'title_i18n'->>'en-US', '') = '')
		`,
			[]string{"swagger", "platform_capability_generated"},
			"api_candidate",
			"com.powerx.plugin.%",
			"com.powerx.plugins.%",
			"api",
		).
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("query invalid permission rows: %w", err)
	}
	if len(ids) == 0 {
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] invalid permission cleanup skipped")
		return nil
	}

	var deletedRoleBindings int64
	var deletedAPIKeyBindings int64
	var deletedPermissions int64
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		roleBindings := tx.Where("permission_id IN ?", ids).Delete(&dbm.RolePermission{})
		if roleBindings.Error != nil {
			return roleBindings.Error
		}
		apiKeyBindings := tx.Where("permission_id IN ?", ids).Delete(&dbm.APIKeyProfilePermission{})
		if apiKeyBindings.Error != nil {
			return apiKeyBindings.Error
		}
		perms := tx.Where("id IN ?", ids).Delete(&dbm.Permission{})
		if perms.Error != nil {
			return perms.Error
		}
		deletedRoleBindings = roleBindings.RowsAffected
		deletedAPIKeyBindings = apiKeyBindings.RowsAffected
		deletedPermissions = perms.RowsAffected
		return nil
	}); err != nil {
		return fmt.Errorf("delete invalid permission rows: %w", err)
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] invalid permission cleanup ready: permissions=%d role_bindings=%d api_key_bindings=%d", deletedPermissions, deletedRoleBindings, deletedAPIKeyBindings)
	return nil
}

type PermissionSeedAuditReport struct {
	InvalidActiveAPIs      int64
	ActiveGeneratedRows    int64
	ActiveSwaggerRows      int64
	ActiveAPICandidates    int64
	LegacyPluginSourceRows int64
}

func AuditSeededPermissionRows(db *gorm.DB) (*PermissionSeedAuditReport, error) {
	ctx := seedCtx()
	report := &PermissionSeedAuditReport{}
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Where("meta->>'type' = ?", "api").
		Where("COALESCE(meta->'title_i18n'->>'zh-CN', meta->'title_i18n'->>'zh', meta->'title_i18n'->>'en', meta->'title_i18n'->>'en-US', '') = ''").
		Count(&report.InvalidActiveAPIs).Error; err != nil {
		return nil, fmt.Errorf("audit active api i18n: %w", err)
	}
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ? AND source = ?", dbm.PermissionStatusActive, "platform_capability_generated").
		Count(&report.ActiveGeneratedRows).Error; err != nil {
		return nil, fmt.Errorf("audit active generated permissions: %w", err)
	}
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ? AND source = ?", dbm.PermissionStatusActive, "swagger").
		Count(&report.ActiveSwaggerRows).Error; err != nil {
		return nil, fmt.Errorf("audit active swagger permissions: %w", err)
	}
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ? AND meta->>'type' = ?", dbm.PermissionStatusActive, "api_candidate").
		Count(&report.ActiveAPICandidates).Error; err != nil {
		return nil, fmt.Errorf("audit active api candidates: %w", err)
	}
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("source LIKE ? OR source LIKE ?", "com.powerx.plugin.%", "com.powerx.plugins.%").
		Count(&report.LegacyPluginSourceRows).Error; err != nil {
		return nil, fmt.Errorf("audit legacy plugin source permissions: %w", err)
	}
	return report, nil
}

func EnsureSeededPermissionRowsValid(db *gorm.DB) error {
	report, err := AuditSeededPermissionRows(db)
	if err != nil {
		return err
	}
	if report.InvalidActiveAPIs > 0 ||
		report.ActiveGeneratedRows > 0 ||
		report.ActiveSwaggerRows > 0 ||
		report.ActiveAPICandidates > 0 ||
		report.LegacyPluginSourceRows > 0 {
		return fmt.Errorf(
			"permission seed audit failed: invalid_active_apis=%d active_generated=%d active_swagger=%d active_api_candidates=%d legacy_plugin_sources=%d",
			report.InvalidActiveAPIs,
			report.ActiveGeneratedRows,
			report.ActiveSwaggerRows,
			report.ActiveAPICandidates,
			report.LegacyPluginSourceRows,
		)
	}
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] permission audit passed")
	return nil
}
