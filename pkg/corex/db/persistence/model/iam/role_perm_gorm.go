// pkg/corex/db/persistence/model/iam/role_perm_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type Role struct {
	model.PowerModel

	Scope    string `gorm:"column:scope;type:varchar(16);not null;default:'tenant';index;uniqueIndex:uk_role_scope_tenant_code" json:"scope"`
	TenantID uint64 `gorm:"column:tenant_id;not null;index;uniqueIndex:uk_role_scope_tenant_code"                                 json:"tenant_id"`
	Code     string `gorm:"column:code;type:varchar(64);not null;uniqueIndex:uk_role_scope_tenant_code"                           json:"code"`

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

type Permission struct {
	model.PowerModel

	Plugin      string `gorm:"column:plugin;type:varchar(128);index;uniqueIndex:uk_perm_plugin_resource_action" json:"plugin"`
	Resource    string `gorm:"column:resource;type:varchar(128);index;uniqueIndex:uk_perm_plugin_resource_action" json:"resource"`
	Action      string `gorm:"column:action;type:varchar(64);index;uniqueIndex:uk_perm_plugin_resource_action"  json:"action"`
	Effect      string `gorm:"column:effect;type:varchar(16);default:'allow'"  json:"effect"`
	Description string `gorm:"column:description;type:text"                    json:"description,omitempty"`
}

func (mdl *Permission) TableName() string { return model.PowerXSchema + "." + model.TableIAMPermission }
func (mdl *Permission) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMPermission
}

type RolePermission struct {
	RoleID       uint64 `gorm:"column:role_id;primaryKey"       json:"role_id"`
	PermissionID uint64 `gorm:"column:permission_id;primaryKey" json:"permission_id"`

	CreatedAt int64 `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
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

type MemberRole struct {
	MemberID  uint64 `gorm:"column:member_id;primaryKey" json:"member_id"`
	RoleID    uint64 `gorm:"column:role_id;primaryKey"   json:"role_id"`
	TenantID  uint64 `gorm:"column:tenant_id;index"      json:"tenant_id"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
}

func (mr *MemberRole) TableName() string { return model.PowerXSchema + "." + model.TableMemberRole }
func (mr *MemberRole) GetTableName(needFull bool) string {
	if needFull {
		return mr.TableName()
	}
	return model.TableMemberRole
}
