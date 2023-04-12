package uc

import (
	"PowerX/internal/config"
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/membership"
	"PowerX/internal/model/product"
	"PowerX/internal/model/reservationcenter"
	"PowerX/internal/uc/powerx"
	customerDomainUC "PowerX/internal/uc/powerx/customerdomain"
	productUC "PowerX/internal/uc/powerx/product"
	reservationCenterUC "PowerX/internal/uc/powerx/reservationcenter"
	"context"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerXUseCase struct {
	db                    *gorm.DB
	AdminAuthorization    *powerx.AdminPermsUseCase
	Organization          *powerx.OrganizationUseCase
	CustomerAuthorization *customerDomainUC.AuthorizationCustomerDomainUseCase
	Customer              *customerDomainUC.CustomerUseCase
	Lead                  *customerDomainUC.LeadUseCase
	Product               *productUC.ProductUseCase
	ProductCategory       *productUC.ProductCategoryUseCase
	PriceBook             *productUC.PriceBookUseCase
	WechatMP              *powerx.WechatMiniProgramUseCase
	WechatOA              *powerx.WechatOfficialAccountUseCase
	Artisan               *reservationCenterUC.ArtisanUseCase
	Reservation           *reservationCenterUC.ReservationUseCase
	CheckinLog            *reservationCenterUC.CheckinLogUseCase
}

func NewPowerXUseCase(conf *config.Config) (uc *PowerXUseCase, clean func()) {
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

	uc = &PowerXUseCase{
		db: db,
	}
	// 加载组织架构UseCase
	uc.Organization = powerx.NewOrganizationUseCase(db)
	uc.AdminAuthorization = powerx.NewAdminPermsUseCase(db, uc.Organization)

	// 加载客域UseCase
	uc.CustomerAuthorization = customerDomainUC.NewAuthorizationCustomerDomainUseCase(db)
	uc.Customer = customerDomainUC.NewCustomerUseCase(db)
	uc.Lead = customerDomainUC.NewLeadUseCase(db)

	// 加载产品服务UseCase
	uc.Product = productUC.NewProductUseCase(db)
	uc.ProductCategory = productUC.NewProductCategoryUseCase(db)
	uc.PriceBook = productUC.NewPriceBookUseCase(db)

	// 加载微信UseCase
	uc.WechatMP = powerx.NewWechatMiniProgramUseCase(db, conf)
	uc.WechatOA = powerx.NewWechatOfficialAccountUseCase(db, conf)

	// 加载预约中心UseCase
	uc.Artisan = reservationCenterUC.NewArtisanUseCase(db)
	uc.Reservation = reservationCenterUC.NewReservationUseCase(db)
	uc.CheckinLog = reservationCenterUC.NewCheckinLogUseCase(db)

	uc.AutoMigrate(context.Background())
	uc.AutoInit()

	return uc, func() {
		_ = sqlDB.Close()
	}
}

func (p *PowerXUseCase) AutoMigrate(ctx context.Context) {
	p.db.AutoMigrate(&powerx.Department{}, &powerx.Employee{})
	p.db.AutoMigrate(&powerx.EmployeeCasbinPolicy{}, powerx.AdminRole{}, powerx.AdminRoleMenuName{}, powerx.AdminAPI{})

	// customerdomain domain
	p.db.AutoMigrate(&customerdomain.Lead{}, &customerdomain.Contact{}, &customerdomain.Customer{}, &membership.Membership{})
	p.db.AutoMigrate(&model.WechatOACustomer{}, &model.WechatMPCustomer{}, &model.WeWorkExternalContact{})
	p.db.AutoMigrate(
		&product.PivotProductToProductCategory{},
	)
	// product
	p.db.AutoMigrate(&product.Product{}, &product.ProductCategory{})
	p.db.AutoMigrate(&product.PriceBook{}, &product.PriceBookEntry{}, &product.PriceConfig{})

	// reservation center
	p.db.AutoMigrate(&reservationcenter.Artisan{}, &reservationcenter.Reservation{}, &reservationcenter.CheckinLog{})
}

func (p *PowerXUseCase) AutoInit() {
	p.AdminAuthorization.Init()
	p.Organization.Init()
}
