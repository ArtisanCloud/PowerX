package schedule

import (
	"PowerX/internal/model/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetScheduleLogic {
	return &GetScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetScheduleLogic) GetSchedule(req *types.GetScheduleRequest) (resp *types.GetScheduleReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformScheduleToScheduleReply(schedule *reservationcenter.Schedule) *types.Schedule {

	return &types.Schedule{
		Id:                     schedule.Id,
		StoreId:                schedule.StoreId,
		ApprovalStatus:         schedule.ApprovalStatus,
		Capacity:               schedule.Capacity,
		CopyFromScheduleId:     schedule.CopyFromScheduleId,
		Name:                   schedule.Name,
		Description:            schedule.Description,
		IsActive:               schedule.IsActive,
		Status:                 schedule.Status,
		StartTime:              schedule.StartTime.String(),
		EndTime:                schedule.EndTime.String(),
		CreatedAt:              schedule.CreatedAt.String(),
		Reservations:           TransformReservationsToScheduleReservationsReply(schedule.Reservations),
		PivotScheduleToArtisan: TransformPivotsArtisanAvailableReply(schedule.PivotScheduleToArtisan),
	}
}

func TransformReservationsToScheduleReservationsReply(reservations []*reservationcenter.Reservation) []*types.ScheduleReservation {

	list := make([]*types.ScheduleReservation, 0, len(reservations))
	for _, item := range reservations {
		list = append(list, TransformReservationToScheduleReservationReply(item))
	}

	return list
}

func TransformReservationToScheduleReservationReply(reservation *reservationcenter.Reservation) *types.ScheduleReservation {
	return &types.ScheduleReservation{
		Id:                reservation.Id,
		CustomerId:        reservation.CustomerId,
		ReservedArtisanId: reservation.ReservedArtisanId,
		ReservedTime:      reservation.ReservedTime.String(),
		CancelTime:        reservation.CancelTime.String(),
		CheckinTime:       reservation.CheckinTime.String(),
		Description:       reservation.Description,
		OperationStatus:   reservation.OperationStatus,
		ReservationStatus: reservation.ReservationStatus,
		CreatedAt:         reservation.CreatedAt.String(),
	}

}

func TransformPivotsArtisanAvailableReply(pivots []*reservationcenter.PivotScheduleToArtisan) []*types.PivotScheduleToArtisan {

	list := make([]*types.PivotScheduleToArtisan, 0, len(pivots))
	for _, item := range pivots {
		list = append(list, TransformPivotArtisanAvailableReply(item))
	}

	return list
}

func TransformPivotArtisanAvailableReply(pivot *reservationcenter.PivotScheduleToArtisan) *types.PivotScheduleToArtisan {
	return &types.PivotScheduleToArtisan{
		ScheduleId:  pivot.ScheduleId,
		ArtisanId:   pivot.ArtisanId,
		IsAvailable: pivot.IsAvailable,
	}

}
