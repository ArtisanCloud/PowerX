package seed

import (
	"PowerX/internal/model"
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/product"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateServiceSpecific(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&product2.ServiceSpecific{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {

		ucDD := powerx.NewDataDictionaryUseCase(db)
		ctx:=context.Background()
		 = ucDD.GetDataDictionaryItem(ctx,product.ProductType,product.ProductTypeService )
		product.ProductPlanOnce
		options

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
			Product: &product.Product{
				Name: "剪发（男）",
				Type: ,
				Plan: ,
				CanSellOnline: true,
				ApprovalStatus: model.ApprovalStatusActive,
				IsActivated: true,
			},
			ParentId: 0,
			IsFree: false,
			Duration: ,
			MandatoryDuration: ,
		},
	}



	return configs
}
