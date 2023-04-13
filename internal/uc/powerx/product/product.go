package product

import (
	"PowerX/internal/model/powermodel"
	model "PowerX/internal/model/product"
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

func (uc *ProductUseCase) CreateProduct(ctx context.Context, product *model.Product) {
	if err := uc.db.WithContext(ctx).Create(&product).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductUseCase) UpsertProduct(ctx context.Context, product *model.Product) (*model.Product, error) {

	products := []*model.Product{product}

	_, err := uc.UpsertProducts(ctx, products)
	if err != nil {
		panic(errors.Wrap(err, "upsert product failed"))
	}

	return product, err
}

func (uc *ProductUseCase) UpsertProducts(ctx context.Context, products []*model.Product) ([]*model.Product, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.Product{}, model.ProductUniqueId, products, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert products failed"))
	}

	return products, err
}

func (uc *ProductUseCase) PatchProduct(ctx context.Context, id int64, product *model.Product) {
	if err := uc.db.WithContext(ctx).Model(&model.Product{}).Where(id).Updates(&product).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductUseCase) GetProduct(ctx context.Context, id int64) (*model.Product, error) {
	var product model.Product
	if err := uc.db.WithContext(ctx).First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &product, nil
}

func (uc *ProductUseCase) DeleteProduct(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&model.Product{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
