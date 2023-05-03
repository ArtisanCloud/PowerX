package uc

import (
	"PowerX/internal/config"
	productCustomUC "PowerX/internal/uc/custom/product"
	reservationCenterCustomUC "PowerX/internal/uc/custom/reservationcenter"
	fmt "PowerX/pkg/printx"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type CustomUseCase struct {
	db              *gorm.DB
	Cron            *cron.Cron
	Schedule        *reservationCenterCustomUC.ScheduleUseCase
	Reservation     *reservationCenterCustomUC.ReservationUseCase
	CheckinLog      *reservationCenterCustomUC.CheckinLogUseCase
	ArtisanSpecific *productCustomUC.ArtisanSpecificUseCase
	ServiceSpecific *productCustomUC.ServiceSpecificUseCase
}

func NewCustomUseCase(conf *config.Config) (uc *CustomUseCase, clean func()) {
	// 启动数据库并测试连通性
	db, err := gorm.Open(postgres.Open(conf.PowerXDatabase.DSN), &gorm.Config{
		//Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		panic(errors.Wrap(err, "connect database failed"))
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(errors.Wrap(err, "get sql db failed"))
	}
	err = sqlDB.Ping()
	if err != nil {
		panic(errors.Wrap(err, "ping database failed"))
	}

	c := cron.New()
	uc = &CustomUseCase{
		db:   db,
		Cron: c,
	}
	uc.Cron.Start()

	// 加载预约中心UseCase
	uc.Schedule = reservationCenterCustomUC.NewScheduleUseCase(db)
	eventTriggerPeriod := "@hourly"
	//eventTriggerPeriod := "@every 1m"
	uc.Cron.AddFunc(eventTriggerPeriod, func() {
		_, err := uc.Schedule.InitSchedules()
		if err != nil {
			fmt.Dump(errors.Wrap(err, "crontab error occurs on init schedules").Error())
		}
	})

	// 加载预约服务
	uc.Reservation = reservationCenterCustomUC.NewReservationUseCase(db)
	uc.CheckinLog = reservationCenterCustomUC.NewCheckinLogUseCase(db)
	uc.ArtisanSpecific = productCustomUC.NewArtisanSpecificUseCase(db)

	// 加载服务UseCase
	uc.ServiceSpecific = productCustomUC.NewServiceSpecificUseCase(db)

	return uc, func() {
		_ = sqlDB.Close()
		_ = uc.Cron.Stop()
	}
}
