package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPluginAPIKeyPermissionConflict = errors.New("PLUGIN_API_KEY_PERMISSION_CONFLICT")

type pluginCapabilityPermissionMeta struct {
	Type           string `json:"type"`
	CapabilityID   string `json:"capability_id"`
	APIKeyExplicit bool   `json:"api_key_explicit"`
}

// UpsertPluginCapabilityPermissions 在同一事务内保护显式权限。返回实际写入的
// 行，调用方只为这些行补默认角色，避免推导引用顺便修改其他声明的角色授权。
func (r *PermissionRepository) UpsertPluginCapabilityPermissions(ctx context.Context, rows []dbm.Permission) ([]dbm.Permission, error) {
	var accepted []dbm.Permission
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if !strings.HasPrefix(row.Source, "plugin:") || strings.TrimPrefix(row.Source, "plugin:") == "" {
				return fmt.Errorf("plugin permission source required")
			}
			var incoming pluginCapabilityPermissionMeta
			if len(row.Meta) > 0 && json.Unmarshal(row.Meta, &incoming) != nil {
				return fmt.Errorf("plugin permission metadata invalid")
			}
			// 并发首次创建时由唯一键确定写入者，再锁定现有行做同一套优先级检查。
			created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "module"}, {Name: "resource"}, {Name: "action"}}, DoNothing: true}).Create(&row)
			if created.Error != nil {
				return created.Error
			}
			if created.RowsAffected == 1 {
				accepted = append(accepted, row)
				continue
			}
			var old dbm.Permission
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("module = ? AND resource = ? AND action = ?", row.Module, row.Resource, row.Action).First(&old).Error; err != nil {
				return err
			}
			var existing pluginCapabilityPermissionMeta
			if len(old.Meta) > 0 && json.Unmarshal(old.Meta, &existing) != nil {
				return fmt.Errorf("existing plugin permission metadata invalid")
			}
			if incoming.Type == "plugin_capability" && (old.Source != row.Source || existing.Type != "plugin_capability" || existing.APIKeyExplicit) {
				continue
			}
			if incoming.APIKeyExplicit || existing.APIKeyExplicit {
				differentOwner := old.Source != row.Source
				differentDeclaration := existing.Type != "plugin_capability" && existing.CapabilityID != "" && incoming.CapabilityID != "" && existing.CapabilityID != incoming.CapabilityID
				if differentOwner || differentDeclaration {
					return fmt.Errorf("%w: permission=%s.%s:%s existing_capability=%s incoming_capability=%s", ErrPluginAPIKeyPermissionConflict, row.Module, row.Resource, row.Action, existing.CapabilityID, incoming.CapabilityID)
				}
			}
			if err := NewPermissionRepository(tx).UpsertBatch(ctx, []dbm.Permission{row}); err != nil {
				return err
			}
			accepted = append(accepted, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return accepted, nil
}
