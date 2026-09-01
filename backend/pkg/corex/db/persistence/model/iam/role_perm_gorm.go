// pkg/corex/db/persistence/model/iam/role_perm_gorm.go
package iam

import (
	"fmt"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Role struct {
	model.PowerUUIDModel

	Scope      string       `gorm:"column:scope;type:varchar(16);not null;default:'tenant';index;uniqueIndex:uk_role_scope_tenant_code" json:"scope"`
	TenantUUID string       `gorm:"column:tenant_uuid;type:char(36);not null;default:'';index;uniqueIndex:uk_role_scope_tenant_code"   json:"tenant_uuid"`
	Code       iam.RoleCode `gorm:"column:code;type:varchar(64);not null;uniqueIndex:uk_role_scope_tenant_code"                         json:"code"`

	Name        string `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Description string `gorm:"column:description;type:text"           json:"description,omitempty"`
	Builtin     bool   `gorm:"column:builtin;default:false;index"     json:"builtin"`
}

func (mdl *Role) TableName() string { return model.PowerXSchema + "." + model.TableIAMRole }
func (mdl *Role) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRole
}

type PermissionStatus string

const (
	PermissionStatusActive     PermissionStatus = "active"
	PermissionStatusDeprecated PermissionStatus = "deprecated"
)

type Permission struct {
	model.PowerModel
	PermissionUUID uuid.UUID `gorm:"column:permission_uuid;type:uuid;uniqueIndex:uk_iam_permission_permission_uuid" json:"permission_uuid"`

	Module      string `gorm:"column:module;type:varchar(255);not null;index:uk_perm_plugin_res_act,unique" json:"module"`
	Resource    string `gorm:"type:varchar(255);not null;index:uk_perm_plugin_res_act,unique" json:"resource"`
	Action      string `gorm:"type:varchar(255);not null;index:uk_perm_plugin_res_act,unique" json:"action"`
	Effect      string `gorm:"type:varchar(16);not null;default:'allow'" json:"effect"`
	Description string `gorm:"type:varchar(255)" json:"description"`
	AllowAPIKey bool   `gorm:"column:allow_api_key;not null;default:false;index" json:"allow_api_key"`

	// —— 新增字段 ——
	Meta         datatypes.JSON   `gorm:"type:jsonb" json:"meta,omitempty"` // UI 元数据（label/module/type/api_endpoint/http_method…）
	Status       PermissionStatus `gorm:"type:varchar(16);not null;default:'active';index" json:"status"`
	Source       string           `gorm:"type:varchar(128);index" json:"source"` // 核心/插件ID
	Introduced   string           `gorm:"type:varchar(32)" json:"introduced"`    // 首次引入版本，如 v1.8.0
	DeprecatedAt *int64           `gorm:"index" json:"deprecated_at,omitempty"`  // 废弃时间戳（秒）
}

func (mdl *Permission) BeforeCreate(tx *gorm.DB) error {
	if mdl.PermissionUUID == uuid.Nil {
		mdl.PermissionUUID = uuid.New()
	}
	return nil
}

func (mdl *Permission) TableName() string { return model.PowerXSchema + "." + model.TableIAMPermission }
func (mdl *Permission) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMPermission
}

type RolePermission struct {
	RoleID         uint64    `gorm:"column:role_id;primaryKey"       json:"role_id"`
	PermissionID   uint64    `gorm:"column:permission_id;primaryKey" json:"permission_id"`
	RoleUUID       uuid.UUID `gorm:"column:role_uuid;type:uuid;uniqueIndex:uk_iam_role_permission_role_permission_uuid" json:"role_uuid"`
	PermissionUUID uuid.UUID `gorm:"column:permission_uuid;type:uuid;uniqueIndex:uk_iam_role_permission_role_permission_uuid" json:"permission_uuid"`

	CreatedAt int64 `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
}

func (mdl *RolePermission) BeforeCreate(tx *gorm.DB) error {
	if mdl.RoleUUID == uuid.Nil {
		var role struct {
			UUID uuid.UUID `gorm:"column:uuid"`
		}
		if err := tx.Table((&Role{}).GetTableName(true)).Select("uuid").Where("id = ?", mdl.RoleID).First(&role).Error; err != nil {
			return fmt.Errorf("resolve IAM role UUID: %w", err)
		}
		if role.UUID == uuid.Nil {
			return fmt.Errorf("IAM role UUID missing")
		}
		mdl.RoleUUID = role.UUID
	}
	if mdl.PermissionUUID == uuid.Nil {
		var permission struct {
			PermissionUUID uuid.UUID `gorm:"column:permission_uuid"`
		}
		if err := tx.Table((&Permission{}).GetTableName(true)).Select("permission_uuid").Where("id = ?", mdl.PermissionID).First(&permission).Error; err != nil {
			return fmt.Errorf("resolve IAM permission UUID: %w", err)
		}
		if permission.PermissionUUID == uuid.Nil {
			return fmt.Errorf("IAM permission UUID missing")
		}
		mdl.PermissionUUID = permission.PermissionUUID
	}
	return nil
}

func (mdl *RolePermission) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRolePermission
}
func (mdl *RolePermission) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRolePermission
}
