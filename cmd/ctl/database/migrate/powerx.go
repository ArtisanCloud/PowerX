package migrate

import (
	"PowerX/cmd/ctl/database/custom/migrate"
	"PowerX/internal/config"
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/market"
	"PowerX/internal/model/media"
	"PowerX/internal/model/membership"
	"PowerX/internal/model/product"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/model/trade"
	"PowerX/internal/uc/powerx"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerMigrator struct {
	db *gorm.DB
}

func NewPowerMigrator(conf *config.Config) (*PowerMigrator, error) {
	db, err := gorm.Open(postgres.Open(conf.PowerXDatabase.DSN), &gorm.Config{
		//Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})

	return &PowerMigrator{
		db: db,
	}, err
}

func (m *PowerMigrator) AutoMigrate() {

	_ = m.db.AutoMigrate(&model.DataDictionaryType{}, &model.DataDictionaryItem{}, &model.PivotDataDictionaryToObject{})
	_ = m.db.AutoMigrate(&organization.Department{}, &organization.Employee{})
	_ = m.db.AutoMigrate(&powerx.EmployeeCasbinPolicy{}, powerx.AdminRole{}, powerx.AdminRoleMenuName{}, powerx.AdminAPI{})

	// customerdomain domain
	_ = m.db.AutoMigrate(&customerdomain.Lead{}, &customerdomain.Contact{}, &customerdomain.Customer{}, &membership.Membership{})
	_ = m.db.AutoMigrate(&model.WechatOACustomer{}, &model.WechatMPCustomer{}, &model.WeWorkExternalContact{})
	_ = m.db.AutoMigrate(
		&product.PivotProductToProductCategory{},
	)
	// product
	_ = m.db.AutoMigrate(&product.Product{}, &product.ProductCategory{})
	_ = m.db.AutoMigrate(&product.ProductSpecific{}, &product.SpecificOption{})
	_ = m.db.AutoMigrate(&product.SKU{}, &product.PivotSkuToSpecificOption{})
	_ = m.db.AutoMigrate(&product.PriceBook{}, &product.PriceBookEntry{}, &product.PriceConfig{})
	_ = m.db.AutoMigrate(&market.Store{}, &product.Artisan{}, &product.PivotStoreToArtisan{})

	// market
	_ = m.db.AutoMigrate(&market.Media{})

	// media
	_ = m.db.AutoMigrate(&media.MediaResource{}, &media.PivotMediaResourceToObject{})

	// trade
	_ = m.db.AutoMigrate(&trade.ShippingAddress{}, &trade.DeliveryAddress{}, &trade.BillingAddress{})
	_ = m.db.AutoMigrate(&trade.Warehouse{}, &trade.Inventory{}, &trade.Logistics{})
	_ = m.db.AutoMigrate(&trade.Cart{}, &trade.CartItem{}, &trade.Order{}, &trade.OrderItem{})
	_ = m.db.AutoMigrate(&trade.OrderStatusTransition{}, &trade.PivotOrderToInventoryLog{})
	_ = m.db.AutoMigrate(&trade.Payment{}, &trade.PaymentItem{})
	_ = m.db.AutoMigrate(&trade.RefundOrder{}, &trade.RefundOrderItem{})

	// custom
	migrate.AutoMigrateCustom(m.db)
}
