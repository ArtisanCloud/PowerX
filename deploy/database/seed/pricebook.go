package seed

import (
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreatePriceBook(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&product.PriceBook{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init price book  failed"))
	}

	data := DefaultPriceBook()
	if count == 0 {
		if err = db.Model(&product.PriceBook{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init price book failed"))
		}
	}
	return err
}

func DefaultPriceBook() (data []*product.PriceBook) {

	data = []*product.PriceBook{
		&product.PriceBook{
			IsStandard:  true,
			Name:        "标准价格手册",
			Description: "标准价格手册",
			StoreId:     0,
		},
	}

	return data

}
