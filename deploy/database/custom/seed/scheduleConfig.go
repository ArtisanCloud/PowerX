package seed

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateScheduleConfig(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&reservationcenter.ScheduleConfig{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {

		stores := []*product.Store{}
		if err := db.Model(&product.Store{}).Find(&stores).Error; err != nil {
			panic(errors.Wrap(err, "get init stores failed"))
		}

		configs := DefaultScheduleConfig(stores)
		if err := db.Model(&reservationcenter.ScheduleConfig{}).Create(&configs).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}

	return err
}

func DefaultScheduleConfig(stores []*product.Store) []*reservationcenter.ScheduleConfig {

	configs := []*reservationcenter.ScheduleConfig{}
	for _, store := range stores {
		config := &reservationcenter.ScheduleConfig{
			StoreId:     store.Id,
			Capacity:    10,
			Name:        store.Name + "-日程配置表",
			Description: store.Name + "-日程配置表",
			IsActive:    true,
		}

		configs = append(configs, config)
	}

	return configs
}
