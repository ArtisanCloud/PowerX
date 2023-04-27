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

		products := []*product.Product{}
		if err := db.Model(&product.Store{}).Find(&products).Error; err != nil {
			panic(errors.Wrap(err, "get init products failed"))
		}

		configs := DefaultScheduleConfig(products)
		if err := db.Model(&product2.ServiceSpecific{}).Create(&configs).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}

	return err
}

func DefaultScheduleConfig(products []*product.Product) []*product2.ServiceSpecific {

	configs := []*product2.ServiceSpecific{}
	for _, product := range products {
		config := &product2.ServiceSpecific{
			ProductId: product.Id,
			//Capacity:    10,
			//Name:        store.Name + "-日程配置表",
			//Description: store.Name + "-日程配置表",
			//IsActive:    true,
		}

		configs = append(configs, config)
	}

	return configs
}
