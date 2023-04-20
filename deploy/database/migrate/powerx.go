package migrate

import (
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/membership"
	"PowerX/internal/model/product"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/uc/powerx"
	"context"
	"gorm.io/gorm"
)

func AutoMigrate(ctx context.Context, db *gorm.DB) {
	db.AutoMigrate(&model.DataDictionaryType{}, &model.DataDictionaryItem{}, &model.PivotDataDictionaryToObject{})
	db.AutoMigrate(&organization.Department{}, &organization.Employee{})
	db.AutoMigrate(&powerx.EmployeeCasbinPolicy{}, powerx.AdminRole{}, powerx.AdminRoleMenuName{}, powerx.AdminAPI{})

	// customerdomain domain
	db.AutoMigrate(&customerdomain.Lead{}, &customerdomain.Contact{}, &customerdomain.Customer{}, &membership.Membership{})
	db.AutoMigrate(&model.WechatOACustomer{}, &model.WechatMPCustomer{}, &model.WeWorkExternalContact{})
	db.AutoMigrate(
		&product.PivotProductToProductCategory{},
	)
	// product
	db.AutoMigrate(&product.Product{}, &product.ProductCategory{})
	db.AutoMigrate(&product.PriceBook{}, &product.PriceBookEntry{}, &product.PriceConfig{})

}
