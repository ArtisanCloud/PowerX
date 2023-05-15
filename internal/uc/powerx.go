package uc

import (
	"PowerX/internal/config"
	"PowerX/internal/uc/powerx"
	customerDomainUC "PowerX/internal/uc/powerx/customerdomain"
	productUC "PowerX/internal/uc/powerx/product"
	"github.com/pkg/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerXUseCase struct {
	db                 *gorm.DB
	DataDictionary     *powerx.DataDictionaryUseCase
	AdminAuthorization *powerx.AdminPermsUseCase

	Organization *powerx.OrganizationUseCase

	CustomerAuthorization *customerDomainUC.AuthorizationCustomerDomainUseCase
	Customer              *customerDomainUC.CustomerUseCase
	Lead                  *customerDomainUC.LeadUseCase
	Product               *productUC.ProductUseCase
	ProductCategory       *productUC.ProductCategoryUseCase
	PriceBook             *productUC.PriceBookUseCase
	Store                 *productUC.StoreUseCase
	Artisan               *productUC.ArtisanUseCase
	WechatMP              *powerx.WechatMiniProgramUseCase
	WechatOA              *powerx.WechatOfficialAccountUseCase
	WeWork                *powerx.WeWorkUseCase
	SCRM                  *powerx.SCRMUseCase
	MediaResource         *powerx.MediaResourceUseCase
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
	// 加载基础UseCase
	uc.DataDictionary = powerx.NewDataDictionaryUseCase(db)

	// 加载组织架构UseCase
	uc.Organization = powerx.NewOrganizationUseCase(db)
	uc.AdminAuthorization = powerx.NewAdminPermsUseCase(conf, db, uc.Organization)

	// 加载客域UseCase
	uc.CustomerAuthorization = customerDomainUC.NewAuthorizationCustomerDomainUseCase(db)
	uc.Customer = customerDomainUC.NewCustomerUseCase(db)
	uc.Lead = customerDomainUC.NewLeadUseCase(db)

	// 加载产品服务UseCase
	uc.Product = productUC.NewProductUseCase(db)
	uc.ProductCategory = productUC.NewProductCategoryUseCase(db)
	uc.PriceBook = productUC.NewPriceBookUseCase(db)
	uc.Store = productUC.NewStoreUseCase(db)
	uc.Artisan = productUC.NewArtisanUseCase(db)

	// 加载微信UseCase
	uc.WechatMP = powerx.NewWechatMiniProgramUseCase(db, conf)
	uc.WechatOA = powerx.NewWechatOfficialAccountUseCase(db, conf)
	uc.WeWork = powerx.NewWeWorkUseCase(db, conf)
	uc.MediaResource = powerx.NewMediaResourceUseCase(db, conf)

	// 加载SCRM UseCase
	uc.SCRM = powerx.NewSCRMUseCase(db, conf)

	return uc, func() {
		_ = sqlDB.Close()
	}
}
