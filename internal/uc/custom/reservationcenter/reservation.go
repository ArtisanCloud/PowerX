package reservationcenter

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ReservationUseCase struct {
	db *gorm.DB
}

func NewReservationUseCase(db *gorm.DB) *ReservationUseCase {
	return &ReservationUseCase{
		db: db,
	}
}

func (uc *ReservationUseCase) CreateReservation(ctx context.Context, lead *reservationcenter.Reservation) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ReservationUseCase) UpsertReservation(ctx context.Context, lead *reservationcenter.Reservation) (*reservationcenter.Reservation, error) {

	leads := []*reservationcenter.Reservation{lead}

	_, err := uc.UpsertReservations(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *ReservationUseCase) UpsertReservations(ctx context.Context, leads []*reservationcenter.Reservation) ([]*reservationcenter.Reservation, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &reservationcenter.Reservation{}, reservationcenter.ReservationUniqueId, leads, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *ReservationUseCase) PatchReservation(ctx context.Context, id int64, lead *reservationcenter.Reservation) {
	if err := uc.db.WithContext(ctx).Model(&reservationcenter.Reservation{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ReservationUseCase) GetReservation(ctx context.Context, id int64) (*reservationcenter.Reservation, error) {
	var lead reservationcenter.Reservation
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
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
