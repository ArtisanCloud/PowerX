package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ProductCategoryUseCase struct {
	db *gorm.DB
}

func NewProductCategoryUseCase(db *gorm.DB) *ProductCategoryUseCase {
	return &ProductCategoryUseCase{
		db: db,
	}
}

func (uc *ProductCategoryUseCase) CreateProductCategory(ctx context.Context, productCategory *product.ProductCategory) {
	if err := uc.db.WithContext(ctx).Create(&productCategory).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductCategoryUseCase) UpsertProductCategory(ctx context.Context, productCategory *product.ProductCategory) (*product.ProductCategory, error) {

	productCategorys := []*product.ProductCategory{productCategory}

	_, err := uc.UpsertProductCategorys(ctx, productCategorys)
	if err != nil {
		panic(errors.Wrap(err, "upsert productCategory failed"))
	}

	return productCategory, err
}

func (uc *ProductCategoryUseCase) UpsertProductCategorys(ctx context.Context, productCategorys []*product.ProductCategory) ([]*product.ProductCategory, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.ProductCategory{}, product.ProductCategoryUniqueId, productCategorys, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert productCategorys failed"))
	}

	return productCategorys, err
}

func (uc *ProductCategoryUseCase) PatchProductCategory(ctx context.Context, id int64, productCategory *product.ProductCategory) {
	if err := uc.db.WithContext(ctx).Model(&product.ProductCategory{}).Where(id).Updates(&productCategory).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductCategoryUseCase) GetProductCategory(ctx context.Context, id int64) (*product.ProductCategory, error) {
	var productCategory product.ProductCategory
	if err := uc.db.WithContext(ctx).First(&productCategory, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &productCategory, nil
}

func (uc *ProductCategoryUseCase) DeleteProductCategory(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ProductCategory{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品品类")
	}
	return nil
}
