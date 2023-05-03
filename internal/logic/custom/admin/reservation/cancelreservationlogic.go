package reservation

import (
	"PowerX/internal/model/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CancelReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCancelReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelReservationLogic {
	return &CancelReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CancelReservationLogic) CancelReservation(req *types.CancelReservationRequest) (resp *types.CancelReservationReply, err error) {

	ddOpsCancelledId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, reservationcenter.OperationStatusType, reservationcenter.OperationStatusCancelled)
	ddRsvCancelledId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, reservationcenter.ReservationStatusType, reservationcenter.ReservationStatusCancelled)

	mdlReservation := &reservationcenter.Reservation{
		OperationStatus:   ddOpsCancelledId,
		ReservationStatus: ddRsvCancelledId,
	}

	// 更新产品对象
	l.svcCtx.Custom.Reservation.PatchReservation(l.ctx, req.ReservationId, mdlReservation)

	return &types.CancelReservationReply{
		Reservation: &types.Reservation{
			Id:                req.ReservationId,
			OperationStatus:   mdlReservation.OperationStatus,
			ReservationStatus: mdlReservation.ReservationStatus,
		},
	}, err

}
