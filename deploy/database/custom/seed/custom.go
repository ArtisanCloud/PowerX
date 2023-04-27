package seed

import "gorm.io/gorm"

func CreateCustomSeeds(db *gorm.DB) {

	//_ = CreateServiceSpecific(db)
	_ = CreateStore(db)
	_ = CreateSchedule(db)
}
