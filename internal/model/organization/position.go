package organization

import (
	"PowerX/internal/model"
	"PowerX/internal/model/permission"
)

// Position 职位
type Position struct {
	model.Model
	Name  string                  `gorm:"comment:职位名称;column:name" json:"name"`
	Desc  string                  `gorm:"comment:描述;column:desc" json:"desc"`
	Roles []*permission.AdminRole `gorm:"many2many:position_roles;foreignKey:Id;References:RoleCode" json:"roles"`
	Level string                  `gorm:"comment:职级;column:level" json:"level"`
}
