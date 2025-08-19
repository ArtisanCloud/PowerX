package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type Department struct {
	model.PowerModel

	TenantID     uint64         `gorm:"column:tenant_id;index;not null"                               json:"tenant_id"`
	Key          string         `gorm:"column:key;type:varchar(64);index:uk_dept_tenant_key,unique,priority:2" json:"key"` // 部门唯一编码（租户内唯一）
	Name         string         `gorm:"column:name;type:varchar(128);not null"                         json:"name"`
	ParentID     *uint64        `gorm:"column:parent_id;index"                                         json:"parent_id,omitempty"`
	Path         string         `gorm:"column:path;type:varchar(512);index"                            json:"path"` // 形如 /1/3/5/
	Depth        int            `gorm:"column:depth;index"                                             json:"depth"`
	LeaderUserID *uint64        `gorm:"column:leader_user_id;index"                                    json:"leader_user_id,omitempty"`
	Sort         int            `gorm:"column:sort;default:0;index"                                    json:"sort"`
	Status       int16          `gorm:"column:status;default:1;index"                                  json:"status"` // 1=active
	Meta         datatypes.JSON `gorm:"column:meta"                                                    json:"meta,omitempty"`
}

func (mdl *Department) TableName() string { return model.PowerXSchema + "." + model.TableIAMDepartment }
func (mdl *Department) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMDepartment
}
