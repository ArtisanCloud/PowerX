package migrate

import (
	"PowerX/cmd/ctl/database/custom/migrate"
	"PowerX/internal/config"
	"PowerX/internal/model"
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/crm/market"
	"PowerX/internal/model/crm/membership"
	"PowerX/internal/model/crm/product"
	"PowerX/internal/model/crm/trade"
	infoorganizatoin "PowerX/internal/model/infoorganization"
	"PowerX/internal/model/media"
	"PowerX/internal/model/origanzation"
	"PowerX/internal/model/permission"
	"PowerX/internal/model/scene"
	"PowerX/internal/model/scrm/app"
	"PowerX/internal/model/scrm/customer"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/model/scrm/resource"
	"PowerX/internal/model/scrm/tag"
	"PowerX/internal/model/wechat"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerMigrator struct {
	db *gorm.DB
}

func NewPowerMigrator(conf *config.Config) (*PowerMigrator, error) {
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

	return &PowerMigrator{
		db: db,
	}, err
}

func (m *PowerMigrator) AutoMigrate() {

	_ = m.db.AutoMigrate(&model.DataDictionaryType{}, &model.DataDictionaryItem{}, &model.PivotDataDictionaryToObject{})
	_ = m.db.AutoMigrate(&origanzation.Department{}, &origanzation.Employee{}, &origanzation.Position{})
	_ = m.db.AutoMigrate(&permission.EmployeeCasbinPolicy{}, permission.AdminRole{}, permission.AdminRoleMenuName{}, permission.AdminAPI{})

	// info organization
	_ = m.db.AutoMigrate(&infoorganizatoin.Category{}, &infoorganizatoin.Label{}, &infoorganizatoin.Tag{})
	_ = m.db.AutoMigrate(&infoorganizatoin.PivotCategoryToObject{})

	// customer domain
	_ = m.db.AutoMigrate(
		&customerdomain.Lead{}, &customerdomain.Contact{}, customerdomain.RegisterCode{},
		&customerdomain.Customer{}, &membership.Membership{},
	)
	_ = m.db.AutoMigrate(&wechat.WechatOACustomer{}, &wechat.WechatMPCustomer{}, &wechat.WeWorkExternalContact{})
	_ = m.db.AutoMigrate(
		&product.PivotProductToProductCategory{},
	)
	// product
	_ = m.db.AutoMigrate(&product.Product{}, &product.ProductCategory{})
	_ = m.db.AutoMigrate(&product.ProductSpecific{}, &product.SpecificOption{}, &product.ProductStatistics{})
	_ = m.db.AutoMigrate(&product.SKU{}, &product.PivotSkuToSpecificOption{})
	_ = m.db.AutoMigrate(&product.PriceBook{}, &product.PriceBookEntry{}, &product.PriceConfig{})
	_ = m.db.AutoMigrate(&market.Store{}, &product.Artisan{}, &product.PivotStoreToArtisan{})

	// market
	_ = m.db.AutoMigrate(&market.Media{})
	_ = m.db.AutoMigrate(&market.MGMRule{}, market.InviteRecord{}, market.CommissionRecord{})

	// media
	_ = m.db.AutoMigrate(&media.MediaResource{}, &media.PivotMediaResourceToObject{})

	// trade
	_ = m.db.AutoMigrate(&trade.ShippingAddress{}, &trade.DeliveryAddress{}, &trade.BillingAddress{})
	_ = m.db.AutoMigrate(&trade.Warehouse{}, &trade.Inventory{}, &trade.Logistics{})
	_ = m.db.AutoMigrate(&trade.Cart{}, &trade.CartItem{}, &trade.Order{}, &trade.OrderItem{})
	_ = m.db.AutoMigrate(&trade.OrderStatusTransition{}, &trade.PivotOrderToInventoryLog{})
	_ = m.db.AutoMigrate(&trade.Payment{}, &trade.PaymentItem{})
	_ = m.db.AutoMigrate(&trade.RefundOrder{}, &trade.RefundOrderItem{})
	_ = m.db.AutoMigrate(&trade.TokenBalance{}, &trade.TokenExchangeRatio{}, &trade.TokenExchangeRecord{})

	// custom
	migrate.AutoMigrateCustom(m.db)

	// wechat organization
	_ = m.db.AutoMigrate(&organization.WeWorkEmployee{}, &organization.WeWorkDepartment{})
	// wechat customer
	_ = m.db.AutoMigrate(&customer.WeWorkExternalContacts{}, &customer.WeWorkExternalContactFollow{})
	// wechat resource
	_ = m.db.AutoMigrate(&resource.WeWorkResource{})
	// wechat app
	_ = m.db.AutoMigrate(&app.WeWorkAppGroup{})
	// wechat tag
	_ = m.db.AutoMigrate(&tag.WeWorkTag{}, &tag.WeWorkTagGroup{})
	// qrcode
	_ = m.db.AutoMigrate(&scene.SceneQrcode{})
}
