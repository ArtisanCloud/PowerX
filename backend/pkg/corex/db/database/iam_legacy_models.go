package database

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// These models define the pre-Phase-2 table shape for the centralized
// AutoMigrate pass. UUID columns are deliberately created only by the versioned
// IAM migration, never as a side effect of runtime model AutoMigrate.
type legacyIAMPermission struct {
	coremodel.PowerModel
	Module       string         `gorm:"column:module;type:varchar(255);not null;index:uk_perm_plugin_res_act,unique"`
	Resource     string         `gorm:"type:varchar(255);not null;index:uk_perm_plugin_res_act,unique"`
	Action       string         `gorm:"type:varchar(255);not null;index:uk_perm_plugin_res_act,unique"`
	Effect       string         `gorm:"type:varchar(16);not null;default:'allow'"`
	Description  string         `gorm:"type:varchar(255)"`
	AllowAPIKey  bool           `gorm:"column:allow_api_key;not null;default:false;index"`
	Meta         datatypes.JSON `gorm:"type:jsonb"`
	Status       string         `gorm:"type:varchar(16);not null;default:'active';index"`
	Source       string         `gorm:"type:varchar(128);index"`
	Introduced   string         `gorm:"type:varchar(32)"`
	DeprecatedAt *int64         `gorm:"index"`
}

func (legacyIAMPermission) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMPermission
}

type legacyIAMDepartment struct {
	coremodel.PowerModel
	TenantUUID     string         `gorm:"column:tenant_uuid;type:char(36);not null;index;uniqueIndex:uk_dept_tenant_key"`
	Key            string         `gorm:"column:key;type:varchar(64);not null;uniqueIndex:uk_dept_tenant_key"`
	Name           string         `gorm:"column:name;type:varchar(128);not null"`
	ParentID       *uint64        `gorm:"column:parent_id;index"`
	Path           string         `gorm:"column:path;type:varchar(512);index"`
	Depth          int            `gorm:"column:depth;index"`
	LeaderMemberID *uint64        `gorm:"column:leader_member_id;index"`
	Sort           int            `gorm:"column:sort;default:0;index"`
	Status         int16          `gorm:"column:status;default:1;index"`
	Meta           datatypes.JSON `gorm:"column:meta;type:jsonb"`
}

func (legacyIAMDepartment) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMDepartment
}

type legacyIAMRolePermission struct {
	RoleID       uint64 `gorm:"column:role_id;primaryKey"`
	PermissionID uint64 `gorm:"column:permission_id;primaryKey"`
	CreatedAt    int64  `gorm:"column:created_at;autoCreateTime:milli"`
}

func (legacyIAMRolePermission) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMRolePermission
}

type legacyIAMMemberDepartment struct {
	MemberID     uint64 `gorm:"column:member_id;primaryKey"`
	DepartmentID uint64 `gorm:"column:department_id;primaryKey"`
	TenantUUID   string `gorm:"column:tenant_uuid;type:char(36);index"`
}

func (legacyIAMMemberDepartment) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMMemberDepartment
}
