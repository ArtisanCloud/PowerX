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
	db                    *gorm.DB
	AdminAuthorization    *powerx.AdminPermsUseCase
	CustomerAuthorization *powerx.AuthorizationCustomerUseCase
	Organization          *powerx.OrganizationUseCase
	WechatMP              *powerx.WechatMiniProgramUseCase
	WechatOA              *powerx.WechatOfficialAccountUseCase
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
	uc.Organization = powerx.NewOrganizationUseCase(db)
	uc.AdminAuthorization = powerx.NewAdminPermsUseCase(db, uc.Organization)
	uc.CustomerAuthorization = powerx.NewAuthorizationCustomerUseCase(db)
	uc.WechatMP = powerx.NewWechatMiniProgramUseCase(db, conf)
	uc.WechatOA = powerx.NewWechatOfficialAccountUseCase(db, conf)

	uc.AutoMigrate(context.Background())
	uc.AutoInit()

	return uc, func() {
		_ = sqlDB.Close()
	}
}

func (p *PowerXUseCase) AutoMigrate(ctx context.Context) {
	p.db.AutoMigrate(&powerx.Department{}, &powerx.Employee{})
	p.db.AutoMigrate(&powerx.EmployeeCasbinPolicy{}, powerx.AdminRole{}, powerx.AdminRoleMenuName{}, powerx.AdminAPI{})

}

func (p *PowerXUseCase) AutoInit() {
	p.AdminAuthorization.Init()
	p.Organization.Init()
}
