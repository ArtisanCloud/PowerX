package seed

import (
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateProductCategories(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&product.ProductCategory{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init price category  failed"))
	}

	data := DefaultProductCategory()
	if count == 0 {
		if err = db.Model(&product.ProductCategory{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init price category failed"))
		}
	}
	return err
}

func DefaultProductCategory() (data []*product.ProductCategory) {

	data = []*product.ProductCategory{
		&product.ProductCategory{
			Name:        "女性",
			Sort:        0,
			ViceName:    "女性用户",
			Description: "女性用户",
		},
		&product.ProductCategory{
			Name:        "男性",
			Sort:        1,
			ViceName:    "男性用户",
			Description: "男性用户",
		},
	}

	return data

}
