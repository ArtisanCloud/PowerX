package iam

import (
	"fmt"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Department struct {
	model.PowerModel
	DepartmentUUID       uuid.UUID  `gorm:"column:department_uuid;type:uuid;uniqueIndex:uk_iam_department_department_uuid" json:"department_uuid"`
	ParentDepartmentUUID *uuid.UUID `gorm:"column:parent_department_uuid;type:uuid;index" json:"parent_department_uuid,omitempty"`
	LeaderMemberUUID     *uuid.UUID `gorm:"column:leader_member_uuid;type:uuid;index" json:"leader_member_uuid,omitempty"`

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

func (mdl *Department) BeforeCreate(tx *gorm.DB) error {
	if mdl.DepartmentUUID == uuid.Nil {
		mdl.DepartmentUUID = uuid.New()
	}
	if mdl.ParentID != nil && mdl.ParentDepartmentUUID == nil {
		var parent struct {
			DepartmentUUID uuid.UUID `gorm:"column:department_uuid"`
		}
		if err := tx.Table((&Department{}).GetTableName(true)).Select("department_uuid").Where("tenant_uuid = ? AND id = ?", mdl.TenantUUID, *mdl.ParentID).First(&parent).Error; err != nil {
			return fmt.Errorf("resolve parent department UUID: %w", err)
		}
		if parent.DepartmentUUID == uuid.Nil {
			return fmt.Errorf("parent department UUID missing")
		}
		mdl.ParentDepartmentUUID = &parent.DepartmentUUID
	}
	if mdl.LeaderMemberID != nil && mdl.LeaderMemberUUID == nil {
		var leader struct {
			UUID uuid.UUID `gorm:"column:uuid"`
		}
		if err := tx.Table((&Member{}).GetTableName(true)).Select("uuid").Where("tenant_uuid = ? AND id = ?", mdl.TenantUUID, *mdl.LeaderMemberID).First(&leader).Error; err != nil {
			return fmt.Errorf("resolve department leader UUID: %w", err)
		}
		if leader.UUID == uuid.Nil {
			return fmt.Errorf("department leader UUID missing")
		}
		mdl.LeaderMemberUUID = &leader.UUID
	}
	return nil
}

func (mdl *Department) TableName() string { return model.PowerXSchema + "." + model.TableIAMDepartment }
func (mdl *Department) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMDepartment
}
