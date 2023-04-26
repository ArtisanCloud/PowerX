package seed

import "gorm.io/gorm"

func CreateCustomSeeds(db *gorm.DB) {

	_ = CreateStore(db)
	_ = CreateScheduleConfig(db)
	_ = CreateSchedule(db)
}
