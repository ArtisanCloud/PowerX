package seed

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/product"
	carbon2 "PowerX/pkg/datetime/carbonx"
	"PowerX/pkg/slicex"
	"fmt"
	"github.com/golang-module/carbon/v2"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateSchedule(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&reservationcenter.Schedule{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init schedule failed"))
	}
	if count == 0 {

		stores, err := GetSeedStores(db)
		if err != nil {
			panic(errors.Wrap(err, "get stores failed"))
		}
		schedules := DefaultSchedule(stores)
		//fmt2.Dump(schedules)
		if err := db.Model(&reservationcenter.Schedule{}).Create(&schedules).Error; err != nil {
			panic(errors.Wrap(err, "init schedule failed"))
		}
	}

	return err
}

func GetSeedStores(db *gorm.DB) ([]*product.Store, error) {
	stores := []*product.Store{}
	err := db.Model(&product.Store{}).Find(&stores).Error

	return stores, err

}

func DefaultSchedule(stores []*product.Store) []*reservationcenter.Schedule {
	today := carbon.Now()
	allSchedules := []*reservationcenter.Schedule{}
	for _, store := range stores {
		schedules := GenerateSchedulesBy(&today, store)
		allSchedules = slicex.Concatenate(allSchedules, schedules)
	}

	return allSchedules
}

func GenerateSchedulesBy(currentDate *carbon.Carbon, store *product.Store) []*reservationcenter.Schedule {

	if currentDate.IsInvalid() {
		*currentDate = carbon.Now()
	}

	// 格式化到10点
	formatDate := func(d *carbon.Carbon) *carbon.Carbon {
		d.SetWeekStartsAt(carbon.Sunday)
		*d = d.SetHour(reservationcenter.StartWorkHour).SetMinute(0).SetSecond(0)
		return d
	}

	currentDate = formatDate(currentDate)

	startOfWeek, endOfWeek := carbon2.GetWeekDaysFromDay(currentDate, formatDate)

	//today.SetTimezone()

	//fmt2.Dump(currentDate, startOfWeek, endOfWeek)

	// 工作日
	workDays := int(startOfWeek.DiffInDays(*endOfWeek)) + 1
	//fmt2.Dump(workDays)
	schedules := []*reservationcenter.Schedule{}
	// 7天的工作两
	for i := 0; i < workDays; i++ {
		workDate := startOfWeek.AddDays(i)
		// 6个bucket
		for j := 0; j < reservationcenter.BucketCount; j++ {
			// 每个bucket开始的时间点
			startDatetime := workDate.AddHours(j * reservationcenter.BucketHours)
			schedule := &reservationcenter.Schedule{
				StoreId:            store.Id,
				ApprovalStatus:     "",
				Capacity:           10,
				CopyFromScheduleId: 0,
				Name:               fmt.Sprintf("%s-%d-%s", store.Name, i+1, startDatetime.Format("H")),
				Description:        "",
				IsActive:           true,
				Status:             "",
				StartTime:          startDatetime.ToStdTime(),
				EndTime:            startDatetime.AddHours(reservationcenter.BucketHours).ToStdTime(),
			}
			schedules = append(schedules, schedule)
		}

	}

	return schedules

}
