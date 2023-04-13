package market

import (
	"PowerX/internal/model/market"
	"PowerX/internal/model/powermodel"
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

func (uc *StoreUseCase) CreateStore(ctx context.Context, lead *market.Store) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *StoreUseCase) UpsertStore(ctx context.Context, lead *market.Store) (*market.Store, error) {

	leads := []*market.Store{lead}

	_, err := uc.UpsertStores(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *StoreUseCase) UpsertStores(ctx context.Context, leads []*market.Store) ([]*market.Store, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &market.Store{}, market.StoreUniqueId, leads, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *StoreUseCase) PatchStore(ctx context.Context, id int64, lead *market.Store) {
	if err := uc.db.WithContext(ctx).Model(&market.Store{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *StoreUseCase) GetStore(ctx context.Context, id int64) (*market.Store, error) {
	var lead market.Store
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *StoreUseCase) DeleteStore(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&market.Store{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
