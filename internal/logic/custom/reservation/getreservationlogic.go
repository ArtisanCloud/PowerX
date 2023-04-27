package reservation

import (
	reservationcenter2 "PowerX/internal/model/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReservationLogic {
	return &GetReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetReservationLogic) GetReservation(req *types.GetReservationRequest) (resp *types.GetReservationReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformReservationToReservationReply(reservation *reservationcenter2.Reservation) *types.Reservation {
	return &types.Reservation{
		Id:                  reservation.Id,
		ScheduleId:          reservation.ScheduleId,
		CustomerId:          reservation.CustomerId,
		SourceChannelId:     reservation.SourceChannelId,
		ReservedArtisanId:   reservation.ReservedArtisanId,
		Type:                reservation.Type,
		ReservedTime:        reservation.ReservedTime.String(),
		CancelTime:          reservation.CancelTime.String(),
		CheckinTime:         reservation.CheckinTime.String(),
		Description:         reservation.Description,
		ConsumedPoints:      reservation.ConsumedPoints,
		ConsumeMembershipId: reservation.ConsumeMembershipId,
		OperationStatus:     reservation.OperationStatus,
		ReservationStatus:   reservation.ReservationStatus,
		CreatedAt:           reservation.CreatedAt.String(),
	}

}
