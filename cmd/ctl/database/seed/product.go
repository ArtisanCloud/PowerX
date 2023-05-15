package seed

import (
	"PowerX/internal/model"
	"PowerX/internal/model/product"
	"PowerX/internal/uc/powerx"
	product2 "PowerX/internal/uc/powerx/product"
	"context"
	"fmt"
	carbon "github.com/golang-module/carbon/v2"
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
						ProductSpecific: product.ProductSpecific{
							Inventory:  100,
							SoldAmount: int16(i * 100),
						},
						ProductCategories: []*product.ProductCategory{
							tCategory,
						},
					}

					data = append(data, seedProduct)
				}
			}

		}
	}

	return data
}
