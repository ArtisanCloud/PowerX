package seed

import (
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateServiceSpecific(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&product2.ServiceSpecific{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {

		configs := DefaultServiceSpecific()
		if err := db.Model(&product2.ServiceSpecific{}).Create(&configs).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}

	return err
}

func DefaultServiceSpecific() []*product2.ServiceSpecific {

	configs := []*product2.ServiceSpecific{
		&product2.ServiceSpecific{
			Product: *product.Product{
				Name: "剪发（男）",
				Type: ProductType,
				Plan: "",
				CanSellOnline: "",
				ApprovalStatus: "",
				IsActivated: "",
			},
			ParentId: 0,
			IsFree: false,
			Duration: ,
			MandatoryDuration: ,
		},
	}



	return configs
}
