package seed

import (
	"PowerX/internal/model/origanzation"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateOrganization(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&origanzation.Department{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {
		dep := DefaultDepartment()
		if err := db.Model(&origanzation.Department{}).Create(&dep).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}

	return err
}

func DefaultDepartment() *origanzation.Department {
	return &origanzation.Department{
		Name:       "组织架构",
		PId:        0,
		Desc:       "根节点, 别删除",
		IsReserved: true,
	}

}

func CreateDefaultDepartments(db *gorm.DB) error {
	departments := DefaultDepartments()
	ucOrg := powerx.NewOrganizationUseCase(db)
	for _, department := range departments {
		_ = ucOrg.CreateDepartment(context.Background(), department)
	}
	return nil
}

func DefaultDepartments() (departments []*origanzation.Department) {
	departments = []*origanzation.Department{
		{
			Name:       "产品部门",
			PId:        1,
			Desc:       "产品经理和产品相关人员",
			IsReserved: false,
		},
		{
			Name:       "技术部门",
			PId:        1,
			Desc:       "",
			IsReserved: false,
		},
		{
			Name:       "运营部门",
			PId:        1,
			Desc:       "",
			IsReserved: false,
		},
		{
			Name:       "人事部门",
			PId:        1,
			Desc:       "",
			IsReserved: false,
		},
		{
			Name:       "财务部门",
			PId:        1,
			Desc:       "",
			IsReserved: false,
		},
	}

	return departments

}
