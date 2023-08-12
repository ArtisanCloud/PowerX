package seed

import (
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
	product2 "PowerX/internal/uc/powerx/product"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"math/rand"
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

		if err = db.Model(&product.PriceBookEntry{}).Count(&count).Error; err != nil {
			panic(errors.Wrap(err, "init price book entry  failed"))
		}
		if count == 0 {
			err = SeedProductPriceBookEntries(db, data[0])
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

func SeedProductPriceBookEntries(db *gorm.DB, book *product.PriceBook) (err error) {

	ucProduct := product2.NewProductUseCase(db)

	products, err := ucProduct.FindManyProducts(context.Background(), &product2.FindManyProductsOption{
		PageEmbedOption: types.PageEmbedOption{
			PageSize: 99,
		},
	})
	if len(products.List) <= 0 {
		return
	}

	entries := []*product.PriceBookEntry{}
	for _, p := range products.List {
		//  初始化一个产品的标准价格条目
		unitPrice := rand.Float64() * 1000
		standardEntry := &product.PriceBookEntry{
			PriceBookId: book.Id,
			ProductId:   p.Id,
			UnitPrice:   unitPrice,
			ListPrice:   unitPrice + 200,
			IsActive:    true,
		}
		standardEntry.UniqueID = standardEntry.GetComposedUniqueID()
		entries = append(entries, standardEntry)

	}

	if err = db.Model(&product.PriceBookEntry{}).Create(entries).Error; err != nil {
		panic(errors.Wrap(err, "init price book failed"))
	}

	ucDD := powerx.NewDataDictionaryUseCase(db)

	productTypeTokenId := ucDD.GetCachedDDId(context.Background(), product.TypeProductType, product.ProductTypeToken)

	// seed skus
	for _, p := range products.List {
		// token 产品暂时不用sku区分
		if p.Type != productTypeTokenId {
			//  根据产品规格，计算对应的sku，生成sku
			//fmt.Dump(p.ProductSpecifics)
			skus := ucProduct.GenerateSKUsFromSpecifics(context.Background(), p)
			if err = db.Model(&product.SKU{}).Create(&skus).Error; err != nil {
				panic(errors.Wrap(err, "init sku failed"))
			}

			pivots := ucProduct.GeneratePivotSKUsFromSpecifics(context.Background(), p.ProductSpecifics, skus)
			if err = db.Model(&product.PivotSkuToSpecificOption{}).Create(&pivots).Error; err != nil {
				panic(errors.Wrap(err, "init sku pivots failed"))
			}

			// 计算sku的价格
			skuEntries := []*product.PriceBookEntry{}

			for _, sku := range skus {
				unitPrice := rand.Float64() * 1000
				skuEntry := &product.PriceBookEntry{
					PriceBookId: book.Id,
					ProductId:   p.Id,
					SkuId:       sku.Id,
					UnitPrice:   unitPrice,
					ListPrice:   unitPrice + 200,
					IsActive:    true,
				}
				skuEntry.UniqueID = skuEntry.GetComposedUniqueID()
				skuEntries = append(skuEntries, skuEntry)
			}

			if err = db.Model(&product.PriceBookEntry{}).Create(skuEntries).Error; err != nil {
				panic(errors.Wrap(err, "init sku price book entries failed"))
			}
		}

	}

	return err

}
