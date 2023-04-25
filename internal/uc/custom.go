package uc

import (
	"PowerX/internal/config"
	productCustomUC "PowerX/internal/uc/custom/product"
	reservationCenterCustomUC "PowerX/internal/uc/custom/reservationcenter"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type CustomUseCase struct {
	db              *gorm.DB
	Schedule        *reservationCenterCustomUC.ScheduleUseCase
	ScheduleConfig  *reservationCenterCustomUC.ScheduleConfigUseCase
	Reservation     *reservationCenterCustomUC.ReservationUseCase
	CheckinLog      *reservationCenterCustomUC.CheckinLogUseCase
	ArtisanSpecific *productCustomUC.ArtisanSpecificUseCase
	Service         *productCustomUC.ServiceSpecificUseCase
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

	uc = &CustomUseCase{
		db: db,
	}

	// 加载预约中心UseCase
	uc.Schedule = reservationCenterCustomUC.NewScheduleUseCase(db)
	uc.Reservation = reservationCenterCustomUC.NewReservationUseCase(db)
	uc.CheckinLog = reservationCenterCustomUC.NewCheckinLogUseCase(db)
	uc.ArtisanSpecific = productCustomUC.NewArtisanSpecificUseCase(db)

	// 加载服务UseCase
	uc.Service = productCustomUC.NewServiceSpecificUseCase(db)

	return uc, func() {
		_ = sqlDB.Close()
	}
}
