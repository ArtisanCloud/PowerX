package reservationcenter

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ScheduleConfigUseCase struct {
	db *gorm.DB
}

func NewScheduleConfigUseCase(db *gorm.DB) *ScheduleConfigUseCase {
	return &ScheduleConfigUseCase{
		db: db,
	}
}

func (uc *ScheduleConfigUseCase) CreateScheduleConfig(ctx context.Context, lead *reservationcenter.ScheduleConfig) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ScheduleConfigUseCase) UpsertScheduleConfig(ctx context.Context, lead *reservationcenter.ScheduleConfig) (*reservationcenter.ScheduleConfig, error) {

	leads := []*reservationcenter.ScheduleConfig{lead}

	_, err := uc.UpsertScheduleConfigs(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *ScheduleConfigUseCase) UpsertScheduleConfigs(ctx context.Context, leads []*reservationcenter.ScheduleConfig) ([]*reservationcenter.ScheduleConfig, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &reservationcenter.ScheduleConfig{}, reservationcenter.ScheduleConfigUniqueId, leads, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *ScheduleConfigUseCase) PatchScheduleConfig(ctx context.Context, id int64, lead *reservationcenter.ScheduleConfig) {
	if err := uc.db.WithContext(ctx).Model(&reservationcenter.ScheduleConfig{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ScheduleConfigUseCase) GetScheduleConfig(ctx context.Context, id int64) (*reservationcenter.ScheduleConfig, error) {
	var lead reservationcenter.ScheduleConfig
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *ScheduleConfigUseCase) DeleteScheduleConfig(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&reservationcenter.ScheduleConfig{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
