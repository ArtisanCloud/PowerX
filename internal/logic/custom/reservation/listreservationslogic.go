package reservation

import (
	"PowerX/internal/uc/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListReservationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListReservationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListReservationsLogic {
	return &ListReservationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListReservationsLogic) ListReservations(req *types.ListReservationsRequest) (resp *types.ListReservationsReply, err error) {
	reservations, err := l.svcCtx.Custom.Reservation.FindAllReservations(l.ctx, &reservationcenter.FindManyReservationsOption{
		ScheduleId: req.ScheduleId,
	})

	if err != nil {
		return nil, err
	}

	return &types.ListReservationsReply{
		List: TransformReservationsToReservationsReply(reservations),
	}, nil

}
