package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type Department struct {
	model.PowerModel

	TenantUUID     string         `gorm:"column:tenant_uuid;type:char(36);not null;index;uniqueIndex:uk_dept_tenant_key" json:"tenant_uuid"`
	Key            string         `gorm:"column:key;type:varchar(64);not null;uniqueIndex:uk_dept_tenant_key" json:"key"`
	Name           string         `gorm:"column:name;type:varchar(128);not null"                         json:"name"`
	ParentID       *uint64        `gorm:"column:parent_id;index"                                         json:"parent_id,omitempty"`
	Path           string         `gorm:"column:path;type:varchar(512);index"                            json:"path"` // 形如 /1/3/5/
	Depth          int            `gorm:"column:depth;index"                                             json:"depth"`
	LeaderMemberID *uint64        `gorm:"column:leader_member_id;index"                                  json:"leader_member_id,omitempty"`
	Sort           int            `gorm:"column:sort;default:0;index"                                    json:"sort"`
	Status         int16          `gorm:"column:status;default:1;index"                                  json:"status"` // 1=active
	Meta           datatypes.JSON `gorm:"column:meta;type:jsonb"                                                    json:"meta,omitempty"`
	Children       []*Department  `gorm:"-" json:"children,omitempty"`
}

func (mdl *Department) TableName() string { return model.PowerXSchema + "." + model.TableIAMDepartment }
func (mdl *Department) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMDepartment
}
