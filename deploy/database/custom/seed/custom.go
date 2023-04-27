package seed

import "gorm.io/gorm"

func CreateCustomSeeds(db *gorm.DB) {

	_ = CreateCustomer(db)
	_ = CreateStore(db)
	_ = CreateServiceSpecific(db)
	_ = CreateSchedule(db)
}
