package uc

import (
	"PowerX/internal/config"
	productCustomModel "PowerX/internal/model/custom/product"
	reservationCenterCustomModel "PowerX/internal/model/custom/reservationcenter"
	productCustomUC "PowerX/internal/uc/custom/product"
	reservationCenterCustomUC "PowerX/internal/uc/custom/reservationcenter"
	"context"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type CustomUseCase struct {
	db *gorm.DB

	Artisan     *reservationCenterCustomUC.ArtisanUseCase
	Reservation *reservationCenterCustomUC.ReservationUseCase
	CheckinLog  *reservationCenterCustomUC.CheckinLogUseCase
	Service     *productCustomUC.ServiceSpecificUseCase
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
	uc.Artisan = reservationCenterCustomUC.NewArtisanUseCase(db)
	uc.Reservation = reservationCenterCustomUC.NewReservationUseCase(db)
	uc.CheckinLog = reservationCenterCustomUC.NewCheckinLogUseCase(db)

	// 加载服务UseCase
	uc.Service = productCustomUC.NewServiceSpecificUseCase(db)

	uc.AutoMigrate(context.Background())
	uc.AutoInit()

	return uc, func() {
		_ = sqlDB.Close()
	}
}

func (p *CustomUseCase) AutoMigrate(ctx context.Context) {

	// product
	p.db.AutoMigrate(&productCustomModel.ServiceSpecific{})

	// reservation center
	p.db.AutoMigrate(&reservationCenterCustomModel.Artisan{}, &reservationCenterCustomModel.Reservation{}, &reservationCenterCustomModel.CheckinLog{})

}

func (p *CustomUseCase) AutoInit() {

}
