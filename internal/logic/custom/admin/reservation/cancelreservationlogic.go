package reservation

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/customerdomain"
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

	ddCancelled := l.svcCtx.PowerX.DataDictionary.GetCachedDD(reservationcenter.OperationStatusType, reservationcenter.OperationStatusCancelled)

	mdlReservation := &reservationcenter.Reservation{
		OperationStatus:        req.,
		ReservationStatus:        req.,
	}

	// 更新产品对象
	err = l.svcCtx.PowerX.Reservation.UpdateReservation(l.ctx, req.ReservationId, mdlReservation)

	return &types.PatchReservationReply{
		Reservation: TransformReservationToReservationReply(mdlReservation),
	}, err
	
}
