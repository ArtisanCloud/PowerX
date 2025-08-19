// pkg/corex/db/persistence/model/iam/user_department_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type UserDepartment struct {
	UserID       uint64 `gorm:"column:user_id;primaryKey"      json:"user_id"`
	DepartmentID uint64 `gorm:"column:department_id;primaryKey" json:"department_id"`
	TenantID     uint64 `gorm:"column:tenant_id;index"         json:"tenant_id"`
}

func (mdl *UserDepartment) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMUserDepartment
}
func (mdl *UserDepartment) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMUserDepartment
}
