package reservationcenter

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/reservationcenter"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type CheckinLogUseCase struct {
	db *gorm.DB
}

func NewCheckinLogUseCase(db *gorm.DB) *CheckinLogUseCase {
	return &CheckinLogUseCase{
		db: db,
	}
}

func (uc *CheckinLogUseCase) CreateCheckinLog(ctx context.Context, lead *reservationcenter.CheckinLog) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *CheckinLogUseCase) UpsertCheckinLog(ctx context.Context, lead *reservationcenter.CheckinLog) (*reservationcenter.CheckinLog, error) {

	leads := []*reservationcenter.CheckinLog{lead}

	_, err := uc.UpsertCheckinLogs(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *CheckinLogUseCase) UpsertCheckinLogs(ctx context.Context, leads []*reservationcenter.CheckinLog) ([]*reservationcenter.CheckinLog, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &reservationcenter.CheckinLog{}, reservationcenter.CheckinLogUniqueId, leads, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *CheckinLogUseCase) PatchCheckinLog(ctx context.Context, id int64, lead *reservationcenter.CheckinLog) {
	if err := uc.db.WithContext(ctx).Model(&reservationcenter.CheckinLog{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *CheckinLogUseCase) GetCheckinLog(ctx context.Context, id int64) (*reservationcenter.CheckinLog, error) {
	var lead reservationcenter.CheckinLog
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *CheckinLogUseCase) DeleteCheckinLog(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&reservationcenter.CheckinLog{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
