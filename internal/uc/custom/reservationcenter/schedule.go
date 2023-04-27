package reservationcenter

import (
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx"
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

func (uc *ScheduleUseCase) MakeAppointment(ctx context.Context, req *AppointmentRequest) (reservation *reservationcenter.Reservation, err error) {

	err = uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		isAvailable, usedTimeResource := uc.IsScheduleAvailable(tx, req.Schedule, req.Artisan, req.ServiceSpecific)
		//fmt.Dump(isAvailable, usedTimeResource)
		if isAvailable {
			ucDD := powerx.NewDataDictionaryUseCase(tx)

			// bucket的开始时间为基准，加上占用的分钟数，为该预约记录的预约时间
			reservedTime := carbon.Time2Carbon(req.Schedule.StartTime).AddMinutes(usedTimeResource)
			//fmt.Dump(reservedTime)
			operationStatus := ucDD.GetCachedDD(ctx, reservationcenter.OperationStatusType, reservationcenter.OperationStatusNone)
			reservationStatus := ucDD.GetCachedDD(ctx, reservationcenter.ReservationStatusType, reservationcenter.ReservationStatusConfirmed)

			// 可以建立预约记录
			reservation = &reservationcenter.Reservation{
				ScheduleId:        req.Req.ScheduleId,
				CustomerId:        req.Req.CustomerId,
				ReservedArtisanId: req.Req.ReservedArtisanId,
				ServiceId:         req.Req.ServiceId,
				ServiceDuration:   req.ServiceSpecific.MandatoryDuration,
				SourceChannelId:   req.Req.SourceChannelId,
				Type:              req.Req.Type,
				ReservedTime:      reservedTime.ToStdTime(),
				Description:       req.Req.Description,
				ConsumedPoints:    req.Req.ConsumedPoints,
				OperationStatus:   operationStatus,
				ReservationStatus: reservationStatus,
			}

			ucReservation := NewReservationUseCase(tx)
			// 创建预约记录
			err = ucReservation.CreateReservation(ctx, reservation)
			if err != nil {
				return err
			} else {
				return nil
			}
		} else {
			_, _ = uc.SavePivotScheduleToArtisan(ctx, req.Req.ScheduleId, req.Req.ReservedArtisanId, false)

			return errors.New("当前预约请求无效")
		}

	})

	return reservation, err
}

func (uc *ScheduleUseCase) IsScheduleAvailable(
	db *gorm.DB,
	schedule *reservationcenter.Schedule,
	artisan *product.Artisan,
	serviceSpecific *product2.ServiceSpecific,
) (isAvailable bool, usedTimeResource int) {

	if db == nil {
		db = uc.db
	}

	operationStatus := []int{
		// 操作状态是正常
		schedule.GetCachedDDId(db, reservationcenter.OperationStatusType, reservationcenter.OperationStatusNone),
		// 操作正常已经是签到
		schedule.GetCachedDDId(db, reservationcenter.OperationStatusType, reservationcenter.OperationStatusCheckIn),
	}
	reservationStatus := []int{
		// 预约状态是已经预约成功
		schedule.GetCachedDDId(db, reservationcenter.ReservationStatusType, reservationcenter.ReservationStatusConfirmed),
	}

	// 加载该发型师的已经预约，正在服务的约单列表
	schedule.LoadReservations(db, &map[string]interface{}{
		"operation_status":    operationStatus,
		"reservation_status":  reservationStatus,
		"reserved_artisan_id": artisan.Id,
	}, false)
	//fmt.Dump(schedule.Reservations)
	if len(schedule.Reservations) == 0 {
		return true, 0
	}

	// 计算当前时间距离schedule的结束时间
	usedTimeResource, remainedTimeResource := uc.CalTimeResource(schedule.Reservations)

	// 计算当前服务的必须时间是否在剩余时间之内
	diffTimeResource := remainedTimeResource - serviceSpecific.MandatoryDuration

	return diffTimeResource >= 0, usedTimeResource
}

func (uc *ScheduleUseCase) CalTimeResource(reservations []*reservationcenter.Reservation) (usedTimeResource int, remainedTimeResource int) {

	usedTimeResource = uc.CalUsedTimeResource(reservations)
	totalTimeResource := reservationcenter.BucketHours * 60

	remainedTimeResource = totalTimeResource - usedTimeResource
	return
}

func (uc *ScheduleUseCase) CalUsedTimeResource(reservations []*reservationcenter.Reservation) int {
	var usedTimeResource int
	for _, reservation := range reservations {
		usedTimeResource = usedTimeResource + reservation.ServiceDuration
	}

	return usedTimeResource
}

func (uc *ScheduleUseCase) SavePivotScheduleToArtisan(ctx context.Context, scheduleId int64, artisanId int64, available bool) (*reservationcenter.PivotScheduleToArtisan, error) {
	var pivot = &reservationcenter.PivotScheduleToArtisan{
		ScheduleId:  scheduleId,
		ArtisanId:   artisanId,
		IsAvailable: available,
	}
	db := uc.db.WithContext(ctx)

	db = db.Model(&pivot).
		Where("schedule_id", scheduleId).
		Where("artisan_id", artisanId).
		Update("is_available", available)
	err := db.Error
	if err != nil {
		return nil, err
	}

	rows := db.RowsAffected
	fmt.Dump(rows)
	if rows == 0 {
		db.Create(&pivot) // create new record from newUser
	}
	return pivot, nil

}

func (uc *ScheduleUseCase) LoadPivotScheduleToArtisan(ctx context.Context, scheduleId int64, artisanId int64) (*reservationcenter.PivotScheduleToArtisan, error) {
	var pivot reservationcenter.PivotScheduleToArtisan
	if err := uc.db.WithContext(ctx).
		Where("schedule_id", scheduleId).
		Where("artisan_id", artisanId).
		First(&pivot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrNotFoundObject, "未找到Pivot")
		}
		panic(err)
	}
	return &pivot, nil
}
