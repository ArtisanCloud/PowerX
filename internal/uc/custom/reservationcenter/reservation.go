package reservationcenter

import (
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type ReservationUseCase struct {
	db *gorm.DB
}

type AppointmentRequest struct {
	Schedule        *reservationcenter.Schedule
	Artisan         *product.Artisan
	Customer        *customerdomain.Customer
	ServiceSpecific *product2.ServiceSpecific
	Req             *types.CreateReservationRequest
}

type FindManyReservationsOption struct {
	ScheduleId        int64
	OperationStatus   []int
	ReservationType   []int
	ReservationStatus []int
	OrderBy           string
	types.PageEmbedOption
}

func NewReservationUseCase(db *gorm.DB) *ReservationUseCase {
	return &ReservationUseCase{
		db: db,
	}
}

func (uc *ReservationUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyReservationsOption) *gorm.DB {
	if len(opt.OperationStatus) > 0 {
		db = db.Where("operation_status IN ?", opt.OperationStatus)
	}
	if len(opt.ReservationType) > 0 {
		db = db.Where("type IN ?", opt.ReservationType)
	}
	if len(opt.ReservationStatus) > 0 {
		db = db.Where("reservation_status IN ?", opt.ReservationStatus)
	}
	if opt.ScheduleId > 0 {
		db = db.Where("schedule_id = ?", opt.ScheduleId)
	}

	orderBy := "id asc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}

	return db
}

func (uc *ReservationUseCase) FindAllReservations(ctx context.Context, opt *FindManyReservationsOption) (schedules []*reservationcenter.Reservation, err error) {
	query := uc.db.WithContext(ctx).Model(&reservationcenter.Reservation{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		Debug().
		Preload("Customer").
		Preload("Artisan").
		Preload("Service").
		//Preload("CheckinLogs").
		Find(&schedules).Error; err != nil {
		panic(errors.Wrap(err, "find all schedules failed"))
	}
	return schedules, err
}

func (uc *ReservationUseCase) FindManyReservations(ctx context.Context, opt *FindManyReservationsOption) (pageList types.Page[*reservationcenter.Reservation], err error) {
	var schedules []*reservationcenter.Reservation
	db := uc.db.WithContext(ctx).Model(&reservationcenter.Reservation{})

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

	return types.Page[*reservationcenter.Reservation]{
		List:      schedules,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *ReservationUseCase) CreateReservation(ctx context.Context, reservation *reservationcenter.Reservation) error {
	if err := uc.db.WithContext(ctx).Create(&reservation).Error; err != nil {
		// todo use errors.Is() when gorm update ErrDuplicatedKey
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrBadRequest, "约单记录已存在")
		}
		panic(err)
	}
	return nil
}

func (uc *ReservationUseCase) UpsertReservation(ctx context.Context, reservation *reservationcenter.Reservation) (*reservationcenter.Reservation, error) {

	reservations := []*reservationcenter.Reservation{reservation}

	_, err := uc.UpsertReservations(ctx, reservations)
	if err != nil {
		panic(errors.Wrap(err, "upsert reservation failed"))
	}

	return reservation, err
}

func (uc *ReservationUseCase) UpsertReservations(ctx context.Context, reservations []*reservationcenter.Reservation) ([]*reservationcenter.Reservation, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &reservationcenter.Reservation{}, reservationcenter.ReservationUniqueId, reservations, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert reservations failed"))
	}

	return reservations, err
}

func (uc *ReservationUseCase) PatchReservation(ctx context.Context, id int64, reservation *reservationcenter.Reservation) {
	if err := uc.db.WithContext(ctx).Model(&reservationcenter.Reservation{}).Where(id).Updates(&reservation).Error; err != nil {
		panic(err)
	}
}

func (uc *ReservationUseCase) GetReservation(ctx context.Context, id int64) (*reservationcenter.Reservation, error) {
	var reservation reservationcenter.Reservation
	if err := uc.db.WithContext(ctx).First(&reservation, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &reservation, nil
}

func (uc *ReservationUseCase) DeleteReservation(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&reservationcenter.Reservation{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
