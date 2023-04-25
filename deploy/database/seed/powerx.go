package seed

import (
	"PowerX/deploy/database/custom/seed"
	"gorm.io/gorm"
)

func CreatePowerX(db *gorm.DB) (err error) {

	_ = CreateOrganization(db)
	_ = CreateDataDictionaries(db)
	_ = CreatePriceBooks(db)
	_ = CreateProductCategories(db)

	// Custom
	_ = seed.CreateStore(db)

	return
}
