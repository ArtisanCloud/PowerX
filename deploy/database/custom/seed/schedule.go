package seed

import (
	"PowerX/internal/model/custom/reservationcenter"
	reservationcenter2 "PowerX/internal/uc/custom/reservationcenter"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateSchedule(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&reservationcenter.Schedule{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init schedule failed"))
	}
	if count == 0 {

		ucSchedule := reservationcenter2.NewScheduleUseCase(db)
		_, err = ucSchedule.InitSchedules()
		if err != nil {
			panic(errors.Wrap(err, "seed init schedules failed"))
		}

	}

	return err
}
