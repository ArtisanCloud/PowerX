package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// pkg/corex/db/persistence/model/iam/member_assignment_gorm.go

type AssignmentDim string

const (
	DimOrg      AssignmentDim = "ORG"
	DimTeam     AssignmentDim = "TEAM"
	DimPosition AssignmentDim = "POSITION"
	DimGroup    AssignmentDim = "GROUP"
)

type MemberAssignment struct {
	ID       uint64        `gorm:"primaryKey"`
	TenantID uint64        `gorm:"column:tenant_id;not null;index"`
	MemberID uint64        `gorm:"column:member_id;not null;index"`
	DimType  AssignmentDim `gorm:"column:dim_type;type:varchar(16);not null;index"`
	DimID    uint64        `gorm:"column:dim_id;not null;index"`

	StartAt *int64         `gorm:"column:start_at"`
	EndAt   *int64         `gorm:"column:end_at"`
	Attrs   datatypes.JSON `gorm:"column:attrs;type:jsonb"`

	// 防重复：同一租户/成员/维度/对象 同时仅一条有效
	// gorm: uniqueIndex:uk_member_assignment,composite:(tenant_id,member_id,dim_type,dim_id)
}

func (m *MemberAssignment) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMMemberAssignment
}
func (m *MemberAssignment) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return model.TableIAMMemberAssignment
}
