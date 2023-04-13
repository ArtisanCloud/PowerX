package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type PriceBookUseCase struct {
	db *gorm.DB
}

func NewPriceBookUseCase(db *gorm.DB) *PriceBookUseCase {
	return &PriceBookUseCase{
		db: db,
	}
}

func (uc *PriceBookUseCase) CreatePriceBook(ctx context.Context, priceBook *product.PriceBook) {
	if err := uc.db.WithContext(ctx).Create(&priceBook).Error; err != nil {
		panic(err)
	}
}

func (uc *PriceBookUseCase) UpsertPriceBook(ctx context.Context, priceBook *product.PriceBook) (*product.PriceBook, error) {

	priceBooks := []*product.PriceBook{priceBook}

	_, err := uc.UpsertPriceBooks(ctx, priceBooks)
	if err != nil {
		panic(errors.Wrap(err, "upsert priceBook failed"))
	}

	return priceBook, err
}

func (uc *PriceBookUseCase) UpsertPriceBooks(ctx context.Context, priceBooks []*product.PriceBook) ([]*product.PriceBook, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.PriceBook{}, product.PriceBookUniqueId, priceBooks, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert priceBooks failed"))
	}

	return priceBooks, err
}

func (uc *PriceBookUseCase) PatchPriceBook(ctx context.Context, id int64, priceBook *product.PriceBook) {
	if err := uc.db.WithContext(ctx).Model(&product.PriceBook{}).Where(id).Updates(&priceBook).Error; err != nil {
		panic(err)
	}
}

func (uc *PriceBookUseCase) GetPriceBook(ctx context.Context, id int64) (*product.PriceBook, error) {
	var priceBook product.PriceBook
	if err := uc.db.WithContext(ctx).First(&priceBook, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &priceBook, nil
}

func (uc *PriceBookUseCase) DeletePriceBook(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.PriceBook{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
