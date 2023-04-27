package reservationcenter

import (
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/datetime/carbonx"
	fmt "PowerX/pkg/printx"
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
		db = db.Where("store_id = ?", opt.StoreId)
	}
	if !opt.CurrentDate.IsZero() {
		cDate := carbon.FromStdTime(opt.CurrentDate)
		startDate, endDate := carbonx.GetWeekDaysFromDay(&cDate, nil)
		db = db.Where("start_time > ? AND end_time < ?", startDate.String(), endDate.String())
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

func (uc *ScheduleUseCase) FindManySchedules(ctx context.Context, opt *FindManySchedulesOption) (pageList types.Page[*reservationcenter.Schedule], err error) {
	var schedules []*reservationcenter.Schedule
	db := uc.db.WithContext(ctx).Model(&reservationcenter.Schedule{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	if err := db.Find(&schedules).Error; err != nil {
		panic(err)
	}

	return types.Page[*reservationcenter.Schedule]{
		List:      schedules,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
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
	if err := uc.db.WithContext(ctx).
		Preload("Store").
		First(&lead, id).Error; err != nil {
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

func (uc *ScheduleUseCase) CalculateScheduleAvailable(
	ctx context.Context,
	schedule *reservationcenter.Schedule,
	artisan *product.Artisan,
	serviceSpecific *product2.ServiceSpecific,
) bool {

	operationStatus := []int{
		schedule.GetCachedDDId(uc.db.WithContext(ctx), reservationcenter.OperationStatusType, reservationcenter.OperationStatusNone),
		schedule.GetCachedDDId(uc.db.WithContext(ctx), reservationcenter.OperationStatusType, reservationcenter.OperationStatusSuccess),
	}
	reservationStatus := []int{
		schedule.GetCachedDDId(uc.db.WithContext(ctx), reservationcenter.ReservationStatusType, reservationcenter.ReservationStatusConfirmed),
	}

	// 加载已经预约，正在服务的约单列表
	schedule.LoadReservations(uc.db.WithContext(ctx), &map[string]interface{}{
		"operation_status":   operationStatus,
		"reservation_status": reservationStatus,
	}, false)
	fmt.Dump(schedule.Reservations)
	// 计算当前时间距离schedule的结束时间

	// 计算当前服务的必须时间是否在剩余时间之内

	return false
}
