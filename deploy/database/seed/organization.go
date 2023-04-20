package seed

import (
	"PowerX/internal/model/scrm/organization"
)

func DefaultDepartment() *organization.Department {
	return &organization.Department{
		Name:       "组织架构",
		PId:        0,
		Desc:       "根节点, 别删除",
		IsReserved: true,
	}

}
