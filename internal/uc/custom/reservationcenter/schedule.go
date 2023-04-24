package reservationcenter

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ScheduleUseCase struct {
	db *gorm.DB
}

func NewScheduleUseCase(db *gorm.DB) *ScheduleUseCase {
	return &ScheduleUseCase{
		db: db,
	}
}

func (uc *ScheduleUseCase) CreateSchedule(ctx context.Context, lead *reservationcenter.Schedule) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ScheduleUseCase) UpsertSchedule(ctx context.Context, lead *reservationcenter.Schedule) (*reservationcenter.Schedule, error) {

	leads := []*reservationcenter.Schedule{lead}

	_, err := uc.UpsertSchedules(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *ScheduleUseCase) UpsertSchedules(ctx context.Context, leads []*reservationcenter.Schedule) ([]*reservationcenter.Schedule, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &reservationcenter.Schedule{}, reservationcenter.ScheduleUniqueId, leads, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *ScheduleUseCase) PatchSchedule(ctx context.Context, id int64, lead *reservationcenter.Schedule) {
	if err := uc.db.WithContext(ctx).Model(&reservationcenter.Schedule{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ScheduleUseCase) GetSchedule(ctx context.Context, id int64) (*reservationcenter.Schedule, error) {
	var lead reservationcenter.Schedule
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *ScheduleUseCase) DeleteSchedule(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&reservationcenter.Schedule{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
