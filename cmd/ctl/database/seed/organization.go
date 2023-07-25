package seed

import (
	"PowerX/internal/model/origanzation"
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
