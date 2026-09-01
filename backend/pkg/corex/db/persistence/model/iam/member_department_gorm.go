// pkg/corex/db/persistence/model/iam/member_department_gorm.go
package iam

import (
	"fmt"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MemberDepartment struct {
	MemberID       uint64    `gorm:"column:member_id;primaryKey"   json:"member_id"`
	DepartmentID   uint64    `gorm:"column:department_id;primaryKey" json:"department_id"`
	TenantUUID     string    `gorm:"column:tenant_uuid;type:char(36);index" json:"tenant_uuid"`
	MemberUUID     uuid.UUID `gorm:"column:member_uuid;type:uuid;uniqueIndex:uk_iam_member_department_member_department_uuid" json:"member_uuid"`
	DepartmentUUID uuid.UUID `gorm:"column:department_uuid;type:uuid;uniqueIndex:uk_iam_member_department_member_department_uuid" json:"department_uuid"`
}

func (mdl *MemberDepartment) BeforeCreate(tx *gorm.DB) error {
	if mdl.MemberUUID == uuid.Nil {
		var member struct {
			UUID uuid.UUID `gorm:"column:uuid"`
		}
		if err := tx.Table((&Member{}).GetTableName(true)).Select("uuid").Where("tenant_uuid = ? AND id = ?", mdl.TenantUUID, mdl.MemberID).First(&member).Error; err != nil {
			return fmt.Errorf("resolve member department member UUID: %w", err)
		}
		if member.UUID == uuid.Nil {
			return fmt.Errorf("member UUID missing")
		}
		mdl.MemberUUID = member.UUID
	}
	if mdl.DepartmentUUID == uuid.Nil {
		var department struct {
			DepartmentUUID uuid.UUID `gorm:"column:department_uuid"`
		}
		if err := tx.Table((&Department{}).GetTableName(true)).Select("department_uuid").Where("tenant_uuid = ? AND id = ?", mdl.TenantUUID, mdl.DepartmentID).First(&department).Error; err != nil {
			return fmt.Errorf("resolve member department UUID: %w", err)
		}
		if department.DepartmentUUID == uuid.Nil {
			return fmt.Errorf("department UUID missing")
		}
		mdl.DepartmentUUID = department.DepartmentUUID
	}
	return nil
}

func (mdl *MemberDepartment) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMMemberDepartment
}
func (mdl *MemberDepartment) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMMemberDepartment
}
