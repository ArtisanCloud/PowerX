package product

import (
	"PowerX/internal/model/custom/product"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ServiceSpecificUseCase struct {
	db *gorm.DB
}

func NewServiceSpecificUseCase(db *gorm.DB) *ServiceSpecificUseCase {
	return &ServiceSpecificUseCase{
		db: db,
	}
}

func (uc *ServiceSpecificUseCase) CreateServiceSpecific(ctx context.Context, lead *product.ServiceSpecific) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ServiceSpecificUseCase) UpsertServiceSpecific(ctx context.Context, lead *product.ServiceSpecific) (*product.ServiceSpecific, error) {

	leads := []*product.ServiceSpecific{lead}

	_, err := uc.UpsertServiceSpecifics(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *ServiceSpecificUseCase) UpsertServiceSpecifics(ctx context.Context, leads []*product.ServiceSpecific) ([]*product.ServiceSpecific, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.ServiceSpecific{}, product.ServiceSpecificUniqueId, leads, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *ServiceSpecificUseCase) PatchServiceSpecific(ctx context.Context, id int64, lead *product.ServiceSpecific) {
	if err := uc.db.WithContext(ctx).Model(&product.ServiceSpecific{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ServiceSpecificUseCase) GetServiceSpecific(ctx context.Context, id int64) (*product.ServiceSpecific, error) {
	var lead product.ServiceSpecific
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *ServiceSpecificUseCase) DeleteServiceSpecific(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ServiceSpecific{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
