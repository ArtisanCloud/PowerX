package migrate

import (
	"PowerX/internal/model/custom"
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/product"
	"gorm.io/gorm"
)

func AutoMigrateCustom(db *gorm.DB) {
	// migrate your custom models
	// product
	_ = db.AutoMigrate(
		&product.ProductSpecific{},
		&reservationcenter.PivotStoreToService{},
	)
	// reservation center
	_ = db.AutoMigrate(
		&custom.ArtisanSpecific{},
		&reservationcenter.Schedule{},
		&reservationcenter.Reservation{},
		&reservationcenter.CheckinLog{},
	)

}
