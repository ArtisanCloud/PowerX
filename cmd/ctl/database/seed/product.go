package seed

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/product"
	"PowerX/internal/uc/powerx"
	product2 "PowerX/internal/uc/powerx/product"
	"PowerX/pkg/mathx"
	"context"
	"fmt"
	"github.com/golang-module/carbon/v2"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateProducts(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&product.Product{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init product failed"))
	}

	data := DefaultProduct(db)
	if count == 0 {
		if err = db.Model(&product.Product{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init price category failed"))
		}

	}
	return err
}

func DefaultProduct(db *gorm.DB) (data []*product.Product) {

	ucCategory := product2.NewProductCategoryUseCase(db)
	ucDD := powerx.NewDataDictionaryUseCase(db)

	categoryTree := ucCategory.ListProductCategoryTree(context.Background(), &product2.FindProductCategoryOption{}, 0)

	productTypeGoods := ucDD.GetCachedDD(context.Background(), product.TypeProductType, product.ProductTypeGoods)
	productPlanOnce := ucDD.GetCachedDD(context.Background(), product.TypeProductPlan, product.ProductPlanOnce)
	approvalStatusSuccess := ucDD.GetCachedDD(context.Background(), model.TypeApprovalStatus, model.ApprovalStatusSuccess)

	now := carbon.Now()
	nextYear := now.AddYear()
	data = []*product.Product{}
	for _, category := range categoryTree {
		if len(category.Children) > 0 {
			sCategory := category.Children[0]
			if len(sCategory.Children) > 0 {
				tCategory := sCategory.Children[0]

				for i := 0; i < 20; i++ {
					seedProduct := &product.Product{
						Name:                fmt.Sprintf("%s-%s-%d", category.Name, tCategory.Name, i+1),
						SPU:                 fmt.Sprintf("SPU%04d", i+1),
						Type:                int(productTypeGoods.Id),
						Plan:                int(productPlanOnce.Id),
						AccountingCategory:  "s-account-category",
						ApprovalStatus:      int(approvalStatusSuccess.Id),
						IsActivated:         true,
						Description:         "this is a product created from powerx seeder",
						AllowedSellQuantity: 10,
						ValidityPeriodDays:  360,
						SaleStartDate:       now.ToStdTime(),
						SaleEndDate:         nextYear.ToStdTime(),
						ProductAttribute: product.ProductAttribute{
							Inventory:  100,
							SoldAmount: int16(i * 100),
						},
						ProductCategories: []*product.ProductCategory{
							tCategory,
						},
						ProductSpecifics: DefaultProductSpecific(),
					}
					seedProduct.PivotCoverImages = getPivotsCoverImages(db, seedProduct)
					seedProduct.PivotDetailImages = getAllPivotsDetailImages(db, seedProduct)

					data = append(data, seedProduct)
				}
			}

		}
	}

	return data
}

func getPivotsCoverImages(db *gorm.DB, p *product.Product) []*media.PivotMediaResourceToObject {
	resources := []*media.MediaResource{}

	// 生成5个不重复的随机整数
	randomNumbers := mathx.GenerateRandomNumbers(5, 1, 20)

	_ = db.Model(&media.MediaResource{}).
		Where("id IN ?", randomNumbers).
		Limit(3).
		Find(&resources)

	pivots, _ := (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(p, resources, media.MediaUsageCover)

	return pivots
}

func getAllPivotsDetailImages(db *gorm.DB, p *product.Product) []*media.PivotMediaResourceToObject {
	var resources = []*media.MediaResource{}
	_ = db.Model(&media.MediaResource{}).Limit(6).Find(&resources).Error

	pivots, _ := (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(p, resources, media.MediaUsageDetail)

	return pivots
}

func DefaultProductSpecific() (data []*product.ProductSpecific) {

	data = []*product.ProductSpecific{
		&product.ProductSpecific{
			Name: "颜色",
			Options: []*product.SpecificOption{
				{
					Name:        "红色",
					IsActivated: true,
				},
				{
					Name:        "褐色",
					IsActivated: true,
				},
				{
					Name:        "白色",
					IsActivated: false,
				},
			},
		},
		&product.ProductSpecific{
			Name: "尺码",
			Options: []*product.SpecificOption{
				{
					Name:        "XL",
					IsActivated: true,
				},
				{
					Name:        "M",
					IsActivated: true,
				},
				{
					Name:        "XXL",
					IsActivated: false,
				},
			},
		},
	}

	return data

}

func CreateTokenProducts(db *gorm.DB) (err error) {

	products := DefaultTokenProduct(db)
	ucOrg := product2.NewProductUseCase(db)
	for _, p := range products {
		existProduct := &product.Product{}
		res := db.Model(&product.Product{}).Where(product.Product{Name: p.Name}).First(existProduct)
		//fmt.Dump(existProduct, res.Error)
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			_ = ucOrg.CreateProduct(context.Background(), p)
		}
	}
	return nil
}

func DefaultTokenProduct(db *gorm.DB) (products []*product.Product) {

	ucDD := powerx.NewDataDictionaryUseCase(db)

	productTypeTokens := ucDD.GetCachedDD(context.Background(), product.TypeProductType, product.ProductTypeToken)
	productPlanOnce := ucDD.GetCachedDD(context.Background(), product.TypeProductPlan, product.ProductPlanOnce)
	approvalStatusSuccess := ucDD.GetCachedDD(context.Background(), model.TypeApprovalStatus, model.ApprovalStatusSuccess)

	products = []*product.Product{
		{
			Name:                "30",
			SPU:                 "token-001",
			Type:                int(productTypeTokens.Id),
			Plan:                int(productPlanOnce.Id),
			AccountingCategory:  "token-account-category",
			CanSellOnline:       true,
			CanUseForDeduct:     true,
			ApprovalStatus:      int(approvalStatusSuccess.Id),
			IsActivated:         true,
			Description:         "30元充值",
			AllowedSellQuantity: -1,
		},
		{
			Name:                "50",
			SPU:                 "token-001",
			Type:                int(productTypeTokens.Id),
			Plan:                int(productPlanOnce.Id),
			AccountingCategory:  "token-account-category",
			CanSellOnline:       true,
			CanUseForDeduct:     true,
			ApprovalStatus:      int(approvalStatusSuccess.Id),
			IsActivated:         true,
			Description:         "50元充值",
			AllowedSellQuantity: -1,
		},
		{
			Name:                "充100送20",
			SPU:                 "token-001",
			Type:                int(productTypeTokens.Id),
			Plan:                int(productPlanOnce.Id),
			AccountingCategory:  "token-account-category",
			CanSellOnline:       true,
			CanUseForDeduct:     true,
			ApprovalStatus:      int(approvalStatusSuccess.Id),
			IsActivated:         true,
			Description:         "充100送20",
			AllowedSellQuantity: -1,
		},
		{
			Name:                "充300送70",
			SPU:                 "token-002",
			Type:                int(productTypeTokens.Id),
			Plan:                int(productPlanOnce.Id),
			AccountingCategory:  "token-account-category",
			CanSellOnline:       true,
			CanUseForDeduct:     true,
			ApprovalStatus:      int(approvalStatusSuccess.Id),
			IsActivated:         true,
			Description:         "充300送70",
			AllowedSellQuantity: -1,
		},
		{
			Name:                "充500送80",
			SPU:                 "token-003",
			Type:                int(productTypeTokens.Id),
			Plan:                int(productPlanOnce.Id),
			AccountingCategory:  "token-account-category",
			CanSellOnline:       true,
			CanUseForDeduct:     true,
			ApprovalStatus:      int(approvalStatusSuccess.Id),
			IsActivated:         true,
			Description:         "充500送80",
			AllowedSellQuantity: -1,
		},
	}

	return products
}
