// pkg/corex/db/persistence/model/iam/member_department_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type MemberDepartment struct {
	MemberID     uint64 `gorm:"column:member_id;primaryKey"   json:"member_id"`
	DepartmentID uint64 `gorm:"column:department_id;primaryKey" json:"department_id"`
	TenantID     uint64 `gorm:"column:tenant_id;index"        json:"tenant_id"`
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
