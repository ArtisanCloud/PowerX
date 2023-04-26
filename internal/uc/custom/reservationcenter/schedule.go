package reservationcenter

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/datetime/carbonx"
	"context"
	"github.com/golang-module/carbon/v2"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"time"
)

type ScheduleUseCase struct {
	db *gorm.DB
}

type FindManySchedulesOption struct {
	Types       []string
	CurrentDate time.Time
	StoreId     int64
	types.PageEmbedOption
}

func NewScheduleUseCase(db *gorm.DB) *ScheduleUseCase {
	return &ScheduleUseCase{
		db: db,
	}
}

func (uc *ScheduleUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManySchedulesOption) *gorm.DB {
	if len(opt.Types) > 0 {
		db = db.Where("type IN ?", opt.Types)
	}
	if opt.StoreId > 0 {
		db = db.Where("storeId = ?", opt.StoreId)
	}
	if !opt.CurrentDate.IsZero() {
		cDate := carbon.FromStdTime(opt.CurrentDate)
		startDate, endDate := carbonx.GetWeekDaysFromDay(&cDate, nil)
		db = db.Where("start_time > ? AND end_time < ?", startDate.Time, endDate.Time)
	}

	return db
}

func (uc *ScheduleUseCase) FindAllSchedules(ctx context.Context, opt *FindManySchedulesOption) (schedules []*reservationcenter.Schedule, err error) {
	query := uc.db.WithContext(ctx).Model(&reservationcenter.Schedule{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		Debug().
		//Preload("Artisans").
		Find(&schedules).Error; err != nil {
		panic(errors.Wrap(err, "find all schedules failed"))
	}
	return schedules, err
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
