package market

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type StoreUseCase struct {
	db *gorm.DB
}

func NewStoreUseCase(db *gorm.DB) *StoreUseCase {
	return &StoreUseCase{
		db: db,
	}
}

func (uc *StoreUseCase) CreateStore(ctx context.Context, store *product.Store) {
	if err := uc.db.WithContext(ctx).Create(&store).Error; err != nil {
		panic(err)
	}
}

func (uc *StoreUseCase) UpsertStore(ctx context.Context, store *product.Store) (*product.Store, error) {

	stores := []*product.Store{store}

	_, err := uc.UpsertStores(ctx, stores)
	if err != nil {
		panic(errors.Wrap(err, "upsert store failed"))
	}

	return store, err
}

func (uc *StoreUseCase) UpsertStores(ctx context.Context, stores []*product.Store) ([]*product.Store, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.Store{}, product.StoreUniqueId, stores, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert stores failed"))
	}

	return stores, err
}

func (uc *StoreUseCase) PatchStore(ctx context.Context, id int64, store *product.Store) {
	if err := uc.db.WithContext(ctx).Model(&product.Store{}).Where(id).Updates(&store).Error; err != nil {
		panic(err)
	}
}

func (uc *StoreUseCase) GetStore(ctx context.Context, id int64) (*product.Store, error) {
	var store product.Store
	if err := uc.db.WithContext(ctx).First(&store, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &store, nil
}

func (uc *StoreUseCase) DeleteStore(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.Store{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
