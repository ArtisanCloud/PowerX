package seed

import "gorm.io/gorm"

func CreateCustomSeeds(db *gorm.DB) {

	_ = CreateStore(db)
}
