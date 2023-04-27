package seed

import (
	"PowerX/internal/model/customerdomain"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateCustomer(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&customerdomain.Customer{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init price book  failed"))
	}

	data := DefaultCustomer()
	if count == 0 {
		if err = db.Model(&customerdomain.Customer{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init customer failed"))
		}
	}
	return err
}

func DefaultCustomer() (data []*customerdomain.Customer) {

	data = []*customerdomain.Customer{
		&customerdomain.Customer{

			Name:        "测试用户",
			Mobile:      "13574839275",
			Email:       "test@test.com",
			InviterID:   0,
			Source:      18,
			Type:        "",
			IsActivated: true,
		},
	}

	return data

}
