package seed

import (
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	product2 "PowerX/internal/uc/powerx/product"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreatePriceBooks(db *gorm.DB) (err error) {

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

	if err = db.Model(&product.PriceBookEntry{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init price book entry  failed"))
	}
	if count == 0 {
		err = SeedProductPriceBookEntries(db, data[0])
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

func SeedProductPriceBookEntries(db *gorm.DB, book *product.PriceBook) (err error) {

	ucProduct := product2.NewProductUseCase(db)

	products, err := ucProduct.FindManyProducts(context.Background(), &product2.FindManyProductsOption{
		PageEmbedOption: types.PageEmbedOption{
			PageSize: 9999,
		},
	})

	entries := []*product.PriceBookEntry{}
	for _, p := range products.List {
		//  初始化一个产品的标准价格条目
		standardEntry := &product.PriceBookEntry{
			PriceBookId: book.Id,
			ProductId:   p.Id,
			UnitPrice:   888,
			RetailPrice: 999,
			IsActive:    true,
		}

		entries = append(entries, standardEntry)

	}

	if err = db.Model(&product.PriceBookEntry{}).Create(entries).Error; err != nil {
		panic(errors.Wrap(err, "init price book failed"))
	}

	// seed skus
	for _, p := range products.List {
		//  根据产品规格，计算对应的sku，生成sku
		//fmt.Dump(p.ProductSpecifics)
		skus := ucProduct.GenerateSKUsFromSpecifics(context.Background(), p)
		if err = db.Model(&product.SKU{}).Create(skus).Error; err != nil {
			panic(errors.Wrap(err, "init sku failed"))
		}
		// 计算sku的价格
		skuEntries := []*product.PriceBookEntry{}
		for _, sku := range skus {
			skuEntry := &product.PriceBookEntry{
				PriceBookId: book.Id,
				ProductId:   p.Id,
				SkuId:       sku.Id,
				UnitPrice:   878,
				RetailPrice: 999,
				IsActive:    true,
			}
			skuEntries = append(skuEntries, skuEntry)
		}

		if err = db.Model(&product.PriceBookEntry{}).Create(skuEntries).Error; err != nil {
			panic(errors.Wrap(err, "init sku price book entries failed"))
		}
	}

	return err

}
