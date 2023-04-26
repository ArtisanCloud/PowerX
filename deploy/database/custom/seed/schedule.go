package seed

import (
	"PowerX/internal/model/custom/reservationcenter"
	"github.com/golang-module/carbon"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateSchedule(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&reservationcenter.Schedule{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init schedule failed"))
	}
	if count == 0 {
		dep := DefaultSchedule()
		if err := db.Model(&reservationcenter.Schedule{}).Create(&dep).Error; err != nil {
			panic(errors.Wrap(err, "init schedule failed"))
		}
	}

	return err
}

func DefaultSchedule() []*reservationcenter.Schedule {

	today := carbon.Now()
	//today.SetTimezone()
	startOfWeek := today.StartOfWeek()
	endOfWeek := today.EndOfWeek()

	
	
	
	return []*reservationcenter.Schedule{
		&reservationcenter.Schedule{
			StoreId:            1,
			ApprovalStatus:     "",
			Capacity:           10,
			CopyFromScheduleId: 0,
			Name:               "",
			Description:        "",
			IsActive:           "",
			Status:             "",
			StartTime:          carbon.Now().Time,
			EndTime:            carbon.Now().Time,
		},
	}
}

func ()  {
	
}