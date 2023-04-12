package uc

import (
	"PowerX/internal/config"
	"PowerX/internal/model"
	"PowerX/internal/model/customer"
	"PowerX/internal/model/membership"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PowerXUseCase struct {
	db                     *gorm.DB
	DataDictionaryUserCase *powerx.DataDictionaryUseCase
	AdminAuthorization     *powerx.AdminPermsUseCase
	CustomerAuthorization  *powerx.AuthorizationCustomerUseCase
	Organization           *powerx.OrganizationUseCase
	WechatMP               *powerx.WechatMiniProgramUseCase
	WechatOA               *powerx.WechatOfficialAccountUseCase
	SCRM                   *powerx.SCRMUseCase
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
	uc.DataDictionaryUserCase = powerx.NewDataDictionaryUseCase(db)
	uc.Organization = powerx.NewOrganizationUseCase(db)
	uc.AdminAuthorization = powerx.NewAdminPermsUseCase(conf, db, uc.Organization)
	uc.CustomerAuthorization = powerx.NewAuthorizationCustomerUseCase(db)
	uc.WechatMP = powerx.NewWechatMiniProgramUseCase(db, conf)
	uc.WechatOA = powerx.NewWechatOfficialAccountUseCase(db, conf)
	uc.SCRM = powerx.NewSCRMUseCase(db, conf)

	uc.AutoMigrate(context.Background())
	uc.AutoInit()

	return uc, func() {
		_ = sqlDB.Close()
	}
}

func (p *PowerXUseCase) AutoMigrate(ctx context.Context) {
	p.db.AutoMigrate(&model.DataDictionaryType{}, &model.DataDictionaryItem{})
	p.db.AutoMigrate(&powerx.Department{}, &powerx.Employee{})
	p.db.AutoMigrate(&powerx.EmployeeCasbinPolicy{}, powerx.AdminRole{}, powerx.AdminRoleMenuName{}, powerx.AdminAPI{})

	// customer domain
	p.db.AutoMigrate(&customer.Lead{}, &customer.Contact{}, &customer.Customer{}, &membership.Membership{})
	p.db.AutoMigrate(&model.WechatOACustomer{}, &model.WechatMPCustomer{}, &model.WeWorkExternalContact{})
	p.db.AutoMigrate(
		&customer.PivotCustomerToWechatMPCustomer{},
		&customer.PivotLeadToWechatMPCustomer{},
	)
}

func (p *PowerXUseCase) AutoInit() {
	p.AdminAuthorization.Init()
	p.Organization.Init()
}
