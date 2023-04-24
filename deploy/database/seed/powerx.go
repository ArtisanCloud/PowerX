package seed

import (
	"gorm.io/gorm"
)

func CreatePowerX(db *gorm.DB) (err error) {

	_ = CreateOrganization(db)
	_ = CreateDataDictionaries(db)
	_ = CreatePriceBooks(db)
	_ = CreateProductCategories(db)

	return
}
