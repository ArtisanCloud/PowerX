// pkg/corex/db/persistence/model/iam/role_binding_gorm.go
package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type SubjectType string

const (
	SubMember   SubjectType = "MEMBER"
	SubOrg      SubjectType = "ORG_UNIT"
	SubTeam     SubjectType = "TEAM"
	SubPosition SubjectType = "POSITION"
	SubGroup    SubjectType = "GROUP"
	SubService  SubjectType = "SERVICE_ACCOUNT"
)

type DataScope string

const (
	ScopeAll      DataScope = "ALL"
	ScopeTenant   DataScope = "TENANT"
	ScopeOrgSelf  DataScope = "ORG_SELF"
	ScopeOrgSub   DataScope = "ORG_SUBTREE"
	ScopeTeam     DataScope = "TEAM"
	ScopePosition DataScope = "POSITION"
	ScopeGroup    DataScope = "GROUP"
	ScopeSelf     DataScope = "SELF"
	ScopeCustom   DataScope = "CUSTOM"
)

type RoleBinding struct {
	ID          uint64      `gorm:"primaryKey"`
	TenantUUID  string      `gorm:"column:tenant_uuid;type:char(36);index;not null"`
	RoleUUID    string      `gorm:"column:role_uuid;type:char(36);index;not null;default:''"`
	RoleID      uint64      `gorm:"index;not null"`
	SubjectType SubjectType `gorm:"type:varchar(24);index;not null"`
	SubjectUUID string      `gorm:"column:subject_uuid;type:char(36);index;not null;default:''"`
	SubjectID   uint64      `gorm:"index;not null"`

	// 数据域与条件
	DataScope DataScope      `gorm:"type:varchar(24);index;not null;default:'TENANT'"`
	ScopeDim  *string        `gorm:"type:varchar(16)"` // ORG/TEAM/POSITION/GROUP，用于限定 DataScope 时的参照维度
	ScopeID   *uint64        `gorm:"index"`
	Condition datatypes.JSON `gorm:"type:jsonb"` // ABAC 条件（按需解析拼 WHERE）

	// 有效期（可选）
	StartAt *int64 `gorm:"index"`
	EndAt   *int64 `gorm:"index"`
}

func (m *RoleBinding) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRoleBinding
}

func (m *RoleBinding) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return model.TableIAMRoleBinding
}
