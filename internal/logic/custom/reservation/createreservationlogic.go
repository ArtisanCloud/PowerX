package reservation

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateReservationLogic {
	return &CreateReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateReservationLogic) CreateReservation(req *types.CreateReservationRequest) (resp *types.CreateReservationReply, err error) {

	reservation := reservationcenter.Reservation{
		ScheduleId:        req.ScheduleId,
		CustomerId:        req.CustomerId,
		ReservedArtisanId: req.ReservedArtisanId,
		ServiceId:         req.ServiceId,
		SourceChannelId:   req.SourceChannelId,
		Type:              req.Type,
		Description:       req.Description,
		ConsumedPoints:    req.ConsumedPoints,
		OperationStatus:   l.svcCtx.PowerX.DataDictionary.GetCachedDD(l.ctx, reservationcenter.OperationStatusType, reservationcenter.OperationStatusNone),
	}

	if err := l.svcCtx.Custom.Reservation.CreateReservation(l.ctx, &reservation); err != nil {
		return nil, err
	}

	return &types.CreateReservationReply{
		ReservationKey: reservation.Id,
	}, nil

}
