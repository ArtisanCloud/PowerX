package seed

import (
	"PowerX/internal/model/scrm/organization"
	"github.com/pkg/errors"
)

func CreateOrganization(s *PowerSeeder) (err error) {
	var count int64
	if err = s.db.Model(&organization.Department{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {
		dep := DefaultDepartment()
		if err := s.db.Model(&organization.Department{}).Create(&dep).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}

	return err
}

func DefaultDepartment() *organization.Department {
	return &organization.Department{
		Name:       "组织架构",
		PId:        0,
		Desc:       "根节点, 别删除",
		IsReserved: true,
	}

}
