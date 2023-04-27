package seed

import "gorm.io/gorm"

func CreateCustomSeeds(db *gorm.DB) {

	_ = CreateStore(db)
	_ = CreateServiceSpecific(db)
	_ = CreateSchedule(db)
}
