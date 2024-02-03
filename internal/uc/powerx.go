package uc

import (
	"PowerX/internal/config"
	"PowerX/internal/uc/powerx"
	customerDomainUC "PowerX/internal/uc/powerx/crm/customerdomain"
	"PowerX/internal/uc/powerx/crm/infoorganization"
	"PowerX/internal/uc/powerx/crm/market"
	"PowerX/internal/uc/powerx/crm/operation"
	productUC "PowerX/internal/uc/powerx/crm/product"
	tradeUC "PowerX/internal/uc/powerx/crm/trade"
	"PowerX/internal/uc/powerx/scrm"
	"PowerX/internal/uc/powerx/wechat"
	"github.com/ArtisanCloud/PowerLibs/v3/cache"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerXUseCase struct {
	db                 *gorm.DB
	redis              *redis.Redis
	Cache              cache.CacheInterface
	DataDictionary     *powerx.DataDictionaryUseCase
	AdminAuthorization *powerx.AdminPermsUseCase

	Organization *powerx.OrganizationUseCase

	Label    *infoorganization.LabelUseCase
	Tag      *infoorganization.TagUseCase
	Category *infoorganization.CategoryUseCase

	CustomerAuthorization *customerDomainUC.AuthorizationCustomerDomainUseCase
	Customer              *customerDomainUC.CustomerUseCase
	Lead                  *customerDomainUC.LeadUseCase
	RegisterCode          *customerDomainUC.RegisterCodeUseCase
	Membership            *operation.MembershipUseCase
	Product               *productUC.ProductUseCase
	ProductStatistics     *productUC.ProductStatisticsUseCase
	ProductSpecific       *productUC.ProductSpecificUseCase
	SKU                   *productUC.SKUUseCase
	ProductCategory       *productUC.ProductCategoryUseCase
	PriceBook             *productUC.PriceBookUseCase
	PriceBookEntry        *productUC.PriceBookEntryUseCase
	Store                 *market.StoreUseCase
	MGM                   *market.MGMRuleUseCase
	Artisan               *productUC.ArtisanUseCase
	ShippingAddress       *tradeUC.ShippingAddressUseCase
	Cart                  *tradeUC.CartUseCase
	Order                 *tradeUC.OrderUseCase
	Payment               *tradeUC.PaymentUseCase
	Logistics             *tradeUC.LogisticsUseCase
	RefundOrder           *tradeUC.RefundOrderUseCase
	Token                 *tradeUC.TokenUseCase
	WechatMP              *wechat.WechatMiniProgramUseCase
	WechatOA              *wechat.WechatOfficialAccountUseCase
	//WeWork                *powerx.WeWorkUseCase
	SCRM          *scrm.SCRMUseCase
	MediaResource *powerx.MediaResourceUseCase
	Media         *market.MediaUseCase
	Scene         *scrm.SceneUseCase
}

func NewPowerXUseCase(conf *config.Config) (uc *PowerXUseCase, clean func()) {
	// 启动数据库并测试连通性
	var dsn gorm.Dialector
	switch conf.PowerXDatabase.Driver {
	case config.DriverMysql:
		dsn = mysql.Open(conf.PowerXDatabase.DSN)
	case config.DriverPostgres:
		dsn = postgres.Open(conf.PowerXDatabase.DSN)
	}
	db, err := gorm.Open(dsn, &gorm.Config{
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
		redis: redis.New(conf.RedisBase.Host, func(r *redis.Redis) {
			r.Pass = conf.RedisBase.Password
		}),
	}

	// 加载Cache对象
	cacheOption := &kernel.RedisOptions{
		Addr: conf.RedisBase.Host,
		//Username: conf.RedisBase.Username,
		Password: conf.RedisBase.Password,
		//DB:       conf.RedisBase.DB,
	}
	uc.Cache = kernel.NewRedisClient(cacheOption)

	// 加载基础UseCase
	uc.DataDictionary = powerx.NewDataDictionaryUseCase(db)

	// 加载组织架构UseCase
	uc.Organization = powerx.NewOrganizationUseCase(db)
	uc.AdminAuthorization = powerx.NewAdminPermsUseCase(conf, db, uc.Organization)

	// 加载信息组织UseCase
	uc.Label = infoorganization.NewLabelUseCase(db)
	uc.Tag = infoorganization.NewTagUseCase(db)
	uc.Category = infoorganization.NewCategoryUseCase(db)

	// 加载客域UseCase
	uc.CustomerAuthorization = customerDomainUC.NewAuthorizationCustomerDomainUseCase(db)
	uc.Customer = customerDomainUC.NewCustomerUseCase(db)
	uc.Lead = customerDomainUC.NewLeadUseCase(db)
	uc.RegisterCode = customerDomainUC.NewRegisterCodeUseCase(db)

	// 会籍权益
	uc.Membership = operation.NewMembershipUseCase(db)

	// 加载产品服务UseCase
	uc.ProductSpecific = productUC.NewProductSpecificUseCase(db)
	uc.ProductStatistics = productUC.NewProductStatisticsUseCase(db)
	uc.SKU = productUC.NewSKUUseCase(db)
	uc.Product = productUC.NewProductUseCase(db)
	uc.ProductCategory = productUC.NewProductCategoryUseCase(db)
	uc.PriceBook = productUC.NewPriceBookUseCase(db)
	uc.PriceBookEntry = productUC.NewPriceBookEntryUseCase(db)
	uc.Store = market.NewStoreUseCase(db)
	uc.Artisan = productUC.NewArtisanUseCase(db)

	// 加载交易UseCase
	uc.ShippingAddress = tradeUC.NewShippingAddressUseCase(db)
	uc.Cart = tradeUC.NewCartUseCase(db)
	uc.Order = tradeUC.NewOrderUseCase(db)
	uc.Payment = tradeUC.NewPaymentUseCase(db, conf)
	uc.RefundOrder = tradeUC.NewRefundOrderUseCase(db)
	uc.Logistics = tradeUC.NewLogisticsUseCase(db)
	uc.Token = tradeUC.NewTokenUseCase(db)

	// 加载微信UseCase
	//uc.WeWork = powerx.NewWeWorkUseCase(db, conf)
	uc.WechatMP = wechat.NewWechatMiniProgramUseCase(db, conf)
	uc.WechatOA = wechat.NewWechatOfficialAccountUseCase(db, conf)

	// 加载市场UseCase
	uc.Media = market.NewMediaUseCase(db)
	uc.MGM = market.NewMGMRuleUseCase(db)

	// 加载Media Resource UseCase
	uc.MediaResource = powerx.NewMediaResourceUseCase(db, conf)

	// 加载SCRM UseCase
	c := cron.New()
	uc.SCRM = scrm.NewSCRMUseCase(db, conf, c, uc.redis)
	uc.SCRM.Schedule()

	// 加载Scene
	uc.Scene = scrm.NewSceneUseCase(db, uc.redis)

	return uc, func() {
		_ = sqlDB.Close()
	}
}

func GetRedis() {

}
