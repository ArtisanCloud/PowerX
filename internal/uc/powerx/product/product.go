package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ProductUseCase struct {
	db *gorm.DB
}

func NewProductUseCase(db *gorm.DB) *ProductUseCase {
	return &ProductUseCase{
		db: db,
	}
}

func (uc *ProductUseCase) CreateProduct(ctx context.Context, lead *product.Product) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductUseCase) UpsertProduct(ctx context.Context, lead *product.Product) (*product.Product, error) {

	leads := []*product.Product{lead}

	_, err := uc.UpsertProducts(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *ProductUseCase) UpsertProducts(ctx context.Context, leads []*product.Product) ([]*product.Product, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.Product{}, product.ProductUniqueId, leads, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *ProductUseCase) PatchProduct(ctx context.Context, id int64, lead *product.Product) {
	if err := uc.db.WithContext(ctx).Model(&product.Product{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductUseCase) GetProduct(ctx context.Context, id int64) (*product.Product, error) {
	var lead product.Product
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *ProductUseCase) DeleteProduct(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.Product{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
