package uc

import (
	"PowerX/internal/config"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PowerXUseCase struct {
	db           *gorm.DB
	Auth         *powerx.AuthUseCase
	Organization *powerx.OrganizationUseCase
	MetadataCtx  *powerx.MetadataCtx
}

func NewPowerXUseCase(conf *config.Config) (uc *PowerXUseCase, clean func()) {
	// 启动数据库并测试连通性
	db, err := gorm.Open(postgres.Open(conf.PowerXDatabase.DSN), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Info),
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

	uc = &PowerXUseCase{
		db: db,
	}
	// 加载子UseCase
	uc.MetadataCtx = powerx.NewMetadataCtx()
	uc.Organization = powerx.NewOrganizationUseCase(db)
	uc.Auth = powerx.NewCasbinUseCase(db, uc.MetadataCtx, uc.Organization)

	uc.AutoMigrate(context.Background())
	uc.AutoInit()

	return uc, func() {
		_ = sqlDB.Close()
	}
}

func (p *PowerXUseCase) AutoMigrate(ctx context.Context) {
	p.db.AutoMigrate(&powerx.Department{}, &powerx.Employee{})
	p.db.AutoMigrate(&powerx.EmployeeCasbinPolicy{}, powerx.AdminRole{}, powerx.AdminAPI{})
	p.db.AutoMigrate(&powerx.Clue{}, &powerx.Customer{})
}

func (p *PowerXUseCase) AutoInit() {
	p.Auth.Init()
	p.Organization.Init()
}
