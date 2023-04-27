package migrate

import (
	"PowerX/internal/model/custom"
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/product"
	"gorm.io/gorm"
)

func AutoMigrateCustom(db *gorm.DB) {
	// migrate your custom models
	// product
	_ = db.AutoMigrate(
		&product.ProductSpecific{},
		&product2.ServiceSpecific{},
		&reservationcenter.PivotStoreToService{},
	)
	// reservation center
	_ = db.AutoMigrate(
		&custom.ArtisanSpecific{},
		&reservationcenter.Schedule{},
		&reservationcenter.PivotScheduleToArtisan{},
		&reservationcenter.Reservation{},
		&reservationcenter.CheckinLog{},
	)

}
