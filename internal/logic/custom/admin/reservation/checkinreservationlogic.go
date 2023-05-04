package reservation

import (
	"PowerX/internal/model/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CheckInReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCheckInReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckInReservationLogic {
	return &CheckInReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CheckInReservationLogic) CheckInReservation(req *types.CheckInReservationRequest) (resp *types.CheckInReservationReply, err error) {
	ddOpsCancelledId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, reservationcenter.OperationStatusType, reservationcenter.OperationStatusCheckIn)

	mdlReservation := &reservationcenter.Reservation{
		OperationStatus: ddOpsCancelledId,
	}

	// 更新产品对象
	l.svcCtx.Custom.Reservation.PatchReservation(l.ctx, req.ReservationId, mdlReservation)

	return &types.CheckInReservationReply{
		Reservation: &types.Reservation{
			Id:              req.ReservationId,
			OperationStatus: mdlReservation.OperationStatus,
		},
	}, err

}
