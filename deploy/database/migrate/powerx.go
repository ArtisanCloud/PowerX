package migrate

import (
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/membership"
	"PowerX/internal/model/product"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/uc/powerx"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) {

	_ = db.AutoMigrate(&model.DataDictionaryType{}, &model.DataDictionaryItem{}, &model.PivotDataDictionaryToObject{})
	_ = db.AutoMigrate(&organization.Department{}, &organization.Employee{})
	_ = db.AutoMigrate(&powerx.EmployeeCasbinPolicy{}, powerx.AdminRole{}, powerx.AdminRoleMenuName{}, powerx.AdminAPI{})

	// customerdomain domain
	_ = db.AutoMigrate(&customerdomain.Lead{}, &customerdomain.Contact{}, &customerdomain.Customer{}, &membership.Membership{})
	_ = db.AutoMigrate(&model.WechatOACustomer{}, &model.WechatMPCustomer{}, &model.WeWorkExternalContact{})
	_ = db.AutoMigrate(
		&product.PivotProductToProductCategory{},
	)
	// product
	_ = db.AutoMigrate(&product.Product{}, &product.ProductCategory{})
	_ = db.AutoMigrate(&product.PriceBook{}, &product.PriceBookEntry{}, &product.PriceConfig{})
	_ = db.AutoMigrate(&product.Store{}, &product.Artisan{}, &product.PivotStoreToArtisan{})
}
