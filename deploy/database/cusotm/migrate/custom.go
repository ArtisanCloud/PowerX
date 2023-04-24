package migrate

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/product"
	"gorm.io/gorm"
)

func AutoMigrateCustom(db *gorm.DB) {
	// migrate your seed models
	// product
	_ = db.AutoMigrate(&product.ProductSpecific{})
	// reservation center
	_ = db.AutoMigrate(
		&reservationcenter.Artisan{},
		&reservationcenter.Schedule{},
		&reservationcenter.Reservation{},
		&reservationcenter.CheckinLog{},
	)

}
